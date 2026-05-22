package scheduler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"gorm.io/gorm/clause"
)

const (
	defaultRuntimeSnapshotPersistenceInterval = 10 * time.Second
	defaultRuntimeSnapshotPersistenceMaxRows  = 5000
	defaultRuntimeSnapshotPersistenceBatch    = 200
)

type RuntimeSnapshotPersistenceOptions struct {
	Interval time.Duration
	MaxRows  int
	Batch    int
}

type RuntimeSnapshotPersistence struct {
	store    core.RuntimeSnapshotStore
	interval time.Duration
	maxRows  int
	batch    int

	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
	started   bool
	startedMu sync.Mutex
}

func NewRuntimeSnapshotPersistence(store core.RuntimeSnapshotStore, options RuntimeSnapshotPersistenceOptions) *RuntimeSnapshotPersistence {
	interval := options.Interval
	if interval <= 0 {
		interval = defaultRuntimeSnapshotPersistenceInterval
	}
	maxRows := options.MaxRows
	if maxRows <= 0 {
		maxRows = defaultRuntimeSnapshotPersistenceMaxRows
	}
	batch := options.Batch
	if batch <= 0 {
		batch = defaultRuntimeSnapshotPersistenceBatch
	}
	return &RuntimeSnapshotPersistence{
		store:    store,
		interval: interval,
		maxRows:  maxRows,
		batch:    batch,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (p *RuntimeSnapshotPersistence) Start() {
	if p == nil {
		return
	}
	p.startOnce.Do(func() {
		if !p.tableExists(context.Background()) {
			return
		}
		p.startedMu.Lock()
		p.started = true
		p.startedMu.Unlock()
		go p.run()
	})
}

func (p *RuntimeSnapshotPersistence) Close() {
	if p == nil {
		return
	}
	if !p.isStarted() {
		return
	}
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
}

func (p *RuntimeSnapshotPersistence) Available(ctx context.Context) bool {
	return p.tableExists(ctx)
}

func (p *RuntimeSnapshotPersistence) HasPersistedSnapshots(ctx context.Context) (bool, error) {
	if p == nil || model.DB == nil || !p.tableExists(ctx) {
		return false, nil
	}
	var count int64
	err := model.DB.WithContext(ctx).Model(&model.ModelGatewayRuntimeSnapshot{}).Count(&count).Error
	return count > 0, err
}

func (p *RuntimeSnapshotPersistence) HasPersistedLatencySamples(ctx context.Context) (bool, error) {
	if p == nil || model.DB == nil || !p.tableExists(ctx) {
		return false, nil
	}
	if !model.DB.WithContext(ctx).Migrator().HasColumn(&model.ModelGatewayRuntimeSnapshot{}, "latency_samples") {
		return false, nil
	}
	var count int64
	err := model.DB.WithContext(ctx).
		Model(&model.ModelGatewayRuntimeSnapshot{}).
		Where("latency_samples <> ?", "").
		Count(&count).Error
	return count > 0, err
}

func (p *RuntimeSnapshotPersistence) Restore(ctx context.Context) error {
	if p == nil || p.store == nil || model.DB == nil {
		return nil
	}
	if !p.tableExists(ctx) {
		return nil
	}
	var rows []model.ModelGatewayRuntimeSnapshot
	query := model.DB.WithContext(ctx).
		Order("updated_at DESC").
		Limit(p.maxRows).
		Find(&rows)
	if query.Error != nil {
		return query.Error
	}
	for _, row := range coalesceRuntimeSnapshotRows(rows) {
		snapshot, ok := runtimeSnapshotFromDB(row)
		if !ok {
			continue
		}
		p.store.Put(snapshot)
	}
	if len(rows) > 0 {
		return p.Flush(ctx)
	}
	return nil
}

func (p *RuntimeSnapshotPersistence) Flush(ctx context.Context) error {
	if p == nil || p.store == nil || model.DB == nil {
		return nil
	}
	if !p.tableExists(ctx) {
		return nil
	}
	snapshots := p.store.ListCandidates(nil)
	if len(snapshots) == 0 {
		return nil
	}
	if p.maxRows > 0 && len(snapshots) > p.maxRows {
		snapshots = snapshots[:p.maxRows]
	}
	now := common.GetTimestamp()
	rows := make([]model.ModelGatewayRuntimeSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		snapshot = normalizeRuntimeSnapshot(snapshot)
		row, ok := runtimeSnapshotToDB(snapshot, now)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := model.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "runtime_key_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"runtime_key",
			"updated_at",
			"requested_model",
			"upstream_model",
			"channel_id",
			"group",
			"endpoint_type",
			"capability_fingerprint",
			"latency_samples",
			"sample_count",
			"success_rate",
			"ttft_ms",
			"duration_ms",
			"tokens_per_second",
			"success_score",
			"speed_score",
			"experience_score",
			"empty_output_rate",
			"experience_issue_rate",
		}),
	}).CreateInBatches(rows, p.batch).Error; err != nil {
		return err
	}
	return p.pruneStaleRows(ctx, rows)
}

func (p *RuntimeSnapshotPersistence) isStarted() bool {
	if p == nil {
		return false
	}
	p.startedMu.Lock()
	defer p.startedMu.Unlock()
	return p.started
}

func (p *RuntimeSnapshotPersistence) tableExists(ctx context.Context) bool {
	if p == nil || model.DB == nil {
		return false
	}
	return model.DB.WithContext(ctx).Migrator().HasTable(&model.ModelGatewayRuntimeSnapshot{})
}

func (p *RuntimeSnapshotPersistence) pruneStaleRows(ctx context.Context, rows []model.ModelGatewayRuntimeSnapshot) error {
	if p == nil || model.DB == nil || len(rows) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.RuntimeKeyHash != "" {
			hashes = append(hashes, row.RuntimeKeyHash)
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	return model.DB.WithContext(ctx).
		Where("endpoint_type = ? OR runtime_key_hash NOT IN ?", "", hashes).
		Delete(&model.ModelGatewayRuntimeSnapshot{}).Error
}

func (p *RuntimeSnapshotPersistence) run() {
	defer close(p.done)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := p.Flush(context.Background()); err != nil {
				common.SysLog(fmt.Sprintf("model gateway runtime snapshot flush failed: %v", err))
			}
		case <-p.stop:
			if err := p.Flush(context.Background()); err != nil {
				common.SysLog(fmt.Sprintf("model gateway runtime snapshot final flush failed: %v", err))
			}
			return
		}
	}
}

func runtimeSnapshotToDB(snapshot core.RuntimeSnapshot, updatedAt int64) (model.ModelGatewayRuntimeSnapshot, bool) {
	snapshot = normalizeRuntimeSnapshot(snapshot)
	if snapshot.Key.ChannelID <= 0 || snapshot.SampleCount <= 0 {
		return model.ModelGatewayRuntimeSnapshot{}, false
	}
	keyJSON, err := common.Marshal(snapshot.Key)
	if err != nil {
		return model.ModelGatewayRuntimeSnapshot{}, false
	}
	latencySamples := ""
	snapshot.RecentLatencySamples = normalizeRuntimeLatencySamples(snapshot.RecentLatencySamples)
	if len(snapshot.RecentLatencySamples) > 0 {
		snapshot.DurationMs, snapshot.TTFTMs, snapshot.SpeedScore = runtimeLatencyStats(snapshot.RecentLatencySamples)
		if samplesJSON, err := common.Marshal(snapshot.RecentLatencySamples); err == nil {
			latencySamples = string(samplesJSON)
		}
	} else {
		snapshot.DurationMs = 0
		snapshot.TTFTMs = 0
		snapshot.SpeedScore = 0
	}
	return model.ModelGatewayRuntimeSnapshot{
		RuntimeKeyHash:        runtimeSnapshotKeyHash(snapshot.Key),
		RuntimeKey:            string(keyJSON),
		UpdatedAt:             updatedAt,
		RequestedModel:        snapshot.Key.RequestedModel,
		UpstreamModel:         snapshot.Key.UpstreamModel,
		ChannelID:             snapshot.Key.ChannelID,
		Group:                 snapshot.Key.Group,
		EndpointType:          string(snapshot.Key.EndpointType),
		CapabilityFingerprint: snapshot.Key.CapabilityFingerprint,
		LatencySamples:        latencySamples,
		SampleCount:           snapshot.SampleCount,
		SuccessRate:           snapshot.SuccessRate,
		TTFTMs:                snapshot.TTFTMs,
		DurationMs:            snapshot.DurationMs,
		TokensPerSecond:       snapshot.TokensPerSecond,
		SuccessScore:          snapshot.SuccessScore,
		SpeedScore:            snapshot.SpeedScore,
		ExperienceScore:       snapshot.ExperienceScore,
		EmptyOutputRate:       snapshot.EmptyOutputRate,
		ExperienceIssueRate:   snapshot.ExperienceIssueRate,
	}, true
}

func runtimeSnapshotFromDB(row model.ModelGatewayRuntimeSnapshot) (core.RuntimeSnapshot, bool) {
	var key core.RuntimeKey
	if row.RuntimeKey != "" {
		_ = common.UnmarshalJsonStr(row.RuntimeKey, &key)
	}
	if key.ChannelID <= 0 {
		key = core.RuntimeKey{
			RequestedModel:        row.RequestedModel,
			UpstreamModel:         row.UpstreamModel,
			ChannelID:             row.ChannelID,
			Group:                 row.Group,
			EndpointType:          constant.EndpointType(row.EndpointType),
			CapabilityFingerprint: row.CapabilityFingerprint,
		}
	}
	key = normalizeRuntimeKey(key)
	if key.ChannelID <= 0 || row.SampleCount <= 0 {
		return core.RuntimeSnapshot{}, false
	}
	latencySamples := make([]core.RuntimeLatencySample, 0)
	if row.LatencySamples != "" {
		_ = common.UnmarshalJsonStr(row.LatencySamples, &latencySamples)
	}
	latencySamples = normalizeRuntimeLatencySamples(latencySamples)
	ttftMs := 0.0
	durationMs := 0.0
	speedScore := 0.0
	if len(latencySamples) > 0 {
		durationMs, ttftMs, speedScore = runtimeLatencyStats(latencySamples)
	}
	return core.RuntimeSnapshot{
		Key:                  key,
		RecentLatencySamples: latencySamples,
		SuccessRate:          row.SuccessRate,
		TTFTMs:               ttftMs,
		DurationMs:           durationMs,
		TokensPerSecond:      row.TokensPerSecond,
		SuccessScore:         row.SuccessScore,
		SpeedScore:           speedScore,
		ExperienceScore:      row.ExperienceScore,
		EmptyOutputRate:      row.EmptyOutputRate,
		ExperienceIssueRate:  row.ExperienceIssueRate,
		GroupPriorityRatio:   1,
		CircuitState:         core.CircuitStateClosed,
		SampleCount:          row.SampleCount,
	}, true
}

func runtimeSnapshotKeyHash(key core.RuntimeKey) string {
	key = normalizeRuntimeKey(key)
	return runtimeSnapshotKeyHashRaw(key)
}

func runtimeSnapshotKeyHashRaw(key core.RuntimeKey) string {
	data, err := common.Marshal(key)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func coalesceRuntimeSnapshotRows(rows []model.ModelGatewayRuntimeSnapshot) []model.ModelGatewayRuntimeSnapshot {
	merged := make(map[string]model.ModelGatewayRuntimeSnapshot, len(rows))
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		snapshot, ok := runtimeSnapshotFromDB(row)
		if !ok {
			continue
		}
		row, ok = runtimeSnapshotToDB(snapshot, row.UpdatedAt)
		if !ok {
			continue
		}
		key := runtimeSnapshotKeyHashRaw(snapshot.Key)
		if key == "" {
			key = runtimeSnapshotKeyHash(snapshot.Key)
		}
		current, exists := merged[key]
		if !exists {
			merged[key] = row
			order = append(order, key)
			continue
		}
		merged[key] = mergeRuntimeSnapshotRows(current, row)
	}
	out := make([]model.ModelGatewayRuntimeSnapshot, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func mergeRuntimeSnapshotRows(left, right model.ModelGatewayRuntimeSnapshot) model.ModelGatewayRuntimeSnapshot {
	if right.UpdatedAt > left.UpdatedAt {
		left.RuntimeKey = right.RuntimeKey
		left.RequestedModel = right.RequestedModel
		left.UpstreamModel = right.UpstreamModel
		left.ChannelID = right.ChannelID
		left.Group = right.Group
		left.EndpointType = right.EndpointType
		left.CapabilityFingerprint = right.CapabilityFingerprint
		left.UpdatedAt = right.UpdatedAt
	}
	total := left.SampleCount + right.SampleCount
	if total <= 0 {
		return left
	}
	left.SuccessRate = weightedRuntimeSnapshotAverage(left.SuccessRate, left.SampleCount, right.SuccessRate, right.SampleCount)
	leftSamples := make([]core.RuntimeLatencySample, 0)
	rightSamples := make([]core.RuntimeLatencySample, 0)
	if left.LatencySamples != "" {
		_ = common.UnmarshalJsonStr(left.LatencySamples, &leftSamples)
	}
	if right.LatencySamples != "" {
		_ = common.UnmarshalJsonStr(right.LatencySamples, &rightSamples)
	}
	latencySamples := mergeRuntimeLatencySamples(leftSamples, rightSamples)
	if len(latencySamples) > 0 {
		if samplesJSON, err := common.Marshal(latencySamples); err == nil {
			left.LatencySamples = string(samplesJSON)
		}
		left.DurationMs, left.TTFTMs, left.SpeedScore = runtimeLatencyStats(latencySamples)
	} else {
		left.TTFTMs = 0
		left.DurationMs = 0
		left.SpeedScore = 0
	}
	left.TokensPerSecond = weightedRuntimeSnapshotAverage(left.TokensPerSecond, left.SampleCount, right.TokensPerSecond, right.SampleCount)
	left.SuccessScore = weightedRuntimeSnapshotAverage(left.SuccessScore, left.SampleCount, right.SuccessScore, right.SampleCount)
	left.ExperienceScore = weightedRuntimeSnapshotAverage(left.ExperienceScore, left.SampleCount, right.ExperienceScore, right.SampleCount)
	left.EmptyOutputRate = weightedRuntimeSnapshotAverage(left.EmptyOutputRate, left.SampleCount, right.EmptyOutputRate, right.SampleCount)
	left.ExperienceIssueRate = weightedRuntimeSnapshotAverage(left.ExperienceIssueRate, left.SampleCount, right.ExperienceIssueRate, right.SampleCount)
	left.SampleCount = total
	return left
}

func weightedRuntimeSnapshotAverage(left float64, leftCount int, right float64, rightCount int) float64 {
	total := leftCount + rightCount
	if total <= 0 {
		return 0
	}
	return (left*float64(leftCount) + right*float64(rightCount)) / float64(total)
}
