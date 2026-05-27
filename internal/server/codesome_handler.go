package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

var (
	switchCodesomeKeyGroup            = provider.SwitchCodesomeKeyGroup
	switchCodesomeKeyGroupOnExhausted = provider.SwitchCodesomeKeyGroupOnExhausted
	createCodesomeKey                 = provider.CreateCodesomeKey
	updateCodesomeKey                 = provider.UpdateCodesomeKey
	fetchCodesomeUsage                = provider.FetchCodesomeUsage
	getCodesomeKeyUsageStats          = provider.GetCodesomeKeyUsageStats
)

// resolveKeyID extracts and validates key_id or key from query params.
// key and key_id are mutually exclusive; exactly one must be provided.
func resolveKeyID(cfg *config.Config, r *http.Request) (int, error) {
	keyParam := r.URL.Query().Get("key")
	keyIDParam := r.URL.Query().Get("key_id")

	if keyParam == "" && keyIDParam == "" {
		return 0, fmt.Errorf("缺少 key 或 key_id 参数")
	}
	if keyParam != "" && keyIDParam != "" {
		return 0, fmt.Errorf("key 和 key_id 不能同时指定")
	}

	if keyIDParam != "" {
		id, err := strconv.Atoi(keyIDParam)
		if err != nil {
			return 0, fmt.Errorf("key_id 必须为整数")
		}
		return id, nil
	}
	return cfg.ResolveCodesomeKeyID(keyParam)
}

func resolveGroupID(r *http.Request) (int, error) {
	groupIDParam := r.URL.Query().Get("group_id")
	if groupIDParam == "" {
		return 0, fmt.Errorf("缺少 group_id 参数")
	}
	groupID, err := strconv.Atoi(groupIDParam)
	if err != nil {
		return 0, fmt.Errorf("group_id 必须为整数")
	}
	if groupID <= 0 {
		return 0, fmt.Errorf("group_id 必须为正整数")
	}
	return groupID, nil
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func ensureCodesomeConfigured(cfg *config.Config) error {
	if cfg == nil || cfg.GetCodesomeConfig() == nil {
		return fmt.Errorf("未找到 Codesome 配置")
	}
	return nil
}

func parseBoolQuery(r *http.Request, name string) bool {
	value := r.URL.Query().Get(name)
	return value == "1" || value == "true" || value == "yes"
}

func resolveDateRange(r *http.Request) (string, string, error) {
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")
	if startDate == "" {
		return "", "", fmt.Errorf("缺少 start_date 参数")
	}
	if endDate == "" {
		return "", "", fmt.Errorf("缺少 end_date 参数")
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return "", "", fmt.Errorf("start_date 必须是 YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return "", "", fmt.Errorf("end_date 必须是 YYYY-MM-DD")
	}
	if end.Before(start) {
		return "", "", fmt.Errorf("end_date 必须大于等于 start_date")
	}
	return startDate, endDate, nil
}

// UsageHandler handles GET /api/codesome/usage
func UsageHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := ensureCodesomeConfigured(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		keys, subs, usageMap, tokenStatsMap, err := fetchCodesomeUsage(cfg, parseBoolQuery(r, "force_update"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"keys":          keys,
			"subscriptions": subs,
			"usage":         usageMap,
			"token_stats":   tokenStatsMap,
		})
	}
}

// UsageStatsHandler handles GET /api/codesome/usage-stats
func UsageStatsHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := ensureCodesomeConfigured(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		keyID, err := resolveKeyID(cfg, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		startDate, endDate, err := resolveDateRange(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		stats, err := getCodesomeKeyUsageStats(cfg, keyID, startDate, endDate, parseBoolQuery(r, "force_update"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"key_id":     keyID,
			"start_date": startDate,
			"end_date":   endDate,
			"timezone":   "Asia/Shanghai",
			"stats":      stats,
		})
	}
}

type createKeyRequest struct {
	Name    string `json:"name"`
	GroupID int    `json:"group_id"`
}

// KeysHandler handles POST/PUT /api/codesome/keys
func KeysHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ensureCodesomeConfigured(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPost:
			handleCreateKey(cfg, w, r)
		case http.MethodPut:
			handleUpdateKey(cfg, w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleCreateKey(cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求体必须是 JSON", http.StatusBadRequest)
		return
	}

	key, err := createCodesomeKey(cfg, req.Name, req.GroupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, key)
}

func handleUpdateKey(cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	keyID, err := resolveKeyID(cfg, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var update provider.CodesomeKeyUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "请求体必须是 JSON", http.StatusBadRequest)
		return
	}

	key, err := updateCodesomeKey(cfg, keyID, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, key)
}

// DailyUsageHandler handles GET /api/codesome/daily-usage
func DailyUsageHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		keyID, err := resolveKeyID(cfg, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		todayCost, err := provider.GetCodesomeKeyDailyUsage(cfg, keyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"key_id":     keyID,
			"today_cost": todayCost,
		})
	}
}

// ResetAllQuotasHandler handles POST /api/codesome/reset-all-quotas
// Iterates over all configured api_key_ids and resets each one.
func ResetAllQuotasHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		codesome := cfg.GetCodesomeConfig()
		if codesome == nil {
			http.Error(w, "未找到 Codesome 配置", http.StatusBadRequest)
			return
		}
		if len(codesome.ApiKeyIDs) == 0 {
			http.Error(w, "未配置 api_key_ids", http.StatusBadRequest)
			return
		}

		type keyResult struct {
			KeyID   int    `json:"key_id"`
			Name    string `json:"name,omitempty"`
			Status  string `json:"status"`
			Message string `json:"message,omitempty"`
		}

		var results []keyResult
		for _, k := range codesome.ApiKeyIDs {
			err := provider.ResetCodesomeQuota(cfg, k.ID)
			if err != nil {
				results = append(results, keyResult{
					KeyID:   k.ID,
					Name:    k.Name,
					Status:  "error",
					Message: err.Error(),
				})
			} else {
				results = append(results, keyResult{
					KeyID:  k.ID,
					Name:   k.Name,
					Status: "ok",
				})
			}
		}

		writeJSON(w, map[string]any{
			"results": results,
		})
	}
}

// ResetQuotaHandler handles POST /api/codesome/reset-quota
func ResetQuotaHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		keyID, err := resolveKeyID(cfg, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := provider.ResetCodesomeQuota(cfg, keyID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]any{
			"key_id":  keyID,
			"message": "配额已重置",
		})
	}
}

// SwitchGroupHandler handles POST /api/codesome/switch-group
func SwitchGroupHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		keyID, err := resolveKeyID(cfg, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		groupID, err := resolveGroupID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		result, err := switchCodesomeKeyGroup(cfg, keyID, groupID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	}
}

// SwitchOnExhaustedHandler handles POST /api/codesome/switch-on-exhausted
func SwitchOnExhaustedHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		keyID, err := resolveKeyID(cfg, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		result, err := switchCodesomeKeyGroupOnExhausted(cfg, keyID, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	}
}
