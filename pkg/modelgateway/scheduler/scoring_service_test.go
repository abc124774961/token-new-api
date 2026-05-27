package scheduler_test

import (
	"testing"

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

func TestCostFirstWeightsKeepFirstByteRelevant(t *testing.T) {
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
	ttftItem := scoreItemByKey(t, evaluation.Score.Items, "ttft_latency")
	firstByteBacklogItem := scoreItemByKey(t, evaluation.Score.Items, "first_byte_backlog")
	require.InEpsilon(t, 0.25, costItem.Weight, 0.0001)
	require.InEpsilon(t, 0.15, ttftItem.Weight, 0.0001)
	require.InEpsilon(t, 0.08, firstByteBacklogItem.Weight, 0.0001)
	require.Greater(t, ttftItem.Weight, costItem.Weight*0.50)
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
