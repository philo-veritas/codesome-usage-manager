package syncer

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestSyncUsageFetchesAndStoresDailyStats(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice", FeishuOpenID: "ou_alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	service := &fakeUsageStatsService{
		stats: &provider.CodesomeUsageStats{
			TotalRequests:   10,
			TotalTokens:     100,
			TotalActualCost: 1.5,
		},
	}

	results, err := NewUsageSyncer(database, service).SyncUsage(ctx, UsageSyncOptions{
		Dates:       []string{"2026-05-26", "2026-05-27"},
		ForceUpdate: true,
	})
	if err != nil {
		t.Fatalf("sync usage: %v", err)
	}
	if len(results) != 2 || len(service.calls) != 2 {
		t.Fatalf("unexpected results=%+v calls=%+v", results, service.calls)
	}
	if results[0].FeishuOpenID != "ou_alice" {
		t.Fatalf("expected feishu open id in usage result, got %+v", results[0])
	}
	if service.calls[0].keyID != 6732 || service.calls[0].usageDate != "2026-05-26" || !service.calls[0].forceUpdate {
		t.Fatalf("unexpected first call: %+v", service.calls[0])
	}
	account := ensureTestCodesomeUsageAccount(t, database, key)
	stored, err := repository.NewUsageDailyRepository(database).Get(ctx, account.ID, "2026-05-27")
	if err != nil {
		t.Fatalf("get stored usage: %v", err)
	}
	if stored.TotalRequests != 10 || stored.TotalTokens != 100 || stored.TotalActualCost != 1.5 {
		t.Fatalf("unexpected stored usage: %+v", stored)
	}
}

func TestSyncUsageReusesExistingDailyStats(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice", FeishuOpenID: "ou_alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	account := ensureTestCodesomeUsageAccount(t, database, key)
	if _, err := repository.NewUsageDailyRepository(database).Upsert(ctx, account.ID, "2026-06-11", provider.CodesomeUsageStats{
		TotalRequests:   10,
		TotalTokens:     100,
		TotalActualCost: 1.5,
	}); err != nil {
		t.Fatalf("upsert existing usage: %v", err)
	}
	service := &fakeUsageStatsService{
		stats: &provider.CodesomeUsageStats{
			TotalRequests:   999,
			TotalTokens:     999,
			TotalActualCost: 9.99,
		},
	}

	results, err := NewUsageSyncer(database, service).SyncUsage(ctx, UsageSyncOptions{
		Dates:         []string{"2026-06-11"},
		ReuseExisting: true,
	})
	if err != nil {
		t.Fatalf("sync usage: %v", err)
	}
	if len(service.calls) != 0 {
		t.Fatalf("expected existing usage to skip remote calls, got %+v", service.calls)
	}
	if len(results) != 1 || results[0].TotalTokens != 100 || results[0].ActualCost != 1.5 || results[0].FeishuOpenID != "ou_alice" {
		t.Fatalf("unexpected reused result: %+v", results)
	}
}

func TestSyncUsageForceUpdateDateBypassesExistingDailyStats(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	account := ensureTestCodesomeUsageAccount(t, database, key)
	if _, err := repository.NewUsageDailyRepository(database).Upsert(ctx, account.ID, "2026-06-12", provider.CodesomeUsageStats{
		TotalTokens:     100,
		TotalActualCost: 1.5,
	}); err != nil {
		t.Fatalf("upsert existing usage: %v", err)
	}
	service := &fakeUsageStatsService{
		stats: &provider.CodesomeUsageStats{
			TotalTokens:     200,
			TotalActualCost: 2.5,
		},
	}

	results, err := NewUsageSyncer(database, service).SyncUsage(ctx, UsageSyncOptions{
		Dates:            []string{"2026-06-12"},
		ReuseExisting:    true,
		ForceUpdateDates: map[string]bool{"2026-06-12": true},
	})
	if err != nil {
		t.Fatalf("sync usage: %v", err)
	}
	if len(service.calls) != 1 || !service.calls[0].forceUpdate {
		t.Fatalf("expected forced remote call, got %+v", service.calls)
	}
	if len(results) != 1 || results[0].TotalTokens != 200 || results[0].ActualCost != 2.5 {
		t.Fatalf("unexpected refreshed result: %+v", results)
	}
}

type fakeUsageStatsService struct {
	stats *provider.CodesomeUsageStats
	calls []fakeUsageStatsCall
}

type fakeUsageStatsCall struct {
	keyID       int
	usageDate   string
	forceUpdate bool
}

func (s *fakeUsageStatsService) GetUsageStats(ctx context.Context, keyID int, usageDate string, forceUpdate bool) (*provider.CodesomeUsageStats, error) {
	s.calls = append(s.calls, fakeUsageStatsCall{keyID: keyID, usageDate: usageDate, forceUpdate: forceUpdate})
	return s.stats, nil
}

func ensureTestCodesomeUsageAccount(t *testing.T, database *sql.DB, key *repository.APIKey) *repository.UsageAccount {
	t.Helper()

	accountRepo := repository.NewUsageAccountRepository(database)
	if err := accountRepo.EnsureCodesomeAccounts(context.Background()); err != nil {
		t.Fatalf("ensure codesome usage accounts: %v", err)
	}
	account, err := accountRepo.FindBySource(context.Background(), repository.UsageSourceCodesome, strconv.Itoa(key.CodesomeKeyID))
	if err != nil {
		t.Fatalf("find codesome usage account: %v", err)
	}
	if account == nil {
		t.Fatalf("expected usage account for key %d", key.CodesomeKeyID)
	}
	return account
}
