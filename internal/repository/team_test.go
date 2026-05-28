package repository

import (
	"context"
	"path/filepath"
	"testing"

	codesomedb "codesome-usage-manager/internal/db"
)

func TestTeamRepositoryCreateListAndUpdate(t *testing.T) {
	repo := newTestTeamRepository(t)
	ctx := context.Background()

	team, err := repo.Create(ctx, "platform", "Platform")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if team.ID == 0 || team.Code != "platform" || team.Name != "Platform" || team.Status != TeamStatusActive {
		t.Fatalf("unexpected team: %+v", team)
	}

	teams, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) != 1 || teams[0].Code != "platform" {
		t.Fatalf("unexpected teams: %+v", teams)
	}

	status := TeamStatusInactive
	updated, err := repo.Update(ctx, "platform", UpdateTeamParams{Status: &status})
	if err != nil {
		t.Fatalf("update team: %v", err)
	}
	if updated.Status != TeamStatusInactive {
		t.Fatalf("expected inactive team, got %+v", updated)
	}
}

func TestTeamRepositoryRejectsInvalidStatus(t *testing.T) {
	repo := newTestTeamRepository(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}

	status := "deleted"
	if _, err := repo.Update(ctx, "platform", UpdateTeamParams{Status: &status}); err == nil {
		t.Fatal("expected invalid status to fail")
	}
}

func TestTeamRepositoryRejectsDuplicateCode(t *testing.T) {
	repo := newTestTeamRepository(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	if _, err := repo.Create(ctx, "platform", "Platform 2"); err == nil {
		t.Fatal("expected duplicate code to fail")
	}
}

func newTestTeamRepository(t *testing.T) *TeamRepository {
	t.Helper()

	database, err := codesomedb.Open(filepath.Join(t.TempDir(), "codesome-manager.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := codesomedb.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return NewTeamRepository(database)
}
