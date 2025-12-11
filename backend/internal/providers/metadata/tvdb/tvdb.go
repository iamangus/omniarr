package tvdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"omniarr/internal/metadata"
)

const (
	BaseURL = "https://api4.thetvdb.com/v4"
)

type Provider struct {
	apiKey string
	token  string
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

// TVDB Response Structures

type Response[T any] struct {
	Status  string `json:"status"`
	Data    T      `json:"data"`
	Message string `json:"message,omitempty"`
}

type SearchResult struct {
	ID                string   `json:"id"` // TVDB returns string IDs in v4 search? Actually it might be string or int. Let's check.
	Name              string   `json:"name"`
	Overview          string   `json:"overview"`
	Image             string   `json:"image_url"`
	FirstAired        string   `json:"first_air_time"`
	Year              string   `json:"year"`
	Type              string   `json:"type"` // "series", "movie"
	TvdbID            string   `json:"tvdb_id"`
	Slug              string   `json:"slug"`
	Status            string   `json:"status"`
	PrimaryLanguage   string   `json:"primary_language"`
	Thumbnail         string   `json:"thumbnail"`
}

// Custom unmarshal for SearchResult because ID might be int or string in some contexts, 
// but usually in search data it's a string or we can handle it. 
// Actually, let's stick to standard struct and see.

type SeriesExtended struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Image        string   `json:"image"`
	FirstAired   string   `json:"firstAired"`
	Overview     string   `json:"overview"`
	Status       Status   `json:"status"`
	Characters   []Character `json:"characters"`
	Seasons      []Season `json:"seasons"`
}

type MovieExtended struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Image        string   `json:"image"`
	FirstAired   string   `json:"firstAired"` // or releaseDate
	Overview     string   `json:"overview"`
	Status       Status   `json:"status"`
	Characters   []Character `json:"characters"`
}

type Status struct {
	Name string `json:"name"`
}

type Character struct {
	Name   string `json:"name"`
	People People `json:"people"`
}

type People struct {
	Name string `json:"name"`
}

type SeasonType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Season struct {
	ID     int64      `json:"id"`
	Name   string     `json:"name"`
	Number int        `json:"number"`
	Image  string     `json:"image"`
	Type   SeasonType `json:"type"`
}

func (p *Provider) GetMetadata(ctx context.Context, entityType string, id string) (*metadata.Metadata, error) {
	// TVDB IDs are integers usually, but we handle them as strings.
	
	var endpoint string
	if entityType == "series" || entityType == "tv" {
		endpoint = fmt.Sprintf("/series/%s/extended", id)
	} else if entityType == "movie" {
		endpoint = fmt.Sprintf("/movies/%s/extended", id)
	} else {
		return nil, fmt.Errorf("tvdb provider does not support entity type: %s", entityType)
	}

	// We need to handle the response differently based on type
	if entityType == "series" || entityType == "tv" {
		var resp Response[SeriesExtended]
		if err := p.makeRequest(ctx, "GET", endpoint, nil, &resp); err != nil {
			return nil, err
		}
		return p.mapSeriesToMetadata(resp.Data), nil
	} else {
		var resp Response[MovieExtended]
		if err := p.makeRequest(ctx, "GET", endpoint, nil, &resp); err != nil {
			return nil, err
		}
		return p.mapMovieToMetadata(resp.Data), nil
	}
}

func (p *Provider) Search(ctx context.Context, query string) ([]metadata.Metadata, error) {
	endpoint := fmt.Sprintf("/search?query=%s&type=series", url.QueryEscape(query))
	
	var resp Response[[]SearchResult]
	if err := p.makeRequest(ctx, "GET", endpoint, nil, &resp); err != nil {
		return nil, err
	}

	var results []metadata.Metadata
	for _, item := range resp.Data {
		results = append(results, p.mapSearchResultToMetadata(item))
	}

	return results, nil
}

func (p *Provider) GetLists(ctx context.Context, listIDs []string) ([]metadata.Metadata, error) {
	// TVDB doesn't have a direct "GetLists" equivalent that matches Hardcover's structure easily accessible 
	// without more complex logic. Returning empty for now.
	return []metadata.Metadata{}, nil
}

func (p *Provider) authenticate(ctx context.Context) error {
	url := BaseURL + "/login"
	body := map[string]string{
		"apikey": p.apiKey,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed with status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return err
	}

	p.token = loginResp.Data.Token
	return nil
}

func (p *Provider) makeRequest(ctx context.Context, method, endpoint string, body interface{}, target interface{}) error {
	if p.token == "" {
		if err := p.authenticate(ctx); err != nil {
			return err
		}
	}

	url := BaseURL + endpoint

	var req *http.Request
	var err error

	req, err = http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)

	fmt.Printf("TVDB Request: %s %s\n", method, url)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Println("TVDB Token expired or invalid, re-authenticating...")
		if err := p.authenticate(ctx); err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+p.token)
		resp, err = p.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("TVDB API error: %s %s returned %d: %s\n", method, url, resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("tvdb api returned status: %d", resp.StatusCode)
	}

	// Read body to log it if needed
	bodyBytes, _ := io.ReadAll(resp.Body)
	// Restore body for decoder
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(target); err != nil {
		return err
	}

	return nil
}

func (p *Provider) mapSeriesToMetadata(item SeriesExtended) *metadata.Metadata {
	year := ""
	if len(item.FirstAired) >= 4 {
		year = item.FirstAired[:4]
	}

	var authors []string // Using Authors field for Actors/Characters for now
	for _, c := range item.Characters {
		if c.People.Name != "" {
			authors = append(authors, c.People.Name)
		}
	}

	meta := &metadata.Metadata{
		ID:          fmt.Sprintf("%d", item.ID),
		Type:        "series",
		Title:       item.Name,
		Description: item.Overview,
		Year:        year,
		Authors:     authors,
		Image:       item.Image,
		Identifiers: map[string]string{
			"slug": item.Slug,
			"tvdb": fmt.Sprintf("%d", item.ID),
		},
	}

	// Map seasons to children
	var uniqueSeasons []Season
	// Map to store the best season found so far for each season number
	// We prefer Type.ID == 1 (Aired Order)
	bestSeasons := make(map[int]Season)

	for _, s := range item.Seasons {
		// Skip specials (Season 0)
		if s.Number == 0 {
			continue
		}

		existing, exists := bestSeasons[s.Number]
		if !exists {
			bestSeasons[s.Number] = s
			continue
		}

		// If we already have a season, check if the new one is better
		// We prefer Type.ID == 1
		if s.Type.ID == 1 && existing.Type.ID != 1 {
			bestSeasons[s.Number] = s
		}
	}

	for _, s := range bestSeasons {
		uniqueSeasons = append(uniqueSeasons, s)
	}

	// Sort seasons by number descending (Newest first)
	sort.Slice(uniqueSeasons, func(i, j int) bool {
		return uniqueSeasons[i].Number > uniqueSeasons[j].Number
	})

	for _, s := range uniqueSeasons {
		title := fmt.Sprintf("Season %d", s.Number)

		meta.Children = append(meta.Children, metadata.Metadata{
			ID:    fmt.Sprintf("%d", s.ID),
			Type:  "season",
			Title: title,
			Extra: map[string]interface{}{
				"number": s.Number,
			},
			Image: s.Image,
		})
	}

	return meta
}

func (p *Provider) mapMovieToMetadata(item MovieExtended) *metadata.Metadata {
	year := ""
	if len(item.FirstAired) >= 4 {
		year = item.FirstAired[:4]
	}

	var authors []string
	for _, c := range item.Characters {
		if c.People.Name != "" {
			authors = append(authors, c.People.Name)
		}
	}

	return &metadata.Metadata{
		ID:          fmt.Sprintf("%d", item.ID),
		Type:        "movie",
		Title:       item.Name,
		Description: item.Overview,
		Year:        year,
		Authors:     authors,
		Image:       item.Image,
		Identifiers: map[string]string{
			"slug": item.Slug,
			"tvdb": fmt.Sprintf("%d", item.ID),
		},
	}
}

func (p *Provider) mapSearchResultToMetadata(item SearchResult) metadata.Metadata {
	// Use TvdbID if available as it's the numeric ID expected by other endpoints
	id := item.ID
	if item.TvdbID != "" {
		id = item.TvdbID
	}

	return metadata.Metadata{
		ID:          id,
		Type:        item.Type,
		Title:       item.Name,
		Description: item.Overview,
		Year:        item.Year,
		Image:       item.Image,
		Identifiers: map[string]string{
			"slug": item.Slug,
			"tvdb": item.TvdbID,
		},
	}
}