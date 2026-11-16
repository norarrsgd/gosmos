package gosmos

import (
	"context"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/norarrsgd/gosmos/internal/cache"
	"github.com/norarrsgd/gosmos/tuner"
	"strings"
	"time"
)

type itemContainer interface {
	CreateItem(context.Context, azcosmos.PartitionKey, []byte, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	ReplaceItem(context.Context, azcosmos.PartitionKey, string, []byte, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	DeleteItem(context.Context, azcosmos.PartitionKey, string, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	ReadItem(context.Context, azcosmos.PartitionKey, string, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
}
type galaxyOptions struct {
	namingPolicy   func(string) string
	statistics     tuner.Storage
	cache          *cache.Cache
	consistency    *azcosmos.ConsistencyLevel
	recordQueries  bool
	crossPartition bool
	autoProvision  bool
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

// WithAutoProvision enables explicit EnsureProvisioned calls; construction never performs I/O.
func WithAutoProvision(enabled bool) Option {
	return func(o *galaxyOptions) error { o.autoProvision = enabled; return nil }
}

// Galaxy is a concurrency-safe repository over one Cosmos container.
type Galaxy[T Entity] struct {
	container               itemContainer
	client                  *azcosmos.Client
	databaseID, containerID string
	metadata                entityMetadata
	options                 galaxyOptions
}

// New validates configuration and creates a repository without network requests.
func New[T Entity](client *azcosmos.Client, database, container string, options ...Option) (*Galaxy[T], error) {
	if client == nil {
		return nil, wrapError("new", "", validation("client is nil"))
	}
	if strings.TrimSpace(database) == "" || strings.TrimSpace(container) == "" {
		return nil, wrapError("new", "", validation("database and container IDs are required"))
	}
	m, err := inspectEntity[T]()
	if err != nil {
		return nil, wrapError("new", "", err)
	}
	config := galaxyOptions{crossPartition: true}
	for _, option := range options {
		if option == nil {
			return nil, wrapError("new", m.name, validation("nil option"))
		}
		if err = option(&config); err != nil {
			return nil, wrapError("new", m.name, err)
		}
	}
	c, err := client.NewContainer(database, container)
	if err != nil {
		return nil, wrapError("new", m.name, err)
	}
	return &Galaxy[T]{container: c, client: client, databaseID: database, containerID: container, metadata: m, options: config}, nil
}

// EnsureProvisioned explicitly creates missing resources when auto-provisioning is enabled.
// Existing containers are never modified.
func (g *Galaxy[T]) EnsureProvisioned(ctx context.Context) error {
	if !g.options.autoProvision {
		return nil
	}
	if len(g.metadata.fields) == 0 {
		return wrapError("provision", g.metadata.name, validation("provisioning requires partition key tags"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := g.client.CreateDatabase(ctx, azcosmos.DatabaseProperties{ID: g.databaseID}, nil)
	if err != nil && !isConflict(err) {
		return wrapError("provision", g.metadata.name, err)
	}
	db, err := g.client.NewDatabase(g.databaseID)
	if err != nil {
		return wrapError("provision", g.metadata.name, err)
	}
	paths := make([]string, len(g.metadata.fields))
	for i, f := range g.metadata.fields {
		paths[i] = f.path
	}
	kind := azcosmos.PartitionKeyKindHash
	if len(paths) > 1 {
		kind = azcosmos.PartitionKeyKindMultiHash
	}
	_, err = db.CreateContainer(ctx, azcosmos.ContainerProperties{ID: g.containerID, PartitionKeyDefinition: azcosmos.PartitionKeyDefinition{Paths: paths, Kind: kind, Version: 2}}, nil)
	if isConflict(err) {
		return nil
	}
	return wrapError("provision", g.metadata.name, err)
}
func isConflict(err error) bool {
	var response *azcore.ResponseError
	return errors.As(err, &response) && response.StatusCode == 409
}
