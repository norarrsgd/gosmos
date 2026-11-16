package repository

import "context"

// AtomicBatch executes all operations in a single logical partition transaction.
type AtomicBatch[T Entity] struct{ builder batchBuilder[T] }

func (g *Galaxy[T]) Atomic(pk ...string) *AtomicBatch[T] {
	key, err := stringKey(pk, len(g.metadata.fields))
	return &AtomicBatch[T]{builder: batchBuilder[T]{galaxy: g, key: &key, err: err}}
}

func (b *AtomicBatch[T]) Create(item T, o ...WriteOption) *AtomicBatch[T] {
	b.builder.model("create", item, o)
	return b
}

func (b *AtomicBatch[T]) Replace(item T, o ...WriteOption) *AtomicBatch[T] {
	b.builder.model("replace", item, o)
	return b
}

func (b *AtomicBatch[T]) Upsert(item T, o ...WriteOption) *AtomicBatch[T] {
	b.builder.model("upsert", item, o)
	return b
}

func (b *AtomicBatch[T]) Read(id string, o ...WriteOption) *AtomicBatch[T] {
	b.builder.point("read", id, nil, nil, o)
	return b
}

func (b *AtomicBatch[T]) Remove(id string, o ...WriteOption) *AtomicBatch[T] {
	b.builder.point("delete", id, nil, nil, o)
	return b
}

func (b *AtomicBatch[T]) Patch(id string, p func(*Patch[T]), o ...WriteOption) *AtomicBatch[T] {
	b.builder.point("patch", id, nil, p, o)
	return b
}

func (b *AtomicBatch[T]) Execute(ctx context.Context) (BatchResult[T], error) {
	result, err := b.builder.initial()
	if err != nil {
		return result, err
	}
	size := 1
	for _, operation := range b.builder.operations {
		size += operation.size
	}
	if len(b.builder.operations) > 100 || size > batchMaxBytes {
		return result.rejected(b.builder.galaxy.failure("batch", validation("atomic batch exceeds 100 operations or 2 MiB")))
	}
	indexes := make([]int, len(result.Items))
	for i := range indexes {
		indexes[i] = i
	}
	ru, err := b.builder.executeChunk(ctx, indexes, &result)
	result.Gravity.RU = ru
	return result, err
}
