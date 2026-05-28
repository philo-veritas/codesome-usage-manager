package repository

import (
	"context"
	"database/sql"
	"fmt"

	"codesome-usage-manager/internal/provider"
)

type UsageDaily struct {
	ID                int64
	APIKeyID          int64
	UsageDate         string
	TotalRequests     int64
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCacheTokens  int64
	TotalTokens       int64
	TotalCost         float64
	TotalActualCost   float64
	AverageDurationMS float64
	FetchedAt         string
}

type UsageDailyRepository struct {
	db *sql.DB
}

func NewUsageDailyRepository(db *sql.DB) *UsageDailyRepository {
	return &UsageDailyRepository{db: db}
}

func (r *UsageDailyRepository) Upsert(ctx context.Context, apiKeyID int64, usageDate string, stats provider.CodesomeUsageStats) (*UsageDaily, error) {
	if apiKeyID <= 0 {
		return nil, fmt.Errorf("api key id must be positive")
	}
	if usageDate == "" {
		return nil, fmt.Errorf("usage date is required")
	}

	fetchedAt := nowString()
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO usage_daily (
  api_key_id,
  usage_date,
  total_requests,
  total_input_tokens,
  total_output_tokens,
  total_cache_tokens,
  total_tokens,
  total_cost,
  total_actual_cost,
  average_duration_ms,
  fetched_at
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(api_key_id, usage_date) DO UPDATE SET
  total_requests = excluded.total_requests,
  total_input_tokens = excluded.total_input_tokens,
  total_output_tokens = excluded.total_output_tokens,
  total_cache_tokens = excluded.total_cache_tokens,
  total_tokens = excluded.total_tokens,
  total_cost = excluded.total_cost,
  total_actual_cost = excluded.total_actual_cost,
  average_duration_ms = excluded.average_duration_ms,
  fetched_at = excluded.fetched_at
`, apiKeyID, usageDate, stats.TotalRequests, stats.TotalInputTokens, stats.TotalOutputTokens, stats.TotalCacheTokens, stats.TotalTokens, stats.TotalCost, stats.TotalActualCost, stats.AverageDurationMS, fetchedAt); err != nil {
		return nil, fmt.Errorf("upsert usage daily: %w", err)
	}
	return r.Get(ctx, apiKeyID, usageDate)
}

func (r *UsageDailyRepository) Get(ctx context.Context, apiKeyID int64, usageDate string) (*UsageDaily, error) {
	var usage UsageDaily
	if err := r.db.QueryRowContext(ctx, `
SELECT
  id,
  api_key_id,
  usage_date,
  total_requests,
  total_input_tokens,
  total_output_tokens,
  total_cache_tokens,
  total_tokens,
  total_cost,
  total_actual_cost,
  average_duration_ms,
  fetched_at
FROM usage_daily
WHERE api_key_id = ? AND usage_date = ?
`, apiKeyID, usageDate).Scan(
		&usage.ID,
		&usage.APIKeyID,
		&usage.UsageDate,
		&usage.TotalRequests,
		&usage.TotalInputTokens,
		&usage.TotalOutputTokens,
		&usage.TotalCacheTokens,
		&usage.TotalTokens,
		&usage.TotalCost,
		&usage.TotalActualCost,
		&usage.AverageDurationMS,
		&usage.FetchedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("usage daily not found: %w", err)
		}
		return nil, fmt.Errorf("scan usage daily: %w", err)
	}
	return &usage, nil
}
