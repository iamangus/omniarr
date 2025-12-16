package download

import "context"

// DownloadItem represents a download in the queue or history
type DownloadItem struct {
	ID       string
	Name     string
	Status   string // "Queued", "Downloading", "Completed", "Failed"
	Path     string // File path on disk (for completed items)
	Category string
	Progress string // Percentage complete (e.g., "45")
}

// DownloadClient defines the interface for interacting with download clients (e.g., NZBGet, SabNZBD).
type DownloadClient interface {
	// Download adds a generic download (NZB/Torrent) to the client.
	// Returns the download ID and an error if any.
	Download(ctx context.Context, nzbUrl string, category string) (string, error)

	// GetStatus returns the status of a specific download by ID.
	GetStatus(ctx context.Context, id string) (string, error)

	// GetHistory returns the list of completed downloads
	GetHistory(ctx context.Context) ([]DownloadItem, error)

	// GetQueue returns the list of active downloads
	GetQueue(ctx context.Context) ([]DownloadItem, error)
}
