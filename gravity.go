package gosmos

// Gravity contains the cost and metadata of a database operation.
type Gravity struct {
	// StatisticsError reports an optional statistics failure without hiding the database result.
	StatisticsError   error
	RU                float64
	ContinuationToken string
	ETag              string
	Query             *QueryInfo
}

// QueryInfo is populated only when query recording is enabled.
type QueryInfo struct {
	Text   string
	Params []QueryParameter
}
type QueryParameter struct {
	Name  string
	Value any
}
