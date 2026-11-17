// Package gosmos provides typed repositories and fluent queries for Azure Cosmos DB.
// Use query for standalone SQL generation and tuner for query statistics.
package gosmos

import (
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos/internal/repository"
)

// Entity is the minimal document contract.
type Entity = repository.Entity

type IDSetter = repository.IDSetter
type Stamper = repository.Stamper

// PartitionKeyer overrides partition key tags for document writes.
type PartitionKeyer = repository.PartitionKeyer

// Base provides the standard document fields and optional mutation capabilities.
type Base = repository.Base

// Galaxy is a concurrency-safe repository over one Cosmos container.
type Galaxy[T Entity] = repository.Galaxy[T]

// Orbit builds a query. Create a separate Orbit for each concurrent operation.
type Orbit[T Entity] = repository.Orbit[T]

type ClusterBuilder = repository.ClusterBuilder
type ListOptions = repository.ListOptions

// Gravity contains the cost and metadata of a database operation.
type Gravity = repository.Gravity

// QueryInfo is populated only when query recording is enabled.
type QueryInfo = repository.QueryInfo
type QueryParameter = repository.QueryParameter

type Strategy = repository.Strategy

const (
	Auto            = repository.Auto
	SinglePartition = repository.SinglePartition
	Gateway         = repository.Gateway
)

// Hints configures execution. Buffer/concurrency/direct hints are compatibility no-ops.
type Hints = repository.Hints

// AtomicBatch executes all operations in a single logical partition transaction.
type AtomicBatch[T Entity] = repository.AtomicBatch[T]

// BulkBatch executes ordered transaction chunks per partition, at most four partitions at once.
// Successfully committed chunks remain committed if another chunk fails.
type BulkBatch[T Entity] = repository.BulkBatch[T]

// BatchItemResult preserves the original operation order and separates service outcomes from local decoding.
type BatchItemResult[T Entity] = repository.BatchItemResult[T]
type BatchResult[T Entity] = repository.BatchResult[T]
type Patch[T Entity] = repository.Patch[T]

// Error retains operation context and the original SDK error.
type Error = repository.Error

var (
	ErrNotFound                  = repository.ErrNotFound
	ErrConflict                  = repository.ErrConflict
	ErrPreconditionFailed        = repository.ErrPreconditionFailed
	ErrValidation                = repository.ErrValidation
	ErrUnsupportedCrossPartition = repository.ErrUnsupportedCrossPartition
)

// New validates configuration and creates a repository without network requests.
func New[T Entity](client *azcosmos.Client, database, container string, options ...Option) (*Galaxy[T], error) {
	return repository.New[T](client, database, container, options...)
}
