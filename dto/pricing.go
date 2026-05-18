package dto

import "github.com/QuantumNous/new-api/constant"

// 这里不好动就不动了，本来想独立出来的（
type OpenAIModels struct {
	Id                         string                  `json:"id"`
	Object                     string                  `json:"object"`
	Created                    int                     `json:"created"`
	OwnedBy                    string                  `json:"owned_by"`
	SupportedEndpointTypes     []constant.EndpointType `json:"supported_endpoint_types"`
	SupportedSessionModes      []string                `json:"supported_session_modes"`
	ActualModelReturned        map[string]string       `json:"actual_model_returned"`
	ExperimentalSupportedTools []string                `json:"experimental_supported_tools,omitempty"`
	InputModalities            []string                `json:"input_modalities,omitempty"`
}

type CodexModelsResponse struct {
	Models []CodexModelInfo `json:"models"`
}

type CodexModelInfo struct {
	Slug                          string                  `json:"slug"`
	DisplayName                   string                  `json:"display_name"`
	Description                   *string                 `json:"description"`
	DefaultReasoningLevel         string                  `json:"default_reasoning_level"`
	SupportedReasoningLevels      []CodexReasoningLevel   `json:"supported_reasoning_levels"`
	ShellType                     string                  `json:"shell_type"`
	Visibility                    string                  `json:"visibility"`
	SupportedInAPI                bool                    `json:"supported_in_api"`
	Priority                      int                     `json:"priority"`
	AvailabilityNux               any                     `json:"availability_nux"`
	Upgrade                       any                     `json:"upgrade"`
	BaseInstructions              string                  `json:"base_instructions"`
	SupportsReasoningSummaries    bool                    `json:"supports_reasoning_summaries"`
	DefaultReasoningSummary       string                  `json:"default_reasoning_summary"`
	SupportVerbosity              bool                    `json:"support_verbosity"`
	DefaultVerbosity              any                     `json:"default_verbosity"`
	ApplyPatchToolType            *string                 `json:"apply_patch_tool_type"`
	WebSearchToolType             string                  `json:"web_search_tool_type"`
	TruncationPolicy              CodexTruncationPolicy   `json:"truncation_policy"`
	SupportsParallelToolCalls     bool                    `json:"supports_parallel_tool_calls"`
	SupportsImageDetailOriginal   bool                    `json:"supports_image_detail_original"`
	ContextWindow                 int                     `json:"context_window"`
	EffectiveContextWindowPercent int                     `json:"effective_context_window_percent"`
	ExperimentalSupportedTools    []string                `json:"experimental_supported_tools"`
	InputModalities               []string                `json:"input_modalities"`
	ActualModelReturned           map[string]string       `json:"actual_model_returned,omitempty"`
	SupportedEndpointTypes        []constant.EndpointType `json:"supported_endpoint_types,omitempty"`
	SupportedSessionModes         []string                `json:"supported_session_modes,omitempty"`
}

type CodexReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type CodexTruncationPolicy struct {
	Mode  string `json:"mode"`
	Limit int    `json:"limit"`
}

type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

type GeminiModel struct {
	Name                       interface{}   `json:"name"`
	BaseModelId                interface{}   `json:"baseModelId"`
	Version                    interface{}   `json:"version"`
	DisplayName                interface{}   `json:"displayName"`
	Description                interface{}   `json:"description"`
	InputTokenLimit            interface{}   `json:"inputTokenLimit"`
	OutputTokenLimit           interface{}   `json:"outputTokenLimit"`
	SupportedGenerationMethods []interface{} `json:"supportedGenerationMethods"`
	Thinking                   interface{}   `json:"thinking"`
	Temperature                interface{}   `json:"temperature"`
	MaxTemperature             interface{}   `json:"maxTemperature"`
	TopP                       interface{}   `json:"topP"`
	TopK                       interface{}   `json:"topK"`
}
