package scheduler

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	costScoreModeRelative          = "relative"
	costScoreModeLogMultiple       = "log_multiple"
	costFirstLogCostZeroMultiple   = 5.0
	costFirstGuardMultiple         = 1.8
	costFirstGuardSuccessAdvantage = 0.03
	costFirstGuardSpeedAdvantage   = 0.08
	progressiveTTFTLatencyDecay    = 2.2
)

func DefaultScoreWeights() core.ScoreWeights {
	return core.ScoreWeights{Success: 0.35, Speed: 0.35, Load: 0.15, Cost: 0.08, Group: 0.07}
}

func normalizeWeights(weights core.ScoreWeights) core.ScoreWeights {
	if weights.Success == 0 && weights.Speed == 0 && weights.Load == 0 && weights.Cost == 0 && weights.Group == 0 {
		weights = DefaultScoreWeights()
	}
	sum := weights.Success + weights.Speed + weights.Load + weights.Cost + weights.Group
	if sum <= 0 {
		return DefaultScoreWeights()
	}
	return core.ScoreWeights{
		Success: weights.Success / sum,
		Speed:   weights.Speed / sum,
		Load:    weights.Load / sum,
		Cost:    weights.Cost / sum,
		Group:   weights.Group / sum,
	}
}

func stableConcurrencyLoadItemScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CircuitOpen || snapshot.FailureAvoidance {
		return 0
	}
	return 1
}

func routingConcurrencyLoadItemScoreForStrategy(snapshot core.RuntimeSnapshot, strategy string) float64 {
	if snapshot.CircuitOpen || snapshot.Cooldown || snapshot.FailureAvoidance {
		return 0
	}
	limit := routingConcurrencyLimit(snapshot)
	if limit <= 0 {
		if strategy == core.StrategyCostFirst {
			return routingConcurrencyLoadItemScoreWithoutKnownLimitForCostFirst(snapshot)
		}
		return routingConcurrencyLoadItemScoreWithoutKnownLimit(snapshot)
	}
	loadRatio := float64(snapshot.ActiveConcurrency) / float64(limit)
	loadPenalty := softLoadPenaltyForStrategy(loadRatio, strategy)
	if snapshot.FirstBytePending > 0 {
		loadPenalty += math.Min(float64(snapshot.FirstBytePending)*0.03, 0.12)
	}
	if snapshot.SlowFirstBytePending > 0 {
		loadPenalty += math.Min(float64(snapshot.SlowFirstBytePending)*0.10, 0.32)
	}
	queuePenalty := 0.0
	if snapshot.QueueDepth > 0 {
		queuePenalty = math.Min(float64(snapshot.QueueDepth)*0.025, 0.18)
	}
	waitPenalty := 0.0
	if snapshot.EstimatedQueueWaitMs > 0 {
		waitPenalty = math.Min(snapshot.EstimatedQueueWaitMs/20000, 0.18)
	}
	return clamp01(1 - loadPenalty - queuePenalty - waitPenalty)
}

func routingConcurrencyLimit(snapshot core.RuntimeSnapshot) int {
	if snapshot.EffectiveConcurrencyLimit > 0 {
		return snapshot.EffectiveConcurrencyLimit
	}
	if snapshot.LearnedConcurrencyLimit > 0 {
		return snapshot.LearnedConcurrencyLimit
	}
	if snapshot.MaxConcurrency > 0 {
		return snapshot.MaxConcurrency
	}
	return snapshot.ConfiguredConcurrencyLimit
}

func softLoadPenaltyForStrategy(ratio float64, strategy string) float64 {
	if strategy == core.StrategyCostFirst {
		if ratio < 0 {
			ratio = 0
		}
		switch {
		case ratio <= 0.80:
			return 0
		case ratio <= 1:
			return (ratio - 0.80) * 1.5
		default:
			return math.Min(0.30+(ratio-1)*0.22, 0.62)
		}
	}
	ratio = clamp01(ratio)
	switch {
	case ratio <= 0.30:
		return ratio * 0.08
	case ratio <= 0.70:
		return 0.024 + (ratio-0.30)*0.25
	default:
		return 0.124 + (ratio-0.70)*1.35
	}
}

func routingConcurrencyLoadItemScoreWithoutKnownLimit(snapshot core.RuntimeSnapshot) float64 {
	score := 1.0
	if snapshot.FirstBytePending > 0 {
		score -= math.Min(float64(snapshot.FirstBytePending)*0.04, 0.16)
	}
	if snapshot.SlowFirstBytePending > 0 {
		score -= math.Min(float64(snapshot.SlowFirstBytePending)*0.12, 0.34)
	}
	switch {
	case snapshot.OldestFirstByteWaitMs >= 30000:
		score -= 0.26
	case snapshot.OldestFirstByteWaitMs >= 20000:
		score -= 0.20
	case snapshot.OldestFirstByteWaitMs >= 12000:
		score -= 0.14
	case snapshot.OldestFirstByteWaitMs >= 8000:
		score -= 0.08
	}
	if snapshot.QueueDepth > 0 {
		score -= math.Min(float64(snapshot.QueueDepth)*0.05, 0.24)
	}
	if snapshot.EstimatedQueueWaitMs > 0 {
		score -= math.Min(snapshot.EstimatedQueueWaitMs/12000, 0.24)
	}
	return clamp01(score)
}

func routingConcurrencyLoadItemScoreWithoutKnownLimitForCostFirst(snapshot core.RuntimeSnapshot) float64 {
	score := 1.0
	if snapshot.QueueDepth > 0 {
		score -= math.Min(float64(snapshot.QueueDepth)*0.03, 0.18)
	}
	if snapshot.EstimatedQueueWaitMs > 0 {
		score -= math.Min(snapshot.EstimatedQueueWaitMs/18000, 0.18)
	}
	return clamp01(score)
}

func groupPriorityItemScoreForStrategy(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, strategy string) float64 {
	if snapshot.GroupPriorityRatio <= 0 {
		return 0.50
	}
	if strategy == core.StrategyCostFirst {
		return costFirstGroupPriorityItemScore(snapshot, policy)
	}
	maxRatio := 0.0
	for _, ratio := range policy.GroupPriorityRatio {
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}
	if maxRatio > 1 {
		return clamp01(snapshot.GroupPriorityRatio / maxRatio)
	}
	return clamp01(snapshot.GroupPriorityRatio)
}

func costFirstGroupPriorityItemScore(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) float64 {
	minRatio := 0.0
	candidateGroups := make(map[string]struct{}, len(policy.CandidateGroups)+1)
	for _, group := range policy.CandidateGroups {
		group = strings.TrimSpace(group)
		if group != "" {
			candidateGroups[group] = struct{}{}
		}
	}
	if snapshot.Key.Group != "" {
		candidateGroups[snapshot.Key.Group] = struct{}{}
	}
	for group, ratio := range policy.GroupPriorityRatio {
		if ratio <= 0 {
			continue
		}
		if len(candidateGroups) > 0 {
			if _, ok := candidateGroups[group]; !ok {
				continue
			}
		}
		if minRatio == 0 || ratio < minRatio {
			minRatio = ratio
		}
	}
	if minRatio > 0 {
		return clamp01(minRatio / snapshot.GroupPriorityRatio)
	}
	return clamp01(1 / (1 + snapshot.GroupPriorityRatio))
}

func firstByteBacklogPenaltyForStrategy(snapshot core.RuntimeSnapshot, strategy string) float64 {
	if strategy == core.StrategyCostFirst {
		return 0
	}
	if snapshot.FirstBytePending <= 0 {
		return 0
	}
	penalty := math.Min(float64(snapshot.FirstBytePending)*0.08, 0.24)
	if snapshot.SlowFirstBytePending > 0 {
		penalty += math.Min(float64(snapshot.SlowFirstBytePending)*0.24, 0.72)
	}
	switch {
	case snapshot.OldestFirstByteWaitMs >= 30000:
		penalty += 0.32
	case snapshot.OldestFirstByteWaitMs >= 20000:
		penalty += 0.24
	case snapshot.OldestFirstByteWaitMs >= 12000:
		penalty += 0.16
	case snapshot.OldestFirstByteWaitMs >= 8000:
		penalty += 0.08
	}
	return clamp01(penalty)
}

func inverseLatencyScore(value, good, poor float64) float64 {
	if value <= good {
		return 1
	}
	if value >= poor {
		return 0
	}
	return clamp01(1 - (value-good)/(poor-good))
}

func progressiveTTFTLatencyScore(value, good, poor float64) float64 {
	if value <= good {
		return 1
	}
	if value >= poor {
		return 0
	}
	if poor <= good || progressiveTTFTLatencyDecay <= 0 {
		return inverseLatencyScore(value, good, poor)
	}
	x := clamp01((value - good) / (poor - good))
	floor := math.Exp(-progressiveTTFTLatencyDecay)
	denominator := 1 - floor
	if denominator <= 0 {
		return inverseLatencyScore(value, good, poor)
	}
	return clamp01((math.Exp(-progressiveTTFTLatencyDecay*x) - floor) / denominator)
}

func throughputScore(value, low, high float64) float64 {
	if value <= 0 {
		return 0.50
	}
	if value <= low {
		return 0
	}
	if value >= high {
		return 1
	}
	return clamp01((value - low) / (high - low))
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
