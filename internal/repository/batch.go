package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

const batchMaxBytes = 2 * 1024 * 1024

// BatchItemResult preserves the original operation order, including unattempted items.
type BatchItemResult[T Entity] struct {
	Index  int
	Item   T
	Status int
	ETag   string
	Err    error
}

type BatchResult[T Entity] struct {
	Items   []BatchItemResult[T]
	Gravity Gravity
}

func (r BatchResult[T]) rejected(err error) (BatchResult[T], error) {
	for i := range r.Items {
		r.Items[i].Err = err
	}
	return r, err
}

type batchOperation[T Entity] struct {
	kind, id string
	key      azcosmos.PartitionKey
	body     []byte
	patch    azcosmos.PatchOperations
	etag     *azcore.ETag
	item     T
	size     int
}

type batchContainer interface {
	NewTransactionalBatch(azcosmos.PartitionKey) azcosmos.TransactionalBatch
	ExecuteTransactionalBatch(context.Context, azcosmos.TransactionalBatch, *azcosmos.TransactionalBatchOptions) (azcosmos.TransactionalBatchResponse, error)
}

type batchBuilder[T Entity] struct {
	galaxy     *Galaxy[T]
	operations []batchOperation[T]
	key        *azcosmos.PartitionKey
	err        error
}

func (b *batchBuilder[T]) model(kind string, item T, options []WriteOption) {
	if b.err != nil {
		return
	}
	copy, v, err := entityCopy(item)
	if err != nil {
		b.err = err
		return
	}
	mutable := capability(v)
	if strings.TrimSpace(copy.GetID()) == "" {
		if kind == "replace" {
			b.err = validation("replace requires document ID")
			return
		}
		setter, ok := mutable.(IDSetter)
		if !ok {
			b.err = validation("blank ID requires IDSetter")
			return
		}
		id, err := newUUIDv7()
		if err != nil {
			b.err = err
			return
		}
		setter.SetID(id)
	}
	if stamper, ok := mutable.(Stamper); ok {
		if kind == "create" {
			stamper.StampCreate(time.Now().UTC())
		} else {
			stamper.StampModify(time.Now().UTC())
		}
	}
	if v.Type() == reflect.TypeFor[T]() {
		copy = v.Interface().(T)
	}
	key, err := b.galaxy.metadata.key(v)
	if err != nil {
		b.err = err
		return
	}
	body, err := json.Marshal(copy)
	if err != nil {
		b.err = err
		return
	}
	b.add(batchOperation[T]{kind: kind, id: copy.GetID(), key: key, body: body, item: copy}, options)
}

func (b *batchBuilder[T]) add(operation batchOperation[T], options []WriteOption) {
	if b.err != nil {
		return
	}
	if strings.TrimSpace(operation.id) == "" || strings.ContainsAny(operation.id, "\"\\/\t\r\n?#") {
		b.err = validation("invalid batch document ID")
		return
	}
	for _, character := range operation.id {
		if character < 32 {
			b.err = validation("batch document ID contains a control character")
			return
		}
	}
	if b.key != nil && !reflect.DeepEqual(operation.key, *b.key) {
		b.err = validation("atomic operations must use the same partition key")
		return
	}
	o, err := writeOptions(options)
	if err != nil {
		b.err = err
		return
	}
	if (operation.kind == "create" || operation.kind == "read") && o.IfMatchEtag != nil {
		b.err = validation("ETag is unsupported for batch %s", operation.kind)
		return
	}
	operation.etag = o.IfMatchEtag
	// Include SDK operation envelopes and ETags, not just document body bytes.
	envelope := map[string]any{"operationType": strings.ToUpper(operation.kind[:1]) + operation.kind[1:]}
	if operation.kind != "create" && operation.kind != "upsert" {
		envelope["id"] = operation.id
	}
	if operation.etag != nil {
		envelope["ifMatch"] = operation.etag
	}
	if len(operation.body) > 0 {
		envelope["resourceBody"] = json.RawMessage(operation.body)
	}
	if operation.kind == "patch" {
		envelope["resourceBody"] = operation.patch
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		b.err = err
		return
	}
	operation.size = len(encoded) + 1
	if operation.size+1 > batchMaxBytes {
		b.err = validation("batch operation exceeds 2 MiB")
		return
	}
	b.operations = append(b.operations, operation)
}

func (b *batchBuilder[T]) point(kind, id string, pk []string, patch func(*Patch[T]), options []WriteOption) {
	if b.err != nil {
		return
	}
	var key azcosmos.PartitionKey
	var err error
	if b.key != nil {
		key = *b.key
	} else {
		key, err = stringKey(pk, len(b.galaxy.metadata.fields))
		if err != nil {
			b.err = err
			return
		}
	}
	operation := batchOperation[T]{kind: kind, id: id, key: key}
	if kind == "patch" {
		if patch == nil {
			b.err = validation("patch callback is required")
			return
		}
		p := &Patch[T]{}
		patch(p)
		operation.patch, err = p.Build()
		if err != nil {
			b.err = err
			return
		}
	}
	b.add(operation, options)
}

func (b *batchBuilder[T]) initial() (BatchResult[T], error) {
	result := BatchResult[T]{Items: make([]BatchItemResult[T], len(b.operations))}
	for i, operation := range b.operations {
		result.Items[i] = BatchItemResult[T]{Index: i, Item: operation.item}
	}
	if b.err != nil {
		return result.rejected(b.galaxy.failure("batch", b.err))
	}
	if len(b.operations) == 0 {
		return result, b.galaxy.failure("batch", validation("batch is empty"))
	}
	return result, nil
}

func (b *batchBuilder[T]) executeChunk(ctx context.Context, indexes []int, result *BatchResult[T]) (float64, error) {
	fail := func(err error) (float64, error) {
		for _, index := range indexes {
			result.Items[index].Err = err
		}
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	container, ok := b.galaxy.container.(batchContainer)
	if !ok {
		return fail(b.galaxy.failure("batch", validation("container does not support batches")))
	}
	batch := container.NewTransactionalBatch(b.operations[indexes[0]].key)
	for _, index := range indexes {
		operation := b.operations[index]
		options := &azcosmos.TransactionalBatchItemOptions{IfMatchETag: operation.etag}
		switch operation.kind {
		case "create":
			batch.CreateItem(operation.body, options)
		case "replace":
			batch.ReplaceItem(operation.id, operation.body, options)
		case "upsert":
			batch.UpsertItem(operation.body, options)
		case "delete":
			batch.DeleteItem(operation.id, options)
		case "read":
			batch.ReadItem(operation.id, options)
		case "patch":
			batch.PatchItem(operation.id, operation.patch, options)
		}
	}
	response, err := container.ExecuteTransactionalBatch(ctx, batch, &azcosmos.TransactionalBatchOptions{})
	// A transport failure can conceal a commit; clear cached reads conservatively.
	if b.galaxy.options.cache != nil {
		b.galaxy.options.cache.Clear()
	}
	if err != nil {
		_, wrapped := fail(b.galaxy.failure("batch", err))
		return float64(response.RequestCharge), wrapped
	}
	if len(response.OperationResults) != len(indexes) {
		return fail(b.galaxy.failure("batch", fmt.Errorf("batch returned %d results for %d operations", len(response.OperationResults), len(indexes))))
	}
	var all []error
	for i, index := range indexes {
		item := &result.Items[index]
		sdk := response.OperationResults[i]
		item.Status = int(sdk.StatusCode)
		item.ETag = string(sdk.ETag)
		if sdk.StatusCode < 200 || sdk.StatusCode >= 300 {
			item.Err = b.galaxy.failure("batch", &azcore.ResponseError{StatusCode: int(sdk.StatusCode)})
		} else if !response.Success {
			item.Err = b.galaxy.failure("batch", fmt.Errorf("transaction was rolled back"))
		} else if len(sdk.ResourceBody) > 0 {
			item.Err = json.Unmarshal(sdk.ResourceBody, &item.Item)
		}
		if item.Err != nil {
			all = append(all, item.Err)
		}
	}
	return float64(response.RequestCharge), errors.Join(all...)
}
