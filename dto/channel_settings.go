package dto

import (
	"slices"
	"strings"
)

type ChannelSettings struct {
	ForceFormat            bool   `json:"force_format,omitempty"`
	ForceStreamResponse    bool   `json:"force_stream_response,omitempty"`
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`
	Proxy                  string `json:"proxy"`
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"`
	SystemPrompt           string `json:"system_prompt,omitempty"`
	SystemPromptOverride   bool   `json:"system_prompt_override,omitempty"`
	MaxConcurrency         int    `json:"max_concurrency,omitempty"`
	MaxConcurrencyCeiling  int    `json:"max_concurrency_ceiling,omitempty"`
	AccountMaxConcurrency  int    `json:"-"`
	AccountConcurrencyKey  string `json:"-"`
}

type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"
	VertexKeyTypeAPIKey VertexKeyType = "api_key"
)

type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk" // 默认
	AwsKeyTypeApiKey AwsKeyType = "api_key"
)

type ChannelOtherSettings struct {
	AzureResponsesVersion                 string             `json:"azure_responses_version,omitempty"`
	WireAPI                               string             `json:"wire_api,omitempty"`
	ProviderProfile                       string             `json:"provider_profile,omitempty"`
	ProxyProfile                          string             `json:"proxy_profile,omitempty"`
	SmartScoreBoosts                      map[string]float64 `json:"smart_score_boosts,omitempty"`       // 智能调度渠道分值加成，key 为评分项，value 为 0..1 的正向加成
	CodexCompatibilityMode                bool               `json:"codex_compatibility_mode,omitempty"` // Codex 兼容模式：用于声明该 OpenAI 兼容上游支持 Codex Responses 能力
	CodexImageGenerationToolSupported     bool               `json:"codex_image_generation_tool_supported,omitempty"`
	CodexImageGenerationToolProbeTime     int64              `json:"codex_image_generation_tool_probe_time,omitempty"`
	CodexImageGenerationToolProbeMessage  string             `json:"codex_image_generation_tool_probe_message,omitempty"`
	CodexImageGenerationToolProbeModels   []string           `json:"codex_image_generation_tool_probe_models,omitempty"`
	CodexSupportedTools                   []string           `json:"codex_supported_tools,omitempty"`
	VertexKeyType                         VertexKeyType      `json:"vertex_key_type,omitempty"` // "json" or "api_key"
	OpenRouterEnterprise                  *bool              `json:"openrouter_enterprise,omitempty"`
	ClaudeBetaQuery                       bool               `json:"claude_beta_query,omitempty"`         // Claude 渠道是否强制追加 ?beta=true
	AllowServiceTier                      bool               `json:"allow_service_tier,omitempty"`        // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	AllowInferenceGeo                     bool               `json:"allow_inference_geo,omitempty"`       // 是否允许 inference_geo 透传（仅 Claude，默认过滤以满足数据驻留合规
	AllowSpeed                            bool               `json:"allow_speed,omitempty"`               // 是否允许 speed 透传（仅 Claude，默认过滤以避免意外切换推理速度模式）
	AllowSafetyIdentifier                 bool               `json:"allow_safety_identifier,omitempty"`   // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	DisableStore                          bool               `json:"disable_store,omitempty"`             // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowIncludeObfuscation               bool               `json:"allow_include_obfuscation,omitempty"` // 是否允许 stream_options.include_obfuscation 透传（默认过滤以避免关闭流混淆保护）
	SupportStreamOptions                  *bool              `json:"support_stream_options,omitempty"`    // 渠道级 stream_options 能力覆盖；空值使用通道类型默认
	SupportResponsesPreviousID            *bool              `json:"support_responses_previous_id,omitempty"`
	AwsKeyType                            AwsKeyType         `json:"aws_key_type,omitempty"`
	UpstreamModelUpdateCheckEnabled       bool               `json:"upstream_model_update_check_enabled,omitempty"`        // 是否检测上游模型更新
	UpstreamModelUpdateAutoSyncEnabled    bool               `json:"upstream_model_update_auto_sync_enabled,omitempty"`    // 是否自动同步上游模型更新
	UpstreamModelUpdateLastCheckTime      int64              `json:"upstream_model_update_last_check_time,omitempty"`      // 上次检测时间
	UpstreamModelUpdateLastDetectedModels []string           `json:"upstream_model_update_last_detected_models,omitempty"` // 上次检测到的可加入模型
	UpstreamModelUpdateLastRemovedModels  []string           `json:"upstream_model_update_last_removed_models,omitempty"`  // 上次检测到的可删除模型
	UpstreamModelUpdateIgnoredModels      []string           `json:"upstream_model_update_ignored_models,omitempty"`       // 手动忽略的模型
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}

func (s *ChannelOtherSettings) UsesResponsesWireAPI() bool {
	if s == nil {
		return false
	}
	wireAPI := strings.ToLower(strings.Trim(strings.TrimSpace(s.WireAPI), "/"))
	if wireAPI == "" {
		return false
	}
	if wireAPI == "responses" || strings.HasPrefix(wireAPI, "responses/") {
		return true
	}
	return strings.HasSuffix(wireAPI, "/responses") || strings.Contains(wireAPI, "/responses/")
}

func (s ChannelOtherSettings) UsesCodexCompatibilityMode() bool {
	return s.CodexCompatibilityMode
}

func (s ChannelOtherSettings) SupportsCodexImageGenerationTool() bool {
	return s.CodexCompatibilityMode && (s.CodexImageGenerationToolSupported || slices.Contains(s.CodexSupportedTools, BuildInToolImageGeneration))
}

func (s *ChannelOtherSettings) GetOpenAIWireAPIPath(isCompact bool) string {
	if s == nil {
		return ""
	}

	wireAPI := strings.TrimSpace(s.WireAPI)
	if wireAPI == "" {
		return ""
	}

	if strings.EqualFold(wireAPI, "responses") {
		if isCompact {
			return "/responses/compact"
		}
		return "/responses"
	}

	if !strings.HasPrefix(wireAPI, "/") {
		wireAPI = "/" + wireAPI
	}
	wireAPI = strings.TrimRight(wireAPI, "/")
	if wireAPI == "" {
		return ""
	}
	if isCompact && !strings.HasSuffix(wireAPI, "/compact") {
		return wireAPI + "/compact"
	}
	return wireAPI
}
