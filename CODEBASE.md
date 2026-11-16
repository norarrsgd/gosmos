# Gosmos

Gosmos provides a query builder and repository layer for Azure Cosmos DB.

- `query`: SQL generation, bound parameters, identifier validation, predicates, and projections. Imports only the standard library.
- Root package: entity metadata, partition keys, CRUD, ETags, queries, paging, batches, and patch operations.
- `internal/exec`: cross-partition execution policy.
- `internal/cache`: bounded FIFO document storage.
- `tuner`: query statistics, storage, and recommendations.
- `tuner/sqlite`: optional SQLite storage in a separate module.
- `examples`: standalone SQL generation and repository examples.
- `testdata/corpus`: query-generation regression fixtures.

Requires Go 1.27. Run `go build ./...`, `go vet ./...`, and `go test -race ./...`. Check the SQLite module separately with `(cd tuner/sqlite && go test -race ./...)`.

Values are bound as parameters and identifiers are validated before rendering. SDK response errors remain accessible through `errors.As`. Unsupported cross-partition query shapes fail before network execution; Gosmos does not implement a client-side fan-out engine.

## Documentation

`README.md` is the entry point. Task-focused guides under `docs/` cover setup, fluent and declarative queries, execution, search, transactions, caching, and statistics. `examples/README.md` maps the executable examples to those capabilities.

Keep documentation examples aligned with exported APIs and JSON field names. Describe Gosmos behavior and limits; keep upstream revisions, agent progress reports, and consumer-specific investigations out of product documentation.
