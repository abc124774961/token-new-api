package controller

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	publicHomeStatusDefaultDays = 30
	publicHomeStatusMaxDays     = 30
	publicHomeStatusCacheTTL    = 2 * time.Minute
	publicHomeStatusErrorTTL    = 30 * time.Second
	publicHomeStatusStaleTTL    = 10 * time.Minute
	publicHomeDynamicBillingTTL = 30 * time.Second
	publicHomeGatewayStatsTTL   = 2 * time.Minute
	publicHomeGatewayStatsStale = 30 * time.Minute
	publicHomeGatewayEmptyTTL   = 5 * time.Second
	publicHomeGatewayEmptyStale = 30 * time.Second
	publicHomeGatewayQueryTTL   = 1800 * time.Millisecond
	publicHomeGatewayFirstWait  = 500 * time.Millisecond
	publicHomeGatewayStatsHours = 24
	publicHomeStatusLogQueryTTL = 900 * time.Millisecond
)

var publicHomeStatusCache = struct {
	sync.Mutex
	items    map[int]publicHomeStatusCacheItem
	inFlight map[int]chan struct{}
}{
	items:    make(map[int]publicHomeStatusCacheItem),
	inFlight: make(map[int]chan struct{}),
}

var publicHomeDynamicBillingCache = struct {
	sync.Mutex
	result    *PublicHomeDynamicBilling
	expiresAt time.Time
}{}

var publicHomeModelGatewayStatsCache = struct {
	sync.Mutex
	items    map[int]publicHomeModelGatewayStatsCacheItem
	inFlight map[int]chan struct{}
}{
	items:    make(map[int]publicHomeModelGatewayStatsCacheItem),
	inFlight: make(map[int]chan struct{}),
}

type publicHomeStatusCacheItem struct {
	result     PublicHomeStatusResponse
	expiresAt  time.Time
	staleUntil time.Time
}

type publicHomeModelGatewayStatsCacheItem struct {
	result     publicHomeModelGatewayStats
	expiresAt  time.Time
	staleUntil time.Time
}

type PublicHomeStatusSummary struct {
	Days            int     `json:"days"`
	SuccessRate     float64 `json:"success_rate"`
	AvgLatencyMs    int64   `json:"avg_latency_ms"`
	AvgTTFTMs       int64   `json:"avg_ttft_ms"`
	Requests        int64   `json:"requests"`
	EnabledChannels int     `json:"enabled_channels"`
	HealthyChannels int     `json:"healthy_channels"`
	ProtectedEvents int64   `json:"protected_events"`
}

type PublicHomeStatusDaily struct {
	Date            string  `json:"date"`
	Requests        int64   `json:"requests"`
	SuccessRate     float64 `json:"success_rate"`
	AvgLatencyMs    int64   `json:"avg_latency_ms"`
	AvgTTFTMs       int64   `json:"avg_ttft_ms"`
	ProtectedEvents int64   `json:"protected_events"`
}

type PublicHomeStatusResponse struct {
	Summary        PublicHomeStatusSummary   `json:"summary"`
	Daily          []PublicHomeStatusDaily   `json:"daily"`
	Groups         []PublicHomeStatusGroup   `json:"groups"`
	DynamicBilling *PublicHomeDynamicBilling `json:"dynamic_billing,omitempty"`
	UpdatedAt      int64                     `json:"updated_at"`
	Partial        bool                      `json:"partial,omitempty"`
}

type PublicHomeStatusGroup struct {
	Key     string                    `json:"key"`
	Name    string                    `json:"name"`
	Summary PublicHomeStatusSummary   `json:"summary"`
	Daily   []PublicHomeStatusDaily   `json:"daily"`
	States  PublicHomeStatusGroupMeta `json:"states"`
}

type PublicHomeStatusGroupMeta struct {
	Healthy  int `json:"healthy"`
	Cooling  int `json:"cooling"`
	Standby  int `json:"standby"`
	Channels int `json:"channels"`
}

type PublicHomeDynamicBilling struct {
	Enabled           bool                           `json:"enabled"`
	Status            string                         `json:"status,omitempty"`
	Group             string                         `json:"group,omitempty"`
	Model             string                         `json:"model,omitempty"`
	CurrentRatio      float64                        `json:"current_ratio,omitempty"`
	MinRatio7d        float64                        `json:"min_ratio_7d,omitempty"`
	MaxRatio7d        float64                        `json:"max_ratio_7d,omitempty"`
	DisplayPricePerM  float64                        `json:"display_price_per_m,omitempty"`
	Trend             *PublicHomeDynamicBillingTrend `json:"trend,omitempty"`
	UpdatedSecondsAgo int64                          `json:"updated_seconds_ago,omitempty"`
	RefreshSeconds    int                            `json:"refresh_seconds,omitempty"`
}

type PublicHomeDynamicBillingTrend struct {
	Today     PublicHomeDynamicBillingTrendSeries `json:"today"`
	Yesterday PublicHomeDynamicBillingTrendSeries `json:"yesterday"`
	SevenDays PublicHomeDynamicBillingTrendSeries `json:"seven_days"`
}

type PublicHomeDynamicBillingTrendSeries struct {
	Points      []PublicHomeDynamicBillingTrendPoint `json:"points"`
	MinRatio    float64                              `json:"min_ratio,omitempty"`
	MaxRatio    float64                              `json:"max_ratio,omitempty"`
	AvgRatio    float64                              `json:"avg_ratio,omitempty"`
	LatestRatio float64                              `json:"latest_ratio,omitempty"`
	SampleCount int                                  `json:"sample_count,omitempty"`
}

type PublicHomeDynamicBillingTrendPoint struct {
	Timestamp   int64   `json:"timestamp"`
	Ratio       float64 `json:"ratio,omitempty"`
	PricePerM   float64 `json:"price_per_m,omitempty"`
	SampleCount int     `json:"sample_count,omitempty"`
}

type publicHomeModelGatewayStats struct {
	Requests     int64
	Successes    int64
	SuccessRate  float64
	AvgLatencyMs int64
	AvgTTFTMs    int64
}

type publicHomeModelGatewayStatsAggRow struct {
	Requests     int64   `gorm:"column:requests"`
	Successes    int64   `gorm:"column:successes"`
	AvgLatencyMs float64 `gorm:"column:avg_latency_ms"`
	AvgTTFTMs    float64 `gorm:"column:avg_ttft_ms"`
}

func GetPublicHomeStatus(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=30, stale-while-revalidate=300")

	days := publicHomeStatusDefaultDays
	if raw := c.Query("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}

	days = normalizePublicHomeStatusDays(days)

	var result PublicHomeStatusResponse
	var err error
	var dynamicBilling *PublicHomeDynamicBilling
	var gatewayStats publicHomeModelGatewayStats
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		result, err = buildPublicHomeStatusCached(days)
	}()
	go func() {
		defer wg.Done()
		dynamicBilling = getCachedPublicHomeDynamicBilling()
	}()
	go func() {
		defer wg.Done()
		gatewayStats = getCachedPublicHomeModelGatewayStats()
	}()
	wg.Wait()

	if err != nil {
		common.ApiError(c, err)
		return
	}
	result.DynamicBilling = dynamicBilling
	applyPublicHomeModelGatewayStats(&result, gatewayStats)
	common.ApiSuccess(c, result)
}

func normalizePublicHomeStatusDays(days int) int {
	if days <= 7 {
		return 7
	}
	if days > publicHomeStatusMaxDays {
		return publicHomeStatusMaxDays
	}
	return publicHomeStatusDefaultDays
}

func buildPublicHomeStatusCached(days int) (PublicHomeStatusResponse, error) {
	days = normalizePublicHomeStatusDays(days)
	now := time.Now()

	publicHomeStatusCache.Lock()
	if cached, ok := publicHomeStatusCache.items[days]; ok && now.Before(cached.expiresAt) {
		result := cached.result
		publicHomeStatusCache.Unlock()
		return result, nil
	}
	if cached, ok := publicHomeStatusCache.items[days]; ok && now.Before(cached.staleUntil) {
		result := cached.result
		if _, refreshing := publicHomeStatusCache.inFlight[days]; !refreshing {
			done := make(chan struct{})
			publicHomeStatusCache.inFlight[days] = done
			go refreshPublicHomeStatusCache(days, done)
		}
		publicHomeStatusCache.Unlock()
		return result, nil
	}
	if done, ok := publicHomeStatusCache.inFlight[days]; ok {
		publicHomeStatusCache.Unlock()
		<-done
		publicHomeStatusCache.Lock()
		if cached, ok := publicHomeStatusCache.items[days]; ok {
			result := cached.result
			publicHomeStatusCache.Unlock()
			return result, nil
		}
		publicHomeStatusCache.Unlock()
		return buildPublicHomeStatusEmpty(days, true), nil
	}
	done := make(chan struct{})
	publicHomeStatusCache.inFlight[days] = done
	publicHomeStatusCache.Unlock()

	return refreshPublicHomeStatusCache(days, done)
}

func refreshPublicHomeStatusCache(days int, done chan struct{}) (PublicHomeStatusResponse, error) {
	result, err := buildPublicHomeStatus(days)
	cacheTTL := publicHomeStatusCacheTTL
	if err != nil {
		publicHomeStatusCache.Lock()
		if cached, ok := publicHomeStatusCache.items[days]; ok {
			result = cached.result
			result.Partial = true
			err = nil
		} else {
			result = buildPublicHomeStatusEmpty(days, true)
			err = nil
		}
		publicHomeStatusCache.Unlock()
		cacheTTL = publicHomeStatusErrorTTL
	} else if result.Partial {
		cacheTTL = publicHomeStatusErrorTTL
	}

	publicHomeStatusCache.Lock()
	now := time.Now()
	publicHomeStatusCache.items[days] = publicHomeStatusCacheItem{
		result:     result,
		expiresAt:  now.Add(cacheTTL),
		staleUntil: now.Add(publicHomeStatusStaleTTL),
	}
	delete(publicHomeStatusCache.inFlight, days)
	close(done)
	publicHomeStatusCache.Unlock()

	return result, err
}

func buildPublicHomeStatus(days int) (PublicHomeStatusResponse, error) {
	days = normalizePublicHomeStatusDays(days)
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return PublicHomeStatusResponse{}, err
	}

	channelIds := make([]int, 0, len(channels))
	enabledChannels := 0
	healthyChannels := 0
	channelGroups := make(map[string]map[int]bool)
	groupStates := make(map[string]*PublicHomeStatusGroupMeta)
	for _, channel := range channels {
		if !shouldIncludeChannelInStatusMonitor(channel) {
			continue
		}
		channelIds = append(channelIds, channel.Id)
		if channel.Status == common.ChannelStatusEnabled {
			enabledChannels++
		}
		healthState := channelHealthState(channel, nil)
		if healthState == "healthy" {
			healthyChannels++
		}
		for _, groupKey := range publicHomeStatusGroupKeysForChannel(channel) {
			if channelGroups[groupKey] == nil {
				channelGroups[groupKey] = make(map[int]bool)
			}
			channelGroups[groupKey][channel.Id] = true
			if groupStates[groupKey] == nil {
				groupStates[groupKey] = &PublicHomeStatusGroupMeta{}
			}
			applyPublicHomeStatusChannelState(groupStates[groupKey], healthState)
		}
	}

	startTs := startOfPublicHomeStatusWindow(days).Unix()
	rows, err := model.GetPublicHomeStatusLogsWithTimeout(startTs, channelIds, publicHomeStatusLogQueryTTL)
	if err != nil {
		common.SysLog("failed to build public home status logs: " + err.Error())
		rows = nil
	}

	result := buildPublicHomeStatusFromRows(days, rows)
	result.Summary.EnabledChannels = enabledChannels
	result.Summary.HealthyChannels = healthyChannels
	applyPublicHomeStatusGroupChannelSummaries(&result, channelGroups, groupStates)
	result.UpdatedAt = time.Now().Unix()
	result.Partial = err != nil
	return result, nil
}

func getCachedPublicHomeDynamicBilling() *PublicHomeDynamicBilling {
	now := time.Now()
	publicHomeDynamicBillingCache.Lock()
	if publicHomeDynamicBillingCache.result != nil && now.Before(publicHomeDynamicBillingCache.expiresAt) {
		result := *publicHomeDynamicBillingCache.result
		publicHomeDynamicBillingCache.Unlock()
		return &result
	}
	publicHomeDynamicBillingCache.Unlock()

	result := buildPublicHomeDynamicBilling(now.Unix())
	ttl := publicHomeDynamicBillingTTL
	if result != nil && result.RefreshSeconds > 0 {
		refreshTTL := time.Duration(result.RefreshSeconds) * time.Second
		if refreshTTL > 0 && refreshTTL < ttl {
			ttl = refreshTTL
		}
	}

	publicHomeDynamicBillingCache.Lock()
	if result == nil {
		publicHomeDynamicBillingCache.result = nil
	} else {
		cached := *result
		publicHomeDynamicBillingCache.result = &cached
	}
	publicHomeDynamicBillingCache.expiresAt = time.Now().Add(ttl)
	publicHomeDynamicBillingCache.Unlock()

	return result
}

func buildPublicHomeDynamicBilling(now int64) *PublicHomeDynamicBilling {
	overview := buildModelGatewayDynamicBillingOverviewForDisplay(now, 0)
	refreshSeconds := overview.RefreshSeconds
	if refreshSeconds <= 0 {
		refreshSeconds = int(publicHomeDynamicBillingTTL.Seconds())
	}
	if !overview.Enabled || len(overview.Groups) == 0 {
		return &PublicHomeDynamicBilling{
			Enabled:        overview.Enabled,
			Status:         "waiting_samples",
			Model:          modelGatewayDynamicBillingDisplayModel,
			RefreshSeconds: refreshSeconds,
		}
	}

	groups := append([]ModelGatewayDynamicBillingGroupOverview(nil), overview.Groups...)
	sort.Slice(groups, func(i, j int) bool {
		statusDiff := publicHomeDynamicBillingStatusRank(groups[i].Status) - publicHomeDynamicBillingStatusRank(groups[j].Status)
		if statusDiff != 0 {
			return statusDiff < 0
		}
		if groups[i].LatestCalculatedAt != groups[j].LatestCalculatedAt {
			return groups[i].LatestCalculatedAt > groups[j].LatestCalculatedAt
		}
		return groups[i].PolicyGroup < groups[j].PolicyGroup
	})

	primary := groups[0]
	ratio := firstPositiveDynamicBillingValue(
		primary.CurrentRatio,
		primary.BlendedRatio,
		primary.AverageRatio,
		primary.MaxRatio,
		primary.MinRatio,
	)
	modelName := firstNonEmptyTrimmed(
		primary.ReferenceModel,
		primary.CurrentModel,
		modelGatewayDynamicBillingDisplayModel,
	)
	displayPrice := firstPositiveDynamicBillingValue(
		primary.CurrentPricePerM,
		primary.BlendedPricePerM,
		primary.AveragePricePerM,
		primary.MaxPricePerM,
		primary.MinPricePerM,
	)
	if displayPrice <= 0 && ratio > 0 {
		displayPrice = modelGatewayDynamicBillingPricePerMillion(modelName, ratio)
	}
	trend := buildPublicHomeDynamicBillingTrend(now, primary.PolicyGroup, modelName)
	minRatio7d, maxRatio7d := 0.0, 0.0
	if trend != nil && trend.SevenDays.SampleCount > 0 {
		minRatio7d = trend.SevenDays.MinRatio
		maxRatio7d = trend.SevenDays.MaxRatio
	} else {
		minRatio7d, maxRatio7d = publicHomeDynamicBillingRatioRange(now, primary.PolicyGroup)
	}

	updatedSecondsAgo := int64(0)
	if primary.LatestCalculatedAt > 0 && now > 0 {
		updatedSecondsAgo = now - primary.LatestCalculatedAt
		if updatedSecondsAgo < 0 {
			updatedSecondsAgo = 0
		}
	}

	return &PublicHomeDynamicBilling{
		Enabled:           overview.Enabled,
		Status:            primary.Status,
		Group:             firstNonEmptyTrimmed(primary.DisplayGroup, primary.CurrentTargetGroup, primary.PolicyGroup),
		Model:             modelName,
		CurrentRatio:      ratio,
		MinRatio7d:        minRatio7d,
		MaxRatio7d:        maxRatio7d,
		DisplayPricePerM:  displayPrice,
		Trend:             trend,
		UpdatedSecondsAgo: updatedSecondsAgo,
		RefreshSeconds:    refreshSeconds,
	}
}

type publicHomeDynamicBillingTrendSample struct {
	Timestamp int64
	Ratio     float64
	PricePerM float64
}

type publicHomeDynamicBillingTrendBucket struct {
	Timestamp int64
	RatioSum  float64
	PriceSum  float64
	Count     int
}

func buildPublicHomeDynamicBillingTrend(now int64, policyGroup string, modelName string) *PublicHomeDynamicBillingTrend {
	policyGroup = strings.TrimSpace(policyGroup)
	if now <= 0 {
		now = time.Now().Unix()
	}
	if policyGroup == "" || model.DB == nil || model.LOG_DB == nil {
		return nil
	}
	location := time.Local
	nowTime := time.Unix(now, 0).In(location)
	todayStart := time.Date(nowTime.Year(), nowTime.Month(), nowTime.Day(), 0, 0, 0, 0, location)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	sevenDaysStart := todayStart.AddDate(0, 0, -6)
	samples, err := loadPublicHomeDynamicBillingTrendSamples(sevenDaysStart.Unix(), policyGroup, modelName)
	if err != nil {
		common.SysLog("failed to build public home dynamic billing trend: " + err.Error())
		return nil
	}
	return &PublicHomeDynamicBillingTrend{
		Today:     buildPublicHomeDynamicBillingTrendSeries(samples, todayStart, 24, time.Hour),
		Yesterday: buildPublicHomeDynamicBillingTrendSeries(samples, yesterdayStart, 24, time.Hour),
		SevenDays: buildPublicHomeDynamicBillingTrendSeries(samples, sevenDaysStart, 7, 24*time.Hour),
	}
}

func loadPublicHomeDynamicBillingTrendSamples(startTime int64, policyGroup string, modelName string) ([]publicHomeDynamicBillingTrendSample, error) {
	setting := scheduler_setting.GetSetting()
	if setting.DynamicBillingEnabledAt > 0 && setting.DynamicBillingEnabledAt > startTime {
		startTime = setting.DynamicBillingEnabledAt
	}
	policyTargets := make(map[string]map[string]struct{})
	for groupName, policy := range setting.GroupPolicies {
		if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
			continue
		}
		normalizedPolicyGroup := strings.TrimSpace(groupName)
		targetGroups := normalizeModelGatewayDynamicTargetGroups(normalizedPolicyGroup, policy.CandidateGroups)
		targetSet := make(map[string]struct{}, len(targetGroups))
		for _, targetGroup := range targetGroups {
			targetSet[targetGroup] = struct{}{}
		}
		policyTargets[normalizedPolicyGroup] = targetSet
	}
	if len(policyTargets) == 0 {
		return nil, nil
	}
	appliedLogs, err := loadModelGatewayDynamicBillingAppliedLogs(startTime)
	if err != nil {
		return nil, err
	}
	summaryByRequestID, err := loadModelGatewayDynamicBillingAppliedSummaries(appliedLogs)
	if err != nil {
		return nil, err
	}
	samples := make([]publicHomeDynamicBillingTrendSample, 0, len(appliedLogs))
	for _, logRow := range appliedLogs {
		other := make(map[string]interface{})
		if err := common.UnmarshalJsonStr(logRow.Other, &other); err != nil {
			continue
		}
		if skipModelGatewayDynamicBillingAppliedLog(other) || !modelGatewayBillingBool(other, "dynamic_billing_applied") {
			continue
		}
		ratio := modelGatewayBillingFloat(other, "dynamic_billing_ratio")
		if ratio <= 0 {
			continue
		}
		summary := summaryByRequestID[strings.TrimSpace(logRow.RequestId)]
		targetGroup := firstNonEmptyTrimmed(
			modelGatewayBillingString(other, "dynamic_billing_group"),
			summary.SelectedGroup,
			logRow.Group,
		)
		resolvedPolicyGroup := resolveModelGatewayDynamicPolicyGroup(
			strings.TrimSpace(summary.RequestedGroup),
			targetGroup,
			policyTargets,
		)
		if strings.TrimSpace(resolvedPolicyGroup) != policyGroup {
			continue
		}
		pricePerM := modelGatewayBillingFloat(other, "dynamic_billing_price_per_m")
		if pricePerM <= 0 {
			pricePerM = modelGatewayDynamicBillingPricePerMillion(modelName, ratio)
		}
		samples = append(samples, publicHomeDynamicBillingTrendSample{
			Timestamp: logRow.CreatedAt,
			Ratio:     ratio,
			PricePerM: pricePerM,
		})
	}
	return samples, nil
}

func buildPublicHomeDynamicBillingTrendSeries(samples []publicHomeDynamicBillingTrendSample, start time.Time, count int, step time.Duration) PublicHomeDynamicBillingTrendSeries {
	if count <= 0 || step <= 0 {
		return PublicHomeDynamicBillingTrendSeries{}
	}
	buckets := make([]publicHomeDynamicBillingTrendBucket, count)
	for idx := range buckets {
		buckets[idx].Timestamp = start.Add(time.Duration(idx) * step).Unix()
	}
	startTs := start.Unix()
	endTs := start.Add(time.Duration(count) * step).Unix()
	for _, sample := range samples {
		if sample.Timestamp < startTs || sample.Timestamp >= endTs || sample.Ratio <= 0 {
			continue
		}
		index := int((sample.Timestamp - startTs) / int64(step.Seconds()))
		if index < 0 || index >= count {
			continue
		}
		buckets[index].RatioSum += sample.Ratio
		if sample.PricePerM > 0 {
			buckets[index].PriceSum += sample.PricePerM
		}
		buckets[index].Count++
	}
	series := PublicHomeDynamicBillingTrendSeries{
		Points: make([]PublicHomeDynamicBillingTrendPoint, 0, count),
	}
	totalRatio := 0.0
	for _, bucket := range buckets {
		point := PublicHomeDynamicBillingTrendPoint{Timestamp: bucket.Timestamp}
		if bucket.Count > 0 {
			point.Ratio = bucket.RatioSum / float64(bucket.Count)
			point.PricePerM = bucket.PriceSum / float64(bucket.Count)
			point.SampleCount = bucket.Count
			series.SampleCount += bucket.Count
			totalRatio += bucket.RatioSum
			series.LatestRatio = point.Ratio
			if series.MinRatio <= 0 || point.Ratio < series.MinRatio {
				series.MinRatio = point.Ratio
			}
			if point.Ratio > series.MaxRatio {
				series.MaxRatio = point.Ratio
			}
		}
		series.Points = append(series.Points, point)
	}
	if series.SampleCount > 0 {
		series.AvgRatio = totalRatio / float64(series.SampleCount)
	}
	return series
}

func publicHomeDynamicBillingRatioRange(now int64, policyGroup string) (float64, float64) {
	policyGroup = strings.TrimSpace(policyGroup)
	if policyGroup == "" || model.DB == nil || model.LOG_DB == nil {
		return 0, 0
	}
	overview7d := buildModelGatewayDynamicBillingOverviewForDisplay(now, 7*24*60)
	for _, item := range overview7d.Groups {
		if strings.TrimSpace(item.PolicyGroup) != policyGroup {
			continue
		}
		minRatio := firstPositiveDynamicBillingValue(
			item.MinRatio,
			item.BlendedRatio,
			item.AverageRatio,
			item.CurrentRatio,
			item.MaxRatio,
		)
		maxRatio := firstPositiveDynamicBillingValue(
			item.MaxRatio,
			item.BlendedRatio,
			item.AverageRatio,
			item.CurrentRatio,
			item.MinRatio,
		)
		if maxRatio > 0 && minRatio <= 0 {
			minRatio = maxRatio
		}
		if minRatio > 0 && maxRatio <= 0 {
			maxRatio = minRatio
		}
		if minRatio > 0 && maxRatio > 0 && minRatio > maxRatio {
			minRatio, maxRatio = maxRatio, minRatio
		}
		return minRatio, maxRatio
	}
	return 0, 0
}

func publicHomeDynamicBillingStatusRank(status string) int {
	switch strings.TrimSpace(status) {
	case "active":
		return 0
	case "waiting_samples":
		return 1
	case "expired":
		return 2
	case "global_disabled":
		return 3
	default:
		return 4
	}
}

func getCachedPublicHomeModelGatewayStats() publicHomeModelGatewayStats {
	now := time.Now()
	publicHomeModelGatewayStatsCache.Lock()
	if cached, ok := publicHomeModelGatewayStatsCache.items[publicHomeGatewayStatsHours]; ok && now.Before(cached.expiresAt) {
		result := cached.result
		publicHomeModelGatewayStatsCache.Unlock()
		return result
	}
	if cached, ok := publicHomeModelGatewayStatsCache.items[publicHomeGatewayStatsHours]; ok && now.Before(cached.staleUntil) {
		result := cached.result
		if _, refreshing := publicHomeModelGatewayStatsCache.inFlight[publicHomeGatewayStatsHours]; !refreshing {
			done := make(chan struct{})
			publicHomeModelGatewayStatsCache.inFlight[publicHomeGatewayStatsHours] = done
			go refreshPublicHomeModelGatewayStats(done)
		}
		publicHomeModelGatewayStatsCache.Unlock()
		return result
	}
	if done, ok := publicHomeModelGatewayStatsCache.inFlight[publicHomeGatewayStatsHours]; ok {
		publicHomeModelGatewayStatsCache.Unlock()
		// Do not make the public homepage wait on a cold expensive stats scan.
		// The request will render with channel-level status and the background
		// refresh will fill the cache for the next visitor.
		select {
		case <-done:
			publicHomeModelGatewayStatsCache.Lock()
			cached, hasResult := publicHomeModelGatewayStatsCache.items[publicHomeGatewayStatsHours]
			publicHomeModelGatewayStatsCache.Unlock()
			if hasResult {
				return cached.result
			}
		case <-time.After(publicHomeGatewayFirstWait):
		}
		return publicHomeModelGatewayStats{}
	}
	done := make(chan struct{})
	publicHomeModelGatewayStatsCache.inFlight[publicHomeGatewayStatsHours] = done
	publicHomeModelGatewayStatsCache.Unlock()

	go refreshPublicHomeModelGatewayStats(done)
	select {
	case <-done:
		publicHomeModelGatewayStatsCache.Lock()
		cached, hasResult := publicHomeModelGatewayStatsCache.items[publicHomeGatewayStatsHours]
		publicHomeModelGatewayStatsCache.Unlock()
		if hasResult {
			return cached.result
		}
	case <-time.After(publicHomeGatewayFirstWait):
	}
	return publicHomeModelGatewayStats{}
}

func refreshPublicHomeModelGatewayStats(done chan struct{}) publicHomeModelGatewayStats {
	result := buildPublicHomeModelGatewayStats()
	now := time.Now()
	cacheTTL := publicHomeGatewayStatsTTL
	staleTTL := publicHomeGatewayStatsStale
	if result.Requests <= 0 {
		cacheTTL = publicHomeGatewayEmptyTTL
		staleTTL = publicHomeGatewayEmptyStale
	}
	publicHomeModelGatewayStatsCache.Lock()
	publicHomeModelGatewayStatsCache.items[publicHomeGatewayStatsHours] = publicHomeModelGatewayStatsCacheItem{
		result:     result,
		expiresAt:  now.Add(cacheTTL),
		staleUntil: now.Add(staleTTL),
	}
	delete(publicHomeModelGatewayStatsCache.inFlight, publicHomeGatewayStatsHours)
	publicHomeModelGatewayStatsCache.Unlock()
	if done != nil {
		close(done)
	}
	return result
}

func buildPublicHomeModelGatewayStats() publicHomeModelGatewayStats {
	if model.DB == nil {
		return publicHomeModelGatewayStats{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), publicHomeGatewayQueryTTL)
	defer cancel()

	startTime := time.Now().Add(-time.Duration(publicHomeGatewayStatsHours) * time.Hour).Unix()
	var row publicHomeModelGatewayStatsAggRow
	tx := model.DB.WithContext(ctx).
		Model(&model.ModelGatewayUserRequestSummary{}).
		Select(
			"COUNT(*) AS requests, "+
				"COALESCE(SUM(CASE WHEN final_success = ? THEN 1 ELSE 0 END), 0) AS successes, "+
				"COALESCE(AVG(CASE WHEN duration_ms > 0 THEN duration_ms ELSE NULL END), 0) AS avg_latency_ms, "+
				"COALESCE(AVG(CASE WHEN ttft_ms > 0 THEN ttft_ms ELSE NULL END), 0) AS avg_ttft_ms",
			true,
		).
		Where("completed_at >= ? AND completed_at > 0", startTime).
		Where("is_health_probe = ? AND client_aborted = ?", false, false)
	tx = applyPublicHomeModelGatewayStatsGroupFilter(tx, publicHomeCodexUserRequestGroups())
	err := tx.Scan(&row).Error
	if err != nil {
		common.SysLog("failed to build public home model gateway stats: " + err.Error())
		return publicHomeModelGatewayStats{}
	}
	if row.Requests <= 0 {
		return publicHomeModelGatewayStats{}
	}
	return publicHomeModelGatewayStats{
		Requests:     row.Requests,
		Successes:    row.Successes,
		SuccessRate:  successRate(row.Successes, row.Requests),
		AvgLatencyMs: int64(row.AvgLatencyMs + 0.5),
		AvgTTFTMs:    int64(row.AvgTTFTMs + 0.5),
	}
}

func publicHomeCodexUserRequestGroups() []string {
	groupNames := []string{"codex", "codex-plus", "codex-pro", "codex-subscription"}
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		common.SysLog("failed to load public home codex groups: " + err.Error())
		return uniquePublicHomeModelGatewayGroups(groupNames)
	}
	for _, channel := range channels {
		if !shouldIncludeChannelInStatusMonitor(channel) {
			continue
		}
		for _, groupName := range channel.GetGroups() {
			if publicHomeStatusGroupKeyFromName(groupName) != "codex" {
				continue
			}
			groupNames = append(groupNames, groupName, strings.ToLower(strings.TrimSpace(groupName)))
		}
	}
	return uniquePublicHomeModelGatewayGroups(groupNames)
}

func applyPublicHomeModelGatewayStatsGroupFilter(tx *gorm.DB, groupNames []string) *gorm.DB {
	groupNames = uniquePublicHomeModelGatewayGroups(groupNames)
	if len(groupNames) == 0 {
		return tx
	}
	return tx.Where("(selected_group IN ? OR (selected_group = ? AND requested_group IN ?))", groupNames, "", groupNames)
}

func uniquePublicHomeModelGatewayGroups(groupNames []string) []string {
	seen := make(map[string]struct{}, len(groupNames))
	result := make([]string, 0, len(groupNames))
	for _, groupName := range groupNames {
		groupName = strings.TrimSpace(groupName)
		if groupName == "" {
			continue
		}
		if _, ok := seen[groupName]; ok {
			continue
		}
		seen[groupName] = struct{}{}
		result = append(result, groupName)
	}
	return result
}

func publicHomeModelGatewayStatsFromUserRequests(rows []model.ModelGatewayUserRequestSummary) publicHomeModelGatewayStats {
	result := publicHomeModelGatewayStats{}
	var latencySum, latencyCount int64
	var ttftSum, ttftCount int64
	for _, row := range channelMonitorUniqueUserRequestRows(rows) {
		if row.IsHealthProbe || channelMonitorUserRequestStatus(row) == "client_aborted" {
			continue
		}
		result.Requests++
		if row.FinalSuccess {
			result.Successes++
		}
		if row.DurationMs > 0 {
			latencySum += row.DurationMs
			latencyCount++
		}
		if row.TTFTMs > 0 {
			ttftSum += row.TTFTMs
			ttftCount++
		}
	}
	if result.Requests <= 0 {
		return publicHomeModelGatewayStats{}
	}
	result.SuccessRate = successRate(result.Successes, result.Requests)
	result.AvgLatencyMs = avgInt64(latencySum, latencyCount)
	result.AvgTTFTMs = avgInt64(ttftSum, ttftCount)
	if result.AvgLatencyMs <= 0 && result.AvgTTFTMs > 0 {
		result.AvgLatencyMs = result.AvgTTFTMs
	}
	return result
}

func applyPublicHomeModelGatewayStats(result *PublicHomeStatusResponse, stats publicHomeModelGatewayStats) {
	if result == nil {
		return
	}
	if stats.Requests > 0 {
		result.Summary.Requests = stats.Requests
		result.Summary.SuccessRate = stats.SuccessRate
	}
	if stats.AvgLatencyMs > 0 {
		result.Summary.AvgLatencyMs = stats.AvgLatencyMs
	}
	if stats.AvgTTFTMs > 0 {
		result.Summary.AvgTTFTMs = stats.AvgTTFTMs
	}
}

func startOfPublicHomeStatusWindow(days int) time.Time {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start.AddDate(0, 0, -normalizePublicHomeStatusDays(days)+1)
}

func buildPublicHomeStatusEmpty(days int, partial bool) PublicHomeStatusResponse {
	result := buildPublicHomeStatusFromRows(days, nil)
	result.UpdatedAt = time.Now().Unix()
	result.Partial = partial
	return result
}

func buildPublicHomeStatusFromRows(days int, rows []model.ChannelStatusMonitorLogRow) PublicHomeStatusResponse {
	days = normalizePublicHomeStatusDays(days)
	start := startOfPublicHomeStatusWindow(days)
	buckets, bucketByDate := newPublicHomeStatusBuckets(days, start)

	requests := buildChannelMonitorRequestAggregates(publicHomeStatusRequestLogs(rows))
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].lastRequestAt == requests[j].lastRequestAt {
			return requests[i].lastId < requests[j].lastId
		}
		return requests[i].lastRequestAt < requests[j].lastRequestAt
	})

	overall := &publicHomeStatusBucket{}
	for _, request := range requests {
		if request == nil || request.lastRequestAt <= 0 {
			continue
		}
		date := time.Unix(request.lastRequestAt, 0).Format("2006-01-02")
		bucket := bucketByDate[date]
		if bucket == nil {
			continue
		}
		applyPublicHomeStatusRequest(bucket, request)
		applyPublicHomeStatusRequest(overall, request)
	}

	groups := buildPublicHomeStatusGroups(days, start, requests)
	daily := make([]PublicHomeStatusDaily, 0, len(buckets))
	for _, bucket := range buckets {
		daily = append(daily, PublicHomeStatusDaily{
			Date:            bucket.date,
			Requests:        bucket.requests,
			SuccessRate:     successRate(bucket.successes, bucket.requests),
			AvgLatencyMs:    avgInt64(bucket.latencySum, bucket.latencyCount),
			AvgTTFTMs:       avgInt64(bucket.firstRespSum, bucket.firstRespCount),
			ProtectedEvents: bucket.protectedEvents,
		})
	}

	return PublicHomeStatusResponse{
		Summary: PublicHomeStatusSummary{
			Days:            days,
			SuccessRate:     successRate(overall.successes, overall.requests),
			AvgLatencyMs:    avgInt64(overall.latencySum, overall.latencyCount),
			AvgTTFTMs:       avgInt64(overall.firstRespSum, overall.firstRespCount),
			Requests:        overall.requests,
			ProtectedEvents: overall.protectedEvents,
		},
		Daily:     daily,
		Groups:    groups,
		UpdatedAt: time.Now().Unix(),
	}
}

type publicHomeStatusBucket struct {
	date            string
	requests        int64
	successes       int64
	latencySum      int64
	latencyCount    int64
	firstRespSum    int64
	firstRespCount  int64
	protectedEvents int64
}

func publicHomeStatusRequestLogs(rows []model.ChannelStatusMonitorLogRow) []channelMonitorRequestLog {
	requestLogs := make([]channelMonitorRequestLog, 0, len(rows))
	for _, row := range rows {
		groupName := normalizeChannelGroup(row.Group)
		if groupName == "" {
			groupName = "default"
		}
		requestLogs = append(requestLogs, buildChannelMonitorRequestLog(row, groupName))
	}
	return requestLogs
}

func applyPublicHomeStatusRequest(bucket *publicHomeStatusBucket, request *channelMonitorRequestAgg) {
	if bucket == nil || request == nil {
		return
	}
	bucket.requests++
	if request.success {
		bucket.successes++
	}
	if request.latencyMs > 0 {
		bucket.latencySum += request.latencyMs
		bucket.latencyCount++
	}
	if request.firstRespMs > 0 {
		bucket.firstRespSum += request.firstRespMs
		bucket.firstRespCount++
	}
	if request.worstStatus != "" && request.worstStatus != "success" {
		bucket.protectedEvents++
	}
}

func newPublicHomeStatusBuckets(days int, start time.Time) ([]*publicHomeStatusBucket, map[string]*publicHomeStatusBucket) {
	buckets := make([]*publicHomeStatusBucket, 0, days)
	bucketByDate := make(map[string]*publicHomeStatusBucket, days)
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		bucket := &publicHomeStatusBucket{date: date}
		buckets = append(buckets, bucket)
		bucketByDate[date] = bucket
	}
	return buckets, bucketByDate
}

func buildPublicHomeStatusGroups(days int, start time.Time, requests []*channelMonitorRequestAgg) []PublicHomeStatusGroup {
	groupBuckets := make(map[string]map[string]*publicHomeStatusBucket)
	groupOverall := make(map[string]*publicHomeStatusBucket)
	for _, item := range publicHomeStatusGroups() {
		_, bucketByDate := newPublicHomeStatusBuckets(days, start)
		groupBuckets[item.key] = bucketByDate
		groupOverall[item.key] = &publicHomeStatusBucket{}
	}

	for _, request := range requests {
		groupKey := publicHomeStatusGroupKeyForRequest(request)
		date := time.Unix(request.lastRequestAt, 0).Format("2006-01-02")
		if bucket := groupBuckets[groupKey][date]; bucket != nil {
			applyPublicHomeStatusRequest(bucket, request)
		}
		applyPublicHomeStatusRequest(groupOverall[groupKey], request)
	}

	result := make([]PublicHomeStatusGroup, 0, len(publicHomeStatusGroups()))
	for _, item := range publicHomeStatusGroups() {
		daily := make([]PublicHomeStatusDaily, 0, days)
		buckets, _ := newPublicHomeStatusBuckets(days, start)
		for _, bucket := range buckets {
			if existing := groupBuckets[item.key][bucket.date]; existing != nil {
				bucket = existing
			}
			daily = append(daily, PublicHomeStatusDaily{
				Date:            bucket.date,
				Requests:        bucket.requests,
				SuccessRate:     successRate(bucket.successes, bucket.requests),
				AvgLatencyMs:    avgInt64(bucket.latencySum, bucket.latencyCount),
				AvgTTFTMs:       avgInt64(bucket.firstRespSum, bucket.firstRespCount),
				ProtectedEvents: bucket.protectedEvents,
			})
		}
		overall := groupOverall[item.key]
		result = append(result, PublicHomeStatusGroup{
			Key:  item.key,
			Name: item.name,
			Summary: PublicHomeStatusSummary{
				Days:            days,
				SuccessRate:     successRate(overall.successes, overall.requests),
				AvgLatencyMs:    avgInt64(overall.latencySum, overall.latencyCount),
				AvgTTFTMs:       avgInt64(overall.firstRespSum, overall.firstRespCount),
				Requests:        overall.requests,
				ProtectedEvents: overall.protectedEvents,
			},
			Daily: daily,
		})
	}
	return result
}

type publicHomeStatusGroupItem struct {
	key  string
	name string
}

func publicHomeStatusGroups() []publicHomeStatusGroupItem {
	return []publicHomeStatusGroupItem{
		{key: "codex", name: "Codex 专用"},
		{key: "claude", name: "Claude Code"},
		{key: "speed", name: "高速组"},
		{key: "value", name: "低价组"},
	}
}

func publicHomeStatusGroupKeyForRequest(request *channelMonitorRequestAgg) string {
	if request == nil {
		return "speed"
	}
	return publicHomeStatusGroupKeyFromName(request.group)
}

func publicHomeStatusGroupKeysForChannel(channel *model.Channel) []string {
	groups := channel.GetGroups()
	if len(groups) == 0 {
		return []string{"speed"}
	}
	seen := make(map[string]bool)
	keys := make([]string, 0, len(groups))
	for _, group := range groups {
		key := publicHomeStatusGroupKeyFromName(group)
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

func publicHomeStatusGroupKeyFromName(group string) string {
	normalized := strings.ToLower(strings.TrimSpace(group))
	switch {
	case strings.Contains(normalized, "codex"):
		return "codex"
	case strings.Contains(normalized, "claude"), normalized == "cc",
		strings.HasPrefix(normalized, "cc-"), strings.HasPrefix(normalized, "cc_"):
		return "claude"
	case strings.Contains(normalized, "low"), strings.Contains(normalized, "cheap"),
		strings.Contains(normalized, "value"), strings.Contains(normalized, "discount"),
		strings.Contains(normalized, "低价"), strings.Contains(normalized, "经济"):
		return "value"
	default:
		return "speed"
	}
}

func applyPublicHomeStatusGroupChannelSummaries(result *PublicHomeStatusResponse, channelGroups map[string]map[int]bool, groupStates map[string]*PublicHomeStatusGroupMeta) {
	if result == nil {
		return
	}
	for index := range result.Groups {
		group := &result.Groups[index]
		channels := channelGroups[group.Key]
		states := groupStates[group.Key]
		if states == nil {
			states = &PublicHomeStatusGroupMeta{}
		}
		states.Channels = len(channels)
		group.States = *states
		group.Summary.EnabledChannels = len(channels)
		group.Summary.HealthyChannels = states.Healthy
	}
}

func applyPublicHomeStatusChannelState(states *PublicHomeStatusGroupMeta, healthState string) {
	if states == nil {
		return
	}
	switch healthState {
	case "healthy":
		states.Healthy++
	case "warning":
		states.Cooling++
	default:
		states.Standby++
	}
}
