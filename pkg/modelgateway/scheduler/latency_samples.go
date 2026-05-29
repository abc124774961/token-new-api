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
	ttftRecencyWeightWindow      = 64.0
	ttftRecencyHalfLifeSamples   = 16.0
	ttftStabilitySlowBaseMs      = 8000.0
	ttftStabilitySlowMultiple    = 4.0
	ttftStabilityMaxPenalty      = 0.35
)

type weightedLatencyValue struct {
	value  float64
	weight float64
}

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
	ttftMs, stabilityPenalty, _ := recencyWeightedTTFTLatency(samples)
	if ttftMs <= 0 {
		ttftMs = trimmedMeanFloat64(ttfts)
		stabilityPenalty = 1
	}
	switch {
	case ttftMs > 0:
		latencyScore = progressiveTTFTLatencyScore(ttftMs, 800, 20000) * stabilityPenalty
	case durationMs > 0:
		latencyScore = inverseLatencyScore(durationMs, 3000, 90000)
	default:
		latencyScore = 0
	}
	return durationMs, ttftMs, latencyScore
}

func recencyWeightedTTFTLatency(samples []core.RuntimeLatencySample) (ttftMs float64, stabilityPenalty float64, sampleCount int) {
	samples = normalizeRuntimeLatencySamples(samples)
	if len(samples) == 0 {
		return 0, 1, 0
	}
	ordered := make([]weightedLatencyValue, 0, len(samples))
	for _, sample := range samples {
		if finitePositive(sample.TTFTMs) {
			ordered = append(ordered, weightedLatencyValue{value: sample.TTFTMs})
		}
	}
	if len(ordered) == 0 {
		return 0, 1, 0
	}
	for idx := range ordered {
		age := float64(len(ordered) - 1 - idx)
		if len(ordered) >= runtimeLatencyTrimMinSamples && float64(len(ordered)) < ttftRecencyWeightWindow {
			age *= ttftRecencyWeightWindow / float64(len(ordered))
		}
		ordered[idx].weight = math.Pow(0.5, age/ttftRecencyHalfLifeSamples)
	}
	p50 := weightedPercentile(ordered, 0.50)
	if p50 <= 0 {
		return 0, 1, len(ordered)
	}
	return p50, ttftStabilityPenalty(ordered, p50), len(ordered)
}

func weightedPercentile(values []weightedLatencyValue, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	clean := make([]weightedLatencyValue, 0, len(values))
	totalWeight := 0.0
	for _, item := range values {
		if finitePositive(item.value) && finitePositive(item.weight) {
			clean = append(clean, item)
			totalWeight += item.weight
		}
	}
	if len(clean) == 0 || totalWeight <= 0 {
		return 0
	}
	sort.SliceStable(clean, func(i, j int) bool {
		return clean[i].value < clean[j].value
	})
	target := clamp01(percentile) * totalWeight
	if target <= 0 {
		return clean[0].value
	}
	running := 0.0
	for _, item := range clean {
		running += item.weight
		if running >= target {
			return item.value
		}
	}
	return clean[len(clean)-1].value
}

func ttftStabilityPenalty(values []weightedLatencyValue, representativeTTFT float64) float64 {
	if len(values) == 0 || representativeTTFT <= 0 {
		return 1
	}
	slowThreshold := math.Max(ttftStabilitySlowBaseMs, representativeTTFT*ttftStabilitySlowMultiple)
	totalWeight := 0.0
	slowWeight := 0.0
	for _, item := range values {
		if !finitePositive(item.value) || !finitePositive(item.weight) {
			continue
		}
		totalWeight += item.weight
		if item.value >= slowThreshold {
			slowWeight += item.weight
		}
	}
	if totalWeight <= 0 || slowWeight <= 0 {
		return 1
	}
	slowRatio := clamp01(slowWeight / totalWeight)
	return clamp01(1 - slowRatio*ttftStabilityMaxPenalty)
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
