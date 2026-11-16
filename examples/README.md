# Gosmos examples

`examples/query` generates SQL without an SDK client. `examples/darkmatter` provides eighteen examples of Gosmos queries and repository operations.

```sh
go run ./examples/query
go run ./examples/darkmatter                 # example 18, offline
go run ./examples/darkmatter -example 9      # vector query SQL, offline
go run ./examples/darkmatter -all            # all eighteen offline examples
go test ./examples/...
```

Offline mode uses synthetic credentials solely to construct the SDK client. Construction and SQL generation perform no network requests.

## Scenarios

1. **Paging:** category filtering, ordering, page size, and continuation tokens.
2. **Grouped aggregates:** counts by category.
3. **TOP and DISTINCT:** five distinct categories in ascending order.
4. **Filtering:** category alternatives, price bounds, and defined-property checks.
5. **CRUD and caching:** create, cached reads, ETag-protected replacement, and deletion.
6. **Bulk operations:** per-partition execution and per-item outcomes.
7. **Aggregates:** category totals, averages, quantities, and counts.
8. **Reporting:** category totals filtered to a reporting period.
9. **Vector search:** top five distances for a three-dimensional vector.
10. **Full-text filtering:** contains and contains-any predicates.
11. **Hybrid ranking:** vector distance, full-text scoring, and weighted reciprocal rank fusion.
12. **SQL generation:** selected columns, filters, TOP, and descending order.
13. **Tuning:** execution hints, shape defaults, statistics, and recommendations.
14. **Input handling:** rejected identifiers and bound literal values.
15. **Collections:** membership and exclusion predicates.
16. **Typed projections:** JSON field selection and deserialization.
17. **Atomic operations:** ETag-protected transactions and patching.
18. **Structured queries:** string, null, type, partial-array, field-comparison, nested-projection, and consistency options.

## Live execution

Use a test account and an existing container with the string partition-key path `/tenantId`. Set `GOSMOS_ENDPOINT`, `GOSMOS_KEY`, `GOSMOS_DATABASE`, `GOSMOS_CONTAINER`, and `GOSMOS_PARTITION` through your secret-management process, then select an example:

```sh
go run ./examples/darkmatter -example 1 -live
```

Only `-live` permits requests. `-all -live` is rejected. Examples 5, 6, and 17 create uniquely identified synthetic documents and attempt cleanup; generated IDs are printed for recovery if a write or cleanup fails. Other live examples read existing data. Live operations consume request units.

## Requirements and limitations

- Ordered, aggregate, vector, and ranking queries use `InPartition`. Grouping or aggregation cannot be combined with ordering or ranking.
- Vector examples 9 and 11 require an embedding policy and index for a three-dimensional `embedding` field.
- Full-text examples 10 and 11 require a policy and index for `description`.
- Example 18 requests Strong consistency, which requires account support.
- Tuner recommendations require at least ten samples. Example 13 uses in-memory storage; SQLite is configured separately.
- Example 17 uses ETag preconditions. Conditional patch filter predicates and stored procedures are not supported.
- The examples do not provision resources or change account policies. Query output contains synthetic parameters; application logs should avoid recording sensitive values.
