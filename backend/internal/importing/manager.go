package importing

import (
	"context"
	"fmt"
	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/domain"
	"omniarr/internal/download"
)

// ImportManager handles the movement of files from download to library.
type ImportManager struct {
	repo           *database.EntityRepository
	downloadClient download.DownloadClient
	config         *config.AcquisitionConfig
}

// NewImportManager creates a new instance of ImportManager.
func NewImportManager(repo *database.EntityRepository, downloadClient download.DownloadClient, cfg *config.AcquisitionConfig) *ImportManager {
	return &ImportManager{
		repo:           repo,
		downloadClient: downloadClient,
		config:         cfg,
	}
}

// ScanDownloadFolder checks for completed downloads and triggers import.
func (m *ImportManager) ScanDownloadFolder(ctx context.Context) error {
	// 1. Get completed items from Download Client
	// items, err := m.downloadClient.GetStatus(ctx, "") // Assuming empty string gets all or we need a specific method

	// 2. For each completed item:
	//    - Identify the Entity (via stored ID or parsing name)
	//    - Call ImportFile
	fmt.Println("Scanning download folder...")
	return nil
}

// ImportFile moves, renames, and updates the entity status.
func (m *ImportManager) ImportFile(ctx context.Context, filePath string, entity *domain.Entity) error {
	// 1. Determine destination path based on NamingConfig
	// destPath := fmt.Sprintf(m.config.Naming.File, ...)

	// 2. Move/Copy file
	// os.Rename(filePath, destPath)

	// 3. Update Entity in DB
	entity.Status = domain.StatusDownloaded
	entity.LocalPath = filePath // Should be the new path
	// return m.repo.Save(ctx, entity)

	fmt.Printf("Importing file %s for entity %s\n", filePath, entity.UUID)
	return nil
}