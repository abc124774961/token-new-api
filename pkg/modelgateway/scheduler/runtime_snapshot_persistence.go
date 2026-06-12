package scheduler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
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
	defaultRuntimeSnapshotPersistenceBatch    = 50
)

type RuntimeSnapshotPersistenceOptions struct {
	Interval time.Duration
	MaxRows  int
	Batch    int
}

type RuntimeSnapshotPersistence struct {
	store        core.RuntimeSnapshotStore
	interval     time.Duration
	maxRows      int
	batch        int
	signaturesMu sync.Mutex
	signatures   map[string]string

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
		store:      store,
		interval:   interval,
		maxRows:    maxRows,
		batch:      batch,
		signatures: make(map[string]string),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
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
	coalescedRows := coalesceRuntimeSnapshotRows(rows)
	needsRewrite := runtimeSnapshotRowsNeedRewrite(rows, coalescedRows)
	for _, row := range coalescedRows {
		snapshot, ok := runtimeSnapshotFromDB(row)
		if !ok {
			continue
		}
		p.store.Put(snapshot)
	}
	if len(rows) > 0 && needsRewrite {
		return p.Flush(ctx)
	}
	p.markPersistedRows(coalescedRows)
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
	changedRows := p.changedRows(rows)
	if len(changedRows) > 0 {
		if err := model.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "runtime_key_hash"}},
			DoUpdates: clause.AssignmentColumns(runtimeSnapshotPersistenceUpdateColumns()),
		}).CreateInBatches(changedRows, p.batch).Error; err != nil {
			return err
		}
		p.markPersistedRows(changedRows)
	}
	return p.pruneStaleRows(ctx, rows)
}

func runtimeSnapshotPersistenceUpdateColumns() []string {
	return []string{
		"runtime_key",
		"updated_at",
		"requested_model",
		"upstream_model",
		"channel_id",
		"resource_id",
		"resource_type",
		"account_id",
		"account_type",
		"brand",
		"provider",
		"credential_index",
		"credential_subject_fingerprint",
		"credential_fingerprint",
		"group",
		"endpoint_type",
		"capability_fingerprint",
		"score_stats_json",
		"latency_samples",
		"sample_count",
		"success_rate",
		"ttft_ms",
		"duration_ms",
		"tokens_per_second",
		"empty_output_rate",
		"experience_issue_rate",
		"recoverable_quality_score",
		"recoverable_quality_baseline",
		"recoverable_quality_baseline_samples",
		"recoverable_quality_drop_ratio",
		"recoverable_quality_item_baselines",
		"probe_recovery_pending",
		"probe_recovery_success_count",
		"probe_recovery_required",
		"probe_trigger_reason",
		"probe_recovery_phase",
		"probe_fast_recovery_attempts",
		"probe_anomaly_trigger_items",
		"last_real_attempt_at",
		"last_real_success_at",
		"last_real_failure_at",
		"real_sample_count_30m",
		"last_probe_at",
		"last_probe_success_at",
		"config_error_isolated",
		"isolation_reason",
		"isolation_until",
		"auth_config_error_count",
		"last_auth_config_error_at",
	}
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

func (p *RuntimeSnapshotPersistence) changedRows(rows []model.ModelGatewayRuntimeSnapshot) []model.ModelGatewayRuntimeSnapshot {
	if p == nil || len(rows) == 0 {
		return nil
	}
	changed := make([]model.ModelGatewayRuntimeSnapshot, 0, len(rows))
	p.signaturesMu.Lock()
	defer p.signaturesMu.Unlock()
	for _, row := range rows {
		if row.RuntimeKeyHash == "" {
			continue
		}
		signature := runtimeSnapshotRowSignature(row)
		if p.signatures[row.RuntimeKeyHash] == signature {
			continue
		}
		changed = append(changed, row)
	}
	return changed
}

func (p *RuntimeSnapshotPersistence) markPersistedRows(rows []model.ModelGatewayRuntimeSnapshot) {
	if p == nil || len(rows) == 0 {
		return
	}
	p.signaturesMu.Lock()
	defer p.signaturesMu.Unlock()
	for _, row := range rows {
		if row.RuntimeKeyHash == "" {
			continue
		}
		p.signatures[row.RuntimeKeyHash] = runtimeSnapshotRowSignature(row)
	}
}

func runtimeSnapshotRowSignature(row model.ModelGatewayRuntimeSnapshot) string {
	parts := []string{
		row.RuntimeKeyHash,
		row.RuntimeKey,
		row.RequestedModel,
		row.UpstreamModel,
		strconv.Itoa(row.ChannelID),
		row.ResourceID,
		row.ResourceType,
		row.AccountID,
		row.AccountType,
		row.Brand,
		row.Provider,
		strconv.Itoa(row.CredentialIndex),
		row.CredentialSubjectFP,
		row.CredentialFP,
		row.Group,
		row.EndpointType,
		row.CapabilityFingerprint,
		row.ScoreStatsJSON,
		row.LatencySamples,
		strconv.Itoa(row.SampleCount),
		strconv.FormatFloat(row.SuccessRate, 'g', -1, 64),
		strconv.FormatFloat(row.TTFTMs, 'g', -1, 64),
		strconv.FormatFloat(row.DurationMs, 'g', -1, 64),
		strconv.FormatFloat(row.TokensPerSecond, 'g', -1, 64),
		strconv.FormatFloat(row.EmptyOutputRate, 'g', -1, 64),
		strconv.FormatFloat(row.ExperienceIssueRate, 'g', -1, 64),
		strconv.FormatFloat(row.RecoverableQualityScore, 'g', -1, 64),
		strconv.FormatFloat(row.RecoverableQualityBaseline, 'g', -1, 64),
		strconv.Itoa(row.RecoverableQualityBaselineSamples),
		strconv.FormatFloat(row.RecoverableQualityDropRatio, 'g', -1, 64),
		row.RecoverableQualityItemBaselines,
		strconv.FormatBool(row.ProbeRecoveryPending),
		strconv.Itoa(row.ProbeRecoverySuccessCount),
		strconv.Itoa(row.ProbeRecoveryRequired),
		row.ProbeTriggerReason,
		row.ProbeRecoveryPhase,
		strconv.Itoa(row.ProbeFastRecoveryAttempts),
		row.ProbeAnomalyTriggerItems,
		strconv.FormatInt(row.LastRealAttemptAt, 10),
		strconv.FormatInt(row.LastRealSuccessAt, 10),
		strconv.FormatInt(row.LastRealFailureAt, 10),
		strconv.Itoa(row.RealSampleCount30m),
		strconv.FormatInt(row.LastProbeAt, 10),
		strconv.FormatInt(row.LastProbeSuccessAt, 10),
		strconv.FormatBool(row.ConfigErrorIsolated),
		row.IsolationReason,
		strconv.FormatInt(row.IsolationUntil, 10),
		strconv.Itoa(row.AuthConfigErrorCount),
		strconv.FormatInt(row.LastAuthConfigErrorAt, 10),
	}
	data, err := common.Marshal(parts)
	if err != nil {
		return fmt.Sprintf("%s:%d:%d", row.RuntimeKeyHash, row.SampleCount, row.UpdatedAt)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
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
		snapshot.DurationMs, snapshot.TTFTMs, _ = runtimeLatencyStats(snapshot.RecentLatencySamples)
		if samplesJSON, err := common.Marshal(snapshot.RecentLatencySamples); err == nil {
			latencySamples = string(samplesJSON)
		}
	} else {
		snapshot.DurationMs = 0
		snapshot.TTFTMs = 0
	}
	recoverableQualityItemBaselines := ""
	if len(snapshot.RecoverableQualityItemBaselines) > 0 {
		if data, err := common.Marshal(snapshot.RecoverableQualityItemBaselines); err == nil {
			recoverableQualityItemBaselines = string(data)
		}
	}
	probeAnomalyTriggerItems := ""
	if len(snapshot.ProbeAnomalyTriggerItems) > 0 {
		if data, err := common.Marshal(snapshot.ProbeAnomalyTriggerItems); err == nil {
			probeAnomalyTriggerItems = string(data)
		}
	}
	return model.ModelGatewayRuntimeSnapshot{
		RuntimeKeyHash:                    runtimeSnapshotKeyHash(snapshot.Key),
		RuntimeKey:                        string(keyJSON),
		UpdatedAt:                         updatedAt,
		RequestedModel:                    snapshot.Key.RequestedModel,
		UpstreamModel:                     snapshot.Key.UpstreamModel,
		ChannelID:                         snapshot.Key.ChannelID,
		ResourceID:                        snapshot.Key.ResourceID,
		ResourceType:                      snapshot.Key.ResourceType,
		AccountID:                         snapshot.Key.AccountID,
		AccountType:                       snapshot.Key.AccountType,
		Brand:                             snapshot.Key.Brand,
		Provider:                          snapshot.Key.Provider,
		CredentialIndex:                   snapshot.Key.CredentialIndex,
		CredentialSubjectFP:               snapshot.Key.CredentialSubjectFP,
		CredentialFP:                      snapshot.Key.CredentialFP,
		Group:                             snapshot.Key.Group,
		EndpointType:                      string(snapshot.Key.EndpointType),
		CapabilityFingerprint:             snapshot.Key.CapabilityFingerprint,
		ScoreStatsJSON:                    snapshot.ScoreStatsJSON,
		LatencySamples:                    latencySamples,
		SampleCount:                       snapshot.SampleCount,
		SuccessRate:                       snapshot.SuccessRate,
		TTFTMs:                            snapshot.TTFTMs,
		DurationMs:                        snapshot.DurationMs,
		TokensPerSecond:                   snapshot.TokensPerSecond,
		EmptyOutputRate:                   snapshot.EmptyOutputRate,
		ExperienceIssueRate:               snapshot.ExperienceIssueRate,
		RecoverableQualityScore:           snapshot.RecoverableQualityScore,
		RecoverableQualityBaseline:        snapshot.RecoverableQualityBaseline,
		RecoverableQualityBaselineSamples: snapshot.RecoverableQualityBaselineSamples,
		RecoverableQualityDropRatio:       snapshot.RecoverableQualityDropRatio,
		RecoverableQualityItemBaselines:   recoverableQualityItemBaselines,
		ProbeRecoveryPending:              snapshot.ProbeRecoveryPending,
		ProbeRecoverySuccessCount:         snapshot.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:             snapshot.ProbeRecoveryRequired,
		ProbeTriggerReason:                snapshot.ProbeTriggerReason,
		ProbeRecoveryPhase:                snapshot.ProbeRecoveryPhase,
		ProbeFastRecoveryAttempts:         snapshot.ProbeFastRecoveryAttempts,
		ProbeAnomalyTriggerItems:          probeAnomalyTriggerItems,
		LastRealAttemptAt:                 snapshot.LastRealAttemptAt,
		LastRealSuccessAt:                 snapshot.LastRealSuccessAt,
		LastRealFailureAt:                 snapshot.LastRealFailureAt,
		RealSampleCount30m:                snapshot.RealSampleCount30m,
		LastProbeAt:                       snapshot.LastProbeAt,
		LastProbeSuccessAt:                snapshot.LastProbeSuccessAt,
		ConfigErrorIsolated:               false,
		IsolationReason:                   "",
		IsolationUntil:                    0,
		AuthConfigErrorCount:              snapshot.AuthConfigErrorCount,
		LastAuthConfigErrorAt:             snapshot.LastAuthConfigErrorAt,
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
			ResourceID:            row.ResourceID,
			ResourceType:          row.ResourceType,
			AccountID:             row.AccountID,
			AccountType:           row.AccountType,
			Brand:                 row.Brand,
			Provider:              row.Provider,
			CredentialIndex:       row.CredentialIndex,
			CredentialSubjectFP:   row.CredentialSubjectFP,
			CredentialFP:          row.CredentialFP,
			Group:                 row.Group,
			EndpointType:          constant.EndpointType(row.EndpointType),
			CapabilityFingerprint: row.CapabilityFingerprint,
		}
	}
	if key.ResourceID == "" {
		key.ResourceID = row.ResourceID
	}
	if key.ResourceType == "" {
		key.ResourceType = row.ResourceType
	}
	if key.AccountID == "" {
		key.AccountID = row.AccountID
	}
	if key.AccountType == "" {
		key.AccountType = row.AccountType
	}
	if key.Brand == "" {
		key.Brand = row.Brand
	}
	if key.Provider == "" {
		key.Provider = row.Provider
	}
	if key.CredentialIndex == 0 {
		key.CredentialIndex = row.CredentialIndex
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = row.CredentialSubjectFP
	}
	if key.CredentialFP == "" {
		key.CredentialFP = row.CredentialFP
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
	recoverableQualityItemBaselines := map[string]float64{}
	if row.RecoverableQualityItemBaselines != "" {
		_ = common.UnmarshalJsonStr(row.RecoverableQualityItemBaselines, &recoverableQualityItemBaselines)
	}
	probeAnomalyTriggerItems := make([]string, 0)
	if row.ProbeAnomalyTriggerItems != "" {
		_ = common.UnmarshalJsonStr(row.ProbeAnomalyTriggerItems, &probeAnomalyTriggerItems)
	}
	ttftMs := 0.0
	durationMs := 0.0
	if len(latencySamples) > 0 {
		durationMs, ttftMs, _ = runtimeLatencyStats(latencySamples)
	}
	return core.RuntimeSnapshot{
		Key:                               key,
		ScoreStatsJSON:                    row.ScoreStatsJSON,
		RecentLatencySamples:              latencySamples,
		SuccessRate:                       row.SuccessRate,
		TTFTMs:                            ttftMs,
		DurationMs:                        durationMs,
		TokensPerSecond:                   row.TokensPerSecond,
		EmptyOutputRate:                   row.EmptyOutputRate,
		ExperienceIssueRate:               row.ExperienceIssueRate,
		RecoverableQualityScore:           row.RecoverableQualityScore,
		RecoverableQualityBaseline:        row.RecoverableQualityBaseline,
		RecoverableQualityBaselineSamples: row.RecoverableQualityBaselineSamples,
		RecoverableQualityDropRatio:       row.RecoverableQualityDropRatio,
		RecoverableQualityItemBaselines:   recoverableQualityItemBaselines,
		ProbeRecoveryPending:              row.ProbeRecoveryPending,
		ProbeRecoverySuccessCount:         row.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:             row.ProbeRecoveryRequired,
		ProbeTriggerReason:                row.ProbeTriggerReason,
		ProbeRecoveryPhase:                row.ProbeRecoveryPhase,
		ProbeFastRecoveryAttempts:         row.ProbeFastRecoveryAttempts,
		ProbeAnomalyTriggerItems:          probeAnomalyTriggerItems,
		LastRealAttemptAt:                 row.LastRealAttemptAt,
		LastRealSuccessAt:                 row.LastRealSuccessAt,
		LastRealFailureAt:                 row.LastRealFailureAt,
		RealSampleCount30m:                row.RealSampleCount30m,
		LastProbeAt:                       row.LastProbeAt,
		LastProbeSuccessAt:                row.LastProbeSuccessAt,
		ConfigErrorIsolated:               false,
		IsolationReason:                   "",
		IsolationUntil:                    0,
		AuthConfigErrorCount:              row.AuthConfigErrorCount,
		LastAuthConfigErrorAt:             row.LastAuthConfigErrorAt,
		GroupPriorityRatio:                1,
		CircuitState:                      core.CircuitStateClosed,
		SampleCount:                       row.SampleCount,
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

func runtimeSnapshotRowsNeedRewrite(rows []model.ModelGatewayRuntimeSnapshot, coalesced []model.ModelGatewayRuntimeSnapshot) bool {
	if len(rows) != len(coalesced) {
		return true
	}
	coalescedByHash := make(map[string]model.ModelGatewayRuntimeSnapshot, len(coalesced))
	for _, row := range coalesced {
		if row.RuntimeKeyHash == "" {
			return true
		}
		coalescedByHash[row.RuntimeKeyHash] = row
	}
	for _, row := range rows {
		if row.RuntimeKeyHash == "" {
			return true
		}
		coalescedRow, ok := coalescedByHash[row.RuntimeKeyHash]
		if !ok {
			return true
		}
		if runtimeSnapshotRowSignature(row) != runtimeSnapshotRowSignature(coalescedRow) {
			return true
		}
	}
	return false
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
		left.DurationMs, left.TTFTMs, _ = runtimeLatencyStats(latencySamples)
	} else {
		left.TTFTMs = 0
		left.DurationMs = 0
	}
	left.TokensPerSecond = weightedRuntimeSnapshotAverage(left.TokensPerSecond, left.SampleCount, right.TokensPerSecond, right.SampleCount)
	left.EmptyOutputRate = weightedRuntimeSnapshotAverage(left.EmptyOutputRate, left.SampleCount, right.EmptyOutputRate, right.SampleCount)
	left.ExperienceIssueRate = weightedRuntimeSnapshotAverage(left.ExperienceIssueRate, left.SampleCount, right.ExperienceIssueRate, right.SampleCount)
	left.ScoreStatsJSON = mergeRuntimeSnapshotScoreStats(left.ScoreStatsJSON, right.ScoreStatsJSON)
	if right.RecoverableQualityBaselineSamples > left.RecoverableQualityBaselineSamples || right.UpdatedAt >= left.UpdatedAt {
		left.RecoverableQualityScore = right.RecoverableQualityScore
		left.RecoverableQualityBaseline = right.RecoverableQualityBaseline
		left.RecoverableQualityBaselineSamples = right.RecoverableQualityBaselineSamples
		left.RecoverableQualityDropRatio = right.RecoverableQualityDropRatio
		left.RecoverableQualityItemBaselines = right.RecoverableQualityItemBaselines
	}
	left.LastRealAttemptAt = maxInt64(left.LastRealAttemptAt, right.LastRealAttemptAt)
	left.LastRealSuccessAt = maxInt64(left.LastRealSuccessAt, right.LastRealSuccessAt)
	left.LastRealFailureAt = maxInt64(left.LastRealFailureAt, right.LastRealFailureAt)
	left.RealSampleCount30m += right.RealSampleCount30m
	left.LastProbeAt = maxInt64(left.LastProbeAt, right.LastProbeAt)
	left.LastProbeSuccessAt = maxInt64(left.LastProbeSuccessAt, right.LastProbeSuccessAt)
	if right.ProbeRecoveryPending || right.UpdatedAt >= left.UpdatedAt {
		left.ProbeRecoveryPending = right.ProbeRecoveryPending
		left.ProbeRecoverySuccessCount = right.ProbeRecoverySuccessCount
		left.ProbeRecoveryRequired = right.ProbeRecoveryRequired
		left.ProbeTriggerReason = right.ProbeTriggerReason
		left.ProbeRecoveryPhase = right.ProbeRecoveryPhase
		left.ProbeFastRecoveryAttempts = right.ProbeFastRecoveryAttempts
		left.ProbeAnomalyTriggerItems = right.ProbeAnomalyTriggerItems
	}
	if right.LastAuthConfigErrorAt > left.LastAuthConfigErrorAt {
		left.ConfigErrorIsolated = false
		left.IsolationReason = ""
		left.IsolationUntil = 0
		left.AuthConfigErrorCount = right.AuthConfigErrorCount
		left.LastAuthConfigErrorAt = right.LastAuthConfigErrorAt
	}
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

func maxInt64(left int64, right int64) int64 {
	if right > left {
		return right
	}
	return left
}
