package integration

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/recording"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
)

var (
	defaultWrapper     *ChannelSelectionWrapper
	defaultRuntime     *DefaultRuntimeObservability
	defaultWrapperMu   sync.Mutex
	defaultWrapperOnce sync.Once
)

type DefaultRuntimeObservability struct {
	SnapshotStore  *scheduler.MemoryRuntimeSnapshotStore
	CircuitBreaker *scheduler.CircuitBreaker
}

func DefaultChannelSelectionWrapper() *ChannelSelectionWrapper {
	defaultWrapperMu.Lock()
	defer defaultWrapperMu.Unlock()
	defaultWrapperOnce.Do(func() {
		settingsProvider := NewSchedulerSettingsProvider()
		groupService := NewServiceGroupPermissionService()
		legacySelector := NewLegacyChannelSelector()
		runtimePolicy := RuntimePolicySetting()
		snapshotStore := scheduler.NewMemoryRuntimeSnapshotStore()
		circuitBreaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
			FailureThreshold:   runtimePolicy.CircuitFailureThreshold,
			MinSamples:         runtimePolicy.CircuitMinSamples,
			OpenDuration:       time.Duration(runtimePolicy.CircuitOpenSeconds) * time.Second,
			HalfOpenProbeCount: runtimePolicy.CircuitHalfOpenProbeCount,
		})
		smartSelector := scheduler.NewDefaultSmartChannelSelector(
			NewModelCandidatePoolBuilder(),
			snapshotStore,
			scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
		).WithRuntimeSnapshotEnricher(scheduler.NewRuntimeSnapshotEnricher(
			scheduler.NewServiceRuntimeStateProvider(),
			runtimePolicy.QueueTimeoutMs,
			runtimePolicy.QueueMaxDepth,
			runtimePolicy.QueueDepthMultiplier,
		).WithCircuitBreaker(circuitBreaker)).WithStickyRouter(scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
			TTLSeconds:           runtimePolicy.StickyTTLSeconds,
			StickyKeepScoreRatio: runtimePolicy.StickyKeepScoreRatio,
			CacheKeepScoreRatio:  runtimePolicy.CacheAffinityKeepScoreRatio,
			Store:                scheduler.NewHybridStickyStore(0),
		}, scheduler.NewServiceCacheAffinitySignalAdapter()))
		recorder := modelgateway.NewExecutionRecorderChain(
			recording.NewAsyncExecutionRecorder(1024),
			scheduler.NewRuntimeHealthMonitor(snapshotStore, circuitBreaker),
		)
		facade := modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
			PolicyResolver: policy.NewDefaultGroupPolicyResolver(settingsProvider),
			AutoResolver:   policy.NewDefaultAutoGroupResolver(groupService),
			Selector:       smartSelector,
			LegacySelector: legacySelector,
			Recorder:       recorder,
		})
		defaultWrapper = NewChannelSelectionWrapper(facade, legacySelector)
		defaultRuntime = &DefaultRuntimeObservability{
			SnapshotStore:  snapshotStore,
			CircuitBreaker: circuitBreaker,
		}
	})
	return defaultWrapper
}

func DefaultRuntimeObservabilityDeps() *DefaultRuntimeObservability {
	DefaultChannelSelectionWrapper()
	return defaultRuntime
}

func ResetDefaultRuntimeObservabilityDeps() {
	defaultWrapperMu.Lock()
	defer defaultWrapperMu.Unlock()
	defaultWrapperOnce = sync.Once{}
	defaultWrapper = nil
	defaultRuntime = nil
}
