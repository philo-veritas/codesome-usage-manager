package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultCodesomeBaseURL = "https://v3.codesome.cn"
	DefaultFeishuBaseURL   = "https://open.feishu.cn/open-apis"
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
	BaseURL        string             `yaml:"base_url"`
	Login          *LoginCredentials  `yaml:"login"`
	DefaultGroupID int                `yaml:"default_group_id"`
	ApiKeyIDs      []CodesomeApiKeyId `yaml:"api_key_ids"`
}

type Config struct {
	Codesome *CodesomeConfig `yaml:"codesome"`
	Feishu   *FeishuConfig   `yaml:"feishu"`
	Database DatabaseConfig  `yaml:"database"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type FeishuConfig struct {
	BaseURL   string        `yaml:"base_url"`
	AppID     string        `yaml:"app_id"`
	AppSecret string        `yaml:"app_secret"`
	Bitable   FeishuBitable `yaml:"bitable"`
}

type FeishuBitable struct {
	AppToken string           `yaml:"app_token"`
	Users    FeishuUsersTable `yaml:"users"`
	Usage    FeishuUsageTable `yaml:"usage"`
}

type FeishuUsersTable struct {
	TableID string           `yaml:"table_id"`
	ViewID  string           `yaml:"view_id"`
	Fields  FeishuUserFields `yaml:"fields"`
}

type FeishuUsageTable struct {
	TableID string `yaml:"table_id"`
	ViewID  string `yaml:"view_id"`
}

type FeishuUserFields struct {
	EmployeeNo string `yaml:"employee_no"`
	Name       string `yaml:"name"`
	Team       string `yaml:"team"`
	GroupID    string `yaml:"group_id"`
	Status     string `yaml:"status"`
	OpenID     string `yaml:"open_id"`
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

// GetCodesomeConfig returns the top-level Codesome config.
func (c *Config) GetCodesomeConfig() *CodesomeConfig {
	if c == nil {
		return nil
	}
	if c.Codesome != nil {
		c.Codesome.normalize()
		return c.Codesome
	}
	return nil
}

func (c *Config) GetFeishuConfig() *FeishuConfig {
	if c == nil || c.Feishu == nil {
		return nil
	}
	c.Feishu.normalize()
	return c.Feishu
}

func (c *CodesomeConfig) normalize() {
	if c.BaseURL == "" {
		c.BaseURL = DefaultCodesomeBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
}

func (c *FeishuConfig) normalize() {
	if c.BaseURL == "" {
		c.BaseURL = DefaultFeishuBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
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
