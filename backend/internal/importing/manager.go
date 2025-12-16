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
		if err := m.ImportFile(ctx, item.Path, item.Name, entity); err != nil {
			log.Printf("Failed to import file %s: %v", item.Path, err)
		}
	}

	return nil
}

// ImportFile moves, renames, and updates the entity status.
func (m *ImportManager) ImportFile(ctx context.Context, sourcePath string, releaseName string, entity *domain.Entity) error {
	// 0. Find Actual File (if directory)
	actualFilePath, err := m.findMainFile(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to find main file in %s: %w", sourcePath, err)
	}

	// 1. Build Data Map for Template
	data, err := m.buildDataMap(ctx, entity)
	if err != nil {
		return err
	}

	// Add file specific data
	ext := filepath.Ext(actualFilePath)
	if len(ext) > 0 {
		ext = ext[1:] // remove dot
	}
	data["Ext"] = ext

	// Hybrid Quality Detection
	// 1. Probe File for Resolution
	mediaInfo, probeErr := utils.ProbeFile(actualFilePath)
	if probeErr != nil {
		log.Printf("Warning: Failed to probe file %s: %v", actualFilePath, probeErr)
	}

	// 2. Parse Release Name for Source/Quality
	// We use the release name (folder name) primarily, as it has more info than the file inside.
	quality := m.detectQuality(releaseName, mediaInfo)
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
	destDir := folderName
	destPath := filepath.Join(destDir, fileName)

	// 3. Move File
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destDir, err)
	}

	// Move (Rename)
	log.Printf("Moving %s to %s", actualFilePath, destPath)
	if err := os.Rename(actualFilePath, destPath); err != nil {
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

func (m *ImportManager) findMainFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		return path, nil
	}

	// It's a directory, find largest video file
	var largestFile string
	var largestSize int64

	videoExtensions := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".wmv": true, ".iso": true,
	}

	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(filePath))
		if videoExtensions[ext] {
			if info.Size() > largestSize {
				largestSize = info.Size()
				largestFile = filePath
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if largestFile == "" {
		return "", fmt.Errorf("no video files found in %s", path)
	}

	return largestFile, nil
}

func (m *ImportManager) detectQuality(releaseName string, mediaInfo *utils.MediaInfo) string {
	if m.qualityConfig == nil {
		return "Unknown"
	}

	// Strategy:
	// 1. Determine Source from Release Name (Bluray, Web, HDTV)
	// 2. Determine Resolution from MediaInfo (if available) or Release Name
	// 3. Match against definitions

	// Simple implementation for now:
	// If we have resolution, try to match resolution + source text in definitions.
	// We iterate through definitions and see which one regex matches the BEST.

	// Synthesize a string to match against?
	// Or just use the releaseName, but inject the resolution if missing?

	// Actually, the QualityConfig.Definitions relies on Regex.
	// So we need to match the Regex against the Release Name.
	// BUT, if we have MediaInfo, we can force a resolution match.

	resolution := ""
	if mediaInfo != nil {
		if mediaInfo.Height >= 2100 {
			resolution = "2160p"
		} else if mediaInfo.Height >= 1000 {
			resolution = "1080p"
		} else if mediaInfo.Height >= 700 {
			resolution = "720p"
		} else {
			resolution = "SD"
		}
	}

	// Iterate definitions
	for _, def := range m.qualityConfig.Definitions {
		// Check regex against Release Name
		matchedName, _ := regexp.MatchString("(?i)"+def.Regex, releaseName)

		// If we have probe data, check if definition name contains our resolution
		// This is a heuristic. Ideally, definitions would have "Resolution" fields.
		// For now, if we found 2160p via probe, and the definition name contains "2160p", we favor it.

		if matchedName {
			// If we have probe resolution, ensure the definition matches it
			if resolution != "" {
				if strings.Contains(strings.ToLower(def.Name), resolution) || strings.Contains(strings.ToLower(def.Regex), resolution) {
					return def.Name
				}
			} else {
				// No probe data, just trust name
				return def.Name
			}
		}
	}

	// Fallback: If we have resolution but no regex matched
	if resolution != "" {
		// Try to find a definition that matches JUST the resolution
		for _, def := range m.qualityConfig.Definitions {
			if strings.Contains(strings.ToLower(def.Name), resolution) {
				return def.Name
			}
		}
		return resolution // Return "2160p" directly if no profile matches
	}

	return "Unknown"
}
