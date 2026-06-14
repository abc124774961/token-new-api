package scheduler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSelectorRetainsHealthyStickyRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-retain-healthy"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 42)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		StickyKeepScoreRatio: 0.85,
	}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        0.92,
		TTFTMs:             600,
		TokensPerSecond:    50,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.96,
		TTFTMs:             450,
		TokensPerSecond:    70,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.StickyRetained)
	require.Equal(t, "user_sticky", plan.StickySource)
	require.Equal(t, "user_sticky_retained", plan.SelectedReason)
	require.Empty(t, plan.StickyBreak)
}

func TestSelectorBreaksStickyRouteOnCooldown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-break-cooldown"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 43)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        1,
		TTFTMs:             300,
		TokensPerSecond:    80,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		Cooldown:           true,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.82,
		TTFTMs:             900,
		TokensPerSecond:    30,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "cooldown", plan.StickyBreak)
	require.Equal(t, "score_items_sticky_broken", plan.SelectedReason)
}

func TestSelectorRejectsConfigIsolatedStickyCandidateAndUsesAlternative(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-config-isolated"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 143)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	peerKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                   stickyKey,
		SuccessRate:           1,
		TTFTMs:                300,
		TokensPerSecond:       80,
		CostRatio:             1,
		GroupPriorityRatio:    1,
		ConfigErrorIsolated:   true,
		IsolationReason:       "auth_config_error",
		IsolationUntil:        1770000000,
		AuthConfigErrorCount:  2,
		LastAuthConfigErrorAt: 1769999900,
		SampleCount:           20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                peerKey,
		SuccessRate:        0.82,
		TTFTMs:             900,
		TokensPerSecond:    30,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: peerKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "config_error_isolated", plan.StickyBreak)
	require.Equal(t, "score_items_sticky_broken", plan.SelectedReason)

	isolated := candidateExplanationByChannel(t, plan.Candidates, 1)
	require.False(t, isolated.Available)
	require.Equal(t, "config_error_isolated", isolated.RejectReason)
	require.True(t, isolated.ConfigErrorIsolated)
	require.Equal(t, "auth_config_error", isolated.IsolationReason)
	require.EqualValues(t, 1770000000, isolated.IsolationUntil)
	require.Equal(t, 2, isolated.AuthConfigErrorCount)
	require.EqualValues(t, 1769999900, isolated.LastAuthConfigErrorAt)
}

func TestSelectorDoesNotBreakStickyRouteBelowConcurrencyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-local-pressure"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 44)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        1,
		TTFTMs:             300,
		TokensPerSecond:    80,
		ActiveConcurrency:  1,
		MaxConcurrency:     2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.75,
		TTFTMs:             900,
		TokensPerSecond:    30,
		ActiveConcurrency:  0,
		MaxConcurrency:     2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
		QueueEnabled:    false,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.StickyRetained)
	require.Empty(t, plan.StickyBreak)
}

func TestSelectorBreaksSaturatedStickyRouteWhenPeerHasCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-saturated-sticky"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 47)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	peerKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        1,
		TTFTMs:             300,
		TokensPerSecond:    80,
		ActiveConcurrency:  2,
		MaxConcurrency:     2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                peerKey,
		SuccessRate:        0.75,
		TTFTMs:             900,
		TokensPerSecond:    30,
		ActiveConcurrency:  0,
		MaxConcurrency:     2,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: peerKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
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
	require.False(t, plan.StickyRetained)
	require.Equal(t, "concurrency_saturated", plan.StickyBreak)
	require.Equal(t, "score_items_sticky_broken", plan.SelectedReason)
}

func TestSelectorBreaksStickyRouteForCostFirstCheaperHigherScore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-cost-first-escape"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 48)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.98,
		TTFTMs:             600,
		TokensPerSecond:    60,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.98,
		TTFTMs:             550,
		TokensPerSecond:    64,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: cheapKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "cost_first_cheaper_speed_acceptable", plan.StickyBreak)
	require.NotNil(t, plan.StickyDecision)
	require.Equal(t, "score_items_sticky_broken", plan.SelectedReason)
}

func TestSelectorRetainsStickyRouteForCostFirstSmallCostGap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-cost-first-small-gap"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 49)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.98,
		TTFTMs:             600,
		TokensPerSecond:    60,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.40,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.98,
		TTFTMs:             550,
		TokensPerSecond:    64,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.32,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: cheapKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.StickyRetained)
	require.Equal(t, "user_sticky_retained", plan.SelectedReason)
	require.Empty(t, plan.StickyBreak)
	require.NotNil(t, plan.StickyDecision)
	require.Equal(t, "retain", plan.StickyDecision.Decision)
	require.Equal(t, "cost_first_sticky_escape_cost_gap_insufficient", plan.StickyDecision.Reason)
	require.InEpsilon(t, 0.8, plan.StickyDecision.CostRatio, 0.001)
}

func TestSelectorRetainsStickyRouteForCostFirstWhenCheapCandidateIsTooSlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-cost-first-slow-cheap"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 149)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.98,
		TTFTMs:             600,
		DurationMs:         6000,
		TokensPerSecond:    60,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.98,
		TTFTMs:             15000,
		DurationMs:         60000,
		TokensPerSecond:    64,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: cheapKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.StickyRetained)
	require.Empty(t, plan.StickyBreak)
	require.NotNil(t, plan.StickyDecision)
	require.Equal(t, "retain", plan.StickyDecision.Decision)
	require.Equal(t, "cost_first_sticky_escape_speed_drop_exceeded", plan.StickyDecision.Reason)
}

func TestSelectorBreaksStickyRouteForCostFirstWhenThroughputMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-cost-first-no-throughput"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 150)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.98,
		TTFTMs:             600,
		DurationMs:         6000,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.98,
		TTFTMs:             650,
		DurationMs:         6200,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: cheapKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "cost_first_cheaper_speed_acceptable", plan.StickyBreak)
	require.NotNil(t, plan.StickyDecision)
	require.Equal(t, "switch", plan.StickyDecision.Decision)
}

func TestSelectorRetainsCacheAffinityForCostFirstUnlessCostGapIsLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 1, Group: "default"}
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.98,
		TTFTMs:             650,
		TokensPerSecond:    55,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.40,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.98,
		TTFTMs:             520,
		TokensPerSecond:    70,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.24,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		CacheKeepScoreRatio: 0.75,
	}, scheduler.NewStaticCacheAffinitySignalAdapter(core.CacheAffinitySignal{
		Key:                "fake-cache-key",
		KeyFingerprint:     "cost-affinity-1",
		Source:             "fake",
		PreferredChannelID: 1,
		PreferredGroup:     "default",
	}, true))
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: cheapKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5",
	}, core.GroupSmartPolicy{
		Mode:                 core.ModeActive,
		RequestedGroup:       "default",
		CandidateGroups:      []string{"default"},
		Strategy:             core.StrategyCostFirst,
		CacheAffinityEnabled: true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.CacheAffinity)
	require.True(t, plan.StickyRetained)
	require.Equal(t, "cache_affinity_retained", plan.SelectedReason)
	require.Empty(t, plan.StickyBreak)
}

func TestSelectorBreaksCostFirstStickyWhenGuardSelectsCheapBaseline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-cost-first-guard"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 151)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	cheapKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.98,
		TTFTMs:             800,
		DurationMs:         3000,
		TokensPerSecond:    45,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                cheapKey,
		SuccessRate:        0.97,
		TTFTMs:             950,
		DurationMs:         3300,
		TokensPerSecond:    42,
		ActiveConcurrency:  1,
		MaxConcurrency:     10,
		CostRatio:          0.02,
		GroupPriorityRatio: 1,
		SampleCount:        30,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "sticky-expensive"}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2, Name: "cheap"}, Group: "default", RuntimeKey: cheapKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyCostFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "cost_first_cheaper_speed_acceptable", plan.StickyBreak)
	require.Equal(t, "score_items_sticky_broken", plan.SelectedReason)
	require.NotNil(t, plan.StickyDecision)
	require.Equal(t, "switch", plan.StickyDecision.Decision)
	require.InEpsilon(t, 0.4, plan.StickyDecision.CostRatio, 0.0001)
}

func TestStickyRouterSharesStoreAcrossInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-shared-store"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 45)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	store := scheduler.NewMemoryStickyStore(8)
	writer := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)
	reader := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)

	writer.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 7},
		SelectedGroup: "default",
	})

	route, ok := reader.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 7, route.ChannelID)
	require.Equal(t, "default", route.Group)
	require.Equal(t, "user_sticky", route.Source)
	require.NotEmpty(t, route.KeyFingerprint)
}

func TestStickyRouterRequiresExplicitSessionSignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 46)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: scheduler.NewMemoryStickyStore(8),
	}, nil)

	router.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 8},
		SelectedGroup: "default",
	})
	_, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterUsesSessionAndConversationSignals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}

	store := scheduler.NewMemoryStickyStore(8)
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)

	firstCtx := newStickyRequestContext(t, `{"conversation":{"id":"conv-a"}}`, nil)
	common.SetContextKey(firstCtx, constant.ContextKeyTokenId, 501)
	router.Save(firstCtx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 17},
		SelectedGroup: "default",
	})

	sameCtx := newStickyRequestContext(t, `{"conversation":{"id":"conv-a"}}`, nil)
	common.SetContextKey(sameCtx, constant.ContextKeyTokenId, 501)
	route, ok := router.Route(sameCtx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 17, route.ChannelID)

	otherCtx := newStickyRequestContext(t, `{"conversation":{"id":"conv-b"}}`, nil)
	common.SetContextKey(otherCtx, constant.ContextKeyTokenId, 501)
	_, ok = router.Route(otherCtx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterUsesCodexTurnMetadataBeforeBodyFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}

	store := scheduler.NewMemoryStickyStore(8)
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)

	headers := map[string]string{
		"X-Codex-Turn-Metadata": `{"thread_id":"thread-a"}`,
	}
	firstCtx := newStickyRequestContext(t, `{"conversation_id":"body-a"}`, headers)
	common.SetContextKey(firstCtx, constant.ContextKeyTokenId, 502)
	router.Save(firstCtx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 18},
		SelectedGroup: "default",
	})

	sameHeaderDifferentBody := newStickyRequestContext(t, `{"conversation_id":"body-b"}`, headers)
	common.SetContextKey(sameHeaderDifferentBody, constant.ContextKeyTokenId, 502)
	route, ok := router.Route(sameHeaderDifferentBody, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 18, route.ChannelID)

	differentHeaderSameBody := newStickyRequestContext(t, `{"conversation_id":"body-a"}`, map[string]string{
		"X-Codex-Turn-Metadata": `{"thread_id":"thread-b"}`,
	})
	common.SetContextKey(differentHeaderSameBody, constant.ContextKeyTokenId, 502)
	_, ok = router.Route(differentHeaderSameBody, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterClearRemovesCurrentSessionEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}

	store := scheduler.NewMemoryStickyStore(8)
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)

	ctx := newStickyRequestContext(t, `{"session_id":"sess-clear-a"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 503)
	router.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 19},
		SelectedGroup: "default",
	})

	route, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 19, route.ChannelID)

	router.Clear(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	_, ok = router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterClearStickyEntriesByGroupAndChannel(t *testing.T) {
	store := scheduler.NewMemoryStickyStore(8)
	store.Set("sticky-a", scheduler.StickyEntry{
		ChannelID: 42,
		Group:     "vip",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	store.Set("sticky-b", scheduler.StickyEntry{
		ChannelID: 42,
		Group:     "vip",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	store.Set("sticky-c", scheduler.StickyEntry{
		ChannelID: 43,
		Group:     "vip",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	store.Set("sticky-d", scheduler.StickyEntry{
		ChannelID: 42,
		Group:     "default",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{Store: store}, nil)

	deleted := router.ClearStickyEntries(scheduler.StickyClearFilter{
		Group:     "vip",
		ChannelID: 42,
	})

	require.Equal(t, 2, deleted)
	entries := router.StickyEntries()
	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.False(t, entry.Group == "vip" && entry.ChannelID == 42)
	}
}

func TestStickyRouterDisabledContextSkipsRouteAndSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-disabled-a"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 505)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryStickyStore(8)
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)

	scheduler.WithStickyRoutingDisabled(ctx, func() {
		router.Save(ctx, &req, &core.DispatchPlan{
			Channel:       &model.Channel{Id: 22},
			SelectedGroup: "default",
		})
		_, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
		require.False(t, ok)
	})

	_, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)

	router.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 23},
		SelectedGroup: "default",
	})
	route, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 23, route.ChannelID)
}

func TestStickyRouterSaveRenewsTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-renew-a"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 504)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryStickyStore(8)
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		TTLSeconds: 1,
		Store:      store,
	}, nil)

	router.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 20},
		SelectedGroup: "default",
	})
	time.Sleep(600 * time.Millisecond)
	router.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 21},
		SelectedGroup: "default",
	})
	time.Sleep(600 * time.Millisecond)

	route, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 21, route.ChannelID)
}

func TestStickyRouterReportSuccessRenewsTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-report-renew-a"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 506)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryStickyStore(8)
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		TTLSeconds:     1,
		RenewOnSuccess: true,
		Store:          store,
	}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 31},
		SelectedGroup: "default",
	})
	time.Sleep(600 * time.Millisecond)
	sticky.Report(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 31},
		SelectedGroup: "default",
		StickySource:  "user_sticky",
	}, core.AttemptResult{
		Success: true,
	})

	time.Sleep(600 * time.Millisecond)
	route, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 31, route.ChannelID)
}

func TestStickyRouterReportSuccessDoesNotRenewSuppressedPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-report-renew-suppressed"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 5061)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		TTLSeconds:     1,
		RenewOnSuccess: true,
		Store:          scheduler.NewMemoryStickyStore(8),
	}, nil)
	plan := &core.DispatchPlan{
		Channel:                 &model.Channel{Id: 31},
		SelectedGroup:           "default",
		StickySource:            "user_sticky",
		StickySaveSuppressed:    true,
		StickySuppressionReason: "negative_current_group_margin",
	}
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 31},
		SelectedGroup: "default",
	})
	time.Sleep(600 * time.Millisecond)
	sticky.Report(ctx, &req, plan, core.AttemptResult{
		Success: true,
	})

	time.Sleep(600 * time.Millisecond)
	_, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterDoesNotSaveRetryAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-retry-save-a"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 508)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
		Retry:          1,
	}
	router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: scheduler.NewMemoryStickyStore(8),
	}, nil)

	router.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 41},
		SelectedGroup: "default",
	})

	req.Retry = 0
	_, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterReportRetrySuccessDoesNotRenewTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-report-retry-success-a"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 509)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryStickyStore(8)
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		TTLSeconds:     1,
		RenewOnSuccess: true,
		Store:          store,
	}, nil)
	plan := &core.DispatchPlan{
		Channel:       &model.Channel{Id: 42},
		SelectedGroup: "default",
		StickySource:  "user_sticky",
	}
	sticky.Save(ctx, &req, plan)
	time.Sleep(600 * time.Millisecond)

	ctx.Set("use_channel", []string{"31", "42"})
	sticky.Report(ctx, &req, plan, core.AttemptResult{
		Success:      true,
		AttemptIndex: 1,
		UsedChannels: []string{"31", "42"},
	})

	time.Sleep(600 * time.Millisecond)
	_, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestStickyRouterReportFailureKeepOrClear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	for _, tc := range []struct {
		name       string
		policy     scheduler.StickyFailurePolicy
		wantExists bool
	}{
		{name: "keep", policy: scheduler.StickyFailurePolicyKeep, wantExists: true},
		{name: "clear", policy: scheduler.StickyFailurePolicyClear, wantExists: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newStickyRequestContext(t, `{"session_id":"sess-report-failure-`+tc.name+`"}`, nil)
			common.SetContextKey(ctx, constant.ContextKeyTokenId, 507)
			router := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
				FailurePolicy: tc.policy,
				Store:         scheduler.NewMemoryStickyStore(8),
			}, nil)
			plan := &core.DispatchPlan{
				Channel:       &model.Channel{Id: 32},
				SelectedGroup: "default",
				StickySource:  "user_sticky",
			}
			router.Save(ctx, &req, plan)
			router.Report(ctx, &req, plan, core.AttemptResult{
				Success:    false,
				StatusCode: 500,
			})

			route, ok := router.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
			require.Equal(t, tc.wantExists, ok)
			if tc.wantExists {
				require.Equal(t, 32, route.ChannelID)
			}
		})
	}
}

func TestHybridStickyStoreStoresAndExpiresEntries(t *testing.T) {
	store := scheduler.NewHybridStickyStore(8)
	key := fmt.Sprintf("sticky-test-%d", time.Now().UnixNano())
	store.Set(key, scheduler.StickyEntry{
		ChannelID:      9,
		Group:          "default",
		KeyFingerprint: "fp",
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	t.Cleanup(func() {
		store.Set(key, scheduler.StickyEntry{ExpiresAt: time.Now().Add(-time.Second)})
	})

	entry, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 9, entry.ChannelID)
	require.Equal(t, "default", entry.Group)
	require.Equal(t, "fp", entry.KeyFingerprint)

	expiredKey := fmt.Sprintf("sticky-expired-%d", time.Now().UnixNano())
	store.Set(expiredKey, scheduler.StickyEntry{
		ChannelID: 10,
		ExpiresAt: time.Now().Add(-time.Second),
	})
	_, ok = store.Get(expiredKey)
	require.False(t, ok)
}

func TestSelectorBreaksStickyRouteOnNegativeCurrentGroupMargin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-break-negative-margin"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 601)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	peerKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.92,
		TTFTMs:             600,
		TokensPerSecond:    50,
		CostRatio:          3,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                peerKey,
		SuccessRate:        0.96,
		TTFTMs:             450,
		TokensPerSecond:    70,
		CostRatio:          0.5,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: peerKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "negative_current_group_margin", plan.StickyBreak)
	require.False(t, plan.StickySaveSuppressed)
	require.Equal(t, "score_items_sticky_broken", plan.SelectedReason)
}

func TestSelectorAllowsNegativeMarginFallbackButSkipsStickySave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-negative-margin-only"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 602)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		SaveOnSelect: true,
	}, nil)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 7, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        0.95,
		TTFTMs:             300,
		TokensPerSecond:    80,
		CostRatio:          2,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 7}, Group: "default", RuntimeKey: key},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 7, plan.Channel.Id)
	require.Equal(t, "negative_margin_fallback", plan.SelectedReason)
	require.True(t, plan.FallbackUsed)
	require.True(t, plan.StickySaveSuppressed)
	require.Equal(t, "negative_current_group_margin", plan.StickySuppressionReason)

	_, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}

func TestSelectorSkipsNegativeMarginCandidateWhenPositiveMarginAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-negative-margin-score-skip"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 604)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	negativeKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 11, Group: "default"}
	positiveKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 12, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                negativeKey,
		SuccessRate:        1,
		TTFTMs:             100,
		TokensPerSecond:    80,
		CostRatio:          2,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                positiveKey,
		SuccessRate:        0.7,
		TTFTMs:             3000,
		TokensPerSecond:    20,
		CostRatio:          0.8,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 11}, Group: "default", RuntimeKey: negativeKey},
			{Channel: &model.Channel{Id: 12}, Group: "default", RuntimeKey: positiveKey},
		}),
		store,
		core.ScoreWeights{Success: 1},
	)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 12, plan.Channel.Id)
	require.False(t, plan.FallbackUsed)
	require.NotEqual(t, "negative_margin_fallback", plan.SelectedReason)
	negative := candidateExplanationByChannel(t, plan.Candidates, 11)
	require.True(t, negative.Available)
	require.True(t, negative.NegativeCurrentGroupMargin)
	require.Equal(t, "negative_current_group_margin", negative.SelectionSkipReason)
	require.False(t, negative.Selected)
}

func TestSelectorReportsNegativeMarginBeforeSaturationWhenPositiveMarginAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-negative-margin-saturated-skip"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 605)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	negativeKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 21, Group: "default"}
	positiveKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 22, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                negativeKey,
		SuccessRate:        1,
		TTFTMs:             100,
		TokensPerSecond:    80,
		ActiveConcurrency:  1,
		MaxConcurrency:     1,
		CostRatio:          2,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                positiveKey,
		SuccessRate:        0.7,
		TTFTMs:             3000,
		TokensPerSecond:    20,
		CostRatio:          0.8,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 21}, Group: "default", RuntimeKey: negativeKey},
			{Channel: &model.Channel{Id: 22}, Group: "default", RuntimeKey: positiveKey},
		}),
		store,
		core.ScoreWeights{Success: 1},
	)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 22, plan.Channel.Id)
	negative := candidateExplanationByChannel(t, plan.Candidates, 21)
	require.True(t, negative.Available)
	require.True(t, negative.NegativeCurrentGroupMargin)
	require.Equal(t, "negative_current_group_margin", negative.SelectionSkipReason)
	require.False(t, negative.Selected)
}

func TestSelectorDoesNotTreatMissingRevenueAsNegativeMargin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"sess-missing-revenue-margin"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 603)
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
	}
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		StickyKeepScoreRatio: 0.85,
	}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 1},
		SelectedGroup: "default",
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 1, Group: "default"}
	peerKey := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.92,
		TTFTMs:             600,
		TokensPerSecond:    50,
		CostRatio:          3,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                peerKey,
		SuccessRate:        0.96,
		TTFTMs:             450,
		TokensPerSecond:    70,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: peerKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "default",
		CandidateGroups: []string{"default"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.StickyRetained)
	require.Empty(t, plan.StickyBreak)
	require.False(t, plan.StickySaveSuppressed)
}

func TestSelectorBreaksCacheAffinityRouteOnNegativeCurrentGroupMargin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	stickyKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 1, Group: "default"}
	peerKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                stickyKey,
		SuccessRate:        0.85,
		TTFTMs:             700,
		TokensPerSecond:    40,
		CostRatio:          3,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                peerKey,
		SuccessRate:        0.98,
		TTFTMs:             350,
		TokensPerSecond:    80,
		CostRatio:          0.5,
		RevenueRatio:       1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		CacheKeepScoreRatio: 0.75,
	}, scheduler.NewStaticCacheAffinitySignalAdapter(core.CacheAffinitySignal{
		Key:                "fake-cache-key-negative-margin",
		KeyFingerprint:     "affinity-negative-margin",
		Source:             "fake",
		PreferredChannelID: 1,
		PreferredGroup:     "default",
	}, true))
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: stickyKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: peerKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5",
	}, core.GroupSmartPolicy{
		Mode:                 core.ModeActive,
		RequestedGroup:       "default",
		CandidateGroups:      []string{"default"},
		Strategy:             core.StrategyBalanced,
		CacheAffinityEnabled: true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.True(t, plan.CacheAffinity)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "cache_affinity", plan.StickySource)
	require.Equal(t, "negative_current_group_margin", plan.StickyBreak)
}

func TestSelectorRetainsCacheAffinityRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 1, Group: "default"}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 2, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        0.85,
		TTFTMs:             700,
		TokensPerSecond:    40,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.98,
		TTFTMs:             350,
		TokensPerSecond:    80,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		CacheKeepScoreRatio: 0.75,
	}, scheduler.NewStaticCacheAffinitySignalAdapter(core.CacheAffinitySignal{
		Key:                "fake-cache-key",
		KeyFingerprint:     "affinity-1",
		Source:             "fake",
		PreferredChannelID: 1,
		PreferredGroup:     "default",
	}, true))
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 2}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5",
	}, core.GroupSmartPolicy{
		Mode:                 core.ModeActive,
		RequestedGroup:       "default",
		CandidateGroups:      []string{"default"},
		Strategy:             core.StrategyBalanced,
		CacheAffinityEnabled: true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.CacheAffinity)
	require.True(t, plan.StickyRetained)
	require.Equal(t, "cache_affinity", plan.StickySource)
	require.NotEmpty(t, plan.StickyKeyFP)
}

func newStickyRequestContext(t *testing.T, body string, headers map[string]string) *gin.Context {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		ctx.Request.Header.Set(key, value)
	}
	return ctx
}

func TestServiceCacheAffinityAdapterUsesPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	var codexRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		if strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli previous response") {
			codexRule = rule
			break
		}
	}
	require.NotNil(t, codexRule)

	previousResponseID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	require.NotEmpty(t, codexRule.KeySources)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"previous_response_id":"%s"}`, previousResponseID)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	signal, ok := service.ResolveChannelAffinitySignal(ctx, "gpt-5", "default")
	require.True(t, ok)
	require.Equal(t, "codex cli previous response", signal.RuleName)
	require.Equal(t, "previous_response_id", signal.KeySourcePath)
	require.Equal(t, 0, signal.PreferredChannelID)
	service.RecordChannelAffinity(ctx, 11)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	firstKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 11, Group: "default"}
	secondKey := core.RuntimeKey{RequestedModel: "gpt-5", ChannelID: 22, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                firstKey,
		SuccessRate:        0.90,
		TTFTMs:             650,
		TokensPerSecond:    50,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                secondKey,
		SuccessRate:        0.98,
		TTFTMs:             250,
		TokensPerSecond:    90,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})

	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		CacheKeepScoreRatio: 0.70,
	}, scheduler.NewServiceCacheAffinitySignalAdapter())
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 11}, Group: "default", RuntimeKey: firstKey},
			{Channel: &model.Channel{Id: 22}, Group: "default", RuntimeKey: secondKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5",
	}, core.GroupSmartPolicy{
		Mode:                 core.ModeActive,
		RequestedGroup:       "default",
		CandidateGroups:      []string{"default"},
		Strategy:             core.StrategyBalanced,
		CacheAffinityEnabled: true,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 11, plan.Channel.Id)
	require.True(t, plan.CacheAffinity)
	require.True(t, plan.StickyRetained)
	require.Equal(t, "cache_affinity", plan.StickySource)
	require.NotEmpty(t, plan.StickyKeyFP)
}
