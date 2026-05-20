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
	ctx, _ := gin.CreateTestContext(nil)
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
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
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
	ctx, _ := gin.CreateTestContext(nil)
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
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
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
	require.Equal(t, "weighted_score_sticky_broken", plan.SelectedReason)
}

func TestSelectorBreaksStickyRouteWhenQueueCannotWait(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
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
		ActiveConcurrency:  2,
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
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
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
	require.Equal(t, 2, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, "concurrency_full", plan.StickyBreak)
}

func TestStickyRouterSharesStoreAcrossInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
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
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
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
		if strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli trace") {
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
	require.Equal(t, "codex cli trace", signal.RuleName)
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
		scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
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
