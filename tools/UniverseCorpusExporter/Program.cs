using System.Text;
using System.Text.Json;
using Microsoft.Azure.Cosmos;
using Universe.Builder;
using Universe.Builder.Options;

const string PinnedCommit = "911dd588ad4f4a8c30e22469f93fc6af377f2d09";
if (args.Length != 2 || args[0] != PinnedCommit)
    throw new ArgumentException("Usage: exporter 911dd588ad4f4a8c30e22469f93fc6af377f2d09 OUTPUT_DIRECTORY; verify the source checkout before building.");
Directory.CreateDirectory(args[1]);
JsonSerializerOptions jsonOptions = new() { WriteIndented = true };
List<(string Name, object Input)> cases = [];

Dictionary<string, object?> Cat(string column, string op = "Eq", object? value = null, string where = "AND", string alias = "c", bool ignoreCase = false, bool partialMatch = false)
    => new() { ["column"] = column, ["operator"] = op, ["value"] = value, ["where"] = where, ["alias"] = alias, ["ignoreCase"] = ignoreCase, ["partialMatch"] = partialMatch };
object Cluster(params object[] catalysts) => new { catalysts, where = "AND" };
object Input(object[]? clusters = null, object? columns = null, object[]? sorting = null, string[]? groups = null)
    => new { clusters = clusters ?? [], columns = columns ?? new { }, sorting = sorting ?? [], groups = groups ?? [] };
void Predicate(string name, string op, object? value, bool ignoreCase = false, bool partialMatch = false)
    => cases.Add((name, Input([Cluster(Cat("data.value", op, value, ignoreCase: ignoreCase, partialMatch: partialMatch))])));

cases.Add(("empty", Input()));
foreach (string op in new[] { "Eq", "NotEq", "Gt", "Gte", "Lt", "Lte" }) Predicate("comparison-" + op, op, 7);
Predicate("large-integer", "Eq", 9007199254740993L);
Predicate("precise-decimal", "Eq", 1234567890.1234567890123456789m);
Predicate("array-in", "In", "owner");
Predicate("array-not-in", "NotIn", "owner");
Predicate("collection-contains", "Contains", new[] { "active", "pending" });
Predicate("collection-not-contains", "NotContains", new[] { 1, 2, 3 });
Predicate("array-length", "Len", 3);
Predicate("like", "Like", "%literal_%");
Predicate("not-like", "NotLike", "test%");
foreach (string op in new[] { "Defined", "NotDefined", "IsNull", "IsString" }) Predicate("unary-" + op, op, null);
foreach (string op in new[] { "StartsWith", "StringContains", "StringEquals" }) {
    Predicate("string-" + op, op, "hello%_' --");
    Predicate("string-ignore-case-" + op, op, "HELLO", ignoreCase: true);
}
Predicate("partial-object-in", "In", new { id = "owner" }, partialMatch: true);
Predicate("partial-object-not-in", "NotIn", new { id = "owner" }, partialMatch: true);
foreach (string op in new[] { "FTContains", "NotFTContains" }) Predicate("fulltext-" + op, op, "term");
foreach (string op in new[] { "FTContainsAll", "NotFTContainsAll", "FTContainsAny", "NotFTContainsAny" }) Predicate("fulltext-" + op, op, new[] { "one", "two" });
cases.Add(("selected-columns", Input(columns: new { names = new[] { "id", "data.owner" } })));
cases.Add(("top-distinct-sort", Input(columns: new { names = new[] { "id" }, top = 5, isDistinct = true }, sorting: [new { column = "id", direction = "DESC" }])));
cases.Add(("groups", Input(groups: ["status", "tenantId"])));
foreach (string ag in new[] { "Count", "Sum", "Min", "Max", "Avg" })
    cases.Add(("aggregate-" + ag, Input(columns: new { names = new[] { "status" }, aggregates = new[] { new { column = "total", aggregate = ag } } })));
cases.Add(("join-columns", Input(columns: new { join = new { alias = "j", arrayPath = "items", columns = new[] { "sku", "price" } } })));
cases.Add(("join-aggregate", Input(columns: new { names = new[] { "id" }, join = new { alias = "j", arrayPath = "items", columns = new[] { "sku" }, aggregates = new[] { new { column = "price", aggregate = "Sum" } } } })));
cases.Add(("multiple-clusters", Input([Cluster(Cat("status", value: "active"), Cat("total", "Gte", 10, where: "OR")), new { where = "OR", catalysts = new[] { Cat("tenantId", value: "tenant") } }])));
object Operand(string kind, string path = "", string alias = "c", object? value = null) => new { kind, path, alias, value };
object Comparison(string leftKind, string leftPath, string op, object right) => new { column = leftPath, @operator = op, left = Operand(leftKind, leftPath), right };
cases.Add(("field-to-field", Input([Cluster(Comparison("Field", "data.left", "Eq", Operand("Field", "data.right")))])));
cases.Add(("array-length-to-field", Input([Cluster(Comparison("ArrayLength", "events", "Gt", Operand("Field", "queued", "share")))])));
cases.Add(("array-length-to-parameter", Input([Cluster(Comparison("ArrayLength", "events", "Gte", Operand("Parameter", value: 2)))])));
object Field(string name, string column) => new { name, column, alias = "c" };
object Projection(params object[] items) => new { items };
cases.Add(("projection-fields", Input(columns: new { projection = Projection(Field("ID", "id"), Field("OwnerName", "data.owner")) })));
cases.Add(("projection-nested", Input(columns: new { names = new[] { "id" }, projection = Projection(new { name = "data", @object = Projection(Field("OwnerName", "data.owner"), new { name = "nested", @object = Projection(Field("quote\"key", "data.ticketId")) }) }) })));
cases.Add(("projection-aggregate", Input(columns: new { projection = Projection(Field("Status", "status"), new { name = "Total", column = "total", alias = "c", aggregate = "Sum" }) })));
cases.Add(("vector-rank", Input([Cluster(Cat("embedding", "VectorDistance", new[] { 0.1f, 0.2f }))], new { names = new[] { "id" }, top = 5 })));
cases.Add(("fulltext-rank", Input([Cluster(Cat("text", "FTScore", new[] { "one", "two" }))], new { names = new[] { "id" }, top = 5 })));
cases.Add(("hybrid-rrf", Input([Cluster(Cat("embedding", "VectorDistance", new[] { 0.1f, 0.2f }), Cat("text", "FTScore", new[] { "one", "two" }))], new { names = new[] { "id" }, top = 5 }, [new { column = "0.7, 0.3", direction = "WEIGHTED" }])));
Dictionary<string, object?> firstVector = Cat("embedding", "VectorDistance", new[] { 0.1f, 0.2f });
firstVector["id"] = "vector00000001";
Dictionary<string, object?> secondVector = Cat("embedding", "VectorDistance", new[] { 0.2f, 0.3f });
secondVector["id"] = "vector00000002";
cases.Add(("multiple-vector-rrf", Input([Cluster(firstVector, secondVector)], new { names = new[] { "id" }, top = 5 })));
foreach (string value in new[] { "x]", "x\"", "x; DROP", "x--", "x/*", "x\u0000", new string('x', 256) }) Predicate("reject-identifier-" + cases.Count, "Eq", 1);
// Replace each rejection's column while retaining its value as data.
string[] malicious = ["x]", "x\"", "x; DROP", "x--", "x/*", "x\u0000", new string('x', 256)];
for (int i = 0; i < malicious.Length; i++) cases[cases.Count - malicious.Length + i] = ("reject-identifier-" + i, Input([Cluster(Cat(malicious[i], value: 1))]));
Predicate("reject-null-equality", "Eq", null);
Predicate("reject-string-number", "StringContains", 1);
Predicate("reject-ignore-case", "Eq", 1, ignoreCase: true);
Predicate("reject-partial-match", "Eq", 1, partialMatch: true);
Predicate("reject-contains-scalar", "Contains", "scalar");
Predicate("reject-unary-value", "IsNull", "value");
Predicate("reject-fulltext-empty", "FTContainsAll", Array.Empty<string>());
cases.Add(("reject-top", Input(columns: new { top = 1001 })));
cases.Add(("reject-rank-without-top", Input([Cluster(Cat("text", "FTScore", new[] { "term" }))])));
cases.Add(("reject-join-alias", Input(columns: new { join = new { alias = "SELECT", arrayPath = "items", columns = Array.Empty<string>() } })));
cases.Add(("reject-projection-collision", Input(columns: new { names = new[] { "id" }, projection = Projection(Field("id", "other")) })));

foreach ((string name, object input) in cases) {
    JsonElement data = JsonSerializer.SerializeToElement(input);
    string? sql = null;
    List<object?> parameters = [];
    string? category = null;
    try {
        using UniverseBuilder builder = new();
        QueryDefinition query = builder.CreateQuery(ReadClusters(data), ReadColumns(data), ReadSorting(data), data.GetProperty("groups").Deserialize<string[]>());
        (sql, List<string> names) = Normalize(query.QueryText);
        Dictionary<string, object> values = query.GetQueryParameters().ToDictionary(p => p.Name, p => p.Value);
        foreach (string parameter in names) parameters.Add(values[parameter]);
    } catch (Exception exception) when (exception is Universe.Exception.UniverseException || exception is ArgumentException) {
        category = "validation";
    }
    bool reject = name.StartsWith("reject-");
    if (reject != (category is not null)) throw new InvalidOperationException($"Unexpected outcome for {name}: {sql ?? category}");
    object output = new { schemaVersion = 1, universeCommit = PinnedCommit, name, tags = new[] { reject ? "reject" : "parity" }, input = data, sql, @params = parameters, errorCategory = category };
    File.WriteAllText(Path.Combine(args[1], name + ".json"), JsonSerializer.Serialize(output, jsonOptions) + "\n");
}
Console.WriteLine($"Exported {cases.Count} cases from Universe {PinnedCommit}.");

static string Text(JsonElement e, string name, string fallback = "") => e.TryGetProperty(name, out JsonElement p) && p.ValueKind == JsonValueKind.String ? p.GetString()! : fallback;
static bool Bool(JsonElement e, string name) => e.TryGetProperty(name, out JsonElement p) && p.ValueKind == JsonValueKind.True;
static object? Value(JsonElement value) => value.ValueKind switch { JsonValueKind.Null => null, JsonValueKind.String => value.GetString(), JsonValueKind.True => true, JsonValueKind.False => false, JsonValueKind.Number => value.TryGetInt64(out long number) ? number : value.GetDecimal(), JsonValueKind.Array => value.EnumerateArray().Select(Value).ToArray(), _ => value.Clone() };
static QueryOperand ReadOperand(JsonElement e) => Text(e,"kind") switch { "Field" => QueryOperand.Field(Text(e,"path"),Text(e,"alias","c")), "ArrayLength" => QueryOperand.ArrayLength(Text(e,"path"),Text(e,"alias","c")), "Parameter" => QueryOperand.Parameter(Value(e.GetProperty("value"))), _ => throw new ArgumentException("invalid operand") };
static List<Cluster> ReadClusters(JsonElement data) {
    List<Cluster> clusters = [];
    foreach (JsonElement cluster in data.GetProperty("clusters").EnumerateArray()) {
        List<Catalyst> catalysts = [];
        foreach (JsonElement c in cluster.GetProperty("catalysts").EnumerateArray()) {
            Q.Operator op = Enum.Parse<Q.Operator>(Text(c,"operator","Eq"));
            Q.Where where = Text(c,"where","AND") == "OR" ? Q.Where.Or : Q.Where.And;
            if (c.TryGetProperty("left",out JsonElement left)) {catalysts.Add(Catalyst.Compare(ReadOperand(left),op,ReadOperand(c.GetProperty("right")),where));continue;}
            object? value = null;
            if (c.TryGetProperty("value",out JsonElement v)) {
                value = op switch { Q.Operator.VectorDistance => v.Deserialize<float[]>(), Q.Operator.FTContainsAll or Q.Operator.NotFTContainsAll or Q.Operator.FTContainsAny or Q.Operator.NotFTContainsAny or Q.Operator.FTScore => v.Deserialize<string[]>(), _ => Value(v) };
            }
            catalysts.Add(new(Text(c,"column"),value,where,op,Text(c,"alias","c")) { CatalystId = Text(c,"id",Guid.NewGuid().ToString("N")), IgnoreCase = Bool(c,"ignoreCase"), PartialMatch = Bool(c,"partialMatch") });
        }
        clusters.Add(new(catalysts,Text(cluster,"where","AND") == "OR" ? Q.Where.Or : Q.Where.And));
    }
    return clusters;
}
static List<AggregationOption>? ReadAggregates(JsonElement data) => data.TryGetProperty("aggregates",out JsonElement a) ? a.EnumerateArray().Select(v => new AggregationOption(Text(v,"column"),Enum.Parse<Q.Aggregate>(Text(v,"aggregate")))).ToList() : null;
static QueryProjection ReadProjection(JsonElement data) {
    ProjectionBuilder b = new();
    FillProjection(b,data);
    return b.Build();
}
static void FillProjection(ProjectionBuilder b, JsonElement data) {
    foreach (JsonElement i in data.GetProperty("items").EnumerateArray()) {
        string name = Text(i,"name");
        if (i.TryGetProperty("object",out JsonElement nested)) b.Object(name,p => FillProjection(p,nested));
        else if (i.TryGetProperty("aggregate",out JsonElement ag)) b.Aggregate(name,Text(i,"column"),Enum.Parse<Q.Aggregate>(ag.GetString()!),Text(i,"alias","c"));
        else b.Field(name,Text(i,"column"),Text(i,"alias","c"));
    }
}
static ColumnOptions ReadColumns(JsonElement data) {
    JsonElement e = data.GetProperty("columns");
    IReadOnlyList<string>? names = e.TryGetProperty("names",out JsonElement n) ? n.Deserialize<string[]>() : null;
    JoinOptions? join = null;
    if (e.TryGetProperty("join",out JsonElement j)) join = new(Text(j,"alias"),Text(j,"arrayPath"),j.GetProperty("columns").Deserialize<string[]>(),ReadAggregates(j));
    ColumnOptions c = new(names,Bool(e,"isDistinct"),e.TryGetProperty("top",out JsonElement top) ? top.GetInt32() : 0,ReadAggregates(e),join);
    if (e.TryGetProperty("projection",out JsonElement p)) c = c with { Projection = ReadProjection(p) };
    return c;
}
static List<Sorting.Option> ReadSorting(JsonElement data) => data.GetProperty("sorting").EnumerateArray().Select(s => new Sorting.Option(Text(s,"column"),Enum.Parse<Sorting.Direction>(Text(s,"direction","ASC")))).ToList();
static (string,List<string>) Normalize(string sql) {
    StringBuilder b = new(); List<string> names = []; char quote = '\0';
    for (int i = 0; i < sql.Length; i++) {
        char c = sql[i];
        if (quote != '\0') { b.Append(c); if (c == '\\' && i + 1 < sql.Length) b.Append(sql[++i]); else if (c == quote) { if (i + 1 < sql.Length && sql[i+1] == quote) b.Append(sql[++i]); else quote = '\0'; } continue; }
        if (c is '\'' or '"') quote = c;
        else if (c == '@') { int end = i+1; while (end < sql.Length && (char.IsLetterOrDigit(sql[end]) || sql[end] == '_')) end++; if (end > i+1) { string name = sql[i..end]; int index = names.IndexOf(name); if (index < 0) {index = names.Count; names.Add(name);} b.Append("@p").Append(index); i=end-1; continue; } }
        b.Append(c);
    }
    return (b.ToString(),names);
}
