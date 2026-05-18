package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/stretchr/testify/require"
)

func TestResponsesRequestFromCompactionPreservesReasoning(t *testing.T) {
	t.Parallel()

	compactReq := &dto.OpenAIResponsesCompactionRequest{
		Model:     "gpt-5.5",
		Reasoning: &dto.Reasoning{Effort: "xhigh"},
	}

	responsesReq := responsesRequestFromRelayRequest(compactReq)

	require.NotNil(t, responsesReq)
	require.NotNil(t, responsesReq.Reasoning)
	require.Equal(t, "xhigh", responsesReq.Reasoning.Effort)
}

func TestCaptureResponsesReasoningEffort(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{}
	req := &dto.OpenAIResponsesRequest{
		Model:     "gpt-5.5",
		Reasoning: &dto.Reasoning{Effort: "xhigh"},
	}

	captureResponsesReasoningEffort(info, req)

	require.Equal(t, "xhigh", info.ReasoningEffort)
	require.Equal(t, "xhigh", info.RequestReasoningEffort)
}

func TestExplicitImageGenerationToolUpdatesResponsesUsageInfo(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Tools: []byte(`[{"type":"image_generation"}]`),
	}

	usageInfo := relaycommon.BuildResponsesUsageInfo(req)

	require.Contains(t, usageInfo.BuiltInTools, dto.BuildInToolImageGeneration)
}

func TestResponsesRequestHasToolDetectsToolChoice(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model:      "gpt-5.5",
		ToolChoice: []byte(`{"type":"image_generation"}`),
	}

	require.True(t, req.HasTool(dto.BuildInToolImageGeneration))
}

func TestResponsesRequestHasToolDetectsNestedToolChoice(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model:      "gpt-5.5",
		ToolChoice: []byte(`{"type":"allowed_tools","tools":[{"type":"image_generation"}]}`),
	}

	require.True(t, req.HasTool(dto.BuildInToolImageGeneration))
}

func TestApplyResponsesCompactBillingModelUsesUpstreamCompactModel(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		OriginModelName: "gpt-5.5-openai-compact",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.4",
		},
	}

	_, restore := applyResponsesCompactBillingModel(info)
	require.Equal(t, "gpt-5.4-openai-compact", info.OriginModelName)

	restore()
	require.Equal(t, "gpt-5.5-openai-compact", info.OriginModelName)
}
