package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codesome-manager.db")

	if err := Init(context.Background(), path); err != nil {
		t.Fatalf("init database: %v", err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	for _, table := range []string{
		"schema_migrations",
		"teams",
		"users",
		"api_keys",
		"usage_daily",
		"feishu_usage_records",
		"sync_runs",
	} {
		if !tableExists(t, database, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codesome-manager.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	for i := 0; i < 2; i++ {
		if err := Migrate(context.Background(), database); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != len(Migrations) {
		t.Fatalf("expected %d migrations, got %d", len(Migrations), count)
	}
}

func TestOpenEnforcesForeignKeysOnPooledConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codesome-manager.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(2)

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	conn, err := database.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve connection: %v", err)
	}
	defer conn.Close()
	if err := conn.PingContext(context.Background()); err != nil {
		t.Fatalf("ping reserved connection: %v", err)
	}

	_, err = database.Exec(`
INSERT INTO api_keys (
  user_id, codesome_key_id, name, status, group_id, created_at, updated_at
) VALUES (
  999, 6732, 'orphan', 'active', 51, '2026-05-28T00:00:00Z', '2026-05-28T00:00:00Z'
);
`)
	if err == nil {
		t.Fatal("expected foreign key constraint to fail on pooled connection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("expected foreign key error, got %v", err)
	}
}

func TestOpenReadOnlySupportsRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	path := "codesome-manager.db"

	if err := Init(context.Background(), path); err != nil {
		t.Fatalf("init database: %v", err)
	}
	database, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only database: %v", err)
	}
	defer database.Close()

	if !tableExists(t, database, "api_keys") {
		t.Fatal("expected api_keys table to exist")
	}
	if _, err := database.Exec("CREATE TABLE should_not_write (id INTEGER)"); err == nil {
		t.Fatal("expected write through read-only database to fail")
	}
}

func tableExists(t *testing.T, database *sql.DB, table string) bool {
	t.Helper()

	var name string
	err := database.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	return name == table
}
