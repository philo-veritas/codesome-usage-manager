package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type LoginCredentials struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
}

type ApiKeyConfig struct {
	Name      string `yaml:"name"`
	ApiID     string `yaml:"api_id"`
	AuthToken string `yaml:"auth_token"`
}

type CodesomeApiKeyId struct {
	ID   int    `yaml:"id"`
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type ProviderConfig struct {
	Name             string            `yaml:"name"`
	BaseURL          string            `yaml:"base_url"`
	ApiKeyConfigs    []ApiKeyConfig    `yaml:"api_key_configs"`
	LoginCredentials *LoginCredentials `yaml:"login_credentials"`
	ApiKeyIDs        []CodesomeApiKeyId `yaml:"api_key_ids"`
}

type Config struct {
	Providers []ProviderConfig `yaml:"providers"`
}

// LoadConfig loads config.yaml from project root directory
func LoadConfig() (*Config, error) {
	// Get project root (parent of go/ directory)
	execPath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Try current directory first, then parent directory
	configPaths := []string{
		filepath.Join(execPath, "config.yaml"),
		filepath.Join(filepath.Dir(execPath), "config.yaml"),
	}

	var data []byte
	var configPath string
	for _, path := range configPaths {
		data, err = os.ReadFile(path)
		if err == nil {
			configPath = path
			break
		}
	}

	if configPath == "" {
		return nil, fmt.Errorf("config.yaml not found in expected locations")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	return &cfg, nil
}

// GetProviderByEnv returns provider config matching ANTHROPIC_BASE_URL
func (c *Config) GetProviderByEnv() (*ProviderConfig, error) {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("ANTHROPIC_BASE_URL environment variable not set")
	}

	for i := range c.Providers {
		if c.Providers[i].BaseURL == baseURL {
			return &c.Providers[i], nil
		}
	}

	return nil, fmt.Errorf("no provider found matching ANTHROPIC_BASE_URL: %s", baseURL)
}

// GetApiIDByEnv returns API ID for the current environment
func (c *Config) GetApiIDByEnv() (string, error) {
	provider, err := c.GetProviderByEnv()
	if err != nil {
		return "", err
	}

	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if authToken == "" {
		return "", fmt.Errorf("ANTHROPIC_AUTH_TOKEN environment variable not set")
	}

	for _, apiKeyConfig := range provider.ApiKeyConfigs {
		if apiKeyConfig.AuthToken == authToken {
			return apiKeyConfig.ApiID, nil
		}
	}

	return "", fmt.Errorf("no API config found matching ANTHROPIC_AUTH_TOKEN")
}

// IsClaudeBuddyProvider checks if current provider is Claude Buddy
func (c *Config) IsClaudeBuddyProvider() (bool, error) {
	provider, err := c.GetProviderByEnv()
	if err != nil {
		return false, err
	}
	return provider.Name == "Claude Buddy", nil
}

// GetApiIDByAuthToken returns API ID for the given auth token (Claude Buddy only)
func (c *Config) GetApiIDByAuthToken(authToken string) (string, error) {
	for _, provider := range c.Providers {
		if provider.Name == "Claude Buddy" {
			for _, apiKeyConfig := range provider.ApiKeyConfigs {
				if apiKeyConfig.AuthToken == authToken {
					return apiKeyConfig.ApiID, nil
				}
			}
		}
	}
	return "", fmt.Errorf("未找到匹配的 API 配置")
}

// GetAllClaudeBuddyAccounts returns all Claude Buddy accounts from config
func (c *Config) GetAllClaudeBuddyAccounts() []ApiKeyConfig {
	var accounts []ApiKeyConfig
	for _, provider := range c.Providers {
		if provider.Name == "Claude Buddy" {
			accounts = append(accounts, provider.ApiKeyConfigs...)
		}
	}
	return accounts
}

// GetCodesomeConfig returns the Codesome provider config, or nil if not found
func (c *Config) GetCodesomeConfig() *ProviderConfig {
	for i := range c.Providers {
		if c.Providers[i].Name == "Codesome" {
			return &c.Providers[i]
		}
	}
	return nil
}

// GetCodesomeLoginCredentials returns login credentials for Codesome
func (c *Config) GetCodesomeLoginCredentials() (*LoginCredentials, error) {
	provider := c.GetCodesomeConfig()
	if provider == nil {
		return nil, fmt.Errorf("Codesome provider not found in config.yaml")
	}
	if provider.LoginCredentials == nil {
		return nil, fmt.Errorf("Codesome login_credentials is missing in config.yaml")
	}
	return provider.LoginCredentials, nil
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
