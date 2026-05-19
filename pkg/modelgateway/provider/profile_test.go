package provider_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/stretchr/testify/require"
)

func TestStandardProviderRegistryMatchesFirstBatchProfiles(t *testing.T) {
	registry := provider.NewStandardProviderRegistry()

	codex := registry.Best(&model.Channel{Id: 1, Type: constant.ChannelTypeCodex, Name: "codex"}, "gpt-5.5")
	require.NotNil(t, codex)
	require.Equal(t, provider.ProfileOpenAICodex, codex.Name())
	require.Equal(t, provider.ProxyModeNativeResponses, codex.ProxyMode(nil, "gpt-5.5"))

	mimo := registry.Best(&model.Channel{Id: 2, Type: constant.ChannelTypeOpenAI, Name: "xiaomi-mimo"}, "mimo-v1")
	require.NotNil(t, mimo)
	require.Equal(t, provider.ProfileMiMoCodexChat, mimo.Name())
	require.Equal(t, provider.ProxyModeResponsesViaChat, mimo.ProxyMode(nil, "mimo-v1"))

	deepseek := registry.Best(&model.Channel{Id: 3, Type: constant.ChannelTypeDeepSeek, Name: "deepseek"}, "deepseek-v4-pro")
	require.NotNil(t, deepseek)
	require.Equal(t, provider.ProfileDeepSeekV4ProCodexChat, deepseek.Name())
	require.Equal(t, provider.ProxyModeResponsesViaChat, deepseek.ProxyMode(nil, "deepseek-v4-pro"))
}

func TestProviderProfileExplicitSettingOverridesInference(t *testing.T) {
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		ProviderProfile: provider.ProfileMiMoCodexChat,
		ProxyProfile:    provider.ProxyModeResponsesViaChat,
	})
	require.NoError(t, err)
	registry := provider.NewStandardProviderRegistry()

	profile := registry.Best(&model.Channel{
		Id:            4,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "custom-upstream",
		OtherSettings: string(settingsBytes),
	}, "unbranded-model")

	require.NotNil(t, profile)
	require.Equal(t, provider.ProfileMiMoCodexChat, profile.Name())
	require.Equal(t, provider.ProxyModeResponsesViaChat, profile.ProxyMode(&model.Channel{OtherSettings: string(settingsBytes)}, "unbranded-model"))
}
