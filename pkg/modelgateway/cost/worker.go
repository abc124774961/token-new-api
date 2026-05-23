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
	if !common.IsMasterNode {
		common.SysLog("model gateway cost worker skipped on non-master node")
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
	if ctx == nil {
		ctx = context.Background()
	}
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

func (w *Worker) tick(ctx context.Context) {
	if w == nil || model.DB == nil || model.LOG_DB == nil {
		return
	}
	if err := w.cache.Refresh(ctx); err != nil {
		common.SysLog(fmt.Sprintf("model gateway cost profile refresh failed: %v", err))
	}
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
		Select("request_id, upstream_cost_total, cost_source, cost_accuracy").
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
	if summary.UpstreamCostTotal > 0 {
		return true
	}
	return source == SourceSystemRatio && accuracy == "estimated"
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

func DefaultSystemRatioProfile(channelID int) *model.ModelGatewayChannelCostProfile {
	return &model.ModelGatewayChannelCostProfile{
		ChannelID:             channelID,
		UpstreamModel:         "*",
		Currency:              "USD",
		PricingMode:           "token",
		Source:                SourceSystemRatio,
		Accuracy:              "estimated",
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
	// The scheduler needs a cheap, stable relative signal. A simple blended
	// 1M-token scenario keeps this out of the request path and avoids per-request
	// usage assumptions.
	input := profile.InputPerMillion
	output := profile.OutputPerMillion
	switch {
	case input > 0 && output > 0:
		return (input + output) / 2, true
	case input > 0:
		return input, true
	case output > 0:
		return output, true
	case profile.RequestPrice > 0:
		return profile.RequestPrice, true
	default:
		return 0, false
	}
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
