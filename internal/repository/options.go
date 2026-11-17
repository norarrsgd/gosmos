package repository

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos/internal/cache"
	"github.com/norarrsgd/gosmos/tuner"
)

type galaxyOptions struct {
	namingPolicy      func(string) string
	statistics        tuner.Storage
	cache             *cache.Cache
	consistency       *azcosmos.ConsistencyLevel
	recordQueries     bool
	crossPartition    bool
	autoProvision     bool
	partitionKeyCount int
}

type Option func(*galaxyOptions) error

// WithNamingPolicy transforms source query path segments, not entity JSON tags or projection output names.
func WithNamingPolicy(policy func(string) string) Option {
	return func(o *galaxyOptions) error {
		if policy == nil {
			return validation("naming policy is nil")
		}
		o.namingPolicy = policy
		return nil
	}
}

func WithTuner(storage tuner.Storage) Option {
	return func(o *galaxyOptions) error {
		if storage == nil {
			return validation("statistics storage is nil")
		}
		o.statistics = storage
		return nil
	}
}

// WithCache enables process-local document caching for this repository.
func WithCache(maxEntries int, ttl time.Duration) Option {
	return func(o *galaxyOptions) error {
		if maxEntries < 1 || ttl <= 0 {
			return validation("cache requires positive capacity and TTL")
		}
		o.cache = cache.New(maxEntries, ttl)
		return nil
	}
}

func WithConsistency(level azcosmos.ConsistencyLevel) Option {
	return func(o *galaxyOptions) error { copy, err := copyConsistency(&level); o.consistency = copy; return err }
}

func WithRecordQueries(enabled bool) Option {
	return func(o *galaxyOptions) error { o.recordQueries = enabled; return nil }
}

func WithCrossPartition(enabled bool) Option {
	return func(o *galaxyOptions) error { o.crossPartition = enabled; return nil }
}

// WithPartitionKeyCount declares the complete key width for an untagged PartitionKeyer.
func WithPartitionKeyCount(count int) Option {
	return func(o *galaxyOptions) error {
		if count < 1 || count > 3 {
			return validation("partition key count must be 1 to 3")
		}
		o.partitionKeyCount = count
		return nil
	}
}

// WithAutoProvision enables explicit EnsureProvisioned calls; construction never performs I/O.
func WithAutoProvision(enabled bool) Option {
	return func(o *galaxyOptions) error { o.autoProvision = enabled; return nil }
}
