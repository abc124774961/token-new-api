package scheduler

import (
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const scoreStatsVersion = 1
const scoreStatsLatencyLimit = 64

type ScoreStats struct {
	Version int                       `json:"version"`
	Samples int                       `json:"samples"`
	Rates   map[string]ScoreRateStats `json:"rates,omitempty"`
	Latency ScoreLatencyStats         `json:"latency,omitempty"`
}

type ScoreRateStats struct {
	Success int     `json:"success,omitempty"`
	Count   int     `json:"count,omitempty"`
	Total   int     `json:"total,omitempty"`
	EWMA    float64 `json:"ewma,omitempty"`
}

type ScoreLatencyStats struct {
	TTFTMs          []float64 `json:"ttft_ms,omitempty"`
	DurationMs      []float64 `json:"duration_ms,omitempty"`
	TokensPerSecond []float64 `json:"tokens_per_second,omitempty"`
}

func scoreStatsFromSnapshot(snapshot core.RuntimeSnapshot) ScoreStats {
	stats := ScoreStats{Version: scoreStatsVersion}
	if snapshot.ScoreStatsJSON != "" {
		_ = common.UnmarshalJsonStr(snapshot.ScoreStatsJSON, &stats)
	}
	if stats.Version <= 0 {
		stats.Version = scoreStatsVersion
	}
	if stats.Rates == nil {
		stats.Rates = map[string]ScoreRateStats{}
	}
	return stats
}

func (s ScoreStats) Marshal() string {
	if s.Version <= 0 {
		s.Version = scoreStatsVersion
	}
	if s.Rates == nil {
		s.Rates = map[string]ScoreRateStats{}
	}
	data, err := common.Marshal(s)
	if err != nil {
		return ""
	}
	return string(data)
}

func updateScoreStats(stats ScoreStats, result core.AttemptResult, decision core.ScoreSampleDecision) ScoreStats {
	if stats.Version <= 0 {
		stats.Version = scoreStatsVersion
	}
	if stats.Rates == nil {
		stats.Rates = map[string]ScoreRateStats{}
	}
	if !decision.ScoreSample {
		return stats
	}
	if internalFirstByteTimeoutRetry(result) {
		return stats
	}
	stats.Samples++
	completed := result.Success && !result.StreamInterrupted
	stats.Rates["completion"] = updateRateStats(stats.Rates["completion"], completed, true)
	upstreamError := scoreStatsCountsUpstreamError(result, decision)
	stats.Rates["upstream_error"] = updateRateStats(stats.Rates["upstream_error"], !upstreamError, true)
	stats.Rates["empty_output"] = updateRateStats(stats.Rates["empty_output"], !result.EmptyOutput, result.Success)
	stats.Rates["stream_interrupted"] = updateRateStats(stats.Rates["stream_interrupted"], !result.StreamInterrupted, true)
	if result.Success && result.TTFT > 0 {
		stats.Latency.TTFTMs = appendLimitedFloat(stats.Latency.TTFTMs, float64(result.TTFT.Milliseconds()), scoreStatsLatencyLimit)
	}
	if completed && result.Duration > 0 {
		stats.Latency.DurationMs = appendLimitedFloat(stats.Latency.DurationMs, float64(result.Duration.Milliseconds()), scoreStatsLatencyLimit)
	}
	if tps := tokensPerSecondFromAttempt(result); tps > 0 {
		stats.Latency.TokensPerSecond = appendLimitedFloat(stats.Latency.TokensPerSecond, tps, scoreStatsLatencyLimit)
	}
	return stats
}

func scoreStatsCountsUpstreamError(result core.AttemptResult, decision core.ScoreSampleDecision) bool {
	switch decision.Reason {
	case "upstream_error":
		return true
	case "empty_output", "stream_interrupted", "health_probe":
		return false
	}
	return !result.Success && !result.EmptyOutput && !result.StreamInterrupted && scoreStatsUpstreamErrorCategory(result.ErrorCategory)
}

func scoreStatsUpstreamErrorCategory(category string) bool {
	switch strings.TrimSpace(category) {
	case core.ErrorCategoryUpstreamError,
		core.ErrorCategoryServerError,
		core.ErrorCategoryTimeout,
		core.ErrorCategoryRateLimit,
		core.ErrorCategoryUpstreamConcurrencyLimit:
		return true
	default:
		return false
	}
}

func updateRateStats(stats ScoreRateStats, success bool, include bool) ScoreRateStats {
	if !include {
		return stats
	}
	stats.Total++
	if success {
		stats.Success++
	} else {
		stats.Count++
	}
	next := 0.0
	if success {
		next = 1
	}
	stats.EWMA = ewma(stats.EWMA, next)
	return stats
}

func appendLimitedFloat(values []float64, value float64, limit int) []float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return values
	}
	if limit <= 0 {
		limit = scoreStatsLatencyLimit
	}
	values = append(values, value)
	if len(values) <= limit {
		return values
	}
	return append([]float64(nil), values[len(values)-limit:]...)
}

func tokensPerSecondFromAttempt(result core.AttemptResult) float64 {
	if !result.Success || result.Duration <= 0 {
		return 0
	}
	// AttemptResult does not carry token counts today. Keep the ring buffer field
	// for schema stability and fill it when a future caller starts providing TPS.
	return 0
}

func mergeRuntimeSnapshotScoreStats(leftRaw string, rightRaw string) string {
	left := ScoreStats{}
	right := ScoreStats{}
	if strings.TrimSpace(leftRaw) != "" {
		_ = common.UnmarshalJsonStr(leftRaw, &left)
	}
	if strings.TrimSpace(rightRaw) != "" {
		_ = common.UnmarshalJsonStr(rightRaw, &right)
	}
	merged := mergeScoreStats(left, right)
	if merged.Samples <= 0 && len(merged.Rates) == 0 &&
		len(merged.Latency.TTFTMs) == 0 &&
		len(merged.Latency.DurationMs) == 0 &&
		len(merged.Latency.TokensPerSecond) == 0 {
		return ""
	}
	return merged.Marshal()
}

func mergeScoreStats(left ScoreStats, right ScoreStats) ScoreStats {
	out := ScoreStats{Version: scoreStatsVersion, Rates: map[string]ScoreRateStats{}}
	out.Samples = left.Samples + right.Samples
	for _, source := range []map[string]ScoreRateStats{left.Rates, right.Rates} {
		for key, value := range source {
			current := out.Rates[key]
			current.Success += value.Success
			current.Count += value.Count
			current.Total += value.Total
			current.EWMA = weightedEWMA(current.EWMA, current.Total-value.Total, value.EWMA, value.Total)
			out.Rates[key] = current
		}
	}
	out.Latency.TTFTMs = appendLimitedFloatSlice(left.Latency.TTFTMs, right.Latency.TTFTMs, scoreStatsLatencyLimit)
	out.Latency.DurationMs = appendLimitedFloatSlice(left.Latency.DurationMs, right.Latency.DurationMs, scoreStatsLatencyLimit)
	out.Latency.TokensPerSecond = appendLimitedFloatSlice(left.Latency.TokensPerSecond, right.Latency.TokensPerSecond, scoreStatsLatencyLimit)
	return out
}

func weightedEWMA(left float64, leftCount int, right float64, rightCount int) float64 {
	if leftCount <= 0 {
		return right
	}
	if rightCount <= 0 {
		return left
	}
	return (left*float64(leftCount) + right*float64(rightCount)) / float64(leftCount+rightCount)
}

func appendLimitedFloatSlice(left []float64, right []float64, limit int) []float64 {
	values := make([]float64, 0, len(left)+len(right))
	for _, value := range left {
		values = appendLimitedFloat(values, value, limit)
	}
	for _, value := range right {
		values = appendLimitedFloat(values, value, limit)
	}
	return values
}

func scoreSampleDecision(result core.AttemptResult) core.ScoreSampleDecision {
	decision := core.ScoreSampleDecision{
		ScoreSample:         true,
		RealUserMetric:      !result.IsHealthProbe,
		DynamicPriceSample:  !result.IsHealthProbe,
		CircuitSample:       true,
		ProbeRecoverySample: result.IsHealthProbe,
		Reason:              "attempt_result",
	}
	switch {
	case result.ClientAborted:
		return skippedScoreSample("client_aborted")
	case result.BalanceInsufficient || isBalanceInsufficientAttempt(result):
		return skippedScoreSample("balance_insufficient")
	case result.ConcurrencyLimited:
		return skippedScoreSample("concurrency_limited")
	case isCircuitOverloadSkipResult(result):
		return skippedScoreSample("circuit_overload_skip")
	case internalFirstByteTimeoutRetry(result):
		return skippedScoreSample(core.RelayAttemptCancelReasonFirstByteTimeout)
	case result.StreamInterrupted:
		decision.Reason = "stream_interrupted"
	case result.EmptyOutput:
		decision.Reason = "empty_output"
	case !result.Success:
		decision.Reason = "upstream_error"
	case result.IsHealthProbe:
		decision.Reason = "health_probe"
	}
	return decision
}

func skippedScoreSample(reason string) core.ScoreSampleDecision {
	return core.ScoreSampleDecision{SkipReason: reason, Reason: reason}
}

func internalFirstByteTimeoutRetry(result core.AttemptResult) bool {
	return strings.TrimSpace(result.RetryReason) == core.RelayAttemptCancelReasonFirstByteTimeout
}

func newScoreTraceID(result core.AttemptResult, observedAt time.Time) string {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	return common.GetUUID()
}
