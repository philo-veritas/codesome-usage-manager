package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	TeamStatusActive   = "active"
	TeamStatusInactive = "inactive"
)

type Team struct {
	ID        int64
	Code      string
	Name      string
	Status    string
	CreatedAt string
	UpdatedAt string
}

type TeamRepository struct {
	db *sql.DB
	tx *sql.Tx
}

func NewTeamRepository(db *sql.DB) *TeamRepository {
	return &TeamRepository{db: db}
}

func NewTeamRepositoryTx(tx *sql.Tx) *TeamRepository {
	return &TeamRepository{tx: tx}
}

func (r *TeamRepository) Create(ctx context.Context, code string, name string) (*Team, error) {
	if code == "" {
		return nil, fmt.Errorf("team code is required")
	}
	if name == "" {
		return nil, fmt.Errorf("team name is required")
	}

	now := nowString()
	res, err := r.execContext(ctx, `
INSERT INTO teams (code, name, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
`, code, name, TeamStatusActive, now, now)
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get team id: %w", err)
	}
	return r.GetByID(ctx, id)
}

func (r *TeamRepository) GetByID(ctx context.Context, id int64) (*Team, error) {
	return scanTeam(r.queryRowContext(ctx, `
SELECT id, code, name, status, created_at, updated_at
FROM teams
WHERE id = ?
`, id))
}

func (r *TeamRepository) GetByCode(ctx context.Context, code string) (*Team, error) {
	return scanTeam(r.queryRowContext(ctx, `
SELECT id, code, name, status, created_at, updated_at
FROM teams
WHERE code = ?
`, code))
}

type UpdateTeamParams struct {
	Name   *string
	Status *string
}

func (r *TeamRepository) Update(ctx context.Context, code string, params UpdateTeamParams) (*Team, error) {
	if code == "" {
		return nil, fmt.Errorf("team code is required")
	}
	if params.Name == nil && params.Status == nil {
		return nil, fmt.Errorf("no team fields to update")
	}
	if params.Status != nil && !IsValidTeamStatus(*params.Status) {
		return nil, fmt.Errorf("invalid team status: %s", *params.Status)
	}

	team, err := r.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	name := team.Name
	if params.Name != nil {
		if *params.Name == "" {
			return nil, fmt.Errorf("team name is required")
		}
		name = *params.Name
	}
	status := team.Status
	if params.Status != nil {
		status = *params.Status
	}

	if _, err := r.execContext(ctx, `
UPDATE teams
SET name = ?, status = ?, updated_at = ?
WHERE code = ?
`, name, status, nowString(), code); err != nil {
		return nil, fmt.Errorf("update team: %w", err)
	}

	return r.GetByCode(ctx, code)
}

func (r *TeamRepository) List(ctx context.Context) ([]Team, error) {
	rows, err := r.queryContext(ctx, `
SELECT id, code, name, status, created_at, updated_at
FROM teams
ORDER BY code
`)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []Team
	for rows.Next() {
		var team Team
		if err := rows.Scan(&team.ID, &team.Code, &team.Name, &team.Status, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teams: %w", err)
	}
	return teams, nil
}

func IsValidTeamStatus(status string) bool {
	return status == TeamStatusActive || status == TeamStatusInactive
}

func (r *TeamRepository) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if r.tx != nil {
		return r.tx.ExecContext(ctx, query, args...)
	}
	return r.db.ExecContext(ctx, query, args...)
}

func (r *TeamRepository) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if r.tx != nil {
		return r.tx.QueryContext(ctx, query, args...)
	}
	return r.db.QueryContext(ctx, query, args...)
}

func (r *TeamRepository) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if r.tx != nil {
		return r.tx.QueryRowContext(ctx, query, args...)
	}
	return r.db.QueryRowContext(ctx, query, args...)
}

func scanTeam(row *sql.Row) (*Team, error) {
	var team Team
	if err := row.Scan(&team.ID, &team.Code, &team.Name, &team.Status, &team.CreatedAt, &team.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("team not found")
		}
		return nil, fmt.Errorf("scan team: %w", err)
	}
	return &team, nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
