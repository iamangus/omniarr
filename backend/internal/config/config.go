package config

// AppConfig holds all the configuration for the application
type AppConfig struct {
	Schema      SchemaConfig
	Catalog     CatalogConfig
	Quality     QualityConfig
	Acquisition AcquisitionConfig
}

// SchemaConfig maps to schema.yaml
type SchemaConfig struct {
	RootEntity string       `yaml:"root_entity"`
	Entities   []EntityType `yaml:"entities"`
}

type EntityType struct {
	Type     string   `yaml:"type"`
	IsLeaf   bool     `yaml:"is_leaf"`
	Children []string `yaml:"children"`
	HasFiles bool     `yaml:"has_files"`
}

// CatalogConfig maps to catalog.yaml
type CatalogConfig struct {
	Provider  string            `yaml:"provider"`
	BaseURL   string            `yaml:"base_url"`
	APIKey    string            `yaml:"api_key"`
	Endpoints []EndpointMapping `yaml:"endpoints"`
}

type EndpointMapping struct {
	EntityType  string            `yaml:"entity_type"`
	URL         string            `yaml:"url"`
	QueryParam  string            `yaml:"query_param"` // Optional: defaults to "query"
	ResultsKey  string            `yaml:"results_key"` // Optional: defaults to "results"
	Attributes  map[string]string `yaml:"attributes"`  // JSONPath mappings
	Identifiers []IdentifierMap   `yaml:"identifiers"`
	Children    []ChildMapping    `yaml:"children"`
}

type ChildMapping struct {
	EntityType string `yaml:"entity_type"`
	Source     string `yaml:"source"` // JSONPath to array
	IDFormat   string `yaml:"id_format"`
}

type IdentifierMap struct {
	Key    string `yaml:"key"`
	Source string `yaml:"source"` // JSONPath
}

// QualityConfig maps to quality.yaml
type QualityConfig struct {
	Profiles    []QualityProfile    `yaml:"profiles"`
	Definitions []QualityDefinition `yaml:"definitions"`
}

type QualityProfile struct {
	Name   string            `yaml:"name"`
	Cutoff string            `yaml:"cutoff"`
	Items  []QualityItem     `yaml:"items"`
}

type QualityItem struct {
	Name  string `yaml:"name"`
	Score int    `yaml:"score"`
}

type QualityDefinition struct {
	Name  string `yaml:"name"`
	Regex string `yaml:"regex"`
}

// AcquisitionConfig maps to acquisition.yaml
type AcquisitionConfig struct {
	SearchQueryFormat string         `yaml:"search_query_format"`
	Naming            NamingConfig   `yaml:"naming"`
	DownloadClient    DownloadClient `yaml:"download_client"`
	Indexers          []IndexerConfig `yaml:"indexers"`
}

type IndexerConfig struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	URL        string `yaml:"url"`
	APIKey     string `yaml:"api_key"`
	Categories []int  `yaml:"categories"`
}

type NamingConfig struct {
	Folder string `yaml:"folder"`
	File   string `yaml:"file"`
}

type DownloadClient struct {
	Type     string `yaml:"type"`
	Category string `yaml:"category"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	APIKey   string `yaml:"api_key"`
	UseSSL   bool   `yaml:"use_ssl"`
}