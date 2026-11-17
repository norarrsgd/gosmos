package query

import (
	"fmt"
	"strconv"
	"strings"
)

type builder struct {
	input                    Input
	shape                    Shape
	groups, scalars, outputs []string
	params                   []Param
	ranks                    []ranked
}

func (b *builder) property(alias, path string) (string, error) {
	return formatProperty(alias, path, b.input.NamingPolicy)
}

func addUnique(list []string, seen map[string]bool, value string) []string {
	if seen[value] {
		return list
	}
	seen[value] = true
	return append(list, value)
}

// Build validates the entire input before returning parameterized Cosmos SQL.
func Build(input Input) (Statement, error) {
	if err := validateInputBudget(input); err != nil {
		return Statement{}, err
	}
	b := builder{input: input}
	text, e := b.build()
	if e != nil {
		return Statement{}, e
	}
	if len(text) > MaxSQLBytes {
		return Statement{}, fmt.Errorf("query exceeds %d SQL bytes", MaxSQLBytes)
	}
	return Statement{Text: text, Params: b.params, Shape: b.shape}, nil
}

func (b *builder) build() (string, error) {
	in := b.input
	o := in.Columns
	if o.Top < 0 || o.Top > MaxItems {
		return "", fmt.Errorf("Top must be between 0 and %d", MaxItems)
	}
	b.shape.Top = o.Top > 0
	b.shape.Distinct = o.IsDistinct
	groupSet := map[string]bool{}
	for _, g := range in.Groups {
		p, e := b.property("c", g)
		if e != nil {
			return "", e
		}
		b.groups = addUnique(b.groups, groupSet, p)
	}
	// Parameter names include an occurrence identity so equal-valued predicates never collapse.
	var filters []string
	nextSuffix := map[string]int{}
	index := 0
	for _, cluster := range in.Clusters {
		join, e := where(cluster.Where)
		if e != nil {
			return "", e
		}
		if len(cluster.Catalysts) == 0 {
			return "", fmt.Errorf("cluster must contain catalysts")
		}
		var predicates []string
		for _, c := range cluster.Catalysts {
			if e := c.validate(); e != nil {
				return "", e
			}
			base := parameterName(c, index)
			name := base
			if n := nextSuffix[base]; n > 0 {
				name = base + "_" + strconv.Itoa(n)
			}
			nextSuffix[base]++
			index++
			if !c.unary() && (c.Right == nil || c.Right.Kind == ParameterOperand) {
				value := c.Value
				if c.Right != nil {
					value = c.Right.Value
				}
				if c.op() == FTScore {
					for i, term := range value.([]string) {
						b.params = append(b.params, Param{Name: "@" + name + "_ft" + strconv.Itoa(i), Value: term})
					}
				} else {
					b.params = append(b.params, Param{Name: "@" + name, Value: value})
				}
			}
			if c.rank() {
				if o.Top < 1 || o.Top > MaxVectorItems {
					return "", fmt.Errorf("rank predicates require Top between 1 and %d", MaxVectorItems)
				}
				b.ranks = append(b.ranks, ranked{c: c, name: name, index: len(b.ranks)})
				if c.op() == VectorDistance {
					b.shape.Vector = true
				} else {
					b.shape.FullText = true
				}
				continue
			}
			expr, e := b.predicate(c, name)
			if e != nil {
				return "", e
			}
			if len(predicates) > 0 {
				w, _ := where(c.Where)
				expr = w + " " + expr
			}
			predicates = append(predicates, expr)
		}
		if len(predicates) > 0 {
			expr := "(" + strings.Join(predicates, " ") + ")"
			if len(filters) > 0 {
				expr = join + " " + expr
			}
			filters = append(filters, expr)
		}
	}
	var cols []string
	var selectedOutputs []string
	addField := func(alias, path string) error {
		expr, e := b.property(alias, path)
		if e != nil {
			return e
		}
		cols = append(cols, expr)
		b.scalars = append(b.scalars, expr)
		parts := strings.Split(path, ".")
		last := ""
		for _, p := range parts {
			if strings.TrimSpace(p) != "" {
				last = p
			}
		}
		if in.NamingPolicy != nil {
			last = in.NamingPolicy(last)
		}
		selectedOutputs = append(selectedOutputs, last)
		return nil
	}
	for _, name := range o.Names {
		if e := addField("c", name); e != nil {
			return "", e
		}
	}
	if o.Projection == nil && len(cols) == 0 {
		cols = []string{"*"}
	}
	columnSet := map[string]bool{}
	for _, column := range cols {
		columnSet[column] = true
	}
	appendAggregates := func(ags []AggregationOption, alias string) error {
		for _, ag := range ags {
			expr, e := b.aggregate(ag.Column, ag.Aggregate, alias)
			if e != nil {
				return e
			}
			name := aliasBase(ag.Column, in.NamingPolicy) + "_" + string(ag.Aggregate)
			if ag.Aggregate == Count {
				name = aliasBase("CountAggregate", in.NamingPolicy)
			}
			cols = addUnique(cols, columnSet, expr+" AS "+name)
			selectedOutputs = append(selectedOutputs, name)
			b.shape.Aggregate = true
		}
		return nil
	}
	if len(o.Aggregates) > 0 && len(o.Names) == 0 && o.Projection == nil {
		return "", fmt.Errorf("aggregate columns require selected Names")
	}
	if e := appendAggregates(o.Aggregates, "c"); e != nil {
		return "", e
	}
	if len(o.Aggregates) > 0 && o.Projection == nil {
		for _, s := range b.scalars {
			b.groups = addUnique(b.groups, groupSet, s)
		}
	}
	joinText := ""
	if j := o.Join; j != nil {
		if e := validateAlias(j.Alias); e != nil {
			return "", e
		}
		path, e := b.property("c", j.ArrayPath)
		if e != nil {
			return "", e
		}
		joinText = " JOIN " + j.Alias + " IN " + path
		b.shape.Join = true
		if len(j.Columns) > 0 && len(cols) == 1 && cols[0] == "*" {
			cols[0] = "c.*"
		}
		for _, p := range j.Columns {
			if e := addField(j.Alias, p); e != nil {
				return "", e
			}
		}
		if e := appendAggregates(j.Aggregates, j.Alias); e != nil {
			return "", e
		}
		if len(j.Aggregates) > 0 && o.Projection == nil {
			for _, p := range j.Columns {
				expr, _ := b.property(j.Alias, p)
				b.groups = addUnique(b.groups, groupSet, expr)
			}
		}
	}
	if o.Projection != nil {
		if len(o.Projection.items) == 0 {
			return "", fmt.Errorf("projection must contain output fields")
		}
		p, e := b.projection(*o.Projection, true)
		if e != nil {
			return "", e
		}
		cols = append(cols, p...)
		selectedOutputs = append(selectedOutputs, b.outputs...)
	}
	vectorIndex := 0
	for _, r := range b.ranks {
		if r.c.op() != VectorDistance {
			continue
		}
		expr, e := b.rankExpr(r)
		if e != nil {
			return "", e
		}
		name := aliasBase(r.c.Column, in.NamingPolicy) + "Score"
		if vectorIndex > 0 {
			suffix := r.c.ID
			if suffix == "" {
				suffix = fmt.Sprintf("%08x", r.index)
			}
			if len(suffix) > 8 {
				suffix = suffix[len(suffix)-8:]
			}
			for _, ch := range suffix {
				if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' {
					name += string(ch)
				} else {
					name += "_"
				}
			}
		}
		vectorIndex++
		cols = append(cols, expr+" AS "+name)
		selectedOutputs = append(selectedOutputs, name)
	}
	if o.Projection != nil {
		seen := map[string]bool{}
		for _, name := range selectedOutputs {
			if seen[name] {
				return "", fmt.Errorf("duplicate projection output %q", name)
			}
			seen[name] = true
		}
		if b.shape.Aggregate {
			for _, scalar := range b.scalars {
				b.groups = addUnique(b.groups, groupSet, scalar)
			}
		} else if len(b.groups) > 0 {
			for _, scalar := range b.scalars {
				if !groupSet[scalar] {
					return "", fmt.Errorf("every projected field must be in GROUP BY")
				}
			}
		}
	}
	// Cosmos rejects ordering and ranking with grouping, including legacy selections.
	if (b.shape.Aggregate || len(b.groups) > 0) && (len(in.Sorting) > 0 || len(b.ranks) > 0) {
		return "", fmt.Errorf("aggregate and grouped queries cannot be ordered or ranked")
	}
	if len(b.groups) > 0 {
		for i, c := range cols {
			if c == "*" {
				cols[i] = strings.Join(b.groups, ", ")
			}
		}
	}
	prefix := ""
	if o.Top > 0 {
		prefix = "TOP " + strconv.Itoa(o.Top) + " "
	}
	if o.IsDistinct && (len(o.Names) > 0 || o.Projection != nil) {
		prefix = "DISTINCT " + prefix
	} else {
		b.shape.Distinct = false
	}
	text := "SELECT " + prefix + strings.Join(cols, ", ") + " FROM c" + joinText
	if len(filters) > 0 {
		text += " WHERE " + strings.Join(filters, " ")
	}
	order, e := b.order()
	if e != nil {
		return "", e
	}
	text += order
	b.shape.OrderBy = order != ""
	b.shape.GroupBy = len(b.groups) > 0
	if len(b.groups) > 0 {
		text += " GROUP BY " + strings.Join(b.groups, ", ")
	}
	return text, nil
}
