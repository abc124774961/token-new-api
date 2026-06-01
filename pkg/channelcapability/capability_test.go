package channelcapability

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestSupportedEndpointTypesKeepsOpenAIImageApiSeparateFromCodexImageTool(t *testing.T) {
	endpoints := SupportedEndpointTypes(constant.ChannelTypeOpenAI, "gpt-image-2", dto.ChannelOtherSettings{})
	require.Equal(t, constant.EndpointTypeImageGeneration, endpoints[0])
	require.Equal(t, constant.EndpointTypeImageEdit, endpoints[1])
	require.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
	require.Contains(t, endpoints, constant.EndpointTypeImageEdit)
	require.NotContains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)

	endpoints = SupportedEndpointTypes(constant.ChannelTypeOpenAI, "gpt-image-2", dto.ChannelOtherSettings{
		CodexCompatibilityMode:            true,
		CodexImageGenerationToolSupported: true,
	})
	require.Equal(t, constant.EndpointTypeImageGeneration, endpoints[0])
	require.Equal(t, constant.EndpointTypeImageEdit, endpoints[1])
	require.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
	require.Contains(t, endpoints, constant.EndpointTypeImageEdit)
	require.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
	require.Contains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestSupportsCodexImageGenerationToolAcceptsGenericToolList(t *testing.T) {
	require.True(t, SupportsCodexImageGenerationTool(constant.ChannelTypeOpenAI, dto.ChannelOtherSettings{
		CodexCompatibilityMode: true,
		CodexSupportedTools:    []string{dto.BuildInToolImageGeneration},
	}))
}

func TestSupportedEndpointTypesKeepsOrdinaryImageModelsForOpenAI(t *testing.T) {
	endpoints := SupportedEndpointTypes(constant.ChannelTypeOpenAI, "dall-e-3", dto.ChannelOtherSettings{})
	require.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
	require.Contains(t, endpoints, constant.EndpointTypeImageEdit)
}

func TestSupportedEndpointTypesKeepsNativeImageChannels(t *testing.T) {
	endpoints := SupportedEndpointTypes(constant.ChannelTypeGemini, "imagen-4.0-generate-001", dto.ChannelOtherSettings{})
	require.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
	require.Contains(t, endpoints, constant.EndpointTypeImageEdit)
}

func TestSupportedEndpointTypesKeepsResponsesWireAPIWithoutCodexImageCapability(t *testing.T) {
	endpoints := SupportedEndpointTypes(constant.ChannelTypeOpenAI, "gpt-image-2", dto.ChannelOtherSettings{WireAPI: "responses"})
	require.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
	require.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
	require.Contains(t, endpoints, constant.EndpointTypeImageEdit)
	require.NotContains(t, endpoints, constant.EndpointTypeOpenAIResponseCompact)
}

func TestEffectiveClassificationPrefersActiveUsageLimitOverCapability(t *testing.T) {
	allowed := true
	capability := AccountCapability{
		CodexBackendResponsesStreamWrite: &allowed,
		CodexBackendCompactWrite:         &allowed,
		UsageLimitStatus:                 UsageLimitStatusLimited,
		UsageLimitReason:                 UsageLimitReasonReached,
		UsageLimitExpiresAt:              9999999999,
		UsageLimitResetSource:            "retry_after_seconds",
	}

	require.Equal(t, ClassificationAccountUsageLimited, capability.EffectiveClassification())
	require.True(t, capability.UsageLimitActiveAt(100))
	require.False(t, capability.UsageLimitActiveAt(9999999999))
	cleared := capability.ClearUsageLimit()
	require.Empty(t, cleared.UsageLimitResetSource)
}

func TestEffectiveClassificationKeepsAuthErrorAheadOfProxyExitError(t *testing.T) {
	capability := AccountCapability{
		CapabilityClassification: ClassificationAuthError,
		ProxyLastError:           "invalid character 'P' looking for beginning of value",
	}

	require.Equal(t, ClassificationAuthError, capability.EffectiveClassification())
}
