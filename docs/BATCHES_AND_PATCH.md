# Transactions, bulk operations, and patch

[Documentation home](../README.md) · [Entity and ETag setup](GETTING_STARTED.md) · [Caching](CACHING_AND_STATISTICS.md)

Choose `Atomic` for one same-partition transaction, `Bulk` for partition-chunked work, and `Patch` for targeted document changes. All execution accepts a caller context and returns operation metadata.

## Pick the right boundary

| API | Transaction boundary | Limits | Failure behavior |
|---|---|---|---|
| `Atomic(pk...)` | All operations in one logical partition | 100 operations, 2 MiB estimated payload | One transaction; no chunking |
| `Bulk()` | Each chunk within one logical partition | 100 operations and 2 MiB per chunk | Earlier chunks may remain committed |
| `Galaxy.Patch` | One document update | 10 patch operations | SDK item error |

Bulk processes at most four partitions concurrently. Chunks for a partition execute in original operation order. The four-worker limit is not changed by query execution hints.

## Replace with an ETag

Examples use the `Order` entity from [Getting started](GETTING_STARTED.md), an `orders` repository, and a caller-provided `ctx`.

```go
order, read, err := orders.Get(ctx, "order-1", []string{"tenant-1"})
if err != nil {
    return err
}
order.Status = "paid"
result, err := orders.Atomic("tenant-1").
    Replace(order, gosmos.IfMatch(read.ETag)).Execute(ctx)
if err != nil {
    return err
}
replacementETag := result.Items[0].ETag
_ = replacementETag
```

Read the ETag from the operation that produced the new document version. Reusing an older read ETag after a successful replacement or patch can fail with `ErrPreconditionFailed`.

`Replace`, `Remove`, and `Patch` accept `gosmos.IfMatch`. `IfMatch("")` is invalid; omit the option only when you intend an unconditional operation.

## Mix operations in one transaction

```go
batch := orders.Atomic("tenant-1")
batch.Create(Order{TenantID: "tenant-1", Status: "open", Total: 10})
batch.Patch("order-2", func(p *gosmos.Patch[Order]) {
    p.Set("status", "paid")
})
batch.Read("order-2")
result, err := batch.Execute(ctx)
if err != nil {
    return err
}
for _, item := range result.Items {
    _ = item.Index
    _ = item.Status
    _ = item.ETag
    _ = item.Committed
    _ = item.DecodeError
    _ = item.Item
}
```

Supported operation builders are `Create`, `Replace`, `Upsert`, `Read`, `Remove`, and `Patch`. Model operations derive partition values from the model. Those values must match the key passed to `Atomic`. Point operations use the transaction's key.

`Create` can generate IDs and creation timestamps through entity capabilities; `Replace` can stamp modification time. Do not rely on a patch to invoke the entity's stamping hooks.

Oversized or invalid atomic batches fail before a request. Gosmos does not split an atomic batch because that would change its transaction boundary. A service-rolled-back transaction does not turn nominally successful per-operation statuses into committed successes: inspect each operation's `Err` together with the overall error.

## Handle partial bulk success

```go
bulk := orders.Bulk()
bulk.Create(Order{TenantID: "tenant-1", Status: "open", Total: 10})
bulk.Create(Order{TenantID: "tenant-2", Status: "open", Total: 20})
result, executeErr := bulk.Execute(ctx)
for _, item := range result.Items {
    if !item.Committed {
        // Record or reconcile this operation using its original index.
        continue
    }
    if item.DecodeError != nil {
        // The operation committed, but the returned document was incompatible.
        continue
    }
    _ = item.Item.GetID()
}
if executeErr != nil {
    return executeErr
}
```

The returned items preserve the original operation order, even when partitions execute concurrently. Check the result when an error is returned: it may contain confirmed commits as well as failures or unattempted operations.

For point operations, bulk builders take explicit key components:

```go
bulk := orders.Bulk()
bulk.Read("order-1", []string{"tenant-1"})
bulk.Remove("order-2", []string{"tenant-2"})
bulk.Patch("order-3", []string{"tenant-1"}, func(p *gosmos.Patch[Order]) {
    p.Set("status", "closed")
})
_ = bulk
```

A failed chunk stops later chunks in that partition. Other partition pipelines can continue. Cancellation does not undo earlier committed chunks. Returned unattempted items have an error rather than being reported as successful operations.

Do not blindly retry the complete bulk request after partial success. Reconcile confirmed writes and retry only the operations your application considers safe. A transport failure may leave the commit outcome uncertain; an error alone is not proof that no write occurred.

## Patch one document

```go
gravity, err := orders.Patch(ctx, "order-1", []string{"tenant-1"},
    func(p *gosmos.Patch[Order]) {
        p.Set("status", "paid").Set("total", 150)
    },
    gosmos.IfMatch(read.ETag))
if err != nil {
    return err
}
_ = gravity.ETag
```

| Patch helper | Meaning |
|---|---|
| `Set(path, value)` | Set a property; may add it |
| `Add(path, value)` | SDK add operation |
| `Replace(path, value)` | Replace an existing property |
| `Remove(path)` | Remove a property |
| `Increment(path, int64)` | Increment a numeric property by an integer |

Service rules still determine whether an operation is valid for the existing JSON value. Path validation checks the Go entity structure, not the current stored document.

Use JSON names in dot notation, such as `data.status`, or JSON Pointer notation, such as `/data/status`. JSON Pointer escapes `~0` and `~1` are supported. Array traversal uses numeric indices; `Add` permits the array append marker where applicable. Map keys can be traversed when the model declares a map.

Paths cannot modify `id` or root system properties beginning with `_`. Empty paths, unknown typed properties, traversal through scalars, malformed pointer escapes, and invalid array indices fail validation. The library does not provide conditional patch filter predicates; use ETag preconditions for version checks.

### Write JSON null

```go
patch := new(gosmos.Patch[Order])
patch.Set("modifiedOn", nil)
operations, err := patch.Build()
if err != nil {
    return err
}
_ = operations
```

A nil value in `Set`, `Add`, or `Replace` is serialized as JSON null. It is different from `Remove`, which deletes the property.

### Validate before execution

`Patch.Build` returns SDK patch operations and a validation error without requests. It requires at least one operation and permits at most ten. Fluent patch and batch builders retain validation failures; subsequent calls do not repair an already-invalid builder. Start a new builder after invalid input.

## Read batch results

`BatchResult[T]` contains `Items` and aggregate `Gravity`. Each item contains:

| Field | Meaning |
|---|---|
| `Index` | Position in the original input sequence |
| `Item` | Prepared or returned document, when available |
| `Status` | Per-operation HTTP status; zero for unattempted operations |
| `ETag` | Returned document ETag, when available |
| `Committed` | Whether the service confirmed a successful transaction and 2xx operation status |
| `DecodeError` | Local error decoding a returned resource body after commit |
| `Err` | Operation failure, rollback, or unattempted reason |

A batch-level validation failure marks included operations as failed/unattempted. SDK response errors remain reachable through the error chain. An item can contain its prepared entity even when it was not committed; do not treat a populated `Item.ID` as a success indicator.

A `DecodeError` does not change `Committed`, populate `Err`, or make `Execute` return an operation error. Do not retry a committed operation because its returned resource body could not be decoded. On decode failure, `Item` retains the prepared entity when one was available rather than exposing a partially decoded document.

## Cache invalidation

Mutating requests invalidate the repository's cached reads. Batch and patch execution clear cached entries conservatively, including transport failures with uncertain outcomes. A successful chunk followed by a later failure cannot leave the repository assuming that no data changed.

Changes made by another process or client are still subject to the cache TTL. [Read the cache policy →](CACHING_AND_STATISTICS.md#document-cache)
