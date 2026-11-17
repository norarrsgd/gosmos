package repository

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

type itemContainer interface {
	CreateItem(context.Context, azcosmos.PartitionKey, []byte, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	ReplaceItem(context.Context, azcosmos.PartitionKey, string, []byte, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	DeleteItem(context.Context, azcosmos.PartitionKey, string, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	ReadItem(context.Context, azcosmos.PartitionKey, string, *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
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
	m, err = m.withPartitionKeyCount(config.partitionKeyCount)
	if err != nil {
		return nil, wrapError("new", m.name, err)
	}
	c, err := client.NewContainer(database, container)
	if err != nil {
		return nil, wrapError("new", m.name, err)
	}
	return &Galaxy[T]{container: c, client: client, databaseID: database, containerID: container, metadata: m, options: config}, nil
}
