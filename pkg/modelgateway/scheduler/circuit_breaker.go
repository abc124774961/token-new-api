package scheduler

import (
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	defaultCircuitFailureThreshold = 0.50
	defaultCircuitMinSamples       = 10
	defaultCircuitOpenDuration     = 30 * time.Second
	defaultCircuitProbeCount       = 3
)

type CircuitBreakerOptions struct {
	FailureThreshold   float64
	MinSamples         int
	OpenDuration       time.Duration
	HalfOpenProbeCount int
}

type CircuitBreaker struct {
	mu      sync.Mutex
	options CircuitBreakerOptions
	states  map[core.RuntimeKey]*circuitState
	now     func() time.Time
}

type circuitState struct {
	state            core.CircuitState
	failureCount     int
	successCount     int
	sampleCount      int
	openUntil        time.Time
	halfOpenProbeMax int
	halfOpenProbeIn  int
}

func NewCircuitBreaker(options CircuitBreakerOptions) *CircuitBreaker {
	return newCircuitBreaker(options, time.Now)
}

func NewCircuitBreakerForTest(options CircuitBreakerOptions, now func() time.Time) *CircuitBreaker {
	return newCircuitBreaker(options, now)
}

func newCircuitBreaker(options CircuitBreakerOptions, now func() time.Time) *CircuitBreaker {
	if options.FailureThreshold <= 0 || options.FailureThreshold > 1 {
		options.FailureThreshold = defaultCircuitFailureThreshold
	}
	if options.MinSamples <= 0 {
		options.MinSamples = defaultCircuitMinSamples
	}
	if options.OpenDuration <= 0 {
		options.OpenDuration = defaultCircuitOpenDuration
	}
	if options.HalfOpenProbeCount <= 0 {
		options.HalfOpenProbeCount = defaultCircuitProbeCount
	}
	if now == nil {
		now = time.Now
	}
	return &CircuitBreaker{
		options: options,
		states:  map[core.RuntimeKey]*circuitState{},
		now:     now,
	}
}

func (b *CircuitBreaker) Snapshot(key core.RuntimeKey) core.CircuitSnapshot {
	if b == nil {
		return core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.stateForLocked(key)
	b.advanceLocked(state, b.now())
	return b.snapshotLocked(key, state)
}

func (b *CircuitBreaker) ListSnapshots() []core.CircuitSnapshot {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	snapshots := make([]core.CircuitSnapshot, 0, len(b.states))
	for key, state := range b.states {
		b.advanceLocked(state, now)
		snapshots = append(snapshots, b.snapshotLocked(key, state))
	}
	return snapshots
}

func (b *CircuitBreaker) AllowProbe(key core.RuntimeKey) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.stateForLocked(key)
	now := b.now()
	b.advanceLocked(state, now)
	switch state.state {
	case core.CircuitStateClosed:
		return true
	case core.CircuitStateHalfOpen:
		if state.halfOpenProbeIn >= state.halfOpenProbeMax {
			return false
		}
		state.halfOpenProbeIn++
		return true
	default:
		return false
	}
}

func (b *CircuitBreaker) Report(result core.AttemptResult) {
	if b == nil || result.ChannelID <= 0 {
		return
	}
	key := result.RuntimeKey()
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.stateForLocked(key)
	b.advanceLocked(state, b.now())
	if result.Success {
		b.reportSuccessLocked(state)
		return
	}
	if !isCircuitFailure(result) {
		return
	}
	b.reportFailureLocked(state)
}

func (b *CircuitBreaker) stateForLocked(key core.RuntimeKey) *circuitState {
	state, ok := b.states[key]
	if ok {
		return state
	}
	state = &circuitState{
		state:            core.CircuitStateClosed,
		halfOpenProbeMax: b.options.HalfOpenProbeCount,
	}
	b.states[key] = state
	return state
}

func (b *CircuitBreaker) advanceLocked(state *circuitState, now time.Time) {
	if state == nil {
		return
	}
	if state.halfOpenProbeMax <= 0 {
		state.halfOpenProbeMax = b.options.HalfOpenProbeCount
	}
	if state.state == core.CircuitStateOpen && !state.openUntil.IsZero() && !state.openUntil.After(now) {
		state.state = core.CircuitStateHalfOpen
		state.halfOpenProbeIn = 0
	}
}

func (b *CircuitBreaker) reportSuccessLocked(state *circuitState) {
	switch state.state {
	case core.CircuitStateHalfOpen:
		state.successCount++
		if state.successCount >= state.halfOpenProbeMax {
			resetCircuitState(state)
		}
	case core.CircuitStateClosed:
		if state.failureCount > 0 {
			state.failureCount--
		}
		state.successCount++
		state.sampleCount++
	default:
		resetCircuitState(state)
	}
}

func (b *CircuitBreaker) reportFailureLocked(state *circuitState) {
	switch state.state {
	case core.CircuitStateHalfOpen:
		b.openLocked(state)
	case core.CircuitStateOpen:
		state.failureCount++
		state.sampleCount++
	default:
		state.failureCount++
		state.sampleCount++
		if state.sampleCount >= b.options.MinSamples && failureRate(state) >= b.options.FailureThreshold {
			b.openLocked(state)
		}
	}
}

func (b *CircuitBreaker) openLocked(state *circuitState) {
	state.state = core.CircuitStateOpen
	state.openUntil = b.now().Add(b.options.OpenDuration)
	state.halfOpenProbeIn = 0
	state.successCount = 0
}

func (b *CircuitBreaker) snapshotLocked(key core.RuntimeKey, state *circuitState) core.CircuitSnapshot {
	if state == nil {
		return core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed}
	}
	return core.CircuitSnapshot{
		Key:               key,
		State:             state.state,
		FailureCount:      state.failureCount,
		SuccessCount:      state.successCount,
		SampleCount:       state.sampleCount,
		FailureRate:       failureRate(state),
		OpenUntil:         state.openUntil,
		HalfOpenProbeUsed: state.halfOpenProbeIn,
		HalfOpenProbeMax:  state.halfOpenProbeMax,
	}
}

func resetCircuitState(state *circuitState) {
	state.state = core.CircuitStateClosed
	state.failureCount = 0
	state.successCount = 0
	state.sampleCount = 0
	state.openUntil = time.Time{}
	state.halfOpenProbeIn = 0
}

func failureRate(state *circuitState) float64 {
	if state == nil || state.sampleCount <= 0 {
		return 0
	}
	return float64(state.failureCount) / float64(state.sampleCount)
}

func isCircuitFailure(result core.AttemptResult) bool {
	if result.StreamInterrupted {
		return true
	}
	if result.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if result.StatusCode >= http.StatusInternalServerError {
		return true
	}
	if result.StatusCode <= 0 && (result.ErrorCode != "" || result.ErrorType != "") {
		return true
	}
	return false
}

var _ core.CircuitBreaker = (*CircuitBreaker)(nil)
