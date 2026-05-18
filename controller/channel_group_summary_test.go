package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/require"
)

func TestBuildChannelGroupSummaryAggregatesCachedAbilities(t *testing.T) {
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		CodexCompatibilityMode:            true,
		CodexImageGenerationToolSupported: true,
	})
	require.NoError(t, err)

	codexToolsBytes, err := common.Marshal([]string{dto.BuildInToolImageGeneration})
	require.NoError(t, err)
	responseEndpointsBytes, err := common.Marshal([]constant.EndpointType{
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
	})
	require.NoError(t, err)
	imageEndpointsBytes, err := common.Marshal([]constant.EndpointType{
		constant.EndpointTypeImageGeneration,
		constant.EndpointTypeImageEdit,
	})
	require.NoError(t, err)

	priority := int64(10)
	weight := uint(100)
	channels := []*model.Channel{
		{
			Id:            1,
			Type:          constant.ChannelTypeOpenAI,
			Status:        common.ChannelStatusEnabled,
			Group:         "codex-pro,test",
			Models:        "gpt-5.5,gpt-image-2",
			Priority:      &priority,
			Weight:        &weight,
			OtherSettings: string(settingsBytes),
		},
		{
			Id:     2,
			Type:   constant.ChannelTypeOpenAI,
			Status: common.ChannelStatusAutoDisabled,
			Group:  "codex-pro",
			Models: "gpt-4.1",
		},
	}
	abilities := []model.Ability{
		{
			Group:                             "codex-pro",
			Model:                             "gpt-5.5",
			ChannelId:                         1,
			Enabled:                           true,
			SupportedEndpointTypes:            string(responseEndpointsBytes),
			CodexImageGenerationToolSupported: true,
			CodexSupportedTools:               string(codexToolsBytes),
		},
		{
			Group:                  "codex-pro",
			Model:                  "gpt-image-2",
			ChannelId:              1,
			Enabled:                true,
			SupportedEndpointTypes: string(imageEndpointsBytes),
		},
		{
			Group:     "codex-pro",
			Model:     "gpt-4.1",
			ChannelId: 2,
			Enabled:   false,
		},
		{
			Group:                             "test",
			Model:                             "gpt-5.5",
			ChannelId:                         1,
			Enabled:                           true,
			SupportedEndpointTypes:            string(responseEndpointsBytes),
			CodexImageGenerationToolSupported: true,
			CodexSupportedTools:               string(codexToolsBytes),
		},
	}

	resp := buildChannelGroupSummary(channels, abilities)
	require.Equal(t, 2, resp.Summary.TotalGroups)
	require.Equal(t, 2, resp.Summary.TotalChannels)
	require.Equal(t, 1, resp.Summary.EnabledChannels)
	require.Equal(t, 1, resp.Summary.DisabledChannels)
	require.Equal(t, 1, resp.Summary.AutoDisabledChannels)
	require.Equal(t, 3, resp.Summary.TotalModels)
	require.Equal(t, 2, resp.Summary.EnabledModels)

	codexGroup := findChannelGroupSummaryItem(t, resp.Groups, "codex-pro")
	require.Equal(t, 2, codexGroup.TotalChannels)
	require.Equal(t, 1, codexGroup.EnabledChannels)
	require.Equal(t, 1, codexGroup.DisabledChannels)
	require.Equal(t, 1, codexGroup.AutoDisabledChannels)
	require.Equal(t, 3, codexGroup.TotalModels)
	require.Equal(t, 2, codexGroup.EnabledModels)
	require.Contains(t, codexGroup.Capabilities, "responses")
	require.Contains(t, codexGroup.Capabilities, "compact")
	require.Contains(t, codexGroup.Capabilities, "image_api")
	require.Contains(t, codexGroup.Capabilities, "codex_tool:image_generation")
	require.Contains(t, codexGroup.CodexSupportedTools, dto.BuildInToolImageGeneration)
	require.Equal(t, 1, codexGroup.TypeCounts[constant.ChannelTypeOpenAI])

	testGroup := findChannelGroupSummaryItem(t, resp.Groups, "test")
	require.Equal(t, 1, testGroup.TotalChannels)
	require.Equal(t, 1, testGroup.EnabledChannels)
	require.Equal(t, 1, testGroup.TotalModels)
	require.Contains(t, testGroup.Capabilities, "codex_tool:image_generation")
	require.Contains(t, testGroup.SampleModels, "gpt-5.5")
}

func findChannelGroupSummaryItem(t *testing.T, groups []channelGroupSummaryItem, group string) channelGroupSummaryItem {
	t.Helper()
	for _, item := range groups {
		if item.Group == group {
			return item
		}
	}
	t.Fatalf("group %q not found in %#v", group, groups)
	return channelGroupSummaryItem{}
}
