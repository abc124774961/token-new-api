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

type testScorer struct {
	weights core.ScoreWeights
}

func newTestScorer(weights core.ScoreWeights) testScorer {
	return testScorer{weights: weights}
}

func (s testScorer) Score(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.ScoreResult {
	if snapshot.Key.ChannelID == 0 && candidate.Channel != nil {
		snapshot.Key.ChannelID = candidate.Channel.Id
	}
	if snapshot.Key.Group == "" {
		snapshot.Key.Group = candidate.Group
	}
	if snapshot.Key.UpstreamModel == "" {
		snapshot.Key.UpstreamModel = candidate.UpstreamModel
	}
	if snapshot.Key.RequestedModel == "" {
		snapshot.Key.RequestedModel = candidate.RuntimeKey.RequestedModel
	}
	if snapshot.Key.EndpointType == "" {
		snapshot.Key.EndpointType = candidate.RuntimeKey.EndpointType
	}
	if len(policy.CandidateGroups) == 0 && snapshot.Key.Group != "" {
		policy.CandidateGroups = []string{snapshot.Key.Group}
	}
	return scheduler.NewCandidateScoringService().EvaluatePreparedCandidate(candidate, snapshot, policy, scheduler.ScoringContext{
		RequestedModel:  snapshot.Key.RequestedModel,
		EndpointType:    snapshot.Key.EndpointType,
		CandidateGroups: append([]string(nil), policy.CandidateGroups...),
		Strategy:        policy.Strategy,
		AutoMode:        policy.AutoMode,
		ScoreWeights:    s.weights,
		ExplainEnabled:  true,
	}, false).Score
}

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

func TestSelectorKeepsPeerAccountAvailableWhenRuntimeAvoided(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 60
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(7001)
	})

	channel := &model.Channel{Id: 7001, Name: "pooled", Status: common.ChannelStatusEnabled}
	accountA := core.RuntimeKey{
		RequestedModel:      "gpt-5.5",
		UpstreamModel:       "gpt-5.5",
		ChannelID:           channel.Id,
		Group:               "default",
		EndpointType:        constant.EndpointTypeOpenAI,
		AccountID:           "acct-a",
		CredentialIndex:     0,
		CredentialSubjectFP: "subject-a",
		CredentialFP:        "credential-a",
	}
	accountB := accountA
	accountB.AccountID = "acct-b"
	accountB.CredentialIndex = 1
	accountB.CredentialSubjectFP = "subject-b"
	accountB.CredentialFP = "credential-b"

	service.RecordChannelRuntimeFailureAvoidanceWithContext(service.ChannelRuntimeIdentity{
		ChannelID:           accountA.ChannelID,
		RequestedModel:      accountA.RequestedModel,
		SelectedGroup:       accountA.Group,
		EndpointType:        accountA.EndpointType,
		AccountID:           accountA.AccountID,
		CredentialIndex:     accountA.CredentialIndex,
		CredentialIndexSet:  true,
		CredentialSubjectFP: accountA.CredentialSubjectFP,
		CredentialFP:        accountA.CredentialFP,
	}, "upstream_error:502:bad_response_status_code", nil)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	store.Put(core.RuntimeSnapshot{Key: accountA, SuccessRate: 0.99, TTFTMs: 100, DurationMs: 500, SampleCount: 20, GroupPriorityRatio: 1})
	store.Put(core.RuntimeSnapshot{Key: accountB, SuccessRate: 0.95, TTFTMs: 300, DurationMs: 800, SampleCount: 20, GroupPriorityRatio: 1})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: channel, Group: "default", RuntimeKey: accountA},
			{Channel: channel, Group: "default", RuntimeKey: accountB},
		}),
		store,
		core.ScoreWeights{Success: 1, Speed: 0, Load: 0, Cost: 0, Group: 0},
	).WithRuntimeSnapshotEnricher(scheduler.NewRuntimeSnapshotEnricher(scheduler.NewServiceRuntimeStateProvider(), 0, 0, 0))

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, "acct-b", plan.RuntimeKey.AccountID)
	accountAExplanation := candidateExplanationByRuntimeAccount(t, plan.Candidates, "acct-a")
	require.Equal(t, "failure_avoidance", accountAExplanation.RejectReason)
}

func TestSelectorSkipsOnlyFailedRuntimeAccountWithinRequest(t *testing.T) {
	channel := &model.Channel{Id: 7002, Name: "pooled", Status: common.ChannelStatusEnabled}
	accountA := core.RuntimeKey{
		RequestedModel:      "gpt-5.5",
		UpstreamModel:       "gpt-5.5",
		ChannelID:           channel.Id,
		Group:               "default",
		EndpointType:        constant.EndpointTypeOpenAI,
		AccountID:           "acct-a",
		CredentialIndex:     0,
		CredentialSubjectFP: "subject-a",
		CredentialFP:        "credential-a",
	}
	accountB := accountA
	accountB.AccountID = "acct-b"
	accountB.CredentialIndex = 1
	accountB.CredentialSubjectFP = "subject-b"
	accountB.CredentialFP = "credential-b"
	ctx, _ := gin.CreateTestContext(nil)
	service.MarkChannelRuntimeSelectionSkipped(ctx, service.ChannelRuntimeIdentity{
		ChannelID:           accountA.ChannelID,
		RequestedModel:      accountA.RequestedModel,
		SelectedGroup:       accountA.Group,
		EndpointType:        accountA.EndpointType,
		AccountID:           accountA.AccountID,
		CredentialIndex:     accountA.CredentialIndex,
		CredentialIndexSet:  true,
		CredentialSubjectFP: accountA.CredentialSubjectFP,
		CredentialFP:        accountA.CredentialFP,
	})

	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: channel, Group: "default", RuntimeKey: accountA},
			{Channel: channel, Group: "default", RuntimeKey: accountB},
		}),
		nil,
		core.ScoreWeights{Success: 1, Speed: 0, Load: 0, Cost: 0, Group: 0},
	).WithRuntimeSnapshotEnricher(scheduler.NewRuntimeSnapshotEnricher(scheduler.NewServiceRuntimeStateProvider(), 0, 0, 0))

	plan, handled, apiErr := selector.Select(ctx, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, "acct-b", plan.RuntimeKey.AccountID)
	accountAExplanation := candidateExplanationByRuntimeAccount(t, plan.Candidates, "acct-a")
	require.Equal(t, "already_failed_in_request", accountAExplanation.RejectReason)
}

func TestBalancedScorerPrefersHealthyFastCandidate(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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
	require.Greater(t, good.Breakdown["completion_rate"], bad.Breakdown["completion_rate"])
	require.Greater(t, good.Breakdown["ttft_latency"], bad.Breakdown["ttft_latency"])
	require.Equal(t, good.Breakdown["concurrency_load"], bad.Breakdown["concurrency_load"])
	require.Greater(t, good.RoutingBreakdown["concurrency_load"], bad.RoutingBreakdown["concurrency_load"])
}

func TestLowSuccessIsVisibleAsCompletionRateWithoutMutatingLatencyScore(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 10}}, core.RuntimeSnapshot{
		SuccessRate:        0.08,
		TTFTMs:             1200,
		DurationMs:         600,
		CostRatio:          0.12,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.LessOrEqual(t, score.Breakdown["completion_rate"], 0.13)
	require.Greater(t, score.Breakdown["ttft_latency"], 0.90)
	require.Equal(t, 1.0, score.Breakdown["upstream_error_rate"])
	require.Equal(t, 1.0, score.Breakdown["stream_interrupted_rate"])
	require.Less(t, score.Total, 0.85)
	require.Less(t, score.RoutingTotal, 0.85)
}

func TestExperienceAnomalyIsSplitIntoFlatOutputRateItems(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	snapshot := core.RuntimeSnapshot{
		SuccessRate:         0.90,
		TTFTMs:              1200,
		DurationMs:          1800,
		CostRatio:           0.25,
		GroupPriorityRatio:  1,
		SampleCount:         80,
		EmptyOutputRate:     0.80,
		ExperienceIssueRate: 0.30,
	}

	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 11}}, snapshot, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.InEpsilon(t, 0.20, score.Breakdown["empty_output_rate"], 0.000001)
	require.Equal(t, score.Breakdown["concurrency_load"], score.RoutingBreakdown["concurrency_load"])
	require.Contains(t, score.Breakdown, "stream_interrupted_rate")
	require.NotContains(t, score.Breakdown, "experience")
	requireFlatScoreTotal(t, score)
}

func TestLongTermLowSuccessCapsRecentRecoveryScore(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 8}}, core.RuntimeSnapshot{
		SuccessRate:        0.52,
		TTFTMs:             3600,
		DurationMs:         3900,
		CostRatio:          0.13,
		GroupPriorityRatio: 1,
		SampleCount:        626,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Less(t, score.Breakdown["completion_rate"], 0.65)
	require.Greater(t, score.Breakdown["completion_rate"], 0.50)
	require.Equal(t, 1.0, score.Breakdown["upstream_error_rate"])
	require.Equal(t, 1.0, score.Breakdown["stream_interrupted_rate"])
	require.Less(t, score.Total, 0.90)
	require.Less(t, score.RoutingTotal, 0.90)
}

func TestScoreFactoryStrategiesShiftWeights(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "default"}
	fastExpensive := core.RuntimeSnapshot{
		SuccessRate:        0.95,
		TTFTMs:             350,
		TokensPerSecond:    80,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          3,
		CostReferenceRatio: 0.5,
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
		CostReferenceRatio: 0.5,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	}

	speedFast := scorer.Score(candidate, fastExpensive, core.GroupSmartPolicy{Strategy: core.StrategySpeedFirst})
	speedCheap := scorer.Score(candidate, slowCheap, core.GroupSmartPolicy{Strategy: core.StrategySpeedFirst})
	require.Greater(t, speedFast.Total, speedCheap.Total)

	costFast := scorer.Score(candidate, fastExpensive, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	costCheap := scorer.Score(candidate, slowCheap, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	require.Greater(t, costCheap.Total, costFast.Total)
}

func TestCostFirstPrefersCheapCandidateOverFasterExpensiveCandidate(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	cheapPlus := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		TTFTMs:             7200,
		DurationMs:         5800,
		CostRatio:          0.13,
		CostReferenceRatio: 0.13,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	fastPro := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		TTFTMs:             3200,
		DurationMs:         3600,
		CostRatio:          0.30,
		CostReferenceRatio: 0.13,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})

	require.Greater(t, cheapPlus.Breakdown["cost"], fastPro.Breakdown["cost"])
	require.Greater(t, cheapPlus.Total, fastPro.Total)
	require.Greater(t, cheapPlus.RoutingTotal, fastPro.RoutingTotal)
}

func TestCostFirstGuardBlocksExpensiveScoreWinner(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 31, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	expensiveKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 32, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.97,
		TTFTMs:             1200,
		DurationMs:         3200,
		TokensPerSecond:    45,
		CostRatio:          0.02,
		GroupPriorityRatio: 1,
		SampleCount:        80,
		ScoreStatsJSON:     `{"version":1,"samples":80,"rates":{"completion":{"success":0,"total":80},"upstream_error":{"success":0,"total":80},"empty_output":{"success":0,"total":80},"stream_interrupted":{"success":0,"total":80}}}`,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                expensiveKey,
		SuccessRate:        0.98,
		TTFTMs:             1200,
		DurationMs:         3000,
		TokensPerSecond:    45,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        80,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 31, Name: "cheap"}, Group: "default", RuntimeKey: cheapKey},
			{Channel: &model.Channel{Id: 32, Name: "expensive"}, Group: "default", RuntimeKey: expensiveKey},
		}),
		store,
		core.ScoreWeights{Success: 0.8, Speed: 0.2, Load: 0, Cost: 0, Group: 0},
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 31, plan.Channel.Id)
	require.Equal(t, "cost_first_guard_baseline_selected", plan.SelectedReason)
	require.NotNil(t, plan.CostGuardDecision)
	require.Equal(t, "baseline", plan.CostGuardDecision.Decision)
	require.InEpsilon(t, 2.5, plan.CostGuardDecision.CostMultiple, 0.0001)
	require.InEpsilon(t, 1.8, plan.CostGuardDecision.CostGuardMultiple, 0.0001)
	expensive := candidateExplanationByChannel(t, plan.Candidates, 32)
	require.Equal(t, "baseline", expensive.CostGuardDecision)
	require.Equal(t, "cost_first_guard_baseline_selected", expensive.CostGuardReason)
}

func TestCostFirstGuardAllowsExpensiveQualityOverride(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 33, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	expensiveKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 34, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.94,
		TTFTMs:             1300,
		DurationMs:         3600,
		TokensPerSecond:    40,
		CostRatio:          0.02,
		GroupPriorityRatio: 1,
		SampleCount:        80,
		ScoreStatsJSON:     `{"version":1,"samples":80,"rates":{"completion":{"success":0,"total":80},"upstream_error":{"success":0,"total":80},"empty_output":{"success":0,"total":80},"stream_interrupted":{"success":0,"total":80}}}`,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                expensiveKey,
		SuccessRate:        0.99,
		TTFTMs:             900,
		DurationMs:         2600,
		TokensPerSecond:    55,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        80,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 33, Name: "cheap"}, Group: "default", RuntimeKey: cheapKey},
			{Channel: &model.Channel{Id: 34, Name: "expensive"}, Group: "default", RuntimeKey: expensiveKey},
		}),
		store,
		core.ScoreWeights{Success: 0.8, Speed: 0.2, Load: 0, Cost: 0, Group: 0},
	)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 34, plan.Channel.Id)
	require.NotNil(t, plan.CostGuardDecision)
	require.Equal(t, "override", plan.CostGuardDecision.Decision)
	require.Equal(t, "cost_first_guard_quality_override", plan.CostGuardDecision.Reason)
	require.GreaterOrEqual(t, plan.CostGuardDecision.SuccessDelta, 0.03)
}

func TestCostItemUsesCandidateMinimumCostReference(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	lowest := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		CostRatio:          8,
		CostReferenceRatio: 8,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	expensive := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		CostRatio:          20,
		CostReferenceRatio: 8,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})

	require.Equal(t, 1.0, lowest.Breakdown["cost"])
	require.InEpsilon(t, 0.4307, expensive.Breakdown["cost"], 0.0001)
	require.Greater(t, lowest.Total, expensive.Total)
	require.Greater(t, lowest.RoutingTotal, expensive.RoutingTotal)
}

func TestCostFirstAmplifiesLargeRelativeCostGap(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	cheap := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.96,
		CostRatio:          0.4091,
		CostReferenceRatio: 0.4091,
		GroupPriorityRatio: 0.1,
		SampleCount:        84,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	expensive := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.96,
		CostRatio:          0.8838,
		CostReferenceRatio: 0.4091,
		GroupPriorityRatio: 0.1,
		SampleCount:        83,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})

	require.Equal(t, 1.0, cheap.Breakdown["cost"])
	require.InEpsilon(t, 0.5214, expensive.Breakdown["cost"], 0.0001)
	require.Greater(t, cheap.Total, expensive.Total)
	require.Greater(t, cheap.RoutingTotal, expensive.RoutingTotal)
}

func TestCostItemSkipsMissingReference(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	score := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		CostRatio:          0.13,
		GroupPriorityRatio: 1,
		SampleCount:        100,
	}, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})

	require.NotContains(t, score.Breakdown, "cost")
	require.True(t, score.CostReferenceMissing)
}

func TestCostFirstUsesConfiguredGroupWeightAndLowerGroupRatio(t *testing.T) {
	scorer := newTestScorer(core.ScoreWeights{
		Success: 0.32,
		Speed:   0.28,
		Load:    0.10,
		Cost:    0.15,
		Group:   0.30,
	})
	candidate := core.Candidate{Channel: &model.Channel{Id: 1}, Group: "auto"}
	cheapPreferredGroup := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        0.99,
		TTFTMs:             6800,
		DurationMs:         7200,
		CostRatio:          0.12,
		CostReferenceRatio: 0.12,
		GroupPriorityRatio: 0.1,
		SampleCount:        120,
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
		TTFTMs:             1600,
		DurationMs:         2600,
		CostRatio:          0.24,
		CostReferenceRatio: 0.12,
		GroupPriorityRatio: 0.2,
		SampleCount:        120,
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
	require.Greater(t, cheapPreferredGroup.Breakdown["group_priority"], fastLowPriorityGroup.Breakdown["group_priority"])
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
		TTFTMs:                    3200,
		DurationMs:                5000,
		ActiveConcurrency:         10,
		EffectiveConcurrencyLimit: 0,
		CostRatio:                 0.05,
		GroupPriorityRatio:        0.1,
		SampleCount:               120,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                fastProKey,
		SuccessRate:        1,
		TTFTMs:             1200,
		DurationMs:         2600,
		ActiveConcurrency:  0,
		CostRatio:          0.12,
		GroupPriorityRatio: 0.2,
		SampleCount:        120,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 4, Name: "toioto-0.05x"}, Group: "codex-plus", UpstreamModel: "gpt-5.5", RuntimeKey: cheapPlusKey},
			{Channel: &model.Channel{Id: 9, Name: "maxtopai-pro"}, Group: "codex-pro", UpstreamModel: "gpt-5.5", RuntimeKey: fastProKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithRuntimeSnapshotEnricher(
		scheduler.NewRuntimeSnapshotEnricher(nil, 1500, 8, 2).WithCostProfileProvider(fakeCostProfileProvider{
			ratiosByChannel: map[int]float64{
				4: 0.05,
				9: 0.12,
			},
			reference: 0.05,
			ok:        true,
		}),
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
	require.Equal(t, 0.05, cheapPlus.CostReferenceRatio)
	require.Equal(t, 0.05, fastPro.CostReferenceRatio)
	require.Equal(t, 1.0, cheapPlus.ScoreBreakdown["cost"])
	require.InEpsilon(t, 0.456, fastPro.ScoreBreakdown["cost"], 0.0001)
	require.Greater(t, cheapPlus.RoutingScoreTotal, fastPro.RoutingScoreTotal)
	require.Equal(t, cheapPlus.ScoreBreakdown["concurrency_load"], cheapPlus.RoutingScoreBreakdown["concurrency_load"])
	require.Contains(t, cheapPlus.RoutingScoreBreakdown, "first_byte_backlog")
}

func TestFirstByteRetryIntentPrefersFastSuccessfulRecoveryChannel(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	cheapSlowKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 21, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI}
	fastStableKey := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 22, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                cheapSlowKey,
		SuccessRate:        0.90,
		TTFTMs:             9800,
		DurationMs:         15000,
		TokensPerSecond:    20,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        60,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                fastStableKey,
		SuccessRate:        0.99,
		TTFTMs:             420,
		DurationMs:         2200,
		TokensPerSecond:    70,
		CostRatio:          0.18,
		GroupPriorityRatio: 1,
		SampleCount:        60,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 21, Name: "cheap-slow"}, Group: "codex-plus", UpstreamModel: "gpt-5.5", RuntimeKey: cheapSlowKey},
			{Channel: &model.Channel{Id: 22, Name: "fast-stable"}, Group: "codex-plus", UpstreamModel: "gpt-5.5", RuntimeKey: fastStableKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	)
	policy := core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "codex-plus",
		CandidateGroups: []string{"codex-plus"},
		Strategy:        core.StrategyCostFirst,
		QueueEnabled:    true,
		GroupPriorityRatio: map[string]float64{
			"codex-plus": 1,
		},
	}

	normalPlan, handled, apiErr := selector.Select(nil, &service.RetryParam{TokenGroup: "codex-plus", ModelName: "gpt-5.5", EndpointType: constant.EndpointTypeOpenAI}, policy)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, 21, normalPlan.Channel.Id)

	ctx, _ := gin.CreateTestContext(nil)
	core.SetRetryRoutingIntent(ctx, core.NewFirstByteTimeoutRetryRoutingIntent(21, "cheap-slow", 0))
	recoveryPlan, handled, apiErr := selector.Select(ctx, &service.RetryParam{Ctx: ctx, TokenGroup: "codex-plus", ModelName: "gpt-5.5", EndpointType: constant.EndpointTypeOpenAI}, policy)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, 22, recoveryPlan.Channel.Id)
	require.True(t, recoveryPlan.RetryIntentApplied)
	require.True(t, recoveryPlan.RetryQueuePriorityBoost)
	require.Equal(t, core.RetryRoutingQueuePriority, recoveryPlan.QueuePriority)
	require.Equal(t, "score_items_retry_intent", recoveryPlan.SelectedReason)
	require.Equal(t, core.RelayAttemptCancelReasonFirstByteTimeout, recoveryPlan.RetryRoutingIntent.Reason)
	fastStable := candidateExplanationByChannel(t, recoveryPlan.Candidates, 22)
	require.True(t, fastStable.RetryIntentApplied)
	require.Contains(t, fastStable.RoutingScoreBreakdown, "retry_intent_recovery")
	require.Greater(t, fastStable.RoutingScoreBreakdown["retry_intent_recovery"], 0.95)
	cheapSlow := candidateExplanationByChannel(t, recoveryPlan.Candidates, 21)
	require.Less(t, cheapSlow.RoutingScoreBreakdown["retry_intent_recovery"], fastStable.RoutingScoreBreakdown["retry_intent_recovery"])
}

func TestConcurrencyLoadItemHardDropsCircuitOpenSnapshot(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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

	require.Zero(t, score.Breakdown["concurrency_load"])
	require.Zero(t, score.RoutingBreakdown["concurrency_load"])
}

func TestStableScoreIgnoresTransientConcurrency(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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
	require.Equal(t, normal.Breakdown["concurrency_load"], busyScore.Breakdown["concurrency_load"])
	require.Greater(t, normal.RoutingTotal, busyScore.RoutingTotal)
	require.Less(t, busyScore.RoutingBreakdown["concurrency_load"], normal.RoutingBreakdown["concurrency_load"])
	require.Contains(t, busyScore.RoutingBreakdown, "first_byte_backlog")
	require.Contains(t, busyScore.Breakdown, "first_byte_backlog")
}

func TestRoutingConcurrencyLoadItemTreatsLowConfiguredUsageAsHealthy(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 8}, Group: "codex-plus"}
	score := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:                0.99,
		TTFTMs:                     3330,
		DurationMs:                 3770,
		ActiveConcurrency:          3,
		ConfiguredConcurrencyLimit: 46,
		CostRatio:                  0.06,
		GroupPriorityRatio:         1,
		SampleCount:                120,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.GreaterOrEqual(t, score.RoutingBreakdown["concurrency_load"], 0.99)
	require.Contains(t, score.RoutingBreakdown, "first_byte_backlog")
}

func TestRoutingConcurrencyLoadItemDropsNearConcurrencyLimit(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 8}, Group: "codex-plus"}
	score := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:                0.99,
		TTFTMs:                     3330,
		DurationMs:                 3770,
		ActiveConcurrency:          46,
		ConfiguredConcurrencyLimit: 46,
		CostRatio:                  0.06,
		GroupPriorityRatio:         1,
		SampleCount:                120,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Less(t, score.RoutingBreakdown["concurrency_load"], 0.50)
	require.Contains(t, score.RoutingBreakdown, "first_byte_backlog")
}

func TestCostFirstRoutingLoadOnlyPenalizesExceededConcurrencyLimit(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 4}, Group: "codex-plus"}
	base := core.RuntimeSnapshot{
		SuccessRate:                0.99,
		TTFTMs:                     3500,
		DurationMs:                 4600,
		ActiveConcurrency:          3,
		EffectiveConcurrencyLimit:  40,
		ConfiguredConcurrencyLimit: 40,
		CostRatio:                  0.05,
		GroupPriorityRatio:         0.1,
		SampleCount:                200,
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	require.Equal(t, 1.0, normal.RoutingBreakdown["concurrency_load"])
	require.Contains(t, normal.RoutingBreakdown, "first_byte_backlog")

	overLimit := base
	overLimit.ActiveConcurrency = 90
	busy := scorer.Score(candidate, overLimit, core.GroupSmartPolicy{Strategy: core.StrategyCostFirst})
	require.Less(t, busy.RoutingBreakdown["concurrency_load"], normal.RoutingBreakdown["concurrency_load"])
	require.Contains(t, busy.RoutingBreakdown, "first_byte_backlog")
	require.Less(t, busy.RoutingTotal, normal.RoutingTotal)
	require.Equal(t, normal.Breakdown["concurrency_load"], busy.Breakdown["concurrency_load"])
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
		scheduler.DefaultScoreWeights(),
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
		scheduler.DefaultScoreWeights(),
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
		CostRatio:                 0.12,
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
		scheduler.DefaultScoreWeights(),
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
	require.NotContains(t, first.ScoreBreakdown, "completion_rate")
	require.NotContains(t, first.ScoreBreakdown, "ttft_latency")
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
		scheduler.DefaultScoreWeights(),
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
	require.Contains(t, selected.ScoreBreakdown, "completion_rate")
	require.Equal(t, selectedKey, selected.RuntimeKey)
}

func TestSelectorIgnoresCodexImageToolDispatchRequirement(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "gpt-5.5",
		ChannelID:             10,
		Group:                 "default",
		EndpointType:          constant.EndpointTypeOpenAIResponse,
		CapabilityFingerprint: "openai_responses_image",
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        0.99,
		TTFTMs:             300,
		TokensPerSecond:    80,
		ActiveConcurrency:  0,
		MaxConcurrency:     4,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{
				Channel:         &model.Channel{Id: 10, Name: "image-capable"},
				Group:           "default",
				UpstreamModel:   "gpt-5.5",
				ProviderProfile: "openai_responses",
				ProxyMode:       "native",
				RuntimeKey:      key,
			},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	)

	plan, handled, apiErr := selector.Select(nil, &service.RetryParam{
		TokenGroup:             "default",
		ModelName:              "gpt-5.5",
		EndpointType:           constant.EndpointTypeOpenAIResponse,
		RequiresCodexImageTool: true,
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.False(t, plan.RequiresCodexImageTool)
	require.Empty(t, plan.RequiredTools)
	require.Empty(t, plan.CandidateFilterConditions)
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
		scheduler.DefaultScoreWeights(),
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
	require.Less(t, plan.ScoreTotal, 0.80)
	require.Len(t, plan.Candidates, 1)
	candidate := plan.Candidates[0]
	require.Equal(t, 3, candidate.SampleCount)
	require.Equal(t, 45000.0, candidate.TTFTMs)
	require.Less(t, candidate.ScoreTotal, 0.80)
	require.Contains(t, candidate.ScoreBreakdown, "ttft_latency")
	require.Zero(t, candidate.ScoreBreakdown["ttft_latency"])
}

func TestSelectorCarriesAccountCredentialFieldsToPlanAndExplanation(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel:      "gpt-5.4",
		UpstreamModel:       "gpt-5.4",
		ChannelID:           44,
		ResourceID:          "platform:channel:44",
		ResourceType:        core.ResourceTypePlatformOwned,
		AccountID:           "openai:openai:acct-a",
		AccountType:         core.AccountTypeAPIKey,
		Brand:               "openai",
		Provider:            "openai",
		CredentialIndex:     1,
		CredentialSubjectFP: "subject-a",
		CredentialFP:        "credential-a",
		Group:               "default",
		EndpointType:        constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        1,
		TTFTMs:             200,
		DurationMs:         300,
		GroupPriorityRatio: 1,
		SampleCount:        5,
	})
	candidate := core.Candidate{
		Channel:     &model.Channel{Id: 44, Name: "accounted"},
		ResourceRef: core.ResourceRef{ResourceID: key.ResourceID, ResourceType: key.ResourceType, ExecutionBindingID: 44, Provider: "openai", Brand: "openai"},
		AccountIdentity: core.AccountIdentity{
			AccountID:                    key.AccountID,
			AccountType:                  key.AccountType,
			Brand:                        key.Brand,
			Provider:                     key.Provider,
			CredentialIndex:              key.CredentialIndex,
			CredentialSubjectFingerprint: key.CredentialSubjectFP,
			CredentialFingerprint:        key.CredentialFP,
		},
		CredentialRef: core.CredentialRef{
			ResourceID:                   key.ResourceID,
			AccountID:                    key.AccountID,
			CredentialIndex:              key.CredentialIndex,
			CredentialSubjectFingerprint: key.CredentialSubjectFP,
			CredentialFingerprint:        key.CredentialFP,
			Resolver:                     "channel_key",
		},
		Group:         "default",
		UpstreamModel: "gpt-5.4",
		RuntimeKey:    key,
		PoolLevel:     core.CandidatePoolPro,
	}
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{candidate}),
		store,
		scheduler.DefaultScoreWeights(),
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
	require.Equal(t, candidate.ResourceRef, plan.ResourceRef)
	require.Equal(t, candidate.AccountIdentity, plan.AccountIdentity)
	require.Equal(t, candidate.CredentialRef, plan.CredentialRef)
	require.Equal(t, core.CandidatePoolPro, plan.PoolLevel)
	require.Len(t, plan.Candidates, 1)
	require.True(t, plan.Candidates[0].Selected)
	require.Equal(t, key.AccountID, plan.Candidates[0].AccountID)
	require.Equal(t, key.CredentialFP, plan.Candidates[0].CredentialFP)
	require.Equal(t, core.CandidatePoolPro, plan.Candidates[0].PoolLevel)
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
		TTFTMs:             900,
		DurationMs:         1500,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        5,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                enrichedKey,
		SuccessRate:        0.998,
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
		scheduler.DefaultScoreWeights(),
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
	require.InEpsilon(t, 0.1697, candidate.ScoreBreakdown["ttft_latency"], 0.0001)
}

func TestSelectorUsesRoutingScoreButReportsStableScore(t *testing.T) {
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
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "busy-healthy"}, Group: "default", RuntimeKey: busyKey},
			{Channel: &model.Channel{Id: 2, Name: "free-good"}, Group: "default", RuntimeKey: freeKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
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
	require.Equal(t, 1.0, busy.ScoreBreakdown["concurrency_load"])
	require.Less(t, busy.RoutingScoreBreakdown["concurrency_load"], free.RoutingScoreBreakdown["concurrency_load"])
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
		scheduler.DefaultScoreWeights(),
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
		scheduler.DefaultScoreWeights(),
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
		scheduler.DefaultScoreWeights(),
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
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 4, Name: "toioto-0.05x"}, Group: "pro", RuntimeKey: saturatedKey},
			{Channel: &model.Channel{Id: 2, Name: "backup"}, Group: "plus", RuntimeKey: availableKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
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
	require.Contains(t, saturated.RoutingScoreBreakdown, "first_byte_backlog")
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
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 3, Name: "chaozhi"}, Group: "codex-plus", RuntimeKey: learnedKey},
			{Channel: &model.Channel{Id: 6, Name: "b886"}, Group: "codex-pro", RuntimeKey: backupKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
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
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
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
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Greater(t, fast.Total, slowCheap.Total)
	require.Greater(t, fast.Breakdown["ttft_latency"], slowCheap.Breakdown["ttft_latency"])
	require.Less(t, slowCheap.Total, 0.65)
}

func TestHighTTFTDirectlyLowersTTFTLatencyItem(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Greater(t, normal.Total, slowFirstByte.Total)
	require.Less(t, slowFirstByte.Breakdown["ttft_latency"], 0.28)
}

func TestFirstBytePendingAppliesDynamicPenalty(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	pending := base
	pending.FirstBytePending = 3
	pending.SlowFirstBytePending = 2
	pending.OldestFirstByteWaitMs = 15000
	pendingScore := scorer.Score(candidate, pending, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Equal(t, normal.Total, pendingScore.Total)
	require.Contains(t, pendingScore.RoutingBreakdown, "first_byte_backlog")
	require.Contains(t, pendingScore.Breakdown, "first_byte_backlog")
	require.Less(t, pendingScore.RoutingBreakdown["concurrency_load"], normal.RoutingBreakdown["concurrency_load"])
	require.Less(t, pendingScore.RoutingTotal, normal.RoutingTotal)
}

func TestUnknownLimitActiveConcurrencyDoesNotAffectRoutingScore(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
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
	}
	normal := scorer.Score(candidate, base, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})
	busy := base
	busy.ActiveConcurrency = 10
	busyScore := scorer.Score(candidate, busy, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Equal(t, normal.Total, busyScore.Total)
	require.Contains(t, busyScore.Breakdown, "first_byte_backlog")
	require.Contains(t, busyScore.RoutingBreakdown, "first_byte_backlog")
	require.Equal(t, normal.RoutingBreakdown["concurrency_load"], busyScore.RoutingBreakdown["concurrency_load"])
	require.Equal(t, normal.RoutingTotal, busyScore.RoutingTotal)
}

func TestTTFTLatencyWithoutSamplesIsMissing(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	score := scorer.Score(core.Candidate{Channel: &model.Channel{Id: 9}}, core.RuntimeSnapshot{
		SuccessRate:        0.80,
		CostRatio:          0.1,
		GroupPriorityRatio: 1,
		SampleCount:        0,
	}, core.GroupSmartPolicy{})

	require.NotContains(t, score.Breakdown, "ttft_latency")
	require.NotContains(t, score.Breakdown, "completion_rate")
	require.NotContains(t, score.Breakdown, "speed")
	require.NotContains(t, score.Breakdown, "explore_baseline")
	require.False(t, scoreHasMissingReason(score.Items, "ttft_latency", ""))
	require.True(t, scoreHasMissingReason(score.Items, "ttft_latency", "sample_missing"))
}

func TestSingleSlowSampleKeepsFlatLatencyPenaltyBounded(t *testing.T) {
	scorer := newTestScorer(scheduler.DefaultScoreWeights())
	candidate := core.Candidate{Channel: &model.Channel{Id: 9}, Group: "default"}

	score := scorer.Score(candidate, core.RuntimeSnapshot{
		SuccessRate:        1,
		TTFTMs:             19260,
		DurationMs:         26410,
		CostRatio:          0.1,
		CostReferenceRatio: 0.1,
		GroupPriorityRatio: 1,
		SampleCount:        1,
	}, core.GroupSmartPolicy{Strategy: core.StrategyBalanced})

	require.Less(t, score.Breakdown["ttft_latency"], 0.05)
	require.Greater(t, score.Total, 0.45)
}

func TestMissingLatencyDoesNotCreateTTFTScoreItem(t *testing.T) {
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
		scheduler.DefaultScoreWeights(),
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
	require.NotContains(t, candidate.ScoreBreakdown, "ttft_latency")
	require.NotContains(t, candidate.ScoreBreakdown, "speed")
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
		scheduler.DefaultScoreWeights(),
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
	require.Equal(t, 0.97, candidate.ScoreBreakdown["completion_rate"])
	require.InEpsilon(t, 0.9015, candidate.ScoreBreakdown["ttft_latency"], 0.0001)
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
		TTFTMs:             4600,
		DurationMs:         5600,
		CostRatio:          0.08,
		GroupPriorityRatio: 1,
		SampleCount:        2,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                sameCapability,
		SuccessRate:        0.84,
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
		scheduler.DefaultScoreWeights(),
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
	require.Less(t, candidate.ScoreBreakdown["ttft_latency"], 0.05)
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
		scheduler.DefaultScoreWeights(),
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

func candidateExplanationByRuntimeAccount(t *testing.T, candidates []core.CandidateExplanation, accountID string) core.CandidateExplanation {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.RuntimeKey.AccountID == accountID || candidate.AccountID == accountID {
			return candidate
		}
	}
	require.Failf(t, "candidate explanation not found", "account_id=%s", accountID)
	return core.CandidateExplanation{}
}

func requireFlatScoreTotal(t *testing.T, score core.ScoreResult) {
	t.Helper()
	expected := 0.0
	for _, item := range score.Items {
		if item.MissingReason != "" || item.Weight <= 0 {
			continue
		}
		expected += item.Score * item.Weight
	}
	require.InEpsilon(t, round4ForScorerTest(expected), score.Total, 0.0002)
}

func scoreHasMissingReason(items []core.ScoreItem, key string, reason string) bool {
	for _, item := range items {
		if item.Key != key {
			continue
		}
		return item.MissingReason == reason
	}
	return false
}

func round4ForScorerTest(value float64) float64 {
	if value < 0 {
		return -round4ForScorerTest(-value)
	}
	return float64(int(value*10000+0.5)) / 10000
}
