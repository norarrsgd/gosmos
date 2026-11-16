# Cross-partition query policy

[Documentation home](../README.md) · [Execution and paging](QUERY_EXECUTION.md) · [Vector search](VECTOR_SEARCH.md)

Gosmos supports two execution scopes: a complete logical partition, or the SDK gateway's eligible cross-partition query path. It does not discover partition ranges, fan out queries, merge results, or create custom cross-partition cursors.

## Supported shapes

| Shape | Complete partition key | No partition key |
|---|---|---|
| Equality/range/string/type filters | Execute | Gateway pass-through |
| Selected fields and object projections | Execute | Gateway pass-through |
| Plain document-array JOIN | Execute | Gateway pass-through |
| ORDER BY | Execute | Reject |
| TOP | Execute | Reject |
| DISTINCT | Execute | Reject |
| COUNT, SUM, MIN, MAX, AVG | Execute | Reject |
| GROUP BY | Execute | Reject |
| Aggregate nested inside a projection | Execute | Reject |
| Vector distance ranking | Execute | Reject |
| Full-text ranking, RRF, hybrid ranking | Execute | Reject |
| Query-based Get / GetAs | Execute | Reject: generates TOP 1 |
| Paged queries | Execute subject to shape | Pass-through for eligible simple shapes |

“Execute” means Gosmos submits the shape to the SDK. Account features, indexes, data shape, and query validity still govern the service result. A complete partition key does not create vector or full-text policies.

## Scope a query

```go
rows, gravity, err := orders.Query().
    OrderBy("addedOn").Top(25).
    InPartition("tenant-1").List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

Without `InPartition`, ordering and TOP are rejected before a request. For hierarchical keys, provide every component in schema order; a tenant prefix alone is not a complete logical partition key.

The guard inspects query structure, including aggregates inside nested projections. It does not infer partition scope from WHERE clauses. Even `Eq("tenantId", "tenant-1")` remains a keyless query unless `InPartition` is supplied.

## Disallow all keyless queries

Queries default to cross-partition execution, so a forgotten `InPartition` call runs across every tenant. Multi-tenant services should disable it and make partition scope mandatory.

```go
orders, err := gosmos.New[Order](client, "shop", "orders",
    gosmos.WithCrossPartition(false))
if err != nil {
    return err
}
```

This rejects even simple keyless filters. The default is to permit eligible gateway shapes. Use the option where repository callers should always supply a full key.

## Handle a rejected shape

```go
_, _, err := orders.Query().OrderBy("addedOn").List(ctx)
if errors.Is(err, gosmos.ErrUnsupportedCrossPartition) {
    // Supply a complete partition key or choose a supported execution path.
    return err
}
```

The error names the unsupported shapes. Local rejection makes no request and consumes no service RU. Gateway failures after an allowed request retain their SDK errors; HTTP 400 responses are not automatically relabeled as cross-partition failures.

`GenerateQuery` remains available without a partition key because it does not execute. `Gateway` strategy hints cannot bypass the guard, and `SinglePartition` requires a key.

## Boundaries

Gosmos does not offer ordered cross-partition continuation tokens, aggregate merging, vector top-k merging, or relevance-score fusion across caller-managed partition queries. If an application performs separate partition queries, the application owns ordering, limits, duplicate handling, consistency, cost, and continuation semantics for any combined result.

[Read paging and consistency behavior →](QUERY_EXECUTION.md)
