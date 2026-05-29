package repository

import (
	"context"
	"testing"
)

func TestUserRepositoryCreateListUpdateAndDelete(t *testing.T) {
	teamRepo, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}

	groupID := 51
	user, err := userRepo.Create(ctx, CreateUserParams{
		EmployeeNo:      "E12345",
		Name:            "Alice",
		TeamCode:        "platform",
		CodesomeGroupID: &groupID,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.ID == 0 || user.EmployeeNo != "E12345" || user.Status != UserStatusActive {
		t.Fatalf("unexpected user: %+v", user)
	}
	if user.TeamCode == nil || *user.TeamCode != "platform" {
		t.Fatalf("expected platform team, got %+v", user.TeamCode)
	}
	if user.CodesomeGroupID == nil || *user.CodesomeGroupID != 51 {
		t.Fatalf("expected group id 51, got %+v", user.CodesomeGroupID)
	}

	users, err := userRepo.List(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].EmployeeNo != "E12345" {
		t.Fatalf("unexpected users: %+v", users)
	}

	status := UserStatusInactive
	updated, err := userRepo.Update(ctx, "E12345", UpdateUserParams{
		Status:       &status,
		ClearGroupID: true,
	})
	if err != nil {
		t.Fatalf("update user: %v", err)
	}
	if updated.Status != UserStatusInactive || updated.CodesomeGroupID != nil {
		t.Fatalf("unexpected updated user: %+v", updated)
	}

	deleted, err := userRepo.SoftDelete(ctx, "E12345")
	if err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if deleted.Status != UserStatusDeleted || deleted.DeletedAt == nil {
		t.Fatalf("expected soft-deleted user, got %+v", deleted)
	}
}

func TestUserRepositoryAllowsUserWithoutTeam(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	user, err := userRepo.Create(ctx, CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
	})
	if err != nil {
		t.Fatalf("create user without team: %v", err)
	}
	if user.TeamID != nil || user.TeamCode != nil {
		t.Fatalf("expected no team, got %+v", user)
	}
}

func TestUserRepositoryRejectsInactiveTeamForActiveUser(t *testing.T) {
	teamRepo, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	inactive := TeamStatusInactive
	if _, err := teamRepo.Update(ctx, "platform", UpdateTeamParams{Status: &inactive}); err != nil {
		t.Fatalf("deactivate team: %v", err)
	}

	if _, err := userRepo.Create(ctx, CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		TeamCode:   "platform",
	}); err == nil {
		t.Fatal("expected active user creation under inactive team to fail")
	}
}

func TestUserRepositoryRejectsActivatingUserUnderInactiveTeam(t *testing.T) {
	teamRepo, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	if _, err := userRepo.Create(ctx, CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		TeamCode:   "platform",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	inactiveUser := UserStatusInactive
	if _, err := userRepo.Update(ctx, "E12345", UpdateUserParams{Status: &inactiveUser}); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}
	inactiveTeam := TeamStatusInactive
	if _, err := teamRepo.Update(ctx, "platform", UpdateTeamParams{Status: &inactiveTeam}); err != nil {
		t.Fatalf("deactivate team: %v", err)
	}

	activeUser := UserStatusActive
	if _, err := userRepo.Update(ctx, "E12345", UpdateUserParams{Status: &activeUser}); err == nil {
		t.Fatal("expected activating user under inactive team to fail")
	}
}

func TestUserRepositoryRejectsInvalidStatus(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	status := "archived"
	if _, err := userRepo.Update(ctx, "E12345", UpdateUserParams{Status: &status}); err == nil {
		t.Fatal("expected invalid status to fail")
	}
}

func TestUserRepositoryCreatesInactiveUserUnderInactiveTeam(t *testing.T) {
	teamRepo, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	inactiveTeam := TeamStatusInactive
	if _, err := teamRepo.Update(ctx, "platform", UpdateTeamParams{Status: &inactiveTeam}); err != nil {
		t.Fatalf("deactivate team: %v", err)
	}

	user, err := userRepo.Create(ctx, CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		TeamCode:   "platform",
		Status:     UserStatusInactive,
	})
	if err != nil {
		t.Fatalf("create inactive user under inactive team: %v", err)
	}
	if user.Status != UserStatusInactive || user.TeamCode == nil || *user.TeamCode != "platform" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestUserRepositoryRejectsInvalidGroupIDOnCreate(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	groupID := 0
	if _, err := userRepo.Create(ctx, CreateUserParams{
		EmployeeNo:      "E12345",
		Name:            "Alice",
		CodesomeGroupID: &groupID,
	}); err == nil {
		t.Fatal("expected invalid group id to fail")
	}
}

func TestUserRepositoryRejectsUpdatingDeletedUser(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := userRepo.SoftDelete(ctx, "E12345"); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	status := UserStatusActive
	if _, err := userRepo.Update(ctx, "E12345", UpdateUserParams{Status: &status}); err == nil {
		t.Fatal("expected updating deleted user to fail")
	}
}

func TestUserRepositoryRejectsDeletedStatusOnUpdate(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	status := UserStatusDeleted
	if _, err := userRepo.Update(ctx, "E12345", UpdateUserParams{Status: &status}); err == nil {
		t.Fatal("expected deleted status update to fail")
	}
}

func TestUserRepositoryRejectsDuplicateEmployeeNo(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()

	if _, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice 2"}); err == nil {
		t.Fatal("expected duplicate employee no to fail")
	}
}

func newTestUserRepositories(t *testing.T) (*TeamRepository, *UserRepository) {
	t.Helper()

	teamRepo := newTestTeamRepository(t)
	return teamRepo, NewUserRepository(teamRepo.db)
}
