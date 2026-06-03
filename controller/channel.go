package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/ollama"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type OpenAIModel struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	Created    int64          `json:"created"`
	OwnedBy    string         `json:"owned_by"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Permission []struct {
		ID                 string `json:"id"`
		Object             string `json:"object"`
		Created            int64  `json:"created"`
		AllowCreateEngine  bool   `json:"allow_create_engine"`
		AllowSampling      bool   `json:"allow_sampling"`
		AllowLogprobs      bool   `json:"allow_logprobs"`
		AllowSearchIndices bool   `json:"allow_search_indices"`
		AllowView          bool   `json:"allow_view"`
		AllowFineTuning    bool   `json:"allow_fine_tuning"`
		Organization       string `json:"organization"`
		Group              string `json:"group"`
		IsBlocking         bool   `json:"is_blocking"`
	} `json:"permission"`
	Root   string `json:"root"`
	Parent string `json:"parent"`
}

type OpenAIModelsResponse struct {
	Data    []OpenAIModel `json:"data"`
	Success bool          `json:"success"`
}

func parseStatusFilter(statusParam string) int {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return common.ChannelStatusEnabled
	case "disabled", "0":
		return 0
	default:
		return -1
	}
}

func clearChannelInfo(channel *model.Channel) {
	if channel.ChannelInfo.IsMultiKey {
		channel.ChannelInfo.MultiKeyDisabledReason = nil
		channel.ChannelInfo.MultiKeyDisabledTime = nil
	}
}

type channelResponse struct {
	*model.Channel
	FailureAvoidance                *service.ChannelFailureAvoidanceStatus   `json:"failure_avoidance,omitempty"`
	ConcurrencyCooldown             *service.ChannelConcurrencyControlStatus `json:"concurrency_cooldown,omitempty"`
	RuntimeCircuit                  *channelRuntimeCircuitStatus             `json:"runtime_circuit,omitempty"`
	StatusReason                    string                                   `json:"status_reason,omitempty"`
	BalanceInsufficient             bool                                     `json:"balance_insufficient"`
	RuntimeBalanceInsufficientCount int                                      `json:"runtime_balance_insufficient_count,omitempty"`
	UpstreamCostDisplay             *channelUpstreamCostDisplay              `json:"upstream_cost_display,omitempty"`
}

type channelRuntimeCircuitStatus struct {
	OpenRuntimeKeys     int            `json:"open_runtime_keys"`
	HalfOpenRuntimeKeys int            `json:"half_open_runtime_keys"`
	OpenReasons         map[string]int `json:"open_reasons,omitempty"`
}

type channelUpstreamCostDisplay struct {
	Configured                 bool    `json:"configured"`
	PriceConfigured            bool    `json:"price_configured"`
	Model                      string  `json:"model,omitempty"`
	PricingModel               string  `json:"pricing_model,omitempty"`
	Currency                   string  `json:"currency,omitempty"`
	PricingMode                string  `json:"pricing_mode,omitempty"`
	PriceSource                string  `json:"price_source,omitempty"`
	Accuracy                   string  `json:"accuracy,omitempty"`
	CostCoefficient            float64 `json:"cost_coefficient,omitempty"`
	FeeMultiplier              float64 `json:"fee_multiplier,omitempty"`
	BaseCostMultiplier         float64 `json:"base_cost_multiplier,omitempty"`
	TokenMultiplier            float64 `json:"token_multiplier,omitempty"`
	RechargeMultiplier         float64 `json:"recharge_multiplier,omitempty"`
	ActualTokenMultiplier      float64 `json:"actual_token_multiplier,omitempty"`
	BaseInputPerMillion        float64 `json:"base_input_per_million,omitempty"`
	BaseOutputPerMillion       float64 `json:"base_output_per_million,omitempty"`
	BaseCacheReadPerMillion    float64 `json:"base_cache_read_per_million,omitempty"`
	BaseCacheWritePerMillion   float64 `json:"base_cache_write_per_million,omitempty"`
	BaseCacheWrite1hPerMillion float64 `json:"base_cache_write_1h_per_million,omitempty"`
	BaseImageInputPerMillion   float64 `json:"base_image_input_per_million,omitempty"`
	BaseAudioInputPerMillion   float64 `json:"base_audio_input_per_million,omitempty"`
	BaseAudioOutputPerMillion  float64 `json:"base_audio_output_per_million,omitempty"`
	InputPerMillion            float64 `json:"input_per_million,omitempty"`
	OutputPerMillion           float64 `json:"output_per_million,omitempty"`
	CacheReadPerMillion        float64 `json:"cache_read_per_million,omitempty"`
	CacheWritePerMillion       float64 `json:"cache_write_per_million,omitempty"`
	CacheWrite1hPerMillion     float64 `json:"cache_write_1h_per_million,omitempty"`
	ImageInputPerMillion       float64 `json:"image_input_per_million,omitempty"`
	AudioInputPerMillion       float64 `json:"audio_input_per_million,omitempty"`
	AudioOutputPerMillion      float64 `json:"audio_output_per_million,omitempty"`
	RequestPrice               float64 `json:"request_price,omitempty"`
}

func buildChannelResponse(channel *model.Channel) *channelResponse {
	if channel == nil {
		return nil
	}
	costDisplays := buildChannelCostDisplays([]*model.Channel{channel})
	circuitDisplays := buildChannelRuntimeCircuitDisplays([]*model.Channel{channel})
	balanceCounts := service.RuntimeBalanceInsufficientChannelCounts()
	return buildChannelResponseWithDisplays(channel, costDisplays[channel.Id], circuitDisplays[channel.Id], balanceCounts[channel.Id])
}

func buildChannelResponseWithDisplays(channel *model.Channel, costDisplay *channelUpstreamCostDisplay, circuitStatus *channelRuntimeCircuitStatus, runtimeBalanceCount int) *channelResponse {
	if channel == nil {
		return nil
	}
	clearChannelInfo(channel)
	channel.CostPerMillion = nil
	statusReason := service.ChannelStatusReason(channel)
	return &channelResponse{
		Channel:                         channel,
		FailureAvoidance:                service.GetChannelFailureAvoidanceStatus(channel.Id),
		ConcurrencyCooldown:             service.GetChannelConcurrencyCooldownStatus(channel.Id),
		RuntimeCircuit:                  circuitStatus,
		StatusReason:                    statusReason,
		BalanceInsufficient:             service.IsKnownBalanceInsufficientChannel(channel) || runtimeBalanceCount > 0,
		RuntimeBalanceInsufficientCount: runtimeBalanceCount,
		UpstreamCostDisplay:             costDisplay,
	}
}

func buildChannelResponses(channels []*model.Channel) []*channelResponse {
	responses := make([]*channelResponse, 0, len(channels))
	costDisplays := buildChannelCostDisplays(channels)
	circuitDisplays := buildChannelRuntimeCircuitDisplays(channels)
	balanceCounts := service.RuntimeBalanceInsufficientChannelCounts()
	for _, channel := range channels {
		responses = append(responses, buildChannelResponseWithDisplays(channel, costDisplays[channel.Id], circuitDisplays[channel.Id], balanceCounts[channel.Id]))
	}
	return responses
}

func buildChannelRuntimeCircuitDisplays(channels []*model.Channel) map[int]*channelRuntimeCircuitStatus {
	displays := make(map[int]*channelRuntimeCircuitStatus, len(channels))
	if len(channels) == 0 {
		return displays
	}
	channelIDs := make(map[int]struct{}, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 {
			continue
		}
		channelIDs[channel.Id] = struct{}{}
	}
	if len(channelIDs) == 0 {
		return displays
	}
	runtimeDeps := modelgatewayintegration.CurrentDefaultRuntimeObservabilityDeps()
	if runtimeDeps == nil || runtimeDeps.CircuitBreaker == nil {
		return displays
	}
	for _, snapshot := range runtimeDeps.CircuitBreaker.ListSnapshots() {
		if _, ok := channelIDs[snapshot.Key.ChannelID]; !ok {
			continue
		}
		if snapshot.State != "open" && snapshot.State != "half_open" {
			continue
		}
		status := displays[snapshot.Key.ChannelID]
		if status == nil {
			status = &channelRuntimeCircuitStatus{}
			displays[snapshot.Key.ChannelID] = status
		}
		switch snapshot.State {
		case "open":
			status.OpenRuntimeKeys++
			reason := strings.TrimSpace(snapshot.OpenReason)
			if reason != "" {
				if status.OpenReasons == nil {
					status.OpenReasons = map[string]int{}
				}
				status.OpenReasons[reason]++
			}
		case "half_open":
			status.HalfOpenRuntimeKeys++
		}
	}
	return displays
}

func buildChannelCostDisplays(channels []*model.Channel) map[int]*channelUpstreamCostDisplay {
	displays := make(map[int]*channelUpstreamCostDisplay, len(channels))
	if len(channels) == 0 || model.DB == nil {
		return displays
	}
	channelByID := make(map[int]*model.Channel, len(channels))
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 {
			continue
		}
		if _, exists := channelByID[channel.Id]; exists {
			continue
		}
		channelByID[channel.Id] = channel
		ids = append(ids, channel.Id)
	}
	if len(ids) == 0 {
		return displays
	}

	profiles := make([]model.ModelGatewayChannelCostProfile, 0, len(ids))
	if err := model.DB.
		Where("channel_id IN ? AND upstream_model = ?", ids, defaultChannelCostModel).
		Find(&profiles).Error; err != nil {
		common.SysError("failed to load channel upstream cost displays: " + err.Error())
		return displays
	}

	now := common.GetTimestamp()
	profileByChannelID := make(map[int]model.ModelGatewayChannelCostProfile, len(profiles))
	for _, profile := range profiles {
		if profile.ChannelID <= 0 || profile.EffectiveTime > now {
			continue
		}
		if current, exists := profileByChannelID[profile.ChannelID]; !exists || betterChannelCostDisplayProfile(profile, current) {
			profileByChannelID[profile.ChannelID] = profile
		}
	}

	for channelID, profile := range profileByChannelID {
		if channel := channelByID[channelID]; channel != nil {
			if display := buildChannelCostDisplay(channel, profile); display != nil {
				displays[channelID] = display
			}
		}
	}
	return displays
}

func betterChannelCostDisplayProfile(next model.ModelGatewayChannelCostProfile, current model.ModelGatewayChannelCostProfile) bool {
	if next.EffectiveTime != current.EffectiveTime {
		return next.EffectiveTime > current.EffectiveTime
	}
	if next.Version != current.Version {
		return next.Version > current.Version
	}
	if next.UpdatedAt != current.UpdatedAt {
		return next.UpdatedAt > current.UpdatedAt
	}
	return next.Id > current.Id
}

func buildChannelCostDisplay(channel *model.Channel, profile model.ModelGatewayChannelCostProfile) *channelUpstreamCostDisplay {
	if channel == nil || profile.Id <= 0 {
		return nil
	}
	models := normalizeCostQuoteModels(channel.GetModels())
	if len(models) == 0 {
		display := channelCostDisplayFromQuote("", "", modelgatewaycost.QuoteSystemRatioProfile("", profile))
		display.Configured = true
		display.PriceConfigured = false
		return display
	}

	var fallback *channelUpstreamCostDisplay
	for _, modelName := range models {
		pricingModel := channel.ResolveMappedModelName(modelName)
		quote := modelgatewaycost.QuoteSystemRatioProfile(pricingModel, profile)
		quote.Model = modelName
		quote.PricingModel = pricingModel
		display := channelCostDisplayFromQuote(modelName, pricingModel, quote)
		if display.PriceConfigured {
			return display
		}
		if fallback == nil {
			fallback = display
		}
	}
	return fallback
}

func channelCostDisplayFromQuote(modelName string, pricingModel string, quote modelgatewaycost.SystemRatioQuote) *channelUpstreamCostDisplay {
	rechargeMultiplier := quote.RechargeMultiplier
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = 1
	}
	display := &channelUpstreamCostDisplay{
		Configured:                 true,
		PriceConfigured:            channelCostQuoteHasPrice(quote),
		Model:                      strings.TrimSpace(modelName),
		PricingModel:               strings.TrimSpace(pricingModel),
		Currency:                   strings.TrimSpace(quote.Currency),
		PricingMode:                strings.TrimSpace(quote.PricingMode),
		PriceSource:                strings.TrimSpace(quote.PriceSource),
		Accuracy:                   strings.TrimSpace(quote.Accuracy),
		CostCoefficient:            quote.CostCoefficient,
		FeeMultiplier:              quote.FeeMultiplier,
		BaseCostMultiplier:         quote.BaseCostMultiplier,
		TokenMultiplier:            quote.TokenMultiplier,
		RechargeMultiplier:         rechargeMultiplier,
		ActualTokenMultiplier:      quote.ActualTokenMultiplier,
		BaseInputPerMillion:        quote.BaseInputPerMillion,
		BaseOutputPerMillion:       quote.BaseOutputPerMillion,
		BaseCacheReadPerMillion:    quote.BaseCacheReadPerMillion,
		BaseCacheWritePerMillion:   quote.BaseCacheWritePerMillion,
		BaseCacheWrite1hPerMillion: quote.BaseCacheWrite1hPerMillion,
		BaseImageInputPerMillion:   quote.BaseImageInputPerMillion,
		BaseAudioInputPerMillion:   quote.BaseAudioInputPerMillion,
		BaseAudioOutputPerMillion:  quote.BaseAudioOutputPerMillion,
		InputPerMillion:            quote.InputPerMillion,
		OutputPerMillion:           quote.OutputPerMillion,
		CacheReadPerMillion:        quote.CacheReadPerMillion,
		CacheWritePerMillion:       quote.CacheWritePerMillion,
		CacheWrite1hPerMillion:     quote.CacheWrite1hPerMillion,
		ImageInputPerMillion:       quote.ImageInputPerMillion,
		AudioInputPerMillion:       quote.AudioInputPerMillion,
		AudioOutputPerMillion:      quote.AudioOutputPerMillion,
		RequestPrice:               quote.RequestPrice,
	}
	if display.PricingModel == "" {
		display.PricingModel = strings.TrimSpace(quote.PricingModel)
	}
	if display.Model == "" {
		display.Model = strings.TrimSpace(quote.Model)
	}
	return display
}

func channelCostQuoteHasPrice(quote modelgatewaycost.SystemRatioQuote) bool {
	return quote.InputPerMillion > 0 ||
		quote.OutputPerMillion > 0 ||
		quote.CacheReadPerMillion > 0 ||
		quote.CacheWritePerMillion > 0 ||
		quote.CacheWrite1hPerMillion > 0 ||
		quote.ImageInputPerMillion > 0 ||
		quote.AudioInputPerMillion > 0 ||
		quote.AudioOutputPerMillion > 0 ||
		quote.RequestPrice > 0
}

func GetAllChannels(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	channelData := make([]*model.Channel, 0)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort)
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	statusParam := c.Query("status")
	// statusFilter: -1 all, 1 enabled, 0 disabled (include auto & manual)
	statusFilter := parseStatusFilter(statusParam)
	// type filter
	typeStr := c.Query("type")
	typeFilter := -1
	if typeStr != "" {
		if t, err := strconv.Atoi(typeStr); err == nil {
			typeFilter = t
		}
	}

	var total int64

	if enableTagMode {
		tags, err := model.GetPaginatedTags(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
		if err != nil {
			common.SysError("failed to get paginated tags: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签失败，请稍后重试"})
			return
		}
		for _, tag := range tags {
			if tag == nil || *tag == "" {
				continue
			}
			tagChannels, err := model.GetChannelsByTag(*tag, idSort, false, sortOptions)
			if err != nil {
				continue
			}
			filtered := make([]*model.Channel, 0)
			for _, ch := range tagChannels {
				if statusFilter == common.ChannelStatusEnabled && ch.Status != common.ChannelStatusEnabled {
					continue
				}
				if statusFilter == 0 && ch.Status == common.ChannelStatusEnabled {
					continue
				}
				if typeFilter >= 0 && ch.Type != typeFilter {
					continue
				}
				filtered = append(filtered, ch)
			}
			channelData = append(channelData, filtered...)
		}
		total, _ = model.CountAllTags()
	} else {
		baseQuery := model.DB.Model(&model.Channel{})
		if typeFilter >= 0 {
			baseQuery = baseQuery.Where("type = ?", typeFilter)
		}
		if statusFilter == common.ChannelStatusEnabled {
			baseQuery = baseQuery.Where("status = ?", common.ChannelStatusEnabled)
		} else if statusFilter == 0 {
			baseQuery = baseQuery.Where("status != ?", common.ChannelStatusEnabled)
		}

		baseQuery.Count(&total)

		err := sortOptions.Apply(baseQuery).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("key").Find(&channelData).Error
		if err != nil {
			common.SysError("failed to get channels: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道列表失败，请稍后重试"})
			return
		}
	}

	countQuery := model.DB.Model(&model.Channel{})
	if statusFilter == common.ChannelStatusEnabled {
		countQuery = countQuery.Where("status = ?", common.ChannelStatusEnabled)
	} else if statusFilter == 0 {
		countQuery = countQuery.Where("status != ?", common.ChannelStatusEnabled)
	}
	var results []struct {
		Type  int64
		Count int64
	}
	_ = countQuery.Select("type, count(*) as count").Group("type").Find(&results).Error
	typeCounts := make(map[int64]int64)
	for _, r := range results {
		typeCounts[r.Type] = r.Count
	}
	common.ApiSuccess(c, gin.H{
		"items":       buildChannelResponses(channelData),
		"total":       total,
		"page":        pageInfo.GetPage(),
		"page_size":   pageInfo.GetPageSize(),
		"type_counts": typeCounts,
	})
	return
}

func buildFetchModelsHeaders(channel *model.Channel, key string) (http.Header, error) {
	var headers http.Header
	switch channel.Type {
	case constant.ChannelTypeAnthropic:
		headers = GetClaudeAuthHeader(key)
	default:
		headers = GetAuthHeader(key)
	}

	headerOverride := channel.GetHeaderOverride()
	for k, v := range headerOverride {
		if relaychannel.IsHeaderPassthroughRuleKey(k) {
			continue
		}
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid header override for key %s", k)
		}
		if strings.Contains(str, "{api_key}") {
			str = strings.ReplaceAll(str, "{api_key}", key)
		}
		headers.Set(k, str)
	}

	return headers, nil
}

func FetchUpstreamModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ids, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取模型列表失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ids,
	})
}

func collectCodexImageGenerationToolModels(models []string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		normalized := strings.TrimSpace(modelName)
		if normalized == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(normalized), "gpt-image-") {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func collectCodexResponsesImageToolProbeModels(models []string) []string {
	priority := []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.3-codex",
		"gpt-5.3",
		"gpt-5.2",
		"gpt-5",
		"gpt-4.1",
		"gpt-4o",
		"o4",
		"o3",
	}
	seen := make(map[string]struct{}, len(models))
	normalized := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		lower := strings.ToLower(modelName)
		if strings.HasPrefix(lower, "gpt-image-") || common.IsImageGenerationModel(modelName) {
			continue
		}
		if !common.IsOpenAITextModel(modelName) {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		normalized = append(normalized, modelName)
	}

	result := make([]string, 0, len(normalized))
	resultSeen := make(map[string]struct{}, len(normalized))
	appendResult := func(modelName string) {
		if _, ok := resultSeen[modelName]; ok {
			return
		}
		resultSeen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	for _, prefix := range priority {
		for _, modelName := range normalized {
			if strings.HasPrefix(strings.ToLower(modelName), prefix) {
				appendResult(modelName)
			}
		}
	}
	for _, modelName := range normalized {
		appendResult(modelName)
	}
	return result
}

type codexImageToolProbeResponse struct {
	Error  any `json:"error,omitempty"`
	Output []struct {
		Type    string `json:"type"`
		Status  string `json:"status,omitempty"`
		Result  string `json:"result,omitempty"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content,omitempty"`
	} `json:"output,omitempty"`
}

type codexImageToolProbeConfigRequest struct {
	BaseURL        string          `json:"base_url"`
	Type           int             `json:"type"`
	Key            string          `json:"key"`
	Settings       json.RawMessage `json:"settings"`
	WireAPI        string          `json:"wire_api"`
	Proxy          string          `json:"proxy"`
	HeaderOverride string          `json:"header_override"`
}

func buildCodexResponsesImageToolProbeURL(channel *model.Channel) string {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}
	requestPath := "/v1/responses"
	settings := channel.GetOtherSettings()
	if customPath := settings.GetOpenAIWireAPIPath(false); customPath != "" {
		requestPath = customPath
	}
	return relaycommon.GetFullRequestURL(strings.TrimRight(baseURL, "/"), requestPath, channel.Type)
}

func parseCodexProbeOtherSettings(raw json.RawMessage) (dto.ChannelOtherSettings, error) {
	settings := dto.ChannelOtherSettings{}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return settings, nil
	}
	if raw[0] == '"' {
		var settingsText string
		if err := common.Unmarshal(raw, &settingsText); err != nil {
			return settings, err
		}
		if strings.TrimSpace(settingsText) == "" {
			return settings, nil
		}
		if err := common.UnmarshalJsonStr(settingsText, &settings); err != nil {
			return settings, err
		}
		return settings, nil
	}
	if err := common.Unmarshal(raw, &settings); err != nil {
		return settings, err
	}
	return settings, nil
}

func buildTransientCodexProbeChannel(req codexImageToolProbeConfigRequest) (*model.Channel, error) {
	if req.Type != constant.ChannelTypeOpenAI {
		return nil, fmt.Errorf("仅 OpenAI 渠道支持 Codex 工具能力检测")
	}

	key := strings.TrimSpace(req.Key)
	if key == "" {
		return nil, fmt.Errorf("请填写密钥")
	}
	key = strings.TrimSpace(strings.Split(key, "\n")[0])

	settings, err := parseCodexProbeOtherSettings(req.Settings)
	if err != nil {
		return nil, fmt.Errorf("解析 settings 失败: %w", err)
	}
	settings.CodexCompatibilityMode = true
	if wireAPI := strings.TrimSpace(req.WireAPI); wireAPI != "" {
		settings.WireAPI = wireAPI
	}
	settingsBytes, err := common.Marshal(settings)
	if err != nil {
		return nil, err
	}

	channelSettings := dto.ChannelSettings{Proxy: strings.TrimSpace(req.Proxy)}
	channelSettingsBytes, err := common.Marshal(channelSettings)
	if err != nil {
		return nil, err
	}

	channel := &model.Channel{
		Type:          req.Type,
		Key:           key,
		OtherSettings: string(settingsBytes),
		Setting:       common.GetPointer(string(channelSettingsBytes)),
	}
	if baseURL := strings.TrimSpace(req.BaseURL); baseURL != "" {
		channel.BaseURL = common.GetPointer(baseURL)
	}
	if headerOverride := strings.TrimSpace(req.HeaderOverride); headerOverride != "" {
		channel.HeaderOverride = common.GetPointer(headerOverride)
	}
	return channel, nil
}

func truncateProbeMessage(message string) string {
	message = strings.TrimSpace(message)
	if len([]rune(message)) <= 180 {
		return message
	}
	return string([]rune(message)[:180]) + "..."
}

func codexImageToolProbeFailureMessage(resp codexImageToolProbeResponse) string {
	if resp.Error != nil {
		if errorBytes, err := common.Marshal(resp.Error); err == nil {
			return truncateProbeMessage(string(errorBytes))
		}
		return truncateProbeMessage(common.Interface2String(resp.Error))
	}
	for _, output := range resp.Output {
		for _, content := range output.Content {
			if content.Text != "" {
				return truncateProbeMessage(content.Text)
			}
		}
	}
	return "未返回 image_generation_call"
}

func codexImageToolProbeSucceeded(resp codexImageToolProbeResponse) bool {
	for _, output := range resp.Output {
		if output.Type != dto.ResponsesOutputTypeImageGenerationCall {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(output.Status))
		switch status {
		case "failed", "error", "cancelled", "canceled":
			return false
		default:
			return true
		}
	}
	return false
}

func probeCodexResponsesImageToolModel(ctx context.Context, channel *model.Channel, url string, headers http.Header, modelName string) (bool, string) {
	body, err := common.Marshal(map[string]any{
		"model": modelName,
		"input": "Generate a tiny cyan square for capability probing.",
		"tools": []map[string]any{
			{"type": dto.BuildInToolImageGeneration},
		},
		"tool_choice": map[string]any{
			"type": dto.BuildInToolImageGeneration,
		},
		"store": false,
	})
	if err != nil {
		return false, err.Error()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, err.Error()
	}
	for k := range headers {
		req.Header.Set(k, headers.Get(k))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return false, err.Error()
	}
	res, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer res.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return false, err.Error()
	}
	if res.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("Responses image_generation smoke test status code: %d, body: %s", res.StatusCode, truncateProbeMessage(string(responseBody)))
	}

	var probeResp codexImageToolProbeResponse
	if err := common.Unmarshal(responseBody, &probeResp); err != nil {
		return false, err.Error()
	}
	if codexImageToolProbeSucceeded(probeResp) {
		return true, ""
	}
	return false, codexImageToolProbeFailureMessage(probeResp)
}

func probeCodexResponsesImageTool(channel *model.Channel, upstreamModels []string, imageModels []string) (bool, string) {
	if len(imageModels) == 0 {
		return false, "未检测到 gpt-image-* 模型"
	}
	probeModels := collectCodexResponsesImageToolProbeModels(upstreamModels)
	if len(probeModels) == 0 {
		return false, fmt.Sprintf("检测到 %s，但未检测到可用于 Responses 工具调用的文本模型", strings.Join(imageModels, ", "))
	}
	if len(probeModels) > 3 {
		probeModels = probeModels[:3]
	}

	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return false, fmt.Sprintf("获取渠道密钥失败: %s", apiErr.Error())
	}
	headers, err := buildFetchModelsHeaders(channel, strings.TrimSpace(key))
	if err != nil {
		return false, err.Error()
	}

	url := buildCodexResponsesImageToolProbeURL(channel)
	var lastMessage string
	for _, modelName := range probeModels {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		ok, message := probeCodexResponsesImageToolModel(ctx, channel, url, headers, modelName)
		cancel()
		if ok {
			return true, fmt.Sprintf("Codex Responses 生图工具验证通过: %s", modelName)
		}
		lastMessage = fmt.Sprintf("%s: %s", modelName, message)
	}
	if lastMessage == "" {
		lastMessage = "未返回 image_generation_call"
	}
	return false, fmt.Sprintf("检测到 %s，但 Responses image_generation 工具调用未通过（%s）", strings.Join(imageModels, ", "), truncateProbeMessage(lastMessage))
}

func probeCodexResponsesImageToolForChannel(channel *model.Channel) (bool, []string, int64, string, error) {
	upstreamModels, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		return false, nil, 0, "", err
	}

	imageModels := collectCodexImageGenerationToolModels(upstreamModels)
	now := common.GetTimestamp()
	supported, message := probeCodexResponsesImageTool(channel, upstreamModels, imageModels)
	return supported, imageModels, now, message, nil
}

func respondCodexImageToolProbe(c *gin.Context, supported bool, imageModels []string, checkedAt int64, message string) {
	tools := []string{}
	if supported {
		tools = []string{dto.BuildInToolImageGeneration}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"supported":  supported,
			"models":     imageModels,
			"checked_at": checkedAt,
			"message":    message,
			"tools":      tools,
		},
	})
}

func ProbeUnsavedChannelCodexImageGenerationTool(c *gin.Context) {
	var req codexImageToolProbeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
		})
		return
	}

	channel, err := buildTransientCodexProbeChannel(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	supported, imageModels, checkedAt, message, err := probeCodexResponsesImageToolForChannel(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("检测失败: %s", err.Error()),
		})
		return
	}

	respondCodexImageToolProbe(c, supported, imageModels, checkedAt, message)
}

func ProbeChannelCodexImageGenerationTool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Type != constant.ChannelTypeOpenAI {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "仅 OpenAI 渠道支持 Codex 工具能力检测",
		})
		return
	}

	supported, imageModels, checkedAt, message, err := probeCodexResponsesImageToolForChannel(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("检测失败: %s", err.Error()),
		})
		return
	}

	settings := channel.GetOtherSettings()
	settings.CodexImageGenerationToolSupported = supported
	settings.CodexImageGenerationToolProbeTime = checkedAt
	settings.CodexImageGenerationToolProbeMessage = message
	settings.CodexImageGenerationToolProbeModels = imageModels
	if supported {
		settings.CodexSupportedTools = []string{dto.BuildInToolImageGeneration}
	} else {
		settings.CodexSupportedTools = []string{}
	}
	if err := updateChannelUpstreamModelSettings(channel, settings, false); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()

	respondCodexImageToolProbe(c, supported, imageModels, checkedAt, message)
}

func FixChannelsAbilities(c *gin.Context) {
	success, fails, err := model.FixAbility()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"success": success,
			"fails":   fails,
		},
	})
}

func ClearChannelFailureAvoidance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if _, err := model.GetChannelById(id, false); err != nil {
		common.ApiError(c, err)
		return
	}

	service.ClearChannelFailureAvoidance(id)
	common.ApiSuccess(c, gin.H{
		"id": id,
	})
}

type ChannelHealthRecoverResponse struct {
	ChannelID                       int  `json:"channel_id"`
	ChannelStatus                   int  `json:"channel_status"`
	RuntimeCircuitsCleared          int  `json:"runtime_circuits_cleared"`
	RuntimeSnapshotsUpdated         int  `json:"runtime_snapshots_updated"`
	RuntimeCooldownSnapshotsUpdated int  `json:"runtime_cooldown_snapshots_updated"`
	FailureAvoidanceCleared         int  `json:"failure_avoidance_cleared"`
	ConcurrencyCooldownCleared      bool `json:"concurrency_cooldown_cleared"`
	RuntimeBalanceCleared           int  `json:"runtime_balance_cleared"`
	BalanceMarkerCleared            bool `json:"balance_marker_cleared"`
	MultiKeyBalanceCleared          int  `json:"multi_key_balance_cleared"`
	StatusUpdated                   bool `json:"status_updated"`
}

func clearChannelMultiKeyBalanceInsufficient(channel *model.Channel) (int, bool, error) {
	if channel == nil || !channel.ChannelInfo.IsMultiKey {
		return 0, false, nil
	}
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	keys := channel.GetKeys()
	if len(keys) == 0 {
		return 0, false, nil
	}
	if channel.ChannelInfo.MultiKeyStatusList == nil {
		channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
	}
	changed := false
	cleared := 0
	if channel.ChannelInfo.MultiKeyDisabledReason != nil {
		for credentialIndex, reason := range channel.ChannelInfo.MultiKeyDisabledReason {
			if !service.IsBalanceInsufficientStatusReason(reason) {
				continue
			}
			delete(channel.ChannelInfo.MultiKeyStatusList, credentialIndex)
			delete(channel.ChannelInfo.MultiKeyDisabledReason, credentialIndex)
			if channel.ChannelInfo.MultiKeyDisabledTime != nil {
				delete(channel.ChannelInfo.MultiKeyDisabledTime, credentialIndex)
			}
			changed = true
			cleared++
		}
	}
	if !changed {
		return 0, false, nil
	}

	enabledCount := 0
	for i := range keys {
		status := common.ChannelStatusEnabled
		if value, ok := channel.ChannelInfo.MultiKeyStatusList[i]; ok {
			status = value
		}
		if status == common.ChannelStatusEnabled {
			enabledCount++
		}
	}

	statusUpdated := false
	info := channel.GetOtherInfo()
	statusReason, _ := info["status_reason"].(string)
	if enabledCount > 0 && channel.Status == common.ChannelStatusAutoDisabled &&
		(strings.TrimSpace(statusReason) == "" ||
			service.IsBalanceInsufficientStatusReason(statusReason) ||
			strings.TrimSpace(statusReason) == channelBalanceAllAccountsDisabledReason) {
		channel.Status = common.ChannelStatusEnabled
		delete(info, "status_reason")
		delete(info, "status_time")
		channel.SetOtherInfo(info)
		statusUpdated = true
	}

	if err := channel.SaveWithoutKey(); err != nil {
		common.SysLog(fmt.Sprintf("failed to clear channel balance insufficient state: channel_id=%d, error=%s", channel.Id, err.Error()))
		return 0, false, err
	}
	if common.MemoryCacheEnabled {
		model.CacheUpdateChannel(channel)
	}
	if statusUpdated {
		_ = model.UpdateAbilityStatus(channel.Id, true)
	}
	return cleared, statusUpdated, nil
}

func RecoverChannelHealth(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	wasBalancePaused := service.IsBalanceInsufficientPausedChannel(channel)
	wasBalanceReason := service.IsBalanceInsufficientStatusReason(service.ChannelStatusReason(channel))
	wasConfirmedBalance := service.IsConfirmedBalanceInsufficientChannel(channel)

	runtimeDeps := modelgatewayintegration.CurrentDefaultRuntimeObservabilityDeps()
	filter := ModelGatewayRuntimeKey{ChannelID: id}
	matchedKeys := modelGatewayRuntimeCircuitClearKeys(runtimeDeps, id, filter)
	circuitsCleared := 0
	if runtimeDeps != nil && runtimeDeps.CircuitBreaker != nil {
		circuitsCleared = runtimeDeps.CircuitBreaker.ResetChannel(id)
	}
	runtimeSnapshotsUpdated := modelGatewayClearRuntimeCircuitSnapshots(runtimeDeps, matchedKeys, true)
	runtimeCooldownSnapshotsUpdated := modelGatewayClearRuntimeCooldownSnapshots(runtimeDeps, matchedKeys)
	concurrencyCooldownCleared := service.ClearChannelConcurrencyCooldown(id)
	service.ClearChannelFailureAvoidance(id)
	runtimeBalanceCleared := service.ClearChannelBalanceInsufficientForChannel(id)
	balanceMarkerCleared := false
	if wasConfirmedBalance {
		balanceMarkerCleared = model.ClearChannelBalanceInsufficientMarker(id)
		if balanceMarkerCleared {
			channel.BalanceUpdatedTime = 0
		}
	}

	multiKeyBalanceCleared := 0
	statusUpdated := false
	if channel.ChannelInfo.IsMultiKey {
		multiKeyBalanceCleared, statusUpdated, err = clearChannelMultiKeyBalanceInsufficient(channel)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	if wasBalancePaused {
		if model.UpdateChannelStatusWholeChannelWithInfo(id, common.ChannelStatusEnabled, "", nil) {
			statusUpdated = true
		}
		channel.Status = common.ChannelStatusEnabled
	} else if wasBalanceReason {
		if model.UpdateChannelStatusWholeChannelWithInfo(id, channel.Status, "", nil) {
			statusUpdated = true
		}
	}

	if statusUpdated || balanceMarkerCleared || runtimeBalanceCleared > 0 || concurrencyCooldownCleared || circuitsCleared > 0 || runtimeSnapshotsUpdated > 0 || runtimeCooldownSnapshotsUpdated > 0 || multiKeyBalanceCleared > 0 {
		model.InitChannelCache()
		modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	}

	common.ApiSuccess(c, ChannelHealthRecoverResponse{
		ChannelID:                       id,
		ChannelStatus:                   channel.Status,
		RuntimeCircuitsCleared:          circuitsCleared,
		RuntimeSnapshotsUpdated:         runtimeSnapshotsUpdated,
		RuntimeCooldownSnapshotsUpdated: runtimeCooldownSnapshotsUpdated,
		FailureAvoidanceCleared:         1,
		ConcurrencyCooldownCleared:      concurrencyCooldownCleared,
		RuntimeBalanceCleared:           runtimeBalanceCleared,
		BalanceMarkerCleared:            balanceMarkerCleared,
		MultiKeyBalanceCleared:          multiKeyBalanceCleared,
		StatusUpdated:                   statusUpdated,
	})
}

func SearchChannels(c *gin.Context) {
	keyword := c.Query("keyword")
	group := c.Query("group")
	modelKeyword := c.Query("model")
	statusParam := c.Query("status")
	statusFilter := parseStatusFilter(statusParam)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort)
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	channelData := make([]*model.Channel, 0)
	if enableTagMode {
		tags, err := model.SearchTags(keyword, group, modelKeyword, idSort)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		for _, tag := range tags {
			if tag != nil && *tag != "" {
				tagChannel, err := model.GetChannelsByTag(*tag, idSort, false, sortOptions)
				if err == nil {
					channelData = append(channelData, tagChannel...)
				}
			}
		}
	} else {
		channels, err := model.SearchChannels(keyword, group, modelKeyword, idSort, sortOptions)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channelData = channels
	}

	if statusFilter == common.ChannelStatusEnabled || statusFilter == 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if statusFilter == common.ChannelStatusEnabled && ch.Status != common.ChannelStatusEnabled {
				continue
			}
			if statusFilter == 0 && ch.Status == common.ChannelStatusEnabled {
				continue
			}
			filtered = append(filtered, ch)
		}
		channelData = filtered
	}

	// calculate type counts for search results
	typeCounts := make(map[int64]int64)
	for _, channel := range channelData {
		typeCounts[int64(channel.Type)]++
	}

	typeParam := c.Query("type")
	typeFilter := -1
	if typeParam != "" {
		if tp, err := strconv.Atoi(typeParam); err == nil {
			typeFilter = tp
		}
	}

	if typeFilter >= 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if ch.Type == typeFilter {
				filtered = append(filtered, ch)
			}
		}
		channelData = filtered
	}

	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	total := len(channelData)
	startIdx := (page - 1) * pageSize
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}

	pagedData := channelData[startIdx:endIdx]

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":       buildChannelResponses(pagedData),
			"total":       total,
			"type_counts": typeCounts,
		},
	})
	return
}

func GetChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildChannelResponse(channel),
	})
	return
}

// GetChannelKey 获取渠道密钥（需要通过安全验证中间件）
// 此函数依赖 SecureVerificationRequired 中间件，确保用户已通过安全验证
func GetChannelKey(c *gin.Context) {
	userId := c.GetInt("id")
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("渠道ID格式错误: %v", err))
		return
	}

	// 获取渠道信息（包含密钥）
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取渠道信息失败: %v", err))
		return
	}

	if channel == nil {
		common.ApiError(c, fmt.Errorf("渠道不存在"))
		return
	}

	// 记录操作日志
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("查看渠道密钥信息 (渠道ID: %d)", channelId))

	// 返回渠道密钥
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data": map[string]interface{}{
			"key": channel.Key,
		},
	})
}

// validateTwoFactorAuth 统一的2FA验证函数
func validateTwoFactorAuth(twoFA *model.TwoFA, code string) bool {
	// 尝试验证TOTP
	if cleanCode, err := common.ValidateNumericCode(code); err == nil {
		if isValid, _ := twoFA.ValidateTOTPAndUpdateUsage(cleanCode); isValid {
			return true
		}
	}

	// 尝试验证备用码
	if isValid, err := twoFA.ValidateBackupCodeAndUpdateUsage(code); err == nil && isValid {
		return true
	}

	return false
}

// validateChannel 通用的渠道校验函数
func validateChannel(channel *model.Channel, isAdd bool) error {
	if channel == nil {
		return fmt.Errorf("channel cannot be empty")
	}

	// 校验 channel settings
	if err := channel.ValidateSettings(); err != nil {
		return fmt.Errorf("渠道额外设置[channel setting] 格式错误：%s", err.Error())
	}

	// 如果是添加操作，检查 channel 和 key 是否为空
	if isAdd {
		// 检查模型名称长度是否超过 255
		for _, m := range channel.GetModels() {
			if len(m) > 255 {
				return fmt.Errorf("模型名称过长: %s", m)
			}
		}
	}

	// VertexAI 特殊校验
	if channel.Type == constant.ChannelTypeVertexAi {
		if channel.Other == "" {
			return fmt.Errorf("部署地区不能为空")
		}

		regionMap, err := common.StrToMap(channel.Other)
		if err != nil {
			return fmt.Errorf("部署地区必须是标准的Json格式，例如{\"default\": \"us-central1\", \"region2\": \"us-east1\"}")
		}

		if regionMap["default"] == nil {
			return fmt.Errorf("部署地区必须包含default字段")
		}
	}

	// Codex OAuth key validation (optional, only when JSON object is provided)
	if channel.Type == constant.ChannelTypeCodex {
		if err := validateCodexChannelCredential(channel.Key); err != nil {
			return err
		}
	}

	return nil
}

func validateCodexChannelCredential(key string) error {
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return nil
	}
	if !strings.HasPrefix(trimmedKey, "{") {
		return fmt.Errorf("Codex channel only supports OAuth JSON credentials. Use OpenAI channel for standard API keys")
	}
	var keyMap map[string]interface{}
	if err := common.Unmarshal([]byte(trimmedKey), &keyMap); err != nil {
		return fmt.Errorf("Codex key must be a valid OAuth JSON object")
	}
	codexauth.NormalizeOAuthJSONCredentialMap(keyMap)
	if v, ok := keyMap["access_token"]; !ok || v == nil || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
		return fmt.Errorf("Codex key JSON must include access_token")
	}
	accountID := ""
	for _, key := range []string{"account_id", "chatgpt_account_id"} {
		if v, ok := keyMap[key]; ok && v != nil {
			accountID = strings.TrimSpace(fmt.Sprintf("%v", v))
			if accountID != "" {
				break
			}
		}
	}
	if accountID == "" {
		return fmt.Errorf("Codex key JSON must include account_id or chatgpt_account_id")
	}
	return nil
}

func isCodexChannelWithoutAccounts(channel *model.Channel) bool {
	if channel == nil || channel.Type != constant.ChannelTypeCodex {
		return false
	}
	if strings.TrimSpace(channel.Key) == "" {
		return true
	}
	for _, key := range channel.GetKeys() {
		if strings.TrimSpace(key) != "" {
			return false
		}
	}
	return true
}

func prepareEmptyCodexAccountPoolChannel(channel *model.Channel) {
	if channel == nil {
		return
	}
	channel.Key = ""
	channel.Status = common.ChannelStatusAutoDisabled
	channel.ChannelInfo.IsMultiKey = false
	channel.ChannelInfo.MultiKeySize = 0
	channel.ChannelInfo.MultiKeyStatusList = nil
	channel.ChannelInfo.MultiKeyDisabledReason = nil
	channel.ChannelInfo.MultiKeyDisabledTime = nil
	channel.ChannelInfo.MultiKeyProxyIDs = nil
	channel.ChannelInfo.MultiKeyAccountTypes = nil
	channel.ChannelInfo.MultiKeyCapabilities = nil
	setChannelAccountStatusReason(channel, channelAccountEmptyCodexReason)
}

func prepareEmptyKeyChannel(channel *model.Channel) {
	if channel == nil {
		return
	}
	channel.Key = ""
	channel.Status = common.ChannelStatusAutoDisabled
	channel.ChannelInfo.IsMultiKey = false
	channel.ChannelInfo.MultiKeySize = 0
	channel.ChannelInfo.MultiKeyStatusList = nil
	channel.ChannelInfo.MultiKeyDisabledReason = nil
	channel.ChannelInfo.MultiKeyDisabledTime = nil
	channel.ChannelInfo.MultiKeyProxyIDs = nil
	channel.ChannelInfo.MultiKeyAccountTypes = nil
	channel.ChannelInfo.MultiKeyCapabilities = nil
	setChannelAccountStatusReason(channel, channelAccountAllKeysDisabledReason)
}

func RefreshCodexChannelCredential(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	oauthKey, ch, err := service.RefreshCodexChannelCredential(ctx, channelId, service.CodexCredentialRefreshOptions{ResetCaches: true})
	if err != nil {
		common.SysError("failed to refresh codex channel credential: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "刷新凭证失败，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "refreshed",
		"data": gin.H{
			"expires_at":   oauthKey.Expired,
			"last_refresh": oauthKey.LastRefresh,
			"account_id":   oauthKey.AccountID,
			"email":        oauthKey.Email,
			"channel_id":   ch.Id,
			"channel_type": ch.Type,
			"channel_name": ch.Name,
		},
	})
}

type AddChannelRequest struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *model.Channel        `json:"channel"`
}

func getVertexArrayKeys(keys string) ([]string, error) {
	if keys == "" {
		return nil, nil
	}
	var keyArray []interface{}
	err := common.Unmarshal([]byte(keys), &keyArray)
	if err != nil {
		return nil, fmt.Errorf("批量添加 Vertex AI 必须使用标准的JsonArray格式，例如[{key1}, {key2}...]，请检查输入: %w", err)
	}
	cleanKeys := make([]string, 0, len(keyArray))
	for _, key := range keyArray {
		var keyStr string
		switch v := key.(type) {
		case string:
			keyStr = strings.TrimSpace(v)
		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("Vertex AI key JSON 编码失败: %w", err)
			}
			keyStr = string(bytes)
		}
		if keyStr != "" {
			cleanKeys = append(cleanKeys, keyStr)
		}
	}
	if len(cleanKeys) == 0 {
		return nil, fmt.Errorf("批量添加 Vertex AI 的 keys 不能为空")
	}
	return cleanKeys, nil
}

func AddChannel(c *gin.Context) {
	addChannelRequest := AddChannelRequest{}
	err := c.ShouldBindJSON(&addChannelRequest)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 使用统一的校验函数
	if err := validateChannel(addChannelRequest.Channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	addChannelRequest.Channel.CostPerMillion = nil
	addChannelRequest.Channel.CreatedTime = common.GetTimestamp()
	keys := make([]string, 0)
	allowEmptyKeyChannel := addChannelRequest.Mode == "single" &&
		strings.TrimSpace(addChannelRequest.Channel.Key) == ""
	switch addChannelRequest.Mode {
	case "multi_to_single":
		addChannelRequest.Channel.ChannelInfo.IsMultiKey = true
		addChannelRequest.Channel.ChannelInfo.MultiKeyMode = addChannelRequest.MultiKeyMode
		if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			array, err := getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(array)
			addChannelRequest.Channel.Key = strings.Join(array, "\n")
		} else {
			cleanKeys := make([]string, 0)
			for _, key := range strings.Split(addChannelRequest.Channel.Key, "\n") {
				if key == "" {
					continue
				}
				key = strings.TrimSpace(key)
				cleanKeys = append(cleanKeys, key)
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(cleanKeys)
			addChannelRequest.Channel.Key = strings.Join(cleanKeys, "\n")
		}
		keys = []string{addChannelRequest.Channel.Key}
	case "batch":
		if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			// multi json
			keys, err = getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			keys = strings.Split(addChannelRequest.Channel.Key, "\n")
		}
	case "single":
		keys = []string{addChannelRequest.Channel.Key}
	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的添加模式",
		})
		return
	}

	channels := make([]model.Channel, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" && !allowEmptyKeyChannel {
			continue
		}
		localChannel := *addChannelRequest.Channel
		localChannel.Key = key
		if allowEmptyKeyChannel {
			if localChannel.Type == constant.ChannelTypeCodex {
				prepareEmptyCodexAccountPoolChannel(&localChannel)
			} else {
				prepareEmptyKeyChannel(&localChannel)
			}
		}
		if addChannelRequest.BatchAddSetKeyPrefix2Name && len(keys) > 1 {
			keyPrefix := localChannel.Key
			if len(localChannel.Key) > 8 {
				keyPrefix = localChannel.Key[:8]
			}
			localChannel.Name = fmt.Sprintf("%s %s", localChannel.Name, keyPrefix)
		}
		channels = append(channels, localChannel)
	}
	if len(channels) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请填写渠道密钥",
		})
		return
	}
	err = model.BatchInsertChannels(channels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	service.ResetProxyClientCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel.Id > 0 {
			channelIDs = append(channelIDs, channel.Id)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_ids": channelIDs,
		},
	})
	return
}

func DeleteChannel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	channel := model.Channel{Id: id}
	err := channel.Delete()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteDisabledChannel(c *gin.Context) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

type ChannelTag struct {
	Tag            string  `json:"tag"`
	NewTag         *string `json:"new_tag"`
	Priority       *int64  `json:"priority"`
	Weight         *uint   `json:"weight"`
	ModelMapping   *string `json:"model_mapping"`
	Models         *string `json:"models"`
	Groups         *string `json:"groups"`
	ParamOverride  *string `json:"param_override"`
	HeaderOverride *string `json:"header_override"`
}

func DisableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.DisableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EnableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.EnableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EditTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	if channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}
	if channelTag.ParamOverride != nil {
		trimmed := strings.TrimSpace(*channelTag.ParamOverride)
		if trimmed != "" && !json.Valid([]byte(trimmed)) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "参数覆盖必须是合法的 JSON 格式",
			})
			return
		}
		channelTag.ParamOverride = common.GetPointer[string](trimmed)
	}
	if channelTag.HeaderOverride != nil {
		trimmed := strings.TrimSpace(*channelTag.HeaderOverride)
		if trimmed != "" && !json.Valid([]byte(trimmed)) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "请求头覆盖必须是合法的 JSON 格式",
			})
			return
		}
		channelTag.HeaderOverride = common.GetPointer[string](trimmed)
	}
	err = model.EditChannelByTag(channelTag.Tag, channelTag.NewTag, channelTag.ModelMapping, channelTag.Models, channelTag.Groups, channelTag.Priority, channelTag.Weight, channelTag.ParamOverride, channelTag.HeaderOverride)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

type ChannelBatch struct {
	Ids []int   `json:"ids"`
	Tag *string `json:"tag"`
}

func DeleteChannelBatch(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchDeleteChannels(channelBatch.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

type PatchChannel struct {
	model.Channel
	MultiKeyMode *string `json:"multi_key_mode"`
	KeyMode      *string `json:"key_mode"` // 多key模式下密钥覆盖或者追加
}

func UpdateChannel(c *gin.Context) {
	channel := PatchChannel{}
	err := c.ShouldBindJSON(&channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 使用统一的校验函数
	if err := validateChannel(&channel.Channel, false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// Preserve existing ChannelInfo to ensure multi-key channels keep correct state even if the client does not send ChannelInfo in the request.
	originChannel, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	targetType := channel.Type
	if targetType == 0 {
		targetType = originChannel.Type
	}
	targetKey := channel.Key
	if strings.TrimSpace(targetKey) == "" {
		targetKey = originChannel.Key
	}
	if targetType == constant.ChannelTypeCodex && strings.TrimSpace(targetKey) != "" {
		if err := validateCodexChannelCredential(targetKey); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	if channel.Status == common.ChannelStatusEnabled && targetType == constant.ChannelTypeCodex {
		candidateChannel := *originChannel
		candidateChannel.Type = targetType
		candidateChannel.Key = targetKey
		if isCodexChannelWithoutAccounts(&candidateChannel) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "请先通过账号管理导入 Codex 账号凭证后再启用渠道",
			})
			return
		}
	}

	// Always copy the original ChannelInfo so that fields like IsMultiKey and MultiKeySize are retained.
	channel.ChannelInfo = originChannel.ChannelInfo
	channel.CostPerMillion = nil

	// If the request explicitly specifies a new MultiKeyMode, apply it on top of the original info.
	if channel.MultiKeyMode != nil && *channel.MultiKeyMode != "" {
		channel.ChannelInfo.MultiKeyMode = constant.MultiKeyMode(*channel.MultiKeyMode)
	}

	// 处理多key模式下的密钥追加/覆盖逻辑
	if channel.KeyMode != nil && channel.ChannelInfo.IsMultiKey {
		switch *channel.KeyMode {
		case "append":
			// 追加模式：将新密钥添加到现有密钥列表
			if originChannel.Key != "" {
				var newKeys []string
				var existingKeys []string

				// 解析现有密钥
				if strings.HasPrefix(strings.TrimSpace(originChannel.Key), "[") {
					// JSON数组格式
					var arr []json.RawMessage
					if err := json.Unmarshal([]byte(strings.TrimSpace(originChannel.Key)), &arr); err == nil {
						existingKeys = make([]string, len(arr))
						for i, v := range arr {
							existingKeys[i] = string(v)
						}
					}
				} else {
					// 换行分隔格式
					existingKeys = strings.Split(strings.Trim(originChannel.Key, "\n"), "\n")
				}

				// 处理 Vertex AI 的特殊情况
				if channel.Type == constant.ChannelTypeVertexAi && channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
					// 尝试解析新密钥为JSON数组
					if strings.HasPrefix(strings.TrimSpace(channel.Key), "[") {
						array, err := getVertexArrayKeys(channel.Key)
						if err != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": "追加密钥解析失败: " + err.Error(),
							})
							return
						}
						newKeys = array
					} else {
						// 单个JSON密钥
						newKeys = []string{channel.Key}
					}
				} else {
					// 普通渠道的处理
					inputKeys := strings.Split(channel.Key, "\n")
					for _, key := range inputKeys {
						key = strings.TrimSpace(key)
						if key != "" {
							newKeys = append(newKeys, key)
						}
					}
				}

				seen := make(map[string]struct{}, len(existingKeys)+len(newKeys))
				for _, key := range existingKeys {
					normalized := strings.TrimSpace(key)
					if normalized == "" {
						continue
					}
					seen[normalized] = struct{}{}
				}
				dedupedNewKeys := make([]string, 0, len(newKeys))
				for _, key := range newKeys {
					normalized := strings.TrimSpace(key)
					if normalized == "" {
						continue
					}
					if _, ok := seen[normalized]; ok {
						continue
					}
					seen[normalized] = struct{}{}
					dedupedNewKeys = append(dedupedNewKeys, normalized)
				}

				allKeys := append(existingKeys, dedupedNewKeys...)
				channel.Key = strings.Join(allKeys, "\n")
			}
		case "replace":
			// 覆盖模式：直接使用新密钥（默认行为，不需要特殊处理）
		}
	}
	err = channel.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	channel.Key = ""
	clearChannelInfo(&channel.Channel)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

func FetchModels(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		Type    int    `json:"type"`
		Key     string `json:"key"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
		})
		return
	}

	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[req.Type]
	}

	// remove line breaks and extra spaces.
	key := strings.TrimSpace(req.Key)
	key = strings.Split(key, "\n")[0]

	if req.Type == constant.ChannelTypeOllama {
		models, err := ollama.FetchOllamaModels(baseURL, key)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("获取Ollama模型失败: %s", err.Error()),
			})
			return
		}

		names := make([]string, 0, len(models))
		for _, modelInfo := range models {
			names = append(names, modelInfo.Name)
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    names,
		})
		return
	}

	if req.Type == constant.ChannelTypeGemini {
		models, err := gemini.FetchGeminiModels(baseURL, key, "")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("获取Gemini模型失败: %s", err.Error()),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    models,
		})
		return
	}

	client := &http.Client{}
	url := fmt.Sprintf("%s/v1/models", baseURL)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	request.Header.Set("Authorization", "Bearer "+key)

	response, err := client.Do(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	//check status code
	if response.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch models",
		})
		return
	}
	defer response.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var models []string
	for _, model := range result.Data {
		models = append(models, model.ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    models,
	})
}

func BatchSetChannelTag(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchSetChannelTag(channelBatch.Ids, channelBatch.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

func GetTagModels(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}

	channels, err := model.GetChannelsByTag(tag, false, false) // idSort=false, selectAll=false
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var longestModels string
	maxLength := 0

	// Find the longest models string among all channels with the given tag
	for _, channel := range channels {
		if channel.Models != "" {
			currentModels := strings.Split(channel.Models, ",")
			if len(currentModels) > maxLength {
				maxLength = len(currentModels)
				longestModels = channel.Models
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    longestModels,
	})
	return
}

// CopyChannel handles cloning an existing channel with its key.
// POST /api/channel/copy/:id
// Optional query params:
//
//	suffix         - string appended to the original name (default "_复制")
//	reset_balance  - bool, when true will reset balance & used_quota to 0 (default true)
func CopyChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid id"})
		return
	}

	suffix := c.DefaultQuery("suffix", "_复制")
	resetBalance := true
	if rbStr := c.DefaultQuery("reset_balance", "true"); rbStr != "" {
		if v, err := strconv.ParseBool(rbStr); err == nil {
			resetBalance = v
		}
	}

	// fetch original channel with key
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		common.SysError("failed to get channel by id: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道信息失败，请稍后重试"})
		return
	}

	// clone channel
	clone := *origin // shallow copy is sufficient as we will overwrite primitives
	clone.Id = 0     // let DB auto-generate
	clone.CreatedTime = common.GetTimestamp()
	clone.Name = origin.Name + suffix
	clone.TestTime = 0
	clone.ResponseTime = 0
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
	}

	// insert
	if err := model.BatchInsertChannels([]model.Channel{clone}); err != nil {
		common.SysError("failed to clone channel: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "复制渠道失败，请稍后重试"})
		return
	}
	model.InitChannelCache()
	// success
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": clone.Id}})
}

// MultiKeyManageRequest represents the request for multi-key management operations
type MultiKeyManageRequest struct {
	ChannelId int    `json:"channel_id"`
	Action    string `json:"action"`              // "disable_key", "enable_key", "delete_key", "delete_disabled_keys", "get_key_status"
	KeyIndex  *int   `json:"key_index,omitempty"` // for disable_key, enable_key, and delete_key actions
	Model     string `json:"model,omitempty"`     // for capability probe actions
	Page      int    `json:"page,omitempty"`      // for get_key_status pagination
	PageSize  int    `json:"page_size,omitempty"` // for get_key_status pagination
	Status    *int   `json:"status,omitempty"`    // for get_key_status filtering: 1=enabled, 2=manual_disabled, 3=auto_disabled, nil=all
}

// MultiKeyStatusResponse represents the response for key status query
type MultiKeyStatusResponse struct {
	Keys       []KeyStatus `json:"keys"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
	// Statistics
	EnabledCount        int `json:"enabled_count"`
	ManualDisabledCount int `json:"manual_disabled_count"`
	AutoDisabledCount   int `json:"auto_disabled_count"`
}

type KeyStatus struct {
	Index        int                             `json:"index"`
	Status       int                             `json:"status"` // 1: enabled, 2: disabled
	DisabledTime int64                           `json:"disabled_time,omitempty"`
	Reason       string                          `json:"reason,omitempty"`
	KeyPreview   string                          `json:"key_preview"` // first 10 chars of key for identification
	Capabilities *model.ChannelAccountCapability `json:"capabilities,omitempty"`
}

func keyStatusCapabilities(channel *model.Channel, index int) *model.ChannelAccountCapability {
	if channel == nil {
		return nil
	}
	capability, ok := channel.ChannelInfo.AccountCapability(index)
	if !ok {
		return nil
	}
	capability.CapabilityClassification = capability.EffectiveClassification()
	return &capability
}

// ManageMultiKeys handles multi-key management operations
func ManageMultiKeys(c *gin.Context) {
	request := MultiKeyManageRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(request.ChannelId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "渠道不存在",
		})
		return
	}

	if !channel.ChannelInfo.IsMultiKey && request.Action != "probe_key_capabilities" && request.Action != "probe_all_key_capabilities" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该渠道不是多密钥模式",
		})
		return
	}

	switch request.Action {
	case "probe_key_capabilities":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要检测的账号索引",
			})
			return
		}
		result, err := probeChannelAccountCapabilities(c, channel, *request.KeyIndex, request.Model)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "账号权限检测完成",
			"data":    result,
		})
		return
	case "diagnose_platform_key_capabilities":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要诊断的账号索引",
			})
			return
		}
		result, err := probePlatformAccountCapabilities(c, channel, *request.KeyIndex, request.Model)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Platform API 诊断完成",
			"data":    result,
		})
		return
	case "probe_all_key_capabilities":
		keys := channel.GetKeys()
		results := make([]accountCapabilityProbeResult, 0, len(keys))
		for index := range keys {
			result, err := probeChannelAccountCapabilities(c, channel, index, request.Model)
			if err != nil {
				result = accountCapabilityProbeResult{
					Index: index,
					Capabilities: model.ChannelAccountCapability{
						CheckedTime:  common.GetTimestamp(),
						LastEndpoint: "",
						LastMessage:  err.Error(),
					},
				}
			}
			results = append(results, result)
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "账号权限检测完成",
			"data":    results,
		})
		return
	}

	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	switch request.Action {
	case "get_key_status":
		keys := channel.GetKeys()

		// Default pagination parameters
		page := request.Page
		pageSize := request.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 50 // Default page size
		}

		// Statistics for all keys (unchanged by filtering)
		var enabledCount, manualDisabledCount, autoDisabledCount int

		// Build all key status data first
		var allKeyStatusList []KeyStatus
		for i, key := range keys {
			status := 1 // default enabled
			var disabledTime int64
			var reason string

			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// Count for statistics (all keys)
			switch status {
			case 1:
				enabledCount++
			case 2:
				manualDisabledCount++
			case 3:
				autoDisabledCount++
			}

			if status != 1 {
				if channel.ChannelInfo.MultiKeyDisabledTime != nil {
					disabledTime = channel.ChannelInfo.MultiKeyDisabledTime[i]
				}
				if channel.ChannelInfo.MultiKeyDisabledReason != nil {
					reason = channel.ChannelInfo.MultiKeyDisabledReason[i]
				}
			}

			// Create key preview (first 10 chars)
			keyPreview := key
			if len(key) > 10 {
				keyPreview = key[:10] + "..."
			}

			allKeyStatusList = append(allKeyStatusList, KeyStatus{
				Index:        i,
				Status:       status,
				DisabledTime: disabledTime,
				Reason:       reason,
				KeyPreview:   keyPreview,
				Capabilities: keyStatusCapabilities(channel, i),
			})
		}

		// Apply status filter if specified
		var filteredKeyStatusList []KeyStatus
		if request.Status != nil {
			for _, keyStatus := range allKeyStatusList {
				if keyStatus.Status == *request.Status {
					filteredKeyStatusList = append(filteredKeyStatusList, keyStatus)
				}
			}
		} else {
			filteredKeyStatusList = allKeyStatusList
		}

		// Calculate pagination based on filtered results
		filteredTotal := len(filteredKeyStatusList)
		totalPages := (filteredTotal + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}

		// Calculate range for current page
		start := (page - 1) * pageSize
		end := start + pageSize
		if end > filteredTotal {
			end = filteredTotal
		}

		// Get the page data
		var pageKeyStatusList []KeyStatus
		if start < filteredTotal {
			pageKeyStatusList = filteredKeyStatusList[start:end]
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": MultiKeyStatusResponse{
				Keys:                pageKeyStatusList,
				Total:               filteredTotal, // Total of filtered results
				Page:                page,
				PageSize:            pageSize,
				TotalPages:          totalPages,
				EnabledCount:        enabledCount,        // Overall statistics
				ManualDisabledCount: manualDisabledCount, // Overall statistics
				AutoDisabledCount:   autoDisabledCount,   // Overall statistics
			},
		})
		return

	case "disable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要禁用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		channel.ChannelInfo.MultiKeyStatusList[keyIndex] = 2 // disabled

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已禁用",
		})
		return

	case "enable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要启用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		// 从状态列表中删除该密钥的记录，使其回到默认启用状态
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			delete(channel.ChannelInfo.MultiKeyStatusList, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledTime, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledReason, keyIndex)
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已启用",
		})
		return

	case "enable_all_keys":
		// 清空所有禁用状态，使所有密钥回到默认启用状态
		var enabledCount int
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			enabledCount = len(channel.ChannelInfo.MultiKeyStatusList)
		}

		channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已启用 %d 个密钥", enabledCount),
		})
		return

	case "disable_all_keys":
		// 禁用所有启用的密钥
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		var disabledCount int
		for i := 0; i < channel.ChannelInfo.MultiKeySize; i++ {
			status := 1 // default enabled
			if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
				status = s
			}

			// 只禁用当前启用的密钥
			if status == 1 {
				channel.ChannelInfo.MultiKeyStatusList[i] = 2 // disabled
				disabledCount++
			}
		}

		if disabledCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有可禁用的密钥",
			})
			return
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已禁用 %d 个密钥", disabledCount),
		})
		return

	case "delete_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要删除的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		keys := channel.GetKeys()
		var remainingKeys []string
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			// 跳过要删除的密钥
			if i == keyIndex {
				continue
			}

			remainingKeys = append(remainingKeys, key)

			// 保留其他密钥的状态信息，重新索引
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if status, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists && status != 1 {
					newStatusList[newIndex] = status
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledTime != nil {
				if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
					newDisabledTime[newIndex] = t
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledReason != nil {
				if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
					newDisabledReason[newIndex] = r
				}
			}
			newIndex++
		}

		if len(remainingKeys) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "不能删除最后一个密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已删除",
		})
		return

	case "delete_disabled_keys":
		keys := channel.GetKeys()
		var remainingKeys []string
		var deletedCount int
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			status := 1 // default enabled
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// 只删除自动禁用（status == 3）的密钥，保留启用（status == 1）和手动禁用（status == 2）的密钥
			if status == 3 {
				deletedCount++
			} else {
				remainingKeys = append(remainingKeys, key)
				// 保留非自动禁用密钥的状态信息，重新索引
				if status != 1 {
					newStatusList[newIndex] = status
					if channel.ChannelInfo.MultiKeyDisabledTime != nil {
						if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
							newDisabledTime[newIndex] = t
						}
					}
					if channel.ChannelInfo.MultiKeyDisabledReason != nil {
						if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
							newDisabledReason[newIndex] = r
						}
					}
				}
				newIndex++
			}
		}

		if deletedCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有需要删除的自动禁用密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已删除 %d 个自动禁用的密钥", deletedCount),
			"data":    deletedCount,
		})
		return

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的操作",
		})
		return
	}
}

// OllamaPullModel 拉取 Ollama 模型
func OllamaPullModel(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	err = ollama.PullOllamaModel(baseURL, key, req.ModelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to pull model: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Model %s pulled successfully", req.ModelName),
	})
}

// OllamaPullModelStream 流式拉取 Ollama 模型
func OllamaPullModelStream(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	// 设置 SSE 头部
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	key := strings.Split(channel.Key, "\n")[0]

	// 创建进度回调函数
	progressCallback := func(progress ollama.OllamaPullResponse) {
		data, _ := json.Marshal(progress)
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
		c.Writer.Flush()
	}

	// 执行拉取
	err = ollama.PullOllamaModelStream(baseURL, key, req.ModelName, progressCallback)

	if err != nil {
		errorData, _ := json.Marshal(gin.H{
			"error": err.Error(),
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(errorData))
	} else {
		successData, _ := json.Marshal(gin.H{
			"message": fmt.Sprintf("Model %s pulled successfully", req.ModelName),
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(successData))
	}

	// 发送结束标志
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}

// OllamaDeleteModel 删除 Ollama 模型
func OllamaDeleteModel(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	err = ollama.DeleteOllamaModel(baseURL, key, req.ModelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to delete model: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Model %s deleted successfully", req.ModelName),
	})
}

// OllamaVersion 获取 Ollama 服务版本信息
func OllamaVersion(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid channel id",
		})
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	version, err := ollama.FetchOllamaVersion(baseURL, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取Ollama版本失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version": version,
		},
	})
}
