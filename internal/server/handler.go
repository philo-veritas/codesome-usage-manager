package server

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"usage-cli/internal/config"
	"usage-cli/internal/provider"
)

// CostResponse represents the API response format
type CostResponse struct {
	Remaining float64 `json:"remaining"`
}

// CostHandler handles GET /api/cost requests
func CostHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract Bearer token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "缺少 Authorization header", http.StatusUnauthorized)
			return
		}

		authToken := strings.TrimPrefix(authHeader, "Bearer ")
		if authToken == authHeader {
			http.Error(w, "Authorization header 格式错误，需要 Bearer token", http.StatusUnauthorized)
			return
		}

		// Find apiID by auth token
		apiID, err := cfg.GetApiIDByAuthToken(authToken)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Fetch usage data (always force update for API requests)
		usageData, err := provider.FetchUsage(apiID, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Calculate remaining
		remaining := usageData.DailyCostLimit - usageData.CurrentDailyCost
		remaining = math.Round(remaining*100) / 100
		if remaining < 0 {
			remaining = 0
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CostResponse{
			Remaining: remaining,
		})
	}
}
