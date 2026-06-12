package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

type APIKeyExportRow struct {
	EmployeeNo    string
	UserName      string
	TeamCode      *string
	FeishuOpenID  string
	KeyName       string
	CodesomeKeyID int
	RawKey        *string
	Status        string
}

type APIKeyUsageTarget struct {
	ID            int64
	CodesomeKeyID int
	Name          string
	UserStatus    string
	FeishuOpenID  string
}

type APIKeySwitchTarget struct {
	CodesomeKeyID int
	Name          string
}

type ListAPIKeyExportRowsParams struct {
	EmployeeNo      string
	TeamCode        string
	IncludeInactive bool
}

type ListAPIKeyDailyUsageTargetsParams struct {
	IncludeInactive bool
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

func (r *APIKeyRepository) GetByCodesomeKeyID(ctx context.Context, codesomeKeyID int) (*APIKey, error) {
	return scanAPIKey(r.db.QueryRowContext(ctx, apiKeySelectSQL()+` WHERE codesome_key_id = ?`, codesomeKeyID))
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

func (r *APIKeyRepository) ListExportRows(ctx context.Context, params ListAPIKeyExportRowsParams) ([]APIKeyExportRow, error) {
	conditions := []string{
		"users.status != ?",
	}
	args := []any{UserStatusDeleted}
	if !params.IncludeInactive {
		conditions = append(conditions, "users.status = ?", "api_keys.status = ?")
		args = append(args, UserStatusActive, APIKeyStatusActive)
	}
	if params.EmployeeNo != "" {
		conditions = append(conditions, "users.employee_no = ?")
		args = append(args, params.EmployeeNo)
	}
	if params.TeamCode != "" {
		conditions = append(conditions, "teams.code = ?")
		args = append(args, params.TeamCode)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
  users.employee_no,
  users.name,
  teams.code,
  users.feishu_open_id,
  api_keys.name,
  api_keys.codesome_key_id,
  api_keys.raw_key,
  api_keys.status
FROM api_keys
JOIN users ON api_keys.user_id = users.id
LEFT JOIN teams ON users.team_id = teams.id
WHERE `+strings.Join(conditions, " AND ")+`
ORDER BY users.employee_no, api_keys.id
`, args...)
	if err != nil {
		return nil, fmt.Errorf("list api key export rows: %w", err)
	}
	defer rows.Close()

	var result []APIKeyExportRow
	for rows.Next() {
		var row APIKeyExportRow
		var teamCode sql.NullString
		var feishuOpenID sql.NullString
		var rawKey sql.NullString
		if err := rows.Scan(
			&row.EmployeeNo,
			&row.UserName,
			&teamCode,
			&feishuOpenID,
			&row.KeyName,
			&row.CodesomeKeyID,
			&rawKey,
			&row.Status,
		); err != nil {
			return nil, fmt.Errorf("scan api key export row: %w", err)
		}
		if teamCode.Valid {
			row.TeamCode = &teamCode.String
		}
		if feishuOpenID.Valid {
			row.FeishuOpenID = feishuOpenID.String
		}
		if rawKey.Valid {
			row.RawKey = &rawKey.String
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key export rows: %w", err)
	}
	return result, nil
}

func (r *APIKeyRepository) ListUsageTargets(ctx context.Context) ([]APIKeyUsageTarget, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  api_keys.id,
  api_keys.codesome_key_id,
  api_keys.name,
  users.status,
  users.feishu_open_id
FROM api_keys
JOIN users ON api_keys.user_id = users.id
WHERE users.status != ?
ORDER BY api_keys.id
`, UserStatusDeleted)
	if err != nil {
		return nil, fmt.Errorf("list api key usage targets: %w", err)
	}
	defer rows.Close()

	var result []APIKeyUsageTarget
	for rows.Next() {
		var target APIKeyUsageTarget
		var feishuOpenID sql.NullString
		if err := rows.Scan(&target.ID, &target.CodesomeKeyID, &target.Name, &target.UserStatus, &feishuOpenID); err != nil {
			return nil, fmt.Errorf("scan api key usage target: %w", err)
		}
		if feishuOpenID.Valid {
			target.FeishuOpenID = feishuOpenID.String
		}
		result = append(result, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key usage targets: %w", err)
	}
	return result, nil
}

func (r *APIKeyRepository) ListDailyUsageTargets(ctx context.Context, params ListAPIKeyDailyUsageTargetsParams) ([]APIKeyUsageTarget, error) {
	conditions := []string{"users.status != ?"}
	args := []any{UserStatusDeleted}
	if !params.IncludeInactive {
		conditions = append(conditions, "users.status = ?", "api_keys.status = ?")
		args = append(args, UserStatusActive, APIKeyStatusActive)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
  api_keys.id,
  api_keys.codesome_key_id,
  api_keys.name,
  users.status,
  users.feishu_open_id
FROM api_keys
JOIN users ON api_keys.user_id = users.id
WHERE `+strings.Join(conditions, " AND ")+`
ORDER BY api_keys.id
`, args...)
	if err != nil {
		return nil, fmt.Errorf("list api key daily usage targets: %w", err)
	}
	defer rows.Close()

	var result []APIKeyUsageTarget
	for rows.Next() {
		var target APIKeyUsageTarget
		var feishuOpenID sql.NullString
		if err := rows.Scan(&target.ID, &target.CodesomeKeyID, &target.Name, &target.UserStatus, &feishuOpenID); err != nil {
			return nil, fmt.Errorf("scan api key daily usage target: %w", err)
		}
		if feishuOpenID.Valid {
			target.FeishuOpenID = feishuOpenID.String
		}
		result = append(result, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key daily usage targets: %w", err)
	}
	return result, nil
}

func (r *APIKeyRepository) ListActiveSwitchTargets(ctx context.Context) ([]APIKeySwitchTarget, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  api_keys.codesome_key_id,
  api_keys.name
FROM api_keys
JOIN users ON api_keys.user_id = users.id
WHERE users.status = ? AND api_keys.status = ?
ORDER BY api_keys.id
`, UserStatusActive, APIKeyStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list api key switch targets: %w", err)
	}
	defer rows.Close()

	var result []APIKeySwitchTarget
	for rows.Next() {
		var target APIKeySwitchTarget
		if err := rows.Scan(&target.CodesomeKeyID, &target.Name); err != nil {
			return nil, fmt.Errorf("scan api key switch target: %w", err)
		}
		result = append(result, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key switch targets: %w", err)
	}
	return result, nil
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
