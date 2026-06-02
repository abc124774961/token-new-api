package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
)

const (
	CodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexOAuthTokenURL = "https://auth.openai.com/oauth/token"
	defaultTimeout     = 20 * time.Second
)

type CodexRefreshClient interface {
	RefreshCodexOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*CodexTokenResult, error)
}

type CodexTokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

type CodexHTTPClient struct {
	TokenURL string
	ClientID string
	Timeout  time.Duration
}

type TokenError struct {
	StatusCode  int
	Code        string
	Description string
}

func (e *TokenError) Error() string {
	parts := []string{"codex oauth refresh failed"}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	if e.Description != "" {
		parts = append(parts, e.Description)
	}
	return strings.Join(parts, ": ")
}

func NewCodexHTTPClient() *CodexHTTPClient {
	return &CodexHTTPClient{
		TokenURL: CodexOAuthTokenURL,
		ClientID: CodexOAuthClientID,
		Timeout:  defaultTimeout,
	}
}

func (c *CodexHTTPClient) RefreshCodexOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*CodexTokenResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errors.New("refresh_token is required")
	}
	client, err := c.httpClient(proxyURL)
	if err != nil {
		return nil, err
	}
	tokenURL := strings.TrimSpace(c.TokenURL)
	if tokenURL == "" {
		tokenURL = CodexOAuthTokenURL
	}
	clientID := strings.TrimSpace(c.ClientID)
	if clientID == "" {
		clientID = CodexOAuthClientID
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if len(body) > 0 {
		_ = jsonx.Unmarshal(body, &payload)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &TokenError{
			StatusCode:  resp.StatusCode,
			Code:        strings.TrimSpace(payload.Error),
			Description: strings.TrimSpace(payload.ErrorDescription),
		}
	}
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 {
		return nil, errors.New("codex oauth refresh response missing fields")
	}
	return &CodexTokenResult{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).Unix(),
	}, nil
}

func ShouldFallbackToBrowser(err error) bool {
	var tokenErr *TokenError
	if !errors.As(err, &tokenErr) {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(tokenErr.Code))
	return tokenErr.StatusCode == http.StatusUnauthorized ||
		tokenErr.StatusCode == http.StatusForbidden ||
		code == "invalid_grant" ||
		code == "invalid_request"
}

func (c *CodexHTTPClient) httpClient(proxyURL string) (*http.Client, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}
