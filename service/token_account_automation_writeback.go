package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/codexauth"
)

const (
	tokenAccountAutomationCredentialTypeAuto = "auto"

	tokenAccountAutomationAllKeysDisabledReason = "All keys are disabled"
	tokenAccountAutomationManualDisabledReason  = "manual"

	tokenAccountAutomationAccountTypeAPIKey        = "api_key"
	tokenAccountAutomationAccountTypeOAuthAccount  = "oauth_account"
	tokenAccountAutomationAccountTypeJSONAuth      = "json_auth"
	tokenAccountAutomationAccountTypeTokenKey      = "token_key"
	tokenAccountAutomationAccountTypeSessionCookie = "session_cookie"
	tokenAccountAutomationAccountTypeComposite     = "composite"
)

type TokenAccountAutomationCredentialWritebackRequest struct {
	ChannelID       int            `json:"channel_id"`
	CredentialIndex int            `json:"credential_index"`
	CredentialType  string         `json:"credential_type,omitempty"`
	Credential      any            `json:"credential"`
	SourceJobID     string         `json:"source_job_id,omitempty"`
	SecretRef       string         `json:"secret_ref,omitempty"`
	Fingerprint     string         `json:"fingerprint,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type TokenAccountAutomationCredentialWritebackResult struct {
	ChannelID           int    `json:"channel_id"`
	CredentialIndex     int    `json:"credential_index"`
	AccountType         string `json:"account_type,omitempty"`
	ChannelStatus       int    `json:"channel_status"`
	AccountEnabled      bool   `json:"account_enabled"`
	ClearedAuthError    bool   `json:"cleared_auth_error"`
	ClearedAutoDisabled bool   `json:"cleared_auto_disabled"`
}

func WritebackTokenAccountAutomationCredential(req TokenAccountAutomationCredentialWritebackRequest) (*TokenAccountAutomationCredentialWritebackResult, error) {
	req.normalize()
	if req.ChannelID <= 0 {
		return nil, errors.New("channel_id is required")
	}
	if req.CredentialIndex < 0 {
		return nil, errors.New("credential_index is required")
	}

	credential, accountType, err := normalizeTokenAccountAutomationCredential(req.Credential, req.CredentialType)
	if err != nil {
		return nil, err
	}

	lock := model.GetChannelPollingLock(req.ChannelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		return nil, errors.New("channel not found")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, errors.New("channel has no account credentials")
	}
	if req.CredentialIndex >= len(keys) {
		return nil, errors.New("credential_index is out of range")
	}
	for index, key := range keys {
		if index != req.CredentialIndex && strings.TrimSpace(key) == credential {
			return nil, errors.New("credential already exists in channel")
		}
	}

	keys[req.CredentialIndex] = credential
	channel.Key = strings.Join(keys, "\n")
	channel.ChannelInfo.IsMultiKey = len(keys) > 1
	channel.ChannelInfo.MultiKeySize = len(keys)
	tokenAccountAutomationSetAccountType(channel, req.CredentialIndex, accountType)

	clearedAuthError := tokenAccountAutomationClearAuthErrorCapability(channel, req.CredentialIndex)
	clearedAutoDisabled := tokenAccountAutomationEnableRecoveredAccount(channel, req.CredentialIndex)
	beforeStatus := channel.Status
	tokenAccountAutomationReconcileChannelStatus(channel, len(keys))

	if err := model.DB.Model(channel).Select("key", "status", "channel_info", "other_info").Updates(channel).Error; err != nil {
		return nil, err
	}
	if beforeStatus != channel.Status {
		if err := model.UpdateAbilityStatus(channel.Id, channel.Status == common.ChannelStatusEnabled); err != nil {
			return nil, err
		}
	}

	model.InitChannelCache()
	ResetProxyClientCache()

	identity := ChannelRuntimeIdentity{
		ChannelID:           channel.Id,
		CredentialIndex:     req.CredentialIndex,
		CredentialIndexSet:  true,
		AccountType:         accountType,
		CredentialSubjectFP: strings.TrimSpace(req.Fingerprint),
		CredentialFP:        strings.TrimSpace(req.Fingerprint),
	}
	ClearChannelRuntimeFailureAvoidance(identity)
	ClearChannelRuntimeFailureAvoidanceForAccountIndex(channel.Id, req.CredentialIndex)
	ClearChannelConfigIsolationForAccountIndex(channel.Id, req.CredentialIndex)

	result := &TokenAccountAutomationCredentialWritebackResult{
		ChannelID:           channel.Id,
		CredentialIndex:     req.CredentialIndex,
		AccountType:         accountType,
		ChannelStatus:       channel.Status,
		AccountEnabled:      tokenAccountAutomationAccountEnabled(channel, req.CredentialIndex),
		ClearedAuthError:    clearedAuthError,
		ClearedAutoDisabled: clearedAutoDisabled,
	}
	common.SysLog(fmt.Sprintf("token account automation credential writeback success: channel_id=%d credential_index=%d account_type=%s channel_status=%d account_enabled=%t cleared_auth_error=%t cleared_auto_disabled=%t source_job_id=%s fingerprint=%s",
		result.ChannelID,
		result.CredentialIndex,
		result.AccountType,
		result.ChannelStatus,
		result.AccountEnabled,
		result.ClearedAuthError,
		result.ClearedAutoDisabled,
		req.SourceJobID,
		req.Fingerprint,
	))
	return result, nil
}

func (req *TokenAccountAutomationCredentialWritebackRequest) normalize() {
	req.CredentialType = strings.ToLower(strings.TrimSpace(req.CredentialType))
	req.SourceJobID = strings.TrimSpace(req.SourceJobID)
	req.SecretRef = strings.TrimSpace(req.SecretRef)
	req.Fingerprint = strings.TrimSpace(req.Fingerprint)
}

func normalizeTokenAccountAutomationCredential(value any, credentialType string) (string, string, error) {
	raw, err := tokenAccountAutomationCredentialToString(value)
	if err != nil {
		return "", "", err
	}
	accountType := strings.ToLower(strings.TrimSpace(credentialType))
	if accountType == "" {
		accountType = tokenAccountAutomationCredentialTypeAuto
	}

	switch accountType {
	case tokenAccountAutomationCredentialTypeAuto:
		if compacted, ok := compactTokenAccountAutomationJSONCredential(raw); ok {
			if codexauth.LooksLikeOAuthJSONCredential(compacted) {
				return compacted, tokenAccountAutomationAccountTypeOAuthAccount, nil
			}
			return compacted, "", nil
		}
		return raw, "", nil
	case tokenAccountAutomationAccountTypeAPIKey:
		if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
			return "", "", errors.New("api_key credential_type does not accept JSON credentials")
		}
		return raw, tokenAccountAutomationAccountTypeAPIKey, nil
	case tokenAccountAutomationAccountTypeJSONAuth,
		tokenAccountAutomationAccountTypeOAuthAccount,
		tokenAccountAutomationAccountTypeTokenKey,
		tokenAccountAutomationAccountTypeSessionCookie,
		tokenAccountAutomationAccountTypeComposite:
		compacted, ok := compactTokenAccountAutomationJSONCredential(raw)
		if !ok {
			return "", "", errors.New("credential_type requires a JSON object credential")
		}
		return compacted, accountType, nil
	default:
		return "", "", errors.New("invalid credential_type")
	}
}

func tokenAccountAutomationCredentialToString(value any) (string, error) {
	if value == nil {
		return "", errors.New("credential is required")
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", errors.New("credential is required")
		}
		return text, nil
	}
	data, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return "", errors.New("credential is required")
	}
	return text, nil
}

func compactTokenAccountAutomationJSONCredential(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return "", false
	}
	var payload map[string]any
	if err := common.Unmarshal([]byte(raw), &payload); err != nil {
		return "", false
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func tokenAccountAutomationSetAccountType(channel *model.Channel, credentialIndex int, accountType string) {
	if channel == nil || credentialIndex < 0 {
		return
	}
	accountType = strings.ToLower(strings.TrimSpace(accountType))
	if accountType == "" {
		return
	}
	if channel.ChannelInfo.MultiKeyAccountTypes == nil {
		channel.ChannelInfo.MultiKeyAccountTypes = make(map[int]string)
	}
	channel.ChannelInfo.MultiKeyAccountTypes[credentialIndex] = accountType
}

func tokenAccountAutomationClearAuthErrorCapability(channel *model.Channel, credentialIndex int) bool {
	if channel == nil || channel.ChannelInfo.MultiKeyCapabilities == nil {
		return false
	}
	capability, ok := channel.ChannelInfo.MultiKeyCapabilities[credentialIndex]
	if !ok || strings.TrimSpace(capability.CapabilityClassification) != channelcapability.ClassificationAuthError {
		return false
	}
	capability.CapabilityClassification = ""
	capability.LastEndpoint = ""
	capability.LastMessage = ""
	capability.CheckedTime = common.GetTimestamp()
	channel.ChannelInfo.MultiKeyCapabilities[credentialIndex] = capability
	return true
}

func tokenAccountAutomationEnableRecoveredAccount(channel *model.Channel, credentialIndex int) bool {
	if channel == nil || credentialIndex < 0 || channel.ChannelInfo.MultiKeyStatusList == nil {
		return false
	}
	status, disabled := channel.ChannelInfo.MultiKeyStatusList[credentialIndex]
	if !disabled || status == common.ChannelStatusEnabled || status == common.ChannelStatusManuallyDisabled {
		return false
	}
	if reason := tokenAccountAutomationDisabledReason(channel, credentialIndex); reason != "" && !tokenAccountAutomationAutoAuthDisabledReason(reason) {
		return false
	}
	delete(channel.ChannelInfo.MultiKeyStatusList, credentialIndex)
	if channel.ChannelInfo.MultiKeyDisabledReason != nil {
		delete(channel.ChannelInfo.MultiKeyDisabledReason, credentialIndex)
		if len(channel.ChannelInfo.MultiKeyDisabledReason) == 0 {
			channel.ChannelInfo.MultiKeyDisabledReason = nil
		}
	}
	if channel.ChannelInfo.MultiKeyDisabledTime != nil {
		delete(channel.ChannelInfo.MultiKeyDisabledTime, credentialIndex)
		if len(channel.ChannelInfo.MultiKeyDisabledTime) == 0 {
			channel.ChannelInfo.MultiKeyDisabledTime = nil
		}
	}
	if len(channel.ChannelInfo.MultiKeyStatusList) == 0 {
		channel.ChannelInfo.MultiKeyStatusList = nil
	}
	return true
}

func tokenAccountAutomationDisabledReason(channel *model.Channel, credentialIndex int) string {
	if channel == nil || channel.ChannelInfo.MultiKeyDisabledReason == nil {
		return ""
	}
	return strings.TrimSpace(channel.ChannelInfo.MultiKeyDisabledReason[credentialIndex])
}

func tokenAccountAutomationAutoAuthDisabledReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	if normalized == "" {
		return true
	}
	if normalized == tokenAccountAutomationManualDisabledReason {
		return false
	}
	return strings.Contains(normalized, "auth") ||
		strings.Contains(normalized, "invalid") ||
		strings.Contains(normalized, "unauthorized") ||
		strings.Contains(normalized, "forbidden") ||
		strings.Contains(normalized, "401") ||
		strings.Contains(normalized, "403")
}

func tokenAccountAutomationReconcileChannelStatus(channel *model.Channel, keyCount int) {
	if channel == nil || keyCount <= 0 {
		return
	}
	enabledCount := 0
	for i := 0; i < keyCount; i++ {
		if tokenAccountAutomationAccountEnabled(channel, i) {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		if channel.Status == common.ChannelStatusEnabled ||
			(channel.Status == common.ChannelStatusAutoDisabled && tokenAccountAutomationStatusReasonIsAllKeysDisabled(channel)) {
			channel.Status = common.ChannelStatusAutoDisabled
			tokenAccountAutomationSetStatusReason(channel, tokenAccountAutomationAllKeysDisabledReason)
		}
		return
	}
	if channel.Status == common.ChannelStatusAutoDisabled && tokenAccountAutomationStatusReasonIsAllKeysDisabled(channel) {
		channel.Status = common.ChannelStatusEnabled
		tokenAccountAutomationClearStatusReason(channel)
	}
}

func tokenAccountAutomationAccountEnabled(channel *model.Channel, credentialIndex int) bool {
	if channel == nil || credentialIndex < 0 {
		return false
	}
	if channel.ChannelInfo.MultiKeyStatusList == nil {
		return true
	}
	status, ok := channel.ChannelInfo.MultiKeyStatusList[credentialIndex]
	return !ok || status == common.ChannelStatusEnabled
}

func tokenAccountAutomationStatusReasonIsAllKeysDisabled(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(channel.GetOtherInfo()["status_reason"])), tokenAccountAutomationAllKeysDisabledReason)
}

func tokenAccountAutomationSetStatusReason(channel *model.Channel, reason string) {
	info := channel.GetOtherInfo()
	info["status_reason"] = reason
	info["status_time"] = common.GetTimestamp()
	channel.SetOtherInfo(info)
}

func tokenAccountAutomationClearStatusReason(channel *model.Channel) {
	info := channel.GetOtherInfo()
	delete(info, "status_reason")
	delete(info, "status_time")
	channel.SetOtherInfo(info)
}
