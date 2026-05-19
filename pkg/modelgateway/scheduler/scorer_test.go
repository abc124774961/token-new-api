package scheduler_test

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
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
