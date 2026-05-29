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

func TestUsageDailyRepositoryMonthlyReport(t *testing.T) {
	teamRepo, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create platform team: %v", err)
	}
	if _, err := teamRepo.Create(ctx, "infra", "Infra"); err != nil {
		t.Fatalf("create infra team: %v", err)
	}
	alice, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice", TeamCode: "platform"})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E99999", Name: "Bob", TeamCode: "infra"})
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	keyRepo := NewAPIKeyRepository(userRepo.db)
	aliceKey, err := keyRepo.Create(ctx, CreateAPIKeyParams{
		UserID:        alice.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create alice key: %v", err)
	}
	bobKey, err := keyRepo.Create(ctx, CreateAPIKeyParams{
		UserID:        bob.ID,
		CodesomeKeyID: 6733,
		Name:          "Bob",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create bob key: %v", err)
	}

	usageRepo := NewUsageDailyRepository(userRepo.db)
	if _, err := usageRepo.Upsert(ctx, aliceKey.ID, "2026-05-01", provider.CodesomeUsageStats{TotalRequests: 10, TotalTokens: 100, TotalActualCost: 1.25}); err != nil {
		t.Fatalf("upsert alice usage 1: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceKey.ID, "2026-05-31", provider.CodesomeUsageStats{TotalRequests: 20, TotalTokens: 200, TotalActualCost: 2.5}); err != nil {
		t.Fatalf("upsert alice usage 2: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceKey.ID, "2026-06-01", provider.CodesomeUsageStats{TotalRequests: 999, TotalTokens: 999, TotalActualCost: 999}); err != nil {
		t.Fatalf("upsert outside usage: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, bobKey.ID, "2026-05-15", provider.CodesomeUsageStats{TotalRequests: 5, TotalTokens: 50, TotalActualCost: 0.75}); err != nil {
		t.Fatalf("upsert bob usage: %v", err)
	}

	rows, err := usageRepo.MonthlyReport(ctx, "2026-05", "")
	if err != nil {
		t.Fatalf("monthly report: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %+v", rows)
	}
	if rows[0].EmployeeNo != "E99999" || rows[1].EmployeeNo != "E12345" {
		t.Fatalf("unexpected order: %+v", rows)
	}
	if rows[1].TotalRequests != 30 || rows[1].TotalTokens != 300 || rows[1].TotalActualCost != 3.75 {
		t.Fatalf("unexpected alice aggregate: %+v", rows[1])
	}

	rows, err = usageRepo.MonthlyReport(ctx, "2026-05", "platform")
	if err != nil {
		t.Fatalf("monthly report by team: %v", err)
	}
	if len(rows) != 1 || rows[0].EmployeeNo != "E12345" {
		t.Fatalf("unexpected platform rows: %+v", rows)
	}
}

func TestUsageDailyRepositoryMonthlyReportKeepsDeletedUserHistory(t *testing.T) {
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
	usageRepo := NewUsageDailyRepository(userRepo.db)
	if _, err := usageRepo.Upsert(ctx, key.ID, "2026-05-01", provider.CodesomeUsageStats{TotalRequests: 10}); err != nil {
		t.Fatalf("upsert usage: %v", err)
	}
	if _, err := userRepo.SoftDelete(ctx, "E12345"); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	rows, err := usageRepo.MonthlyReport(ctx, "2026-05", "")
	if err != nil {
		t.Fatalf("monthly report: %v", err)
	}
	if len(rows) != 1 || rows[0].EmployeeNo != "E12345" || rows[0].TotalRequests != 10 {
		t.Fatalf("expected deleted user history, got %+v", rows)
	}
}
