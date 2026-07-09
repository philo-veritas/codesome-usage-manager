package syncer

import (
	"context"
	"testing"

	"codesome-usage-manager/internal/repository"
)

func TestParseCCUsageCodexDailyJSON(t *testing.T) {
	rows, err := ParseCCUsageCodexDailyJSON([]byte(`{
		"type": "daily",
		"data": [
			{
				"date": "2026-07-08",
				"inputTokens": 10,
				"outputTokens": 20,
				"cacheCreationTokens": 30,
				"cacheReadTokens": 40,
				"totalTokens": 100,
				"costUSD": 1.25
			}
		]
	}`))
	if err != nil {
		t.Fatalf("parse ccusage json: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %+v", rows)
	}
	row := rows[0]
	if row.UsageDate != "2026-07-08" || row.TotalInputTokens != 10 || row.TotalOutputTokens != 20 || row.TotalCacheTokens != 70 || row.TotalTokens != 100 || row.TotalCost != 1.25 {
		t.Fatalf("unexpected parsed row: %+v", row)
	}
}

func TestCodexUsageImporterImportsSkipsAndOverwrites(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	source := &fakeCodexDailyUsageSource{
		rows: []CodexDailyUsage{
			{UsageDate: "2026-07-08", TotalInputTokens: 10, TotalOutputTokens: 20, TotalCacheTokens: 30, TotalTokens: 60, TotalCost: 1.25},
		},
	}
	importer := NewCodexUsageImporter(database, source)

	results, err := importer.Import(ctx, CodexUsageImportOptions{
		EmployeeNo: user.EmployeeNo,
		Dates:      []string{"2026-07-08"},
	})
	if err != nil {
		t.Fatalf("import codex usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "imported" || results[0].Tokens != 60 || results[0].Cost != 1.25 {
		t.Fatalf("unexpected import results: %+v", results)
	}

	account, err := repository.NewUsageAccountRepository(database).FindBySource(ctx, repository.UsageSourceCodex, "E12345")
	if err != nil {
		t.Fatalf("find codex account: %v", err)
	}
	if account == nil {
		t.Fatal("expected codex account")
	}
	stored, err := repository.NewUsageDailyRepository(database).Get(ctx, account.ID, "2026-07-08")
	if err != nil {
		t.Fatalf("get stored usage: %v", err)
	}
	if stored.TotalTokens != 60 || stored.TotalActualCost != 1.25 {
		t.Fatalf("unexpected stored usage: %+v", stored)
	}

	source.rows = []CodexDailyUsage{{UsageDate: "2026-07-08", TotalTokens: 99, TotalCost: 9.99}}
	results, err = importer.Import(ctx, CodexUsageImportOptions{
		EmployeeNo: user.EmployeeNo,
		Dates:      []string{"2026-07-08"},
	})
	if err != nil {
		t.Fatalf("reimport codex usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "skipped_existing" || results[0].Tokens != 60 {
		t.Fatalf("expected existing row to be skipped, got %+v", results)
	}

	results, err = importer.Import(ctx, CodexUsageImportOptions{
		EmployeeNo: user.EmployeeNo,
		Dates:      []string{"2026-07-08"},
		Overwrite:  true,
	})
	if err != nil {
		t.Fatalf("overwrite codex usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "overwritten" || results[0].Tokens != 99 {
		t.Fatalf("expected overwrite result, got %+v", results)
	}
}

type fakeCodexDailyUsageSource struct {
	rows []CodexDailyUsage
}

func (s *fakeCodexDailyUsageSource) DailyUsage(ctx context.Context, dates []string) ([]CodexDailyUsage, error) {
	return s.rows, nil
}
