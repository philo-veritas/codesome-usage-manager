package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetCodesomeConfigReadsTopLevelCodesome(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(`
codesome:
  base_url: "https://example.test/"
  login:
    email: "user@example.test"
    password: "secret"
  default_group_id: 51
  api_key_ids:
    - id: 6732
      name: "main"
      key: "main"
`), &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	codesome := cfg.GetCodesomeConfig()
	if codesome == nil {
		t.Fatal("expected Codesome config")
	}
	if codesome.BaseURL != "https://example.test" {
		t.Fatalf("unexpected base URL: %s", codesome.BaseURL)
	}
	if codesome.Login == nil || codesome.Login.Email != "user@example.test" {
		t.Fatalf("unexpected login config: %+v", codesome.Login)
	}
	if codesome.DefaultGroupID != 51 {
		t.Fatalf("unexpected default group id: %d", codesome.DefaultGroupID)
	}
	if len(codesome.ApiKeyIDs) != 1 || codesome.ApiKeyIDs[0].Key != "main" {
		t.Fatalf("unexpected api_key_ids: %+v", codesome.ApiKeyIDs)
	}
}

func TestGetCodesomeConfigIgnoresLegacyProviders(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(`
providers:
  - name: "Codesome"
    base_url: "https://legacy.example.test"
    login_credentials:
      email: "legacy@example.test"
      password: "secret"
    api_key_ids:
      - id: 6732
        name: "main"
        key: "main"
`), &cfg); err != nil {
		t.Fatalf("failed to parse legacy config: %v", err)
	}

	codesome := cfg.GetCodesomeConfig()
	if codesome != nil {
		t.Fatalf("expected legacy providers to be ignored, got %+v", codesome)
	}
}

func TestDatabasePath(t *testing.T) {
	cfg := &Config{}
	if got := cfg.DatabasePath(); got != DefaultDatabasePath {
		t.Fatalf("expected default path %q, got %q", DefaultDatabasePath, got)
	}

	cfg.Database.Path = "/tmp/custom.db"
	if got := cfg.DatabasePath(); got != "/tmp/custom.db" {
		t.Fatalf("expected custom path, got %q", got)
	}
}

func TestGetFeishuConfigReadsBitableLocation(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(`
feishu:
  base_url: "https://open.feishu.cn/open-apis/"
  app_id: "cli_test"
  app_secret: "secret"
  bitable:
    app_token: "app_token"
    users:
      table_id: "tbl1"
`), &cfg); err != nil {
		t.Fatalf("failed to parse feishu config: %v", err)
	}

	feishu := cfg.GetFeishuConfig()
	if feishu == nil {
		t.Fatal("expected Feishu config")
	}
	if feishu.BaseURL != "https://open.feishu.cn/open-apis" {
		t.Fatalf("unexpected base URL: %s", feishu.BaseURL)
	}
	if feishu.Bitable.AppToken != "app_token" || feishu.Bitable.Users.TableID != "tbl1" {
		t.Fatalf("unexpected bitable location: %+v", feishu.Bitable)
	}
}
