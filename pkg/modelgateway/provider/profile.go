package provider

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	ProfileOpenAICodex              = "openai_codex"
	ProfileMiMoCodexChat            = "mimo_codex_chat"
	ProfileDeepSeekV4ProCodexChat   = "deepseek_v4_pro_codex_chat"
	ProfileStandardOpenAICompatible = "standard_openai_compatible"

	ProxyModeNative                  = "native"
	ProxyModeNativeResponses         = "native_responses"
	ProxyModeCodexCompatibleResponse = "codex_compatible_responses"
	ProxyModeResponsesViaChat        = "responses_via_chat"
)

type CapabilitySet struct {
	NativeResponses       bool     `json:"native_responses,omitempty"`
	ResponsesCompact      bool     `json:"responses_compact,omitempty"`
	ResponsesViaChat      bool     `json:"responses_via_chat,omitempty"`
	ChatCompletions       bool     `json:"chat_completions,omitempty"`
	Streaming             bool     `json:"streaming,omitempty"`
	ToolCall              bool     `json:"tool_call,omitempty"`
	WebSearch             bool     `json:"web_search,omitempty"`
	Reasoning             bool     `json:"reasoning,omitempty"`
	CodexImageTool        bool     `json:"codex_image_tool,omitempty"`
	SupportedProxyModes   []string `json:"supported_proxy_modes,omitempty"`
	CapabilityFingerprint string   `json:"capability_fingerprint,omitempty"`
}

type ProviderProfile interface {
	Name() string
	Family() string
	Match(channel *model.Channel, modelName string) bool
	Capabilities(channel *model.Channel, modelName string) CapabilitySet
	ProxyMode(channel *model.Channel, modelName string) string
}

type BaseProviderProfile struct {
	name          string
	family        string
	modelPatterns []string
	capabilities  CapabilitySet
	proxyMode     string
	match         func(channel *model.Channel, modelName string) bool
}

func NewBaseProviderProfile(name string, family string, modelPatterns []string, capabilities CapabilitySet, proxyMode string, match func(channel *model.Channel, modelName string) bool) BaseProviderProfile {
	return BaseProviderProfile{
		name:          name,
		family:        family,
		modelPatterns: append([]string(nil), modelPatterns...),
		capabilities:  capabilities,
		proxyMode:     proxyMode,
		match:         match,
	}
}

func (p BaseProviderProfile) Name() string {
	return p.name
}

func (p BaseProviderProfile) Family() string {
	return p.family
}

func (p BaseProviderProfile) Match(channel *model.Channel, modelName string) bool {
	if channel == nil {
		return false
	}
	if explicit := explicitProviderProfile(channel); explicit != "" {
		return explicit == p.name
	}
	if p.match != nil && p.match(channel, modelName) {
		return true
	}
	return p.matchModel(modelName)
}

func (p BaseProviderProfile) Capabilities(channel *model.Channel, modelName string) CapabilitySet {
	capability := p.capabilities
	if channel != nil && channel.GetOtherSettings().SupportsCodexImageGenerationTool() {
		capability.CodexImageTool = true
	}
	capability.CapabilityFingerprint = p.capabilityFingerprint(capability)
	return capability
}

func (p BaseProviderProfile) ProxyMode(channel *model.Channel, modelName string) string {
	if channel != nil {
		if proxyProfile := strings.TrimSpace(channel.GetOtherSettings().ProxyProfile); proxyProfile != "" {
			return proxyProfile
		}
	}
	if p.proxyMode != "" {
		return p.proxyMode
	}
	return ProxyModeNative
}

func (p BaseProviderProfile) matchModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if modelName == "" {
		return false
	}
	for _, pattern := range p.modelPatterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(modelName, strings.TrimSuffix(pattern, "*")) {
				return true
			}
			continue
		}
		if modelName == pattern {
			return true
		}
	}
	return false
}

func (p BaseProviderProfile) capabilityFingerprint(capability CapabilitySet) string {
	payload := map[string]any{
		"profile":               p.name,
		"family":                p.family,
		"native_responses":      capability.NativeResponses,
		"responses_compact":     capability.ResponsesCompact,
		"responses_via_chat":    capability.ResponsesViaChat,
		"chat_completions":      capability.ChatCompletions,
		"streaming":             capability.Streaming,
		"tool_call":             capability.ToolCall,
		"web_search":            capability.WebSearch,
		"reasoning":             capability.Reasoning,
		"codex_image_tool":      capability.CodexImageTool,
		"supported_proxy_modes": capability.SupportedProxyModes,
	}
	bytes, err := common.Marshal(payload)
	if err != nil {
		return p.name
	}
	return string(bytes)
}

type OpenAICodexProfile struct {
	BaseProviderProfile
}

func NewOpenAICodexProfile() *OpenAICodexProfile {
	return &OpenAICodexProfile{BaseProviderProfile: NewBaseProviderProfile(
		ProfileOpenAICodex,
		"openai",
		nil,
		CapabilitySet{
			NativeResponses:     true,
			ResponsesCompact:    true,
			ChatCompletions:     false,
			Streaming:           true,
			ToolCall:            true,
			SupportedProxyModes: []string{ProxyModeNativeResponses},
		},
		ProxyModeNativeResponses,
		func(channel *model.Channel, modelName string) bool {
			return channel.Type == constant.ChannelTypeCodex || channel.GetOtherSettings().UsesCodexCompatibilityMode()
		},
	)}
}

type MiMoCodexChatProfile struct {
	BaseProviderProfile
}

func NewMiMoCodexChatProfile() *MiMoCodexChatProfile {
	return &MiMoCodexChatProfile{BaseProviderProfile: NewBaseProviderProfile(
		ProfileMiMoCodexChat,
		"mimo",
		[]string{"mimo*", "xiaomi*", "mi-mo*"},
		CapabilitySet{
			ResponsesViaChat:    true,
			ChatCompletions:     true,
			Streaming:           true,
			ToolCall:            true,
			WebSearch:           true,
			Reasoning:           true,
			SupportedProxyModes: []string{ProxyModeResponsesViaChat},
		},
		ProxyModeResponsesViaChat,
		func(channel *model.Channel, modelName string) bool {
			return containsAny(channel.Name, "mimo", "xiaomi", "mi-mo") ||
				containsAny(channel.GetBaseURL(), "mimo", "xiaomi")
		},
	)}
}

type DeepSeekV4ProCodexChatProfile struct {
	BaseProviderProfile
}

func NewDeepSeekV4ProCodexChatProfile() *DeepSeekV4ProCodexChatProfile {
	return &DeepSeekV4ProCodexChatProfile{BaseProviderProfile: NewBaseProviderProfile(
		ProfileDeepSeekV4ProCodexChat,
		"deepseek",
		[]string{"deepseek-v4-pro*"},
		CapabilitySet{
			ResponsesViaChat:    true,
			ChatCompletions:     true,
			Streaming:           true,
			ToolCall:            true,
			Reasoning:           true,
			SupportedProxyModes: []string{ProxyModeResponsesViaChat},
		},
		ProxyModeResponsesViaChat,
		func(channel *model.Channel, modelName string) bool {
			return channel.Type == constant.ChannelTypeDeepSeek && strings.HasPrefix(strings.ToLower(modelName), "deepseek-v4-pro")
		},
	)}
}

type StandardOpenAICompatibleProfile struct {
	BaseProviderProfile
}

func NewStandardOpenAICompatibleProfile() *StandardOpenAICompatibleProfile {
	return &StandardOpenAICompatibleProfile{BaseProviderProfile: NewBaseProviderProfile(
		ProfileStandardOpenAICompatible,
		"openai_compatible",
		nil,
		CapabilitySet{
			ResponsesViaChat:    true,
			ChatCompletions:     true,
			Streaming:           true,
			ToolCall:            true,
			SupportedProxyModes: []string{ProxyModeNative, ProxyModeResponsesViaChat},
		},
		ProxyModeNative,
		func(channel *model.Channel, modelName string) bool {
			return channel.Type == constant.ChannelTypeOpenAI ||
				channel.Type == constant.ChannelTypeCustom ||
				channel.Type == constant.ChannelTypeDeepSeek
		},
	)}
}

func explicitProviderProfile(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	return strings.TrimSpace(channel.GetOtherSettings().ProviderProfile)
}

func containsAny(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func ChannelOtherSettingsForTest(providerProfile string, proxyProfile string) string {
	settings := dto.ChannelOtherSettings{
		ProviderProfile: providerProfile,
		ProxyProfile:    proxyProfile,
	}
	bytes, err := common.Marshal(settings)
	if err != nil {
		return ""
	}
	return string(bytes)
}
