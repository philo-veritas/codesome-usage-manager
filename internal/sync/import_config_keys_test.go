package syncer

import (
	"context"
	"testing"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/repository"
)

func TestConfigKeyImporterDryRunDoesNotWrite(t *testing.T) {
	database := newTestDatabase(t)
	cfg := testImportConfig(51)

	results, err := NewConfigKeyImporter(database).Import(context.Background(), cfg, ImportConfigKeysOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	if len(results) != 2 || results[0].Action != "create" || results[0].EmployeeNo != "legacy:main" {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
	if _, err := repository.NewUserRepository(database).GetByEmployeeNo(context.Background(), "legacy:main"); err == nil {
		t.Fatal("expected dry-run to avoid writing user")
	}
}

func TestConfigKeyImporterCreatesLegacyUsersAndKeys(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	cfg := testImportConfig(51)

	results, err := NewConfigKeyImporter(database).Import(ctx, cfg, ImportConfigKeysOptions{})
	if err != nil {
		t.Fatalf("import config keys: %v", err)
	}
	if len(results) != 2 || results[0].Action != "create" || results[1].Action != "create" {
		t.Fatalf("unexpected import results: %+v", results)
	}

	user, err := repository.NewUserRepository(database).GetByEmployeeNo(ctx, "legacy:main")
	if err != nil {
		t.Fatalf("get legacy user: %v", err)
	}
	key, err := repository.NewAPIKeyRepository(database).GetByCodesomeKeyID(ctx, 6732)
	if err != nil {
		t.Fatalf("get imported key: %v", err)
	}
	if key.UserID != user.ID || key.Name != "Main Key" || key.GroupID != 51 || key.RawKey != nil {
		t.Fatalf("unexpected imported key: %+v user=%+v", key, user)
	}

	results, err = NewConfigKeyImporter(database).Import(ctx, cfg, ImportConfigKeysOptions{})
	if err != nil {
		t.Fatalf("second import config keys: %v", err)
	}
	if len(results) != 2 || results[0].Action != "skip" || results[1].Action != "skip" {
		t.Fatalf("expected idempotent skips, got %+v", results)
	}
}

func TestConfigKeyImporterRequiresGroupID(t *testing.T) {
	database := newTestDatabase(t)
	cfg := testImportConfig(0)

	if _, err := NewConfigKeyImporter(database).Import(context.Background(), cfg, ImportConfigKeysOptions{}); err == nil {
		t.Fatal("expected missing group id to fail")
	}
}

func TestConfigKeyImporterAllowsGroupIDOverride(t *testing.T) {
	database := newTestDatabase(t)
	cfg := testImportConfig(0)

	results, err := NewConfigKeyImporter(database).Import(context.Background(), cfg, ImportConfigKeysOptions{GroupID: 60})
	if err != nil {
		t.Fatalf("import with override group: %v", err)
	}
	if len(results) != 2 || results[0].GroupID != 60 {
		t.Fatalf("unexpected override results: %+v", results)
	}
}

func testImportConfig(defaultGroupID int) *config.Config {
	return &config.Config{
		Codesome: &config.CodesomeConfig{
			DefaultGroupID: defaultGroupID,
			ApiKeyIDs: []config.CodesomeApiKeyId{
				{ID: 6732, Key: "main", Name: "Main Key"},
				{ID: 6733, Name: "Legacy 6733"},
			},
		},
	}
}
