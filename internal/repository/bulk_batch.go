package repository

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

// BulkBatch executes ordered transaction chunks per partition, at most four partitions at once.
// Successfully committed chunks remain committed if another chunk fails.
type BulkBatch[T Entity] struct{ builder batchBuilder[T] }

func (g *Galaxy[T]) Bulk() *BulkBatch[T] { return &BulkBatch[T]{builder: batchBuilder[T]{galaxy: g}} }

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
	var groups [][]int
	for i, operation := range b.builder.operations {
		found := -1
		for j, group := range groups {
			if reflect.DeepEqual(operation.key, b.builder.operations[group[0]].key) {
				found = j
				break
			}
		}
		if found < 0 {
			groups = append(groups, []int{i})
		} else {
			groups[found] = append(groups[found], i)
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
	for _, group := range groups {
		jobs <- group
	}
	close(jobs)
	wg.Wait()
	return result, errors.Join(failures...)
}
