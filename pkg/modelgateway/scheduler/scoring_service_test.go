package scheduler_test

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/stretchr/testify/require"
)

func TestCandidateScoringServiceSkipsMissingCostReference(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 7, Group: "default", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        0.98,
		TTFTMs:             450,
		DurationMs:         1800,
		TokensPerSecond:    60,
		CostRatio:          0.42,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	}

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "default"}, snapshot, core.GroupSmartPolicy{
		Strategy:        core.StrategyCostFirst,
		CandidateGroups: []string{"default"},
		GroupPriorityRatio: map[string]float64{
			"default": 1,
		},
	}, scheduler.ScoringContext{Strategy: core.StrategyCostFirst}, false)

	require.True(t, evaluation.Score.CostReferenceMissing)
	require.True(t, evaluation.Explanation.CostReferenceMissing)
	costItem := scoreItemByKey(t, evaluation.Score.Items, "cost")
	require.Equal(t, "cost_reference_missing", costItem.MissingReason)
	require.Zero(t, costItem.Weight)
	require.Zero(t, costItem.WeightedScore)
	require.NotContains(t, evaluation.Score.Breakdown, "cost")
	require.Greater(t, evaluation.Score.Total, 0.80)
}

func TestCandidateScoringServiceTotalComesFromFlatScoreItems(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 9, Group: "vip", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		TTFTMs:             600,
		DurationMs:         2200,
		TokensPerSecond:    72,
		ActiveConcurrency:  1,
		MaxConcurrency:     12,
		CostRatio:          0.10,
		CostReferenceRatio: 0.05,
		GroupPriorityRatio: 1,
		SampleCount:        32,
	}

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "vip"}, snapshot, core.GroupSmartPolicy{
		Strategy:        core.StrategyBalanced,
		CandidateGroups: []string{"vip"},
		GroupPriorityRatio: map[string]float64{
			"vip": 1,
		},
	}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)

	expected := 0.0
	for _, item := range evaluation.Score.Items {
		if item.MissingReason == "" {
			expected += item.Score * item.Weight
		}
	}
	require.InEpsilon(t, round4ForTest(expected), evaluation.Score.Total, 0.0001)
	require.Contains(t, evaluation.Score.Breakdown, "completion_rate")
	require.NotContains(t, evaluation.Score.Breakdown, "success")
	require.Equal(t, scoreItemByKey(t, evaluation.Score.Items, "completion_rate").Score, evaluation.Explanation.ScoreBreakdown["completion_rate"])
}

func TestCostFirstWeightsShiftRequestedSignalsToCost(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 12, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		TTFTMs:             6740,
		DurationMs:         18670,
		TokensPerSecond:    30,
		CostRatio:          0.12,
		CostReferenceRatio: 0.12,
		GroupPriorityRatio: 1,
		SampleCount:        40,
	}

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "codex-plus"}, snapshot, core.GroupSmartPolicy{
		Strategy:        core.StrategyCostFirst,
		CandidateGroups: []string{"codex-plus"},
		GroupPriorityRatio: map[string]float64{
			"codex-plus": 1,
		},
	}, scheduler.ScoringContext{Strategy: core.StrategyCostFirst}, false)

	costItem := scoreItemByKey(t, evaluation.Score.Items, "cost")
	completionItem := scoreItemByKey(t, evaluation.Score.Items, "completion_rate")
	upstreamErrorItem := scoreItemByKey(t, evaluation.Score.Items, "upstream_error_rate")
	ttftItem := scoreItemByKey(t, evaluation.Score.Items, "ttft_latency")
	firstByteBacklogItem := scoreItemByKey(t, evaluation.Score.Items, "first_byte_backlog")
	require.InEpsilon(t, 0.3401, costItem.Weight, 0.0001)
	require.InEpsilon(t, 0.1107, completionItem.Weight, 0.0001)
	require.InEpsilon(t, 0.0791, upstreamErrorItem.Weight, 0.0001)
	require.InEpsilon(t, 0.13, ttftItem.Weight, 0.0001)
	require.InEpsilon(t, 0.0633, firstByteBacklogItem.Weight, 0.0001)
	totalWeight := 0.0
	for _, item := range evaluation.Score.Items {
		if item.MissingReason == "" {
			totalWeight += item.Weight
		}
	}
	require.InDelta(t, 1.0, totalWeight, 0.0005)
}

func TestTTFTLatencyKeepsMinimumWeightWithRetryIntent(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 12, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		TTFTMs:             5140,
		DurationMs:         11270,
		TokensPerSecond:    30,
		CostRatio:          0.89,
		CostReferenceRatio: 0.89,
		GroupPriorityRatio: 1,
		SampleCount:        40,
	}
	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "codex-plus"}, snapshot, core.GroupSmartPolicy{
		Strategy:        core.StrategyCostFirst,
		CandidateGroups: []string{"codex-plus"},
		GroupPriorityRatio: map[string]float64{
			"codex-plus": 1,
		},
	}, scheduler.ScoringContext{
		Strategy: core.StrategyCostFirst,
		RetryRoutingIntent: core.NewFirstByteTimeoutRetryRoutingIntent(
			9,
			"slow-first-byte",
			0,
		),
	}, true)

	ttftItem := scoreItemByKey(t, evaluation.Score.RoutingItems, "ttft_latency")
	costItem := scoreItemByKey(t, evaluation.Score.RoutingItems, "cost")
	retryItem := scoreItemByKey(t, evaluation.Score.RoutingItems, "retry_intent_recovery")
	require.InEpsilon(t, 0.13, ttftItem.Weight, 0.0001)
	require.InEpsilon(t, 0.2022, costItem.Weight, 0.0001)
	require.InEpsilon(t, 0.3527, retryItem.Weight, 0.0001)
	totalWeight := 0.0
	for _, item := range evaluation.Score.RoutingItems {
		if item.MissingReason == "" {
			totalWeight += item.Weight
		}
	}
	require.InDelta(t, 1.0, totalWeight, 0.0005)
}

func TestCostFirstUsesLogMultipleCostScore(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	policy := core.GroupSmartPolicy{
		Strategy:        core.StrategyCostFirst,
		CandidateGroups: []string{"default"},
		GroupPriorityRatio: map[string]float64{
			"default": 1,
		},
	}
	base := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", Group: "default", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		CostReferenceRatio: 0.02,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	}

	cheap := base
	cheap.Key.ChannelID = 1
	cheap.CostRatio = 0.02
	mid := base
	mid.Key.ChannelID = 2
	mid.CostRatio = 0.05
	expensive := base
	expensive.Key.ChannelID = 3
	expensive.CostRatio = 0.08

	cheapScore := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: cheap.Key, Group: "default"}, cheap, policy, scheduler.ScoringContext{Strategy: core.StrategyCostFirst}, false).Score
	midScore := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: mid.Key, Group: "default"}, mid, policy, scheduler.ScoringContext{Strategy: core.StrategyCostFirst}, false).Score
	expensiveScore := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: expensive.Key, Group: "default"}, expensive, policy, scheduler.ScoringContext{Strategy: core.StrategyCostFirst}, false).Score

	cheapCost := scoreItemByKey(t, cheapScore.Items, "cost")
	midCost := scoreItemByKey(t, midScore.Items, "cost")
	expensiveCost := scoreItemByKey(t, expensiveScore.Items, "cost")
	require.Equal(t, 1.0, cheapCost.Score)
	require.InEpsilon(t, 0.4307, midCost.Score, 0.0001)
	require.InEpsilon(t, 0.1386, expensiveCost.Score, 0.0001)
	require.Equal(t, "log_multiple", midCost.Reason)
	require.Equal(t, "1 - log(current_cost / min_cost) / log(zero_multiple)", midCost.Formula)
	require.InEpsilon(t, 2.5, midCost.FormulaParameters["cost_multiple"], 0.0001)
	require.InEpsilon(t, 5.0, midCost.FormulaParameters["cost_zero_multiple"], 0.0001)
}

func TestCandidateScoringServiceDoesNotDuplicateCompletionPenaltyIntoSeparateRateItems(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 10, Group: "default", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        0.25,
		TTFTMs:             500,
		DurationMs:         2500,
		TokensPerSecond:    30,
		CostRatio:          0.10,
		CostReferenceRatio: 0.10,
		GroupPriorityRatio: 1,
		SampleCount:        12,
	}

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "default"}, snapshot, core.GroupSmartPolicy{
		Strategy:        core.StrategyBalanced,
		CandidateGroups: []string{"default"},
		GroupPriorityRatio: map[string]float64{
			"default": 1,
		},
	}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)

	require.Equal(t, 0.25, scoreItemByKey(t, evaluation.Score.Items, "completion_rate").Score)
	require.Equal(t, 1.0, scoreItemByKey(t, evaluation.Score.Items, "upstream_error_rate").Score)
	require.Equal(t, 1.0, scoreItemByKey(t, evaluation.Score.Items, "stream_interrupted_rate").Score)
}

func TestCandidateScoringServiceExplanationUsesScoreItemLatencyWindow(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	fastSnapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 4, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		TTFTMs:             13990,
		DurationMs:         42000,
		CostRatio:          0.1,
		CostReferenceRatio: 0.1,
		GroupPriorityRatio: 1,
		SampleCount:        8,
		ScoreStatsJSON:     `{"version":1,"samples":8,"rates":{},"latency":{"ttft_ms":[3160],"duration_ms":[11450]}}`,
	}
	slowSnapshot := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 12, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		TTFTMs:             4410,
		DurationMs:         8000,
		CostRatio:          0.1,
		CostReferenceRatio: 0.1,
		GroupPriorityRatio: 1,
		SampleCount:        8,
		ScoreStatsJSON:     `{"version":1,"samples":8,"rates":{},"latency":{"ttft_ms":[10550],"duration_ms":[23350]}}`,
	}

	fast := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: fastSnapshot.Key, Group: "codex-plus"}, fastSnapshot, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)
	slow := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: slowSnapshot.Key, Group: "codex-plus"}, slowSnapshot, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)

	fastTTFT := scoreItemByKey(t, fast.Score.Items, "ttft_latency")
	slowTTFT := scoreItemByKey(t, slow.Score.Items, "ttft_latency")
	require.NotNil(t, fastTTFT.RawNumber)
	require.NotNil(t, slowTTFT.RawNumber)
	require.Equal(t, 3160.0, *fastTTFT.RawNumber)
	require.Equal(t, 10550.0, *slowTTFT.RawNumber)
	require.Equal(t, "score_stats_latency", fastTTFT.Source)
	require.Equal(t, fast.Explanation.TTFTMs, *fastTTFT.RawNumber)
	require.Equal(t, slow.Explanation.TTFTMs, *slowTTFT.RawNumber)
	require.Greater(t, fastTTFT.Score, slowTTFT.Score)
}

func TestTTFTLatencyUsesProgressiveScore(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	policy := core.GroupSmartPolicy{
		Strategy:        core.StrategyBalanced,
		CandidateGroups: []string{"default"},
		GroupPriorityRatio: map[string]float64{
			"default": 1,
		},
	}
	base := core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", Group: "default", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		DurationMs:         5000,
		TokensPerSecond:    45,
		CostRatio:          1,
		CostReferenceRatio: 1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	}

	scoreForTTFT := func(ttftMs float64) core.ScoreItem {
		snapshot := base
		snapshot.TTFTMs = ttftMs
		evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "default"}, snapshot, policy, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)
		return scoreItemByKey(t, evaluation.Score.Items, "ttft_latency")
	}

	at800 := scoreForTTFT(800)
	at1930 := scoreForTTFT(1930)
	at3990 := scoreForTTFT(3990)
	at8050 := scoreForTTFT(8050)
	at18000 := scoreForTTFT(18000)
	at20000 := scoreForTTFT(20000)

	require.Equal(t, 1.0, at800.Score)
	require.Greater(t, at1930.Score, at3990.Score)
	require.Greater(t, at3990.Score, at8050.Score)
	require.Greater(t, at8050.Score, at18000.Score)
	require.InEpsilon(t, 0.8634, at1930.Score, 0.0001)
	require.InEpsilon(t, 0.6557, at3990.Score, 0.0001)
	require.InEpsilon(t, 0.3654, at8050.Score, 0.0001)
	require.InEpsilon(t, 0.0321, at18000.Score, 0.0001)
	require.Equal(t, 0.0, at20000.Score)
	require.Equal(t, "recency_weighted_p50_progressive_score(ttft, decay=2.2, half_life=16, stability_penalty)", at3990.Formula)
}

func TestTTFTLatencyUsesRecencyWeightedP50ForOccasionalSlowSample(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := scoringServiceLatencySnapshot()
	for i := 0; i < 19; i++ {
		snapshot.RecentLatencySamples = append(snapshot.RecentLatencySamples, runtimeLatencySampleForTest(i, 2000))
	}
	snapshot.RecentLatencySamples = append(snapshot.RecentLatencySamples, runtimeLatencySampleForTest(19, 16000))
	snapshot.TTFTMs = 2700
	snapshot.ScoreStatsJSON = `{"version":1,"samples":20,"rates":{},"latency":{"ttft_ms":[2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,2000,16000]}}`

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "default"}, snapshot, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)
	ttftItem := scoreItemByKey(t, evaluation.Score.Items, "ttft_latency")

	require.NotNil(t, ttftItem.RawNumber)
	require.Equal(t, 2000.0, *ttftItem.RawNumber)
	require.Equal(t, "runtime_latency_samples", ttftItem.Source)
	require.Greater(t, ttftItem.Score, 0.75)
	require.Less(t, ttftItem.FormulaParameters["stability_penalty"], 1.0)
}

func TestTTFTLatencyPrefersRecentSamples(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := scoringServiceLatencySnapshot()
	for i := 0; i < 20; i++ {
		ttft := 2000.0
		if i >= 15 {
			ttft = 9000
		}
		snapshot.RecentLatencySamples = append(snapshot.RecentLatencySamples, runtimeLatencySampleForTest(i, ttft))
	}
	snapshot.TTFTMs = 3750

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "default"}, snapshot, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)
	ttftItem := scoreItemByKey(t, evaluation.Score.Items, "ttft_latency")

	require.NotNil(t, ttftItem.RawNumber)
	require.Equal(t, 9000.0, *ttftItem.RawNumber)
	require.Equal(t, "runtime_latency_samples", ttftItem.Source)
	require.Less(t, ttftItem.Score, 0.35)
}

func TestTTFTLatencyFallsBackToScoreStatsWithoutRuntimeSamples(t *testing.T) {
	service := scheduler.NewCandidateScoringService()
	snapshot := scoringServiceLatencySnapshot()
	snapshot.TTFTMs = 12000
	snapshot.ScoreStatsJSON = `{"version":1,"samples":4,"rates":{},"latency":{"ttft_ms":[2100,2200,2300,2400]}}`

	evaluation := service.EvaluatePreparedCandidate(core.Candidate{RuntimeKey: snapshot.Key, Group: "default"}, snapshot, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, scheduler.ScoringContext{Strategy: core.StrategyBalanced}, false)
	ttftItem := scoreItemByKey(t, evaluation.Score.Items, "ttft_latency")

	require.NotNil(t, ttftItem.RawNumber)
	require.Equal(t, 2250.0, *ttftItem.RawNumber)
	require.Equal(t, "score_stats_latency", ttftItem.Source)
}

func scoringServiceLatencySnapshot() core.RuntimeSnapshot {
	return core.RuntimeSnapshot{
		Key:                core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", Group: "default", EndpointType: constant.EndpointTypeOpenAI},
		SuccessRate:        1,
		DurationMs:         5000,
		TokensPerSecond:    45,
		CostRatio:          1,
		CostReferenceRatio: 1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	}
}

func runtimeLatencySampleForTest(index int, ttftMs float64) core.RuntimeLatencySample {
	return core.RuntimeLatencySample{
		ObservedAt: time.Unix(1700000000+int64(index), 0).Unix(),
		DurationMs: ttftMs + 1000,
		TTFTMs:     ttftMs,
	}
}

func scoreItemByKey(t *testing.T, items []core.ScoreItem, key string) core.ScoreItem {
	t.Helper()
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	require.Failf(t, "score item not found", "key=%s", key)
	return core.ScoreItem{}
}

func round4ForTest(value float64) float64 {
	return float64(int(value*10000+0.5)) / 10000
}
