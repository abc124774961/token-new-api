package scheduler

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
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

func TestDefaultSelectorAppliesChannelScoreBoostsToCandidateExplanations(t *testing.T) {
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		SmartScoreBoosts: map[string]float64{
			scoreItemCost: 0.6,
		},
	})
	require.NoError(t, err)

	channel := &model.Channel{
		Id:            68,
		Name:          "boosted",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: string(settingsBytes),
	}
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.4",
		UpstreamModel:  "gpt-5.4",
		ChannelID:      channel.Id,
		Group:          "codex-plus",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := NewMemoryRuntimeSnapshotStore()
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        10,
		SuccessRate:        0.8,
		TTFTMs:             900,
		DurationMs:         3000,
		TokensPerSecond:    20,
		CostRatio:          1.4,
		CostReferenceRatio: 1,
		GroupPriorityRatio: 1,
	})

	selector := NewDefaultSmartChannelSelector(
		NewStaticCandidatePoolBuilder([]core.Candidate{{
			Channel:       channel,
			Group:         key.Group,
			UpstreamModel: key.UpstreamModel,
			RuntimeKey:    key,
		}}),
		store,
		core.ScoreWeights{Success: 1, Speed: 1, Load: 1, Cost: 1, Group: 1},
	)
	plan, handled, apiErr := selector.Select(nil, &service.RetryParam{
		TokenGroup:   key.Group,
		ModelName:    key.RequestedModel,
		EndpointType: key.EndpointType,
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  key.Group,
		CandidateGroups: []string{key.Group},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Len(t, plan.Candidates, 1)
	item := scoreItemByKeyForTest(t, plan.Candidates[0].ScoreItems, scoreItemCost)
	require.NotNil(t, item.BaseScore)
	require.Equal(t, 0.6, item.ScoreBoost)
	require.True(t, item.ScoreAdjusted)
	require.Greater(t, item.Score, *item.BaseScore)
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
