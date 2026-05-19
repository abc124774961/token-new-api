package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"

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
}

func TestResponsesRequestRequiresCodexImageGenerationToolDetectsCodexSkillIntent(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []byte(`[
			{"role":"user","content":[
				{"type":"input_text","text":"[$imagegen](/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md) 风景"}
			]},
			{"role":"user","content":[
				{"type":"input_text","text":"<skill><name>imagegen</name><path>/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md</path></skill>"}
			]}
		]`),
		ToolChoice: []byte(`"auto"`),
	}

	require.True(t, ResponsesRequestRequiresCodexImageGenerationTool(req))
	require.True(t, ResponsesRequestHasImageGenerationKeywordHits(req))
	require.Contains(t, ResponsesRequestImageGenerationKeywordHits(req), "$imagegen")
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
