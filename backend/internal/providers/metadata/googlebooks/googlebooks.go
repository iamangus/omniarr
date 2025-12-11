package googlebooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"omniarr/internal/metadata"
)

const (
	BaseURL = "https://www.googleapis.com/books/v1"
)

type Provider struct {
	apiKey string
	client *http.Client
}

func New(apiKey string) *Provider {
	return &Provider{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Google Books API Response Structures
type Volume struct {
	ID         string     `json:"id"`
	VolumeInfo VolumeInfo `json:"volumeInfo"`
}

type VolumeInfo struct {
	Title               string              `json:"title"`
	Authors             []string            `json:"authors"`
	PublishedDate       string              `json:"publishedDate"`
	Description         string              `json:"description"`
	PageCount           int                 `json:"pageCount"`
	ImageLinks          ImageLinks          `json:"imageLinks"`
	IndustryIdentifiers []IndustryIdentifier `json:"industryIdentifiers"`
}

type ImageLinks struct {
	Thumbnail string `json:"thumbnail"`
	Small     string `json:"small"`
}

type IndustryIdentifier struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

type SearchResponse struct {
	Items []Volume `json:"items"`
}

func (p *Provider) GetMetadata(ctx context.Context, entityType string, id string) (*metadata.Metadata, error) {
	// Google Books only supports books
	if entityType != "book" {
		return nil, fmt.Errorf("google books provider does not support entity type: %s", entityType)
	}

	reqURL := fmt.Sprintf("%s/volumes/%s?key=%s", BaseURL, id, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books api returned status: %d", resp.StatusCode)
	}

	var vol Volume
	if err := json.NewDecoder(resp.Body).Decode(&vol); err != nil {
		return nil, err
	}

	return p.mapVolumeToMetadata(vol), nil
}

func (p *Provider) Search(ctx context.Context, query string) ([]metadata.Metadata, error) {
	reqURL := fmt.Sprintf("%s/volumes?q=%s&key=%s", BaseURL, url.QueryEscape(query), p.apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books api returned status: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	var results []metadata.Metadata
	for _, item := range searchResp.Items {
		results = append(results, *p.mapVolumeToMetadata(item))
	}

	return results, nil
}

// GetLists is not supported by Google Books provider
func (p *Provider) GetLists(ctx context.Context, listIDs []string) ([]metadata.Metadata, error) {
	return []metadata.Metadata{}, nil
}

func (p *Provider) mapVolumeToMetadata(vol Volume) *metadata.Metadata {
	year := ""
	if len(vol.VolumeInfo.PublishedDate) >= 4 {
		year = vol.VolumeInfo.PublishedDate[:4]
	}

	identifiers := make(map[string]string)
	for _, id := range vol.VolumeInfo.IndustryIdentifiers {
		identifiers[id.Type] = id.Identifier
	}

	return &metadata.Metadata{
		ID:          vol.ID,
		Title:       vol.VolumeInfo.Title,
		Description: vol.VolumeInfo.Description,
		Year:        year,
		Authors:     vol.VolumeInfo.Authors,
		Image:       vol.VolumeInfo.ImageLinks.Thumbnail,
		PageCount:   vol.VolumeInfo.PageCount,
		Identifiers: identifiers,
	}
}