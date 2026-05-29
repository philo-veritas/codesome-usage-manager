package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codesome-usage-manager/internal/config"
	importsync "codesome-usage-manager/internal/sync"
)

func TestResolveDatabasePathUsesExplicitFlag(t *testing.T) {
	original := dbPath
	defer func() { dbPath = original }()

	dbPath = "/tmp/explicit.db"
	got, err := resolveDatabasePath()
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	if got != dbPath {
		t.Fatalf("expected explicit path %q, got %q", dbPath, got)
	}
}

func TestResolveDatabasePathUsesConfigOrDefault(t *testing.T) {
	original := dbPath
	defer func() { dbPath = original }()

	dbPath = ""
	t.Chdir(t.TempDir())

	got, err := resolveDatabasePath()
	if err != nil {
		t.Fatalf("resolve default path: %v", err)
	}
	if got != config.DefaultDatabasePath {
		t.Fatalf("expected default path %q, got %q", config.DefaultDatabasePath, got)
	}

	configuredPath := filepath.Join(t.TempDir(), "configured.db")
	if err := os.WriteFile("config.yaml", []byte("database:\n  path: "+configuredPath+"\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	got, err = resolveDatabasePath()
	if err != nil {
		t.Fatalf("resolve configured path: %v", err)
	}
	if got != configuredPath {
		t.Fatalf("expected configured path %q, got %q", configuredPath, got)
	}
}

func TestPrintDBImportConfigKeysResults(t *testing.T) {
	var buf bytes.Buffer
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = originalStdout }()

	printDBImportConfigKeysResults([]importsync.ImportConfigKeysResult{
		{
			Action:        "create",
			EmployeeNo:    "legacy:main",
			UserName:      "Main Key",
			CodesomeKeyID: 6732,
			GroupID:       51,
		},
	})
	writer.Close()
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "ACTION") || !strings.Contains(got, "legacy:main") || !strings.Contains(got, "6732") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestPrintDBImportRemoteKeysResults(t *testing.T) {
	var buf bytes.Buffer
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = originalStdout }()

	printDBImportRemoteKeysResults([]importsync.ImportRemoteKeysResult{
		{
			Action:        "create",
			EmployeeNo:    "codesome-key:6732",
			UserName:      "Main Key",
			CodesomeKeyID: 6732,
			GroupID:       51,
		},
	})
	writer.Close()
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "ACTION") || !strings.Contains(got, "codesome-key:6732") || !strings.Contains(got, "6732") {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestRunDBImportConfigKeysDryRunDoesNotCreateDatabaseFile(t *testing.T) {
	originalPath := dbPath
	originalDryRun := dbImportConfigKeysDryRun
	originalGroupID := dbImportConfigKeysGroupID
	defer func() {
		dbPath = originalPath
		dbImportConfigKeysDryRun = originalDryRun
		dbImportConfigKeysGroupID = originalGroupID
	}()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	databasePath := filepath.Join(tempDir, "missing", "codesome.db")
	dbPath = databasePath
	dbImportConfigKeysDryRun = true
	dbImportConfigKeysGroupID = 0

	configData := []byte(`codesome:
  default_group_id: 51
  api_key_ids:
    - id: 6732
      key: main
      name: Main Key
`)
	if err := os.WriteFile("config.yaml", configData, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = originalStdout }()

	if err := runDBImportConfigKeys(nil, nil); err != nil {
		t.Fatalf("run dry-run import: %v", err)
	}
	writer.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if !strings.Contains(buf.String(), "create") {
		t.Fatalf("expected create plan, got: %s", buf.String())
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create database file, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create database directory, stat err: %v", err)
	}
}
