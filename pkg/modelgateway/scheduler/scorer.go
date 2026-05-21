package scheduler

import (
	"math"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	breakdownSuccess     = "success"
	breakdownSpeed       = "speed"
	breakdownLoad        = "load"
	breakdownCost        = "cost"
	breakdownGroup       = "group"
	breakdownTTFTPenalty = "ttft_penalty"
)

const (
	ttftPenaltyWarningMs  = 8000
	ttftPenaltySlowMs     = 12000
	ttftPenaltyPoorMs     = 20000
	ttftPenaltyCriticalMs = 30000
	ttftPenaltySevereMs   = 60000
	ttftPenaltyExtremeMs  = 120000
)

type WeightedScoreCalculator struct {
	weights core.ScoreWeights
}

func NewWeightedScoreCalculator(weights core.ScoreWeights) *WeightedScoreCalculator {
	return &WeightedScoreCalculator{weights: normalizeWeights(weights)}
}

func (c *WeightedScoreCalculator) Score(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.ScoreResult {
	successScore := successScore(snapshot)
	firstBytePenalty := ttftPenaltyScore(snapshot)
	speedScore := speedScore(snapshot)
	loadScore := loadScore(snapshot)
	costScore := costScore(snapshot)
	groupScore := groupScore(snapshot)
	experienceScore := experienceScore(snapshot)
	total := successScore*c.weights.Success +
		speedScore*c.weights.Speed +
		loadScore*c.weights.Load +
		costScore*c.weights.Cost +
		groupScore*c.weights.Group
	total = applyExperienceGate(total, experienceScore)
	total = applyTTFTPenaltyGate(total, firstBytePenalty)
	total = applyLatencyGate(total, snapshot)
	breakdown := map[string]float64{
		breakdownSuccess:     round4(successScore),
		breakdownSpeed:       round4(speedScore),
		breakdownLoad:        round4(loadScore),
		breakdownCost:        round4(costScore),
		breakdownGroup:       round4(groupScore),
		"experience":         round4(experienceScore),
		"confidence_samples": round4(confidenceScore(snapshot.SampleCount)),
	}
	if firstBytePenalty > 0 {
		breakdown[breakdownTTFTPenalty] = round4(firstBytePenalty)
	}
	return core.ScoreResult{
		Total:     clamp01(total),
		Breakdown: breakdown,
		Reason:    "weighted_score",
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
		weights = core.ScoreWeights{Success: 0.30, Speed: 0.45, Load: 0.12, Cost: 0.08, Group: 0.05}
	case core.StrategyCostFirst:
		weights = core.ScoreWeights{Success: 0.30, Speed: 0.27, Load: 0.12, Cost: 0.26, Group: 0.05}
	case core.StrategyStabilityFirst:
		weights = core.ScoreWeights{Success: 0.46, Speed: 0.28, Load: 0.11, Cost: 0.10, Group: 0.05}
	}
	return NewWeightedScoreCalculator(weights)
}

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

func successScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.SuccessScore > 0 {
		return clamp01(snapshot.SuccessScore)
	}
	if snapshot.SampleCount == 0 && snapshot.SuccessRate == 0 {
		return 0.55
	}
	if snapshot.SampleCount == 0 {
		return clamp01(snapshot.SuccessRate)
	}
	return clamp01(snapshot.SuccessRate)
}

func speedScore(snapshot core.RuntimeSnapshot) float64 {
	score := 0.0
	if snapshot.SpeedScore > 0 {
		score = clamp01(snapshot.SpeedScore)
	} else if snapshot.TTFTMs > 0 {
		ttft := inverseLatencyScore(snapshot.TTFTMs, 800, 20000)
		tps := throughputScore(snapshot.TokensPerSecond, 5, 80)
		if snapshot.TokensPerSecond > 0 {
			score = clamp01(ttft*0.82 + tps*0.18)
		} else {
			score = ttft
		}
	} else if snapshot.DurationMs > 0 {
		score = inverseLatencyScore(snapshot.DurationMs, 3000, 90000)
	} else if snapshot.SampleCount == 0 {
		score = 0.45
	} else {
		score = 0.35
	}
	return applyTTFTPenaltyToSpeed(score, ttftPenaltyScore(snapshot))
}

func loadScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CircuitOpen || snapshot.Cooldown || snapshot.FailureAvoidance {
		return 0
	}
	limit := snapshot.EffectiveConcurrencyLimit
	if limit <= 0 {
		limit = snapshot.MaxConcurrency
	}
	if limit <= 0 {
		return 0.72
	}
	loadRatio := float64(snapshot.ActiveConcurrency) / float64(limit)
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
	return clamp01(1 / (1 + snapshot.CostRatio))
}

func groupScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.GroupPriorityRatio <= 0 {
		return 0.50
	}
	return clamp01(snapshot.GroupPriorityRatio)
}

func experienceScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.ExperienceScore > 0 {
		return clamp01(snapshot.ExperienceScore)
	}
	score := 1.0
	if snapshot.EmptyOutputRate > 0 {
		score -= math.Min(0.85, snapshot.EmptyOutputRate*0.85)
	}
	if snapshot.ExperienceIssueRate > 0 {
		score -= math.Min(0.65, snapshot.ExperienceIssueRate*0.65)
	}
	return clamp01(score)
}

func applyExperienceGate(total float64, experience float64) float64 {
	return total * (0.55 + 0.45*clamp01(experience))
}

func ttftPenaltyScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.SampleCount <= 0 || snapshot.TTFTMs <= ttftPenaltyWarningMs {
		return 0
	}
	switch {
	case snapshot.TTFTMs >= ttftPenaltyExtremeMs:
		return 0.94
	case snapshot.TTFTMs >= ttftPenaltySevereMs:
		return 0.86
	case snapshot.TTFTMs >= ttftPenaltyCriticalMs:
		return 0.78
	case snapshot.TTFTMs >= ttftPenaltyPoorMs:
		return 0.55
	case snapshot.TTFTMs >= ttftPenaltySlowMs:
		return 0.32
	default:
		return 0.18
	}
}

func applyTTFTPenaltyToSpeed(score float64, penalty float64) float64 {
	score = clamp01(score)
	if penalty <= 0 {
		return score
	}
	capValue := clamp01(1 - penalty)
	weighted := score * (1 - penalty*0.65)
	return math.Min(weighted, capValue)
}

func applyTTFTPenaltyGate(total float64, penalty float64) float64 {
	if penalty <= 0 {
		return total
	}
	gated := total * (1 - penalty*0.45)
	capValue := clamp01(1 - penalty*0.72)
	return math.Min(gated, capValue)
}

func applyLatencyGate(total float64, snapshot core.RuntimeSnapshot) float64 {
	if snapshot.SampleCount <= 0 {
		return total * 0.92
	}
	switch {
	case snapshot.TTFTMs >= 60000:
		return math.Min(total, 0.20)
	case snapshot.TTFTMs >= 45000:
		return math.Min(total, 0.26)
	case snapshot.TTFTMs >= 30000:
		return math.Min(total, 0.32)
	case snapshot.TTFTMs >= 20000:
		return math.Min(total, 0.40)
	case snapshot.TTFTMs >= 12000:
		return math.Min(total, 0.52)
	case snapshot.TTFTMs >= 8000:
		return math.Min(total, 0.65)
	case snapshot.DurationMs >= 120000:
		return math.Min(total, 0.38)
	case snapshot.DurationMs >= 90000:
		return math.Min(total, 0.45)
	case snapshot.DurationMs >= 60000:
		return math.Min(total, 0.55)
	default:
		return total
	}
}

func confidenceScore(sampleCount int) float64 {
	if sampleCount <= 0 {
		return 0
	}
	if sampleCount >= 20 {
		return 1
	}
	return float64(sampleCount) / 20
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
