// Package query builds parameterized Cosmos DB SQL without an SDK dependency.
package query

// Operator identifies a supported predicate or ranking operation.
type Operator string

const (
	Eq               Operator = "Eq"
	NotEq            Operator = "NotEq"
	Gt               Operator = "Gt"
	Gte              Operator = "Gte"
	Lt               Operator = "Lt"
	Lte              Operator = "Lte"
	In               Operator = "In"
	NotIn            Operator = "NotIn"
	Contains         Operator = "Contains"
	NotContains      Operator = "NotContains"
	Len              Operator = "Len"
	Like             Operator = "Like"
	NotLike          Operator = "NotLike"
	Defined          Operator = "Defined"
	NotDefined       Operator = "NotDefined"
	VectorDistance   Operator = "VectorDistance"
	FTContains       Operator = "FTContains"
	NotFTContains    Operator = "NotFTContains"
	FTContainsAll    Operator = "FTContainsAll"
	NotFTContainsAll Operator = "NotFTContainsAll"
	FTContainsAny    Operator = "FTContainsAny"
	NotFTContainsAny Operator = "NotFTContainsAny"
	FTScore          Operator = "FTScore"
	StartsWith       Operator = "StartsWith"
	StringContains   Operator = "StringContains"
	StringEquals     Operator = "StringEquals"
	IsNull           Operator = "IsNull"
	IsString         Operator = "IsString"
)

var operators = map[Operator]string{Eq: "=", NotEq: "!=", Gt: ">", Gte: ">=", Lt: "<", Lte: "<=", In: "ARRAY_CONTAINS", NotIn: "NOT ARRAY_CONTAINS", Contains: "ARRAY_CONTAINS", NotContains: "NOT ARRAY_CONTAINS", Len: "ARRAY_LENGTH", Like: "LIKE", NotLike: "NOT LIKE", Defined: "IS_DEFINED", NotDefined: "NOT IS_DEFINED", VectorDistance: "VectorDistance", FTContains: "FullTextContains", NotFTContains: "NOT FullTextContains", FTContainsAll: "FullTextContainsAll", NotFTContainsAll: "NOT FullTextContainsAll", FTContainsAny: "FullTextContainsAny", NotFTContainsAny: "NOT FullTextContainsAny", FTScore: "FullTextScore", StartsWith: "STARTSWITH", StringContains: "CONTAINS", StringEquals: "STRINGEQUALS", IsNull: "IS_NULL", IsString: "IS_STRING"}

// Where joins adjacent predicates or clusters. Its zero value means AND.
type Where string

const (
	And Where = "AND"
	Or  Where = "OR"
)

// Aggregate names a supported aggregate.
type Aggregate string

const (
	Count Aggregate = "Count"
	Sum   Aggregate = "Sum"
	Min   Aggregate = "Min"
	Max   Aggregate = "Max"
	Avg   Aggregate = "Avg"
)

// Direction controls scalar ordering or RRF weights. Its zero value means ASC.
type Direction string

const (
	Asc      Direction = "ASC"
	Desc     Direction = "DESC"
	Weighted Direction = "WEIGHTED"
)
const (
	MaxItems               = 1000
	MaxVectorItems         = 50
	MaxVectorDimensions    = 4096
	MaxContinuationTokenKB = 16
	MaxRU                  = 1000
)

// Input describes the complete query. NamingPolicy applies to each source path segment.
type Input struct {
	Clusters     []Cluster
	Columns      ColumnOptions
	Sorting      []Sorting
	Groups       []string
	NamingPolicy func(string) string
}
type ColumnOptions struct {
	Names      []string
	Top        int
	IsDistinct bool
	Aggregates []AggregationOption
	Join       *JoinOptions
	Projection *Projection
}
type JoinOptions struct {
	Alias      string
	ArrayPath  string
	Columns    []string
	Aggregates []AggregationOption
}
type AggregationOption struct {
	Column    string
	Aggregate Aggregate
}
type Sorting struct {
	Column    string
	Direction Direction
}
type Page struct {
	Size              int
	ContinuationToken string
}
type Cluster struct {
	Catalysts []Catalyst
	Where     Where
}

// Catalyst is a predicate or a rank expression. The zero Operator means Eq.
type Catalyst struct {
	Column       string
	Alias        string
	ID           string
	Operator     Operator
	Value        any
	Where        Where
	IgnoreCase   bool
	PartialMatch bool
	Left, Right  *Operand
}

// OperandKind is a field, an array length, or a bound parameter.
type OperandKind string

const (
	FieldOperand       OperandKind = "Field"
	ArrayLengthOperand OperandKind = "ArrayLength"
	ParameterOperand   OperandKind = "Parameter"
)

type Operand struct {
	Kind        OperandKind
	Path, Alias string
	Value       any
}

func Field(path string, alias ...string) Operand {
	return Operand{Kind: FieldOperand, Path: path, Alias: operandAlias(alias)}
}
func ArrayLength(path string, alias ...string) Operand {
	return Operand{Kind: ArrayLengthOperand, Path: path, Alias: operandAlias(alias)}
}
func Parameter(value any) Operand { return Operand{Kind: ParameterOperand, Value: value} }
func operandAlias(alias []string) string {
	if len(alias) > 0 {
		return alias[0]
	}
	return "c"
}

// Compare constructs a comparison; Build validates operand kinds and operator.
func Compare(left Operand, op Operator, right Operand) Catalyst {
	return Catalyst{Column: left.Path, Alias: left.Alias, Operator: op, Value: right.Value, Left: &left, Right: &right}
}

type Param struct {
	Name  string
	Value any
}

// Shape records structural features without parsing SQL or inspecting parameter values.
type Shape struct{ OrderBy, Top, Distinct, Aggregate, GroupBy, Vector, FullText, RRF, Join bool }
type Statement struct {
	Text   string
	Params []Param
	Shape  Shape
}

// PredicateOption customizes the supported behavior of a Catalyst.
type PredicateOption func(*Catalyst)

// IgnoreCase enables case-insensitive literal string matching.
var IgnoreCase PredicateOption = func(c *Catalyst) { c.IgnoreCase = true }

// PartialMatch enables partial-object matching for In and NotIn predicates.
var PartialMatch PredicateOption = func(c *Catalyst) { c.PartialMatch = true }
