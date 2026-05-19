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
