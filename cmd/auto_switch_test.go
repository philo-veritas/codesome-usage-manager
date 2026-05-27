package cmd

import (
	"testing"
	"time"

	"codesome-usage-manager/internal/provider"
)

func TestNextAutoSwitchIntervalReturnsMinWhenBelowThreshold(t *testing.T) {
	got := nextAutoSwitchInterval(9, 10, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), 0, 2*time.Minute, 2*time.Hour)
	if got != 2*time.Minute {
		t.Fatalf("expected min interval, got %s", got)
	}
}

func TestNextAutoSwitchIntervalClampsToMaxWhenRemainingIsHigh(t *testing.T) {
	got := nextAutoSwitchInterval(500, 10, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), 0, 2*time.Minute, 2*time.Hour)
	if got != 2*time.Hour {
		t.Fatalf("expected max interval, got %s", got)
	}
}

func TestNextAutoSwitchIntervalShrinksNearThreshold(t *testing.T) {
	got := nextAutoSwitchInterval(12, 10, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC), 0, 2*time.Minute, 2*time.Hour)
	if got != 2*time.Minute {
		t.Fatalf("expected min interval near threshold, got %s", got)
	}
}

func TestNextAutoSwitchIntervalUsesObservedBurnRate(t *testing.T) {
	now := time.Date(2026, 5, 20, 1, 0, 0, 0, time.UTC)
	withoutObserved := nextAutoSwitchInterval(100, 10, now, 0, 2*time.Minute, 2*time.Hour)
	withObserved := nextAutoSwitchInterval(100, 10, now, 120, 2*time.Minute, 2*time.Hour)
	if withObserved >= withoutObserved {
		t.Fatalf("expected observed burn rate to shorten interval, got %s >= %s", withObserved, withoutObserved)
	}
}

func TestRemainingFromSwitchResultsPrefersTargetRemainingAfterSwitch(t *testing.T) {
	results := []provider.CodesomeGroupSwitchBatchResult{
		{
			Result: &provider.CodesomeGroupSwitchResult{
				Switched:            true,
				CurrentRemainingUSD: 3,
				TargetRemainingUSD:  100,
			},
		},
		{
			Result: &provider.CodesomeGroupSwitchResult{
				CurrentRemainingUSD: 100,
			},
		},
	}

	got := remainingFromSwitchResults(results)
	if got != 100 {
		t.Fatalf("expected remaining 100, got %.2f", got)
	}
}

func TestDurationUntilNextDay(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 5, 20, 23, 30, 0, 0, loc)
	got := durationUntilNextDay(now)
	if got != 30*time.Minute {
		t.Fatalf("expected 30m, got %s", got)
	}
}
