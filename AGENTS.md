# AGENTS.md

Read `CODEBASE.md` first. It contains the repository map, architecture boundaries, implementation nuances, test locations, and documentation index. Keep architectural detail there; this file defines how to work in the repository.

## Project overview

Gosmos is a Go library for Azure Cosmos DB with a standalone SQL builder and typed repository API. Preserve the existing domain names: `Galaxy`, `Orbit`, `Cluster`, `Catalyst`, and `Gravity`.

Requires Go 1.27. The root module uses the Azure `azcosmos` SDK; entity serialization uses `encoding/json`. The optional SQLite statistics adapter is a separate module under `tuner/sqlite/`.

## Source boundaries

- The root package exposes aliases and forwarding functions through `gosmos.go`, `options.go`, and `query.go`. Keep repository implementation under `internal/repository/`.
- `query/` owns SQL generation and imports only the standard library. Do not introduce SDK, repository, or tuner dependencies there.
- `internal/exec/` owns cross-partition execution policy; `internal/cache/` owns document-byte storage.
- `tuner/` owns statistics contracts, memory/file storage, and recommendations. Keep the SQLite driver in `tuner/sqlite/`.
- Add tests beside the implementation. Keep `gosmos_test.go` testing the public facade as an external consumer.
- Inspect neighboring files before adding a file, helper, or abstraction. Reuse the established package structure.

## Development commands

Run from the repository root:

```sh
go build ./...
go vet ./...
go test -race ./...
(cd tuner/sqlite && go test -race ./...)
```

Root `./...` commands do not cover the nested SQLite module. Run its tests when changing shared statistics contracts or adapter behavior. Format changed Go files with `gofmt`; avoid unrelated formatting changes.

Offline examples:

```sh
go run ./examples/query
go run ./examples/darkmatter -all
```

Normal Go checks use vendored query fixtures and require neither .NET nor a Cosmos account. `.github/workflows/ci.yml` also checks formatting and runs the standalone query example. `.github/workflows/fuzz.yml` exercises identifier validation, normalization, and SQL construction.

## Query and execution rules

- Bind values as parameters. Validate identifiers and aliases before inserting them into SQL, including after naming-policy transformation. Add injection cases when expanding any SQL-fragment surface.
- Keep `query.Statement.Shape` accurate when adding predicates, projections, or ranking behavior. Execution relies on it to reject unsupported cross-partition shapes before network requests.
- Use `query.Normalize` for parameter normalization; it preserves quoted text and repeated parameter references. Do not assume generated parameter names are stable.
- Preserve the separation between SQL generation and execution. `GenerateQuery` performs no network requests and does not enforce partition execution policy.
- Gosmos has no client-side fan-out or result-merge engine. Do not silently route unsupported shapes around the guard.
- Keep repositories safe to share between goroutines. Builders are mutable; use a fresh `Orbit` for concurrent operations.
- Preserve query `Get` shape rewriting, paging through empty service pages, and grouped paging behavior. Consult `CODEBASE.md` before changing these paths.
- Execution strategy labels and compatibility hints do not imply SDK transport switching. Tuner recommendations are advisory and are not automatically applied.

## Entity and write rules

- Keep JSON names, query paths, projection outputs, and partition-key paths aligned. `WithNamingPolicy` changes source query paths, not entity serialization.
- Preserve optional entity capabilities and returned-copy semantics for generated IDs and timestamps. Embed `Base` by value.
- Require complete ordered string partition keys. Tagged keys have one to three contiguous levels; partial hierarchical keys are unsupported.
- Preserve transaction boundaries: never split `Atomic` into multiple requests. `Bulk` can partially commit; keep original result order and mark failed or unattempted operations accurately.
- Preserve ETag preconditions, patch path validation, and SDK error chains. Use `errors.Is` for library sentinels and `errors.As` for SDK response errors.
- Do not interpret a transport error as proof that a write did not commit, or retry an entire partially successful bulk operation automatically.

## Cache, statistics, and security

- Cache ownership is repository-local. Preserve conservative invalidation and generation checks against stale in-flight reads.
- Any explicit read-consistency level bypasses cache lookup and population. Do not satisfy an explicit consistency request from cached data.
- Preserve statistics-storage errors separately in `Gravity.StatisticsError`; they must not replace database results or primary errors.
- Keep file-storage path validation, resolved-parent pinning, context-aware shared locks, and owner-only files intact. File storage provides process-local serialization, not coordination between processes.
- Treat statistics paths as application configuration, never request input. Keep statistics free of raw parameter values.
- `WithRecordQueries(true)` and `GenerateQuery` expose SQL and bindings. Do not introduce logs containing credentials, document values, search terms, vectors, or continuation tokens.
- Construction remains free of network requests. Provisioning stays explicit through `EnsureProvisioned`; do not modify existing container policies implicitly.
- Downloading or installing external tools, applications, or SDKs requires explicit user approval. Computer use also requires explicit approval before invoking it.
- Examples are offline by default. Live examples consume RU and some write documents; use only a dedicated account within the authorized task scope.

## Documentation and verification

- Make a meaningful `CODEBASE.md` update when a feature or fix changes mapped behavior, ownership, dependencies, constraints, or verification. Do not add date-only entries or progress logs.
- Update affected public guides under `docs/`, `README.md`, and examples when exported behavior or JSON shapes change. Use the documentation index in `CODEBASE.md`.
- Test SQL changes locally, including malicious inputs where relevant. Use fake SDK containers for deterministic repository behavior; live account behavior requires separate evidence.
- Validate documentation paths and commands, review the final diff, and run `git diff --check`. Distinguish checks actually run from documented commands and unverified live behavior.

## Git workflow

- Inspect the current branch and working tree before changing workflow state. Preserve unrelated user changes and do not assume Universe's branch or publishing conventions apply here.
- When isolation is needed, use repository-local `.worktrees/`, never `~/.codex/worktrees`. Verify it is ignored and flag a missing ignore rule on the main/default branch.
- Before every commit, read the configured `commit.template` and recent commit messages, follow their format, and validate the final message.
- Include `Co-authored-by: Codex <noreply@openai.com>` in every Codex-authored commit.
- Keep implementation updates concise and technical. Report material risks, blockers, failed approaches, and verification results; omit routine narration.
