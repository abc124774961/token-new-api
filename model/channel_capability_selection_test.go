package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelCapabilitySelectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGetNextEnabledKeyForEndpointSkipsCodexAccountsWithDeniedCapabilities(t *testing.T) {
	db := setupChannelCapabilitySelectionTestDB(t)
	streamDenied := false
	streamAllowed := true
	compactDenied := false
	compactAllowed := true
	channel := &Channel{
		Id:     9001,
		Type:   constant.ChannelTypeCodex,
		Key:    "oauth-a\noauth-b\noauth-c",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 0,
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamDenied,
					CodexBackendCompactWrite:         &compactDenied,
				},
				1: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
					CodexBackendCompactWrite:         &compactDenied,
				},
				2: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
					CodexBackendCompactWrite:         &compactAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	key, index, apiErr := channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponse, false)
	require.Nil(t, apiErr)
	require.Equal(t, "oauth-b", key)
	require.Equal(t, 1, index)

	key, index, apiErr = channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponseCompact, false)
	require.Nil(t, apiErr)
	require.Equal(t, "oauth-c", key)
	require.Equal(t, 2, index)
}

func TestGetNextEnabledKeyForEndpointSkipsUsageLimitedCodexAccount(t *testing.T) {
	db := setupChannelCapabilitySelectionTestDB(t)
	streamAllowed := true
	channel := &Channel{
		Id:     9004,
		Type:   constant.ChannelTypeCodex,
		Key:    "oauth-a\noauth-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 0,
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
					UsageLimitStatus:                 "limited",
					UsageLimitReason:                 "usage_limit_reached",
					UsageLimitExpiresAt:              common.GetTimestamp() + 3600,
				},
				1: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	key, index, apiErr := channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponse, false)

	require.Nil(t, apiErr)
	require.Equal(t, "oauth-b", key)
	require.Equal(t, 1, index)
}

func TestGetNextEnabledKeyForEndpointSkipsCodexImageGenerationUnsupportedAccount(t *testing.T) {
	db := setupChannelCapabilitySelectionTestDB(t)
	imageToolDenied := false
	imageToolAllowed := true
	channel := &Channel{
		Id:     9007,
		Type:   constant.ChannelTypeCodex,
		Key:    "oauth-a\noauth-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 0,
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					CodexImageGenerationTool: &imageToolDenied,
				},
				1: {
					CodexImageGenerationTool: &imageToolAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	key, index, apiErr := channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponse, true)
	require.Nil(t, apiErr)
	require.Equal(t, "oauth-b", key)
	require.Equal(t, 1, index)

	key, index, apiErr = channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponse, false)
	require.Nil(t, apiErr)
	require.Equal(t, "oauth-a", key)
	require.Equal(t, 0, index)
}

func TestGetNextEnabledKeyForRequestSkipsResponsesPreviousIDUnsupportedAccount(t *testing.T) {
	db := setupChannelCapabilitySelectionTestDB(t)
	previousIDDenied := false
	previousIDAllowed := true
	channel := &Channel{
		Id:            9008,
		Type:          constant.ChannelTypeOpenAI,
		Key:           "sk-denied\nsk-allowed",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: `{"wire_api":"responses"}`,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 0,
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					ResponsesPreviousID: &previousIDDenied,
				},
				1: {
					ResponsesPreviousID: &previousIDAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	key, index, apiErr := channel.GetNextEnabledKeyForRequest(constant.EndpointTypeOpenAIResponse, false, true)
	require.Nil(t, apiErr)
	require.Equal(t, "sk-allowed", key)
	require.Equal(t, 1, index)

	key, index, apiErr = channel.GetNextEnabledKeyForRequest(constant.EndpointTypeOpenAIResponse, false, false)
	require.Nil(t, apiErr)
	require.Equal(t, "sk-denied", key)
	require.Equal(t, 0, index)
}

func TestGetNextEnabledKeyForEndpointSkipsAuthErrorAccount(t *testing.T) {
	channel := &Channel{
		Id:     9005,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-auth\nsk-ok",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
				},
			},
		},
	}

	key, index, apiErr := channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAI, false)

	require.Nil(t, apiErr)
	require.Equal(t, "sk-ok", key)
	require.Equal(t, 1, index)
}

func TestGetNextEnabledKeyForEndpointRejectsSingleCodexAccountWithDeniedCapability(t *testing.T) {
	streamDenied := false
	channel := &Channel{
		Id:     9002,
		Type:   constant.ChannelTypeCodex,
		Key:    "oauth-a",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamDenied,
				},
			},
		},
	}

	key, index, apiErr := channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponse, false)

	require.NotNil(t, apiErr)
	require.Empty(t, key)
	require.Equal(t, 0, index)
}

func TestGetNextEnabledKeyForEndpointSkipsOpenAICodexOAuthAccountsWithDeniedCapabilities(t *testing.T) {
	db := setupChannelCapabilitySelectionTestDB(t)
	streamDenied := false
	streamAllowed := true
	channel := &Channel{
		Id:     9003,
		Type:   constant.ChannelTypeOpenAI,
		Key:    `{"access_token":"access-a","account_id":"acct-a"}` + "\n" + `{"access_token":"access-b","account_id":"acct-b"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 0,
			MultiKeyCapabilities: map[int]ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamDenied,
				},
				1: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	key, index, apiErr := channel.GetNextEnabledKeyForEndpoint(constant.EndpointTypeOpenAIResponse, false)

	require.Nil(t, apiErr)
	require.JSONEq(t, `{"access_token":"access-b","account_id":"acct-b"}`, key)
	require.Equal(t, 1, index)
}
