package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

func testConfig() *config.Config {
	return &config.Config{
		Codesome: &config.CodesomeConfig{
			ApiKeyIDs: []config.CodesomeApiKeyId{
				{ID: 6732, Key: "main", Name: "main key"},
			},
		},
	}
}

func testConfigWithoutCodesome() *config.Config {
	return &config.Config{}
}

func TestUsageHandlerReturnsProviderResult(t *testing.T) {
	original := fetchCodesomeUsage
	defer func() { fetchCodesomeUsage = original }()

	fetchCodesomeUsage = func(cfg *config.Config, forceUpdate bool) ([]provider.CodesomeApiKey, []provider.CodesomeSubscription, map[int]provider.CodesomeKeyUsage, map[int]*provider.CodesomeTokenStats, error) {
		if !forceUpdate {
			t.Fatal("expected forceUpdate true")
		}
		return []provider.CodesomeApiKey{{ID: 6732, Name: "main"}}, nil, map[int]provider.CodesomeKeyUsage{
			6732: {ApiKeyID: 6732, TodayCost: 1.5},
		}, nil, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/codesome/usage?force_update=true", nil)
	rec := httptest.NewRecorder()

	UsageHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUsageHandlerRejectsMissingCodesomeConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/codesome/usage", nil)
	rec := httptest.NewRecorder()

	UsageHandler(testConfigWithoutCodesome()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUsageStatsHandlerReturnsProviderResult(t *testing.T) {
	original := getCodesomeKeyUsageStats
	defer func() { getCodesomeKeyUsageStats = original }()

	getCodesomeKeyUsageStats = func(cfg *config.Config, keyID int, startDate string, endDate string, forceUpdate bool) (*provider.CodesomeUsageStats, error) {
		if keyID != 6732 {
			t.Fatalf("expected keyID 6732, got %d", keyID)
		}
		if startDate != "2026-05-26" || endDate != "2026-05-26" {
			t.Fatalf("unexpected date range: %s %s", startDate, endDate)
		}
		if !forceUpdate {
			t.Fatal("expected forceUpdate true")
		}
		return &provider.CodesomeUsageStats{
			TotalRequests:   309,
			TotalActualCost: 59.2088295,
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/codesome/usage-stats?key=main&start_date=2026-05-26&end_date=2026-05-26&force_update=true", nil)
	rec := httptest.NewRecorder()

	UsageStatsHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		KeyID int                         `json:"key_id"`
		Stats provider.CodesomeUsageStats `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result.KeyID != 6732 || result.Stats.TotalRequests != 309 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestUsageStatsHandlerRejectsInvalidDateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/codesome/usage-stats?key=main&start_date=2026-05-27&end_date=2026-05-26", nil)
	rec := httptest.NewRecorder()

	UsageStatsHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestUsageStatsHandlerRejectsMissingCodesomeConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/codesome/usage-stats?key_id=6732&start_date=2026-05-26&end_date=2026-05-26", nil)
	rec := httptest.NewRecorder()

	UsageStatsHandler(testConfigWithoutCodesome()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestKeysHandlerCreatesKey(t *testing.T) {
	original := createCodesomeKey
	defer func() { createCodesomeKey = original }()

	createCodesomeKey = func(cfg *config.Config, name string, groupID int) (*provider.CodesomeApiKeyWithSecret, error) {
		if name != "test" {
			t.Fatalf("expected name test, got %s", name)
		}
		if groupID != 51 {
			t.Fatalf("expected groupID 51, got %d", groupID)
		}
		return &provider.CodesomeApiKeyWithSecret{
			CodesomeApiKey: provider.CodesomeApiKey{ID: 9356, Name: name, GroupID: groupID, Status: "active"},
			Key:            "sk-test",
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/codesome/keys", strings.NewReader(`{"name":"test","group_id":51}`))
	rec := httptest.NewRecorder()

	KeysHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result provider.CodesomeApiKeyWithSecret
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result.ID != 9356 || result.Key != "sk-test" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestKeysHandlerRejectsMissingCodesomeConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/codesome/keys", strings.NewReader(`{"name":"test","group_id":51}`))
	rec := httptest.NewRecorder()

	KeysHandler(testConfigWithoutCodesome()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestKeysHandlerUpdatesKey(t *testing.T) {
	original := updateCodesomeKey
	defer func() { updateCodesomeKey = original }()

	updateCodesomeKey = func(cfg *config.Config, keyID int, update provider.CodesomeKeyUpdate) (*provider.CodesomeApiKey, error) {
		if keyID != 6732 {
			t.Fatalf("expected keyID 6732, got %d", keyID)
		}
		if update.Status == nil || *update.Status != "inactive" {
			t.Fatalf("expected inactive status update, got %+v", update.Status)
		}
		return &provider.CodesomeApiKey{ID: keyID, Name: "main", Status: "inactive"}, nil
	}

	req := httptest.NewRequest(http.MethodPut, "/api/codesome/keys?key=main", strings.NewReader(`{"status":"inactive"}`))
	rec := httptest.NewRecorder()

	KeysHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result provider.CodesomeApiKey
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result.ID != 6732 || result.Status != "inactive" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestSwitchGroupHandlerRequiresGroupID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/codesome/switch-group?key_id=6732", nil)
	rec := httptest.NewRecorder()

	SwitchGroupHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSwitchGroupHandlerRejectsInvalidKeyID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/codesome/switch-group?key_id=abc&group_id=60", nil)
	rec := httptest.NewRecorder()

	SwitchGroupHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSwitchGroupHandlerReturnsProviderResult(t *testing.T) {
	original := switchCodesomeKeyGroup
	defer func() { switchCodesomeKeyGroup = original }()

	switchCodesomeKeyGroup = func(cfg *config.Config, keyID int, groupID int) (*provider.CodesomeGroupSwitchResult, error) {
		if keyID != 6732 {
			t.Fatalf("expected keyID 6732, got %d", keyID)
		}
		if groupID != 60 {
			t.Fatalf("expected groupID 60, got %d", groupID)
		}
		return &provider.CodesomeGroupSwitchResult{
			KeyID:       keyID,
			Switched:    true,
			ToGroupID:   groupID,
			ToGroupName: "target",
			Message:     "ok",
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/codesome/switch-group?key=main&group_id=60", nil)
	rec := httptest.NewRecorder()

	SwitchGroupHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result provider.CodesomeGroupSwitchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !result.Switched || result.ToGroupID != 60 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestSwitchGroupHandlerMapsProviderError(t *testing.T) {
	original := switchCodesomeKeyGroup
	defer func() { switchCodesomeKeyGroup = original }()

	switchCodesomeKeyGroup = func(cfg *config.Config, keyID int, groupID int) (*provider.CodesomeGroupSwitchResult, error) {
		return nil, fmt.Errorf("provider failed")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/codesome/switch-group?key_id=6732&group_id=60", nil)
	rec := httptest.NewRecorder()

	SwitchGroupHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestSwitchOnExhaustedHandlerReturnsProviderResult(t *testing.T) {
	original := switchCodesomeKeyGroupOnExhausted
	defer func() { switchCodesomeKeyGroupOnExhausted = original }()

	switchCodesomeKeyGroupOnExhausted = func(cfg *config.Config, keyID int, minRemainingUSD float64) (*provider.CodesomeGroupSwitchResult, error) {
		if keyID != 6732 {
			t.Fatalf("expected keyID 6732, got %d", keyID)
		}
		if minRemainingUSD != 0 {
			t.Fatalf("expected minRemainingUSD 0, got %.2f", minRemainingUSD)
		}
		return &provider.CodesomeGroupSwitchResult{
			KeyID:    keyID,
			Switched: false,
			Message:  "not exhausted",
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/codesome/switch-on-exhausted?key=main", nil)
	rec := httptest.NewRecorder()

	SwitchOnExhaustedHandler(testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result provider.CodesomeGroupSwitchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result.Switched || result.KeyID != 6732 {
		t.Fatalf("unexpected response: %+v", result)
	}
}
