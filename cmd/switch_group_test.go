package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestPrintSubscriptionUsageSummary(t *testing.T) {
	var got string
	printSubscriptionUsageSummaryWith(
		provider.CodesomeSubscriptionUsageSummary{
			RemainingUSD: 410,
			LimitUSD:     450,
		},
		func(format string, args ...any) {
			got += fmt.Sprintf(format, args...)
		},
	)

	want := "今日总余额：$410.00 / $450.00\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLoadSwitchOnExhaustedAllKeyConfigsUsesActiveDBKeys(t *testing.T) {
	originalPath := dbPath
	defer func() { dbPath = originalPath }()

	dbPath = filepath.Join(t.TempDir(), "codesome-manager.db")
	ctx := context.Background()
	database, err := openLocalDatabase(ctx)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	userRepo := repository.NewUserRepository(database)
	activeUser, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create active user: %v", err)
	}
	inactiveUser, err := userRepo.Create(ctx, repository.CreateUserParams{EmployeeNo: "E99999", Name: "Bob"})
	if err != nil {
		t.Fatalf("create inactive user: %v", err)
	}
	inactiveStatus := repository.UserStatusInactive
	if _, err := userRepo.Update(ctx, "E99999", repository.UpdateUserParams{Status: &inactiveStatus}); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}
	keyRepo := repository.NewAPIKeyRepository(database)
	if _, err := keyRepo.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        activeUser.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create active key: %v", err)
	}
	if _, err := keyRepo.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        inactiveUser.ID,
		CodesomeKeyID: 6733,
		Name:          "Bob",
		Status:        repository.APIKeyStatusActive,
		GroupID:       51,
	}); err != nil {
		t.Fatalf("create inactive user key: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	configs, err := loadSwitchOnExhaustedAllKeyConfigs(ctx)
	if err != nil {
		t.Fatalf("load switch configs: %v", err)
	}
	if len(configs) != 1 || configs[0].ID != 6732 || configs[0].Name != "Alice" {
		t.Fatalf("unexpected switch configs: %+v", configs)
	}
}
