package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadConfig reads all configuration files from the specified directory
func LoadConfig(configDir string) (*AppConfig, error) {
	appConfig := &AppConfig{}

	if err := loadYaml(filepath.Join(configDir, "schema.yaml"), &appConfig.Schema); err != nil {
		return nil, fmt.Errorf("failed to load schema.yaml: %w", err)
	}

	if err := loadYaml(filepath.Join(configDir, "catalog.yaml"), &appConfig.Catalog); err != nil {
		return nil, fmt.Errorf("failed to load catalog.yaml: %w", err)
	}

	if err := loadYaml(filepath.Join(configDir, "quality.yaml"), &appConfig.Quality); err != nil {
		return nil, fmt.Errorf("failed to load quality.yaml: %w", err)
	}

	if err := loadYaml(filepath.Join(configDir, "acquisition.yaml"), &appConfig.Acquisition); err != nil {
		return nil, fmt.Errorf("failed to load acquisition.yaml: %w", err)
	}

	return appConfig, nil
}

func loadYaml(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, target)
}