package provider

import (
	"encoding/json"
	"os"
	"testing"

	"codesome-usage-manager/internal/config"
)

func testSubscription(groupID int, name string, limit float64, used float64, status string) CodesomeSubscription {
	return CodesomeSubscription{
		Status:        status,
		DailyUsageUSD: used,
		Group: &CodesomeGroup{
			ID:            groupID,
			Name:          name,
			DailyLimitUSD: limit,
		},
	}
}

func TestBestAvailableSubscriptionChoosesLargestRemaining(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 120, "active"),
		testSubscription(60, "large", 330, 10, "active"),
		testSubscription(51, "small", 30, 0, "active"),
	}

	target, remaining, ok := bestAvailableSubscription(subs, 64)
	if !ok {
		t.Fatal("expected target subscription")
	}
	if target.Group.ID != 60 {
		t.Fatalf("expected group 60, got %d", target.Group.ID)
	}
	if remaining != 320 {
		t.Fatalf("expected remaining 320, got %.2f", remaining)
	}
}

func TestBestAvailableSubscriptionSkipsCurrentAndExhausted(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 0, "active"),
		testSubscription(60, "exhausted", 330, 330, "active"),
		testSubscription(51, "inactive", 30, 0, "inactive"),
	}

	_, _, ok := bestAvailableSubscription(subs, 64)
	if ok {
		t.Fatal("expected no target subscription")
	}
}

func TestBestAvailableSubscriptionTieUsesLowerGroupID(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 120, "active"),
		testSubscription(60, "group60", 100, 50, "active"),
		testSubscription(51, "group51", 80, 30, "active"),
	}

	target, _, ok := bestAvailableSubscription(subs, 64)
	if !ok {
		t.Fatal("expected target subscription")
	}
	if target.Group.ID != 51 {
		t.Fatalf("expected lower group ID 51 for tie, got %d", target.Group.ID)
	}
}

func TestBestSubscriptionChoosesLargestRemaining(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "small", 120, 20, "active"),
		testSubscription(60, "large", 330, 10, "active"),
		testSubscription(51, "inactive", 500, 0, "inactive"),
	}

	target, remaining, ok := bestSubscription(subs)
	if !ok {
		t.Fatal("expected target subscription")
	}
	if target.Group.ID != 60 {
		t.Fatalf("expected group 60, got %d", target.Group.ID)
	}
	if remaining != 320 {
		t.Fatalf("expected remaining 320, got %.2f", remaining)
	}
}

func TestBestSubscriptionTieUsesLowerGroupID(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "group64", 100, 50, "active"),
		testSubscription(60, "group60", 100, 50, "active"),
	}

	target, _, ok := bestSubscription(subs)
	if !ok {
		t.Fatal("expected target subscription")
	}
	if target.Group.ID != 60 {
		t.Fatalf("expected lower group ID 60 for tie, got %d", target.Group.ID)
	}
}

func TestPlanSwitchOnExhaustedNoSwitchWhenCurrentHasRemaining(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 10, "active"),
		testSubscription(60, "large", 330, 10, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != nil {
		t.Fatal("expected no switch target")
	}
	if result.Switched {
		t.Fatal("expected no switch")
	}
	if result.CurrentRemainingUSD != 110 {
		t.Fatalf("expected current remaining 110, got %.2f", result.CurrentRemainingUSD)
	}
}

func TestPlanSwitchOnExhaustedSwitchesWhenCurrentExhausted(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 120, "active"),
		testSubscription(60, "large", 330, 10, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target == nil {
		t.Fatal("expected switch target")
	}
	if !result.Switched {
		t.Fatal("expected switch")
	}
	if result.ToGroupID != 60 {
		t.Fatalf("expected target group 60, got %d", result.ToGroupID)
	}
}

func TestPlanSwitchOnExhaustedErrorsWithoutAvailableTarget(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 120, "active"),
		testSubscription(60, "exhausted", 330, 330, "active"),
	}

	_, _, err := planSwitchOnExhausted(6732, 64, "current", subs, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPlanSwitchOnExhaustedSwitchesWhenBelowThreshold(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 115, "active"),
		testSubscription(60, "large", 330, 10, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target == nil {
		t.Fatal("expected switch target")
	}
	if !result.Switched {
		t.Fatal("expected switch")
	}
	if result.CurrentRemainingUSD != 5 {
		t.Fatalf("expected current remaining 5, got %.2f", result.CurrentRemainingUSD)
	}
	if result.ToGroupID != 60 {
		t.Fatalf("expected target group 60, got %d", result.ToGroupID)
	}
}

func TestPlanSwitchOnExhaustedDoesNotSwitchBelowThresholdToLowerRemaining(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 115, "active"),
		testSubscription(60, "lower", 330, 329, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != nil {
		t.Fatal("expected no switch target")
	}
	if result.Switched {
		t.Fatal("expected no switch")
	}
	if result.CurrentRemainingUSD != 5 {
		t.Fatalf("expected current remaining 5, got %.2f", result.CurrentRemainingUSD)
	}
}

func TestPlanSwitchOnExhaustedDoesNotErrorBelowThresholdWithoutBetterTarget(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 115, "active"),
		testSubscription(60, "exhausted", 330, 330, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != nil {
		t.Fatal("expected no switch target")
	}
	if result.Switched {
		t.Fatal("expected no switch")
	}
	if result.CurrentRemainingUSD != 5 {
		t.Fatalf("expected current remaining 5, got %.2f", result.CurrentRemainingUSD)
	}
}

func TestPlanSwitchOnExhaustedSwitchesBelowThresholdToHigherRemaining(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 115, "active"),
		testSubscription(60, "higher", 330, 322, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target == nil {
		t.Fatal("expected switch target")
	}
	if !result.Switched {
		t.Fatal("expected switch")
	}
	if result.TargetRemainingUSD != 8 {
		t.Fatalf("expected target remaining 8, got %.2f", result.TargetRemainingUSD)
	}
	if result.ToGroupID != 60 {
		t.Fatalf("expected target group 60, got %d", result.ToGroupID)
	}
}

func TestPlanSwitchOnExhaustedDoesNotSwitchWhenEqualThreshold(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 110, "active"),
		testSubscription(60, "large", 330, 10, "active"),
	}

	result, target, err := planSwitchOnExhausted(6732, 64, "current", subs, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != nil {
		t.Fatal("expected no switch target")
	}
	if result.Switched {
		t.Fatal("expected no switch")
	}
	if result.CurrentRemainingUSD != 10 {
		t.Fatalf("expected current remaining 10, got %.2f", result.CurrentRemainingUSD)
	}
}

func TestSummarizeSubscriptionsUsesActiveSubscriptions(t *testing.T) {
	subs := []CodesomeSubscription{
		testSubscription(64, "current", 120, 10, "active"),
		testSubscription(60, "large", 330, 30, "active"),
		testSubscription(51, "inactive", 500, 0, "inactive"),
	}

	got := summarizeSubscriptions(subs)
	if got.RemainingUSD != 410 {
		t.Fatalf("expected remaining 410, got %.2f", got.RemainingUSD)
	}
	if got.LimitUSD != 450 {
		t.Fatalf("expected limit 450, got %.2f", got.LimitUSD)
	}
}

func TestActiveCodesomeKeysFiltersInactiveKeys(t *testing.T) {
	keys := []CodesomeApiKey{
		{ID: 6732, Name: "active", Status: "active"},
		{ID: 9356, Name: "inactive", Status: "inactive"},
		{ID: 2085, Name: "deleted", Status: "deleted"},
		{ID: 1111, Name: "empty"},
	}

	got := activeCodesomeKeys(keys)
	if len(got) != 1 {
		t.Fatalf("expected one active key, got %+v", got)
	}
	if got[0].ID != 6732 {
		t.Fatalf("expected active key 6732, got %+v", got[0])
	}
}

func TestSanitizeCodesomeApiKeyForCacheDropsRawKey(t *testing.T) {
	raw := []byte(`{
		"id": 6732,
		"name": "architecture-extra",
		"key": "sk-secret",
		"group_id": 60,
		"quota": 0,
		"quota_used": 0,
		"rate_multiplier": 1,
		"group": {
			"id": 60,
			"name": "330",
			"daily_limit_usd": 330
		}
	}`)

	var key CodesomeApiKey
	if err := json.Unmarshal(raw, &key); err != nil {
		t.Fatalf("failed to unmarshal key: %v", err)
	}

	cached := sanitizeCodesomeApiKeyForCache(key)
	if _, ok := cached["key"]; ok {
		t.Fatal("sanitized cache item must not include raw key")
	}
	if cached["group_id"] != 60 {
		t.Fatalf("expected group_id 60, got %v", cached["group_id"])
	}
}

func TestCodesomeKeysCacheHasRawKey(t *testing.T) {
	unsafeCache := map[string]any{
		"_list": []any{
			map[string]any{
				"id":  6732,
				"key": "sk-secret",
			},
		},
	}
	if !codesomeKeysCacheHasRawKey(unsafeCache) {
		t.Fatal("expected raw key cache to be detected")
	}

	safeCache := map[string]any{
		"_list": []any{
			map[string]any{
				"id":       6732,
				"group_id": 60,
			},
		},
	}
	if codesomeKeysCacheHasRawKey(safeCache) {
		t.Fatal("did not expect sanitized cache to be detected as unsafe")
	}
}

func TestDeleteUnsafeCodesomeKeysCacheRemovesRawKeyEntry(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWD)

	cacheJSON := []byte(`{
		"codesome_keys": {
			"timestamp": 0,
			"data": {
				"_list": [
					{"id": 6732, "key": "sk-secret"}
				]
			}
		}
	}`)
	if err := os.WriteFile(".usage_cache.json", cacheJSON, 0644); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	if !codesomeKeysCacheFileHasRawKey("codesome_keys") {
		t.Fatal("expected raw key cache file to be detected")
	}

	deleteUnsafeCodesomeKeysCache("codesome_keys")

	if codesomeKeysCacheFileHasRawKey("codesome_keys") {
		t.Fatal("expected unsafe codesome_keys cache entry to be removed")
	}
}

func TestCreateCodesomeKeyValidatesInput(t *testing.T) {
	if _, err := CreateCodesomeKey(nil, "", 51); err == nil {
		t.Fatal("expected empty name to fail")
	}
	if _, err := CreateCodesomeKey(nil, "test", 0); err == nil {
		t.Fatal("expected invalid group id to fail")
	}
}

func TestUpdateCodesomeKeyValidatesInput(t *testing.T) {
	status := "disabled"
	if _, err := UpdateCodesomeKey(nil, 9356, CodesomeKeyUpdate{Status: &status}); err == nil {
		t.Fatal("expected invalid status to fail")
	}

	validStatus := "inactive"
	if _, err := UpdateCodesomeKey(nil, 0, CodesomeKeyUpdate{Status: &validStatus}); err == nil {
		t.Fatal("expected invalid key id to fail")
	}

	if _, err := UpdateCodesomeKey(nil, 9356, CodesomeKeyUpdate{}); err == nil {
		t.Fatal("expected empty update to fail")
	}
}

func TestGetCodesomeKeyUsageStatsValidatesInput(t *testing.T) {
	if _, err := GetCodesomeKeyUsageStats(nil, 0, "2026-05-26", "2026-05-26", false); err == nil {
		t.Fatal("expected invalid key id to fail")
	}
	if _, err := GetCodesomeKeyUsageStats(nil, 6732, "2026/05/26", "2026-05-26", false); err == nil {
		t.Fatal("expected invalid start date to fail")
	}
	if _, err := GetCodesomeKeyUsageStats(nil, 6732, "2026-05-27", "2026-05-26", false); err == nil {
		t.Fatal("expected reversed date range to fail")
	}
}

func TestGetCodesomeKeysDailyUsageValidatesInput(t *testing.T) {
	cfg := &config.Config{Codesome: &config.CodesomeConfig{}}
	if _, err := GetCodesomeKeysDailyUsage(&config.Config{}, []int{6732}); err == nil {
		t.Fatal("expected missing Codesome config to fail")
	}
	if _, err := GetCodesomeKeysDailyUsage(cfg, nil); err == nil {
		t.Fatal("expected empty key ids to fail")
	}
	if _, err := GetCodesomeKeysDailyUsage(cfg, []int{6732, 0}); err == nil {
		t.Fatal("expected invalid key id to fail")
	}
}

func TestFetchCodesomeUsageRequiresCodesomeConfig(t *testing.T) {
	cfg := &config.Config{}
	if _, _, _, _, err := FetchCodesomeUsage(cfg, false); err == nil {
		t.Fatal("expected missing Codesome config to fail")
	}
}
