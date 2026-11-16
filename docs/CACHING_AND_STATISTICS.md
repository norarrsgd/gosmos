# Caching, query statistics, and storage

[Documentation home](../README.md) · [Execution options](QUERY_EXECUTION.md) · [Transactions](BATCHES_AND_PATCH.md)

Document caching avoids repeated reads. Query statistics record the cost and outcome of executed queries. Both are opt-in repository options; neither changes the document schema.

## Document cache

```go
orders, err := gosmos.New[Order](client, "shop", "orders",
    gosmos.WithCache(500, 2*time.Minute))
if err != nil {
    return err
}
```

Both capacity and TTL must be positive. The cache belongs to that repository instance; constructing a second repository does not share its entries, even if it uses the same SDK client and container.

### What is cached

| Operation | Cached |
|---|---|
| Point `Galaxy.Get` | Yes |
| Single-result `Orbit.Get` / `GetAs` | Yes |
| `List` / `ListAs` | No |
| `Paged` / `PagedAs` | No |
| Any read with explicit consistency | No |

Entries contain serialized data. Cache reads deserialize independent values rather than returning a shared mutable object. Point-read cache hits retain the cached ETag and return zero RU. Query-based hits also return zero RU.

Keys are hashes of operation inputs. Single-result query keys distinguish the normalized SQL, bindings, partition key, and result type; changing projection or parameter values does not reuse an unrelated result.

### TTL and eviction

The bound is first-in-first-out, not least-recently-used. Reading an entry does not refresh its TTL or promote it in the eviction order. Replacing an entry removes and reinserts it with a new expiry. Expired entries are treated as misses.

Capacity bounds stored entries. TTL does not arrange a background refresh; an expired read goes back to the service.

### Mutation invalidation

Create, replace, remove, patch, and batch execution invalidate cached reads in the same repository. Uncertain transport outcomes also clear cached entries conservatively. An in-flight read cannot repopulate the cache after a later invalidation using its older generation.

This is process-local caching. A write through another repository, SDK client, or process does not notify this instance; the cached value can remain visible until expiry. Choose TTL to match your application's tolerance for external writes, or leave caching disabled.

### Consistency overrides bypass the cache

```go
level := azcosmos.ConsistencyLevelStrong
fresh, gravity, err := orders.Get(ctx, "order-1", []string{"tenant-1"},
    gosmos.ReadOptions{ConsistencyLevel: &level})
if err != nil {
    return err
}
_ = fresh
_ = gravity
```

Any explicit level bypasses lookup and population, whether supplied per call or through `WithConsistency` on the repository. It does not refresh or evict an existing cached read. A later default read can still retrieve the older entry until its TTL expires or a mutation invalidates it.

Invalid levels fail before cache access. An explicit consistency request never silently falls back to cached data.

## Record query executions

```go
statistics := tuner.NewMemory()
orders, err := gosmos.New[Order](client, "shop", "orders",
    gosmos.WithTuner(statistics))
if err != nil {
    return err
}
rows, gravity, err := orders.Query().InPartition("tenant-1").List(ctx)
if err != nil {
    return err
}
if gravity.StatisticsError != nil {
    // Handle an optional statistics-storage failure separately.
}
_ = rows
```

Automatic recording covers query execution paths, not CRUD or transaction requests. A single-result query served from the document cache does not record a database execution sample.

| Sample field | Meaning |
|---|---|
| `Hash` | SHA-256 of normalized SQL |
| `RU` | Query request charge |
| `Duration` | Elapsed query execution time |
| `Count` | Number of returned results |
| `Success` | Whether execution returned an error |
| `Timestamp` | Start time of the recorded execution |
| `Strategy` | Partition-scoped or gateway classification |
| `Hints` | Recorded execution configuration; automatic recording captures page-size hint |

SQL normalization removes generated parameter names from the hash while preserving quoted text. Parameter values are not included in the hash. Queries with the same structure but different values can therefore share a statistics group.

A statistics write failure is returned in `Gravity.StatisticsError`. It does not replace a successful database result or obscure the query's primary error. Recording has its own bounded context, so a cancelled query can still attempt to record its outcome.

## Recommend settings

```go
statement, err := orders.Query().
    Cluster(func(c *gosmos.ClusterBuilder) { c.Eq("status", "open") }).
    InPartition("tenant-1").GenerateQuery()
if err != nil {
    return err
}
normalized, _ := query.Normalize(statement.Text)
hash := fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
recommendation, err := tuner.Recommend(ctx, statistics, hash, time.Now())
if err != nil {
    return err
}
_ = recommendation.Strategy
_ = recommendation.Hints
```

Recommendation rules:

| Rule | Threshold or selection |
|---|---|
| History window | Last 24 hours, excluding future timestamps |
| Overall minimum | 10 samples |
| Candidate hint configuration | At least 3 samples |
| Candidate strategy | At least 5 samples |
| Hint preference | Lower average RU, then higher success rate |
| Strategy preference | Higher success rate, then lower average RU |

Insufficient history returns an empty recommendation without an error. Recommendations are advisory. They do not modify an existing query or bypass partition restrictions.

Use the effective fields when constructing `gosmos.Hints` for a new query. Buffer/concurrency compatibility fields have no query-execution effect; a suggested single-partition strategy still requires a partition key.

### Shape-based defaults

```go
defaults := tuner.Defaults("aggregation")
_ = defaults.PageSize
_ = defaults.BufferSize
```

| Kind | Page size | Buffer size | Other recommendation |
|---|---|---|---|
| `vector`, `fulltext`, `hybrid` | 50 | 50 | — |
| `aggregation` | 500 | 1,000 | — |
| `complex` | Unset | Unset | Concurrency 1, continuation limit 1 KB |
| Other values | Unset | Unset | None |

Call `Defaults` explicitly with an application-chosen kind. Gosmos does not automatically classify a query and apply these settings. Buffer and concurrency recommendations remain advisory compatibility values in the Go execution layer.

## Choose statistics storage

| Storage | Setup | Retention | Suitable use |
|---|---|---|---|
| `tuner.NewMemory()` | No filesystem | Latest 1,000 records per store | Process-local observations |
| `tuner.NewFile(path)` | Existing writable parent directory | Latest 1,000 records per file | Small persistent history, one process per file |
| `tuner/sqlite.Open(path)` | Separate module and driver | No automatic retention limit | SQLite-backed history |

All stores implement `Record(context.Context, Sample)` and `Samples(context.Context, hash, since)`.

### JSON file storage

The following uses an application-owned directory beneath the operating system's user-cache location. Do not derive statistics paths from request parameters.

```go
base, err := os.UserCacheDir()
if err != nil {
    return err
}
directory := filepath.Join(base, "gosmos")
if err = os.MkdirAll(directory, 0700); err != nil {
    return err
}
statistics, err := tuner.NewFile(filepath.Join(directory, "query-statistics.json"))
if err != nil {
    return err
}
_ = statistics
```

Paths must be absolute and already clean. Existing files must be regular and owner-only; symlinks at the file path are rejected by construction-time checks. The parent directory must exist.

Each record write reads the current history, keeps the newest 1,000 records, writes an owner-only temporary file, syncs it, and renames it into place. Access is serialized within the process. Separate processes do not share that lock, so use one writer process per file.

If a file contains invalid JSON, storage moves it aside under `.gosmos-corrupt-*` and starts with empty history. Recovery preserves the old bytes for inspection instead of overwriting them. Storage methods can return filesystem errors if backup or replacement fails.

### SQLite storage

SQLite is a nested module, `github.com/norarrsgd/gosmos/tuner/sqlite`, using the pure-Go driver `modernc.org/sqlite`. The core module does not import the driver.

For local development, replace both modules in the consuming application:

```sh
go mod edit -require=github.com/norarrsgd/gosmos/tuner/sqlite@v0.0.0
go mod edit -replace=github.com/norarrsgd/gosmos/tuner/sqlite=/absolute/path/to/gosmos/tuner/sqlite
go mod edit -replace=github.com/norarrsgd/gosmos=/absolute/path/to/gosmos
go mod tidy
```

Replace the absolute paths before running the commands. Import storage with an alias to distinguish it from other SQLite packages:

```go
import statisticsqlite "github.com/norarrsgd/gosmos/tuner/sqlite"
```

```go
statistics, err := statisticsqlite.Open(filepath.Join(directory, "query-statistics.db"))
if err != nil {
    return err
}
defer statistics.Close()
orders, err := gosmos.New[Order](client, "shop", "orders",
    gosmos.WithTuner(statistics))
if err != nil {
    return err
}
_ = orders
```

Close the store after the repositories using it have finished. The database uses an owner-only regular file, creates its sample table if needed, and serializes access through one database connection. Writes are immediate inserts; there is no background batch queue, automatic pruning, or application-level retry loop.

Storage uses a `samples` table containing hash, timestamp, and JSON sample value. The application owns database lifecycle and retention policy.

## Query recording is separate

`WithRecordQueries(true)` attaches SQL and bound values to `Gravity.Query`. `WithTuner` records execution statistics instead. You can use either option independently.

The default query-recording setting is off. Enable it only where the application deliberately handles the associated data; vectors, document IDs, search terms, and other parameters can be sensitive.

[Back to execution options →](QUERY_EXECUTION.md)
