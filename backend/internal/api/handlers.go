package api

import (
	"context"
	"fmt"
	"net/http"

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
	MetadataManager  *metadata.Manager
	LifecycleManager *lifecycle.Manager
	SearchManager    *search.SearchManager
	ImportManager    *importing.ImportManager
	Repo             *database.EntityRepository
}

func NewServer(
	schemaCfg *config.SchemaConfig,
	qualityCfg *config.QualityConfig,
	metadataMgr *metadata.Manager,
	lifecycleMgr *lifecycle.Manager,
	searchMgr *search.SearchManager,
	importMgr *importing.ImportManager,
	repo *database.EntityRepository,
) *Server {
	return &Server{
		SchemaConfig:     schemaCfg,
		QualityConfig:    qualityCfg,
		MetadataManager:  metadataMgr,
		LifecycleManager: lifecycleMgr,
		SearchManager:    searchMgr,
		ImportManager:    importMgr,
		Repo:             repo,
	}
}

// GetConfig returns the system configuration
func (s *Server) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"root_entity":      s.SchemaConfig.RootEntity,
		"quality_profiles": s.QualityConfig.Profiles,
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// CreateEntity adds a new item
func (s *Server) CreateEntity(c *gin.Context) {
	var req domain.Entity
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	if req.UUID == uuid.Nil {
		req.UUID = uuid.New()
	}
	req.Status = domain.StatusWanted
	req.Monitored = true

	if err := s.Repo.Save(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Trigger Background Jobs (Refresh + Search)
	go func() {
		ctx := context.Background()
		// 1. Refresh Metadata
		if err := s.LifecycleManager.RefreshEntity(ctx, req.UUID.String()); err != nil {
			fmt.Printf("Failed to refresh metadata for %s: %v\n", req.UUID, err)
			return
		}

		// Reload entity to get updated metadata
		updatedEntity, err := s.Repo.Get(ctx, req.UUID.String())
		if err != nil {
			fmt.Printf("Failed to reload entity %s: %v\n", req.UUID, err)
			return
		}

		// 2. Search
		if err := s.SearchManager.PerformSearch(ctx, updatedEntity); err != nil {
			fmt.Printf("Search failed for %s: %v\n", req.UUID, err)
		}
	}()

	c.JSON(http.StatusCreated, req)
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

	if err := s.ImportManager.ImportFile(c.Request.Context(), req.FilePath, entity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "import triggered"})
}