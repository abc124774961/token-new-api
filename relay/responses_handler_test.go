package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	modelgatewayproxy "github.com/QuantumNous/new-api/pkg/modelgateway/proxy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"

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

func TestExplicitImageGenerationToolKeepsRequestBillingSpec(t *testing.T) {
	t.Parallel()

	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Tools: []byte(`[{"type":"image_generation","quality":"low","size":"1024x1024"}]`),
	}

	usageInfo := relaycommon.BuildResponsesUsageInfo(req)
	imageTool := usageInfo.BuiltInTools[dto.BuildInToolImageGeneration]

	require.NotNil(t, imageTool)
	require.Equal(t, "low", imageTool.ImageGenerationQuality)
	require.Equal(t, "1024x1024", imageTool.ImageGenerationSize)
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

func TestApplyProxyBridgeRequestModeRestoresRelayInfo(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:               relayconstant.RelayModeResponses,
		RequestURLPath:          "/v1/responses",
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	restore := applyProxyBridgeRequestMode(info)

	require.Equal(t, relayconstant.RelayModeChatCompletions, info.RelayMode)
	require.Equal(t, "/v1/chat/completions", info.RequestURLPath)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), info.FinalRequestRelayFormat)

	restore()
	require.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	require.Equal(t, "/v1/responses", info.RequestURLPath)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.FinalRequestRelayFormat)
}

func TestHandleProxyBridgeResponseConvertsChatToResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	chatBody, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_1",
		"object":  "chat.completion",
		"created": 1710000200,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "bridge response",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     8,
			"completion_tokens": 4,
			"total_tokens":      12,
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(chatBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
	}

	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, integration.NewProxyBridge(nil), integration.ProxyBridgeDecision{
		Enabled:         true,
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 12, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), `"object":"response"`)
	require.Contains(t, recorder.Body.String(), "bridge response")
}

func TestHandleProxyBridgeStreamResponseConvertsChatSSEToResponsesSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disableStreamPingForResponsesTest(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	contentDelta, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_stream_1",
		"object":  "chat.completion.chunk",
		"created": 1710000300,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{"content": "hello"},
			},
		},
	})
	require.NoError(t, err)
	usageChunk, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_stream_1",
		"object":  "chat.completion.chunk",
		"created": 1710000300,
		"model":   "mimo-v1",
		"choices": []map[string]any{},
		"usage": map[string]any{
			"prompt_tokens":     5,
			"completion_tokens": 2,
			"total_tokens":      7,
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"data: " + string(contentDelta),
			"data: " + string(usageChunk),
			"data: [DONE]",
		}, "\n"))),
	}
	startTime := time.Now()
	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
		ChannelMeta:      &relaycommon.ChannelMeta{UpstreamModelName: "mimo-v1"},
		IsStream:         true,
		StartTime:        startTime,
	}
	info.SetEstimatePromptTokens(5)

	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, integration.NewProxyBridge(nil), integration.ProxyBridgeDecision{
		Enabled:         true,
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.TotalTokens)
	body := recorder.Body.String()
	require.Contains(t, body, "event: response.created")
	require.Contains(t, body, "event: response.output_text.delta")
	require.Contains(t, body, "event: response.completed")
	require.Contains(t, body, `"delta":"hello"`)
	require.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayResponseStarted))
}

func TestHandleProxyBridgeStreamResponseTreatsEOFAsInterrupted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disableStreamPingForResponsesTest(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	contentDelta, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_stream_cut",
		"object":  "chat.completion.chunk",
		"created": 1710000300,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{"content": "partial"},
			},
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: " + string(contentDelta) + "\n")),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
		ChannelMeta:      &relaycommon.ChannelMeta{UpstreamModelName: "mimo-v1"},
		IsStream:         true,
		StartTime:        time.Now(),
	}
	info.SetEstimatePromptTokens(9)

	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, integration.NewProxyBridge(nil), integration.ProxyBridgeDecision{
		Enabled:         true,
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, 9, usage.PromptTokens)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), "response.output_text.delta")
	require.NotContains(t, recorder.Body.String(), "response.completed")
	require.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayResponseStarted))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayStreamInterrupted))
}

func TestProxyBridgeStreamSenderUsageFallbackTextAggregatesDeliveredEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	sender := newProxyBridgeStreamSender()
	for _, event := range []dto.ResponsesStreamResponse{
		{Type: "response.created"},
		{Type: "response.output_text.delta", Delta: "partial "},
		{Type: "response.output_text.delta", Delta: "answer"},
		{Type: "response.reasoning_summary_text.delta", Delta: "brief reasoning"},
		{
			Type:   dto.ResponsesOutputTypeItemAdded,
			ItemID: "call_1",
			Item: &dto.ResponsesOutput{
				Type: "function_call",
				ID:   "call_1",
				Name: "lookup",
			},
		},
		{Type: "response.function_call_arguments.delta", ItemID: "call_1", Delta: `{"q":"tok`},
		{Type: "response.function_call_arguments.done", ItemID: "call_1", Delta: `{"q":"token"}`},
	} {
		data, err := common.Marshal(event)
		require.NoError(t, err)
		apiErr := sender.Send(ctx, &modelgatewayproxy.ConvertStreamResult{
			DownstreamEvents: []string{string(data)},
		})
		require.Nil(t, apiErr)
	}

	fallbackText := sender.UsageFallbackText()
	require.Equal(t, "partial answer\nbrief reasoning\nlookup\n{\"q\":\"token\"}", fallbackText)
	require.Equal(t, service.EstimateTokenByModel("mimo-v1", fallbackText), service.ResponseText2Usage(ctx, fallbackText, "mimo-v1", 3).CompletionTokens)
	require.Contains(t, recorder.Body.String(), "response.output_text.delta")
	require.NotContains(t, recorder.Body.String(), "response.completed")
}

func TestHandleProxyBridgeStreamResponseMarksUpstreamErrorEventFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disableStreamPingForResponsesTest(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	errorChunk, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_stream_error",
		"object":  "chat.completion.chunk",
		"created": 1710000300,
		"model":   "mimo-v1",
		"error": map[string]any{
			"message": "upstream overloaded",
			"type":    "server_error",
			"code":    "overloaded",
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"data: " + string(errorChunk),
			"data: [DONE]",
		}, "\n"))),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
		ChannelMeta:      &relaycommon.ChannelMeta{UpstreamModelName: "mimo-v1"},
		IsStream:         true,
		StartTime:        time.Now(),
	}

	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, integration.NewProxyBridge(nil), integration.ProxyBridgeDecision{
		Enabled:         true,
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	body := recorder.Body.String()
	require.Contains(t, body, "response.failed")
	require.Contains(t, body, "upstream overloaded")
	require.NotContains(t, body, "response.completed")
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayResponseStarted))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayStreamInterrupted))
	require.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestMergeResponsesStreamUsageNormalizesResponsesTokenFields(t *testing.T) {
	usage := &dto.Usage{}
	mergeResponsesStreamUsage(usage, &dto.Usage{
		InputTokens:  11,
		OutputTokens: 5,
		TotalTokens:  16,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 2,
			ImageTokens:  3,
		},
	})

	require.Equal(t, 11, usage.PromptTokens)
	require.Equal(t, 5, usage.CompletionTokens)
	require.Equal(t, 16, usage.TotalTokens)
	require.Equal(t, 11, usage.InputTokens)
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 3, usage.PromptTokensDetails.ImageTokens)
	require.NotNil(t, usage.InputTokensDetails)
	require.Equal(t, 2, usage.InputTokensDetails.CachedTokens)
}

func TestHandleProxyBridgeStreamResponseBuffersCreatedThenEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disableStreamPingForResponsesTest(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	roleChunk, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_stream_created_only",
		"object":  "chat.completion.chunk",
		"created": 1710000300,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{"role": "assistant"},
			},
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: " + string(roleChunk) + "\n")),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
		ChannelMeta:      &relaycommon.ChannelMeta{UpstreamModelName: "mimo-v1"},
		IsStream:         true,
		StartTime:        time.Now(),
	}

	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, integration.NewProxyBridge(nil), integration.ProxyBridgeDecision{
		Enabled:         true,
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	require.Empty(t, recorder.Body.String())
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyRelayResponseStarted))
	require.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
}

func disableStreamPingForResponsesTest(t *testing.T) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	settings := operation_setting.GetGeneralSetting()
	oldEnabled := settings.PingIntervalEnabled
	oldSeconds := settings.PingIntervalSeconds
	settings.PingIntervalEnabled = false
	settings.PingIntervalSeconds = 10
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
		settings.PingIntervalEnabled = oldEnabled
		settings.PingIntervalSeconds = oldSeconds
	})
}
