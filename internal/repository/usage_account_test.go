package repository

import (
	"context"
	"testing"
)

func TestUsageAccountRepositoryEnsuresCodesomeAccounts(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	user, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice", FeishuOpenID: "ou_alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := NewAPIKeyRepository(userRepo.db).Create(ctx, CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: 6732,
		Name:          "Alice Key",
		Status:        APIKeyStatusActive,
		GroupID:       51,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	repo := NewUsageAccountRepository(userRepo.db)
	if err := repo.EnsureCodesomeAccounts(ctx); err != nil {
		t.Fatalf("ensure accounts: %v", err)
	}
	if err := repo.EnsureCodesomeAccounts(ctx); err != nil {
		t.Fatalf("ensure accounts again: %v", err)
	}

	account, err := repo.FindBySource(ctx, UsageSourceCodesome, "6732")
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if account == nil || account.UserID != user.ID || account.APIKeyID == nil || *account.APIKeyID != key.ID {
		t.Fatalf("unexpected account: %+v", account)
	}

	targets, err := repo.ListCodesomeUsageTargets(ctx)
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(targets) != 1 || targets[0].UsageAccountID != account.ID || targets[0].CodesomeKeyID != 6732 || targets[0].FeishuOpenID != "ou_alice" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

func TestUsageAccountRepositoryEnsureCodexAccount(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	ctx := context.Background()
	user, err := userRepo.Create(ctx, CreateUserParams{EmployeeNo: "E12345", Name: "Alice"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo := NewUsageAccountRepository(userRepo.db)
	account, err := repo.EnsureCodexAccount(ctx, user.ID, user.EmployeeNo, `{"CODEX_HOME":"~/.codex"}`)
	if err != nil {
		t.Fatalf("ensure codex account: %v", err)
	}
	again, err := repo.EnsureCodexAccount(ctx, user.ID, user.EmployeeNo, "")
	if err != nil {
		t.Fatalf("ensure codex account again: %v", err)
	}
	if again.ID != account.ID {
		t.Fatalf("expected idempotent account, got first=%d second=%d", account.ID, again.ID)
	}
	if account.Source != UsageSourceCodex || account.SourceAccountID != "E12345" || account.DisplayName != "Codex 官方订阅" {
		t.Fatalf("unexpected codex account: %+v", account)
	}
}
