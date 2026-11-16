package gosmos

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/norarrsgd/gosmos/tuner"

	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"

	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos/internal/cache"
	"github.com/norarrsgd/gosmos/query"
	"testing"
	"time"
)

func TestQueryRejectsCrossPartitionBeforeTransport(t *testing.T) {
	g := &Galaxy[testQueryEntity]{options: galaxyOptions{crossPartition: true}}
	_, _, err := g.Query().Top(1).List(context.Background())
	if !errors.Is(err, ErrUnsupportedCrossPartition) {
		t.Fatal(err)
	}
}

type testQueryEntity struct {
	Base
	Tenant string `json:"tenant" gosmos:"pk=1"`
}

func TestQueryGenerationDoesNotRequirePartition(t *testing.T) {
	g := &Galaxy[testQueryEntity]{}
	s, e := g.Query().Cluster(func(c *ClusterBuilder) { c.Eq("tenant", "x") }).Top(1).GenerateQuery()
	if e != nil || !s.Shape.Top || len(s.Params) != 1 {
		t.Fatal(s, e)
	}
}
func TestInvalidPageRejected(t *testing.T) {
	g := &Galaxy[testQueryEntity]{}
	_, _, e := g.Query().InPartition("x").Paged(context.Background(), query.Page{Size: 0})
	if !errors.Is(e, ErrValidation) {
		t.Fatal(e)
	}
}

func TestPointCacheExplicitConsistencyBypass(t *testing.T) {
	g, f := testGalaxy(t)
	g.options.cache = cache.New(2, time.Minute)
	f.body = []byte(`{"id":"one","tenant":"t"}`)
	ctx := context.Background()
	first, _, e := g.Get(ctx, "one", []string{"t"})
	if e != nil || first.ID != "one" {
		t.Fatal(first, e)
	}
	f.body = []byte(`{"id":"two","tenant":"t"}`)
	second, _, _ := g.Get(ctx, "one", []string{"t"})
	if second.ID != "one" {
		t.Fatal("cache miss")
	}
	level := azcosmos.ConsistencyLevelStrong
	third, _, e := g.Get(ctx, "one", []string{"t"}, ReadOptions{ConsistencyLevel: &level})
	if e != nil || third.ID != "two" {
		t.Fatal(third, e)
	}
	fourth, _, _ := g.Get(ctx, "one", []string{"t"})
	if fourth.ID != "one" {
		t.Fatal("override refreshed cache")
	}
}

type pagedFake struct {
	fakeContainer
	pages []azcosmos.QueryItemsResponse
	calls int
	opts  *azcosmos.QueryOptions
}

func (f *pagedFake) NewQueryItemsPager(_ string, _ azcosmos.PartitionKey, o *azcosmos.QueryOptions) *runtime.Pager[azcosmos.QueryItemsResponse] {
	f.opts = o
	return runtime.NewPager(runtime.PagingHandler[azcosmos.QueryItemsResponse]{More: func(r azcosmos.QueryItemsResponse) bool { return r.ContinuationToken != nil }, Fetcher: func(context.Context, *azcosmos.QueryItemsResponse) (azcosmos.QueryItemsResponse, error) {
		r := f.pages[f.calls]
		f.calls++
		return r, nil
	}})
}
func TestPagingSkipsEmptyPagesAndCapturesConsistency(t *testing.T) {
	token := "next"
	f := &pagedFake{pages: []azcosmos.QueryItemsResponse{{ContinuationToken: &token, Response: azcosmos.Response{RequestCharge: 2}}, {Items: [][]byte{[]byte(`{"id":"one"}`)}, Response: azcosmos.Response{RequestCharge: 3}}}}
	g := &Galaxy[testQueryEntity]{container: f}
	rows, gravity, e := g.Query().InPartition("t").WithConsistency(azcosmos.ConsistencyLevelStrong).Paged(context.Background(), query.Page{Size: 2})
	if e != nil || len(rows) != 1 || gravity.RU != 5 || f.calls != 2 || f.opts.ConsistencyLevel == nil || *f.opts.ConsistencyLevel != azcosmos.ConsistencyLevelStrong {
		t.Fatal(rows, gravity, e)
	}
}
func TestSelectAsHonorsJSONTags(t *testing.T) {
	type summary struct {
		ID     string `json:"id"`
		Secret string `json:"-"`
	}
	g := &Galaxy[testQueryEntity]{}
	o := SelectAs[summary](g.Query())
	s, e := o.GenerateQuery()
	if e != nil || strings.Contains(s.Text, "Secret") || !strings.Contains(s.Text, `c["id"]`) {
		t.Fatal(s, e)
	}
}
func TestAtomicInvalidatesPointCache(t *testing.T) {
	g, _ := testGalaxy(t)
	g.options.cache = cache.New(2, time.Minute)
	f := &batchFake{}
	f.body = []byte(`{"id":"old","tenant":"t"}`)
	g.container = f
	ctx := context.Background()
	g.Get(ctx, "id", []string{"t"})
	f.body = []byte(`{"id":"new","tenant":"t"}`)
	if _, e := g.Atomic("t").Create(testEntity{Tenant: "t"}).Execute(ctx); e != nil {
		t.Fatal(e)
	}
	got, _, e := g.Get(ctx, "id", []string{"t"})
	if e != nil || got.ID != "new" {
		t.Fatal("stale after transaction", got, e)
	}
}
func TestFailedWriteInvalidatesCache(t *testing.T) {
	g, f := testGalaxy(t)
	g.options.cache = cache.New(2, time.Minute)
	f.body = []byte(`{"id":"old","tenant":"t"}`)
	g.Get(context.Background(), "id", []string{"t"})
	f.err = errors.New("connection lost after commit")
	g.Modify(context.Background(), testEntity{Base: Base{ID: "id"}, Tenant: "t"})
	f.err = nil
	f.body = []byte(`{"id":"new","tenant":"t"}`)
	got, _, e := g.Get(context.Background(), "id", []string{"t"})
	if e != nil || got.ID != "new" {
		t.Fatal(got, e)
	}
}
func TestEmbeddedPointerEntityRejected(t *testing.T) {
	type unsafeEntity struct {
		*Base
		Tenant string `gosmos:"pk=1"`
	}
	if _, e := inspectEntity[unsafeEntity](); !errors.Is(e, ErrValidation) {
		t.Fatal(e)
	}
}
func TestQueryRecordsStatistics(t *testing.T) {
	s := tuner.NewMemory()
	f := &pagedFake{pages: []azcosmos.QueryItemsResponse{{Items: [][]byte{[]byte(`{"id":"one"}`)}, Response: azcosmos.Response{RequestCharge: 2}}}}
	g := &Galaxy[testQueryEntity]{container: f}
	if e := WithTuner(s)(&g.options); e != nil {
		t.Fatal(e)
	}
	o := g.Query().InPartition("t")
	_, _, e := o.List(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	statement, _ := o.GenerateQuery()
	normalized, _ := query.Normalize(statement.Text)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
	rows, e := s.Samples(context.Background(), hash, time.Time{})
	if e != nil || len(rows) != 1 || rows[0].RU != 2 || !rows[0].Success {
		t.Fatal(rows, e)
	}
}
func TestSingleResultQueryCache(t *testing.T) {
	f := &pagedFake{pages: []azcosmos.QueryItemsResponse{{Items: [][]byte{[]byte(`{"id":"one"}`)}}}}
	g := &Galaxy[testQueryEntity]{container: f, options: galaxyOptions{cache: cache.New(2, time.Minute)}}
	for range 2 {
		got, _, e := g.Query().InPartition("t").Get(context.Background())
		if e != nil || got.ID != "one" {
			t.Fatal(got, e)
		}
	}
	if f.calls != 1 {
		t.Fatal("cache miss", f.calls)
	}
}
func TestGalaxyNamingPolicy(t *testing.T) {
	g := &Galaxy[testQueryEntity]{}
	if e := WithNamingPolicy(strings.ToLower)(&g.options); e != nil {
		t.Fatal(e)
	}
	s, e := g.Query().Select("Status").GenerateQuery()
	if e != nil || !strings.Contains(s.Text, `c["status"]`) {
		t.Fatal(s, e)
	}
}
