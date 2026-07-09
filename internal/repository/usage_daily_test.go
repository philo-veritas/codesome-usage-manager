package repository

import (
	"context"
	"strconv"
	"testing"

	codesomedb "codesome-usage-manager/internal/db"
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
	account := ensureCodesomeUsageAccount(t, userRepo, key)

	repo := NewUsageDailyRepository(userRepo.db)
	first, err := repo.Upsert(ctx, account.ID, "2026-05-26", provider.CodesomeUsageStats{
		TotalRequests:   10,
		TotalTokens:     100,
		TotalActualCost: 1.25,
	})
	if err != nil {
		t.Fatalf("upsert first usage: %v", err)
	}
	second, err := repo.Upsert(ctx, account.ID, "2026-05-26", provider.CodesomeUsageStats{
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
	aliceAccount := ensureCodesomeUsageAccount(t, userRepo, aliceKey)
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
	bobAccount := ensureCodesomeUsageAccount(t, userRepo, bobKey)

	usageRepo := NewUsageDailyRepository(userRepo.db)
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-05-01", provider.CodesomeUsageStats{TotalRequests: 10, TotalTokens: 100, TotalActualCost: 1.25}); err != nil {
		t.Fatalf("upsert alice usage 1: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-05-31", provider.CodesomeUsageStats{TotalRequests: 20, TotalTokens: 200, TotalActualCost: 2.5}); err != nil {
		t.Fatalf("upsert alice usage 2: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-06-01", provider.CodesomeUsageStats{TotalRequests: 999, TotalTokens: 999, TotalActualCost: 999}); err != nil {
		t.Fatalf("upsert outside usage: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, bobAccount.ID, "2026-05-15", provider.CodesomeUsageStats{TotalRequests: 5, TotalTokens: 50, TotalActualCost: 0.75}); err != nil {
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
	account := ensureCodesomeUsageAccount(t, userRepo, key)
	usageRepo := NewUsageDailyRepository(userRepo.db)
	if _, err := usageRepo.Upsert(ctx, account.ID, "2026-05-01", provider.CodesomeUsageStats{TotalRequests: 10}); err != nil {
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

func TestUsageDailyRepositoryRecentDailyActualCosts(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	alice, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E54321", Name: "Bob"})
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
	aliceAccount := ensureCodesomeUsageAccount(t, userRepo, aliceKey)
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
	bobAccount := ensureCodesomeUsageAccount(t, userRepo, bobKey)

	usageRepo := NewUsageDailyRepository(userRepo.db)
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-05-09", provider.CodesomeUsageStats{TotalActualCost: 99}); err != nil {
		t.Fatalf("upsert outside usage: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-05-10", provider.CodesomeUsageStats{TotalActualCost: 1.25}); err != nil {
		t.Fatalf("upsert alice usage: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, bobAccount.ID, "2026-05-10", provider.CodesomeUsageStats{TotalActualCost: 2.75}); err != nil {
		t.Fatalf("upsert bob usage: %v", err)
	}
	codexAccount, err := NewUsageAccountRepository(userRepo.db).EnsureCodexAccount(ctx, alice.ID, alice.EmployeeNo, "")
	if err != nil {
		t.Fatalf("ensure codex account: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, codexAccount.ID, "2026-05-10", provider.CodesomeUsageStats{TotalActualCost: 100}); err != nil {
		t.Fatalf("upsert codex usage: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-05-11", provider.CodesomeUsageStats{TotalActualCost: 5}); err != nil {
		t.Fatalf("upsert next usage: %v", err)
	}
	if _, err := usageRepo.Upsert(ctx, aliceAccount.ID, "2026-05-12", provider.CodesomeUsageStats{TotalActualCost: 100}); err != nil {
		t.Fatalf("upsert before-date usage: %v", err)
	}

	rows, err := usageRepo.RecentDailyActualCosts(ctx, "2026-05-12", 2)
	if err != nil {
		t.Fatalf("recent daily actual costs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %+v", rows)
	}
	if rows[0].UsageDate != "2026-05-10" || rows[0].TotalActualCost != 4 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].UsageDate != "2026-05-11" || rows[1].TotalActualCost != 5 {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestUsageDailyRepositoryRecentDailyActualCostsSupportsLegacySchema(t *testing.T) {
	database, err := codesomedb.Open(t.TempDir() + "/legacy.db")
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(`
	CREATE TABLE usage_daily (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  api_key_id INTEGER NOT NULL,
	  usage_date TEXT NOT NULL,
	  total_actual_cost REAL NOT NULL DEFAULT 0
	);
	INSERT INTO usage_daily (api_key_id, usage_date, total_actual_cost)
	VALUES (1, '2026-05-10', 1.25), (2, '2026-05-10', 2.75), (1, '2026-05-11', 5);
	`); err != nil {
		t.Fatalf("seed legacy usage: %v", err)
	}

	rows, err := NewUsageDailyRepository(database).RecentDailyActualCosts(context.Background(), "2026-05-12", 2)
	if err != nil {
		t.Fatalf("recent daily actual costs: %v", err)
	}
	if len(rows) != 2 || rows[0].UsageDate != "2026-05-10" || rows[0].TotalActualCost != 4 || rows[1].UsageDate != "2026-05-11" || rows[1].TotalActualCost != 5 {
		t.Fatalf("unexpected legacy rows: %+v", rows)
	}
}

func ensureCodesomeUsageAccount(t *testing.T, userRepo *UserRepository, key *APIKey) *UsageAccount {
	t.Helper()

	accountRepo := NewUsageAccountRepository(userRepo.db)
	if err := accountRepo.EnsureCodesomeAccounts(context.Background()); err != nil {
		t.Fatalf("ensure codesome usage accounts: %v", err)
	}
	account, err := accountRepo.FindBySource(context.Background(), UsageSourceCodesome, strconv.Itoa(key.CodesomeKeyID))
	if err != nil {
		t.Fatalf("find codesome usage account: %v", err)
	}
	if account == nil {
		t.Fatalf("expected usage account for key %d", key.CodesomeKeyID)
	}
	return account
}
