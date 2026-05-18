package model

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`

	SupportedEndpointTypes            string `json:"supported_endpoint_types,omitempty" gorm:"type:text"`
	CodexImageGenerationToolSupported bool   `json:"codex_image_generation_tool_supported,omitempty" gorm:"default:false"`
	CodexSupportedTools               string `json:"codex_supported_tools,omitempty" gorm:"type:text"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType          int    `json:"channel_type"`
	ChannelOtherSettings string `json:"channel_other_settings" gorm:"column:channel_other_settings"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type, channels.settings as channel_other_settings").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func buildAbilityForChannel(channel *Channel, group string, modelName string) Ability {
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	settings := channel.GetOtherSettings()
	endpointTypes := channelcapability.SupportedEndpointTypes(channel.Type, modelName, settings)
	endpointBytes, err := common.Marshal(endpointTypes)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal ability endpoint types: channel_id=%d, model=%s, error=%v", channel.Id, modelName, err))
	}

	codexTools := make([]string, 0)
	codexImageToolSupported := channelcapability.SupportsCodexImageGenerationTool(channel.Type, settings)
	if codexImageToolSupported {
		codexTools = append(codexTools, dto.BuildInToolImageGeneration)
	}
	codexToolBytes, err := common.Marshal(codexTools)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal ability codex tools: channel_id=%d, model=%s, error=%v", channel.Id, modelName, err))
	}

	return Ability{
		Group:                             group,
		Model:                             modelName,
		ChannelId:                         channel.Id,
		Enabled:                           channel.Status == common.ChannelStatusEnabled,
		Priority:                          channel.Priority,
		Weight:                            uint(channel.GetWeight()),
		Tag:                               channel.Tag,
		SupportedEndpointTypes:            string(endpointBytes),
		CodexImageGenerationToolSupported: codexImageToolSupported,
		CodexSupportedTools:               string(codexToolBytes),
	}
}

func (ability Ability) GetSupportedEndpointTypes(channelType int, settings dto.ChannelOtherSettings) []constant.EndpointType {
	if strings.TrimSpace(ability.SupportedEndpointTypes) != "" {
		var endpoints []constant.EndpointType
		if err := common.UnmarshalJsonStr(ability.SupportedEndpointTypes, &endpoints); err == nil {
			return endpoints
		}
	}
	return channelcapability.SupportedEndpointTypes(channelType, ability.Model, settings)
}

func (ability Ability) SupportsCodexImageGenerationTool(channelType int, settings dto.ChannelOtherSettings) bool {
	if ability.CodexImageGenerationToolSupported {
		return true
	}
	if strings.TrimSpace(ability.CodexSupportedTools) != "" {
		var tools []string
		if err := common.UnmarshalJsonStr(ability.CodexSupportedTools, &tools); err == nil {
			return slices.Contains(tools, dto.BuildInToolImageGeneration)
		}
	}
	return channelcapability.SupportsCodexImageGenerationTool(channelType, settings)
}

func getMatchingAbilityModels(group string, model string, excludedChannelIDs map[int]struct{}) ([]string, error) {
	models := []string{model}
	normalized := ratio_setting.FormatMatchingModelName(model)
	if normalized == "" || normalized == model {
		return models, nil
	}

	var exactCount int64
	query := DB.Model(&Ability{}).Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true)
	if len(excludedChannelIDs) > 0 {
		query = query.Where("channel_id NOT IN ?", lo.Keys(excludedChannelIDs))
	}
	if err := query.Count(&exactCount).Error; err != nil {
		return nil, err
	}
	if exactCount > 0 {
		return models, nil
	}
	return []string{normalized}, nil
}

func getPriority(group string, modelNames []string, retry int) (*int, error) {
	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where(commonGroupCol+" = ? and model IN ? and enabled = ?", group, modelNames, true).
		Order("priority DESC").
		Pluck("priority", &priorities).Error
	if err != nil {
		return nil, err
	}
	if len(priorities) == 0 {
		return nil, nil
	}

	var priorityToUse int
	if retry >= len(priorities) {
		priorityToUse = priorities[len(priorities)-1]
	} else {
		priorityToUse = priorities[retry]
	}
	return &priorityToUse, nil
}

func getChannelQuery(group string, model string, retry int, excludedChannelIDs map[int]struct{}) (*gorm.DB, error) {
	modelNames, err := getMatchingAbilityModels(group, model, excludedChannelIDs)
	if err != nil {
		return nil, err
	}
	if len(modelNames) == 0 {
		return nil, nil
	}

	query := DB.Model(&Ability{}).Where(commonGroupCol+" = ? and model IN ? and enabled = ?", group, modelNames, true)
	if len(excludedChannelIDs) > 0 {
		query = query.Where("channel_id NOT IN ?", lo.Keys(excludedChannelIDs))
	}

	getMaxPriority := func(q *gorm.DB) (*int, error) {
		row := q.Session(&gorm.Session{}).Select("MAX(priority)").Row()
		if row == nil {
			return nil, nil
		}
		var maxPriority sql.NullInt64
		if err := row.Scan(&maxPriority); err != nil {
			return nil, err
		}
		if !maxPriority.Valid {
			return nil, nil
		}
		value := int(maxPriority.Int64)
		return &value, nil
	}

	var targetPriority int
	if len(excludedChannelIDs) > 0 {
		maxPriority, err := getMaxPriority(query)
		if err != nil {
			return nil, err
		}
		if maxPriority == nil {
			return nil, nil
		}
		targetPriority = *maxPriority
	} else if retry != 0 {
		priority, err := getPriority(group, modelNames, retry)
		if err != nil {
			return nil, err
		}
		if priority == nil {
			return nil, nil
		}
		targetPriority = *priority
	} else {
		maxPriority, err := getMaxPriority(query)
		if err != nil {
			return nil, err
		}
		if maxPriority == nil {
			return nil, nil
		}
		targetPriority = *maxPriority
	}

	return query.Where("priority = ?", targetPriority), nil
}

func GetChannel(group string, model string, retry int, excludedChannelIDs map[int]struct{}) (*Channel, error) {
	var abilities []Ability

	var err error = nil
	channelQuery, err := getChannelQuery(group, model, retry, excludedChannelIDs)
	if err != nil {
		return nil, err
	}
	if channelQuery == nil {
		return nil, nil
	}
	if common.UsingSQLite || common.UsingPostgreSQL {
		err = channelQuery.Order("weight DESC").Find(&abilities).Error
	} else {
		err = channelQuery.Order("weight DESC").Find(&abilities).Error
	}
	if err != nil {
		return nil, err
	}
	channel := Channel{}
	if len(abilities) > 0 {
		weightSum := uint(0)
		for _, ability_ := range abilities {
			weightSum += ability_.Weight + 10
		}
		weight := common.GetRandomInt(int(weightSum))
		for _, ability_ := range abilities {
			weight -= int(ability_.Weight) + 10
			if weight <= 0 {
				channel.Id = ability_.ChannelId
				break
			}
		}
	} else {
		return nil, nil
	}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		for _, group := range groups_ {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			abilities = append(abilities, buildAbilityForChannel(channel, group, model))
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	if tx == nil {
		InvalidatePricingCache()
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	err := DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		for _, group := range groups_ {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			abilities = append(abilities, buildAbilityForChannel(channel, group, model))
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		err = tx.Commit().Error
		if err == nil {
			InvalidatePricingCache()
		}
		return err
	}

	return nil
}

func (channel *Channel) UpdateAbilityCapabilities(tx *gorm.DB) error {
	return channel.updateAbilityCapabilities(tx, tx == nil)
}

func (channel *Channel) updateAbilityCapabilities(tx *gorm.DB, invalidate bool) error {
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	for _, modelName := range models_ {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		for _, group := range groups_ {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			ability := buildAbilityForChannel(channel, group, modelName)
			err := useDB.Model(&Ability{}).
				Where("channel_id = ? AND "+commonGroupCol+" = ? AND model = ?", channel.Id, group, modelName).
				Select("supported_endpoint_types", "codex_image_generation_tool_supported", "codex_supported_tools").
				Updates(ability).Error
			if err != nil {
				return err
			}
		}
	}
	if invalidate {
		InvalidatePricingCache()
	}
	return nil
}

func BackfillAbilityCapabilities() error {
	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return err
	}
	updated := false
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if err := channel.updateAbilityCapabilities(nil, false); err != nil {
			return err
		}
		updated = true
	}
	if updated {
		InvalidatePricingCache()
	}
	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	err := DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	err := DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	err := DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
