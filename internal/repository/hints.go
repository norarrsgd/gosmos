package repository

type Strategy string

const (
	Auto            Strategy = ""
	SinglePartition Strategy = "SinglePartition"
	Gateway         Strategy = "Gateway"
)

// Hints configures execution. Buffer/concurrency/direct hints are compatibility no-ops.
type Hints struct {
	Strategy                             Strategy
	MaxItemCount                         int32
	ResponseContinuationTokenLimitInKB   int32
	MaxConcurrency, MaxBufferedItemCount int
	EnableOptimisticDirectExecution      bool
}
