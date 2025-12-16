package search

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"omniarr/internal/config"
	"omniarr/internal/database"
	"omniarr/internal/domain"
	"omniarr/internal/download"
	"omniarr/internal/providers/indexer"
	"omniarr/internal/utils"
)

// SearchManager handles finding releases for WANTED items.
type SearchManager struct {
	repo           *database.EntityRepository
	downloadClient download.DownloadClient
	config         *config.AcquisitionConfig
	qualityConfig  *config.QualityConfig
	schemaConfig   *config.SchemaConfig
	indexers       []*indexer.TorznabClient
}

// NewSearchManager creates a new instance of SearchManager.
func NewSearchManager(repo *database.EntityRepository, downloadClient download.DownloadClient, cfg *config.AcquisitionConfig, qualityCfg *config.QualityConfig, schemaCfg *config.SchemaConfig) *SearchManager {
	var indexers []*indexer.TorznabClient
	for _, idxCfg := range cfg.Indexers {
		if idxCfg.Type == "torznab" {
			indexers = append(indexers, indexer.NewTorznabClient(idxCfg))
		}
	}

	return &SearchManager{
		repo:           repo,
		downloadClient: downloadClient,
		config:         cfg,
		qualityConfig:  qualityCfg,
		schemaConfig:   schemaCfg,
		indexers:       indexers,
	}
}

// PerformSearch triggers a search for a specific entity.
func (m *SearchManager) PerformSearch(ctx context.Context, entity *domain.Entity) error {
	// Check if this entity type has files (is a leaf/downloadable)
	hasFiles := false
	for _, et := range m.schemaConfig.Entities {
		if et.Type == entity.EntityType {
			hasFiles = et.HasFiles
			break
		}
	}

	if hasFiles {
		if entity.Status == domain.StatusWanted {
			return m.searchLeafEntity(ctx, entity)
		}
		return nil
	}

	// If not a leaf, recurse children
	children, err := m.repo.GetChildren(ctx, entity.UUID.String())
	if err != nil {
		return fmt.Errorf("failed to get children for %s: %w", entity.UUID, err)
	}

	for _, child := range children {
		if err := m.PerformSearch(ctx, &child); err != nil {
			fmt.Printf("Failed to search for child %s: %v\n", child.UUID, err)
			// Continue with other children
		}
	}

	return nil
}

func (m *SearchManager) searchLeafEntity(ctx context.Context, entity *domain.Entity) error {
	// 1. Generate search query
	query, err := m.buildQuery(ctx, entity)
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	fmt.Printf("Searching for entity: %s (Query: %s)\n", entity.UUID, query)

	var allReleases []domain.Release

	// 2. Query Indexers
	for _, idx := range m.indexers {
		releases, err := idx.Search(query)
		if err != nil {
			fmt.Printf("Error searching indexer: %v\n", err)
			continue
		}
		allReleases = append(allReleases, releases...)
	}

	fmt.Printf("Found %d releases\n", len(allReleases))

	// 3. Parse and Score Results
	bestRelease := m.processResults(entity, allReleases)

	// 4. Send to Download Client if a valid release is found
	if bestRelease != nil {
		fmt.Printf("Found best release: %s\n", bestRelease.Title)
		id, err := m.downloadClient.Download(ctx, bestRelease.Link, m.config.DownloadClient.Category)
		if err != nil {
			return fmt.Errorf("failed to send to download client: %w", err)
		}
		fmt.Printf("Sent to download client. ID: %s\n", id)

		// 5. Update Entity with Download Client ID
		entity.DownloadClientID = &id
		if err := m.repo.Save(ctx, entity); err != nil {
			return fmt.Errorf("failed to save entity download ID: %w", err)
		}
	}

	return nil
}

func (m *SearchManager) buildQuery(ctx context.Context, entity *domain.Entity) (string, error) {
	// Collect metadata from entity and all parents
	data := make(map[string]interface{})

	current := entity
	for {
		var meta map[string]interface{}
		if len(current.Metadata) > 0 {
			if err := json.Unmarshal(current.Metadata, &meta); err != nil {
				return "", fmt.Errorf("failed to unmarshal metadata for %s: %w", current.UUID, err)
			}
			// Merge metadata (child overrides parent if collision, but we usually namespace or use distinct keys)
			// Ideally we should namespace by EntityType? e.g. {Series: {...}, Season: {...}}
			// The config uses {Series.Title}, so we should structure it that way.

			// Capitalize first letter of entity type for consistency with config (Series, Season, Episode)
			// Or just use the type as is. Config uses "Series", "Season".
			// Let's assume Title Case.
			key := strings.Title(current.EntityType)

			// Ensure keys are Title Case for template matching (e.g. {Book.Title} matches "title" in json)
			for k, v := range meta {
				if len(k) > 0 {
					// Simple capitalization of first letter
					newKey := strings.ToUpper(k[:1]) + k[1:]
					if _, exists := meta[newKey]; !exists {
						meta[newKey] = v
					}
				}
			}

			data[key] = meta

			// Also flatten for direct access if needed?
			// {Title} usually refers to the leaf title?
			// In "Series S01E01", {Title} is Series Title.
			// But in acquisition.yaml: "{Title} S{Season:02d}E{Episode:02d}"
			// Wait, {Title} usually means the Series Title in Sonarr context?
			// Or does it mean the Episode Title?
			// In the config provided:
			// search_query_format: "{Title} S{Season:02d}E{Episode:02d}"
			// This implies {Title} is Series Title, {Season} is Season Number, {Episode} is Episode Number.
			// But where do these come from?
			// If we namespace, we'd need {Series.Title}.
			// If we don't namespace, we have collisions (Series Title vs Episode Title).

			// Let's look at catalog.yaml:
			// Series attributes: Title
			// Season attributes: Title (Season 1), SeasonNumber
			// Episode attributes: Title (Episode Name), SeasonNumber (maybe?), EpisodeNumber (implied?)

			// If we flatten, Episode Title overwrites Series Title.
			// So we MUST namespace.
			// But the config `search_query_format: "{Title} S{Season:02d}E{Episode:02d}"`
			// uses {Title} without namespace.
			// This suggests {Title} might be special or come from the Series?
			// OR the user config is wrong/simplified.
			// Let's assume we expose namespaced data: data["Series"]["Title"]
			// And maybe we also expose "Title" as the Root Entity Title?

			// Let's stick to namespacing as seen in `naming` config:
			// folder: "/tv/{Series.Title} ({Series.Year})"
			// file: "{Series.Title} - S{Season:02d}E{Episode:02d} - {Title} - {Quality}.{Ext}"
			// Here {Title} likely means Episode Title.

			// So for search query: "{Title} S{Season:02d}E{Episode:02d}"
			// If {Title} is Episode Title, searching "My Episode Name S01E01" is wrong.
			// It should be "Series Name S01E01".
			// So the config should probably be "{Series.Title} ..."

			// However, I should support what's in the config.
			// If the config says {Title}, and I provide {Series.Title}, it won't match.
			// I'll provide both namespaced and flattened (with child winning).
			// But for "Title", we might want the Root Title?
			// Let's provide namespaced data.
		}

		if current.ParentUUID == nil {
			break
		}
		parent, err := m.repo.Get(ctx, current.ParentUUID.String())
		if err != nil {
			return "", fmt.Errorf("failed to get parent %s: %w", current.ParentUUID, err)
		}
		current = parent
	}

	return utils.ResolveTemplate(m.config.SearchQueryFormat, data)
}

// processResults parses, scores, and returns the best candidate.
func (m *SearchManager) processResults(entity *domain.Entity, results []domain.Release) *domain.Release {
	// 1. Get Profile
	profileID := 0
	if entity.QualityProfileID != nil {
		profileID = *entity.QualityProfileID
	}
	if profileID < 0 || profileID >= len(m.qualityConfig.Profiles) {
		profileID = 0 // Fallback
	}
	profile := m.qualityConfig.Profiles[profileID]

	type ScoredRelease struct {
		Release domain.Release
		Score   int
	}
	var scored []ScoredRelease

	for _, r := range results {
		qualityName := m.parseQuality(r.Title)
		score := m.calculateScore(profile, qualityName)
		// We include it even if score is 0, but usually we want > 0
		scored = append(scored, ScoredRelease{Release: r, Score: score})
	}

	// Sort by Score Descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > 0 {
		// Check cutoff?
		// For now, just return the best one.
		return &scored[0].Release
	}
	return nil
}

func (m *SearchManager) parseQuality(title string) string {
	for _, def := range m.qualityConfig.Definitions {
		// Case insensitive matching
		matched, _ := regexp.MatchString("(?i)"+def.Regex, title)
		if matched {
			return def.Name
		}
	}
	return "Unknown"
}

func (m *SearchManager) calculateScore(profile config.QualityProfile, qualityName string) int {
	for _, item := range profile.Items {
		if item.Name == qualityName {
			return item.Score
		}
	}
	return 0
}
