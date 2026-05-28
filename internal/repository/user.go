package repository

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	UserStatusActive   = "active"
	UserStatusInactive = "inactive"
	UserStatusDeleted  = "deleted"
)

type User struct {
	ID              int64
	EmployeeNo      string
	Name            string
	TeamID          *int64
	TeamCode        *string
	Status          string
	CodesomeGroupID *int
	CreatedAt       string
	UpdatedAt       string
	DeletedAt       *string
}

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

type CreateUserParams struct {
	EmployeeNo      string
	Name            string
	TeamCode        string
	CodesomeGroupID *int
}

func (r *UserRepository) Create(ctx context.Context, params CreateUserParams) (*User, error) {
	if params.EmployeeNo == "" {
		return nil, fmt.Errorf("employee no is required")
	}
	if params.Name == "" {
		return nil, fmt.Errorf("user name is required")
	}
	if params.CodesomeGroupID != nil && *params.CodesomeGroupID <= 0 {
		return nil, fmt.Errorf("codesome group id must be positive")
	}

	teamID, err := r.resolveTeamIDForActiveUser(ctx, params.TeamCode)
	if err != nil {
		return nil, err
	}

	now := nowString()
	res, err := r.db.ExecContext(ctx, `
INSERT INTO users (
  employee_no, name, team_id, status, codesome_group_id, created_at, updated_at
) VALUES (
  ?, ?, ?, ?, ?, ?, ?
)
`, params.EmployeeNo, params.Name, nullableInt64(teamID), UserStatusActive, nullableInt(params.CodesomeGroupID), now, now)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	return r.GetByID(ctx, id)
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(r.db.QueryRowContext(ctx, userSelectSQL()+` WHERE users.id = ?`, id))
}

func (r *UserRepository) GetByEmployeeNo(ctx context.Context, employeeNo string) (*User, error) {
	return scanUser(r.db.QueryRowContext(ctx, userSelectSQL()+` WHERE users.employee_no = ?`, employeeNo))
}

type UpdateUserParams struct {
	Name            *string
	TeamCode        *string
	Status          *string
	CodesomeGroupID *int
	ClearGroupID    bool
}

func (r *UserRepository) Update(ctx context.Context, employeeNo string, params UpdateUserParams) (*User, error) {
	if employeeNo == "" {
		return nil, fmt.Errorf("employee no is required")
	}
	if params.Name == nil && params.TeamCode == nil && params.Status == nil && params.CodesomeGroupID == nil && !params.ClearGroupID {
		return nil, fmt.Errorf("no user fields to update")
	}
	if params.Status != nil && !IsValidUserStatus(*params.Status) {
		return nil, fmt.Errorf("invalid user status: %s", *params.Status)
	}
	if params.Status != nil && *params.Status == UserStatusDeleted {
		return nil, fmt.Errorf("use delete to soft-delete user")
	}
	if params.CodesomeGroupID != nil && *params.CodesomeGroupID <= 0 {
		return nil, fmt.Errorf("codesome group id must be positive")
	}

	user, err := r.GetByEmployeeNo(ctx, employeeNo)
	if err != nil {
		return nil, err
	}
	if user.Status == UserStatusDeleted {
		return nil, fmt.Errorf("deleted user cannot be updated")
	}

	name := user.Name
	if params.Name != nil {
		if *params.Name == "" {
			return nil, fmt.Errorf("user name is required")
		}
		name = *params.Name
	}

	status := user.Status
	if params.Status != nil {
		status = *params.Status
	}

	teamID := user.TeamID
	if params.TeamCode != nil {
		resolvedTeamID, err := r.resolveTeamIDForStatus(ctx, *params.TeamCode, status)
		if err != nil {
			return nil, err
		}
		teamID = resolvedTeamID
	} else if status == UserStatusActive && teamID != nil {
		if err := r.ensureTeamActive(ctx, *teamID); err != nil {
			return nil, err
		}
	}

	codesomeGroupID := user.CodesomeGroupID
	if params.ClearGroupID {
		codesomeGroupID = nil
	}
	if params.CodesomeGroupID != nil {
		codesomeGroupID = params.CodesomeGroupID
	}

	if _, err := r.db.ExecContext(ctx, `
UPDATE users
SET name = ?, team_id = ?, status = ?, codesome_group_id = ?, updated_at = ?
WHERE employee_no = ?
`, name, nullableInt64(teamID), status, nullableInt(codesomeGroupID), nowString(), employeeNo); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return r.GetByEmployeeNo(ctx, employeeNo)
}

func (r *UserRepository) SoftDelete(ctx context.Context, employeeNo string) (*User, error) {
	if employeeNo == "" {
		return nil, fmt.Errorf("employee no is required")
	}
	if _, err := r.GetByEmployeeNo(ctx, employeeNo); err != nil {
		return nil, err
	}

	now := nowString()
	if _, err := r.db.ExecContext(ctx, `
UPDATE users
SET status = ?, updated_at = ?, deleted_at = ?
WHERE employee_no = ?
`, UserStatusDeleted, now, now, employeeNo); err != nil {
		return nil, fmt.Errorf("delete user: %w", err)
	}
	return r.GetByEmployeeNo(ctx, employeeNo)
}

func (r *UserRepository) List(ctx context.Context) ([]User, error) {
	rows, err := r.db.QueryContext(ctx, userSelectSQL()+` ORDER BY users.employee_no`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		user, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (r *UserRepository) resolveTeamIDForActiveUser(ctx context.Context, teamCode string) (*int64, error) {
	return r.resolveTeamIDForStatus(ctx, teamCode, UserStatusActive)
}

func (r *UserRepository) resolveTeamIDForStatus(ctx context.Context, teamCode string, status string) (*int64, error) {
	if teamCode == "" {
		return nil, nil
	}
	team, err := NewTeamRepository(r.db).GetByCode(ctx, teamCode)
	if err != nil {
		return nil, err
	}
	if status == UserStatusActive && team.Status != TeamStatusActive {
		return nil, fmt.Errorf("active user cannot belong to inactive team: %s", teamCode)
	}
	return &team.ID, nil
}

func (r *UserRepository) ensureTeamActive(ctx context.Context, teamID int64) error {
	team, err := NewTeamRepository(r.db).GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	if team.Status != TeamStatusActive {
		return fmt.Errorf("active user cannot belong to inactive team: %s", team.Code)
	}
	return nil
}

func IsValidUserStatus(status string) bool {
	return status == UserStatusActive || status == UserStatusInactive || status == UserStatusDeleted
}

func userSelectSQL() string {
	return `
SELECT
  users.id,
  users.employee_no,
  users.name,
  users.team_id,
  teams.code,
  users.status,
  users.codesome_group_id,
  users.created_at,
  users.updated_at,
  users.deleted_at
FROM users
LEFT JOIN teams ON users.team_id = teams.id`
}

func scanUser(row *sql.Row) (*User, error) {
	var user User
	var teamID sql.NullInt64
	var teamCode sql.NullString
	var codesomeGroupID sql.NullInt64
	var deletedAt sql.NullString
	if err := row.Scan(
		&user.ID,
		&user.EmployeeNo,
		&user.Name,
		&teamID,
		&teamCode,
		&user.Status,
		&codesomeGroupID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	assignNullableUserFields(&user, teamID, teamCode, codesomeGroupID, deletedAt)
	return &user, nil
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUserRows(row userScanner) (*User, error) {
	var user User
	var teamID sql.NullInt64
	var teamCode sql.NullString
	var codesomeGroupID sql.NullInt64
	var deletedAt sql.NullString
	if err := row.Scan(
		&user.ID,
		&user.EmployeeNo,
		&user.Name,
		&teamID,
		&teamCode,
		&user.Status,
		&codesomeGroupID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	); err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	assignNullableUserFields(&user, teamID, teamCode, codesomeGroupID, deletedAt)
	return &user, nil
}

func assignNullableUserFields(user *User, teamID sql.NullInt64, teamCode sql.NullString, codesomeGroupID sql.NullInt64, deletedAt sql.NullString) {
	if teamID.Valid {
		user.TeamID = &teamID.Int64
	}
	if teamCode.Valid {
		user.TeamCode = &teamCode.String
	}
	if codesomeGroupID.Valid {
		groupID := int(codesomeGroupID.Int64)
		user.CodesomeGroupID = &groupID
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.String
	}
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
