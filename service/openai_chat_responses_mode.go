package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, model)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesGlobal(channelID, channelType, model)
}

func ShouldChatCompletionsUseResponsesForEndpoint(policy model_setting.ChatCompletionsToResponsesPolicy, forceResponsesWireAPI bool, channelID int, channelType int, model string) bool {
	if forceResponsesWireAPI {
		return true
	}
	if common.IsOpenAIResponseOnlyModel(model) {
		return true
	}
	if !ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, model) {
		return false
	}
	return !ChannelSupportsOpenAIChatCompletions(channelType, model)
}

func ShouldChatCompletionsUseResponsesForEndpointGlobal(forceResponsesWireAPI bool, channelID int, channelType int, model string) bool {
	return ShouldChatCompletionsUseResponsesForEndpoint(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		forceResponsesWireAPI,
		channelID,
		channelType,
		model,
	)
}

func ChannelSupportsOpenAIChatCompletions(channelType int, model string) bool {
	for _, endpointType := range common.GetEndpointTypesByChannelType(channelType, model) {
		if endpointType == constant.EndpointTypeOpenAI {
			return true
		}
	}
	return false
}
