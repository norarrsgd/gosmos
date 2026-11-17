package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type batchFake struct {
	fakeContainer
	calls   int
	failure bool
}

func (f *batchFake) NewTransactionalBatch(pk azcosmos.PartitionKey) azcosmos.TransactionalBatch {
	return azcosmos.TransactionalBatch{}
}
func (f *batchFake) ExecuteTransactionalBatch(context.Context, azcosmos.TransactionalBatch, *azcosmos.TransactionalBatchOptions) (azcosmos.TransactionalBatchResponse, error) {
	f.calls++
	return azcosmos.TransactionalBatchResponse{Success: !f.failure, OperationResults: []azcosmos.TransactionalBatchResult{{StatusCode: 201, ResourceBody: f.body}}}, nil
}
func TestAtomicValidation(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &batchFake{}
	g.container = f
	b := g.Atomic("t")
	for range 101 {
		b.Create(testEntity{Tenant: "t"})
	}
	if _, err := b.Execute(context.Background()); !errors.Is(err, ErrValidation) || f.calls != 0 {
		t.Fatalf("%v calls=%d", err, f.calls)
	}
	b = g.Atomic("t")
	b.Create(testEntity{Tenant: "other"})
	if _, err := b.Execute(context.Background()); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}
func TestAtomicSuccess(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &batchFake{}
	g.container = f
	result, err := g.Atomic("t").Create(testEntity{Tenant: "t"}).Execute(context.Background())
	if err != nil || f.calls != 1 || len(result.Items) != 1 || !result.Items[0].Committed || result.Items[0].DecodeError != nil || result.Items[0].Err != nil {
		t.Fatalf("%+v %v", result, err)
	}
}

type chunkFake struct {
	fakeContainer
	mu                sync.Mutex
	sizes             []int
	active, maxActive int
	status            int32
	cancel            context.CancelFunc
}

func (f *chunkFake) NewTransactionalBatch(pk azcosmos.PartitionKey) azcosmos.TransactionalBatch {
	var c *azcosmos.ContainerClient
	return c.NewTransactionalBatch(pk)
}
func (f *chunkFake) ExecuteTransactionalBatch(ctx context.Context, b azcosmos.TransactionalBatch, _ *azcosmos.TransactionalBatchOptions) (azcosmos.TransactionalBatchResponse, error) {
	count := reflect.ValueOf(b).FieldByName("operations").Len()
	f.mu.Lock()
	f.sizes = append(f.sizes, count)
	f.active++
	f.maxActive = max(f.maxActive, f.active)
	f.mu.Unlock()
	runtime.Gosched()
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	results := make([]azcosmos.TransactionalBatchResult, count)
	for i := range results {
		results[i].StatusCode = 201
		results[i].ResourceBody = f.body
		if f.status != 0 {
			results[i].StatusCode = f.status
		}
	}
	if f.cancel != nil {
		f.cancel()
	}
	return azcosmos.TransactionalBatchResponse{Success: f.status == 0, Response: azcosmos.Response{RequestCharge: 2}, OperationResults: results}, nil
}
func TestBulkChunkingAndPartialCancellation(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &chunkFake{}
	g.container = f
	b := g.Bulk()
	for range 205 {
		b.Create(testEntity{Tenant: "t"})
	}
	result, err := b.Execute(context.Background())
	if err != nil || !reflect.DeepEqual(f.sizes, []int{100, 100, 5}) || len(result.Items) != 205 || result.Gravity.RU != 6 {
		t.Fatalf("chunks=%v result=%+v err=%v", f.sizes, result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f = &chunkFake{cancel: cancel}
	g.container = f
	b = g.Bulk()
	for range 105 {
		b.Create(testEntity{Tenant: "t"})
	}
	result, err = b.Execute(ctx)
	if !errors.Is(err, context.Canceled) || len(f.sizes) != 1 || result.Items[0].Err != nil || !errors.Is(result.Items[100].Err, context.Canceled) {
		t.Fatalf("partial cancellation: calls=%v err=%v items=%+v", f.sizes, err, result.Items)
	}
}
func TestBulkConcurrencyBound(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &chunkFake{}
	g.container = f
	b := g.Bulk()
	for i := range 50 {
		b.Create(testEntity{Tenant: fmt.Sprintf("t%d", i)})
	}
	result, err := b.Execute(context.Background())
	if err != nil || f.maxActive > 4 || len(result.Items) != 50 {
		t.Fatalf("active=%d %v", f.maxActive, err)
	}
}
func TestCommittedBatchDecodeFailureIsSeparated(t *testing.T) {
	const incompatibleBody = `{"tenant":42}`
	t.Run("valid body", func(t *testing.T) {
		g, _ := testGalaxy(t)
		f := &batchFake{}
		f.body = []byte(`{"id":"returned","tenant":"t"}`)
		g.container = f
		result, err := g.Atomic("t").Create(testEntity{Tenant: "t"}).Execute(context.Background())
		if err != nil || len(result.Items) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
		item := result.Items[0]
		if !item.Committed || item.Err != nil || item.DecodeError != nil || item.Item.ID != "returned" {
			t.Fatalf("%+v", item)
		}
	})
	t.Run("atomic", func(t *testing.T) {
		g, _ := testGalaxy(t)
		f := &batchFake{}
		f.body = []byte(incompatibleBody)
		g.container = f
		result, err := g.Atomic("t").Create(testEntity{Tenant: "t"}).Execute(context.Background())
		if err != nil || len(result.Items) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
		item := result.Items[0]
		if !item.Committed || item.Err != nil || item.DecodeError == nil || item.Item.Tenant != "t" {
			t.Fatalf("%+v", item)
		}
	})
	t.Run("bulk continues", func(t *testing.T) {
		g, _ := testGalaxy(t)
		f := &chunkFake{}
		f.body = []byte(incompatibleBody)
		g.container = f
		b := g.Bulk()
		for range 101 {
			b.Create(testEntity{Tenant: "t"})
		}
		result, err := b.Execute(context.Background())
		if err != nil || !reflect.DeepEqual(f.sizes, []int{100, 1}) || len(result.Items) != 101 {
			t.Fatalf("chunks=%v result=%+v err=%v", f.sizes, result, err)
		}
		for i, item := range result.Items {
			if !item.Committed || item.Err != nil || item.DecodeError == nil || item.Item.Tenant != "t" {
				t.Fatalf("item %d: %+v", i, item)
			}
		}
	})
}
func TestBulkAdmissionLimits(t *testing.T) {
	t.Run("operation count", func(t *testing.T) {
		g, _ := testGalaxy(t)
		f := &chunkFake{}
		g.container = f
		b := g.Bulk()
		if b.builder.operationLimit != bulkMaxOperations || b.builder.byteLimit != bulkMaxBytes {
			t.Fatalf("limits=%d/%d", b.builder.operationLimit, b.builder.byteLimit)
		}
		b.builder.operationLimit = 2
		b.Create(testEntity{Tenant: "t"}).Create(testEntity{Tenant: "t"}).Create(testEntity{Tenant: "t"})
		result, err := b.Execute(context.Background())
		if !errors.Is(err, ErrValidation) || len(result.Items) != 2 || len(f.sizes) != 0 {
			t.Fatalf("items=%d calls=%v err=%v", len(result.Items), f.sizes, err)
		}
	})
	t.Run("aggregate bytes", func(t *testing.T) {
		g, _ := testGalaxy(t)
		f := &chunkFake{}
		g.container = f
		b := g.Bulk().Create(testEntity{Tenant: "t"})
		b.builder.byteLimit = 1 + b.builder.estimatedBytes
		b.Create(testEntity{Tenant: "t"})
		result, err := b.Execute(context.Background())
		if !errors.Is(err, ErrValidation) || len(result.Items) != 1 || len(f.sizes) != 0 {
			t.Fatalf("items=%d calls=%v err=%v", len(result.Items), f.sizes, err)
		}
	})
}
func TestBulkCancellationBeforeGrouping(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &chunkFake{}
	g.container = f
	b := g.Bulk()
	for i := range 50 {
		b.Create(testEntity{Tenant: fmt.Sprintf("t%d", i)})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := b.Execute(ctx)
	if !errors.Is(err, context.Canceled) || len(f.sizes) != 0 || len(result.Items) != 50 {
		t.Fatalf("items=%d calls=%v err=%v", len(result.Items), f.sizes, err)
	}
	for i, item := range result.Items {
		if !errors.Is(item.Err, context.Canceled) {
			t.Fatalf("item %d: %v", i, item.Err)
		}
	}
}
func TestBulkCancellationStopsDispatch(t *testing.T) {
	g, _ := testGalaxy(t)
	ctx, cancel := context.WithCancel(context.Background())
	f := &chunkFake{cancel: cancel}
	g.container = f
	b := g.Bulk()
	for i := range 50 {
		b.Create(testEntity{Tenant: fmt.Sprintf("t%d", i)})
	}
	result, err := b.Execute(ctx)
	if !errors.Is(err, context.Canceled) || len(f.sizes) == 0 || len(f.sizes) > 4 {
		t.Fatalf("calls=%v err=%v", f.sizes, err)
	}
	for i, item := range result.Items {
		if item.Status == 0 && !errors.Is(item.Err, context.Canceled) {
			t.Fatalf("unattempted item %d: %v", i, item.Err)
		}
	}
}
func TestPartitionIdentitySeparatesComponents(t *testing.T) {
	if partitionIdentity([]string{"ab", "c"}) == partitionIdentity([]string{"a", "bc"}) {
		t.Fatal("component boundaries collided")
	}
	if partitionIdentity([]string{string([]byte{0xff})}) == partitionIdentity([]string{"�"}) {
		t.Fatal("invalid UTF-8 collided")
	}
	if partitionIdentity([]string{"tenant", "region"}) != partitionIdentity([]string{"tenant", "region"}) {
		t.Fatal("equal components produced distinct identities")
	}
}
func TestBatchFailuresAndSize(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &chunkFake{status: 409}
	g.container = f
	result, err := g.Atomic("t").Read("id").Execute(context.Background())
	if !errors.Is(err, ErrConflict) || !errors.Is(result.Items[0].Err, ErrConflict) {
		t.Fatalf("%v %+v", err, result)
	}
	rolledBack := &batchFake{failure: true}
	g.container = rolledBack
	result, err = g.Atomic("t").Read("id").Execute(context.Background())
	if err == nil || result.Items[0].Committed || result.Items[0].DecodeError != nil || result.Items[0].Err == nil {
		t.Fatalf("rollback: %v %+v", err, result)
	}
	b := g.Atomic("t")
	for range 2 {
		b.Create(testEntity{Base: Base{ID: strings.Repeat("a", 1024*1024)}, Tenant: "t"})
	}
	before := len(f.sizes)
	if _, err = b.Execute(context.Background()); !errors.Is(err, ErrValidation) || len(f.sizes) != before {
		t.Fatalf("oversize: %v", err)
	}
}
func TestBatchAllOperations(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &chunkFake{}
	g.container = f
	b := g.Atomic("t").Create(testEntity{Tenant: "t"}).Replace(testEntity{Base: Base{ID: "replace"}, Tenant: "t"}, IfMatch("etag")).Upsert(testEntity{Tenant: "t"}).Remove("remove", IfMatch("etag")).Read("read").Patch("patch", func(p *Patch[testEntity]) { p.Set("tenant", "t") })
	result, err := b.Execute(context.Background())
	if err != nil || len(result.Items) != 6 {
		t.Fatalf("%v %+v", err, result)
	}
}

func TestPreflightErrorsMarkEveryIncludedItem(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &batchFake{}
	g.container = f
	assertRejected := func(t *testing.T, result BatchResult[testEntity], err error, want int) {
		t.Helper()
		if !errors.Is(err, ErrValidation) || len(result.Items) != want {
			t.Fatalf("items=%d err=%v", len(result.Items), err)
		}
		for i, item := range result.Items {
			if item.Err != err || item.Status != 0 {
				t.Fatalf("item %d: status=%d err=%v, overall %v", i, item.Status, item.Err, err)
			}
		}
		if f.calls != 0 {
			t.Fatal("preflight reached network")
		}
	}
	t.Run("sticky atomic", func(t *testing.T) {
		b := g.Atomic("t").Create(testEntity{Tenant: "t"}).Create(testEntity{Tenant: "other"})
		result, err := b.Execute(context.Background())
		assertRejected(t, result, err, 1)
	})
	t.Run("sticky bulk", func(t *testing.T) {
		b := g.Bulk().Create(testEntity{Tenant: "t"}).Read("invalid/id", []string{"t"})
		result, err := b.Execute(context.Background())
		assertRejected(t, result, err, 1)
	})
	t.Run("operation count", func(t *testing.T) {
		b := g.Atomic("t")
		for range 101 {
			b.Create(testEntity{Tenant: "t"})
		}
		result, err := b.Execute(context.Background())
		assertRejected(t, result, err, 101)
	})
	t.Run("payload limit", func(t *testing.T) {
		b := g.Atomic("t")
		for range 2 {
			b.Create(testEntity{Base: Base{ID: strings.Repeat("a", 1024*1024)}, Tenant: "t"})
		}
		result, err := b.Execute(context.Background())
		assertRejected(t, result, err, 2)
	})
}
