package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
)

const (
	codexBackendResponsesURL = "https://chatgpt.com/backend-api/codex/responses"
	codexBackendCompactURL   = "https://chatgpt.com/backend-api/codex/responses/compact"
	codexCapabilityProbeTime = 30 * time.Second
)

type CodexCapabilityProbeOptions struct {
	ProbePlatformAPI bool
}

type CodexCapabilityProbeResult struct {
	Capability model.ChannelAccountCapability
	OAuthJSON  bool
}

type codexProbeCredential struct {
	AccessToken string `json:"access_token,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
}

func ProbeCodexOAuthAccountCapabilities(ctx context.Context, channel *model.Channel, credentialIndex int, options CodexCapabilityProbeOptions) (CodexCapabilityProbeResult, error) {
	if channel == nil {
		return CodexCapabilityProbeResult{}, errors.New("渠道不存在")
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return CodexCapabilityProbeResult{}, errors.New("账号索引超出范围")
	}
	var credential codexProbeCredential
	if err := common.Unmarshal([]byte(strings.TrimSpace(keys[credentialIndex])), &credential); err != nil {
		return CodexCapabilityProbeResult{}, errors.New("codex oauth key json invalid")
	}
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	credential.AccountID = strings.TrimSpace(credential.AccountID)
	if credential.AccessToken == "" || credential.AccountID == "" {
		return CodexCapabilityProbeResult{}, errors.New("codex oauth key missing access_token or account_id")
	}

	capability := model.ChannelAccountCapability{
		CheckedTime:            common.GetTimestamp(),
		CapabilityProbeSurface: channelcapability.ProbeSurfaceCodexBackend,
	}
	if existing, ok := channel.ChannelInfo.AccountCapability(credentialIndex); ok {
		capability = existing
		capability.CheckedTime = common.GetTimestamp()
		capability.CapabilityProbeSurface = channelcapability.ProbeSurfaceCodexBackend
	}

	proxyID, proxyURL, proxyErr := codexProbeCredentialProxy(channel, credentialIndex)
	if proxyID > 0 {
		capability.ProxyID = proxyID
	}
	capability.ProxyCheckedTime = common.GetTimestamp()
	if proxyErr != nil {
		capability.ProxyLastError = proxyErr.Error()
		capability.CapabilityClassification = channelcapability.ClassificationProxyError
		capability.LastEndpoint = "proxy"
		capability.LastMessage = proxyErr.Error()
		return CodexCapabilityProbeResult{Capability: capability, OAuthJSON: true}, nil
	}

	client, err := GetHttpClientWithProxy(proxyURL)
	if err != nil {
		capability.ProxyLastError = err.Error()
		capability.CapabilityClassification = channelcapability.ClassificationProxyError
		capability.LastEndpoint = "proxy"
		capability.LastMessage = err.Error()
		return CodexCapabilityProbeResult{Capability: capability, OAuthJSON: true}, nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	clientCopy.Timeout = codexCapabilityProbeTime
	client = &clientCopy

	if proxyURL != "" {
		exitIP, region, exitErr := probeCodexProxyExit(ctx, client)
		if exitErr != nil {
			capability.ProxyLastError = exitErr.Error()
		} else {
			capability.ProxyExitIP = exitIP
			capability.ProxyRegion = region
			capability.ProxyLastError = ""
		}
	}

	messages := make([]string, 0, 4)
	requiresStream := true
	capability.CodexBackendRequiresStream = &requiresStream

	nonStreamResult := probeCodexBackendResponses(ctx, client, credential, false)
	if strings.Contains(strings.ToLower(nonStreamResult.Message), "stream must be set to true") {
		capability.CodexBackendRequiresStream = &requiresStream
		messages = append(messages, "Codex Responses: requires stream=true")
	}

	streamResult := probeCodexBackendResponses(ctx, client, credential, true)
	capability.CodexBackendResponsesStreamWrite = capabilityBool(streamResult.Success)
	if streamResult.Success {
		capability.ResponsesWrite = capabilityBool(true)
		capability.CapabilityClassification = channelcapability.ClassificationCodexBackendAvailable
	} else {
		messages = append(messages, "Codex Stream: "+streamResult.Message)
	}

	compactResult := probeCodexBackendCompact(ctx, client, credential)
	capability.CodexBackendCompactWrite = capabilityBool(compactResult.Success)
	if compactResult.Success {
		capability.ResponsesCompactWrite = capabilityBool(true)
		if streamResult.Success {
			capability.CapabilityClassification = channelcapability.ClassificationCodexCompactAvailable
		}
	} else {
		messages = append(messages, "Codex Compact: "+compactResult.Message)
	}

	classification := classifyCodexCapability(capability, streamResult, compactResult)
	capability.CapabilityClassification = classification
	capability.LastEndpoint = codexBackendResponsesURL
	if len(messages) == 0 {
		capability.LastMessage = "Codex backend 能力检测完成"
	} else {
		capability.LastMessage = strings.Join(messages, "；")
	}
	if options.ProbePlatformAPI {
		capability.CapabilityProbeSurface = channelcapability.ProbeSurfacePlatformAPI
	}
	return CodexCapabilityProbeResult{Capability: capability, OAuthJSON: true}, nil
}

func ProbeCodexOAuthPlatformCapabilities(ctx context.Context, channel *model.Channel, credentialIndex int) (CodexCapabilityProbeResult, error) {
	if channel == nil {
		return CodexCapabilityProbeResult{}, errors.New("渠道不存在")
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return CodexCapabilityProbeResult{}, errors.New("账号索引超出范围")
	}
	var credential codexProbeCredential
	if err := common.Unmarshal([]byte(strings.TrimSpace(keys[credentialIndex])), &credential); err != nil {
		return CodexCapabilityProbeResult{}, errors.New("codex oauth key json invalid")
	}
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	credential.AccountID = strings.TrimSpace(credential.AccountID)
	if credential.AccessToken == "" || credential.AccountID == "" {
		return CodexCapabilityProbeResult{}, errors.New("codex oauth key missing access_token or account_id")
	}
	capability := model.ChannelAccountCapability{
		CheckedTime:            common.GetTimestamp(),
		CapabilityProbeSurface: channelcapability.ProbeSurfacePlatformAPI,
	}
	if existing, ok := channel.ChannelInfo.AccountCapability(credentialIndex); ok {
		capability = existing
		capability.CheckedTime = common.GetTimestamp()
		capability.CapabilityProbeSurface = channelcapability.ProbeSurfacePlatformAPI
	}
	proxyID, proxyURL, proxyErr := codexProbeCredentialProxy(channel, credentialIndex)
	if proxyID > 0 {
		capability.ProxyID = proxyID
	}
	if proxyErr != nil {
		capability.ProxyLastError = proxyErr.Error()
		capability.LastEndpoint = "proxy"
		capability.LastMessage = proxyErr.Error()
		return CodexCapabilityProbeResult{Capability: capability, OAuthJSON: true}, nil
	}
	client, err := GetHttpClientWithProxy(proxyURL)
	if err != nil {
		capability.ProxyLastError = err.Error()
		capability.LastEndpoint = "proxy"
		capability.LastMessage = err.Error()
		return CodexCapabilityProbeResult{Capability: capability, OAuthJSON: true}, nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	clientCopy.Timeout = codexCapabilityProbeTime
	client = &clientCopy

	messages := make([]string, 0, 3)
	chatResult := probePlatformChatCompletions(ctx, client, credential, false)
	capability.PlatformChatCompletionsWrite = capabilityBool(chatResult.Success)
	capability.ChatCompletionsWrite = capability.PlatformChatCompletionsWrite
	if !chatResult.Success {
		messages = append(messages, "Platform Chat: "+chatResult.Message)
	}

	responsesResult := probePlatformResponses(ctx, client, credential, false)
	capability.PlatformResponsesWrite = capabilityBool(responsesResult.Success)
	if !responsesResult.Success {
		messages = append(messages, "Platform Responses: "+responsesResult.Message)
	}

	compactResult := probePlatformResponsesCompact(ctx, client, credential)
	capability.PlatformResponsesCompactWrite = capabilityBool(compactResult.Success)
	if !compactResult.Success {
		messages = append(messages, "Platform Compact: "+compactResult.Message)
	}

	capability.LastEndpoint = "api.openai.com"
	rawMessage := strings.Join(messages, "；")
	rawLower := strings.ToLower(rawMessage)
	if len(messages) == 0 {
		capability.LastMessage = "Platform API 诊断完成"
	} else {
		capability.LastMessage = SummarizePlatformAPIDiagnosticMessages(messages)
	}
	if !capability.HasCodexBackendResponsesStreamAllowed() {
		if strings.Contains(rawLower, "insufficient_quota") || strings.Contains(rawLower, "exceeded your current quota") {
			capability.CapabilityClassification = channelcapability.ClassificationPlatformQuotaInsufficient
		} else if strings.Contains(rawLower, "api.responses.write") || strings.Contains(rawLower, "missing scopes") || strings.Contains(rawLower, "insufficient permissions") {
			capability.CapabilityClassification = channelcapability.ClassificationPlatformResponsesScopeMiss
		}
	} else if capability.HasCodexBackendCompactAllowed() {
		capability.CapabilityClassification = channelcapability.ClassificationCodexCompactAvailable
	} else {
		capability.CapabilityClassification = channelcapability.ClassificationCodexBackendAvailable
	}
	return CodexCapabilityProbeResult{Capability: capability, OAuthJSON: true}, nil
}

func SummarizePlatformAPIDiagnosticMessages(messages []string) string {
	joined := strings.Join(messages, "；")
	lower := strings.ToLower(joined)
	switch {
	case strings.Contains(lower, "insufficient_quota") || strings.Contains(lower, "exceeded your current quota"):
		return "Platform API 额度不足或未开通计费；这不影响 Codex backend 调度。"
	case strings.Contains(lower, "api.responses.write") || strings.Contains(lower, "missing scopes") || strings.Contains(lower, "insufficient permissions"):
		return "Platform Responses API 权限不足；这不影响 Codex backend 调度。"
	}
	return truncateCapabilityMessage(joined, 360)
}

func truncateCapabilityMessage(message string, maxRunes int) string {
	message = strings.TrimSpace(message)
	if maxRunes <= 0 {
		return message
	}
	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}
	return string(runes[:maxRunes]) + "..."
}

type codexHTTPProbeResult struct {
	Success    bool
	StatusCode int
	Message    string
}

func probeCodexBackendResponses(ctx context.Context, client *http.Client, credential codexProbeCredential, stream bool) codexHTTPProbeResult {
	body := map[string]any{
		"model":        "gpt-5.4",
		"input":        []map[string]string{{"role": "user", "content": "Reply with only: ok"}},
		"instructions": "",
		"store":        false,
		"stream":       stream,
	}
	return doCodexBackendProbe(ctx, client, credential, codexBackendResponsesURL, body, stream)
}

func probeCodexBackendCompact(ctx context.Context, client *http.Client, credential codexProbeCredential) codexHTTPProbeResult {
	body := map[string]any{
		"model":        "gpt-5.4",
		"input":        []map[string]string{{"role": "user", "content": "hi"}},
		"instructions": "",
	}
	return doCodexBackendProbe(ctx, client, credential, codexBackendCompactURL, body, false)
}

func probePlatformChatCompletions(ctx context.Context, client *http.Client, credential codexProbeCredential, stream bool) codexHTTPProbeResult {
	body := map[string]any{
		"model":      "gpt-4o-mini",
		"messages":   []map[string]string{{"role": "user", "content": "Reply with only: ok"}},
		"max_tokens": 8,
		"stream":     stream,
	}
	return doCodexBackendProbe(ctx, client, credential, "https://api.openai.com/v1/chat/completions", body, stream)
}

func probePlatformResponses(ctx context.Context, client *http.Client, credential codexProbeCredential, stream bool) codexHTTPProbeResult {
	body := map[string]any{
		"model":             "gpt-4o-mini",
		"input":             "Reply with only: ok",
		"store":             false,
		"stream":            stream,
		"max_output_tokens": 16,
	}
	return doCodexBackendProbe(ctx, client, credential, "https://api.openai.com/v1/responses", body, stream)
}

func probePlatformResponsesCompact(ctx context.Context, client *http.Client, credential codexProbeCredential) codexHTTPProbeResult {
	body := map[string]any{
		"model":        "gpt-5.4",
		"input":        []map[string]string{{"role": "user", "content": "hi"}},
		"instructions": "",
	}
	return doCodexBackendProbe(ctx, client, credential, "https://api.openai.com/v1/responses/compact", body, false)
}

func doCodexBackendProbe(ctx context.Context, client *http.Client, credential codexProbeCredential, target string, body map[string]any, stream bool) codexHTTPProbeResult {
	jsonData, err := common.Marshal(body)
	if err != nil {
		return codexHTTPProbeResult{Message: err.Error()}
	}
	reqCtx, cancel := context.WithTimeout(ctx, codexCapabilityProbeTime)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(jsonData))
	if err != nil {
		return codexHTTPProbeResult{Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	req.Header.Set("chatgpt-account-id", credential.AccountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return codexHTTPProbeResult{Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	message := codexProbeMessage(raw)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return codexHTTPProbeResult{StatusCode: resp.StatusCode, Message: message}
	}
	return codexHTTPProbeResult{
		Success:    codexProbeHasOutput(raw),
		StatusCode: resp.StatusCode,
		Message:    message,
	}
}

func codexProbeMessage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(raw, &payload); err == nil {
		if errObj, ok := payload["error"].(map[string]any); ok {
			if message := strings.TrimSpace(fmt.Sprint(errObj["message"])); message != "" && message != "<nil>" {
				return message
			}
		}
		if message := strings.TrimSpace(fmt.Sprint(payload["message"])); message != "" && message != "<nil>" {
			return message
		}
	}
	text := strings.TrimSpace(string(raw))
	if len(text) > 360 {
		text = text[:360] + "..."
	}
	return text
}

func codexProbeHasOutput(raw []byte) bool {
	lower := strings.ToLower(string(raw))
	return strings.Contains(lower, "response.output_text.delta") ||
		strings.Contains(lower, "response.completed") ||
		strings.Contains(lower, `"response.compaction"`) ||
		strings.Contains(lower, `"output"`)
}

func classifyCodexCapability(capability model.ChannelAccountCapability, streamResult codexHTTPProbeResult, compactResult codexHTTPProbeResult) string {
	if streamResult.Success {
		if compactResult.Success {
			return channelcapability.ClassificationCodexCompactAvailable
		}
		return channelcapability.ClassificationCodexBackendAvailable
	}
	lower := strings.ToLower(streamResult.Message + " " + compactResult.Message)
	if strings.Contains(lower, "unsupported_country_region_territory") || strings.Contains(lower, "country, region") {
		return channelcapability.ClassificationRegionError
	}
	if strings.Contains(lower, "token_invalidated") || strings.Contains(lower, "unauthorized") || streamResult.StatusCode == http.StatusUnauthorized || streamResult.StatusCode == http.StatusForbidden {
		return channelcapability.ClassificationAuthError
	}
	return channelcapability.ClassificationUnknown
}

func capabilityBool(value bool) *bool {
	return &value
}

func codexProbeCredentialProxy(channel *model.Channel, credentialIndex int) (int, string, error) {
	if channel == nil || channel.ChannelInfo.MultiKeyProxyIDs == nil {
		return 0, "", nil
	}
	proxyID := channel.ChannelInfo.MultiKeyProxyIDs[credentialIndex]
	if proxyID <= 0 {
		return 0, "", nil
	}
	proxyConfig, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil {
		return proxyID, "", fmt.Errorf("credential proxy not found: proxy_id=%d: %w", proxyID, err)
	}
	if proxyConfig == nil || !proxyConfig.Enabled {
		return proxyID, "", fmt.Errorf("credential proxy disabled: proxy_id=%d", proxyID)
	}
	proxyURL, err := proxyConfig.ProxyURL()
	if err != nil {
		return proxyID, "", fmt.Errorf("credential proxy invalid: proxy_id=%d: %w", proxyID, err)
	}
	return proxyID, proxyURL, nil
}

func probeCodexProxyExit(ctx context.Context, client *http.Client) (string, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://ipapi.co/json/", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var payload struct {
		IP          string `json:"ip"`
		CountryName string `json:"country_name"`
		Region      string `json:"region"`
		City        string `json:"city"`
	}
	if err := common.DecodeJson(io.LimitReader(resp.Body, 16*1024), &payload); err != nil {
		return "", "", err
	}
	region := strings.TrimSpace(strings.Join([]string{payload.CountryName, payload.Region, payload.City}, " "))
	return strings.TrimSpace(payload.IP), region, nil
}
