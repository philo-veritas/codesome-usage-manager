package syncer

import (
	"context"
	"testing"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestRemoteKeyImporterDryRunDoesNotWrite(t *testing.T) {
	database := newTestDatabase(t)
	keys := testRemoteKeys()

	results, err := NewRemoteKeyImporter(database).Import(context.Background(), keys, ImportRemoteKeysOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run import remote keys: %v", err)
	}
	if len(results) != 2 || results[0].Action != "create" || results[0].EmployeeNo != "codesome-key:6732" {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
	if _, err := repository.NewUserRepository(database).GetByEmployeeNo(context.Background(), "codesome-key:6732"); err == nil {
		t.Fatal("expected dry-run to avoid writing user")
	}
}

func TestRemoteKeyImporterCreatesVirtualUsersAndKeys(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()

	results, err := NewRemoteKeyImporter(database).Import(ctx, testRemoteKeys(), ImportRemoteKeysOptions{})
	if err != nil {
		t.Fatalf("import remote keys: %v", err)
	}
	if len(results) != 2 || results[0].Action != "create" || results[1].Action != "create" {
		t.Fatalf("unexpected import results: %+v", results)
	}

	user, err := repository.NewUserRepository(database).GetByEmployeeNo(ctx, "codesome-key:6732")
	if err != nil {
		t.Fatalf("get virtual user: %v", err)
	}
	key, err := repository.NewAPIKeyRepository(database).GetByCodesomeKeyID(ctx, 6732)
	if err != nil {
		t.Fatalf("get imported key: %v", err)
	}
	if key.UserID != user.ID || key.Name != "Main Key" || key.GroupID != 51 || key.Status != repository.APIKeyStatusActive || key.RawKey != nil {
		t.Fatalf("unexpected imported key: %+v user=%+v", key, user)
	}
	if user.CodesomeGroupID == nil || *user.CodesomeGroupID != 51 {
		t.Fatalf("expected virtual user group 51, got %+v", user)
	}

	results, err = NewRemoteKeyImporter(database).Import(ctx, testRemoteKeys(), ImportRemoteKeysOptions{})
	if err != nil {
		t.Fatalf("second import remote keys: %v", err)
	}
	if len(results) != 2 || results[0].Action != "skip" || results[1].Action != "skip" {
		t.Fatalf("expected idempotent skips, got %+v", results)
	}
}

func TestRemoteKeyImporterUsesEmbeddedGroupID(t *testing.T) {
	database := newTestDatabase(t)
	keys := []provider.CodesomeApiKey{
		{ID: 6732, Name: "Main Key", Status: "active", Group: &provider.CodesomeGroup{ID: 51, Name: "default"}},
	}

	results, err := NewRemoteKeyImporter(database).Import(context.Background(), keys, ImportRemoteKeysOptions{})
	if err != nil {
		t.Fatalf("import remote keys with embedded group: %v", err)
	}
	if len(results) != 1 || results[0].GroupID != 51 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRemoteKeyImporterValidatesRemoteKey(t *testing.T) {
	database := newTestDatabase(t)

	if _, err := NewRemoteKeyImporter(database).Import(context.Background(), []provider.CodesomeApiKey{{ID: 6732, Name: "bad", Status: "disabled", GroupID: 51}}, ImportRemoteKeysOptions{}); err == nil {
		t.Fatal("expected unsupported status to fail")
	}
	if _, err := NewRemoteKeyImporter(database).Import(context.Background(), []provider.CodesomeApiKey{{ID: 6732, Name: "missing group", Status: "active"}}, ImportRemoteKeysOptions{}); err == nil {
		t.Fatal("expected missing group to fail")
	}
}

func testRemoteKeys() []provider.CodesomeApiKey {
	return []provider.CodesomeApiKey{
		{ID: 6732, Name: "Main Key", GroupID: 51, Status: "active"},
		{ID: 6733, Name: "Spare Key", GroupID: 60, Status: "inactive"},
	}
}
