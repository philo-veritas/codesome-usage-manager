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
