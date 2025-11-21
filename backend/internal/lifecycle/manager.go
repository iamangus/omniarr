package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/domain"
	"omniarr/internal/metadata"
	"omniarr/internal/utils"
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
		if err := m.RefreshEntity(ctx, entity.UUID.String()); err != nil {
			log.Printf("Failed to refresh entity %s: %v", entity.UUID, err)
			// Continue with next entity
		}
	}

	log.Println("Reconciliation process completed.")
	return nil
}

// RefreshEntity fetches new metadata for an entity and updates it
func (m *Manager) RefreshEntity(ctx context.Context, entityUUID string) error {
	log.Printf("Refreshing entity with UUID: %s", entityUUID)

	// 1. Get entity from repo
	entity, err := m.repo.Get(ctx, entityUUID)
	if err != nil {
		return fmt.Errorf("failed to get entity: %w", err)
	}

	// 2. Extract external ID from metadata
	var metaMap map[string]interface{}
	if len(entity.Metadata) > 0 {
		if err := json.Unmarshal(entity.Metadata, &metaMap); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Assuming "id" is the external ID key in metadata
	externalID, ok := metaMap["id"]
	if !ok {
		// If no ID in metadata, we can't refresh.
		// This might happen for newly added entities that haven't been fetched yet?
		// But usually we add them with metadata.
		return fmt.Errorf("entity metadata missing external ID")
	}
	externalIDStr := fmt.Sprintf("%v", externalID)

	// 3. Fetch metadata from provider
	newMeta, err := m.provider.GetMetadata(ctx, entity.EntityType, externalIDStr)
	if err != nil {
		return fmt.Errorf("failed to fetch metadata: %w", err)
	}

	// 4. Update entity with new metadata
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

	// 5. Handle Hierarchy (Generic)
	if err := m.refreshHierarchy(ctx, entity, newMeta, externalIDStr); err != nil {
		return fmt.Errorf("failed to refresh hierarchy: %w", err)
	}

	return nil
}

func (m *Manager) refreshHierarchy(ctx context.Context, parent *domain.Entity, parentMeta map[string]interface{}, parentExternalID string) error {
	log.Printf("Refreshing hierarchy for parent %s (Type: %s)", parent.UUID, parent.EntityType)
	// Find endpoint config for this entity type
	var endpoint *config.EndpointMapping
	for _, ep := range m.config.Catalog.Endpoints {
		if ep.EntityType == parent.EntityType {
			endpoint = &ep
			break
		}
	}

	if endpoint == nil {
		log.Printf("No endpoint config found for type %s", parent.EntityType)
		return nil
	}
	if len(endpoint.Children) == 0 {
		log.Printf("No children configured for type %s", parent.EntityType)
		return nil
	}

	for _, childMapping := range endpoint.Children {
		log.Printf("Processing child mapping for type %s (Source: %s)", childMapping.EntityType, childMapping.Source)
		// Extract children list from metadata
		childrenRaw := utils.ExtractValue(parentMeta, childMapping.Source)
		if childrenRaw == nil {
			log.Printf("No children found at source %s", childMapping.Source)
			continue
		}

		childrenList, ok := childrenRaw.([]interface{})
		if !ok {
			log.Printf("Children data at %s is not a list", childMapping.Source)
			continue
		}

		log.Printf("Found %d children", len(childrenList))

		for _, childRaw := range childrenList {
			childMap, ok := childRaw.(map[string]interface{})
			if !ok {
				continue
			}

			// Determine Child External ID
			var childExternalID string
			
			if childMapping.IDFormat != "" {
				// Construct ID using format
				id := childMapping.IDFormat
				id = strings.ReplaceAll(id, "{ParentID}", parentExternalID)
				
				// Replace other placeholders from childMap
				for k, v := range childMap {
					placeholder := fmt.Sprintf("{%s}", k)
					if strings.Contains(id, placeholder) {
						id = strings.ReplaceAll(id, placeholder, fmt.Sprintf("%v", v))
					}
				}
				childExternalID = id
			} else if idVal, ok := childMap["id"]; ok {
				childExternalID = fmt.Sprintf("%v", idVal)
			} else {
				log.Printf("Could not determine ID for child of type %s", childMapping.EntityType)
				continue
			}

			var childMeta map[string]interface{}
			childMeta = childMap

			// If there is an endpoint for this child type, fetch full details
			if m.hasEndpoint(childMapping.EntityType) {
				log.Printf("Fetching full details for child %s (Type: %s)", childExternalID, childMapping.EntityType)
				fetchedMeta, err := m.provider.GetMetadata(ctx, childMapping.EntityType, childExternalID)
				if err != nil {
					log.Printf("Failed to fetch metadata for child %s: %v", childExternalID, err)
					// Fallback to what we have? Or skip?
					// If we can't fetch details, we might miss children (e.g. episodes).
					// But we can still save the season entity.
				} else {
					childMeta = fetchedMeta
				}
			}

			// Create/Update Child Entity
			childUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(childExternalID))
			
			existingChild, err := m.repo.Get(ctx, childUUID.String())
			var childEntity *domain.Entity
			if err == nil {
				childEntity = existingChild
			} else {
				childEntity = &domain.Entity{
					UUID:       childUUID,
					ParentUUID: &parent.UUID,
					EntityType: childMapping.EntityType,
					Status:     domain.StatusWanted,
					Monitored:  parent.Monitored,
					LocalPath:  "",
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
			if err := m.refreshHierarchy(ctx, childEntity, childMeta, childExternalID); err != nil {
				log.Printf("Failed to refresh hierarchy for child %s: %v", childExternalID, err)
			}
		}
	}
	return nil
}

func (m *Manager) hasEndpoint(entityType string) bool {
	for _, ep := range m.config.Catalog.Endpoints {
		if ep.EntityType == entityType {
			return true
		}
	}
	return false
}
