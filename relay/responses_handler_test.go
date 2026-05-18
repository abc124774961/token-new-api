package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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

func TestMaybeInjectImageGenerationToolForImageIntent(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: mustRawMessage(t, "生成一张红色方块图片"),
	}

	injected, err := maybeInjectImageGenerationTool(req)
	require.NoError(t, err)

	require.True(t, injected)
	require.True(t, req.HasTool(dto.BuildInToolImageGeneration))
	require.JSONEq(t, `[{"output_format":"png","type":"image_generation"}]`, string(req.Tools))
	require.JSONEq(t, `{"type":"image_generation"}`, string(req.ToolChoice))
}

func TestMaybeInjectImageGenerationToolSkipsNormalText(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: mustRawMessage(t, "解释一下 Go 的 interface"),
	}

	injected, err := maybeInjectImageGenerationTool(req)
	require.NoError(t, err)

	require.False(t, injected)
	require.Empty(t, req.Tools)
	require.Empty(t, req.ToolChoice)
}

func TestMaybeInjectImageGenerationToolSkipsExistingTools(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: mustRawMessage(t, "生成一张红色方块图片"),
		Tools: mustRawMessage(t, []map[string]any{{"type": dto.BuildInToolImageGeneration}}),
	}

	injected, err := maybeInjectImageGenerationTool(req)
	require.NoError(t, err)

	require.False(t, injected)
	require.True(t, req.HasTool(dto.BuildInToolImageGeneration))
	require.JSONEq(t, `[{"type":"image_generation"}]`, string(req.Tools))
	require.Empty(t, req.ToolChoice)
}

func TestMaybeInjectImageGenerationToolAppendsWhenOtherToolsExist(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: mustRawMessage(t, "Please generate a tiny image of a red square."),
		Tools: mustRawMessage(t, []map[string]any{{"type": dto.BuildInToolWebSearchPreview}}),
	}

	injected, err := maybeInjectImageGenerationTool(req)
	require.NoError(t, err)

	require.True(t, injected)
	require.True(t, req.HasTool(dto.BuildInToolWebSearchPreview))
	require.True(t, req.HasTool(dto.BuildInToolImageGeneration))
	require.JSONEq(t, `[{"type":"web_search_preview"},{"output_format":"png","type":"image_generation"}]`, string(req.Tools))
	require.JSONEq(t, `{"type":"image_generation"}`, string(req.ToolChoice))
}

func TestMaybeInjectImageGenerationToolDetectsNestedInputText(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "请创建一张极简风格登录页面插画"},
				},
			},
		}),
	}

	injected, err := maybeInjectImageGenerationTool(req)
	require.NoError(t, err)

	require.True(t, injected)
	require.True(t, req.HasTool(dto.BuildInToolImageGeneration))
}

func TestInjectedImageGenerationToolUpdatesResponsesUsageInfo(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: mustRawMessage(t, "Create a polished product image"),
		Tools: mustRawMessage(t, []map[string]any{{"type": dto.BuildInToolWebSearchPreview}}),
	}

	injected, err := maybeInjectImageGenerationTool(req)
	require.NoError(t, err)
	require.True(t, injected)

	usageInfo := relaycommon.BuildResponsesUsageInfo(req)
	require.Contains(t, usageInfo.BuiltInTools, dto.BuildInToolWebSearchPreview)
	require.Contains(t, usageInfo.BuiltInTools, dto.BuildInToolImageGeneration)
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

func mustRawMessage(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
