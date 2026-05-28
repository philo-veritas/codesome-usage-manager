package repository

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	APIKeyStatusActive   = "active"
	APIKeyStatusInactive = "inactive"
)

type APIKey struct {
	ID             int64
	UserID         int64
	CodesomeKeyID  int
	Name           string
	Status         string
	GroupID        int
	RawKey         *string
	RawKeyStoredAt *string
	CreatedAt      string
	UpdatedAt      string
	LastSyncedAt   *string
}

type APIKeyRepository struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

type CreateAPIKeyParams struct {
	UserID        int64
	CodesomeKeyID int
	Name          string
	Status        string
	GroupID       int
	RawKey        string
}

func (r *APIKeyRepository) Create(ctx context.Context, params CreateAPIKeyParams) (*APIKey, error) {
	if params.UserID <= 0 {
		return nil, fmt.Errorf("user id must be positive")
	}
	if params.CodesomeKeyID <= 0 {
		return nil, fmt.Errorf("codesome key id must be positive")
	}
	if params.Name == "" {
		return nil, fmt.Errorf("api key name is required")
	}
	if !IsValidAPIKeyStatus(params.Status) {
		return nil, fmt.Errorf("invalid api key status: %s", params.Status)
	}
	if params.GroupID <= 0 {
		return nil, fmt.Errorf("group id must be positive")
	}

	now := nowString()
	rawKeyStoredAt := any(nil)
	if params.RawKey != "" {
		rawKeyStoredAt = now
	}
	res, err := r.db.ExecContext(ctx, `
INSERT INTO api_keys (
  user_id, codesome_key_id, name, status, group_id, raw_key, raw_key_stored_at,
  created_at, updated_at, last_synced_at
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
`, params.UserID, params.CodesomeKeyID, params.Name, params.Status, params.GroupID, nullableString(params.RawKey), rawKeyStoredAt, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get api key id: %w", err)
	}
	return r.GetByID(ctx, id)
}

func (r *APIKeyRepository) GetByID(ctx context.Context, id int64) (*APIKey, error) {
	return scanAPIKey(r.db.QueryRowContext(ctx, apiKeySelectSQL()+` WHERE id = ?`, id))
}

func (r *APIKeyRepository) GetLatestByUserID(ctx context.Context, userID int64) (*APIKey, error) {
	return scanAPIKey(r.db.QueryRowContext(ctx, apiKeySelectSQL()+` WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID))
}

type UpdateAPIKeyParams struct {
	Name    string
	Status  string
	GroupID int
}

func (r *APIKeyRepository) UpdateSynced(ctx context.Context, id int64, params UpdateAPIKeyParams) (*APIKey, error) {
	if id <= 0 {
		return nil, fmt.Errorf("api key id must be positive")
	}
	if params.Name == "" {
		return nil, fmt.Errorf("api key name is required")
	}
	if !IsValidAPIKeyStatus(params.Status) {
		return nil, fmt.Errorf("invalid api key status: %s", params.Status)
	}
	if params.GroupID <= 0 {
		return nil, fmt.Errorf("group id must be positive")
	}

	now := nowString()
	if _, err := r.db.ExecContext(ctx, `
UPDATE api_keys
SET name = ?, status = ?, group_id = ?, updated_at = ?, last_synced_at = ?
WHERE id = ?
`, params.Name, params.Status, params.GroupID, now, now, id); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
	}
	return r.GetByID(ctx, id)
}

func (r *APIKeyRepository) TouchSynced(ctx context.Context, id int64) (*APIKey, error) {
	if id <= 0 {
		return nil, fmt.Errorf("api key id must be positive")
	}
	now := nowString()
	if _, err := r.db.ExecContext(ctx, `
UPDATE api_keys
SET last_synced_at = ?
WHERE id = ?
`, now, id); err != nil {
		return nil, fmt.Errorf("touch api key sync: %w", err)
	}
	return r.GetByID(ctx, id)
}

func IsValidAPIKeyStatus(status string) bool {
	return status == APIKeyStatusActive || status == APIKeyStatusInactive
}

func apiKeySelectSQL() string {
	return `
SELECT
  id,
  user_id,
  codesome_key_id,
  name,
  status,
  group_id,
  raw_key,
  raw_key_stored_at,
  created_at,
  updated_at,
  last_synced_at
FROM api_keys`
}

func scanAPIKey(row *sql.Row) (*APIKey, error) {
	var key APIKey
	var rawKey sql.NullString
	var rawKeyStoredAt sql.NullString
	var lastSyncedAt sql.NullString
	if err := row.Scan(
		&key.ID,
		&key.UserID,
		&key.CodesomeKeyID,
		&key.Name,
		&key.Status,
		&key.GroupID,
		&rawKey,
		&rawKeyStoredAt,
		&key.CreatedAt,
		&key.UpdatedAt,
		&lastSyncedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("api key not found: %w", err)
		}
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	assignNullableAPIKeyFields(&key, rawKey, rawKeyStoredAt, lastSyncedAt)
	return &key, nil
}

func assignNullableAPIKeyFields(key *APIKey, rawKey sql.NullString, rawKeyStoredAt sql.NullString, lastSyncedAt sql.NullString) {
	if rawKey.Valid {
		key.RawKey = &rawKey.String
	}
	if rawKeyStoredAt.Valid {
		key.RawKeyStoredAt = &rawKeyStoredAt.String
	}
	if lastSyncedAt.Valid {
		key.LastSyncedAt = &lastSyncedAt.String
	}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
