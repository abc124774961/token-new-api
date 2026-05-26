package scheduler

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

const (
	defaultHealthDurationMs  = 1000
	defaultHealthSuccessRate = 0.80
	defaultHealthScore       = 0.55
	healthEWMAAlpha          = 0.20
	healthSlowTTFTAlpha      = 0.45
	realSampleWindow         = 30 * time.Minute
	probeReasonLongNoSuccess = "long_no_success"
	probeReasonLowScore      = "low_score"
	probeReasonLowTraffic    = "low_traffic"
)

type RuntimeHealthMonitor struct {
	store   core.RuntimeSnapshotStore
	breaker core.CircuitBreaker
	mu      sync.Mutex
	stats   map[core.RuntimeKey]*healthStats
}

type healthStats struct {
	successCount   int
	sampleCount    int
	durationMs     float64
	ttftMs         float64
	speedSamples   int
	successScore   float64
	speedScore     float64
	emptyRate      float64
	issueRate      float64
	latencySamples []core.RuntimeLatencySample
	realSampleAt   []int64
}

func NewRuntimeHealthMonitor(store core.RuntimeSnapshotStore, breaker core.CircuitBreaker) *RuntimeHealthMonitor {
	return &RuntimeHealthMonitor{
		store:   store,
		breaker: breaker,
		stats:   map[core.RuntimeKey]*healthStats{},
	}
}

func (m *RuntimeHealthMonitor) Record(ctx context.Context, record core.DispatchRecord) {}

func (m *RuntimeHealthMonitor) Report(ctx context.Context, result core.AttemptResult) {
	if m == nil {
		return
	}
	if result.ClientAborted {
		return
	}
	if result.BalanceInsufficient || isBalanceInsufficientAttempt(result) {
		if result.ChannelID > 0 {
			service.MarkChannelBalanceInsufficient(result.ChannelID)
		}
		return
	}
	if isCircuitOverloadSkipResult(result) {
		return
	}
	if m.breaker != nil {
		m.breaker.Report(result)
	}
	if result.ConcurrencyLimited {
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
	stats.sampleCount++
	if result.Success {
		stats.successCount++
	}
	resultSuccess := result.Success && !result.EmptyOutput && strings.TrimSpace(result.ExperienceIssue) == ""
	stats.successScore = ewma(stats.successScore, boolScore(resultSuccess, 0.05))
	if result.Success {
		if sample, valid := runtimeLatencySampleFromAttempt(result, observedAt); valid {
			stats.latencySamples = appendRuntimeLatencySample(stats.latencySamples, sample)
			stats.durationMs, stats.ttftMs, stats.speedScore = runtimeLatencyStats(stats.latencySamples)
			stats.speedSamples = len(stats.latencySamples)
		}
	} else if stats.durationMs <= 0 {
		stats.durationMs = durationMs(result.Duration, 0)
	}
	stats.emptyRate = rateEWMA(stats.emptyRate, result.EmptyOutput)
	stats.issueRate = rateEWMA(stats.issueRate, nonEmptyOutputExperienceIssue(result))
	if !ok {
		snapshot = core.RuntimeSnapshot{
			Key:                key,
			CostRatio:          0,
			GroupPriorityRatio: 1,
		}
	}
	snapshot.Key = key
	snapshot.SampleCount = stats.sampleCount
	snapshot.RecentLatencySamples = append([]core.RuntimeLatencySample(nil), stats.latencySamples...)
	snapshot.SuccessRate = successRate(stats)
	snapshot.SuccessScore = stats.successScore
	snapshot.SpeedScore = stats.speedScore
	snapshot.DurationMs = stats.durationMs
	if stats.ttftMs > 0 {
		snapshot.TTFTMs = stats.ttftMs
	}
	snapshot.EmptyOutputRate = stats.emptyRate
	snapshot.ExperienceIssueRate = stats.issueRate
	snapshot.ExperienceScore = experienceScoreFromRates(stats.emptyRate, stats.issueRate)
	snapshot.HealthScoreAverage = HealthScoreAverage(snapshot)
	m.applyProbeRecovery(&snapshot, result)
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
		snapshot.CircuitState = circuit.State
		snapshot.CircuitOpen = circuit.State == core.CircuitStateOpen
	}
	m.store.Put(snapshot)
	m.mu.Unlock()
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
		stats.durationMs, stats.ttftMs, stats.speedScore = runtimeLatencyStats(stats.latencySamples)
		stats.speedSamples = len(stats.latencySamples)
	} else {
		stats.durationMs = 0
		stats.ttftMs = 0
	}
	stats.successScore = snapshot.SuccessScore
	if len(stats.latencySamples) == 0 {
		stats.speedScore = 0
	}
	stats.emptyRate = snapshot.EmptyOutputRate
	stats.issueRate = snapshot.ExperienceIssueRate
	stats.realSampleAt = realSamplesFromSnapshot(snapshot, time.Now())
	return stats
}

func (m *RuntimeHealthMonitor) applyProbeRecovery(snapshot *core.RuntimeSnapshot, result core.AttemptResult) {
	if snapshot == nil || result.ChannelID <= 0 || result.ConcurrencyLimited || result.ClientAborted {
		return
	}
	setting := scheduler_setting.GetSetting()
	required := setting.ProbeRecoverySuccessesRequired
	if required <= 0 {
		required = 2
	}
	lowScoreThreshold := setting.ProbeLowScoreThreshold
	if lowScoreThreshold <= 0 {
		lowScoreThreshold = 0.62
	}
	snapshot.ProbeRecoveryRequired = required
	if result.IsHealthProbe {
		snapshot.ProbeTriggerReason = strings.TrimSpace(result.ProbeReason)
	}
	if result.IsHealthProbe && result.Success {
		snapshot.ProbeRecoverySuccessCount++
		if snapshot.ProbeRecoverySuccessCount >= required && service.GetChannelFailureAvoidanceStatus(result.ChannelID) != nil {
			service.ClearChannelFailureAvoidance(result.ChannelID)
			snapshot.FailureAvoidance = false
			snapshot.ProbeRecoveryPending = false
			snapshot.ProbeRecoverySuccessCount = required
			return
		}
	}
	if (result.IsHealthProbe && !result.Success) || (!result.IsHealthProbe && !result.Success) {
		snapshot.ProbeRecoverySuccessCount = 0
	}
	if !result.IsHealthProbe && !result.Success {
		snapshot.FailureAvoidance = snapshot.FailureAvoidance || service.GetChannelFailureAvoidanceStatus(result.ChannelID) != nil
	}
	snapshot.FailureAvoidance = snapshot.FailureAvoidance || service.GetChannelFailureAvoidanceStatus(result.ChannelID) != nil
	snapshot.ProbeRecoveryPending = snapshot.FailureAvoidance || (snapshot.HealthScoreAverage > 0 && snapshot.HealthScoreAverage < lowScoreThreshold)
	if !snapshot.ProbeRecoveryPending && probeTriggerReasonClearedOnRecovery(snapshot.ProbeTriggerReason) {
		snapshot.ProbeTriggerReason = ""
	}
	if !snapshot.ProbeRecoveryPending && snapshot.ProbeRecoverySuccessCount > 0 {
		snapshot.ProbeRecoverySuccessCount = 0
	}
}

func HealthScoreAverage(snapshot core.RuntimeSnapshot) float64 {
	scores := []float64{}
	if snapshot.SuccessScore > 0 {
		scores = append(scores, snapshot.SuccessScore)
	}
	if snapshot.SpeedScore > 0 {
		scores = append(scores, snapshot.SpeedScore)
	}
	if snapshot.ExperienceScore > 0 {
		scores = append(scores, snapshot.ExperienceScore)
	}
	if len(scores) == 0 {
		return clampHealthScore(snapshot.HealthScoreAverage)
	}
	total := 0.0
	for _, score := range scores {
		total += score
	}
	return total / float64(len(scores))
}

func probeTriggerReasonClearedOnRecovery(reason string) bool {
	switch strings.TrimSpace(reason) {
	case probeReasonLongNoSuccess, probeReasonLowScore, probeReasonLowTraffic:
		return true
	default:
		return false
	}
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
	if strings.TrimSpace(result.ErrorCategory) == "balance_or_quota" {
		return true
	}
	label := strings.ToLower(strings.TrimSpace(result.ErrorCode + " " + result.ErrorType + " " + result.ErrorMessage))
	return service.IsBalanceInsufficientMessage(label)
}

func experienceScoreFromRates(emptyRate float64, issueRate float64) float64 {
	return clampHealthScore(1 - clamp01(emptyRate)*0.85 - clamp01(issueRate)*0.65)
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

func ttftEWMAAlpha(current float64, next float64) float64 {
	if next >= ttftPenaltyPoorMs || (current > 0 && next > current*1.8 && next >= ttftPenaltySlowMs) {
		return healthSlowTTFTAlpha
	}
	return healthEWMAAlpha
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

func boolScore(value bool, falseScore float64) float64 {
	if value {
		return 1
	}
	return falseScore
}

func attemptSpeedScore(result core.AttemptResult) float64 {
	if sample, ok := runtimeLatencySampleFromAttempt(result, time.Time{}); ok {
		_, _, speedScore := runtimeLatencyStats([]core.RuntimeLatencySample{sample})
		if speedScore > 0 {
			return speedScore
		}
	}
	if result.TTFT > 0 {
		ttftMs := durationMs(result.TTFT, 0)
		return inverseLatencyScore(ttftMs, 800, 20000)
	}
	if result.Duration > 0 {
		durationMs := durationMs(result.Duration, defaultHealthDurationMs)
		return inverseLatencyScore(durationMs, 3000, 90000)
	}
	return defaultHealthScore
}

func clampHealthScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

var _ core.ExecutionRecorder = (*RuntimeHealthMonitor)(nil)
