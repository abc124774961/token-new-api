package account

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	ProviderManualAPIKey = "manual_api_key"
	ProviderCodexOAuth   = "codex_oauth"
	ProviderOpenAI       = "openai"
	ProviderCodex        = "codex"
	ResolverChannelKey   = "channel_key"
)

type Registry struct{}

func NewRegistry() *Registry {
	return &Registry{}
}

type ChannelAccount struct {
	ResourceRef        core.ResourceRef              `json:"resource_ref"`
	AccountIdentity    core.AccountIdentity          `json:"account_identity"`
	CredentialRef      core.CredentialRef            `json:"credential_ref"`
	ProxyRef           core.ProxyRef                 `json:"proxy_ref,omitempty"`
	CodexEnvironmentID int                           `json:"codex_environment_id,omitempty"`
	MaxConcurrency     int                           `json:"max_concurrency,omitempty"`
	RateLimit          model.ChannelAccountRateLimit `json:"rate_limit,omitempty"`
	ChannelID          int                           `json:"channel_id"`
	CredentialIndex    int                           `json:"credential_index"`
	KeyEnabled         bool                          `json:"key_enabled"`
	DisabledReason     string                        `json:"disabled_reason,omitempty"`
}

func (r *Registry) AccountsForChannel(channel *model.Channel) []ChannelAccount {
	if channel == nil || channel.Id <= 0 {
		return nil
	}
	keys := channel.GetKeys()
	if len(keys) == 0 && strings.TrimSpace(channel.Key) != "" {
		keys = []string{channel.Key}
	}
	if len(keys) == 0 {
		return nil
	}
	resourceRef := ResourceRefForChannel(channel)
	accounts := make([]ChannelAccount, 0, len(keys))
	for idx, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		identity := AccountIdentityForChannelKey(channel, idx, key)
		accountResourceRef := resourceRef
		if identity.Provider != "" {
			accountResourceRef.Provider = identity.Provider
		}
		if identity.Brand != "" {
			accountResourceRef.Brand = identity.Brand
		}
		credentialRef := core.CredentialRef{
			ResourceID:                   accountResourceRef.ResourceID,
			AccountID:                    identity.AccountID,
			CredentialIndex:              idx,
			CredentialSubjectFingerprint: identity.CredentialSubjectFingerprint,
			CredentialFingerprint:        identity.CredentialFingerprint,
			Resolver:                     ResolverChannelKey,
		}
		enabled, reason := channelKeyEnabled(channel, idx)
		accounts = append(accounts, ChannelAccount{
			ResourceRef:        accountResourceRef,
			AccountIdentity:    identity,
			CredentialRef:      credentialRef,
			ProxyRef:           proxyRefForChannelKey(channel, idx),
			CodexEnvironmentID: CodexEnvironmentIDForChannelKey(channel, idx, key),
			MaxConcurrency:     channel.ChannelInfo.AccountMaxConcurrency(idx),
			RateLimit:          accountRateLimitForChannelKey(channel, idx),
			ChannelID:          channel.Id,
			CredentialIndex:    idx,
			KeyEnabled:         enabled,
			DisabledReason:     reason,
		})
	}
	return accounts
}

func ResourceRefForChannel(channel *model.Channel) core.ResourceRef {
	if channel == nil {
		return core.ResourceRef{}
	}
	return core.ResourceRef{
		ResourceID:         fmt.Sprintf("platform:channel:%d", channel.Id),
		ResourceType:       core.ResourceTypePlatformOwned,
		ExecutionBindingID: channel.Id,
		Provider:           providerForChannel(channel),
		Brand:              brandForChannel(channel),
	}
}

func AccountIdentityForChannelKey(channel *model.Channel, credentialIndex int, rawKey string) core.AccountIdentity {
	if channel == nil {
		return core.AccountIdentity{}
	}
	provider := providerForChannelKey(channel, rawKey)
	brand := brandForChannelKey(channel, rawKey)
	accountType := accountTypeForChannelKey(channel, credentialIndex, rawKey)
	planType := planTypeForChannelKey(channel, rawKey)
	subjectSource := subjectSourceForChannelKey(channel, rawKey)
	subjectFP := fingerprint(subjectSource)
	credentialFP := fingerprint(credentialSourceForKey(rawKey))
	accountIdentityKey := strings.Join([]string{provider, brand, subjectFP}, ":")
	accountUniqueKey := strings.Join([]string{strconv.Itoa(channel.Id), brand, subjectFP}, ":")
	return core.AccountIdentity{
		AccountID:                    accountIdentityKey,
		AccountType:                  accountType,
		PlanType:                     planType,
		Brand:                        brand,
		Provider:                     provider,
		CredentialIndex:              credentialIndex,
		CredentialSubjectFingerprint: subjectFP,
		CredentialFingerprint:        credentialFP,
		AccountIdentityKey:           accountIdentityKey,
		AccountUniqueKey:             accountUniqueKey,
		DisplayName:                  fmt.Sprintf("%s #%d", brand, credentialIndex+1),
		Status:                       statusForChannelKey(channel, credentialIndex),
	}
}

func planTypeForChannelKey(channel *model.Channel, rawKey string) string {
	if channel == nil {
		return ""
	}
	key := strings.TrimSpace(rawKey)
	if !strings.HasPrefix(key, "{") {
		return ""
	}
	if channel.Type == constant.ChannelTypeCodex || channel.Type == constant.ChannelTypeOpenAI {
		if oauthKey, ok := codexauth.ParseOAuthJSONCredentialLoose(key); ok {
			if planType := strings.ToLower(strings.TrimSpace(oauthKey.ChatGPTPlanType)); planType != "" {
				return planType
			}
		}
	}
	var payload map[string]interface{}
	if err := common.Unmarshal([]byte(key), &payload); err != nil {
		return ""
	}
	for _, field := range []string{"plan_type", "chatgpt_plan_type", "account_plan_type"} {
		if value, ok := payload[field].(string); ok {
			if planType := strings.ToLower(strings.TrimSpace(value)); planType != "" {
				return planType
			}
		}
	}
	return ""
}

func RuntimeKeyForChannelAccount(base core.RuntimeKey, account ChannelAccount) core.RuntimeKey {
	key := base
	key.ChannelID = account.ChannelID
	key.ResourceID = account.ResourceRef.ResourceID
	key.ResourceType = account.ResourceRef.ResourceType
	key.AccountID = account.AccountIdentity.AccountID
	key.AccountType = account.AccountIdentity.AccountType
	key.Brand = account.AccountIdentity.Brand
	key.Provider = account.AccountIdentity.Provider
	key.CredentialIndex = account.CredentialIndex
	key.CredentialSubjectFP = account.AccountIdentity.CredentialSubjectFingerprint
	key.CredentialFP = account.AccountIdentity.CredentialFingerprint
	return key
}

func CandidateAccountFields(candidate *core.Candidate, account ChannelAccount) {
	if candidate == nil {
		return
	}
	candidate.ResourceRef = account.ResourceRef
	candidate.AccountIdentity = account.AccountIdentity
	candidate.CredentialRef = account.CredentialRef
	candidate.ProxyRef = account.ProxyRef
	candidate.PoolLevel = core.CandidatePoolPro
	candidate.RuntimeKey = RuntimeKeyForChannelAccount(candidate.RuntimeKey, account)
}

func accountRateLimitForChannelKey(channel *model.Channel, index int) model.ChannelAccountRateLimit {
	if channel == nil {
		return model.ChannelAccountRateLimit{}
	}
	limit, ok := channel.ChannelInfo.AccountRateLimit(index)
	if !ok {
		return model.ChannelAccountRateLimit{}
	}
	return limit
}

func proxyRefForChannelKey(channel *model.Channel, index int) core.ProxyRef {
	if channel == nil || channel.ChannelInfo.MultiKeyProxyIDs == nil {
		return core.ProxyRef{}
	}
	proxyID := channel.ChannelInfo.MultiKeyProxyIDs[index]
	if proxyID <= 0 {
		return core.ProxyRef{}
	}
	return core.ProxyRef{ProxyID: proxyID}
}

func CodexEnvironmentIDForChannelKey(channel *model.Channel, index int, rawKey string) int {
	if channel == nil || index < 0 {
		return 0
	}
	if strings.TrimSpace(rawKey) != "" && channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys != nil {
		identity := AccountIdentityForChannelKey(channel, index, rawKey)
		accountUniqueKey := strings.TrimSpace(identity.AccountUniqueKey)
		if accountUniqueKey != "" {
			if environmentID := channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys[accountUniqueKey]; environmentID > 0 {
				return environmentID
			}
		}
	}
	if channel.ChannelInfo.MultiKeyCodexEnvironmentIDs != nil {
		if environmentID := channel.ChannelInfo.MultiKeyCodexEnvironmentIDs[index]; environmentID > 0 {
			return environmentID
		}
	}
	return 0
}

func providerForChannel(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	switch channel.Type {
	case constant.ChannelTypeCodex:
		return ProviderCodexOAuth
	case constant.ChannelTypeOpenAI:
		if channel.GetOtherSettings().UsesCodexCompatibilityMode() {
			return ProviderCodex
		}
		return ProviderOpenAI
	default:
		name := strings.TrimSpace(constant.GetChannelTypeName(channel.Type))
		if name == "" || strings.EqualFold(name, "unknown") {
			return ProviderManualAPIKey
		}
		return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	}
}

func brandForChannel(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	switch channel.Type {
	case constant.ChannelTypeCodex:
		return "codex"
	case constant.ChannelTypeOpenAI:
		if channel.GetOtherSettings().UsesCodexCompatibilityMode() {
			return "codex"
		}
		return "openai"
	default:
		name := strings.TrimSpace(constant.GetChannelTypeName(channel.Type))
		if name == "" || strings.EqualFold(name, "unknown") {
			return "unknown"
		}
		return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	}
}

func providerForChannelKey(channel *model.Channel, rawKey string) string {
	if channel != nil && channel.Type == constant.ChannelTypeOpenAI && codexauth.LooksLikeOAuthJSONCredential(rawKey) {
		return ProviderCodexOAuth
	}
	return providerForChannel(channel)
}

func brandForChannelKey(channel *model.Channel, rawKey string) string {
	if channel != nil && channel.Type == constant.ChannelTypeOpenAI && codexauth.LooksLikeOAuthJSONCredential(rawKey) {
		return "codex"
	}
	return brandForChannel(channel)
}

func accountTypeForChannelKey(channel *model.Channel, credentialIndex int, rawKey string) string {
	if channel == nil {
		return core.AccountTypeAPIKey
	}
	if channel.ChannelInfo.MultiKeyAccountTypes != nil {
		if accountType := strings.TrimSpace(channel.ChannelInfo.MultiKeyAccountTypes[credentialIndex]); accountType != "" {
			return accountType
		}
	}
	key := strings.TrimSpace(rawKey)
	if channel.Type == constant.ChannelTypeCodex || codexauth.LooksLikeOAuthJSONCredential(key) {
		return core.AccountTypeOAuthAccount
	}
	if accountType := accountTypeFromJSONCredential(key); accountType != "" {
		return accountType
	}
	if strings.HasPrefix(key, "{") {
		return core.AccountTypeJSONAuth
	}
	return core.AccountTypeAPIKey
}

func accountTypeFromJSONCredential(rawKey string) string {
	rawKey = strings.TrimSpace(rawKey)
	if !strings.HasPrefix(rawKey, "{") {
		return ""
	}
	var payload map[string]interface{}
	if err := common.Unmarshal([]byte(rawKey), &payload); err != nil {
		return ""
	}
	if hasAnyNonEmpty(payload, "account_type", "credential_type", "type") {
		for _, key := range []string{"account_type", "credential_type", "type"} {
			accountType := normalizeAccountType(fmt.Sprint(payload[key]))
			if accountType != "" {
				return accountType
			}
		}
	}
	if hasAnyNonEmpty(payload, "refresh_token", "access_token", "id_token", "client_id") {
		return core.AccountTypeOAuthAccount
	}
	if hasAnyNonEmpty(payload, "session_cookie", "cookie", "cookies", "session_token") {
		return core.AccountTypeSessionCookie
	}
	if hasAnyNonEmpty(payload, "token_key", "api_token") {
		return core.AccountTypeTokenKey
	}
	return ""
}

func normalizeAccountType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case core.AccountTypeAPIKey, "api-key", "apikey":
		return core.AccountTypeAPIKey
	case core.AccountTypeOAuthAccount, "oauth", "oauth_json", "oauth-json", "codex", "codex_oauth":
		return core.AccountTypeOAuthAccount
	case core.AccountTypeJSONAuth, "json", "json-auth":
		return core.AccountTypeJSONAuth
	case core.AccountTypeTokenKey, "token", "token-key":
		return core.AccountTypeTokenKey
	case core.AccountTypeSessionCookie, "session", "cookie", "session-cookie":
		return core.AccountTypeSessionCookie
	case core.AccountTypeComposite:
		return core.AccountTypeComposite
	default:
		return ""
	}
}

func hasAnyNonEmpty(payload map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
}

func subjectSourceForChannelKey(channel *model.Channel, rawKey string) string {
	key := strings.TrimSpace(rawKey)
	if channel != nil && (channel.Type == constant.ChannelTypeCodex || channel.Type == constant.ChannelTypeOpenAI) && strings.HasPrefix(key, "{") {
		if oauthKey, ok := codexauth.ParseOAuthJSONCredentialLoose(key); ok {
			accountID := strings.TrimSpace(oauthKey.AccountID)
			if accountID != "" {
				return "codex:account_id:" + accountID
			}
			email := strings.TrimSpace(strings.ToLower(oauthKey.Email))
			if email != "" {
				return "codex:email:" + email
			}
			refresh := strings.TrimSpace(oauthKey.RefreshToken)
			if refresh != "" {
				return "codex:refresh:" + refresh
			}
		}
	}
	return credentialSourceForKey(key)
}

func credentialSourceForKey(rawKey string) string {
	return strings.TrimSpace(rawKey)
}

func fingerprint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return common.GenerateHMAC(value)
}

func ShortFingerprint(fingerprint string) string {
	fingerprint = strings.TrimSpace(fingerprint)
	if len(fingerprint) <= 8 {
		return fingerprint
	}
	return fingerprint[:8]
}

func channelKeyEnabled(channel *model.Channel, index int) (bool, string) {
	if channel == nil {
		return false, ""
	}
	if !channel.ChannelInfo.IsMultiKey {
		return channel.Status == common.ChannelStatusEnabled, ""
	}
	status := common.ChannelStatusEnabled
	if channel.ChannelInfo.MultiKeyStatusList != nil {
		if value, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok {
			status = value
		}
	}
	reason := ""
	if channel.ChannelInfo.MultiKeyDisabledReason != nil {
		reason = strings.TrimSpace(channel.ChannelInfo.MultiKeyDisabledReason[index])
	}
	if capability, ok := channel.ChannelInfo.MultiKeyCapabilities[index]; ok && accountCapabilityBlocksScheduling(capability) {
		if reason == "" {
			reason = capability.EffectiveClassification()
		}
		return false, reason
	}
	return status == common.ChannelStatusEnabled, reason
}

func accountCapabilityBlocksScheduling(capability model.ChannelAccountCapability) bool {
	if capability.UsageLimitActiveAt(common.GetTimestamp()) {
		return true
	}
	return strings.TrimSpace(capability.CapabilityClassification) == channelcapability.ClassificationAuthError
}

func statusForChannelKey(channel *model.Channel, index int) string {
	enabled, _ := channelKeyEnabled(channel, index)
	if enabled {
		return "enabled"
	}
	return "disabled"
}
