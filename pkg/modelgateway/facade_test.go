package modelgateway_test

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/QuantumNous/new-api/service"
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
