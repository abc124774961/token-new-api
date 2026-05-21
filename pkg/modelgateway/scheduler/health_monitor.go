package scheduler

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	defaultHealthDurationMs  = 1000
	defaultHealthSuccessRate = 0.80
	defaultHealthScore       = 0.55
	healthEWMAAlpha          = 0.20
	healthSlowTTFTAlpha      = 0.45
)

type RuntimeHealthMonitor struct {
	store   core.RuntimeSnapshotStore
	breaker core.CircuitBreaker
	mu      sync.Mutex
	stats   map[core.RuntimeKey]*healthStats
}

type healthStats struct {
	successCount int
	sampleCount  int
	durationMs   float64
	ttftMs       float64
	successScore float64
	speedScore   float64
	emptyRate    float64
	issueRate    float64
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
	if m.breaker != nil {
		m.breaker.Report(result)
	}
	if m.store == nil {
		return
	}
	key := result.RuntimeKey()
	if key.ChannelID <= 0 {
		return
	}
	m.mu.Lock()
	stats := m.stats[key]
	if stats == nil {
		stats = &healthStats{}
		m.stats[key] = stats
	}
	stats.sampleCount++
	if result.Success {
		stats.successCount++
	}
	resultSuccess := result.Success && !result.EmptyOutput && strings.TrimSpace(result.ExperienceIssue) == ""
	stats.successScore = ewma(stats.successScore, boolScore(resultSuccess, 0.05))
	stats.durationMs = ewma(stats.durationMs, durationMs(result.Duration, defaultHealthDurationMs))
	if result.TTFT > 0 {
		nextTTFTMs := durationMs(result.TTFT, 0)
		stats.ttftMs = ewmaWithAlpha(stats.ttftMs, nextTTFTMs, ttftEWMAAlpha(stats.ttftMs, nextTTFTMs))
	}
	stats.speedScore = ewmaWithAlpha(stats.speedScore, attemptSpeedScore(result), ttftEWMAAlpha(stats.ttftMs, durationMs(result.TTFT, 0)))
	stats.emptyRate = ewma(stats.emptyRate, boolScore(result.EmptyOutput, 0))
	stats.issueRate = ewma(stats.issueRate, boolScore(strings.TrimSpace(result.ExperienceIssue) != "", 0))
	snapshot, ok := m.store.Get(key)
	if !ok {
		snapshot = core.RuntimeSnapshot{
			Key:                key,
			CostRatio:          0,
			GroupPriorityRatio: 1,
		}
	}
	snapshot.Key = key
	snapshot.SampleCount = stats.sampleCount
	snapshot.SuccessRate = successRate(stats)
	snapshot.SuccessScore = stats.successScore
	snapshot.SpeedScore = stats.speedScore
	snapshot.DurationMs = stats.durationMs
	if stats.ttftMs > 0 {
		snapshot.TTFTMs = stats.ttftMs
	}
	snapshot.EmptyOutputRate = stats.emptyRate
	snapshot.ExperienceIssueRate = stats.issueRate
	snapshot.ExperienceScore = clampHealthScore(1 - stats.emptyRate*0.85 - stats.issueRate*0.65)
	if m.breaker != nil {
		circuit := m.breaker.Snapshot(key)
		snapshot.CircuitState = circuit.State
		snapshot.CircuitOpen = circuit.State == core.CircuitStateOpen
	}
	m.store.Put(snapshot)
	m.mu.Unlock()
}

func ewma(current float64, next float64) float64 {
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
