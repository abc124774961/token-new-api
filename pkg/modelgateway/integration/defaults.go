package integration

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	SnapshotStore         *scheduler.SyncedRuntimeSnapshotStore
	LocalSnapshotStore    *scheduler.MemoryRuntimeSnapshotStore
	CircuitBreaker        *scheduler.SyncedCircuitBreaker
	LocalCircuitBreaker   *scheduler.CircuitBreaker
	StickyRouter          *scheduler.MemoryStickyRouter
	RuntimeSyncStore      scheduler.RuntimeSyncStore
	RuntimeSyncEventStore *scheduler.RuntimeSyncEventStore
	QueueSnapshotSyncer   *scheduler.RuntimeQueueSnapshotSyncer
	RuntimeSyncNodeID     string
}

func DefaultChannelSelectionWrapper() *ChannelSelectionWrapper {
	defaultWrapperMu.Lock()
	defer defaultWrapperMu.Unlock()
	defaultWrapperOnce.Do(func() {
		settingsProvider := NewSchedulerSettingsProvider()
		groupService := NewServiceGroupPermissionService()
		legacySelector := NewLegacyChannelSelector()
		runtimePolicy := RuntimePolicySetting()
		localSnapshotStore := scheduler.NewMemoryRuntimeSnapshotStore()
		runtimeSyncNodeID := defaultRuntimeSyncNodeID(runtimePolicy.RuntimeSyncNodeID)
		runtimeSyncTTL := defaultRuntimeSyncTTL(runtimePolicy)
		var runtimeSyncStore scheduler.RuntimeSyncStore
		var runtimeSyncEventStore *scheduler.RuntimeSyncEventStore
		var queueSnapshotSyncer *scheduler.RuntimeQueueSnapshotSyncer
		if runtimePolicy.RuntimeSyncEnabled {
			runtimeSyncStore = scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
				TTL: runtimeSyncTTL,
				RedisEnabled: func() bool {
					return runtimePolicy.RuntimeSyncRedisEnabled
				},
			})
			if runtimePolicy.RuntimeSyncEventPushEnabled {
				var runtimeSyncEventPublisher scheduler.RuntimeSyncEventPublisher
				if runtimePolicy.RuntimeSyncRedisEnabled && common.RedisEnabled && common.RDB != nil {
					runtimeSyncEventPublisher = scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{
						Client: common.RDB,
						Enabled: func() bool {
							return runtimePolicy.RuntimeSyncRedisEnabled && common.RedisEnabled && common.RDB != nil
						},
					})
				}
				runtimeSyncEventStore = scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
					Store:         runtimeSyncStore,
					FlushInterval: defaultRuntimeSyncEventFlushInterval(runtimePolicy),
					Publisher:     runtimeSyncEventPublisher,
				})
				runtimeSyncStore = runtimeSyncEventStore
			}
			queueSnapshotSyncer = scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{
				Store:       runtimeSyncStore,
				NodeID:      runtimeSyncNodeID,
				MinInterval: time.Duration(runtimePolicy.RuntimeSyncQueueMinInterval) * time.Millisecond,
			})
		}
		snapshotStore := scheduler.NewSyncedRuntimeSnapshotStore(localSnapshotStore, runtimeSyncStore)
		localCircuitBreaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
			FailureThreshold:   runtimePolicy.CircuitFailureThreshold,
			MinSamples:         runtimePolicy.CircuitMinSamples,
			OpenDuration:       time.Duration(runtimePolicy.CircuitOpenSeconds) * time.Second,
			HalfOpenProbeCount: runtimePolicy.CircuitHalfOpenProbeCount,
			ErrorPolicies:      runtimePolicy.CircuitErrorPolicies,
		})
		circuitBreaker := scheduler.NewSyncedCircuitBreaker(localCircuitBreaker, runtimeSyncStore)
		stickyRouter := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{
			TTLSeconds:           runtimePolicy.StickyTTLSeconds,
			StickyKeepScoreRatio: runtimePolicy.StickyKeepScoreRatio,
			CacheKeepScoreRatio:  runtimePolicy.CacheAffinityKeepScoreRatio,
			SaveOnSelect:         runtimePolicy.StickySaveOnSelect,
			RenewOnSuccess:       runtimePolicy.StickyRenewOnSuccess,
			FailurePolicy:        scheduler.StickyFailurePolicy(runtimePolicy.StickyFailurePolicy),
			Store:                scheduler.NewHybridStickyStore(0),
		}, scheduler.NewServiceCacheAffinitySignalAdapter())
		smartSelector := scheduler.NewDefaultSmartChannelSelector(
			NewModelCandidatePoolBuilder(),
			snapshotStore,
			scheduler.NewScoreCalculatorFactory(scheduler.DefaultScoreWeights()),
		).WithRuntimeSnapshotEnricher(scheduler.NewRuntimeSnapshotEnricher(
			scheduler.NewServiceRuntimeStateProvider(),
			runtimePolicy.QueueTimeoutMs,
			runtimePolicy.QueueMaxDepth,
			runtimePolicy.QueueDepthMultiplier,
		).WithCircuitBreaker(circuitBreaker)).WithStickyRouter(stickyRouter)
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
			SnapshotStore:         snapshotStore,
			LocalSnapshotStore:    localSnapshotStore,
			CircuitBreaker:        circuitBreaker,
			LocalCircuitBreaker:   localCircuitBreaker,
			StickyRouter:          stickyRouter,
			RuntimeSyncStore:      runtimeSyncStore,
			RuntimeSyncEventStore: runtimeSyncEventStore,
			QueueSnapshotSyncer:   queueSnapshotSyncer,
			RuntimeSyncNodeID:     runtimeSyncNodeID,
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
	if defaultRuntime != nil && defaultRuntime.RuntimeSyncEventStore != nil {
		defaultRuntime.RuntimeSyncEventStore.Close()
	}
	defaultWrapperOnce = sync.Once{}
	defaultWrapper = nil
	defaultRuntime = nil
}

func defaultRuntimeSyncTTL(settings RuntimePolicySettings) time.Duration {
	if settings.RuntimeSyncTTLSeconds > 0 {
		return time.Duration(settings.RuntimeSyncTTLSeconds) * time.Second
	}
	if settings.CircuitOpenSeconds > 0 {
		return time.Duration(settings.CircuitOpenSeconds*3) * time.Second
	}
	return 90 * time.Second
}

func defaultRuntimeSyncEventFlushInterval(settings RuntimePolicySettings) time.Duration {
	if settings.RuntimeSyncQueueMinInterval > 0 {
		interval := time.Duration(settings.RuntimeSyncQueueMinInterval) * time.Millisecond
		if interval < 100*time.Millisecond {
			return 100 * time.Millisecond
		}
		return interval
	}
	return 500 * time.Millisecond
}

func defaultRuntimeSyncNodeID(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured
	}
	hostname, err := os.Hostname()
	if err == nil {
		hostname = strings.TrimSpace(hostname)
		if hostname != "" {
			return hostname
		}
	}
	return "local"
}
