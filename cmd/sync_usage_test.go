package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestBuildUsageSyncOptionsReusesExistingByDefault(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-06-12")
	defer restoreNow()

	restoreFlags := setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-06-11"})
	defer restoreFlags()

	options := buildUsageSyncOptions([]string{"2026-06-11"})
	if !options.ReuseExisting {
		t.Fatal("expected usage sync to reuse existing rows by default")
	}
	if options.ForceUpdate {
		t.Fatal("expected force update to follow flag default")
	}
	if options.ForceUpdateDates["2026-06-11"] {
		t.Fatalf("expected past date not to be force-updated: %+v", options.ForceUpdateDates)
	}
}

func TestBuildUsageSyncOptionsForceUpdatesIncludedToday(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-06-12")
	defer restoreNow()

	restoreFlags := setSyncUsageDateFlags(syncUsageDateFlagState{date: "2026-06-12", includeToday: true})
	defer restoreFlags()

	options := buildUsageSyncOptions([]string{"2026-06-12"})
	if !options.ReuseExisting {
		t.Fatal("expected usage sync to reuse existing rows where allowed")
	}
	if !options.ForceUpdateDates["2026-06-12"] {
		t.Fatalf("expected included today to be force-updated: %+v", options.ForceUpdateDates)
	}
}

func TestResolveUsageImportCodexDatesAllowsToday(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-07-08")
	defer restoreNow()

	restoreFlags := setUsageImportCodexDateFlags(usageImportCodexDateFlagState{date: "2026-07-08"})
	defer restoreFlags()

	dates, err := resolveUsageImportCodexDates()
	if err != nil {
		t.Fatalf("resolve codex import today: %v", err)
	}
	if len(dates) != 1 || dates[0] != "2026-07-08" {
		t.Fatalf("unexpected codex import dates: %+v", dates)
	}
}

func TestResolveUsageImportCodexDatesRejectsFuture(t *testing.T) {
	restoreNow := setSyncUsageNow("2026-07-08")
	defer restoreNow()

	restoreFlags := setUsageImportCodexDateFlags(usageImportCodexDateFlagState{from: "2026-07-08", to: "2026-07-09"})
	defer restoreFlags()

	if _, err := resolveUsageImportCodexDates(); err == nil {
		t.Fatal("expected future codex import date to fail")
	}
}

func TestRunUsageImportCodexDryRunDoesNotCreateDatabaseFile(t *testing.T) {
	originalPath := dbPath
	originalEmployeeNo := usageImportCodexEmployeeNo
	originalDate := usageImportCodexDate
	originalFrom := usageImportCodexFromDate
	originalTo := usageImportCodexToDate
	originalDryRun := usageImportCodexDryRun
	defer func() {
		dbPath = originalPath
		usageImportCodexEmployeeNo = originalEmployeeNo
		usageImportCodexDate = originalDate
		usageImportCodexFromDate = originalFrom
		usageImportCodexToDate = originalTo
		usageImportCodexDryRun = originalDryRun
	}()

	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "missing", "codesome.db")
	dbPath = databasePath
	usageImportCodexEmployeeNo = "E12345"
	usageImportCodexDate = "2026-07-08"
	usageImportCodexFromDate = ""
	usageImportCodexToDate = ""
	usageImportCodexDryRun = true

	err := runUsageImportCodex(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "需要已初始化数据库") {
		t.Fatalf("expected initialized database error, got %v", err)
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create database file, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create database directory, stat err: %v", err)
	}
}

func TestCCUsageCodexDailySourceParsesStdoutWhenStderrHasNotice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test uses sh")
	}
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.Mkdir(binDir, 0755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	npxPath := filepath.Join(binDir, "npx")
	script := `#!/bin/sh
echo "npm notice installing ccusage" >&2
cat <<'JSON'
{"data":[{"date":"2026-07-08","inputTokens":10,"outputTokens":20,"totalTokens":30,"costUSD":1.25}]}
JSON
`
	if err := os.WriteFile(npxPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake npx: %v", err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	rows, err := (ccusageCodexDailySource{Offline: true}).DailyUsage(context.Background(), []string{"2026-07-08"})
	if err != nil {
		t.Fatalf("read ccusage source: %v", err)
	}
	if len(rows) != 1 || rows[0].TotalTokens != 30 || rows[0].TotalCost != 1.25 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

type syncUsageDateFlagState struct {
	date         string
	from         string
	to           string
	yesterday    bool
	includeToday bool
}

type usageImportCodexDateFlagState struct {
	date string
	from string
	to   string
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

func setUsageImportCodexDateFlags(state usageImportCodexDateFlagState) func() {
	originalDate := usageImportCodexDate
	originalFrom := usageImportCodexFromDate
	originalTo := usageImportCodexToDate

	usageImportCodexDate = state.date
	usageImportCodexFromDate = state.from
	usageImportCodexToDate = state.to

	return func() {
		usageImportCodexDate = originalDate
		usageImportCodexFromDate = originalFrom
		usageImportCodexToDate = originalTo
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
