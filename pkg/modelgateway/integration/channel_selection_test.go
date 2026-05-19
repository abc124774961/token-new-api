package integration_test

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelSelectionWrapperActiveUsesSmartPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Selector.Plan = &core.DispatchPlan{
		Channel:       &model.Channel{Id: 10, Name: "smart"},
		SelectedGroup: "default",
	}
	h.Selector.Handled = true
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.SmartHandled)
	require.False(t, result.FallbackUsed)
	require.Equal(t, 10, result.Channel.Id)
	require.Zero(t, h.Legacy.Calls)
}

func TestSelectorRetainsSmartStickyContextOnlyWhenHandled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Selector.Plan = &core.DispatchPlan{
		Channel:       &model.Channel{Id: 11, Name: "smart-sticky"},
		SelectedGroup: "default",
		StickySource:  "user_sticky",
	}
	h.Selector.Handled = true
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.SmartHandled)
	selectedPlan, ok := integration.GetSelectedPlan(ctx)
	require.True(t, ok)
	require.Equal(t, 11, selectedPlan.Channel.Id)
	failedStickyPlan, ok := integration.GetFailedStickyPlan(ctx)
	require.True(t, ok)
	require.Equal(t, selectedPlan, failedStickyPlan)
}

func TestSelectorBreaksSmartStickyContextOnFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeOff, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Legacy.Channel = &model.Channel{Id: 12, Name: "legacy"}
	h.Legacy.Group = "default"
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)
	ctx, _ := gin.CreateTestContext(nil)
	integration.SetSelectedPlan(ctx, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 99, Name: "stale-smart"},
		SelectedGroup: "default",
	})
	integration.SetFailedStickyPlan(ctx, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 98, Name: "stale-sticky"},
		SelectedGroup: "default",
		StickySource:  "user_sticky",
	})

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.FallbackUsed)
	require.Equal(t, 12, result.Channel.Id)
	_, ok := integration.GetSelectedPlan(ctx)
	require.False(t, ok)
	_, ok = integration.GetFailedStickyPlan(ctx)
	require.False(t, ok)
}

func TestSelectorBreaksStickyDuringFallbackShadow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	facade := &shadowInspectFacade{}
	legacy := &testkit.FakeLegacyChannelSelector{
		Channel: &model.Channel{Id: 13, Name: "legacy-shadow"},
		Group:   "default",
	}
	wrapper := integration.NewChannelSelectionWrapper(facade, legacy)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.FallbackUsed)
	require.Equal(t, 13, result.Channel.Id)
	require.True(t, facade.ShadowCalled)
	require.True(t, facade.StickyDisabledDuringShadow)
	require.False(t, scheduler.StickyRoutingDisabled(ctx))
}

func TestChannelSelectionWrapperOffFallsBackToLegacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeOff, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Legacy.Channel = &model.Channel{Id: 3, Name: "legacy"}
	h.Legacy.Group = "default"
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.False(t, result.SmartHandled)
	require.True(t, result.FallbackUsed)
	require.Equal(t, 3, result.Channel.Id)
	require.Equal(t, 1, h.Legacy.Calls)
}

func TestChannelSelectionWrapperShadowFallsBackAndRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeShadow, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Legacy.Channel = &model.Channel{Id: 4, Name: "legacy-shadow"}
	h.Legacy.Group = "default"
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.FallbackUsed)
	require.Equal(t, 4, result.Channel.Id)
	records := h.Recorder.SnapshotRecords()
	require.Len(t, records, 1)
	require.True(t, records[0].Shadow)
	require.Equal(t, "default", records[0].ActualGroup)
}

type shadowInspectFacade struct {
	ShadowCalled               bool
	StickyDisabledDuringShadow bool
}

func (f *shadowInspectFacade) Select(c *gin.Context, param *service.RetryParam) (*core.DispatchPlan, bool, *types.NewAPIError) {
	return nil, false, nil
}

func (f *shadowInspectFacade) Shadow(c *gin.Context, param *service.RetryParam, actual *model.Channel, actualGroup string) {
	f.ShadowCalled = true
	f.StickyDisabledDuringShadow = scheduler.StickyRoutingDisabled(c)
}

func (f *shadowInspectFacade) Report(c *gin.Context, result *core.AttemptResult) {}
