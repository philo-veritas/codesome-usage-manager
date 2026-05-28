package repository

import (
	"context"
	"testing"
)

func TestAPIKeyRepositoryCreateAndUpdateSynced(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	user, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo := NewAPIKeyRepository(userRepo.db)
	key, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        APIKeyStatusActive,
		GroupID:       51,
		RawKey:        "sk-test",
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if key.ID == 0 || key.RawKey == nil || *key.RawKey != "sk-test" || key.LastSyncedAt == nil {
		t.Fatalf("unexpected api key: %+v", key)
	}

	updated, err := repo.UpdateSynced(ctx, key.ID, UpdateAPIKeyParams{
		Name:    "Alice 2",
		Status:  APIKeyStatusInactive,
		GroupID: 60,
	})
	if err != nil {
		t.Fatalf("update api key: %v", err)
	}
	if updated.Name != "Alice 2" || updated.Status != APIKeyStatusInactive || updated.GroupID != 60 {
		t.Fatalf("unexpected updated api key: %+v", updated)
	}

	latest, err := repo.GetLatestByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get latest api key: %v", err)
	}
	if latest.ID != key.ID {
		t.Fatalf("expected latest key %d, got %d", key.ID, latest.ID)
	}
}

func TestAPIKeyRepositoryRejectsInvalidInput(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	repo := NewAPIKeyRepository(userRepo.db)
	ctx := context.Background()

	if _, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        999,
		CodesomeKeyID: 6732,
		Name:          "orphan",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	}); err == nil {
		t.Fatal("expected foreign key error")
	}

	if _, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        1,
		CodesomeKeyID: 0,
		Name:          "invalid",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	}); err == nil {
		t.Fatal("expected invalid codesome key id to fail")
	}
}

func TestAPIKeyRepositoryListExportRowsFiltersActiveByDefault(t *testing.T) {
	teamRepo, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	activeUser, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice", TeamCode: "platform"})
	if err != nil {
		t.Fatalf("create active user: %v", err)
	}
	inactiveUser, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E99999", Name: "Bob", TeamCode: "platform"})
	if err != nil {
		t.Fatalf("create inactive user: %v", err)
	}
	inactiveStatus := UserStatusInactive
	if _, err := userRepo.Update(ctx, "E99999", UpdateUserParams{Status: &inactiveStatus}); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	repo := NewAPIKeyRepository(userRepo.db)
	if _, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        activeUser.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        APIKeyStatusActive,
		GroupID:       51,
		RawKey:        "sk-active",
	}); err != nil {
		t.Fatalf("create active key: %v", err)
	}
	if _, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        inactiveUser.ID,
		CodesomeKeyID: 6733,
		Name:          "Bob",
		Status:        APIKeyStatusInactive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create inactive key: %v", err)
	}

	rows, err := repo.ListExportRows(ctx, ListAPIKeyExportRowsParams{TeamCode: "platform"})
	if err != nil {
		t.Fatalf("list export rows: %v", err)
	}
	if len(rows) != 1 || rows[0].EmployeeNo != "E12345" || rows[0].TeamCode == nil || *rows[0].TeamCode != "platform" {
		t.Fatalf("unexpected active export rows: %+v", rows)
	}
	if rows[0].RawKey == nil || *rows[0].RawKey != "sk-active" {
		t.Fatalf("unexpected raw key: %+v", rows[0].RawKey)
	}

	rows, err = repo.ListExportRows(ctx, ListAPIKeyExportRowsParams{TeamCode: "platform", IncludeInactive: true})
	if err != nil {
		t.Fatalf("list export rows with inactive: %v", err)
	}
	if len(rows) != 2 || rows[1].EmployeeNo != "E99999" || rows[1].RawKey != nil {
		t.Fatalf("unexpected include-inactive rows: %+v", rows)
	}
}

func TestAPIKeyRepositoryListExportRowsFiltersByEmployeeNo(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	alice, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E99999", Name: "Bob"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	repo := NewAPIKeyRepository(userRepo.db)
	for _, item := range []struct {
		userID int64
		keyID  int
		name   string
	}{
		{alice.ID, 6732, "Alice"},
		{bob.ID, 6733, "Bob"},
	} {
		if _, err := repo.Create(ctx, CreateAPIKeyParams{
			UserID:        item.userID,
			CodesomeKeyID: item.keyID,
			Name:          item.name,
			Status:        APIKeyStatusActive,
			GroupID:       51,
		}); err != nil {
			t.Fatalf("create api key: %v", err)
		}
	}

	rows, err := repo.ListExportRows(ctx, ListAPIKeyExportRowsParams{EmployeeNo: "E99999"})
	if err != nil {
		t.Fatalf("list export rows: %v", err)
	}
	if len(rows) != 1 || rows[0].EmployeeNo != "E99999" || rows[0].CodesomeKeyID != 6733 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestAPIKeyRepositoryListUsageTargetsSkipsDeletedUsers(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	activeUser, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create active user: %v", err)
	}
	deletedUser, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E99999", Name: "Bob"})
	if err != nil {
		t.Fatalf("create deleted user: %v", err)
	}

	repo := NewAPIKeyRepository(userRepo.db)
	if _, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        activeUser.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create active api key: %v", err)
	}
	if _, err := repo.Create(ctx, CreateAPIKeyParams{
		UserID:        deletedUser.ID,
		CodesomeKeyID: 6733,
		Name:          "Bob",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create deleted api key: %v", err)
	}
	if _, err := userRepo.SoftDelete(ctx, "E99999"); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	targets, err := repo.ListUsageTargets(ctx)
	if err != nil {
		t.Fatalf("list usage targets: %v", err)
	}
	if len(targets) != 1 || targets[0].CodesomeKeyID != 6732 {
		t.Fatalf("unexpected usage targets: %+v", targets)
	}
}
