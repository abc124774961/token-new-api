package integration_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelSelectionWrapperActiveUsesSmartPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

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

func TestChannelSelectionWrapperSkipsDisabledSmartPlanAndRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	facade := &sequenceFacade{
		plans: []*core.DispatchPlan{
			{
				Channel:       &model.Channel{Id: 201, Name: "disabled-smart", Status: common.ChannelStatusManuallyDisabled},
				SelectedGroup: "default",
			},
			{
				Channel:       &model.Channel{Id: 202, Name: "healthy-smart", Status: common.ChannelStatusEnabled},
				SelectedGroup: "default",
			},
		},
	}
	wrapper := integration.NewChannelSelectionWrapper(facade, nil)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.SelectSmartOnly(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, result)
	require.Equal(t, 202, result.Channel.Id)
	require.Equal(t, 2, facade.SelectCalls)
	require.True(t, service.IsChannelSelectionSkipped(ctx, 201))
	require.Equal(t, 1, service.GetChannelSelectionReservations(202))
	service.ReleaseChannelSelectionReservations(ctx)
}

func TestChannelSelectionWrapperSkipsUsageLimitedSmartPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	now := common.GetTimestamp()
	limited := &model.Channel{
		Id:     211,
		Type:   constant.ChannelTypeCodex,
		Name:   "usage-limited",
		Status: common.ChannelStatusEnabled,
		Key:    `{"access_token":"limited","account_id":"limited"}`,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					UsageLimitStatus:    channelcapability.UsageLimitStatusLimited,
					UsageLimitReason:    channelcapability.UsageLimitReasonReached,
					UsageLimitExpiresAt: now + 1800,
				},
			},
		},
	}
	healthy := &model.Channel{
		Id:     212,
		Type:   constant.ChannelTypeCodex,
		Name:   "healthy",
		Status: common.ChannelStatusEnabled,
		Key:    `{"access_token":"healthy","account_id":"healthy"}`,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
		},
	}
	facade := &sequenceFacade{
		plans: []*core.DispatchPlan{
			{
				Channel:       limited,
				SelectedGroup: "default",
				RuntimeKey: core.RuntimeKey{
					EndpointType: constant.EndpointTypeOpenAI,
				},
				CredentialRef: core.CredentialRef{
					ResourceID:      "platform:channel:211",
					CredentialIndex: 0,
					Resolver:        "channel_key",
				},
			},
			{
				Channel:       healthy,
				SelectedGroup: "default",
				RuntimeKey: core.RuntimeKey{
					EndpointType: constant.EndpointTypeOpenAI,
				},
				CredentialRef: core.CredentialRef{
					ResourceID:      "platform:channel:212",
					CredentialIndex: 0,
					Resolver:        "channel_key",
				},
			},
		},
	}
	wrapper := integration.NewChannelSelectionWrapper(facade, nil)
	ctx, _ := gin.CreateTestContext(nil)

	result, apiErr := wrapper.SelectSmartOnly(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, result)
	require.Equal(t, 212, result.Channel.Id)
	require.Equal(t, 2, facade.SelectCalls)
	require.True(t, service.IsChannelSelectionSkipped(ctx, 211))
	service.ReleaseChannelSelectionReservations(ctx)
}

func TestChannelSelectionWrapperSmartPlanReservesRoutingSlotBeforeRelayAcquire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Selector.Plan = &core.DispatchPlan{
		Channel:       &model.Channel{Id: 101, Name: "reserved-smart", Setting: common.GetPointer(`{"max_concurrency":1}`)},
		SelectedGroup: "default",
		Candidates: []core.CandidateExplanation{
			{ChannelID: 101, Selected: true, SampleCount: 12},
		},
	}
	h.Selector.Handled = true
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)

	firstCtx, _ := gin.CreateTestContext(nil)
	first, apiErr := wrapper.Select(firstCtx, &service.RetryParam{
		Ctx:          firstCtx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})
	require.Nil(t, apiErr)
	require.NotNil(t, first)
	require.Equal(t, 1, service.GetChannelSelectionReservations(101))
	service.ReleaseChannelSelectionReservations(firstCtx)
	require.Equal(t, 0, service.GetChannelSelectionReservations(101))

	secondCtx, _ := gin.CreateTestContext(nil)
	second, apiErr := wrapper.SelectSmartOnly(secondCtx, &service.RetryParam{
		Ctx:          secondCtx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})
	require.Nil(t, apiErr)
	require.NotNil(t, second)
	require.Equal(t, 101, second.Channel.Id)
	require.Equal(t, 1, service.GetChannelSelectionReservations(101))
	service.ReleaseChannelSelectionReservations(secondCtx)
	require.Equal(t, 0, service.GetChannelSelectionReservations(101))
}

func TestChannelSelectionWrapperDoesNotColdStartProbeCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Selector.Plan = &core.DispatchPlan{
		Channel:       &model.Channel{Id: 102, Name: "cold-smart", Setting: common.GetPointer(`{"max_concurrency":20}`)},
		SelectedGroup: "default",
		Candidates: []core.CandidateExplanation{
			{ChannelID: 102, Selected: true, Available: true, SampleCount: 0},
			{ChannelID: 105, Available: true, SampleCount: 0},
		},
	}
	h.Selector.Handled = true
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)

	for i := 0; i < 6; i++ {
		ctx, _ := gin.CreateTestContext(nil)
		result, apiErr := wrapper.SelectSmartOnly(ctx, &service.RetryParam{
			Ctx:          ctx,
			TokenGroup:   "default",
			ModelName:    "gpt-4.1",
			EndpointType: constant.EndpointTypeOpenAI,
		})
		require.Nil(t, apiErr)
		require.NotNil(t, result)
		require.Equal(t, 102, result.Channel.Id)
		service.ReleaseChannelSelectionReservations(ctx)
	}
	require.Equal(t, 0, service.GetChannelSelectionReservations(102))
}

func TestChannelSelectionWrapperDoesNotColdStartCapWhenNoConfiguredLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Selector.Plan = &core.DispatchPlan{
		Channel:       &model.Channel{Id: 106, Name: "uncapped-cold", Setting: common.GetPointer(`{"max_concurrency_ceiling":46}`)},
		SelectedGroup: "default",
		Candidates: []core.CandidateExplanation{
			{ChannelID: 106, Selected: true, Available: true, SampleCount: 0},
			{ChannelID: 107, Available: true, SampleCount: 0},
		},
	}
	h.Selector.Handled = true
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)

	for i := 0; i < 6; i++ {
		ctx, _ := gin.CreateTestContext(nil)
		result, apiErr := wrapper.SelectSmartOnly(ctx, &service.RetryParam{
			Ctx:          ctx,
			TokenGroup:   "default",
			ModelName:    "gpt-4.1",
			EndpointType: constant.EndpointTypeOpenAI,
		})
		require.Nil(t, apiErr)
		require.NotNil(t, result)
		require.Equal(t, 106, result.Channel.Id)
		service.ReleaseChannelSelectionReservations(ctx)
	}
	require.Equal(t, 0, service.GetChannelSelectionReservations(106))
}

func TestChannelSelectionWrapperDoesNotRetryWhenSelectedPlanWouldHaveBeenBurstFull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	facade := &sequenceFacade{
		plans: []*core.DispatchPlan{
			{
				Channel:       &model.Channel{Id: 103, Name: "slow-busy", Setting: common.GetPointer(`{"max_concurrency":50}`)},
				SelectedGroup: "default",
				Candidates: []core.CandidateExplanation{
					{ChannelID: 103, Selected: true, Available: true, SampleCount: 20, SuccessRate: 0.90, TTFTMs: 13000},
					{ChannelID: 104, Available: true, SampleCount: 20, SuccessRate: 0.99, TTFTMs: 1000},
				},
			},
			{
				Channel:       &model.Channel{Id: 104, Name: "fast-peer", Setting: common.GetPointer(`{"max_concurrency":50}`)},
				SelectedGroup: "default",
				Candidates: []core.CandidateExplanation{
					{ChannelID: 103, Available: true, SampleCount: 20, SuccessRate: 0.90, TTFTMs: 13000},
					{ChannelID: 104, Selected: true, Available: true, SampleCount: 20, SuccessRate: 0.99, TTFTMs: 1000},
				},
			},
		},
	}
	wrapper := integration.NewChannelSelectionWrapper(facade, &testkit.FakeLegacyChannelSelector{})

	ctx, _ := gin.CreateTestContext(nil)
	result, apiErr := wrapper.SelectSmartOnly(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, result)
	require.Equal(t, 103, result.Channel.Id)
	require.Equal(t, 1, facade.SelectCalls)
	require.Equal(t, 1, service.GetChannelSelectionReservations(103))
	service.ReleaseChannelSelectionReservations(ctx)
	require.Equal(t, 0, service.GetChannelSelectionReservations(103))
}

func TestSelectorRetainsSmartStickyContextOnlyWhenHandled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

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

func TestSmartStickySuccessRenewsAndFailureClearsLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newIntegrationStickyContext(601, "sess-lifecycle-a")
	settings := core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	}
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-4.1",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryStickyStore(8)
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		TTLSeconds:     1,
		RenewOnSuccess: true,
		FailurePolicy:  scheduler.StickyFailurePolicyClear,
		Store:          store,
	}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 31},
		SelectedGroup: "default",
	})
	time.Sleep(600 * time.Millisecond)

	key := core.RuntimeKey{RequestedModel: "gpt-4.1", ChannelID: 31, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	snapshots := scheduler.NewMemoryRuntimeSnapshotStore()
	snapshots.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        0.95,
		TTFTMs:             500,
		TokensPerSecond:    50,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 31, Name: "sticky"}, Group: "default", RuntimeKey: key},
		}),
		snapshots,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)
	recorder := &testkit.FakeExecutionRecorder{}
	facade := modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{Settings: settings}),
		AutoResolver:   policy.NewDefaultAutoGroupResolver(&testkit.FakeGroupPermissionService{}),
		Selector:       selector,
		Recorder:       recorder,
	})
	wrapper := integration.NewChannelSelectionWrapper(facade, &testkit.FakeLegacyChannelSelector{})
	param := &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	}

	result, apiErr := wrapper.Select(ctx, param)
	require.Nil(t, apiErr)
	require.True(t, result.SmartHandled)
	require.True(t, result.Plan.StickyRetained)
	facade.Report(ctx, &core.AttemptResult{
		Key:            result.Plan.RuntimeKey,
		ChannelID:      result.Channel.Id,
		RequestedGroup: "default",
		SelectedGroup:  result.Group,
		ModelName:      "gpt-4.1",
		EndpointType:   constant.EndpointTypeOpenAI,
		Success:        true,
	})
	time.Sleep(600 * time.Millisecond)
	route, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.True(t, ok)
	require.Equal(t, 31, route.ChannelID)

	result, apiErr = wrapper.Select(ctx, param)
	require.Nil(t, apiErr)
	require.True(t, result.SmartHandled)
	facade.Report(ctx, &core.AttemptResult{
		Key:            result.Plan.RuntimeKey,
		ChannelID:      result.Channel.Id,
		RequestedGroup: "default",
		SelectedGroup:  result.Group,
		ModelName:      "gpt-4.1",
		EndpointType:   constant.EndpointTypeOpenAI,
		Success:        false,
		StatusCode:     500,
	})
	_, ok = sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
	require.Len(t, recorder.SnapshotResults(), 2)
}

func TestSmartStickyFallbackAndShadowDoNotPolluteStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := newIntegrationStickyContext(602, "sess-shadow-clean-a")
	settings := core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeShadow, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	}
	req := core.DispatchRequest{
		RequestedGroup: "default",
		UserGroup:      "default",
		ModelName:      "gpt-4.1",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryStickyStore(8)
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		Store: store,
	}, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", ChannelID: 41, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	snapshots := scheduler.NewMemoryRuntimeSnapshotStore()
	snapshots.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        0.99,
		TTFTMs:             400,
		TokensPerSecond:    60,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        10,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 41, Name: "shadow-suggestion"}, Group: "default", RuntimeKey: key},
		}),
		snapshots,
		scheduler.DefaultScoreWeights(),
	).WithStickyRouter(sticky)
	facade := modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{Settings: settings}),
		AutoResolver:   policy.NewDefaultAutoGroupResolver(&testkit.FakeGroupPermissionService{}),
		Selector:       selector,
		Recorder:       &testkit.FakeExecutionRecorder{},
	})
	legacy := &testkit.FakeLegacyChannelSelector{
		Channel: &model.Channel{Id: 42, Name: "legacy"},
		Group:   "default",
	}
	wrapper := integration.NewChannelSelectionWrapper(facade, legacy)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.FallbackUsed)
	require.Equal(t, 42, result.Channel.Id)
	_, ok := sticky.Route(ctx, &req, core.GroupSmartPolicy{RequestedGroup: "default"})
	require.False(t, ok)
	require.Empty(t, sticky.StickyEntries())
	_, ok = integration.GetFailedStickyPlan(ctx)
	require.False(t, ok)
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

func TestChannelSelectionWrapperFallbackConsumesRetryRoutingIntent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeOff, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Legacy.Channel = &model.Channel{Id: 5, Name: "legacy-recovery"}
	h.Legacy.Group = "default"
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, h.Legacy)
	ctx, _ := gin.CreateTestContext(nil)
	core.SetRetryRoutingIntent(ctx, core.NewFirstByteTimeoutRetryRoutingIntent(4, "slow-first-byte", 0))

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-4.1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.FallbackUsed)
	require.Equal(t, 5, result.Channel.Id)
	_, ok := core.GetRetryRoutingIntent(ctx)
	require.False(t, ok)
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

func TestDefaultRuntimeObservabilityUsesEventStoreOnlyWhenEnabled(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		integration.ResetDefaultRuntimeObservabilityDeps()
	})

	setting := scheduler_setting.DefaultSetting()
	setting.RuntimeSyncEnabled = true
	setting.RuntimeSyncRedisEnabled = false
	setting.RuntimeSyncEventPushEnabled = true
	setting.RuntimeSyncQueueMinIntervalMs = 10
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restoreSetting)

	integration.ResetDefaultRuntimeObservabilityDeps()
	runtimeDeps := integration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps.RuntimeSyncStore)
	require.NotNil(t, runtimeDeps.RuntimeSyncEventStore)
	require.Same(t, runtimeDeps.RuntimeSyncEventStore, runtimeDeps.RuntimeSyncStore)

	setting.RuntimeSyncEventPushEnabled = false
	restoreSetting()
	restoreSetting = scheduler_setting.SetSettingForTest(setting)
	integration.ResetDefaultRuntimeObservabilityDeps()
	runtimeDeps = integration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps.RuntimeSyncStore)
	require.Nil(t, runtimeDeps.RuntimeSyncEventStore)
}

func newIntegrationStickyContext(tokenID int, sessionID string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"session_id":"`+sessionID+`"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, tokenID)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	return ctx
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

type sequenceFacade struct {
	plans       []*core.DispatchPlan
	SelectCalls int
}

func (f *sequenceFacade) Select(c *gin.Context, param *service.RetryParam) (*core.DispatchPlan, bool, *types.NewAPIError) {
	f.SelectCalls++
	if len(f.plans) == 0 {
		return nil, false, nil
	}
	plan := f.plans[0]
	if len(f.plans) > 1 {
		f.plans = f.plans[1:]
	}
	return plan, true, nil
}

func (f *sequenceFacade) Shadow(c *gin.Context, param *service.RetryParam, actual *model.Channel, actualGroup string) {
}

func (f *sequenceFacade) Report(c *gin.Context, result *core.AttemptResult) {}
