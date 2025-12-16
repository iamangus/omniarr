package tmdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProvider_Search(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/multi" {
			t.Errorf("Expected path /search/multi, got %s", r.URL.Path)
		}

		response := `{
			"page": 1,
			"results": [
				{
					"id": 1,
					"media_type": "movie",
					"title": "Batman Begins",
					"overview": "Dark Knight origin",
					"release_date": "2005-06-15",
					"poster_path": "/poster.jpg"
				},
				{
					"id": 2,
					"media_type": "tv",
					"name": "Batman: The Animated Series",
					"overview": "Best cartoon ever",
					"first_air_date": "1992-09-05",
					"poster_path": "/poster_tv.jpg"
				},
				{
					"id": 3,
					"media_type": "person",
					"name": "Christian Bale"
				}
			],
			"total_pages": 1,
			"total_results": 3
		}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, response)
	}))
	defer ts.Close()

	p := &Provider{
		apiKey:  "test-key",
		baseURL: ts.URL,
		client:  &http.Client{Timeout: 1 * time.Second},
	}

	results, err := p.Search(context.Background(), "batman")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results (filtered person), got %d", len(results))
	}

	// Check Movie
	if results[0].Title != "Batman Begins" {
		t.Errorf("Expected title 'Batman Begins', got '%s'", results[0].Title)
	}
	if results[0].Type != "movie" {
		t.Errorf("Expected type 'movie', got '%s'", results[0].Type)
	}
	if results[0].Year != "2005" {
		t.Errorf("Expected year '2005', got '%s'", results[0].Year)
	}

	// Check TV
	if results[1].Title != "Batman: The Animated Series" {
		t.Errorf("Expected title 'Batman: The Animated Series', got '%s'", results[1].Title)
	}
	if results[1].Type != "series" {
		t.Errorf("Expected type 'series', got '%s'", results[1].Type)
	}
	if results[1].Year != "1992" {
		t.Errorf("Expected year '1992', got '%s'", results[1].Year)
	}
}

func TestProvider_GetMetadata_Movie(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/1" {
			t.Errorf("Expected path /movie/1, got %s", r.URL.Path)
		}

		response := `{
			"id": 1,
			"title": "Test Movie",
			"overview": "Test Overview",
			"release_date": "2023-01-01",
			"poster_path": "/test.jpg",
			"external_ids": {
				"imdb_id": "tt12345"
			},
			"credits": {
				"cast": [
					{"name": "Actor 1"},
					{"name": "Actor 2"}
				]
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, response)
	}))
	defer ts.Close()

	p := &Provider{
		apiKey:  "test-key",
		baseURL: ts.URL,
		client:  &http.Client{Timeout: 1 * time.Second},
	}

	meta, err := p.GetMetadata(context.Background(), "movie", "1")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	if meta.Title != "Test Movie" {
		t.Errorf("Expected title 'Test Movie', got '%s'", meta.Title)
	}
	if meta.Year != "2023" {
		t.Errorf("Expected year '2023', got '%s'", meta.Year)
	}
	if len(meta.Authors) != 2 {
		t.Errorf("Expected 2 authors, got %d", len(meta.Authors))
	}
	if meta.Identifiers["imdb"] != "tt12345" {
		t.Errorf("Expected imdb id tt12345, got %s", meta.Identifiers["imdb"])
	}
}

func TestProvider_GetMetadata_TV(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/1" {
			t.Errorf("Expected path /tv/1, got %s", r.URL.Path)
		}

		response := `{
			"id": 1,
			"name": "Test Series",
			"overview": "Test Overview",
			"first_air_date": "2020-01-01",
			"poster_path": "/test.jpg",
			"external_ids": {
				"imdb_id": "tt67890",
				"tvdb_id": 999
			},
			"credits": {
				"cast": []
			},
			"seasons": [
				{
					"id": 10,
					"name": "Season 1",
					"season_number": 1,
					"episode_count": 10
				},
				{
					"id": 11,
					"name": "Specials",
					"season_number": 0,
					"episode_count": 2
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, response)
	}))
	defer ts.Close()

	p := &Provider{
		apiKey:  "test-key",
		baseURL: ts.URL,
		client:  &http.Client{Timeout: 1 * time.Second},
	}

	meta, err := p.GetMetadata(context.Background(), "tv", "1")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	if meta.Title != "Test Series" {
		t.Errorf("Expected title 'Test Series', got '%s'", meta.Title)
	}
	if meta.Identifiers["tvdb"] != "999" {
		t.Errorf("Expected tvdb id 999, got %s", meta.Identifiers["tvdb"])
	}

	// Should have 1 child (Season 1), skipping Season 0
	if len(meta.Children) != 1 {
		t.Errorf("Expected 1 season, got %d", len(meta.Children))
	}
	if meta.Children[0].Title != "Season 1" {
		t.Errorf("Expected Season 1, got %s", meta.Children[0].Title)
	}
}
