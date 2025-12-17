package config

// AppConfig holds all the configuration for the application
type AppConfig struct {
	Server      ServerConfig      `yaml:"server"`
	Schema      SchemaConfig      `yaml:"schema"`
	Catalog     CatalogConfig     `yaml:"catalog"`
	Quality     QualityConfig     `yaml:"quality"`
	Acquisition AcquisitionConfig `yaml:"acquisition"`
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
	Type                  string   `yaml:"type"`
	IsLeaf                bool     `yaml:"is_leaf"`
	Children              []string `yaml:"children"`
	HasFiles              bool     `yaml:"has_files"`
	Variants              []string `yaml:"variants"`
	DefaultQualityProfile string   `yaml:"default_quality_profile"`
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
	Name   string        `yaml:"name"`
	Cutoff string        `yaml:"cutoff"`
	Items  []QualityItem `yaml:"items"`
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
	SearchQueryFormat string          `yaml:"search_query_format"`
	Naming            NamingConfig    `yaml:"naming"`
	DownloadClient    DownloadClient  `yaml:"download_client"`
	Indexers          []IndexerConfig `yaml:"indexers"`
}

// SystemInfo represents the complete system information exposed to external clients
type SystemInfo struct {
	Version      string           `json:"version"`
	RootEntity   string           `json:"root_entity"`
	MediaTypes   []MediaTypeInfo  `json:"media_types"`
	Architecture ArchitectureInfo `json:"architecture"`
	Providers    []ProviderInfo   `json:"providers"`
	Capabilities CapabilitiesInfo `json:"capabilities"`
}

// MediaTypeInfo represents information about a media type
type MediaTypeInfo struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"display_name"`
	Description    string   `json:"description"`
	Icon           string   `json:"icon"`
	IsLeaf         bool     `json:"is_leaf"`
	HasFiles       bool     `json:"has_files"`
	ParentTypes    []string `json:"parent_types"`
	ChildTypes     []string `json:"child_types"`
	Variants       []string `json:"variants"`
	QualityProfile string   `json:"default_quality_profile"`
}

// ArchitectureInfo represents system architecture details
type ArchitectureInfo struct {
	Version  string       `json:"version"`
	Database DatabaseInfo `json:"database"`
	Storage  StorageInfo  `json:"storage"`
	Features []string     `json:"features"`
}

// DatabaseInfo represents database configuration
type DatabaseInfo struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	MaxEntities int    `json:"max_entities"`
}

// StorageInfo represents storage configuration
type StorageInfo struct {
	Type      string `json:"type"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
}

// ProviderInfo represents metadata provider information
type ProviderInfo struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Available      bool     `json:"available"`
	SupportedTypes []string `json:"supported_types"`
}

// CapabilitiesInfo represents system capabilities
type CapabilitiesInfo struct {
	Streaming        bool `json:"streaming"`
	Transcoding      bool `json:"transcoding"`
	MetadataFetching bool `json:"metadata_fetching"`
	Search           bool `json:"search"`
	Download         bool `json:"download"`
	Import           bool `json:"import"`
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
