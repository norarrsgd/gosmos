package query

import (
	"encoding/json"
	"testing"
)

func TestProjectionJSONRoundTrip(t *testing.T) {
	original := new(ProjectionBuilder).Object("data", func(b *ProjectionBuilder) { b.Field("OwnerName", "data.owner").Aggregate("Total", "total", Sum) }).Build()
	wire, e := json.Marshal(original)
	if e != nil {
		t.Fatal(e)
	}
	var decoded Projection
	if e = json.Unmarshal(wire, &decoded); e != nil {
		t.Fatal(e)
	}
	want, _ := sql(t, Input{Columns: ColumnOptions{Projection: &original}})
	got, _ := sql(t, Input{Columns: ColumnOptions{Projection: &decoded}})
	if got != want {
		t.Fatalf("got %s want %s; wire=%s", got, want, wire)
	}
}
func TestCatalystJSONTypedValues(t *testing.T) {
	for _, data := range []string{`{"column":"text","operator":"FTScore","value":["one","two"]}`, `{"column":"v","operator":"VectorDistance","value":[0.1,0.2]}`} {
		var c Catalyst
		if e := json.Unmarshal([]byte(data), &c); e != nil {
			t.Fatal(e)
		}
		if _, e := Build(Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: []Catalyst{c}}}}); e != nil {
			t.Fatal(e)
		}
	}
}
func TestUniverseJSONEscaping(t *testing.T) {
	p := new(ProjectionBuilder).Object("data", func(b *ProjectionBuilder) { b.Field("quote\"key'σ", "source") }).Build()
	got, _ := sql(t, Input{Columns: ColumnOptions{Projection: &p}})
	want := `SELECT {"quote\u0022key\u0027\u03C3": c["source"]} AS data FROM c`
	if got != want {
		t.Fatalf("%s", got)
	}
}
func TestPredicateOptions(t *testing.T) {
	c := Catalyst{Column: "title", Operator: StringContains, Value: "hello"}
	var option PredicateOption = IgnoreCase
	option(&c)
	if !c.IgnoreCase {
		t.Fatal("IgnoreCase option did not apply")
	}
	c = Catalyst{Column: "shares", Operator: In, Value: map[string]any{"id": "owner"}}
	PartialMatch(&c)
	if !c.PartialMatch {
		t.Fatal("PartialMatch option did not apply")
	}
	if _, e := Build(Input{Clusters: []Cluster{{Catalysts: []Catalyst{c}}}}); e != nil {
		t.Fatal(e)
	}
}

func TestCatalystJSONPreservesNumbers(t *testing.T) {
	for _, literal := range []string{"9007199254740993", "1234567890.1234567890123456789"} {
		var c Catalyst
		if e := json.Unmarshal([]byte(`{"column":"n","value":`+literal+`}`), &c); e != nil {
			t.Fatal(e)
		}
		if number, ok := c.Value.(json.Number); !ok || number.String() != literal {
			t.Fatalf("lost numeric precision: %T %v", c.Value, c.Value)
		}
		if e := json.Unmarshal([]byte(`{"column":"n","operator":"Eq","left":{"kind":"Field","path":"n"},"right":{"kind":"Parameter","value":`+literal+`}}`), &c); e != nil {
			t.Fatal(e)
		}
		if number, ok := c.Right.Value.(json.Number); !ok || number.String() != literal {
			t.Fatalf("lost operand precision: %T %v", c.Right.Value, c.Right.Value)
		}
	}
}
