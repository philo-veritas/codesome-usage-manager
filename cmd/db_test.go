package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"codesome-usage-manager/internal/config"
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
