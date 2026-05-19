package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestResponsesRequestHasImageGenerationToolDetectsTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "image_generation"}]
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {"type": "image_generation"}
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsStringToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": "image_generation"
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsNestedToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {
			"type": "allowed_tools",
			"tools": [{"type": "image_generation"}]
		}
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
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
