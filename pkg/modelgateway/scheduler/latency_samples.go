package scheduler

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	maxRuntimeLatencySamples     = 120
	runtimeLatencyTrimRatio      = 0.10
	runtimeLatencyTrimMinSamples = 10
	runtimeLatencySmallTrimMin   = 3
)

func runtimeLatencySampleFromAttempt(result core.AttemptResult, observedAt time.Time) (core.RuntimeLatencySample, bool) {
	sample := core.RuntimeLatencySample{}
	if !observedAt.IsZero() {
		sample.ObservedAt = observedAt.Unix()
	}
	if result.Duration > 0 {
		sample.DurationMs = float64(result.Duration.Milliseconds())
	}
	if result.TTFT > 0 {
		sample.TTFTMs = float64(result.TTFT.Milliseconds())
	}
	return sample, runtimeLatencySampleValid(sample)
}

func appendRuntimeLatencySample(samples []core.RuntimeLatencySample, sample core.RuntimeLatencySample) []core.RuntimeLatencySample {
	if !runtimeLatencySampleValid(sample) {
		return normalizeRuntimeLatencySamples(samples)
	}
	samples = append(samples, sample)
	return normalizeRuntimeLatencySamples(samples)
}

func mergeRuntimeLatencySamples(left, right []core.RuntimeLatencySample) []core.RuntimeLatencySample {
	if len(left) == 0 {
		return normalizeRuntimeLatencySamples(right)
	}
	if len(right) == 0 {
		return normalizeRuntimeLatencySamples(left)
	}
	merged := make([]core.RuntimeLatencySample, 0, len(left)+len(right))
	merged = append(merged, left...)
	merged = append(merged, right...)
	return normalizeRuntimeLatencySamples(merged)
}

func normalizeRuntimeLatencySamples(samples []core.RuntimeLatencySample) []core.RuntimeLatencySample {
	if len(samples) == 0 {
		return nil
	}
	out := make([]core.RuntimeLatencySample, 0, len(samples))
	for _, sample := range samples {
		if !runtimeLatencySampleValid(sample) {
			continue
		}
		out = append(out, sample)
	}
	if len(out) == 0 {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].ObservedAt
		right := out[j].ObservedAt
		if left == right {
			return i < j
		}
		if left == 0 {
			return true
		}
		if right == 0 {
			return false
		}
		return left < right
	})
	if len(out) > maxRuntimeLatencySamples {
		out = append([]core.RuntimeLatencySample(nil), out[len(out)-maxRuntimeLatencySamples:]...)
	}
	return out
}

func runtimeLatencySampleValid(sample core.RuntimeLatencySample) bool {
	return finitePositive(sample.DurationMs) || finitePositive(sample.TTFTMs)
}

func runtimeLatencyStats(samples []core.RuntimeLatencySample) (durationMs float64, ttftMs float64, latencyScore float64) {
	samples = normalizeRuntimeLatencySamples(samples)
	if len(samples) == 0 {
		return 0, 0, 0
	}
	durations := make([]float64, 0, len(samples))
	ttfts := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if finitePositive(sample.DurationMs) {
			durations = append(durations, sample.DurationMs)
		}
		if finitePositive(sample.TTFTMs) {
			ttfts = append(ttfts, sample.TTFTMs)
		}
	}
	durationMs = trimmedMeanFloat64(durations)
	ttftMs = trimmedMeanFloat64(ttfts)
	switch {
	case ttftMs > 0:
		latencyScore = progressiveTTFTLatencyScore(ttftMs, 800, 20000)
	case durationMs > 0:
		latencyScore = inverseLatencyScore(durationMs, 3000, 90000)
	default:
		latencyScore = 0
	}
	return durationMs, ttftMs, latencyScore
}

func trimmedMeanFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	clean := make([]float64, 0, len(values))
	for _, value := range values {
		if finitePositive(value) {
			clean = append(clean, value)
		}
	}
	if len(clean) == 0 {
		return 0
	}
	sort.Float64s(clean)
	if len(clean) >= runtimeLatencyTrimMinSamples {
		trim := int(math.Floor(float64(len(clean)) * runtimeLatencyTrimRatio))
		if trim < 1 {
			trim = 1
		}
		if len(clean)-trim*2 >= 1 {
			clean = clean[trim : len(clean)-trim]
		}
	} else if len(clean) >= runtimeLatencySmallTrimMin {
		clean = clean[1 : len(clean)-1]
	}
	sum := 0.0
	for _, value := range clean {
		sum += value
	}
	return sum / float64(len(clean))
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func runtimeLatencySamplesDebugValue(samples []core.RuntimeLatencySample) string {
	if len(samples) == 0 {
		return ""
	}
	durationMs, ttftMs, latencyScore := runtimeLatencyStats(samples)
	return fmt.Sprintf("samples=%d duration=%.0f ttft=%.0f latency=%.4f", len(samples), durationMs, ttftMs, latencyScore)
}
