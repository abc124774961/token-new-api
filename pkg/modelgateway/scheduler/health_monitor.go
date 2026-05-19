package scheduler

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	defaultHealthDurationMs  = 1000
	defaultHealthSuccessRate = 0.80
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
	stats.durationMs = ewma(stats.durationMs, durationMs(result.Duration, defaultHealthDurationMs))
	if result.TTFT > 0 {
		stats.ttftMs = ewma(stats.ttftMs, durationMs(result.TTFT, 0))
	}
	snapshot, ok := m.store.Get(key)
	if !ok {
		snapshot = core.RuntimeSnapshot{
			Key:                key,
			CostRatio:          1,
			GroupPriorityRatio: 1,
		}
	}
	snapshot.Key = key
	snapshot.SampleCount = stats.sampleCount
	snapshot.SuccessRate = successRate(stats)
	snapshot.DurationMs = stats.durationMs
	if stats.ttftMs > 0 {
		snapshot.TTFTMs = stats.ttftMs
	}
	if m.breaker != nil {
		circuit := m.breaker.Snapshot(key)
		snapshot.CircuitState = circuit.State
		snapshot.CircuitOpen = circuit.State == core.CircuitStateOpen
	}
	m.store.Put(snapshot)
	m.mu.Unlock()
}

func ewma(current float64, next float64) float64 {
	if next <= 0 || math.IsNaN(next) || math.IsInf(next, 0) {
		return current
	}
	if current <= 0 {
		return next
	}
	return current*0.80 + next*0.20
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
