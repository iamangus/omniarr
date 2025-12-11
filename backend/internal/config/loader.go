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

	if err := loadYaml(filepath.Join(configDir, "server.yaml"), &appConfig.Server); err != nil {
		// Optional for now, or default? Let's make it optional but log warning if needed.
		// Actually, let's just return error if it fails, assuming it should exist now.
		// Or better, if it doesn't exist, we can proceed with defaults (port 8080, no auth?).
		// But the user asked to implement it. Let's assume it must exist or at least try to load it.
		// Given I just created it, it should be fine.
		return nil, fmt.Errorf("failed to load server.yaml: %w", err)
	}

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