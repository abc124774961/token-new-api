package scheduler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/stretchr/testify/require"
)

func TestRuntimeHealthMonitorUpdatesSnapshotAndCircuit(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	breaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         2,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	})
	monitor := scheduler.NewRuntimeHealthMonitor(store, breaker)
	key := core.RuntimeKey{RequestedModel: "mimo", ChannelID: 77, Group: "codex"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:        key,
		ChannelID:  77,
		StatusCode: http.StatusInternalServerError,
		Duration:   1200 * time.Millisecond,
	})
	monitor.Report(context.Background(), core.AttemptResult{
		Key:        key,
		ChannelID:  77,
		StatusCode: http.StatusBadGateway,
		Duration:   800 * time.Millisecond,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 2, snapshot.SampleCount)
	require.Equal(t, 0.0, snapshot.SuccessRate)
	require.Greater(t, snapshot.DurationMs, 0.0)
	require.Equal(t, core.CircuitStateOpen, snapshot.CircuitState)
	require.True(t, snapshot.CircuitOpen)
}

func TestRuntimeHealthMonitorSuccessImprovesSnapshot(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "standard-openai", ChannelID: 78, Group: "default"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:       key,
		ChannelID: 78,
		Success:   true,
		Duration:  300 * time.Millisecond,
		TTFT:      120 * time.Millisecond,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 1, snapshot.SampleCount)
	require.Equal(t, 1.0, snapshot.SuccessRate)
	require.Equal(t, 300.0, snapshot.DurationMs)
	require.Equal(t, 120.0, snapshot.TTFTMs)
}
