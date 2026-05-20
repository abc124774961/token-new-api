package integration_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProxyBridgeResolvesOnlySmartResponsesViaChatPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	bridge := integration.NewProxyBridge(nil)

	decision := bridge.Resolve(ctx, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses})
	require.False(t, decision.Enabled)
	require.Equal(t, "no_smart_dispatch_plan", decision.Reason)

	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	decision = bridge.Resolve(ctx, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses})
	require.True(t, decision.Enabled)
	require.Equal(t, provider.ProfileMiMoCodexChat, decision.ProviderProfile)
	require.Equal(t, provider.ProxyModeResponsesViaChat, decision.ProxyMode)
}

func TestProxyBridgeConvertsResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	input, err := common.Marshal([]map[string]any{
		{
			"role":    "user",
			"content": "hello",
		},
	})
	require.NoError(t, err)
	req := &dto.OpenAIResponsesRequest{
		Model: "mimo-v1",
		Input: input,
	}

	result, handled, err := integration.NewProxyBridge(nil).ConvertResponsesRequest(ctx, &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
	}, req)

	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", result.UpstreamPath)
	require.Contains(t, string(result.UpstreamBody), `"messages"`)
}

func TestProxyBridgeDisabledPlansBypassWithoutErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	input, err := common.Marshal([]map[string]any{
		{
			"role":    "user",
			"content": "hello",
		},
	})
	require.NoError(t, err)
	req := &dto.OpenAIResponsesRequest{
		Model: "mimo-v1",
		Input: input,
	}
	chatBody, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_disabled",
		"object":  "chat.completion",
		"created": 1710001600,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "bypass",
				},
				"finish_reason": "stop",
			},
		},
	})
	require.NoError(t, err)
	streamEvent, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_disabled_stream",
		"object":  "chat.completion.chunk",
		"created": 1710001601,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{"content": "bypass"},
			},
		},
	})
	require.NoError(t, err)

	type disabledCase struct {
		name       string
		plan       *core.DispatchPlan
		info       *relaycommon.RelayInfo
		wantReason string
	}
	cases := []disabledCase{
		{
			name:       "no selected plan",
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses},
			wantReason: "no_smart_dispatch_plan",
		},
		{
			name: "nil relay info",
			plan: &core.DispatchPlan{
				ProviderProfile: provider.ProfileMiMoCodexChat,
				ProxyMode:       provider.ProxyModeResponsesViaChat,
			},
			wantReason: "nil_relay_info",
		},
		{
			name: "unsupported relay mode",
			plan: &core.DispatchPlan{
				ProviderProfile: provider.ProfileMiMoCodexChat,
				ProxyMode:       provider.ProxyModeResponsesViaChat,
			},
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions},
			wantReason: "unsupported_relay_mode",
		},
		{
			name: "empty proxy mode",
			plan: &core.DispatchPlan{
				ProviderProfile: provider.ProfileMiMoCodexChat,
			},
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses},
			wantReason: "native_or_empty_proxy_mode",
		},
		{
			name: "native proxy mode",
			plan: &core.DispatchPlan{
				ProviderProfile: provider.ProfileMiMoCodexChat,
				ProxyMode:       provider.ProxyModeNative,
			},
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses},
			wantReason: "native_or_empty_proxy_mode",
		},
		{
			name: "native responses proxy mode",
			plan: &core.DispatchPlan{
				ProviderProfile: provider.ProfileOpenAICodex,
				ProxyMode:       provider.ProxyModeNativeResponses,
			},
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses},
			wantReason: "native_or_empty_proxy_mode",
		},
		{
			name: "missing provider profile",
			plan: &core.DispatchPlan{
				ProxyMode: provider.ProxyModeResponsesViaChat,
			},
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses},
			wantReason: "missing_provider_profile",
		},
		{
			name: "unsupported proxy mode",
			plan: &core.DispatchPlan{
				ProviderProfile: provider.ProfileMiMoCodexChat,
				ProxyMode:       provider.ProxyModeCodexCompatibleResponse,
			},
			info:       &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses},
			wantReason: "unsupported_proxy_mode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &gin.Context{}
			if tc.plan != nil {
				integration.SetSelectedPlan(ctx, tc.plan)
			}
			bridge := integration.NewProxyBridge(nil)

			decision := bridge.Resolve(ctx, tc.info)
			require.False(t, decision.Enabled)
			require.Equal(t, tc.wantReason, decision.Reason)

			requestResult, handled, err := bridge.ConvertResponsesRequest(ctx, tc.info, req)
			require.NoError(t, err)
			require.False(t, handled)
			require.Nil(t, requestResult)

			responseResult, handled, err := bridge.ConvertChatResponse(ctx, tc.info, chatBody)
			require.NoError(t, err)
			require.False(t, handled)
			require.Nil(t, responseResult)

			streamResult, handled, err := bridge.ConvertChatStream(ctx, tc.info, []string{string(streamEvent)})
			require.NoError(t, err)
			require.False(t, handled)
			require.Nil(t, streamResult)

			converter, handled, err := bridge.NewChatStreamConverter(ctx, tc.info)
			require.NoError(t, err)
			require.False(t, handled)
			require.Nil(t, converter)
		})
	}
}
