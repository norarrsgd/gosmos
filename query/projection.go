package query

import (
	"fmt"
	"strings"
)

// Projection is an immutable snapshot created by ProjectionBuilder.Build.
type Projection struct{ items []projectionItem }
type projectionItem struct {
	name, column, alias string
	aggregate           Aggregate
	object              *Projection
}
type ProjectionBuilder struct{ items []projectionItem }

func (b *ProjectionBuilder) Field(name, column string, alias ...string) *ProjectionBuilder {
	b.items = append(b.items, projectionItem{name: name, column: column, alias: operandAlias(alias)})
	return b
}
func (b *ProjectionBuilder) Object(name string, configure func(*ProjectionBuilder)) *ProjectionBuilder {
	nested := new(ProjectionBuilder)
	if configure != nil {
		configure(nested)
	}
	p := nested.Build()
	b.items = append(b.items, projectionItem{name: name, object: &p})
	return b
}
func (b *ProjectionBuilder) Aggregate(name, column string, aggregate Aggregate, alias ...string) *ProjectionBuilder {
	b.items = append(b.items, projectionItem{name: name, column: column, alias: operandAlias(alias), aggregate: aggregate})
	return b
}
func (b *ProjectionBuilder) Build() Projection {
	return Projection{items: append([]projectionItem(nil), b.items...)}
}
func (b *builder) aggregate(column string, ag Aggregate, alias string) (string, error) {
	p, e := b.property(alias, column)
	if e != nil {
		return "", e
	}
	switch ag {
	case Count:
		return "COUNT(1)", nil
	case Sum, Min, Max, Avg:
		return strings.ToUpper(string(ag)) + "(" + p + ")", nil
	}
	return "", fmt.Errorf("unrecognized aggregate %q", ag)
}
func (b *builder) projection(p Projection, root bool) ([]string, error) {
	names := map[string]bool{}
	var out []string
	for _, item := range p.items {
		if strings.TrimSpace(item.name) == "" {
			return nil, fmt.Errorf("empty projection output name")
		}
		if names[item.name] {
			return nil, fmt.Errorf("duplicate projection output %q", item.name)
		}
		names[item.name] = true
		if root {
			if e := validateAlias(item.name); e != nil {
				return nil, e
			}
		}
		var expr string
		var e error
		if item.object != nil {
			children, err := b.projection(*item.object, false)
			if err != nil {
				return nil, err
			}
			expr = "{" + strings.Join(children, ", ") + "}"
		} else if item.aggregate != "" {
			expr, e = b.aggregate(item.column, item.aggregate, item.alias)
			b.shape.Aggregate = true
		} else {
			expr, e = b.property(item.alias, item.column)
			b.scalars = append(b.scalars, expr)
		}
		if e != nil {
			return nil, e
		}
		if root {
			out = append(out, expr+" AS "+item.name)
			b.outputs = append(b.outputs, item.name)
		} else {
			out = append(out, jsonString(item.name)+": "+expr)
		}
	}
	return out, nil
}
