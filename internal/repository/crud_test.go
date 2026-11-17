package repository

import (
	"context"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"sync"
	"testing"
)

type testEntity struct {
	Base
	Tenant string `json:"tenant" gosmos:"pk=1"`
}

type shadowIDEntity struct {
	Base
	BodyID string `json:"id"`
	Tenant string `json:"tenant" gosmos:"pk=1"`
}

type customJSONEntity struct {
	methodID string
	body     string
}

func (e customJSONEntity) GetID() string                { return e.methodID }
func (e customJSONEntity) MarshalJSON() ([]byte, error) { return []byte(e.body), nil }

type fakeContainer struct {
	body    []byte
	options *azcosmos.ItemOptions
	err     error
}

func (f *fakeContainer) CreateItem(_ context.Context, _ azcosmos.PartitionKey, b []byte, o *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	f.body = b
	f.options = o
	return azcosmos.ItemResponse{Response: azcosmos.Response{RequestCharge: 2, ETag: "etag"}}, f.err
}
func (f *fakeContainer) ReplaceItem(c context.Context, p azcosmos.PartitionKey, _ string, b []byte, o *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	return f.CreateItem(c, p, b, o)
}
func (f *fakeContainer) DeleteItem(c context.Context, p azcosmos.PartitionKey, _ string, o *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	return f.CreateItem(c, p, nil, o)
}
func (f *fakeContainer) ReadItem(_ context.Context, _ azcosmos.PartitionKey, _ string, o *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	f.options = o
	return azcosmos.ItemResponse{Value: f.body, Response: azcosmos.Response{RequestCharge: 1, ETag: "read"}}, f.err
}
func testGalaxy(t *testing.T) (*Galaxy[testEntity], *fakeContainer) {
	t.Helper()
	m, err := inspectEntity[testEntity]()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeContainer{}
	return &Galaxy[testEntity]{container: f, metadata: m}, f
}
func TestCRUD(t *testing.T) {
	g, f := testGalaxy(t)
	input := testEntity{Tenant: "t"}
	created, gravity, err := g.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.AddedOn.IsZero() || input.ID != "" || gravity.RU != 2 || gravity.ETag != "etag" {
		t.Fatalf("bad creation: %+v %+v", created, gravity)
	}
	created, _, err = g.Modify(context.Background(), created, IfMatch("etag"))
	if err != nil || created.ModifiedOn == nil || f.options.IfMatchEtag == nil || *f.options.IfMatchEtag != "etag" {
		t.Fatalf("modify: %+v %v", created, err)
	}
	read, _, err := g.Get(context.Background(), created.ID, []string{"t"})
	if err != nil || read.ID != created.ID {
		t.Fatalf("read: %+v %v", read, err)
	}
	_, err = g.Remove(context.Background(), created.ID, []string{"t"}, IfMatch("etag"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestMarshalEntityValidatesDocumentID(t *testing.T) {
	body, id, err := marshalEntity(customJSONEntity{methodID: "trusted", body: `{"id":"trusted","tenant":"t"}`})
	if err != nil || id != "trusted" || string(body) != `{"id":"trusted","tenant":"t"}` {
		t.Fatalf("valid entity: body=%s id=%q err=%v", body, id, err)
	}

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"tenant":"t"}`},
		{name: "duplicate escaped name", body: `{"\u0069d":"trusted","id":"trusted"}`},
		{name: "non-string", body: `{"id":null,"tenant":"t"}`},
		{name: "mismatch", body: `{"id":"attacker","tenant":"t"}`},
		{name: "non-object", body: `["trusted"]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := marshalEntity(customJSONEntity{methodID: "trusted", body: test.body}); !errors.Is(err, ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestWriteRejectsDivergentSerializedID(t *testing.T) {
	m, err := inspectEntity[shadowIDEntity]()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeContainer{err: errors.New("request reached fake")}
	g := &Galaxy[shadowIDEntity]{container: f, metadata: m}
	item := shadowIDEntity{Base: Base{ID: "trusted"}, BodyID: "attacker", Tenant: "t"}

	for _, test := range []struct {
		name  string
		write func() error
	}{
		{name: "create", write: func() error { _, _, err := g.Create(context.Background(), item); return err }},
		{name: "modify", write: func() error { _, _, err := g.Modify(context.Background(), item); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.body = nil
			if err := test.write(); !errors.Is(err, ErrValidation) || f.body != nil {
				t.Fatalf("body=%s err=%v", f.body, err)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	g, f := testGalaxy(t)
	for status, sentinel := range map[int]error{404: ErrNotFound, 409: ErrConflict, 412: ErrPreconditionFailed} {
		f.err = &azcore.ResponseError{StatusCode: status}
		_, _, err := g.Get(context.Background(), "id", []string{"t"})
		var sdk *azcore.ResponseError
		if !errors.Is(err, sentinel) || !errors.As(err, &sdk) {
			t.Fatalf("status %d: %v", status, err)
		}
	}
}
func TestConsistency(t *testing.T) {
	g, f := testGalaxy(t)
	level := azcosmos.ConsistencyLevelStrong
	g.options.consistency = &level
	_, _, _ = g.Get(context.Background(), "id", []string{"t"})
	if f.options.ConsistencyLevel == nil || *f.options.ConsistencyLevel != level {
		t.Fatal("default not passed")
	}
	override := azcosmos.ConsistencyLevelEventual
	_, _, _ = g.Get(context.Background(), "id", []string{"t"}, ReadOptions{ConsistencyLevel: &override})
	if *f.options.ConsistencyLevel != override || *g.options.consistency != level {
		t.Fatal("override mutated default")
	}
	bad := azcosmos.ConsistencyLevel("invalid")
	_, _, err := g.Get(context.Background(), "id", []string{"t"}, ReadOptions{ConsistencyLevel: &bad})
	if !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}
func TestUUID(t *testing.T) {
	prev := ""
	for range 1000 {
		id, err := newUUIDv7()
		if err != nil || id <= prev || id[14] != '7' || !containsByte("89ab", id[19]) {
			t.Fatalf("%s after %s: %v", id, prev, err)
		}
		prev = id
	}
	var wg sync.WaitGroup
	ids := make(chan string, 1000)
	for range 10 {
		wg.Go(func() {
			for range 100 {
				id, err := newUUIDv7()
				if err != nil {
					t.Error(err)
					return
				}
				ids <- id
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatal("duplicate UUID")
		}
		seen[id] = true
	}
}
func containsByte(s string, b byte) bool {
	for i := range len(s) {
		if s[i] == b {
			return true
		}
	}
	return false
}

func TestPointerEntityCopy(t *testing.T) {
	m, err := inspectEntity[*testEntity]()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeContainer{}
	g := &Galaxy[*testEntity]{container: f, metadata: m}
	input := &testEntity{Tenant: "t"}
	created, _, err := g.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if created == input || input.ID != "" || !input.AddedOn.IsZero() || created.ID == "" {
		t.Fatalf("copy changed input: %+v %+v", input, created)
	}
	var nilEntity *testEntity
	if _, _, err = g.Create(context.Background(), nilEntity); !errors.Is(err, ErrValidation) {
		t.Fatalf("nil: %v", err)
	}
}
func TestValidationBeforeIO(t *testing.T) {
	g, f := testGalaxy(t)
	f.err = errors.New("request reached fake")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := g.Create(ctx, testEntity{Tenant: "t"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err := g.Get(context.Background(), "", []string{"t"}); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, _, err := g.Get(context.Background(), "id", nil); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, _, err := g.Modify(context.Background(), testEntity{Tenant: "t"}); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, err := g.Remove(context.Background(), "id", []string{"t"}, IfMatch("")); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}
