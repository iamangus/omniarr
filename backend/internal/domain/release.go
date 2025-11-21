package domain

import "time"

// Release represents a downloadable item found on an indexer.
type Release struct {
	Title       string    `json:"title"`
	Link        string    `json:"link"` // URL to the NZB or Torrent file
	Indexer     string    `json:"indexer"`
	Size        int64     `json:"size"`
	PublishDate time.Time `json:"publish_date"`
	Protocol    string    `json:"protocol"` // "usenet" or "torrent"
	InfoHash    string    `json:"info_hash,omitempty"`
	Grabs       int       `json:"grabs"`
	Seeders     int       `json:"seeders,omitempty"`
	Peers       int       `json:"peers,omitempty"`
}