package integration_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/stretchr/testify/require"
)

func TestModelCandidatePoolBuilderUsesProviderRegistry(t *testing.T) {
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		ProviderProfile: provider.ProfileDeepSeekV4ProCodexChat,
		ProxyProfile:    provider.ProxyModeResponsesViaChat,
	})
	require.NoError(t, err)
	builder := integration.NewModelCandidatePoolBuilder()
	profile := builder.ProviderRegistry.Best(&model.Channel{
		Id:            9,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "compat-deepseek",
		OtherSettings: string(settingsBytes),
	}, "deepseek-v4-pro")

	require.NotNil(t, profile)
	require.Equal(t, provider.ProfileDeepSeekV4ProCodexChat, profile.Name())
	capability := profile.Capabilities(&model.Channel{OtherSettings: string(settingsBytes)}, "deepseek-v4-pro")
	require.NotEmpty(t, capability.CapabilityFingerprint)
	require.True(t, capability.ResponsesViaChat)
}
