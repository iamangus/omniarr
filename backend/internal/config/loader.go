package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig reads the configuration file from the specified path
func LoadConfig(configPath string) (*AppConfig, error) {
	appConfig := &AppConfig{}

	if err := loadYaml(configPath, appConfig); err != nil {
		return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
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