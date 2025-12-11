package hardcover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"omniarr/internal/metadata"
)

const (
	BaseURL = "https://api.hardcover.app/v1/graphql"
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

// GraphQL Request/Response Structures
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   Data   `json:"data"`
	Errors []Error `json:"errors,omitempty"`
}

type Error struct {
	Message string `json:"message"`
}

type Data struct {
	Books  []Book       `json:"books"`
	Search SearchResult `json:"search"`
	Lists  []List       `json:"lists"`
}

type List struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Slug        string `json:"slug"`
	ListBooks   []struct {
		Book Book `json:"book"`
	} `json:"list_books"`
}

type SearchResult struct {
	Results ResultsData `json:"results"`
}

type ResultsData struct {
	Hits []Hit `json:"hits"`
}

type Hit struct {
	Document SearchResultBook `json:"document"`
}

type SearchResultBook struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ReleaseYear int      `json:"release_year"`
	Pages       int      `json:"pages"`
	Slug        string   `json:"slug"`
	AuthorNames []string `json:"author_names"`
	Image       Image    `json:"image"`
	HasEbook    bool     `json:"has_ebook"`
}

type Book struct {
	ID            int            `json:"id"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	ReleaseDate   string         `json:"release_date"`
	Pages         int            `json:"pages"`
	Slug          string         `json:"slug"`
	Image         Image          `json:"image"`
	Contributions []Contribution `json:"contributions"`
}

type Image struct {
	URL string `json:"url"`
}

type Contribution struct {
	Author Author `json:"author"`
}

type Author struct {
	Name string `json:"name"`
}

func (p *Provider) GetMetadata(ctx context.Context, entityType string, id string) (*metadata.Metadata, error) {
	if entityType != "book" {
		return nil, fmt.Errorf("hardcover provider does not support entity type: %s", entityType)
	}

	// Hardcover IDs are integers, but our system uses strings.
	// We might need to handle slug vs ID lookup.
	// For now, assuming ID lookup.

	query := `
		query GetBook($id: Int!) {
			books(where: {id: {_eq: $id}}) {
				id
				title
				description
				release_date
				pages
				slug
				image {
					url
				}
				contributions {
					author {
						name
					}
				}
			}
		}
	`

	vars := map[string]interface{}{
		"id": id,
	}

	respData, err := p.makeRequest(ctx, query, vars)
	if err != nil {
		return nil, err
	}

	if len(respData.Books) == 0 {
		return nil, fmt.Errorf("book not found")
	}

	return p.mapBookToMetadata(respData.Books[0]), nil
}

func (p *Provider) Search(ctx context.Context, query string) ([]metadata.Metadata, error) {
	// Using the 'search' query as per documentation
	// sort: "users_count:desc,_text_match:desc"
	gqlQuery := `
		query SearchBooks($query: String!) {
			search(
				query: $query,
				query_type: "Book",
				per_page: 20,
				page: 1,
				fields: "title,series_names,author_names,alternative_titles",
				weights: "1,2,3,4"
				sort: "activities_count:desc,_text_match:desc"
			) {
				results
			}
		}
	`

	vars := map[string]interface{}{
		"query": query,
	}

	respData, err := p.makeRequest(ctx, gqlQuery, vars)
	if err != nil {
		return nil, err
	}

	var results []metadata.Metadata
	for _, hit := range respData.Search.Results.Hits {
		results = append(results, *p.mapSearchResultToMetadata(hit.Document))
	}

	return results, nil
}

func (p *Provider) GetLists(ctx context.Context, listIDs []string) ([]metadata.Metadata, error) {
	if len(listIDs) == 0 {
		return []metadata.Metadata{}, nil
	}

	// Convert string IDs to ints for the query
	// Note: Hardcover uses integer IDs for lists
	var ids []int
	for _, idStr := range listIDs {
		var id int
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return []metadata.Metadata{}, nil
	}

	query := `
		query GetLists($ids: [Int!]) {
			lists(where: {id: {_in: $ids}}) {
				id
				name
				description
				slug
				list_books(limit: 1000) {
					book {
						id
						title
						description
						release_date
						pages
						slug
						image {
							url
						}
						contributions {
							author {
								name
							}
						}
					}
				}
			}
		}
	`

	vars := map[string]interface{}{
		"ids": ids,
	}

	respData, err := p.makeRequest(ctx, query, vars)
	if err != nil {
		return nil, err
	}

	var results []metadata.Metadata
	for _, list := range respData.Lists {
		listMeta := metadata.Metadata{
			ID:          fmt.Sprintf("%d", list.ID),
			Type:        "list",
			Title:       list.Name,
			Description: list.Description,
			Identifiers: map[string]string{
				"slug": list.Slug,
			},
		}

		for _, lb := range list.ListBooks {
			listMeta.Children = append(listMeta.Children, *p.mapBookToMetadata(lb.Book))
		}

		results = append(results, listMeta)
	}

	return results, nil
}

func (p *Provider) makeRequest(ctx context.Context, query string, vars map[string]interface{}) (*Data, error) {
	reqBody := GraphQLRequest{
		Query:     query,
		Variables: vars,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", BaseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hardcover api returned status: %d", resp.StatusCode)
	}

	var gqlResp GraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, err
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	return &gqlResp.Data, nil
}

func (p *Provider) mapBookToMetadata(book Book) *metadata.Metadata {
	year := ""
	if len(book.ReleaseDate) >= 4 {
		year = book.ReleaseDate[:4]
	}

	var authors []string
	for _, c := range book.Contributions {
		authors = append(authors, c.Author.Name)
	}

	return &metadata.Metadata{
		ID:          fmt.Sprintf("%d", book.ID),
		Title:       book.Title,
		Description: book.Description,
		Year:        year,
		Authors:     authors,
		Image:       book.Image.URL,
		PageCount:   book.Pages,
		Identifiers: map[string]string{
			"slug": book.Slug,
		},
	}
}

func (p *Provider) mapSearchResultToMetadata(item SearchResultBook) *metadata.Metadata {
	return &metadata.Metadata{
		ID:          item.ID,
		Title:       item.Title,
		Description: item.Description,
		Year:        fmt.Sprintf("%d", item.ReleaseYear),
		Authors:     item.AuthorNames,
		Image:       item.Image.URL,
		PageCount:   item.Pages,
		Identifiers: map[string]string{
			"slug": item.Slug,
		},
	}
}