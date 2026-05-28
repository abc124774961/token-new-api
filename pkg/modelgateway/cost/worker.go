package cost

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"gorm.io/gorm/clause"
)

type WorkerConfig struct {
	Enabled  bool
	Interval time.Duration
	Workers  int
	Batch    int
}

type Worker struct {
	config            WorkerConfig
	cache             *ProfileCache
	stop              chan struct{}
	once              sync.Once
	cursorMu          sync.Mutex
	lastLogID         int
	cursorInitialized bool
}

type ProfileCache struct {
	mu       sync.RWMutex
	profiles map[string]model.ModelGatewayChannelCostProfile
	loadedAt time.Time
}

var (
	defaultWorkerMu sync.Mutex
	defaultWorker   *Worker
	defaultCache    = &ProfileCache{}
)

func NewWorker(config WorkerConfig) *Worker {
	config = normalizeWorkerConfig(config)
	return &Worker{
		config: config,
		cache:  defaultCache,
		stop:   make(chan struct{}),
	}
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || !w.config.Enabled {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !common.IsMasterNode {
		w.once.Do(func() {
			go w.runCacheRefresher(ctx)
		})
		common.SysLog("model gateway cost worker skipped on non-master node; profile cache refresher started")
		return
	}
	w.once.Do(func() {
		go w.run(ctx)
	})
}

func (w *Worker) Stop() {
	if w == nil || w.stop == nil {
		return
	}
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
}

func (w *Worker) run(ctx context.Context) {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	common.SysLog(fmt.Sprintf("model gateway cost worker started: interval=%s workers=%d batch=%d", w.config.Interval, w.config.Workers, w.config.Batch))
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) runCacheRefresher(ctx context.Context) {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	w.refreshProfileCache(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			w.refreshProfileCache(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if w == nil || model.DB == nil || model.LOG_DB == nil {
		return
	}
	w.refreshProfileCache(ctx)
	logs, scannedThroughID, err := w.loadPendingConsumeLogs(w.config.Batch)
	if err != nil {
		common.SysLog(fmt.Sprintf("model gateway cost pending log load failed: %v", err))
		return
	}
	if len(logs) == 0 {
		w.advanceCursor(scannedThroughID)
		return
	}

	jobs := make(chan model.Log)
	results := make(chan costLogResult, len(logs))
	var wg sync.WaitGroup
	workers := w.config.Workers
	if workers > len(logs) {
		workers = len(logs)
	}
	for idx := 0; idx < workers; idx++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for log := range jobs {
				if err := w.calculateLog(log); err != nil {
					results <- costLogResult{id: log.Id, requestID: log.RequestId, err: err}
					continue
				}
				results <- costLogResult{id: log.Id, requestID: log.RequestId}
			}
		}()
	}
	for _, log := range logs {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- log:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	failedMinID := 0
	for result := range results {
		if result.err == nil {
			continue
		}
		common.SysLog(fmt.Sprintf("model gateway cost calculation failed: request_id=%s error=%v", result.requestID, result.err))
		if result.id > 0 && (failedMinID == 0 || result.id < failedMinID) {
			failedMinID = result.id
		}
	}
	if failedMinID > 0 {
		w.advanceCursor(failedMinID - 1)
		return
	}
	w.advanceCursor(scannedThroughID)
}

func (w *Worker) refreshProfileCache(ctx context.Context) {
	if w == nil || w.cache == nil || model.DB == nil {
		return
	}
	if err := w.cache.Refresh(ctx); err != nil {
		common.SysLog(fmt.Sprintf("model gateway cost profile refresh failed: %v", err))
	}
}

func (w *Worker) calculateLog(log model.Log) error {
	usage := UsageSnapshotFromLog(log)
	if strings.TrimSpace(usage.RequestID) == "" {
		return nil
	}
	profile := w.cache.Lookup(usage.ChannelID, usage.UpstreamModel)
	if profile == nil {
		profile = DefaultSystemRatioProfile(usage.ChannelID)
	}
	result := Calculate(usage, profile)
	now := common.GetTimestamp()
	summary := result.Summary(now)
	return upsertCostSummary(summary)
}

type costLogResult struct {
	id        int
	requestID string
	err       error
}

func (w *Worker) loadPendingConsumeLogs(limit int) ([]model.Log, int, error) {
	if limit <= 0 {
		limit = 100
	}
	if w == nil {
		return nil, 0, nil
	}

	if !w.isCursorInitialized() {
		rows, scannedThroughID, err := loadRecentConsumeLogsForBootstrap(limit)
		if err != nil {
			return nil, 0, err
		}
		pending, err := filterPendingConsumeLogs(rows, limit)
		if err != nil {
			return nil, 0, err
		}
		w.markCursorInitialized()
		return pending, scannedThroughID, err
	}

	cursor := w.currentCursor()
	rows := make([]model.Log, 0, limit)
	err := model.LOG_DB.
		Where("id > ? AND type = ? AND request_id <> ''", cursor, model.LogTypeConsume).
		Order("id asc").
		Limit(limit).
		Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, cursor, err
	}
	scannedThroughID := maxLogID(rows)
	pending, err := filterPendingConsumeLogs(rows, limit)
	return pending, scannedThroughID, err
}

func loadRecentConsumeLogsForBootstrap(limit int) ([]model.Log, int, error) {
	scanLimit := limit
	if scanLimit < 50 {
		scanLimit = 50
	}
	if scanLimit > 200 {
		scanLimit = 200
	}
	rows := make([]model.Log, 0, scanLimit)
	err := model.LOG_DB.
		Where("type = ? AND request_id <> ''", model.LogTypeConsume).
		Order("id desc").
		Limit(scanLimit).
		Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, 0, err
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return rows, maxLogID(rows), nil
}

func filterPendingConsumeLogs(rows []model.Log, limit int) ([]model.Log, error) {
	requestIDs := make([]string, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	logByRequestID := make(map[string]model.Log, len(rows))
	for _, row := range rows {
		requestID := strings.TrimSpace(row.RequestId)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
		logByRequestID[requestID] = row
	}
	if len(requestIDs) == 0 {
		return nil, nil
	}
	existing := make([]model.ModelGatewayRequestCostSummary, 0, len(requestIDs))
	if err := model.DB.
		Select("request_id, upstream_cost_total, breakdown_json, cost_source, cost_accuracy").
		Where("request_id IN ?", requestIDs).
		Find(&existing).Error; err != nil {
		return nil, err
	}
	for _, summary := range existing {
		if shouldSkipExistingCostSummary(summary) {
			delete(logByRequestID, strings.TrimSpace(summary.RequestId))
		}
	}
	pending := make([]model.Log, 0, minInt(limit, len(logByRequestID)))
	for _, requestID := range requestIDs {
		row, ok := logByRequestID[requestID]
		if !ok {
			continue
		}
		pending = append(pending, row)
		if len(pending) >= limit {
			break
		}
	}
	return pending, nil
}

func shouldSkipExistingCostSummary(summary model.ModelGatewayRequestCostSummary) bool {
	source := strings.TrimSpace(summary.CostSource)
	accuracy := strings.TrimSpace(summary.CostAccuracy)
	if source == "" || accuracy == "" {
		return false
	}
	if source == SourceMissing || source == SourcePending {
		return false
	}
	if accuracy == AccuracyMissing || accuracy == AccuracyPending {
		return false
	}
	if isDefaultSystemRatioCostSummary(summary) {
		return false
	}
	if summary.UpstreamCostTotal > 0 {
		return true
	}
	return source == SourceSystemRatio && accuracy == "estimated"
}

func isDefaultSystemRatioCostSummary(summary model.ModelGatewayRequestCostSummary) bool {
	if strings.TrimSpace(summary.CostSource) != SourceSystemRatio || strings.TrimSpace(summary.CostAccuracy) != "estimated" {
		return false
	}
	if summary.UpstreamCostTotal <= 0 || strings.TrimSpace(summary.BreakdownJSON) == "" {
		return false
	}
	breakdown := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(summary.BreakdownJSON, &breakdown); err != nil {
		return false
	}
	return mapFloatNearOne(breakdown, "cost_coefficient") &&
		mapFloatNearOne(breakdown, "fee_multiplier") &&
		mapFloatNearOne(breakdown, "token_multiplier") &&
		mapFloatNearOne(breakdown, "recharge_multiplier")
}

func (w *Worker) isCursorInitialized() bool {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	return w.cursorInitialized
}

func (w *Worker) markCursorInitialized() {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	w.cursorInitialized = true
}

func (w *Worker) currentCursor() int {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	return w.lastLogID
}

func (w *Worker) advanceCursor(logID int) {
	if w == nil || logID <= 0 {
		return
	}
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	w.cursorInitialized = true
	if logID > w.lastLogID {
		w.lastLogID = logID
	}
}

func maxLogID(rows []model.Log) int {
	maxID := 0
	for _, row := range rows {
		if row.Id > maxID {
			maxID = row.Id
		}
	}
	return maxID
}

func (c *ProfileCache) Refresh(ctx context.Context) error {
	if c == nil || model.DB == nil {
		return nil
	}
	c.mu.RLock()
	if !c.loadedAt.IsZero() && time.Since(c.loadedAt) < 30*time.Second {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	profiles := make([]model.ModelGatewayChannelCostProfile, 0)
	err := model.DB.WithContext(ctx).
		Where("upstream_model = ?", "*").
		Find(&profiles).Error
	if err != nil {
		return err
	}
	next := make(map[string]model.ModelGatewayChannelCostProfile, len(profiles)*2)
	now := common.GetTimestamp()
	for _, profile := range profiles {
		if profile.ChannelID <= 0 {
			continue
		}
		profile.UpstreamModel = normalizeProfileModel(profile.UpstreamModel)
		if profile.EffectiveTime > now {
			continue
		}
		if profile.Version <= 0 {
			profile.Version = 1
		}
		key := profileKey(profile.ChannelID, profile.UpstreamModel)
		if current, ok := next[key]; !ok || betterProfile(profile, current) {
			next[key] = profile
		}
	}
	c.mu.Lock()
	c.profiles = next
	c.loadedAt = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *ProfileCache) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.loadedAt = time.Time{}
	c.profiles = nil
	c.mu.Unlock()
}

func (c *ProfileCache) Store(profile model.ModelGatewayChannelCostProfile) {
	if c == nil || profile.ChannelID <= 0 {
		return
	}
	profile.UpstreamModel = normalizeProfileModel(profile.UpstreamModel)
	if profile.EffectiveTime > common.GetTimestamp() {
		return
	}
	if profile.Version <= 0 {
		profile.Version = 1
	}
	c.mu.Lock()
	if c.profiles == nil {
		c.profiles = make(map[string]model.ModelGatewayChannelCostProfile)
	}
	c.profiles[profileKey(profile.ChannelID, profile.UpstreamModel)] = profile
	c.mu.Unlock()
}

func (c *ProfileCache) DeleteChannel(channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	prefix := fmt.Sprintf("%d:", channelID)
	c.mu.Lock()
	for key := range c.profiles {
		if strings.HasPrefix(key, prefix) {
			delete(c.profiles, key)
		}
	}
	c.mu.Unlock()
}

func (c *ProfileCache) Lookup(channelID int, upstreamModel string) *model.ModelGatewayChannelCostProfile {
	if c == nil || channelID <= 0 {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.profiles) == 0 {
		return nil
	}
	if profile, ok := c.profiles[profileKey(channelID, upstreamModel)]; ok {
		copy := profile
		return &copy
	}
	if profile, ok := c.profiles[profileKey(channelID, "*")]; ok {
		copy := profile
		return &copy
	}
	return nil
}

func (c *ProfileCache) ReferenceCostRatio(upstreamModel string, pricingMode string) (float64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.profiles) == 0 || c.loadedAt.IsZero() {
		return 0, false
	}
	pricingMode = strings.TrimSpace(strings.ToLower(pricingMode))
	reference := 0.0
	for _, profile := range c.profiles {
		mode := CostPricingModeFromProfileForModel(&profile, upstreamModel)
		if pricingMode != "" && mode != pricingMode {
			continue
		}
		ratio, ok := CostRatioFromProfileForModel(&profile, upstreamModel)
		if !ok || ratio <= 0 {
			continue
		}
		if reference <= 0 || ratio < reference {
			reference = ratio
		}
	}
	if reference <= 0 {
		return 0, false
	}
	return reference, true
}

func (c *ProfileCache) loaded() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.loadedAt.IsZero()
}

func upsertCostSummary(summary model.ModelGatewayRequestCostSummary) error {
	if model.DB == nil || strings.TrimSpace(summary.RequestId) == "" {
		return nil
	}
	return model.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "request_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"channel_id",
			"upstream_model",
			"upstream_cost_total",
			"breakdown_json",
			"cost_source",
			"cost_accuracy",
			"calculated_at",
			"updated_at",
		}),
	}).Create(&summary).Error
}

func profileKey(channelID int, upstreamModel string) string {
	return fmt.Sprintf("%d:%s", channelID, strings.ToLower(normalizeProfileModel(upstreamModel)))
}

func normalizeProfileModel(upstreamModel string) string {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return "*"
	}
	return upstreamModel
}

func betterProfile(next model.ModelGatewayChannelCostProfile, current model.ModelGatewayChannelCostProfile) bool {
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

func LookupCachedDefaultProfile(channelID int, upstreamModel string) *model.ModelGatewayChannelCostProfile {
	return defaultCache.Lookup(channelID, upstreamModel)
}

func LookupDefaultProfile(channelID int, upstreamModel string) *model.ModelGatewayChannelCostProfile {
	return LookupCachedDefaultProfile(channelID, upstreamModel)
}

func LookupCachedReferenceCostRatio(upstreamModel string, pricingMode string) (float64, bool) {
	return defaultCache.ReferenceCostRatio(upstreamModel, pricingMode)
}

func DefaultSystemRatioProfile(channelID int) *model.ModelGatewayChannelCostProfile {
	return &model.ModelGatewayChannelCostProfile{
		ChannelID:             channelID,
		UpstreamModel:         "*",
		Currency:              "USD",
		PricingMode:           "token",
		Source:                SourceSystemRatio,
		Accuracy:              "estimated",
		CostCoefficient:       1,
		InputCostMultiplier:   1,
		OutputCostMultiplier:  1,
		CacheReadMultiplier:   1,
		CacheWriteMultiplier:  1,
		RequestCostMultiplier: 1,
		RechargeMultiplier:    1,
		TokenMultiplier:       1,
		Version:               1,
	}
}

func CostRatioFromProfile(profile *model.ModelGatewayChannelCostProfile) (float64, bool) {
	return CostRatioFromProfileForModel(profile, "")
}

func CostRatioFromProfileForModel(profile *model.ModelGatewayChannelCostProfile, upstreamModel string) (float64, bool) {
	if profile == nil {
		return 0, false
	}
	derived := DeriveSystemRatioProfile(upstreamModel, *profile)
	profile = &derived
	if cost := blendedTokenReferenceCost(*profile); cost > 0 {
		return cost, true
	}
	if profile.RequestPrice > 0 {
		return profile.RequestPrice, true
	}
	return 0, false
}

func CostPricingModeFromProfileForModel(profile *model.ModelGatewayChannelCostProfile, upstreamModel string) string {
	if profile == nil {
		return ""
	}
	derived := DeriveSystemRatioProfile(upstreamModel, *profile)
	if blendedTokenReferenceCost(derived) > 0 {
		return "token"
	}
	if derived.RequestPrice > 0 {
		return "request"
	}
	return ""
}

func blendedTokenReferenceCost(profile model.ModelGatewayChannelCostProfile) float64 {
	input := profile.InputPerMillion
	output := profile.OutputPerMillion
	cacheRead := profile.CacheReadPerMillion
	cacheWrite := profile.CacheWritePerMillion
	if profile.CacheWrite5mPerMillion > 0 {
		cacheWrite = profile.CacheWrite5mPerMillion
	}
	imageInput := profile.ImageInputPerMillion
	audioInput := profile.AudioInputPerMillion
	audioOutput := profile.AudioOutputPerMillion

	switch {
	case input > 0 && output > 0:
		cost := input*0.45 + output*0.45
		weight := 0.90
		if cacheRead > 0 {
			cost += cacheRead * 0.05
			weight += 0.05
		}
		if cacheWrite > 0 {
			cost += cacheWrite * 0.03
			weight += 0.03
		}
		if imageInput > 0 {
			cost += imageInput * 0.01
			weight += 0.01
		}
		if audioInput > 0 {
			cost += audioInput * 0.005
			weight += 0.005
		}
		if audioOutput > 0 {
			cost += audioOutput * 0.005
			weight += 0.005
		}
		if weight > 0 {
			return cost / weight
		}
	case input > 0:
		return input
	case output > 0:
		return output
	}
	return 0
}

func normalizeWorkerConfig(config WorkerConfig) WorkerConfig {
	if config.Interval <= 0 {
		config.Interval = 5 * time.Second
	}
	if config.Workers <= 0 {
		config.Workers = 2
	}
	if config.Batch <= 0 {
		config.Batch = 100
	}
	if config.Batch > 1000 {
		config.Batch = 1000
	}
	return config
}

func SyncDefaultWorkerLifecycle() *Worker {
	defaultWorkerMu.Lock()
	defer defaultWorkerMu.Unlock()
	stopDefaultWorkerLocked()
	setting := scheduler_setting.GetSetting()
	config := WorkerConfig{
		Enabled:  setting.CostCalculationEnabled,
		Interval: time.Duration(setting.CostCalculationIntervalSeconds) * time.Second,
		Workers:  setting.CostCalculationWorkerCount,
		Batch:    setting.CostCalculationBatchSize,
	}
	if !config.Enabled {
		return nil
	}
	worker := NewWorker(config)
	worker.Start(context.Background())
	defaultWorker = worker
	return worker
}

func StopDefaultWorker() {
	defaultWorkerMu.Lock()
	defer defaultWorkerMu.Unlock()
	stopDefaultWorkerLocked()
}

func InvalidateDefaultProfileCache() {
	defaultWorkerMu.Lock()
	defer defaultWorkerMu.Unlock()
	defaultCache.Invalidate()
}

func StoreCachedDefaultProfile(profile model.ModelGatewayChannelCostProfile) {
	defaultCache.Store(profile)
}

func RemoveCachedDefaultProfilesForChannel(channelID int) {
	defaultCache.DeleteChannel(channelID)
}

func stopDefaultWorkerLocked() {
	if defaultWorker == nil {
		return
	}
	defaultWorker.Stop()
	defaultWorker = nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mapFloatNearOne(values map[string]interface{}, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	default:
		return false
	}
	if number > 1 {
		return number-1 < 0.0000001
	}
	return 1-number < 0.0000001
}
