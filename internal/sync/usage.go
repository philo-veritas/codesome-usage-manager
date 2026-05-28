package syncer

import (
	"context"
	"database/sql"
	"fmt"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

type UsageStatsService interface {
	GetUsageStats(ctx context.Context, keyID int, usageDate string, forceUpdate bool) (*provider.CodesomeUsageStats, error)
}

type UsageSyncer struct {
	keys  *repository.APIKeyRepository
	usage *repository.UsageDailyRepository
	stats UsageStatsService
}

type UsageSyncOptions struct {
	Dates       []string
	ForceUpdate bool
}

type UsageSyncResult struct {
	UsageDate     string
	LocalAPIKeyID int64
	CodesomeKeyID int
	KeyName       string
	TotalRequests int64
	TotalTokens   int64
	ActualCost    float64
}

func NewUsageSyncer(database *sql.DB, stats UsageStatsService) *UsageSyncer {
	return &UsageSyncer{
		keys:  repository.NewAPIKeyRepository(database),
		usage: repository.NewUsageDailyRepository(database),
		stats: stats,
	}
}

func (s *UsageSyncer) SyncUsage(ctx context.Context, options UsageSyncOptions) ([]UsageSyncResult, error) {
	if len(options.Dates) == 0 {
		return nil, fmt.Errorf("usage dates are required")
	}
	if s.stats == nil {
		return nil, fmt.Errorf("usage stats service is nil")
	}

	targets, err := s.keys.ListUsageTargets(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]UsageSyncResult, 0, len(targets)*len(options.Dates))
	for _, target := range targets {
		for _, date := range options.Dates {
			stats, err := s.stats.GetUsageStats(ctx, target.CodesomeKeyID, date, options.ForceUpdate)
			if err != nil {
				return nil, fmt.Errorf("sync usage key %d date %s: %w", target.CodesomeKeyID, date, err)
			}
			if stats == nil {
				return nil, fmt.Errorf("sync usage key %d date %s returned nil stats", target.CodesomeKeyID, date)
			}
			stored, err := s.usage.Upsert(ctx, target.ID, date, *stats)
			if err != nil {
				return nil, err
			}
			results = append(results, UsageSyncResult{
				UsageDate:     date,
				LocalAPIKeyID: target.ID,
				CodesomeKeyID: target.CodesomeKeyID,
				KeyName:       target.Name,
				TotalRequests: stored.TotalRequests,
				TotalTokens:   stored.TotalTokens,
				ActualCost:    stored.TotalActualCost,
			})
		}
	}
	return results, nil
}
