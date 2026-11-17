# Get started with Gosmos

[Documentation home](../README.md) · [Query guide](FLUENT_QUERY_BUILDER.md) · [Transactions](BATCHES_AND_PATCH.md)

This guide creates a repository, writes an order, reads it by partition key, and replaces it with an ETag precondition.

## Requirements

- Go 1.27 or newer.
- An Azure Cosmos DB account using the NoSQL API for account operations.
- A database and container matching the entity's partition-key paths.
- An authenticated `azcosmos.Client`. Gosmos does not load credentials itself.

The examples below use a container named `orders` in database `shop`, with partition-key path `/tenantId`. Client construction and repository construction do not provision those resources.

## Use a local checkout

Import `github.com/norarrsgd/gosmos` for the repository API, including `gosmos.New`, document reads and writes, and fluent query execution. The separate `github.com/norarrsgd/gosmos/query` package generates SQL without executing database requests. To use your checkout from another Go module, add a local replacement with the absolute path to that checkout:

```sh
go mod edit -require=github.com/norarrsgd/gosmos@v0.0.0
go mod edit -replace=github.com/norarrsgd/gosmos=/absolute/path/to/gosmos
go mod tidy
```

Replace `/absolute/path/to/gosmos` before running these commands. `v0.0.0` is a local development requirement resolved by the replacement, not a release selection.

Imports used in this guide:

```go
import (
    "context"
    "errors"
    "os"
    "time"

    "github.com/Azure/azure-sdk-for-go/sdk/azcore"
    "github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
    "github.com/norarrsgd/gosmos"
)
```

## Define an entity

```go
type Order struct {
    gosmos.Base
    TenantID string  `json:"tenantId" gosmos:"pk=1"`
    Status   string  `json:"status"`
    Total    float64 `json:"total"`
}
```

`Base` supplies these JSON fields:

| Field | JSON name | Behavior |
|---|---|---|
| `ID` | `id` | Blank IDs receive a UUIDv7 during create |
| `AddedOn` | `addedOn` | Set during create |
| `ModifiedOn` | `modifiedOn` | Set during replace; omitted when nil |
| `CountAggregate` | `countAggregate` | Optional aggregate result field |

Embed `Base` by value. Anonymous pointer embeddings are rejected. JSON tags define stored names; query paths and patch paths use those names, not Go field names.

## Supply a client

This account-key example reads the caller's environment. Set `GOSMOS_ENDPOINT` and `GOSMOS_KEY` through your existing secret-management process before executing it.

```go
endpoint := os.Getenv("GOSMOS_ENDPOINT")
key := os.Getenv("GOSMOS_KEY")
if endpoint == "" || key == "" {
    return errors.New("GOSMOS_ENDPOINT and GOSMOS_KEY are required")
}
credential, err := azcosmos.NewKeyCredential(key)
if err != nil {
    return err
}
client, err := azcosmos.NewClientWithKey(endpoint, credential, nil)
if err != nil {
    return err
}
orders, err := gosmos.New[Order](client, "shop", "orders")
if err != nil {
    return err
}
```

You can instead construct an SDK client with a caller-supplied token credential. Gosmos does not require an identity-provider package or change the authentication of an injected client.

Use a bounded context for account operations:

```go
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
```

## Create and read

```go
created, write, err := orders.Create(ctx, Order{
    TenantID: "tenant-1",
    Status:   "open",
    Total:    125,
})
if err != nil {
    return err
}

stored, read, err := orders.Get(ctx, created.ID, []string{"tenant-1"})
if err != nil {
    return err
}
_ = write.RU
_ = read.ETag
_ = stored
```

Create returns a prepared copy containing the generated ID and timestamps. Keep that returned value when you need its ID. The original top-level value is not the prepared document. Custom setters and stampers should not mutate shared reference data.

A point read needs both ID and the complete partition key. The returned `ETag` identifies the document version observed by that read.

## Replace with optimistic concurrency

```go
stored.Status = "paid"
updated, replacement, err := orders.Modify(ctx, stored, gosmos.IfMatch(read.ETag))
if errors.Is(err, gosmos.ErrPreconditionFailed) {
    // Another writer changed the document; reload before deciding how to retry.
    return err
}
if err != nil {
    return err
}
_ = updated
_ = replacement.ETag
```

`Modify` replaces the complete document. Properties absent from your serialized model are not retained automatically. Use [Patch](BATCHES_AND_PATCH.md#patch-one-document) for targeted changes.

Delete can use the same ETag protection:

```go
removed, err := orders.Remove(ctx, updated.ID, []string{"tenant-1"},
    gosmos.IfMatch(replacement.ETag))
if err != nil {
    return err
}
_ = removed.RU
```

## Read operation metadata

```go
rows, gravity, err := orders.Query().InPartition("tenant-1").List(ctx)
if err != nil {
    return err
}
_ = rows
_ = gravity.RU
_ = gravity.ContinuationToken
_ = gravity.StatisticsError
```

| `Gravity` field | Meaning |
|---|---|
| `RU` | Request units consumed across fetched pages; zero for a cache hit |
| `ETag` | Version metadata for item operations |
| `ContinuationToken` | SDK token from the last query response |
| `Query` | SQL and bound values when query recording is enabled |
| `StatisticsError` | Optional statistics-storage error, separate from the query result |

`WithRecordQueries(true)` enables query recording. Its values may contain application data; avoid logging them indiscriminately.

## Handle errors

Use `errors.Is` for library categories and `errors.As` for SDK details:

```go
_, _, err := orders.Get(ctx, "order-1", []string{"tenant-1"})
switch {
case errors.Is(err, gosmos.ErrNotFound):
    // Missing document.
case errors.Is(err, gosmos.ErrConflict):
    // Duplicate create or another HTTP 409 conflict.
case errors.Is(err, gosmos.ErrPreconditionFailed):
    // ETag no longer matches.
case errors.Is(err, gosmos.ErrValidation):
    // Invalid model, query, partition key, or options.
}
var response *azcore.ResponseError
if errors.As(err, &response) {
    _ = response.StatusCode
}
```

`ErrUnsupportedCrossPartition` identifies execution shapes rejected by the local guard. Other service failures retain their SDK response errors. Cancellation remains discoverable through the error chain.

## Hierarchical partition keys

```go
type RegionalOrder struct {
    gosmos.Base
    TenantID string `json:"tenantId" gosmos:"pk=1"`
    Region   string `json:"region" gosmos:"pk=2"`
    Status   string `json:"status"`
}
```

The corresponding container paths are `/tenantId` and `/region`, in that order. Reads and queries supply both values:

```go
regional, err := gosmos.New[RegionalOrder](client, "shop", "regional-orders")
if err != nil {
    return err
}
order, gravity, err := regional.Get(ctx, "order-1", []string{"tenant-1", "APAC"})
if err != nil {
    return err
}
_ = order
_ = gravity
```

Keys support one to three string components. Duplicate levels, gaps, non-string tagged properties, null key values, and incomplete keys are rejected. Empty strings are string values, not missing key components.

## Use your own document envelope

`Base` is optional. The required contract is `GetID() string`; optional interfaces add specific behavior:

```go
type Document struct {
    ID     string `json:"id"`
    Tenant string `json:"tenant"`
}

func (d Document) GetID() string { return d.ID }
func (d *Document) SetID(id string) { d.ID = id }
func (d Document) PartitionKeyValues() []string { return []string{d.Tenant} }
```

`PartitionKeyer` takes precedence over tag extraction for document writes. Because this example has no partition-key tags, declare its complete key width when constructing the repository:

```go
documents, err := gosmos.New[Document](client, "app", "documents",
    gosmos.WithPartitionKeyCount(1))
```

The count must be one to three. Alternatively, valid `gosmos:"pk=N"` tags on a `PartitionKeyer` entity declare the width while the interface still supplies write values. Point reads and queries reject explicit keys when the width is unknown or incomplete. `IDSetter` enables generated IDs; without it, create requires an existing ID. `Stamper` provides `StampCreate(time.Time)` and `StampModify(time.Time)` hooks. `WithPartitionKeyCount` does not provide container paths, so custom partition-key extraction still cannot auto-provision a container.

## Provision explicitly

```go
orders, err := gosmos.New[Order](client, "shop", "orders",
    gosmos.WithAutoProvision(true))
if err != nil {
    return err
}
if err = orders.EnsureProvisioned(ctx); err != nil {
    return err
}
```

Provisioning is disabled by default. Even when enabled, `New` makes no requests: call `EnsureProvisioned` explicitly. It creates missing resources using tagged partition paths and leaves existing resources unchanged. It does not reconcile indexes, TTL, vector/full-text policies, throughput, or an existing partition schema.

## Repository options

| Option | Default | Purpose |
|---|---|---|
| `WithRecordQueries(bool)` | Off | Attach SQL and values to query metadata |
| `WithCrossPartition(bool)` | On | Permit eligible gateway queries without a key |
| `WithAutoProvision(bool)` | Off | Enable explicit provisioning calls |
| `WithConsistency(level)` | Account/SDK behavior | Default explicit read-consistency override |
| `WithCache(capacity, ttl)` | Off | Cache point reads and single-result query reads |
| `WithTuner(storage)` | Off | Record query execution statistics |
| `WithNamingPolicy(fn)` | No transform | Transform query source-path segments |

Continue with [fluent queries](FLUENT_QUERY_BUILDER.md), [query execution](QUERY_EXECUTION.md), or [caching and statistics](CACHING_AND_STATISTICS.md).
