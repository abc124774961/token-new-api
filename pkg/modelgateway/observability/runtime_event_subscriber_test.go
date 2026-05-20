package observability_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestRuntimeEventSubscriberRecorderRecordsRuntimeSyncEvents(t *testing.T) {
	now := time.Unix(1710000400, 0)
	companion := observability.NewRuntimeEventSubscriberRecorder(observability.RuntimeEventSubscriberOptions{
		Now: func() time.Time {
			return now
		},
		RecentLimit: 2,
	})

	companion.Observe(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|a"})
	companion.Observe(scheduler.RuntimeSyncEvent{Kind: "circuit", CacheKey: "circuit|a", UpdatedAt: 1710000399})
	companion.Observe(scheduler.RuntimeSyncEvent{Kind: "queue", NodeID: "node-a"})

	snapshot := companion.Snapshot()
	require.Equal(t, int64(1710000400), snapshot.UpdatedAt)
	require.Equal(t, 3, snapshot.Total)
	require.Equal(t, 1, snapshot.ByKind["snapshot"])
	require.Equal(t, 1, snapshot.ByKind["circuit"])
	require.Equal(t, 1, snapshot.ByKind["queue"])
	require.Equal(t, "queue", snapshot.LastEvent.Kind)
	require.Equal(t, int64(1710000400), snapshot.LastEvent.UpdatedAt)
	require.Len(t, snapshot.Recent, 2)
	require.Equal(t, "circuit", snapshot.Recent[0].Kind)
	require.Equal(t, "queue", snapshot.Recent[1].Kind)
}

func TestRuntimeEventSubscriberRecorderSnapshotIsDefensiveCopy(t *testing.T) {
	companion := observability.NewRuntimeEventSubscriberRecorder(observability.RuntimeEventSubscriberOptions{
		Now: func() time.Time {
			return time.Unix(1710000500, 0)
		},
	})
	companion.Observe(scheduler.RuntimeSyncEvent{Kind: "snapshot", CacheKey: "snapshot|a"})

	first := companion.Snapshot()
	first.ByKind["snapshot"] = 99
	first.Recent[0].Kind = "mutated"

	second := companion.Snapshot()
	require.Equal(t, 1, second.ByKind["snapshot"])
	require.Equal(t, "snapshot", second.Recent[0].Kind)
}

func TestRuntimeEventSubscriberCloseIsIdempotentWhenDisabled(t *testing.T) {
	companion := observability.NewRuntimeEventSubscriber(observability.RuntimeEventSubscriberOptions{
		RedisOptions: scheduler.RedisRuntimeSyncEventSubscriberOptions{
			Enabled: func() bool {
				return false
			},
			OnEvent: func(scheduler.RuntimeSyncEvent) {
				t.Fatal("disabled runtime event subscriber should not receive events")
			},
		},
	})

	require.NoError(t, companion.Close())
	require.NoError(t, companion.Close())
	require.Zero(t, companion.Snapshot().Total)
}

func TestRuntimeEventSubscriberRedisOptInSmoke(t *testing.T) {
	redisAddr := os.Getenv("MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("set MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR to run Redis runtime event subscriber smoke")
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

	channel := "new-api:modelgateway:runtime_events:observability"
	ready := make(chan struct{})
	var readyOnce sync.Once
	companion := observability.NewRedisRuntimeEventSubscriber(client, observability.RuntimeEventSubscriberOptions{
		RedisOptions: scheduler.RedisRuntimeSyncEventSubscriberOptions{
			Channel: channel,
			OnReady: func() {
				readyOnce.Do(func() {
					close(ready)
				})
			},
		},
		Now: func() time.Time {
			return time.Unix(1710000600, 0)
		},
		RecentLimit: 4,
	})
	t.Cleanup(func() {
		require.NoError(t, companion.Close())
	})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Redis runtime event subscriber readiness")
	}

	publisher := scheduler.NewRedisRuntimeSyncEventPublisher(scheduler.RedisRuntimeSyncEventPublisherOptions{
		Client:  client,
		Channel: channel,
	})
	require.NoError(t, publisher.Publish(scheduler.RuntimeSyncEvent{
		Kind:          "snapshot",
		CacheKey:      "snapshot|observability",
		UpdatedAt:     1710000599,
		UpdatedAtNano: 1710000599000000000,
	}))

	require.Eventually(t, func() bool {
		return companion.Snapshot().Total == 1
	}, 3*time.Second, 20*time.Millisecond)
	snapshot := companion.Snapshot()
	require.Equal(t, 1, snapshot.ByKind["snapshot"])
	require.Equal(t, "snapshot|observability", snapshot.LastEvent.CacheKey)
}
