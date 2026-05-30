package scheduler

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/stretchr/testify/require"
)

func TestScoreBoostServiceNormalizeFiltersInvalidValues(t *testing.T) {
	service := NewScoreBoostService()

	boosts := service.Normalize(map[string]float64{
		scoreItemCompletionRate:      0.6,
		scoreItemTTFTLatency:         -0.2,
		scoreItemDurationLatency:     math.NaN(),
		scoreItemCost:                2,
		scoreItemGroupPriority:       0.5,
		scoreItemRetryIntentRecovery: 0.5,
		"bad":                        0.5,
	})

	require.Equal(t, map[string]float64{
		scoreItemCompletionRate: 0.6,
		scoreItemCost:           1,
	}, boosts)
}

func TestBuildScoreItemsAppliesBoostAndCapsScore(t *testing.T) {
	items := NewCandidateScoringService().BuildScoreItems(core.RuntimeSnapshot{
		SampleCount:        10,
		SuccessRate:        0.5,
		TTFTMs:             1200,
		DurationMs:         3000,
		TokensPerSecond:    20,
		CostRatio:          1,
		CostReferenceRatio: 1,
		GroupPriorityRatio: 1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, ScoringContext{
		ScoreBoosts: map[string]float64{scoreItemCompletionRate: 0.6},
	}, false)

	item := scoreItemByKeyForTest(t, items, scoreItemCompletionRate)
	require.NotNil(t, item.BaseScore)
	require.Equal(t, 0.5, *item.BaseScore)
	require.Equal(t, 0.6, item.ScoreBoost)
	require.True(t, item.ScoreAdjusted)
	require.Equal(t, 1.0, item.Score)
}

func TestBuildScoreItemsDoesNotBoostMissingSamples(t *testing.T) {
	items := NewCandidateScoringService().BuildScoreItems(core.RuntimeSnapshot{}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, ScoringContext{
		ScoreBoosts: map[string]float64{scoreItemCompletionRate: 0.6},
	}, false)

	item := scoreItemByKeyForTest(t, items, scoreItemCompletionRate)
	require.Equal(t, "sample_missing", item.MissingReason)
	require.Nil(t, item.BaseScore)
	require.False(t, item.ScoreAdjusted)
	require.Zero(t, item.Score)
}

func TestBuildScoreItemsDoesNotBoostMissingCostReference(t *testing.T) {
	items := NewCandidateScoringService().BuildScoreItems(core.RuntimeSnapshot{
		SampleCount: 10,
		SuccessRate: 0.8,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, ScoringContext{
		ScoreBoosts: map[string]float64{scoreItemCost: 0.6},
	}, false)

	item := scoreItemByKeyForTest(t, items, scoreItemCost)
	require.Equal(t, "cost_reference_missing", item.MissingReason)
	require.Nil(t, item.BaseScore)
	require.False(t, item.ScoreAdjusted)
	require.Zero(t, item.Score)
}

func TestCandidateScoringServiceLoadsBoostsFromChannelSettings(t *testing.T) {
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		SmartScoreBoosts: map[string]float64{
			scoreItemCompletionRate: 0.4,
		},
	})
	require.NoError(t, err)

	evaluation := NewCandidateScoringService().EvaluatePreparedCandidate(core.Candidate{
		Channel: &model.Channel{OtherSettings: string(settingsBytes)},
	}, core.RuntimeSnapshot{
		SampleCount:        10,
		SuccessRate:        0.5,
		CostRatio:          1,
		CostReferenceRatio: 1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced}, ScoringContext{}, false)

	item := scoreItemByKeyForTest(t, evaluation.Score.Items, scoreItemCompletionRate)
	require.NotNil(t, item.BaseScore)
	require.Equal(t, 0.5, *item.BaseScore)
	require.Equal(t, 0.4, item.ScoreBoost)
	require.True(t, item.ScoreAdjusted)
	require.Equal(t, 0.9, item.Score)
}

func scoreItemByKeyForTest(t *testing.T, items []core.ScoreItem, key string) core.ScoreItem {
	t.Helper()
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	require.Failf(t, "score item not found", "key=%s", key)
	return core.ScoreItem{}
}
