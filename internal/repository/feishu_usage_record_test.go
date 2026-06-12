package repository

import (
	"context"
	"testing"
)

func TestFeishuUsageRecordRepositoryUpsertAndGet(t *testing.T) {
	_, userRepo := newTestUserRepositories(t)
	repo := NewFeishuUsageRecordRepository(userRepo.db)
	ctx := context.Background()

	recordID, err := repo.GetRecordID(ctx, "app1", "tbl1", "2026-06-11#6732")
	if err != nil {
		t.Fatalf("get missing record id: %v", err)
	}
	if recordID != "" {
		t.Fatalf("expected missing record id, got %q", recordID)
	}

	if err := repo.Upsert(ctx, "app1", "tbl1", "2026-06-11#6732", "rec_old"); err != nil {
		t.Fatalf("upsert old record id: %v", err)
	}
	if err := repo.Upsert(ctx, "app1", "tbl1", "2026-06-11#6732", "rec_new"); err != nil {
		t.Fatalf("upsert new record id: %v", err)
	}

	recordID, err = repo.GetRecordID(ctx, "app1", "tbl1", "2026-06-11#6732")
	if err != nil {
		t.Fatalf("get record id: %v", err)
	}
	if recordID != "rec_new" {
		t.Fatalf("expected updated record id, got %q", recordID)
	}
}
