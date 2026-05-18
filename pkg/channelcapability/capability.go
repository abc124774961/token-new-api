package channelcapability

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

// SupportedEndpointTypes returns the effective endpoint capabilities for one
// concrete channel. It keeps default OpenAI-compatible behavior broad, and uses
// explicit channel settings for Codex-specific routing decisions.
func SupportedEndpointTypes(channelType int, modelName string, settings dto.ChannelOtherSettings) []constant.EndpointType {
	endpointTypes := common.GetEndpointTypesByChannelType(channelType, modelName)
	isImageModel := common.IsImageGenerationModel(modelName)
	if requiresCodexCompatibilityForOpenAIImage(modelName, channelType) &&
		!settings.UsesCodexCompatibilityMode() {
		endpointTypes = withoutImageEndpoints(endpointTypes)
	}
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
		return settings.UsesCodexCompatibilityMode()
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
	if requiresCodexCompatibilityForOpenAIImage(modelName, channelType) {
		return settings.UsesCodexCompatibilityMode()
	}
	return slices.Contains(common.GetEndpointTypesByChannelType(channelType, modelName), constant.EndpointTypeImageGeneration)
}

func requiresCodexCompatibilityForOpenAIImage(modelName string, channelType int) bool {
	return channelType == constant.ChannelTypeOpenAI && isCodexImageModel(modelName)
}

func isCodexImageModel(modelName string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "gpt-image-")
}

func appendUniqueEndpointType(items []constant.EndpointType, values ...constant.EndpointType) []constant.EndpointType {
	for _, value := range values {
		if !slices.Contains(items, value) {
			items = append(items, value)
		}
	}
	return items
}

func withoutImageEndpoints(items []constant.EndpointType) []constant.EndpointType {
	result := make([]constant.EndpointType, 0, len(items))
	for _, item := range items {
		switch item {
		case constant.EndpointTypeImageGeneration, constant.EndpointTypeImageEdit:
			continue
		default:
			result = append(result, item)
		}
	}
	return result
}
