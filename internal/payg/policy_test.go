package payg

import (
	"context"
	"path/filepath"
	"testing"

	"codesome-usage-manager/internal/config"
	codesomedb "codesome-usage-manager/internal/db"
)

func TestPercentileNearestRank(t *testing.T) {
	got := PercentileNearestRank([]float64{10, 50, 20, 40, 30}, 0.8)
	if got != 40 {
		t.Fatalf("expected p80 40, got %.2f", got)
	}
}

func TestLoadFallbackPolicyMarksMissingDatabaseAsHistoryError(t *testing.T) {
	cfg := &config.Config{Codesome: &config.CodesomeConfig{}}
	policy, err := LoadFallbackPolicyWithPath(context.Background(), cfg, filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if policy.HistoryLoadError == "" {
		t.Fatal("expected missing database to block pay as you go history guard")
	}
}

func TestLoadFallbackPolicyMarksEmptyHistoryAsHistoryError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codesome-manager.db")
	if err := codesomedb.Init(context.Background(), path); err != nil {
		t.Fatalf("init database: %v", err)
	}
	cfg := &config.Config{Codesome: &config.CodesomeConfig{}}

	policy, err := LoadFallbackPolicyWithPath(context.Background(), cfg, path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if policy.HistoryLoadError == "" {
		t.Fatal("expected empty history to block pay as you go history guard")
	}
}
