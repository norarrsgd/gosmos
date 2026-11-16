package repository

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

type WriteOption func(*azcosmos.ItemOptions) error

// IfMatch applies optimistic concurrency to replace and delete operations.
func IfMatch(etag string) WriteOption {
	return func(o *azcosmos.ItemOptions) error {
		if etag == "" {
			return validation("ETag is empty")
		}
		v := azcore.ETag(etag)
		o.IfMatchEtag = &v
		return nil
	}
}
