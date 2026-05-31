package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
		Where("channel_id = ?", channelID).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "upstream_model"}, Desc: false}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "updated_at"}, Desc: true}).
		Find(&profiles).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	sortChannelCostProfiles(profiles)
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
	quoteProfile.CostCoefficient = 1
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
	existing, err := findExistingChannelCostProfileForSave(channelID, profile)
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
	modelgatewaycost.StoreCachedDefaultProfile(saved)
	common.ApiSuccess(c, saved)
}

type channelUpstreamCostRecalculateRequest struct {
	UpstreamModel  string `json:"upstream_model"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	Limit          int    `json:"limit"`
}

type channelUpstreamCostRecalculateResponse struct {
	ChannelID          int     `json:"channel_id"`
	UpstreamModel      string  `json:"upstream_model,omitempty"`
	StartTimestamp     int64   `json:"start_timestamp"`
	EndTimestamp       int64   `json:"end_timestamp"`
	Scanned            int     `json:"scanned"`
	Recalculated       int     `json:"recalculated"`
	Skipped            int     `json:"skipped"`
	BeforeCostUSD      float64 `json:"before_cost_usd"`
	AfterCostUSD       float64 `json:"after_cost_usd"`
	LastRequestID      string  `json:"last_request_id,omitempty"`
	LastCostSource     string  `json:"last_cost_source,omitempty"`
	LastCostAccuracy   string  `json:"last_cost_accuracy,omitempty"`
	Limit              int     `json:"limit"`
	HasMorePotentially bool    `json:"has_more_potentially"`
}

func RecalculateChannelUpstreamCost(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	if !channelExists(channelID) {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	if model.LOG_DB == nil {
		common.ApiErrorMsg(c, "日志数据库未初始化")
		return
	}
	var req channelUpstreamCostRecalculateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	now := common.GetTimestamp()
	if req.EndTimestamp <= 0 {
		req.EndTimestamp = now
	}
	if req.StartTimestamp <= 0 {
		req.StartTimestamp = req.EndTimestamp - 24*60*60
	}
	if req.StartTimestamp > req.EndTimestamp {
		common.ApiErrorMsg(c, "开始时间不能晚于结束时间")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 1000
	}
	if req.Limit > 5000 {
		req.Limit = 5000
	}
	upstreamModel := strings.TrimSpace(req.UpstreamModel)
	logs := make([]model.Log, 0, req.Limit)
	err := model.LOG_DB.
		Where("type = ? AND channel_id = ? AND request_id <> '' AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, channelID, req.StartTimestamp, req.EndTimestamp).
		Order("id asc").
		Limit(req.Limit).
		Find(&logs).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	requestIDs := make([]string, 0, len(logs))
	seenRequestIDs := make(map[string]bool, len(logs))
	for _, row := range logs {
		requestID := strings.TrimSpace(row.RequestId)
		if requestID == "" || seenRequestIDs[requestID] {
			continue
		}
		seenRequestIDs[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	beforeByRequestID := make(map[string]float64, len(requestIDs))
	if len(requestIDs) > 0 {
		summaries := make([]model.ModelGatewayRequestCostSummary, 0, len(requestIDs))
		if err := model.DB.
			Select("request_id, upstream_cost_total").
			Where("request_id IN ?", requestIDs).
			Find(&summaries).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		for _, summary := range summaries {
			beforeByRequestID[strings.TrimSpace(summary.RequestId)] = summary.UpstreamCostTotal
		}
	}

	response := channelUpstreamCostRecalculateResponse{
		ChannelID:          channelID,
		UpstreamModel:      upstreamModel,
		StartTimestamp:     req.StartTimestamp,
		EndTimestamp:       req.EndTimestamp,
		Limit:              req.Limit,
		HasMorePotentially: len(logs) >= req.Limit,
	}
	for _, row := range logs {
		response.Scanned++
		usage := modelgatewaycost.UsageSnapshotFromLog(row)
		if !channelCostRecalculateModelMatches(upstreamModel, usage.UpstreamModel, row.ModelName) {
			response.Skipped++
			continue
		}
		beforeCost := beforeByRequestID[strings.TrimSpace(row.RequestId)]
		summary, err := modelgatewaycost.RecalculateUsageSnapshot(context.Background(), usage)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		response.Recalculated++
		response.BeforeCostUSD += beforeCost
		response.AfterCostUSD += summary.UpstreamCostTotal
		response.LastRequestID = summary.RequestId
		response.LastCostSource = summary.CostSource
		response.LastCostAccuracy = summary.CostAccuracy
	}
	common.ApiSuccess(c, response)
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
	modelgatewaycost.RemoveCachedDefaultProfilesForChannel(channelID)
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
	return findChannelCostProfile(channelID, defaultChannelCostModel)
}

func findExistingChannelCostProfileForSave(channelID int, profile model.ModelGatewayChannelCostProfile) (model.ModelGatewayChannelCostProfile, error) {
	if profile.Id > 0 {
		var existing model.ModelGatewayChannelCostProfile
		err := model.DB.Where("id = ? AND channel_id = ?", profile.Id, channelID).First(&existing).Error
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return existing, err
		}
	}
	return findChannelCostProfile(channelID, profile.UpstreamModel)
}

func findChannelCostProfile(channelID int, upstreamModel string) (model.ModelGatewayChannelCostProfile, error) {
	var profile model.ModelGatewayChannelCostProfile
	upstreamModel = normalizeChannelCostProfileModel(upstreamModel)
	err := model.DB.
		Where("channel_id = ? AND upstream_model = ?", channelID, upstreamModel).
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
		CostCoefficient:       1,
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
	profile.UpstreamModel = normalizeChannelCostProfileModel(req.UpstreamModel)
	profile.PricingMode = normalizeChannelCostProfilePricingMode(req.PricingMode)
	profile.Accuracy = normalizeChannelCostProfileAccuracy(req.Accuracy)
	profile.EffectiveTime = 0
	profile.Version = 1
	profile.Currency = defaultChannelCostCurrency
	profile.Source = normalizeChannelCostProfileSource(req.Source)
	profile.SourceURL = ""
	profile.SyncedAt = 0
	if profile.Source == "" {
		if profile.UpstreamModel != defaultChannelCostModel && profile.PricingMode == "request" {
			profile.Source = modelgatewaycost.SourceManual
		} else {
			profile.Source = channelCostProfileSourceSystemRatio
		}
	}
	if profile.UpstreamModel != defaultChannelCostModel && profile.PricingMode == "request" {
		profile.Source = modelgatewaycost.SourceManual
		profile.Accuracy = modelgatewaycost.AccuracyPrecise
	}
	if profile.CostCoefficient < 0 || profile.TokenMultiplier < 0 || profile.RechargeMultiplier < 0 {
		return profile, fmt.Errorf("倍率必须是非负数")
	}
	if profile.CostCoefficient <= 0 {
		profile.CostCoefficient = 1
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
	profile.ToolPricesJSON = strings.TrimSpace(profile.ToolPricesJSON)
	if len(profile.ToolPricesJSON) > maxToolPricesJSONLength {
		return profile, fmt.Errorf("工具成本 JSON 过长")
	}
	if profile.ToolPricesJSON != "" {
		var parsed map[string]float64
		if err := common.UnmarshalJsonStr(profile.ToolPricesJSON, &parsed); err != nil {
			return profile, fmt.Errorf("工具成本 JSON 格式错误: %w", err)
		}
	}
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
	if profile.Source == channelCostProfileSourceSystemRatio {
		profile.PricingMode = defaultChannelCostPricingMode
		profile.Accuracy = defaultChannelCostAccuracy
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
	}
	return profile, nil
}

func normalizeChannelCostProfileModel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultChannelCostModel
	}
	return value
}

func normalizeChannelCostProfilePricingMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "request":
		return "request"
	default:
		return defaultChannelCostPricingMode
	}
}

func normalizeChannelCostProfileSource(value string) string {
	switch strings.TrimSpace(value) {
	case modelgatewaycost.SourceManual:
		return modelgatewaycost.SourceManual
	case modelgatewaycost.SourceAutoSynced:
		return modelgatewaycost.SourceAutoSynced
	case channelCostProfileSourceSystemRatio:
		return channelCostProfileSourceSystemRatio
	default:
		return ""
	}
}

func normalizeChannelCostProfileAccuracy(value string) string {
	switch strings.TrimSpace(value) {
	case modelgatewaycost.AccuracyPrecise:
		return modelgatewaycost.AccuracyPrecise
	case "estimated":
		return "estimated"
	default:
		return defaultChannelCostAccuracy
	}
}

func sortChannelCostProfiles(profiles []model.ModelGatewayChannelCostProfile) {
	sort.SliceStable(profiles, func(i, j int) bool {
		leftDefault := strings.TrimSpace(profiles[i].UpstreamModel) == defaultChannelCostModel
		rightDefault := strings.TrimSpace(profiles[j].UpstreamModel) == defaultChannelCostModel
		if leftDefault != rightDefault {
			return leftDefault
		}
		leftModel := strings.ToLower(strings.TrimSpace(profiles[i].UpstreamModel))
		rightModel := strings.ToLower(strings.TrimSpace(profiles[j].UpstreamModel))
		if leftModel != rightModel {
			return leftModel < rightModel
		}
		if profiles[i].UpdatedAt != profiles[j].UpdatedAt {
			return profiles[i].UpdatedAt > profiles[j].UpdatedAt
		}
		return profiles[i].Id > profiles[j].Id
	})
}

func channelCostRecalculateModelMatches(filter string, usageModel string, logModel string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == defaultChannelCostModel {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(usageModel), filter) ||
		strings.EqualFold(strings.TrimSpace(logModel), filter)
}
