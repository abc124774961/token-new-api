package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenAccountAutomationWritebackDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		clearAllChannelFailureAvoidanceForTest()
		clearAllChannelConfigIsolationForTest()
	})
	clearAllChannelFailureAvoidanceForTest()
	clearAllChannelConfigIsolationForTest()
	return db
}

func TestWritebackTokenAccountAutomationCredentialUpdatesOnlyTargetAccount(t *testing.T) {
	db := setupTokenAccountAutomationWritebackDB(t)
	channel := model.Channel{
		Id:     91001,
		Name:   "automation-writeback",
		Status: common.ChannelStatusAutoDisabled,
		Key:    `{"access_token":"old-a","refresh_token":"old-refresh-a","account_id":"acct-a"}` + "\n" + `{"access_token":"old-b","refresh_token":"old-refresh-b","account_id":"acct-b"}`,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{0: "auth_error"},
			MultiKeyDisabledTime:   map[int]int64{0: 1700000000},
			MultiKeyAccountTypes:   map[int]string{0: tokenAccountAutomationAccountTypeOAuthAccount, 1: tokenAccountAutomationAccountTypeOAuthAccount},
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
					LastEndpoint:             "auth",
					LastMessage:              "401 invalid token",
					CheckedTime:              1700000001,
					PlatformResponsesWrite:   common.GetPointer(true),
				},
				1: {
					CapabilityClassification: channelcapability.ClassificationCodexBackendAvailable,
					LastEndpoint:             "responses",
					LastMessage:              "ok",
					CheckedTime:              1700000002,
				},
			},
		},
	}
	channel.SetOtherInfo(map[string]any{"status_reason": tokenAccountAutomationAllKeysDisabledReason, "status_time": int64(1700000003)})
	require.NoError(t, db.Create(&channel).Error)

	accountIdentity := ChannelRuntimeIdentity{ChannelID: channel.Id, CredentialIndex: 0, CredentialIndexSet: true}
	accountBIdentity := ChannelRuntimeIdentity{ChannelID: channel.Id, CredentialIndex: 1, CredentialIndexSet: true}
	require.NotNil(t, RecordChannelRuntimeFailureAvoidanceWithContext(accountIdentity, "upstream_error:502", nil))
	require.NotNil(t, RecordChannelRuntimeFailureAvoidanceWithContext(accountBIdentity, "upstream_error:502", nil))
	RecordChannelConfigAuthError(NewChannelRuntimeConfigIsolationKey(accountIdentity, "", "", ""), "auth")

	result, err := WritebackTokenAccountAutomationCredential(TokenAccountAutomationCredentialWritebackRequest{
		ChannelID:       channel.Id,
		CredentialIndex: 0,
		CredentialType:  tokenAccountAutomationAccountTypeOAuthAccount,
		Credential: map[string]any{
			"access_token":  "new-a",
			"refresh_token": "new-refresh-a",
			"account_id":    "acct-a",
			"provider":      "codex",
		},
		Fingerprint: "new-fp",
	})
	require.NoError(t, err)
	require.True(t, result.AccountEnabled)
	require.True(t, result.ClearedAuthError)
	require.True(t, result.ClearedAutoDisabled)
	require.Equal(t, common.ChannelStatusEnabled, result.ChannelStatus)

	var updated model.Channel
	require.NoError(t, db.First(&updated, "id = ?", channel.Id).Error)
	keys := updated.GetKeys()
	require.Len(t, keys, 2)
	require.Contains(t, keys[0], `"new-refresh-a"`)
	require.Contains(t, keys[0], `"new-a"`)
	require.Contains(t, keys[1], `"old-b"`)
	require.Nil(t, updated.ChannelInfo.MultiKeyStatusList)
	require.Equal(t, tokenAccountAutomationAccountTypeOAuthAccount, updated.ChannelInfo.MultiKeyAccountTypes[0])
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[0].CapabilityClassification)
	require.True(t, *updated.ChannelInfo.MultiKeyCapabilities[0].PlatformResponsesWrite)
	require.Equal(t, channelcapability.ClassificationCodexBackendAvailable, updated.ChannelInfo.MultiKeyCapabilities[1].CapabilityClassification)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.GetOtherInfo(), "status_reason")
	require.Nil(t, GetChannelRuntimeFailureAvoidanceStatus(accountIdentity))
	require.NotNil(t, GetChannelRuntimeFailureAvoidanceStatus(accountBIdentity))
	require.Nil(t, GetChannelConfigIsolationStatus(NewChannelRuntimeConfigIsolationKey(accountIdentity, "", "", "")))
}

func TestWritebackTokenAccountAutomationCredentialDoesNotEnableManualDisabledAccount(t *testing.T) {
	db := setupTokenAccountAutomationWritebackDB(t)
	channel := model.Channel{
		Id:     91002,
		Name:   "automation-writeback-manual",
		Status: common.ChannelStatusEnabled,
		Key:    "old-a\nold-b",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusManuallyDisabled},
			MultiKeyDisabledReason: map[int]string{0: "manual"},
			MultiKeyDisabledTime:   map[int]int64{0: 1700000000},
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	result, err := WritebackTokenAccountAutomationCredential(TokenAccountAutomationCredentialWritebackRequest{
		ChannelID:       channel.Id,
		CredentialIndex: 0,
		CredentialType:  tokenAccountAutomationAccountTypeAPIKey,
		Credential:      "new-a",
	})
	require.NoError(t, err)
	require.False(t, result.AccountEnabled)
	require.False(t, result.ClearedAutoDisabled)

	var updated model.Channel
	require.NoError(t, db.First(&updated, "id = ?", channel.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, "manual", updated.ChannelInfo.MultiKeyDisabledReason[0])
	require.Equal(t, "new-a", updated.GetKeys()[0])
}

func TestWritebackTokenAccountAutomationCredentialEnablesSingleKeyAuthDisabledChannel(t *testing.T) {
	db := setupTokenAccountAutomationWritebackDB(t)
	channel := model.Channel{
		Id:     91003,
		Name:   "automation-writeback-single-auth",
		Status: common.ChannelStatusAutoDisabled,
		Key:    `{"access_token":"old-a","refresh_token":"old-refresh-a","account_id":"acct-a"}`,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   false,
			MultiKeySize: 1,
			MultiKeyAccountTypes: map[int]string{
				0: tokenAccountAutomationAccountTypeOAuthAccount,
			},
		},
	}
	channel.SetOtherInfo(map[string]any{"status_reason": "auth_reauthorization_pending", "status_time": int64(1700000003)})
	require.NoError(t, db.Create(&channel).Error)

	result, err := WritebackTokenAccountAutomationCredential(TokenAccountAutomationCredentialWritebackRequest{
		ChannelID:       channel.Id,
		CredentialIndex: 0,
		CredentialType:  tokenAccountAutomationAccountTypeOAuthAccount,
		Credential: map[string]any{
			"access_token":  "new-a",
			"refresh_token": "new-refresh-a",
			"account_id":    "acct-a",
			"provider":      "codex",
		},
	})
	require.NoError(t, err)
	require.True(t, result.AccountEnabled)
	require.Equal(t, common.ChannelStatusEnabled, result.ChannelStatus)

	var updated model.Channel
	require.NoError(t, db.First(&updated, "id = ?", channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.GetOtherInfo(), "status_reason")
	require.Contains(t, updated.GetKeys()[0], `"new-refresh-a"`)
}
