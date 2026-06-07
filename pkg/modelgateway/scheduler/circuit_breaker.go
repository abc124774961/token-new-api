package scheduler

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
)

const (
	defaultCircuitFailureThreshold = 0.50
	defaultCircuitMinSamples       = 10
	defaultCircuitOpenDuration     = 30 * time.Second
	defaultCircuitProbeCount       = 3
)

const (
	CircuitErrorStreamInterrupted = "stream_interrupted"
	CircuitErrorRateLimit         = "rate_limit"
	CircuitErrorAuth              = "auth"
	CircuitErrorQuota             = "quota"
	CircuitErrorServer            = "server_error"
	CircuitErrorUpstream          = "upstream_error"
	CircuitErrorConcurrencyLimit  = "concurrency_limit"
)

type CircuitErrorPolicy struct {
	FailureThreshold   float64
	MinSamples         int
	OpenDuration       time.Duration
	HalfOpenProbeCount int
}

type CircuitBreakerOptions struct {
	FailureThreshold   float64
	MinSamples         int
	OpenDuration       time.Duration
	HalfOpenProbeCount int
	ErrorPolicies      map[string]CircuitErrorPolicy
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
	openReason       string
	errorCounts      map[string]int
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
	options.ErrorPolicies = normalizeCircuitErrorPolicies(options.ErrorPolicies, options)
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
	key = normalizeRuntimeKey(key)
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
	key = normalizeRuntimeKey(key)
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
	if b == nil || result.ChannelID <= 0 || result.ClientAborted || isCircuitClientRequestResult(result) || result.BalanceInsufficient || isCircuitBalanceInsufficientResult(result) || isCircuitOverloadSkipResult(result) || isCircuitSchedulerExhaustedResult(result) {
		return
	}
	key := normalizeRuntimeKey(result.RuntimeKey())
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.stateForLocked(key)
	b.advanceLocked(state, b.now())
	if result.Success {
		b.reportSuccessLocked(state)
		return
	}
	kind, ok := b.classifyFailure(result)
	if !ok {
		return
	}
	b.reportFailureLocked(state, kind)
}

func (b *CircuitBreaker) Reset(key core.RuntimeKey) bool {
	if b == nil {
		return false
	}
	key = normalizeRuntimeKey(key)
	if key.ChannelID <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state, ok := b.states[key]
	if !ok {
		return false
	}
	changed := circuitStateNeedsReset(state)
	delete(b.states, key)
	return changed
}

func (b *CircuitBreaker) ResetChannel(channelID int) int {
	if b == nil || channelID <= 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cleared := 0
	for key, state := range b.states {
		if key.ChannelID != channelID {
			continue
		}
		if circuitStateNeedsReset(state) {
			cleared++
		}
		delete(b.states, key)
	}
	return cleared
}

func (b *CircuitBreaker) stateForLocked(key core.RuntimeKey) *circuitState {
	state, ok := b.states[key]
	if ok {
		return state
	}
	state = &circuitState{
		state:            core.CircuitStateClosed,
		halfOpenProbeMax: b.options.HalfOpenProbeCount,
		errorCounts:      map[string]int{},
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

func (b *CircuitBreaker) reportFailureLocked(state *circuitState, kind string) {
	if kind == "" {
		kind = CircuitErrorUpstream
	}
	switch state.state {
	case core.CircuitStateHalfOpen:
		b.openLocked(state, kind)
	case core.CircuitStateOpen:
		state.failureCount++
		state.sampleCount++
		incrementCircuitErrorCount(state, kind)
	default:
		state.failureCount++
		state.sampleCount++
		incrementCircuitErrorCount(state, kind)
		if policy, ok := b.options.ErrorPolicies[kind]; ok {
			if circuitErrorPolicyOpen(state, kind, policy) {
				b.openLockedWithPolicy(state, kind, policy)
			}
			return
		}
		if state.sampleCount >= b.options.MinSamples && failureRate(state) >= b.options.FailureThreshold {
			b.openLocked(state, kind)
		}
	}
}

func (b *CircuitBreaker) openLocked(state *circuitState, reason string) {
	policy := CircuitErrorPolicy{
		OpenDuration:       b.options.OpenDuration,
		HalfOpenProbeCount: b.options.HalfOpenProbeCount,
	}
	if override, ok := b.options.ErrorPolicies[reason]; ok {
		policy = override
	}
	b.openLockedWithPolicy(state, reason, policy)
}

func (b *CircuitBreaker) openLockedWithPolicy(state *circuitState, reason string, policy CircuitErrorPolicy) {
	state.state = core.CircuitStateOpen
	state.openReason = reason
	state.openUntil = b.now().Add(policy.OpenDuration)
	state.halfOpenProbeMax = policy.HalfOpenProbeCount
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
		OpenReason:        state.openReason,
		ErrorCounts:       copyCircuitErrorCounts(state.errorCounts),
		OpenUntil:         state.openUntil,
		HalfOpenProbeUsed: state.halfOpenProbeIn,
		HalfOpenProbeMax:  state.halfOpenProbeMax,
	}
}

func (b *CircuitBreaker) applySnapshot(snapshot core.CircuitSnapshot) bool {
	if b == nil {
		return false
	}
	snapshot = normalizeCircuitSnapshot(snapshot)
	if snapshot.Key.ChannelID <= 0 || snapshot.State == core.CircuitStateClosed {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.stateForLocked(snapshot.Key)
	b.advanceLocked(state, b.now())
	if state.state == core.CircuitStateOpen && snapshot.State != core.CircuitStateOpen {
		return false
	}
	if circuitSeverity(snapshot.State) < circuitSeverity(state.state) {
		return false
	}
	state.state = snapshot.State
	state.failureCount = snapshot.FailureCount
	state.successCount = snapshot.SuccessCount
	state.sampleCount = snapshot.SampleCount
	state.openReason = snapshot.OpenReason
	state.errorCounts = copyCircuitErrorCounts(snapshot.ErrorCounts)
	state.openUntil = snapshot.OpenUntil
	state.halfOpenProbeIn = snapshot.HalfOpenProbeUsed
	state.halfOpenProbeMax = snapshot.HalfOpenProbeMax
	if state.halfOpenProbeMax <= 0 {
		state.halfOpenProbeMax = b.options.HalfOpenProbeCount
	}
	return true
}

func resetCircuitState(state *circuitState) {
	state.state = core.CircuitStateClosed
	state.failureCount = 0
	state.successCount = 0
	state.sampleCount = 0
	state.openReason = ""
	state.errorCounts = map[string]int{}
	state.openUntil = time.Time{}
	state.halfOpenProbeIn = 0
	state.halfOpenProbeMax = 0
}

func circuitStateNeedsReset(state *circuitState) bool {
	if state == nil {
		return false
	}
	return state.state != core.CircuitStateClosed ||
		state.failureCount > 0 ||
		state.successCount > 0 ||
		state.sampleCount > 0 ||
		state.openReason != "" ||
		len(state.errorCounts) > 0 ||
		!state.openUntil.IsZero() ||
		state.halfOpenProbeIn > 0
}

func failureRate(state *circuitState) float64 {
	if state == nil || state.sampleCount <= 0 {
		return 0
	}
	return float64(state.failureCount) / float64(state.sampleCount)
}

func (b *CircuitBreaker) classifyFailure(result core.AttemptResult) (string, bool) {
	kind := ClassifyCircuitError(result)
	if kind == "" {
		return "", false
	}
	if kind == CircuitErrorConcurrencyLimit {
		return kind, false
	}
	if isDefaultCircuitFailure(result) {
		return kind, true
	}
	_, ok := b.options.ErrorPolicies[kind]
	return kind, ok
}

// ClassifyCircuitError normalizes an attempt failure into the circuit-breaker
// error label used by policies and observability. A returned label does not
// mean the failure is counted by the circuit breaker; policies still decide
// that in classifyFailure.
func ClassifyCircuitError(result core.AttemptResult) string {
	if result.ClientAborted || isCircuitClientRequestResult(result) || strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryUserQuotaExhausted || result.BalanceInsufficient || isCircuitBalanceInsufficientResult(result) || isCircuitOverloadSkipResult(result) || isCircuitSchedulerExhaustedResult(result) {
		return ""
	}
	if result.StreamInterrupted {
		return CircuitErrorStreamInterrupted
	}
	if result.ConcurrencyLimited || isCircuitConcurrencyLimitResult(result) {
		return CircuitErrorConcurrencyLimit
	}
	label := strings.ToLower(strings.TrimSpace(result.ErrorCode + " " + result.ErrorType + " " + result.ErrorMessage))
	if containsAnyCircuitLabel(label, "concurrency limit exceeded for user", "too many pending requests") {
		return CircuitErrorConcurrencyLimit
	}
	if containsAnyCircuitLabel(label, "rate_limit", "rate-limited", "rate_limited", "too_many_requests", "throttle") {
		return CircuitErrorRateLimit
	}
	if containsAnyCircuitLabel(label, "insufficient_user_quota", "pre_consume_token_quota_failed", "quota", "balance_not_enough", "quota_not_enough") {
		return CircuitErrorQuota
	}
	if result.StatusCode == http.StatusUnauthorized || result.StatusCode == http.StatusForbidden {
		return CircuitErrorAuth
	}
	if containsAnyCircuitLabel(label, "invalid_key", "invalid api key", "unauthorized", "forbidden", "access_denied", "permission_denied") {
		return CircuitErrorAuth
	}
	if result.StatusCode >= http.StatusInternalServerError {
		return CircuitErrorServer
	}
	if result.StatusCode <= 0 && (result.ErrorCode != "" || result.ErrorType != "") {
		return CircuitErrorUpstream
	}
	return ""
}

func isDefaultCircuitFailure(result core.AttemptResult) bool {
	if result.ClientAborted || isCircuitClientRequestResult(result) || strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryUserQuotaExhausted || result.BalanceInsufficient || isCircuitBalanceInsufficientResult(result) || isCircuitOverloadSkipResult(result) || isCircuitSchedulerExhaustedResult(result) {
		return false
	}
	if result.StreamInterrupted {
		return true
	}
	if result.ConcurrencyLimited || isCircuitConcurrencyLimitResult(result) {
		return false
	}
	if result.StatusCode >= http.StatusInternalServerError {
		return true
	}
	if result.StatusCode <= 0 && (result.ErrorCode != "" || result.ErrorType != "") {
		return true
	}
	return false
}

func isCircuitClientRequestResult(result core.AttemptResult) bool {
	if strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryClientRequestError {
		return true
	}
	label := strings.Join([]string{
		result.ErrorCode,
		result.ErrorType,
		result.ErrorMessage,
	}, " ")
	return service.IsClientContextLimitMessage(label)
}

func isCircuitOverloadSkipResult(result core.AttemptResult) bool {
	if strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryOverloadSkip {
		return true
	}
	return result.StatusCode == http.StatusTooManyRequests
}

func isCircuitSchedulerExhaustedResult(result core.AttemptResult) bool {
	return strings.TrimSpace(result.ErrorCategory) == core.ErrorCategorySchedulerExhausted
}

func isCircuitConcurrencyLimitResult(result core.AttemptResult) bool {
	if result.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if result.ErrorCode == string(types.ErrorCodeChannelConcurrencyLimit) {
		return true
	}
	label := strings.ToLower(strings.TrimSpace(result.ErrorCode + " " + result.ErrorType + " " + result.ErrorMessage))
	return containsAnyCircuitLabel(label, "concurrency limit exceeded for user", "too many pending requests")
}

func isCircuitBalanceInsufficientResult(result core.AttemptResult) bool {
	if strings.TrimSpace(result.ErrorCategory) == core.ErrorCategoryUserQuotaExhausted {
		return false
	}
	if strings.TrimSpace(result.ErrorCategory) == "balance_or_quota" {
		return true
	}
	label := strings.ToLower(strings.TrimSpace(result.ErrorCode + " " + result.ErrorType + " " + result.ErrorMessage))
	return containsAnyCircuitLabel(label,
		"balance_insufficient",
		"insufficient_user_quota",
		"insufficient account balance",
		"insufficient balance",
		"insufficient credit",
		"insufficient credits",
		"balance_not_enough",
		"quota_not_enough",
		"quota not enough",
		"余额不足",
	)
}

func containsAnyCircuitLabel(label string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(label, needle) {
			return true
		}
	}
	return false
}

func incrementCircuitErrorCount(state *circuitState, kind string) {
	if state == nil || kind == "" {
		return
	}
	if state.errorCounts == nil {
		state.errorCounts = map[string]int{}
	}
	state.errorCounts[kind]++
}

func circuitErrorPolicyOpen(state *circuitState, kind string, policy CircuitErrorPolicy) bool {
	if state == nil || state.sampleCount <= 0 || kind == "" {
		return false
	}
	failures := state.errorCounts[kind]
	if failures < policy.MinSamples {
		return false
	}
	return float64(failures)/float64(state.sampleCount) >= policy.FailureThreshold
}

func copyCircuitErrorCounts(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]int, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func normalizeCircuitErrorPolicies(src map[string]CircuitErrorPolicy, defaults CircuitBreakerOptions) map[string]CircuitErrorPolicy {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]CircuitErrorPolicy, len(src))
	for kind, policy := range src {
		kind = normalizeCircuitErrorKind(kind)
		if kind == "" {
			continue
		}
		if policy.FailureThreshold <= 0 || policy.FailureThreshold > 1 {
			policy.FailureThreshold = defaults.FailureThreshold
		}
		if policy.MinSamples <= 0 {
			policy.MinSamples = defaults.MinSamples
		}
		if policy.OpenDuration <= 0 {
			policy.OpenDuration = defaults.OpenDuration
		}
		if policy.HalfOpenProbeCount <= 0 {
			policy.HalfOpenProbeCount = defaults.HalfOpenProbeCount
		}
		out[kind] = policy
	}
	return out
}

func normalizeCircuitErrorKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case CircuitErrorStreamInterrupted, CircuitErrorRateLimit, CircuitErrorAuth, CircuitErrorQuota, CircuitErrorServer, CircuitErrorUpstream, CircuitErrorConcurrencyLimit:
		return kind
	default:
		return ""
	}
}

var _ core.CircuitBreaker = (*CircuitBreaker)(nil)
