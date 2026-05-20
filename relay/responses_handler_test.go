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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	modelgatewayproxy "github.com/QuantumNous/new-api/pkg/modelgateway/proxy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestResponsesHelperNativeNonStreamSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupResponsesHelperSmokeDB(t)

	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer sk-responses-smoke", r.Header.Get("Authorization"))
		require.Contains(t, r.Header.Get("Content-Type"), "application/json")

		require.NoError(t, common.DecodeJson(r.Body, &upstreamBody))
		require.Equal(t, "gpt-5.5", upstreamBody["model"])
		require.NotNil(t, upstreamBody["input"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "resp_smoke",
			"object": "response",
			"created_at": 1710000000,
			"model": "gpt-5.5",
			"output": [
				{
					"id": "msg_1",
					"type": "message",
					"role": "assistant",
					"content": [
						{
							"type": "output_text",
							"text": "native responses ok"
						}
					]
				}
			],
			"usage": {
				"input_tokens": 7,
				"output_tokens": 3,
				"total_tokens": 10,
				"input_tokens_details": {
					"cached_tokens": 2
				}
			}
		}`))
	}))
	defer upstream.Close()

	ctx, recorder, info := prepareResponsesHelperSmoke(t, db, upstream.URL, "req-responses-helper-smoke", "gpt-5.5")

	apiErr := ResponsesHelper(ctx, info)

	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, recorder.Code)
	var downstream dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &downstream))
	require.Equal(t, "response", downstream.Object)
	require.Equal(t, "gpt-5.5", downstream.Model)
	require.Equal(t, "native responses ok", downstream.Output[0].Content[0].Text)
	require.NotNil(t, downstream.Usage)
	require.Equal(t, 7, downstream.Usage.InputTokens)
	require.Equal(t, 3, downstream.Usage.OutputTokens)
	require.Equal(t, 10, downstream.Usage.TotalTokens)
	require.Equal(t, "gpt-5.5", info.ResponseModelName)
	require.Equal(t, "gpt-5.5", info.DownstreamModelName)

	var user model.User
	require.NoError(t, db.Select("used_quota", "request_count", "quota").Where("id = ?", 1).First(&user).Error)
	require.Equal(t, 10, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, 1_000_000-10, user.Quota)

	var token model.Token
	require.NoError(t, db.Select("remain_quota", "used_quota").Where("id = ?", 9).First(&token).Error)
	require.Equal(t, 1_000_000, token.RemainQuota)
	require.Equal(t, 0, token.UsedQuota)

	var channel model.Channel
	require.NoError(t, db.Select("used_quota").Where("id = ?", 11).First(&channel).Error)
	require.Equal(t, int64(10), channel.UsedQuota)

	var consumeLog model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", 1, model.LogTypeConsume).First(&consumeLog).Error)
	require.Equal(t, 11, consumeLog.ChannelId)
	require.Equal(t, "gpt-5.5", consumeLog.ModelName)
	require.Equal(t, "responses-token", consumeLog.TokenName)
	require.Equal(t, 7, consumeLog.PromptTokens)
	require.Equal(t, 3, consumeLog.CompletionTokens)
	require.Equal(t, 10, consumeLog.Quota)
	require.Equal(t, "req-responses-helper-smoke", consumeLog.RequestId)
	require.Contains(t, consumeLog.Other, `"request_path":"/v1/responses"`)
	require.Contains(t, consumeLog.Other, `"request_conversion":["OpenAI Responses"]`)
}

func TestResponsesHelperNativeNonStreamErrorDoesNotConsume(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           string
		wantStatusCode int
	}{
		{
			name:           "upstream non-200",
			statusCode:     http.StatusTooManyRequests,
			wantStatusCode: http.StatusTooManyRequests,
			body: `{
				"error": {
					"message": "rate limited",
					"type": "rate_limit_error",
					"code": "rate_limited"
				}
			}`,
		},
		{
			name:           "http 200 error body",
			statusCode:     http.StatusOK,
			wantStatusCode: http.StatusOK,
			body: `{
				"error": {
					"message": "model unavailable",
					"type": "server_error",
					"code": "model_unavailable"
				}
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db := setupResponsesHelperSmokeDB(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/v1/responses", r.URL.Path)
				require.Equal(t, "Bearer sk-responses-smoke", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			ctx, _, info := prepareResponsesHelperSmoke(t, db, upstream.URL, "req-responses-helper-error", "gpt-5.5")

			apiErr := ResponsesHelper(ctx, info)

			require.NotNil(t, apiErr)
			require.Equal(t, tc.wantStatusCode, apiErr.StatusCode)
			requireResponsesHelperSmokeNoConsume(t, db)
		})
	}
}

func TestResponsesHelperResponsesViaChatSelectedPlanHTTPRelaySmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupResponsesHelperSmokeDB(t)

	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-responses-smoke", r.Header.Get("Authorization"))
		require.Contains(t, r.Header.Get("Content-Type"), "application/json")

		require.NoError(t, common.DecodeJson(r.Body, &upstreamBody))
		require.Equal(t, "mimo-v1", upstreamBody["model"])
		require.NotNil(t, upstreamBody["messages"])
		require.Nil(t, upstreamBody["input"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl_smoke",
			"object": "chat.completion",
			"created": 1710000000,
			"model": "mimo-v1",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "responses via chat ok"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 7,
				"completion_tokens": 3,
				"total_tokens": 10
			}
		}`))
	}))
	defer upstream.Close()

	ctx, recorder, info := prepareResponsesHelperSmoke(t, db, upstream.URL, "req-responses-via-chat-smoke", "mimo-v1")
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
		SelectedGroup:   "default",
	})

	apiErr := ResponsesHelper(ctx, info)

	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, recorder.Code)
	var downstream dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &downstream))
	require.Equal(t, "response", downstream.Object)
	require.Equal(t, "mimo-v1", downstream.Model)
	require.Equal(t, "responses via chat ok", downstream.Output[0].Content[0].Text)
	require.NotNil(t, downstream.Usage)
	require.Equal(t, 7, downstream.Usage.PromptTokens)
	require.Equal(t, 3, downstream.Usage.CompletionTokens)
	require.Equal(t, 10, downstream.Usage.TotalTokens)
	require.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	require.Equal(t, "/v1/responses", info.RequestURLPath)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), info.GetFinalRequestRelayFormat())
	require.Contains(t, info.RequestConversionChain, types.RelayFormat(types.RelayFormatOpenAIResponses))
	require.Contains(t, info.RequestConversionChain, types.RelayFormat(types.RelayFormatOpenAI))

	var user model.User
	require.NoError(t, db.Select("used_quota", "request_count", "quota").Where("id = ?", 1).First(&user).Error)
	require.Equal(t, 10, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, 1_000_000-10, user.Quota)

	var channel model.Channel
	require.NoError(t, db.Select("used_quota").Where("id = ?", 11).First(&channel).Error)
	require.Equal(t, int64(10), channel.UsedQuota)

	var consumeLog model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", 1, model.LogTypeConsume).First(&consumeLog).Error)
	require.Equal(t, 11, consumeLog.ChannelId)
	require.Equal(t, "mimo-v1", consumeLog.ModelName)
	require.Equal(t, "responses-token", consumeLog.TokenName)
	require.Equal(t, 7, consumeLog.PromptTokens)
	require.Equal(t, 3, consumeLog.CompletionTokens)
	require.Equal(t, 10, consumeLog.Quota)
	require.Equal(t, "req-responses-via-chat-smoke", consumeLog.RequestId)
	require.Contains(t, consumeLog.Other, `"request_path":"/v1/responses"`)
	require.Contains(t, consumeLog.Other, `"request_conversion":["OpenAI Responses","OpenAI Compatible"]`)
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
	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "response", response.Object)
	require.Equal(t, "mimo-v1", response.Model)
	require.NotNil(t, response.Usage)
	require.Equal(t, 8, response.Usage.PromptTokens)
	require.Equal(t, 4, response.Usage.CompletionTokens)
	require.Equal(t, 12, response.Usage.TotalTokens)
}

func TestRelayProxyBridgeRequestAndResponseNarrowSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})
	input, err := common.Marshal([]map[string]any{
		{
			"role":    "user",
			"content": "bridge this responses request",
		},
	})
	require.NoError(t, err)
	req := &dto.OpenAIResponsesRequest{
		Model: "mimo-v1",
		Input: input,
	}
	info := &relaycommon.RelayInfo{
		RelayMode:               relayconstant.RelayModeResponses,
		RequestURLPath:          "/v1/responses",
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
		RequestModelName:        "mimo-v1",
		OriginModelName:         "mimo-v1",
		ChannelMeta:             &relaycommon.ChannelMeta{UpstreamModelName: "mimo-v1"},
	}
	bridge := integration.NewProxyBridge(nil)
	decision := bridge.Resolve(ctx, info)
	require.True(t, decision.Enabled)
	require.Equal(t, "enabled", decision.Reason)

	bridgeResult, handled, err := bridge.ConvertResponsesRequest(ctx, info, req)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", bridgeResult.UpstreamPath)
	require.Contains(t, string(bridgeResult.UpstreamBody), `"messages"`)
	info.AppendRequestConversion(types.RelayFormatOpenAI)
	restoreProxyMode := applyProxyBridgeRequestMode(info)
	require.Equal(t, relayconstant.RelayModeChatCompletions, info.RelayMode)
	require.Equal(t, "/v1/chat/completions", info.RequestURLPath)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), info.FinalRequestRelayFormat)
	restoreProxyMode()
	require.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	require.Equal(t, "/v1/responses", info.RequestURLPath)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.FinalRequestRelayFormat)

	chatBody, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_narrow_smoke",
		"object":  "chat.completion",
		"created": 1710001500,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "restored responses output",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     9,
			"completion_tokens": 3,
			"total_tokens":      12,
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(chatBody)),
	}
	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, bridge, decision)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 12, usage.TotalTokens)
	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "response", response.Object)
	require.Equal(t, "mimo-v1", response.Model)
	require.Equal(t, "restored responses output", response.Output[0].Content[0].Text)
	require.NotNil(t, response.Usage)
	require.Equal(t, 9, response.Usage.PromptTokens)
	require.Equal(t, 3, response.Usage.CompletionTokens)
	require.Equal(t, 12, response.Usage.TotalTokens)
}

func TestRelayProxyBridgeWithoutSelectedPlanDoesNotHandleResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
	}
	bridge := integration.NewProxyBridge(nil)
	decision := bridge.Resolve(ctx, info)
	require.False(t, decision.Enabled)
	require.Equal(t, "no_smart_dispatch_plan", decision.Reason)

	chatBody, err := common.Marshal(map[string]any{
		"id":      "chatcmpl_no_plan",
		"object":  "chat.completion",
		"created": 1710001501,
		"model":   "mimo-v1",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "should not be bridged",
				},
				"finish_reason": "stop",
			},
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(chatBody)),
	}
	usage, apiErr := handleProxyBridgeResponse(ctx, info, resp, bridge, integration.ProxyBridgeDecision{
		Enabled:         true,
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	})

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	require.Empty(t, recorder.Body.String())
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
	completed := requireResponsesSSEEvent(t, body, "response.completed")
	require.NotNil(t, completed.Response)
	require.Equal(t, "response", completed.Response.Object)
	require.Equal(t, "mimo-v1", completed.Response.Model)
	require.NotNil(t, completed.Response.Usage)
	require.Equal(t, 5, completed.Response.Usage.PromptTokens)
	require.Equal(t, 2, completed.Response.Usage.CompletionTokens)
	require.Equal(t, 7, completed.Response.Usage.TotalTokens)
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

func requireResponsesSSEEvent(t *testing.T, body string, eventType string) dto.ResponsesStreamResponse {
	t.Helper()
	for _, block := range strings.Split(body, "\n\n") {
		var eventName string
		var data string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			}
		}
		if eventName != eventType {
			continue
		}
		require.NotEmpty(t, data)
		var event dto.ResponsesStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(data, &event))
		require.Equal(t, eventType, event.Type)
		return event
	}
	require.Failf(t, "missing responses SSE event", "event %q not found in body:\n%s", eventType, body)
	return dto.ResponsesStreamResponse{}
}

func setupResponsesHelperSmokeDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldDataExportEnabled := common.DataExportEnabled
	globalSettings := model_setting.GetGlobalSettings()
	oldPassThroughRequestEnabled := globalSettings.PassThroughRequestEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	globalSettings.PassThroughRequestEnabled = false
	service.InitHttpClient()

	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Channel{},
		&model.Log{},
	))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.DataExportEnabled = oldDataExportEnabled
		globalSettings.PassThroughRequestEnabled = oldPassThroughRequestEnabled
		_ = sqlDB.Close()
	})

	return db
}

func prepareResponsesHelperSmoke(t *testing.T, db *gorm.DB, baseURL string, requestID string, modelName string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "responses_user",
		Quota:    1_000_000,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:          9,
		UserId:      1,
		Key:         "token-responses-smoke",
		Name:        "responses-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 1_000_000,
		Group:       "default",
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:      11,
		Type:    constant.ChannelTypeOpenAI,
		Name:    "responses-openai",
		Key:     "sk-responses-smoke",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
		Models:  modelName,
		Group:   "default",
	}).Error)

	requestBody := []byte(`{"model":"` + modelName + `","input":[{"role":"user","content":"ping"}]}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, requestID)
	ctx.Set("username", "responses_user")
	ctx.Set("token_name", "responses-token")
	ctx.Set("token_quota", 1_000_000)
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(ctx, constant.ContextKeyUserId, 1)
	common.SetContextKey(ctx, constant.ContextKeyUserName, "responses_user")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserQuota, 1_000_000)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 9)
	common.SetContextKey(ctx, constant.ContextKeyTokenKey, "token-responses-smoke")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenUnlimited, false)
	common.SetContextKey(ctx, constant.ContextKeyEstimatedTokens, 7)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 11)
	common.SetContextKey(ctx, constant.ContextKeyChannelName, "responses-openai")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "sk-responses-smoke")
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, baseURL)
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{})
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, modelName)

	var responsesReq dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal(requestBody, &responsesReq))
	info := relaycommon.GenRelayInfoResponses(ctx, &responsesReq)
	info.InitRequestConversionChain()
	info.IsPlayground = true
	info.UserSetting = dto.UserSetting{BillingPreference: "wallet_only"}
	info.PriceData = types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 1,
		CacheRatio:      1,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 1,
		},
		QuotaToPreConsume: 8,
	}
	require.Nil(t, service.PreConsumeBilling(ctx, info.PriceData.QuotaToPreConsume, info))
	return ctx, recorder, info
}

func requireResponsesHelperSmokeNoConsume(t *testing.T, db *gorm.DB) {
	t.Helper()
	var user model.User
	require.NoError(t, db.Select("used_quota", "request_count").Where("id = ?", 1).First(&user).Error)
	require.Equal(t, 0, user.UsedQuota)
	require.Equal(t, 0, user.RequestCount)

	var channel model.Channel
	require.NoError(t, db.Select("used_quota").Where("id = ?", 11).First(&channel).Error)
	require.Equal(t, int64(0), channel.UsedQuota)

	var token model.Token
	require.NoError(t, db.Select("remain_quota", "used_quota").Where("id = ?", 9).First(&token).Error)
	require.Equal(t, 1_000_000, token.RemainQuota)
	require.Equal(t, 0, token.UsedQuota)

	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 1, model.LogTypeConsume).Count(&logCount).Error)
	require.Equal(t, int64(0), logCount)
}
