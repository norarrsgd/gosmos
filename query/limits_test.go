package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBuildRejectsOversizedStructures(t *testing.T) {
	names := make([]string, 1025)
	for i := range names {
		names[i] = "id"
	}
	catalysts := make([]Catalyst, 1024)
	for i := range catalysts {
		catalysts[i] = Catalyst{Column: "id", ID: "same", Value: i}
	}
	clusters := make([]Cluster, 1025)
	for i := range clusters {
		clusters[i] = Cluster{Catalysts: []Catalyst{{Column: "id", Value: i}}}
	}
	ags := make([]AggregationOption, 1025)
	for i := range ags {
		ags[i] = AggregationOption{Column: "id", Aggregate: Count}
	}
	sorts := make([]Sorting, 1025)
	for i := range sorts {
		sorts[i] = Sorting{Column: "id"}
	}
	cases := map[string]Input{
		"groups":          {Groups: names},
		"columns":         {Columns: ColumnOptions{Names: names}},
		"aggregates":      {Columns: ColumnOptions{Names: []string{"id"}, Aggregates: ags}},
		"join columns":    {Columns: ColumnOptions{Join: &JoinOptions{Alias: "j", ArrayPath: "items", Columns: names}}},
		"join aggregates": {Columns: ColumnOptions{Join: &JoinOptions{Alias: "j", ArrayPath: "items", Aggregates: ags}}},
		"sorting":         {Sorting: sorts},
		"clusters":        {Clusters: clusters},
		"catalysts":       {Clusters: []Cluster{{Catalysts: catalysts}}},
		"combined":        {Columns: ColumnOptions{Names: names[:600]}, Groups: names[:600]},
		"identity bytes":  {Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "id", ID: strings.Repeat("x", 65537), Value: 1}}}}},
		"SQL bytes":       {Columns: ColumnOptions{Names: repeatedNames(590, strings.Repeat("'", 110))}},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			statement, err := Build(input)
			if err == nil || statement.Text != "" || len(statement.Params) != 0 {
				t.Fatalf("oversized input returned SQL (%d bytes), %d parameters, error %v", len(statement.Text), len(statement.Params), err)
			}
		})
	}
}

func repeatedNames(n int, value string) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = value
	}
	return names
}

func TestFullTextBudgets(t *testing.T) {
	for _, op := range []Operator{FTScore, FTContainsAll, NotFTContainsAll, FTContainsAny, NotFTContainsAny} {
		for name, terms := range map[string][]string{
			"term count": repeatedNames(65, "word"),
			"term bytes": {strings.Repeat("a", 1025)},
		} {
			t.Run(string(op)+"/"+name, func(t *testing.T) {
				_, err := Build(Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "text", Operator: op, Value: terms}}}}})
				if err == nil {
					t.Fatal("accepted oversized full-text terms")
				}
			})
		}
	}
	for name, catalysts := range map[string][]Catalyst{
		"parameters":           make([]Catalyst, 17),
		"aggregate term bytes": make([]Catalyst, 2),
	} {
		for i := range catalysts {
			terms := repeatedNames(64, "word")
			if name == "aggregate term bytes" {
				terms = repeatedNames(64, strings.Repeat("x", 1024))
			}
			catalysts[i] = Catalyst{Column: "text", Operator: FTScore, Value: terms}
		}
		t.Run(name, func(t *testing.T) {
			if _, err := Build(Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: catalysts}}}); err == nil {
				t.Fatal("accepted oversized combined full-text input")
			}
		})
	}
}

func nestedProjection(depth int) Projection {
	p := new(ProjectionBuilder).Field("id", "id").Build()
	for i := 1; i < depth; i++ {
		child := p
		p = Projection{items: []projectionItem{{name: "data", object: &child}}}
	}
	return p
}

func TestProjectionBudgets(t *testing.T) {
	wide := new(ProjectionBuilder)
	for i := 0; i < 1025; i++ {
		wide.Field(fmt.Sprintf("f%d", i), "id")
	}
	for name, projection := range map[string]Projection{"depth": nestedProjection(33), "nodes": wide.Build()} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(Input{Columns: ColumnOptions{Projection: &projection}}); err == nil {
				t.Error("Build accepted oversized projection")
			}
			if _, err := json.Marshal(projection); err == nil {
				t.Error("Marshal accepted oversized projection")
			}
		})
	}
	for name, data := range map[string]string{
		"depth":           strings.Repeat(`{"items":[{"name":"data","object":`, 32) + `{"items":[{"name":"id","column":"id"}]}` + strings.Repeat(`}]}`, 32),
		"nodes":           `{"items":[` + strings.Repeat(`{"name":"id","column":"id"},`, 1024) + `{"name":"last","column":"id"}]}`,
		"bytes":           `{"padding":"` + strings.Repeat("x", 262144) + `","items":[]}`,
		"unknown nesting": `{"unknown":` + strings.Repeat("[", 100) + "0" + strings.Repeat("]", 100) + `}`,
	} {
		t.Run("JSON/"+name, func(t *testing.T) {
			p := new(ProjectionBuilder).Field("kept", "id").Build()
			if err := json.Unmarshal([]byte(data), &p); err == nil {
				t.Fatal("Unmarshal accepted oversized projection")
			}
			got, _ := sql(t, Input{Columns: ColumnOptions{Projection: &p}})
			if got != `SELECT c["id"] AS kept FROM c` {
				t.Fatal("failed Unmarshal changed the existing projection")
			}
		})
	}
}

func TestQueryBudgetBoundariesPreserveBindings(t *testing.T) {
	catalysts := make([]Catalyst, 1023)
	for i := range catalysts {
		catalysts[i] = Catalyst{Column: "id", ID: "same", Value: i}
	}
	statement, err := Build(Input{Clusters: []Cluster{{Catalysts: catalysts}}})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for i, param := range statement.Params {
		if seen[param.Name] || param.Value != i {
			t.Fatal("lost predicate occurrence or value")
		}
		seen[param.Name] = true
	}
	if len(seen) != 1023 {
		t.Fatal("lost predicates")
	}
	p := nestedProjection(32)
	wire, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Projection
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Input{Columns: ColumnOptions{Projection: &decoded}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: []Catalyst{{Column: "text", Operator: FTScore, Value: repeatedNames(64, strings.Repeat("x", 1024))}}}}}); err != nil {
		t.Fatal(err)
	}
	ranks := make([]Catalyst, 16)
	for i := range ranks {
		ranks[i] = Catalyst{Column: "text", ID: "same", Operator: FTScore, Value: repeatedNames(64, `'; --`)}
	}
	statement, err = Build(Input{Columns: ColumnOptions{Top: 1}, Clusters: []Cluster{{Catalysts: ranks}}})
	if err != nil || len(statement.Params) != 1024 || strings.Contains(statement.Text, `'; --`) {
		t.Fatalf("parameter budget boundary failed: %d parameters, %v", len(statement.Params), err)
	}
}

func TestBudgetRejectionPrecedesRendering(t *testing.T) {
	called := false
	_, err := Build(Input{Groups: repeatedNames(1025, "id"), NamingPolicy: func(s string) string {
		called = true
		return s
	}})
	if err == nil || called {
		t.Fatal("oversized input reached SQL rendering")
	}
}

func FuzzProjectionJSON(f *testing.F) {
	f.Add(`{"items":[{"name":"data","object":{"items":[{"name":"id","column":"id"}]}}]}`)
	f.Add(`{"items":[{"name":"quote\\\"key","column":"id"}],"ignored":1e1000}`)
	f.Fuzz(func(t *testing.T, data string) {
		var p Projection
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			return
		}
		wire, err := json.Marshal(p)
		if err != nil {
			return // Escaped output may exceed the JSON byte budget.
		}
		var decoded Projection
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		statement, err := Build(Input{Columns: ColumnOptions{Projection: &decoded}})
		if err == nil && (len(statement.Text) > 262144 || len(statement.Params) != 0) {
			t.Fatal("projection escaped generation budgets")
		}
	})
}
