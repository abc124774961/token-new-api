package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	channelCostProfileSourceSystemRatio = "system_ratio"
	defaultChannelCostCurrency          = "USD"
	defaultChannelCostModel             = "*"
	defaultChannelCostPricingMode       = "token"
	defaultChannelCostAccuracy          = "estimated"
	maxToolPricesJSONLength             = 8192
)

func ListChannelUpstreamCostProfiles(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	if !channelExists(channelID) {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}

	profiles := make([]model.ModelGatewayChannelCostProfile, 0)
	if err := model.DB.
		Where("channel_id = ? AND upstream_model = ?", channelID, defaultChannelCostModel).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "updated_at"}, Desc: true}).
		Limit(1).
		Find(&profiles).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, profiles)
}

func GetChannelUpstreamCostQuote(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}

	models := make([]string, 0)
	for _, modelName := range channel.GetModels() {
		models = append(models, modelName)
	}
	models = normalizeCostQuoteModels(models)
	baseProfile, err := findChannelDefaultCostProfile(channelID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}
	if baseProfile.Id == 0 {
		baseProfile = defaultChannelCostProfile(channelID)
	}
	quoteProfile := baseProfile
	quoteProfile.TokenMultiplier = 1
	quoteProfile.InputCostMultiplier = 1
	quoteProfile.OutputCostMultiplier = 1
	quoteProfile.CacheReadMultiplier = 1
	quoteProfile.CacheWriteMultiplier = 1
	quoteProfile.RequestCostMultiplier = 1
	quoteProfile.RechargeMultiplier = 1
	quotes := make(map[string]modelgatewaycost.SystemRatioQuote, len(models))
	for _, requestedModel := range models {
		pricingModel := channel.ResolveMappedModelName(requestedModel)
		quote := modelgatewaycost.QuoteSystemRatioProfile(pricingModel, quoteProfile)
		quote.Model = requestedModel
		quote.PricingModel = pricingModel
		quotes[requestedModel] = quote
	}

	common.ApiSuccess(c, gin.H{
		"models":          models,
		"quotes":          quotes,
		"default_profile": baseProfile,
	})
}

type unsavedChannelUpstreamCostQuoteRequest struct {
	Models       []string `json:"models"`
	ModelMapping string   `json:"model_mapping"`
}

func GetUnsavedChannelUpstreamCostQuote(c *gin.Context) {
	var req unsavedChannelUpstreamCostQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	models := normalizeCostQuoteModels(req.Models)
	baseProfile := defaultChannelCostProfile(0)
	quoteProfile := baseProfile
	quotes := make(map[string]modelgatewaycost.SystemRatioQuote, len(models))
	for _, requestedModel := range models {
		pricingModel := resolveUnsavedQuotePricingModel(requestedModel, req.ModelMapping)
		quote := modelgatewaycost.QuoteSystemRatioProfile(pricingModel, quoteProfile)
		quote.Model = requestedModel
		quote.PricingModel = pricingModel
		quotes[requestedModel] = quote
	}
	common.ApiSuccess(c, gin.H{
		"models":          models,
		"quotes":          quotes,
		"default_profile": baseProfile,
	})
}

func SaveChannelUpstreamCostProfile(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	if !channelExists(channelID) {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}

	var req model.ModelGatewayChannelCostProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	profile, err := normalizeChannelCostProfile(channelID, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := common.GetTimestamp()
	existing, err := findChannelDefaultCostProfile(channelID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}
	if existing.Id > 0 {
		profile.Id = existing.Id
		profile.CreatedAt = existing.CreatedAt
		profile.UpdatedAt = now
		if err := model.DB.Save(&profile).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		profile.CreatedAt = now
		profile.UpdatedAt = now
		if err := model.DB.Create(&profile).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	}

	modelgatewaycost.InvalidateDefaultProfileCache()

	var saved model.ModelGatewayChannelCostProfile
	if err := model.DB.First(&saved, "id = ?", profile.Id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, saved)
}

func DeleteChannelUpstreamCostProfile(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	profileID, err := strconv.Atoi(c.Param("profile_id"))
	if err != nil || profileID <= 0 {
		common.ApiError(c, fmt.Errorf("成本规则 ID 无效"))
		return
	}

	tx := model.DB.Where("id = ? AND channel_id = ?", profileID, channelID).Delete(&model.ModelGatewayChannelCostProfile{})
	if tx.Error != nil {
		common.ApiError(c, tx.Error)
		return
	}
	if tx.RowsAffected == 0 {
		common.ApiErrorMsg(c, "成本规则不存在")
		return
	}
	modelgatewaycost.InvalidateDefaultProfileCache()
	common.ApiSuccess(c, nil)
}

func parseChannelIDParam(c *gin.Context) (int, bool) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiError(c, fmt.Errorf("渠道 ID 无效"))
		return 0, false
	}
	return channelID, true
}

func channelExists(channelID int) bool {
	var count int64
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func findChannelDefaultCostProfile(channelID int) (model.ModelGatewayChannelCostProfile, error) {
	var profile model.ModelGatewayChannelCostProfile
	err := model.DB.
		Where("channel_id = ? AND upstream_model = ?", channelID, defaultChannelCostModel).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "updated_at"}, Desc: true}).
		First(&profile).Error
	return profile, err
}

func defaultChannelCostProfile(channelID int) model.ModelGatewayChannelCostProfile {
	return model.ModelGatewayChannelCostProfile{
		ChannelID:             channelID,
		UpstreamModel:         defaultChannelCostModel,
		Currency:              defaultChannelCostCurrency,
		PricingMode:           defaultChannelCostPricingMode,
		Source:                channelCostProfileSourceSystemRatio,
		Accuracy:              defaultChannelCostAccuracy,
		TokenMultiplier:       1,
		InputCostMultiplier:   1,
		OutputCostMultiplier:  1,
		CacheReadMultiplier:   1,
		CacheWriteMultiplier:  1,
		RequestCostMultiplier: 1,
		RechargeMultiplier:    1,
		Version:               1,
	}
}

func normalizeCostQuoteModels(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || seen[modelName] {
			continue
		}
		seen[modelName] = true
		normalized = append(normalized, modelName)
	}
	return normalized
}

func resolveUnsavedQuotePricingModel(modelName string, modelMapping string) string {
	channel := &model.Channel{ModelMapping: &modelMapping}
	return channel.ResolveMappedModelName(modelName)
}

func normalizeChannelCostProfile(channelID int, req model.ModelGatewayChannelCostProfile) (model.ModelGatewayChannelCostProfile, error) {
	profile := req
	profile.ChannelID = channelID
	profile.UpstreamModel = defaultChannelCostModel
	profile.PricingMode = defaultChannelCostPricingMode
	profile.Accuracy = defaultChannelCostAccuracy
	profile.EffectiveTime = 0
	profile.Version = 1
	profile.Currency = defaultChannelCostCurrency
	profile.Source = channelCostProfileSourceSystemRatio
	profile.SourceURL = ""
	profile.SyncedAt = 0
	if profile.TokenMultiplier < 0 || profile.RechargeMultiplier < 0 {
		return profile, fmt.Errorf("倍率必须是非负数")
	}
	if profile.TokenMultiplier <= 0 {
		if profile.InputCostMultiplier < 0 {
			return profile, fmt.Errorf("倍率必须是非负数")
		}
		profile.TokenMultiplier = profile.InputCostMultiplier
	}
	if profile.TokenMultiplier <= 0 {
		profile.TokenMultiplier = 1
	}
	profile.InputCostMultiplier = profile.TokenMultiplier
	profile.OutputCostMultiplier = profile.TokenMultiplier
	profile.CacheReadMultiplier = profile.TokenMultiplier
	profile.CacheWriteMultiplier = profile.TokenMultiplier
	profile.RequestCostMultiplier = 1
	if profile.RechargeMultiplier <= 0 {
		profile.RechargeMultiplier = 1
	}
	profile.InputPerMillion = 0
	profile.OutputPerMillion = 0
	profile.CacheReadPerMillion = 0
	profile.CacheWritePerMillion = 0
	profile.CacheWrite5mPerMillion = 0
	profile.CacheWrite1hPerMillion = 0
	profile.ImageInputPerMillion = 0
	profile.ImageOutputPerMillion = 0
	profile.AudioInputPerMillion = 0
	profile.AudioOutputPerMillion = 0
	profile.ToolPricesJSON = ""
	profile.MetadataJSON = strings.TrimSpace(profile.MetadataJSON)
	if len(profile.MetadataJSON) > maxToolPricesJSONLength {
		return profile, fmt.Errorf("成本规则元数据 JSON 过长")
	}
	if profile.MetadataJSON != "" {
		var parsed map[string]interface{}
		if err := common.UnmarshalJsonStr(profile.MetadataJSON, &parsed); err != nil {
			return profile, fmt.Errorf("成本规则元数据 JSON 格式错误: %w", err)
		}
		if parsed == nil {
			return profile, fmt.Errorf("成本规则元数据 JSON 必须是对象")
		}
	}

	if profile.InputPerMillion < 0 ||
		profile.OutputPerMillion < 0 ||
		profile.CacheReadPerMillion < 0 ||
		profile.CacheWritePerMillion < 0 ||
		profile.CacheWrite5mPerMillion < 0 ||
		profile.CacheWrite1hPerMillion < 0 ||
		profile.ImageInputPerMillion < 0 ||
		profile.ImageOutputPerMillion < 0 ||
		profile.AudioInputPerMillion < 0 ||
		profile.AudioOutputPerMillion < 0 ||
		profile.RequestPrice < 0 {
		return profile, fmt.Errorf("成本价格必须是非负数")
	}
	return profile, nil
}
