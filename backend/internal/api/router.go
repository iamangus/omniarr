package api

import (
	"github.com/gin-gonic/gin"
)

func NewRouter(server *Server) *gin.Engine {
	r := gin.Default()

	// System
	r.GET("/system/config", server.GetConfig)

	// Catalog
	r.GET("/catalog/lookup", server.LookupCatalog)

	// Entities
	r.POST("/entities", server.CreateEntity)
	r.DELETE("/entities/:uuid", server.DeleteEntity)

	// Actions
	r.POST("/acquisition/search/:uuid", server.ForceSearch)
	r.POST("/queue/import", server.ForceImport)

	return r
}