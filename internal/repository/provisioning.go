package repository

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

// EnsureProvisioned explicitly creates missing resources when auto-provisioning is enabled.
// Existing containers are never modified.
func (g *Galaxy[T]) EnsureProvisioned(ctx context.Context) error {
	if !g.options.autoProvision {
		return nil
	}
	if g.metadata.override || len(g.metadata.fields) == 0 {
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
