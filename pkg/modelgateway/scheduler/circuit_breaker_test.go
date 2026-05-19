package scheduler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerOpensAndHalfOpenProbeBudget(t *testing.T) {
	now := time.Unix(100, 0)
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         2,
		OpenDuration:       time.Second,
		HalfOpenProbeCount: 1,
	}, func() time.Time { return now })
	key := core.RuntimeKey{RequestedModel: "gpt-5-codex", ChannelID: 42, Group: "default"}

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 42, StatusCode: http.StatusInternalServerError})
	require.Equal(t, core.CircuitStateClosed, breaker.Snapshot(key).State)

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 42, StatusCode: http.StatusBadGateway})
	require.Equal(t, core.CircuitStateOpen, breaker.Snapshot(key).State)
	require.False(t, breaker.AllowProbe(key))

	now = now.Add(time.Second + time.Millisecond)
	require.Equal(t, core.CircuitStateHalfOpen, breaker.Snapshot(key).State)
	require.Equal(t, 0, breaker.Snapshot(key).HalfOpenProbeUsed)
	require.True(t, breaker.AllowProbe(key))
	require.False(t, breaker.AllowProbe(key))

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 42, Success: true})
	require.Equal(t, core.CircuitStateClosed, breaker.Snapshot(key).State)
}

func TestCircuitBreakerReopensOnHalfOpenFailure(t *testing.T) {
	now := time.Unix(200, 0)
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         1,
		OpenDuration:       time.Second,
		HalfOpenProbeCount: 1,
	}, func() time.Time { return now })
	key := core.RuntimeKey{RequestedModel: "deepseek-v4-pro", ChannelID: 7, Group: "codex"}

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 7, StatusCode: http.StatusServiceUnavailable})
	now = now.Add(time.Second + time.Millisecond)
	require.True(t, breaker.AllowProbe(key))

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 7, StatusCode: http.StatusTooManyRequests})
	require.Equal(t, core.CircuitStateOpen, breaker.Snapshot(key).State)
}
