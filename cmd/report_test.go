package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codesome-usage-manager/internal/repository"
)

func TestResolveReportMonth(t *testing.T) {
	month, err := resolveReportMonth("2026-05")
	if err != nil {
		t.Fatalf("resolve month: %v", err)
	}
	if month != "2026-05" {
		t.Fatalf("unexpected month: %s", month)
	}
	if _, err := resolveReportMonth("2026-5"); err == nil {
		t.Fatal("expected malformed month to fail")
	}
}

func TestWriteMonthlyReportCSV(t *testing.T) {
	var buf bytes.Buffer
	team := "platform"
	rows := []repository.MonthlyReportRow{
		{
			Month:           "2026-05",
			TeamCode:        &team,
			UserName:        "Alice",
			EmployeeNo:      "E12345",
			TotalRequests:   10,
			TotalTokens:     100,
			TotalActualCost: 1.25,
		},
	}

	if err := writeMonthlyReportCSV(&buf, rows); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "month,team,user,employee_no,total_requests,total_tokens,total_actual_cost") {
		t.Fatalf("missing header: %s", got)
	}
	if !strings.Contains(got, "2026-05,platform,Alice,E12345,10,100,1.250000") {
		t.Fatalf("missing row: %s", got)
	}
}

func TestCreateReportOutputRestrictsExistingFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.csv")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	file, err := createReportOutput(path)
	if err != nil {
		t.Fatalf("create report output: %v", err)
	}
	if _, err := file.WriteString("report"); err != nil {
		t.Fatalf("write report output: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close report output: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat report output: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", got)
	}
}
