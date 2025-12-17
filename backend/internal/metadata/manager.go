package metadata

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager implements the MetadataProvider interface by delegating to a specific provider
type Manager struct {
	provider         MetadataProvider
	imageStoragePath string
	client           *http.Client
}

// NewManager creates a new instance of the Metadata Manager
func NewManager(provider MetadataProvider, imageStoragePath string) *Manager {
	return &Manager{
		provider:         provider,
		imageStoragePath: imageStoragePath,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetMetadata fetches metadata for a specific entity type and ID
func (m *Manager) GetMetadata(ctx context.Context, entityType string, id string) (*Metadata, error) {
	return m.GetMetadataWithEntityID(ctx, entityType, id, "")
}

// GetMetadataWithEntityID fetches metadata for a specific entity type and ID, with optional entity UUID for image naming
func (m *Manager) GetMetadataWithEntityID(ctx context.Context, entityType string, id string, entityUUID string) (*Metadata, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("no metadata provider configured")
	}

	meta, err := m.provider.GetMetadata(ctx, entityType, id)
	if err != nil {
		return nil, err
	}

	// Handle Poster Image Download
	if meta.Image != "" && m.imageStoragePath != "" {
		log.Printf("Found poster URL for %s %s: %s", entityType, id, meta.Image)
		localPath, err := m.downloadImage(ctx, meta.Image, entityUUID, "poster")
		if err != nil {
			log.Printf("Failed to download poster for %s %s: %v", entityType, id, err)
		} else {
			log.Printf("Successfully downloaded poster to %s", localPath)
			// We store the local path in the Extra map so it can be used by the lifecycle manager
			if meta.Extra == nil {
				meta.Extra = make(map[string]interface{})
			}
			meta.Extra["_local_image_path"] = localPath
		}
	}

	// Handle Backdrop Image Download
	if meta.Extra != nil {
		if backdropURL, ok := meta.Extra["backdrop"].(string); ok && backdropURL != "" && m.imageStoragePath != "" {
			log.Printf("Found backdrop URL for %s %s: %s", entityType, id, backdropURL)
			localPath, err := m.downloadImage(ctx, backdropURL, entityUUID, "backdrop")
			if err != nil {
				log.Printf("Failed to download backdrop for %s %s: %v", entityType, id, err)
			} else {
				log.Printf("Successfully downloaded backdrop to %s", localPath)
				meta.Extra["_local_backdrop_path"] = localPath
			}
		}
	}

	return meta, nil
}

// Search queries the provider for entities matching the query string
func (m *Manager) Search(ctx context.Context, query string) ([]Metadata, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("no metadata provider configured")
	}
	return m.provider.Search(ctx, query)
}

// GetLists fetches curated lists of content
func (m *Manager) GetLists(ctx context.Context, listIDs []string) ([]Metadata, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("no metadata provider configured")
	}
	return m.provider.GetLists(ctx, listIDs)
}

func (m *Manager) downloadImage(ctx context.Context, imageURL string, entityUUID string, imageType string) (string, error) {
	log.Printf("Downloading %s image from %s", imageType, imageURL)
	// Create directory if not exists
	if err := os.MkdirAll(m.imageStoragePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create image directory: %w", err)
	}

	// Determine filename based on entity UUID and image type
	ext := filepath.Ext(imageURL)
	// Handle query params in extension (e.g. image.jpg?v=1)
	if idx := strings.Index(ext, "?"); idx != -1 {
		ext = ext[:idx]
	}
	if ext == "" {
		ext = ".jpg" // Default to jpg
	}

	// Use UUID-based naming for deterministic URLs
	// Format: {uuid}_poster.jpg or {uuid}_backdrop.jpg
	var filename string
	if entityUUID != "" {
		filename = fmt.Sprintf("%s_%s%s", entityUUID, imageType, ext)
	} else {
		// Fallback for legacy calls without UUID
		filename = fmt.Sprintf("unknown_%s_%d%s", imageType, time.Now().Unix(), ext)
	}

	localPath := filepath.Join(m.imageStoragePath, filename)

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return filename, nil
}
