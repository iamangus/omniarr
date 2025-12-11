package metadata

import "context"

// Metadata represents the standardized metadata for an entity
type Metadata struct {
	ID          string            `json:"id"`
	Type        string            `json:"type,omitempty"` // e.g. "book", "season", "episode"
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Year        string            `json:"year"`
	Authors     []string          `json:"authors"`
	Image       string            `json:"image"`
	PageCount   int               `json:"page_count"`
	Identifiers map[string]string `json:"identifiers"`
	Children    []Metadata        `json:"children,omitempty"`
	// Extra fields can be stored here if needed, or we can add more fields as we discover them
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// MetadataProvider abstracts the external data source (TMDB, TVDB, GoogleBooks, Hardcover, etc.)
type MetadataProvider interface {
	// GetMetadata fetches metadata for a specific entity type and ID
	GetMetadata(ctx context.Context, entityType string, id string) (*Metadata, error)

	// Search queries the provider for entities matching the query string
	Search(ctx context.Context, query string) ([]Metadata, error)

	// GetLists fetches curated lists of content
	GetLists(ctx context.Context, listIDs []string) ([]Metadata, error)
}