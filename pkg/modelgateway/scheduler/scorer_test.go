package scheduler_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
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

func TestBalancedScorerPrefersHealthyFastLowLoadCandidate(t *testing.T) {
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
	require.Greater(t, good.Breakdown["success"], bad.Breakdown["success"])
	require.Greater(t, good.Breakdown["speed"], bad.Breakdown["speed"])
	require.Greater(t, good.Breakdown["load"], bad.Breakdown["load"])
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
}

func TestSelectorSkipsFullConcurrencyCandidateWhenQueueDisabled(t *testing.T) {
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
}

func TestSelectorSkipsRequestLocalConcurrencyMarkedCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	service.MarkChannelConcurrencySkipped(ctx, 1)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        1,
		ActiveConcurrency:  0,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.5,
		ActiveConcurrency:  0,
		MaxConcurrency:     10,
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
	require.Equal(t, "local_concurrency_full", skipped.RejectReason)
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
	require.Equal(t, 1, plan.Channel.Id)
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
