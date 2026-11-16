# Query execution, paging, and consistency

[Documentation home](../README.md) · [Fluent builder](FLUENT_QUERY_BUILDER.md) · [Cross-partition policy](CROSS_PARTITION.md)

Gosmos separates query generation from execution. The builder validates SQL inputs; execution applies partition policy, consistency, hints, and SDK paging.

## Choose the operation

| Operation | Fetch behavior | Missing result |
|---|---|---|
| `Galaxy.Get(ctx, id, pk)` | SDK point read | `ErrNotFound` |
| `Orbit.List(ctx)` | Consume all service pages | Empty slice |
| `Orbit.Get(ctx)` | Stop after first decoded item | `ErrNotFound` |
| `Orbit.Paged(ctx, page)` | Read through empty pages to a non-empty page | Empty slice when exhausted |
| `GenerateQuery()` | No network request | Returns statement or validation error |

Use `ListAs[R]`, `GetAs[R]`, and `PagedAs[R]` for results that differ from the repository entity. They deserialize into `R`; they do not infer the selected query shape.

## Supply a complete partition key

```go
rows, gravity, err := orders.Query().InPartition("tenant-1").List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

For a two-level hierarchical key, supply both values in schema order: `InPartition("tenant-1", "APAC")`. A prefix is not a complete logical partition. Point reads take the same ordered components as a `[]string`.

Without a key, the SDK gateway handles eligible simple shapes. `WithCrossPartition(false)` disables that path entirely. The [shape matrix](CROSS_PARTITION.md) describes which queries require a key.

`GenerateQuery` does not enforce that matrix: it can return SQL for a shape that execution would reject without a partition key.

## Page through results

```go
token := ""
for {
    rows, gravity, err := orders.Query().
        Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("status", "open") }).
        OrderBy("id").
        InPartition("tenant-1").
        Paged(ctx, query.Page{Size: 100, ContinuationToken: token})
    if err != nil {
        return err
    }
    for _, row := range rows {
        _ = row.ID
    }
    if gravity.ContinuationToken == "" {
        break
    }
    if gravity.ContinuationToken == token {
        return errors.New("query returned a repeated continuation token")
    }
    token = gravity.ContinuationToken
}
```

Create a fresh builder for each operation. Keep the query shape, parameters, consistency, and partition key stable while following a token.

Page size accepts 1 through 1,000 and maps to the SDK's page-size hint. It is not a promise of exactly that many returned rows. The service can produce empty pages; Gosmos continues until it finds a non-empty page or reaches exhaustion. RU includes every page consumed, including empty pages.

`GROUP BY` is the exception: paged execution consumes the available pages for the grouped query instead of stopping at the first non-empty page. Do not use grouped paging as an ordinary page-size boundary.

Tokens are opaque SDK values, not Gosmos cursors. Gosmos does not encode sort positions or merge partition-range tokens.

## Single-result queries

```go
order, gravity, err := orders.Query().
    Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("status", "open") }).
    InPartition("tenant-1").Get(ctx)
if err != nil {
    return err
}
_ = order
_ = gravity
```

Query-based `Get` requests one item per service page and reads past empty pages. It stops at the first decoded item. Its SQL shape depends on whether a structured projection is present:

- **Without `Project`:** filters and selected columns remain; TOP becomes 1, while distinct, aggregate, join, sorting, grouping, and hints are cleared.
- **With `Project`:** the full configured query shape and hints remain; an explicit TOP is retained, otherwise TOP 1 is added.

Consequently, `OrderBy(...).Get(ctx)` without `Project` does not mean “the first item in that sort order.” For an ordered entity result, execute `Top(1).List(ctx)` and handle an empty result. For an ordered projected result, use `Project` with `GetAs`.

All query-based Get paths contain TOP and require a complete partition key. For a known document ID, prefer the point-read API.

## Select read consistency

Set a repository default:

```go
orders, err := gosmos.New[Order](client, "shop", "orders",
    gosmos.WithConsistency(azcosmos.ConsistencyLevelSession))
if err != nil {
    return err
}
```

Override one point read:

```go
level := azcosmos.ConsistencyLevelStrong
order, gravity, err := orders.Get(ctx, "order-1", []string{"tenant-1"},
    gosmos.ReadOptions{ConsistencyLevel: &level})
if err != nil {
    return err
}
_ = order
_ = gravity
```

Override one query:

```go
rows, gravity, err := orders.Query().InPartition("tenant-1").
    WithConsistency(azcosmos.ConsistencyLevelSession).List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

Precedence is per-operation override, then repository default, then SDK/account behavior. Nil means no override at that level. Supported values are Strong, BoundedStaleness, Session, ConsistentPrefix, and Eventual.

An override cannot strengthen account consistency. Gosmos forwards the requested level and preserves service errors; it does not silently substitute another level or mutate the shared client's settings.

Any explicit level, including a repository default, bypasses document-cache lookup and population. It does not refresh existing entries, even if the chosen level is the same as the account default. Invalid levels fail before cache access.

## Apply execution hints

```go
rows, gravity, err := orders.Query().InPartition("tenant-1").
    WithHints(gosmos.Hints{
        Strategy:                          gosmos.SinglePartition,
        MaxItemCount:                      100,
        ResponseContinuationTokenLimitInKB: 4,
    }).List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

| Hint | Behavior |
|---|---|
| `Strategy: Auto` | No additional constraint; key and query shape determine execution scope |
| `Strategy: SinglePartition` | Requires a partition key |
| `Strategy: Gateway` | Does not force a different scope or bypass the shape guard |
| `MaxItemCount` | SDK page-size hint; 0 means unset, maximum 1,000 |
| `ResponseContinuationTokenLimitInKB` | SDK token-size hint; 0 means unset, maximum 16 |
| `MaxConcurrency` | Nonnegative compatibility setting; no execution effect |
| `MaxBufferedItemCount` | Nonnegative compatibility setting; no execution effect |
| `EnableOptimisticDirectExecution` | Compatibility setting; no execution effect |

Strategy labels are validation/compatibility hints. They do not override partition-key-based scope selection or switch the SDK's transport. `SinglePartition` adds a key requirement; `Gateway` does not remove an existing key. Bulk transaction concurrency is separate and fixed at four partition workers.

`Paged` and nonzero `Hints` cannot be combined. Use `query.Page.Size` for the page-size hint in a paged call. For unpaged `List`, `MaxItemCount` affects individual service pages rather than total results; use TOP to limit the query result count.

## Understand cost and cancellation

`Gravity.RU` accumulates request charges across fetched pages. A cache hit returns zero RU. If a query fails after consuming earlier pages, its returned metadata retains the charges accumulated so far.

Each network operation receives your context. A single `List` can fetch many pages, so choose a deadline for the complete operation rather than assuming one page equals one request. Statistics recording uses a separate bounded context and reports its errors through `Gravity.StatisticsError`.

`Orbit`, patch, and batch builders are mutable and should not be shared across goroutines. A configured repository can be shared; create a new builder for each request.

[Next: caching and statistics →](CACHING_AND_STATISTICS.md)
