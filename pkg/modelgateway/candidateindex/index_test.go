package candidateindex

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/account"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProIndexExpandsMultiKeyChannelIntoAccountCandidates(t *testing.T) {
	common.CryptoSecret = "test-secret"
	priority := int64(10)
	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:       1,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-a\nsk-b\nsk-c",
		Status:   common.ChannelStatusEnabled,
		Name:     "openai-multi",
		Group:    "default",
		Models:   "gpt-5.4",
		Priority: &priority,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusManuallyDisabled,
			},
		},
	}

	stats := index.Rebuild([]*model.Channel{channel})
	candidates := index.Query(Query{
		Groups:       []string{"default"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Equal(t, 1, stats.Channels)
	require.Equal(t, 3, stats.Accounts)
	require.Equal(t, 1, stats.DisabledKeys)
	require.Equal(t, 2, stats.Candidates)
	require.Len(t, candidates, 2)
	require.Equal(t, core.CandidatePoolPro, candidates[0].PoolLevel)
	require.Equal(t, 0, candidates[0].CredentialRef.CredentialIndex)
	require.Equal(t, 2, candidates[1].CredentialRef.CredentialIndex)
	require.NotEqual(t, candidates[0].RuntimeKey.AccountID, candidates[1].RuntimeKey.AccountID)
	require.Equal(t, constant.EndpointTypeOpenAI, candidates[0].RuntimeKey.EndpointType)
}

func TestProIndexQueryAppliesEndpointToolAndExclusionFilters(t *testing.T) {
	common.CryptoSecret = "test-secret"
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		CodexCompatibilityMode:            true,
		CodexImageGenerationToolSupported: true,
	})
	require.NoError(t, err)
	index := New(account.NewRegistry(), nil)
	imageToolChannel := &model.Channel{
		Id:            2,
		Type:          constant.ChannelTypeOpenAI,
		Key:           "sk-image",
		Status:        common.ChannelStatusEnabled,
		Group:         "codex",
		Models:        "gpt-5.4",
		OtherSettings: string(settingsBytes),
	}
	plainChannel := &model.Channel{
		Id:     3,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-plain",
		Status: common.ChannelStatusEnabled,
		Group:  "codex",
		Models: "gpt-5.4",
	}

	index.Rebuild([]*model.Channel{plainChannel, imageToolChannel})
	candidates := index.Query(Query{
		Groups:                 []string{"codex"},
		ModelName:              "gpt-5.4",
		EndpointType:           constant.EndpointTypeOpenAIResponseCompact,
		RequiresCodexImageTool: true,
	})

	require.Len(t, candidates, 1)
	require.Equal(t, imageToolChannel.Id, candidates[0].RuntimeKey.ChannelID)
	require.True(t, candidates[0].RequiresCodexImageTool)

	excluded := index.Query(Query{
		Groups:                 []string{"codex"},
		ModelName:              "gpt-5.4",
		EndpointType:           constant.EndpointTypeOpenAIResponseCompact,
		RequiresCodexImageTool: true,
		ExcludedChannelIDs: map[int]struct{}{
			imageToolChannel.Id: {},
		},
	})
	require.Empty(t, excluded)
}

func TestProIndexQueryFiltersCodexAccountCapabilities(t *testing.T) {
	common.CryptoSecret = "test-secret"
	streamDenied := false
	streamAllowed := true
	compactDenied := false
	compactAllowed := true
	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:     6,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"a","account_id":"a"}` + "\n" + `{"access_token":"b","account_id":"b"}` + "\n" + `{"access_token":"c","account_id":"c"}`,
		Status: common.ChannelStatusEnabled,
		Group:  "codex",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
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

	index.Rebuild([]*model.Channel{channel})
	responseCandidates := index.Query(Query{
		Groups:       []string{"codex"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAIResponse,
	})
	compactCandidates := index.Query(Query{
		Groups:       []string{"codex"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAIResponseCompact,
	})

	require.Len(t, responseCandidates, 2)
	require.Equal(t, 1, responseCandidates[0].CredentialRef.CredentialIndex)
	require.Equal(t, 2, responseCandidates[1].CredentialRef.CredentialIndex)
	require.Len(t, compactCandidates, 1)
	require.Equal(t, 2, compactCandidates[0].CredentialRef.CredentialIndex)
}

func TestProIndexQueryFiltersUsageLimitedCodexAccount(t *testing.T) {
	common.CryptoSecret = "test-secret"
	streamAllowed := true
	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:     16,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"a","account_id":"a"}` + "\n" + `{"access_token":"b","account_id":"b"}`,
		Status: common.ChannelStatusEnabled,
		Group:  "codex",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
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

	index.Rebuild([]*model.Channel{channel})
	candidates := index.Query(Query{
		Groups:       []string{"codex"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAIResponse,
	})

	require.Len(t, candidates, 1)
	require.Equal(t, 1, candidates[0].CredentialRef.CredentialIndex)
}

func TestProIndexQueryFiltersUnavailableAccountProxy(t *testing.T) {
	common.CryptoSecret = "test-secret"
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayProxy{}))
	oldDB := model.DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       9101,
		Name:     "disabled",
		Protocol: model.ModelGatewayProxyProtocolSOCKS5,
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	require.NoError(t, db.Model(&model.ModelGatewayProxy{}).Where("id = ?", 9101).Update("enabled", false).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       9102,
		Name:     "enabled",
		Protocol: model.ModelGatewayProxyProtocolSOCKS5,
		Address:  "127.0.0.1:1081",
		Enabled:  true,
	}).Error)

	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:     17,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-disabled-proxy\nsk-enabled-proxy",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:       true,
			MultiKeyProxyIDs: map[int]int{0: 9101, 1: 9102},
		},
	}

	index.Rebuild([]*model.Channel{channel})
	candidates := index.Query(Query{
		Groups:       []string{"default"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Len(t, candidates, 1)
	require.Equal(t, 1, candidates[0].CredentialRef.CredentialIndex)
	require.Equal(t, 9102, candidates[0].ProxyRef.ProxyID)
}

func TestProIndexQueryFiltersSingleCodexAccountDeniedCapability(t *testing.T) {
	common.CryptoSecret = "test-secret"
	streamDenied := false
	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:     7,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"a","account_id":"a"}`,
		Status: common.ChannelStatusEnabled,
		Group:  "codex",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamDenied,
				},
			},
		},
	}

	index.Rebuild([]*model.Channel{channel})
	candidates := index.Query(Query{
		Groups:       []string{"codex"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAIResponse,
	})

	require.Empty(t, candidates)
}

func TestProIndexQueryFiltersOpenAICodexOAuthAccountCapabilities(t *testing.T) {
	common.CryptoSecret = "test-secret"
	streamDenied := false
	streamAllowed := true
	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:     8,
		Type:   constant.ChannelTypeOpenAI,
		Key:    `{"access_token":"access-a","account_id":"acct-a"}` + "\n" + `{"access_token":"access-b","account_id":"acct-b"}`,
		Status: common.ChannelStatusEnabled,
		Group:  "codex",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamDenied,
				},
				1: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}

	index.Rebuild([]*model.Channel{channel})
	candidates := index.Query(Query{
		Groups:       []string{"codex"},
		ModelName:    "gpt-5.4",
		EndpointType: constant.EndpointTypeOpenAIResponse,
	})

	require.Len(t, candidates, 1)
	require.Equal(t, 1, candidates[0].CredentialRef.CredentialIndex)
	require.Equal(t, account.ProviderCodexOAuth, candidates[0].AccountIdentity.Provider)
	require.Equal(t, "codex", candidates[0].AccountIdentity.Brand)
}

func TestProIndexFallsBackToNormalizedModelButKeepsRequestedRuntimeModel(t *testing.T) {
	common.CryptoSecret = "test-secret"
	index := New(account.NewRegistry(), nil)
	channel := &model.Channel{
		Id:     4,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-normalized",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-4o-gizmo-*",
	}

	index.Rebuild([]*model.Channel{channel})
	candidates := index.Query(Query{
		Groups:       []string{"default"},
		ModelName:    "gpt-4o-gizmo-test",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Len(t, candidates, 1)
	require.Equal(t, "gpt-4o-gizmo-test", candidates[0].RuntimeKey.RequestedModel)
	require.Equal(t, "gpt-4o-gizmo-test", candidates[0].UpstreamModel)
}

func TestProIndexRebuildIsCopyOnWrite(t *testing.T) {
	common.CryptoSecret = "test-secret"
	index := New(account.NewRegistry(), nil)
	first := &model.Channel{
		Id:     5,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-first",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
	}
	second := &model.Channel{
		Id:     6,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-second",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
	}

	firstStats := index.Rebuild([]*model.Channel{first})
	firstCandidates := index.Query(Query{Groups: []string{"default"}, ModelName: "gpt-5.4", EndpointType: constant.EndpointTypeOpenAI})
	secondStats := index.Rebuild([]*model.Channel{second})
	secondCandidates := index.Query(Query{Groups: []string{"default"}, ModelName: "gpt-5.4", EndpointType: constant.EndpointTypeOpenAI})

	require.Equal(t, int64(1), firstStats.Version)
	require.Equal(t, int64(2), secondStats.Version)
	require.Len(t, firstCandidates, 1)
	require.Equal(t, first.Id, firstCandidates[0].RuntimeKey.ChannelID)
	require.Len(t, secondCandidates, 1)
	require.Equal(t, second.Id, secondCandidates[0].RuntimeKey.ChannelID)
}

func BenchmarkProIndexQuery100kAccounts(b *testing.B) {
	common.CryptoSecret = "test-secret"
	index := New(account.NewRegistry(), nil)
	channels := make([]*model.Channel, 0, 1000)
	for channelID := 1; channelID <= 1000; channelID++ {
		keys := make([]byte, 0, 16*100)
		for keyIndex := 0; keyIndex < 100; keyIndex++ {
			if keyIndex > 0 {
				keys = append(keys, '\n')
			}
			keys = append(keys, fmt.Sprintf("sk-%d-%d", channelID, keyIndex)...)
		}
		priority := int64(channelID % 10)
		channels = append(channels, &model.Channel{
			Id:       channelID,
			Type:     constant.ChannelTypeOpenAI,
			Key:      string(keys),
			Status:   common.ChannelStatusEnabled,
			Group:    "default,pro",
			Models:   "gpt-5.4,gpt-5.5",
			Priority: &priority,
			ChannelInfo: model.ChannelInfo{
				IsMultiKey: true,
			},
		})
	}
	index.Rebuild(channels)
	query := Query{
		Groups:        []string{"pro"},
		ModelName:     "gpt-5.4",
		EndpointType:  constant.EndpointTypeOpenAI,
		MaxCandidates: 256,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		candidates := index.Query(query)
		if len(candidates) != 256 {
			b.Fatalf("expected 256 candidates, got %d", len(candidates))
		}
	}
}
