package scheduler

import (
	"math"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	breakdownSuccess = "success"
	breakdownSpeed   = "speed"
	breakdownLoad    = "load"
	breakdownCost    = "cost"
	breakdownGroup   = "group"
)

type WeightedScoreCalculator struct {
	weights core.ScoreWeights
}

func NewWeightedScoreCalculator(weights core.ScoreWeights) *WeightedScoreCalculator {
	return &WeightedScoreCalculator{weights: normalizeWeights(weights)}
}

func (c *WeightedScoreCalculator) Score(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.ScoreResult {
	successScore := clamp01(snapshot.SuccessRate)
	if snapshot.SampleCount == 0 && successScore == 0 {
		successScore = 0.80
	}
	speedScore := speedScore(snapshot)
	loadScore := loadScore(snapshot)
	costScore := costScore(snapshot)
	groupScore := groupScore(snapshot)
	total := successScore*c.weights.Success +
		speedScore*c.weights.Speed +
		loadScore*c.weights.Load +
		costScore*c.weights.Cost +
		groupScore*c.weights.Group
	return core.ScoreResult{
		Total: clamp01(total),
		Breakdown: map[string]float64{
			breakdownSuccess: round4(successScore),
			breakdownSpeed:   round4(speedScore),
			breakdownLoad:    round4(loadScore),
			breakdownCost:    round4(costScore),
			breakdownGroup:   round4(groupScore),
		},
		Reason: "weighted_score",
	}
}

type ScoreCalculatorFactory struct {
	defaultWeights core.ScoreWeights
}

func NewScoreCalculatorFactory(defaultWeights core.ScoreWeights) *ScoreCalculatorFactory {
	return &ScoreCalculatorFactory{defaultWeights: normalizeWeights(defaultWeights)}
}

func (f *ScoreCalculatorFactory) ForStrategy(strategy string) core.ScoreCalculator {
	weights := f.defaultWeights
	switch strategy {
	case core.StrategySpeedFirst:
		weights = core.ScoreWeights{Success: 0.25, Speed: 0.40, Load: 0.18, Cost: 0.12, Group: 0.05}
	case core.StrategyCostFirst:
		weights = core.ScoreWeights{Success: 0.30, Speed: 0.20, Load: 0.15, Cost: 0.30, Group: 0.05}
	case core.StrategyStabilityFirst:
		weights = core.ScoreWeights{Success: 0.45, Speed: 0.20, Load: 0.18, Cost: 0.12, Group: 0.05}
	}
	return NewWeightedScoreCalculator(weights)
}

func DefaultScoreWeights() core.ScoreWeights {
	return core.ScoreWeights{Success: 0.32, Speed: 0.28, Load: 0.20, Cost: 0.15, Group: 0.05}
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

func speedScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.TTFTMs > 0 {
		ttft := inverseLatencyScore(snapshot.TTFTMs, 300, 6000)
		tps := throughputScore(snapshot.TokensPerSecond, 5, 80)
		if snapshot.TokensPerSecond > 0 {
			return clamp01(ttft*0.70 + tps*0.30)
		}
		return ttft
	}
	if snapshot.DurationMs > 0 {
		return inverseLatencyScore(snapshot.DurationMs, 1000, 60000)
	}
	return 0.70
}

func loadScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CircuitOpen || snapshot.Cooldown || snapshot.FailureAvoidance {
		return 0
	}
	if snapshot.MaxConcurrency <= 0 {
		return 0.80
	}
	loadRatio := float64(snapshot.ActiveConcurrency) / float64(snapshot.MaxConcurrency)
	queuePenalty := 0.0
	if snapshot.QueueDepth > 0 {
		queuePenalty = math.Min(float64(snapshot.QueueDepth)*0.05, 0.30)
	}
	waitPenalty := 0.0
	if snapshot.EstimatedQueueWaitMs > 0 {
		waitPenalty = math.Min(snapshot.EstimatedQueueWaitMs/10000, 0.30)
	}
	return clamp01(1 - loadRatio - queuePenalty - waitPenalty)
}

func costScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CostRatio <= 0 {
		return 0.70
	}
	return clamp01(1 / snapshot.CostRatio)
}

func groupScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.GroupPriorityRatio <= 0 {
		return 0.50
	}
	return clamp01(snapshot.GroupPriorityRatio)
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

var _ core.ScoreCalculator = (*WeightedScoreCalculator)(nil)
var _ core.ScoreCalculatorFactory = (*ScoreCalculatorFactory)(nil)
