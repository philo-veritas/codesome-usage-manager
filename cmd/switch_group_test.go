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

func TestPrintGroupSwitchBatchResultsGroupsEquivalentResults(t *testing.T) {
	results := []provider.CodesomeGroupSwitchBatchResult{
		{
			KeyID:  6732,
			Name:   "Alice",
			Result: groupSwitchResult(51, "default", 120, "API Key 已绑定当前剩余额度最高的 group，无需切换"),
		},
		{
			KeyID:  6733,
			Name:   "Bob",
			Result: groupSwitchResult(51, "default", 120, "API Key 已绑定当前剩余额度最高的 group，无需切换"),
		},
		{
			KeyID:  6734,
			Name:   "Carol",
			Result: groupSwitchResult(51, "default", 120, "API Key 已绑定当前剩余额度最高的 group，无需切换"),
		},
		{
			KeyID:  6735,
			Name:   "Dave",
			Result: groupSwitchResult(51, "default", 120, "API Key 已绑定当前剩余额度最高的 group，无需切换"),
		},
	}

	got, hasErrors := collectGroupSwitchBatchOutput(results)
	if hasErrors {
		t.Fatal("expected no errors")
	}
	want := "API Keys 6732(Alice), 6733(Bob), 6734(Carol), ...（共 4 个） 未切换：API Key 已绑定当前剩余额度最高的 group，无需切换，当前 group 51(default) 剩余额度 $120.00\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPrintGroupSwitchBatchResultsKeepsDifferentGroupsSeparate(t *testing.T) {
	results := []provider.CodesomeGroupSwitchBatchResult{
		{
			KeyID:  6732,
			Result: groupSwitchResult(51, "", 120, "当前 group 剩余额度 $120.00，不低于阈值 $10.00"),
		},
		{
			KeyID:  6733,
			Result: groupSwitchResult(60, "", 80, "当前 group 剩余额度 $80.00，不低于阈值 $10.00"),
		},
	}

	got, _ := collectGroupSwitchBatchOutput(results)
	want := "API Key 6732 未切换：当前 group 剩余额度 $120.00，不低于阈值 $10.00，当前 group 51 剩余额度 $120.00\n" +
		"API Key 6733 未切换：当前 group 剩余额度 $80.00，不低于阈值 $10.00，当前 group 60 剩余额度 $80.00\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPrintGroupSwitchBatchResultsKeepsErrorsPerKey(t *testing.T) {
	results := []provider.CodesomeGroupSwitchBatchResult{
		{KeyID: 6732, Name: "Alice", Error: "not found"},
		{KeyID: 6733, Name: "Bob"},
	}

	got, hasErrors := collectGroupSwitchBatchOutput(results)
	if !hasErrors {
		t.Fatal("expected errors")
	}
	want := "API Key 6732(Alice) 执行失败：not found\n" +
		"API Key 6733(Bob) 执行失败：未返回执行结果\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPrintGroupSwitchBatchResultsGroupsSwitchedResults(t *testing.T) {
	results := []provider.CodesomeGroupSwitchBatchResult{
		{
			KeyID:  6732,
			Result: switchedGroupSwitchResult(6732),
		},
		{
			KeyID:  6733,
			Result: switchedGroupSwitchResult(6733),
		},
	}

	got, _ := collectGroupSwitchBatchOutput(results)
	want := "API Keys 6732, 6733 已切换 group: 51 -> 60，当前剩余额度 $5.00，目标剩余额度 $200.00\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func collectGroupSwitchBatchOutput(results []provider.CodesomeGroupSwitchBatchResult) (string, bool) {
	var got string
	hasErrors := printGroupSwitchBatchResultsWith(results, func(format string, args ...any) {
		got += fmt.Sprintf(format, args...)
	})
	return got, hasErrors
}

func groupSwitchResult(groupID int, groupName string, remaining float64, message string) *provider.CodesomeGroupSwitchResult {
	return &provider.CodesomeGroupSwitchResult{
		FromGroupID:         groupID,
		FromGroupName:       groupName,
		CurrentRemainingUSD: remaining,
		Message:             message,
	}
}

func switchedGroupSwitchResult(keyID int) *provider.CodesomeGroupSwitchResult {
	return &provider.CodesomeGroupSwitchResult{
		Switched:            true,
		FromGroupID:         51,
		ToGroupID:           60,
		CurrentRemainingUSD: 5,
		TargetRemainingUSD:  200,
		Message:             fmt.Sprintf("API Key %d 已切换到当前剩余额度最高的 group 60", keyID),
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
