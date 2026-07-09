package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

type CodexDailyUsageSource interface {
	DailyUsage(ctx context.Context, dates []string) ([]CodexDailyUsage, error)
}

type CodexDailyUsage struct {
	UsageDate         string
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCacheTokens  int64
	TotalTokens       int64
	TotalCost         float64
}

type CodexUsageImportOptions struct {
	EmployeeNo string
	Dates      []string
	Overwrite  bool
	DryRun     bool
}

type CodexUsageImportResult struct {
	UsageDate       string
	EmployeeNo      string
	Source          string
	SourceAccountID string
	FeishuOpenID    string
	Tokens          int64
	Cost            float64
	Action          string
}

type CodexUsageImporter struct {
	database *sql.DB
	users    *repository.UserRepository
	accounts *repository.UsageAccountRepository
	usage    *repository.UsageDailyRepository
	source   CodexDailyUsageSource
}

func NewCodexUsageImporter(database *sql.DB, source CodexDailyUsageSource) *CodexUsageImporter {
	return &CodexUsageImporter{
		database: database,
		users:    repository.NewUserRepository(database),
		accounts: repository.NewUsageAccountRepository(database),
		usage:    repository.NewUsageDailyRepository(database),
		source:   source,
	}
}

func (i *CodexUsageImporter) Import(ctx context.Context, options CodexUsageImportOptions) ([]CodexUsageImportResult, error) {
	if options.EmployeeNo == "" {
		return nil, fmt.Errorf("employee no is required")
	}
	if len(options.Dates) == 0 {
		return nil, fmt.Errorf("usage dates are required")
	}
	if i.source == nil {
		return nil, fmt.Errorf("codex usage source is nil")
	}

	user, err := i.users.GetByEmployeeNo(ctx, options.EmployeeNo)
	if err != nil {
		return nil, err
	}
	if user.Status == repository.UserStatusDeleted {
		return nil, fmt.Errorf("deleted user cannot import codex usage: %s", options.EmployeeNo)
	}

	rows, err := i.source.DailyUsage(ctx, options.Dates)
	if err != nil {
		return nil, err
	}

	if options.DryRun {
		return i.importRows(ctx, i.accounts, i.usage, user, rows, options)
	}

	tx, err := i.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin codex usage import: %w", err)
	}
	defer tx.Rollback()

	results, err := i.importRows(ctx, repository.NewUsageAccountRepositoryTx(tx), repository.NewUsageDailyRepositoryTx(tx), user, rows, options)
	if err != nil {
		return results, err
	}
	if err := tx.Commit(); err != nil {
		return results, fmt.Errorf("commit codex usage import: %w", err)
	}
	return results, nil
}

func (i *CodexUsageImporter) importRows(ctx context.Context, accounts *repository.UsageAccountRepository, usage *repository.UsageDailyRepository, user *repository.User, rows []CodexDailyUsage, options CodexUsageImportOptions) ([]CodexUsageImportResult, error) {
	account, err := prepareCodexAccount(ctx, accounts, user, options.DryRun)
	if err != nil {
		return nil, err
	}
	rowsByDate := codexRowsByDate(rows)

	results := make([]CodexUsageImportResult, 0, len(options.Dates))
	for _, date := range options.Dates {
		row, ok := rowsByDate[date]
		if !ok {
			results = append(results, codexImportResult(user, date, 0, 0, "missing_source"))
			continue
		}
		if account != nil {
			stored, err := usage.Find(ctx, account.ID, date)
			if err != nil {
				return results, err
			}
			if stored != nil && !options.Overwrite {
				results = append(results, codexImportResult(user, date, stored.TotalTokens, stored.TotalActualCost, "skipped_existing"))
				continue
			}
			if options.DryRun {
				action := "would_import"
				if stored != nil {
					action = "would_overwrite"
				}
				results = append(results, codexImportResult(user, date, row.TotalTokens, row.TotalCost, action))
				continue
			}
			if _, err := usage.Upsert(ctx, account.ID, date, row.toUsageStats()); err != nil {
				return results, err
			}
			action := "imported"
			if stored != nil {
				action = "overwritten"
			}
			results = append(results, codexImportResult(user, date, row.TotalTokens, row.TotalCost, action))
			continue
		}
		results = append(results, codexImportResult(user, date, row.TotalTokens, row.TotalCost, "would_import"))
	}
	return results, nil
}

func prepareCodexAccount(ctx context.Context, accounts *repository.UsageAccountRepository, user *repository.User, dryRun bool) (*repository.UsageAccount, error) {
	existing, err := accounts.FindBySource(ctx, repository.UsageSourceCodex, user.EmployeeNo)
	if err != nil {
		return nil, err
	}
	if existing != nil || dryRun {
		return existing, nil
	}
	return accounts.EnsureCodexAccount(ctx, user.ID, user.EmployeeNo, "")
}

func codexRowsByDate(rows []CodexDailyUsage) map[string]CodexDailyUsage {
	result := make(map[string]CodexDailyUsage, len(rows))
	for _, row := range rows {
		if row.UsageDate == "" {
			continue
		}
		result[row.UsageDate] = row
	}
	return result
}

func codexImportResult(user *repository.User, date string, tokens int64, cost float64, action string) CodexUsageImportResult {
	return CodexUsageImportResult{
		UsageDate:       date,
		EmployeeNo:      user.EmployeeNo,
		Source:          repository.UsageSourceCodex,
		SourceAccountID: user.EmployeeNo,
		FeishuOpenID:    user.FeishuOpenID,
		Tokens:          tokens,
		Cost:            cost,
		Action:          action,
	}
}

func (u CodexDailyUsage) toUsageStats() provider.CodesomeUsageStats {
	return provider.CodesomeUsageStats{
		TotalInputTokens:  u.TotalInputTokens,
		TotalOutputTokens: u.TotalOutputTokens,
		TotalCacheTokens:  u.TotalCacheTokens,
		TotalTokens:       u.TotalTokens,
		TotalCost:         u.TotalCost,
		TotalActualCost:   u.TotalCost,
	}
}

func ParseCCUsageCodexDailyJSON(data []byte) ([]CodexDailyUsage, error) {
	var payload struct {
		Type  string            `json:"type"`
		Data  []ccusageDailyRow `json:"data"`
		Daily []ccusageDailyRow `json:"daily"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse ccusage codex daily json: %w", err)
	}
	rows := payload.Data
	if len(rows) == 0 {
		rows = payload.Daily
	}
	result := make([]CodexDailyUsage, 0, len(rows))
	for _, row := range rows {
		if row.Date == "" {
			continue
		}
		result = append(result, CodexDailyUsage{
			UsageDate:         row.Date,
			TotalInputTokens:  row.InputTokens,
			TotalOutputTokens: row.OutputTokens,
			TotalCacheTokens:  row.CacheCreationTokens + row.CacheReadTokens,
			TotalTokens:       row.TotalTokens,
			TotalCost:         row.cost(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UsageDate < result[j].UsageDate
	})
	return result, nil
}

type ccusageDailyRow struct {
	Date                string  `json:"date"`
	InputTokens         int64   `json:"inputTokens"`
	OutputTokens        int64   `json:"outputTokens"`
	CacheCreationTokens int64   `json:"cacheCreationTokens"`
	CacheReadTokens     int64   `json:"cacheReadTokens"`
	TotalTokens         int64   `json:"totalTokens"`
	CostUSD             float64 `json:"costUSD"`
	TotalCost           float64 `json:"totalCost"`
}

func (r ccusageDailyRow) cost() float64 {
	if r.CostUSD != 0 {
		return r.CostUSD
	}
	return r.TotalCost
}
