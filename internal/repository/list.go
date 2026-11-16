package repository

import (
	"context"

	"github.com/norarrsgd/gosmos/query"
)

type ListOptions struct {
	Columns      query.ColumnOptions
	Sorting      []query.Sorting
	Groups       []string
	PartitionKey []string
	ReadOptions  ReadOptions
	Hints        Hints
}

func (g *Galaxy[T]) List(ctx context.Context, clusters []query.Cluster, opts ListOptions) ([]T, Gravity, error) {
	return (&Orbit[T]{g: g, input: query.Input{Clusters: clusters, Columns: opts.Columns, Sorting: opts.Sorting, Groups: opts.Groups}, pk: opts.PartitionKey, read: opts.ReadOptions, hints: opts.Hints}).List(ctx)
}

func (g *Galaxy[T]) GenerateQuery(clusters []query.Cluster, opts ListOptions) (query.Statement, error) {
	return (&Orbit[T]{g: g, input: query.Input{Clusters: clusters, Columns: opts.Columns, Sorting: opts.Sorting, Groups: opts.Groups}}).GenerateQuery()
}

func ListAs[R any, T Entity](ctx context.Context, o *Orbit[T]) ([]R, Gravity, error) {
	return executeQuery[R](ctx, o, nil, false)
}

func PagedAs[R any, T Entity](ctx context.Context, o *Orbit[T], p query.Page) ([]R, Gravity, error) {
	return executeQuery[R](ctx, o, &p, false)
}

func GetAs[R any, T Entity](ctx context.Context, o *Orbit[T]) (R, Gravity, error) {
	var zero R
	copy := *o
	copy.input = o.input
	if copy.input.Columns.Projection == nil {
		copy.input.Columns.Top = 1
		copy.input.Columns.IsDistinct = false
		copy.input.Columns.Aggregates = nil
		copy.input.Columns.Join = nil
		copy.input.Sorting = nil
		copy.input.Groups = nil
		copy.hints = Hints{}
	} else if copy.input.Columns.Top == 0 {
		copy.input.Columns.Top = 1
	}
	rows, g, e := executeQuery[R](ctx, &copy, nil, true)
	if e != nil {
		return zero, g, e
	}
	if len(rows) == 0 {
		return zero, g, wrapError("get", o.g.metadata.name, ErrNotFound)
	}
	return rows[0], g, nil
}
