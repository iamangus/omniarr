package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"omniarr/internal/config"
	"omniarr/internal/download"
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

type sabnzbdHistoryResponse struct {
	History struct {
		Slots []struct {
			NzoID       string `json:"nzo_id"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			Storage     string `json:"storage"` // Path where it was stored
			Category    string `json:"category"`
			FailMessage string `json:"fail_message"`
		} `json:"slots"`
	} `json:"history"`
}

type sabnzbdQueueResponse struct {
	Queue struct {
		Slots []struct {
			NzoID      string `json:"nzo_id"`
			Filename   string `json:"filename"`
			Status     string `json:"status"`
			Cat        string `json:"cat"`
			Percentage string `json:"percentage"`
		} `json:"slots"`
	} `json:"queue"`
}

func (c *SabnzbdClient) GetHistory(ctx context.Context) ([]download.DownloadItem, error) {
	baseURL := c.buildBaseURL()
	params := url.Values{}
	params.Add("mode", "history")
	params.Add("apikey", c.config.APIKey)
	params.Add("output", "json")
	// Limit? Default is usually small, maybe need limit=0 for all? Or limit=50.
	params.Add("limit", "50")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sabnzbd returned non-200 status: %d", resp.StatusCode)
	}

	var result sabnzbdHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sabnzbd history: %w", err)
	}

	var items []download.DownloadItem
	for _, slot := range result.History.Slots {
		status := "Completed"
		if slot.Status == "Failed" || slot.FailMessage != "" {
			status = "Failed"
		}

		items = append(items, download.DownloadItem{
			ID:       slot.NzoID,
			Name:     slot.Name,
			Status:   status,
			Path:     slot.Storage,
			Category: slot.Category,
		})
	}
	return items, nil
}

func (c *SabnzbdClient) GetQueue(ctx context.Context) ([]download.DownloadItem, error) {
	baseURL := c.buildBaseURL()
	params := url.Values{}
	params.Add("mode", "queue")
	params.Add("apikey", c.config.APIKey)
	params.Add("output", "json")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sabnzbd returned non-200 status: %d", resp.StatusCode)
	}

	var result sabnzbdQueueResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sabnzbd queue: %w", err)
	}

	var items []download.DownloadItem
	for _, slot := range result.Queue.Slots {
		items = append(items, download.DownloadItem{
			ID:       slot.NzoID,
			Name:     slot.Filename,
			Status:   slot.Status, // e.g., "Downloading", "Paused"
			Category: slot.Cat,
			Progress: slot.Percentage,
		})
	}
	return items, nil
}

func (c *SabnzbdClient) buildBaseURL() string {
	protocol := "http"
	if c.config.UseSSL {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s:%d/sabnzbd/api", protocol, c.config.Host, c.config.Port)
}
