# Gosmos codebase map

This is the working map for repository structure, architectural boundaries, and implementation details that are easy to miss from API names. Start here when changing the library; use `README.md` and `docs/` for consumer-facing usage.

## Project identity

Gosmos is a Go library for Azure Cosmos DB, published as `github.com/norarrsgd/gosmos`. It combines standalone parameterized SQL generation with typed repositories, fluent queries, projections, transactions, patching, caching, and query statistics.

The main concepts are:

- `Galaxy[T]`: a concurrency-safe repository for one container.
- `Entity`: the minimal `GetID() string` contract; `Base` supplies standard document fields and optional mutation capabilities.
- `Orbit[T]`: a mutable fluent query builder. Create a separate builder for each concurrent operation.
- `query.Input`, `Cluster`, and `Catalyst`: declarative inputs shared with the fluent API.
- `query.Statement`: SQL, named bindings, and structured execution-shape flags.
- `Gravity`: request units, ETag, continuation token, optional query details, and statistics-storage errors.
- `AtomicBatch`, `BulkBatch`, and `Patch`: transaction and partial-update builders.
- `tuner.Storage`: execution-sample storage used by advisory recommendation functions.

Requires Go 1.27. The Cosmos SDK is `github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos`. Entity serialization uses `encoding/json`.

## Repository map

```text
.
|-- AGENTS.md                  Agent workflow and contribution rules (Codex)
|-- CLAUDE.md                  Agent workflow and contribution rules (Claude Code)
|-- gosmos.go                  Public aliases, errors, and construction
|-- options.go                 Public configuration and predicate options
|-- query.go                   Typed projection/result helpers
|-- gosmos_test.go             External-consumer API tests
|-- query/                     Standalone SQL builder; standard library only
|-- internal/
|   |-- repository/            Repository implementation and SDK integration
|   |-- exec/                  Cross-partition query policy
|   `-- cache/                 Bounded FIFO document-byte cache
|-- tuner/                     Statistics, memory/file stores, recommendations
|   `-- sqlite/                Optional adapter with its own go.mod
|-- examples/
|   |-- query/                 Standalone SQL example
|   `-- darkmatter/            Offline and opt-in live repository examples
|-- testdata/corpus/           Vendored query-regression fixtures
|-- tools/
|   |-- sync-corpus/           Fixture validation and import tool
|   `-- UniverseCorpusExporter/ Optional .NET fixture generator
|-- docs/                      Task-focused public guides
`-- .github/workflows/         Go CI, Semgrep, and scheduled fuzzing
```

## Architecture and dependency boundaries

The root package is a public facade over `internal/repository`. Generic type aliases preserve fluent return types and callback signatures without duplicating implementation. Put repository behavior and its tests under `internal/repository/`; expose public types or forwarding functions through the three root source files as needed. Keep `gosmos_test.go` exercising the API as an external consumer.

```text
gosmos -> internal/repository -> query
                            -> internal/exec -> query
                            -> internal/cache
                            -> tuner
tuner/sqlite -> tuner
```

`query` imports only the standard library; `query/architecture_test.go` enforces that boundary. Neither `query` nor `tuner` imports the root or repository package. Consumers can generate SQL without an SDK client or account.

SQLite lives in a separate module to keep its driver out of the core dependency graph. Its `go.mod` replaces the core module with `../..` for local development. Root `./...` checks do not traverse that module; test it separately.

SQL generation and execution policy are separate. `query.Build` validates and describes the query; `internal/exec.Guard` decides whether that shape can execute without a partition key. `GenerateQuery` can therefore return valid SQL that repository execution rejects.

## Source ownership

### Repository implementation

All paths in this section are under `internal/repository/`.

- `galaxy.go`: validates construction, inspects entity metadata, and creates an SDK container handle without network requests. Small container interfaces let tests replace SDK operations.
- `options.go`: repository configuration, including opt-in caching, statistics, query recording, consistency, and provisioning.
- `provisioning.go`: explicit `EnsureProvisioned` calls. Disabled provisioning is a no-op; enabled provisioning creates missing resources and leaves existing containers unchanged. Provisioning requires partition-key tags rather than only a runtime `PartitionKeyer` override.
- `entity.go`, `partitionkey.go`, `uuid.go`: entity capabilities, cached reflection metadata, ordered key extraction, and generated UUIDv7 IDs.
- `crud.go`: point reads, creates, full-document replacement through `Modify`, deletion, stamping, and cache integration.
- `read_options.go`, `write_options.go`: consistency validation and ETag preconditions.
- `orbit.go`, `cluster.go`: fluent state and predicates; `list.go` adapts declarative inputs and implements typed result entry points.
- `select.go`: selected top-level JSON field names derived from a result struct.
- `query_execution.go`: query validation, shape guard, SDK options/paging, decoding, single-result caching, and statistics recording.
- `batch.go`: shared operation preparation, validation, transaction execution, and results; `atomic_batch.go` and `bulk_batch.go` define the public builders and execution boundaries.
- `patch.go`: typed path validation, SDK patch construction, and point-patch execution.
- `gravity.go`, `hints.go`, `errors.go`: metadata, execution hints, contextual errors, and public error classification.

### SQL generation

All paths in this section are under `query/`.

- `options.go`: declarative input, statement, shape, operator, sorting, and paging types.
- `build.go`: SQL assembly, predicate occurrence identities, bindings, and execution-shape flags.
- `catalyst.go`: predicate validation, operands, and scalar rendering.
- `projection.go`: structured field/object/aggregate projections and expression rendering.
- `ranking.go`: ordering, vector/full-text ranking, and reciprocal rank fusion.
- `identifier.go`: property/alias validation, bracket formatting, and derived output aliases.
- `limits.go`: preflight structural/metadata/parameter/full-text budgets and bounded projection traversal, shared by standalone and repository query generation.
- `normalize.go`: quote-aware parameter normalization for fixture comparison, cache signatures, and statistics hashes.
- `catalyst_json.go`, `projection_json.go`: JSON representations of structured inputs.

Values belong in bindings, not interpolated SQL. Source identifiers are validated before bracket formatting, including after naming-policy transformation. New identifier-bearing inputs must use those checks; new predicates and projections must keep shape flags accurate so execution policy cannot be bypassed.

Parameter names are opaque and distinguish predicate occurrences, even when values match. Use the bindings returned with the statement. `Normalize` replaces parameters outside quoted text in first-occurrence order, preserves repeated references, and returns the original unique names. Do not replace it with a regular-expression substitution.

Generation preflights at most 1,024 combined structural nodes, 64 KiB of metadata, and 1,024 parameters before rendering or hashing predicate IDs. Ordered deduplication uses membership maps; repeated binding identities use per-base suffix counters. Final SQL is limited to 256 KiB. Full-text arrays have at most 64 terms per predicate, 1,024 bytes per term, and 64 KiB total term bytes across a query. These are local library budgets, not Cosmos service limits; arbitrary ordinary parameter values still need application limits.

Projection JSON uses a token preflight to cap container nesting and a single standard-library wire-tree decode, with no nested custom-decoder calls. JSON input/output is limited to 256 KiB per projection; tree conversion and programmatic `Build` share the 32-level depth, item-count, and metadata budgets. Failed JSON decoding preserves the previous projection. See `docs/DECLARATIVE_QUERY_SYNTAX.md` for the exported budget constants and counting rules.

## Entities, JSON, and partition keys

`Entity` requires only an ID getter. ID generation and timestamps depend on optional `IDSetter` and `Stamper` interfaces. Embed `Base` by value to supply those capabilities; pointer-embedded capabilities are rejected during metadata inspection.

Create and modify prepare an addressable top-level copy. Read the returned entity for generated IDs or timestamps. This is a shallow copy: custom capability methods must not assume nested pointers, maps, or slices were deep-cloned. `Modify` replaces the entire stored document; use `Patch` for targeted changes.

Tagged partition keys use `gosmos:"pk=1"` through `pk=3`, with unique contiguous levels. Fields must be exported strings or string pointers and cannot be excluded from JSON. Null pointer values are invalid. JSON tags determine stored field names and provisioning paths; untagged anonymous value structs are traversed.

`PartitionKeyer` overrides tagged extraction for writes. Partition-key tags still declare the complete component count; an untagged `PartitionKeyer` must use `WithPartitionKeyCount` before explicit point/query keys can execute. Unknown widths and partial hierarchical keys fail before transport. The option declares width only and does not provide provisioning paths.

`WithNamingPolicy` transforms source query path segments. It does not change entity JSON serialization or explicit projection output names. Keep query paths and stored JSON consistent. `SelectAs[R]` selects result-struct JSON field names; `ListAs[R]`, `GetAs[R]`, and `PagedAs[R]` only choose the decoding type and do not infer a projection. Use `Project` for nested output shapes and computed expressions.

## Query execution and paging

The normal flow is `Orbit` or declarative `ListOptions` -> `query.Build` -> shape guard and complete-key validation -> SDK pager -> JSON decoding and `Gravity`. Gosmos does not discover partition ranges, fan out queries, or merge results on the client.

Without a key, eligible filters, field/object projections, and plain joins pass through the SDK gateway. Ordering, TOP, DISTINCT, aggregates, GROUP BY, vector search, full-text ranking, and RRF require a complete key. `WithCrossPartition(false)` rejects every keyless query before network execution. Grouped or aggregate queries cannot also request ordering or ranking. Search still requires suitable Cosmos features, policies, and indexes.

Result entry points differ:

- Point `Galaxy.Get` uses the SDK item-read API with an ID and full key.
- `List` consumes all service pages. A page-size hint does not limit total results.
- Query `Get` reads past empty pages and stops at the first decoded item, requesting one item per service page. No result becomes `ErrNotFound`.
- Without `Project`, query `Get` sets TOP 1 and clears distinct, aggregates, joins, sorting, grouping, and hints while preserving filters and selected columns. `OrderBy(...).Get()` therefore does not select by that order; use `Top(1).List()` for ordered entity results.
- With `Project`, query `Get` preserves the configured shape and hints, adding TOP 1 only if TOP is unset. Every query `Get` has TOP and requires a partition key.
- `Paged` accepts sizes 1 through 1,000 and consumes empty pages until a non-empty page or exhaustion. GROUP BY instead consumes available pages. Tokens are opaque SDK values; keep the query, bindings, consistency, and key stable between calls.

Any nonzero `Hints` value is mutually exclusive with paging. `MaxItemCount` and continuation-token size map to SDK hints. `MaxConcurrency`, `MaxBufferedItemCount`, and optimistic direct execution are compatibility settings with no query-execution effect. Strategy labels do not select a different SDK transport: `SinglePartition` requires a key, and `Gateway` does not bypass the guard or discard a supplied key.

Read consistency precedence is operation override -> repository default -> SDK/account behavior. An override cannot strengthen account consistency; service errors remain visible. Any explicit level bypasses cache lookup and population without refreshing old entries.

## Writes, transactions, and errors

`Atomic` executes one transaction in one logical partition, with at most 100 operations and a 2 MiB estimated payload. Oversized batches fail before execution; splitting would change the transaction boundary. Model-derived keys must match the transaction key.

`Bulk` accepts at most 10,000 operations or 64 MiB of aggregate estimated payload, groups them by a collision-safe partition identity in expected linear time, chunks at the atomic operation/payload limits, and runs at most four partition pipelines concurrently. Grouping and partition dispatch observe context cancellation. Chunks within a partition retain input order. A failed chunk stops later chunks in that partition; other partitions may continue. Earlier commits survive later errors or cancellation. Results preserve original order and identify unattempted operations; reconcile them before retrying. Transport errors can leave commit outcomes uncertain.

`Patch` accepts one to ten operations and validates paths against entity structure. Dot notation and JSON Pointer notation use JSON field names; pointer escapes, array indices, and declared maps are supported. Paths cannot change `id` or root system properties beginning with `_`. Nil means JSON null, while `Remove` deletes a property. Patching does not invoke stamping hooks. Conditional filter predicates are not implemented.

`IfMatch` supports optimistic concurrency; an empty ETag is invalid. Fluent batch and patch builders retain validation failures, so start a new builder after invalid input.

`Error` adds operation/entity context while preserving the underlying error through `Unwrap`. Use `errors.Is` for library sentinels and `errors.As` for SDK `*azcore.ResponseError`. Preserve this chain when changing error handling. A prepared batch item or generated ID is not evidence of a committed write.

## Cache and statistics ownership

### Document cache

`WithCache` enables repository-local caching for point reads and single-result queries. Lists, paged results, and SQL generation are uncached. Entries store copied JSON bytes; reads decode new values rather than sharing mutable documents.

`internal/cache/cache.go` uses a mutex, map, and FIFO list. Reads do not refresh TTL or promote entries. Replacement removes and reinserts an entry with a new expiry. Capacity bounds both entries and eviction metadata; there is no background refresh.

Mutating requests clear the repository cache conservatively, including uncertain transport outcomes. Generation checks prevent an older in-flight read from repopulating after invalidation. Other repositories, clients, and processes do not notify this cache; external changes remain subject to TTL.

### Query statistics and recommendations

`WithTuner` records query executions, not CRUD or batches. Cache hits do not record a database execution. Samples contain a hash of normalized SQL and execution metadata rather than raw parameter values. Recording uses a bounded context independent of query cancellation. Storage failure goes into `Gravity.StatisticsError` without replacing the database result or primary error.

`tuner/recommendation.go` analyzes the last 24 hours of samples, excluding future timestamps. It requires ten samples overall, three per candidate hint configuration, and five per candidate strategy. Recommendations and shape-based `Defaults` are explicit advisory calls; execution never applies them automatically.

Storage implementations share the contract in `tuner/storage.go`:

- `memory.go`: latest 1,000 records per store.
- `file.go`: latest 1,000 records per JSON file. Paths must be absolute and clean, the parent must exist, and existing files must be regular and owner-only. Construction resolves and pins parent-directory symlinks. Context-aware locks serialize aliases of the same on-disk file within one process, release canceled acquisitions before I/O, and remove idle entries. Use one writer process per file.
- File writes use an owner-only temporary file and rename without `fsync`. Corrupt JSON moves to `.gosmos-corrupt-*` and history restarts empty. Backups are not pruned automatically; crash recovery can discard retained statistics.
- `sqlite/storage.go`: optional pure-Go SQLite adapter, one connection, transactional insert/prune, and latest 10,000 rows across hashes. There is no background batching or application retry loop. The caller owns `Close` after repository use finishes.

Statistics paths are application configuration, not request inputs. `WithRecordQueries(true)` is separate from statistics and attaches SQL plus bindings to `Gravity.Query`; it defaults off. `GenerateQuery` always exposes bindings. Avoid logging sensitive values.

## Tests and verification

Run the standard checks from the root:

```sh
go build ./...
go vet ./...
go test -race ./...
(cd tuner/sqlite && go test -race ./...)
```

CI also checks `gofmt` across tracked Go files and runs `go run ./examples/query`. `.github/workflows/semgrep.yml` scans PRs and pushes to `main` with the pinned Semgrep 1.185.0 container digest and the `go/` rules from `semgrep/semgrep-rules` at commit `a5ad1714f00c9f7fde980cab0ea1d6dc6d414fd2` (same pins as Universe), fails on findings, and uploads SARIF; bump the pins only through reviewed changes. `.github/workflows/fuzz.yml` runs `FuzzValidateIdentifier`, `FuzzNormalize`, `FuzzBuild`, and `FuzzProjectionJSON` separately on a schedule and by manual dispatch.

Add tests beside the package that owns the behavior:

- `query/build_test.go`, `security_test.go`, `limits_test.go`, and projection JSON tests: SQL shape, validation, injection attempts, naming policies, structured inputs, normalization, complexity/full-text budgets, and projection JSON depth/size/fuzz regressions.
- `query/corpus_test.go` with `testdata/corpus/`: vendored SQL/binding/error regression cases. Normal Go tests need neither .NET nor Cosmos. `tools/sync-corpus` validates fixture metadata/schema; exporter requirements are in `tools/UniverseCorpusExporter/README.md`.
- `internal/repository/*_test.go`: fake SDK containers for metadata, CRUD, consistency, paging, cache interaction, statistics, batches, and patch behavior.
- `internal/exec/guard_test.go`: cross-partition rejection before SDK execution.
- `internal/cache/cache_test.go`: FIFO, expiry, copying, and invalidation generations.
- `tuner/storage_test.go` and `tuner/sqlite/storage_test.go`: storage, recommendations, paths/locks, retention, and transactional failure behavior.
- `gosmos_test.go`: public facade compatibility. `examples/darkmatter/main_test.go` covers the example harness.
- `tools/UniverseCorpusExporter.Tests`: optional .NET 10 console regression checks, run with `dotnet run --project tools/UniverseCorpusExporter.Tests`. Links the exporter's `FixtureOutput.cs` without Universe or third-party test packages. The exporter writes exclusively into a private user-temporary staging directory and publishes by directory move to a nonexistent output. Output and staging must share a filesystem, and their parents must be trusted. See its README for permissions and failure behavior; normal Go checks remain independent of .NET.

Fake-container tests prove library behavior; they do not prove Cosmos feature availability or deployed indexing policies.

## Examples and documentation

`examples/query` generates SQL without a client. `examples/darkmatter` provides eighteen scenarios and defaults to offline generation, using synthetic credentials only to construct a client. Only `-live` permits requests; `-all -live` is rejected. Live examples 5, 6, and 17 write synthetic documents and attempt cleanup. Use a dedicated account; examples do not provision resources or configure policies.

Documentation entry points:

- `AGENTS.md`, `CLAUDE.md`: agent workflow, contribution rules, and verification expectations (Codex and Claude Code respectively); architecture details stay in this map. Keep their shared rules in sync.
- `README.md`: public overview and development commands.
- `docs/GETTING_STARTED.md`: client/repository setup, entities, keys, and local module use.
- `docs/FLUENT_QUERY_BUILDER.md`, `docs/DECLARATIVE_QUERY_SYNTAX.md`: fluent and structured inputs.
- `docs/QUERY_EXECUTION.md`, `docs/CROSS_PARTITION.md`: result semantics, paging, consistency, hints, and execution boundaries.
- `docs/VECTOR_SEARCH.md`, `docs/FULLTEXT_SEARCH.md`: search prerequisites and construction.
- `docs/BATCHES_AND_PATCH.md`: ETags, transaction boundaries, partial success, and patch paths.
- `docs/CACHING_AND_STATISTICS.md`: cache ownership, persistence, and advisory recommendations.
- `examples/README.md`: scenario index and live-run requirements.

Keep this map aligned when ownership, dependencies, public behavior, or verification commands change. Keep public examples aligned with exported APIs and JSON names. Product documentation should describe Gosmos behavior and limits; upstream revision reports, agent progress, and consumer-specific investigations do not belong there.
