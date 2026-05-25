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
	if input.Setting.DynamicBillingMinSamples > 0 && baseline.SampleCount < input.Setting.DynamicBillingMinSamples {
		snapshot.FallbackReason = FallbackInsufficientSamples
		return snapshot
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
	snapshot.Applied = true
	return snapshot
}

func BuildRatioBaselines(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64) (map[string]RatioBaseline, error) {
	return BuildRatioBaselinesWithFilter(db, logDB, setting, now, SnapshotFilter{})
}

func BuildRatioBaselinesWithFilter(db *gorm.DB, logDB *gorm.DB, setting scheduler_setting.SchedulerSetting, now int64, filter SnapshotFilter) (map[string]RatioBaseline, error) {
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
		Group         string
		SampleCount   int
		ModelCount    int
		TotalCost     float64
		TotalBaseQuota float64
		ReferenceRow  *baselineRow
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
		if ratio <= 0 {
			continue
		}
		referenceModel := ""
		if accumulator.ReferenceRow != nil {
			referenceModel = strings.TrimSpace(accumulator.ReferenceRow.ModelName)
		}
		baseline := RatioBaseline{
			RequestedModel: referenceModel,
			ReferenceModel: referenceModel,
			Group:          accumulator.Group,
			Ratio:          ratio,
			PricePerM:      requestedModelPricePerMillion(referenceModel, ratio),
			SampleCount:    accumulator.SampleCount,
			ModelCount:     accumulator.ModelCount,
			CalculatedAt:   now,
			WindowStart:    windowStart,
			WindowEnd:      now,
			ProfitRate:     profitRate,
		}
		result[groupCacheKey(accumulator.Group)] = baseline
	}
	return result, nil
}

type baselineRow struct {
	ModelName     string
	Group         string
	SampleCount   int
	LatestCalculatedAt int64
	PromptSum     int64
	CompleteSum   int64
	CacheSum      int64
	CacheWriteSum int64
	ImageSum      int64
	AudioSum      int64
	ToolQuotaSum  int64
	ModelPriceSum float64
	BaseQuotaSum  float64
	QuotaTotal    int64
	CostTotal     float64
}

type costRow struct {
	ID           int
	RequestId    string
	Cost         float64
	CalculatedAt int64
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
		Select("id, request_id, upstream_cost_total AS cost, calculated_at").
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
	RequestID    string
	Cost         float64
	CalculatedAt int64
	Log          model.Log
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
		if groupRatio > 0 {
			row.BaseQuotaSum += float64(log.Quota) / groupRatio
		}
		row.QuotaTotal += int64(log.Quota)
		row.CostTotal += pair.Cost
		rollups[key] = row
	}

	result := make([]baselineRow, 0, len(rollups))
	for _, row := range rollups {
		result = append(result, row)
	}
	return result, oldestCalculatedAt, nil
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
			RequestedModel: strings.TrimSpace(row.ReferenceModel),
			ReferenceModel: strings.TrimSpace(row.ReferenceModel),
			Group:          group,
			Ratio:          row.Ratio,
			PricePerM:      row.ReferencePricePerM,
			SampleCount:    row.SampleCount,
			ModelCount:     row.ModelCount,
			CalculatedAt:   row.CalculatedAt,
			WindowStart:    row.WindowStart,
			WindowEnd:      row.WindowEnd,
			ProfitRate:     row.ProfitRate,
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
			BillingGroup:       group,
			ReferenceModel:     referenceModel,
			Ratio:              value.Ratio,
			ReferencePricePerM: value.PricePerM,
			SampleCount:        value.SampleCount,
			ModelCount:         value.ModelCount,
			WindowStart:        value.WindowStart,
			WindowEnd:          value.WindowEnd,
			ProfitRate:         value.ProfitRate,
			CalculatedAt:       value.CalculatedAt,
			UpdatedAt:          now,
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
				"reference_model":      row.ReferenceModel,
				"ratio":                row.Ratio,
				"reference_price_per_m": row.ReferencePricePerM,
				"sample_count":         row.SampleCount,
				"model_count":          row.ModelCount,
				"window_start":         row.WindowStart,
				"window_end":           row.WindowEnd,
				"profit_rate":          row.ProfitRate,
				"calculated_at":        row.CalculatedAt,
				"updated_at":           now,
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
