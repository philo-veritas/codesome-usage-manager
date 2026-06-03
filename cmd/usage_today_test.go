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
	printUsageTodayResults(&buf, []usageTodayResult{
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
	if !strings.Contains(got, "KEY_ID") || !strings.Contains(got, "USAGE_STATUS") || !strings.Contains(got, "TODAY_ACTUAL_COST") {
		t.Fatalf("missing header: %s", got)
	}
	if !strings.Contains(got, "6732") || !strings.Contains(got, "Alice") || !strings.Contains(got, "ok") || !strings.Contains(got, "1.250000") {
		t.Fatalf("missing ok row: %s", got)
	}
	if !strings.Contains(got, "9362") || !strings.Contains(got, "remote_missing") {
		t.Fatalf("missing remote missing row: %s", got)
	}
}
