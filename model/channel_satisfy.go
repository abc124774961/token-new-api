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

type actualModelReturnRow struct {
	Model        string  `gorm:"column:model"`
	ModelMapping *string `gorm:"column:model_mapping"`
}

// GetPreferredActualModelsForGroups returns the upstream model advertised for each
// visible model, using the first available group and highest-priority enabled channel.
func GetPreferredActualModelsForGroups(groups []string, modelNames []string) map[string]string {
	result := make(map[string]string)
	if DB == nil || len(groups) == 0 || len(modelNames) == 0 {
		return result
	}

	modelSet := make(map[string]struct{}, len(modelNames))
	missingModels := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := modelSet[modelName]; ok {
			continue
		}
		modelSet[modelName] = struct{}{}
		missingModels = append(missingModels, modelName)
	}
	if len(missingModels) == 0 {
		return result
	}

	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || len(missingModels) == 0 {
			continue
		}

		groupResult := getPreferredActualModelsForGroup(group, missingModels)
		nextMissing := missingModels[:0]
		for _, modelName := range missingModels {
			if actualModel, ok := groupResult[modelName]; ok {
				result[modelName] = actualModel
			} else {
				nextMissing = append(nextMissing, modelName)
			}
		}
		missingModels = nextMissing
	}

	return result
}

func getPreferredActualModelsForGroup(group string, modelNames []string) map[string]string {
	result := make(map[string]string)
	rows := make([]actualModelReturnRow, 0)
	err := DB.Table("abilities").
		Select("abilities.model, channels.model_mapping").
		Joins("JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Where("channels.status = ?", common.ChannelStatusEnabled).
		Where("abilities."+commonGroupCol+" = ?", group).
		Where("abilities.model IN ?", modelNames).
		Order("abilities.priority DESC, abilities.weight DESC, abilities.channel_id ASC").
		Scan(&rows).Error
	if err != nil {
		common.SysLog("get preferred actual models failed: " + err.Error())
		return result
	}

	for _, row := range rows {
		if row.Model == "" {
			continue
		}
		if _, ok := result[row.Model]; ok {
			continue
		}
		modelMapping := ""
		if row.ModelMapping != nil {
			modelMapping = *row.ModelMapping
		}
		result[row.Model] = resolveMappedModelName(row.Model, modelMapping)
	}
	return result
}

func resolveMappedModelName(modelName string, modelMapping string) string {
	modelMapping = strings.TrimSpace(modelMapping)
	if modelMapping == "" || modelMapping == "{}" {
		return modelName
	}

	modelMap := make(map[string]string)
	if err := common.UnmarshalJsonStr(modelMapping, &modelMap); err != nil {
		return modelName
	}

	currentModel := modelName
	visitedModels := map[string]bool{
		currentModel: true,
	}
	for {
		mappedModel := strings.TrimSpace(modelMap[currentModel])
		if mappedModel == "" {
			return currentModel
		}
		if visitedModels[mappedModel] {
			return currentModel
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
	}
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

	groupNames := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groupNames = append(groupNames, group)
	}

	var models []string
	err := DB.Table("abilities").
		Select("DISTINCT abilities.model").
		Joins("LEFT JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Where("abilities."+commonGroupCol+" IN ?", groupNames).
		Where("(channels.id IS NOT NULL AND channels.status = ?) OR NOT EXISTS (SELECT 1 FROM channels)", common.ChannelStatusEnabled).
		Pluck("model", &models).Error
	if err != nil {
		return []string{}
	}
	sort.Strings(models)
	return models
}
