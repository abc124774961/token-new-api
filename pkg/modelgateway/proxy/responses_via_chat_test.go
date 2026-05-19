package proxy_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	modelgatewayproxy "github.com/QuantumNous/new-api/pkg/modelgateway/proxy"
	"github.com/stretchr/testify/require"
)

func TestResponsesViaChatConverterMapsRequestCoreFields(t *testing.T) {
	input, err := common.Marshal([]map[string]any{
		{
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": "hello"},
			},
		},
	})
	require.NoError(t, err)
	tools, err := common.Marshal([]map[string]any{
		{
			"type":        "function",
			"name":        "search",
			"description": "Search",
			"parameters":  map[string]any{"type": "object"},
		},
	})
	require.NoError(t, err)
	promptCacheKey, err := common.Marshal("cache-1")
	require.NoError(t, err)
	maxOutputTokens := uint(128)
	stream := true

	req := &dto.OpenAIResponsesRequest{
		Model:           "mimo-v1",
		Input:           input,
		MaxOutputTokens: &maxOutputTokens,
		Reasoning:       &dto.Reasoning{Effort: "high"},
		Stream:          &stream,
		Tools:           tools,
		PromptCacheKey:  promptCacheKey,
	}
	got, err := modelgatewayproxy.NewResponsesViaChatConverter().ConvertRequest(req, "mimo-upstream")

	require.NoError(t, err)
	require.Equal(t, "mimo-upstream", got.Model)
	require.Len(t, got.Messages, 1)
	require.Equal(t, "user", got.Messages[0].Role)
	require.Equal(t, "high", got.ReasoningEffort)
	require.Equal(t, "cache-1", got.PromptCacheKey)
	require.NotNil(t, got.MaxCompletionTokens)
	require.EqualValues(t, 128, *got.MaxCompletionTokens)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "search", got.Tools[0].Function.Name)
}

func TestResponsesViaChatConverterMapsToolCallResponse(t *testing.T) {
	toolCalls, err := common.Marshal([]dto.ToolCallRequest{
		{
			ID:   "call_1",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "run_shell",
				Arguments: `{"cmd":"pwd"}`,
			},
		},
	})
	require.NoError(t, err)
	resp := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Model:   "deepseek-v4-pro-max",
		Object:  "chat.completion",
		Created: float64(1710000002),
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:      "assistant",
					ToolCalls: toolCalls,
				},
				FinishReason: "tool_calls",
			},
		},
	}

	got, err := modelgatewayproxy.NewResponsesViaChatConverter().ConvertResponse(resp, "deepseek-v4-pro-max")

	require.NoError(t, err)
	require.Equal(t, "response", got.Object)
	require.Equal(t, "deepseek-v4-pro-max", got.Model)
	require.Len(t, got.Output, 1)
	require.Equal(t, "function_call", got.Output[0].Type)
	require.Equal(t, "run_shell", got.Output[0].Name)
	require.JSONEq(t, `{"cmd":"pwd"}`, string(got.Output[0].Arguments))
}

func TestResponsesViaChatConverterMapsStreamDataErrorToFailed(t *testing.T) {
	result, err := modelgatewayproxy.NewResponsesViaChatConverter().ConvertStream([]string{
		`{"id":"chatcmpl_error_1","object":"chat.completion.chunk","created":1710000003,"model":"mimo-v1","error":{"message":"upstream overloaded","type":"server_error","code":"overloaded"}}`,
	}, "mimo-v1")

	require.NoError(t, err)
	require.Len(t, result.DownstreamEvents, 1)

	var event dto.ResponsesStreamResponse
	require.NoError(t, common.UnmarshalJsonStr(result.DownstreamEvents[0], &event))
	require.Equal(t, "response.failed", event.Type)
	require.NotNil(t, event.Response)
	require.Equal(t, "chatcmpl_error_1", event.Response.ID)
	require.JSONEq(t, `"failed"`, string(event.Response.Status))
	openaiErr := event.Response.GetOpenAIError()
	require.NotNil(t, openaiErr)
	require.Equal(t, "server_error", openaiErr.Type)
	require.Equal(t, "upstream overloaded", openaiErr.Message)
	require.Equal(t, "overloaded", openaiErr.Code)
}

func TestResponsesViaChatStreamConverterMapsSSEErrorEventAndSuppressesCompleted(t *testing.T) {
	stream := modelgatewayproxy.NewResponsesViaChatConverter().NewStreamConverter("deepseek-v4-pro-max")
	first, err := stream.Accept(`data: {"id":"chatcmpl_error_2","object":"chat.completion.chunk","created":1710000004,"model":"deepseek-v4-pro-max","choices":[{"index":0,"delta":{"role":"assistant"}}]}`)
	require.NoError(t, err)
	require.Len(t, first.DownstreamEvents, 1)

	failed, err := stream.Accept("event: error\ndata: {\"message\":\"bad upstream stream\",\"type\":\"api_error\",\"code\":\"bad_stream\"}")
	require.NoError(t, err)
	require.Len(t, failed.DownstreamEvents, 1)

	var event dto.ResponsesStreamResponse
	require.NoError(t, common.UnmarshalJsonStr(failed.DownstreamEvents[0], &event))
	require.Equal(t, "response.failed", event.Type)
	require.NotNil(t, event.Response)
	require.Equal(t, "chatcmpl_error_2", event.Response.ID)
	openaiErr := event.Response.GetOpenAIError()
	require.NotNil(t, openaiErr)
	require.Equal(t, "api_error", openaiErr.Type)
	require.Equal(t, "bad upstream stream", openaiErr.Message)
	require.Equal(t, "bad_stream", openaiErr.Code)

	final, err := stream.Finish()
	require.NoError(t, err)
	require.Empty(t, final.DownstreamEvents)
}

func TestResponsesViaChatStreamConverterMapsProviderErrorEnvelope(t *testing.T) {
	result, err := modelgatewayproxy.NewResponsesViaChatConverter().ConvertStream([]string{
		`{"id":"chatcmpl_error_3","object":"chat.completion.chunk","created":1710000005,"model":"deepseek-v4-pro-max","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"error_msg":"deepseek upstream rate limited","error_type":"rate_limit","code":"rate_limit_exceeded"}`,
	}, "deepseek-v4-pro-max")

	require.NoError(t, err)
	require.Len(t, result.DownstreamEvents, 2)

	var event dto.ResponsesStreamResponse
	require.NoError(t, common.UnmarshalJsonStr(result.DownstreamEvents[1], &event))
	require.Equal(t, "response.failed", event.Type)
	require.NotNil(t, event.Response)
	require.Equal(t, "chatcmpl_error_3", event.Response.ID)
	openaiErr := event.Response.GetOpenAIError()
	require.NotNil(t, openaiErr)
	require.Equal(t, "rate_limit", openaiErr.Type)
	require.Equal(t, "deepseek upstream rate limited", openaiErr.Message)
	require.Equal(t, "rate_limit_exceeded", openaiErr.Code)
}
