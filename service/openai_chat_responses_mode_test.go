package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func TestShouldChatCompletionsUseResponsesForEndpointPrefersChatWhenSupported(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{`^gpt-5\.4$`},
	}

	if ShouldChatCompletionsUseResponsesForEndpoint(policy, false, 4, constant.ChannelTypeOpenAI, "gpt-5.4") {
		t.Fatal("expected OpenAI-compatible chat request to keep using chat completions")
	}
}

func TestShouldChatCompletionsUseResponsesForEndpointHonorsExplicitWireAPI(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{}

	if !ShouldChatCompletionsUseResponsesForEndpoint(policy, true, 4, constant.ChannelTypeOpenAI, "gpt-5.4") {
		t.Fatal("expected explicit responses wire API setting to force responses")
	}
}

func TestShouldChatCompletionsUseResponsesForEndpointUsesResponsesOnlyModels(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{}

	if !ShouldChatCompletionsUseResponsesForEndpoint(policy, false, 4, constant.ChannelTypeOpenAI, "o3-pro") {
		t.Fatal("expected responses-only model to use responses")
	}
}

func TestShouldChatCompletionsUseResponsesForEndpointUsesPolicyWhenChatUnsupported(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{`^gpt-5\.4$`},
	}

	if !ShouldChatCompletionsUseResponsesForEndpoint(policy, false, 57, constant.ChannelTypeCodex, "gpt-5.4") {
		t.Fatal("expected policy to route through responses when channel has no chat completions endpoint")
	}
}
