package repository

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	UsageSourceCodesome = "codesome"
	UsageSourceCodex    = "codex"

	UsageAccountStatusActive   = "active"
	UsageAccountStatusInactive = "inactive"
)

type UsageAccount struct {
	ID              int64
	UserID          int64
	Source          string
	SourceAccountID string
	DisplayName     string
	Status          string
	APIKeyID        *int64
	MetadataJSON    *string
	CreatedAt       string
	UpdatedAt       string
	LastSyncedAt    *string
}

type CodesomeUsageAccountTarget struct {
	UsageAccountID  int64
	CodesomeKeyID   int
	Source          string
	SourceAccountID string
	DisplayName     string
	UserStatus      string
	FeishuOpenID    string
}

type UsageAccountRepository struct {
	db usageAccountStore
}

type usageAccountStore interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewUsageAccountRepository(db *sql.DB) *UsageAccountRepository {
	return &UsageAccountRepository{db: db}
}

func NewUsageAccountRepositoryTx(tx *sql.Tx) *UsageAccountRepository {
	return &UsageAccountRepository{db: tx}
}

func (r *UsageAccountRepository) EnsureCodesomeAccounts(ctx context.Context) error {
	now := nowString()
	if _, err := r.db.ExecContext(ctx, `
	INSERT OR IGNORE INTO usage_accounts (
	  user_id,
	  source,
	  source_account_id,
	  display_name,
	  status,
	  api_key_id,
	  created_at,
	  updated_at,
	  last_synced_at
	)
	SELECT
	  user_id,
	  ?,
	  CAST(codesome_key_id AS TEXT),
	  name,
	  status,
	  id,
	  ?,
	  ?,
	  last_synced_at
	FROM api_keys
	`, UsageSourceCodesome, now, now); err != nil {
		return fmt.Errorf("ensure codesome usage accounts: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
	UPDATE usage_accounts
	SET
	  user_id = (
	    SELECT api_keys.user_id FROM api_keys WHERE CAST(api_keys.codesome_key_id AS TEXT) = usage_accounts.source_account_id
	  ),
	  display_name = (
	    SELECT api_keys.name FROM api_keys WHERE CAST(api_keys.codesome_key_id AS TEXT) = usage_accounts.source_account_id
	  ),
	  status = (
	    SELECT api_keys.status FROM api_keys WHERE CAST(api_keys.codesome_key_id AS TEXT) = usage_accounts.source_account_id
	  ),
	  api_key_id = (
	    SELECT api_keys.id FROM api_keys WHERE CAST(api_keys.codesome_key_id AS TEXT) = usage_accounts.source_account_id
	  ),
	  updated_at = ?,
	  last_synced_at = (
	    SELECT api_keys.last_synced_at FROM api_keys WHERE CAST(api_keys.codesome_key_id AS TEXT) = usage_accounts.source_account_id
	  )
	WHERE source = ?
	  AND EXISTS (
	    SELECT 1 FROM api_keys WHERE CAST(api_keys.codesome_key_id AS TEXT) = usage_accounts.source_account_id
	  )
	`, now, UsageSourceCodesome); err != nil {
		return fmt.Errorf("refresh codesome usage accounts: %w", err)
	}
	return nil
}

func (r *UsageAccountRepository) ListCodesomeUsageTargets(ctx context.Context) ([]CodesomeUsageAccountTarget, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT
	  usage_accounts.id,
	  api_keys.codesome_key_id,
	  usage_accounts.source,
	  usage_accounts.source_account_id,
	  usage_accounts.display_name,
	  users.status,
	  users.feishu_open_id
	FROM usage_accounts
	JOIN api_keys ON usage_accounts.api_key_id = api_keys.id
	JOIN users ON usage_accounts.user_id = users.id
	WHERE usage_accounts.source = ?
	  AND usage_accounts.status = ?
	  AND users.status != ?
	ORDER BY usage_accounts.id
	`, UsageSourceCodesome, UsageAccountStatusActive, UserStatusDeleted)
	if err != nil {
		return nil, fmt.Errorf("list codesome usage targets: %w", err)
	}
	defer rows.Close()

	var result []CodesomeUsageAccountTarget
	for rows.Next() {
		var target CodesomeUsageAccountTarget
		var feishuOpenID sql.NullString
		if err := rows.Scan(
			&target.UsageAccountID,
			&target.CodesomeKeyID,
			&target.Source,
			&target.SourceAccountID,
			&target.DisplayName,
			&target.UserStatus,
			&feishuOpenID,
		); err != nil {
			return nil, fmt.Errorf("scan codesome usage target: %w", err)
		}
		if feishuOpenID.Valid {
			target.FeishuOpenID = feishuOpenID.String
		}
		result = append(result, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate codesome usage targets: %w", err)
	}
	return result, nil
}

func (r *UsageAccountRepository) FindBySource(ctx context.Context, source string, sourceAccountID string) (*UsageAccount, error) {
	var account UsageAccount
	var apiKeyID sql.NullInt64
	var metadataJSON sql.NullString
	var lastSyncedAt sql.NullString
	if err := r.db.QueryRowContext(ctx, usageAccountSelectSQL()+`
	WHERE source = ? AND source_account_id = ?
	`, source, sourceAccountID).Scan(
		&account.ID,
		&account.UserID,
		&account.Source,
		&account.SourceAccountID,
		&account.DisplayName,
		&account.Status,
		&apiKeyID,
		&metadataJSON,
		&account.CreatedAt,
		&account.UpdatedAt,
		&lastSyncedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find usage account: %w", err)
	}
	assignNullableUsageAccountFields(&account, apiKeyID, metadataJSON, lastSyncedAt)
	return &account, nil
}

func (r *UsageAccountRepository) EnsureCodexAccount(ctx context.Context, userID int64, employeeNo string, metadataJSON string) (*UsageAccount, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("user id must be positive")
	}
	if employeeNo == "" {
		return nil, fmt.Errorf("employee no is required")
	}
	existing, err := r.FindBySource(ctx, UsageSourceCodex, employeeNo)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	now := nowString()
	if _, err := r.db.ExecContext(ctx, `
	INSERT INTO usage_accounts (
	  user_id,
	  source,
	  source_account_id,
	  display_name,
	  status,
	  metadata_json,
	  created_at,
	  updated_at
	) VALUES (
	  ?, ?, ?, ?, ?, ?, ?, ?
	)
	`, userID, UsageSourceCodex, employeeNo, "Codex 官方订阅", UsageAccountStatusActive, nullableString(metadataJSON), now, now); err != nil {
		return nil, fmt.Errorf("create codex usage account: %w", err)
	}
	return r.FindBySource(ctx, UsageSourceCodex, employeeNo)
}

func usageAccountSelectSQL() string {
	return `
	SELECT
	  id,
	  user_id,
	  source,
	  source_account_id,
	  display_name,
	  status,
	  api_key_id,
	  metadata_json,
	  created_at,
	  updated_at,
	  last_synced_at
	FROM usage_accounts`
}

func assignNullableUsageAccountFields(account *UsageAccount, apiKeyID sql.NullInt64, metadataJSON sql.NullString, lastSyncedAt sql.NullString) {
	if apiKeyID.Valid {
		account.APIKeyID = &apiKeyID.Int64
	}
	if metadataJSON.Valid {
		account.MetadataJSON = &metadataJSON.String
	}
	if lastSyncedAt.Valid {
		account.LastSyncedAt = &lastSyncedAt.String
	}
}
