package dynamicbilling

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

const (
	FallbackDisabled            = "disabled"
	FallbackStaticMode          = "static_mode"
	FallbackMissingGroup        = "missing_group"
	FallbackCacheNotLoaded      = "cache_not_loaded"
	FallbackMissingKey          = "missing_key"
	FallbackInsufficientSamples = "insufficient_samples"
	FallbackBaselineExpired     = "baseline_expired"
	FallbackObserveMode         = "observe_mode"
	FallbackManualConfirm       = "manual_confirm_required"
	FallbackStepChangeTooLarge  = "step_change_too_large"
	FallbackInsufficientUsage   = "insufficient_usage"
	FallbackNoCostData          = "no_cost_data"
	FallbackMissingBaseQuota    = "base_quota_missing"
	FallbackTrafficNotReady     = "traffic_not_ready"

	ApplyReasonAutoApplied           = "auto_applied"
	ApplyReasonManualModeAutoApplied = "manual_mode_auto_applied"
	ApplyReasonStepChangeAutoApplied = "step_change_auto_applied"
)

type RatioBaseline struct {
	RequestedModel string  `json:"requested_model"`
	ReferenceModel string  `json:"reference_model,omitempty"`
	Group          string  `json:"group"`
	Ratio          float64 `json:"ratio"`
	PricePerM      float64 `json:"price_per_m,omitempty"`
	SampleCount    int     `json:"sample_count"`
	ModelCount     int     `json:"model_count,omitempty"`
	CalculatedAt   int64   `json:"calculated_at"`
	WindowStart    int64   `json:"window_start"`
	WindowEnd      int64   `json:"window_end"`
	ProfitRate     float64 `json:"profit_rate"`
	CostSource     string  `json:"cost_source,omitempty"`
	ApplyMode      string  `json:"apply_mode,omitempty"`
	ApplyReason    string  `json:"apply_reason,omitempty"`

	OperatingCostUSD     float64 `json:"operating_cost_usd,omitempty"`
	RequiredRevenueUSD   float64 `json:"required_revenue_usd,omitempty"`
	BaseQuotaAtRatio1    float64 `json:"base_quota_at_ratio_1,omitempty"`
	CostMultiplier       float64 `json:"cost_multiplier,omitempty"`
	TargetRatio          float64 `json:"target_ratio,omitempty"`
	EffectiveRatio       float64 `json:"effective_ratio,omitempty"`
	Clamped              bool    `json:"clamped,omitempty"`
	PendingManualConfirm bool    `json:"pending_manual_confirm,omitempty"`
	FallbackReason       string  `json:"fallback_reason,omitempty"`

	RequestCount        int64   `json:"request_count,omitempty"`
	SuccessRequestCount int64   `json:"success_request_count,omitempty"`
	TotalTokens         int64   `json:"total_tokens,omitempty"`
	TrafficCostUSD      float64 `json:"traffic_cost_usd,omitempty"`
	TrafficEstimated    bool    `json:"traffic_estimated,omitempty"`
	TrafficDataReady    bool    `json:"traffic_data_ready,omitempty"`
	ServerCostUSD       float64 `json:"server_cost_usd,omitempty"`
	ResourceCostUSD     float64 `json:"resource_cost_usd,omitempty"`
	UpstreamCostUSD     float64 `json:"upstream_cost_usd,omitempty"`
}

type SnapshotFilter struct {
	MinCalculatedAt int64
}

type RatioProvider interface {
	Lookup(requestedModel string, group string) (RatioBaseline, bool)
	Loaded() bool
}

type RatioCache struct {
	mu       sync.RWMutex
	values   map[string]RatioBaseline
	loadedAt time.Time
}

type RefresherConfig struct {
	Enabled  bool
	Interval time.Duration
}

type Refresher struct {
	config RefresherConfig
	cache  *RatioCache
	cancel context.CancelFunc
	done   chan struct{}
}

var (
	defaultCacheMu   sync.Mutex
	defaultCache     = NewRatioCache()
	defaultRefresher *Refresher
)

func NewRatioCache() *RatioCache {
	return &RatioCache{values: map[string]RatioBaseline{}}
}

func NewRefresher(config RefresherConfig, cache *RatioCache) *Refresher {
	if cache == nil {
		cache = NewRatioCache()
	}
	if config.Interval <= 0 {
		config.Interval = 30 * time.Second
	}
	return &Refresher{
		config: config,
		cache:  cache,
		done:   make(chan struct{}),
	}
}

func (r *Refresher) Start(ctx context.Context) {
	if r == nil || !r.config.Enabled {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	go r.run(ctx)
}

func (r *Refresher) Stop() {
	if r == nil {
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
	if r.done != nil {
		<-r.done
	}
}

func (r *Refresher) run(ctx context.Context) {
	defer close(r.done)
	r.refresh()
	ticker := time.NewTicker(r.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refresh()
		}
	}
}

func (r *Refresher) refresh() {
	if r == nil || r.cache == nil {
		return
	}
	setting := scheduler_setting.GetSetting()
	if !setting.DynamicBillingEnabled {
		r.cache.Store(map[string]RatioBaseline{})
		return
	}
	values, err := BuildRatioBaselines(model.DB, model.LOG_DB, setting, time.Now().Unix())
	if err != nil {
		if persisted := loadPersistedBaselines(model.DB); len(persisted) > 0 {
			r.cache.Store(persisted)
		}
		common.SysLog(fmt.Sprintf("model gateway dynamic billing refresh failed: %v", err))
		return
	}
	if err := persistBaselines(model.DB, values); err != nil {
		common.SysLog(fmt.Sprintf("model gateway dynamic billing persist failed: %v", err))
	}
	r.cache.Store(values)
	common.SysLog(fmt.Sprintf("model gateway dynamic billing refreshed: baselines=%d", len(values)))
}

func SyncDefaultRefresherLifecycle() *Refresher {
	defaultCacheMu.Lock()
	defer defaultCacheMu.Unlock()
	stopDefaultRefresherLocked()
	setting := scheduler_setting.GetSetting()
	if !setting.DynamicBillingEnabled {
		defaultCache.Store(map[string]RatioBaseline{})
		return nil
	}
	if persisted := loadPersistedBaselines(model.DB); len(persisted) > 0 {
		defaultCache.Store(persisted)
	}
	interval := time.Duration(setting.DynamicBillingRefreshSeconds) * time.Second
	refresher := NewRefresher(RefresherConfig{Enabled: true, Interval: interval}, defaultCache)
	refresher.Start(context.Background())
	defaultRefresher = refresher
	return refresher
}

func StopDefaultRefresher() {
	defaultCacheMu.Lock()
	defer defaultCacheMu.Unlock()
	stopDefaultRefresherLocked()
}

func DefaultRatioProvider() RatioProvider {
	return defaultCache
}

func DefaultBaselineSnapshots() []RatioBaseline {
	return defaultCache.Snapshots()
}

func StoreDefaultBaselinesForTest(values map[string]RatioBaseline) func() {
	oldValues, oldLoadedAt := defaultCache.snapshotState()
	defaultCache.Store(values)
	return func() {
		defaultCache.restoreState(oldValues, oldLoadedAt)
	}
}

func RefreshDefaultNow() error {
	setting := scheduler_setting.GetSetting()
	values, err := BuildRatioBaselines(model.DB, model.LOG_DB, setting, time.Now().Unix())
	if err != nil {
		return err
	}
	if err := persistBaselines(model.DB, values); err != nil {
		return err
	}
	defaultCache.Store(values)
	return nil
}

func BuildRatioBaselineSnapshots(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64, windowMinutesOverride int) ([]RatioBaseline, error) {
	return BuildRatioBaselineSnapshotsWithFilter(db, logDB, setting, now, windowMinutesOverride, SnapshotFilter{})
}

func BuildRatioBaselineSnapshotsWithFilter(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64, windowMinutesOverride int, filter SnapshotFilter) ([]RatioBaseline, error) {
	if db == nil || logDB == nil {
		return nil, nil
	}
	overrideSetting := setting
	if windowMinutesOverride > 0 {
		overrideSetting.DynamicBillingWindowSamples = 0
		overrideSetting.DynamicBillingWindowMinutes = windowMinutesOverride
	}
	values, err := BuildRatioBaselinesWithFilter(db, logDB, overrideSetting, now, filter)
	if err != nil {
		return nil, err
	}
	result := make([]RatioBaseline, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result, nil
}

func stopDefaultRefresherLocked() {
	if defaultRefresher == nil {
		return
	}
	defaultRefresher.Stop()
	defaultRefresher = nil
}

func (c *RatioCache) Store(values map[string]RatioBaseline) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values = make(map[string]RatioBaseline, len(values))
	for _, value := range values {
		key := groupCacheKey(value.Group)
		if key == "" || value.Ratio <= 0 {
			continue
		}
		c.values[key] = value
	}
	c.loadedAt = time.Now()
}

func (c *RatioCache) Lookup(requestedModel string, group string) (RatioBaseline, bool) {
	if c == nil {
		return RatioBaseline{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.values[groupCacheKey(group)]
	return value, ok
}

func (c *RatioCache) Snapshots() []RatioBaseline {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]RatioBaseline, 0, len(c.values))
	for _, value := range c.values {
		result = append(result, value)
	}
	return result
}

func (c *RatioCache) snapshotState() (map[string]RatioBaseline, time.Time) {
	if c == nil {
		return nil, time.Time{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]RatioBaseline, len(c.values))
	for key, value := range c.values {
		result[key] = value
	}
	return result, c.loadedAt
}

func (c *RatioCache) restoreState(values map[string]RatioBaseline, loadedAt time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values = make(map[string]RatioBaseline, len(values))
	for key, value := range values {
		c.values[key] = value
	}
	c.loadedAt = loadedAt
}

func (c *RatioCache) Loaded() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.loadedAt.IsZero()
}

type ApplyInput struct {
	RequestedModel   string
	Group            string
	StaticGroupRatio float64
	Mode             string
	Setting          scheduler_setting.SchedulerSetting
	Provider         RatioProvider
	Now              int64
}

func Apply(input ApplyInput) types.DynamicBillingSnapshot {
	staticRatio := input.StaticGroupRatio
	if staticRatio <= 0 {
		staticRatio = ratio_setting.GetGroupRatio(strings.TrimSpace(input.Group))
	}
	snapshot := types.DynamicBillingSnapshot{
		RequestedModel:   strings.TrimSpace(input.RequestedModel),
		Group:            strings.TrimSpace(input.Group),
		StaticGroupRatio: staticRatio,
		ProfitRate:       input.Setting.DynamicBillingProfitRate,
		CostSource:       normalizeCostSource(input.Setting.DynamicBillingCostSource),
		ApplyMode:        normalizeApplyMode(input.Setting.DynamicBillingApplyMode),
	}
	if !input.Setting.DynamicBillingEnabled {
		snapshot.FallbackReason = FallbackDisabled
		return snapshot
	}
	if strings.TrimSpace(input.Mode) != scheduler_setting.BillingRatioModeDynamic {
		snapshot.FallbackReason = FallbackStaticMode
		return snapshot
	}
	if snapshot.Group == "" {
		snapshot.FallbackReason = FallbackMissingGroup
		return snapshot
	}
	if input.Provider == nil || !input.Provider.Loaded() {
		snapshot.FallbackReason = FallbackCacheNotLoaded
		return snapshot
	}
	baseline, ok := input.Provider.Lookup(snapshot.RequestedModel, snapshot.Group)
	if !ok {
		snapshot.FallbackReason = FallbackMissingKey
		return snapshot
	}
	snapshot.DynamicRatio = baseline.Ratio
	snapshot.PricePerM = requestedModelPricePerMillion(snapshot.RequestedModel, baseline.Ratio)
	snapshot.SampleCount = baseline.SampleCount
	snapshot.CalculatedAt = baseline.CalculatedAt
	snapshot.WindowStart = baseline.WindowStart
	snapshot.WindowEnd = baseline.WindowEnd
	snapshot.ProfitRate = baseline.ProfitRate
	snapshot.CostSource = firstNonEmptyTrimmed(baseline.CostSource, snapshot.CostSource)
	snapshot.ApplyMode = firstNonEmptyTrimmed(baseline.ApplyMode, snapshot.ApplyMode)
	snapshot.ApplyReason = strings.TrimSpace(baseline.ApplyReason)
	snapshot.OperatingCostUSD = baseline.OperatingCostUSD
	snapshot.RequiredRevenueUSD = baseline.RequiredRevenueUSD
	snapshot.BaseQuotaAtRatio1 = baseline.BaseQuotaAtRatio1
	snapshot.CostMultiplier = baseline.CostMultiplier
	snapshot.TargetRatio = baseline.TargetRatio
	snapshot.EffectiveRatio = baseline.EffectiveRatio
	snapshot.Clamped = baseline.Clamped
	snapshot.PendingManualConfirm = false
	snapshot.RequestCount = baseline.RequestCount
	snapshot.SuccessRequestCount = baseline.SuccessRequestCount
	snapshot.TotalTokens = baseline.TotalTokens
	snapshot.TrafficCostUSD = baseline.TrafficCostUSD
	snapshot.TrafficEstimated = baseline.TrafficEstimated
	snapshot.TrafficDataReady = baseline.TrafficDataReady
	snapshot.ServerCostUSD = baseline.ServerCostUSD
	snapshot.ResourceCostUSD = baseline.ResourceCostUSD
	snapshot.UpstreamCostUSD = baseline.UpstreamCostUSD
	if fallbackReason := strings.TrimSpace(baseline.FallbackReason); fallbackReason != "" && !isAutoAppliedLegacyFallback(fallbackReason) {
		snapshot.FallbackReason = fallbackReason
		return snapshot
	} else if fallbackReason != "" && snapshot.ApplyReason == "" {
		snapshot.ApplyReason = applyReasonForLegacyFallback(fallbackReason)
	}
	if input.Setting.DynamicBillingMinSamples > 0 && baseline.SampleCount < input.Setting.DynamicBillingMinSamples {
		snapshot.FallbackReason = FallbackInsufficientSamples
		return snapshot
	}
	if isProfit24hBaseline(baseline) {
		if baseline.RequestCount < int64(input.Setting.DynamicBillingMinRequests) ||
			baseline.SuccessRequestCount < int64(input.Setting.DynamicBillingMinSuccessRequests) ||
			baseline.TotalTokens < int64(input.Setting.DynamicBillingMinTokens) {
			snapshot.FallbackReason = FallbackInsufficientUsage
			return snapshot
		}
		if baseline.BaseQuotaAtRatio1 <= 0 {
			snapshot.FallbackReason = FallbackMissingBaseQuota
			return snapshot
		}
		switch normalizeApplyMode(snapshot.ApplyMode) {
		case scheduler_setting.DynamicBillingApplyModeObserve:
			snapshot.FallbackReason = FallbackObserveMode
			return snapshot
		}
	}
	maxAge := input.Setting.DynamicBillingMaxAgeSeconds
	if maxAge > 0 {
		now := input.Now
		if now <= 0 {
			now = time.Now().Unix()
		}
		if baseline.CalculatedAt <= 0 || now-baseline.CalculatedAt > int64(maxAge) {
			snapshot.FallbackReason = FallbackBaselineExpired
			return snapshot
		}
	}
	if baseline.Ratio <= 0 {
		snapshot.FallbackReason = FallbackMissingKey
		return snapshot
	}
	if snapshot.ApplyReason == "" {
		snapshot.ApplyReason = defaultApplyReason(snapshot.ApplyMode)
	}
	snapshot.Applied = true
	return snapshot
}

func BuildRatioBaselines(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64) (map[string]RatioBaseline, error) {
	return BuildRatioBaselinesWithFilter(db, logDB, setting, now, SnapshotFilter{})
}

func BuildRatioBaselinesWithFilter(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64, filter SnapshotFilter) (map[string]RatioBaseline, error) {
	if normalizeCostSource(setting.DynamicBillingCostSource) == scheduler_setting.DynamicBillingCostSourceProfit24h {
		return buildProfit24hRatioBaselines(db, logDB, setting, now, filter)
	}
	return buildSampleCostRatioBaselinesWithFilter(db, logDB, setting, now, filter)
}

func buildSampleCostRatioBaselinesWithFilter(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64, filter SnapshotFilter) (map[string]RatioBaseline, error) {
	if db == nil || logDB == nil {
		return map[string]RatioBaseline{}, nil
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	windowSamples := setting.DynamicBillingWindowSamples
	windowMinutes := 0
	if windowSamples <= 0 {
		windowMinutes = setting.DynamicBillingWindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = scheduler_setting.DefaultSetting().DynamicBillingWindowMinutes
		}
	}
	profitRate := setting.DynamicBillingProfitRate
	if profitRate < 0 {
		profitRate = 0
	}
	rows, windowStart, err := loadBaselineRows(db, logDB, now, windowSamples, windowMinutes, filter)
	if err != nil {
		return nil, err
	}
	result := make(map[string]RatioBaseline, len(rows))
	type groupAccumulator struct {
		Group           string
		SampleCount     int
		ModelCount      int
		TotalCost       float64
		TotalBaseQuota  float64
		CostRatioSum    float64
		CostRatioWeight float64
		ReferenceRow    *baselineRow
	}
	accumulators := make(map[string]*groupAccumulator, len(rows))
	for _, row := range rows {
		group := strings.TrimSpace(row.Group)
		modelName := strings.TrimSpace(row.ModelName)
		if group == "" || modelName == "" || row.CostTotal <= 0 || row.QuotaTotal <= 0 {
			continue
		}
		modelRatio, ok, _ := ratio_setting.GetModelRatio(modelName)
		if !ok || modelRatio <= 0 {
			continue
		}
		baseQuotaPerStaticRatio := row.baseQuotaPerStaticRatio(modelRatio)
		if baseQuotaPerStaticRatio <= 0 {
			continue
		}
		accumulator := accumulators[group]
		if accumulator == nil {
			accumulator = &groupAccumulator{Group: group}
			accumulators[group] = accumulator
		}
		accumulator.SampleCount += row.SampleCount
		accumulator.ModelCount++
		accumulator.TotalCost += row.CostTotal
		accumulator.TotalBaseQuota += baseQuotaPerStaticRatio
		if row.CostMultiplier > 0 && row.CostMultiplierWeight > 0 {
			accumulator.CostRatioSum += row.CostMultiplier * row.CostMultiplierWeight
			accumulator.CostRatioWeight += row.CostMultiplierWeight
		}
		if shouldReplaceReferenceRow(accumulator.ReferenceRow, &row) {
			rowCopy := row
			accumulator.ReferenceRow = &rowCopy
		}
	}
	for _, accumulator := range accumulators {
		if accumulator == nil || accumulator.TotalCost <= 0 || accumulator.TotalBaseQuota <= 0 {
			continue
		}
		ratio := accumulator.TotalCost * (1 + profitRate) * common.QuotaPerUnit / accumulator.TotalBaseQuota
		costMultiplier := 0.0
		if accumulator.CostRatioWeight > 0 {
			costMultiplier = accumulator.CostRatioSum / accumulator.CostRatioWeight
			ratio = costMultiplier * (1 + profitRate)
		}
		if ratio <= 0 {
			continue
		}
		referenceModel := ""
		if accumulator.ReferenceRow != nil {
			referenceModel = strings.TrimSpace(accumulator.ReferenceRow.ModelName)
		}
		baseline := RatioBaseline{
			RequestedModel:    referenceModel,
			ReferenceModel:    referenceModel,
			Group:             accumulator.Group,
			Ratio:             ratio,
			PricePerM:         requestedModelPricePerMillion(referenceModel, ratio),
			SampleCount:       accumulator.SampleCount,
			ModelCount:        accumulator.ModelCount,
			CalculatedAt:      now,
			WindowStart:       windowStart,
			WindowEnd:         now,
			ProfitRate:        profitRate,
			CostSource:        scheduler_setting.DynamicBillingCostSourceSampleCost,
			ApplyMode:         normalizeApplyMode(setting.DynamicBillingApplyMode),
			ApplyReason:       applyReasonForConfiguredMode(setting.DynamicBillingApplyMode),
			UpstreamCostUSD:   accumulator.TotalCost,
			BaseQuotaAtRatio1: accumulator.TotalBaseQuota,
			CostMultiplier:    costMultiplier,
			TargetRatio:       ratio,
			EffectiveRatio:    ratio,
		}
		result[groupCacheKey(accumulator.Group)] = baseline
	}
	return result, nil
}

type baselineRow struct {
	ModelName            string
	Group                string
	SampleCount          int
	LatestCalculatedAt   int64
	PromptSum            int64
	CompleteSum          int64
	CacheSum             int64
	CacheWriteSum        int64
	ImageSum             int64
	AudioSum             int64
	ToolQuotaSum         int64
	ModelPriceSum        float64
	BaseQuotaSum         float64
	QuotaTotal           int64
	CostTotal            float64
	CostMultiplier       float64
	CostMultiplierWeight float64
}

type costRow struct {
	ID            int
	RequestId     string
	Cost          float64
	BreakdownJSON string
	CalculatedAt  int64
}

func (r baselineRow) TokenTotal() float64 {
	return float64(r.PromptSum + r.CompleteSum)
}

func (r baselineRow) tokenTotal() float64 {
	return r.TokenTotal()
}

func (r baselineRow) baseQuotaPerStaticRatio(modelRatio float64) float64 {
	if r.BaseQuotaSum > 0 {
		return r.BaseQuotaSum
	}
	if r.ModelPriceSum > 0 {
		return r.ModelPriceSum * common.QuotaPerUnit
	}
	baseTokens := float64(r.PromptSum - r.CacheSum - r.CacheWriteSum - r.ImageSum - r.AudioSum)
	if baseTokens < 0 {
		baseTokens = 0
	}
	return baseTokens*modelRatio + float64(r.CompleteSum)*modelRatio*ratio_setting.GetCompletionRatio(r.ModelName) +
		float64(r.CacheSum)*modelRatio*cacheRatio(r.ModelName) +
		float64(r.CacheWriteSum)*modelRatio*cacheWriteRatio(r.ModelName) +
		float64(r.ImageSum)*modelRatio*imageRatio(r.ModelName) +
		float64(r.AudioSum)*modelRatio*audioRatio(r.ModelName) +
		float64(r.ToolQuotaSum)
}

func requestedModelPricePerMillion(modelName string, ratio float64) float64 {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || ratio <= 0 {
		return 0
	}
	value, usePrice, ok := ratio_setting.GetModelRatioOrPrice(modelName)
	if !ok || value <= 0 {
		return 0
	}
	if usePrice {
		return value * ratio
	}
	return value * 2 * ratio
}

func cacheRatio(modelName string) float64 {
	ratio, _ := ratio_setting.GetCacheRatio(modelName)
	return ratio
}

func cacheWriteRatio(modelName string) float64 {
	ratio, _ := ratio_setting.GetCreateCacheRatio(modelName)
	return ratio
}

func imageRatio(modelName string) float64 {
	ratio, _ := ratio_setting.GetImageRatio(modelName)
	return ratio
}

func audioRatio(modelName string) float64 {
	return ratio_setting.GetAudioRatio(modelName)
}

func loadBaselineRows(db *gorm.DB, logDB *gorm.DB, windowEnd int64, windowSamples int, windowMinutes int, filter SnapshotFilter) ([]baselineRow, int64, error) {
	queryBase := db.Model(&model.ModelGatewayRequestCostSummary{}).
		Select("id, request_id, upstream_cost_total AS cost, breakdown_json, calculated_at").
		Where("upstream_cost_total > 0")
	if filter.MinCalculatedAt > 0 {
		queryBase = queryBase.Where("calculated_at >= ?", filter.MinCalculatedAt)
	}
	if windowSamples <= 0 && windowMinutes > 0 {
		windowStart := windowEnd - int64(windowMinutes)*60
		if filter.MinCalculatedAt > windowStart {
			windowStart = filter.MinCalculatedAt
		}
		queryBase = queryBase.Where("calculated_at >= ? AND calculated_at <= ?", windowStart, windowEnd).
			Order("calculated_at desc, id desc")
	}
	if windowSamples <= 0 && windowMinutes <= 0 {
		windowSamples = scheduler_setting.DefaultSetting().DynamicBillingWindowSamples
	}

	if windowSamples > 0 {
		return loadBaselineRowsByValidSamples(queryBase, logDB, windowSamples)
	}
	return loadBaselineRowsByTimeWindow(queryBase, logDB)
}

func loadBaselineRowsByValidSamples(query *gorm.DB, logDB *gorm.DB, targetSamples int) ([]baselineRow, int64, error) {
	if targetSamples <= 0 {
		return nil, 0, nil
	}
	pageSize := targetSamples
	if pageSize < 200 {
		pageSize = 200
	}
	if pageSize > 500 {
		pageSize = 500
	}

	collected := make([]requestCostLogPair, 0, targetSamples)
	seenRequestIDs := make(map[string]struct{})
	cursorCalculatedAt := int64(0)
	cursorID := 0

	for {
		batch := make([]costRow, 0, pageSize)
		pageQuery := query.Session(&gorm.Session{}).Order("calculated_at desc, id desc").Limit(pageSize)
		if cursorCalculatedAt > 0 {
			pageQuery = pageQuery.Where(
				"(calculated_at < ?) OR (calculated_at = ? AND id < ?)",
				cursorCalculatedAt,
				cursorCalculatedAt,
				cursorID,
			)
		}
		if err := pageQuery.Find(&batch).Error; err != nil {
			return nil, 0, err
		}
		if len(batch) == 0 {
			break
		}

		pairs, err := loadEligibleRequestCostLogPairs(logDB, batch)
		if err != nil {
			return nil, 0, err
		}
		for _, pair := range pairs {
			requestID := strings.TrimSpace(pair.RequestID)
			if requestID == "" {
				continue
			}
			if _, exists := seenRequestIDs[requestID]; exists {
				continue
			}
			seenRequestIDs[requestID] = struct{}{}
			collected = append(collected, pair)
			if len(collected) >= targetSamples {
				break
			}
		}

		last := batch[len(batch)-1]
		cursorCalculatedAt = last.CalculatedAt
		cursorID = last.ID
		if len(collected) >= targetSamples || len(batch) < pageSize {
			break
		}
	}

	return rollupBaselineRowsFromPairs(collected)
}

func loadBaselineRowsByTimeWindow(query *gorm.DB, logDB *gorm.DB) ([]baselineRow, int64, error) {
	costRows := make([]costRow, 0)
	if err := query.Session(&gorm.Session{}).Find(&costRows).Error; err != nil {
		return nil, 0, err
	}
	pairs, err := loadEligibleRequestCostLogPairs(logDB, costRows)
	if err != nil {
		return nil, 0, err
	}
	return rollupBaselineRowsFromPairs(pairs)
}

type requestCostLogPair struct {
	RequestID            string
	Cost                 float64
	BaseQuotaAtRatio1    float64
	CostMultiplier       float64
	CostMultiplierWeight float64
	CalculatedAt         int64
	Log                  model.Log
}

func loadEligibleRequestCostLogPairs(logDB *gorm.DB, costRows []costRow) ([]requestCostLogPair, error) {
	if len(costRows) == 0 {
		return nil, nil
	}
	costByRequest := make(map[string]requestCostLogPair, len(costRows))
	requestIDs := make([]string, 0, len(costRows))
	for _, row := range costRows {
		requestID := strings.TrimSpace(row.RequestId)
		if requestID == "" || row.Cost <= 0 {
			continue
		}
		entry, exists := costByRequest[requestID]
		if !exists {
			entry = requestCostLogPair{
				RequestID:    requestID,
				CalculatedAt: row.CalculatedAt,
			}
			requestIDs = append(requestIDs, requestID)
		}
		entry.Cost += row.Cost
		if multiplier, ok := modelgatewaycost.MultiplierFromBreakdownJSON(row.BreakdownJSON); ok {
			entry.BaseQuotaAtRatio1 += row.Cost / multiplier * common.QuotaPerUnit
		}
		if row.CalculatedAt > 0 && (entry.CalculatedAt <= 0 || row.CalculatedAt < entry.CalculatedAt) {
			entry.CalculatedAt = row.CalculatedAt
		}
		costByRequest[requestID] = entry
	}
	if len(requestIDs) == 0 {
		return nil, nil
	}

	logRows := make([]model.Log, 0, len(requestIDs))
	if err := logDB.Model(&model.Log{}).
		Where("request_id IN ?", requestIDs).
		Where("type = ? AND quota > 0", model.LogTypeConsume).
		Order("created_at desc, id desc").
		Find(&logRows).Error; err != nil {
		return nil, err
	}

	pairs := make([]requestCostLogPair, 0, len(requestIDs))
	seenLogRequestIDs := make(map[string]struct{}, len(requestIDs))
	for _, log := range logRows {
		requestID := strings.TrimSpace(log.RequestId)
		if requestID == "" {
			continue
		}
		if _, exists := seenLogRequestIDs[requestID]; exists {
			continue
		}
		costPair, exists := costByRequest[requestID]
		if !exists || costPair.Cost <= 0 {
			continue
		}
		if costPair.BaseQuotaAtRatio1 > 0 {
			costPair.CostMultiplier = costPair.Cost * common.QuotaPerUnit / costPair.BaseQuotaAtRatio1
		}
		if log.Quota <= 0 || log.PromptTokens+log.CompletionTokens <= 0 {
			continue
		}
		if skipLog(log) {
			continue
		}
		modelName := strings.TrimSpace(log.ModelName)
		group := strings.TrimSpace(log.Group)
		if modelName == "" || group == "" {
			continue
		}
		other := parseOther(log.Other)
		costPair.BaseQuotaAtRatio1 = requestBaseQuotaAtRatio1(costPair, log, other)
		if costPair.BaseQuotaAtRatio1 <= 0 {
			continue
		}
		if costPair.CostMultiplier > 0 {
			costPair.CostMultiplierWeight = costPair.BaseQuotaAtRatio1
		} else {
			costPair.CostMultiplierWeight = 0
		}
		costPair.Log = log
		pairs = append(pairs, costPair)
		seenLogRequestIDs[requestID] = struct{}{}
	}
	return pairs, nil
}

func rollupBaselineRowsFromPairs(pairs []requestCostLogPair) ([]baselineRow, int64, error) {
	if len(pairs) == 0 {
		return nil, 0, nil
	}
	rollups := map[string]baselineRow{}
	oldestCalculatedAt := int64(0)
	for _, pair := range pairs {
		log := pair.Log
		if pair.Cost <= 0 || log.Quota <= 0 {
			continue
		}
		if pair.CalculatedAt > 0 && (oldestCalculatedAt <= 0 || pair.CalculatedAt < oldestCalculatedAt) {
			oldestCalculatedAt = pair.CalculatedAt
		}
		modelName := strings.TrimSpace(log.ModelName)
		group := strings.TrimSpace(log.Group)
		other := parseOther(log.Other)
		groupRatio := floatMapValue(other, "group_ratio")
		if groupRatio <= 0 {
			groupRatio = ratio_setting.GetGroupRatio(group)
		}
		key := cacheKey(modelName, group)
		row := rollups[key]
		row.ModelName = modelName
		row.Group = group
		row.SampleCount++
		if pair.CalculatedAt > row.LatestCalculatedAt {
			row.LatestCalculatedAt = pair.CalculatedAt
		}
		row.PromptSum += int64(log.PromptTokens)
		row.CompleteSum += int64(log.CompletionTokens)
		row.CacheSum += int64(intMapValue(other, "cache_tokens"))
		row.CacheWriteSum += int64(cacheWriteTokensFromOther(other))
		row.ImageSum += int64(intMapValue(other, "image_output"))
		row.AudioSum += int64(intMapValue(other, "audio_input_token_count"))
		row.ToolQuotaSum += int64(toolQuotaFromOther(other, groupRatio))
		row.ModelPriceSum += floatMapValue(other, "model_price")
		row.BaseQuotaSum += pair.BaseQuotaAtRatio1
		row.QuotaTotal += int64(log.Quota)
		row.CostTotal += pair.Cost
		if pair.CostMultiplier > 0 && pair.CostMultiplierWeight > 0 {
			row.CostMultiplier += pair.CostMultiplier * pair.CostMultiplierWeight
			row.CostMultiplierWeight += pair.CostMultiplierWeight
		}
		rollups[key] = row
	}

	result := make([]baselineRow, 0, len(rollups))
	for _, row := range rollups {
		if row.CostMultiplierWeight > 0 {
			row.CostMultiplier = row.CostMultiplier / row.CostMultiplierWeight
		}
		result = append(result, row)
	}
	return result, oldestCalculatedAt, nil
}

func requestBaseQuotaAtRatio1(pair requestCostLogPair, log model.Log, other map[string]interface{}) float64 {
	if pair.BaseQuotaAtRatio1 > 0 {
		return pair.BaseQuotaAtRatio1
	}
	if pair.Cost > 0 && pair.CostMultiplier > 0 {
		return pair.Cost / pair.CostMultiplier * common.QuotaPerUnit
	}
	return logBaseQuotaAtRatio1(log, other)
}

func logBaseQuotaAtRatio1(log model.Log, other map[string]interface{}) float64 {
	if log.Quota <= 0 {
		return 0
	}
	groupRatio := floatMapValue(other, "group_ratio")
	if groupRatio <= 0 {
		groupRatio = ratio_setting.GetGroupRatio(strings.TrimSpace(log.Group))
	}
	if groupRatio <= 0 {
		return 0
	}
	return float64(log.Quota) / groupRatio
}

func skipLog(log model.Log) bool {
	other := parseOther(log.Other)
	if boolMapValue(other, "is_health_probe") || boolMapValue(other, "client_aborted") || boolMapValue(other, "stream_interrupted") || boolMapValue(other, "empty_output") {
		return true
	}
	if strings.TrimSpace(stringMapValue(other, "experience_issue")) != "" {
		return true
	}
	if strings.TrimSpace(stringMapValue(other, "billing_source")) == "model_gateway_probe" {
		return true
	}
	return false
}

func parseOther(other string) map[string]interface{} {
	if strings.TrimSpace(other) == "" {
		return map[string]interface{}{}
	}
	result := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(other, &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

func cacheWriteTokensFromOther(values map[string]interface{}) int {
	if value := intMapValue(values, "cache_write_tokens"); value > 0 {
		return value
	}
	return intMapValue(values, "cache_creation_tokens") +
		intMapValue(values, "cache_creation_tokens_5m") +
		intMapValue(values, "cache_creation_tokens_1h")
}

func toolQuotaFromOther(values map[string]interface{}, groupRatio float64) int {
	if groupRatio <= 0 {
		return 0
	}
	result := 0.0
	if calls := intMapValue(values, "web_search_call_count"); calls > 0 {
		result += floatMapValue(values, "web_search_price") * float64(calls) / 1000 * common.QuotaPerUnit * groupRatio
	}
	if calls := intMapValue(values, "file_search_call_count"); calls > 0 {
		result += floatMapValue(values, "file_search_price") * float64(calls) / 1000 * common.QuotaPerUnit * groupRatio
	}
	if calls := intMapValue(values, "image_generation_call_count"); calls > 0 {
		result += floatMapValue(values, "image_generation_call_price") * float64(calls) * common.QuotaPerUnit * groupRatio
	}
	return int(result / groupRatio)
}

func intMapValue(values map[string]interface{}, key string) int {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case uint:
		return int(typed)
	case uint64:
		return int(typed)
	case uint32:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func floatMapValue(values map[string]interface{}, key string) float64 {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint64:
		return float64(typed)
	case uint32:
		return float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func boolMapValue(values map[string]interface{}, key string) bool {
	value, ok := values[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	default:
		return false
	}
}

func stringMapValue(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func cacheKey(requestedModel string, group string) string {
	_ = requestedModel
	return groupCacheKey(group)
}

func groupCacheKey(group string) string {
	group = strings.ToLower(strings.TrimSpace(group))
	if group == "" {
		return ""
	}
	return group
}

func normalizeCostSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case scheduler_setting.DynamicBillingCostSourceProfit24h:
		return scheduler_setting.DynamicBillingCostSourceProfit24h
	default:
		return scheduler_setting.DynamicBillingCostSourceSampleCost
	}
}

func isAutoAppliedLegacyFallback(reason string) bool {
	switch strings.TrimSpace(reason) {
	case FallbackManualConfirm, FallbackStepChangeTooLarge:
		return true
	default:
		return false
	}
}

func IsAutoAppliedLegacyFallback(reason string) bool {
	return isAutoAppliedLegacyFallback(reason)
}

func applyReasonForLegacyFallback(reason string) string {
	switch strings.TrimSpace(reason) {
	case FallbackStepChangeTooLarge:
		return ApplyReasonStepChangeAutoApplied
	case FallbackManualConfirm:
		return ApplyReasonManualModeAutoApplied
	default:
		return ""
	}
}

func defaultApplyReason(applyMode string) string {
	if normalizeApplyMode(applyMode) == scheduler_setting.DynamicBillingApplyModeManual {
		return ApplyReasonManualModeAutoApplied
	}
	return ApplyReasonAutoApplied
}

func applyReasonForConfiguredMode(applyMode string) string {
	if normalizeApplyMode(applyMode) == scheduler_setting.DynamicBillingApplyModeObserve {
		return ""
	}
	return defaultApplyReason(applyMode)
}

func normalizeApplyMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case scheduler_setting.DynamicBillingApplyModeObserve:
		return scheduler_setting.DynamicBillingApplyModeObserve
	case scheduler_setting.DynamicBillingApplyModeAuto:
		return scheduler_setting.DynamicBillingApplyModeAuto
	case scheduler_setting.DynamicBillingApplyModeManual:
		return scheduler_setting.DynamicBillingApplyModeManual
	default:
		return scheduler_setting.DynamicBillingApplyModeAuto
	}
}

func isProfit24hBaseline(baseline RatioBaseline) bool {
	return normalizeCostSource(baseline.CostSource) == scheduler_setting.DynamicBillingCostSourceProfit24h
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func shouldReplaceReferenceRow(current *baselineRow, candidate *baselineRow) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.LatestCalculatedAt != current.LatestCalculatedAt {
		return candidate.LatestCalculatedAt > current.LatestCalculatedAt
	}
	if candidate.SampleCount != current.SampleCount {
		return candidate.SampleCount > current.SampleCount
	}
	if candidate.CostTotal != current.CostTotal {
		return candidate.CostTotal > current.CostTotal
	}
	return strings.TrimSpace(candidate.ModelName) < strings.TrimSpace(current.ModelName)
}

func loadPersistedBaselines(db *gorm.DB) map[string]RatioBaseline {
	if db == nil {
		return nil
	}
	rows := make([]model.ModelGatewayDynamicBillingBaseline, 0)
	if err := db.Order("billing_group asc").Find(&rows).Error; err != nil {
		common.SysLog(fmt.Sprintf("model gateway dynamic billing persisted baseline load failed: %v", err))
		return nil
	}
	values := make(map[string]RatioBaseline, len(rows))
	for _, row := range rows {
		group := strings.TrimSpace(row.BillingGroup)
		if group == "" || row.Ratio <= 0 {
			continue
		}
		values[groupCacheKey(group)] = RatioBaseline{
			RequestedModel:       strings.TrimSpace(row.ReferenceModel),
			ReferenceModel:       strings.TrimSpace(row.ReferenceModel),
			Group:                group,
			Ratio:                row.Ratio,
			PricePerM:            row.ReferencePricePerM,
			SampleCount:          row.SampleCount,
			ModelCount:           row.ModelCount,
			CalculatedAt:         row.CalculatedAt,
			WindowStart:          row.WindowStart,
			WindowEnd:            row.WindowEnd,
			ProfitRate:           row.ProfitRate,
			CostSource:           normalizeCostSource(row.CostSource),
			ApplyMode:            normalizeApplyMode(row.ApplyMode),
			ApplyReason:          strings.TrimSpace(row.ApplyReason),
			OperatingCostUSD:     row.OperatingCostUSD,
			RequiredRevenueUSD:   row.RequiredRevenueUSD,
			BaseQuotaAtRatio1:    row.BaseQuotaAtRatio1,
			CostMultiplier:       row.CostMultiplier,
			TargetRatio:          row.TargetRatio,
			EffectiveRatio:       row.EffectiveRatio,
			Clamped:              row.Clamped,
			PendingManualConfirm: row.PendingManualConfirm,
			FallbackReason:       strings.TrimSpace(row.FallbackReason),
			RequestCount:         row.RequestCount,
			SuccessRequestCount:  row.SuccessRequestCount,
			TotalTokens:          row.TotalTokens,
			TrafficCostUSD:       row.TrafficCostUSD,
			TrafficEstimated:     row.TrafficEstimated,
			TrafficDataReady:     row.TrafficDataReady,
			ServerCostUSD:        row.ServerCostUSD,
			ResourceCostUSD:      row.ResourceCostUSD,
			UpstreamCostUSD:      row.UpstreamCostUSD,
		}
	}
	return values
}

func persistBaselines(db *gorm.DB, values map[string]RatioBaseline) error {
	if db == nil {
		return nil
	}
	rows := make([]model.ModelGatewayDynamicBillingBaseline, 0, len(values))
	groups := make([]string, 0, len(values))
	now := common.GetTimestamp()
	for _, value := range values {
		group := strings.TrimSpace(value.Group)
		if group == "" || value.Ratio <= 0 {
			continue
		}
		referenceModel := strings.TrimSpace(value.ReferenceModel)
		if referenceModel == "" {
			referenceModel = strings.TrimSpace(value.RequestedModel)
		}
		groups = append(groups, group)
		rows = append(rows, model.ModelGatewayDynamicBillingBaseline{
			BillingGroup:         group,
			ReferenceModel:       referenceModel,
			Ratio:                value.Ratio,
			ReferencePricePerM:   value.PricePerM,
			SampleCount:          value.SampleCount,
			ModelCount:           value.ModelCount,
			WindowStart:          value.WindowStart,
			WindowEnd:            value.WindowEnd,
			ProfitRate:           value.ProfitRate,
			CostSource:           normalizeCostSource(value.CostSource),
			ApplyMode:            normalizeApplyMode(value.ApplyMode),
			ApplyReason:          strings.TrimSpace(value.ApplyReason),
			OperatingCostUSD:     value.OperatingCostUSD,
			RequiredRevenueUSD:   value.RequiredRevenueUSD,
			BaseQuotaAtRatio1:    value.BaseQuotaAtRatio1,
			CostMultiplier:       value.CostMultiplier,
			TargetRatio:          value.TargetRatio,
			EffectiveRatio:       value.EffectiveRatio,
			Clamped:              value.Clamped,
			PendingManualConfirm: value.PendingManualConfirm,
			FallbackReason:       strings.TrimSpace(value.FallbackReason),
			RequestCount:         value.RequestCount,
			SuccessRequestCount:  value.SuccessRequestCount,
			TotalTokens:          value.TotalTokens,
			TrafficCostUSD:       value.TrafficCostUSD,
			TrafficEstimated:     value.TrafficEstimated,
			TrafficDataReady:     value.TrafficDataReady,
			ServerCostUSD:        value.ServerCostUSD,
			ResourceCostUSD:      value.ResourceCostUSD,
			UpstreamCostUSD:      value.UpstreamCostUSD,
			CalculatedAt:         value.CalculatedAt,
			UpdatedAt:            now,
		})
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if len(rows) == 0 {
			return tx.Where("1 = 1").Delete(&model.ModelGatewayDynamicBillingBaseline{}).Error
		}
		for _, row := range rows {
			existing := model.ModelGatewayDynamicBillingBaseline{}
			err := tx.Where("billing_group = ?", row.BillingGroup).First(&existing).Error
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					row.CreatedAt = now
					if row.CalculatedAt <= 0 {
						row.CalculatedAt = now
					}
					if err := tx.Create(&row).Error; err != nil {
						return err
					}
					continue
				}
				return err
			}
			updates := map[string]interface{}{
				"reference_model":        row.ReferenceModel,
				"ratio":                  row.Ratio,
				"reference_price_per_m":  row.ReferencePricePerM,
				"sample_count":           row.SampleCount,
				"model_count":            row.ModelCount,
				"window_start":           row.WindowStart,
				"window_end":             row.WindowEnd,
				"profit_rate":            row.ProfitRate,
				"cost_source":            row.CostSource,
				"apply_mode":             row.ApplyMode,
				"apply_reason":           row.ApplyReason,
				"operating_cost_usd":     row.OperatingCostUSD,
				"required_revenue_usd":   row.RequiredRevenueUSD,
				"base_quota_at_ratio_1":  row.BaseQuotaAtRatio1,
				"cost_multiplier":        row.CostMultiplier,
				"target_ratio":           row.TargetRatio,
				"effective_ratio":        row.EffectiveRatio,
				"clamped":                row.Clamped,
				"pending_manual_confirm": row.PendingManualConfirm,
				"fallback_reason":        row.FallbackReason,
				"request_count":          row.RequestCount,
				"success_request_count":  row.SuccessRequestCount,
				"total_tokens":           row.TotalTokens,
				"traffic_cost_usd":       row.TrafficCostUSD,
				"traffic_estimated":      row.TrafficEstimated,
				"traffic_data_ready":     row.TrafficDataReady,
				"server_cost_usd":        row.ServerCostUSD,
				"resource_cost_usd":      row.ResourceCostUSD,
				"upstream_cost_usd":      row.UpstreamCostUSD,
				"calculated_at":          row.CalculatedAt,
				"updated_at":             now,
			}
			if err := tx.Model(&model.ModelGatewayDynamicBillingBaseline{}).
				Where("billing_group = ?", row.BillingGroup).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		return tx.Where("billing_group NOT IN ?", groups).Delete(&model.ModelGatewayDynamicBillingBaseline{}).Error
	})
}
