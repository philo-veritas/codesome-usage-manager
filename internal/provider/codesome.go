package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"codesome-usage-manager/internal/auth"
	"codesome-usage-manager/internal/cache"
	"codesome-usage-manager/internal/config"
)

var codesomeHTTPClient = &http.Client{Timeout: 15 * time.Second}

type CodesomeApiKey struct {
	ID             int            `json:"id"`
	Name           string         `json:"name"`
	GroupID        int            `json:"group_id"`
	Status         string         `json:"status,omitempty"`
	Quota          float64        `json:"quota"`
	QuotaUsed      float64        `json:"quota_used"`
	RateMultiplier float64        `json:"rate_multiplier"`
	Group          *CodesomeGroup `json:"group"`
}

type CodesomeApiKeyWithSecret struct {
	CodesomeApiKey
	Key string `json:"key,omitempty"`
}

type CodesomeKeyUpdate struct {
	Name    *string `json:"name,omitempty"`
	GroupID *int    `json:"group_id,omitempty"`
	Status  *string `json:"status,omitempty"`
}

type CodesomeKeyUsage struct {
	ApiKeyID  int     `json:"api_key_id"`
	TodayCost float64 `json:"today_actual_cost"`
	TotalCost float64 `json:"total_actual_cost"`
}

type CodesomeTokenStats struct {
	TotalInputTokens  int64 `json:"total_input_tokens"`
	TotalOutputTokens int64 `json:"total_output_tokens"`
	TotalCacheTokens  int64 `json:"total_cache_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
}

type CodesomeUsageStats struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCacheTokens  int64   `json:"total_cache_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCost         float64 `json:"total_cost"`
	TotalActualCost   float64 `json:"total_actual_cost"`
	AverageDurationMS float64 `json:"average_duration_ms"`
}

type CodesomeGroup struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	DailyLimitUSD float64 `json:"daily_limit_usd"`
}

type CodesomeSubscription struct {
	Name          string         `json:"name"`
	DailyUsageUSD float64        `json:"daily_usage_usd"`
	Status        string         `json:"status"`
	Group         *CodesomeGroup `json:"group"`
}

type CodesomeGroupSwitchResult struct {
	KeyID               int     `json:"key_id"`
	Switched            bool    `json:"switched"`
	FromGroupID         int     `json:"from_group_id,omitempty"`
	FromGroupName       string  `json:"from_group_name,omitempty"`
	ToGroupID           int     `json:"to_group_id,omitempty"`
	ToGroupName         string  `json:"to_group_name,omitempty"`
	CurrentRemainingUSD float64 `json:"current_remaining_usd,omitempty"`
	TargetRemainingUSD  float64 `json:"target_remaining_usd,omitempty"`
	Message             string  `json:"message"`
}

type CodesomeGroupSwitchBatchResult struct {
	KeyID  int                        `json:"key_id"`
	Name   string                     `json:"name,omitempty"`
	Result *CodesomeGroupSwitchResult `json:"result,omitempty"`
	Error  string                     `json:"error,omitempty"`
}

type CodesomeSubscriptionUsageSummary struct {
	RemainingUSD float64 `json:"remaining_usd"`
	LimitUSD     float64 `json:"limit_usd"`
}

type codesomeKeysPage struct {
	Items []json.RawMessage `json:"items"`
	Total int               `json:"total"`
	Pages int               `json:"pages"`
}

type codesomeUsageResponse struct {
	Stats map[string]CodesomeKeyUsage `json:"stats"`
}

type codesomeClient struct {
	authClient *auth.CodesomeAuth
	baseURL    string
}

func newCodesomeClient(cfg *config.Config) *codesomeClient {
	baseURL := config.DefaultCodesomeBaseURL
	if cfg != nil {
		if codesome := cfg.GetCodesomeConfig(); codesome != nil {
			baseURL = codesome.BaseURL
		}
	}
	return &codesomeClient{
		authClient: auth.NewCodesomeAuth(cfg),
		baseURL:    baseURL,
	}
}

func (c *codesomeClient) request(method, path string, body any) ([]byte, error) {
	url := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(data)
	}
	token, err := c.authClient.GetAccessToken(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := codesomeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		token, err = c.authClient.GetAccessToken(true)
		if err != nil {
			return nil, fmt.Errorf("re-login failed: %w", err)
		}
		if body != nil {
			data, _ := json.Marshal(body)
			reqBody = bytes.NewBuffer(data)
		}
		req, _ = http.NewRequest(method, url, reqBody)
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request %s %s returned status %d", method, path, resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(respBody, &envelope) == nil && envelope.Data != nil {
		return envelope.Data, nil
	}
	return respBody, nil
}

func (c *codesomeClient) fetchApiKeys(forceUpdate bool) ([]CodesomeApiKey, error) {
	cacheKey := "codesome_keys"
	deleteUnsafeCodesomeKeysCache(cacheKey)
	if !forceUpdate {
		cached, _ := cache.LoadCachedData(cacheKey)
		if cached != nil {
			if codesomeKeysCacheHasRawKey(cached) {
				cache.DeleteCacheKey(cacheKey)
			} else {
				return parseApiKeysFromCache(cacheKey)
			}
		}
	}
	var allItems []json.RawMessage
	page := 1
	for {
		data, err := c.request("GET", fmt.Sprintf("/api/v1/keys?page=%d&size=100", page), nil)
		if err != nil {
			return nil, err
		}
		var pageData codesomeKeysPage
		if err := json.Unmarshal(data, &pageData); err != nil {
			return nil, fmt.Errorf("failed to parse keys page: %w", err)
		}
		allItems = append(allItems, pageData.Items...)
		if page >= pageData.Pages || len(pageData.Items) == 0 {
			break
		}
		page++
	}

	cacheData := make([]map[string]any, 0, len(allItems))
	var keys []CodesomeApiKey
	for _, raw := range allItems {
		var key CodesomeApiKey
		if err := json.Unmarshal(raw, &key); err == nil {
			keys = append(keys, key)
			cacheData = append(cacheData, sanitizeCodesomeApiKeyForCache(key))
		}
	}

	saveCacheList(cacheKey, cacheData)
	return keys, nil
}

func (c *codesomeClient) fetchKeysUsage(keyIDs []int, forceUpdate bool) ([]CodesomeKeyUsage, error) {
	cacheKey := "codesome_usage"
	if !forceUpdate {
		cached, _ := cache.LoadCachedData(cacheKey)
		if cached != nil {
			return parseKeyUsageFromCache(cached)
		}
	}

	data, err := c.request("POST", "/api/v1/usage/dashboard/api-keys-usage", map[string]any{
		"api_key_ids": keyIDs,
	})
	if err != nil {
		return nil, err
	}

	var resp codesomeUsageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse usage data: %w", err)
	}

	usageList := make([]CodesomeKeyUsage, 0, len(resp.Stats))
	for _, u := range resp.Stats {
		usageList = append(usageList, u)
	}

	// Save to cache in the same format as the API response (compatible with Python)
	cache.SaveCache(cacheKey, map[string]any{"stats": resp.Stats})
	return usageList, nil
}

func (c *codesomeClient) fetchSubscriptions(forceUpdate bool) ([]CodesomeSubscription, error) {
	cacheKey := "codesome_subscriptions"
	if !forceUpdate {
		cached, _ := cache.LoadCachedData(cacheKey)
		if cached != nil {
			return parseSubscriptionsFromCache(cacheKey)
		}
	}

	data, err := c.request("GET", "/api/v1/subscriptions", nil)
	if err != nil {
		return nil, err
	}

	var subs []CodesomeSubscription
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, fmt.Errorf("failed to parse subscriptions: %w", err)
	}
	subs = filterSubscriptions(subs)

	var cacheData []map[string]any
	for _, raw := range subs {
		b, _ := json.Marshal(raw)
		var m map[string]any
		json.Unmarshal(b, &m)
		cacheData = append(cacheData, m)
	}
	saveCacheList(cacheKey, cacheData)
	return subs, nil
}

func (c *codesomeClient) createApiKey(name string, groupID int) (*CodesomeApiKeyWithSecret, error) {
	data, err := c.request("POST", "/api/v1/keys", map[string]any{
		"name":     name,
		"group_id": groupID,
	})
	if err != nil {
		return nil, err
	}

	var key CodesomeApiKeyWithSecret
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("failed to parse created key: %w", err)
	}
	return &key, nil
}

func (c *codesomeClient) updateApiKey(keyID int, update CodesomeKeyUpdate) (*CodesomeApiKey, error) {
	body := make(map[string]any)
	if update.Name != nil {
		body["name"] = *update.Name
	}
	if update.GroupID != nil {
		body["group_id"] = *update.GroupID
	}
	if update.Status != nil {
		body["status"] = *update.Status
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("至少指定一个要更新的字段")
	}

	data, err := c.request("PUT", fmt.Sprintf("/api/v1/keys/%d", keyID), body)
	if err != nil {
		return nil, err
	}

	var key CodesomeApiKey
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("failed to parse updated key: %w", err)
	}
	return &key, nil
}

func filterSubscriptions(subs []CodesomeSubscription) []CodesomeSubscription {
	filtered := make([]CodesomeSubscription, 0, len(subs))
	for _, sub := range subs {
		if sub.Status == "active" {
			filtered = append(filtered, sub)
		}
	}
	return filtered
}

func saveCacheList(key string, items []map[string]any) {
	cache.SaveCache(key, map[string]any{"_list": items})
}

func loadCacheList(key string) ([]map[string]any, bool) {
	cached, err := cache.LoadCachedData(key)
	if err != nil || cached == nil {
		return nil, false
	}
	raw, ok := cached["_list"]
	if !ok {
		return nil, false
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	result := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result, len(result) > 0
}

func codesomeKeysCacheHasRawKey(cached map[string]any) bool {
	raw, ok := cached["_list"]
	if !ok {
		return false
	}
	list, ok := raw.([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := m["key"]; ok {
			return true
		}
	}
	return false
}

func codesomeKeysCacheFileHasRawKey(cacheKey string) bool {
	data, err := os.ReadFile(cache.GetCacheFilePath())
	if err != nil {
		return false
	}
	var cacheFile cache.CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return false
	}
	entry, ok := cacheFile[cacheKey]
	if !ok {
		return false
	}
	return codesomeKeysCacheHasRawKey(entry.Data)
}

func deleteUnsafeCodesomeKeysCache(cacheKey string) {
	if codesomeKeysCacheFileHasRawKey(cacheKey) {
		cache.DeleteCacheKey(cacheKey)
	}
}

func sanitizeCodesomeApiKeyForCache(key CodesomeApiKey) map[string]any {
	groupID := key.GroupID
	if groupID == 0 && key.Group != nil {
		groupID = key.Group.ID
	}

	item := map[string]any{
		"id":              key.ID,
		"name":            key.Name,
		"group_id":        groupID,
		"status":          key.Status,
		"quota":           key.Quota,
		"quota_used":      key.QuotaUsed,
		"rate_multiplier": key.RateMultiplier,
	}
	if key.Group != nil {
		item["group"] = map[string]any{
			"id":              key.Group.ID,
			"name":            key.Group.Name,
			"daily_limit_usd": key.Group.DailyLimitUSD,
		}
	}
	return item
}

func parseApiKeysFromCache(cacheKey string) ([]CodesomeApiKey, error) {
	return unmarshalCacheList[CodesomeApiKey](cacheKey)
}
func parseKeyUsageFromCache(cached map[string]any) ([]CodesomeKeyUsage, error) {
	statsRaw, ok := cached["stats"]
	if !ok {
		return nil, fmt.Errorf("no stats in cached usage data")
	}
	statsMap, ok := statsRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid stats format in cached usage data")
	}
	result := make([]CodesomeKeyUsage, 0, len(statsMap))
	for _, v := range statsMap {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		var u CodesomeKeyUsage
		if json.Unmarshal(b, &u) == nil {
			result = append(result, u)
		}
	}
	return result, nil
}
func parseSubscriptionsFromCache(cacheKey string) ([]CodesomeSubscription, error) {
	return unmarshalCacheList[CodesomeSubscription](cacheKey)
}

func unmarshalCacheList[T any](cacheKey string) ([]T, error) {
	items, ok := loadCacheList(cacheKey)
	if !ok {
		return nil, fmt.Errorf("no cached data for %s", cacheKey)
	}
	result := make([]T, 0, len(items))
	for _, m := range items {
		b, _ := json.Marshal(m)
		var v T
		if json.Unmarshal(b, &v) == nil {
			result = append(result, v)
		}
	}
	return result, nil
}

func (c *codesomeClient) resetQuota(keyID int) error {
	_, err := c.request("PUT", fmt.Sprintf("/api/v1/keys/%d", keyID), map[string]any{
		"reset_quota": true,
	})
	return err
}

func (c *codesomeClient) switchKeyGroup(keyID int, groupID int) error {
	_, err := c.request("PUT", fmt.Sprintf("/api/v1/keys/%d", keyID), map[string]any{
		"group_id": groupID,
	})
	return err
}

func (c *codesomeClient) fetchApiKeyByID(keyID int) (*CodesomeApiKey, error) {
	keys, err := c.fetchApiKeys(true)
	if err != nil {
		return nil, err
	}
	return findCodesomeApiKeyByID(keys, keyID)
}

func findCodesomeApiKeyByID(keys []CodesomeApiKey, keyID int) (*CodesomeApiKey, error) {
	for i := range keys {
		if keys[i].ID == keyID {
			return &keys[i], nil
		}
	}
	return nil, fmt.Errorf("未找到 API Key %d", keyID)
}

func keyGroupID(key *CodesomeApiKey) int {
	if key == nil {
		return 0
	}
	if key.GroupID != 0 {
		return key.GroupID
	}
	if key.Group != nil {
		return key.Group.ID
	}
	return 0
}

func keyGroupName(key *CodesomeApiKey) string {
	if key != nil && key.Group != nil {
		return key.Group.Name
	}
	return ""
}

func subscriptionRemainingUSD(sub CodesomeSubscription) float64 {
	if sub.Group == nil {
		return 0
	}
	return math.Max(sub.Group.DailyLimitUSD-sub.DailyUsageUSD, 0)
}

func summarizeSubscriptions(subs []CodesomeSubscription) CodesomeSubscriptionUsageSummary {
	var summary CodesomeSubscriptionUsageSummary
	for _, sub := range subs {
		if sub.Status != "active" || sub.Group == nil {
			continue
		}
		summary.LimitUSD += sub.Group.DailyLimitUSD
		summary.RemainingUSD += subscriptionRemainingUSD(sub)
	}
	return summary
}

func activeSubscriptionForGroup(subs []CodesomeSubscription, groupID int) (*CodesomeSubscription, float64, bool) {
	for i := range subs {
		if subs[i].Status != "active" || subs[i].Group == nil || subs[i].Group.ID != groupID {
			continue
		}
		return &subs[i], subscriptionRemainingUSD(subs[i]), true
	}
	return nil, 0, false
}

func bestAvailableSubscription(subs []CodesomeSubscription, currentGroupID int) (*CodesomeSubscription, float64, bool) {
	var best *CodesomeSubscription
	bestRemaining := 0.0
	for i := range subs {
		sub := &subs[i]
		if sub.Status != "active" || sub.Group == nil || sub.Group.ID == currentGroupID {
			continue
		}
		remaining := subscriptionRemainingUSD(*sub)
		if remaining <= 0 {
			continue
		}
		if best == nil || remaining > bestRemaining || (remaining == bestRemaining && sub.Group.ID < best.Group.ID) {
			best = sub
			bestRemaining = remaining
		}
	}
	return best, bestRemaining, best != nil
}

func bestSubscription(subs []CodesomeSubscription) (*CodesomeSubscription, float64, bool) {
	var best *CodesomeSubscription
	bestRemaining := 0.0
	for i := range subs {
		sub := &subs[i]
		if sub.Status != "active" || sub.Group == nil {
			continue
		}
		remaining := subscriptionRemainingUSD(*sub)
		if remaining <= 0 {
			continue
		}
		if best == nil || remaining > bestRemaining || (remaining == bestRemaining && sub.Group.ID < best.Group.ID) {
			best = sub
			bestRemaining = remaining
		}
	}
	return best, bestRemaining, best != nil
}

func planSwitchOnExhausted(
	keyID int,
	currentGroupID int,
	currentGroupName string,
	subs []CodesomeSubscription,
	minRemainingUSD float64,
) (*CodesomeGroupSwitchResult, *CodesomeSubscription, error) {
	if minRemainingUSD < 0 {
		return nil, nil, fmt.Errorf("min_remaining 必须大于等于 0")
	}
	if currentGroupID == 0 {
		return nil, nil, fmt.Errorf("API Key %d 未绑定 group", keyID)
	}

	currentSub, currentRemaining, ok := activeSubscriptionForGroup(subs, currentGroupID)
	if !ok {
		return nil, nil, fmt.Errorf("API Key %d 当前 group %d 没有 active subscription", keyID, currentGroupID)
	}
	if currentGroupName == "" && currentSub.Group != nil {
		currentGroupName = currentSub.Group.Name
	}

	result := &CodesomeGroupSwitchResult{
		KeyID:               keyID,
		FromGroupID:         currentGroupID,
		FromGroupName:       currentGroupName,
		ToGroupID:           currentGroupID,
		ToGroupName:         currentGroupName,
		CurrentRemainingUSD: currentRemaining,
	}

	shouldSwitch := currentRemaining <= 0
	if minRemainingUSD > 0 {
		shouldSwitch = currentRemaining < minRemainingUSD
	}
	if !shouldSwitch {
		if minRemainingUSD > 0 {
			result.Message = fmt.Sprintf("当前 group 剩余额度 $%.2f，不低于阈值 $%.2f", currentRemaining, minRemainingUSD)
		} else {
			result.Message = fmt.Sprintf("当前 group 剩余额度 $%.2f，未耗尽", currentRemaining)
		}
		return result, nil, nil
	}

	target, targetRemaining, ok := bestAvailableSubscription(subs, currentGroupID)
	if !ok {
		if minRemainingUSD > 0 && currentRemaining > 0 {
			result.Message = fmt.Sprintf("当前 group 剩余额度 $%.2f，低于阈值 $%.2f，但没有剩余额度更高的可切换 group", currentRemaining, minRemainingUSD)
			return result, nil, nil
		}
		return nil, nil, fmt.Errorf("没有可切换的 active subscription group")
	}
	if minRemainingUSD > 0 && targetRemaining <= currentRemaining {
		result.Message = fmt.Sprintf("当前 group 剩余额度 $%.2f，低于阈值 $%.2f，但没有剩余额度更高的可切换 group", currentRemaining, minRemainingUSD)
		return result, nil, nil
	}

	result.Switched = true
	result.ToGroupID = target.Group.ID
	result.ToGroupName = target.Group.Name
	result.TargetRemainingUSD = targetRemaining
	if minRemainingUSD > 0 {
		result.Message = fmt.Sprintf("当前 group 剩余额度 $%.2f，低于阈值 $%.2f，切换到剩余额度最多的 group %d", currentRemaining, minRemainingUSD, target.Group.ID)
	} else {
		result.Message = fmt.Sprintf("当前 group 已无剩余额度，切换到剩余额度最多的 group %d", target.Group.ID)
	}
	return result, target, nil
}

func clearCodesomeGroupSwitchCaches() {
	cache.DeleteCacheKey("codesome_keys")
	cache.DeleteCacheKey("codesome_usage")
	cache.DeleteCacheKey("codesome_subscriptions")
}

// fetchKeyDailyUsage queries the usage API for a single key and returns its TodayCost.
func (c *codesomeClient) fetchKeyDailyUsage(keyID int) (float64, error) {
	data, err := c.request("POST", "/api/v1/usage/dashboard/api-keys-usage", map[string]any{
		"api_key_ids": []int{keyID},
	})
	if err != nil {
		return 0, err
	}
	var resp codesomeUsageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse usage data: %w", err)
	}
	for _, u := range resp.Stats {
		if u.ApiKeyID == keyID {
			return u.TodayCost, nil
		}
	}
	return 0, fmt.Errorf("usage data not found for key %d", keyID)
}

// GetCodesomeKeyDailyUsage returns the today cost for a specific API key.
func GetCodesomeKeyDailyUsage(cfg *config.Config, keyID int) (float64, error) {
	client := newCodesomeClient(cfg)
	return client.fetchKeyDailyUsage(keyID)
}

// GetCodesomeKeyUsageStats returns aggregate usage stats for an inclusive date range.
func GetCodesomeKeyUsageStats(cfg *config.Config, keyID int, startDate string, endDate string, forceUpdate bool) (*CodesomeUsageStats, error) {
	if err := validateUsageStatsDateRange(keyID, startDate, endDate); err != nil {
		return nil, err
	}
	client := newCodesomeClient(cfg)
	return client.fetchKeyUsageStats(keyID, startDate, endDate, forceUpdate)
}

// ListCodesomeKeys returns Codesome API keys from the remote API or cache.
func ListCodesomeKeys(cfg *config.Config, forceUpdate bool) ([]CodesomeApiKey, error) {
	if cfg == nil || cfg.GetCodesomeConfig() == nil {
		return nil, fmt.Errorf("未找到 Codesome 配置")
	}
	client := newCodesomeClient(cfg)
	return client.fetchApiKeys(forceUpdate)
}

// CreateCodesomeKey creates a Codesome API key and clears key-related caches.
func CreateCodesomeKey(cfg *config.Config, name string, groupID int) (*CodesomeApiKeyWithSecret, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name 不能为空")
	}
	if groupID <= 0 {
		return nil, fmt.Errorf("group_id 必须为正整数")
	}

	client := newCodesomeClient(cfg)
	key, err := client.createApiKey(name, groupID)
	if err != nil {
		return nil, err
	}
	cache.DeleteCacheKey("codesome_keys")
	cache.DeleteCacheKey("codesome_usage")
	return key, nil
}

// UpdateCodesomeKey updates selected Codesome API key fields and clears related caches.
func UpdateCodesomeKey(cfg *config.Config, keyID int, update CodesomeKeyUpdate) (*CodesomeApiKey, error) {
	if keyID <= 0 {
		return nil, fmt.Errorf("key_id 必须为正整数")
	}
	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" {
			return nil, fmt.Errorf("name 不能为空")
		}
		update.Name = &name
	}
	if update.GroupID != nil && *update.GroupID <= 0 {
		return nil, fmt.Errorf("group_id 必须为正整数")
	}
	if update.Status != nil {
		status := strings.TrimSpace(*update.Status)
		if status != "active" && status != "inactive" {
			return nil, fmt.Errorf("status 必须是 active 或 inactive")
		}
		update.Status = &status
	}
	if update.Name == nil && update.GroupID == nil && update.Status == nil {
		return nil, fmt.Errorf("至少指定一个要更新的字段")
	}

	client := newCodesomeClient(cfg)
	key, err := client.updateApiKey(keyID, update)
	if err != nil {
		return nil, err
	}
	cache.DeleteCacheKey("codesome_keys")
	cache.DeleteCacheKey("codesome_usage")
	cache.DeleteCacheKey("codesome_subscriptions")
	return key, nil
}

// ResetCodesomeQuota resets the quota for a specific API key and clears related caches.
// Each key can only be reset once per day (Beijing time).
func ResetCodesomeQuota(cfg *config.Config, keyID int) error {
	cst := time.FixedZone("CST", 8*3600)
	resetCacheKey := fmt.Sprintf("codesome_reset_%d", keyID)

	if lastReset, ok := cache.LoadCacheTimestamp(resetCacheKey); ok {
		lastDate := lastReset.In(cst).Format("2006-01-02")
		todayDate := time.Now().In(cst).Format("2006-01-02")
		if lastDate == todayDate {
			return fmt.Errorf("该 Key 今日已重置过配额，每天只能重置一次")
		}
	}

	client := newCodesomeClient(cfg)
	if err := client.resetQuota(keyID); err != nil {
		return err
	}

	if err := cache.SaveCache(resetCacheKey, map[string]any{"reset": true}); err != nil {
		return fmt.Errorf("配额已重置，但记录限频状态失败: %w", err)
	}
	cache.DeleteCacheKey("codesome_keys")
	cache.DeleteCacheKey("codesome_usage")
	return nil
}

func SwitchCodesomeKeyGroup(cfg *config.Config, keyID int, groupID int) (*CodesomeGroupSwitchResult, error) {
	if groupID <= 0 {
		return nil, fmt.Errorf("group_id 必须为正整数")
	}

	client := newCodesomeClient(cfg)
	key, err := client.fetchApiKeyByID(keyID)
	if err != nil {
		return nil, fmt.Errorf("获取 API Key 失败: %w", err)
	}
	fromGroupID := keyGroupID(key)
	fromGroupName := keyGroupName(key)

	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return nil, fmt.Errorf("获取订阅信息失败: %w", err)
	}
	targetSub, targetRemaining, ok := activeSubscriptionForGroup(subs, groupID)
	if !ok {
		return nil, fmt.Errorf("目标 group %d 没有 active subscription", groupID)
	}
	currentRemaining := 0.0
	if currentSub, remaining, ok := activeSubscriptionForGroup(subs, fromGroupID); ok {
		currentRemaining = remaining
		if fromGroupName == "" && currentSub.Group != nil {
			fromGroupName = currentSub.Group.Name
		}
	}

	result := &CodesomeGroupSwitchResult{
		KeyID:               keyID,
		FromGroupID:         fromGroupID,
		FromGroupName:       fromGroupName,
		ToGroupID:           groupID,
		ToGroupName:         targetSub.Group.Name,
		CurrentRemainingUSD: currentRemaining,
		TargetRemainingUSD:  targetRemaining,
	}
	if fromGroupID == groupID {
		result.Message = "API Key 已绑定目标 group，无需切换"
		return result, nil
	}

	if err := client.switchKeyGroup(keyID, groupID); err != nil {
		return nil, err
	}
	clearCodesomeGroupSwitchCaches()

	result.Switched = true
	result.Message = fmt.Sprintf("API Key %d 已切换到 group %d", keyID, groupID)
	return result, nil
}

func BestCodesomeGroupID(cfg *config.Config) (int, error) {
	if cfg == nil || cfg.GetCodesomeConfig() == nil {
		return 0, fmt.Errorf("未找到 Codesome 配置")
	}
	client := newCodesomeClient(cfg)
	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return 0, fmt.Errorf("获取订阅信息失败: %w", err)
	}
	target, _, ok := bestSubscription(subs)
	if !ok || target.Group == nil {
		return 0, fmt.Errorf("没有可用的 active subscription group")
	}
	return target.Group.ID, nil
}

func SwitchCodesomeKeyGroupOnExhausted(cfg *config.Config, keyID int, minRemainingUSD float64) (*CodesomeGroupSwitchResult, error) {
	client := newCodesomeClient(cfg)
	key, err := client.fetchApiKeyByID(keyID)
	if err != nil {
		return nil, fmt.Errorf("获取 API Key 失败: %w", err)
	}
	currentGroupID := keyGroupID(key)

	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return nil, fmt.Errorf("获取订阅信息失败: %w", err)
	}
	result, target, err := planSwitchOnExhausted(keyID, currentGroupID, keyGroupName(key), subs, minRemainingUSD)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return result, nil
	}

	if err := client.switchKeyGroup(keyID, target.Group.ID); err != nil {
		return nil, err
	}
	clearCodesomeGroupSwitchCaches()
	return result, nil
}

func SwitchCodesomeKeysGroupOnExhausted(
	cfg *config.Config,
	keyConfigs []config.CodesomeApiKeyId,
	minRemainingUSD float64,
) ([]CodesomeGroupSwitchBatchResult, error) {
	results, _, err := SwitchCodesomeKeysGroupOnExhaustedWithSummary(cfg, keyConfigs, minRemainingUSD)
	return results, err
}

func SwitchAllCodesomeKeysGroupOnExhausted(
	cfg *config.Config,
	minRemainingUSD float64,
) ([]CodesomeGroupSwitchBatchResult, error) {
	results, _, err := SwitchAllCodesomeKeysGroupOnExhaustedWithSummary(cfg, minRemainingUSD)
	return results, err
}

func SwitchAllCodesomeKeysGroupOnExhaustedWithSummary(
	cfg *config.Config,
	minRemainingUSD float64,
) ([]CodesomeGroupSwitchBatchResult, CodesomeSubscriptionUsageSummary, error) {
	if minRemainingUSD < 0 {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("min_remaining 必须大于等于 0")
	}

	client := newCodesomeClient(cfg)
	keys, err := client.fetchApiKeys(true)
	if err != nil {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("获取 API Key 失败: %w", err)
	}
	keys = activeCodesomeKeys(keys)
	if len(keys) == 0 {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("未找到 active Codesome API Key")
	}
	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("获取订阅信息失败: %w", err)
	}
	summary := summarizeSubscriptions(subs)

	results := make([]CodesomeGroupSwitchBatchResult, 0, len(keys))
	switchedAny := false
	for _, key := range keys {
		item := CodesomeGroupSwitchBatchResult{
			KeyID: key.ID,
			Name:  key.Name,
		}

		result, target, err := planSwitchOnExhausted(
			key.ID,
			keyGroupID(&key),
			keyGroupName(&key),
			subs,
			minRemainingUSD,
		)
		if err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}
		if target != nil {
			if err := client.switchKeyGroup(key.ID, target.Group.ID); err != nil {
				item.Error = err.Error()
				results = append(results, item)
				continue
			}
			switchedAny = true
		}
		item.Result = result
		results = append(results, item)
	}

	if switchedAny {
		clearCodesomeGroupSwitchCaches()
	}
	return results, summary, nil
}

func SwitchCodesomeKeysGroupOnExhaustedWithSummary(
	cfg *config.Config,
	keyConfigs []config.CodesomeApiKeyId,
	minRemainingUSD float64,
) ([]CodesomeGroupSwitchBatchResult, CodesomeSubscriptionUsageSummary, error) {
	if minRemainingUSD < 0 {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("min_remaining 必须大于等于 0")
	}
	if len(keyConfigs) == 0 {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("未配置 legacy api_key_ids")
	}

	client := newCodesomeClient(cfg)
	keys, err := client.fetchApiKeys(true)
	if err != nil {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("获取 API Key 失败: %w", err)
	}
	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return nil, CodesomeSubscriptionUsageSummary{}, fmt.Errorf("获取订阅信息失败: %w", err)
	}
	summary := summarizeSubscriptions(subs)

	results := make([]CodesomeGroupSwitchBatchResult, 0, len(keyConfigs))
	switchedAny := false
	for _, keyConfig := range keyConfigs {
		item := CodesomeGroupSwitchBatchResult{
			KeyID: keyConfig.ID,
			Name:  keyConfig.Name,
		}
		key, err := findCodesomeApiKeyByID(keys, keyConfig.ID)
		if err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}

		result, target, err := planSwitchOnExhausted(
			keyConfig.ID,
			keyGroupID(key),
			keyGroupName(key),
			subs,
			minRemainingUSD,
		)
		if err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}
		if target != nil {
			if err := client.switchKeyGroup(keyConfig.ID, target.Group.ID); err != nil {
				item.Error = err.Error()
				results = append(results, item)
				continue
			}
			switchedAny = true
		}
		item.Result = result
		results = append(results, item)
	}

	if switchedAny {
		clearCodesomeGroupSwitchCaches()
	}
	return results, summary, nil
}

func SwitchCodesomeKeysToBestGroup(
	cfg *config.Config,
	keyConfigs []config.CodesomeApiKeyId,
) ([]CodesomeGroupSwitchBatchResult, error) {
	if len(keyConfigs) == 0 {
		return nil, fmt.Errorf("未配置 legacy api_key_ids")
	}

	client := newCodesomeClient(cfg)
	keys, err := client.fetchApiKeys(true)
	if err != nil {
		return nil, fmt.Errorf("获取 API Key 失败: %w", err)
	}
	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return nil, fmt.Errorf("获取订阅信息失败: %w", err)
	}

	target, targetRemaining, ok := bestSubscription(subs)
	if !ok {
		return nil, fmt.Errorf("没有可用的 active subscription group")
	}

	results := make([]CodesomeGroupSwitchBatchResult, 0, len(keyConfigs))
	switchedAny := false
	for _, keyConfig := range keyConfigs {
		item := CodesomeGroupSwitchBatchResult{
			KeyID: keyConfig.ID,
			Name:  keyConfig.Name,
		}

		key, err := findCodesomeApiKeyByID(keys, keyConfig.ID)
		if err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}

		fromGroupID := keyGroupID(key)
		fromGroupName := keyGroupName(key)
		currentRemaining := 0.0
		if currentSub, remaining, ok := activeSubscriptionForGroup(subs, fromGroupID); ok {
			currentRemaining = remaining
			if fromGroupName == "" && currentSub.Group != nil {
				fromGroupName = currentSub.Group.Name
			}
		}

		result := &CodesomeGroupSwitchResult{
			KeyID:               keyConfig.ID,
			FromGroupID:         fromGroupID,
			FromGroupName:       fromGroupName,
			ToGroupID:           target.Group.ID,
			ToGroupName:         target.Group.Name,
			CurrentRemainingUSD: currentRemaining,
			TargetRemainingUSD:  targetRemaining,
		}

		if fromGroupID == target.Group.ID {
			result.Message = "API Key 已绑定当前剩余额度最高的 group，无需切换"
			item.Result = result
			results = append(results, item)
			continue
		}

		if err := client.switchKeyGroup(keyConfig.ID, target.Group.ID); err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}

		switchedAny = true
		result.Switched = true
		result.Message = fmt.Sprintf("API Key %d 已切换到当前剩余额度最高的 group %d", keyConfig.ID, target.Group.ID)
		item.Result = result
		results = append(results, item)
	}

	if switchedAny {
		clearCodesomeGroupSwitchCaches()
	}
	return results, nil
}

func SwitchAllCodesomeKeysToBestGroup(cfg *config.Config) ([]CodesomeGroupSwitchBatchResult, error) {
	client := newCodesomeClient(cfg)
	keys, err := client.fetchApiKeys(true)
	if err != nil {
		return nil, fmt.Errorf("获取 API Key 失败: %w", err)
	}
	keys = activeCodesomeKeys(keys)
	if len(keys) == 0 {
		return nil, fmt.Errorf("未找到 active Codesome API Key")
	}
	subs, err := client.fetchSubscriptions(true)
	if err != nil {
		return nil, fmt.Errorf("获取订阅信息失败: %w", err)
	}

	target, targetRemaining, ok := bestSubscription(subs)
	if !ok {
		return nil, fmt.Errorf("没有可用的 active subscription group")
	}

	results := make([]CodesomeGroupSwitchBatchResult, 0, len(keys))
	switchedAny := false
	for _, key := range keys {
		item := CodesomeGroupSwitchBatchResult{
			KeyID: key.ID,
			Name:  key.Name,
		}

		fromGroupID := keyGroupID(&key)
		fromGroupName := keyGroupName(&key)
		currentRemaining := 0.0
		if currentSub, remaining, ok := activeSubscriptionForGroup(subs, fromGroupID); ok {
			currentRemaining = remaining
			if fromGroupName == "" && currentSub.Group != nil {
				fromGroupName = currentSub.Group.Name
			}
		}

		result := &CodesomeGroupSwitchResult{
			KeyID:               key.ID,
			FromGroupID:         fromGroupID,
			FromGroupName:       fromGroupName,
			ToGroupID:           target.Group.ID,
			ToGroupName:         target.Group.Name,
			CurrentRemainingUSD: currentRemaining,
			TargetRemainingUSD:  targetRemaining,
		}

		if fromGroupID == target.Group.ID {
			result.Message = "API Key 已绑定当前剩余额度最高的 group，无需切换"
			item.Result = result
			results = append(results, item)
			continue
		}

		if err := client.switchKeyGroup(key.ID, target.Group.ID); err != nil {
			item.Error = err.Error()
			results = append(results, item)
			continue
		}

		switchedAny = true
		result.Switched = true
		result.Message = fmt.Sprintf("API Key %d 已切换到当前剩余额度最高的 group %d", key.ID, target.Group.ID)
		item.Result = result
		results = append(results, item)
	}

	if switchedAny {
		clearCodesomeGroupSwitchCaches()
	}
	return results, nil
}

func activeCodesomeKeys(keys []CodesomeApiKey) []CodesomeApiKey {
	active := make([]CodesomeApiKey, 0, len(keys))
	for _, key := range keys {
		if key.Status == "active" {
			active = append(active, key)
		}
	}
	return active
}

func formatTokenCount(tokens int64) string {
	switch {
	case tokens >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(tokens)/1e9)
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1e6)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1e3)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

func validateUsageStatsDateRange(keyID int, startDate string, endDate string) error {
	if keyID <= 0 {
		return fmt.Errorf("key_id 必须为正整数")
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Errorf("start_date 必须是 YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Errorf("end_date 必须是 YYYY-MM-DD")
	}
	if end.Before(start) {
		return fmt.Errorf("end_date 必须大于等于 start_date")
	}
	return nil
}

func parseUsageStatsFromCache(cached map[string]any) (*CodesomeUsageStats, error) {
	data, err := json.Marshal(cached)
	if err != nil {
		return nil, err
	}
	var stats CodesomeUsageStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func cacheUsageStats(cacheKey string, stats *CodesomeUsageStats) {
	cache.SaveCache(cacheKey, map[string]any{
		"total_requests":      stats.TotalRequests,
		"total_input_tokens":  stats.TotalInputTokens,
		"total_output_tokens": stats.TotalOutputTokens,
		"total_cache_tokens":  stats.TotalCacheTokens,
		"total_tokens":        stats.TotalTokens,
		"total_cost":          stats.TotalCost,
		"total_actual_cost":   stats.TotalActualCost,
		"average_duration_ms": stats.AverageDurationMS,
	})
}

func (c *codesomeClient) fetchKeyUsageStats(keyID int, startDate string, endDate string, forceUpdate bool) (*CodesomeUsageStats, error) {
	cacheKey := fmt.Sprintf("codesome_usage_stats_%d_%s_%s", keyID, startDate, endDate)
	if !forceUpdate {
		cached, _ := cache.LoadCachedData(cacheKey)
		if cached != nil {
			return parseUsageStatsFromCache(cached)
		}
	}

	path := fmt.Sprintf("/api/v1/usage/stats?start_date=%s&end_date=%s&api_key_id=%d&timezone=Asia%%2FShanghai",
		startDate, endDate, keyID)
	data, err := c.request("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var stats CodesomeUsageStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse usage stats: %w", err)
	}

	cacheUsageStats(cacheKey, &stats)
	return &stats, nil
}

func (c *codesomeClient) fetchKeyTokenStats(keyID int, forceUpdate bool) (*CodesomeTokenStats, error) {
	cacheKey := fmt.Sprintf("codesome_token_stats_%d", keyID)
	if !forceUpdate {
		cached, _ := cache.LoadCachedData(cacheKey)
		if cached != nil {
			return parseTokenStatsFromCache(cached)
		}
	}

	cst := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cst)
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -30).Format("2006-01-02")

	path := fmt.Sprintf("/api/v1/usage/stats?start_date=%s&end_date=%s&api_key_id=%d&timezone=Asia/Shanghai",
		startDate, endDate, keyID)
	data, err := c.request("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var stats CodesomeTokenStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse token stats: %w", err)
	}

	cache.SaveCache(cacheKey, map[string]any{
		"total_input_tokens":  stats.TotalInputTokens,
		"total_output_tokens": stats.TotalOutputTokens,
		"total_cache_tokens":  stats.TotalCacheTokens,
		"total_tokens":        stats.TotalTokens,
	})
	return &stats, nil
}

func (c *codesomeClient) fetchAllKeyTokenStats(keyIDs []int, forceUpdate bool) map[int]*CodesomeTokenStats {
	result := make(map[int]*CodesomeTokenStats, len(keyIDs))
	for _, id := range keyIDs {
		stats, err := c.fetchKeyTokenStats(id, forceUpdate)
		if err != nil {
			continue
		}
		result[id] = stats
	}
	return result
}

func parseTokenStatsFromCache(cached map[string]any) (*CodesomeTokenStats, error) {
	toInt64 := func(v any) int64 {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int64:
			return n
		default:
			return 0
		}
	}
	return &CodesomeTokenStats{
		TotalInputTokens:  toInt64(cached["total_input_tokens"]),
		TotalOutputTokens: toInt64(cached["total_output_tokens"]),
		TotalCacheTokens:  toInt64(cached["total_cache_tokens"]),
		TotalTokens:       toInt64(cached["total_tokens"]),
	}, nil
}

// FetchCodesomeUsage fetches all Codesome usage data.
func FetchCodesomeUsage(cfg *config.Config, forceUpdate bool) (
	[]CodesomeApiKey, []CodesomeSubscription, map[int]CodesomeKeyUsage, map[int]*CodesomeTokenStats, error,
) {
	if cfg == nil {
		return nil, nil, nil, nil, fmt.Errorf("未找到 Codesome 配置")
	}
	codesomeCfg := cfg.GetCodesomeConfig()
	if codesomeCfg == nil {
		return nil, nil, nil, nil, fmt.Errorf("未找到 Codesome 配置")
	}
	client := newCodesomeClient(cfg)

	var keys []CodesomeApiKey
	var keyIDs []int
	if len(codesomeCfg.ApiKeyIDs) > 0 {
		for _, kid := range codesomeCfg.ApiKeyIDs {
			keyIDs = append(keyIDs, kid.ID)
			keys = append(keys, CodesomeApiKey{
				ID:   kid.ID,
				Name: kid.Name,
			})
		}
	} else {
		var err error
		keys, err = client.fetchApiKeys(forceUpdate)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("获取 API Keys 失败: %w", err)
		}
		keyIDs = make([]int, len(keys))
		for i, k := range keys {
			keyIDs[i] = k.ID
		}
	}

	subs, err := client.fetchSubscriptions(forceUpdate)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("获取订阅信息失败: %w", err)
	}

	usageMap := make(map[int]CodesomeKeyUsage)
	if len(keyIDs) > 0 {
		usages, err := client.fetchKeysUsage(keyIDs, forceUpdate)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("获取用量数据失败: %w", err)
		}
		for _, u := range usages {
			usageMap[u.ApiKeyID] = u
		}
	}

	tokenStatsMap := client.fetchAllKeyTokenStats(keyIDs, forceUpdate)

	return keys, subs, usageMap, tokenStatsMap, nil
}

// DisplayCodesomeUsage prints formatted Codesome usage.
func DisplayCodesomeUsage(
	keys []CodesomeApiKey,
	subs []CodesomeSubscription,
	usageMap map[int]CodesomeKeyUsage,
	tokenStatsMap map[int]*CodesomeTokenStats,
	debug bool,
) {
	if debug {
		data := map[string]any{
			"api_keys":        keys,
			"subscriptions":   subs,
			"usage_map":       usageMap,
			"token_stats_map": tokenStatsMap,
		}
		jsonData, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(jsonData))
		return
	}

	for _, sub := range subs {
		dailyLimit := 0.0
		if sub.Group != nil {
			dailyLimit = sub.Group.DailyLimitUSD
		}
		remaining := math.Max(dailyLimit-sub.DailyUsageUSD, 0)
		pctRemaining := 0.0
		if dailyLimit > 0 {
			pctRemaining = math.Round(remaining/dailyLimit*10000) / 100
		}
		fmt.Printf("[Codesome] %s: 剩余 $%.2f / $%.2f (%.2f%%)\n",
			sub.Name, remaining, dailyLimit, pctRemaining)
	}

	if len(keys) == 0 {
		return
	}

	type keyRow struct {
		name, todayStr, totalStr, quotaStr, tokenStr string
	}
	rows := make([]keyRow, 0, len(keys))
	maxName, maxToday, maxTotal, maxQuota := 0, 0, 0, 0

	for _, key := range keys {
		usage, ok := usageMap[key.ID]
		today := 0.0
		total := 0.0
		if ok {
			today = usage.TodayCost
			total = usage.TotalCost
		}

		quotaStr := ""
		if key.Quota > 0 {
			remaining := math.Max(key.Quota-key.QuotaUsed, 0)
			quotaStr = fmt.Sprintf("配额 $%.2f/$%.2f", remaining, key.Quota)
		} else {
			quotaStr = "按量计费"
			if key.RateMultiplier != 0 && key.RateMultiplier != 1.0 {
				quotaStr += fmt.Sprintf("(%.1fx)", key.RateMultiplier)
			}
		}

		tokenStr := ""
		if ts, ok := tokenStatsMap[key.ID]; ok && ts.TotalTokens > 0 {
			tokenStr = fmt.Sprintf("近30天 %s (入%s/出%s/缓%s)",
				formatTokenCount(ts.TotalTokens),
				formatTokenCount(ts.TotalInputTokens),
				formatTokenCount(ts.TotalOutputTokens),
				formatTokenCount(ts.TotalCacheTokens))
		}

		todayStr := fmt.Sprintf("今日 $%.2f", today)
		totalStr := fmt.Sprintf("总计 $%.2f", total)

		r := keyRow{key.Name, todayStr, totalStr, quotaStr, tokenStr}
		rows = append(rows, r)

		if w := displayWidth(r.name); w > maxName {
			maxName = w
		}
		if w := displayWidth(r.todayStr); w > maxToday {
			maxToday = w
		}
		if w := displayWidth(r.totalStr); w > maxTotal {
			maxTotal = w
		}
		if w := displayWidth(r.quotaStr); w > maxQuota {
			maxQuota = w
		}
	}

	for _, r := range rows {
		line := fmt.Sprintf("  %s  %s  %s  %s",
			padRight(r.name, maxName),
			padRight(r.todayStr, maxToday),
			padRight(r.totalStr, maxTotal),
			padRight(r.quotaStr, maxQuota))
		if r.tokenStr != "" {
			line += "  " + r.tokenStr
		}
		fmt.Println(line)
	}
}

// displayWidth returns the terminal display width of a string,
// counting CJK characters as 2 columns and others as 1.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115F,
			r >= 0x2E80 && r <= 0x303E,
			r >= 0x3040 && r <= 0x33BF,
			r >= 0x3400 && r <= 0x4DBF,
			r >= 0x4E00 && r <= 0x9FFF,
			r >= 0xA960 && r <= 0xA97C,
			r >= 0xAC00 && r <= 0xD7A3,
			r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFE10 && r <= 0xFE19,
			r >= 0xFE30 && r <= 0xFE6B,
			r >= 0xFF01 && r <= 0xFF60,
			r >= 0xFFE0 && r <= 0xFFE6,
			r >= 0x20000 && r <= 0x2FA1F,
			r >= 0x30000 && r <= 0x3134F:
			w += 2
		default:
			w++
		}
	}
	return w
}

// padRight pads s with spaces on the right so its display width reaches width.
func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}
