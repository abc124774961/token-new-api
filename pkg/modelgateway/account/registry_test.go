package account

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/stretchr/testify/require"
)

func TestRegistryBuildsDefaultAccountForSingleKeyChannel(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     42,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 1)
	account := accounts[0]
	require.Equal(t, "platform:channel:42", account.ResourceRef.ResourceID)
	require.Equal(t, core.ResourceTypePlatformOwned, account.ResourceRef.ResourceType)
	require.Equal(t, 42, account.ResourceRef.ExecutionBindingID)
	require.Equal(t, "openai", account.AccountIdentity.Brand)
	require.Equal(t, core.AccountTypeAPIKey, account.AccountIdentity.AccountType)
	require.Equal(t, 0, account.CredentialIndex)
	require.True(t, account.KeyEnabled)
	require.NotEmpty(t, account.AccountIdentity.CredentialSubjectFingerprint)
	require.Equal(t, account.AccountIdentity.CredentialSubjectFingerprint, account.CredentialRef.CredentialSubjectFingerprint)
}

func TestRegistryBuildsAccountsForMultiKeyChannel(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     7,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two\nsk-three",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusManuallyDisabled,
			},
			MultiKeyDisabledReason: map[int]string{
				1: "quota exhausted",
			},
		},
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 3)
	require.True(t, accounts[0].KeyEnabled)
	require.False(t, accounts[1].KeyEnabled)
	require.Equal(t, "quota exhausted", accounts[1].DisabledReason)
	require.Equal(t, "disabled", accounts[1].AccountIdentity.Status)
	require.True(t, accounts[2].KeyEnabled)
	require.NotEqual(t, accounts[0].AccountIdentity.AccountID, accounts[1].AccountIdentity.AccountID)
	require.Equal(t, 2, accounts[2].CredentialIndex)
}

func TestRegistryMarksCapabilityBlockedAccountsDisabled(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     77,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-auth\nsk-usage\nsk-ok",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
				},
				1: {
					UsageLimitStatus:    channelcapability.UsageLimitStatusLimited,
					UsageLimitExpiresAt: common.GetTimestamp() + 3600,
				},
			},
		},
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 3)
	require.False(t, accounts[0].KeyEnabled)
	require.Equal(t, channelcapability.ClassificationAuthError, accounts[0].DisabledReason)
	require.False(t, accounts[1].KeyEnabled)
	require.Equal(t, channelcapability.ClassificationAccountUsageLimited, accounts[1].DisabledReason)
	require.True(t, accounts[2].KeyEnabled)
}

func TestRegistryMarksSingleKeyCapabilityBlockedAccountDisabled(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     78,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"auth","account_id":"auth"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
				},
			},
		},
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 1)
	require.False(t, accounts[0].KeyEnabled)
	require.Equal(t, channelcapability.ClassificationAuthError, accounts[0].DisabledReason)
	require.Equal(t, "disabled", accounts[0].AccountIdentity.Status)
}

func TestRegistryCarriesAccountProxyRef(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     71,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:       true,
			MultiKeyProxyIDs: map[int]int{1: 9001},
		},
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 2)
	require.Zero(t, accounts[0].ProxyRef.ProxyID)
	require.Equal(t, 9001, accounts[1].ProxyRef.ProxyID)
}

func TestRegistryDetectsJSONAuthPerMultiKeyEntry(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     8,
		Type:   constant.ChannelTypeOpenAI,
		Key:    `{"account_id":"acct-1","secret":"a"}` + "\n" + `{"account_id":"acct-2","secret":"b"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
		},
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 2)
	require.Equal(t, core.AccountTypeJSONAuth, accounts[0].AccountIdentity.AccountType)
	require.Equal(t, core.AccountTypeJSONAuth, accounts[1].AccountIdentity.AccountType)
}

func TestRegistryUsesMultiKeyAccountTypeOverride(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     18,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\n" + `{"account_id":"acct-2","refresh_token":"rt-2"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeyAccountTypes: map[int]string{0: core.AccountTypeTokenKey, 1: core.AccountTypeOAuthAccount},
		},
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 2)
	require.Equal(t, core.AccountTypeTokenKey, accounts[0].AccountIdentity.AccountType)
	require.Equal(t, core.AccountTypeOAuthAccount, accounts[1].AccountIdentity.AccountType)
}

func TestRegistryInfersOAuthAccountFromJSONCredential(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     19,
		Type:   constant.ChannelTypeOpenAI,
		Key:    `{"account_id":"acct-json","access_token":"access","refresh_token":"refresh"}`,
		Status: common.ChannelStatusEnabled,
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 1)
	require.Equal(t, core.AccountTypeOAuthAccount, accounts[0].AccountIdentity.AccountType)
}

func TestRegistryCodexOAuthSubjectFingerprintStableAcrossAccessTokenRefresh(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     9,
		Type:   constant.ChannelTypeCodex,
		Status: common.ChannelStatusEnabled,
	}
	channelA := *channel
	channelA.Key = `{"account_id":"acct-123","email":"a@example.com","access_token":"access-a","refresh_token":"refresh-a","expired":"2026-01-01T00:00:00Z"}`
	channelB := *channel
	channelB.Key = `{"account_id":"acct-123","email":"a@example.com","access_token":"access-b","refresh_token":"refresh-b","expired":"2026-02-01T00:00:00Z"}`

	accountA := NewRegistry().AccountsForChannel(&channelA)
	accountB := NewRegistry().AccountsForChannel(&channelB)

	require.Len(t, accountA, 1)
	require.Len(t, accountB, 1)
	require.Equal(t, "codex", accountA[0].AccountIdentity.Brand)
	require.Equal(t, core.AccountTypeOAuthAccount, accountA[0].AccountIdentity.AccountType)
	require.Equal(t, accountA[0].AccountIdentity.CredentialSubjectFingerprint, accountB[0].AccountIdentity.CredentialSubjectFingerprint)
	require.Equal(t, accountA[0].AccountIdentity.AccountID, accountB[0].AccountIdentity.AccountID)
	require.NotEqual(t, accountA[0].AccountIdentity.CredentialFingerprint, accountB[0].AccountIdentity.CredentialFingerprint)
}

func TestRegistryOpenAIOAuthJSONAccountUsesCodexIdentity(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     20,
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
	}
	channelA := *channel
	channelA.Key = `{"account_id":"acct-123","email":"a@example.com","access_token":"access-a","refresh_token":"refresh-a"}`
	channelB := *channel
	channelB.Key = `{"account_id":"acct-123","email":"a@example.com","access_token":"access-b","refresh_token":"refresh-b"}`

	accountA := NewRegistry().AccountsForChannel(&channelA)
	accountB := NewRegistry().AccountsForChannel(&channelB)

	require.Len(t, accountA, 1)
	require.Len(t, accountB, 1)
	require.Equal(t, ProviderCodexOAuth, accountA[0].AccountIdentity.Provider)
	require.Equal(t, "codex", accountA[0].AccountIdentity.Brand)
	require.Equal(t, ProviderCodexOAuth, accountA[0].ResourceRef.Provider)
	require.Equal(t, "codex", accountA[0].ResourceRef.Brand)
	require.Equal(t, core.AccountTypeOAuthAccount, accountA[0].AccountIdentity.AccountType)
	require.Equal(t, accountA[0].AccountIdentity.CredentialSubjectFingerprint, accountB[0].AccountIdentity.CredentialSubjectFingerprint)
	require.Equal(t, accountA[0].AccountIdentity.AccountID, accountB[0].AccountIdentity.AccountID)
	require.NotEqual(t, accountA[0].AccountIdentity.CredentialFingerprint, accountB[0].AccountIdentity.CredentialFingerprint)
}

func TestRegistryOpenAIOAuthJSONWithoutAccountIDStillUsesCodexIdentity(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     21,
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Key:    `{"access_token":"access-a","email":"missing-account@example.com","chatgpt_user_id":"user-a"}`,
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 1)
	require.Equal(t, ProviderCodexOAuth, accounts[0].AccountIdentity.Provider)
	require.Equal(t, "codex", accounts[0].AccountIdentity.Brand)
	require.Equal(t, core.AccountTypeOAuthAccount, accounts[0].AccountIdentity.AccountType)
	require.Equal(t, "codex", accounts[0].ResourceRef.Brand)
}

func TestRegistryOpenAIOAuthJWTClaimsExposePlanType(t *testing.T) {
	common.CryptoSecret = "test-secret"
	accessToken := testRegistryJWT(t, map[string]interface{}{
		"iss": "https://auth.openai.com",
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-team-123",
			"chatgpt_plan_type":  "team",
			"chatgpt_user_id":    "user-fake",
		},
		"https://api.openai.com/profile": map[string]interface{}{
			"email": "team-user@example.com",
		},
	})
	channel := &model.Channel{
		Id:     22,
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Key:    `{"type":"codex","access_token":"` + accessToken + `","refresh_token":"rt_fake_refresh"}`,
	}

	accounts := NewRegistry().AccountsForChannel(channel)

	require.Len(t, accounts, 1)
	require.Equal(t, ProviderCodexOAuth, accounts[0].AccountIdentity.Provider)
	require.Equal(t, core.AccountTypeOAuthAccount, accounts[0].AccountIdentity.AccountType)
	require.Equal(t, "team", accounts[0].AccountIdentity.PlanType)
}

func TestRuntimeKeyForChannelAccountAddsAccountScope(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     11,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
	}
	accounts := NewRegistry().AccountsForChannel(channel)
	require.Len(t, accounts, 1)

	key := RuntimeKeyForChannelAccount(core.RuntimeKey{RequestedModel: "gpt-5.4", Group: "default"}, accounts[0])

	require.Equal(t, 11, key.ChannelID)
	require.Equal(t, "platform:channel:11", key.ResourceID)
	require.NotEmpty(t, key.AccountID)
	require.Equal(t, core.AccountTypeAPIKey, key.AccountType)
	require.Equal(t, "openai", key.Brand)
	require.Equal(t, accounts[0].AccountIdentity.CredentialFingerprint, key.CredentialFP)
}

func testRegistryJWT(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	header, err := common.Marshal(map[string]interface{}{"alg": "none"})
	require.NoError(t, err)
	payload, err := common.Marshal(claims)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
