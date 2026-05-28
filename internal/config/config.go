package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultCodesomeBaseURL = "https://v3.codesome.cn"
	DefaultDatabasePath    = "codesome-manager.db"
)

type LoginCredentials struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

type CodesomeApiKeyId struct {
	ID   int    `yaml:"id"`
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type CodesomeConfig struct {
	BaseURL          string             `yaml:"base_url"`
	Login            *LoginCredentials  `yaml:"login"`
	DefaultGroupID   int                `yaml:"default_group_id"`
	ApiKeyIDs        []CodesomeApiKeyId `yaml:"api_key_ids"`
	LoginCredentials *LoginCredentials  `yaml:"login_credentials,omitempty"`
}

type legacyProviderConfig struct {
	Name             string             `yaml:"name"`
	BaseURL          string             `yaml:"base_url"`
	LoginCredentials *LoginCredentials  `yaml:"login_credentials"`
	ApiKeyIDs        []CodesomeApiKeyId `yaml:"api_key_ids"`
}

type Config struct {
	Codesome  *CodesomeConfig        `yaml:"codesome"`
	Database  DatabaseConfig         `yaml:"database"`
	Providers []legacyProviderConfig `yaml:"providers,omitempty"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// LoadConfig loads config.yaml from the current working directory.
func LoadConfig() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("config.yaml not found in current directory: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	return &cfg, nil
}

func (c *Config) DatabasePath() string {
	if c != nil && c.Database.Path != "" {
		return c.Database.Path
	}
	return DefaultDatabasePath
}

// GetCodesomeConfig returns the Codesome config, including legacy provider fallback.
func (c *Config) GetCodesomeConfig() *CodesomeConfig {
	if c == nil {
		return nil
	}
	if c.Codesome != nil {
		c.Codesome.normalize()
		return c.Codesome
	}
	for _, provider := range c.Providers {
		if provider.Name == "Codesome" {
			c.Codesome = &CodesomeConfig{
				BaseURL:   provider.BaseURL,
				Login:     provider.LoginCredentials,
				ApiKeyIDs: provider.ApiKeyIDs,
			}
			c.Codesome.normalize()
			return c.Codesome
		}
	}
	return nil
}

func (c *CodesomeConfig) normalize() {
	if c.BaseURL == "" {
		c.BaseURL = DefaultCodesomeBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.Login == nil && c.LoginCredentials != nil {
		c.Login = c.LoginCredentials
	}
}

// GetCodesomeLoginCredentials returns login credentials for Codesome
func (c *Config) GetCodesomeLoginCredentials() (*LoginCredentials, error) {
	codesome := c.GetCodesomeConfig()
	if codesome == nil {
		return nil, fmt.Errorf("codesome config not found in config.yaml")
	}
	if codesome.Login == nil {
		return nil, fmt.Errorf("codesome.login is missing in config.yaml")
	}
	return codesome.Login, nil
}

// ResolveCodesomeKeyID resolves a key alias to its numeric ID from api_key_ids config.
func (c *Config) ResolveCodesomeKeyID(key string) (int, error) {
	codesome := c.GetCodesomeConfig()
	if codesome == nil {
		return 0, fmt.Errorf("未找到 Codesome 配置")
	}
	for _, k := range codesome.ApiKeyIDs {
		if k.Key == key {
			return k.ID, nil
		}
	}
	available := make([]string, 0, len(codesome.ApiKeyIDs))
	for _, k := range codesome.ApiKeyIDs {
		if k.Key != "" {
			available = append(available, k.Key)
		}
	}
	return 0, fmt.Errorf("未找到 key=%q 对应的 API Key", key)
}
