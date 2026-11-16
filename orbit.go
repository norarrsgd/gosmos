package gosmos

import (
	"context"
	"encoding/json"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos/query"
)

// Orbit builds a query. Create a separate Orbit for each concurrent operation.
type Orbit[T Entity] struct {
	g     *Galaxy[T]
	input query.Input
	pk    []string
	read  ReadOptions
	hints Hints
	err   error
}

func (g *Galaxy[T]) Query() *Orbit[T] { return &Orbit[T]{g: g} }
func (o *Orbit[T]) Cluster(fn func(*ClusterBuilder)) *Orbit[T] {
	b := new(ClusterBuilder)
	if fn != nil {
		fn(b)
	}
	o.input.Clusters = append(o.input.Clusters, query.Cluster{Catalysts: b.items})
	return o
}
func (o *Orbit[T]) InPartition(pk ...string) *Orbit[T] { o.pk = append([]string(nil), pk...); return o }
func (o *Orbit[T]) Top(n int) *Orbit[T] {
	if n < 1 {
		o.err = validation("TOP must be positive")
	}
	o.input.Columns.Top = n
	return o
}
func (o *Orbit[T]) Distinct() *Orbit[T] { o.input.Columns.IsDistinct = true; return o }
func (o *Orbit[T]) Select(columns ...string) *Orbit[T] {
	o.input.Columns.Names = append([]string(nil), columns...)
	return o
}
func (o *Orbit[T]) OrderBy(column string) *Orbit[T] {
	o.input.Sorting = append(o.input.Sorting, query.Sorting{Column: column, Direction: query.Asc})
	return o
}
func (o *Orbit[T]) OrderByDescending(column string) *Orbit[T] {
	o.input.Sorting = append(o.input.Sorting, query.Sorting{Column: column, Direction: query.Desc})
	return o
}
func (o *Orbit[T]) GroupBy(columns ...string) *Orbit[T] {
	o.input.Groups = append([]string(nil), columns...)
	return o
}
func (o *Orbit[T]) Aggregate(column string, ag query.Aggregate) *Orbit[T] {
	o.input.Columns.Aggregates = append(o.input.Columns.Aggregates, query.AggregationOption{Column: column, Aggregate: ag})
	return o
}
func (o *Orbit[T]) Join(join query.JoinOptions) *Orbit[T] { o.input.Columns.Join = &join; return o }
func (o *Orbit[T]) Project(fn func(*query.ProjectionBuilder)) *Orbit[T] {
	b := new(query.ProjectionBuilder)
	if fn != nil {
		fn(b)
	}
	p := b.Build()
	o.input.Columns.Projection = &p
	return o
}
func (o *Orbit[T]) WithConsistency(level azcosmos.ConsistencyLevel) *Orbit[T] {
	v, e := copyConsistency(&level)
	o.read.ConsistencyLevel = v
	if e != nil {
		o.err = e
	}
	return o
}
func (o *Orbit[T]) WithHints(h Hints) *Orbit[T] { o.hints = h; return o }
func (o *Orbit[T]) GenerateQuery() (query.Statement, error) {
	if o.err != nil {
		return query.Statement{}, o.err
	}
	input := o.input
	if input.NamingPolicy == nil {
		input.NamingPolicy = o.g.options.namingPolicy
	}
	s, e := query.Build(input)
	if e != nil {
		return s, validation("%s", e)
	}
	return s, nil
}
func (o *Orbit[T]) List(ctx context.Context) ([]T, Gravity, error) { return ListAs[T](ctx, o) }
func (o *Orbit[T]) Get(ctx context.Context) (T, Gravity, error)    { return GetAs[T](ctx, o) }
func (o *Orbit[T]) Paged(ctx context.Context, p query.Page) ([]T, Gravity, error) {
	return PagedAs[T](ctx, o, p)
}

// WithWeights supplies RRF weights. The query builder validates their rank count.
func (o *Orbit[T]) WithWeights(weights ...float64) *Orbit[T] {
	b, e := json.Marshal(weights)
	if e != nil {
		o.err = validation("invalid rank weights")
		return o
	}
	o.input.Sorting = append(o.input.Sorting, query.Sorting{Column: string(b), Direction: query.Weighted})
	return o
}
