package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/recording"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
)

var (
	defaultWrapper         *ChannelSelectionWrapper
	defaultWrapperOverride *ChannelSelectionWrapper
	defaultRuntime         *DefaultRuntimeObservability
	defaultWrapperMu       sync.Mutex
	defaultWrapperOnce     sync.Once
)

type DefaultRuntimeObservability struct {
	SnapshotStore         *scheduler.SyncedRuntimeSnapshotStore
	LocalSnapshotStore    *scheduler.MemoryRuntimeSnapshotStore
	CircuitBreaker        *scheduler.SyncedCircuitBreaker
	LocalCircuitBreaker   *scheduler.CircuitBreaker
	StickyRouter          *scheduler.MemoryStickyRouter
	RuntimeEnricher       *scheduler.RuntimeSnapshotEnricher
	CostBaselineCache     *scheduler.CostBaselineCache
	RuntimeSyncStore      scheduler.RuntimeSyncStore
	RuntimeSyncEventStore *scheduler.RuntimeSyncEventStore
	QueueSnapshotSyncer   *scheduler.RuntimeQueueSnapshotSyncer
	SnapshotPersistence   *scheduler.RuntimeSnapshotPersistence
	AccountCandidateIndex *AccountCandidateIndexRuntime
	RuntimeSyncNodeID     string
}

func DefaultChannelSelectionWrapper() *ChannelSelectionWrapper {
	defaultWrapperMu.Lock()
	defer defaultWrapperMu.Unlock()
	if defaultWrapperOverride != nil {
		return defaultWrapperOverride
	}
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
		snapshotPersistence := scheduler.NewRuntimeSnapshotPersistence(snapshotStore, scheduler.RuntimeSnapshotPersistenceOptions{})
		costBaselineCache := scheduler.NewCostBaselineCache(time.Duration(maxInt(runtimePolicy.SnapshotRefreshMs, 100)) * time.Millisecond)
		runtimeSnapshotCtx := context.Background()
		if snapshotPersistence.Available(runtimeSnapshotCtx) {
			if err := snapshotPersistence.Restore(runtimeSnapshotCtx); err != nil {
				common.SysLog(fmt.Sprintf("model gateway runtime snapshot restore failed: %v", err))
			}
			hasSnapshots, err := snapshotPersistence.HasPersistedSnapshots(runtimeSnapshotCtx)
			if err != nil {
				common.SysLog(fmt.Sprintf("model gateway runtime snapshot count failed: %v", err))
			}
			hasLatencySamples, sampleErr := snapshotPersistence.HasPersistedLatencySamples(runtimeSnapshotCtx)
			if sampleErr != nil {
				common.SysLog(fmt.Sprintf("model gateway runtime snapshot latency sample count failed: %v", sampleErr))
			}
			if err == nil && (!hasSnapshots || !hasLatencySamples) {
				seeded, seedErr := SeedRuntimeSnapshotsFromExecutionRecords(runtimeSnapshotCtx, snapshotStore, defaultRuntimeSnapshotSeedLimit)
				if seedErr != nil {
					common.SysLog(fmt.Sprintf("model gateway runtime snapshot seed failed: %v", seedErr))
				} else if seeded > 0 {
					if flushErr := snapshotPersistence.Flush(runtimeSnapshotCtx); flushErr != nil {
						common.SysLog(fmt.Sprintf("model gateway runtime snapshot seed flush failed: %v", flushErr))
					} else {
						common.SysLog(fmt.Sprintf("model gateway runtime snapshot seeded from %d historical attempts", seeded))
					}
				}
			}
		}
		snapshotPersistence.Start()
		costBaselineCache.Start(runtimeSnapshotCtx)
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
		runtimeEnricher := scheduler.NewRuntimeSnapshotEnricher(
			scheduler.NewServiceRuntimeStateProvider(),
			runtimePolicy.QueueTimeoutMs,
			runtimePolicy.QueueMaxDepth,
			runtimePolicy.QueueDepthMultiplier,
		).WithCircuitBreaker(circuitBreaker).WithCostBaselineProvider(costBaselineCache)
		candidateBuilder, accountCandidateIndex := coreCandidatePoolBuilder(runtimePolicy)
		smartSelector := scheduler.NewDefaultSmartChannelSelector(
			candidateBuilder,
			snapshotStore,
			runtimePolicy.ScoreWeights,
		).
			WithRuntimeSnapshotEnricher(runtimeEnricher).
			WithCostBaselineProvider(costBaselineCache).
			WithStickyRouter(stickyRouter).
			WithCostFirstStickyEscapeConfig(runtimePolicy.CostFirstStickyEscape).
			WithCostFirstGuardConfig(runtimePolicy.CostFirstGuard)
		healthMonitor := scheduler.NewRuntimeHealthMonitor(snapshotStore, circuitBreaker).
			WithScoringService(scheduler.NewCandidateScoringService().WithCostBaselineProvider(costBaselineCache)).
			WithScoreWeights(runtimePolicy.ScoreWeights)
		recorder := modelgateway.NewExecutionRecorderChain(
			recording.NewAsyncExecutionRecorder(1024).WithPostProcessors(healthMonitor),
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
			RuntimeEnricher:       runtimeEnricher,
			CostBaselineCache:     costBaselineCache,
			RuntimeSyncStore:      runtimeSyncStore,
			RuntimeSyncEventStore: runtimeSyncEventStore,
			QueueSnapshotSyncer:   queueSnapshotSyncer,
			SnapshotPersistence:   snapshotPersistence,
			AccountCandidateIndex: accountCandidateIndex,
			RuntimeSyncNodeID:     runtimeSyncNodeID,
		}
	})
	return defaultWrapper
}

func SetDefaultChannelSelectionWrapperForTest(wrapper *ChannelSelectionWrapper) func() {
	defaultWrapperMu.Lock()
	previousOverride := defaultWrapperOverride
	defaultWrapperOverride = wrapper
	defaultWrapperMu.Unlock()
	return func() {
		defaultWrapperMu.Lock()
		defaultWrapperOverride = previousOverride
		defaultWrapperMu.Unlock()
	}
}

func WarmupDefaultRuntimeObservability() {
	DefaultChannelSelectionWrapper()
}

func DefaultRuntimeObservabilityDeps() *DefaultRuntimeObservability {
	DefaultChannelSelectionWrapper()
	return defaultRuntime
}

func CurrentDefaultRuntimeObservabilityDeps() *DefaultRuntimeObservability {
	defaultWrapperMu.Lock()
	defer defaultWrapperMu.Unlock()
	return defaultRuntime
}

func RefreshDefaultAccountCandidateIndex() {
	defaultWrapperMu.Lock()
	runtimeDeps := defaultRuntime
	defaultWrapperMu.Unlock()
	if runtimeDeps == nil || runtimeDeps.AccountCandidateIndex == nil {
		return
	}
	runtimeDeps.AccountCandidateIndex.Refresh()
}

func ResetDefaultRuntimeObservabilityDeps() {
	defaultWrapperMu.Lock()
	defer defaultWrapperMu.Unlock()
	if defaultRuntime != nil && defaultRuntime.RuntimeSyncEventStore != nil {
		defaultRuntime.RuntimeSyncEventStore.Close()
	}
	if defaultRuntime != nil && defaultRuntime.SnapshotPersistence != nil {
		defaultRuntime.SnapshotPersistence.Close()
	}
	if defaultRuntime != nil && defaultRuntime.CostBaselineCache != nil {
		defaultRuntime.CostBaselineCache.Close()
	}
	if defaultRuntime != nil && defaultRuntime.AccountCandidateIndex != nil {
		defaultRuntime.AccountCandidateIndex.Close()
	}
	defaultWrapperOnce = sync.Once{}
	defaultWrapper = nil
	defaultWrapperOverride = nil
	defaultRuntime = nil
}

func coreCandidatePoolBuilder(runtimePolicy RuntimePolicySettings) (core.CandidatePoolBuilder, *AccountCandidateIndexRuntime) {
	primary := NewModelCandidatePoolBuilder()
	if !runtimePolicy.AccountCandidateIndexEnabled {
		return primary, nil
	}
	refreshInterval := time.Duration(runtimePolicy.AccountCandidateIndexRefreshMs) * time.Millisecond
	indexRuntime := NewAccountCandidateIndexRuntime(nil, AccountCandidateIndexOptions{
		RefreshInterval: refreshInterval,
		ShadowLog:       runtimePolicy.AccountCandidateIndexShadowLog,
	})
	indexRuntime.Start()
	indexed := NewIndexedCandidatePoolBuilder(indexRuntime.Index(), primary.MaxCandidatesPerGroup)
	return NewAccountCandidatePrimaryBuilder(indexed, primary, AccountCandidateIndexOptions{
		RefreshInterval: refreshInterval,
		LogInterval:     refreshInterval,
		ShadowLog:       runtimePolicy.AccountCandidateIndexShadowLog,
	}), indexRuntime
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

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
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
