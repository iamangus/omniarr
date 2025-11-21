# Technical Design: OmniArr

## 1. Project Layout

We will adhere to the Standard Go Project Layout to ensure maintainability and idiomatic structure.

```
omniarr/
├── cmd/
│   └── omniarr/
│       └── main.go           # Application entry point
├── config/                   # Default configuration files (schema.yaml, etc.)
├── internal/
│   ├── config/               # Configuration loading and parsing logic
│   ├── database/             # Database connection, migrations, and repositories
│   ├── domain/               # Core business entities and interface definitions
│   ├── api/                  # HTTP Handlers and Router (Gin/Echo)
│   ├── managers/             # Core logic implementations
│   │   ├── lifecycle/        # Metadata syncing and tree reconciliation
│   │   ├── search/           # Search coordination and scoring
│   │   └── importer/         # File moving and renaming logic
│   └── providers/            # External service adapters
│       ├── metadata/         # TMDB, TVDB, etc.
│       ├── download/         # NZBGet, SabNZBD
│       └── indexer/          # Prowlarr/Torznab
├── pkg/                      # (Optional) Shared libraries if code needs to be imported by other projects
├── go.mod
├── go.sum
└── Dockerfile
```

## 2. Core Structs & Interfaces

### 2.1 Domain Entities (`internal/domain/entity.go`)

These structs map directly to the database schema and the generic abstraction principle.

```go
package domain

import (
	"encoding/json"
	"time"
	"github.com/google/uuid"
)

type EntityStatus string

const (
	StatusWanted     EntityStatus = "WANTED"
	StatusAvailable  EntityStatus = "AVAILABLE"
	StatusDownloaded EntityStatus = "DOWNLOADED"
	StatusMissing    EntityStatus = "MISSING"
)

// Entity represents the generic media item (Movie, Series, Season, Episode)
type Entity struct {
	UUID            uuid.UUID       `db:"uuid" json:"uuid"`
	ParentUUID      *uuid.UUID      `db:"parent_uuid" json:"parent_uuid,omitempty"`
	EntityType      string          `db:"entity_type" json:"entity_type"`
	Status          EntityStatus    `db:"status" json:"status"`
	Monitored       bool            `db:"monitored" json:"monitored"`
	LastRefreshedAt *time.Time      `db:"last_refreshed_at" json:"last_refreshed_at"`
	QualityProfileID *int           `db:"quality_profile_id" json:"quality_profile_id"`
	LocalPath       string          `db:"local_path" json:"local_path"`
	Metadata        json.RawMessage `db:"metadata" json:"metadata"` // Stored as JSONB
}

// Identifier represents a lookup key for an entity (e.g., imdb_id: tt12345)
type Identifier struct {
	EntityUUID uuid.UUID `db:"entity_uuid"`
	Key        string    `db:"key"`
	Value      string    `db:"value"`
}
```

### 2.2 Configuration Structs (`internal/config/config.go`)

These structs are used to parse the YAML configuration files that drive the application logic.

```go
package config

// SchemaConfig maps to schema.yaml
type SchemaConfig struct {
	RootEntity string       `yaml:"root_entity"`
	Entities   []EntityType `yaml:"entities"`
}

type EntityType struct {
	Type     string   `yaml:"type"`
	IsLeaf   bool     `yaml:"is_leaf"`
	Children []string `yaml:"children"`
	HasFiles bool     `yaml:"has_files"`
}

// CatalogConfig maps to catalog.yaml
type CatalogConfig struct {
	Provider  string             `yaml:"provider"`
	BaseURL   string             `yaml:"base_url"`
	Endpoints []EndpointMapping  `yaml:"endpoints"`
}

type EndpointMapping struct {
	EntityType string            `yaml:"entity_type"`
	URL        string            `yaml:"url"`
	Attributes map[string]string `yaml:"attributes"` // JSONPath mappings
	Identifiers []IdentifierMap  `yaml:"identifiers"`
}

type IdentifierMap struct {
	Key    string `yaml:"key"`
	Source string `yaml:"source"` // JSONPath
}
```

## 3. Manager Interfaces

These interfaces define the contracts for the core application logic, allowing for easier testing and modularity.

```go
package domain

import "context"

// LifecycleManager handles metadata updates and hierarchy reconciliation
type LifecycleManager interface {
	// RefreshMetadata fetches new data from the provider for stale entities
	RefreshMetadata(ctx context.Context) error
	// ReconcileTree ensures child entities exist (e.g., adding new Seasons/Episodes)
	ReconcileTree(ctx context.Context, parentEntity Entity) error
}

// SearchManager handles finding releases for WANTED items
type SearchManager interface {
	// PerformSearch triggers a search for a specific entity
	PerformSearch(ctx context.Context, entity Entity) error
	// ProcessResults parses, scores, and sends the best candidate to the download client
	ProcessResults(ctx context.Context, results []SearchResult, profile QualityProfile) error
}

// ImportManager handles the movement of files from download to library
type ImportManager interface {
	// ScanDownloadClient checks for completed downloads
	ScanDownloadClient(ctx context.Context) error
	// ImportFile moves, renames, and updates the entity status
	ImportFile(ctx context.Context, downloadID string, targetEntity Entity) error
}

// MetadataProvider abstracts the external data source (TMDB, etc.)
type MetadataProvider interface {
	GetResource(ctx context.Context, url string) ([]byte, error)
	Search(ctx context.Context, query string) ([]byte, error)
}

// DownloadClient abstracts clients like NZBGet or SabNZBD
type DownloadClient interface {
    AddNZB(ctx context.Context, url string, category string) (string, error)
    GetStatus(ctx context.Context) ([]DownloadItem, error)
}
```

## 4. Database Schema Mapping

We recommend using **sqlx** (github.com/jmoiron/sqlx). It provides a lightweight wrapper around `database/sql` that supports struct scanning, which is ideal for our use case where we need performance but also convenience.

### Why sqlx?
-   **Performance:** Closer to raw SQL than full ORMs like GORM.
-   **Control:** Essential for the "Search Index Pattern" where we manually manage the `idx_identifiers` table and JSONB columns.
-   **Struct Tags:** Uses `db:"..."` tags to map rows to structs easily.

### Implementation Example

```go
package database

import (
	"context"
	"github.com/jmoiron/sqlx"
	"omniarr/internal/domain"
)

type EntityRepository struct {
	db *sqlx.DB
}

func (r *EntityRepository) GetByID(ctx context.Context, uuid string) (*domain.Entity, error) {
	var entity domain.Entity
	// sqlx handles the scanning into the struct based on db tags
	err := r.db.GetContext(ctx, &entity, "SELECT * FROM entities WHERE uuid = $1", uuid)
	return &entity, err
}

func (r *EntityRepository) FindByIdentifier(ctx context.Context, key, value string) (*domain.Entity, error) {
	query := `
		SELECT e.* FROM entities e
		JOIN idx_identifiers i ON e.uuid = i.entity_uuid
		WHERE i.key = $1 AND i.value = $2
	`
	var entity domain.Entity
	err := r.db.GetContext(ctx, &entity, query, key, value)
	return &entity, err
}

// Save handles the upsert logic and JSONB serialization
func (r *EntityRepository) Save(ctx context.Context, e *domain.Entity) error {
    // Transaction to save entity and update identifiers
    tx, err := r.db.BeginTxx(ctx, nil)
    if err != nil {
        return err
    }
    
    // 1. Upsert Entity
    query := `
        INSERT INTO entities (uuid, parent_uuid, entity_type, status, monitored, last_refreshed_at, quality_profile_id, local_path, metadata)
        VALUES (:uuid, :parent_uuid, :entity_type, :status, :monitored, :last_refreshed_at, :quality_profile_id, :local_path, :metadata)
        ON CONFLICT (uuid) DO UPDATE SET
        status = :status,
        last_refreshed_at = :last_refreshed_at,
        metadata = :metadata
    `
    _, err = tx.NamedExecContext(ctx, query, e)
    if err != nil {
        tx.Rollback()
        return err
    }

    // 2. Update Identifiers (Logic to extract from metadata and update idx_identifiers would go here)
    // This likely involves deleting old identifiers for this UUID and inserting new ones
    
    return tx.Commit()
}