package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSmallTrafficE2ESmokeSmartAutoQueueAndProxyBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "vip")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 1001)

	channelDefault := &model.Channel{Id: 7201, Name: "legacy-openai", Group: "default"}
	channelFast := &model.Channel{Id: 7202, Name: "mimo-fast", Group: "fast"}
	settings := core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		QueueFairness: core.QueueFairnessPolicySetting{
			HighPriorityThreshold:  100,
			HighPriorityExtraDepth: 1,
			AbsoluteMaxDepth:       2,
		},
		GroupPolicies: map[string]core.GroupPolicySetting{
			"auto": {
				Mode:              core.ModeActive,
				Strategy:          core.StrategySpeedFirst,
				AutoMode:          core.AutoModeFusion,
				QueueEnabled:      true,
				QueueHighPriority: true,
			},
		},
	}
	groupService := &testkit.FakeGroupPermissionService{
		UsableGroups: map[string]map[string]string{
			"vip": {"default": "default", "fast": "fast"},
		},
		AutoGroups: map[string][]string{
			"vip": {"default", "fast"},
		},
	}
	snapshots := testkit.NewFakeRuntimeSnapshotStore()
	putRuntimeSnapshot(snapshots, "mimo-v1", channelDefault.Id, "default", core.RuntimeSnapshot{
		SuccessRate:        0.88,
		TTFTMs:             2800,
		TokensPerSecond:    18,
		ActiveConcurrency:  4,
		MaxConcurrency:     4,
		QueueCapacity:      4,
		QueueTimeoutMs:     200,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	putRuntimeSnapshot(snapshots, "mimo-v1", channelFast.Id, "fast", core.RuntimeSnapshot{
		SuccessRate:          0.99,
		TTFTMs:               320,
		TokensPerSecond:      85,
		ActiveConcurrency:    1,
		MaxConcurrency:       1,
		QueueDepth:           0,
		QueueCapacity:        2,
		QueueTimeoutMs:       200,
		EstimatedQueueWaitMs: 50,
		CostRatio:            1.1,
		GroupPriorityRatio:   1,
		SampleCount:          30,
	})
	facade := newE2ESmokeFacade(settings, groupService, snapshots, []core.Candidate{
		{
			Channel:         channelDefault,
			Group:           "default",
			ProviderProfile: provider.ProfileStandardOpenAICompatible,
			ProxyMode:       provider.ProxyModeNative,
			RuntimeKey:      runtimeKey("mimo-v1", channelDefault.Id, "default"),
		},
		{
			Channel:         channelFast,
			Group:           "fast",
			ProviderProfile: provider.ProfileMiMoCodexChat,
			ProxyMode:       provider.ProxyModeResponsesViaChat,
			RuntimeKey:      runtimeKey("mimo-v1", channelFast.Id, "fast"),
		},
	}, nil, &testkit.FakeLegacyChannelSelector{Channel: channelDefault, Group: "default"})
	wrapper := integration.NewChannelSelectionWrapper(facade, &testkit.FakeLegacyChannelSelector{Channel: channelDefault, Group: "default"})

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "auto",
		ModelName:    "mimo-v1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.SmartHandled)
	require.False(t, result.FallbackUsed)
	require.Equal(t, channelFast.Id, result.Channel.Id)
	require.Equal(t, "fast", result.Group)
	require.Equal(t, provider.ProfileMiMoCodexChat, result.Plan.ProviderProfile)
	require.Equal(t, provider.ProxyModeResponsesViaChat, result.Plan.ProxyMode)
	require.Equal(t, core.AutoModeFusion, result.Plan.AutoMode)
	require.True(t, result.Plan.QueueEnabled)
	require.Equal(t, 100, result.Plan.QueuePriority)
	require.Equal(t, 200, result.Plan.QueueWaitMs)
	requireCandidateSelected(t, result.Plan.Candidates, channelFast.Id, true)
	requireCandidateSelected(t, result.Plan.Candidates, channelDefault.Id, false)

	info := &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
	}
	bridge := integration.NewProxyBridge(nil)
	decision := bridge.Resolve(ctx, info)
	require.True(t, decision.Enabled)
	require.Equal(t, "enabled", decision.Reason)
	input, err := common.Marshal([]map[string]any{{"role": "user", "content": "hello"}})
	require.NoError(t, err)
	converted, handled, err := bridge.ConvertResponsesRequest(ctx, info, &dto.OpenAIResponsesRequest{
		Model: "mimo-v1",
		Input: input,
	})
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", converted.UpstreamPath)
	require.Contains(t, string(converted.UpstreamBody), `"messages"`)

	assertHighPriorityQueueAdmissionFromPlan(t, result.Plan)
}

func TestSmallTrafficE2ESmokeCrossGroupCacheAffinity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "vip")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 1002)

	channelVIP := &model.Channel{Id: 7211, Name: "vip-cache-channel", Group: "vip"}
	channelFast := &model.Channel{Id: 7212, Name: "fast-best-channel", Group: "fast"}
	settings := core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"vip": {
				Mode:                 core.ModeActive,
				Strategy:             core.StrategyBalanced,
				AutoMode:             core.AutoModeSequential,
				CrossGroupFusion:     true,
				CandidateGroups:      []string{"vip", "fast", "forbidden"},
				CacheAffinityEnabled: true,
			},
		},
	}
	groupService := &testkit.FakeGroupPermissionService{
		UsableGroups: map[string]map[string]string{
			"vip": {"vip": "vip", "fast": "fast"},
		},
	}
	snapshots := testkit.NewFakeRuntimeSnapshotStore()
	putRuntimeSnapshot(snapshots, "gpt-5", channelVIP.Id, "vip", core.RuntimeSnapshot{
		SuccessRate:        0.91,
		TTFTMs:             900,
		TokensPerSecond:    35,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		QueueCapacity:      8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	putRuntimeSnapshot(snapshots, "gpt-5", channelFast.Id, "fast", core.RuntimeSnapshot{
		SuccessRate:        0.98,
		TTFTMs:             340,
		TokensPerSecond:    80,
		ActiveConcurrency:  1,
		MaxConcurrency:     8,
		QueueCapacity:      8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
		CacheKeepScoreRatio: 0.65,
	}, scheduler.NewStaticCacheAffinitySignalAdapter(core.CacheAffinitySignal{
		Key:                "cache-affinity-e2e",
		KeyFingerprint:     "cache-fp-e2e",
		Source:             "prompt_cache_key",
		PreferredChannelID: channelVIP.Id,
		PreferredGroup:     "vip",
	}, true))
	facade := newE2ESmokeFacade(settings, groupService, snapshots, []core.Candidate{
		{
			Channel:         channelVIP,
			Group:           "vip",
			ProviderProfile: provider.ProfileOpenAICodex,
			ProxyMode:       provider.ProxyModeNativeResponses,
			RuntimeKey:      runtimeKey("gpt-5", channelVIP.Id, "vip"),
		},
		{
			Channel:         channelFast,
			Group:           "fast",
			ProviderProfile: provider.ProfileMiMoCodexChat,
			ProxyMode:       provider.ProxyModeResponsesViaChat,
			RuntimeKey:      runtimeKey("gpt-5", channelFast.Id, "fast"),
		},
	}, sticky, &testkit.FakeLegacyChannelSelector{Channel: channelVIP, Group: "vip"})
	wrapper := integration.NewChannelSelectionWrapper(facade, &testkit.FakeLegacyChannelSelector{Channel: channelVIP, Group: "vip"})

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "vip",
		ModelName:    "gpt-5",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.True(t, result.SmartHandled)
	require.Equal(t, channelVIP.Id, result.Channel.Id)
	require.Equal(t, "vip", result.Group)
	require.True(t, result.Plan.CacheAffinity)
	require.True(t, result.Plan.StickyRetained)
	require.Equal(t, "cache_affinity", result.Plan.StickySource)
	require.Equal(t, "cache-fp-e2e", result.Plan.StickyKeyFP)
	requireCandidateSelected(t, result.Plan.Candidates, channelVIP.Id, true)
	requireCandidateSelected(t, result.Plan.Candidates, channelFast.Id, false)
	requireNoCandidateForGroup(t, result.Plan.Candidates, "forbidden")
}

func TestSmallTrafficE2ESmokeGroupOffFallsBackAndDisablesProxyBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 1003)

	legacy := &testkit.FakeLegacyChannelSelector{
		Channel: &model.Channel{Id: 7221, Name: "legacy-default", Group: "default"},
		Group:   "default",
	}
	h := testkit.NewDispatchTestHarness(core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			"default": {Mode: core.ModeOff, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
		},
	})
	h.Legacy = legacy
	wrapper := integration.NewChannelSelectionWrapper(h.Facade, legacy)

	result, apiErr := wrapper.Select(ctx, &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "mimo-v1",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, apiErr)
	require.False(t, result.SmartHandled)
	require.True(t, result.FallbackUsed)
	require.Equal(t, legacy.Channel.Id, result.Channel.Id)
	require.Equal(t, 1, legacy.Calls)
	_, ok := integration.GetSelectedPlan(ctx)
	require.False(t, ok)
	decision := integration.NewProxyBridge(nil).Resolve(ctx, &relaycommon.RelayInfo{
		RelayMode:        relayconstant.RelayModeResponses,
		RequestModelName: "mimo-v1",
		OriginModelName:  "mimo-v1",
	})
	require.False(t, decision.Enabled)
	require.Equal(t, "no_smart_dispatch_plan", decision.Reason)
}

func newE2ESmokeFacade(settings core.SchedulerSettings, groupService *testkit.FakeGroupPermissionService, snapshots *testkit.FakeRuntimeSnapshotStore, candidates []core.Candidate, sticky core.StickyRouter, legacy core.LegacyChannelSelector) *modelgateway.SmartDispatchFacade {
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder(candidates),
		snapshots,
		scheduler.DefaultScoreWeights(),
	)
	if sticky != nil {
		selector = selector.WithStickyRouter(sticky)
	}
	return modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{Settings: settings}),
		AutoResolver:   policy.NewDefaultAutoGroupResolver(groupService),
		Selector:       selector,
		LegacySelector: legacy,
		Recorder:       &testkit.FakeExecutionRecorder{},
	})
}

func putRuntimeSnapshot(store *testkit.FakeRuntimeSnapshotStore, modelName string, channelID int, group string, snapshot core.RuntimeSnapshot) {
	snapshot.Key = runtimeKey(modelName, channelID, group)
	store.Put(snapshot)
}

func runtimeKey(modelName string, channelID int, group string) core.RuntimeKey {
	return core.RuntimeKey{
		RequestedModel: modelName,
		ChannelID:      channelID,
		Group:          group,
		EndpointType:   constant.EndpointTypeOpenAI,
	}
}

func assertHighPriorityQueueAdmissionFromPlan(t *testing.T, plan *core.DispatchPlan) {
	t.Helper()
	require.NotNil(t, plan)
	require.NotNil(t, plan.Channel)

	held, acquired := service.TryAcquireChannelConcurrency(plan.Channel.Id, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer held.Release()

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 1, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityThreshold:  plan.QueuePriority,
		HighPriorityExtraDepth: 1,
		AbsoluteMaxDepth:       2,
	}))
	waitingPlan := *plan
	waitingPlan.QueueWaitMs = 400

	normalCtx, cancelNormal := context.WithCancel(context.Background())
	normalDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		normalDone <- manager.AcquireWithOptions(normalCtx, &waitingPlan, plan.Channel.Id, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group:      "default",
			Priority:   0,
			RuntimeKey: runtimeKey("mimo-v1", plan.Channel.Id, "default"),
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(plan.Channel.Id) == 1
	}, 200*time.Millisecond, 10*time.Millisecond)

	highCtx, cancelHigh := context.WithCancel(context.Background())
	highDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		highDone <- manager.AcquireWithOptions(highCtx, &waitingPlan, plan.Channel.Id, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group:      plan.SelectedGroup,
			Priority:   plan.QueuePriority,
			RuntimeKey: plan.RuntimeKey,
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(plan.Channel.Id) == 2
	}, 200*time.Millisecond, 10*time.Millisecond)

	cancelHigh()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-highDone).Status)
	cancelNormal()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-normalDone).Status)
	require.Eventually(t, func() bool {
		return manager.Depth(plan.Channel.Id) == 0
	}, 200*time.Millisecond, 10*time.Millisecond)
}

func requireCandidateSelected(t *testing.T, candidates []core.CandidateExplanation, channelID int, selected bool) {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.ChannelID == channelID {
			require.Equal(t, selected, candidate.Selected, "channel %d selected mismatch", channelID)
			require.True(t, candidate.Available, "channel %d should be available", channelID)
			return
		}
	}
	require.Failf(t, "missing candidate", "channel %d not found", channelID)
}

func requireNoCandidateForGroup(t *testing.T, candidates []core.CandidateExplanation, group string) {
	t.Helper()
	for _, candidate := range candidates {
		require.NotEqual(t, group, candidate.Group)
	}
}
