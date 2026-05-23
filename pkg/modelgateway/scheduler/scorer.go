package scheduler

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	breakdownSuccess       = "success"
	breakdownSpeed         = "speed"
	breakdownLoad          = "load"
	breakdownCost          = "cost"
	breakdownGroup         = "group"
	breakdownExplore       = "explore_baseline"
	breakdownTTFTPenalty   = "ttft_penalty"
	breakdownTTFTPending   = "ttft_pending"
	breakdownRoutePressure = "routing_pressure"
)

const (
	ttftPenaltyWarningMs  = 8000
	ttftPenaltySlowMs     = 12000
	ttftPenaltyPoorMs     = 20000
	ttftPenaltyCriticalMs = 30000
	ttftPenaltySevereMs   = 60000
	ttftPenaltyExtremeMs  = 120000
)

const costFirstRelativeCostPower = 1.35
const missingCostReferenceScore = 0.50

type WeightedScoreCalculator struct {
	weights  core.ScoreWeights
	strategy string
}

func NewWeightedScoreCalculator(weights core.ScoreWeights) *WeightedScoreCalculator {
	return &WeightedScoreCalculator{weights: normalizeWeights(weights)}
}

func newWeightedScoreCalculatorWithStrategy(weights core.ScoreWeights, strategy string) *WeightedScoreCalculator {
	return &WeightedScoreCalculator{weights: normalizeWeights(weights), strategy: strategy}
}

func (c *WeightedScoreCalculator) Score(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.ScoreResult {
	strategy := c.strategy
	if policy.Strategy != "" {
		strategy = policy.Strategy
	}
	successScore := successScore(snapshot)
	firstBytePenalty := ttftPenaltyScore(snapshot)
	firstBytePendingPenalty := firstBytePendingPenaltyScoreForStrategy(snapshot, strategy)
	speedScore := reliabilityAdjustedSpeedScore(speedScore(snapshot), successScore, snapshot.SampleCount)
	healthLoadScore := healthLoadScore(snapshot)
	routingLoadScore := routingLoadScoreForStrategy(snapshot, strategy)
	costScore := costScoreForStrategy(snapshot, strategy)
	groupScore := groupScoreForStrategy(snapshot, policy, strategy)
	experienceScore := experienceScore(snapshot)
	total := successScore*c.weights.Success +
		speedScore*c.weights.Speed +
		healthLoadScore*c.weights.Load +
		costScore*c.weights.Cost +
		groupScore*c.weights.Group
	total = applyTTFTPenaltyGate(total, firstBytePenalty)
	total = applyLatencyGate(total, snapshot)
	routingTotal := successScore*c.weights.Success +
		speedScore*c.weights.Speed +
		routingLoadScore*c.weights.Load +
		costScore*c.weights.Cost +
		groupScore*c.weights.Group
	routingTotal = applyTTFTPenaltyGate(routingTotal, firstBytePenalty)
	routingTotal = applyFirstBytePendingGate(routingTotal, firstBytePendingPenalty)
	routingTotal = applyLatencyGate(routingTotal, snapshot)
	routePressurePenalty := routePressurePenaltyScoreForStrategy(snapshot, strategy)
	routingTotal = applyRoutePressureGate(routingTotal, routePressurePenalty)
	breakdown := map[string]float64{
		breakdownLoad:        round4(healthLoadScore),
		breakdownCost:        round4(costScore),
		breakdownGroup:       round4(groupScore),
		"experience":         round4(experienceScore),
		"confidence_samples": round4(confidenceScore(snapshot.SampleCount)),
	}
	if snapshot.SampleCount > 0 {
		breakdown[breakdownSuccess] = round4(successScore)
		breakdown[breakdownSpeed] = round4(speedScore)
	} else {
		breakdown[breakdownExplore] = 1
	}
	if firstBytePenalty > 0 {
		breakdown[breakdownTTFTPenalty] = round4(firstBytePenalty)
	}
	routingBreakdown := copyScoreBreakdown(breakdown)
	routingBreakdown[breakdownLoad] = round4(routingLoadScore)
	if firstBytePendingPenalty > 0 {
		routingBreakdown[breakdownTTFTPending] = round4(firstBytePendingPenalty)
	}
	if routePressurePenalty > 0 {
		routingBreakdown[breakdownRoutePressure] = round4(routePressurePenalty)
	}
	return core.ScoreResult{
		Total:            clamp01(total),
		Breakdown:        breakdown,
		RoutingTotal:     clamp01(routingTotal),
		RoutingBreakdown: routingBreakdown,
		Reason:           "weighted_score",
	}
}

func copyScoreBreakdown(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

type ScoreCalculatorFactory struct {
	defaultWeights    core.ScoreWeights
	rawDefaultWeights core.ScoreWeights
}

func NewScoreCalculatorFactory(defaultWeights core.ScoreWeights) *ScoreCalculatorFactory {
	return &ScoreCalculatorFactory{
		defaultWeights:    normalizeWeights(defaultWeights),
		rawDefaultWeights: defaultWeights,
	}
}

func (f *ScoreCalculatorFactory) ForStrategy(strategy string) core.ScoreCalculator {
	weights := f.defaultWeights
	switch strategy {
	case core.StrategySpeedFirst:
		weights = core.ScoreWeights{Success: 0.30, Speed: 0.45, Load: 0.12, Cost: 0.08, Group: 0.05}
	case core.StrategyCostFirst:
		weights = f.costFirstWeights()
	case core.StrategyStabilityFirst:
		weights = core.ScoreWeights{Success: 0.46, Speed: 0.28, Load: 0.11, Cost: 0.10, Group: 0.05}
	}
	return newWeightedScoreCalculatorWithStrategy(weights, strategy)
}

func (f *ScoreCalculatorFactory) costFirstWeights() core.ScoreWeights {
	weights := core.ScoreWeights{Success: 0.24, Speed: 0.07, Load: 0.03, Cost: 0.51, Group: 0.15}
	raw := f.rawDefaultWeights
	if raw.Cost > weights.Cost {
		weights.Cost = raw.Cost
	}
	if raw.Group > weights.Group {
		weights.Group = raw.Group
	}
	return weights
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
	if snapshot.SampleCount <= 0 && snapshot.SuccessRate == 0 {
		if snapshot.SuccessScore > 0 {
			return clamp01(snapshot.SuccessScore)
		}
		return 0.55
	}
	longTerm := clamp01(snapshot.SuccessRate)
	if snapshot.SampleCount <= 0 {
		if snapshot.SuccessScore > 0 {
			return clamp01(snapshot.SuccessScore)
		}
		return longTerm
	}
	if snapshot.SuccessScore > 0 {
		return confidenceAdjustedSuccessScore(longTerm, clamp01(snapshot.SuccessScore), snapshot.SampleCount)
	}
	return longTerm
}

func confidenceAdjustedSuccessScore(longTerm float64, recent float64, sampleCount int) float64 {
	longTerm = clamp01(longTerm)
	recent = clamp01(recent)
	if sampleCount <= 0 {
		return recent
	}
	confidence := confidenceScore(sampleCount)
	if recent <= longTerm {
		return recent
	}
	recoveryAllowance := 0.10 + (1-confidence)*0.20
	cappedRecent := math.Min(recent, longTerm+recoveryAllowance)
	historyWeight := 0.70*confidence + 0.35*(1-confidence)
	return clamp01(longTerm*historyWeight + cappedRecent*(1-historyWeight))
}

func speedScore(snapshot core.RuntimeSnapshot) float64 {
	score := rawSpeedScore(snapshot)
	if score <= 0 {
		if snapshot.SampleCount <= 0 {
			score = 0.45
		} else {
			score = 0.35
		}
	}
	return applyTTFTPenaltyToSpeed(score, ttftPenaltyScore(snapshot))
}

func rawSpeedScore(snapshot core.RuntimeSnapshot) float64 {
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
	}
	return clamp01(score)
}

func reliabilityAdjustedSpeedScore(speed float64, success float64, sampleCount int) float64 {
	speed = clamp01(speed)
	if sampleCount <= 0 {
		return speed
	}
	success = clamp01(success)
	if success >= 0.92 {
		return speed
	}
	capValue := 0.08 + success*0.92
	multiplier := 0.25 + success*0.75
	return math.Min(speed*multiplier, capValue)
}

func healthLoadScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CircuitOpen || snapshot.FailureAvoidance {
		return 0
	}
	return 1
}

func routingLoadScore(snapshot core.RuntimeSnapshot) float64 {
	return routingLoadScoreForStrategy(snapshot, "")
}

func routingLoadScoreForStrategy(snapshot core.RuntimeSnapshot, strategy string) float64 {
	if snapshot.CircuitOpen || snapshot.Cooldown || snapshot.FailureAvoidance {
		return 0
	}
	limit := routingConcurrencyLimit(snapshot)
	if limit <= 0 {
		if strategy == core.StrategyCostFirst {
			return routingLoadScoreWithoutKnownLimitForCostFirst(snapshot)
		}
		return routingLoadScoreWithoutKnownLimit(snapshot)
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

func softLoadPenalty(ratio float64) float64 {
	return softLoadPenaltyForStrategy(ratio, "")
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

func routingLoadScoreWithoutKnownLimit(snapshot core.RuntimeSnapshot) float64 {
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

func routingLoadScoreWithoutKnownLimitForCostFirst(snapshot core.RuntimeSnapshot) float64 {
	score := 1.0
	if snapshot.QueueDepth > 0 {
		score -= math.Min(float64(snapshot.QueueDepth)*0.03, 0.18)
	}
	if snapshot.EstimatedQueueWaitMs > 0 {
		score -= math.Min(snapshot.EstimatedQueueWaitMs/18000, 0.18)
	}
	return clamp01(score)
}

func routePressurePenaltyScore(snapshot core.RuntimeSnapshot) float64 {
	return routePressurePenaltyScoreForStrategy(snapshot, "")
}

func routePressurePenaltyScoreForStrategy(snapshot core.RuntimeSnapshot, strategy string) float64 {
	if snapshot.ActiveConcurrency <= 0 {
		return 0
	}
	limit := routingConcurrencyLimit(snapshot)
	if limit > 0 {
		ratio := float64(snapshot.ActiveConcurrency) / float64(limit)
		if strategy == core.StrategyCostFirst {
			if ratio <= 1 {
				return 0
			}
			return clamp01(math.Min((ratio-1)*0.35, 0.36))
		}
		if ratio <= 0.85 {
			return 0
		}
		return clamp01(math.Min((ratio-0.85)*1.2, 0.42))
	}
	return 0
}

func applyRoutePressureGate(total float64, penalty float64) float64 {
	if penalty <= 0 {
		return total
	}
	gated := total * (1 - penalty*0.58)
	capValue := clamp01(1 - penalty*0.72)
	return math.Min(gated, capValue)
}

func costScore(snapshot core.RuntimeSnapshot) float64 {
	return costScoreForStrategy(snapshot, "")
}

func costScoreForStrategy(snapshot core.RuntimeSnapshot, strategy string) float64 {
	if snapshot.CostRatio <= 0 || snapshot.CostReferenceRatio <= 0 {
		return missingCostReferenceScore
	}
	score := clamp01(snapshot.CostReferenceRatio / snapshot.CostRatio)
	if strategy == core.StrategyCostFirst {
		return clamp01(math.Pow(score, costFirstRelativeCostPower))
	}
	return score
}

func groupScore(snapshot core.RuntimeSnapshot) float64 {
	return groupScoreForStrategy(snapshot, core.GroupSmartPolicy{}, "")
}

func groupScoreForStrategy(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, strategy string) float64 {
	if snapshot.GroupPriorityRatio <= 0 {
		return 0.50
	}
	if strategy == core.StrategyCostFirst {
		return groupCostScore(snapshot, policy)
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

func groupCostScore(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) float64 {
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

func experienceScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.ExperienceScore > 0 {
		return clamp01(snapshot.ExperienceScore)
	}
	return clamp01(1 - clamp01(snapshot.EmptyOutputRate)*0.85 - clamp01(snapshot.ExperienceIssueRate)*0.65)
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

func firstBytePendingPenaltyScore(snapshot core.RuntimeSnapshot) float64 {
	return firstBytePendingPenaltyScoreForStrategy(snapshot, "")
}

func firstBytePendingPenaltyScoreForStrategy(snapshot core.RuntimeSnapshot, strategy string) float64 {
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

func applyFirstBytePendingGate(total float64, penalty float64) float64 {
	if penalty <= 0 {
		return total
	}
	gated := total * (1 - penalty*0.42)
	capValue := clamp01(1 - penalty*0.62)
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
