package gosmos

import (
	"context"

	"github.com/norarrsgd/gosmos/internal/repository"
	"github.com/norarrsgd/gosmos/query"
)

// SelectAs selects the top-level JSON fields of a result struct.
func SelectAs[R any, T Entity](o *Orbit[T]) *Orbit[T] {
	return repository.SelectAs[R](o)
}

func ListAs[R any, T Entity](ctx context.Context, o *Orbit[T]) ([]R, Gravity, error) {
	return repository.ListAs[R](ctx, o)
}

func PagedAs[R any, T Entity](ctx context.Context, o *Orbit[T], p query.Page) ([]R, Gravity, error) {
	return repository.PagedAs[R](ctx, o, p)
}

func GetAs[R any, T Entity](ctx context.Context, o *Orbit[T]) (R, Gravity, error) {
	return repository.GetAs[R](ctx, o)
}
