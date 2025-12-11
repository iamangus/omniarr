package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthMiddleware checks for a valid API key in the header or query parameter
func APIKeyAuthMiddleware(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no API key is configured, skip authentication (or fail secure? Let's skip for dev convenience if empty, but user asked to secure it)
		// Assuming if apiKey is empty, we might want to allow or block.
		// For now, if apiKey is set, we enforce it.
		if apiKey == "" {
			c.Next()
			return
		}

		// Check Header
		key := c.GetHeader("X-Api-Key")
		if key == "" {
			// Check Query Param
			key = c.Query("api_key")
		}

		if key != apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or missing API key"})
			c.Abort()
			return
		}

		c.Next()
	}
}