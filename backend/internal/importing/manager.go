package importing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/domain"
	"omniarr/internal/download"
	"omniarr/internal/utils"
)

// ImportManager handles the movement of files from download to library.
type ImportManager struct {
	repo           *database.EntityRepository
	downloadClient download.DownloadClient
	config         *config.AcquisitionConfig
	qualityConfig  *config.QualityConfig
}

// NewImportManager creates a new instance of ImportManager.
func NewImportManager(repo *database.EntityRepository, downloadClient download.DownloadClient, cfg *config.AcquisitionConfig, qualityCfg *config.QualityConfig) *ImportManager {
	return &ImportManager{
		repo:           repo,
		downloadClient: downloadClient,
		config:         cfg,
		qualityConfig:  qualityCfg,
	}
}

// ScanDownloadFolder checks for completed downloads and triggers import.
func (m *ImportManager) ScanDownloadFolder(ctx context.Context) error {
	log.Println("Scanning download folder...")

	// 1. Check Queue and Log Progress
	queueItems, err := m.downloadClient.GetQueue(ctx)
	if err != nil {
		log.Printf("Failed to get download queue: %v", err)
	} else {
		for _, item := range queueItems {
			log.Printf("[Download Queue] %s: %s (%s%%)", item.Name, item.Status, item.Progress)
		}
	}

	// 2. Get completed items from Download Client
	items, err := m.downloadClient.GetHistory(ctx)
	if err != nil {
		return fmt.Errorf("failed to get download history: %w", err)
	}

	for _, item := range items {
		// Only process completed items
		if item.Status != "Completed" {
			continue
		}

		// 3. Identify the Entity
		entity, err := m.repo.FindByDownloadClientID(ctx, item.ID)
		if err != nil {
			// Entity not found for this download ID.
			// Could attempt name parsing here as fallback, but for now skip.
			log.Printf("No entity found for download ID %s (%s)", item.ID, item.Name)
			continue
		}

		// 4. Check if already imported
		if entity.Status == domain.StatusDownloaded {
			// Already imported, skip.
			// Unless we want to force re-import? For now, skip.
			continue
		}

		// 5. Import
		log.Printf("Found completed download for entity %s: %s", entity.UUID, item.Name)
		if err := m.ImportFile(ctx, item.Path, entity); err != nil {
			log.Printf("Failed to import file %s: %v", item.Path, err)
		}
	}

	return nil
}

// ImportFile moves, renames, and updates the entity status.
func (m *ImportManager) ImportFile(ctx context.Context, sourcePath string, entity *domain.Entity) error {
	// 1. Build Data Map for Template
	data, err := m.buildDataMap(ctx, entity)
	if err != nil {
		return err
	}

	// Add file specific data
	ext := filepath.Ext(sourcePath)
	if len(ext) > 0 {
		ext = ext[1:] // remove dot
	}
	data["Ext"] = ext

	// Try to parse quality from filename or entity profile?
	// Entity has QualityProfileID, but that's what we *wanted*.
	// We should guess quality from source filename.
	quality := m.parseQuality(filepath.Base(sourcePath))
	data["Quality"] = quality

	// 2. Resolve Destination Paths
	folderName, err := utils.ResolveTemplate(m.config.Naming.Folder, data)
	if err != nil {
		return fmt.Errorf("failed to resolve folder template: %w", err)
	}

	fileName, err := utils.ResolveTemplate(m.config.Naming.File, data)
	if err != nil {
		return fmt.Errorf("failed to resolve file template: %w", err)
	}

	// Construct full path
	// We assume config.Naming.Folder defines the structure *relative* to some media root,
	// or it is absolute.
	// If it starts with /, it's absolute.
	// For this prototype, we'll assume it's absolute or we treat it as is.
	destDir := folderName
	destPath := filepath.Join(destDir, fileName)

	// 3. Move File
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destDir, err)
	}

	// Move (Rename)
	log.Printf("Moving %s to %s", sourcePath, destPath)
	if err := os.Rename(sourcePath, destPath); err != nil {
		// Fallback to Copy+Delete if Rename fails (cross-device)
		// For prototype, just error out or try copy?
		// Let's assume same volume for now.
		return fmt.Errorf("failed to move file: %w", err)
	}

	// 4. Update Entity in DB
	entity.Status = domain.StatusDownloaded
	entity.LocalPath = destPath

	if err := m.repo.Save(ctx, entity); err != nil {
		return fmt.Errorf("failed to update entity status: %w", err)
	}

	log.Printf("Successfully imported entity %s", entity.UUID)
	return nil
}

func (m *ImportManager) buildDataMap(ctx context.Context, entity *domain.Entity) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	current := entity
	for {
		var meta map[string]interface{}
		if len(current.Metadata) > 0 {
			if err := json.Unmarshal(current.Metadata, &meta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata for %s: %w", current.UUID, err)
			}
			key := strings.Title(current.EntityType)

			// Ensure keys are Title Case for template matching (e.g. {Book.Title} matches "title" in json)
			for k, v := range meta {
				if len(k) > 0 {
					// Simple capitalization of first letter
					newKey := strings.ToUpper(k[:1]) + k[1:]
					if _, exists := meta[newKey]; !exists {
						meta[newKey] = v
					}
				}
			}

			data[key] = meta
		}

		if current.ParentUUID == nil {
			break
		}
		parent, err := m.repo.Get(ctx, current.ParentUUID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to get parent %s: %w", current.ParentUUID, err)
		}
		current = parent
	}
	return data, nil
}

func (m *ImportManager) parseQuality(filename string) string {
	if m.qualityConfig == nil {
		return "Unknown"
	}
	for _, def := range m.qualityConfig.Definitions {
		matched, _ := regexp.MatchString("(?i)"+def.Regex, filename)
		if matched {
			return def.Name
		}
	}
	return "Unknown"
}
