package syncer

import (
	"context"
	"testing"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestSyncUsageFetchesAndStoresDailyStats(t *testing.T) {
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
	if service.calls[0].keyID != 6732 || service.calls[0].usageDate != "2026-05-26" || !service.calls[0].forceUpdate {
		t.Fatalf("unexpected first call: %+v", service.calls[0])
	}
	stored, err := repository.NewUsageDailyRepository(database).Get(ctx, key.ID, "2026-05-27")
	if err != nil {
		t.Fatalf("get stored usage: %v", err)
	}
	if stored.TotalRequests != 10 || stored.TotalTokens != 100 || stored.TotalActualCost != 1.5 {
		t.Fatalf("unexpected stored usage: %+v", stored)
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
