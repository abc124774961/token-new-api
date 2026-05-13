package model

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, modelName, channelID, true).
		Count(&count).Error
	if err == nil && count > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	count = 0
	err = DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, normalized, channelID, true).
		Count(&count).Error
	return err == nil && count > 0
}

func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}

func GetGroupAvailableModels(group string) []string {
	if group == "" {
		return []string{}
	}
	if !common.MemoryCacheEnabled {
		return getAvailableModelsFromChannels([]string{group})
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	model2channels, ok := group2model2channels[group]
	if !ok || len(model2channels) == 0 {
		return []string{}
	}

	models := make([]string, 0, len(model2channels))
	for modelName, channelIDs := range model2channels {
		if len(channelIDs) == 0 {
			continue
		}
		models = append(models, modelName)
	}
	sort.Strings(models)
	return models
}

func GetAvailableModelsForGroups(groups []string) []string {
	if len(groups) == 0 {
		return []string{}
	}
	if !common.MemoryCacheEnabled {
		return getAvailableModelsFromChannels(groups)
	}

	modelSet := make(map[string]struct{})
	for _, group := range groups {
		for _, modelName := range GetGroupAvailableModels(group) {
			modelSet[modelName] = struct{}{}
		}
	}

	models := make([]string, 0, len(modelSet))
	for modelName := range modelSet {
		models = append(models, modelName)
	}
	sort.Strings(models)
	return models
}

func getAvailableModelsFromChannels(groups []string) []string {
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		groupSet[group] = struct{}{}
	}
	if len(groupSet) == 0 {
		return []string{}
	}

	var channels []*Channel
	err := DB.Select("status", commonGroupCol, "models").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&channels).Error
	if err != nil {
		return []string{}
	}

	modelSet := make(map[string]struct{})
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		matched := false
		for _, channelGroup := range strings.Split(channel.Group, ",") {
			if _, ok := groupSet[strings.TrimSpace(channelGroup)]; ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, modelName := range strings.Split(channel.Models, ",") {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			modelSet[modelName] = struct{}{}
		}
	}

	models := make([]string, 0, len(modelSet))
	for modelName := range modelSet {
		models = append(models, modelName)
	}
	sort.Strings(models)
	return models
}
