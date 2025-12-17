package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/domain"
	"omniarr/internal/importing"
	"omniarr/internal/lifecycle"
	"omniarr/internal/metadata"
	"omniarr/internal/search"
)

type Server struct {
	SchemaConfig     *config.SchemaConfig
	QualityConfig    *config.QualityConfig
	CatalogConfig    *config.CatalogConfig
	MetadataManager  *metadata.Manager
	LifecycleManager *lifecycle.Manager
	SearchManager    *search.SearchManager
	ImportManager    *importing.ImportManager
	Repo             *database.EntityRepository
	ImageStoragePath string
}

func NewServer(
	schemaCfg *config.SchemaConfig,
	qualityCfg *config.QualityConfig,
	catalogCfg *config.CatalogConfig,
	metadataMgr *metadata.Manager,
	lifecycleMgr *lifecycle.Manager,
	searchMgr *search.SearchManager,
	importMgr *importing.ImportManager,
	repo *database.EntityRepository,
	imageStoragePath string,
) *Server {
	return &Server{
		SchemaConfig:     schemaCfg,
		QualityConfig:    qualityCfg,
		CatalogConfig:    catalogCfg,
		MetadataManager:  metadataMgr,
		LifecycleManager: lifecycleMgr,
		SearchManager:    searchMgr,
		ImportManager:    importMgr,
		Repo:             repo,
		ImageStoragePath: imageStoragePath,
	}
}

// GetConfig returns the system configuration with enhanced information for yaffw discovery
func (s *Server) buildMediaTypesInfo() []config.MediaTypeInfo {
	mediaTypes := make([]config.MediaTypeInfo, 0, len(s.SchemaConfig.Entities))

	for _, entity := range s.SchemaConfig.Entities {
		mediaType := config.MediaTypeInfo{
			Name:           entity.Type,
			DisplayName:    s.formatEntityName(entity.Type),
			Description:    s.getEntityDescription(entity.Type),
			Icon:           s.getEntityIcon(entity.Type),
			IsLeaf:         entity.IsLeaf,
			HasFiles:       entity.HasFiles,
			ParentTypes:    s.getParentTypes(entity.Type),
			ChildTypes:     entity.Children,
			Variants:       entity.Variants,
			QualityProfile: entity.DefaultQualityProfile,
		}
		mediaTypes = append(mediaTypes, mediaType)
	}

	return mediaTypes
}

// buildProviderInfo converts catalog configuration to provider information
func (s *Server) buildProviderInfo() []config.ProviderInfo {
	providers := []config.ProviderInfo{}

	if s.CatalogConfig != nil && s.CatalogConfig.Provider != "" {
		provider := config.ProviderInfo{
			Name:           s.CatalogConfig.Provider,
			Type:           "metadata",
			Available:      true, // TODO: Add actual availability check
			SupportedTypes: s.getSupportedTypesForProvider(s.CatalogConfig.Provider),
		}
		providers = append(providers, provider)
	}

	return providers
}

// getSystemFeatures returns available system features
func (s *Server) getSystemFeatures() []string {
	features := []string{}

	if s.SearchManager != nil {
		features = append(features, "search")
	}

	if s.ImportManager != nil {
		features = append(features, "import")
	}

	if s.LifecycleManager != nil {
		features = append(features, "lifecycle_management")
	}

	if s.MetadataManager != nil {
		features = append(features, "metadata_fetching")
	}

	if s.ImageStoragePath != "" {
		features = append(features, "image_serving")
	}

	return features
}

// Helper methods for formatting and descriptions
func (s *Server) formatEntityName(entityType string) string {
	switch entityType {
	case "movie":
		return "Movies"
	case "series":
		return "TV Series"
	case "season":
		return "Seasons"
	case "episode":
		return "Episodes"
	case "book":
		return "Books"
	case "chapter":
		return "Chapters"
	case "album":
		return "Albums"
	case "track":
		return "Tracks"
	default:
		// Capitalize first letter and pluralize
		if len(entityType) > 0 {
			return strings.ToUpper(entityType[:1]) + entityType[1:] + "s"
		}
		return entityType
	}
}

func (s *Server) getEntityDescription(entityType string) string {
	switch entityType {
	case "movie":
		return "Feature films and movies"
	case "series":
		return "Television series and shows"
	case "season":
		return "Seasons within a TV series"
	case "episode":
		return "Individual episodes of TV shows"
	case "book":
		return "Books and literary works"
	case "chapter":
		return "Chapters within books"
	case "album":
		return "Music albums"
	case "track":
		return "Individual music tracks"
	default:
		return "Media content"
	}
}

func (s *Server) getEntityIcon(entityType string) string {
	switch entityType {
	case "movie":
		return "movie"
	case "series":
		return "tv"
	case "season":
		return "folder_open"
	case "episode":
		return "tv_episode"
	case "book":
		return "book"
	case "chapter":
		return "description"
	case "album":
		return "album"
	case "track":
		return "music_note"
	default:
		return "folder"
	}
}

func (s *Server) getParentTypes(entityType string) []string {
	for _, entity := range s.SchemaConfig.Entities {
		for _, child := range entity.Children {
			if child == entityType {
				return append([]string{entity.Type}, s.getParentTypes(entity.Type)...)
			}
		}
	}
	return []string{s.SchemaConfig.RootEntity}
}

func (s *Server) getSupportedTypesForProvider(provider string) []string {
	// Map providers to their supported entity types
	switch provider {
	case "tmdb":
		return []string{"movie", "series", "season", "episode"}
	case "tvdb":
		return []string{"series", "season", "episode"}
	case "google_books":
		return []string{"book", "chapter"}
	case "music":
		return []string{"album", "track"}
	default:
		// Assume all types are supported for generic providers
		var types []string
		for _, entity := range s.SchemaConfig.Entities {
			types = append(types, entity.Type)
		}
		return types
	}
}

// getEntityTypeInfo returns detailed information about a specific entity type
func (s *Server) getEntityTypeInfo(entityType string) map[string]interface{} {
	for _, entity := range s.SchemaConfig.Entities {
		if entity.Type == entityType {
			return map[string]interface{}{
				"name":                    entity.Type,
				"display_name":            s.formatEntityName(entity.Type),
				"description":             s.getEntityDescription(entity.Type),
				"icon":                    s.getEntityIcon(entity.Type),
				"is_leaf":                 entity.IsLeaf,
				"has_files":               entity.HasFiles,
				"children":                entity.Children,
				"variants":                entity.Variants,
				"default_quality_profile": entity.DefaultQualityProfile,
			}
		}
	}

	// Return generic info for unknown types
	return map[string]interface{}{
		"name":                    entityType,
		"display_name":            s.formatEntityName(entityType),
		"description":             "Unknown entity type",
		"icon":                    "help_outline",
		"is_leaf":                 true,
		"has_files":               false,
		"children":                []string{},
		"variants":                []string{},
		"default_quality_profile": "",
	}
}

// GetConfig returns the system configuration with enhanced information for yaffw discovery
func (s *Server) GetConfig(c *gin.Context) {
	systemInfo := config.SystemInfo{
		Version:    "1.0.0", // TODO: Get from build info
		RootEntity: s.SchemaConfig.RootEntity,
		MediaTypes: s.buildMediaTypesInfo(),
		Architecture: config.ArchitectureInfo{
			Version: "1.0.0",
			Database: config.DatabaseInfo{
				Type:        "sqlite",
				Version:     "3.x",
				MaxEntities: 100000, // Placeholder
			},
			Storage: config.StorageInfo{
				Type:      "local",
				Path:      s.ImageStoragePath,
				Available: true,
			},
			Features: s.getSystemFeatures(),
		},
		Providers: s.buildProviderInfo(),
		Capabilities: config.CapabilitiesInfo{
			Streaming:        true,
			Transcoding:      true,
			MetadataFetching: s.CatalogConfig != nil,
			Search:           true,
			Download:         s.SearchManager != nil,
			Import:           s.ImportManager != nil,
		},
	}

	c.JSON(http.StatusOK, systemInfo)
}

// GetMediaTypes returns detailed information about available media types
func (s *Server) GetMediaTypes(c *gin.Context) {
	mediaTypes := s.buildMediaTypesInfo()
	c.JSON(http.StatusOK, gin.H{
		"media_types": mediaTypes,
		"count":       len(mediaTypes),
	})
}

// GetArchitecture returns system architecture information
func (s *Server) GetArchitecture(c *gin.Context) {
	architecture := config.ArchitectureInfo{
		Version: "1.0.0",
		Database: config.DatabaseInfo{
			Type:        "sqlite",
			Version:     "3.x",
			MaxEntities: 100000,
		},
		Storage: config.StorageInfo{
			Type:      "local",
			Path:      s.ImageStoragePath,
			Available: s.ImageStoragePath != "",
		},
		Features: s.getSystemFeatures(),
	}

	c.JSON(http.StatusOK, architecture)
}

// GetProviders returns information about configured metadata providers
func (s *Server) GetProviders(c *gin.Context) {
	providers := s.buildProviderInfo()
	c.JSON(http.StatusOK, gin.H{
		"providers": providers,
		"count":     len(providers),
	})
}

// GetEntities returns all tracked entities
func (s *Server) GetEntities(c *gin.Context) {
	criteria := make(map[string]interface{})

	// Filter by Entity Type (e.g., "movie", "series", "season", "episode")
	if t := c.Query("type"); t != "" {
		criteria["entity_type"] = t
	}

	// Filter by Parent UUID (e.g., get seasons for a show)
	if p := c.Query("parent"); p != "" {
		criteria["parent_uuid"] = p
	}

	// Filter by Status (e.g., "DOWNLOADED", "WANTED")
	if st := c.Query("status"); st != "" {
		criteria["status"] = st
	}

	entities, err := s.Repo.Find(c.Request.Context(), criteria)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, entities)
}

// GetEntity returns a single entity by UUID
func (s *Server) GetEntity(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	entity, err := s.Repo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entity not found"})
		return
	}

	// Add hierarchy context to the entity response
	response := map[string]interface{}{
		"entity":    entity,
		"hierarchy": map[string]interface{}{},
	}

	// Build hierarchy information if entity has parent
	if entity.ParentUUID != nil {
		hierarchy, err := s.buildHierarchyPath(c.Request.Context(), entity.UUID.String())
		if err == nil && len(hierarchy) > 1 {
			response["hierarchy"] = map[string]interface{}{
				"path":        hierarchy,
				"parent":      hierarchy[len(hierarchy)-2], // Parent (excluding entity itself)
				"grandparent": nil,
			}
			if len(hierarchy) > 2 {
				response["hierarchy"].(map[string]interface{})["grandparent"] = hierarchy[len(hierarchy)-3]
			}
		}
	}

	// Add entity type context
	entityTypeInfo := s.getEntityTypeInfo(entity.EntityType)
	response["entity_type_info"] = entityTypeInfo

	// DEBUG LOG
	// fmt.Printf("[API] Serving Entity %s: Metadata Keys: %v\n", entity.UUID, getJSONKeys(entity.Metadata))

	c.JSON(http.StatusOK, response)
}

func getJSONKeys(data []byte) []string {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return []string{}
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// GetEntityHierarchy returns the hierarchical path from root to this entity
func (s *Server) GetEntityHierarchy(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	// Build hierarchy path from entity to root
	hierarchy, err := s.buildHierarchyPath(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entity_id": id,
		"hierarchy": hierarchy,
		"path":      s.hierarchyToPath(hierarchy),
	})
}

// GetEntityChildren returns direct children of this entity
func (s *Server) GetEntityChildren(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	// Get children using existing repository method
	criteria := map[string]interface{}{
		"parent_uuid": id,
	}

	children, err := s.Repo.Find(c.Request.Context(), criteria)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"parent_id": id,
		"children":  children,
		"count":     len(children),
	})
}

// GetEntityDescendants returns all descendants recursively
func (s *Server) GetEntityDescendants(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	// Build complete descendant tree
	descendants, err := s.buildDescendantsTree(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entity_id":   id,
		"descendants": descendants,
	})
}

// LookupCatalog searches for new content to add
func (s *Server) LookupCatalog(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter is required"})
		return
	}

	results, err := s.MetadataManager.Search(c.Request.Context(), query)
	if err != nil {
		fmt.Printf("Metadata search failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetCatalogItem fetches details for a specific item from the metadata provider
func (s *Server) GetCatalogItem(c *gin.Context) {
	entityType := c.Query("type")
	id := c.Query("id")

	if entityType == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and id parameters are required"})
		return
	}

	result, err := s.MetadataManager.GetMetadata(c.Request.Context(), entityType, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetLists fetches curated lists from the metadata provider
func (s *Server) GetLists(c *gin.Context) {
	fmt.Println("GetLists called")
	if s.CatalogConfig == nil {
		fmt.Println("CatalogConfig is nil")
		c.JSON(http.StatusOK, []interface{}{})
		return
	}
	fmt.Printf("Configured lists: %v\n", s.CatalogConfig.Lists)

	if len(s.CatalogConfig.Lists) == 0 {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	results, err := s.MetadataManager.GetLists(c.Request.Context(), s.CatalogConfig.Lists)
	if err != nil {
		fmt.Printf("Error fetching lists: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("Fetched %d lists\n", len(results))
	c.JSON(http.StatusOK, results)
}
func (s *Server) CreateEntity(c *gin.Context) {
	var rawBody map[string]interface{}
	if err := c.ShouldBindJSON(&rawBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var entity domain.Entity

	// 1. Entity Type
	if et, ok := rawBody["entity_type"].(string); ok {
		entity.EntityType = et
	} else {
		entity.EntityType = s.SchemaConfig.RootEntity
	}

	// 2. Metadata
	// If "metadata" field exists, use it. Otherwise, use the entire body as metadata.
	if meta, ok := rawBody["metadata"].(map[string]interface{}); ok {
		metaBytes, _ := json.Marshal(meta)
		entity.Metadata = metaBytes
	} else {
		metaBytes, _ := json.Marshal(rawBody)
		entity.Metadata = metaBytes
	}

	// 3. UUID
	if idStr, ok := rawBody["uuid"].(string); ok {
		if id, err := uuid.Parse(idStr); err == nil {
			entity.UUID = id
		}
	}
	if entity.UUID == uuid.Nil {
		entity.UUID = uuid.New()
	}

	// 4. Other defaults
	entity.Status = domain.StatusWanted
	entity.Monitored = true
	now := time.Now()
	entity.RequestedAt = &now

	// 5. Child Overrides
	childOverrides := make(map[string]bool)
	if co, ok := rawBody["child_overrides"].(map[string]interface{}); ok {
		for k, v := range co {
			if b, ok := v.(bool); ok {
				childOverrides[k] = b
			}
		}
	}

	if err := s.Repo.Save(c.Request.Context(), &entity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Trigger Background Jobs (Refresh + Search)
	go func() {
		ctx := context.Background()
		// 1. Refresh Metadata
		if err := s.LifecycleManager.RefreshEntity(ctx, entity.UUID.String(), childOverrides); err != nil {
			fmt.Printf("Failed to refresh metadata for %s: %v\n", entity.UUID, err)
			return
		}

		// Reload entity to get updated metadata
		updatedEntity, err := s.Repo.Get(ctx, entity.UUID.String())
		if err != nil {
			fmt.Printf("Failed to reload entity %s: %v\n", entity.UUID, err)
			return
		}

		// 2. Search
		if err := s.SearchManager.PerformSearch(ctx, updatedEntity); err != nil {
			fmt.Printf("Search failed for %s: %v\n", entity.UUID, err)
		}
	}()

	c.JSON(http.StatusCreated, entity)
}

// UpdateEntity modifies an existing item
func (s *Server) UpdateEntity(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	// Check if entity exists
	entity, err := s.Repo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entity not found"})
		return
	}

	// Define a struct for fields we allow updating
	type UpdateEntityRequest struct {
		Monitored          *bool `json:"monitored"`
		MonitorNewChildren *bool `json:"monitor_new_children"`
		QualityProfileID   *int  `json:"quality_profile_id"`
	}

	var req UpdateEntityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Apply updates
	if req.Monitored != nil {
		entity.Monitored = *req.Monitored
	}
	if req.MonitorNewChildren != nil {
		entity.MonitorNewChildren = *req.MonitorNewChildren
	}
	if req.QualityProfileID != nil {
		entity.QualityProfileID = req.QualityProfileID
	}

	// Save updated entity
	if err := s.Repo.Save(c.Request.Context(), entity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, entity)
}

// DeleteEntity removes an item
func (s *Server) DeleteEntity(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	if err := s.Repo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// ForceSearch triggers a search for a specific entity
func (s *Server) ForceSearch(c *gin.Context) {
	id := c.Param("uuid")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid parameter is required"})
		return
	}

	entity, err := s.Repo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entity not found"})
		return
	}

	if err := s.SearchManager.PerformSearch(c.Request.Context(), entity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "search triggered"})
}

// ForceImport triggers a manual import of a file
func (s *Server) ForceImport(c *gin.Context) {
	// This is a bit tricky as ImportFile needs a file path and an entity.
	// The request should probably provide both.
	// For now, let's assume the request body contains the file path and entity UUID.
	type ImportRequest struct {
		FilePath   string `json:"file_path"`
		EntityUUID string `json:"entity_uuid"`
	}

	var req ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entity, err := s.Repo.Get(c.Request.Context(), req.EntityUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entity not found"})
		return
	}

	if err := s.ImportManager.ImportFile(c.Request.Context(), req.FilePath, "Manual Import", entity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "import triggered"})
}

// buildHierarchyPath builds a hierarchical path from entity to root
func (s *Server) buildHierarchyPath(ctx context.Context, entityID string) ([]map[string]interface{}, error) {
	path := []map[string]interface{}{}
	currentID := entityID

	for {
		entity, err := s.Repo.Get(ctx, currentID)
		if err != nil {
			return nil, err
		}

		path = append([]map[string]interface{}{
			{
				"id":          entity.UUID.String(),
				"type":        entity.EntityType,
				"title":       entity.GetTitle(),
				"parent_uuid": entity.ParentUUID,
			},
		}, path...)

		if entity.ParentUUID == nil {
			break
		}
		currentID = entity.ParentUUID.String()
	}

	return path, nil
}

// buildDescendantsTree builds a complete tree of all descendants
func (s *Server) buildDescendantsTree(ctx context.Context, entityID string) (map[string]interface{}, error) {
	entity, err := s.Repo.Get(ctx, entityID)
	if err != nil {
		return nil, err
	}

	tree := map[string]interface{}{
		"id":       entity.UUID.String(),
		"type":     entity.EntityType,
		"title":    entity.GetTitle(),
		"children": []map[string]interface{}{},
	}

	// Recursively fetch children
	childrenCriteria := map[string]interface{}{
		"parent_uuid": entityID,
	}

	children, err := s.Repo.Find(ctx, childrenCriteria)
	if err != nil {
		return tree, nil // Return entity with empty children on error
	}

	childrenTrees := []map[string]interface{}{}
	for _, child := range children {
		childTree, err := s.buildDescendantsTree(ctx, child.UUID.String())
		if err != nil {
			continue // Skip children that fail
		}
		childrenTrees = append(childrenTrees, childTree)
	}

	tree["children"] = childrenTrees
	tree["child_count"] = len(childrenTrees)

	return tree, nil
}

// hierarchyToPath converts a hierarchy path to a human-readable path string
func (s *Server) hierarchyToPath(hierarchy []map[string]interface{}) string {
	var parts []string
	for _, level := range hierarchy {
		title, ok := level["title"].(string)
		if !ok || title == "" {
			// Fallback to type if title is not available
			title, _ = level["type"].(string)
		}
		if title != "" {
			parts = append(parts, title)
		}
	}
	return strings.Join(parts, " / ")
}
