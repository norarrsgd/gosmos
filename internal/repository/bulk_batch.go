package repository

import (
	"context"
	"errors"
	"sync"
)

// BulkBatch accepts at most 10,000 operations or 64 MiB of estimated payload and executes
// ordered transaction chunks per partition, at most four partitions at once.
// Successfully committed chunks remain committed if another chunk fails.
type BulkBatch[T Entity] struct{ builder batchBuilder[T] }

func (g *Galaxy[T]) Bulk() *BulkBatch[T] {
	return &BulkBatch[T]{builder: batchBuilder[T]{
		galaxy:         g,
		operationLimit: bulkMaxOperations,
		byteLimit:      bulkMaxBytes,
	}}
}

func (b *BulkBatch[T]) Create(item T, o ...WriteOption) *BulkBatch[T] {
	b.builder.model("create", item, o)
	return b
}

func (b *BulkBatch[T]) Replace(item T, o ...WriteOption) *BulkBatch[T] {
	b.builder.model("replace", item, o)
	return b
}

func (b *BulkBatch[T]) Upsert(item T, o ...WriteOption) *BulkBatch[T] {
	b.builder.model("upsert", item, o)
	return b
}

func (b *BulkBatch[T]) Read(id string, pk []string, o ...WriteOption) *BulkBatch[T] {
	b.builder.point("read", id, pk, nil, o)
	return b
}

func (b *BulkBatch[T]) Remove(id string, pk []string, o ...WriteOption) *BulkBatch[T] {
	b.builder.point("delete", id, pk, nil, o)
	return b
}

func (b *BulkBatch[T]) Patch(id string, pk []string, p func(*Patch[T]), o ...WriteOption) *BulkBatch[T] {
	b.builder.point("patch", id, pk, p, o)
	return b
}

func (b *BulkBatch[T]) Execute(ctx context.Context) (BatchResult[T], error) {
	result, err := b.builder.initial()
	if err != nil {
		return result, err
	}
	groups := make([][]int, 0, len(b.builder.operations))
	groupIndexes := make(map[string]int, len(b.builder.operations))
	for i, operation := range b.builder.operations {
		if err := ctx.Err(); err != nil {
			return result.rejected(err)
		}
		groupIndex, ok := groupIndexes[operation.groupKey]
		if !ok {
			groupIndex = len(groups)
			groupIndexes[operation.groupKey] = groupIndex
			groups = append(groups, []int{i})
		} else {
			groups[groupIndex] = append(groups[groupIndex], i)
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []error
	jobs := make(chan []int)
	for range min(4, len(groups)) {
		wg.Go(func() {
			for group := range jobs {
				size := 1
				start := 0
				for start < len(group) {
					end := start
					size = 1
					for end < len(group) && end-start < 100 && size+b.builder.operations[group[end]].size <= batchMaxBytes {
						size += b.builder.operations[group[end]].size
						end++
					}
					ru, err := b.builder.executeChunk(ctx, group[start:end], &result)
					mu.Lock()
					result.Gravity.RU += ru
					if err != nil {
						failures = append(failures, err)
					}
					mu.Unlock()
					if err != nil {
						for _, index := range group[end:] {
							result.Items[index].Err = err
						}
						break
					}
					start = end
				}
			}
		})
	}
	nextGroup := 0
	var dispatchErr error
dispatch:
	for nextGroup < len(groups) {
		if err := ctx.Err(); err != nil {
			dispatchErr = err
			break
		}
		select {
		case jobs <- groups[nextGroup]:
			nextGroup++
		case <-ctx.Done():
			dispatchErr = ctx.Err()
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
	if dispatchErr != nil {
		for _, group := range groups[nextGroup:] {
			for _, index := range group {
				result.Items[index].Err = dispatchErr
			}
		}
		failures = append(failures, dispatchErr)
	}
	return result, errors.Join(failures...)
}
