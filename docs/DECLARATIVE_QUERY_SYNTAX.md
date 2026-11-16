# Declarative query syntax

[Documentation home](../README.md) · [Fluent builder](FLUENT_QUERY_BUILDER.md) · [Search](FULLTEXT_SEARCH.md)

Use `query.Input` when query configuration comes from application code, saved filters, or a reusable query definition. The standalone builder returns SQL and parameters without constructing an SDK client.

## Build a statement

```go
statement, err := query.Build(query.Input{
    Clusters: []query.Cluster{{Catalysts: []query.Catalyst{
        {Column: "status", Operator: query.Eq, Value: "open"},
        {Column: "total", Operator: query.Gte, Value: 100},
    }}},
    Columns: query.ColumnOptions{Names: []string{"id", "total"}, Top: 25},
    Sorting: []query.Sorting{{Column: "total", Direction: query.Desc}},
})
if err != nil {
    return err
}
_ = statement
```

With normalized parameter names:

```sql
SELECT TOP 25 c["id"], c["total"] FROM c
WHERE (c["status"] = @p0 AND c["total"] >= @p1)
ORDER BY c["total"] DESC
```

The parameter values are `"open"` and `100`. A zero-valued operator means `Eq`. The SQL builder returns ordinary validation errors; repository entry points wrap query validation with `gosmos.ErrValidation`.

## Input reference

| Field | Role |
|---|---|
| `Clusters` | Parenthesized predicate groups |
| `Columns.Names` | Selected document source paths |
| `Columns.Top` | Maximum results; zero omits TOP, 1–1,000 sets it |
| `Columns.IsDistinct` | Add DISTINCT to the selected result shape |
| `Columns.Aggregates` | Root aggregate expressions |
| `Columns.Join` | One document-array join |
| `Columns.Projection` | Named/nested output projection |
| `Sorting` | Ordered scalar fields or ranking weights |
| `Groups` | Explicit group-by source paths |
| `NamingPolicy` | Function applied to each source-path segment |

Without selected columns or a projection, the builder selects full documents. Adding aggregates changes the selection and infers grouping from selected scalar fields.

## Combine clusters

```go
clusters := []query.Cluster{
    {Catalysts: []query.Catalyst{
        {Column: "status", Value: "open"},
        {Column: "total", Operator: query.Gte, Value: 100},
    }},
    {Where: query.Or, Catalysts: []query.Catalyst{
        {Column: "status", Value: "priority"},
        {Column: "total", Operator: query.Gte, Value: 50},
    }},
}
_ = clusters
```

```sql
WHERE (c["status"] = @p0 AND c["total"] >= @p1)
   OR (c["status"] = @p2 AND c["total"] >= @p3)
```

A catalyst's `Where` joins it to the previous catalyst. A cluster's `Where` joins it to the previous cluster. Both default to AND. The first connector at either level has no preceding item to join.

The representation provides two grouping levels, not an arbitrary recursive expression tree. Normalize deeper expressions into equivalent groups in application code; do not splice raw boolean fragments into identifier strings.

## Predicate options

```go
predicates := []query.Catalyst{
    {Column: "name", Operator: query.StringContains, Value: "camera", IgnoreCase: true},
    {Column: "shares", Operator: query.In,
        Value: map[string]any{"userId": "user-1"}, PartialMatch: true},
    {Column: "archivedOn", Operator: query.NotDefined},
    query.Compare(query.ArrayLength("events"), query.Gt, query.Field("processedCount")),
}
_ = predicates
```

- `IgnoreCase` is accepted only for the three literal string operators.
- `PartialMatch` is accepted only for `In` and `NotIn`.
- `Defined`, `NotDefined`, `IsNull`, and `IsString` take no value.
- Comparison operands allow six comparison operators and no null parameter.
- Full-text multi-term operators require a non-empty `[]string` with no blank terms.
- Vector values must be supported numeric vectors; see [vector limits](VECTOR_SEARCH.md#requirements-and-limits).

A `Catalyst.ID` is an optional binding identity, not a SQL fragment. Leave it empty unless you need explicit identities. Use returned parameter names rather than deriving names from IDs.

## Build a structured projection

```go
builder := new(query.ProjectionBuilder)
builder.Field("id", "id").Object("data", func(data *query.ProjectionBuilder) {
    data.Field("state", "status").Field("amount", "total")
})
projection := builder.Build()
statement, err := query.Build(query.Input{
    Columns: query.ColumnOptions{Projection: &projection},
})
if err != nil {
    return err
}
_ = statement
```

`Build` returns a snapshot. Adding later fields to the builder does not change that snapshot. Projection output names are independent of source names; nested keys are JSON-escaped, while root aliases must meet SQL-alias validation.

For aggregate projections, selected scalar fields become grouping keys, including scalar fields nested inside an object. Ranking and ordering cannot accompany grouped or aggregate projections.

## Execute declarative inputs

Repository `List` accepts the same clusters and column settings:

```go
rows, gravity, err := orders.List(ctx, []query.Cluster{{
    Catalysts: []query.Catalyst{{Column: "status", Value: "open"}},
}}, gosmos.ListOptions{
    Columns:      query.ColumnOptions{Names: []string{"id", "status"}, Top: 20},
    Sorting:      []query.Sorting{{Column: "status", Direction: query.Asc}},
    PartitionKey: []string{"tenant-1"},
})
if err != nil {
    return err
}
_ = rows
_ = gravity
```

`ListOptions` also carries `Groups`, `ReadOptions`, and `Hints`. To inspect the SQL without executing, use `orders.GenerateQuery(clusters, options)`.

For arbitrary result types, use the fluent result helpers `ListAs`, `GetAs`, or `PagedAs`; selected fields must match the target's JSON shape.

## Bind values and inspect SQL

```go
normalized, names := query.Normalize(statement.Text)
values := make(map[string]any, len(statement.Params))
for _, parameter := range statement.Params {
    values[parameter.Name] = parameter.Value
}
_ = normalized
for _, name := range names {
    _ = values[name]
}
```

`Normalize` rewrites parameter references outside quoted text to `@p0`, `@p1`, and so on. The names slice identifies the original bindings in first-reference order. Repeated references retain their relationship to the original parameter.

Normalization is useful for comparisons and statistics. It is not a value sanitizer, an execution method, or a reason to treat SQL text as safe to log with application values.

## Identifier and value validation

Source paths use dot-separated JSON properties. A path such as `customer.name` becomes `c["customer"]["name"]`. Each segment is quoted; aliases are validated separately.

Paths reject empty/dot-only input, excessive length, control characters, quotes/brackets used as SQL delimiters, comment markers, and semicolons. Numeric and string values belong in `Value` or `Parameter`, never in a property path.

`NamingPolicy` transforms source segments before formatting and validation. It does not rename entity JSON serialization, root output aliases, or nested projection output keys.

The builder does not prove that a property exists in your container schema, that an index supports a query, or that an account policy enables a search feature. Those are execution requirements, described in the feature guides.

[Next: fluent projections →](FLUENT_QUERY_BUILDER.md#projections)
