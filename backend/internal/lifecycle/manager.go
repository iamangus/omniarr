package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/domain"
	"omniarr/internal/metadata"
)

// Manager handles metadata updates and hierarchy reconciliation
type Manager struct {
	repo     *database.EntityRepository
	provider metadata.MetadataProvider
	config   *config.AppConfig
}

// NewManager creates a new instance of the Lifecycle Manager
func NewManager(repo *database.EntityRepository, provider metadata.MetadataProvider, cfg *config.AppConfig) *Manager {
	return &Manager{
		repo:     repo,
		provider: provider,
		config:   cfg,
	}
}

// Reconcile iterates over entities and refreshes them if needed
func (m *Manager) Reconcile(ctx context.Context) error {
	log.Println("Starting reconciliation process...")

	// Find entities older than 24 hours
	olderThan := time.Now().Add(-24 * time.Hour)
	entities, err := m.repo.FindStale(ctx, olderThan)
	if err != nil {
		return fmt.Errorf("failed to find stale entities: %w", err)
	}

	log.Printf("Found %d stale entities to refresh", len(entities))

	for _, entity := range entities {
		if err := m.RefreshEntity(ctx, entity.UUID.String(), nil); err != nil {
			log.Printf("Failed to refresh entity %s: %v", entity.UUID, err)
			// Continue with next entity
		}
	}

	log.Println("Reconciliation process completed.")
	return nil
}

// RefreshEntity fetches new metadata for an entity and updates it
func (m *Manager) RefreshEntity(ctx context.Context, entityUUID string, childOverrides map[string]bool) error {
	log.Printf("Refreshing entity with UUID: %s", entityUUID)

	// 1. Get entity from repo
	entity, err := m.repo.Get(ctx, entityUUID)
	if err != nil {
		return fmt.Errorf("failed to get entity: %w", err)
	}

	// 2. Extract external ID from metadata
	// We try to unmarshal into Metadata struct first
	var meta metadata.Metadata
	if len(entity.Metadata) > 0 {
		if err := json.Unmarshal(entity.Metadata, &meta); err != nil {
			// Fallback: try map if struct fails (legacy data?)
			var metaMap map[string]interface{}
			if err := json.Unmarshal(entity.Metadata, &metaMap); err == nil {
				if id, ok := metaMap["id"]; ok {
					meta.ID = fmt.Sprintf("%v", id)
				}
			}
		}
	}

	if meta.ID == "" {
		return fmt.Errorf("entity metadata missing external ID")
	}

	// 3. Fetch metadata from provider (with entity UUID for image naming)
	// Check if provider supports GetMetadataWithEntityID (metadata.Manager)
	var newMeta *metadata.Metadata

	if mgr, ok := m.provider.(*metadata.Manager); ok {
		// Use the enhanced method that passes entity UUID for deterministic image naming
		var metaErr error
		newMeta, metaErr = mgr.GetMetadataWithEntityID(ctx, entity.EntityType, meta.ID, entity.UUID.String())
		if metaErr != nil {
			return fmt.Errorf("failed to fetch metadata: %w", metaErr)
		}
	} else {
		// Fallback for other providers
		var metaErr error
		newMeta, metaErr = m.provider.GetMetadata(ctx, entity.EntityType, meta.ID)
		if metaErr != nil {
			return fmt.Errorf("failed to fetch metadata: %w", metaErr)
		}
	}

	// 4. Update entity with new metadata
	// Extract local image path if present in Extra
	if newMeta.Extra != nil {
		if localPath, ok := newMeta.Extra["_local_image_path"].(string); ok {
			entity.ImagePath = &localPath
		}
	}

	metaJSON, err := json.Marshal(newMeta)
	if err != nil {
		return fmt.Errorf("failed to marshal new metadata: %w", err)
	}

	now := time.Now()
	entity.Metadata = metaJSON
	entity.LastRefreshedAt = &now

	if err := m.repo.Save(ctx, entity); err != nil {
		return fmt.Errorf("failed to save entity: %w", err)
	}

	// 5. Handle Hierarchy
	if err := m.refreshHierarchy(ctx, entity, newMeta, childOverrides); err != nil {
		return fmt.Errorf("failed to refresh hierarchy: %w", err)
	}

	// 6. Handle Variants
	if err := m.refreshVariants(ctx, entity, newMeta); err != nil {
		return fmt.Errorf("failed to refresh variants: %w", err)
	}

	return nil
}

func (m *Manager) refreshHierarchy(ctx context.Context, parent *domain.Entity, parentMeta *metadata.Metadata, childOverrides map[string]bool) error {
	if len(parentMeta.Children) == 0 {
		return nil
	}

	log.Printf("Refreshing hierarchy for parent %s (Type: %s). Found %d children.", parent.UUID, parent.EntityType, len(parentMeta.Children))

	for _, childMeta := range parentMeta.Children {
		// Determine Child External ID
		childExternalID := childMeta.ID
		if childExternalID == "" {
			log.Printf("Child metadata missing ID, skipping")
			continue
		}

		// Determine Child Entity Type
		childType := childMeta.Type
		if childType == "" {
			// Fallback logic? Or assume same as parent? No, usually different.
			// If provider doesn't set it, we might be in trouble.
			// For now, log warning.
			log.Printf("Child metadata missing Type, skipping %s", childExternalID)
			continue
		}

		// Create/Update Child Entity
		childUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(childExternalID))

		existingChild, err := m.repo.Get(ctx, childUUID.String())
		var childEntity *domain.Entity
		if err == nil {
			childEntity = existingChild
		} else {
			// Determine monitored status
			monitored := parent.MonitorNewChildren
			if override, ok := childOverrides[childExternalID]; ok {
				monitored = override
			}

			childEntity = &domain.Entity{
				UUID:       childUUID,
				ParentUUID: &parent.UUID,
				EntityType: childType,
				Status:     domain.StatusWanted,
				Monitored:  monitored,
				LocalPath:  "",
			}

			// Set default quality profile if new
			if childConfig := m.getEntityTypeConfig(childType); childConfig != nil {
				childEntity.QualityProfileID = m.getQualityProfileID(childConfig.DefaultQualityProfile)
			}
		}

		// Extract local image path for child if present
		if childMeta.Extra != nil {
			if localPath, ok := childMeta.Extra["_local_image_path"].(string); ok {
				childEntity.ImagePath = &localPath
			}
		}

		childMetaJSON, _ := json.Marshal(childMeta)
		now := time.Now()
		childEntity.Metadata = childMetaJSON
		childEntity.LastRefreshedAt = &now

		if err := m.repo.Save(ctx, childEntity); err != nil {
			log.Printf("Failed to save child %s: %v", childExternalID, err)
			continue
		}

		// Recurse
		if err := m.refreshHierarchy(ctx, childEntity, &childMeta, nil); err != nil {
			log.Printf("Failed to refresh hierarchy for child %s: %v", childExternalID, err)
		}
	}
	return nil
}

func (m *Manager) refreshVariants(ctx context.Context, parent *domain.Entity, parentMeta *metadata.Metadata) error {
	// Find entity type config
	var typeConfig *config.EntityType
	for _, et := range m.config.Schema.Entities {
		if et.Type == parent.EntityType {
			typeConfig = &et
			break
		}
	}

	if typeConfig == nil || len(typeConfig.Variants) == 0 {
		return nil
	}

	log.Printf("Refreshing variants for parent %s (Type: %s). Found %d variants.", parent.UUID, parent.EntityType, len(typeConfig.Variants))

	for _, variantType := range typeConfig.Variants {
		// Create deterministic UUID for variant
		// We use parent UUID + variant type to ensure uniqueness and consistency
		variantUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(parent.UUID.String()+"-"+variantType))

		existingVariant, err := m.repo.Get(ctx, variantUUID.String())
		var variantEntity *domain.Entity
		if err == nil {
			variantEntity = existingVariant
		} else {
			variantEntity = &domain.Entity{
				UUID:       variantUUID,
				ParentUUID: &parent.UUID,
				EntityType: variantType,
				Status:     domain.StatusWanted,
				Monitored:  parent.Monitored, // Variants inherit monitored status
				LocalPath:  "",
			}

			// Set default quality profile if new
			if variantConfig := m.getEntityTypeConfig(variantType); variantConfig != nil {
				variantEntity.QualityProfileID = m.getQualityProfileID(variantConfig.DefaultQualityProfile)
			}
		}

		// Create variant metadata based on parent metadata
		variantMeta := *parentMeta
		variantMeta.Type = variantType
		// Append variant type to ID to make it unique in metadata if needed,
		// though internally we use UUID.
		// Ideally, the metadata ID should also be unique if we ever look it up by ID.
		variantMeta.ID = parentMeta.ID + "-" + variantType
		variantMeta.Title = fmt.Sprintf("%s (%s)", parentMeta.Title, variantType)

		// Clear children for variant to avoid recursion issues if we ever process it
		variantMeta.Children = nil

		variantMetaJSON, _ := json.Marshal(variantMeta)
		now := time.Now()
		variantEntity.Metadata = variantMetaJSON
		variantEntity.LastRefreshedAt = &now

		// Inherit image path if parent has one
		if parent.ImagePath != nil {
			variantEntity.ImagePath = parent.ImagePath
		}

		if err := m.repo.Save(ctx, variantEntity); err != nil {
			log.Printf("Failed to save variant %s: %v", variantType, err)
			continue
		}
	}

	return nil
}

func (m *Manager) getEntityTypeConfig(entityType string) *config.EntityType {
	for _, et := range m.config.Schema.Entities {
		if et.Type == entityType {
			return &et
		}
	}
	return nil
}

func (m *Manager) getQualityProfileID(name string) *int {
	if name == "" {
		return nil
	}
	for i, p := range m.config.Quality.Profiles {
		if p.Name == name {
			id := i
			return &id
		}
	}
	return nil
}
