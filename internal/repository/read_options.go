package repository

import "github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"

// ReadOptions overrides the repository's read consistency for one request.
type ReadOptions struct{ ConsistencyLevel *azcosmos.ConsistencyLevel }

func copyConsistency(level *azcosmos.ConsistencyLevel) (*azcosmos.ConsistencyLevel, error) {
	if level == nil {
		return nil, nil
	}
	switch *level {
	case azcosmos.ConsistencyLevelStrong, azcosmos.ConsistencyLevelBoundedStaleness, azcosmos.ConsistencyLevelSession, azcosmos.ConsistencyLevelConsistentPrefix, azcosmos.ConsistencyLevelEventual:
		copy := *level
		return &copy, nil
	default:
		return nil, validation("invalid consistency level %q", *level)
	}
}
