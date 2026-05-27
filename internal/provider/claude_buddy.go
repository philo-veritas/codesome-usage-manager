package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"usage-cli/internal/cache"
)

const BaseURL = "https://codex.claudebuddy.fun/apiStats/api"

type LimitsData struct {
	CurrentDailyCost float64 `json:"currentDailyCost"`
	DailyCostLimit   float64 `json:"dailyCostLimit"`
}

type StatsData struct {
	Limits LimitsData `json:"limits"`
}

type StatsResponse struct {
	Data StatsData `json:"data"`
}

type UsageData struct {
	CurrentDailyCost float64 `json:"current_daily_cost"`
	DailyCostLimit   float64 `json:"daily_cost_limit"`
}

// FetchUsage fetches usage data from Claude Buddy API
func FetchUsage(apiID string, forceUpdate bool) (*UsageData, error) {
	cacheKey := fmt.Sprintf("claude_buddy_info_%s", apiID)

	// Try to load from cache
	if !forceUpdate {
		cachedData, err := cache.LoadCachedData(cacheKey)
		if err == nil && cachedData != nil {
			currentCost, ok1 := cachedData["current_daily_cost"].(float64)
			costLimit, ok2 := cachedData["daily_cost_limit"].(float64)
			if ok1 && ok2 {
				return &UsageData{
					CurrentDailyCost: currentCost,
					DailyCostLimit:   costLimit,
				}, nil
			}
		}
	}

	// Fetch from API
	reqBody := map[string]string{"apiId": apiID}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(
		BaseURL+"/user-stats",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch usage: %w", err)
	}
	defer resp.Body.Close()

	var statsResp StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&statsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Round current daily cost to 2 decimal places
	currentCost := math.Round(statsResp.Data.Limits.CurrentDailyCost*100) / 100

	usageData := &UsageData{
		CurrentDailyCost: currentCost,
		DailyCostLimit:   statsResp.Data.Limits.DailyCostLimit,
	}

	// Save to cache
	cacheData := map[string]interface{}{
		"current_daily_cost": usageData.CurrentDailyCost,
		"daily_cost_limit":   usageData.DailyCostLimit,
	}
	cache.SaveCache(cacheKey, cacheData)

	return usageData, nil
}

// FormatUsageLine returns formatted usage string (without newline)
func FormatUsageLine(usageData *UsageData) string {
	remaining := usageData.DailyCostLimit - usageData.CurrentDailyCost
	remaining = math.Round(remaining*100) / 100

	percentageRemaining := (1 - usageData.CurrentDailyCost/usageData.DailyCostLimit) * 100
	percentageRemaining = math.Round(percentageRemaining*100) / 100

	// Format daily cost limit: show as integer if it's a whole number, otherwise show decimals
	limitStr := fmt.Sprintf("%.1f", usageData.DailyCostLimit)
	if usageData.DailyCostLimit == float64(int(usageData.DailyCostLimit)) {
		limitStr = fmt.Sprintf("%.0f", usageData.DailyCostLimit)
	}

	return fmt.Sprintf("剩余 $%.2f / $%s (%.2f%%)",
		remaining,
		limitStr,
		percentageRemaining,
	)
}

// DisplayUsage prints formatted usage information
func DisplayUsage(usageData *UsageData, debug bool) {
	if debug {
		jsonData, _ := json.MarshalIndent(usageData, "", "  ")
		fmt.Println(string(jsonData))
		return
	}

	fmt.Println(FormatUsageLine(usageData))
}
