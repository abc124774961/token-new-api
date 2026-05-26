package testkit

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/gin-gonic/gin"
)

type DispatchTestHarness struct {
	Facade        *modelgateway.SmartDispatchFacade
	Settings      StaticSettingsProvider
	GroupService  *FakeGroupPermissionService
	Legacy        *FakeLegacyChannelSelector
	Selector      *FakeSmartChannelSelector
	Recorder      *FakeExecutionRecorder
	SnapshotStore *FakeRuntimeSnapshotStore
}

func NewDispatchTestHarness(settings core.SchedulerSettings) *DispatchTestHarness {
	groupService := &FakeGroupPermissionService{
		UsableGroups: map[string]map[string]string{},
		AutoGroups:   map[string][]string{},
	}
	settingsProvider := StaticSettingsProvider{Settings: settings}
	selector := &FakeSmartChannelSelector{}
	recorder := &FakeExecutionRecorder{}
	h := &DispatchTestHarness{
		Settings:      settingsProvider,
		GroupService:  groupService,
		Legacy:        &FakeLegacyChannelSelector{},
		Selector:      selector,
		Recorder:      recorder,
		SnapshotStore: NewFakeRuntimeSnapshotStore(),
	}
	h.Facade = modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(settingsProvider),
		AutoResolver:   policy.NewDefaultAutoGroupResolver(groupService),
		Selector:       selector,
		LegacySelector: h.Legacy,
		Recorder:       recorder,
	})
	return h
}

func NewScenarioHarness(scenario *DispatchScenario) *DispatchTestHarness {
	return NewScenarioHarnessWithContext(scenario, nil)
}

func NewScenarioHarnessWithContext(scenario *DispatchScenario, ctx *gin.Context) *DispatchTestHarness {
	settings := core.SchedulerSettings{
		Enabled:         true,
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
		GroupPolicies: map[string]core.GroupPolicySetting{
			scenario.Request.RequestedGroup: {
				Mode:                 scenario.Policy.Mode,
				Strategy:             scenario.Policy.Strategy,
				AutoMode:             scenario.Policy.AutoMode,
				CrossGroupFusion:     scenario.Policy.CrossGroupFusion,
				CandidateGroups:      append([]string(nil), scenario.Policy.CandidateGroups...),
				QueueEnabled:         scenario.Policy.QueueEnabled,
				CacheAffinityEnabled: scenario.CacheAffinity != nil && scenario.CacheAffinity.Enabled,
			},
		},
	}
	h := NewDispatchTestHarness(settings)
	usable := map[string]string{}
	for _, group := range scenario.UsableGroups {
		usable[group] = group
	}
	h.GroupService.UsableGroups[scenario.Request.UserGroup] = usable
	h.GroupService.AutoGroups[scenario.Request.UserGroup] = append([]string(nil), scenario.AutoGroups...)

	channels := make([]*model.Channel, 0, len(scenario.Channels))
	groups := map[int]string{}
	for _, fixture := range scenario.Channels {
		channels = append(channels, &model.Channel{
			Id:    fixture.ID,
			Name:  fixture.Name,
			Group: fixture.Group,
		})
		groups[fixture.ID] = fixture.Group
	}
	h.Selector = nil
	h.SnapshotStore = NewFakeRuntimeSnapshotStore()
	for _, fixture := range scenario.RuntimeSnapshots {
		h.SnapshotStore.Put(runtimeSnapshotFromFixture(scenario, fixture))
	}
	realSelector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilderFromChannels(channels, groups),
		h.SnapshotStore,
		scheduler.DefaultScoreWeights(),
	)
	if scenario.StickyState != nil || scenario.CacheAffinity != nil {
		realSelector = realSelector.WithStickyRouter(stickyRouterFromScenario(scenario, ctx))
	}
	h.Facade = modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(h.Settings),
		AutoResolver:   policy.NewDefaultAutoGroupResolver(h.GroupService),
		Selector:       realSelector,
		LegacySelector: h.Legacy,
		Recorder:       h.Recorder,
	})
	return h
}

func stickyRouterFromScenario(scenario *DispatchScenario, ctx *gin.Context) core.StickyRouter {
	options := scheduler.StickyRouterOptions{}
	var adapter core.CacheAffinitySignalAdapter
	if scenario.CacheAffinity != nil {
		options.CacheKeepScoreRatio = scenario.CacheAffinity.KeepScoreRatio
		adapter = scheduler.NewStaticCacheAffinitySignalAdapter(core.CacheAffinitySignal{
			Key:                "scenario-cache-affinity",
			KeyFingerprint:     scenario.CacheAffinity.KeyFingerprint,
			Source:             "scenario",
			PreferredChannelID: scenario.CacheAffinity.ChannelID,
			PreferredGroup:     scenario.CacheAffinity.Group,
		}, scenario.CacheAffinity.Enabled)
	}
	if scenario.StickyState != nil {
		options.StickyKeepScoreRatio = scenario.StickyState.KeepScoreRatio
	}
	router := scheduler.NewMemoryStickyRouter(options, adapter)
	if scenario.StickyState != nil {
		router.Save(ctx, &core.DispatchRequest{
			RequestedGroup: scenario.Request.RequestedGroup,
			UserGroup:      scenario.Request.UserGroup,
			ModelName:      scenario.Request.ModelName,
		}, &core.DispatchPlan{
			Channel:       &model.Channel{Id: scenario.StickyState.ChannelID},
			SelectedGroup: scenario.StickyState.Group,
		})
	}
	return router
}

func runtimeSnapshotFromFixture(scenario *DispatchScenario, fixture RuntimeSnapshotFixture) core.RuntimeSnapshot {
	return core.RuntimeSnapshot{
		Key: core.RuntimeKey{
			RequestedModel: scenario.Request.ModelName,
			ChannelID:      fixture.ChannelID,
			Group:          fixture.Group,
		},
		SuccessRate:          fixture.SuccessRate,
		TTFTMs:               fixture.TTFTMs,
		DurationMs:           fixture.DurationMs,
		TokensPerSecond:      fixture.TokensPerSecond,
		ActiveConcurrency:    fixture.ActiveConcurrency,
		MaxConcurrency:       fixture.MaxConcurrency,
		QueueDepth:           fixture.QueueDepth,
		QueueCapacity:        fixture.MaxConcurrency * 2,
		QueueTimeoutMs:       2000,
		EstimatedQueueWaitMs: fixture.EstimatedQueueWaitMs,
		CostRatio:            fixture.CostRatio,
		GroupPriorityRatio:   fixture.GroupPriorityRatio,
		CircuitOpen:          fixture.CircuitOpen,
		Cooldown:             fixture.Cooldown,
		FailureAvoidance:     fixture.FailureAvoidance,
		SampleCount:          fixture.SampleCount,
	}
}
