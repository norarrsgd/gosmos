package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"reflect"
	"strings"
	"time"
)

func (g *Galaxy[T]) failure(op string, err error) error { return wrapError(op, g.metadata.name, err) }
func gravity(response azcosmos.ItemResponse) Gravity {
	return Gravity{RU: float64(response.RequestCharge), ETag: string(response.ETag)}
}
func writeOptions(options []WriteOption) (*azcosmos.ItemOptions, error) {
	o := &azcosmos.ItemOptions{}
	for _, option := range options {
		if option == nil {
			return nil, validation("nil write option")
		}
		if err := option(o); err != nil {
			return nil, err
		}
	}
	return o, nil
}

// entityCopy returns an addressable top-level copy so optional pointer setters work
// without changing the caller's document.
func entityCopy[T Entity](item T) (T, reflect.Value, error) {
	var zero T
	v := reflect.ValueOf(item)
	if !v.IsValid() || (v.Kind() == reflect.Pointer && v.IsNil()) {
		return zero, reflect.Value{}, validation("entity is nil")
	}
	if v.Kind() == reflect.Pointer {
		copy := reflect.New(v.Type().Elem())
		copy.Elem().Set(v.Elem())
		return copy.Interface().(T), copy.Elem(), nil
	}
	copy := reflect.New(v.Type()).Elem()
	copy.Set(v)
	return copy.Interface().(T), copy, nil
}
func capability(v reflect.Value) any { return v.Addr().Interface() }
func (g *Galaxy[T]) write(ctx context.Context, item T, replace bool, options []WriteOption) (T, Gravity, error) {
	op := "create"
	if replace {
		op = "modify"
	}
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, Gravity{}, g.failure(op, err)
	}
	copy, v, err := entityCopy(item)
	if err != nil {
		return zero, Gravity{}, g.failure(op, err)
	}
	mutable := capability(v)
	if strings.TrimSpace(copy.GetID()) == "" {
		if replace {
			return zero, Gravity{}, g.failure(op, validation("document ID is required"))
		}
		setter, ok := mutable.(IDSetter)
		if !ok {
			return zero, Gravity{}, g.failure(op, validation("blank ID requires IDSetter"))
		}
		id, err := newUUIDv7()
		if err != nil {
			return zero, Gravity{}, g.failure(op, err)
		}
		setter.SetID(id)
	}
	if stamper, ok := mutable.(Stamper); ok {
		now := time.Now().UTC()
		if replace {
			stamper.StampModify(now)
		} else {
			stamper.StampCreate(now)
		}
	}
	if v.Type() == reflect.TypeFor[T]() {
		copy = v.Interface().(T)
	}
	key, err := g.metadata.key(v)
	if err != nil {
		return zero, Gravity{}, g.failure(op, err)
	}
	o, err := writeOptions(options)
	if err != nil {
		return zero, Gravity{}, g.failure(op, err)
	}
	body, err := json.Marshal(copy)
	if err != nil {
		return zero, Gravity{}, g.failure(op, err)
	}
	var response azcosmos.ItemResponse
	if replace {
		response, err = g.container.ReplaceItem(ctx, key, copy.GetID(), body, o)
	} else {
		response, err = g.container.CreateItem(ctx, key, body, o)
	}
	if g.options.cache != nil {
		g.options.cache.Clear()
	}
	if err != nil {
		return zero, gravity(response), g.failure(op, err)
	}
	return copy, gravity(response), nil
}

// Create assigns an ID and creation timestamp when the entity supports them.
// The returned copy contains generated values; the input is not modified.
func (g *Galaxy[T]) Create(ctx context.Context, item T, options ...WriteOption) (T, Gravity, error) {
	return g.write(ctx, item, false, options)
}

// Modify replaces the entire document and stamps its modification time.
func (g *Galaxy[T]) Modify(ctx context.Context, item T, options ...WriteOption) (T, Gravity, error) {
	return g.write(ctx, item, true, options)
}

// Get reads one document by its ID and complete partition key.
func (g *Galaxy[T]) Get(ctx context.Context, id string, pk []string, options ...ReadOptions) (T, Gravity, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, Gravity{}, g.failure("get", err)
	}
	if strings.TrimSpace(id) == "" {
		return zero, Gravity{}, g.failure("get", validation("document ID is required"))
	}
	if len(options) > 1 {
		return zero, Gravity{}, g.failure("get", validation("only one ReadOptions value is allowed"))
	}
	level := g.options.consistency
	if len(options) == 1 && options[0].ConsistencyLevel != nil {
		level = options[0].ConsistencyLevel
	}
	level, err := copyConsistency(level)
	if err != nil {
		return zero, Gravity{}, g.failure("get", err)
	}
	key, err := stringKey(pk, len(g.metadata.fields))
	if err != nil {
		return zero, Gravity{}, g.failure("get", err)
	}
	cacheInput, _ := json.Marshal([]any{id, pk})
	cacheKey := fmt.Sprintf("%x", sha256.Sum256(cacheInput))
	var cacheVersion uint64
	if level == nil && g.options.cache != nil {
		cacheVersion = g.options.cache.Version()
		if b, ok := g.options.cache.Get(cacheKey); ok {
			var cached struct {
				Item T
				ETag string
			}
			if json.Unmarshal(b, &cached) == nil {
				return cached.Item, Gravity{ETag: cached.ETag}, nil
			}
			g.options.cache.Delete(cacheKey)
		}
	}
	response, err := g.container.ReadItem(ctx, key, id, &azcosmos.ItemOptions{ConsistencyLevel: level})
	if err != nil {
		return zero, gravity(response), g.failure("get", err)
	}
	var item T
	if err = json.Unmarshal(response.Value, &item); err != nil {
		return zero, gravity(response), g.failure("get", err)
	}
	if reflect.ValueOf(item).Kind() == reflect.Pointer && reflect.ValueOf(item).IsNil() {
		return zero, gravity(response), g.failure("get", validation("service returned a null document"))
	}
	if level == nil && g.options.cache != nil {
		b, marshalErr := json.Marshal(struct {
			Item T
			ETag string
		}{item, string(response.ETag)})
		if marshalErr == nil {
			g.options.cache.PutIfVersion(cacheKey, b, cacheVersion)
		}
	}
	return item, gravity(response), nil
}

// Remove deletes one document, optionally guarded by an ETag.
func (g *Galaxy[T]) Remove(ctx context.Context, id string, pk []string, options ...WriteOption) (Gravity, error) {
	if err := ctx.Err(); err != nil {
		return Gravity{}, g.failure("remove", err)
	}
	if strings.TrimSpace(id) == "" {
		return Gravity{}, g.failure("remove", validation("document ID is required"))
	}
	key, err := stringKey(pk, len(g.metadata.fields))
	if err != nil {
		return Gravity{}, g.failure("remove", err)
	}
	o, err := writeOptions(options)
	if err != nil {
		return Gravity{}, g.failure("remove", err)
	}
	response, err := g.container.DeleteItem(ctx, key, id, o)
	if g.options.cache != nil {
		g.options.cache.Clear()
	}
	return gravity(response), g.failure("remove", err)
}
