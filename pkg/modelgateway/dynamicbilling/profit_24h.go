package dynamicbilling

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaytraffic "github.com/QuantumNous/new-api/pkg/modelgateway/traffic"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"gorm.io/gorm"
)

const profitMonitorConfigOptionKey = "model_gateway.profit_monitor.config"

type profit24hMonitorConfig struct {
	ServerDailyCostUSD            float64 `json:"server_daily_cost_usd"`
	TrafficCostPerGBUSD           float64 `json:"traffic_cost_per_gb_usd"`
	TrafficEstimationEnabled      bool    `json:"traffic_estimation_enabled"`
	TrafficEstimatedBytesPerToken int     `json:"traffic_estimated_bytes_per_token"`
	ResourceCostEnabled           bool    `json:"resource_cost_enabled"`
	DynamicRatioMinLimit          float64 `json:"dynamic_ratio_min_limit"`
	DynamicRatioMaxLimit          float64 `json:"dynamic_ratio_max_limit"`
	DynamicRatioFixedValue        float64 `json:"dynamic_ratio_fixed_value"`
}

type profit24hGroupAccumulator struct {
	Group                string
	ReferenceModel       string
	SampleCount          int
	ModelSet             map[string]struct{}
	RequestCount         int64
	SuccessRequestCount  int64
	TotalTokens          int64
	BaseQuotaAtRatio1    float64
	UpstreamCostUSD      float64
	CostMultiplier       float64
	CostMultiplierWeight float64
	TrafficCostUSD       float64
	TrafficEstimated     bool
	TrafficDataReady     bool
	ServerCostUSD        float64
	ResourceCostUSD      float64
	OperatingCostUSD     float64
	RequiredRevenueUSD   float64
	TargetRatio          float64
	EffectiveRatio       float64
	Clamped              bool
	PendingManualConfirm bool
	FallbackReason       string
	ApplyReason          string
	LatestCalculatedAt   int64
}

type profit24hUsageGroupRow struct {
	Group           string  `gorm:"column:billing_group"`
	Requests        int64   `gorm:"column:requests"`
	SuccessRequests int64   `gorm:"column:success_requests"`
	TotalTokens     int64   `gorm:"column:total_tokens"`
	BillingQuota    int64   `gorm:"column:billing_quota"`
	UpstreamCostUSD float64 `gorm:"column:upstream_cost_usd"`
}

func buildProfit24hRatioBaselines(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64, filter SnapshotFilter) (map[string]RatioBaseline, error) {
	if db == nil || logDB == nil {
		return map[string]RatioBaseline{}, nil
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}
	windowHours := setting.DynamicBillingProfitWindowHours
	if windowHours <= 0 {
		windowHours = scheduler_setting.DefaultSetting().DynamicBillingProfitWindowHours
	}
	windowStart := now - int64(windowHours)*3600
	if filter.MinCalculatedAt > windowStart {
		windowStart = filter.MinCalculatedAt
	}
	if windowStart > now {
		windowStart = now - 86400
	}

	config := loadProfit24hMonitorConfig()
	accumulators, err := loadProfit24hCostAccumulators(db, logDB, windowStart, now)
	if err != nil {
		return nil, err
	}
	usageRows, err := loadProfit24hUsageGroups(db, windowStart, now)
	if err != nil {
		return nil, err
	}
	for _, row := range usageRows {
		group := strings.TrimSpace(row.Group)
		if group == "" {
			continue
		}
		accumulator := ensureProfit24hAccumulator(accumulators, group)
		accumulator.RequestCount = row.Requests
		accumulator.SuccessRequestCount = row.SuccessRequests
		if row.TotalTokens > accumulator.TotalTokens {
			accumulator.TotalTokens = row.TotalTokens
		}
	}
	applyProfit24hOperatingCosts(db, accumulators, windowStart, now, config)

	previous := loadPersistedBaselines(db)
	result := make(map[string]RatioBaseline, len(accumulators))
	for _, accumulator := range accumulators {
		if accumulator == nil || strings.TrimSpace(accumulator.Group) == "" {
			continue
		}
		groupKey := groupCacheKey(accumulator.Group)
		finalizeProfit24hAccumulator(accumulator, setting, previous[groupKey], config)
		ratio := accumulator.EffectiveRatio
		if ratio <= 0 {
			ratio = ratio_setting.GetGroupRatio(accumulator.Group)
		}
		if ratio <= 0 {
			continue
		}
		baseline := RatioBaseline{
			RequestedModel:       accumulator.ReferenceModel,
			ReferenceModel:       accumulator.ReferenceModel,
			Group:                accumulator.Group,
			Ratio:                ratio,
			PricePerM:            requestedModelPricePerMillion(accumulator.ReferenceModel, ratio),
			SampleCount:          accumulator.SampleCount,
			ModelCount:           len(accumulator.ModelSet),
			CalculatedAt:         now,
			WindowStart:          windowStart,
			WindowEnd:            now,
			ProfitRate:           sanitizeProfitRate(setting.DynamicBillingProfitRate),
			CostSource:           scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:            normalizeApplyMode(setting.DynamicBillingApplyMode),
			ApplyReason:          accumulator.ApplyReason,
			OperatingCostUSD:     accumulator.OperatingCostUSD,
			RequiredRevenueUSD:   accumulator.RequiredRevenueUSD,
			BaseQuotaAtRatio1:    accumulator.BaseQuotaAtRatio1,
			CostMultiplier:       accumulator.CostMultiplier,
			TargetRatio:          accumulator.TargetRatio,
			EffectiveRatio:       accumulator.EffectiveRatio,
			Clamped:              accumulator.Clamped,
			PendingManualConfirm: accumulator.PendingManualConfirm,
			FallbackReason:       accumulator.FallbackReason,
			RequestCount:         accumulator.RequestCount,
			SuccessRequestCount:  accumulator.SuccessRequestCount,
			TotalTokens:          accumulator.TotalTokens,
			TrafficCostUSD:       accumulator.TrafficCostUSD,
			TrafficEstimated:     accumulator.TrafficEstimated,
			TrafficDataReady:     accumulator.TrafficDataReady,
			ServerCostUSD:        accumulator.ServerCostUSD,
			ResourceCostUSD:      accumulator.ResourceCostUSD,
			UpstreamCostUSD:      accumulator.UpstreamCostUSD,
		}
		result[groupCacheKey(accumulator.Group)] = baseline
	}
	return result, nil
}

func loadProfit24hCostAccumulators(db *gorm.DB, logDB *gorm.DB, windowStart int64, windowEnd int64) (map[string]*profit24hGroupAccumulator, error) {
	costRows := make([]costRow, 0)
	err := db.Model(&model.ModelGatewayRequestCostSummary{}).
		Select("id, request_id, upstream_cost_total AS cost, breakdown_json, calculated_at").
		Where("upstream_cost_total > 0").
		Where("calculated_at >= ? AND calculated_at <= ?", windowStart, windowEnd).
		Order("calculated_at desc, id desc").
		Find(&costRows).Error
	if err != nil {
		return nil, err
	}
	pairs, err := loadEligibleRequestCostLogPairs(logDB, costRows)
	if err != nil {
		return nil, err
	}
	accumulators := make(map[string]*profit24hGroupAccumulator)
	for _, pair := range pairs {
		log := pair.Log
		group := strings.TrimSpace(log.Group)
		modelName := strings.TrimSpace(log.ModelName)
		if group == "" || modelName == "" || log.Quota <= 0 || pair.Cost <= 0 {
			continue
		}
		other := parseOther(log.Other)
		groupRatio := floatMapValue(other, "group_ratio")
		if groupRatio <= 0 {
			groupRatio = ratio_setting.GetGroupRatio(group)
		}
		baseQuotaAtRatio1 := pair.BaseQuotaAtRatio1
		if baseQuotaAtRatio1 <= 0 {
			if groupRatio <= 0 {
				continue
			}
			baseQuotaAtRatio1 = logBaseQuotaAtRatio1(log, other)
		}
		if baseQuotaAtRatio1 <= 0 {
			continue
		}
		accumulator := ensureProfit24hAccumulator(accumulators, group)
		accumulator.SampleCount++
		accumulator.ModelSet[modelName] = struct{}{}
		if accumulator.ReferenceModel == "" || pair.CalculatedAt >= accumulator.LatestCalculatedAt {
			accumulator.ReferenceModel = modelName
			accumulator.LatestCalculatedAt = pair.CalculatedAt
		}
		accumulator.SuccessRequestCount++
		accumulator.TotalTokens += int64(log.PromptTokens + log.CompletionTokens)
		accumulator.BaseQuotaAtRatio1 += baseQuotaAtRatio1
		accumulator.UpstreamCostUSD += pair.Cost
		if pair.CostMultiplier > 0 && pair.CostMultiplierWeight > 0 {
			accumulator.CostMultiplier += pair.CostMultiplier * pair.CostMultiplierWeight
			accumulator.CostMultiplierWeight += pair.CostMultiplierWeight
		}
	}
	return accumulators, nil
}

func ensureProfit24hAccumulator(accumulators map[string]*profit24hGroupAccumulator, group string) *profit24hGroupAccumulator {
	group = strings.TrimSpace(group)
	accumulator := accumulators[group]
	if accumulator == nil {
		accumulator = &profit24hGroupAccumulator{
			Group:    group,
			ModelSet: map[string]struct{}{},
		}
		accumulators[group] = accumulator
	}
	return accumulator
}

func loadProfit24hUsageGroups(db *gorm.DB, windowStart int64, windowEnd int64) ([]profit24hUsageGroupRow, error) {
	if db == nil {
		return nil, nil
	}
	groupExpr := "COALESCE(NULLIF(selected_group, ''), requested_group, '')"
	rows := make([]profit24hUsageGroupRow, 0)
	err := profit24hUsageBaseQuery(db, windowStart, windowEnd).
		Select(groupExpr+" AS billing_group, "+
			"COUNT(*) AS requests, "+
			"COALESCE(SUM(CASE WHEN success = ? THEN 1 ELSE 0 END), 0) AS success_requests, "+
			"COALESCE(SUM(CASE WHEN success = ? THEN total_tokens ELSE 0 END), 0) AS total_tokens, "+
			"COALESCE(SUM(CASE WHEN success = ? THEN quota ELSE 0 END), 0) AS billing_quota, "+
			"COALESCE(SUM(CASE WHEN success = ? THEN upstream_cost_total ELSE 0 END), 0) AS upstream_cost_usd", true, true, true, true).
		Group(groupExpr).
		Scan(&rows).Error
	return rows, err
}

func profit24hUsageBaseQuery(db *gorm.DB, windowStart int64, windowEnd int64) *gorm.DB {
	return db.Model(&model.ChannelAccountUsageEvent{}).
		Where("is_health_probe = ?", false).
		Where(
			"((completed_at >= ? AND completed_at <= ?) OR (completed_at <= ? AND updated_at >= ? AND updated_at <= ?) OR (completed_at <= ? AND updated_at <= ? AND created_at >= ? AND created_at <= ?))",
			windowStart, windowEnd,
			0, windowStart, windowEnd,
			0, 0, windowStart, windowEnd,
		).
		Where("(quota <> ? OR upstream_cost_total > ? OR total_tokens > ? OR success = ? OR status_code <> ? OR error_category <> ?)", 0, 0, 0, true, 0, "")
}

func applyProfit24hOperatingCosts(db *gorm.DB, accumulators map[string]*profit24hGroupAccumulator, windowStart int64, windowEnd int64, config profit24hMonitorConfig) {
	if len(accumulators) == 0 {
		return
	}
	totalBaseQuota := 0.0
	totalSuccessRequests := int64(0)
	for _, accumulator := range accumulators {
		totalBaseQuota += accumulator.BaseQuotaAtRatio1
		totalSuccessRequests += accumulator.SuccessRequestCount
	}
	trafficByGroup, hasTrafficBreakdown := loadProfit24hTrafficCosts(windowStart, windowEnd, config)
	serverWindowCost := config.ServerDailyCostUSD * math.Max(0, float64(windowEnd-windowStart)) / 86400
	resourceCosts := loadProfit24hResourceCosts(db, windowStart, windowEnd, config.ResourceCostEnabled)
	for _, accumulator := range accumulators {
		if accumulator == nil {
			continue
		}
		accumulator.TrafficDataReady = config.TrafficCostPerGBUSD <= 0
		if hasTrafficBreakdown {
			if cost, ok := trafficByGroup[groupCacheKey(accumulator.Group)]; ok {
				accumulator.TrafficCostUSD = cost
				accumulator.TrafficDataReady = true
			}
		}
		if !accumulator.TrafficDataReady && config.TrafficEstimationEnabled && config.TrafficEstimatedBytesPerToken > 0 {
			accumulator.TrafficEstimated = true
			accumulator.TrafficDataReady = true
			estimatedBytes := accumulator.TotalTokens * int64(config.TrafficEstimatedBytesPerToken)
			accumulator.TrafficCostUSD = trafficCostUSD(estimatedBytes, config.TrafficCostPerGBUSD)
		}
		baseShare := ratioOrZero(accumulator.BaseQuotaAtRatio1, totalBaseQuota)
		requestShare := ratioOrZero(float64(accumulator.SuccessRequestCount), float64(totalSuccessRequests))
		accumulator.ServerCostUSD = serverWindowCost * baseShare
		for _, resource := range resourceCosts {
			if resource.totalCost <= 0 {
				continue
			}
			scope := model.NormalizeModelGatewayProfitResourceScope(resource.row.ScopeType)
			if scope == model.ModelGatewayProfitResourceScopeGroup {
				if resourceMatchesProfit24hGroup(resource.row, accumulator.Group) {
					accumulator.ResourceCostUSD += resource.totalCost
				}
				continue
			}
			share := baseShare
			if model.NormalizeModelGatewayProfitResourceAllocationMode(resource.row.AllocationMode) == model.ModelGatewayProfitResourceAllocationRequest {
				share = requestShare
			}
			accumulator.ResourceCostUSD += resource.totalCost * share
		}
		accumulator.OperatingCostUSD = accumulator.UpstreamCostUSD + accumulator.TrafficCostUSD + accumulator.ServerCostUSD + accumulator.ResourceCostUSD
	}
}

func finalizeProfit24hAccumulator(accumulator *profit24hGroupAccumulator, setting scheduler_setting.SchedulerSetting, previous RatioBaseline, config profit24hMonitorConfig) {
	if accumulator == nil {
		return
	}
	if accumulator.RequestCount <= 0 && accumulator.SampleCount > 0 {
		accumulator.RequestCount = int64(accumulator.SampleCount)
	}
	if accumulator.SuccessRequestCount <= 0 && accumulator.SampleCount > 0 {
		accumulator.SuccessRequestCount = int64(accumulator.SampleCount)
	}
	profitRate := SanitizeTargetGrossMargin(setting.DynamicBillingProfitRate)
	minRatio := setting.DynamicBillingMinRatio
	if minRatio <= 0 {
		minRatio = scheduler_setting.DefaultSetting().DynamicBillingMinRatio
	}
	maxRatio := setting.DynamicBillingMaxRatio
	if maxRatio <= 0 {
		maxRatio = scheduler_setting.DefaultSetting().DynamicBillingMaxRatio
	}
	if maxRatio < minRatio {
		maxRatio = minRatio
	}
	if config.DynamicRatioMinLimit > 0 && config.DynamicRatioMinLimit > minRatio {
		minRatio = config.DynamicRatioMinLimit
	}
	if config.DynamicRatioMaxLimit > 0 && config.DynamicRatioMaxLimit < maxRatio {
		maxRatio = config.DynamicRatioMaxLimit
	}
	if maxRatio < minRatio {
		maxRatio = minRatio
	}
	if accumulator.CostMultiplierWeight > 0 {
		accumulator.CostMultiplier = accumulator.CostMultiplier / accumulator.CostMultiplierWeight
	}

	switch {
	case accumulator.UpstreamCostUSD <= 0:
		accumulator.FallbackReason = FallbackNoCostData
	case accumulator.BaseQuotaAtRatio1 <= 0:
		accumulator.FallbackReason = FallbackMissingBaseQuota
	default:
		costUSD := accumulator.OperatingCostUSD
		if costUSD <= 0 {
			costUSD = accumulator.UpstreamCostUSD
		}
		accumulator.RequiredRevenueUSD = RequiredRevenueForGrossMargin(costUSD, profitRate)
		accumulator.TargetRatio = accumulator.RequiredRevenueUSD * common.QuotaPerUnit / accumulator.BaseQuotaAtRatio1
		accumulator.EffectiveRatio = clampFloat(accumulator.TargetRatio, minRatio, maxRatio)
		accumulator.Clamped = math.Abs(accumulator.EffectiveRatio-accumulator.TargetRatio) > 0.0000001
	}
	if accumulator.RequestCount < int64(setting.DynamicBillingMinRequests) ||
		accumulator.SuccessRequestCount < int64(setting.DynamicBillingMinSuccessRequests) ||
		accumulator.TotalTokens < int64(setting.DynamicBillingMinTokens) {
		accumulator.FallbackReason = FallbackInsufficientUsage
	}
	if accumulator.BaseQuotaAtRatio1 <= 0 {
		accumulator.FallbackReason = FallbackMissingBaseQuota
	}
	stepChange := setting.DynamicBillingMaxStepChange
	if stepChange <= 0 {
		stepChange = scheduler_setting.DefaultSetting().DynamicBillingMaxStepChange
	}
	stepChangeExceeded := accumulator.FallbackReason == "" &&
		accumulator.EffectiveRatio > 0 &&
		previous.Ratio > 0 &&
		ratioOrZero(math.Abs(accumulator.EffectiveRatio-previous.Ratio), previous.Ratio) > stepChange
	switch normalizeApplyMode(setting.DynamicBillingApplyMode) {
	case scheduler_setting.DynamicBillingApplyModeObserve:
		if accumulator.FallbackReason == "" {
			accumulator.FallbackReason = FallbackObserveMode
		}
	case scheduler_setting.DynamicBillingApplyModeManual:
		if accumulator.FallbackReason == "" {
			if stepChangeExceeded {
				accumulator.ApplyReason = ApplyReasonStepChangeAutoApplied
			} else {
				accumulator.ApplyReason = ApplyReasonManualModeAutoApplied
			}
		}
	default:
		if accumulator.FallbackReason == "" {
			if stepChangeExceeded {
				accumulator.ApplyReason = ApplyReasonStepChangeAutoApplied
			} else {
				accumulator.ApplyReason = ApplyReasonAutoApplied
			}
		}
	}
}

type profit24hResourceCost struct {
	row       model.ModelGatewayProfitResourceCost
	totalCost float64
}

func loadProfit24hResourceCosts(db *gorm.DB, windowStart int64, windowEnd int64, enabled bool) []profit24hResourceCost {
	if db == nil || !enabled {
		return nil
	}
	rows := make([]model.ModelGatewayProfitResourceCost, 0)
	if err := db.Model(&model.ModelGatewayProfitResourceCost{}).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		common.SysLog("model gateway dynamic billing resource cost load failed: " + err.Error())
		return nil
	}
	result := make([]profit24hResourceCost, 0, len(rows))
	for _, row := range rows {
		row.Normalize()
		total := profit24hResourceWindowCost(row, windowStart, windowEnd) + profit24hResourceWindowLoss(row, windowStart, windowEnd)
		if total <= 0 {
			continue
		}
		result = append(result, profit24hResourceCost{row: row, totalCost: total})
	}
	return result
}

func resourceMatchesProfit24hGroup(resource model.ModelGatewayProfitResourceCost, group string) bool {
	if model.NormalizeModelGatewayProfitResourceScope(resource.ScopeType) != model.ModelGatewayProfitResourceScopeGroup {
		return false
	}
	scopeKey := strings.TrimSpace(resource.ScopeKey)
	return scopeKey != "" && strings.EqualFold(scopeKey, strings.TrimSpace(group))
}

func profit24hResourceWindowCost(resource model.ModelGatewayProfitResourceCost, windowStart int64, windowEnd int64) float64 {
	if !resource.Enabled || resource.AmountUSD <= 0 || resource.PeriodSeconds <= 0 || windowEnd <= windowStart {
		return 0
	}
	activeStart := resource.AmortizeStartAt
	if activeStart <= 0 {
		activeStart = resource.CreatedAt
	}
	if activeStart <= 0 {
		activeStart = windowStart
	}
	activeEnd := resource.AmortizeEndAt
	if activeEnd <= 0 {
		activeEnd = windowEnd
	}
	overlapStart := maxInt64(windowStart, activeStart)
	overlapEnd := minInt64(windowEnd, activeEnd)
	if overlapEnd <= overlapStart {
		return 0
	}
	return resource.AmountUSD * float64(overlapEnd-overlapStart) / float64(resource.PeriodSeconds)
}

func profit24hResourceWindowLoss(resource model.ModelGatewayProfitResourceCost, windowStart int64, windowEnd int64) float64 {
	if !resource.Enabled || resource.LossAmountUSD <= 0 || resource.LossRecordedAt <= 0 {
		return 0
	}
	if resource.LossRecordedAt < windowStart || resource.LossRecordedAt > windowEnd {
		return 0
	}
	return resource.LossAmountUSD
}

func loadProfit24hTrafficCosts(windowStart int64, windowEnd int64, config profit24hMonitorConfig) (map[string]float64, bool) {
	result := map[string]float64{}
	if config.TrafficCostPerGBUSD <= 0 {
		return result, true
	}
	rows, err := modelgatewaytraffic.QueryBreakdown(windowStart, windowEnd, "group")
	if err != nil {
		common.SysLog("model gateway dynamic billing traffic cost load failed: " + err.Error())
		return result, false
	}
	totalBytes := int64(0)
	for _, row := range rows {
		if row.TotalBytes <= 0 {
			continue
		}
		totalBytes += row.TotalBytes
		result[groupCacheKey(row.DimensionKey)] = trafficCostUSD(row.TotalBytes, config.TrafficCostPerGBUSD)
	}
	return result, totalBytes > 0
}

func trafficCostUSD(bytes int64, costPerGB float64) float64 {
	if bytes <= 0 || costPerGB <= 0 {
		return 0
	}
	return float64(bytes) / 1024 / 1024 / 1024 * costPerGB
}

func loadProfit24hMonitorConfig() profit24hMonitorConfig {
	config := profit24hMonitorConfig{
		ResourceCostEnabled: true,
	}
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[profitMonitorConfigOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(raw) == "" {
		return config
	}
	if err := common.UnmarshalJsonStr(raw, &config); err != nil {
		return profit24hMonitorConfig{ResourceCostEnabled: true}
	}
	if config.ServerDailyCostUSD < 0 {
		config.ServerDailyCostUSD = 0
	}
	if config.TrafficCostPerGBUSD < 0 {
		config.TrafficCostPerGBUSD = 0
	}
	if config.TrafficEstimatedBytesPerToken < 0 {
		config.TrafficEstimatedBytesPerToken = 0
	}
	if config.DynamicRatioMinLimit < 0 {
		config.DynamicRatioMinLimit = 0
	}
	if config.DynamicRatioMaxLimit < 0 {
		config.DynamicRatioMaxLimit = 0
	}
	if config.DynamicRatioFixedValue < 0 {
		config.DynamicRatioFixedValue = 0
	}
	if config.DynamicRatioMinLimit > 100 {
		config.DynamicRatioMinLimit = 100
	}
	if config.DynamicRatioMaxLimit > 100 {
		config.DynamicRatioMaxLimit = 100
	}
	if config.DynamicRatioFixedValue > 100 {
		config.DynamicRatioFixedValue = 100
	}
	if config.DynamicRatioMaxLimit > 0 && config.DynamicRatioMinLimit > config.DynamicRatioMaxLimit {
		config.DynamicRatioMaxLimit = config.DynamicRatioMinLimit
	}
	return config
}

func sanitizeProfitRate(value float64) float64 {
	return SanitizeTargetGrossMargin(value)
}

func clampFloat(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func ratioOrZero(numerator float64, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
