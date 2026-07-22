package cmd

import (
	"bytes"
	"strings"
	"testing"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestBuildUsageTodayResultsPreservesTargetOrder(t *testing.T) {
	targets := []repository.APIKeyUsageTarget{
		{CodesomeKeyID: 6732, Name: "Alice", UserStatus: repository.UserStatusActive},
		{CodesomeKeyID: 6733, Name: "Bob", UserStatus: repository.UserStatusActive},
	}
	usageMap := map[int]provider.CodesomeKeyUsage{
		6733: {ApiKeyID: 6733, TodayCost: 2.5, TotalCost: 3.5},
		6732: {ApiKeyID: 6732, TodayCost: 1.25, TotalCost: 1.25},
	}

	results := buildUsageTodayResults(targets, usageMap)
	if len(results) != 2 || results[0].target.CodesomeKeyID != 6732 || results[1].target.CodesomeKeyID != 6733 {
		t.Fatalf("unexpected result order: %+v", results)
	}
}

func TestBuildUsageTodayResultsKeepsMissingUsage(t *testing.T) {
	results := buildUsageTodayResults(
		[]repository.APIKeyUsageTarget{{CodesomeKeyID: 6732, Name: "Alice"}},
		map[int]provider.CodesomeKeyUsage{},
	)
	if len(results) != 1 || results[0].usageFound {
		t.Fatalf("expected missing usage result, got %+v", results)
	}
}

func TestSortUsageTodayResultsByCostDescending(t *testing.T) {
	results := []usageTodayResult{
		{
			target:     repository.APIKeyUsageTarget{CodesomeKeyID: 6732, Name: "Alice"},
			usage:      provider.CodesomeKeyUsage{TodayCost: 1.25},
			usageFound: true,
		},
		{
			target:     repository.APIKeyUsageTarget{CodesomeKeyID: 6733, Name: "Bob"},
			usage:      provider.CodesomeKeyUsage{TodayCost: 7.5},
			usageFound: true,
		},
		{
			target: repository.APIKeyUsageTarget{CodesomeKeyID: 9362, Name: "Deleted"},
		},
	}

	sortUsageTodayResultsByCost(results)

	if results[0].target.CodesomeKeyID != 6733 || results[1].target.CodesomeKeyID != 6732 || results[2].target.CodesomeKeyID != 9362 {
		t.Fatalf("unexpected sorted order: %+v", results)
	}
}

func TestPrintUsageTodayResults(t *testing.T) {
	var buf bytes.Buffer
	printUsageTodayResults(&buf, 10, []usageTodayResult{
		{
			target: repository.APIKeyUsageTarget{
				CodesomeKeyID: 6732,
				Name:          "Alice",
				UserStatus:    repository.UserStatusActive,
			},
			usage:      provider.CodesomeKeyUsage{TodayCost: 1.25, TotalCost: 3.5},
			usageFound: true,
		},
		{
			target: repository.APIKeyUsageTarget{
				CodesomeKeyID: 9362,
				Name:          "Deleted",
				UserStatus:    repository.UserStatusActive,
			},
		},
	})

	got := buf.String()
	if !strings.Contains(got, "KEY_ID") || !strings.Contains(got, "USAGE_STATUS") || !strings.Contains(got, "TODAY_ACTUAL_COST_ACC") {
		t.Fatalf("missing header: %s", got)
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 3 || strings.Fields(lines[0])[0] != "NO" || strings.Fields(lines[1])[0] != "1" || strings.Fields(lines[2])[0] != "2" {
		t.Fatalf("unexpected row numbers: %s", got)
	}
	if !strings.Contains(got, "6732") || !strings.Contains(got, "Alice") || !strings.Contains(got, "ok") || !strings.Contains(got, "1.250000 (12.50%)") {
		t.Fatalf("missing ok row: %s", got)
	}
	if !strings.Contains(got, "9362") || !strings.Contains(got, "remote_missing") || strings.Count(got, "1.250000 (12.50%)") != 2 {
		t.Fatalf("missing remote missing row: %s", got)
	}
}

func TestPrintUsageTodayResultsAccumulatesInDisplayOrder(t *testing.T) {
	var buf bytes.Buffer
	printUsageTodayResults(&buf, 12, []usageTodayResult{
		{usage: provider.CodesomeKeyUsage{TodayCost: 3}},
		{usage: provider.CodesomeKeyUsage{TodayCost: 2}},
		{usage: provider.CodesomeKeyUsage{TodayCost: 1}},
	})

	got := buf.String()
	for _, want := range []string{
		"3.000000 (25.00%)",
		"5.000000 (41.67%)",
		"6.000000 (50.00%)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing cumulative cost %q: %s", want, got)
		}
	}
}

func TestPrintUsageTodayResultsHandlesZeroTotalLimit(t *testing.T) {
	var buf bytes.Buffer
	printUsageTodayResults(&buf, 0, []usageTodayResult{
		{usage: provider.CodesomeKeyUsage{TodayCost: 3}},
	})

	if got := buf.String(); !strings.Contains(got, "3.000000 (0.00%)") {
		t.Fatalf("unexpected cumulative cost percentage: %s", got)
	}
}

func TestPrintUsageTodayReportIncludesSubscriptionSummary(t *testing.T) {
	var buf bytes.Buffer
	printUsageTodayReport(
		&buf,
		provider.CodesomeSubscriptionUsageSummary{
			RemainingUSD: 315.49,
			LimitUSD:     1740,
		},
		[]usageTodayResult{
			{
				target: repository.APIKeyUsageTarget{
					CodesomeKeyID: 6732,
					Name:          "Alice",
					UserStatus:    repository.UserStatusActive,
				},
				usage:      provider.CodesomeKeyUsage{TodayCost: 1.25, TotalCost: 3.5},
				usageFound: true,
			},
		},
	)

	got := buf.String()
	if !strings.Contains(got, "今日总余额：$315.49 / $1740.00") {
		t.Fatalf("missing subscription summary: %s", got)
	}
	if !strings.Contains(got, "KEY_ID") || !strings.Contains(got, "6732") || !strings.Contains(got, "Alice") {
		t.Fatalf("missing usage table: %s", got)
	}
}
