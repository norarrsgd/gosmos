# Vector search

[Documentation home](../README.md) · [Hybrid ranking](FULLTEXT_SEARCH.md#combine-vector-and-text-ranking) · [Execution](QUERY_EXECUTION.md)

Use `VectorDistance` to rank documents by an embedding vector. Gosmos builds the SQL, binds the vector, and delegates ranking to Cosmos. It does not generate embeddings or merge ranked results across partitions.

## Prepare the document shape

```go
type Product struct {
    gosmos.Base
    TenantID    string    `json:"tenantId" gosmos:"pk=1"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Embedding   []float64 `json:"embedding"`
}
```

The container needs a vector embedding policy and an appropriate index for `/embedding`. The query vector must match the dimensions expected by that policy. Gosmos's provisioning helper does not create vector policies or indexes.

Construct the repository using a caller-authenticated client:

```go
products, err := gosmos.New[Product](client, "shop", "products")
if err != nil {
    return err
}
```

## Run a top-k query

```go
type ProductMatch struct {
    ID          string  `json:"id"`
    Name        string  `json:"name"`
    VectorScore float64 `json:"embeddingScore"`
}
```

```go
embedding := []float64{0.12, 0.34, 0.56}
orbit := products.Query().
    Select("id", "name").
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.VectorDistance("embedding", embedding)
    }).
    Top(5).
    InPartition("tenant-1")
matches, gravity, err := gosmos.ListAs[ProductMatch](ctx, orbit)
if err != nil {
    return err
}
_ = matches
_ = gravity.RU
```

Normalized SQL for the ranking expression:

```sql
SELECT TOP 5 c["id"], c["name"],
       VectorDistance(c["embedding"], @p0) AS embeddingScore
FROM c
ORDER BY VectorDistance(c["embedding"], @p0)
```

The same vector parameter is reused in the selected score and ranking expression. The score alias derives from the source path: `embedding` produces `embeddingScore`; a result type can include that JSON field to receive it. Additional vector expressions receive distinct generated aliases; inspect `GenerateQuery` rather than depending on an inferred alias convention.

`VectorScore` is the service's `VectorDistance` result. Gosmos does not convert it into a percentage, reverse its sign, or normalize it to a probability.

## Combine vector ranking with filters

```go
orbit := products.Query().Select("id", "name").
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.StartsWith("name", "camera", query.IgnoreCase).
            And().VectorDistance("embedding", []float64{0.12, 0.34, 0.56})
    }).Top(10).InPartition("tenant-1")
_ = orbit
```

Filter predicates contribute to WHERE. Vector predicates contribute score expressions and ranking. A vector predicate is not an ordinary boolean WHERE expression.

Do not add `OrderBy` or `OrderByDescending` to a ranking query. Scalar sorting with rank predicates is rejected. Aggregate and grouped queries also reject ranking.

## Generate vector SQL without an account

```go
statement, err := query.Build(query.Input{
    Clusters: []query.Cluster{{Catalysts: []query.Catalyst{
        {Column: "embedding", Operator: query.VectorDistance,
            Value: []float32{0.12, 0.34, 0.56}},
    }}},
    Columns: query.ColumnOptions{Names: []string{"id"}, Top: 5},
})
if err != nil {
    return err
}
_ = statement.Text
_ = statement.Params
```

The standalone builder can describe the query without a partition key. Repository execution still requires one because vector ranking is not eligible for the gateway cross-partition path.

## Requirements and limits

| Requirement | Behavior |
|---|---|
| Vector value | Non-empty float32 or float64 slice or array |
| Dimensions | At most 4,096; must also match the container policy |
| Numeric values | NaN and infinity are rejected |
| TOP | Required for ranking; at most 50 for all ranking queries |
| Partition key | Complete logical key required for execution |
| Scalar sorting | Cannot accompany ranking |
| Aggregation/grouping | Cannot accompany ranking |
| Embedding generation | Supplied by the application |
| Client-side merge | Not supported |

The SQL builder validates the supplied vector and query structure. Container policy, index configuration, stored vector shape, and account capabilities remain service requirements.

## Inspect cost and choose hints

The normal execution metadata applies: RU accumulates across fetched pages, and query recording can expose the vector parameter when enabled. Embeddings can contain sensitive application information; choose query recording deliberately.

```go
hints := tuner.Defaults("vector")
_ = hints.PageSize
_ = hints.BufferSize
```

The vector default is page size 50 and buffer size 50. These are recommendations, not an automatic execution policy. The SDK compatibility buffer setting has no effect in Gosmos. Read [statistics and recommendations](CACHING_AND_STATISTICS.md#recommend-settings) before translating recommendations into query hints.

For an executable SQL example:

```sh
go run ./examples/darkmatter -example 9
```

[Next: full-text and hybrid ranking →](FULLTEXT_SEARCH.md)
