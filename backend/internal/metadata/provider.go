package metadata

import "context"

// MetadataProvider abstracts the external data source (TMDB, TVDB, etc.)
type MetadataProvider interface {
	// GetMetadata fetches metadata for a specific entity type and ID
	GetMetadata(ctx context.Context, entityType string, id string) (map[string]interface{}, error)

	// Search queries the provider for entities matching the query string
	Search(ctx context.Context, query string) ([]map[string]interface{}, error)
}