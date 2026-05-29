package syncer

import (
	"context"
	"strings"
	"testing"

	"codesome-usage-manager/internal/repository"
)

func TestUserCSVImporterDryRunDoesNotWrite(t *testing.T) {
	database := newTestDatabase(t)
	csvData := `employee_no,name,team,group_id,status
E12345,Alice,,51,active
`

	results, err := NewUserCSVImporter(database).ImportCSV(context.Background(), strings.NewReader(csvData), ImportUsersOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run import users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].EmployeeNo != "E12345" {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
	if _, err := repository.NewUserRepository(database).GetByEmployeeNo(context.Background(), "E12345"); err == nil {
		t.Fatal("expected dry-run to avoid writing user")
	}
}

func TestUserCSVImporterDryRunValidatesTeams(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	teamRepo := repository.NewTeamRepository(database)
	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	inactive := repository.TeamStatusInactive
	if _, err := teamRepo.Update(ctx, "platform", repository.UpdateTeamParams{Status: &inactive}); err != nil {
		t.Fatalf("deactivate team: %v", err)
	}

	csvData := `employee_no,name,team,status
E12345,Alice,platform,active
`
	if _, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{DryRun: true}); err == nil {
		t.Fatal("expected dry-run to reject active user under inactive team")
	}

	missingTeamCSV := `employee_no,name,team,status
E12345,Alice,missing,active
`
	if _, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(missingTeamCSV), ImportUsersOptions{DryRun: true}); err == nil {
		t.Fatal("expected dry-run to reject missing team")
	}
}

func TestUserCSVImporterCreatesAndUpdatesUsers(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	teamRepo := repository.NewTeamRepository(database)
	if _, err := teamRepo.Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	if _, err := teamRepo.Create(ctx, "infra", "Infra"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	userRepo := repository.NewUserRepository(database)
	groupID := 51
	if _, err := userRepo.Create(ctx, repository.CreateUserParams{
		EmployeeNo:      "E12345",
		Name:            "Alice Old",
		TeamCode:        "platform",
		CodesomeGroupID: &groupID,
	}); err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	csvData := `employee_no,name,team,group_id,status
E12345,Alice,infra,,inactive
E12346,Bob,platform,60,active
`
	results, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{})
	if err != nil {
		t.Fatalf("import users: %v", err)
	}
	if len(results) != 2 || results[0].Action != "update" || results[1].Action != "create" {
		t.Fatalf("unexpected import results: %+v", results)
	}

	alice, err := userRepo.GetByEmployeeNo(ctx, "E12345")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if alice.Name != "Alice" || alice.Status != repository.UserStatusInactive || alice.TeamCode == nil || *alice.TeamCode != "infra" || alice.CodesomeGroupID != nil {
		t.Fatalf("unexpected alice: %+v", alice)
	}
	bob, err := userRepo.GetByEmployeeNo(ctx, "E12346")
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	if bob.TeamCode == nil || *bob.TeamCode != "platform" || bob.CodesomeGroupID == nil || *bob.CodesomeGroupID != 60 {
		t.Fatalf("unexpected bob: %+v", bob)
	}

	results, err = NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{})
	if err != nil {
		t.Fatalf("second import users: %v", err)
	}
	if len(results) != 2 || results[0].Action != "skip" || results[1].Action != "skip" {
		t.Fatalf("expected idempotent skips, got %+v", results)
	}
}

func TestUserCSVImporterValidatesAllRowsBeforeWriting(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewTeamRepository(database).Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}

	csvData := `employee_no,name,team
E12345,Alice,platform
E12346,Bob,missing
`
	if _, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{}); err == nil {
		t.Fatal("expected import with missing team to fail")
	}
	if _, err := repository.NewUserRepository(database).GetByEmployeeNo(ctx, "E12345"); err == nil {
		t.Fatal("expected failed import to avoid partial user writes")
	}
}

func TestUserCSVImporterRollsBackFailedWrites(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, `
CREATE TRIGGER fail_second_user_import
BEFORE INSERT ON users
WHEN NEW.employee_no = 'E12346'
BEGIN
  SELECT RAISE(ABORT, 'forced import failure');
END
`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	csvData := `employee_no,name
E12345,Alice
E12346,Bob
`
	if _, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{}); err == nil {
		t.Fatal("expected import write failure")
	}
	if _, err := repository.NewUserRepository(database).GetByEmployeeNo(ctx, "E12345"); err == nil {
		t.Fatal("expected failed import transaction to roll back first user")
	}
}

func TestUserCSVImporterBlankStatusDoesNotReactivateExistingUser(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	if _, err := userRepo.Create(ctx, repository.CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		Status:     repository.UserStatusInactive,
	}); err != nil {
		t.Fatalf("create inactive user: %v", err)
	}

	csvData := `employee_no,name,status
E12345,Alice 2,
`
	results, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{})
	if err != nil {
		t.Fatalf("import users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" || results[0].Status != "" {
		t.Fatalf("unexpected import results: %+v", results)
	}
	user, err := userRepo.GetByEmployeeNo(ctx, "E12345")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.Name != "Alice 2" || user.Status != repository.UserStatusInactive {
		t.Fatalf("blank status should not reactivate user, got %+v", user)
	}
}

func TestUserCSVImporterRejectsDeletedExistingUser(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	if _, err := userRepo.Create(ctx, repository.CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := userRepo.SoftDelete(ctx, "E12345"); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	csvData := `employee_no,name
E12345,Alice
`
	if _, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{}); err == nil {
		t.Fatal("expected deleted user import to fail")
	}
	if _, err := NewUserCSVImporter(database).ImportCSV(ctx, strings.NewReader(csvData), ImportUsersOptions{DryRun: true}); err == nil {
		t.Fatal("expected deleted user dry-run import to fail")
	}
}

func TestUserCSVImporterValidatesInput(t *testing.T) {
	database := newTestDatabase(t)

	if _, err := NewUserCSVImporter(database).ImportCSV(context.Background(), strings.NewReader("name\nAlice\n"), ImportUsersOptions{}); err == nil {
		t.Fatal("expected missing employee_no header to fail")
	}
	if _, err := NewUserCSVImporter(database).ImportCSV(context.Background(), strings.NewReader("employee_no,name\nE12345,\n"), ImportUsersOptions{}); err == nil {
		t.Fatal("expected missing name to fail")
	}
	if _, err := NewUserCSVImporter(database).ImportCSV(context.Background(), strings.NewReader("employee_no,name,status\nE12345,Alice,deleted\n"), ImportUsersOptions{}); err == nil {
		t.Fatal("expected deleted status to fail")
	}
	if _, err := NewUserCSVImporter(database).ImportCSV(context.Background(), strings.NewReader("employee_no,name\nE12345,Alice\nE12345,Alice 2\n"), ImportUsersOptions{}); err == nil {
		t.Fatal("expected duplicate employee_no to fail")
	}
}
