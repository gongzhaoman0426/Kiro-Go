package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"kiro-api-proxy/config"
	"net/http"
	"time"
)

// RefreshToken 刷新 access token（带重试）
func RefreshToken(account *config.Account) (string, string, int64, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
			fmt.Printf("[RefreshToken] Retry attempt %d for %s\n", attempt+1, account.Email)
		}

		var accessToken, refreshToken string
		var expiresAt int64
		var err error

		if account.AuthMethod == "social" {
			accessToken, refreshToken, expiresAt, err = refreshSocialToken(account.RefreshToken)
		} else {
			accessToken, refreshToken, expiresAt, err = refreshOIDCToken(account.RefreshToken, account.ClientID, account.ClientSecret, account.Region)
		}

		if err == nil {
			return accessToken, refreshToken, expiresAt, nil
		}
		lastErr = err
	}
	return "", "", 0, fmt.Errorf("token refresh failed after retries: %w", lastErr)
}

// refreshOIDCToken IdC/Builder ID token 刷新
func refreshOIDCToken(refreshToken, clientID, clientSecret, region string) (string, string, int64, error) {
	if region == "" {
		region = "us-east-1"
	}

	url := fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)

	payload := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := string(respBody)
		if resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 {
			return "", "", 0, fmt.Errorf("OIDC refresh token invalid or expired (HTTP %d): %s — need to re-login", resp.StatusCode, errMsg)
		}
		return "", "", 0, fmt.Errorf("OIDC refresh failed (HTTP %d): %s", resp.StatusCode, errMsg)
	}

	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", 0, err
	}

	if result.AccessToken == "" {
		return "", "", 0, fmt.Errorf("OIDC refresh returned empty access token")
	}
	// expiresIn 为 0 时默认 1 小时
	if result.ExpiresIn <= 0 {
		result.ExpiresIn = 3600
	}
	expiresAt := time.Now().Unix() + int64(result.ExpiresIn)
	return result.AccessToken, result.RefreshToken, expiresAt, nil
}

// refreshSocialToken Social (GitHub/Google) token 刷新
func refreshSocialToken(refreshToken string) (string, string, int64, error) {
	url := "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

	payload := map[string]string{
		"refreshToken": refreshToken,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := string(respBody)
		if resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 {
			return "", "", 0, fmt.Errorf("Social refresh token invalid or expired (HTTP %d): %s — need to re-login", resp.StatusCode, errMsg)
		}
		return "", "", 0, fmt.Errorf("Social refresh failed (HTTP %d): %s", resp.StatusCode, errMsg)
	}

	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", 0, err
	}

	if result.AccessToken == "" {
		return "", "", 0, fmt.Errorf("Social refresh returned empty access token")
	}
	// expiresIn 为 0 时默认 1 小时
	if result.ExpiresIn <= 0 {
		result.ExpiresIn = 3600
	}
	expiresAt := time.Now().Unix() + int64(result.ExpiresIn)
	return result.AccessToken, result.RefreshToken, expiresAt, nil
}
