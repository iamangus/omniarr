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

// GetSystemConfig returns the enhanced system configuration from a backend
func (c *Client) GetSystemConfig(backendID string) (*SystemConfig, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/system/config"

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

	var config SystemConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// GetMediaTypes returns available media types from a backend
func (c *Client) GetMediaTypes(backendID string) ([]MediaTypeInfo, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/system/media-types"

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

	var response struct {
		MediaTypes []MediaTypeInfo `json:"media_types"`
		Count      int             `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.MediaTypes, nil
}

// GetArchitecture returns system architecture information from a backend
func (c *Client) GetArchitecture(backendID string) (*ArchitectureInfo, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/system/architecture"

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

	var architecture ArchitectureInfo
	if err := json.NewDecoder(resp.Body).Decode(&architecture); err != nil {
		return nil, err
	}

	return &architecture, nil
}

// GetProviders returns metadata provider information from a backend
func (c *Client) GetProviders(backendID string) ([]ProviderInfo, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = "/system/providers"

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

	var response struct {
		Providers []ProviderInfo `json:"providers"`
		Count     int            `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Providers, nil
}

// GetEntityHierarchy returns hierarchy information for a specific entity
func (c *Client) GetEntityHierarchy(backendID, entityUUID string) (*EntityHierarchy, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = fmt.Sprintf("/entities/%s/hierarchy", entityUUID)

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

	var hierarchy EntityHierarchy
	if err := json.NewDecoder(resp.Body).Decode(&hierarchy); err != nil {
		return nil, err
	}

	return &hierarchy, nil
}

// GetEntityChildren returns direct children of a specific entity
func (c *Client) GetEntityChildren(backendID, entityUUID string) ([]Entity, error) {
	backend, ok := c.backends[backendID]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}

	u, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}
	u.Path = fmt.Sprintf("/entities/%s/children", entityUUID)

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

	var response struct {
		Children []Entity `json:"children"`
		Count    int      `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Children, nil
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
	Title          string          `json:"title"`
	ID             string          `json:"id"` // External ID (e.g. IMDB/TMDB ID)
	RequestedBy    string          `json:"requested_by,omitempty"`
	ChildOverrides map[string]bool `json:"child_overrides,omitempty"`
}

type Metadata struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type,omitempty"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Year        string                 `json:"year"`
	Authors     []string               `json:"authors"`
	Image       string                 `json:"image"`
	PageCount   int                    `json:"page_count"`
	Identifiers map[string]string      `json:"identifiers"`
	Children    []Metadata             `json:"children,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// SystemConfig represents the enhanced system configuration from omniarr
type SystemConfig struct {
	Version      string           `json:"version"`
	RootEntity   string           `json:"root_entity"`
	MediaTypes   []MediaTypeInfo  `json:"media_types"`
	Architecture ArchitectureInfo `json:"architecture"`
	Providers    []ProviderInfo   `json:"providers"`
	Capabilities CapabilitiesInfo `json:"capabilities"`
}

// MediaTypeInfo represents information about a media type
type MediaTypeInfo struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"display_name"`
	Description    string   `json:"description"`
	Icon           string   `json:"icon"`
	IsLeaf         bool     `json:"is_leaf"`
	HasFiles       bool     `json:"has_files"`
	ParentTypes    []string `json:"parent_types"`
	ChildTypes     []string `json:"child_types"`
	Variants       []string `json:"variants"`
	QualityProfile string   `json:"default_quality_profile"`
}

// ArchitectureInfo represents system architecture details
type ArchitectureInfo struct {
	Version  string       `json:"version"`
	Database DatabaseInfo `json:"database"`
	Storage  StorageInfo  `json:"storage"`
	Features []string     `json:"features"`
}

// DatabaseInfo represents database configuration
type DatabaseInfo struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	MaxEntities int    `json:"max_entities"`
}

// StorageInfo represents storage configuration
type StorageInfo struct {
	Type      string `json:"type"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
}

// ProviderInfo represents metadata provider information
type ProviderInfo struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Available      bool     `json:"available"`
	SupportedTypes []string `json:"supported_types"`
}

// CapabilitiesInfo represents system capabilities
type CapabilitiesInfo struct {
	Streaming        bool `json:"streaming"`
	Transcoding      bool `json:"transcoding"`
	MetadataFetching bool `json:"metadata_fetching"`
	Search           bool `json:"search"`
	Download         bool `json:"download"`
	Import           bool `json:"import"`
}

// EntityHierarchy represents hierarchy information for an entity
type EntityHierarchy struct {
	EntityID  string                   `json:"entity_id"`
	Hierarchy []map[string]interface{} `json:"hierarchy"`
	Path      string                   `json:"path"`
}

type Entity struct {
	UUID               string          `json:"uuid"`
	ParentUUID         *string         `json:"parent_uuid,omitempty"`
	EntityType         string          `json:"entity_type"`
	Status             string          `json:"status"`
	Monitored          bool            `json:"monitored"`
	LastRefreshedAt    *time.Time      `json:"last_refreshed_at"`
	QualityProfileID   *int            `json:"quality_profile_id"`
	LocalPath          string          `json:"local_path"`
	ImagePath          *string         `json:"image_path"`
	Metadata           json.RawMessage `json:"metadata"`
	MonitorNewChildren bool            `json:"monitor_new_children"`
	RequestedBy        *string         `json:"requested_by"`
	RequestedAt        *time.Time      `json:"requested_at"`
}
