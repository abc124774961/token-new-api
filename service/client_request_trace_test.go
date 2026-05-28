package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildSanitizedRequestHeadersForLogMasksSensitiveHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer secret")
	headers.Set("X-Api-Key", "secret-key")
	headers.Set("X-Codex-Turn-Metadata", `{"thread_id":"abc"}`)
	headers.Set("User-Agent", "Codex Desktop/0.131.0")

	trace := BuildSanitizedRequestHeadersForLog(headers, 4096)

	require.Equal(t, "***masked***", trace["authorization"])
	require.Equal(t, "***masked***", trace["x-api-key"])
	require.Equal(t, `{"thread_id":"abc"}`, trace["x-codex-turn-metadata"])
	require.Equal(t, "Codex Desktop/0.131.0", trace["user-agent"])
}

func TestBuildResponsesRequestToolTraceForLogDetectsImageGeneration(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:      "gpt-5.5",
		Tools:      []byte(`[{"type":"image_generation","quality":"low","size":"1024x1024"}]`),
		ToolChoice: []byte(`{"type":"image_generation"}`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.Equal(t, true, trace["has_image_generation_tool"])
	require.Equal(t, []string{"image_generation"}, trace["tool_types"])
	require.Contains(t, trace["tools_raw"], "image_generation")
	require.Empty(t, trace["imagegen_keyword_hits"])
	require.Equal(t, map[string][]string{
		"tools":       {"image_generation"},
		"tool_choice": {"image_generation"},
	}, trace["imagegen_keyword_sources"])
}

func TestResponsesRequestRequiresCodexImageGenerationToolIgnoresDeclaredImageGenerationTool(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:      "gpt-5.5",
		Tools:      []byte(`[{"type":"image_generation"}]`),
		ToolChoice: []byte(`{"type":"image_generation"}`),
		Input: []byte(`[
			{"role":"user","content":[
				{"type":"input_text","text":"你的模型版本"}
			]}
		]`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.True(t, trace["has_image_generation_tool"].(bool))
	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, ResponsesRequestImageGenerationKeywordHits(req))
	require.Empty(t, trace["imagegen_keyword_hits"])
	require.Contains(t, trace["imagegen_keyword_sources"], "tools")
	require.Contains(t, trace["imagegen_keyword_sources"], "tool_choice")
}

func TestResponsesRequestRequiresCodexImageGenerationToolDetectsCodexSkillIntent(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"user","content":[
				{"type":"input_text","text":"[$imagegen](/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md) 风景"}
			]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	require.True(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.True(t, ResponsesRequestHasImageGenerationKeywordHits(req))
	require.Contains(t, ResponsesRequestImageGenerationKeywordHits(req), "$imagegen")
	require.Equal(t, map[string][]string{
		"input_text": {"$imagegen"},
	}, ResponsesRequestRecentInputImageGenerationKeywordSources(req))
}

func TestResponsesRequestRequiresCodexImageGenerationToolIgnoresSystemImagegenKeyword(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"system","content":[
				{"type":"input_text","text":"Available skills: $imagegen can generate raster images."}
			]},
			{"role":"user","content":[
				{"type":"input_text","text":"这段调度详情哪里有问题？"}
			]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, ResponsesRequestImageGenerationKeywordHits(req))
	require.False(t, trace["requires_codex_image_tool"].(bool))
	require.Empty(t, trace["imagegen_keyword_hits"])
}

func TestResponsesRequestRequiresCodexImageGenerationToolIgnoresPlainImagegenWord(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"user","content":[
				{"type":"input_text","text":"为什么日志里出现 imagegen 或 image_generation 字样？"}
			]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, ResponsesRequestImageGenerationKeywordHits(req))
	require.False(t, trace["requires_codex_image_tool"].(bool))
}

func TestResponsesRequestRequiresCodexImageGenerationToolIgnoresInlineSkillDefinitionBlock(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"user","content":[
				{"type":"input_text","text":"PLEASE IMPLEMENT THIS PLAN"}
			]},
			{"role":"user","content":[
				{"type":"input_text","text":"<skill><name>imagegen</name><path>/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md</path></skill>"}
			]}
		]`),
		Tools: []byte(`[{"type":"image_generation"}]`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, trace["imagegen_keyword_hits"])
	require.Contains(t, trace["imagegen_keyword_sources"], "tools")
}

func TestResponsesRequestRequiresCodexImageGenerationToolDetectsSkillCatalogKeywordHit(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"system","content":[
				{"type":"input_text","text":"<skills_instructions>- imagegen: Generate or edit raster images when the task benefits from AI-created bitmap visuals.</skills_instructions>"}
			]},
			{"role":"user","content":[
				{"type":"input_text","text":"分析一下客户端有几个情况？"}
			]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, trace["imagegen_keyword_hits"])
}

func TestResponsesRequestRequiresCodexImageGenerationToolIgnoresPlainChineseImagePromptWithoutKeywordHit(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"user","content":[
				{"type":"input_text","text":"生成一张风景图"}
			]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, trace["imagegen_keyword_hits"])
}

func TestResponsesRequestRequiresCodexImageGenerationToolIgnoresOldSessionImagegenKeyword(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"user","content":[{"type":"input_text","text":"[$imagegen](/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md) 风景"}]},
			{"role":"assistant","content":[{"type":"input_text","text":"Generated image saved."}]},
			{"role":"user","content":[{"type":"input_text","text":"你的模型版本"}]},
			{"role":"assistant","content":[{"type":"input_text","text":"我是 GPT。"}]},
			{"role":"user","content":[{"type":"input_text","text":"你的模型版本"}]},
			{"role":"assistant","content":[{"type":"input_text","text":"我是 GPT。"}]},
			{"role":"user","content":[{"type":"input_text","text":"你的模型版本"}]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	trace := BuildResponsesRequestToolTraceForLog(req)

	require.False(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.Empty(t, trace["imagegen_keyword_hits"])
}

func TestShouldLogClientRequestTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origDebug := common.DebugEnabled
	common.DebugEnabled = false
	t.Cleanup(func() {
		common.DebugEnabled = origDebug
	})

	t.Setenv("CLIENT_REQUEST_TRACE_ENABLED", "false")

	disabledCtx := newTraceTestContext(http.MethodPost, "/v1/responses", map[string]string{
		"User-Agent": "Codex",
	})
	require.False(t, ShouldLogClientRequestTrace(disabledCtx))

	t.Setenv("CLIENT_REQUEST_TRACE_ENABLED", "true")

	responsesCtx := newTraceTestContext(http.MethodPost, "/v1/responses", nil)
	require.True(t, ShouldLogClientRequestTrace(responsesCtx))

	codexCtx := newTraceTestContext(http.MethodPost, "/v1/chat/completions", map[string]string{
		"User-Agent": "Codex",
	})
	require.True(t, ShouldLogClientRequestTrace(codexCtx))
}

func newTraceTestContext(method, target string, headers map[string]string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	ctx.Request = req
	return ctx
}
