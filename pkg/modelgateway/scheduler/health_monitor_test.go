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

func TestRuntimeHealthMonitorIgnoresClientAbort(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	breaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	})
	monitor := scheduler.NewRuntimeHealthMonitor(store, breaker)
	key := core.RuntimeKey{RequestedModel: "standard-openai", ChannelID: 79, Group: "default"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:               key,
		ChannelID:         79,
		StatusCode:        499,
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          600 * time.Millisecond,
	})

	_, ok := store.Get(key)
	require.False(t, ok)
	require.Equal(t, core.CircuitStateClosed, breaker.Snapshot(key).State)
	require.Zero(t, breaker.Snapshot(key).SampleCount)
}

func TestRuntimeHealthMonitorTracksExperiencePenalty(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.4", ChannelID: 88, Group: "default"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:             key,
		ChannelID:       88,
		Success:         true,
		Duration:        5 * time.Second,
		TTFT:            1500 * time.Millisecond,
		EmptyOutput:     true,
		ExperienceIssue: "empty_output",
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 1, snapshot.SampleCount)
	require.True(t, snapshot.EmptyOutputRate > 0)
	require.True(t, snapshot.ExperienceIssueRate > 0)
	require.True(t, snapshot.ExperienceScore < 1)
	require.True(t, snapshot.SuccessScore < 1)
}

func TestRuntimeHealthMonitorFastTracksSlowTTFTRegression(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 99, Group: "codex-plus"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:       key,
		ChannelID: 99,
		Success:   true,
		Duration:  2 * time.Second,
		TTFT:      500 * time.Millisecond,
	})
	monitor.Report(context.Background(), core.AttemptResult{
		Key:       key,
		ChannelID: 99,
		Success:   true,
		Duration:  70 * time.Second,
		TTFT:      36 * time.Second,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 2, snapshot.SampleCount)
	require.GreaterOrEqual(t, snapshot.TTFTMs, 16000.0)
	require.Less(t, snapshot.SpeedScore, 0.60)
}
