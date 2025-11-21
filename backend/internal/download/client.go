package download

import "context"

// DownloadClient defines the interface for interacting with download clients (e.g., NZBGet, SabNZBD).
type DownloadClient interface {
	// Download adds a generic download (NZB/Torrent) to the client.
	// Returns the download ID and an error if any.
	Download(ctx context.Context, nzbUrl string, category string) (string, error)

	// GetStatus returns the status of a specific download by ID.
	GetStatus(ctx context.Context, id string) (string, error)
}