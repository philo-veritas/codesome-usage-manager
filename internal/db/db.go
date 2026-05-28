package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	database, err := sql.Open(DriverName, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return database, nil
}

func Init(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("database path is required")
	}
	database, err := Open(path)
	if err != nil {
		return err
	}
	defer database.Close()
	return Migrate(ctx, database)
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "foreign_keys(1)")
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + values.Encode()
}
