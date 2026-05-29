package scheduler

import "github.com/QuantumNous/new-api/pkg/modelgateway/core"

const (
	scoreItemCompletionRate        = "completion_rate"
	scoreItemUpstreamErrorRate     = "upstream_error_rate"
	scoreItemTTFTLatency           = "ttft_latency"
	scoreItemDurationLatency       = "duration_latency"
	scoreItemThroughput            = "throughput"
	scoreItemEmptyOutputRate       = "empty_output_rate"
	scoreItemStreamInterruptedRate = "stream_interrupted_rate"
	scoreItemConcurrencyLoad       = "concurrency_load"
	scoreItemQueuePressure         = "queue_pressure"
	scoreItemFirstByteBacklog      = "first_byte_backlog"
	scoreItemCost                  = "cost"
	scoreItemGroupPriority         = "group_priority"
	scoreItemRetryIntentRecovery   = "retry_intent_recovery"
)

const (
	scoreCategorySample   = "sample"
	scoreCategoryPressure = "pressure"
	scoreCategoryFormula  = "formula"
)

const scoreItemTTFTLatencyMinWeight = 0.13

type StrategyProfile struct {
	Strategy      string
	Weights       map[string]float64
	CostPower     float64
	CostScoreMode string
	CostZeroPoint float64
	PressureRoute bool
}

func (p StrategyProfile) WithScoreWeights(weights core.ScoreWeights) StrategyProfile {
	if p.Strategy != core.StrategyBalanced {
		return p
	}
	normalized := normalizeWeights(weights)
	if normalized.Success <= 0 &&
		normalized.Speed <= 0 &&
		normalized.Load <= 0 &&
		normalized.Cost <= 0 &&
		normalized.Group <= 0 {
		return p
	}
	next := make(map[string]float64, len(p.Weights))
	for key, value := range p.Weights {
		next[key] = value
	}
	scaleScoreItemGroup(next, []string{
		scoreItemCompletionRate,
		scoreItemUpstreamErrorRate,
		scoreItemEmptyOutputRate,
		scoreItemStreamInterruptedRate,
	}, normalized.Success)
	scaleScoreItemGroup(next, []string{
		scoreItemTTFTLatency,
		scoreItemDurationLatency,
		scoreItemThroughput,
	}, normalized.Speed)
	scaleScoreItemGroup(next, []string{
		scoreItemConcurrencyLoad,
		scoreItemQueuePressure,
		scoreItemFirstByteBacklog,
	}, normalized.Load)
	scaleScoreItemGroup(next, []string{scoreItemCost}, normalized.Cost)
	scaleScoreItemGroup(next, []string{scoreItemGroupPriority}, normalized.Group)
	p.Weights = next
	return p
}

func scaleScoreItemGroup(weights map[string]float64, keys []string, target float64) {
	if len(weights) == 0 || len(keys) == 0 || target < 0 {
		return
	}
	current := 0.0
	for _, key := range keys {
		current += weights[key]
	}
	if current <= 0 {
		share := target / float64(len(keys))
		for _, key := range keys {
			weights[key] = share
		}
		return
	}
	for _, key := range keys {
		weights[key] = weights[key] / current * target
	}
}

func StrategyProfileFor(strategy string) StrategyProfile {
	profile := StrategyProfile{
		Strategy:      strategy,
		Weights:       defaultScoreItemWeights(),
		CostPower:     1,
		CostScoreMode: costScoreModeRelative,
		CostZeroPoint: costFirstLogCostZeroMultiple,
		PressureRoute: true,
	}
	switch strategy {
	case core.StrategySpeedFirst:
		profile.Weights = map[string]float64{
			scoreItemCompletionRate:        0.16,
			scoreItemUpstreamErrorRate:     0.10,
			scoreItemTTFTLatency:           0.20,
			scoreItemDurationLatency:       0.14,
			scoreItemThroughput:            0.12,
			scoreItemEmptyOutputRate:       0.05,
			scoreItemStreamInterruptedRate: 0.06,
			scoreItemConcurrencyLoad:       0.08,
			scoreItemQueuePressure:         0.05,
			scoreItemFirstByteBacklog:      0.03,
			scoreItemCost:                  0.01,
			scoreItemGroupPriority:         0.00,
		}
	case core.StrategyCostFirst:
		profile.Weights = map[string]float64{
			scoreItemCompletionRate:        0.112,
			scoreItemUpstreamErrorRate:     0.08,
			scoreItemTTFTLatency:           0.12,
			scoreItemDurationLatency:       0.05,
			scoreItemThroughput:            0.03,
			scoreItemEmptyOutputRate:       0.04,
			scoreItemStreamInterruptedRate: 0.05,
			scoreItemConcurrencyLoad:       0.03,
			scoreItemQueuePressure:         0.03,
			scoreItemFirstByteBacklog:      0.064,
			scoreItemCost:                  0.344,
			scoreItemGroupPriority:         0.05,
		}
		profile.CostPower = 1
		profile.CostScoreMode = costScoreModeLogMultiple
	case core.StrategyStabilityFirst:
		profile.Weights = map[string]float64{
			scoreItemCompletionRate:        0.26,
			scoreItemUpstreamErrorRate:     0.18,
			scoreItemTTFTLatency:           0.10,
			scoreItemDurationLatency:       0.08,
			scoreItemThroughput:            0.05,
			scoreItemEmptyOutputRate:       0.08,
			scoreItemStreamInterruptedRate: 0.10,
			scoreItemConcurrencyLoad:       0.05,
			scoreItemQueuePressure:         0.04,
			scoreItemFirstByteBacklog:      0.03,
			scoreItemCost:                  0.02,
			scoreItemGroupPriority:         0.01,
		}
	default:
		profile.Strategy = core.StrategyBalanced
	}
	return profile
}

func defaultScoreItemWeights() map[string]float64 {
	return map[string]float64{
		scoreItemCompletionRate:        0.20,
		scoreItemUpstreamErrorRate:     0.12,
		scoreItemTTFTLatency:           0.14,
		scoreItemDurationLatency:       0.10,
		scoreItemThroughput:            0.08,
		scoreItemEmptyOutputRate:       0.06,
		scoreItemStreamInterruptedRate: 0.06,
		scoreItemConcurrencyLoad:       0.06,
		scoreItemQueuePressure:         0.05,
		scoreItemFirstByteBacklog:      0.04,
		scoreItemCost:                  0.07,
		scoreItemGroupPriority:         0.02,
	}
}
