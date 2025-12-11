package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Auth     AuthConfig      `yaml:"auth"`
	Backends []BackendConfig `yaml:"backends"`
}

type ServerConfig struct {
	Port     int    `yaml:"port"`
	BaseURL  string `yaml:"base_url"`
	LogLevel string `yaml:"log_level"`
}

type AuthConfig struct {
	ProviderURL  string `yaml:"provider_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
	AdminRole    string `yaml:"admin_role"`
}

type BackendConfig struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
	Icon   string `yaml:"icon"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}