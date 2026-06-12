package scheduler

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

const (
	defaultHealthSuccessRate = 0.80
	healthEWMAAlpha          = 0.20
	realSampleWindow         = 30 * time.Minute
	probeReasonLongNoSuccess = "long_no_success"
	probeReasonLowScore      = "low_score"
	probeReasonLowTraffic    = "low_traffic"
)

type RuntimeHealthMonitor struct {
	store          core.RuntimeSnapshotStore
	breaker        core.CircuitBreaker
	scoringService *CandidateScoringService
	scoreEvents    *ScoreEventRecorder
	scoreWeights   core.ScoreWeights
	mu             sync.Mutex
	stats          map[core.RuntimeKey]*healthStats
}

type healthStats struct {
	successCount   int
	sampleCount    int
	durationMs     float64
	ttftMs         float64
	emptyRate      float64
	issueRate      float64
	latencySamples []core.RuntimeLatencySample
	realSampleAt   []int64
}

func NewRuntimeHealthMonitor(store core.RuntimeSnapshotStore, breaker core.CircuitBreaker) *RuntimeHealthMonitor {
	return &RuntimeHealthMonitor{
		store:          store,
		breaker:        breaker,
		scoringService: NewCandidateScoringService(),
		scoreEvents:    NewScoreEventRecorder(1024),
		stats:          map[core.RuntimeKey]*healthStats{},
	}
}

func (m *RuntimeHealthMonitor) WithScoringService(service *CandidateScoringService) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	if service != nil {
		m.scoringService = service
	}
	return m
}

func (m *RuntimeHealthMonitor) WithScoreWeights(weights core.ScoreWeights) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	m.scoreWeights = weights
	return m
}

func (m *RuntimeHealthMonitor) Record(ctx context.Context, record core.DispatchRecord) {}

func (m *RuntimeHealthMonitor) Report(ctx context.Context, result core.AttemptResult) {
	if m == nil {
		return
	}
	if result.ClientAborted {
		return
	}
	if strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryUserQuotaExhausted {
		return
	}
	if result.BalanceInsufficient || isBalanceInsufficientAttempt(result) {
		if result.ChannelID > 0 {
			service.MarkChannelRuntimeBalanceInsufficient(serviceRuntimeIdentityFromKey(result.RuntimeKey()))
		}
		return
	}
	decision := scoreSampleDecision(result)
	if m.breaker != nil && decision.CircuitSample {
		m.breaker.Report(result)
	}
	if result.ConcurrencyLimited {
		return
	}
	if !decision.ScoreSample && !decision.CircuitSample && !decision.ProbeRecoverySample {
		return
	}
	if m.store == nil {
		return
	}
	key := normalizeRuntimeKey(result.RuntimeKey())
	if key.ChannelID <= 0 {
		return
	}
	m.mu.Lock()
	stats := m.stats[key]
	snapshot, ok := m.store.Get(key)
	if stats == nil {
		stats = healthStatsFromSnapshot(snapshot, ok)
		m.stats[key] = stats
	}
	observedAt := result.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	observedAtUnix := observedAt.Unix()
	if !ok {
		snapshot = core.RuntimeSnapshot{
			Key:                key,
			CostRatio:          0,
			GroupPriorityRatio: 1,
		}
	}
	beforeScore := core.ScoreResult{}
	if decision.ScoreSample || decision.ProbeRecoverySample {
		beforeScore = m.scoreSnapshot(result, snapshot)
	}
	scoreStats := scoreStatsFromSnapshot(snapshot)
	if decision.ScoreSample {
		stats.sampleCount++
		if result.Success {
			stats.successCount++
		}
		resultSuccess := result.Success && !result.EmptyOutput && strings.TrimSpace(result.ExperienceIssue) == ""
		_ = resultSuccess
		if result.Success {
			if sample, valid := runtimeLatencySampleFromAttempt(result, observedAt); valid {
				stats.latencySamples = appendRuntimeLatencySample(stats.latencySamples, sample)
				stats.durationMs, stats.ttftMs, _ = runtimeLatencyStats(stats.latencySamples)
			}
		} else if stats.durationMs <= 0 {
			stats.durationMs = durationMs(result.Duration, 0)
		}
		stats.emptyRate = rateEWMA(stats.emptyRate, result.EmptyOutput)
		stats.issueRate = rateEWMA(stats.issueRate, nonEmptyOutputExperienceIssue(result))
		scoreStats = updateScoreStats(scoreStats, result, decision)
	}
	snapshot.Key = key
	snapshot.ScoreStatsJSON = scoreStats.Marshal()
	snapshot.SampleCount = stats.sampleCount
	snapshot.RecentLatencySamples = append([]core.RuntimeLatencySample(nil), stats.latencySamples...)
	snapshot.SuccessRate = successRate(stats)
	snapshot.DurationMs = stats.durationMs
	if stats.ttftMs > 0 {
		snapshot.TTFTMs = stats.ttftMs
	}
	snapshot.EmptyOutputRate = stats.emptyRate
	snapshot.ExperienceIssueRate = stats.issueRate
	recoveryScore := core.ScoreResult{}
	if decision.ScoreSample || decision.ProbeRecoverySample {
		recoveryScore = m.scoreSnapshot(result, snapshot)
	}
	scoreAnomalyActiveBefore := scoreAnomalyRecoveryActive(snapshot)
	recoverableQualityAnomaly := m.applyRecoverableQuality(&snapshot, result, recoveryScore)
	scoreAnomalyJustTriggered := recoverableQualityAnomaly && !scoreAnomalyActiveBefore
	m.applyProbeRecovery(&snapshot, result, recoveryScore, scoreAnomalyJustTriggered)
	if result.IsHealthProbe {
		snapshot.LastProbeAt = observedAtUnix
		if result.Success {
			snapshot.LastProbeSuccessAt = observedAtUnix
		}
	} else {
		stats.realSampleAt = appendRecentRealSample(stats.realSampleAt, observedAtUnix, observedAt)
		snapshot.LastRealAttemptAt = observedAtUnix
		if result.Success {
			snapshot.LastRealSuccessAt = observedAtUnix
			if strings.TrimSpace(snapshot.ProbeTriggerReason) == probeReasonLongNoSuccess {
				snapshot.ProbeTriggerReason = ""
			}
		} else {
			snapshot.LastRealFailureAt = observedAtUnix
		}
		snapshot.RealSampleCount30m = len(stats.realSampleAt)
	}
	if m.breaker != nil {
		circuit := m.breaker.Snapshot(key)
		applyCircuitToRuntimeSnapshot(&snapshot, circuit)
	}
	afterScore := core.ScoreResult{}
	if decision.ScoreSample || decision.ProbeRecoverySample {
		afterScore = m.scoreSnapshot(result, snapshot)
	}
	if m.scoreEvents != nil && (decision.ScoreSample || decision.ProbeRecoverySample) {
		m.scoreEvents.ReportAdjustment(result, snapshot, decision, beforeScore, afterScore)
	}
	snapshot.ConfigErrorIsolated = false
	snapshot.IsolationReason = ""
	snapshot.IsolationUntil = 0
	m.store.Put(snapshot)
	m.mu.Unlock()
}

func (m *RuntimeHealthMonitor) scoreSnapshot(result core.AttemptResult, snapshot core.RuntimeSnapshot) core.ScoreResult {
	service := m.scoringService
	if service == nil {
		service = NewCandidateScoringService()
	}
	policy := core.GroupSmartPolicy{
		Strategy:        scoreStrategyFromAttempt(result),
		AutoMode:        strings.TrimSpace(result.AutoMode),
		CandidateGroups: []string{snapshot.Key.Group},
		GroupPriorityRatio: map[string]float64{
			snapshot.Key.Group: snapshot.GroupPriorityRatio,
		},
	}
	candidate := core.Candidate{
		Group:         snapshot.Key.Group,
		UpstreamModel: snapshot.Key.UpstreamModel,
		RuntimeKey:    snapshot.Key,
	}
	return service.EvaluatePreparedCandidate(candidate, snapshot, policy, ScoringContext{
		RequestedModel:  snapshot.Key.RequestedModel,
		EndpointType:    snapshot.Key.EndpointType,
		CandidateGroups: []string{snapshot.Key.Group},
		Strategy:        scoreStrategyFromAttempt(result),
		AutoMode:        strings.TrimSpace(result.AutoMode),
		ScoreWeights:    m.scoreWeights,
		ExplainEnabled:  true,
	}, false).Score
}

func scoreStrategyFromAttempt(result core.AttemptResult) string {
	if strategy := strings.TrimSpace(result.Strategy); strategy != "" {
		return strategy
	}
	return core.StrategyBalanced
}

func healthStatsFromSnapshot(snapshot core.RuntimeSnapshot, ok bool) *healthStats {
	stats := &healthStats{}
	if !ok || snapshot.SampleCount <= 0 {
		return stats
	}
	stats.sampleCount = snapshot.SampleCount
	stats.successCount = int(math.Round(snapshot.SuccessRate * float64(snapshot.SampleCount)))
	if stats.successCount < 0 {
		stats.successCount = 0
	}
	if stats.successCount > stats.sampleCount {
		stats.successCount = stats.sampleCount
	}
	stats.durationMs = snapshot.DurationMs
	stats.ttftMs = snapshot.TTFTMs
	stats.latencySamples = normalizeRuntimeLatencySamples(snapshot.RecentLatencySamples)
	if len(stats.latencySamples) > 0 {
		stats.durationMs, stats.ttftMs, _ = runtimeLatencyStats(stats.latencySamples)
	} else {
		stats.durationMs = 0
		stats.ttftMs = 0
	}
	stats.emptyRate = snapshot.EmptyOutputRate
	stats.issueRate = snapshot.ExperienceIssueRate
	stats.realSampleAt = realSamplesFromSnapshot(snapshot, time.Now())
	return stats
}

func (m *RuntimeHealthMonitor) applyRecoverableQuality(snapshot *core.RuntimeSnapshot, result core.AttemptResult, score core.ScoreResult) bool {
	if snapshot == nil {
		return false
	}
	setting := scheduler_setting.GetSetting()
	evaluation := EvaluateRecoverableQuality(score, setting.ProbeRecoverableScoreItems)
	if evaluation.Valid {
		snapshot.RecoverableQualityScore = evaluation.Score
	}
	triggers, dropRatio := recoverableQualityAnomalyTriggers(*snapshot, evaluation, recoverableQualityRecentRealSuccess(*snapshot, result))
	snapshot.RecoverableQualityDropRatio = dropRatio
	if len(triggers) > 0 {
		snapshot.ProbeAnomalyTriggerItems = triggers
		if snapshot.ProbeFastRecoveryAttempts < scoreAnomalyFastProbeMaxAttempts &&
			strings.TrimSpace(snapshot.ProbeRecoveryPhase) != core.ProbeRecoveryPhasePendingRealConfirmation {
			snapshot.ProbeRecoveryPhase = core.ProbeRecoveryPhaseFastProbe
			snapshot.ProbeTriggerReason = core.ProbeReasonScoreAnomalyFastProbe
		}
	} else if strings.TrimSpace(snapshot.ProbeTriggerReason) == core.ProbeReasonScoreAnomalyFastProbe &&
		strings.TrimSpace(snapshot.ProbeRecoveryPhase) != core.ProbeRecoveryPhasePendingRealConfirmation {
		snapshot.ProbeRecoveryPhase = ""
		snapshot.ProbeTriggerReason = ""
		snapshot.ProbeAnomalyTriggerItems = nil
		snapshot.ProbeFastRecoveryAttempts = 0
	} else if len(triggers) == 0 &&
		strings.TrimSpace(snapshot.ProbeRecoveryPhase) != core.ProbeRecoveryPhasePendingRealConfirmation &&
		snapshot.ProbeFastRecoveryAttempts > 0 {
		snapshot.ProbeFastRecoveryAttempts = 0
	}
	if shouldUpdateRecoverableQualityBaseline(*snapshot, result, evaluation, len(triggers) > 0) {
		updateRecoverableQualityBaseline(snapshot, evaluation)
	}
	return len(triggers) > 0
}

func (m *RuntimeHealthMonitor) applyProbeRecovery(snapshot *core.RuntimeSnapshot, result core.AttemptResult, score core.ScoreResult, scoreAnomalyJustTriggered bool) {
	if snapshot == nil || result.ChannelID <= 0 || result.ConcurrencyLimited || result.ClientAborted {
		return
	}
	setting := scheduler_setting.GetSetting()
	required := setting.ProbeRecoverySuccessesRequired
	if required <= 0 {
		required = 2
	}
	identity := serviceRuntimeIdentityFromKey(result.RuntimeKey())
	avoidance := service.GetChannelRuntimeFailureAvoidanceStatus(identity)
	probeRecoveryRequired := avoidance != nil && avoidance.ProbeRecoveryRequired
	timeoutRecovery := (avoidance != nil && service.IsTimeoutRecoveryReason(avoidance.Reason)) ||
		strings.TrimSpace(snapshot.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason ||
		strings.TrimSpace(result.ProbeReason) == service.ChannelTimeoutRecoveryReason
	if timeoutRecovery {
		required = setting.ChannelTimeoutRecoveryProbeSuccesses
		if required <= 0 {
			required = setting.ProbeRecoverySuccessesRequired
		}
		if required <= 0 {
			required = 2
		}
	}
	lowScoreThreshold := setting.ProbeLowScoreThreshold
	if lowScoreThreshold <= 0 {
		lowScoreThreshold = 0.62
	}
	snapshot.ProbeRecoveryRequired = required
	if result.IsHealthProbe {
		snapshot.ProbeTriggerReason = strings.TrimSpace(result.ProbeReason)
	}
	if probeRecoveryRequired && strings.TrimSpace(snapshot.ProbeTriggerReason) == "" {
		snapshot.ProbeTriggerReason = strings.TrimSpace(avoidance.Reason)
	}
	scoreAnomalyProbe := strings.TrimSpace(result.ProbeReason) == core.ProbeReasonScoreAnomalyFastProbe ||
		strings.TrimSpace(snapshot.ProbeTriggerReason) == core.ProbeReasonScoreAnomalyFastProbe ||
		strings.TrimSpace(snapshot.ProbeRecoveryPhase) == core.ProbeRecoveryPhaseFastProbe
	if scoreAnomalyProbe {
		snapshot.ProbeTriggerReason = core.ProbeReasonScoreAnomalyFastProbe
		if strings.TrimSpace(snapshot.ProbeRecoveryPhase) == "" {
			snapshot.ProbeRecoveryPhase = core.ProbeRecoveryPhaseFastProbe
		}
	}
	if timeoutRecovery {
		snapshot.ProbeTriggerReason = service.ChannelTimeoutRecoveryReason
		snapshot.FailureAvoidance = true
		snapshot.ProbeRecoveryPending = true
	}
	if probeRecoveryRequired {
		snapshot.FailureAvoidance = true
		snapshot.ProbeRecoveryPending = true
	}
	if result.IsHealthProbe && result.Success {
		snapshot.ProbeRecoverySuccessCount++
		if scoreAnomalyProbe && ScoreAnomalyRecovered(*snapshot, EvaluateRecoverableQuality(score, setting.ProbeRecoverableScoreItems)) {
			snapshot.ProbeRecoveryPhase = core.ProbeRecoveryPhasePendingRealConfirmation
			snapshot.ProbeRecoveryPending = true
			return
		}
		if snapshot.ProbeRecoverySuccessCount >= required && avoidance != nil && (!avoidance.ProbeRecoveryRequired || probeRecoveryRequired) {
			service.ClearChannelRuntimeProbeRecoveryAvoidance(identity)
			snapshot.FailureAvoidance = false
			snapshot.ProbeRecoveryPending = false
			snapshot.ProbeRecoverySuccessCount = required
			if probeRecoveryRequired || timeoutRecovery {
				snapshot.ProbeTriggerReason = ""
			}
			return
		}
	}
	if (result.IsHealthProbe && !result.Success) || (!result.IsHealthProbe && !result.Success) {
		snapshot.ProbeRecoverySuccessCount = 0
	}
	if !result.IsHealthProbe && !result.Success {
		snapshot.FailureAvoidance = snapshot.FailureAvoidance || avoidance != nil
	}
	snapshot.FailureAvoidance = snapshot.FailureAvoidance || avoidance != nil
	if !scoreAnomalyJustTriggered && shouldClearScoreAnomalyRecoveryAfterRealSuccess(*snapshot, result, score, lowScoreThreshold) {
		clearScoreAnomalyRecovery(snapshot)
	}
	scoreAnomalyPending := ScoreAnomalyFastProbePending(*snapshot)
	lowScorePending := score.Total > 0 && score.Total < lowScoreThreshold
	if scoreAnomalyProbe && !scoreAnomalyPending &&
		strings.TrimSpace(snapshot.ProbeRecoveryPhase) != core.ProbeRecoveryPhasePendingRealConfirmation {
		clearScoreAnomalyRecovery(snapshot)
	}
	snapshot.ProbeRecoveryPending = timeoutRecovery || snapshot.FailureAvoidance || scoreAnomalyPending || lowScorePending ||
		strings.TrimSpace(snapshot.ProbeRecoveryPhase) == core.ProbeRecoveryPhasePendingRealConfirmation
	if scoreAnomalyPending {
		snapshot.ProbeTriggerReason = core.ProbeReasonScoreAnomalyFastProbe
	}
	if !snapshot.ProbeRecoveryPending && probeTriggerReasonClearedOnRecovery(snapshot.ProbeTriggerReason) {
		snapshot.ProbeTriggerReason = ""
	}
	if !snapshot.ProbeRecoveryPending && snapshot.ProbeRecoverySuccessCount > 0 {
		snapshot.ProbeRecoverySuccessCount = 0
	}
}

func probeTriggerReasonClearedOnRecovery(reason string) bool {
	switch strings.TrimSpace(reason) {
	case probeReasonLongNoSuccess, probeReasonLowScore, probeReasonLowTraffic, core.ProbeReasonScoreAnomalyFastProbe, service.ChannelOverloadRecoveryReason, service.ChannelAuthConfigRecoveryReason:
		return true
	default:
		return false
	}
}

func recoverableQualityAnomalyTriggers(snapshot core.RuntimeSnapshot, evaluation RecoverableQualityEvaluation, recentRealSuccess bool) ([]string, float64) {
	triggers := make([]string, 0, len(evaluation.ItemScores)+len(evaluation.MissingItems)+1)
	seen := map[string]struct{}{}
	add := func(item string) {
		item = strings.TrimSpace(item)
		if item == "" {
			return
		}
		if _, exists := seen[item]; exists {
			return
		}
		seen[item] = struct{}{}
		triggers = append(triggers, item)
	}
	dropRatio := 0.0
	if evaluation.Valid && snapshot.RecoverableQualityBaseline > 0 {
		dropRatio = (snapshot.RecoverableQualityBaseline - evaluation.Score) / snapshot.RecoverableQualityBaseline
		if dropRatio < 0 || math.IsNaN(dropRatio) || math.IsInf(dropRatio, 0) {
			dropRatio = 0
		}
	}
	if evaluation.Valid && snapshot.RecoverableQualityBaselineSamples >= recoverableQualityMinBaselineSamples &&
		dropRatio > recoverableQualityDropRatioThreshold {
		add("recoverable_quality_score")
	}
	if snapshot.RecoverableQualityBaselineSamples >= recoverableQualityMinBaselineSamples && len(snapshot.RecoverableQualityItemBaselines) > 0 {
		for item, score := range evaluation.ItemScores {
			baseline := snapshot.RecoverableQualityItemBaselines[item]
			if baseline > 0 && baseline-score > recoverableQualityItemDropThreshold {
				add(item)
			}
		}
	}
	if recentRealSuccess {
		for _, item := range evaluation.MissingItems {
			add(item)
		}
	}
	sort.Strings(triggers)
	return triggers, round4(dropRatio)
}

func recoverableQualityRecentRealSuccess(snapshot core.RuntimeSnapshot, result core.AttemptResult) bool {
	return !result.IsHealthProbe && result.Success
}

func shouldUpdateRecoverableQualityBaseline(snapshot core.RuntimeSnapshot, result core.AttemptResult, evaluation RecoverableQualityEvaluation, anomaly bool) bool {
	if !evaluation.Valid || result.IsHealthProbe || !result.Success || result.ClientAborted || result.ConcurrencyLimited {
		return false
	}
	if strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryAuthConfigError {
		return false
	}
	if anomaly && snapshot.RecoverableQualityBaselineSamples >= recoverableQualityMinBaselineSamples {
		return false
	}
	return true
}

func updateRecoverableQualityBaseline(snapshot *core.RuntimeSnapshot, evaluation RecoverableQualityEvaluation) {
	if snapshot == nil || !evaluation.Valid {
		return
	}
	if snapshot.RecoverableQualityBaselineSamples <= 0 || snapshot.RecoverableQualityBaseline <= 0 {
		snapshot.RecoverableQualityBaseline = evaluation.Score
	} else {
		snapshot.RecoverableQualityBaseline = ewmaWithAlpha(snapshot.RecoverableQualityBaseline, evaluation.Score, recoverableQualityBaselineAlpha)
	}
	snapshot.RecoverableQualityBaseline = round4(snapshot.RecoverableQualityBaseline)
	snapshot.RecoverableQualityBaselineSamples++
	if snapshot.RecoverableQualityItemBaselines == nil {
		snapshot.RecoverableQualityItemBaselines = map[string]float64{}
	}
	for item, score := range evaluation.ItemScores {
		current := snapshot.RecoverableQualityItemBaselines[item]
		if current <= 0 || snapshot.RecoverableQualityBaselineSamples <= 1 {
			snapshot.RecoverableQualityItemBaselines[item] = score
			continue
		}
		snapshot.RecoverableQualityItemBaselines[item] = round4(ewmaWithAlpha(current, score, recoverableQualityBaselineAlpha))
	}
}

func clearScoreAnomalyRecovery(snapshot *core.RuntimeSnapshot) {
	if snapshot == nil {
		return
	}
	if strings.TrimSpace(snapshot.ProbeTriggerReason) == core.ProbeReasonScoreAnomalyFastProbe {
		snapshot.ProbeTriggerReason = ""
	}
	snapshot.ProbeRecoveryPending = false
	snapshot.ProbeRecoveryPhase = ""
	snapshot.ProbeFastRecoveryAttempts = 0
	snapshot.ProbeAnomalyTriggerItems = nil
}

func shouldClearScoreAnomalyRecoveryAfterRealSuccess(snapshot core.RuntimeSnapshot, result core.AttemptResult, score core.ScoreResult, lowScoreThreshold float64) bool {
	if result.IsHealthProbe || !result.Success || result.EmptyOutput || result.StreamInterrupted {
		return false
	}
	if strings.TrimSpace(result.ExperienceIssue) != "" || attemptHasUpstreamFailureSignal(result) {
		return false
	}
	if snapshot.FailureAvoidance || snapshot.CircuitOpen || snapshot.Cooldown {
		return false
	}
	if score.Total <= lowScoreThreshold {
		return false
	}
	return scoreAnomalyRecoveryActive(snapshot)
}

func scoreAnomalyRecoveryActive(snapshot core.RuntimeSnapshot) bool {
	switch strings.TrimSpace(snapshot.ProbeRecoveryPhase) {
	case core.ProbeRecoveryPhaseFastProbe, core.ProbeRecoveryPhasePendingRealConfirmation:
		return true
	}
	return strings.TrimSpace(snapshot.ProbeTriggerReason) == core.ProbeReasonScoreAnomalyFastProbe
}

func attemptHasUpstreamFailureSignal(result core.AttemptResult) bool {
	if strings.TrimSpace(result.ErrorCategory) != "" ||
		strings.TrimSpace(result.ErrorCode) != "" ||
		strings.TrimSpace(result.ErrorType) != "" {
		return true
	}
	if result.UpstreamStatus >= 400 {
		return true
	}
	if result.StatusCode >= 500 {
		return true
	}
	return false
}

func ewma(current float64, next float64) float64 {
	return ewmaWithAlpha(current, next, healthEWMAAlpha)
}

func rateEWMA(current float64, hit bool) float64 {
	next := 0.0
	if hit {
		next = 1
	}
	if current <= 0 {
		return next * healthEWMAAlpha
	}
	return ewmaWithAlpha(current, next, healthEWMAAlpha)
}

func ewmaWithAlpha(current float64, next float64, alpha float64) float64 {
	if next < 0 || math.IsNaN(next) || math.IsInf(next, 0) {
		return current
	}
	if current <= 0 {
		return next
	}
	if alpha <= 0 || alpha > 1 {
		alpha = healthEWMAAlpha
	}
	return current*(1-alpha) + next*alpha
}

func nonEmptyOutputExperienceIssue(result core.AttemptResult) bool {
	issue := strings.TrimSpace(result.ExperienceIssue)
	return issue != "" && issue != "empty_output" && !result.EmptyOutput
}

func isBalanceInsufficientAttempt(result core.AttemptResult) bool {
	if strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryUserQuotaExhausted {
		return false
	}
	if strings.TrimSpace(result.ErrorCategory) == "balance_or_quota" {
		return true
	}
	label := strings.ToLower(strings.TrimSpace(result.ErrorCode + " " + result.ErrorType + " " + result.ErrorMessage))
	return service.IsBalanceInsufficientMessage(label)
}

func appendRecentRealSample(samples []int64, observedAtUnix int64, now time.Time) []int64 {
	cutoff := now.Add(-realSampleWindow).Unix()
	out := samples[:0]
	for _, sampleAt := range samples {
		if sampleAt >= cutoff {
			out = append(out, sampleAt)
		}
	}
	if observedAtUnix > 0 {
		out = append(out, observedAtUnix)
	}
	return out
}

func realSamplesFromSnapshot(snapshot core.RuntimeSnapshot, now time.Time) []int64 {
	count := snapshot.RealSampleCount30m
	if count <= 0 || snapshot.LastRealAttemptAt <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	if snapshot.LastRealAttemptAt < now.Add(-realSampleWindow).Unix() {
		return nil
	}
	if count > snapshot.SampleCount && snapshot.SampleCount > 0 {
		count = snapshot.SampleCount
	}
	samples := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		samples = append(samples, snapshot.LastRealAttemptAt)
	}
	return samples
}

func durationMs(duration time.Duration, fallback float64) float64 {
	if duration <= 0 {
		return fallback
	}
	return float64(duration.Milliseconds())
}

func successRate(stats *healthStats) float64 {
	if stats == nil || stats.sampleCount <= 0 {
		return defaultHealthSuccessRate
	}
	return float64(stats.successCount) / float64(stats.sampleCount)
}

var _ core.ExecutionRecorder = (*RuntimeHealthMonitor)(nil)
