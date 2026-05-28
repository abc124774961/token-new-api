package integration_test

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/candidateindex"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/stretchr/testify/require"
)

func TestModelCandidatePoolBuilderUsesProviderRegistry(t *testing.T) {
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		ProviderProfile: provider.ProfileDeepSeekV4ProCodexChat,
		ProxyProfile:    provider.ProxyModeResponsesViaChat,
	})
	require.NoError(t, err)
	builder := integration.NewModelCandidatePoolBuilder()
	profile := builder.ProviderRegistry.Best(&model.Channel{
		Id:            9,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "compat-deepseek",
		OtherSettings: string(settingsBytes),
	}, "deepseek-v4-pro")

	require.NotNil(t, profile)
	require.Equal(t, provider.ProfileDeepSeekV4ProCodexChat, profile.Name())
	capability := profile.Capabilities(&model.Channel{OtherSettings: string(settingsBytes)}, "deepseek-v4-pro")
	require.NotEmpty(t, capability.CapabilityFingerprint)
	require.True(t, capability.ResponsesViaChat)
}

func TestIndexedCandidatePoolBuilderExpandsAccounts(t *testing.T) {
	common.CryptoSecret = "test-secret"
	index := candidateindex.New(nil, nil)
	index.Rebuild([]*model.Channel{
		{
			Id:     11,
			Type:   constant.ChannelTypeOpenAI,
			Key:    "sk-a\nsk-b",
			Status: common.ChannelStatusEnabled,
			Group:  "default",
			Models: "gpt-5.4",
			ChannelInfo: model.ChannelInfo{
				IsMultiKey: true,
			},
		},
	})
	builder := integration.NewIndexedCandidatePoolBuilder(index, 16)

	candidates := builder.Build(&core.DispatchRequest{
		RequestedGroup: "default",
		ModelName:      "gpt-5.4",
		EndpointType:   constant.EndpointTypeOpenAI,
	}, core.GroupSmartPolicy{})

	require.Len(t, candidates, 2)
	require.Equal(t, "default", candidates[0].RuntimeKey.Group)
	require.NotEmpty(t, candidates[0].RuntimeKey.AccountID)
	require.NotEqual(t, candidates[0].RuntimeKey.AccountID, candidates[1].RuntimeKey.AccountID)
	require.Equal(t, core.CandidatePoolPro, candidates[0].PoolLevel)
}

func TestAccountCandidatePrimaryBuilderUsesIndexedCandidates(t *testing.T) {
	primaryCandidate := core.Candidate{
		Channel: &model.Channel{Id: 21},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			ChannelID: 21,
			Group:     "default",
		},
	}
	shadowCandidate := core.Candidate{
		Channel: &model.Channel{Id: 22},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			ChannelID: 22,
			Group:     "default",
			AccountID: "shadow-account",
		},
	}
	builder := integration.NewAccountCandidatePrimaryBuilder(
		staticCandidateBuilder{candidates: []core.Candidate{shadowCandidate}},
		staticCandidateBuilder{candidates: []core.Candidate{primaryCandidate}},
		integration.AccountCandidateIndexOptions{
			ShadowLog:   true,
			LogInterval: time.Nanosecond,
		},
	)

	candidates := builder.Build(&core.DispatchRequest{
		RequestedGroup: "default",
		ModelName:      "gpt-5.4",
		EndpointType:   constant.EndpointTypeOpenAI,
	}, core.GroupSmartPolicy{})

	require.Equal(t, []core.Candidate{shadowCandidate}, candidates)
}

func TestAccountCandidatePrimaryBuilderFallsBackWhenIndexEmpty(t *testing.T) {
	primaryCandidate := core.Candidate{
		Channel: &model.Channel{Id: 21},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			ChannelID: 21,
			Group:     "default",
		},
	}
	builder := integration.NewAccountCandidatePrimaryBuilder(
		staticCandidateBuilder{},
		staticCandidateBuilder{candidates: []core.Candidate{primaryCandidate}},
		integration.AccountCandidateIndexOptions{},
	)

	candidates := builder.Build(&core.DispatchRequest{
		RequestedGroup: "default",
		ModelName:      "gpt-5.4",
		EndpointType:   constant.EndpointTypeOpenAI,
	}, core.GroupSmartPolicy{})

	require.Equal(t, []core.Candidate{primaryCandidate}, candidates)
}

type staticCandidateBuilder struct {
	candidates []core.Candidate
}

func (b staticCandidateBuilder) Build(_ *core.DispatchRequest, _ core.GroupSmartPolicy) []core.Candidate {
	return append([]core.Candidate(nil), b.candidates...)
}
