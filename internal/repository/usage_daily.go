package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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

type MonthlyReportRow struct {
	Month           string
	TeamCode        *string
	UserName        string
	EmployeeNo      string
	TotalRequests   int64
	TotalTokens     int64
	TotalActualCost float64
}

type DailyActualCost struct {
	UsageDate       string
	TotalActualCost float64
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
	usage, err := r.Find(ctx, apiKeyID, usageDate)
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, fmt.Errorf("usage daily not found: %w", sql.ErrNoRows)
	}
	return usage, nil
}

func (r *UsageDailyRepository) Find(ctx context.Context, apiKeyID int64, usageDate string) (*UsageDaily, error) {
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
			return nil, nil
		}
		return nil, fmt.Errorf("scan usage daily: %w", err)
	}
	return &usage, nil
}

func (r *UsageDailyRepository) MonthlyReport(ctx context.Context, month string, teamCode string) ([]MonthlyReportRow, error) {
	if month == "" {
		return nil, fmt.Errorf("month is required")
	}
	startDate := month + "-01"
	endDate, err := nextMonthStart(month)
	if err != nil {
		return nil, err
	}

	query := `
SELECT
  ? AS month,
  teams.code,
  users.name,
  users.employee_no,
  COALESCE(SUM(usage_daily.total_requests), 0),
  COALESCE(SUM(usage_daily.total_tokens), 0),
  COALESCE(SUM(usage_daily.total_actual_cost), 0)
FROM usage_daily
JOIN api_keys ON usage_daily.api_key_id = api_keys.id
JOIN users ON api_keys.user_id = users.id
LEFT JOIN teams ON users.team_id = teams.id
WHERE usage_daily.usage_date >= ?
  AND usage_daily.usage_date < ?`
	args := []any{month, startDate, endDate}
	if teamCode != "" {
		query += `
  AND teams.code = ?`
		args = append(args, teamCode)
	}
	query += `
GROUP BY teams.code, users.id, users.name, users.employee_no
ORDER BY teams.code, users.employee_no`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query monthly report: %w", err)
	}
	defer rows.Close()

	var result []MonthlyReportRow
	for rows.Next() {
		var row MonthlyReportRow
		var teamCode sql.NullString
		if err := rows.Scan(
			&row.Month,
			&teamCode,
			&row.UserName,
			&row.EmployeeNo,
			&row.TotalRequests,
			&row.TotalTokens,
			&row.TotalActualCost,
		); err != nil {
			return nil, fmt.Errorf("scan monthly report row: %w", err)
		}
		if teamCode.Valid {
			row.TeamCode = &teamCode.String
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monthly report rows: %w", err)
	}
	return result, nil
}

func (r *UsageDailyRepository) RecentDailyActualCosts(ctx context.Context, beforeDate string, days int) ([]DailyActualCost, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive")
	}
	end, err := time.Parse("2006-01-02", beforeDate)
	if err != nil {
		return nil, fmt.Errorf("before date must be YYYY-MM-DD: %s", beforeDate)
	}
	startDate := end.AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := r.db.QueryContext(ctx, `
SELECT
  usage_date,
  COALESCE(SUM(total_actual_cost), 0)
FROM usage_daily
WHERE usage_date >= ?
  AND usage_date < ?
GROUP BY usage_date
ORDER BY usage_date
`, startDate, beforeDate)
	if err != nil {
		return nil, fmt.Errorf("query recent daily actual costs: %w", err)
	}
	defer rows.Close()

	var result []DailyActualCost
	for rows.Next() {
		var row DailyActualCost
		if err := rows.Scan(&row.UsageDate, &row.TotalActualCost); err != nil {
			return nil, fmt.Errorf("scan recent daily actual cost: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent daily actual costs: %w", err)
	}
	return result, nil
}

func nextMonthStart(month string) (string, error) {
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return "", fmt.Errorf("month must be YYYY-MM: %s", month)
	}
	return parsed.AddDate(0, 1, 0).Format("2006-01-02"), nil
}
