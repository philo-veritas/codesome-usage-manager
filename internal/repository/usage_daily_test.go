package repository

import (
	"context"
	"testing"

	"codesome-usage-manager/internal/provider"
)

func TestUsageDailyRepositoryUpsertIsIdempotent(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	user, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := NewAPIKeyRepository(userRepo.db).Create(ctx, CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	repo := NewUsageDailyRepository(userRepo.db)
	first, err := repo.Upsert(ctx, key.ID, "2026-05-26", provider.CodesomeUsageStats{
		TotalRequests:   10,
		TotalTokens:     100,
		TotalActualCost: 1.25,
	})
	if err != nil {
		t.Fatalf("upsert first usage: %v", err)
	}
	second, err := repo.Upsert(ctx, key.ID, "2026-05-26", provider.CodesomeUsageStats{
		TotalRequests:     20,
		TotalInputTokens:  30,
		TotalOutputTokens: 40,
		TotalCacheTokens:  50,
		TotalTokens:       120,
		TotalCost:         2.5,
		TotalActualCost:   2.25,
		AverageDurationMS: 345.6,
	})
	if err != nil {
		t.Fatalf("upsert second usage: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same row id, got first=%d second=%d", first.ID, second.ID)
	}
	if second.TotalRequests != 20 || second.TotalTokens != 120 || second.TotalActualCost != 2.25 || second.AverageDurationMS != 345.6 {
		t.Fatalf("unexpected updated usage: %+v", second)
	}
}
