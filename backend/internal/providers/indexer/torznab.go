package indexer

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"omniarr/internal/config"
	"omniarr/internal/domain"
	"strconv"
	"strings"
	"time"
)

// TorznabClient handles communication with Torznab-compatible indexers (like Prowlarr).
type TorznabClient struct {
	config config.IndexerConfig
	client *http.Client
}

// NewTorznabClient creates a new TorznabClient.
func NewTorznabClient(cfg config.IndexerConfig) *TorznabClient {
	return &TorznabClient{
		config: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Search performs a search on the indexer.
func (c *TorznabClient) Search(query string) ([]domain.Release, error) {
	// Construct URL
	u, err := url.Parse(c.config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid indexer URL: %w", err)
	}

	q := u.Query()
	q.Set("apikey", c.config.APIKey)
	q.Set("t", "search")
	q.Set("q", query)
	
	// Add categories if configured
	if len(c.config.Categories) > 0 {
		cats := make([]string, len(c.config.Categories))
		for i, cat := range c.config.Categories {
			cats[i] = strconv.Itoa(cat)
		}
		q.Set("cat", strings.Join(cats, ","))
	}

	u.RawQuery = q.Encode()

	// Make Request
	resp, err := c.client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query indexer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("indexer returned status: %s", resp.Status)
	}

	// Parse Response
	return c.parseResponse(resp.Body)
}

// XML Structures for Torznab response
type rss struct {
	Channel channel `xml:"channel"`
}

type channel struct {
	Items []item `xml:"item"`
}

type item struct {
	Title       string       `xml:"title"`
	Link        string       `xml:"link"`
	Description string       `xml:"description"`
	PubDate     string       `xml:"pubDate"`
	Size        int64        `xml:"size"` // Standard RSS size
	Enclosure   enclosure    `xml:"enclosure"`
	Attrs       []torznabAttr `xml:"attr"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type torznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func (c *TorznabClient) parseResponse(body io.Reader) ([]domain.Release, error) {
	var feed rss
	if err := xml.NewDecoder(body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	var releases []domain.Release
	for _, item := range feed.Channel.Items {
		release := domain.Release{
			Title:    item.Title,
			Link:     item.Link,
			Indexer:  c.config.Name,
			Protocol: "usenet", // Default, logic below might change it
		}

		// Parse Date
		if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
			release.PublishDate = t
		} else if t, err := time.Parse(time.RFC1123, item.PubDate); err == nil {
			release.PublishDate = t
		}

		// Size can be in enclosure or standard RSS field
		if item.Enclosure.Length > 0 {
			release.Size = item.Enclosure.Length
		} else {
			release.Size = item.Size
		}

		// Parse attributes for extra info
		for _, attr := range item.Attrs {
			switch attr.Name {
			case "grabs":
				if v, err := strconv.Atoi(attr.Value); err == nil {
					release.Grabs = v
				}
			case "seeders":
				if v, err := strconv.Atoi(attr.Value); err == nil {
					release.Seeders = v
					release.Protocol = "torrent"
				}
			case "peers":
				if v, err := strconv.Atoi(attr.Value); err == nil {
					release.Peers = v
				}
			case "infohash":
				release.InfoHash = attr.Value
				release.Protocol = "torrent"
			}
		}

		releases = append(releases, release)
	}

	return releases, nil
}