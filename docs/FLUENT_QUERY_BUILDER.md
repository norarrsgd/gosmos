# Fluent query builder

[Documentation home](../README.md) · [Declarative queries](DECLARATIVE_QUERY_SYNTAX.md) · [Execution and paging](QUERY_EXECUTION.md)

`Galaxy.Query()` creates an `Orbit[T]`: a mutable builder for predicates, selected fields, projections, sorting, and execution options. Configuration methods return the builder; execution methods return data, `Gravity`, and an error.

Examples assume `orders` is a `*gosmos.Galaxy[Order]` from [Getting started](GETTING_STARTED.md), with `ctx` supplied by the caller. Import `github.com/norarrsgd/gosmos/query` for operators, operands, and projection options.

## Build a filtered query

```go
rows, gravity, err := orders.Query().
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.Eq("status", "open").And().Gte("total", 100)
    }).
    InPartition("tenant-1").
    List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity.RU
```

The corresponding WHERE expression uses bound values:

```sql
WHERE (c["status"] = @p0 AND c["total"] >= @p1)
```

SQL in this guide uses normalized parameter names for readability. Actual names are opaque and come from the statement's parameter collection.

## Choose a predicate

| Helper | Input | Meaning |
|---|---|---|
| `Eq`, `NotEq` | Non-null value | Equality or inequality |
| `Gt`, `Gte`, `Lt`, `Lte` | Non-null value | Ordered comparison |
| `StartsWith` | String | Literal prefix |
| `StringContains` | String | Literal substring |
| `StringEquals` | String | String equality with optional case handling |
| `Like`, `NotLike` | Pattern containing `%` | SQL pattern matching |
| `Defined`, `NotDefined` | Path only | Property exists or is missing |
| `IsNull`, `IsString` | Path only | JSON type predicate |
| `ArrayContains`, `In`, `NotIn` | Element or object pattern | Search a document array |
| `Contains`, `NotContains` | Caller-supplied collection | Search supplied values for a document field |
| `Len` | Integer | Array length equals a bound value |
| `Compare` | Operands and comparison operator | Field/array-length/value comparison |

Search-specific helpers are described in [Vector search](VECTOR_SEARCH.md) and [Full-text search](FULLTEXT_SEARCH.md).

### Literal text and case handling

```go
orbit := orders.Query().Cluster(func(c *gosmos.ClusterBuilder) {
    c.StartsWith("status", "op", query.IgnoreCase).
        And().StringContains("status", "pen", query.IgnoreCase)
})
statement, err := orbit.GenerateQuery()
if err != nil {
    return err
}
_ = statement
```

`IgnoreCase` applies only to `StartsWith`, `StringContains`, and `StringEquals`. Literal text helpers do not interpret `%` or `_` as wildcard characters. For pattern matching, use `Like("status", "op%")` instead.

### Null and missing properties

```go
orbit := orders.Query().Cluster(func(c *gosmos.ClusterBuilder) {
    c.IsNull("modifiedOn").Or().NotDefined("modifiedOn")
})
_ = orbit
```

A missing property and an explicit JSON null are different. `IsNull` matches the latter; `NotDefined` matches the former. These unary helpers bind no value. Ordinary comparisons do not accept a null parameter.

### Membership has two directions

```go
orbit := orders.Query().Cluster(func(c *gosmos.ClusterBuilder) {
    c.Contains("status", []string{"open", "paid"})
})
_ = orbit
```

This checks whether the caller's array contains the document's status:

```sql
ARRAY_CONTAINS(@p0, c["status"])
```

For documents with a stored `tags` array, use `ArrayContains("tags", "priority")`:

```sql
ARRAY_CONTAINS(c["tags"], @p0)
```

For an array of objects, `ArrayContains("shares", map[string]any{"userId": "user-1"}, query.PartialMatch)` matches an object containing those properties. The whole pattern is one bound value. `PartialMatch` is valid only with `In`/`NotIn` and the `ArrayContains` helper that uses `In`.

### Compare fields and array lengths

```go
orbit := orders.Query().Cluster(func(c *gosmos.ClusterBuilder) {
    c.Compare(query.Field("total"), query.Gt, query.Parameter(100))
})
_ = orbit
```

`Field` and `ArrayLength` can appear on either side. `Parameter` is allowed only on the right. Operators are limited to `Eq`, `NotEq`, `Gt`, `Gte`, `Lt`, and `Lte`. Field-to-field comparisons create no parameter binding.

For a document schema containing `items` and `processedCount`:

```go
comparison := query.Compare(query.ArrayLength("items"), query.Gt,
    query.Field("processedCount"))
_ = comparison
```

That expression renders as `ARRAY_LENGTH(c["items"]) > c["processedCount"]`.

## Group boolean expressions

Each `Cluster` is parenthesized. `And()` and `Or()` set the connector for the next predicate; absent a connector, predicates use AND. Multiple fluent `Cluster` calls are joined by AND.

```go
orbit := orders.Query().
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.Eq("status", "open").Or().Eq("status", "paid")
    }).
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.Gte("total", 100).And().Lt("total", 500)
    })
_ = orbit
```

```sql
WHERE (c["status"] = @p0 OR c["status"] = @p1)
  AND (c["total"] >= @p2 AND c["total"] < @p3)
```

Within one cluster, ordinary SQL AND/OR precedence applies. The fluent API has no recursive group builder. Use [declarative clusters](DECLARATIVE_QUERY_SYNTAX.md#combine-clusters) for OR between groups, or normalize a deeper expression into equivalent groups.

## Sort, limit, and deduplicate

```go
orbit := orders.Query().
    Select("status").Distinct().Top(10).
    OrderBy("status").
    InPartition("tenant-1")
_ = orbit
```

`OrderByDescending` adds descending order. Multiple sorting calls preserve their order. `Top` accepts 1 through 1,000. `Distinct` applies to the selected result shape, not to a separate identity key.

Ordering, TOP, and DISTINCT require a complete partition key for execution. SQL generation itself does not require a key.

## Projections

### Select source fields

```go
orbit := orders.Query().Select("id", "status", "total")
_ = orbit
```

Selected fields retain their source names. Use nested JSON paths such as `customer.name` for source traversal; Gosmos renders each segment in bracket notation.

### Select by result type

```go
type OrderSummary struct {
    ID     string  `json:"id"`
    Status string  `json:"status"`
    Total  float64 `json:"total"`
    Local  string  `json:"-"`
}
```

```go
orbit := gosmos.SelectAs[OrderSummary](orders.Query()).InPartition("tenant-1")
rows, gravity, err := gosmos.ListAs[OrderSummary](ctx, orbit)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

`SelectAs` uses exported JSON field names and ignores `json:"-"`. It selects fields; `ListAs` chooses the type used to deserialize results. `ListAs` alone does not change the selected SQL shape.

### Build named objects

```go
type OrderCard struct {
    ID   string `json:"id"`
    Data struct {
        State  string  `json:"state"`
        Amount float64 `json:"amount"`
    } `json:"data"`
}
```

```go
orbit := orders.Query().Select("id").
    Project(func(p *query.ProjectionBuilder) {
        p.Object("data", func(data *query.ProjectionBuilder) {
            data.Field("state", "status").Field("amount", "total")
        })
    }).InPartition("tenant-1")
rows, gravity, err := gosmos.ListAs[OrderCard](ctx, orbit)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

```sql
SELECT c["id"], {"state": c["status"], "amount": c["total"]} AS data FROM c
```

`Field(outputName, sourcePath)` separates result names from source paths. `Object` nests a projection. `Aggregate` adds a named aggregate. Output names remain exact; naming policies transform only source paths. Root output names must be valid SQL aliases. Nested object keys are JSON-escaped. Empty projections, duplicate names, and output collisions are rejected.

## Aggregate and group

```go
type StatusTotal struct {
    Status string  `json:"status"`
    Total  float64 `json:"total_Sum"`
}
```

```go
orbit := orders.Query().Select("status").Aggregate("total", query.Sum).
    InPartition("tenant-1")
rows, gravity, err := gosmos.ListAs[StatusTotal](ctx, orbit)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

```sql
SELECT c["status"], SUM(c["total"]) AS total_Sum FROM c GROUP BY c["status"]
```

Supported aggregates are `Count`, `Sum`, `Min`, `Max`, and `Avg`. Ordinary aggregate aliases use `<column>_<aggregate>`. Selected non-aggregate fields become grouping keys. `GroupBy` adds explicit keys. In a structured projection, use `Aggregate("revenue", "total", query.Sum)` to choose an output name.

Grouped or aggregate queries reject scalar ordering and ranking. They also require a partition key at execution time.

## Join an array

For a stored document array `items` with fields `sku` and `price`:

```go
orbit := orders.Query().Select("id").Join(query.JoinOptions{
    Alias:     "item",
    ArrayPath: "items",
    Columns:   []string{"sku", "price"},
}).InPartition("tenant-1")
_ = orbit
```

This adds `JOIN item IN c["items"]` and selects the requested joined fields. A join expands document arrays; it does not join separate containers. Joined field comparisons can use `query.Field("price", "item")` as an operand. Join aliases, source paths, and selected fields are validated.

## Generate without executing

```go
statement, err := orders.Query().Select("id").Top(10).GenerateQuery()
if err != nil {
    return err
}
_ = statement.Text
_ = statement.Params
_ = statement.Shape
```

`GenerateQuery` performs no I/O and does not apply the cross-partition execution guard. Build an unsupported cross-partition shape to inspect its SQL, then supply `InPartition` before executing it.

## Choose an execution method

| Method | Result |
|---|---|
| `List(ctx)` / `ListAs[R](ctx, orbit)` | Fetch all query pages |
| `Get(ctx)` / `GetAs[R](ctx, orbit)` | Fetch one result; missing result wraps `ErrNotFound` |
| `Paged(ctx, query.Page{...})` / `PagedAs[R]` | Fetch a non-empty service page, with grouped-query exception |
| `GenerateQuery()` | Return a statement without requests |

Query `Get` has specific shape rules; it is not simply `List` followed by taking the first element. Read [single-result execution](QUERY_EXECUTION.md#single-result-queries) before combining it with grouping, sorting, or projections.

[Next: query execution and paging →](QUERY_EXECUTION.md)
