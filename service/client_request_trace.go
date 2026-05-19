package service

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
)

const clientRequestTraceMaxValueLen = 4096
const responsesInputTextProbeWindow = 5

var codexImageGenerationFeatureKeywords = []string{
	"$imagegen",
	"imagegen",
	"image generation",
	dto.BuildInToolImageGeneration,
}

var codexImageGenerationKeywordIgnoredBlocks = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<skills_instructions\b[^>]*>.*?</skills_instructions>`),
	regexp.MustCompile(`(?is)<plugins_instructions\b[^>]*>.*?</plugins_instructions>`),
}

// ShouldLogClientRequestTrace returns true for Codex-like requests and Responses
// requests. These traces are intentionally INFO-level so they are visible in
// production console logs while debugging channel selection.
func ShouldLogClientRequestTrace(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/responses") {
		return true
	}
	return isCodexLikeHeader(c.Request.Header)
}

func isCodexLikeHeader(header http.Header) bool {
	for key, values := range header {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lowerKey, "codex") {
			return true
		}
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), "codex") {
				return true
			}
		}
	}
	return false
}

// BuildClientRequestTraceForLog builds a sanitized request trace. Sensitive
// headers are kept as keys but masked, so presence/order of client metadata is
// still visible without leaking credentials.
func BuildClientRequestTraceForLog(c *gin.Context) map[string]any {
	trace := map[string]any{}
	if c == nil || c.Request == nil {
		return trace
	}
	req := c.Request
	trace["method"] = req.Method
	if req.URL != nil {
		trace["path"] = req.URL.Path
		if req.URL.RawQuery != "" {
			trace["query"] = truncateTraceValue(req.URL.RawQuery, clientRequestTraceMaxValueLen)
		}
	}
	if req.Host != "" {
		trace["host"] = req.Host
	}
	if req.RemoteAddr != "" {
		trace["remote_addr"] = req.RemoteAddr
	}
	if req.ContentLength >= 0 {
		trace["content_length"] = req.ContentLength
	}
	trace["codex_like_client"] = isCodexLikeHeader(req.Header)
	trace["headers"] = BuildSanitizedRequestHeadersForLog(req.Header, clientRequestTraceMaxValueLen)
	return trace
}

func BuildSanitizedRequestHeadersForLog(header http.Header, maxValueLen int) map[string]string {
	if maxValueLen <= 0 {
		maxValueLen = clientRequestTraceMaxValueLen
	}
	traceHeaders := make(map[string]string, len(header))
	for key, values := range header {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "" {
			continue
		}
		if isSensitiveHeaderName(normalizedKey) {
			traceHeaders[normalizedKey] = "***masked***"
			continue
		}
		joinedValue := strings.TrimSpace(strings.Join(values, ", "))
		if joinedValue == "" {
			continue
		}
		traceHeaders[normalizedKey] = truncateTraceValue(joinedValue, maxValueLen)
	}
	return traceHeaders
}

func BuildResponsesRequestToolTraceForLog(req *dto.OpenAIResponsesRequest) map[string]any {
	trace := map[string]any{}
	if req == nil {
		return trace
	}
	trace["model"] = req.Model
	if req.Stream != nil {
		trace["stream"] = *req.Stream
	}
	if req.Reasoning != nil {
		trace["reasoning_effort"] = req.Reasoning.Effort
	}
	if req.MaxOutputTokens != nil {
		trace["max_output_tokens"] = *req.MaxOutputTokens
	}
	trace["input_bytes"] = len(req.Input)
	trace["input_text_probe"] = BuildResponsesInputTextProbeForLog(req)
	trace["tools_bytes"] = len(req.Tools)
	trace["tool_choice_bytes"] = len(req.ToolChoice)
	trace["has_image_generation_tool"] = req.HasTool(dto.BuildInToolImageGeneration)
	trace["imagegen_keyword_hits"] = ResponsesRequestImageGenerationKeywordHits(req)
	if len(req.Tools) > 0 {
		trace["tool_types"] = toolTypesFromRawForTrace(req.Tools)
		trace["tools_raw"] = truncateTraceValue(string(req.Tools), clientRequestTraceMaxValueLen)
	}
	if len(req.ToolChoice) > 0 {
		trace["tool_choice_types"] = toolTypesFromRawForTrace(req.ToolChoice)
		trace["tool_choice_raw"] = truncateTraceValue(string(req.ToolChoice), clientRequestTraceMaxValueLen)
	}
	return trace
}

func BuildResponsesRequestToolTraceFromContextForLog(c *gin.Context) map[string]any {
	if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.URL.Path, "/v1/responses") {
		return map[string]any{}
	}
	req := dto.OpenAIResponsesRequest{}
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return map[string]any{"parse_error": err.Error()}
	}
	return BuildResponsesRequestToolTraceForLog(&req)
}

func BuildResponsesInputTextProbeForLog(req *dto.OpenAIResponsesRequest) map[string]any {
	probe := map[string]any{}
	if req == nil {
		return probe
	}
	texts := responsesRequestInputTexts(req)
	probe["text_part_count"] = len(texts)
	if len(texts) == 0 {
		return probe
	}
	last := recentResponsesRequestInputTexts(req)
	snippets := make([]string, 0, len(last))
	for _, text := range last {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		snippets = append(snippets, truncateTraceValue(text, 800))
	}
	probe["last_text_snippets"] = snippets
	probe["imagegen_keyword_hits"] = ResponsesRequestImageGenerationKeywordHits(req)
	return probe
}

func ResponsesRequestRequiresCodexImageGenerationTool(req *dto.OpenAIResponsesRequest) bool {
	if req == nil {
		return false
	}
	return len(ResponsesRequestImageGenerationKeywordHits(req)) > 0
}

func ResponsesRequestHasImageGenerationKeywordHits(req *dto.OpenAIResponsesRequest) bool {
	return len(ResponsesRequestImageGenerationKeywordHits(req)) > 0
}

func ResponsesRequestImageGenerationKeywordHits(req *dto.OpenAIResponsesRequest) []string {
	if req == nil {
		return nil
	}
	seen := map[string]struct{}{}
	hits := make([]string, 0)
	addText := func(text string) {
		for _, keyword := range imageGenerationKeywordHitsFromText(text) {
			if _, ok := seen[keyword]; ok {
				continue
			}
			seen[keyword] = struct{}{}
			hits = append(hits, keyword)
		}
	}
	for _, text := range recentResponsesRequestInputTexts(req) {
		addText(text)
	}
	if len(req.Tools) > 0 {
		addText(string(req.Tools))
	}
	if len(req.ToolChoice) > 0 {
		addText(string(req.ToolChoice))
	}
	return hits
}

func imageGenerationKeywordHitsFromText(text string) []string {
	text = stripImageGenerationKeywordIgnoredBlocks(text)
	lower := strings.ToLower(text)
	hits := make([]string, 0)
	for _, keyword := range codexImageGenerationFeatureKeywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			hits = append(hits, keyword)
		}
	}
	return hits
}

func responsesRequestInputTexts(req *dto.OpenAIResponsesRequest) []string {
	if req == nil {
		return nil
	}
	texts := make([]string, 0)
	for _, input := range req.ParseInput() {
		text := strings.TrimSpace(input.Text)
		if text != "" {
			texts = append(texts, text)
		}
	}
	if len(req.Instructions) > 0 {
		if instructions := strings.TrimSpace(string(req.Instructions)); instructions != "" {
			texts = append(texts, instructions)
		}
	}
	return texts
}

func recentResponsesRequestInputTexts(req *dto.OpenAIResponsesRequest) []string {
	texts := responsesRequestInputTexts(req)
	if len(texts) <= responsesInputTextProbeWindow {
		return texts
	}
	return texts[len(texts)-responsesInputTextProbeWindow:]
}

func stripImageGenerationKeywordIgnoredBlocks(text string) string {
	cleaned := text
	for _, pattern := range codexImageGenerationKeywordIgnoredBlocks {
		cleaned = pattern.ReplaceAllString(cleaned, "\n")
	}
	return cleaned
}

func MarshalTraceForLog(value any) string {
	bytes, err := common.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(bytes)
}

func truncateTraceValue(value string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = clientRequestTraceMaxValueLen
	}
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "...(truncated)"
}

func toolTypesFromRawForTrace(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var payload any
	if err := common.Unmarshal(raw, &payload); err != nil {
		return []string{"<parse_error>"}
	}
	seen := map[string]struct{}{}
	collectToolTypesForTrace(payload, seen)
	if len(seen) == 0 {
		return nil
	}
	result := make([]string, 0, len(seen))
	for toolType := range seen {
		result = append(result, toolType)
	}
	sort.Strings(result)
	return result
}

func collectToolTypesForTrace(value any, seen map[string]struct{}) {
	switch typedValue := value.(type) {
	case []any:
		for _, item := range typedValue {
			collectToolTypesForTrace(item, seen)
		}
	case []map[string]any:
		for _, item := range typedValue {
			collectToolTypesForTrace(item, seen)
		}
	case map[string]any:
		if toolType := strings.TrimSpace(common.Interface2String(typedValue["type"])); toolType != "" {
			seen[toolType] = struct{}{}
		}
		if toolType := strings.TrimSpace(common.Interface2String(typedValue["name"])); toolType == dto.BuildInToolImageGeneration {
			seen[toolType] = struct{}{}
		}
		collectToolTypesForTrace(typedValue["tool"], seen)
		collectToolTypesForTrace(typedValue["tools"], seen)
	case string:
		if typedValue == dto.BuildInToolImageGeneration {
			seen[typedValue] = struct{}{}
		}
	}
}
