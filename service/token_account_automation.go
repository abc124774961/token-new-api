package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	tokenAccountAutomationURLEnv            = "TOKEN_ACCOUNT_AUTOMATION_URL"
	tokenAccountAutomationAPITokenEnv       = "TOKEN_ACCOUNT_AUTOMATION_API_TOKEN"
	tokenAccountAutomationTimeoutSecondsEnv = "TOKEN_ACCOUNT_AUTOMATION_TIMEOUT_SECONDS"
)

type TokenAccountAutomationAuthInvalidEvent struct {
	TargetRef       string         `json:"target_ref,omitempty"`
	ChannelID       int            `json:"channel_id,omitempty"`
	CredentialIndex int            `json:"credential_index,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	SubjectKey      string         `json:"subject_key,omitempty"`
	DisplayName     string         `json:"display_name,omitempty"`
	Source          string         `json:"source,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Context         map[string]any `json:"context,omitempty"`
}

func TokenAccountAutomationConfigured() bool {
	return strings.TrimSpace(common.GetEnvOrDefaultString(tokenAccountAutomationURLEnv, "")) != "" &&
		strings.TrimSpace(common.GetEnvOrDefaultString(tokenAccountAutomationAPITokenEnv, "")) != ""
}

func NotifyTokenAccountAutomationAuthInvalid(ctx context.Context, event TokenAccountAutomationAuthInvalidEvent) error {
	baseURL := strings.TrimRight(strings.TrimSpace(common.GetEnvOrDefaultString(tokenAccountAutomationURLEnv, "")), "/")
	token := strings.TrimSpace(common.GetEnvOrDefaultString(tokenAccountAutomationAPITokenEnv, ""))
	if baseURL == "" || token == "" {
		return nil
	}
	event.normalize()
	if event.TargetRef == "" && (event.ChannelID <= 0 || event.CredentialIndex < 0) {
		return errors.New("target_ref or channel_id + credential_index is required")
	}
	payload, err := common.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/events/account-auth-invalid", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	timeoutSeconds := common.GetEnvOrDefault(tokenAccountAutomationTimeoutSecondsEnv, 3)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	ctxWithTimeout, cancel := context.WithTimeout(req.Context(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req = req.WithContext(ctxWithTimeout)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}
	_ = common.DecodeJson(resp.Body, &result)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !result.Success {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("token account automation auth invalid event failed: status=%d message=%s", resp.StatusCode, message)
	}
	return nil
}

func (event *TokenAccountAutomationAuthInvalidEvent) normalize() {
	event.TargetRef = strings.TrimSpace(event.TargetRef)
	event.Provider = strings.TrimSpace(event.Provider)
	event.SubjectKey = strings.TrimSpace(event.SubjectKey)
	event.DisplayName = strings.TrimSpace(event.DisplayName)
	event.Source = strings.TrimSpace(event.Source)
	event.Reason = strings.TrimSpace(event.Reason)
	if event.CredentialIndex < 0 {
		event.CredentialIndex = -1
	}
}
