package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codesome-usage-manager/internal/repository"
)

func TestResolveKeyExportParamsRequiresOneSelector(t *testing.T) {
	restore := setKeyExportFlags("", "", false, false)
	defer restore()

	if _, err := resolveKeyExportParams(); err == nil {
		t.Fatal("expected missing selector to fail")
	}

	restore = setKeyExportFlags("E12345", "platform", false, false)
	defer restore()
	if _, err := resolveKeyExportParams(); err == nil {
		t.Fatal("expected multiple selectors to fail")
	}

	restore = setKeyExportFlags("E12345", "", false, true)
	defer restore()
	params, err := resolveKeyExportParams()
	if err != nil {
		t.Fatalf("resolve params: %v", err)
	}
	if params.EmployeeNo != "E12345" || !params.IncludeInactive {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestWriteAPIKeyExportCSVMarksMissingRawKey(t *testing.T) {
	var buf bytes.Buffer
	rawKey := "sk-test"
	team := "platform"

	err := writeAPIKeyExportCSV(&buf, []repository.APIKeyExportRow{
		{
			EmployeeNo:    "E12345",
			UserName:      "Alice",
			TeamCode:      &team,
			KeyName:       "Alice",
			CodesomeKeyID: 6732,
			RawKey:        &rawKey,
			Status:        repository.APIKeyStatusActive,
		},
		{
			EmployeeNo:    "E99999",
			UserName:      "Bob",
			KeyName:       "Bob",
			CodesomeKeyID: 6733,
			Status:        repository.APIKeyStatusInactive,
		},
	})
	if err != nil {
		t.Fatalf("write csv: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "employee_no,name,team,key_name,codesome_key_id,raw_key,raw_key_missing,status") {
		t.Fatalf("missing header: %s", got)
	}
	if !strings.Contains(got, "E12345,Alice,platform,Alice,6732,sk-test,false,active") {
		t.Fatalf("missing raw key row: %s", got)
	}
	if !strings.Contains(got, "E99999,Bob,,Bob,6733,,true,inactive") {
		t.Fatalf("missing raw-key-missing row: %s", got)
	}
}

func TestCreateKeyExportOutputRestrictsExistingFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.csv")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	file, err := createKeyExportOutput(path)
	if err != nil {
		t.Fatalf("create export output: %v", err)
	}
	if _, err := file.WriteString("secret"); err != nil {
		t.Fatalf("write export output: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close export output: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat export output: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", got)
	}
}

func setKeyExportFlags(employeeNo string, team string, all bool, includeInactive bool) func() {
	originalEmployeeNo := keyExportEmployeeNo
	originalTeam := keyExportTeam
	originalAll := keyExportAll
	originalIncludeInactive := keyExportIncludeInactive

	keyExportEmployeeNo = employeeNo
	keyExportTeam = team
	keyExportAll = all
	keyExportIncludeInactive = includeInactive

	return func() {
		keyExportEmployeeNo = originalEmployeeNo
		keyExportTeam = originalTeam
		keyExportAll = originalAll
		keyExportIncludeInactive = originalIncludeInactive
	}
}
