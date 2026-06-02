package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
)

const credentialWritebackPath = "/api/internal/token-account-automation/credential"

type Client struct {
	baseURL string
	token   string
	timeout time.Duration
	client  *http.Client
}

type CredentialWritebackRequest struct {
	ChannelID       int            `json:"channel_id"`
	CredentialIndex int            `json:"credential_index"`
	CredentialType  string         `json:"credential_type,omitempty"`
	Credential      any            `json:"credential"`
	SourceJobID     string         `json:"source_job_id,omitempty"`
	SecretRef       string         `json:"secret_ref,omitempty"`
	Fingerprint     string         `json:"fingerprint,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type CredentialWritebackResult struct {
	ChannelID           int    `json:"channel_id"`
	CredentialIndex     int    `json:"credential_index"`
	AccountType         string `json:"account_type,omitempty"`
	ChannelStatus       int    `json:"channel_status"`
	AccountEnabled      bool   `json:"account_enabled"`
	ClearedAuthError    bool   `json:"cleared_auth_error"`
	ClearedAutoDisabled bool   `json:"cleared_auto_disabled"`
}

func New(baseURL string, token string, timeoutSeconds int) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		timeout: timeout,
		client:  &http.Client{},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

func (c *Client) WriteCredential(ctx context.Context, req CredentialWritebackRequest) (*CredentialWritebackResult, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if req.ChannelID <= 0 || req.CredentialIndex < 0 {
		return nil, errors.New("channel_id and credential_index are required for gateway writeback")
	}
	payload, err := jsonx.Marshal(req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+credentialWritebackPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Success bool                      `json:"success"`
		Message string                    `json:"message,omitempty"`
		Data    CredentialWritebackResult `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway credential writeback failed: status=%d message=%s", resp.StatusCode, message)
	}
	return &envelope.Data, nil
}
