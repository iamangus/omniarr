package api

import (
	"github.com/gin-gonic/gin"
)

func NewRouter(server *Server, apiKey string) *gin.Engine {
	r := gin.Default()

	// Apply API Key Middleware
	r.Use(APIKeyAuthMiddleware(apiKey))

	// System
	r.GET("/system/config", server.GetConfig)

	// Catalog
	r.GET("/catalog/lookup", server.LookupCatalog)
	r.GET("/catalog/lists", server.GetLists)
	r.GET("/catalog/item", server.GetCatalogItem)

	// Entities
	r.GET("/entities", server.GetEntities)
	r.POST("/entities", server.CreateEntity)
	r.PUT("/entities/:uuid", server.UpdateEntity)
	r.DELETE("/entities/:uuid", server.DeleteEntity)

	// Actions
	r.POST("/acquisition/search/:uuid", server.ForceSearch)
	r.POST("/queue/import", server.ForceImport)

	// Images
	if server.ImageStoragePath != "" {
		r.Static("/images", server.ImageStoragePath)
	}

	return r
}