package scheduler_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestSyncedRuntimeSnapshotStoreMergesLocalAndSharedSnapshots(t *testing.T) {
	syncStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
	})
	local := scheduler.NewMemoryRuntimeSnapshotStore()
	store := scheduler.NewSyncedRuntimeSnapshotStore(local, syncStore)

	localKey := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      101,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	remoteKey := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo",
		ChannelID:      202,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	store.Put(core.RuntimeSnapshot{
		Key:         localKey,
		SuccessRate: 0.99,
		SampleCount: 3,
	})
	syncStore.PutSnapshot(core.RuntimeSnapshot{
		Key:         remoteKey,
		SuccessRate: 0.91,
		SampleCount: 7,
	})

	snapshots := store.ListCandidates(&core.DispatchRequest{ModelName: "gpt-5.5"})
	require.Len(t, snapshots, 2)
	require.Equal(t, 101, snapshots[0].Key.ChannelID)
	require.Equal(t, 202, snapshots[1].Key.ChannelID)

	localSnapshot, ok := store.Get(localKey)
	require.True(t, ok)
	require.Equal(t, 0.99, localSnapshot.SuccessRate)

	remoteSnapshot, ok := store.Get(remoteKey)
	require.True(t, ok)
	require.Equal(t, 0.91, remoteSnapshot.SuccessRate)
}

func TestSyncedCircuitBreakerSharesOpenCircuitState(t *testing.T) {
	syncStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
	})
	key := core.RuntimeKey{
		RequestedModel: "deepseek-v4-pro",
		ChannelID:      303,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	options := scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         2,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	}
	writer := scheduler.NewSyncedCircuitBreaker(scheduler.NewCircuitBreaker(options), syncStore)
	reader := scheduler.NewSyncedCircuitBreaker(scheduler.NewCircuitBreaker(options), syncStore)

	writer.Report(core.AttemptResult{Key: key, ChannelID: key.ChannelID, StatusCode: http.StatusInternalServerError})
	writer.Report(core.AttemptResult{Key: key, ChannelID: key.ChannelID, StatusCode: http.StatusBadGateway})

	snapshot := reader.Snapshot(key)
	require.Equal(t, core.CircuitStateOpen, snapshot.State)
	require.False(t, reader.AllowProbe(key))
	require.Len(t, reader.ListSnapshots(), 1)
}

func TestRuntimeQueueSnapshotSyncAggregatesNodesAndRuntimeKeys(t *testing.T) {
	syncStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
	})
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo",
		ChannelID:      404,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	local := core.RuntimeQueueSnapshot{
		UpdatedAt: 100,
		Summary:   core.RuntimeQueueSummary{UpdatedAt: 100},
		Channels: []core.RuntimeQueueChannelSnapshot{
			{
				ChannelID:            404,
				QueueDepth:           2,
				QueuedRequests:       2,
				WaitingRequests:      2,
				QueueCapacity:        8,
				HighPriorityDepth:    1,
				NormalDepth:          1,
				HighPriorityCapacity: 4,
				NormalCapacity:       4,
				Groups: []core.RuntimeQueueGroupSnapshot{
					{ChannelID: 404, Group: "vip", QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2, HighPriorityDepth: 1, NormalDepth: 1},
				},
			},
		},
		RuntimeKeys: []core.RuntimeQueueKeySnapshot{
			{
				RuntimeKey:        key,
				RequestedModel:    key.RequestedModel,
				UpstreamModel:     key.UpstreamModel,
				ChannelID:         key.ChannelID,
				Group:             key.Group,
				EndpointType:      string(key.EndpointType),
				QueueDepth:        2,
				QueuedRequests:    2,
				WaitingRequests:   2,
				HighPriorityDepth: 1,
				NormalDepth:       1,
			},
		},
	}
	remote := core.RuntimeQueueSnapshot{
		UpdatedAt: 110,
		Summary:   core.RuntimeQueueSummary{UpdatedAt: 110},
		Channels: []core.RuntimeQueueChannelSnapshot{
			{
				ChannelID:         404,
				QueueDepth:        3,
				QueuedRequests:    3,
				WaitingRequests:   3,
				QueueCapacity:     8,
				HighPriorityDepth: 2,
				NormalDepth:       1,
				Groups: []core.RuntimeQueueGroupSnapshot{
					{ChannelID: 404, Group: "vip", QueueDepth: 3, QueuedRequests: 3, WaitingRequests: 3, HighPriorityDepth: 2, NormalDepth: 1},
				},
			},
		},
		RuntimeKeys: []core.RuntimeQueueKeySnapshot{
			{
				RuntimeKey:        key,
				RequestedModel:    key.RequestedModel,
				UpstreamModel:     key.UpstreamModel,
				ChannelID:         key.ChannelID,
				Group:             key.Group,
				EndpointType:      string(key.EndpointType),
				QueueDepth:        3,
				QueuedRequests:    3,
				WaitingRequests:   3,
				HighPriorityDepth: 2,
				NormalDepth:       1,
			},
		},
	}

	syncStore.PutQueueSnapshot("node-b", remote)
	aggregated := scheduler.SyncAndAggregateQueueSnapshot(syncStore, "node-a", local)

	require.Equal(t, int64(110), aggregated.UpdatedAt)
	require.Equal(t, 5, aggregated.Summary.TotalQueued)
	require.Equal(t, 3, aggregated.Summary.HighPriorityDepth)
	require.Equal(t, 2, aggregated.Summary.NormalDepth)
	require.Len(t, aggregated.Channels, 1)
	require.Equal(t, 5, aggregated.Channels[0].QueueDepth)
	require.Len(t, aggregated.RuntimeKeys, 1)
	require.Equal(t, 5, aggregated.RuntimeKeys[0].QueueDepth)
	require.Len(t, aggregated.Groups, 1)
	require.Equal(t, 5, aggregated.Groups[0].QueueDepth)
	require.Equal(t, 2, aggregated.Summary.QueueNodes)
	require.Len(t, aggregated.Nodes, 2)
	require.Equal(t, "node-b", aggregated.Nodes[0].NodeID)
	require.Equal(t, 3, aggregated.Nodes[0].Summary.TotalQueued)
	require.Equal(t, "node-a", aggregated.Nodes[1].NodeID)
	require.Equal(t, 2, aggregated.Nodes[1].Summary.TotalQueued)
}

func TestRuntimeQueueSnapshotSyncerThrottlesWritesAndUsesLatestLocalSnapshot(t *testing.T) {
	syncStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
	})
	now := time.Unix(1710000000, 0)
	syncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{
		Store:       syncStore,
		NodeID:      "node-a",
		MinInterval: time.Second,
		Now: func() time.Time {
			return now
		},
	})

	first := core.RuntimeQueueSnapshot{
		UpdatedAt: 100,
		Summary:   core.RuntimeQueueSummary{UpdatedAt: 100},
		Channels: []core.RuntimeQueueChannelSnapshot{
			{ChannelID: 701, QueueDepth: 1, QueuedRequests: 1, WaitingRequests: 1, QueueCapacity: 4},
		},
	}
	second := core.RuntimeQueueSnapshot{
		UpdatedAt: 101,
		Summary:   core.RuntimeQueueSummary{UpdatedAt: 101},
		Channels: []core.RuntimeQueueChannelSnapshot{
			{ChannelID: 701, QueueDepth: 3, QueuedRequests: 3, WaitingRequests: 3, QueueCapacity: 4},
		},
	}

	aggregated := syncer.SyncAndAggregate(first)
	require.Equal(t, 1, aggregated.Summary.TotalQueued)
	stored := syncStore.ListQueueSnapshots()
	require.Len(t, stored, 1)
	require.Equal(t, 1, stored[0].Channels[0].QueueDepth)

	aggregated = syncer.SyncAndAggregate(second)
	require.Equal(t, 3, aggregated.Summary.TotalQueued)
	stored = syncStore.ListQueueSnapshots()
	require.Len(t, stored, 1)
	require.Equal(t, 1, stored[0].Channels[0].QueueDepth)

	now = now.Add(time.Second)
	aggregated = syncer.SyncAndAggregate(second)
	require.Equal(t, 3, aggregated.Summary.TotalQueued)
	stored = syncStore.ListQueueSnapshots()
	require.Len(t, stored, 1)
	require.Equal(t, 3, stored[0].Channels[0].QueueDepth)
}

func TestHybridRuntimeSyncStoreCanDisableRedisFallbackStillWorks(t *testing.T) {
	syncStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
		RedisEnabled: func() bool {
			return false
		},
	})
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      909,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}

	syncStore.PutSnapshot(core.RuntimeSnapshot{
		Key:         key,
		SuccessRate: 0.97,
		SampleCount: 4,
	})
	snapshots := syncStore.ListSnapshots(&core.DispatchRequest{ModelName: "gpt-5.5"})
	require.Len(t, snapshots, 1)
	require.Equal(t, 909, snapshots[0].Key.ChannelID)

	syncStore.PutCircuit(core.CircuitSnapshot{
		Key:   key,
		State: core.CircuitStateOpen,
	})
	circuit, ok := syncStore.GetCircuit(key)
	require.True(t, ok)
	require.Equal(t, core.CircuitStateOpen, circuit.State)
}

func TestRuntimeQueueSnapshotSyncerSkipsSharedWriteWithinMinIntervalButUsesLatestLocal(t *testing.T) {
	store := newRecordingRuntimeSyncStore()
	store.PutQueueSnapshot("node-b", runtimeQueueSnapshotForTest("node-b", 504, "vip", 3, 110))

	now := time.Unix(200, 0)
	syncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{
		Store:       store,
		NodeID:      "node-a",
		MinInterval: time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	first := runtimeQueueSnapshotForTest("", 504, "vip", 2, 120)
	aggregated := syncer.SyncAndAggregate(first)
	require.Equal(t, 1, store.queuePutCount("node-a"))
	require.Equal(t, 5, aggregated.Summary.TotalQueued)

	second := runtimeQueueSnapshotForTest("", 504, "vip", 7, 130)
	now = now.Add(30 * time.Second)
	aggregated = syncer.SyncAndAggregate(second)

	require.Equal(t, 1, store.queuePutCount("node-a"))
	stored, ok := store.queueSnapshot("node-a")
	require.True(t, ok)
	require.Equal(t, 2, stored.Channels[0].QueueDepth)
	require.Equal(t, 10, aggregated.Summary.TotalQueued)
	require.Equal(t, 10, aggregated.Channels[0].QueueDepth)
	require.Equal(t, 10, aggregated.RuntimeKeys[0].QueueDepth)
	require.Equal(t, int64(130), aggregated.UpdatedAt)
}

func TestRuntimeQueueSnapshotSyncerWritesAgainAfterMinInterval(t *testing.T) {
	store := newRecordingRuntimeSyncStore()
	now := time.Unix(300, 0)
	syncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{
		Store:       store,
		NodeID:      "node-a",
		MinInterval: time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	syncer.SyncAndAggregate(runtimeQueueSnapshotForTest("", 604, "default", 2, 210))
	now = now.Add(time.Minute)
	aggregated := syncer.SyncAndAggregate(runtimeQueueSnapshotForTest("", 604, "default", 6, 220))

	require.Equal(t, 2, store.queuePutCount("node-a"))
	stored, ok := store.queueSnapshot("node-a")
	require.True(t, ok)
	require.Equal(t, 6, stored.Channels[0].QueueDepth)
	require.Equal(t, 6, aggregated.Summary.TotalQueued)
	require.Equal(t, int64(220), aggregated.UpdatedAt)
}

func TestRuntimeSyncMultiNodeSmokeWithEventStoreAndMemoryFallback(t *testing.T) {
	baseStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
		RedisEnabled: func() bool {
			return false
		},
	})
	eventStore := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         baseStore,
		FlushInterval: time.Hour,
		QueueSize:     16,
	})
	t.Cleanup(eventStore.Close)

	nodeAStore := scheduler.NewSyncedRuntimeSnapshotStore(scheduler.NewMemoryRuntimeSnapshotStore(), eventStore)
	nodeBStore := scheduler.NewSyncedRuntimeSnapshotStore(scheduler.NewMemoryRuntimeSnapshotStore(), eventStore)
	keyA := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "openai-codex",
		ChannelID:      801,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	keyB := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo",
		ChannelID:      802,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	nodeAStore.Put(core.RuntimeSnapshot{Key: keyA, SuccessRate: 0.99, SampleCount: 5, DurationMs: 800})
	nodeBStore.Put(core.RuntimeSnapshot{Key: keyB, SuccessRate: 0.92, SampleCount: 7, DurationMs: 1200})

	nodeAView := nodeAStore.ListCandidates(&core.DispatchRequest{ModelName: "gpt-5.5"})
	require.Len(t, nodeAView, 2)
	require.Equal(t, 801, nodeAView[0].Key.ChannelID)
	require.Equal(t, 802, nodeAView[1].Key.ChannelID)

	circuitOptions := scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         2,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	}
	nodeABreaker := scheduler.NewSyncedCircuitBreaker(scheduler.NewCircuitBreaker(circuitOptions), eventStore)
	nodeBBreaker := scheduler.NewSyncedCircuitBreaker(scheduler.NewCircuitBreaker(circuitOptions), eventStore)
	nodeBBreaker.Report(core.AttemptResult{Key: keyB, ChannelID: keyB.ChannelID, StatusCode: http.StatusInternalServerError})
	nodeBBreaker.Report(core.AttemptResult{Key: keyB, ChannelID: keyB.ChannelID, StatusCode: http.StatusBadGateway})

	require.Equal(t, core.CircuitStateOpen, nodeABreaker.Snapshot(keyB).State)
	require.False(t, nodeABreaker.AllowProbe(keyB))

	nodeASyncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{
		Store:  eventStore,
		NodeID: "node-a",
	})
	nodeBSyncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{
		Store:  eventStore,
		NodeID: "node-b",
	})
	nodeAQueue := runtimeQueueSnapshotForTest("", keyA.ChannelID, keyA.Group, 2, 510)
	nodeAQueue.RuntimeKeys[0].RuntimeKey = keyA
	nodeAQueue.RuntimeKeys[0].RequestedModel = keyA.RequestedModel
	nodeAQueue.RuntimeKeys[0].UpstreamModel = keyA.UpstreamModel
	nodeAQueue.RuntimeKeys[0].EndpointType = string(keyA.EndpointType)
	nodeBQueue := runtimeQueueSnapshotForTest("", keyB.ChannelID, keyB.Group, 4, 520)
	nodeBQueue.RuntimeKeys[0].RuntimeKey = keyB
	nodeBQueue.RuntimeKeys[0].RequestedModel = keyB.RequestedModel
	nodeBQueue.RuntimeKeys[0].UpstreamModel = keyB.UpstreamModel
	nodeBQueue.RuntimeKeys[0].EndpointType = string(keyB.EndpointType)

	nodeASyncer.SyncAndAggregate(nodeAQueue)
	aggregated := nodeBSyncer.SyncAndAggregate(nodeBQueue)

	require.Equal(t, int64(520), aggregated.UpdatedAt)
	require.Equal(t, 6, aggregated.Summary.TotalQueued)
	require.Equal(t, 2, aggregated.Summary.QueueNodes)
	require.Len(t, aggregated.Nodes, 2)
	require.Equal(t, "node-b", aggregated.Nodes[0].NodeID)
	require.Equal(t, "node-a", aggregated.Nodes[1].NodeID)
	require.Len(t, aggregated.RuntimeKeys, 2)

	eventStore.Flush()
	sharedSnapshots := baseStore.ListSnapshots(&core.DispatchRequest{ModelName: "gpt-5.5"})
	require.Len(t, sharedSnapshots, 2)
	sharedQueues := baseStore.ListQueueSnapshots()
	require.Len(t, sharedQueues, 2)
	sharedCircuit, ok := baseStore.GetCircuit(keyB)
	require.True(t, ok)
	require.Equal(t, core.CircuitStateOpen, sharedCircuit.State)
}

func TestRuntimeSyncMultiNodeSmokeWithRedisOptIn(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis-backed runtime sync smoke")
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	t.Cleanup(func() {
		_ = client.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.FlushDB(cleanupCtx).Err()
	})

	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	baseStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
		RedisEnabled: func() bool {
			return true
		},
	})
	eventStore := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         baseStore,
		FlushInterval: time.Hour,
		QueueSize:     16,
	})
	t.Cleanup(eventStore.Close)

	keyA := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "openai-codex",
		ChannelID:      1201,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	keyB := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "deepseek-v4-pro",
		ChannelID:      1202,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	nodeAStore := scheduler.NewSyncedRuntimeSnapshotStore(scheduler.NewMemoryRuntimeSnapshotStore(), eventStore)
	nodeBStore := scheduler.NewSyncedRuntimeSnapshotStore(scheduler.NewMemoryRuntimeSnapshotStore(), eventStore)
	nodeAStore.Put(core.RuntimeSnapshot{Key: keyA, SuccessRate: 0.98, SampleCount: 8})
	nodeBStore.Put(core.RuntimeSnapshot{Key: keyB, SuccessRate: 0.93, SampleCount: 9})

	eventStore.Flush()
	redisBackedStore := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
		RedisEnabled: func() bool {
			return true
		},
	})
	snapshots := redisBackedStore.ListSnapshots(&core.DispatchRequest{ModelName: "gpt-5.5"})
	require.Len(t, snapshots, 2)

	circuitOptions := scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         2,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	}
	nodeBBreaker := scheduler.NewSyncedCircuitBreaker(scheduler.NewCircuitBreaker(circuitOptions), eventStore)
	nodeBBreaker.Report(core.AttemptResult{Key: keyB, ChannelID: keyB.ChannelID, StatusCode: http.StatusInternalServerError})
	nodeBBreaker.Report(core.AttemptResult{Key: keyB, ChannelID: keyB.ChannelID, StatusCode: http.StatusBadGateway})
	eventStore.Flush()
	circuit, ok := redisBackedStore.GetCircuit(keyB)
	require.True(t, ok)
	require.Equal(t, core.CircuitStateOpen, circuit.State)

	nodeASyncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{Store: eventStore, NodeID: "redis-node-a"})
	nodeBSyncer := scheduler.NewRuntimeQueueSnapshotSyncer(scheduler.RuntimeQueueSnapshotSyncerOptions{Store: eventStore, NodeID: "redis-node-b"})
	queueA := runtimeQueueSnapshotForTest("", keyA.ChannelID, keyA.Group, 3, 710)
	queueA.RuntimeKeys[0].RuntimeKey = keyA
	queueA.RuntimeKeys[0].RequestedModel = keyA.RequestedModel
	queueA.RuntimeKeys[0].UpstreamModel = keyA.UpstreamModel
	queueA.RuntimeKeys[0].EndpointType = string(keyA.EndpointType)
	queueB := runtimeQueueSnapshotForTest("", keyB.ChannelID, keyB.Group, 5, 720)
	queueB.RuntimeKeys[0].RuntimeKey = keyB
	queueB.RuntimeKeys[0].RequestedModel = keyB.RequestedModel
	queueB.RuntimeKeys[0].UpstreamModel = keyB.UpstreamModel
	queueB.RuntimeKeys[0].EndpointType = string(keyB.EndpointType)

	nodeASyncer.SyncAndAggregate(queueA)
	aggregated := nodeBSyncer.SyncAndAggregate(queueB)
	require.Equal(t, 8, aggregated.Summary.TotalQueued)
	require.Equal(t, 2, aggregated.Summary.QueueNodes)
	eventStore.Flush()

	redisQueues := redisBackedStore.ListQueueSnapshots()
	require.Len(t, redisQueues, 2)
	redisAggregated := scheduler.AggregateRuntimeQueueSnapshots(redisQueues)
	require.Equal(t, 8, redisAggregated.Summary.TotalQueued)
	require.Equal(t, 2, redisAggregated.Summary.QueueNodes)
	require.Len(t, redisAggregated.Nodes, 2)
}

func TestHybridRuntimeSyncStoreRedisDisabledOptionFallsBackToMemory(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	store := scheduler.NewHybridRuntimeSyncStore(scheduler.RuntimeSyncStoreOptions{
		TTL: time.Minute,
		RedisEnabled: func() bool {
			return false
		},
	})
	store.PutQueueSnapshot("node-memory", runtimeQueueSnapshotForTest("", 704, "fallback", 4, 310))

	snapshots := store.ListQueueSnapshots()
	require.Len(t, snapshots, 1)
	require.Equal(t, "node-memory", snapshots[0].NodeID)
	require.Equal(t, 4, snapshots[0].Channels[0].QueueDepth)
}

func TestRuntimeSyncEventStoreCoalescesQueueSnapshotAndKeepsPendingVisible(t *testing.T) {
	base := newRecordingRuntimeSyncStore()
	store := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         base,
		FlushInterval: time.Hour,
		QueueSize:     8,
	})
	t.Cleanup(store.Close)

	store.PutQueueSnapshot("node-a", runtimeQueueSnapshotForTest("", 804, "vip", 1, 410))
	store.PutQueueSnapshot("node-a", runtimeQueueSnapshotForTest("", 804, "vip", 5, 420))

	require.Equal(t, 0, base.queuePutCount("node-a"))
	pending := store.ListQueueSnapshots()
	require.Len(t, pending, 1)
	require.Equal(t, "node-a", pending[0].NodeID)
	require.Equal(t, 5, pending[0].Channels[0].QueueDepth)

	store.Flush()

	require.Equal(t, 1, base.queuePutCount("node-a"))
	stored, ok := base.queueSnapshot("node-a")
	require.True(t, ok)
	require.Equal(t, 5, stored.Channels[0].QueueDepth)
	require.Len(t, store.ListQueueSnapshots(), 1)
}

func TestRuntimeSyncEventStoreCoalescesSnapshotsAndCircuits(t *testing.T) {
	base := newRecordingRuntimeSyncStore()
	store := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         base,
		FlushInterval: time.Hour,
		QueueSize:     8,
	})
	t.Cleanup(store.Close)
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      904,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}

	store.PutSnapshot(core.RuntimeSnapshot{Key: key, SuccessRate: 0.7, SampleCount: 1})
	store.PutSnapshot(core.RuntimeSnapshot{Key: key, SuccessRate: 0.95, SampleCount: 4})
	snapshots := store.ListSnapshots(&core.DispatchRequest{ModelName: "gpt-5.5"})
	require.Len(t, snapshots, 1)
	require.Equal(t, 0.95, snapshots[0].SuccessRate)
	require.Equal(t, 0, base.snapshotPutCount(key))

	store.PutCircuit(core.CircuitSnapshot{Key: key, State: core.CircuitStateOpen, FailureCount: 3})
	circuit, ok := store.GetCircuit(key)
	require.True(t, ok)
	require.Equal(t, core.CircuitStateOpen, circuit.State)
	require.Equal(t, 0, base.circuitPutCount(key))

	store.Flush()

	require.Equal(t, 1, base.snapshotPutCount(key))
	storedSnapshot, ok := base.snapshot(key)
	require.True(t, ok)
	require.Equal(t, 0.95, storedSnapshot.SuccessRate)
	require.Equal(t, 1, base.circuitPutCount(key))
	storedCircuit, ok := base.circuit(key)
	require.True(t, ok)
	require.Equal(t, core.CircuitStateOpen, storedCircuit.State)
}

func TestRuntimeSyncEventStorePublishesFlushEvents(t *testing.T) {
	base := newRecordingRuntimeSyncStore()
	publisher := &recordingRuntimeSyncEventPublisher{}
	store := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         base,
		FlushInterval: time.Hour,
		QueueSize:     8,
		Publisher:     publisher,
	})
	t.Cleanup(store.Close)
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "openai-codex",
		ChannelID:      1301,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}

	store.PutSnapshot(core.RuntimeSnapshot{Key: key, SuccessRate: 0.98, SampleCount: 3})
	store.PutCircuit(core.CircuitSnapshot{Key: key, State: core.CircuitStateOpen, SampleCount: 3})
	store.PutQueueSnapshot("node-a", runtimeQueueSnapshotForTest("", key.ChannelID, key.Group, 2, 810))
	store.Flush()

	events := publisher.events()
	require.Len(t, events, 3)
	kinds := map[string]bool{}
	for _, event := range events {
		kinds[event.Kind] = true
		require.NotZero(t, event.UpdatedAt)
	}
	require.True(t, kinds["snapshot"])
	require.True(t, kinds["circuit"])
	require.True(t, kinds["queue"])
}

func TestRedisRuntimeSyncEventPubSubOptIn(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis runtime sync pubsub smoke")
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	t.Cleanup(func() {
		_ = client.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.FlushDB(cleanupCtx).Err()
	})

	channel := "new-api:modelgateway:runtime_events:test"
	ready := make(chan struct{})
	var readyOnce sync.Once
	received := make(chan scheduler.RuntimeSyncEvent, 1)
	subscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		Client:  client,
		Channel: channel,
		OnReady: func() {
			readyOnce.Do(func() {
				close(ready)
			})
		},
		OnEvent: func(event scheduler.RuntimeSyncEvent) {
			received <- event
		},
	})
	t.Cleanup(func() {
		require.NoError(t, subscriber.Close())
	})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Redis runtime sync subscriber readiness")
	}
	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{
		Client:  client,
		Channel: channel,
	})

	event := scheduler.RuntimeSyncEvent{
		Kind:     "snapshot",
		CacheKey: "snapshot|gpt-5.5|vip|1302",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5.5",
			ChannelID:      1302,
			Group:          "vip",
			EndpointType:   constant.EndpointTypeOpenAIResponse,
		},
	}
	require.NoError(t, publisher.Publish(event))

	select {
	case got := <-received:
		require.Equal(t, "snapshot", got.Kind)
		require.Equal(t, event.CacheKey, got.CacheKey)
		require.Equal(t, 1302, got.RuntimeKey.ChannelID)
		require.NotZero(t, got.UpdatedAt)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Redis runtime sync event")
	}
}

func TestRedisRuntimeSyncEventPubSubNoopWhenDisabledOrNilClient(t *testing.T) {
	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{
		Enabled: func() bool {
			return false
		},
	})
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot"}))

	disabledSubscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		Enabled: func() bool {
			return false
		},
		OnEvent: func(scheduler.RuntimeSyncEvent) {
			t.Fatal("disabled subscriber should not receive events")
		},
	})
	require.NoError(t, disabledSubscriber.Close())
	require.NoError(t, disabledSubscriber.Close())

	nilClientSubscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		OnEvent: func(scheduler.RuntimeSyncEvent) {
			t.Fatal("nil-client subscriber should not receive events")
		},
	})
	require.NoError(t, nilClientSubscriber.Close())
	require.NoError(t, nilClientSubscriber.Close())
}

func TestRedisRuntimeSyncEventPublisherNoopsWhenEnabledTurnsFalseDynamically(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis runtime sync pubsub smoke")
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	t.Cleanup(func() {
		_ = client.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.FlushDB(cleanupCtx).Err()
	})

	channel := "new-api:modelgateway:runtime_events:dynamic_disable"
	ready := make(chan struct{})
	var readyOnce sync.Once
	received := make(chan scheduler.RuntimeSyncEvent, 2)
	subscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		Client:  client,
		Channel: channel,
		OnReady: func() {
			readyOnce.Do(func() {
				close(ready)
			})
		},
		OnEvent: func(event scheduler.RuntimeSyncEvent) {
			received <- event
		},
	})
	t.Cleanup(func() {
		require.NoError(t, subscriber.Close())
	})
	waitRuntimeSyncSubscriberReady(t, ready)

	enabled := atomic.Bool{}
	enabled.Store(true)
	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{
		Client:  client,
		Channel: channel,
		Enabled: func() bool {
			return enabled.Load()
		},
	})
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|dynamic", UpdatedAtNano: 1}))
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first Redis runtime sync event")
	}

	enabled.Store(false)
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|dynamic", UpdatedAtNano: 2}))
	select {
	case event := <-received:
		t.Fatalf("disabled publisher should not publish event: %+v", event)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestRedisRuntimeSyncEventSubscriberContinuesAfterMalformedMessage(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis runtime sync pubsub smoke")
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	t.Cleanup(func() {
		_ = client.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.FlushDB(cleanupCtx).Err()
	})

	channel := "new-api:modelgateway:runtime_events:malformed"
	ready := make(chan struct{})
	var readyOnce sync.Once
	errorsSeen := make(chan error, 1)
	received := make(chan scheduler.RuntimeSyncEvent, 1)
	subscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		Client:  client,
		Channel: channel,
		OnReady: func() {
			readyOnce.Do(func() {
				close(ready)
			})
		},
		OnError: func(err error) {
			errorsSeen <- err
		},
		OnEvent: func(event scheduler.RuntimeSyncEvent) {
			received <- event
		},
	})
	t.Cleanup(func() {
		require.NoError(t, subscriber.Close())
	})
	waitRuntimeSyncSubscriberReady(t, ready)

	require.NoError(t, client.Publish(ctx, channel, "{bad json").Err())
	select {
	case err := <-errorsSeen:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for malformed Redis runtime sync event error")
	}

	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{
		Client:  client,
		Channel: channel,
	})
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|malformed", UpdatedAtNano: 1}))
	select {
	case got := <-received:
		require.Equal(t, "snapshot", got.Kind)
		require.Equal(t, "snapshot|malformed", got.CacheKey)
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber should continue after malformed Redis runtime sync event")
	}
}

func TestRedisRuntimeSyncEventSubscriberFiltersDuplicateAndOutOfOrderEvents(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis runtime sync pubsub smoke")
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	t.Cleanup(func() {
		_ = client.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.FlushDB(cleanupCtx).Err()
	})

	channel := "new-api:modelgateway:runtime_events:dedupe"
	ready := make(chan struct{})
	var readyOnce sync.Once
	received := make(chan scheduler.RuntimeSyncEvent, 4)
	subscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		Client:  client,
		Channel: channel,
		OnReady: func() {
			readyOnce.Do(func() {
				close(ready)
			})
		},
		OnEvent: func(event scheduler.RuntimeSyncEvent) {
			received <- event
		},
	})
	t.Cleanup(func() {
		require.NoError(t, subscriber.Close())
	})
	waitRuntimeSyncSubscriberReady(t, ready)

	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{Client: client, Channel: channel})
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|dedupe", UpdatedAtNano: 200}))
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|dedupe", UpdatedAtNano: 200}))
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|dedupe", UpdatedAtNano: 100}))
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|dedupe", UpdatedAtNano: 300}))

	require.Eventually(t, func() bool {
		return len(received) == 2
	}, 3*time.Second, 20*time.Millisecond)
	first := <-received
	second := <-received
	require.Equal(t, int64(200), first.UpdatedAtNano)
	require.Equal(t, int64(300), second.UpdatedAtNano)
}

func TestRedisRuntimeSyncEventSubscriberCallbackQueueFullDoesNotBlockReceiveLoop(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis runtime sync pubsub smoke")
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 15})
	t.Cleanup(func() {
		_ = client.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = client.FlushDB(cleanupCtx).Err()
	})

	channel := "new-api:modelgateway:runtime_events:callback_full"
	ready := make(chan struct{})
	var readyOnce sync.Once
	releaseCallback := make(chan struct{})
	queueFull := make(chan error, 8)
	processed := make(chan scheduler.RuntimeSyncEvent, 8)
	var firstCallback atomic.Bool
	subscriber := scheduler.NewRedisRuntimeSyncEventSubscriber(scheduler.RedisRuntimeSyncEventSubscriberOptions{
		Client:         client,
		Channel:        channel,
		CallbackBuffer: 1,
		OnReady: func() {
			readyOnce.Do(func() {
				close(ready)
			})
		},
		OnError: func(err error) {
			if errors.Is(err, scheduler.ErrRuntimeSyncEventCallbackQueueFull) {
				queueFull <- err
			}
		},
		OnEvent: func(event scheduler.RuntimeSyncEvent) {
			processed <- event
			if firstCallback.CompareAndSwap(false, true) {
				<-releaseCallback
			}
		},
	})
	t.Cleanup(func() {
		close(releaseCallback)
		require.NoError(t, subscriber.Close())
	})
	waitRuntimeSyncSubscriberReady(t, ready)

	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{Client: client, Channel: channel})
	for i := int64(1); i <= 6; i++ {
		require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|callback_full", UpdatedAtNano: i}))
	}
	select {
	case <-queueFull:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback queue full signal")
	}
	require.NotEmpty(t, processed)
}

func TestRuntimeSyncEventStoreFallsBackWhenQueueFullAndAfterClose(t *testing.T) {
	base := newRecordingRuntimeSyncStore()
	store := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         base,
		FlushInterval: time.Hour,
		QueueSize:     1,
	})

	keyA := core.RuntimeKey{RequestedModel: "m-a", ChannelID: 1001, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	keyB := core.RuntimeKey{RequestedModel: "m-b", ChannelID: 1002, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.PutSnapshot(core.RuntimeSnapshot{Key: keyA, SuccessRate: 0.8})
	store.PutSnapshot(core.RuntimeSnapshot{Key: keyB, SuccessRate: 0.9})

	require.Equal(t, 0, base.snapshotPutCount(keyA))
	require.Equal(t, 1, base.snapshotPutCount(keyB))

	store.Close()
	require.Equal(t, 1, base.snapshotPutCount(keyA))
	store.PutSnapshot(core.RuntimeSnapshot{Key: keyA, SuccessRate: 0.99})
	require.Equal(t, 2, base.snapshotPutCount(keyA))
	stored, ok := base.snapshot(keyA)
	require.True(t, ok)
	require.Equal(t, 0.99, stored.SuccessRate)
}

func TestRuntimeSyncEventStoreCoalescesHighFrequencyUpdates(t *testing.T) {
	base := newRecordingRuntimeSyncStore()
	store := scheduler.NewRuntimeSyncEventStore(scheduler.RuntimeSyncEventStoreOptions{
		Store:         base,
		FlushInterval: time.Hour,
		QueueSize:     16,
	})
	t.Cleanup(store.Close)
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo",
		ChannelID:      1101,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}

	for i := 1; i <= 50; i++ {
		store.PutSnapshot(core.RuntimeSnapshot{Key: key, SuccessRate: 0.5 + float64(i)/100, SampleCount: i})
		store.PutCircuit(core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed, SampleCount: i})
		store.PutQueueSnapshot("node-a", runtimeQueueSnapshotForTest("", key.ChannelID, key.Group, i, int64(600+i)))
	}

	require.Equal(t, 0, base.snapshotPutCount(key))
	require.Equal(t, 0, base.circuitPutCount(key))
	require.Equal(t, 0, base.queuePutCount("node-a"))
	pendingSnapshots := store.ListSnapshots(&core.DispatchRequest{ModelName: key.RequestedModel})
	require.Len(t, pendingSnapshots, 1)
	require.Equal(t, 50, pendingSnapshots[0].SampleCount)
	pendingQueues := store.ListQueueSnapshots()
	require.Len(t, pendingQueues, 1)
	require.Equal(t, 50, pendingQueues[0].Channels[0].QueueDepth)

	store.Flush()

	require.Equal(t, 1, base.snapshotPutCount(key))
	require.Equal(t, 1, base.circuitPutCount(key))
	require.Equal(t, 1, base.queuePutCount("node-a"))
	storedSnapshot, ok := base.snapshot(key)
	require.True(t, ok)
	require.Equal(t, 50, storedSnapshot.SampleCount)
	storedCircuit, ok := base.circuit(key)
	require.True(t, ok)
	require.Equal(t, 50, storedCircuit.SampleCount)
	storedQueue, ok := base.queueSnapshot("node-a")
	require.True(t, ok)
	require.Equal(t, 50, storedQueue.Channels[0].QueueDepth)
}

type recordingRuntimeSyncStore struct {
	mu             sync.Mutex
	snapshots      map[string]core.RuntimeSnapshot
	snapshotPuts   map[string]int
	circuits       map[string]core.CircuitSnapshot
	circuitPuts    map[string]int
	queueSnapshots map[string]core.RuntimeQueueSnapshot
	queuePuts      map[string]int
}

type recordingRuntimeSyncEventPublisher struct {
	mu         sync.Mutex
	eventsList []scheduler.RuntimeSyncEvent
}

func waitRuntimeSyncSubscriberReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Redis runtime sync subscriber readiness")
	}
}

func (p *recordingRuntimeSyncEventPublisher) Publish(event scheduler.RuntimeSyncEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventsList = append(p.eventsList, event)
	return nil
}

func (p *recordingRuntimeSyncEventPublisher) events() []scheduler.RuntimeSyncEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]scheduler.RuntimeSyncEvent(nil), p.eventsList...)
}

func newRecordingRuntimeSyncStore() *recordingRuntimeSyncStore {
	return &recordingRuntimeSyncStore{
		snapshots:      map[string]core.RuntimeSnapshot{},
		snapshotPuts:   map[string]int{},
		circuits:       map[string]core.CircuitSnapshot{},
		circuitPuts:    map[string]int{},
		queueSnapshots: map[string]core.RuntimeQueueSnapshot{},
		queuePuts:      map[string]int{},
	}
}

func (s *recordingRuntimeSyncStore) PutSnapshot(snapshot core.RuntimeSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeSyncTestKey(snapshot.Key)
	s.snapshots[key] = snapshot
	s.snapshotPuts[key]++
}

func (s *recordingRuntimeSyncStore) ListSnapshots(*core.DispatchRequest) []core.RuntimeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshots := make([]core.RuntimeSnapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *recordingRuntimeSyncStore) PutCircuit(snapshot core.CircuitSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runtimeSyncTestKey(snapshot.Key)
	s.circuits[key] = snapshot
	s.circuitPuts[key]++
}

func (s *recordingRuntimeSyncStore) GetCircuit(key core.RuntimeKey) (core.CircuitSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.circuits[runtimeSyncTestKey(key)]
	return snapshot, ok
}

func (s *recordingRuntimeSyncStore) ListCircuits() []core.CircuitSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshots := make([]core.CircuitSnapshot, 0, len(s.circuits))
	for _, snapshot := range s.circuits {
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *recordingRuntimeSyncStore) PutQueueSnapshot(nodeID string, snapshot core.RuntimeQueueSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot.NodeID = nodeID
	s.queueSnapshots[nodeID] = snapshot
	s.queuePuts[nodeID]++
}

func (s *recordingRuntimeSyncStore) ListQueueSnapshots() []core.RuntimeQueueSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshots := make([]core.RuntimeQueueSnapshot, 0, len(s.queueSnapshots))
	for _, snapshot := range s.queueSnapshots {
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *recordingRuntimeSyncStore) queuePutCount(nodeID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queuePuts[nodeID]
}

func (s *recordingRuntimeSyncStore) queueSnapshot(nodeID string) (core.RuntimeQueueSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.queueSnapshots[nodeID]
	return snapshot, ok
}

func (s *recordingRuntimeSyncStore) snapshotPutCount(key core.RuntimeKey) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotPuts[runtimeSyncTestKey(key)]
}

func (s *recordingRuntimeSyncStore) snapshot(key core.RuntimeKey) (core.RuntimeSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.snapshots[runtimeSyncTestKey(key)]
	return snapshot, ok
}

func (s *recordingRuntimeSyncStore) circuitPutCount(key core.RuntimeKey) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.circuitPuts[runtimeSyncTestKey(key)]
}

func (s *recordingRuntimeSyncStore) circuit(key core.RuntimeKey) (core.CircuitSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.circuits[runtimeSyncTestKey(key)]
	return snapshot, ok
}

func runtimeSyncTestKey(key core.RuntimeKey) string {
	return key.RequestedModel + "|" + key.UpstreamModel + "|" + key.Group + "|" + string(key.EndpointType) + "|" + key.CapabilityFingerprint + "|" + strconv.Itoa(key.ChannelID)
}

func runtimeQueueSnapshotForTest(nodeID string, channelID int, group string, depth int, updatedAt int64) core.RuntimeQueueSnapshot {
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo",
		ChannelID:      channelID,
		Group:          group,
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	highPriorityDepth := depth / 2
	normalDepth := depth - highPriorityDepth
	return core.RuntimeQueueSnapshot{
		NodeID:    nodeID,
		UpdatedAt: updatedAt,
		Summary:   core.RuntimeQueueSummary{UpdatedAt: updatedAt, TotalQueued: depth},
		Channels: []core.RuntimeQueueChannelSnapshot{
			{
				ChannelID:            channelID,
				QueueDepth:           depth,
				QueuedRequests:       depth,
				WaitingRequests:      depth,
				QueueCapacity:        16,
				HighPriorityDepth:    highPriorityDepth,
				NormalDepth:          normalDepth,
				HighPriorityCapacity: 8,
				NormalCapacity:       8,
				Groups: []core.RuntimeQueueGroupSnapshot{
					{
						ChannelID:         channelID,
						Group:             group,
						QueueDepth:        depth,
						QueuedRequests:    depth,
						WaitingRequests:   depth,
						HighPriorityDepth: highPriorityDepth,
						NormalDepth:       normalDepth,
					},
				},
			},
		},
		RuntimeKeys: []core.RuntimeQueueKeySnapshot{
			{
				RuntimeKey:        key,
				RequestedModel:    key.RequestedModel,
				UpstreamModel:     key.UpstreamModel,
				ChannelID:         key.ChannelID,
				Group:             key.Group,
				EndpointType:      string(key.EndpointType),
				QueueDepth:        depth,
				QueuedRequests:    depth,
				WaitingRequests:   depth,
				HighPriorityDepth: highPriorityDepth,
				NormalDepth:       normalDepth,
			},
		},
	}
}

var _ scheduler.RuntimeSyncStore = (*recordingRuntimeSyncStore)(nil)
