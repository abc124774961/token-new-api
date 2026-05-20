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

func TestCircuitBreakerUsesErrorPolicyForRateLimit(t *testing.T) {
	now := time.Unix(300, 0)
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         10,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 3,
		ErrorPolicies: map[string]scheduler.CircuitErrorPolicy{
			scheduler.CircuitErrorRateLimit: {
				FailureThreshold:   1,
				MinSamples:         2,
				OpenDuration:       2 * time.Second,
				HalfOpenProbeCount: 2,
			},
		},
	}, func() time.Time { return now })
	key := core.RuntimeKey{RequestedModel: "mimo", ChannelID: 10, Group: "codex"}

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 10, StatusCode: http.StatusTooManyRequests})
	require.Equal(t, core.CircuitStateClosed, breaker.Snapshot(key).State)

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 10, StatusCode: http.StatusTooManyRequests})
	snapshot := breaker.Snapshot(key)
	require.Equal(t, core.CircuitStateOpen, snapshot.State)
	require.Equal(t, scheduler.CircuitErrorRateLimit, snapshot.OpenReason)
	require.Equal(t, 2, snapshot.ErrorCounts[scheduler.CircuitErrorRateLimit])
	require.Equal(t, now.Add(2*time.Second).Unix(), snapshot.OpenUntil.Unix())
	require.Equal(t, 2, snapshot.HalfOpenProbeMax)
}

func TestCircuitBreakerErrorPolicyCanCountAuthAndQuota(t *testing.T) {
	now := time.Unix(400, 0)
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         10,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 3,
		ErrorPolicies: map[string]scheduler.CircuitErrorPolicy{
			scheduler.CircuitErrorAuth: {
				FailureThreshold:   1,
				MinSamples:         1,
				OpenDuration:       5 * time.Second,
				HalfOpenProbeCount: 1,
			},
			scheduler.CircuitErrorQuota: {
				FailureThreshold:   1,
				MinSamples:         1,
				OpenDuration:       time.Second,
				HalfOpenProbeCount: 1,
			},
		},
	}, func() time.Time { return now })
	authKey := core.RuntimeKey{RequestedModel: "openai-codex", ChannelID: 11, Group: "codex"}
	quotaKey := core.RuntimeKey{RequestedModel: "deepseek-v4-pro", ChannelID: 12, Group: "codex"}

	breaker.Report(core.AttemptResult{Key: authKey, ChannelID: 11, StatusCode: http.StatusUnauthorized})
	authSnapshot := breaker.Snapshot(authKey)
	require.Equal(t, core.CircuitStateOpen, authSnapshot.State)
	require.Equal(t, scheduler.CircuitErrorAuth, authSnapshot.OpenReason)

	breaker.Report(core.AttemptResult{Key: quotaKey, ChannelID: 12, StatusCode: http.StatusForbidden, ErrorCode: "insufficient_user_quota"})
	quotaSnapshot := breaker.Snapshot(quotaKey)
	require.Equal(t, core.CircuitStateOpen, quotaSnapshot.State)
	require.Equal(t, scheduler.CircuitErrorQuota, quotaSnapshot.OpenReason)
}

func TestCircuitBreakerDoesNotCountAuthWithoutErrorPolicy(t *testing.T) {
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Second,
		HalfOpenProbeCount: 1,
	}, func() time.Time { return time.Unix(500, 0) })
	key := core.RuntimeKey{RequestedModel: "openai-codex", ChannelID: 13, Group: "codex"}

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 13, StatusCode: http.StatusUnauthorized})
	require.Equal(t, core.CircuitStateClosed, breaker.Snapshot(key).State)
}
