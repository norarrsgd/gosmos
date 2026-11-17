# Gosmos

**Fluent queries, typed repositories, and explicit execution control for Azure Cosmos DB.**

Gosmos is the Go counterpart of [Universe](https://github.com/norarrsgd/universe). It combines a standalone SQL builder with a repository API for documents, projections, pagination, transactions, and search.

Import `github.com/norarrsgd/gosmos` for database operations, `github.com/norarrsgd/gosmos/query` for standalone SQL generation, and `github.com/norarrsgd/gosmos/tuner` for query statistics. The root package exposes the public API; repository implementation lives under `internal/`.

```go
orders, err := gosmos.New[Order](client, "shop", "orders")
if err != nil {
    return err
}

rows, gravity, err := orders.Query().
    Cluster(func(c *gosmos.ClusterBuilder) {
        c.Eq("status", "open").And().Gte("total", 100)
    }).
    OrderByDescending("addedOn").
    Top(25).
    InPartition("tenant-1").
    List(ctx)
```

Property paths are validated, values are bound as parameters, and `Gravity` carries request cost and operation metadata.

[Get started](docs/GETTING_STARTED.md) · [Fluent queries](docs/FLUENT_QUERY_BUILDER.md) · [Declarative queries](docs/DECLARATIVE_QUERY_SYNTAX.md) · [Examples](examples/README.md)

## What you can build

| Capability | Gosmos API | Guide |
|---|---|---|
| Document repositories | `Galaxy[T]`, `Base`, CRUD, ETags, hierarchical keys | [Getting started](docs/GETTING_STARTED.md) |
| Fluent queries | `Orbit`, predicates, sorting, grouping, joins | [Fluent query builder](docs/FLUENT_QUERY_BUILDER.md) |
| SQL generation without a client | `query.Build`, `Statement`, structured inputs | [Declarative query syntax](docs/DECLARATIVE_QUERY_SYNTAX.md) |
| Typed and nested projections | `SelectAs`, `ListAs`, `Project`, field operands | [Fluent query builder](docs/FLUENT_QUERY_BUILDER.md#projections) |
| Vector similarity | `VectorDistance`, top-k queries, score fields | [Vector search](docs/VECTOR_SEARCH.md) |
| Text search and hybrid ranking | Literal string predicates, full-text filters, RRF | [Full-text and hybrid search](docs/FULLTEXT_SEARCH.md) |
| Transactions and partial updates | `Atomic`, `Bulk`, `Patch`, ETag preconditions | [Batches and patch](docs/BATCHES_AND_PATCH.md) |
| Paging and consistency | Opaque tokens, per-request consistency, execution hints | [Query execution](docs/QUERY_EXECUTION.md) |
| Caching and query statistics | `WithCache`, `WithTuner`, memory/JSON/SQLite storage | [Caching and statistics](docs/CACHING_AND_STATISTICS.md) |

## Two ways to use Gosmos

### Build and execute a query

Embed `Base` and mark the partition key with its JSON field name:

```go
type Order struct {
    gosmos.Base
    TenantID string  `json:"tenantId" gosmos:"pk=1"`
    Status   string  `json:"status"`
    Total    float64 `json:"total"`
}
```

Supply an authenticated `azcosmos.Client` to `gosmos.New`. Construction validates configuration without making requests. Create a fresh `Orbit` for each query; the repository can be shared between goroutines.

[Create your first repository →](docs/GETTING_STARTED.md)

### Generate SQL without Cosmos

The `query` package imports only the Go standard library. No account, credential, or SDK client is needed:

```go
statement, err := query.Build(query.Input{
    Clusters: []query.Cluster{{Catalysts: []query.Catalyst{
        {Column: "status", Value: "open"},
        {Column: "total", Operator: query.Gte, Value: 100},
    }}},
    Columns: query.ColumnOptions{Names: []string{"id", "total"}, Top: 25},
})
if err != nil {
    return err
}
```

`statement.Text` contains SQL; `statement.Params` contains the named bindings. Parameter names are opaque. Use the names returned with the statement rather than constructing them yourself.

[Build declarative queries →](docs/DECLARATIVE_QUERY_SYNTAX.md)

## Know the execution boundary

Gosmos executes queries in a logical partition or passes supported cross-partition shapes through the SDK gateway. It does not discover ranges, fan out queries, or merge results on the client.

| Query shape | Full partition key | No partition key |
|---|---|---|
| Filters, field/object projections, plain joins | Execute | Gateway pass-through |
| Ordering, `TOP`, `DISTINCT` | Execute | Reject |
| Aggregates and `GROUP BY` | Execute | Reject |
| Vector, full-text ranking, and RRF | Execute | Reject |
| Query-based `Get` | Execute | Reject: adds `TOP 1` |

Rejected shapes wrap `ErrUnsupportedCrossPartition` before a request is sent. Vector and full-text queries also require suitable account features, policies, and indexes. [Read the cross-partition policy →](docs/CROSS_PARTITION.md)

Other limits:

- Partition keys contain one to three string components; partial hierarchical keys are not supported. Untagged `PartitionKeyer` entities declare their complete width with `WithPartitionKeyCount`.
- `Atomic` is one same-partition transaction. `Bulk` may partially succeed across transaction chunks.
- Grouped or aggregate queries cannot also request ordering or ranking.
- Query generation enforces [complexity, projection, and full-text budgets](docs/DECLARATIVE_QUERY_SYNTAX.md#query-construction-budgets) before execution.
- Explicit read consistency cannot strengthen the account's consistency setting.
- Stored procedures, conditional patch filter predicates, and automatic application of tuner recommendations are not supported.

## Run the examples

Requires **Go 1.27**.

```sh
go run ./examples/query
go run ./examples/darkmatter -example 18
go run ./examples/darkmatter -all
```

These commands generate queries without account access. The [eighteen examples](examples/README.md) cover CRUD, paging, projections, transactions, search, input handling, and tuning. Account operations require an explicit `-live` flag.

For a local checkout, see [module setup](docs/GETTING_STARTED.md#use-a-local-checkout), including the separate SQLite module.

## Develop

```sh
go build ./...
go vet ./...
go test -race ./...
(cd tuner/sqlite && go test -race ./...)
```

Query regression fixtures are vendored, so normal Go checks do not require .NET or a Cosmos account. Use a dedicated test account or emulator for account operations.

## License

[MIT](LICENSE).
