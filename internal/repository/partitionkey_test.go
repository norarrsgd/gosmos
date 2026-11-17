package repository

import (
	"errors"
	"reflect"
	"testing"
)

func TestPartitionMetadata(t *testing.T) {
	cases := []struct {
		name   string
		sample any
		valid  bool
		paths  []string
	}{
		{"ordered", struct {
			Base
			Second string `json:"second" gosmos:"pk=2"`
			First  string `json:"first" gosmos:"pk=1"`
		}{}, true, []string{"/first", "/second"}},
		{"missing", struct{ Base }{}, false, nil},
		{"gap", struct {
			Base
			Key string `json:"key" gosmos:"pk=2"`
		}{}, false, nil},
		{"duplicate", struct {
			Base
			A string `json:"a" gosmos:"pk=1"`
			B string `json:"b" gosmos:"pk=1"`
		}{}, false, nil},
		{"too many", struct {
			Base
			Key string `json:"key" gosmos:"pk=4"`
		}{}, false, nil},
		{"bad tag", struct {
			Base
			Key string `json:"key" gosmos:"pk=x"`
		}{}, false, nil},
		{"ignored", struct {
			Base
			Key string `json:"-" gosmos:"pk=1"`
		}{}, false, nil},
		{"unexported", struct {
			Base
			key string `gosmos:"pk=1"`
		}{}, false, nil},
		{"unsupported numeric", struct {
			Base
			Key int `json:"key" gosmos:"pk=1"`
		}{}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := inspectType(reflect.TypeOf(tc.sample))
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				for i, path := range tc.paths {
					if m.fields[i].path != path {
						t.Fatalf("path %s", m.fields[i].path)
					}
				}
			} else if !errors.Is(err, ErrValidation) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

type overrideEntity struct {
	Base
	Key string
}

func (e *overrideEntity) PartitionKeyValues() []string { return []string{e.Key} }
func TestPartitionOverrideAndNull(t *testing.T) {
	m, err := inspectEntity[overrideEntity]()
	if err != nil {
		t.Fatal(err)
	}
	v := reflect.ValueOf(&overrideEntity{Key: "tenant"}).Elem()
	if _, err = m.key(v); err != nil {
		t.Fatal(err)
	}
	sample := struct {
		Base
		Key *string `json:"key" gosmos:"pk=1"`
	}{}
	m, err = inspectType(reflect.TypeOf(sample))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.key(reflect.ValueOf(sample)); !errors.Is(err, ErrValidation) {
		t.Fatalf("null key: %v", err)
	}
}

type shortOverrideEntity struct {
	Base
	Tenant string `json:"tenant" gosmos:"pk=1"`
	User   string `json:"user" gosmos:"pk=2"`
}

func (e shortOverrideEntity) PartitionKeyValues() []string { return []string{e.Tenant} }

func TestPartitionOverridePreservesTaggedWidth(t *testing.T) {
	m, err := inspectEntity[shortOverrideEntity]()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.fields) != 2 || m.fields[0].path != "/tenant" || m.fields[1].path != "/user" {
		t.Fatalf("lost tagged schema: %+v", m.fields)
	}
	if _, err := m.key(reflect.ValueOf(shortOverrideEntity{})); !errors.Is(err, ErrValidation) {
		t.Fatalf("prefix override accepted: %v", err)
	}
	m, err = inspectEntity[hierarchicalOverrideEntity]()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.key(reflect.ValueOf(hierarchicalOverrideEntity{Tenant: "tenant", User: "user"})); err != nil {
		t.Fatalf("complete override rejected: %v", err)
	}
}
