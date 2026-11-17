# Text search and hybrid ranking

[Documentation home](../README.md) · [Vector search](VECTOR_SEARCH.md) · [Fluent predicates](FLUENT_QUERY_BUILDER.md#choose-a-predicate)

Gosmos supports two different text-query families: literal string predicates and Cosmos full-text predicates/ranking. Choose literal predicates for substring or prefix matching; use full-text operators when the container has the relevant full-text policy and index.

## Choose the text operation

| Need | Helper | Full-text policy required |
|---|---|---|
| Literal prefix | `StartsWith` | No |
| Literal substring | `StringContains` | No |
| Case-aware string equality | `StringEquals` | No |
| SQL pattern matching | `Like`, `NotLike` | No |
| Full-text phrase/filter | `FTContains`, `NotFTContains` | Yes |
| All or any search terms | `FTContainsAll`, `FTContainsAny` and negative forms | Yes |
| Relevance ranking | `FTScore` | Yes |
| Vector plus text ranking | `VectorDistance` + `FTScore` + optional `WithWeights` | Vector and full-text policies |

“No full-text policy” does not mean a query is free of index or RU considerations. Literal predicates still execute against Cosmos.

## Match literal text

```go
orbit := products.Query().Cluster(func(c *gosmos.ClusterBuilder) {
    c.StartsWith("name", "camera", query.IgnoreCase).
        And().StringContains("description", "weather sealed", query.IgnoreCase)
}).InPartition("tenant-1")
_ = orbit
```

`query.IgnoreCase` applies only to the literal string helpers. The input is bound as a string, not embedded in SQL. `%` and `_` have no wildcard meaning for these helpers; `Like` is the separate pattern-matching API and requires a `%` wildcard.

`StringContains` is a string predicate. `Contains` is collection membership; they are not interchangeable.

## Filter with full-text predicates

Examples use the `Product` entity and `products` repository from [Vector search](VECTOR_SEARCH.md#prepare-the-document-shape).

```go
rows, gravity, err := products.Query().
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.FTContains("description", "mirrorless camera").
            And().FTContainsAny("description", []string{"travel", "outdoor"})
    }).InPartition("tenant-1").List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

Predicate fragments:

```sql
FullTextContains(c["description"], @p0)
FullTextContainsAny(c["description"], @p1)
```

Single-term/phrase helpers take a string. All/any helpers take a non-empty `[]string`; blank terms in that array are rejected. Negative helpers are `NotFTContains`, `NotFTContainsAll`, and `NotFTContainsAny`.

Full-text filters do not themselves add ranking or require TOP. A simple full-text filter remains subject to the normal execution policy; adding ranking changes the required partition scope.

## Rank by text relevance

```go
type TextMatch struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}
```

```go
orbit := products.Query().Select("id", "name").
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.FTScore("description", []string{"mirrorless", "travel"})
    }).Top(10).InPartition("tenant-1")
rows, gravity, err := gosmos.ListAs[TextMatch](ctx, orbit)
if err != nil {
    return err
}
_ = rows
_ = gravity
```

The ranking clause uses one binding per term:

```sql
ORDER BY RANK FullTextScore(c["description"], @p0, @p1)
```

`FTScore` controls ordering. It does not filter documents by itself; add a full-text filter if the query also needs a matching condition. Full-text scoring is not selected as a scalar result field, and `Project` cannot expose it as one. Use a result type containing only the fields actually selected.

Text ranking requires TOP between 1 and 50 and a complete partition key. Scalar sorting, aggregation, and grouping cannot accompany ranking.

## Combine vector and text ranking

```go
orbit := products.Query().Select("id", "name").
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.VectorDistance("embedding", []float64{0.12, 0.34, 0.56}).
            FTScore("description", []string{"mirrorless", "travel"})
    }).WithWeights(0.7, 0.3).Top(5).InPartition("tenant-1")
matches, gravity, err := gosmos.ListAs[ProductMatch](ctx, orbit)
if err != nil {
    return err
}
_ = matches
_ = gravity
```

Multiple rank expressions produce reciprocal rank fusion:

```sql
ORDER BY RANK RRF(
    VectorDistance(c["embedding"], @p0),
    FullTextScore(c["description"], @p1, @p2),
    [0.7, 0.3]
)
```

Weights follow rank-expression order. Supply one weight per expression. Gosmos's text validator permits numeric weight lists; the service determines the remaining semantic constraints. Omitting `WithWeights` emits RRF without an explicit weight list. Weights require multiple ranking expressions; scalar ordering cannot be mixed into the same query.

Hybrid ranking includes vector constraints: a maximum TOP of 50, valid vector dimensions/values, and the appropriate container policies. The first vector score can be selected as `embeddingScore` for the `embedding` source path; there is no selected scalar “RRF score” or “full-text score” property.

## Declarative search

```go
statement, err := query.Build(query.Input{
    Clusters: []query.Cluster{{Catalysts: []query.Catalyst{
        {Column: "description", Operator: query.FTContainsAll,
            Value: []string{"weather", "sealed"}},
        {Column: "description", Operator: query.FTScore,
            Value: []string{"travel", "camera"}},
    }}},
    Columns: query.ColumnOptions{Names: []string{"id", "name"}, Top: 10},
})
if err != nil {
    return err
}
_ = statement
```

The same validation and parameterization rules apply to fluent and declarative inputs. Multi-term filter inputs are bound as arrays; score terms use individual parameters.

Term arrays are limited to 64 terms per predicate and 1,024 UTF-8 bytes per term, with 64 KiB of term bytes across the query. The query also permits at most 1,024 parameters and 256 KiB of generated SQL. Oversized inputs fail locally; see [query construction budgets](DECLARATIVE_QUERY_SYNTAX.md#query-construction-budgets).

## Execution requirements

- The account must support the full-text features being used.
- The container policy and index must cover the queried text paths.
- Vector/hybrid queries also need an embedding policy and compatible vector index.
- Gosmos does not provision or modify these policies.
- Full-text ranking, vector ranking, and RRF require `InPartition`; the gateway cross-partition path rejects them locally.
- Gosmos does not implement text analysis, embedding generation, score merging, or search across containers.

Run the offline examples to inspect generated queries:

```sh
go run ./examples/darkmatter -example 10
go run ./examples/darkmatter -example 11
```

[Next: query execution and consistency →](QUERY_EXECUTION.md)
