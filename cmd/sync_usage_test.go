package cmd

import (
	"testing"
	"time"
)

func TestResolveSyncUsageDates(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-05-28")
	defer restoreNow()

	restoreFlags := setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-05-26"})
	defer restoreFlags()
	dates, err := resolveSyncUsageDates()
	if err != nil {
		t.Fatalf("resolve date: %v", err)
	}
	if len(dates) != 1 || dates[0] != "2026-05-26" {
		t.Fatalf("unexpected dates: %+v", dates)
	}

	restoreFlags()
	restoreFlags = setSyncUsageDateFlags(syncUsageDateFlagState{from: "2026-05-26", to: "2026-05-28"})
	dates, err = resolveSyncUsageDates()
	if err != nil {
		t.Fatalf("resolve range: %v", err)
	}
	if len(dates) != 2 || dates[0] != "2026-05-26" || dates[1] != "2026-05-27" {
		t.Fatalf("expected today to be skipped, got %+v", dates)
	}

	restoreFlags()
	restoreFlags = setSyncUsageDateFlags(syncUsageDateFlagState{yesterday: true})
	dates, err = resolveSyncUsageDates()
	if err != nil {
		t.Fatalf("resolve yesterday: %v", err)
	}
	if len(dates) != 1 || dates[0] != "2026-05-27" {
		t.Fatalf("unexpected yesterday dates: %+v", dates)
	}
}

func TestResolveSyncUsageDatesRequiresIncludeToday(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-05-28")
	defer restoreNow()

	restoreFlags := setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-05-28"})
	defer restoreFlags()
	if _, err := resolveSyncUsageDates(); err == nil {
		t.Fatal("expected today without include-today to fail")
	}

	restoreFlags()
	restoreFlags = setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-05-28", includeToday: true})
	dates, err := resolveSyncUsageDates()
	if err != nil {
		t.Fatalf("resolve today with include: %v", err)
	}
	if len(dates) != 1 || dates[0] != "2026-05-28" {
		t.Fatalf("unexpected dates: %+v", dates)
	}
}

func TestResolveSyncUsageDatesRejectsInvalidSelectors(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-05-28")
	defer restoreNow()

	restoreFlags := setSyncUsageDateFlags(syncUsageDateFlagState{})
	defer restoreFlags()
	if _, err := resolveSyncUsageDates(); err == nil {
		t.Fatal("expected missing selector to fail")
	}

	restoreFlags()
	restoreFlags = setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-05-26", yesterday: true})
	if _, err := resolveSyncUsageDates(); err == nil {
		t.Fatal("expected multiple selectors to fail")
	}

	restoreFlags()
	restoreFlags = setSyncUsageDateFlags(syncUsageDateFlagState{from: "2026-05-26"})
	if _, err := resolveSyncUsageDates(); err == nil {
		t.Fatal("expected partial range to fail")
	}

	restoreFlags()
	restoreFlags = setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-05-29"})
	if _, err := resolveSyncUsageDates(); err == nil {
		t.Fatal("expected future date to fail")
	}
}

type syncUsageDateFlagState struct {
	date         string
	from         string
	to           string
	yesterday    bool
	includeToday bool
}

func setSyncUsageDateFlags(state syncUsageDateFlagState) func() {
	originalDate := syncUsageDate
	originalFrom := syncUsageFromDate
	originalTo := syncUsageToDate
	originalYesterday := syncUsageYesterday
	originalIncludeToday := syncUsageIncludeToday

	syncUsageDate = state.date
	syncUsageFromDate = state.from
	syncUsageToDate = state.to
	syncUsageYesterday = state.yesterday
	syncUsageIncludeToday = state.includeToday

	return func() {
		syncUsageDate = originalDate
		syncUsageFromDate = originalFrom
		syncUsageToDate = originalTo
		syncUsageYesterday = originalYesterday
		syncUsageIncludeToday = originalIncludeToday
	}
}

func setSyncUsageNow(date string) func() {
	original := syncUsageNow
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	loc := time.FixedZone("CST", 8*3600)
	syncUsageNow = func() time.Time {
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 12, 0, 0, 0, loc)
	}
	return func() {
		syncUsageNow = original
	}
}
