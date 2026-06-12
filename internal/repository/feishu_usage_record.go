package repository

import (
	"context"
	"database/sql"
	"fmt"
)

type FeishuUsageRecordRepository struct {
	db *sql.DB
}

func NewFeishuUsageRecordRepository(db *sql.DB) *FeishuUsageRecordRepository {
	return &FeishuUsageRecordRepository{db: db}
}

func (r *FeishuUsageRecordRepository) GetRecordID(ctx context.Context, appToken string, tableID string, syncID string) (string, error) {
	if appToken == "" || tableID == "" || syncID == "" {
		return "", fmt.Errorf("app_token, table_id, and sync_id are required")
	}

	var recordID string
	err := r.db.QueryRowContext(ctx, `
SELECT record_id
FROM feishu_usage_records
WHERE app_token = ? AND table_id = ? AND sync_id = ?
`, appToken, tableID, syncID).Scan(&recordID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get feishu usage record: %w", err)
	}
	return recordID, nil
}

func (r *FeishuUsageRecordRepository) Upsert(ctx context.Context, appToken string, tableID string, syncID string, recordID string) error {
	if appToken == "" || tableID == "" || syncID == "" || recordID == "" {
		return fmt.Errorf("app_token, table_id, sync_id, and record_id are required")
	}

	if _, err := r.db.ExecContext(ctx, `
INSERT INTO feishu_usage_records (
  app_token,
  table_id,
  sync_id,
  record_id,
  synced_at
) VALUES (
  ?, ?, ?, ?, ?
)
ON CONFLICT(app_token, table_id, sync_id) DO UPDATE SET
  record_id = excluded.record_id,
  synced_at = excluded.synced_at
`, appToken, tableID, syncID, recordID, nowString()); err != nil {
		return fmt.Errorf("upsert feishu usage record: %w", err)
	}
	return nil
}
