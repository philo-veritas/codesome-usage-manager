package db

import (
	"context"
	"database/sql"
	"fmt"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var Migrations = []Migration{
	{
		Version: 1,
		Name:    "initial_schema",
		SQL: `
CREATE TABLE teams (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  employee_no TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  team_id INTEGER REFERENCES teams(id),
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive', 'deleted')),
  codesome_group_id INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);

CREATE INDEX idx_users_team_id ON users(team_id);
CREATE INDEX idx_users_status ON users(status);

CREATE TABLE api_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  codesome_key_id INTEGER NOT NULL UNIQUE,
  name TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
  group_id INTEGER NOT NULL,
  raw_key TEXT,
  raw_key_stored_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_synced_at TEXT
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_status ON api_keys(status);

CREATE TABLE usage_daily (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  api_key_id INTEGER NOT NULL REFERENCES api_keys(id),
  usage_date TEXT NOT NULL,
  total_requests INTEGER NOT NULL DEFAULT 0,
  total_input_tokens INTEGER NOT NULL DEFAULT 0,
  total_output_tokens INTEGER NOT NULL DEFAULT 0,
  total_cache_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost REAL NOT NULL DEFAULT 0,
  total_actual_cost REAL NOT NULL DEFAULT 0,
  average_duration_ms REAL NOT NULL DEFAULT 0,
  fetched_at TEXT NOT NULL,
  UNIQUE(api_key_id, usage_date)
);

CREATE INDEX idx_usage_daily_date ON usage_daily(usage_date);
CREATE INDEX idx_usage_daily_api_key_date ON usage_daily(api_key_id, usage_date);

CREATE TABLE sync_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL CHECK (kind IN ('users', 'usage', 'import')),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
  message TEXT
);
`,
	},
	{
		Version: 2,
		Name:    "add_user_feishu_open_id",
		SQL: `
ALTER TABLE users ADD COLUMN feishu_open_id TEXT;
CREATE INDEX idx_users_feishu_open_id ON users(feishu_open_id);
`,
	},
	{
		Version: 3,
		Name:    "unique_user_feishu_open_id",
		SQL: `
DROP INDEX IF EXISTS idx_users_feishu_open_id;
CREATE UNIQUE INDEX idx_users_feishu_open_id ON users(feishu_open_id) WHERE feishu_open_id IS NOT NULL;
`,
	},
	{
		Version: 4,
		Name:    "add_feishu_usage_records",
		SQL: `
CREATE TABLE feishu_usage_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  app_token TEXT NOT NULL,
  table_id TEXT NOT NULL,
  sync_id TEXT NOT NULL,
  record_id TEXT NOT NULL,
  synced_at TEXT NOT NULL,
  UNIQUE(app_token, table_id, sync_id)
);

CREATE INDEX idx_feishu_usage_records_record_id ON feishu_usage_records(record_id);
`,
	},
}

func Migrate(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	if _, err := database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, migration := range Migrations {
		applied, err := isMigrationApplied(ctx, database, migration.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(ctx, database, migration); err != nil {
			return err
		}
	}
	return nil
}

func isMigrationApplied(ctx context.Context, database *sql.DB, version int) (bool, error) {
	var exists int
	err := database.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return true, nil
}

func applyMigration(ctx context.Context, database *sql.DB, migration Migration) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %d %s: %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
		migration.Version,
		migration.Name,
	); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}
