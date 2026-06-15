package modelgateway_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPortableFacadeOffDoesNotHandle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {
				Mode:     core.ModeOff,
				Strategy: core.StrategyBalanced,
				AutoMode: core.AutoModeSequential,
			},
		},
	})
	ctx, _ := gin.CreateTestContext(nil)
	plan, handled, apiErr := h.Facade.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.False(t, handled)
	require.Nil(t, plan)
	require.Zero(t, h.Selector.Calls)
}

func TestPortableFacadeActiveDelegatesSelector(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {
				Mode:     core.ModeActive,
				Strategy: core.StrategyBalanced,
				AutoMode: core.AutoModeSequential,
			},
		},
	})
	h.Selector.Plan = &core.DispatchPlan{
		Channel:        &model.Channel{Id: 7, Name: "smart"},
		SelectedGroup:  "default",
		RequestedGroup: "default",
	}
	h.Selector.Handled = true

	ctx, _ := gin.CreateTestContext(nil)
	plan, handled, apiErr := h.Facade.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 7, plan.Channel.Id)
	require.Equal(t, core.ModeActive, plan.PolicyMode)
	require.Equal(t, core.AutoModeSequential, plan.AutoMode)
	require.Equal(t, 1, h.Selector.Calls)
	records := h.Recorder.SnapshotRecords()
	require.Len(t, records, 1)
	require.Equal(t, core.ModeActive, records[0].Policy.Mode)
	require.Equal(t, core.AutoModeSequential, records[0].Policy.AutoMode)
	require.Equal(t, 7, records[0].Plan.Channel.Id)
	require.False(t, records[0].Shadow)
}

func TestPortableFacadeActiveNoPlanStillHandled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {
				Mode:     core.ModeActive,
				Strategy: core.StrategyBalanced,
				AutoMode: core.AutoModeSequential,
			},
		},
	})
	h.Selector.Handled = false

	ctx, _ := gin.CreateTestContext(nil)
	plan, handled, apiErr := h.Facade.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Nil(t, plan)
	require.Equal(t, 1, h.Selector.Calls)
	require.Empty(t, h.Recorder.SnapshotRecords())
}

func TestFacadeKeepsLossMakingSelectionAsLastRetryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	selector := &lossMakingFallbackCaptureSelector{
		plan: &core.DispatchPlan{
			Channel:        &model.Channel{Id: 84, Name: "loss-only"},
			SelectedGroup:  "default",
			RequestedGroup: "default",
			SelectedReason: "negative_margin_fallback",
			FallbackUsed:   true,
		},
	}
	recorder := &testkit.FakeExecutionRecorder{}
	facade := modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{Settings: core.SchedulerSettings{
			Enabled:         true,
			DefaultMode:     core.ModeOff,
			DefaultStrategy: core.StrategyBalanced,
			GroupPolicies: map[string]core.GroupPolicySetting{
				"default": {
					Mode:     core.ModeActive,
					Strategy: core.StrategyBalanced,
					AutoMode: core.AutoModeSequential,
				},
			},
		}}),
		AutoResolver: policy.NewDefaultAutoGroupResolver(&testkit.FakeGroupPermissionService{}),
		Selector:     selector,
		Recorder:     recorder,
	})
	ctx, _ := gin.CreateTestContext(nil)
	retry := 1

	plan, handled, apiErr := facade.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
		Retry:        &retry,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 84, plan.Channel.Id)
	require.Len(t, selector.policies, 2)
	require.True(t, selector.policies[0].SuppressLossMakingFallback)
	require.False(t, selector.policies[1].SuppressLossMakingFallback)
	records := recorder.SnapshotRecords()
	require.Len(t, records, 1)
	require.False(t, records[0].Policy.SuppressLossMakingFallback)
}

func TestPortableFacadeShadowRecordsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {
				Mode:     core.ModeShadow,
				Strategy: core.StrategyBalanced,
				AutoMode: core.AutoModeSequential,
			},
		},
	})
	ctx, _ := gin.CreateTestContext(nil)
	param := &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	}
	plan, handled, apiErr := h.Facade.Select(ctx, param)
	require.Nil(t, apiErr)
	require.False(t, handled)
	require.Nil(t, plan)

	h.Facade.Shadow(ctx, param, &model.Channel{Id: 3, Name: "legacy"}, "default")
	records := h.Recorder.SnapshotRecords()
	require.Len(t, records, 1)
	require.True(t, records[0].Shadow)
	require.Equal(t, "default", records[0].ActualGroup)
	require.Equal(t, 3, records[0].Actual.Id)
}

type lossMakingFallbackCaptureSelector struct {
	policies []core.GroupSmartPolicy
	plan     *core.DispatchPlan
}

func (s *lossMakingFallbackCaptureSelector) Select(c *gin.Context, param *service.RetryParam, policy core.GroupSmartPolicy) (*core.DispatchPlan, bool, *types.NewAPIError) {
	s.policies = append(s.policies, policy)
	if policy.SuppressLossMakingFallback {
		return nil, false, nil
	}
	return s.plan, true, nil
}

func TestFacadeReportAppliesStickyLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("X-Session-Id", "facade-sticky-lifecycle")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 901)

	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		TTLSeconds:           1,
		RenewOnSuccess:       true,
		FailurePolicy:        scheduler.StickyFailurePolicyClear,
		StickyKeepScoreRatio: 0.85,
		Store:                scheduler.NewMemoryStickyStore(8),
	}, nil)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {
				Mode:     core.ModeActive,
				Strategy: core.StrategyBalanced,
				AutoMode: core.AutoModeSequential,
			},
		},
	})
	h.Selector.Sticky = sticky

	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	plan := &core.DispatchPlan{
		Channel:        &model.Channel{Id: 51, Name: "sticky-success"},
		SelectedGroup:  "default",
		RequestedGroup: "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5.5",
			ChannelID:      51,
			Group:          "default",
			EndpointType:   constant.EndpointTypeOpenAIResponse,
		},
		StickySource: "user_sticky",
	}
	sticky.Save(ctx, &req, plan)
	integration.SetFailedStickyPlan(ctx, plan)
	time.Sleep(600 * time.Millisecond)

	h.Facade.Report(ctx, &core.AttemptResult{
		ChannelID:      51,
		RequestedGroup: "default",
		SelectedGroup:  "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
		Success:        true,
	})
	time.Sleep(600 * time.Millisecond)

	route, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 51, route.ChannelID)

	h.Facade.Report(ctx, &core.AttemptResult{
		ChannelID:      51,
		RequestedGroup: "default",
		SelectedGroup:  "default",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
		Success:        false,
		StatusCode:     500,
	})
	_, ok = sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
}
