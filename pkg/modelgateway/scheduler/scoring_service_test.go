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
