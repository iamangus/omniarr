package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// GetConfig returns the system configuration
func (s *Server) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"root_entity":      s.SchemaConfig.RootEntity,
		"quality_profiles": s.QualityConfig.Profiles,
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

	c.JSON(http.StatusOK, entity)
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
