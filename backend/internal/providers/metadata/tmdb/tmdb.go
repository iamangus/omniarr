package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"omniarr/internal/metadata"
)

const (
	DefaultBaseURL = "https://api.themoviedb.org/3"
	ImageBaseURL   = "https://image.tmdb.org/t/p/original"
)

type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TMDB Response Structures

type SearchResponse struct {
	Page         int            `json:"page"`
	Results      []SearchResult `json:"results"`
	TotalPages   int            `json:"total_pages"`
	TotalResults int            `json:"total_results"`
}

type SearchResult struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"` // "movie", "tv", "person"
	Name         string  `json:"name,omitempty"`
	Title        string  `json:"title,omitempty"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	ReleaseDate  string  `json:"release_date,omitempty"`   // For movies
	FirstAirDate string  `json:"first_air_date,omitempty"` // For tv
	VoteAverage  float64 `json:"vote_average"`
}

type MovieDetails struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Overview    string     `json:"overview"`
	PosterPath  string     `json:"poster_path"`
	ReleaseDate string     `json:"release_date"`
	Genres      []Genre    `json:"genres"`
	Runtime     int        `json:"runtime"`
	Credits     Credits    `json:"credits"`
	ExternalIDs ExternalID `json:"external_ids"`
}

type TVDetails struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Overview     string     `json:"overview"`
	PosterPath   string     `json:"poster_path"`
	FirstAirDate string     `json:"first_air_date"`
	Genres       []Genre    `json:"genres"`
	Seasons      []Season   `json:"seasons"`
	Credits      Credits    `json:"credits"`
	ExternalIDs  ExternalID `json:"external_ids"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Season struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"poster_path"`
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date"`
}

type Credits struct {
	Cast []CastMember `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

type CastMember struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Role string `json:"character"`
}

type CrewMember struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Job  string `json:"job"`
}

type ExternalID struct {
	IMDBID string `json:"imdb_id"`
	TVDBID int    `json:"tvdb_id"`
}

// GetMetadata fetches metadata for a specific entity type and ID
func (p *Provider) GetMetadata(ctx context.Context, entityType string, id string) (*metadata.Metadata, error) {
	// TMDB IDs are integers
	tmdbID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid tmdb id: %s", id)
	}

	if entityType == "series" || entityType == "tv" {
		return p.getTVMetadata(ctx, tmdbID)
	} else if entityType == "movie" {
		return p.getMovieMetadata(ctx, tmdbID)
	}

	return nil, fmt.Errorf("tmdb provider does not support entity type: %s", entityType)
}

func (p *Provider) getMovieMetadata(ctx context.Context, id int) (*metadata.Metadata, error) {
	endpoint := fmt.Sprintf("/movie/%d", id)
	params := url.Values{}
	params.Set("append_to_response", "credits,external_ids")

	var details MovieDetails
	if err := p.makeRequest(ctx, endpoint, params, &details); err != nil {
		return nil, err
	}

	year := ""
	if len(details.ReleaseDate) >= 4 {
		year = details.ReleaseDate[:4]
	}

	var authors []string
	// Use top 5 cast members as authors/actors
	count := 0
	for _, cast := range details.Credits.Cast {
		authors = append(authors, cast.Name)
		count++
		if count >= 5 {
			break
		}
	}

	identifiers := map[string]string{
		"tmdb": fmt.Sprintf("%d", details.ID),
	}
	if details.ExternalIDs.IMDBID != "" {
		identifiers["imdb"] = details.ExternalIDs.IMDBID
	}

	imageURL := ""
	if details.PosterPath != "" {
		imageURL = ImageBaseURL + details.PosterPath
	}

	return &metadata.Metadata{
		ID:          fmt.Sprintf("%d", details.ID),
		Type:        "movie",
		Title:       details.Title,
		Description: details.Overview,
		Year:        year,
		Authors:     authors,
		Image:       imageURL,
		Identifiers: identifiers,
	}, nil
}

func (p *Provider) getTVMetadata(ctx context.Context, id int) (*metadata.Metadata, error) {
	endpoint := fmt.Sprintf("/tv/%d", id)
	params := url.Values{}
	params.Set("append_to_response", "credits,external_ids")

	var details TVDetails
	if err := p.makeRequest(ctx, endpoint, params, &details); err != nil {
		return nil, err
	}

	year := ""
	if len(details.FirstAirDate) >= 4 {
		year = details.FirstAirDate[:4]
	}

	var authors []string
	count := 0
	for _, cast := range details.Credits.Cast {
		authors = append(authors, cast.Name)
		count++
		if count >= 5 {
			break
		}
	}

	identifiers := map[string]string{
		"tmdb": fmt.Sprintf("%d", details.ID),
	}
	if details.ExternalIDs.IMDBID != "" {
		identifiers["imdb"] = details.ExternalIDs.IMDBID
	}
	if details.ExternalIDs.TVDBID != 0 {
		identifiers["tvdb"] = fmt.Sprintf("%d", details.ExternalIDs.TVDBID)
	}

	imageURL := ""
	if details.PosterPath != "" {
		imageURL = ImageBaseURL + details.PosterPath
	}

	meta := &metadata.Metadata{
		ID:          fmt.Sprintf("%d", details.ID),
		Type:        "series",
		Title:       details.Name,
		Description: details.Overview,
		Year:        year,
		Authors:     authors,
		Image:       imageURL,
		Identifiers: identifiers,
	}

	// Map seasons
	var seasons []Season
	for _, s := range details.Seasons {
		if s.SeasonNumber > 0 { // Skip specials for now or handle them? usually > 0 is main
			seasons = append(seasons, s)
		}
	}
	// Sort seasons desc
	sort.Slice(seasons, func(i, j int) bool {
		return seasons[i].SeasonNumber > seasons[j].SeasonNumber
	})

	for _, s := range seasons {
		sImage := ""
		if s.PosterPath != "" {
			sImage = ImageBaseURL + s.PosterPath
		}

		meta.Children = append(meta.Children, metadata.Metadata{
			ID:    fmt.Sprintf("%d", s.ID),
			Type:  "season",
			Title: s.Name,
			Extra: map[string]interface{}{
				"number":        s.SeasonNumber,
				"episode_count": s.EpisodeCount,
			},
			Image: sImage,
			Year:  s.AirDate,
		})
	}

	return meta, nil
}

// Search queries the provider for entities matching the query string
func (p *Provider) Search(ctx context.Context, query string) ([]metadata.Metadata, error) {
	endpoint := "/search/multi"
	params := url.Values{}
	params.Set("query", query)
	params.Set("include_adult", "false")

	var resp SearchResponse
	if err := p.makeRequest(ctx, endpoint, params, &resp); err != nil {
		return nil, err
	}

	var results []metadata.Metadata
	for _, item := range resp.Results {
		if item.MediaType == "person" {
			continue
		}

		title := item.Title
		if item.Name != "" {
			title = item.Name
		}

		date := item.ReleaseDate
		if item.FirstAirDate != "" {
			date = item.FirstAirDate
		}
		year := ""
		if len(date) >= 4 {
			year = date[:4]
		}

		imageURL := ""
		if item.PosterPath != "" {
			imageURL = ImageBaseURL + item.PosterPath
		}

		entityType := item.MediaType
		if entityType == "tv" {
			entityType = "series"
		}

		results = append(results, metadata.Metadata{
			ID:          fmt.Sprintf("%d", item.ID),
			Type:        entityType,
			Title:       title,
			Description: item.Overview,
			Year:        year,
			Image:       imageURL,
			Identifiers: map[string]string{
				"tmdb": fmt.Sprintf("%d", item.ID),
			},
		})
	}

	return results, nil
}

// GetLists fetches curated lists of content
func (p *Provider) GetLists(ctx context.Context, listIDs []string) ([]metadata.Metadata, error) {
	return []metadata.Metadata{}, nil
}

func (p *Provider) makeRequest(ctx context.Context, endpoint string, params url.Values, target interface{}) error {
	reqURL := p.baseURL + endpoint

	// Add API Key

	params.Set("api_key", p.apiKey)
	reqURL = fmt.Sprintf("%s?%s", reqURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Debug log
	// fmt.Printf("TMDB Request: %s\n", reqURL)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb api returned status: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return err
	}

	return nil
}
