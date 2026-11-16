package query_test

import (
	"bytes"
	"encoding/json"
	"github.com/norarrsgd/gosmos/query"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUniverseCorpus(t *testing.T) {
	files, e := filepath.Glob("../testdata/corpus/*.json")
	if e != nil || len(files) < 50 {
		t.Fatalf("missing corpus: %d cases: %v", len(files), e)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			var c struct {
				SchemaVersion            int
				UniverseCommit           string
				Name, SQL, ErrorCategory string
				Input                    query.Input
				Params                   []any
			}
			d := json.NewDecoder(bytes.NewReader(data))
			d.UseNumber()
			if e = d.Decode(&c); e != nil {
				t.Fatal(e)
			}
			if c.SchemaVersion != 1 || c.UniverseCommit != "911dd588ad4f4a8c30e22469f93fc6af377f2d09" {
				t.Fatal("unreviewed schema/source change")
			}
			s, e := query.Build(c.Input)
			if c.ErrorCategory != "" {
				if c.ErrorCategory != "validation" || e == nil {
					t.Fatal("expected validation error", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			sql, names := query.Normalize(s.Text)
			values := map[string]any{}
			for _, p := range s.Params {
				values[p.Name] = p.Value
			}
			actual := []any{}
			for _, name := range names {
				v, ok := values[name]
				if !ok {
					t.Fatal("unbound parameter", name)
				}
				actual = append(actual, v)
			}
			b, e := json.Marshal(actual)
			if e != nil {
				t.Fatal(e)
			}
			d = json.NewDecoder(bytes.NewReader(b))
			d.UseNumber()
			if e = d.Decode(&actual); e != nil {
				t.Fatal(e)
			}
			if sql != c.SQL || !reflect.DeepEqual(actual, c.Params) {
				t.Fatalf("want %s %#v; got %s %#v", c.SQL, c.Params, sql, actual)
			}
		})
	}
}

func FuzzBuild(f *testing.F) {
	f.Add("status", "open")
	f.Add("id", `x' OR true --`)
	f.Fuzz(func(t *testing.T, column, value string) {
		s, e := query.Build(query.Input{Clusters: []query.Cluster{{Catalysts: []query.Catalyst{{Column: column, Value: value}}}}})
		if e != nil {
			return
		}
		if len(s.Params) != 1 || s.Params[0].Value != value {
			t.Fatal("value was not preserved as a bound parameter")
		}
		_, names := query.Normalize(s.Text)
		if len(names) != 1 || names[0] != s.Params[0].Name {
			t.Fatal("missing parameter reference")
		}
	})
}
