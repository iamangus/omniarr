package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"omniarr/internal/api"
	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/download"
	"omniarr/internal/importing"
	"omniarr/internal/lifecycle"
	"omniarr/internal/metadata"
	downloadProvider "omniarr/internal/providers/download"
	"omniarr/internal/providers/metadata/googlebooks"
	"omniarr/internal/providers/metadata/hardcover"
	"omniarr/internal/providers/metadata/tmdb"
	"omniarr/internal/providers/metadata/tvdb"
	"omniarr/internal/search"
	"strings"
)

// MockDownloadClient satisfies download.DownloadClient
type MockDownloadClient struct{}

func (m *MockDownloadClient) Download(ctx context.Context, nzbUrl string, category string) (string, error) {
	log.Printf("Mock Download: %s (Category: %s)", nzbUrl, category)
	return "mock-id", nil
}

func (m *MockDownloadClient) GetStatus(ctx context.Context, id string) (string, error) {
	return "COMPLETED", nil
}

func (m *MockDownloadClient) GetHistory(ctx context.Context) ([]download.DownloadItem, error) {
	return []download.DownloadItem{}, nil
}

func (m *MockDownloadClient) GetQueue(ctx context.Context) ([]download.DownloadItem, error) {
	return []download.DownloadItem{}, nil
}

func main() {
	// 1. Load Configuration
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "config/config.yaml"
	}

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Initialize Repository (Mock or Real)
	var repo *database.EntityRepository

	if os.Getenv("MOCK_MODE") == "true" {
		log.Println("Starting in MOCK MODE (No Database Connection)")
		repo = database.NewMockRepository(&cfg.Catalog)
	} else {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			dsn = "host=localhost user=postgres password=postgres dbname=omniarr sslmode=disable"
		}

		db, err := database.NewPostgresDB(dsn)
		if err != nil {
			log.Printf("Warning: Failed to connect to database: %v", err)
			// Proceeding without DB might cause panics if repo is used,
			// but allows app to start for testing other parts if needed.
		}
		repo = database.NewEntityRepository(db, &cfg.Catalog)
	}

	// 4. Initialize Managers
	// Default image storage path if not set
	imagePath := cfg.Server.ImageStoragePath
	if imagePath == "" {
		imagePath = "./images"
	}
	var metaProvider metadata.MetadataProvider
	switch strings.ToLower(cfg.Catalog.Provider) {
	case "googlebooks":
		metaProvider = googlebooks.New(cfg.Catalog.APIKey)
	case "hardcover":
		metaProvider = hardcover.New(cfg.Catalog.APIKey)
	case "tvdb":
		metaProvider = tvdb.New(cfg.Catalog.APIKey)
	case "tmdb":
		metaProvider = tmdb.New(cfg.Catalog.APIKey)
	default:
		log.Printf("Unknown provider: %s. Defaulting to GoogleBooks.", cfg.Catalog.Provider)
		metaProvider = googlebooks.New(cfg.Catalog.APIKey)
	}

	metadataMgr := metadata.NewManager(metaProvider, imagePath)
	lifecycleMgr := lifecycle.NewManager(repo, metadataMgr, cfg)

	var downloadClient download.DownloadClient
	if cfg.Acquisition.DownloadClient.Type == "sabnzbd" {
		downloadClient = downloadProvider.NewSabnzbdClient(cfg.Acquisition.DownloadClient)
	} else {
		downloadClient = &MockDownloadClient{}
	}

	searchMgr := search.NewSearchManager(repo, downloadClient, &cfg.Acquisition, &cfg.Quality, &cfg.Schema)
	importMgr := importing.NewImportManager(repo, downloadClient, &cfg.Acquisition, &cfg.Quality)

	// Start Lifecycle Reconciliation Loop
	go func() {
		// Initial run after startup
		if err := lifecycleMgr.Reconcile(context.Background()); err != nil {
			log.Printf("Error during initial reconciliation: %v", err)
		}

		// Run periodically (e.g., every 12 hours)
		// Using a shorter interval for demonstration if needed, but 12h is reasonable for metadata
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			if err := lifecycleMgr.Reconcile(context.Background()); err != nil {
				log.Printf("Error during reconciliation: %v", err)
			}
		}
	}()

	// Start Import Scan Loop
	go func() {
		// Run periodically (e.g., every 1 minute)
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if err := importMgr.ScanDownloadFolder(context.Background()); err != nil {
				log.Printf("Error scanning download folder: %v", err)
			}
		}
	}()

	// 5. Initialize API Server and Router
	server := api.NewServer(
		&cfg.Schema,
		&cfg.Quality,
		&cfg.Catalog,
		metadataMgr,
		lifecycleMgr,
		searchMgr,
		importMgr,
		repo,
		imagePath,
	)

	router := api.NewRouter(server, cfg.Server.APIKey)

	// 6. Start HTTP Server
	port := os.Getenv("PORT")
	if port == "" {
		if cfg.Server.Port != 0 {
			port = fmt.Sprintf("%d", cfg.Server.Port)
		} else {
			port = "8080"
		}
	}
	log.Printf("Starting server on port %s...", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
