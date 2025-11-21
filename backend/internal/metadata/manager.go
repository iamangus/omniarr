package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"omniarr/internal/config"
	"omniarr/internal/utils"
)

// Manager implements the MetadataProvider interface
type Manager struct {
	config *config.CatalogConfig
	client *http.Client
}

// NewManager creates a new instance of the Metadata Manager
func NewManager(cfg *config.CatalogConfig) *Manager {
	return &Manager{
		config: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetMetadata fetches metadata for a specific entity type and ID
func (m *Manager) GetMetadata(ctx context.Context, entityType string, id string) (map[string]interface{}, error) {
	endpoint, err := m.getEndpoint(entityType)
	if err != nil {
		return nil, err
	}

	// Construct URL
	reqURL := m.config.BaseURL + strings.Replace(endpoint.URL, "{id}", id, 1)

	// Make request
	data, err := m.makeRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	// Map attributes
	result := make(map[string]interface{})
	result["id"] = id
	result["entity_type"] = entityType

	for key, path := range endpoint.Attributes {
		val := utils.ExtractValue(data, path)
		if val != nil {
			result[key] = val
		}
	}

	// Extract identifiers
	identifiers := make(map[string]string)
	for _, idMap := range endpoint.Identifiers {
		val := utils.ExtractValue(data, idMap.Source)
		if val != nil {
			identifiers[idMap.Key] = fmt.Sprintf("%v", val)
		}
	}
	if len(identifiers) > 0 {
		result["identifiers"] = identifiers
	}

	// Extract children
	for _, child := range endpoint.Children {
		val := utils.ExtractValue(data, child.Source)
		if val != nil {
			// Use the path as the key (stripping $.)
			// This assumes simple paths for now.
			key := strings.TrimPrefix(child.Source, "$.")
			result[key] = val
		}
	}

	return result, nil
}

// Search queries the provider for entities matching the query string
func (m *Manager) Search(ctx context.Context, query string) ([]map[string]interface{}, error) {
	// Assuming "search" is the entity type for search in config
	// We might need a better way to identify the search endpoint if there are multiple
	endpoint, err := m.getEndpoint("search")
	if err != nil {
		return nil, err
	}

	reqURL := m.config.BaseURL + endpoint.URL

	// Add query params
	u, err := url.Parse(reqURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	queryParam := endpoint.QueryParam
	if queryParam == "" {
		queryParam = "query"
	}
	q.Set(queryParam, query)
	u.RawQuery = q.Encode()

	data, err := m.makeRequest(ctx, u.String())
	if err != nil {
		return nil, err
	}

	// Search results are typically in a "results" array
	// This might need to be configurable if other providers use different keys
	resultsKey := endpoint.ResultsKey
	if resultsKey == "" {
		resultsKey = "results"
	}

	resultsRaw, ok := data[resultsKey].([]interface{})
	if !ok {
		// Try to see if the root is the array (if resultsKey is special, e.g. ".")
		// For now, just fallback logic or error
		return nil, fmt.Errorf("invalid search response format: '%s' key not found", resultsKey)
	}

	var results []map[string]interface{}
	for _, itemRaw := range resultsRaw {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}

		mappedItem := make(map[string]interface{})
		for key, path := range endpoint.Attributes {
			val := utils.ExtractValue(item, path)
			if val != nil {
				mappedItem[key] = val
			}
		}
		results = append(results, mappedItem)
	}

	return results, nil
}

func (m *Manager) getEndpoint(entityType string) (*config.EndpointMapping, error) {
	for _, ep := range m.config.Endpoints {
		if ep.EntityType == entityType {
			return &ep, nil
		}
	}
	return nil, fmt.Errorf("no endpoint configuration found for entity type: %s", entityType)
}

func (m *Manager) makeRequest(ctx context.Context, reqURL string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	// Add API Key
	// TODO: Make auth method configurable (header vs query param)
	// Defaulting to query param 'api_key' for now as per current usage
	q := req.URL.Query()
	if m.config.APIKey != "" {
		q.Add("api_key", m.config.APIKey)
	}
	req.URL.RawQuery = q.Encode()

	log.Printf("Making request to: %s", req.URL.String())

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return data, nil
}
