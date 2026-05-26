package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newResponsesCapabilityTestContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func TestResponsesRequestHasImageGenerationToolIgnoresDeclaredTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "image_generation"}]
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {"type": "image_generation"}
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresStringToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": "image_generation"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresNestedToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {
			"type": "allowed_tools",
			"tools": [{"type": "image_generation"}]
		}
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsCodexSkillIntent(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "[$imagegen](/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md) 风景"}
				]
			}
		],
		"tool_choice": "auto"
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresOtherTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "web_search_preview"}],
		"tool_choice": "auto"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresPlainImagePromptWithoutKeywordHit(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "生成一张风景图"}
				]
			}
		],
		"tool_choice": "auto"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestApplySelectedGroupContextUsesSelectedGroupForAutoRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")

	applySelectedGroupContext(ctx, "auto", "codex-plus")

	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}
