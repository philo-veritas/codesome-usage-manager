package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"codesome-usage-manager/internal/config"
)

const (
	expiryBufferSec = 300
	authFileName    = ".codesome_auth.json"
)

var authHTTPClient = &http.Client{Timeout: 15 * time.Second}

type authState struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresAt    float64 `json:"expires_at"`
}

// CodesomeAuth handles login, token refresh, and persistence.
type CodesomeAuth struct {
	authFile string
	baseURL  string
	cfg      *config.Config
}

// NewCodesomeAuth creates a new auth client.
func NewCodesomeAuth(cfg *config.Config) *CodesomeAuth {
	baseURL := config.DefaultCodesomeBaseURL
	if cfg != nil {
		if codesome := cfg.GetCodesomeConfig(); codesome != nil {
			baseURL = codesome.BaseURL
		}
	}
	return &CodesomeAuth{
		authFile: getAuthFilePath(),
		baseURL:  baseURL,
		cfg:      cfg,
	}
}

func getAuthFilePath() string {
	abs, _ := filepath.Abs(authFileName)
	return abs
}

func (a *CodesomeAuth) loadState() *authState {
	data, err := os.ReadFile(a.authFile)
	if err != nil {
		return nil
	}
	var state authState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	return &state
}

func (a *CodesomeAuth) saveState(state *authState) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(a.authFile, data, 0600)
}

func (a *CodesomeAuth) isValid(state *authState) bool {
	now := float64(time.Now().Unix())
	return state.ExpiresAt > now+expiryBufferSec
}

func (a *CodesomeAuth) parseTokenResponse(
	body []byte, fallbackRefresh string,
) (*authState, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse auth response: %w", err)
	}

	payload, ok := raw["data"].(map[string]interface{})
	if !ok {
		payload = raw
	}

	accessToken, _ := payload["access_token"].(string)
	refreshToken, _ := payload["refresh_token"].(string)
	if refreshToken == "" {
		refreshToken = fallbackRefresh
	}
	if accessToken == "" {
		return nil, fmt.Errorf("auth response missing access_token")
	}

	expiresIn := 3600.0
	if v, ok := payload["expires_in"].(float64); ok {
		expiresIn = v
	}
	expiresAt := float64(time.Now().Unix()) + expiresIn

	return &authState{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func (a *CodesomeAuth) login() (*authState, error) {
	creds, err := a.cfg.GetCodesomeLoginCredentials()
	if err != nil {
		return nil, err
	}

	reqBody, _ := json.Marshal(map[string]string{
		"email":    creds.Email,
		"password": creds.Password,
	})

	resp, err := authHTTPClient.Post(
		a.baseURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read login response: %w", err)
	}
	state, err := a.parseTokenResponse(bodyBytes, "")
	if err != nil {
		return nil, err
	}
	a.saveState(state)
	return state, nil
}

func (a *CodesomeAuth) refresh(refreshToken string) (*authState, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
	})

	resp, err := authHTTPClient.Post(
		a.baseURL+"/api/v1/auth/refresh",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed with status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}
	state, err := a.parseTokenResponse(bodyBytes, refreshToken)
	if err != nil {
		return nil, err
	}
	a.saveState(state)
	return state, nil
}

// GetAccessToken returns a valid access token, refreshing or logging in as needed.
func (a *CodesomeAuth) GetAccessToken(forceLogin bool) (string, error) {
	if forceLogin {
		state, err := a.login()
		if err != nil {
			return "", err
		}
		return state.AccessToken, nil
	}

	state := a.loadState()
	if state != nil && a.isValid(state) {
		return state.AccessToken, nil
	}

	if state != nil && state.RefreshToken != "" {
		refreshed, err := a.refresh(state.RefreshToken)
		if err == nil {
			return refreshed.AccessToken, nil
		}
	}

	loginState, err := a.login()
	if err != nil {
		return "", err
	}
	return loginState.AccessToken, nil
}
