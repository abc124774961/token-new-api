package scheduler_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayprovider "github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSnapshotStorePutGetAndList(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel: "gpt-4.1",
		ChannelID:      1,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{Key: key, SuccessRate: 0.95})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 0.95, snapshot.SuccessRate)

	candidates := store.ListCandidates(&core.DispatchRequest{ModelName: "gpt-4.1"})
	require.Len(t, candidates, 1)
	candidates = store.ListCandidates(&core.DispatchRequest{ModelName: "other"})
	require.Empty(t, candidates)
}

func TestBalancedScorerPrefersHealthyFastCandidate(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	good := scorer.Score(core.Candidate{
		Channel: &model.Channel{Id: 1},
		Group:   "default",
	}, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             500,
		TokensPerSecond:    60,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        50,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	bad := scorer.Score(core.Candidate{
		Channel: &model.Channel{Id: 2},
		Group:   "default",
	}, core.RuntimeSnapshot{
		SuccessRate:        0.70,
		TTFTMs:             5000,
		TokensPerSecond:    8,
		ActiveConcurrency:  9,
		MaxConcurrency:     10,
		CostRatio:          2,
		GroupPriorityRatio: 1,
		SampleCount:        50,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Greater(t, good.Total, bad.Total)
	require.Greater(t, good.RoutingTotal, bad.RoutingTotal)
	require.Greater(t, good.Breakdown["success"], bad.Breakdown["success"])
	require.Greater(t, good.Breakdown["speed"], bad.Breakdown["speed"])
	require.Equal(t, good.Breakdown["load"], bad.Breakdown["load"])
	require.Greater(t, good.RoutingBreakdown["load"], bad.RoutingBreakdown["load"])
}

func TestLowSuccessCapsSpeedContribution(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 10}}, core.RuntimeSnapshot{
		SuccessRate:        0.08,
		SuccessScore:       0.05,
		SpeedScore:         1.0,
		TTFTMs:             1200,
		DurationMs:         600,
		CostRatio:          0.12,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.LessOrEqual(t, score.Breakdown["speed"], 0.13)
	require.Less(t, score.Total, 0.50)
	require.Less(t, score.RoutingTotal, 0.50)
}

func TestLongTermLowSuccessCapsRecentRecoveryScore(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 8}}, core.RuntimeSnapshot{
		SuccessRate:        0.52,
		SuccessScore:       1.0,
		SpeedScore:         0.86,
		TTFTMs:             3600,
		DurationMs:         3900,
		CostRatio:          0.13,
		GroupPriorityRatio: 1,
		SampleCount:        626,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Less(t, score.Breakdown["success"], 0.65)
	require.Greater(t, score.Breakdown["success"], 0.50)
	require.Less(t, score.Total, 0.80)
	require.Less(t, score.RoutingTotal, 0.80)
}

func TestScoreFactoryStrategiesShiftWeights(t *testing.T) {
	factory := scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}
	fastExpensive := core.RuntimeSnapshot{
		SuccessRate:        0.95,
		TTFTMs:             350,
		TokensPerSecond:    80,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          3,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	}
	slowCheap := core.RuntimeSnapshot{
		SuccessRate:        0.95,
		TTFTMs:             3500,
		TokensPerSecond:    10,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.5,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	}

	speedFast := factory.ForStrategy(core.StrategySpeedFirst).Score(candidate, fastExpensive, core.GroupSmartPolicy{})
	speedCheap := factory.ForStrategy(core.StrategySpeedFirst).Score(candidate, slowCheap, core.GroupSmartPolicy{})
	require.Greater(t, speedFast.Total, speedCheap.Total)

	costFast := factory.ForStrategy(core.StrategyCostFirst).Score(candidate, fastExpensive, core.GroupSmartPolicy{})
	costCheap := factory.ForStrategy(core.StrategyCostFirst).Score(candidate, slowCheap, core.GroupSmartPolicy{})
	require.Greater(t, costCheap.Total, costFast.Total)
}

func TestCostFirstPrefersCheapCandidateOverFasterExpensiveCandidate(t *testing.T) {
	scorer := scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()).ForStrategy(core.StrategyCostFirst)
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	cheapPlus := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.66,
		TTFTMs:             7200,
		DurationMs:         5800,
		CostRatio:          0.13,
		GroupPriorityRatio: 1,
		SampleCount:        100,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	fastPro := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.87,
		TTFTMs:             3200,
		DurationMs:         3600,
		CostRatio:          0.30,
		GroupPriorityRatio: 1,
		SampleCount:        100,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})

	require.Greater(t, cheapPlus.Breakdown["cost"], fastPro.Breakdown["cost"])
	require.Greater(t, cheapPlus.Total, fastPro.Total)
	require.Greater(t, cheapPlus.RoutingTotal, fastPro.RoutingTotal)
}

func TestCostFirstUsesConfiguredGroupWeightAndLowerGroupRatio(t *testing.T) {
	scorer := scheduler.NewScoreCalculatorFactory(core.ScoreWeights{
		Success: 0.32,
		Speed:   0.28,
		Load:    0.10,
		Cost:    0.15,
		Group:   0.30,
	}).ForStrategy(core.StrategyCostFirst)
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	cheapPreferredGroup := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		SuccessScore:       0.99,
		SpeedScore:         0.64,
		TTFTMs:             6800,
		DurationMs:         7200,
		CostRatio:          0.12,
		GroupPriorityRatio: 0.1,
		SampleCount:        120,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{
		Strategy:        core.StrategyCostFirst,
		CandidateGroups: []string{"codex-plus", "codex-pro"},
		GroupPriorityRatio: map[string]float64{
			"auto":       0.09,
			"codex-plus": 0.1,
			"codex-pro":  0.2,
		},
	})
	fastLowPriorityGroup := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		SuccessScore:       0.99,
		SpeedScore:         0.91,
		TTFTMs:             1600,
		DurationMs:         2600,
		CostRatio:          0.24,
		GroupPriorityRatio: 0.2,
		SampleCount:        120,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{
		Strategy:        core.StrategyCostFirst,
		CandidateGroups: []string{"codex-plus", "codex-pro"},
		GroupPriorityRatio: map[string]float64{
			"auto":       0.09,
			"codex-plus": 0.1,
			"codex-pro":  0.2,
		},
	})

	require.Greater(t, cheapPreferredGroup.Breakdown["cost"], fastLowPriorityGroup.Breakdown["cost"])
	require.Greater(t, cheapPreferredGroup.Breakdown["group"], fastLowPriorityGroup.Breakdown["group"])
	require.Greater(t, cheapPreferredGroup.Total, fastLowPriorityGroup.Total)
	require.Greater(t, cheapPreferredGroup.RoutingTotal, fastLowPriorityGroup.RoutingTotal)
}

func TestCostFirstDoesNotRouteToHigherCostGroupOnlyBecauseCheapGroupIsBusy(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	cheapPlusKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 4, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI}
	fastProKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 9, Group: "codex-pro", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                       cheapPlusKey,
		SuccessRate:               0.99,
		SuccessScore:              0.99,
		SpeedScore:                0.50,
		TTFTMs:                    3200,
		DurationMs:                5000,
		ActiveConcurrency:         10,
		EffectiveConcurrencyLimit: 0,
		CostRatio:                 0.05,
		GroupPriorityRatio:        0.1,
		SampleCount:               120,
		ExperienceScore:           1,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                fastProKey,
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.95,
		TTFTMs:             1200,
		DurationMs:         2600,
		ActiveConcurrency:  0,
		CostRatio:          0.12,
		GroupPriorityRatio: 0.2,
		SampleCount:        120,
		ExperienceScore:    1,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 4, Name: "toioto-0.05x"}, Group: "codex-plus", UpstreamModel: "gpt-5.5", RuntimeKey: cheapPlusKey},
			{Channel: &model.Channel{Id: 9, Name: "maxtopai-pro"}, Group: "codex-pro", UpstreamModel: "gpt-5.5", RuntimeKey: fastProKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "auto",
		CandidateGroups: []string{"codex-plus", "codex-pro"},
		Strategy:        core.StrategyCostFirst,
		GroupPriorityRatio: map[string]float64{
			"codex-plus": 0.1,
			"codex-pro":  0.2,
		},
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 4, plan.Channel.Id)
	cheapPlus := candidateExplanationByChannel(t, plan.Candidates, 4)
	fastPro := candidateExplanationByChannel(t, plan.Candidates, 9)
	require.Greater(t, cheapPlus.RoutingScoreTotal, fastPro.RoutingScoreTotal)
	require.Equal(t, cheapPlus.ScoreBreakdown["load"], cheapPlus.RoutingScoreBreakdown["load"])
	require.NotContains(t, cheapPlus.RoutingScoreBreakdown, "routing_pressure")
}

func TestLoadScoreHardDropsCircuitOpenSnapshot(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 1}}, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             500,
		TokensPerSecond:    60,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		CircuitOpen:        true,
		SampleCount:        20,
	}, core.GroupSmartPolicy{})

	require.Zero(t, score.Breakdown["load"])
	require.Zero(t, score.RoutingBreakdown["load"])
}

func TestHealthScoreIgnoresTransientConcurrency(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}
	base := core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             1200,
		DurationMs:         5000,
		TokensPerSecond:    45,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.99,
		SpeedScore:         0.90,
		ExperienceScore:    1,
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	busy := base
	busy.ActiveConcurrency = 8
	busy.QueueDepth = 4
	busy.EstimatedQueueWaitMs = 8000
	busy.FirstBytePending = 3
	busy.SlowFirstBytePending = 2
	busy.OldestFirstByteWaitMs = 15000
	busyScore := scorer.Score(candidate, busy, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Equal(t, normal.Total, busyScore.Total)
	require.Equal(t, normal.Breakdown["load"], busyScore.Breakdown["load"])
	require.Greater(t, normal.RoutingTotal, busyScore.RoutingTotal)
	require.Less(t, busyScore.RoutingBreakdown["load"], normal.RoutingBreakdown["load"])
	require.Contains(t, busyScore.RoutingBreakdown, "ttft_pending")
	require.NotContains(t, busyScore.Breakdown, "ttft_pending")
}

func TestRoutingLoadScoreTreatsLowConfiguredConcurrencyUsageAsHealthy(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 8}, Group: "codex-plus"}
	score := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:                0.99,
		SuccessScore:               0.99,
		SpeedScore:                 0.70,
		TTFTMs:                     3330,
		DurationMs:                 3770,
		ActiveConcurrency:          3,
		ConfiguredConcurrencyLimit: 46,
		CostRatio:                  0.06,
		GroupPriorityRatio:         1,
		SampleCount:                120,
		ExperienceScore:            1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.GreaterOrEqual(t, score.RoutingBreakdown["load"], 0.99)
	require.NotContains(t, score.RoutingBreakdown, "routing_pressure")
}

func TestRoutingLoadScoreDropsNearConcurrencyLimit(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 8}, Group: "codex-plus"}
	score := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:                0.99,
		SuccessScore:               0.99,
		SpeedScore:                 0.70,
		TTFTMs:                     3330,
		DurationMs:                 3770,
		ActiveConcurrency:          46,
		ConfiguredConcurrencyLimit: 46,
		CostRatio:                  0.06,
		GroupPriorityRatio:         1,
		SampleCount:                120,
		ExperienceScore:            1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Less(t, score.RoutingBreakdown["load"], 0.50)
	require.Contains(t, score.RoutingBreakdown, "routing_pressure")
}

func TestCostFirstRoutingLoadOnlyPenalizesExceededConcurrencyLimit(t *testing.T) {
	scorer := scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()).ForStrategy(core.StrategyCostFirst)
	candidate := core.Candidate{Channel: &model.Channel{Id: 4}, Group: "codex-plus"}
	base := core.RuntimeSnapshot{
		SuccessRate:                0.99,
		SuccessScore:               0.99,
		SpeedScore:                 0.86,
		TTFTMs:                     3500,
		DurationMs:                 4600,
		ActiveConcurrency:          3,
		EffectiveConcurrencyLimit:  40,
		ConfiguredConcurrencyLimit: 40,
		CostRatio:                  0.05,
		GroupPriorityRatio:         0.1,
		SampleCount:                200,
		ExperienceScore:            1,
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	require.Equal(t, 1.0, normal.RoutingBreakdown["load"])
	require.NotContains(t, normal.RoutingBreakdown, "routing_pressure")

	overLimit := base
	overLimit.ActiveConcurrency = 90
	busy := scorer.Score(candidate, overLimit, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	require.Less(t, busy.RoutingBreakdown["load"], normal.RoutingBreakdown["load"])
	require.Contains(t, busy.RoutingBreakdown, "routing_pressure")
	require.Less(t, busy.RoutingTotal, normal.RoutingTotal)
	require.Equal(t, normal.Breakdown["load"], busy.Breakdown["load"])
}

func TestSelectorDoesNotSkipFullConcurrencyCandidateWhenQueueDisabled(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        0.99,
		ActiveConcurrency:  2,
		MaxConcurrency:     2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.80,
		ActiveConcurrency:  1,
		MaxConcurrency:     2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    false,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	first := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.True(t, first.Available)
	require.Empty(t, first.RejectReason)
}

func TestSelectorSkipsRequestBalanceMarkedCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	service.MarkChannelBalanceSkipped(ctx, 1)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "balance-empty", Status: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2, Name: "healthy", Status: 1}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(ctx, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    false,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	skipped := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.False(t, skipped.Available)
	require.Equal(t, "balance_insufficient", skipped.RejectReason)
}

func TestSelectorDoesNotLimitColdStartProbeConcurrency(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                       firstKey,
		SuccessRate:               0.80,
		ActiveConcurrency:         2,
		MaxConcurrency:            6,
		EffectiveConcurrencyLimit: 6,
		CostRatio:                 0.08,
		GroupPriorityRatio:        1,
		SampleCount:               0,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                       secondKey,
		SuccessRate:               0.80,
		ActiveConcurrency:         0,
		MaxConcurrency:            6,
		EffectiveConcurrencyLimit: 6,
		CostRatio:                 0.12,
		GroupPriorityRatio:        1,
		SampleCount:               0,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "cold-cheap"}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2, Name: "cold-peer"}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    false,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	first := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.True(t, first.Available)
	require.Empty(t, first.RejectReason)
	require.Zero(t, first.SuccessScore)
	require.Zero(t, first.SpeedScore)
}

func TestSelectorRecordsCandidateExplanations(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	rejectedKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI, CapabilityFingerprint: "openai_codex"}
	selectedKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "mimo-v1", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI, CapabilityFingerprint: "mimo_codex_chat"}
	store.Put(core.RuntimeSnapshot{
		Key:                rejectedKey,
		SuccessRate:        0.99,
		TTFTMs:             250,
		TokensPerSecond:    100,
		ActiveConcurrency:  0,
		MaxConcurrency:     4,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		CircuitOpen:        true,
		SampleCount:        10,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                selectedKey,
		SuccessRate:        0.80,
		TTFTMs:             800,
		TokensPerSecond:    40,
		ActiveConcurrency:  1,
		MaxConcurrency:     4,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "openai"}, Group: "default", UpstreamModel: "gpt-5.5", ProviderProfile: "openai_codex", ProxyMode: "native", RuntimeKey: rejectedKey},
			{Channel: &model.Channel{Id: 2, Name: "mimo"}, Group: "default", UpstreamModel: "mimo-v1", ProviderProfile: "mimo_codex_chat", ProxyMode: "responses_via_chat", RuntimeKey: selectedKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    false,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.Len(t, plan.Candidates, 2)

	rejected := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.False(t, rejected.Available)
	require.False(t, rejected.Selected)
	require.Equal(t, "circuit_open", rejected.RejectReason)
	require.Equal(t, "openai_codex", rejected.ProviderProfile)
	require.Equal(t, rejectedKey, rejected.RuntimeKey)

	selected := candidateExplanationByChannel(t, plan.Candidates, 2)
	require.True(t, selected.Available)
	require.True(t, selected.Selected)
	require.Empty(t, selected.RejectReason)
	require.Equal(t, "mimo_codex_chat", selected.ProviderProfile)
	require.Equal(t, "responses_via_chat", selected.ProxyMode)
	require.Greater(t, selected.ScoreTotal, 0.0)
	require.Contains(t, selected.ScoreBreakdown, "success")
	require.Equal(t, selectedKey, selected.RuntimeKey)
}

func TestSelectorReadsSnapshotStoredUnderEnrichedRuntimeKey(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	baseKey := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo-v1",
		ChannelID:      2,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	enrichedKey := baseKey
	enrichedKey.CapabilityFingerprint = modelgatewayprovider.ProfileMiMoCodexChat + "|" + modelgatewayprovider.ProxyModeResponsesViaChat
	store.Put(core.RuntimeSnapshot{
		Key:                enrichedKey,
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.03,
		TTFTMs:             45000,
		DurationMs:         46000,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        3,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{
				Channel:         &model.Channel{Id: 2, Name: "dora"},
				Group:           "default",
				UpstreamModel:   "mimo-v1",
				ProviderProfile: modelgatewayprovider.ProfileMiMoCodexChat,
				ProxyMode:       modelgatewayprovider.ProxyModeResponsesViaChat,
				RuntimeKey:      baseKey,
			},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	).WithRuntimeSnapshotEnricher(scheduler.NewRuntimeSnapshotEnricher(nil, 1500, 8, 2))

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, enrichedKey, plan.RuntimeKey)
	require.Less(t, plan.ScoreTotal, 0.35)
	require.Len(t, plan.Candidates, 1)
	candidate := plan.Candidates[0]
	require.Equal(t, 3, candidate.SampleCount)
	require.Equal(t, 45000.0, candidate.TTFTMs)
	require.Less(t, candidate.ScoreTotal, 0.35)
	require.Less(t, candidate.SpeedScore, 0.10)
	require.Equal(t, candidate.ScoreBreakdown["speed"], candidate.ScoreSpeedFactor)
	require.Equal(t, 0.78, candidate.ScoreBreakdown["ttft_penalty"])
}

func TestSelectorPrefersEnrichedSnapshotOverLegacyRuntimeKey(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	baseKey := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      4,
		Group:          "codex-plus",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	enrichedKey := baseKey
	enrichedKey.CapabilityFingerprint = modelgatewayprovider.ProfileOpenAICodex + "|" + modelgatewayprovider.ProxyModeNativeResponses
	legacyKey := enrichedKey
	legacyKey.EndpointType = ""

	store.Put(core.RuntimeSnapshot{
		Key:                legacyKey,
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.96,
		TTFTMs:             900,
		DurationMs:         1500,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        5,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                enrichedKey,
		SuccessRate:        0.998,
		SuccessScore:       1,
		SpeedScore:         0.50,
		TTFTMs:             12500,
		DurationMs:         9400,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        1705,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{
				Channel:         &model.Channel{Id: 4, Name: "toioto"},
				Group:           "codex-plus",
				UpstreamModel:   "gpt-5.5",
				ProviderProfile: modelgatewayprovider.ProfileOpenAICodex,
				ProxyMode:       modelgatewayprovider.ProxyModeNativeResponses,
				RuntimeKey:      baseKey,
			},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	).WithRuntimeSnapshotEnricher(scheduler.NewRuntimeSnapshotEnricher(nil, 1500, 8, 2))

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "codex-plus",
		CandidateGroups: []string{"codex-plus"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, enrichedKey, plan.RuntimeKey)
	candidate := candidateExplanationByChannel(t, plan.Candidates, 4)
	require.Equal(t, "exact", candidate.ScoreSampleSource)
	require.Equal(t, enrichedKey, candidate.MatchedRuntimeKey)
	require.Equal(t, 1705, candidate.SampleCount)
	require.Equal(t, 12500.0, candidate.TTFTMs)
	require.Equal(t, candidate.ScoreBreakdown["speed"], candidate.ScoreSpeedFactor)
	require.Equal(t, 0.50, candidate.SpeedScore)
}

func TestSelectorUsesRoutingScoreButReportsHealthScore(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	busyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	freeKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                busyKey,
		SuccessRate:        0.99,
		TTFTMs:             900,
		DurationMs:         3000,
		ActiveConcurrency:  6,
		MaxConcurrency:     8,
		QueueDepth:         2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.99,
		SpeedScore:         0.90,
		ExperienceScore:    1,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                freeKey,
		SuccessRate:        0.96,
		TTFTMs:             1200,
		DurationMs:         3500,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.96,
		SpeedScore:         0.84,
		ExperienceScore:    1,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "busy-healthy"}, Group: "default", RuntimeKey: busyKey},
			{Channel: &model.Channel{Id: 2, Name: "free-good"}, Group: "default", RuntimeKey: freeKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	busy := candidateExplanationByChannel(t, plan.Candidates, 1)
	free := candidateExplanationByChannel(t, plan.Candidates, 2)
	require.Greater(t, busy.ScoreTotal, free.ScoreTotal)
	require.Less(t, busy.RoutingScoreTotal, free.RoutingScoreTotal)
	require.Equal(t, 1.0, busy.ScoreBreakdown["load"])
	require.Less(t, busy.RoutingScoreBreakdown["load"], free.RoutingScoreBreakdown["load"])
}

func TestSelectorMarksBalanceInsufficientCandidateUnavailable(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	balanceKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	selectedKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	balancePaused := &model.Channel{Id: 1, Name: "balance-paused", Status: 3}
	balancePaused.SetOtherInfo(map[string]interface{}{"status_reason": "balance_insufficient"})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: balancePaused, Group: "default", RuntimeKey: balanceKey},
			{Channel: &model.Channel{Id: 2, Name: "healthy", Status: 1}, Group: "default", RuntimeKey: selectedKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	paused := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.False(t, paused.Available)
	require.True(t, paused.BalanceInsufficient)
	require.Equal(t, "balance_insufficient", paused.StatusReason)
	require.Equal(t, "balance_insufficient", paused.RejectReason)
}

func TestSelectorMarksConfirmedZeroBalanceCandidateUnavailable(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	balanceKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	selectedKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	zeroBalance := &model.Channel{Id: 1, Name: "zero-balance", Status: 1, Balance: 0, BalanceUpdatedTime: common.GetTimestamp()}
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: zeroBalance, Group: "default", RuntimeKey: balanceKey},
			{Channel: &model.Channel{Id: 2, Name: "healthy", Status: 1}, Group: "default", RuntimeKey: selectedKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	paused := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.False(t, paused.Available)
	require.True(t, paused.BalanceInsufficient)
	require.Equal(t, "balance_insufficient", paused.StatusReason)
	require.Equal(t, "balance_insufficient", paused.RejectReason)
}

func TestSelectorAllowsShortQueueWhenQueueEnabled(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                  firstKey,
		SuccessRate:          1,
		TTFTMs:               300,
		TokensPerSecond:      80,
		ActiveConcurrency:    2,
		MaxConcurrency:       2,
		QueueDepth:           0,
		QueueCapacity:        4,
		QueueTimeoutMs:       2000,
		EstimatedQueueWaitMs: 500,
		CostRatio:            1,
		GroupPriorityRatio:   1,
		SampleCount:          10,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.55,
		TTFTMs:             10000,
		TokensPerSecond:    1,
		ActiveConcurrency:  0,
		MaxConcurrency:     2,
		CostRatio:          3,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.Zero(t, plan.QueueWaitMs)
}

func TestSelectorSkipsSaturatedCandidateWhenPeerHasCapacity(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	saturatedKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 4, Group: "pro", EndpointType: constant.EndpointTypeOpenAI}
	availableKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "plus", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                       saturatedKey,
		SuccessRate:               1,
		TTFTMs:                    450,
		TokensPerSecond:           90,
		ActiveConcurrency:         84,
		MaxConcurrency:            40,
		EffectiveConcurrencyLimit: 40,
		QueueDepth:                0,
		QueueCapacity:             80,
		QueueTimeoutMs:            2000,
		CostRatio:                 0.05,
		GroupPriorityRatio:        1,
		SampleCount:               100,
		SuccessScore:              0.99,
		SpeedScore:                0.96,
		ExperienceScore:           1,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                       availableKey,
		SuccessRate:               0.92,
		TTFTMs:                    3200,
		TokensPerSecond:           15,
		ActiveConcurrency:         3,
		MaxConcurrency:            46,
		EffectiveConcurrencyLimit: 46,
		CostRatio:                 0.18,
		GroupPriorityRatio:        0.7,
		SampleCount:               30,
		SuccessScore:              0.92,
		SpeedScore:                0.54,
		ExperienceScore:           1,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 4, Name: "toioto-0.05x"}, Group: "pro", RuntimeKey: saturatedKey},
			{Channel: &model.Channel{Id: 2, Name: "backup"}, Group: "plus", RuntimeKey: availableKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "pro",
		CandidateGroups: []string{"pro", "plus"},
		Strategy:        core.StrategyCostFirst,
		QueueEnabled:    true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.Zero(t, plan.QueueWaitMs)
	selected := candidateExplanationByChannel(t, plan.Candidates, 2)
	saturated := candidateExplanationByChannel(t, plan.Candidates, 4)
	require.True(t, selected.Selected)
	require.False(t, saturated.Selected)
	require.Equal(t, "concurrency_saturated", saturated.SelectionSkipReason)
	require.Greater(t, saturated.RoutingScoreTotal, 0.0)
	require.Contains(t, saturated.RoutingScoreBreakdown, "routing_pressure")
}

func TestSelectorTreatsLearnedEffectiveLimitAsRoutingCeiling(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	learnedKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 3, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI}
	backupKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 6, Group: "codex-pro", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                        learnedKey,
		SuccessRate:                1,
		TTFTMs:                     1200,
		TokensPerSecond:            80,
		ActiveConcurrency:          31,
		MaxConcurrency:             44,
		ConfiguredConcurrencyLimit: 44,
		LearnedConcurrencyLimit:    31,
		EffectiveConcurrencyLimit:  31,
		CostRatio:                  0.06,
		GroupPriorityRatio:         1,
		SampleCount:                120,
		SuccessScore:               1,
		SpeedScore:                 0.94,
		ExperienceScore:            1,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                       backupKey,
		SuccessRate:               1,
		TTFTMs:                    2600,
		TokensPerSecond:           40,
		ActiveConcurrency:         8,
		MaxConcurrency:            0,
		EffectiveConcurrencyLimit: 0,
		CostRatio:                 0.15,
		GroupPriorityRatio:        0.8,
		SampleCount:               120,
		SuccessScore:              1,
		SpeedScore:                0.78,
		ExperienceScore:           1,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 3, Name: "chaozhi"}, Group: "codex-plus", RuntimeKey: learnedKey},
			{Channel: &model.Channel{Id: 6, Name: "b886"}, Group: "codex-pro", RuntimeKey: backupKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "auto",
		CandidateGroups: []string{"codex-plus", "codex-pro"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 6, plan.Channel.Id)
	learned := candidateExplanationByChannel(t, plan.Candidates, 3)
	require.False(t, learned.Selected)
	require.Equal(t, 31, learned.EffectiveConcurrencyLimit)
}

func TestSelectorQueuesOnlyWhenAllCandidatesSaturated(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                       firstKey,
		SuccessRate:               1,
		TTFTMs:                    300,
		TokensPerSecond:           90,
		ActiveConcurrency:         2,
		MaxConcurrency:            2,
		EffectiveConcurrencyLimit: 2,
		QueueDepth:                0,
		QueueCapacity:             4,
		QueueTimeoutMs:            1800,
		CostRatio:                 1,
		GroupPriorityRatio:        1,
		SampleCount:               20,
		SuccessScore:              1,
		SpeedScore:                0.95,
		ExperienceScore:           1,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                       secondKey,
		SuccessRate:               0.80,
		TTFTMs:                    10000,
		TokensPerSecond:           4,
		ActiveConcurrency:         2,
		MaxConcurrency:            2,
		EffectiveConcurrencyLimit: 2,
		QueueDepth:                1,
		QueueCapacity:             4,
		QueueTimeoutMs:            1200,
		CostRatio:                 2,
		GroupPriorityRatio:        1,
		SampleCount:               20,
		SuccessScore:              0.80,
		SpeedScore:                0.28,
		ExperienceScore:           1,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.Equal(t, 1800, plan.QueueWaitMs)
}

func TestSlowCandidateCannotOutscoreFastCandidateWithRealLatencySamples(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}

	fast := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.96,
		TTFTMs:             900,
		DurationMs:         4200,
		TokensPerSecond:    42,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		CostRatio:          1.1,
		GroupPriorityRatio: 1,
		SampleCount:        18,
		SuccessScore:       0.96,
		SpeedScore:         0.91,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	slowCheap := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             52000,
		DurationMs:         96000,
		TokensPerSecond:    3,
		ActiveConcurrency:  0,
		MaxConcurrency:     8,
		CostRatio:          0.2,
		GroupPriorityRatio: 1,
		SampleCount:        16,
		SuccessScore:       0.99,
		SpeedScore:         0.08,
		ExperienceScore:    0.95,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Greater(t, fast.Total, slowCheap.Total)
	require.Greater(t, fast.Breakdown["speed"], slowCheap.Breakdown["speed"])
	require.Less(t, slowCheap.Total, 0.45)
}

func TestHighTTFTAppliesExplicitPenaltyEvenWithHealthySpeedScore(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}

	normal := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             1500,
		DurationMs:         5000,
		TokensPerSecond:    45,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.99,
		SpeedScore:         0.92,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	slowFirstByte := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             36000,
		DurationMs:         55000,
		TokensPerSecond:    45,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		CostRatio:          0.2,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.99,
		SpeedScore:         0.92,
		ExperienceScore:    1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Greater(t, normal.Total, slowFirstByte.Total)
	require.Contains(t, slowFirstByte.Breakdown, "ttft_penalty")
	require.GreaterOrEqual(t, slowFirstByte.Breakdown["ttft_penalty"], 0.72)
	require.Less(t, slowFirstByte.Breakdown["speed"], 0.28)
	require.LessOrEqual(t, slowFirstByte.Total, 0.32)
}

func TestFirstBytePendingAppliesDynamicPenalty(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}
	base := core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             1200,
		DurationMs:         5000,
		TokensPerSecond:    45,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.99,
		SpeedScore:         0.90,
		ExperienceScore:    1,
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	pending := base
	pending.FirstBytePending = 3
	pending.SlowFirstBytePending = 2
	pending.OldestFirstByteWaitMs = 15000
	pendingScore := scorer.Score(candidate, pending, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Equal(t, normal.Total, pendingScore.Total)
	require.Contains(t, pendingScore.RoutingBreakdown, "ttft_pending")
	require.NotContains(t, pendingScore.Breakdown, "ttft_pending")
	require.Less(t, pendingScore.RoutingBreakdown["load"], normal.RoutingBreakdown["load"])
	require.Less(t, pendingScore.RoutingTotal, 0.70)
}

func TestUnknownLimitActiveConcurrencyDoesNotAffectRoutingScore(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}
	base := core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             1800,
		DurationMs:         3600,
		TokensPerSecond:    45,
		ActiveConcurrency:  0,
		CostRatio:          0.2,
		GroupPriorityRatio: 1,
		SampleCount:        20,
		SuccessScore:       0.99,
		SpeedScore:         0.88,
		ExperienceScore:    1,
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	busy := base
	busy.ActiveConcurrency = 10
	busyScore := scorer.Score(candidate, busy, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Equal(t, normal.Total, busyScore.Total)
	require.NotContains(t, busyScore.Breakdown, "routing_pressure")
	require.NotContains(t, busyScore.RoutingBreakdown, "routing_pressure")
	require.Equal(t, normal.RoutingBreakdown["load"], busyScore.RoutingBreakdown["load"])
	require.Equal(t, normal.RoutingTotal, busyScore.RoutingTotal)
}

func TestSpeedScoreWithoutSamplesIsConservative(t *testing.T) {
	scorer := scheduler.NewWeightedScoreCalculator(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 9}}, core.RuntimeSnapshot{
		SuccessRate:        0.80,
		CostRatio:          0.1,
		GroupPriorityRatio: 1,
		SampleCount:        0,
	}, core.GroupSmartPolicy{})

	require.LessOrEqual(t, score.Breakdown["speed"], 0.45)
	require.Less(t, score.Total, 0.70)
	require.NotContains(t, score.Breakdown, "success")
	require.NotContains(t, score.Breakdown, "speed")
	require.Equal(t, 1.0, score.Breakdown["explore_baseline"])
}

func TestSpeedFactorDoesNotOverwriteDisplayedSpeedScore(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      11,
		Group:          "default",
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        1,
		SuccessScore:       0.2,
		SpeedScore:         0.83,
		CostRatio:          0.1,
		GroupPriorityRatio: 1,
		SampleCount:        12,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{
				Channel:       &model.Channel{Id: 11, Name: "demo"},
				Group:         "default",
				UpstreamModel: "gpt-5.5",
				RuntimeKey:    key,
			},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Len(t, plan.Candidates, 1)
	candidate := plan.Candidates[0]
	require.InEpsilon(t, 0.83, candidate.SpeedScore, 0.0001)
	require.Less(t, candidate.ScoreSpeedFactor, candidate.SpeedScore)
	require.InEpsilon(t, 0.264, candidate.ScoreSpeedFactor, 0.0001)
}

func TestSelectorReusesSimilarRuntimeSnapshotForRealSamples(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	exactKey := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "gpt-5.5",
		ChannelID:             7,
		Group:                 "codex-plus",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "profile-a",
	}
	candidateKey := exactKey
	candidateKey.CapabilityFingerprint = "profile-b"
	store.Put(core.RuntimeSnapshot{
		Key:                exactKey,
		SuccessRate:        0.97,
		SuccessScore:       0.97,
		SpeedScore:         0.81,
		TTFTMs:             1600,
		DurationMs:         3600,
		CostRatio:          0.2,
		GroupPriorityRatio: 1,
		SampleCount:        28,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 7, Name: "hist"}, Group: "codex-plus", RuntimeKey: candidateKey},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "codex-plus",
		CandidateGroups: []string{"codex-plus"},
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	candidate := candidateExplanationByChannel(t, plan.Candidates, 7)
	require.Equal(t, 28, candidate.SampleCount)
	require.Equal(t, "similar", candidate.ScoreSampleSource)
	require.Equal(t, exactKey, candidate.MatchedRuntimeKey)
	require.Equal(t, candidateKey, candidate.RuntimeKey)
	require.Equal(t, 0.97, candidate.SuccessScore)
	require.Equal(t, 0.81, candidate.SpeedScore)
}

func TestSelectorSimilarSnapshotPrefersCapabilityOverGroupOnlyMatch(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	sameGroupLegacy := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      7,
		Group:          "codex-pro",
	}
	sameCapability := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "gpt-5.5",
		ChannelID:             7,
		Group:                 "codex-plus",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: modelgatewayprovider.ProfileOpenAICodex + "|" + modelgatewayprovider.ProxyModeNativeResponses,
	}
	candidateKey := sameCapability
	candidateKey.Group = "codex-pro"
	store.Put(core.RuntimeSnapshot{
		Key:                sameGroupLegacy,
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.80,
		TTFTMs:             4600,
		DurationMs:         5600,
		CostRatio:          0.08,
		GroupPriorityRatio: 1,
		SampleCount:        2,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                sameCapability,
		SuccessRate:        0.84,
		SuccessScore:       0.20,
		SpeedScore:         0.47,
		TTFTMs:             29300,
		DurationMs:         21600,
		CostRatio:          0.08,
		GroupPriorityRatio: 1,
		SampleCount:        188,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{
				Channel:         &model.Channel{Id: 7, Name: "dora"},
				Group:           "codex-pro",
				UpstreamModel:   "gpt-5.5",
				ProviderProfile: modelgatewayprovider.ProfileOpenAICodex,
				ProxyMode:       modelgatewayprovider.ProxyModeNativeResponses,
				RuntimeKey:      candidateKey,
			},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "codex-pro",
		CandidateGroups: []string{"codex-pro"},
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	candidate := candidateExplanationByChannel(t, plan.Candidates, 7)
	require.Equal(t, "similar", candidate.ScoreSampleSource)
	require.Equal(t, sameCapability, candidate.MatchedRuntimeKey)
	require.Equal(t, 188, candidate.SampleCount)
	require.Equal(t, 29300.0, candidate.TTFTMs)
	require.Equal(t, candidate.ScoreBreakdown["speed"], candidate.ScoreSpeedFactor)
	require.Equal(t, 0.47, candidate.SpeedScore)
}

func TestSelectorDoesNotRejectUnsampledCandidateBelowConcurrencyLimit(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      4,
		Group:          "codex-plus",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{
		Key:                       key,
		ActiveConcurrency:         4,
		MaxConcurrency:            40,
		EffectiveConcurrencyLimit: 40,
		CostRatio:                 0.1,
		GroupPriorityRatio:        1,
		SampleCount:               0,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 4, Name: "toioto"}, Group: "codex-plus", RuntimeKey: key},
		}),
		store,
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "codex-plus",
		CandidateGroups: []string{"codex-plus"},
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	candidate := candidateExplanationByChannel(t, plan.Candidates, 4)
	require.True(t, candidate.Available)
	require.Empty(t, candidate.RejectReason)
	require.Equal(t, 4, candidate.ActiveConcurrency)
	require.Equal(t, 40, candidate.EffectiveConcurrencyLimit)
}

func candidateExplanationByChannel(t *testing.T, candidates []core.CandidateExplanation, channelID int) core.CandidateExplanation {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.ChannelID == channelID {
			return candidate
		}
	}
	require.Failf(t, "candidate explanation not found", "channel_id=%d", channelID)
	return core.CandidateExplanation{}
}
