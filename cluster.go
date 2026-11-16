package gosmos

import "github.com/norarrsgd/gosmos/query"

type PredicateOption = query.PredicateOption

var IgnoreCase PredicateOption = func(c *query.Catalyst) { c.IgnoreCase = true }
var PartialMatch PredicateOption = func(c *query.Catalyst) { c.PartialMatch = true }

type ClusterBuilder struct {
	items []query.Catalyst
	next  query.Where
}

func (b *ClusterBuilder) And() *ClusterBuilder { b.next = query.And; return b }
func (b *ClusterBuilder) Or() *ClusterBuilder  { b.next = query.Or; return b }
func (b *ClusterBuilder) Where(c query.Catalyst) *ClusterBuilder {
	c.Where = b.next
	b.next = query.And
	b.items = append(b.items, c)
	return b
}
func (b *ClusterBuilder) Compare(l query.Operand, op query.Operator, r query.Operand) *ClusterBuilder {
	return b.Where(query.Compare(l, op, r))
}
func (b *ClusterBuilder) predicate(column string, op query.Operator, v any, options ...PredicateOption) *ClusterBuilder {
	c := query.Catalyst{Column: column, Operator: op, Value: v}
	for _, o := range options {
		if o != nil {
			o(&c)
		}
	}
	return b.Where(c)
}
func (b *ClusterBuilder) Eq(c string, v any) *ClusterBuilder { return b.predicate(c, query.Eq, v) }
func (b *ClusterBuilder) NotEq(c string, v any) *ClusterBuilder {
	return b.predicate(c, query.NotEq, v)
}
func (b *ClusterBuilder) Gt(c string, v any) *ClusterBuilder  { return b.predicate(c, query.Gt, v) }
func (b *ClusterBuilder) Gte(c string, v any) *ClusterBuilder { return b.predicate(c, query.Gte, v) }
func (b *ClusterBuilder) Lt(c string, v any) *ClusterBuilder  { return b.predicate(c, query.Lt, v) }
func (b *ClusterBuilder) Lte(c string, v any) *ClusterBuilder { return b.predicate(c, query.Lte, v) }
func (b *ClusterBuilder) StartsWith(c string, v string, o ...PredicateOption) *ClusterBuilder {
	return b.predicate(c, query.StartsWith, v, o...)
}
func (b *ClusterBuilder) StringContains(c string, v string, o ...PredicateOption) *ClusterBuilder {
	return b.predicate(c, query.StringContains, v, o...)
}
func (b *ClusterBuilder) StringEquals(c string, v string, o ...PredicateOption) *ClusterBuilder {
	return b.predicate(c, query.StringEquals, v, o...)
}
func (b *ClusterBuilder) ArrayContains(c string, v any, o ...PredicateOption) *ClusterBuilder {
	return b.predicate(c, query.In, v, o...)
}
func (b *ClusterBuilder) Contains(c string, v any) *ClusterBuilder {
	return b.predicate(c, query.Contains, v)
}
func (b *ClusterBuilder) IsNull(c string) *ClusterBuilder { return b.predicate(c, query.IsNull, nil) }
func (b *ClusterBuilder) IsString(c string) *ClusterBuilder {
	return b.predicate(c, query.IsString, nil)
}
func (b *ClusterBuilder) Defined(c string) *ClusterBuilder { return b.predicate(c, query.Defined, nil) }
func (b *ClusterBuilder) NotDefined(c string) *ClusterBuilder {
	return b.predicate(c, query.NotDefined, nil)
}
func (b *ClusterBuilder) In(c string, v any, o ...PredicateOption) *ClusterBuilder {
	return b.predicate(c, query.In, v, o...)
}
func (b *ClusterBuilder) NotIn(c string, v any, o ...PredicateOption) *ClusterBuilder {
	return b.predicate(c, query.NotIn, v, o...)
}
func (b *ClusterBuilder) NotContains(c string, v any) *ClusterBuilder {
	return b.predicate(c, query.NotContains, v)
}
func (b *ClusterBuilder) Like(c string, v string) *ClusterBuilder {
	return b.predicate(c, query.Like, v)
}
func (b *ClusterBuilder) NotLike(c string, v string) *ClusterBuilder {
	return b.predicate(c, query.NotLike, v)
}
func (b *ClusterBuilder) Len(c string, v int) *ClusterBuilder { return b.predicate(c, query.Len, v) }
func (b *ClusterBuilder) VectorDistance(c string, v []float64) *ClusterBuilder {
	return b.predicate(c, query.VectorDistance, v)
}
func (b *ClusterBuilder) FTContains(c string, v string) *ClusterBuilder {
	return b.predicate(c, query.FTContains, v)
}
func (b *ClusterBuilder) NotFTContains(c string, v string) *ClusterBuilder {
	return b.predicate(c, query.NotFTContains, v)
}
func (b *ClusterBuilder) FTContainsAll(c string, v []string) *ClusterBuilder {
	return b.predicate(c, query.FTContainsAll, v)
}
func (b *ClusterBuilder) NotFTContainsAll(c string, v []string) *ClusterBuilder {
	return b.predicate(c, query.NotFTContainsAll, v)
}
func (b *ClusterBuilder) FTContainsAny(c string, v []string) *ClusterBuilder {
	return b.predicate(c, query.FTContainsAny, v)
}
func (b *ClusterBuilder) NotFTContainsAny(c string, v []string) *ClusterBuilder {
	return b.predicate(c, query.NotFTContainsAny, v)
}
func (b *ClusterBuilder) FTScore(c string, v []string) *ClusterBuilder {
	return b.predicate(c, query.FTScore, v)
}
