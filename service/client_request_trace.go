package service

import (
	"encoding/json"
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

const (
	responsesImageGenerationKeywordSourceInputText    = "input_text"
	responsesImageGenerationKeywordSourceInstructions = "instructions"
	responsesImageGenerationKeywordSourceTools        = "tools"
	responsesImageGenerationKeywordSourceToolChoice   = "tool_choice"
)

const (
	responsesInputRoleUser         = "user"
	responsesInputRoleInstructions = "instructions"
)

var codexImageGenerationFeatureKeywords = []string{
	"$imagegen",
	"imagegen",
	"image generation",
	dto.BuildInToolImageGeneration,
}

var codexImageGenerationIntentMarkers = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{
		name:    "$imagegen",
		pattern: regexp.MustCompile(`(?i)(^|[^a-z0-9_])\$imagegen([^a-z0-9_-]|$)`),
	},
	{
		name:    "@imagegen",
		pattern: regexp.MustCompile(`(?i)(^|[^a-z0-9_])@imagegen([^a-z0-9_-]|$)`),
	},
}

var codexImageGenerationKeywordIgnoredBlocks = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<skills_instructions\b[^>]*>.*?</skills_instructions>`),
	regexp.MustCompile(`(?is)<plugins_instructions\b[^>]*>.*?</plugins_instructions>`),
	regexp.MustCompile(`(?is)<skill\b[^>]*>.*?</skill>`),
}

// ShouldLogClientRequestTrace returns true only when client request tracing is
// explicitly enabled. This keeps production quiet by default while still
// allowing Codex-like / Responses traces during debugging.
func ShouldLogClientRequestTrace(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if !common.DebugEnabled && !common.GetEnvOrDefaultBool("CLIENT_REQUEST_TRACE_ENABLED", false) {
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
	trace["requires_codex_image_tool"] = ResponsesRequestRequiresCodexImageGenerationTool(req)
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
	return probe
}

func ResponsesRequestRequiresCodexImageGenerationTool(req *dto.OpenAIResponsesRequest) bool {
	return req != nil && req.HasTool(dto.BuildInToolImageGeneration)
}

func ResponsesRequestHasImageGenerationKeywordHits(req *dto.OpenAIResponsesRequest) bool {
	return false
}

func ResponsesRequestImageGenerationKeywordHits(req *dto.OpenAIResponsesRequest) []string {
	return nil
}

func ResponsesRequestImageGenerationKeywordSources(req *dto.OpenAIResponsesRequest) map[string][]string {
	return nil
}

func ResponsesRequestRecentInputImageGenerationKeywordSources(req *dto.OpenAIResponsesRequest) map[string][]string {
	return nil
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

func imageGenerationIntentHitsFromText(text string) []string {
	text = stripImageGenerationKeywordIgnoredBlocks(text)
	hits := make([]string, 0)
	for _, marker := range codexImageGenerationIntentMarkers {
		if marker.pattern.MatchString(text) {
			hits = append(hits, marker.name)
		}
	}
	return hits
}

func responsesRequestInputTexts(req *dto.OpenAIResponsesRequest) []string {
	parts := responsesRequestInputTextParts(req)
	if len(parts) == 0 {
		return nil
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		texts = append(texts, part.Text)
	}
	return texts
}

type responsesRequestInputTextPart struct {
	Text   string
	Source string
	Role   string
}

func responsesRequestInputTextParts(req *dto.OpenAIResponsesRequest) []responsesRequestInputTextPart {
	return newResponsesImageGenerationRequirementDetector(req).InputTextParts()
}

func recentResponsesRequestInputTexts(req *dto.OpenAIResponsesRequest) []string {
	parts := recentResponsesRequestInputTextParts(req)
	if len(parts) == 0 {
		return nil
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		texts = append(texts, part.Text)
	}
	return texts
}

func recentResponsesRequestInputTextParts(req *dto.OpenAIResponsesRequest) []responsesRequestInputTextPart {
	return newResponsesImageGenerationRequirementDetector(req).RecentInputTextParts()
}

type responsesImageGenerationRequirementDetector struct {
	req *dto.OpenAIResponsesRequest
}

type responsesRawInputItem struct {
	Type    string          `json:"type,omitempty"`
	Role    string          `json:"role,omitempty"`
	Text    string          `json:"text,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
}

func newResponsesImageGenerationRequirementDetector(req *dto.OpenAIResponsesRequest) *responsesImageGenerationRequirementDetector {
	return &responsesImageGenerationRequirementDetector{req: req}
}

func (d *responsesImageGenerationRequirementDetector) RequiresCodexImageGenerationTool() bool {
	return d != nil && d.req != nil && d.req.HasTool(dto.BuildInToolImageGeneration)
}

func (d *responsesImageGenerationRequirementDetector) IntentHits() []string {
	seen := map[string]struct{}{}
	hits := make([]string, 0)
	for _, part := range d.RecentIntentTextParts() {
		for _, keyword := range imageGenerationIntentHitsFromText(part.Text) {
			if _, ok := seen[keyword]; ok {
				continue
			}
			seen[keyword] = struct{}{}
			hits = append(hits, keyword)
		}
	}
	return hits
}

func (d *responsesImageGenerationRequirementDetector) KeywordSources() map[string][]string {
	if d == nil || d.req == nil {
		return nil
	}
	sources := d.RecentIntentSources()
	if sources == nil {
		sources = map[string][]string{}
	}
	if len(d.req.Tools) > 0 {
		addImageGenerationSourceHits(sources, responsesImageGenerationKeywordSourceTools, imageGenerationKeywordHitsFromText(string(d.req.Tools)))
	}
	if len(d.req.ToolChoice) > 0 {
		addImageGenerationSourceHits(sources, responsesImageGenerationKeywordSourceToolChoice, imageGenerationKeywordHitsFromText(string(d.req.ToolChoice)))
	}
	if len(sources) == 0 {
		return nil
	}
	return sources
}

func (d *responsesImageGenerationRequirementDetector) RecentIntentSources() map[string][]string {
	sources := map[string][]string{}
	for _, part := range d.RecentIntentTextParts() {
		addImageGenerationSourceHits(sources, part.Source, imageGenerationIntentHitsFromText(part.Text))
	}
	if len(sources) == 0 {
		return nil
	}
	return sources
}

func (d *responsesImageGenerationRequirementDetector) RecentIntentTextParts() []responsesRequestInputTextPart {
	// Keep the public trace field names stable, but only let the current
	// trailing user turn drive routing requirements.
	parts := d.CurrentUserInputTextParts()
	filtered := make([]responsesRequestInputTextPart, 0, len(parts))
	for _, part := range parts {
		if part.Source != responsesImageGenerationKeywordSourceInputText {
			continue
		}
		if !responsesInputRoleAllowsUserIntent(part.Role) {
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

func (d *responsesImageGenerationRequirementDetector) CurrentUserInputTextParts() []responsesRequestInputTextPart {
	if d == nil || d.req == nil {
		return nil
	}
	parts := d.inputTextPartsFromRaw()
	if len(parts) == 0 {
		return nil
	}
	lastUserIndex := -1
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part.Source != responsesImageGenerationKeywordSourceInputText {
			continue
		}
		if responsesInputRoleAllowsUserIntent(part.Role) {
			lastUserIndex = i
			break
		}
		if responsesInputRoleIsConversationBoundary(part.Role) {
			return nil
		}
	}
	if lastUserIndex < 0 {
		return nil
	}
	start := lastUserIndex
	for start > 0 {
		prev := parts[start-1]
		if prev.Source != responsesImageGenerationKeywordSourceInputText || !responsesInputRoleAllowsUserIntent(prev.Role) {
			break
		}
		start--
	}
	return parts[start : lastUserIndex+1]
}

func (d *responsesImageGenerationRequirementDetector) RecentInputTextParts() []responsesRequestInputTextPart {
	parts := d.InputTextParts()
	if len(parts) <= responsesInputTextProbeWindow {
		return parts
	}
	return parts[len(parts)-responsesInputTextProbeWindow:]
}

func (d *responsesImageGenerationRequirementDetector) InputTextParts() []responsesRequestInputTextPart {
	if d == nil || d.req == nil {
		return nil
	}
	parts := d.inputTextPartsFromRaw()
	if len(d.req.Instructions) > 0 {
		if instructions := strings.TrimSpace(string(d.req.Instructions)); instructions != "" {
			parts = append(parts, responsesRequestInputTextPart{
				Text:   instructions,
				Source: responsesImageGenerationKeywordSourceInstructions,
				Role:   responsesInputRoleInstructions,
			})
		}
	}
	return parts
}

func (d *responsesImageGenerationRequirementDetector) inputTextPartsFromRaw() []responsesRequestInputTextPart {
	if d == nil || d.req == nil || len(d.req.Input) == 0 {
		return nil
	}
	return responsesInputTextPartsFromRaw(d.req.Input, responsesInputRoleUser)
}

func responsesInputTextPartsFromRaw(raw json.RawMessage, defaultRole string) []responsesRequestInputTextPart {
	switch common.GetJsonType(raw) {
	case "string":
		var text string
		if err := common.Unmarshal(raw, &text); err != nil {
			return nil
		}
		return appendResponsesTextPart(nil, text, responsesImageGenerationKeywordSourceInputText, defaultRole)
	case "array":
		var items []json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil
		}
		parts := make([]responsesRequestInputTextPart, 0, len(items))
		for _, item := range items {
			parts = append(parts, responsesInputTextPartsFromRaw(item, defaultRole)...)
		}
		return parts
	case "object":
		var item responsesRawInputItem
		if err := common.Unmarshal(raw, &item); err != nil {
			return nil
		}
		role := normalizeResponsesInputRole(item.Role, defaultRole)
		parts := make([]responsesRequestInputTextPart, 0, 1)
		if strings.TrimSpace(item.Text) != "" {
			parts = appendResponsesTextPart(parts, item.Text, responsesImageGenerationKeywordSourceInputText, role)
		}
		if len(item.Content) > 0 {
			parts = append(parts, responsesContentTextPartsFromRaw(item.Content, role)...)
		}
		return parts
	default:
		return nil
	}
}

func responsesContentTextPartsFromRaw(raw json.RawMessage, role string) []responsesRequestInputTextPart {
	switch common.GetJsonType(raw) {
	case "string":
		var text string
		if err := common.Unmarshal(raw, &text); err != nil {
			return nil
		}
		return appendResponsesTextPart(nil, text, responsesImageGenerationKeywordSourceInputText, role)
	case "array":
		var items []json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil
		}
		parts := make([]responsesRequestInputTextPart, 0, len(items))
		for _, item := range items {
			parts = append(parts, responsesContentTextPartsFromRaw(item, role)...)
		}
		return parts
	case "object":
		var item responsesRawInputItem
		if err := common.Unmarshal(raw, &item); err != nil {
			return nil
		}
		return appendResponsesTextPart(nil, item.Text, responsesImageGenerationKeywordSourceInputText, role)
	default:
		return nil
	}
}

func appendResponsesTextPart(parts []responsesRequestInputTextPart, text string, source string, role string) []responsesRequestInputTextPart {
	text = strings.TrimSpace(text)
	if text == "" {
		return parts
	}
	return append(parts, responsesRequestInputTextPart{
		Text:   text,
		Source: source,
		Role:   normalizeResponsesInputRole(role, responsesInputRoleUser),
	})
}

func normalizeResponsesInputRole(role string, fallback string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(fallback))
	}
	return role
}

func responsesInputRoleAllowsUserIntent(role string) bool {
	role = normalizeResponsesInputRole(role, responsesInputRoleUser)
	return role == responsesInputRoleUser
}

func responsesInputRoleIsConversationBoundary(role string) bool {
	switch normalizeResponsesInputRole(role, "") {
	case "assistant", "tool", "function":
		return true
	default:
		return false
	}
}

func addImageGenerationSourceHits(sources map[string][]string, source string, hits []string) {
	if len(hits) == 0 {
		return
	}
	existing := sources[source]
	seen := make(map[string]struct{}, len(existing)+len(hits))
	for _, keyword := range existing {
		seen[keyword] = struct{}{}
	}
	for _, keyword := range hits {
		if _, ok := seen[keyword]; ok {
			continue
		}
		seen[keyword] = struct{}{}
		existing = append(existing, keyword)
	}
	sources[source] = existing
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
