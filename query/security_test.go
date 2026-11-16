package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestIdentifierAndAliasBoundaries(t *testing.T) {
	for _, id := range []string{"id", "a.b", "full name", "O'Reilly", "a\\b", "σ", "a[b"} {
		if e := ValidateIdentifier(id); e != nil {
			t.Errorf("valid identifier %q: %v", id, e)
		}
	}
	for _, alias := range []string{"SELECT", "x.y", "1x", "x] OR true", "x--", "x x", strings.Repeat("x", 129)} {
		_, e := Build(Input{Columns: ColumnOptions{Join: &JoinOptions{Alias: alias, ArrayPath: "items"}}})
		if e == nil {
			t.Errorf("accepted alias %q", alias)
		}
	}
	for _, bad := range []string{"id; DROP", "x]", "x\"", "x/*", "x\x00"} {
		inputs := []Input{{Columns: ColumnOptions{Names: []string{bad}}}, {Columns: ColumnOptions{Aggregates: []AggregationOption{{Column: bad, Aggregate: Count}}}}, {Columns: ColumnOptions{Join: &JoinOptions{Alias: "j", ArrayPath: bad}}}, {Groups: []string{bad}}, {Sorting: []Sorting{{Column: bad}}}, {Clusters: []Cluster{{Catalysts: []Catalyst{{Column: bad, Value: 1}}}}}, {Clusters: []Cluster{{Catalysts: []Catalyst{Compare(Field("x"), Eq, Field(bad))}}}}}
		for _, in := range inputs {
			if s, e := Build(in); e == nil {
				t.Errorf("accepted %q: %s", bad, s.Text)
			}
		}
	}
}
func TestProjectionValidation(t *testing.T) {
	cases := []Input{}
	duplicate := new(ProjectionBuilder).Field("x", "a").Object("x", func(b *ProjectionBuilder) { b.Field("z", "c") }).Build()
	cases = append(cases, Input{Columns: ColumnOptions{Projection: &duplicate}})
	nestedDuplicate := new(ProjectionBuilder).Object("x", func(b *ProjectionBuilder) { b.Field("y", "a").Field("y", "b") }).Build()
	cases = append(cases, Input{Columns: ColumnOptions{Projection: &nestedDuplicate}})
	unsafeName := new(ProjectionBuilder).Field("x;--", "a").Build()
	cases = append(cases, Input{Columns: ColumnOptions{Projection: &unsafeName}})
	collision := new(ProjectionBuilder).Field("id", "other").Build()
	cases = append(cases, Input{Columns: ColumnOptions{Names: []string{"id"}, Projection: &collision}})
	missingGroup := new(ProjectionBuilder).Field("x", "a").Build()
	cases = append(cases, Input{Columns: ColumnOptions{Projection: &missingGroup}, Groups: []string{"b"}})
	aggregate := new(ProjectionBuilder).Aggregate("n", "id", Count).Build()
	cases = append(cases, Input{Columns: ColumnOptions{Projection: &aggregate}, Sorting: []Sorting{{Column: "id"}}})
	empty := new(ProjectionBuilder).Build()
	cases = append(cases, Input{Columns: ColumnOptions{Projection: &empty}})
	for i, in := range cases {
		if s, e := Build(in); e == nil {
			t.Errorf("case %d: %s", i, s.Text)
		}
	}
}
func TestNamingPolicyAndBoundValues(t *testing.T) {
	value := `" ]; DROP FROM c --`
	n, s := sql(t, Input{NamingPolicy: strings.ToLower, Columns: ColumnOptions{Names: []string{"ID"}}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "Data.TITLE", Value: value}, {Column: "Data.TITLE", Value: value}}}}})
	if n != `SELECT c["id"] FROM c WHERE (c["data"]["title"] = @p0 AND c["data"]["title"] = @p1)` || len(s.Params) != 2 || s.Params[0].Name == s.Params[1].Name || s.Params[0].Value != value {
		t.Fatalf("%s %+v", n, s.Params)
	}
	_, e := Build(Input{NamingPolicy: func(string) string { return `x"] OR true` }, Columns: ColumnOptions{Names: []string{"x"}}})
	if e == nil {
		t.Fatal("unsafe naming policy accepted")
	}
}
func TestParameterIdentityCollision(t *testing.T) {
	n, s := sql(t, Input{Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "x", ID: "same", Value: 1}, {Column: "x", ID: "same", Value: 2}}}}})
	if len(s.Params) != 2 || s.Params[0].Name == s.Params[1].Name || !strings.Contains(n, "@p1") {
		t.Fatalf("%s %+v", n, s)
	}
}
func TestSingleRanksAndFilters(t *testing.T) {
	n, s := sql(t, Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "text", Operator: FTScore, Value: []string{"word"}}, {Column: "id", Value: "one"}}}}})
	if n != `SELECT TOP 1 * FROM c WHERE (c["id"] = @p0) ORDER BY RANK FullTextScore(c["text"], @p1)` || !s.Shape.FullText || s.Shape.RRF {
		t.Fatalf("%s %+v", n, s.Shape)
	}
	n, s = sql(t, Input{Columns: ColumnOptions{Top: 2}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "v", Operator: VectorDistance, Value: []float64{0.1, 0.2}}}}}})
	if n != `SELECT TOP 2 *, VectorDistance(c["v"], @p0) AS vScore FROM c ORDER BY VectorDistance(c["v"], @p0)` || !s.Shape.Vector {
		t.Fatalf("%s %+v", n, s.Shape)
	}
}
func TestWeightSafety(t *testing.T) {
	for _, w := range []string{"NaN", "0.5; DROP", "[0.5,]", "-1,2", "1e3,2", "1, /*2*/ 2"} {
		_, e := Build(Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "a", Operator: FTScore, Value: []string{"a"}}, {Column: "b", Operator: FTScore, Value: []string{"b"}}}}}, Sorting: []Sorting{{Direction: Weighted, Column: w}}})
		if e == nil {
			t.Errorf("accepted %q", w)
		}
	}
}
func TestComparisonParameterAndNoParameter(t *testing.T) {
	n, s := sql(t, Input{Clusters: []Cluster{{Catalysts: []Catalyst{Compare(ArrayLength("a"), Gte, Parameter(2)), Compare(Field("a"), NotEq, Field("b"))}}}})
	if n != `SELECT * FROM c WHERE (ARRAY_LENGTH(c["a"]) >= @p0 AND c["a"] != c["b"])` || !reflect.DeepEqual(s.Params[0].Value, 2) || len(s.Params) != 1 {
		t.Fatalf("%s %+v", n, s.Params)
	}
}
func FuzzValidateIdentifier(f *testing.F) {
	for _, s := range []string{"id", "x.y", `x"] OR true --`, "a\x00", ".", "σ"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidateIdentifier(s) != nil {
			return
		}
		p, e := FormatProperty("c", s)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.HasPrefix(p, "c[") {
			t.Fatalf("accepted property does not render segments: %q", s)
		}
	})
}
func FuzzNormalize(f *testing.F) {
	f.Add(`SELECT c["@name"] FROM c WHERE c["id"] = @p_deadbeef`)
	f.Fuzz(func(t *testing.T, s string) {
		normalized, _ := Normalize(s)
		again, _ := Normalize(normalized)
		if normalized != again {
			t.Fatalf("normalization is not idempotent: %q / %q", normalized, again)
		}
	})
}

func TestWhitespaceOnlyPathSegments(t *testing.T) {
	if ValidateIdentifier(". .") == nil {
		t.Fatal("path with no property segments accepted")
	}
}

func TestMultipleVectorsHaveDistinctScoreAliases(t *testing.T) {
	p := new(ProjectionBuilder).Field("ID", "id").Build()
	n, _ := sql(t, Input{Columns: ColumnOptions{Projection: &p, Top: 2}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "v", Operator: VectorDistance, Value: []float64{1}}, {Column: "v", Operator: VectorDistance, Value: []float64{2}}}}}})
	if !strings.Contains(n, " AS vScore,") || !strings.Contains(n, " AS vScore00000001 FROM") {
		t.Fatal(n)
	}
}

func TestRejectGroupedOrderingAndRanking(t *testing.T) {
	cases := []Input{
		{Groups: []string{"status"}, Sorting: []Sorting{{Column: "status", Direction: Asc}}},
		{Columns: ColumnOptions{Names: []string{"status"}, Aggregates: []AggregationOption{{Column: "amount", Aggregate: Sum}}}, Sorting: []Sorting{{Column: "status"}}},
		{Columns: ColumnOptions{Top: 2}, Groups: []string{"status"}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "embedding", Operator: VectorDistance, Value: []float64{1, 2}}}}}},
		{Columns: ColumnOptions{Top: 2, Join: &JoinOptions{Alias: "j", ArrayPath: "items", Aggregates: []AggregationOption{{Column: "price", Aggregate: Sum}}}}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "text", Operator: FTScore, Value: []string{"term"}}}}}},
	}
	for i, input := range cases {
		statement, err := Build(input)
		if err == nil {
			t.Errorf("case %d accepted unsupported grouped ordering: %s", i, statement.Text)
		}
	}
}
