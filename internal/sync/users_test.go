package syncer

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	codesomedb "codesome-usage-manager/internal/db"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestSyncUsersCreatesMissingKeyForActiveUser(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &fakeUserKeyService{
		createResult: &provider.CodesomeApiKeyWithSecret{
			CodesomeApiKey: provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 51, Status: "active"},
			Key:            "sk-test",
		},
	}

	results, err := NewUserSyncer(database, service, 51).SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].CodesomeKeyID != 6732 || results[0].RawKey != "sk-test" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.creates) != 1 || service.creates[0].name != "Alice" || service.creates[0].groupID != 51 {
		t.Fatalf("unexpected create calls: %+v", service.creates)
	}

	key, err := repository.NewAPIKeyRepository(database).GetLatestByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get api key: %v", err)
	}
	if key.CodesomeKeyID != 6732 || key.RawKey == nil || *key.RawKey != "sk-test" {
		t.Fatalf("unexpected stored api key: %+v", key)
	}
}

func TestSyncUsersCreatesMissingKeyWithRuntimeBestGroup(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &fakeUserKeyService{
		createResult: &provider.CodesomeApiKeyWithSecret{
			CodesomeApiKey: provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 60, Status: "active"},
			Key:            "sk-test",
		},
	}

	results, err := NewUserSyncer(database, service, 51).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return 60, nil
		}).
		SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].GroupID != 60 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.creates) != 1 || service.creates[0].groupID != 60 {
		t.Fatalf("expected create to use runtime best group, got %+v", service.creates)
	}
}

func TestSyncUsersCreateFallsBackToConfiguredDefaultGroup(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &fakeUserKeyService{
		createResult: &provider.CodesomeApiKeyWithSecret{
			CodesomeApiKey: provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 51, Status: "active"},
			Key:            "sk-test",
		},
	}

	results, err := NewUserSyncer(database, service, 51).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return 0, errors.New("subscription unavailable")
		}).
		SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].GroupID != 51 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.creates) != 1 || service.creates[0].groupID != 51 {
		t.Fatalf("expected create to fall back to configured default group, got %+v", service.creates)
	}
}

func TestSyncUsersCreateFailsWhenRuntimeBestGroupAndDefaultGroupUnavailable(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &fakeUserKeyService{}

	_, err := NewUserSyncer(database, service, 0).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return 0, errors.New("subscription unavailable")
		}).
		SyncUsers(ctx, UserSyncOptions{})
	if err == nil {
		t.Fatal("expected sync users to fail when runtime best group is unavailable")
	}
	if len(service.creates) != 0 {
		t.Fatalf("expected no create calls, got %+v", service.creates)
	}
}

func TestSyncUsersDryRunDoesNotCreateKey(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 51).SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if _, err := repository.NewAPIKeyRepository(database).GetLatestByUserID(ctx, user.ID); err == nil {
		t.Fatal("expected no api key to be stored")
	}
}

func TestSyncUsersDryRunPlansRuntimeGroupSelection(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 0).SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].GroupID != 0 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].Message == "" || !strings.Contains(results[0].Message, "真实运行时选择") {
		t.Fatalf("expected runtime group selection message, got %+v", results[0])
	}
}

func TestSyncUsersDryRunRuntimeGroupPlanOverridesConfiguredDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 51).
		WithRuntimeGroupSelectionPlan().
		SyncUsers(ctx, UserSyncOptions{DryRun: true, Full: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].GroupID != 0 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if !strings.Contains(results[0].Message, "真实运行时选择") {
		t.Fatalf("expected runtime group message, got %+v", results[0])
	}
}

func TestSyncUsersDryRunRuntimeGroupPlanForExistingKey(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 51).
		WithRuntimeGroupSelectionPlan().
		SyncUsers(ctx, UserSyncOptions{DryRun: true, Full: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "sync" || results[0].GroupID != 0 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if !strings.Contains(results[0].Message, "真实运行时") {
		t.Fatalf("expected runtime group message, got %+v", results[0])
	}
}

func TestSyncUsersDryRunRuntimeGroupPlanForUnchangedExistingKeyByDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 51).
		WithRuntimeGroupSelectionPlan().
		SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "sync" || results[0].GroupID != 0 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if !strings.Contains(results[0].Message, "真实运行时") {
		t.Fatalf("expected runtime group message, got %+v", results[0])
	}
}

func TestSyncUsersDryRunWithResolverSkipsUnchangedExistingKey(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 51).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return 51, nil
		}).
		SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "noop" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSyncUsersDryRunWithResolverPlansLiveGroupChange(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 51).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return 60, nil
		}).
		SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" || results[0].GroupID != 60 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSyncUsersDoesNotResolveDefaultGroupWhenInactiveUserNeedsNoKey(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		Status:     repository.UserStatusInactive,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	called := false
	results, err := NewUserSyncer(database, &fakeUserKeyService{}, 0).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			called = true
			return 0, nil
		}).
		SyncUsers(ctx, UserSyncOptions{EmployeeNo: "E12345"})
	if err != nil {
		t.Fatalf("sync inactive user: %v", err)
	}
	if called {
		t.Fatal("default group resolver should not be called")
	}
	if len(results) != 1 || results[0].Action != "noop" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSyncUsersDryRunInactiveUserKeepsExistingKeyGroup(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		Status:     repository.UserStatusInactive,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusInactive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 60).SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "noop" || results[0].GroupID != 51 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSyncUsersUpdatesExistingActiveKey(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	groupID := 60
	user, err := userRepo.Create(ctx, repository.CreateUserParams{
		EmployeeNo:      "E12345",
		Name:            "Alice",
		CodesomeGroupID: &groupID,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	keyRepo := repository.NewAPIKeyRepository(database)
	if _, err := keyRepo.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice Old",
		Status:        repository.APIKeyStatusInactive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 60, Status: "active"},
	}

	results, err := NewUserSyncer(database, service, 51).SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 {
		t.Fatalf("expected one update call, got %+v", service.updates)
	}
	update := service.updates[0].update
	if update.Name == nil || *update.Name != "Alice" || update.GroupID == nil || *update.GroupID != 60 || update.Status == nil || *update.Status != "active" {
		t.Fatalf("unexpected update payload: %+v", update)
	}
}

func TestSyncUsersUsesExistingKeyGroupWhenDefaultGroupMissing(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	results, err := NewUserSyncer(database, nil, 0).SyncUsers(ctx, UserSyncOptions{DryRun: true})
	if err != nil {
		t.Fatalf("sync users dry run: %v", err)
	}
	if len(results) != 1 || results[0].Action != "noop" || results[0].GroupID != 51 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSyncUsersSkipsUnchangedExistingKeyByDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 51, Status: "active"},
	}
	called := false

	results, err := NewUserSyncer(database, service, 0).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			called = true
			return 51, nil
		}).
		SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "noop" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 0 {
		t.Fatalf("expected no update calls, got %+v", service.updates)
	}
	if !called {
		t.Fatal("default group resolver should be called to detect live group changes")
	}
}

func TestSyncUsersReappliesExistingKeyState(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	keyRepo := repository.NewAPIKeyRepository(database)
	if _, err := keyRepo.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 51, Status: "active"},
	}

	results, err := NewUserSyncer(database, service, 51).SyncUsers(ctx, UserSyncOptions{Full: true})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "sync" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 {
		t.Fatalf("expected one update call, got %+v", service.updates)
	}
	update := service.updates[0].update
	if update.Name == nil || *update.Name != "Alice" || update.GroupID == nil || *update.GroupID != 51 || update.Status == nil || *update.Status != "active" {
		t.Fatalf("unexpected update payload: %+v", update)
	}
}

func TestSyncUsersSyncsExistingKeyWithoutLastSyncedAtByDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE api_keys SET last_synced_at = NULL WHERE id = ?`, key.ID); err != nil {
		t.Fatalf("clear last synced at: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 51, Status: "active"},
	}

	results, err := NewUserSyncer(database, service, 51).SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "sync" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 {
		t.Fatalf("expected one update call, got %+v", service.updates)
	}
}

func TestSyncUsersSyncsLocallyUpdatedUserByDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE users SET updated_at = ? WHERE id = ?`, "2099-01-01T00:00:00Z", user.ID); err != nil {
		t.Fatalf("update user timestamp: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 51, Status: "active"},
	}

	results, err := NewUserSyncer(database, service, 51).SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "sync" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 {
		t.Fatalf("expected one update call, got %+v", service.updates)
	}
}

func TestSyncUsersUpdatesConfiguredDefaultGroupChangeByDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 60, Status: "active"},
	}

	results, err := NewUserSyncer(database, service, 60).SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 {
		t.Fatalf("expected one update call, got %+v", service.updates)
	}
	update := service.updates[0].update
	if update.GroupID == nil || *update.GroupID != 60 {
		t.Fatalf("unexpected update payload: %+v", update)
	}
}

func TestSyncUsersUpdatesLiveDefaultGroupChangeByDefault(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	user, err := repository.NewUserRepository(database).Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repository.NewAPIKeyRepository(database).Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Name: "Alice", GroupID: 60, Status: "active"},
	}

	results, err := NewUserSyncer(database, service, 51).
		WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return 60, nil
		}).
		SyncUsers(ctx, UserSyncOptions{})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 {
		t.Fatalf("expected one update call, got %+v", service.updates)
	}
	update := service.updates[0].update
	if update.GroupID == nil || *update.GroupID != 60 {
		t.Fatalf("unexpected update payload: %+v", update)
	}
}

func TestSyncUsersDisablesDeletedUserKey(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepository(database)
	user, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	keyRepo := repository.NewAPIKeyRepository(database)
	key, err := keyRepo.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if _, err := userRepo.SoftDelete(ctx, "E12345"); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	service := &fakeUserKeyService{
		updateResult: &provider.CodesomeApiKey{ID: 6732, Status: "inactive"},
	}

	results, err := NewUserSyncer(database, service, 51).SyncUsers(ctx, UserSyncOptions{EmployeeNo: "E12345"})
	if err != nil {
		t.Fatalf("sync users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(service.updates) != 1 || service.updates[0].update.Status == nil || *service.updates[0].update.Status != "inactive" {
		t.Fatalf("unexpected update calls: %+v", service.updates)
	}
	stored, err := keyRepo.GetByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("get api key: %v", err)
	}
	if stored.Status != repository.APIKeyStatusInactive || stored.Name != "Alice" || stored.GroupID != 51 {
		t.Fatalf("unexpected stored key: %+v", stored)
	}
}

func newTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	database, err := codesomedb.Open(filepath.Join(t.TempDir(), "codesome-manager.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := codesomedb.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return database
}

type fakeUserKeyService struct {
	createResult *provider.CodesomeApiKeyWithSecret
	updateResult *provider.CodesomeApiKey
	creates      []fakeCreateCall
	updates      []fakeUpdateCall
}

type fakeCreateCall struct {
	name    string
	groupID int
}

type fakeUpdateCall struct {
	keyID  int
	update provider.CodesomeKeyUpdate
}

func (s *fakeUserKeyService) CreateKey(ctx context.Context, name string, groupID int) (*provider.CodesomeApiKeyWithSecret, error) {
	s.creates = append(s.creates, fakeCreateCall{name: name, groupID: groupID})
	return s.createResult, nil
}

func (s *fakeUserKeyService) UpdateKey(ctx context.Context, keyID int, update provider.CodesomeKeyUpdate) (*provider.CodesomeApiKey, error) {
	s.updates = append(s.updates, fakeUpdateCall{keyID: keyID, update: update})
	return s.updateResult, nil
}
