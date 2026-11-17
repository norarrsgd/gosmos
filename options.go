package gosmos

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos/internal/repository"
	"github.com/norarrsgd/gosmos/tuner"
)

type Option = repository.Option

// ReadOptions overrides the repository's read consistency for one request.
type ReadOptions = repository.ReadOptions
type WriteOption = repository.WriteOption
type PredicateOption = repository.PredicateOption

// IgnoreCase enables case-insensitive literal string matching.
var IgnoreCase = repository.IgnoreCase

// PartialMatch enables partial-object matching for In and NotIn predicates.
var PartialMatch = repository.PartialMatch

// WithNamingPolicy transforms source query path segments, not entity JSON tags or projection output names.
func WithNamingPolicy(policy func(string) string) Option {
	return repository.WithNamingPolicy(policy)
}

func WithTuner(storage tuner.Storage) Option { return repository.WithTuner(storage) }

// WithCache enables process-local document caching for this repository.
func WithCache(maxEntries int, ttl time.Duration) Option {
	return repository.WithCache(maxEntries, ttl)
}

func WithConsistency(level azcosmos.ConsistencyLevel) Option {
	return repository.WithConsistency(level)
}

func WithRecordQueries(enabled bool) Option { return repository.WithRecordQueries(enabled) }

func WithCrossPartition(enabled bool) Option { return repository.WithCrossPartition(enabled) }

// WithPartitionKeyCount declares the complete key width for an untagged PartitionKeyer.
func WithPartitionKeyCount(count int) Option { return repository.WithPartitionKeyCount(count) }

// WithAutoProvision enables explicit EnsureProvisioned calls; construction never performs I/O.
func WithAutoProvision(enabled bool) Option { return repository.WithAutoProvision(enabled) }

// IfMatch applies optimistic concurrency to replace and delete operations.
func IfMatch(etag string) WriteOption { return repository.IfMatch(etag) }
