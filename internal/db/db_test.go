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
		"usage_accounts",
		"usage_daily",
		"feishu_usage_records",
		"sync_runs",
	} {
		if !tableExists(t, database, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func TestMigrateUsageAccountsPreservesExistingUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codesome-manager.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if _, err := database.ExecContext(context.Background(), `
	CREATE TABLE IF NOT EXISTS schema_migrations (
	  version INTEGER PRIMARY KEY,
	  name TEXT NOT NULL,
	  applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`); err != nil {
		t.Fatalf("create schema migrations: %v", err)
	}
	for _, migration := range Migrations[:4] {
		if err := applyMigration(context.Background(), database, migration); err != nil {
			t.Fatalf("apply migration %d: %v", migration.Version, err)
		}
	}
	if _, err := database.ExecContext(context.Background(), `
	INSERT INTO users (employee_no, name, status, created_at, updated_at)
	VALUES ('E12345', 'Alice', 'active', '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z');
	INSERT INTO api_keys (user_id, codesome_key_id, name, status, group_id, created_at, updated_at, last_synced_at)
	VALUES (1, 6732, 'Alice', 'active', 51, '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z', '2026-05-02T00:00:00Z');
	INSERT INTO usage_daily (
	  api_key_id, usage_date, total_requests, total_tokens, total_actual_cost, fetched_at
	) VALUES (
	  1, '2026-05-26', 10, 100, 1.25, '2026-05-27T00:00:00Z'
	);
	`); err != nil {
		t.Fatalf("seed legacy usage: %v", err)
	}

	if err := Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	var accountID int64
	var sourceAccountID string
	if err := database.QueryRow(`
	SELECT id, source_account_id
	FROM usage_accounts
	WHERE source = 'codesome'
	`).Scan(&accountID, &sourceAccountID); err != nil {
		t.Fatalf("query usage account: %v", err)
	}
	if sourceAccountID != "6732" {
		t.Fatalf("unexpected source account id: %s", sourceAccountID)
	}

	var usageAccountID int64
	var tokens int64
	if err := database.QueryRow(`
	SELECT usage_account_id, total_tokens
	FROM usage_daily
	WHERE usage_date = '2026-05-26'
	`).Scan(&usageAccountID, &tokens); err != nil {
		t.Fatalf("query migrated usage: %v", err)
	}
	if usageAccountID != accountID || tokens != 100 {
		t.Fatalf("unexpected migrated usage account=%d tokens=%d", usageAccountID, tokens)
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
