package payg

import (
	"context"
	"math"
	"sort"
	"time"

	"codesome-usage-manager/internal/config"
	codesomedb "codesome-usage-manager/internal/db"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func LoadFallbackPolicy(ctx context.Context, cfg *config.Config) (provider.PayAsYouGoFallbackPolicy, error) {
	return LoadFallbackPolicyWithPath(ctx, cfg, "")
}

func LoadFallbackPolicyWithPath(ctx context.Context, cfg *config.Config, databasePath string) (provider.PayAsYouGoFallbackPolicy, error) {
	if cfg == nil {
		return provider.PayAsYouGoFallbackPolicy{}, nil
	}
	codesome := cfg.GetCodesomeConfig()
	if codesome == nil {
		return provider.PayAsYouGoFallbackPolicy{}, nil
	}

	policy := provider.PayAsYouGoFallbackPolicy{
		GroupID:                      codesome.PayAsYouGoGroupID,
		MinSubscriptionDailyLimitUSD: codesome.PayAsYouGoMinSubscriptionDailyLimitUSD,
	}
	if codesome.PayAsYouGoHistoryDays <= 0 {
		return policy, nil
	}

	if databasePath == "" {
		databasePath = cfg.DatabasePath()
	}
	database, err := codesomedb.OpenReadOnly(databasePath)
	if err != nil {
		policy.HistoryLoadError = err.Error()
		return policy, nil
	}
	defer database.Close()

	rows, err := repository.NewUsageDailyRepository(database).RecentDailyActualCosts(ctx, todayInShanghai(), codesome.PayAsYouGoHistoryDays)
	if err != nil {
		policy.HistoryLoadError = err.Error()
		return policy, nil
	}
	if len(rows) == 0 {
		policy.HistoryLoadError = "未找到按量付费保护所需的历史用量"
		return policy, nil
	}
	costs := make([]float64, 0, len(rows))
	for _, row := range rows {
		costs = append(costs, row.TotalActualCost)
	}
	policy.RecentDailyUsageP80USD = PercentileNearestRank(costs, 0.8)
	return policy, nil
}

func PercentileNearestRank(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if percentile <= 0 {
		percentile = 0
	}
	if percentile > 1 {
		percentile = 1
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func todayInShanghai() string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return time.Now().In(loc).Format("2006-01-02")
}
