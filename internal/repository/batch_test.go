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
	return azcosmos.TransactionalBatchResponse{Success: !f.failure, OperationResults: []azcosmos.TransactionalBatchResult{{StatusCode: 201}}}, nil
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
	if err != nil || f.calls != 1 || len(result.Items) != 1 || result.Items[0].Err != nil {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestBatchRejectsDivergentSerializedID(t *testing.T) {
	m, err := inspectEntity[shadowIDEntity]()
	if err != nil {
		t.Fatal(err)
	}
	f := &batchFake{}
	g := &Galaxy[shadowIDEntity]{container: f, metadata: m}
	item := shadowIDEntity{Base: Base{ID: "trusted"}, BodyID: "attacker", Tenant: "t"}

	for _, test := range []struct {
		name    string
		execute func() error
	}{
		{name: "atomic create", execute: func() error { _, err := g.Atomic("t").Create(item).Execute(context.Background()); return err }},
		{name: "atomic replace", execute: func() error { _, err := g.Atomic("t").Replace(item).Execute(context.Background()); return err }},
		{name: "atomic upsert", execute: func() error { _, err := g.Atomic("t").Upsert(item).Execute(context.Background()); return err }},
		{name: "bulk create", execute: func() error { _, err := g.Bulk().Create(item).Execute(context.Background()); return err }},
		{name: "bulk replace", execute: func() error { _, err := g.Bulk().Replace(item).Execute(context.Background()); return err }},
		{name: "bulk upsert", execute: func() error { _, err := g.Bulk().Upsert(item).Execute(context.Background()); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.execute(); !errors.Is(err, ErrValidation) || f.calls != 0 {
				t.Fatalf("calls=%d err=%v", f.calls, err)
			}
		})
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
func TestBatchFailuresAndSize(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &chunkFake{status: 409}
	g.container = f
	result, err := g.Atomic("t").Read("id").Execute(context.Background())
	if !errors.Is(err, ErrConflict) || !errors.Is(result.Items[0].Err, ErrConflict) {
		t.Fatalf("%v %+v", err, result)
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
