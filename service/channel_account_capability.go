package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewayprovider "github.com/QuantumNous/new-api/pkg/modelgateway/provider"
)

func MarkChannelAccountStreamOptionsCapability(channelID int, credentialIndex int, supported bool, message string) (bool, error) {
	return updateChannelAccountCapability(channelID, credentialIndex, func(capability model.ChannelAccountCapability, now int64) (model.ChannelAccountCapability, bool) {
		if capability.StreamOptions != nil && *capability.StreamOptions == supported {
			return capability, false
		}
		capability.StreamOptions = common.GetPointer(supported)
		capability.CheckedTime = now
		capability.LastEndpoint = "stream_options"
		capability.LastMessage = truncateCapabilityMessage(message, 360)
		if !supported {
			capability.CapabilityClassification = channelcapability.ClassificationUnsupportedCapability
		}
		return capability, true
	})
}

func MarkChannelAccountResponsesPreviousIDCapability(channelID int, credentialIndex int, supported bool, message string) (bool, error) {
	return updateChannelAccountCapability(channelID, credentialIndex, func(capability model.ChannelAccountCapability, now int64) (model.ChannelAccountCapability, bool) {
		if capability.ResponsesPreviousID != nil && *capability.ResponsesPreviousID == supported {
			return capability, false
		}
		capability.ResponsesPreviousID = common.GetPointer(supported)
		capability.CheckedTime = now
		capability.LastEndpoint = "previous_response_id"
		capability.LastMessage = truncateCapabilityMessage(message, 360)
		if !supported {
			capability.CapabilityClassification = channelcapability.ClassificationUnsupportedCapability
		}
		return capability, true
	})
}

func MarkChannelAccountCodexImageGenerationToolCapability(channelID int, credentialIndex int, supported bool, message string) (bool, error) {
	return updateChannelAccountCapability(channelID, credentialIndex, func(capability model.ChannelAccountCapability, now int64) (model.ChannelAccountCapability, bool) {
		if capability.CodexImageGenerationTool != nil && *capability.CodexImageGenerationTool == supported {
			return capability, false
		}
		capability.CodexImageGenerationTool = common.GetPointer(supported)
		capability.CheckedTime = now
		capability.LastEndpoint = "codex_image_generation_tool"
		capability.LastMessage = truncateCapabilityMessage(message, 360)
		if !supported {
			capability.CapabilityClassification = channelcapability.ClassificationUnsupportedCapability
		}
		return capability, true
	})
}

func MarkChannelAccountAuthErrorCandidate(channelID int, credentialIndex int, message string) (bool, error) {
	return updateChannelAccountCapability(channelID, credentialIndex, func(capability model.ChannelAccountCapability, now int64) (model.ChannelAccountCapability, bool) {
		if capability.CapabilityClassification == channelcapability.ClassificationAuthError &&
			strings.TrimSpace(capability.LastMessage) == strings.TrimSpace(message) {
			return capability, false
		}
		capability.CheckedTime = now
		capability.CapabilityClassification = channelcapability.ClassificationAuthError
		capability.LastEndpoint = "auth"
		capability.LastMessage = truncateCapabilityMessage(message, 360)
		return capability, true
	})
}

func updateChannelAccountCapability(channelID int, credentialIndex int, mutate func(model.ChannelAccountCapability, int64) (model.ChannelAccountCapability, bool)) (bool, error) {
	if channelID <= 0 || credentialIndex < 0 || mutate == nil {
		return false, nil
	}
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return false, err
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return false, errors.New("账号索引超出范围")
	}
	if channel.ChannelInfo.MultiKeyCapabilities == nil {
		channel.ChannelInfo.MultiKeyCapabilities = make(map[int]model.ChannelAccountCapability)
	}
	capability := channel.ChannelInfo.MultiKeyCapabilities[credentialIndex]
	next, changed := mutate(capability, common.GetTimestamp())
	if !changed {
		return false, nil
	}
	channel.ChannelInfo.MultiKeyCapabilities[credentialIndex] = next
	if err := channel.SaveChannelInfo(); err != nil {
		return false, err
	}
	model.InitChannelCache()
	return true, nil
}

func ChannelAccountCapabilitySupportsResponsesPreviousID(capability model.ChannelAccountCapability) bool {
	return !capability.HasResponsesPreviousIDDenied()
}

func ChannelSupportsResponsesPreviousID(channel *model.Channel, proxyMode string, capability *model.ChannelAccountCapability) bool {
	if capability != nil {
		if capability.HasResponsesPreviousIDDenied() {
			return false
		}
		if capability.HasResponsesPreviousIDAllowed() {
			return true
		}
	}
	if channel == nil {
		return false
	}
	settings := channel.GetOtherSettings()
	if settings.SupportResponsesPreviousID != nil {
		return *settings.SupportResponsesPreviousID
	}
	if strings.TrimSpace(proxyMode) == modelgatewayprovider.ProxyModeResponsesViaChat {
		return false
	}
	return false
}

func ChannelDefaultSupportsStreamOptions(channelType int, settings dto.ChannelOtherSettings) bool {
	if settings.SupportStreamOptions != nil {
		return *settings.SupportStreamOptions
	}
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeAnthropic,
		constant.ChannelTypeAws,
		constant.ChannelTypeGemini,
		constant.ChannelCloudflare,
		constant.ChannelTypeAzure,
		constant.ChannelTypeVolcEngine,
		constant.ChannelTypeOllama,
		constant.ChannelTypeXai,
		constant.ChannelTypeDeepSeek,
		constant.ChannelTypeBaiduV2,
		constant.ChannelTypeZhipu_v4,
		constant.ChannelTypeAli,
		constant.ChannelTypeSubmodel,
		constant.ChannelTypeCodex,
		constant.ChannelTypeMoonshot,
		constant.ChannelTypeMiniMax,
		constant.ChannelTypeSiliconFlow:
		return true
	default:
		return false
	}
}
