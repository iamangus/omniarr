package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/angoo/omniarr/frontend/internal/config"
)

type Client struct {
	httpClient *http.Client
	backends   map[string]config.BackendConfig
}

func NewClient(backends []config.BackendConfig) *Client {
	backendMap := make(map[string]config.BackendConfig)
	for _, b := range backends {
		backendMap[b.ID] = b
	}

	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		backends:   backendMap,
	}
}

func (c *Client) GetBackend(id string) (config.BackendConfig, bool) {
	b, ok := c.backends[id]
	return b, ok
}

func (c *Client) Search(backendID, query string) ([]SearchResult, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/catalog/lookup"
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", backend.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status: %d", resp.StatusCode)
	}

	var results []SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

func (c *Client) GetCatalogItem(backendID, entityType, id string) (*Metadata, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/catalog/item"
	q := u.Query()
	q.Set("type", entityType)
	q.Set("id", id)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", backend.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status: %d", resp.StatusCode)
	}

	var meta Metadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func (c *Client) Request(backendID string, payload RequestPayload, userEmail string) error {
	backend, ok := c.backends[backendID]
	if !ok {
		return fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return err
	}
	u.Path = "/entities"

	// Add user tracking
	payload.RequestedBy = userEmail

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", backend.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-By", userEmail)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("backend returned status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (c *Client) GetLists(backendID string) ([]List, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/catalog/lists"

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", backend.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status: %d", resp.StatusCode)
	}

	var lists []List
	if err := json.NewDecoder(resp.Body).Decode(&lists); err != nil {
		return nil, err
	}

	return lists, nil
}

func (c *Client) GetEntities(backendID string) ([]Entity, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/entities"

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", backend.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status: %d", resp.StatusCode)
	}

	var entities []Entity
	if err := json.NewDecoder(resp.Body).Decode(&entities); err != nil {
		return nil, err
	}

	return entities, nil
}

func (c *Client) DeleteEntity(backendID, uuid string) error {
	backend, ok := c.backends[backendID]
	if !ok {
		return fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return err
	}
	u.Path = "/entities/" + uuid

	req, err := http.NewRequest("DELETE", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", backend.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned status: %d", resp.StatusCode)
	}

	return nil
}

type List struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Children    []SearchResult `json:"children"`
}

type SearchResult struct {
	ID          string `json:"id"` // This might need to be mapped depending on backend response
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Year        string `json:"year"`
	Image       string `json:"image"` // Matches the key in catalog.yaml
}

type RequestPayload struct {
	Title       string `json:"title"`
	ID          string `json:"id"` // External ID (e.g. IMDB/TMDB ID)
	RequestedBy string `json:"requested_by,omitempty"`
	ChildOverrides map[string]bool `json:"child_overrides,omitempty"`
}

type Metadata struct {
	ID          string            `json:"id"`
	Type        string            `json:"type,omitempty"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Year        string            `json:"year"`
	Authors     []string          `json:"authors"`
	Image       string            `json:"image"`
	PageCount   int               `json:"page_count"`
	Identifiers map[string]string `json:"identifiers"`
	Children    []Metadata        `json:"children,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

type Entity struct {
	UUID            string          `json:"uuid"`
	ParentUUID      *string         `json:"parent_uuid,omitempty"`
	EntityType      string          `json:"entity_type"`
	Status          string          `json:"status"`
	Monitored       bool            `json:"monitored"`
	LastRefreshedAt *time.Time      `json:"last_refreshed_at"`
	QualityProfileID *int           `json:"quality_profile_id"`
	LocalPath       string          `json:"local_path"`
	ImagePath       *string         `json:"image_path"`
	Metadata        json.RawMessage `json:"metadata"`
	MonitorNewChildren bool         `json:"monitor_new_children"`
	RequestedBy     *string         `json:"requested_by"`
	RequestedAt     *time.Time      `json:"requested_at"`
}