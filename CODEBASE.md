# Gosmos

Gosmos provides a query builder and repository layer for Azure Cosmos DB.

- `query`: SQL generation, bound parameters, identifier validation, predicates, and projections. Imports only the standard library.
- Root package: public repository types, construction, options, and typed query entry points.
- `internal/repository`: entity metadata, partition keys, CRUD, ETags, queries, paging, batches, and patch operations.
- `internal/exec`: cross-partition execution policy.
- `internal/cache`: bounded FIFO document storage.
- `tuner`: query statistics, storage, and recommendations.
- `tuner/sqlite`: optional SQLite storage in a separate module.
- `examples`: standalone SQL generation and repository examples.
- `testdata/corpus`: query-generation regression fixtures.

## Source organization

The root exposes `gosmos` through three source files: `gosmos.go` aliases public types and forwards construction, `options.go` forwards configuration, and `query.go` forwards typed query helpers. Type aliases preserve fluent method signatures and callbacks without duplicating implementation. `gosmos_test.go` tests the root API as an external consumer.

Implementation and its tests live in `internal/repository/`, which imports `query` and `tuner`; neither imports the repository or root package. Consumers use `gosmos` for database reads and writes, `query` for standalone SQL generation, and `tuner` for statistics. The optional SQLite adapter remains a separate module under `tuner/sqlite`.

Within `internal/repository/`:

- `galaxy.go` constructs repositories; `options.go` configures them; `provisioning.go` creates missing Cosmos resources.
- `crud.go`, `read_options.go`, and `write_options.go` cover point operations and their request options.
- `orbit.go` and `cluster.go` provide the fluent query API; `select.go` derives projections from result types; `list.go` provides result entry points; `query_execution.go` owns SDK query execution, paging, caching, and statistics recording.
- `batch.go` contains shared batch preparation and execution; `atomic_batch.go` and `bulk_batch.go` contain the two public batch APIs; `patch.go` builds and executes patches.
- `entity.go` defines entity capabilities; `partitionkey.go` inspects entity metadata and resolves partition keys; `gravity.go`, `hints.go`, `errors.go`, and `uuid.go` hold operation metadata, execution hints, errors, and ID generation.

Within `query`, `build.go` assembles SQL, `catalyst.go` validates and renders predicates, `projection.go` builds and renders projections, and `ranking.go` renders ordering and ranking expressions. Within `tuner`, `storage.go` defines shared contracts and sample validation, `memory.go` and `file.go` implement storage, and `recommendation.go` analyzes samples and supplies default hints. Tests remain beside the packages they exercise; the separate SQLite module implements the same storage contract.

File statistics storage pins the resolved parent-directory path at construction and uses that path for I/O. Its context-aware lock is shared by every name for the same on-disk file, releases canceled acquisitions before I/O, and removes idle lock entries.

Requires Go 1.27. Run `go build ./...`, `go vet ./...`, and `go test -race ./...`. Check the SQLite module separately with `(cd tuner/sqlite && go test -race ./...)`.

Values are bound as parameters and identifiers are validated before rendering. SDK response errors remain accessible through `errors.As`. Unsupported cross-partition query shapes fail before network execution; Gosmos does not implement a client-side fan-out engine.

## Documentation

`README.md` is the entry point. Task-focused guides under `docs/` cover setup, fluent and declarative queries, execution, search, transactions, caching, and statistics. `examples/README.md` maps the executable examples to those capabilities.

Keep documentation examples aligned with exported APIs and JSON field names. Describe Gosmos behavior and limits; keep upstream revisions, agent progress reports, and consumer-specific investigations out of product documentation.
