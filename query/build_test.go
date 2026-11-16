package query

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func sql(t *testing.T, in Input) (string, Statement) {
	t.Helper()
	s, e := Build(in)
	if e != nil {
		t.Fatal(e)
	}
	n, _ := Normalize(s.Text)
	return n, s
}
func TestPredicates(t *testing.T) {
	cases := []struct {
		op    Operator
		value any
		want  string
	}{
		{Eq, "x", `c["a"]["b"] = @p0`}, {NotEq, 1, `c["a"]["b"] != @p0`}, {Gt, 1, `c["a"]["b"] > @p0`}, {Gte, 1, `c["a"]["b"] >= @p0`}, {Lt, 1, `c["a"]["b"] < @p0`}, {Lte, 1, `c["a"]["b"] <= @p0`},
		{In, "x", `ARRAY_CONTAINS(c["a"]["b"], @p0)`}, {NotIn, "x", `NOT ARRAY_CONTAINS(c["a"]["b"], @p0)`}, {Contains, []string{"x"}, `ARRAY_CONTAINS(@p0, c["a"]["b"])`}, {NotContains, []int{1}, `NOT ARRAY_CONTAINS(@p0, c["a"]["b"])`},
		{Len, 2, `ARRAY_LENGTH(c["a"]["b"]) = @p0`}, {Like, "x%", `c["a"]["b"] LIKE @p0`}, {NotLike, "x%", `c["a"]["b"] NOT LIKE @p0`},
		{Defined, nil, `IS_DEFINED(c["a"]["b"])`}, {NotDefined, nil, `NOT IS_DEFINED(c["a"]["b"])`}, {IsNull, nil, `IS_NULL(c["a"]["b"])`}, {IsString, nil, `IS_STRING(c["a"]["b"])`},
		{StartsWith, "x", `STARTSWITH(c["a"]["b"], @p0)`}, {StringContains, "x", `CONTAINS(c["a"]["b"], @p0)`}, {StringEquals, "x", `STRINGEQUALS(c["a"]["b"], @p0)`},
		{FTContains, "x", `FullTextContains(c["a"]["b"], @p0)`}, {NotFTContains, "x", `NOT FullTextContains(c["a"]["b"], @p0)`}, {FTContainsAll, []string{"x"}, `FullTextContainsAll(c["a"]["b"], @p0)`}, {NotFTContainsAll, []string{"x"}, `NOT FullTextContainsAll(c["a"]["b"], @p0)`}, {FTContainsAny, []string{"x"}, `FullTextContainsAny(c["a"]["b"], @p0)`}, {NotFTContainsAny, []string{"x"}, `NOT FullTextContainsAny(c["a"]["b"], @p0)`},
	}
	for _, c := range cases {
		t.Run(string(c.op), func(t *testing.T) {
			n, s := sql(t, Input{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "a.b", Operator: c.op, Value: c.value}}}}})
			if n != "SELECT * FROM c WHERE ("+c.want+")" {
				t.Fatal(n)
			}
			if c.value == nil && len(s.Params) != 0 {
				t.Fatal(s.Params)
			}
			if c.value != nil && !reflect.DeepEqual(s.Params[0].Value, c.value) {
				t.Fatal(s.Params)
			}
		})
	}
}
func TestComposition(t *testing.T) {
	n, s := sql(t, Input{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "title", Operator: StringContains, Value: "%;' --", IgnoreCase: true}, {Column: "shares", Operator: In, Value: map[string]any{"id": "a"}, PartialMatch: true, Where: Or}}}, {Where: Or, Catalysts: []Catalyst{Compare(ArrayLength("events"), Gt, Field("queued", "share"))}}}})
	want := `SELECT * FROM c WHERE (CONTAINS(c["title"], @p0, true) OR ARRAY_CONTAINS(c["shares"], @p1, true)) OR (ARRAY_LENGTH(c["events"]) > share["queued"])`
	if n != want || len(s.Params) != 2 {
		t.Fatalf("%s %+v", n, s.Params)
	}
}
func TestProjectionAndSnapshot(t *testing.T) {
	b := new(ProjectionBuilder).Object("data", func(b *ProjectionBuilder) {
		b.Field("OwnerName", "data.owner").Object("nested", func(b *ProjectionBuilder) { b.Field(`quote"key`, "data.ticketId") })
	})
	p := b.Build()
	b.Field("extra", "x")
	n, _ := sql(t, Input{Columns: ColumnOptions{Names: []string{"id"}, Projection: &p}})
	if n != `SELECT c["id"], {"OwnerName": c["data"]["owner"], "nested": {"quote\u0022key": c["data"]["ticketId"]}} AS data FROM c` {
		t.Fatal(n)
	}
}
func TestAggregatesJoinGroup(t *testing.T) {
	n, s := sql(t, Input{Columns: ColumnOptions{Names: []string{"status"}, Top: 5, IsDistinct: true, Aggregates: []AggregationOption{{Column: "amount", Aggregate: Sum}}, Join: &JoinOptions{Alias: "j", ArrayPath: "items", Columns: []string{"sku"}, Aggregates: []AggregationOption{{Column: "price", Aggregate: Avg}}}}})
	want := `SELECT DISTINCT TOP 5 c["status"], SUM(c["amount"]) AS amount_Sum, j["sku"], AVG(j["price"]) AS price_Avg FROM c JOIN j IN c["items"] GROUP BY c["status"], j["sku"]`
	if n != want || !s.Shape.Aggregate || !s.Shape.Join || !s.Shape.GroupBy || s.Shape.OrderBy || !s.Shape.Top || !s.Shape.Distinct {
		t.Fatalf("%s %+v", n, s.Shape)
	}
}
func TestNestedAggregateShape(t *testing.T) {
	p := new(ProjectionBuilder).Object("data", func(b *ProjectionBuilder) { b.Field("name", "owner").Aggregate("total", "amount", Sum) }).Build()
	n, s := sql(t, Input{Columns: ColumnOptions{Projection: &p}})
	if !s.Shape.Aggregate || !s.Shape.GroupBy || !strings.HasSuffix(n, ` GROUP BY c["owner"]`) {
		t.Fatalf("%s %+v", n, s.Shape)
	}
}
func TestRanking(t *testing.T) {
	n, s := sql(t, Input{Columns: ColumnOptions{Names: []string{"id"}, Top: 5}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "vector", Operator: VectorDistance, Value: []float32{1, 2}}, {Column: "text", Operator: FTScore, Value: []string{"one", "two"}}}}}, Sorting: []Sorting{{Direction: Weighted, Column: "0.7, 0.3"}}})
	want := `SELECT TOP 5 c["id"], VectorDistance(c["vector"], @p0) AS vectorScore FROM c ORDER BY RANK RRF(VectorDistance(c["vector"], @p0), FullTextScore(c["text"], @p1, @p2), [0.7, 0.3])`
	if n != want || len(s.Params) != 3 || !s.Shape.RRF || !s.Shape.Vector || !s.Shape.FullText {
		t.Fatalf("%s %+v", n, s)
	}
}
func TestNormalize(t *testing.T) {
	n, names := Normalize(`SELECT "@a", '@b''@c', "x\"@d" FROM c WHERE c["@e"] = @abc AND @abc = @other`)
	if n != `SELECT "@a", '@b''@c', "x\"@d" FROM c WHERE c["@e"] = @p0 AND @p0 = @p1` || !reflect.DeepEqual(names, []string{"@abc", "@other"}) {
		t.Fatalf("%s %v", n, names)
	}
}
func TestValidation(t *testing.T) {
	bad := []Input{
		{Clusters: []Cluster{{}}}, {Columns: ColumnOptions{Top: -1}}, {Columns: ColumnOptions{Top: 1001}},
		{Columns: ColumnOptions{Join: &JoinOptions{Alias: "x;--", ArrayPath: "items"}}}, {Sorting: []Sorting{{Column: "x]", Direction: Asc}}}, {Groups: []string{"x/*"}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: Operator("oops"), Value: 1}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Value: nil}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: Defined, Value: 1}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: Contains, Value: "x"}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: StringContains, Value: 4}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Value: 1, IgnoreCase: true}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Value: 1, PartialMatch: true}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: VectorDistance, Value: []float64{math.NaN()}}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: FTScore, Value: []string{"a"}}}}}},
		{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", Operator: FTScore, Value: []string{""}}}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{Compare(Field("x"), Eq, Parameter(nil))}}}},
		{Clusters: []Cluster{{Catalysts: []Catalyst{Compare(Parameter(1), Eq, Field("x"))}}}},
		{Sorting: []Sorting{{Column: "x", Direction: Direction("bad")}}},
	}
	for i, in := range bad {
		if s, e := Build(in); e == nil {
			t.Errorf("case %d accepted: %s", i, s.Text)
		}
	}
	for _, id := range []string{"", ".", "a;", `a"`, "a]", "a--b", "a/*b", "a\n", strings.Repeat("a", 256)} {
		if ValidateIdentifier(id) == nil {
			t.Errorf("accepted %q", id)
		}
	}
}
