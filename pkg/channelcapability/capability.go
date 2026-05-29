package channelcapability

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

type AccountCapability struct {
	ResponsesWrite        *bool `json:"responses_write,omitempty"`
	ResponsesCompactWrite *bool `json:"responses_compact_write,omitempty"`
	ChatCompletionsWrite  *bool `json:"chat_completions_write,omitempty"`

	CodexBackendResponsesStreamWrite *bool `json:"codex_backend_responses_stream_write,omitempty"`
	CodexBackendCompactWrite         *bool `json:"codex_backend_compact_write,omitempty"`
	CodexBackendRequiresStream       *bool `json:"codex_backend_requires_stream,omitempty"`
	PlatformChatCompletionsWrite     *bool `json:"platform_chat_completions_write,omitempty"`
	PlatformResponsesWrite           *bool `json:"platform_responses_write,omitempty"`
	PlatformResponsesCompactWrite    *bool `json:"platform_responses_compact_write,omitempty"`

	CapabilityClassification string `json:"capability_classification,omitempty"`
	CapabilityProbeSurface   string `json:"capability_probe_surface,omitempty"`
	ProxyID                  int    `json:"proxy_id,omitempty"`
	ProxyExitIP              string `json:"proxy_exit_ip,omitempty"`
	ProxyRegion              string `json:"proxy_region,omitempty"`
	ProxyCheckedTime         int64  `json:"proxy_checked_time,omitempty"`
	ProxyLastError           string `json:"proxy_last_error,omitempty"`
	CheckedTime              int64  `json:"checked_time,omitempty"`
	LastEndpoint             string `json:"last_endpoint,omitempty"`
	LastMessage              string `json:"last_message,omitempty"`
}

const (
	ClassificationCodexBackendAvailable      = "codex_backend_available"
	ClassificationCodexCompactAvailable      = "codex_compact_available"
	ClassificationStreamOnly                 = "stream_only"
	ClassificationPlatformQuotaInsufficient  = "platform_quota_insufficient"
	ClassificationPlatformResponsesScopeMiss = "platform_responses_scope_missing"
	ClassificationProxyError                 = "proxy_error"
	ClassificationAuthError                  = "auth_error"
	ClassificationRegionError                = "region_error"
	ClassificationUnknown                    = "unknown"
	ProbeSurfaceCodexBackend                 = "codex_backend"
	ProbeSurfacePlatformAPI                  = "platform_api"
)

func (cap AccountCapability) HasResponsesWriteDenied() bool {
	return cap.ResponsesWrite != nil && !*cap.ResponsesWrite
}

func (cap AccountCapability) HasChatCompletionsWriteAllowed() bool {
	return cap.ChatCompletionsWrite != nil && *cap.ChatCompletionsWrite
}

func (cap AccountCapability) HasResponsesWriteAllowed() bool {
	return cap.ResponsesWrite != nil && *cap.ResponsesWrite
}

func (cap AccountCapability) HasResponsesCompactWriteAllowed() bool {
	return cap.ResponsesCompactWrite != nil && *cap.ResponsesCompactWrite
}

func (cap AccountCapability) HasCodexBackendResponsesStreamAllowed() bool {
	return cap.CodexBackendResponsesStreamWrite != nil && *cap.CodexBackendResponsesStreamWrite
}

func (cap AccountCapability) HasCodexBackendCompactAllowed() bool {
	return cap.CodexBackendCompactWrite != nil && *cap.CodexBackendCompactWrite
}

func (cap AccountCapability) EffectiveClassification() string {
	if cap.HasCodexBackendResponsesStreamAllowed() {
		if cap.HasCodexBackendCompactAllowed() {
			return ClassificationCodexCompactAvailable
		}
		return ClassificationCodexBackendAvailable
	}
	if strings.TrimSpace(cap.ProxyLastError) != "" {
		return ClassificationProxyError
	}
	if classification := strings.TrimSpace(cap.CapabilityClassification); classification != "" {
		return classification
	}
	return ""
}

// SupportedEndpointTypes returns the effective endpoint capabilities for one
// concrete channel. It keeps default OpenAI-compatible behavior broad, and uses
// explicit channel settings for Codex-specific routing decisions.
func SupportedEndpointTypes(channelType int, modelName string, settings dto.ChannelOtherSettings) []constant.EndpointType {
	endpointTypes := common.GetEndpointTypesByChannelType(channelType, modelName)
	isImageModel := common.IsImageGenerationModel(modelName)
	if SupportsResponsesWireAPI(channelType, settings) {
		if isImageModel {
			endpointTypes = appendUniqueEndpointType(endpointTypes, constant.EndpointTypeOpenAIResponse)
		} else {
			endpointTypes = appendUniqueEndpointType([]constant.EndpointType{constant.EndpointTypeOpenAIResponse}, endpointTypes...)
		}
	}
	if SupportsResponsesCompact(channelType, settings) {
		endpointTypes = appendUniqueEndpointType(endpointTypes, constant.EndpointTypeOpenAIResponseCompact)
	}
	return endpointTypes
}

func SupportsEndpoint(channelType int, modelName string, settings dto.ChannelOtherSettings, endpointType constant.EndpointType) bool {
	if endpointType == "" {
		return true
	}
	return slices.Contains(SupportedEndpointTypes(channelType, modelName, settings), endpointType)
}

func SupportsCodexImageGenerationTool(channelType int, settings dto.ChannelOtherSettings) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI:
		return settings.SupportsCodexImageGenerationTool()
	case constant.ChannelTypeCodex:
		return true
	default:
		return false
	}
}

func SupportsResponsesWireAPI(channelType int, settings dto.ChannelOtherSettings) bool {
	if settings.UsesCodexCompatibilityMode() {
		return channelType == constant.ChannelTypeOpenAI || channelType == constant.ChannelTypeCodex
	}
	if !settings.UsesResponsesWireAPI() {
		return false
	}
	switch channelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeCodex, constant.ChannelTypeXai:
		return true
	default:
		return false
	}
}

func SupportsResponsesCompact(channelType int, settings dto.ChannelOtherSettings) bool {
	if channelType == constant.ChannelTypeCodex {
		return true
	}
	return channelType == constant.ChannelTypeOpenAI && settings.UsesCodexCompatibilityMode()
}

func SupportsOpenAIImage(modelName string, channelType int, settings dto.ChannelOtherSettings) bool {
	if !common.IsImageGenerationModel(modelName) {
		return false
	}
	return slices.Contains(common.GetEndpointTypesByChannelType(channelType, modelName), constant.EndpointTypeImageGeneration)
}

func appendUniqueEndpointType(items []constant.EndpointType, values ...constant.EndpointType) []constant.EndpointType {
	for _, value := range values {
		if !slices.Contains(items, value) {
			items = append(items, value)
		}
	}
	return items
}
