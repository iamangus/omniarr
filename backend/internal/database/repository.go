package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"omniarr/internal/config"
	"omniarr/internal/domain"
	"omniarr/internal/utils"
)

type EntityRepository struct {
	db       *sqlx.DB
	config   *config.CatalogConfig
	mockData map[string]*domain.Entity
	isMock   bool
}

func NewEntityRepository(db *sqlx.DB, cfg *config.CatalogConfig) *EntityRepository {
	return &EntityRepository{
		db:     db,
		config: cfg,
		isMock: false,
	}
}

func NewMockRepository(cfg *config.CatalogConfig) *EntityRepository {
	return &EntityRepository{
		config:   cfg,
		mockData: make(map[string]*domain.Entity),
		isMock:   true,
	}
}

func (r *EntityRepository) Get(ctx context.Context, uuid string) (*domain.Entity, error) {
	if r.isMock {
		if entity, ok := r.mockData[uuid]; ok {
			// Return a copy to simulate DB behavior
			e := *entity
			return &e, nil
		}
		return nil, fmt.Errorf("entity not found")
	}

	var entity domain.Entity
	err := r.db.GetContext(ctx, &entity, "SELECT * FROM entities WHERE uuid = $1", uuid)
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *EntityRepository) GetChildren(ctx context.Context, parentUUID string) ([]domain.Entity, error) {
	if r.isMock {
		var entities []domain.Entity
		for _, entity := range r.mockData {
			if entity.ParentUUID != nil && entity.ParentUUID.String() == parentUUID {
				entities = append(entities, *entity)
			}
		}
		return entities, nil
	}

	var entities []domain.Entity
	err := r.db.SelectContext(ctx, &entities, "SELECT * FROM entities WHERE parent_uuid = $1", parentUUID)
	if err != nil {
		return nil, err
	}
	return entities, nil
}

func (r *EntityRepository) Find(ctx context.Context, criteria map[string]interface{}) ([]domain.Entity, error) {
	if r.isMock {
		var entities []domain.Entity
		for _, entity := range r.mockData {
			match := true
			for key, value := range criteria {
				// Simple matching for mock mode
				switch key {
				case "status":
					if string(entity.Status) != value {
						match = false
					}
				case "monitored":
					if entity.Monitored != value {
						match = false
					}
				case "entity_type":
					if entity.EntityType != value {
						match = false
					}
				// Add other fields as needed
				}
				if !match {
					break
				}
			}
			if match {
				entities = append(entities, *entity)
			}
		}
		return entities, nil
	}

	query := "SELECT * FROM entities WHERE 1=1"
	var args []interface{}
	i := 1

	for key, value := range criteria {
		query += fmt.Sprintf(" AND %s = $%d", key, i)
		args = append(args, value)
		i++
	}

	var entities []domain.Entity
	err := r.db.SelectContext(ctx, &entities, query, args...)
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// FindStale returns entities that haven't been refreshed since the given time
func (r *EntityRepository) FindStale(ctx context.Context, olderThan time.Time) ([]domain.Entity, error) {
	if r.isMock {
		var entities []domain.Entity
		for _, entity := range r.mockData {
			if entity.LastRefreshedAt == nil || entity.LastRefreshedAt.Before(olderThan) {
				entities = append(entities, *entity)
			}
		}
		return entities, nil
	}

	var entities []domain.Entity
	query := "SELECT * FROM entities WHERE last_refreshed_at IS NULL OR last_refreshed_at < $1"
	err := r.db.SelectContext(ctx, &entities, query, olderThan)
	if err != nil {
		return nil, err
	}
	return entities, nil
}

func (r *EntityRepository) Save(ctx context.Context, e *domain.Entity) error {
	if r.isMock {
		r.mockData[e.UUID.String()] = e
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Upsert Entity
	query := `
		INSERT INTO entities (uuid, parent_uuid, entity_type, status, monitored, last_refreshed_at, quality_profile_id, local_path, metadata)
		VALUES (:uuid, :parent_uuid, :entity_type, :status, :monitored, :last_refreshed_at, :quality_profile_id, :local_path, :metadata)
		ON CONFLICT (uuid) DO UPDATE SET
		status = :status,
		monitored = :monitored,
		last_refreshed_at = :last_refreshed_at,
		quality_profile_id = :quality_profile_id,
		local_path = :local_path,
		metadata = :metadata
	`
	_, err = tx.NamedExecContext(ctx, query, e)
	if err != nil {
		return fmt.Errorf("failed to upsert entity: %w", err)
	}

	// 2. Update Identifiers
	// First, delete existing identifiers for this entity
	_, err = tx.ExecContext(ctx, "DELETE FROM idx_identifiers WHERE entity_uuid = $1", e.UUID)
	if err != nil {
		return fmt.Errorf("failed to delete old identifiers: %w", err)
	}

	// Extract and insert new identifiers
	identifiers, err := r.extractIdentifiers(e)
	if err != nil {
		// Log error but maybe don't fail the whole save? 
		// For now, let's fail to ensure data consistency.
		return fmt.Errorf("failed to extract identifiers: %w", err)
	}

	for _, id := range identifiers {
		_, err := tx.ExecContext(ctx, "INSERT INTO idx_identifiers (entity_uuid, key, value) VALUES ($1, $2, $3)", id.EntityUUID, id.Key, id.Value)
		if err != nil {
			return fmt.Errorf("failed to insert identifier %s: %w", id.Key, err)
		}
	}

	return tx.Commit()
}

func (r *EntityRepository) extractIdentifiers(e *domain.Entity) ([]domain.Identifier, error) {
	var identifiers []domain.Identifier
	
	// Find the endpoint config for this entity type
	var endpointConfig *config.EndpointMapping
	for _, ep := range r.config.Endpoints {
		if ep.EntityType == e.EntityType {
			endpointConfig = &ep
			break
		}
	}

	if endpointConfig == nil {
		return identifiers, nil // No config found, no identifiers to extract
	}

	if len(e.Metadata) == 0 {
		return identifiers, nil
	}

	var metadataMap map[string]interface{}
	if err := json.Unmarshal(e.Metadata, &metadataMap); err != nil {
		return nil, err
	}

	for _, idMap := range endpointConfig.Identifiers {
		val := utils.ExtractValue(metadataMap, idMap.Source)
		if val != nil {
			identifiers = append(identifiers, domain.Identifier{
				EntityUUID: e.UUID,
				Key:        idMap.Key,
				Value:      fmt.Sprintf("%v", val),
			})
		}
	}

	return identifiers, nil
}
func (r *EntityRepository) Delete(ctx context.Context, uuid string) error {
	if r.isMock {
		delete(r.mockData, uuid)
		return nil
	}
	_, err := r.db.ExecContext(ctx, "DELETE FROM entities WHERE uuid = $1", uuid)
	return err
}