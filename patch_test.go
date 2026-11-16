package gosmos

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"reflect"
	"strings"
	"testing"
)

func TestPatchValidation(t *testing.T) {
	for _, path := range []string{"typo", "/typo", "/tenant/x", "/id"} {
		p := &Patch[testEntity]{}
		p.Set(path, "x")
		if _, err := p.Build(); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	p := &Patch[testEntity]{}
	p.Set("tenant", "x")
	if _, err := p.Build(); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		p.Set("tenant", "x")
	}
	if _, err := p.Build(); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}

type patchFake struct {
	fakeContainer
	calls int
}

func (f *patchFake) PatchItem(context.Context, azcosmos.PartitionKey, string, azcosmos.PatchOperations, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error) {
	f.calls++
	return azcosmos.ItemResponse{}, nil
}
func TestPatchRejectBeforeNetwork(t *testing.T) {
	g, _ := testGalaxy(t)
	f := &patchFake{}
	g.container = f
	_, err := g.Patch(context.Background(), "id", []string{"t"}, func(p *Patch[testEntity]) { p.Set("typo", 1) })
	if !errors.Is(err, ErrValidation) || f.calls != 0 {
		t.Fatalf("%v calls=%d", err, f.calls)
	}
}

func TestPatchNullAndJSONPaths(t *testing.T) {
	p := &Patch[testEntity]{}
	p.Set("tenant", nil)
	operations, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(operations)
	if err != nil || !strings.Contains(string(encoded), `"value":null`) {
		t.Fatalf("null value: %s %v", encoded, err)
	}
	type nested struct {
		Base
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		Labels map[string]string `json:"labels"`
	}
	for _, path := range []string{"items.0.name", "/items/-", "/labels/a~1b"} {
		if _, err := patchPath(reflect.TypeFor[nested](), path); err != nil {
			t.Fatal(path, err)
		}
	}
	for _, path := range []string{"/items/-1/name", "/labels/a~2b", "/items/01/name"} {
		if _, err := patchPath(reflect.TypeFor[nested](), path); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s: %v", path, err)
		}
	}
}
