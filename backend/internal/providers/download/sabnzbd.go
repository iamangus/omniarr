package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"omniarr/internal/config"
	"time"
)

type SabnzbdClient struct {
	config config.DownloadClient
	client *http.Client
}

func NewSabnzbdClient(cfg config.DownloadClient) *SabnzbdClient {
	return &SabnzbdClient{
		config: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type sabnzbdResponse struct {
	Status bool     `json:"status"`
	NzoIds []string `json:"nzo_ids"`
	Error  string   `json:"error"`
}

func (c *SabnzbdClient) Download(ctx context.Context, nzbUrl string, category string) (string, error) {
	baseURL := c.buildBaseURL()
	
	params := url.Values{}
	params.Add("mode", "addurl")
	params.Add("name", nzbUrl)
	params.Add("apikey", c.config.APIKey)
	params.Add("output", "json")
	
	if category != "" {
		params.Add("cat", category)
	} else if c.config.Category != "" {
		params.Add("cat", c.config.Category)
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to sabnzbd: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sabnzbd returned non-200 status: %d", resp.StatusCode)
	}

	var result sabnzbdResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode sabnzbd response: %w", err)
	}

	if !result.Status {
		return "", fmt.Errorf("sabnzbd error: %s", result.Error)
	}

	if len(result.NzoIds) == 0 {
		return "", fmt.Errorf("no nzo_id returned from sabnzbd")
	}

	return result.NzoIds[0], nil
}

func (c *SabnzbdClient) GetStatus(ctx context.Context, id string) (string, error) {
	// TODO: Implement queue check to find status of specific item
	// For now, just return "unknown" as this requires parsing the full queue
	return "unknown", nil
}

func (c *SabnzbdClient) buildBaseURL() string {
	protocol := "http"
	if c.config.UseSSL {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s:%d/sabnzbd/api", protocol, c.config.Host, c.config.Port)
}