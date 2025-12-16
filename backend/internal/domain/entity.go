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
	UUID               uuid.UUID       `db:"uuid" json:"uuid"`
	ParentUUID         *uuid.UUID      `db:"parent_uuid" json:"parent_uuid,omitempty"`
	EntityType         string          `db:"entity_type" json:"entity_type"`
	Status             EntityStatus    `db:"status" json:"status"`
	Monitored          bool            `db:"monitored" json:"monitored"`
	LastRefreshedAt    *time.Time      `db:"last_refreshed_at" json:"last_refreshed_at"`
	QualityProfileID   *int            `db:"quality_profile_id" json:"quality_profile_id"`
	LocalPath          string          `db:"local_path" json:"local_path"`
	ImagePath          *string         `db:"image_path" json:"image_path"`
	Metadata           json.RawMessage `db:"metadata" json:"metadata"` // Stored as JSONB
	MonitorNewChildren bool            `db:"monitor_new_children" json:"monitor_new_children"`
	RequestedBy        *string         `db:"requested_by" json:"requested_by"`
	RequestedAt        *time.Time      `db:"requested_at" json:"requested_at"`
	DownloadClientID   *string         `db:"download_client_id" json:"download_client_id"`
}

// Identifier represents a lookup key for an entity (e.g., imdb_id: tt12345)
type Identifier struct {
	EntityUUID uuid.UUID `db:"entity_uuid"`
	Key        string    `db:"key"`
	Value      string    `db:"value"`
}
