package config

// AppConfig holds all the configuration for the application
type AppConfig struct {
	Server      ServerConfig
	Schema      SchemaConfig
	Catalog     CatalogConfig
	Quality     QualityConfig
	Acquisition AcquisitionConfig
}

// ServerConfig maps to server.yaml
type ServerConfig struct {
	Port             int    `yaml:"port"`
	APIKey           string `yaml:"api_key"`
	ImageStoragePath string `yaml:"image_storage_path"`
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
	Variants []string `yaml:"variants"`
	DefaultQualityProfile string `yaml:"default_quality_profile"`
}

// CatalogConfig maps to catalog.yaml
type CatalogConfig struct {
	Provider string   `yaml:"provider"`
	APIKey   string   `yaml:"api_key"`
	Lists    []string `yaml:"lists"`
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