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

	breaker.Report(core.AttemptResult{Key: key, ChannelID: 7, StatusCode: http.StatusBadGateway})
	require.Equal(t, core.CircuitStateOpen, breaker.Snapshot(key).State)
}

func TestCircuitBreakerDoesNotOpenFor429EvenWithRateLimitPolicy(t *testing.T) {
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
	require.Equal(t, core.CircuitStateClosed, snapshot.State)
	require.Zero(t, snapshot.SampleCount)
	require.Empty(t, snapshot.OpenReason)
	require.Empty(t, snapshot.ErrorCounts)
	require.Empty(t, scheduler.ClassifyCircuitError(core.AttemptResult{
		StatusCode: http.StatusTooManyRequests,
		ErrorCode:  "rate_limit",
	}))
}

func TestCircuitBreakerDoesNotOpenForOverloadSkip429(t *testing.T) {
	now := time.Unix(350, 0)
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
		ErrorPolicies: map[string]scheduler.CircuitErrorPolicy{
			scheduler.CircuitErrorRateLimit: {
				FailureThreshold:   1,
				MinSamples:         1,
				OpenDuration:       time.Second,
				HalfOpenProbeCount: 1,
			},
		},
	}, func() time.Time { return now })
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 15, Group: "vip"}

	for idx := 0; idx < 3; idx++ {
		breaker.Report(core.AttemptResult{
			Key:                key,
			ChannelID:          15,
			StatusCode:         http.StatusTooManyRequests,
			ErrorMessage:       "Too many pending requests, please retry later",
			ConcurrencyLimited: true,
		})
	}

	snapshot := breaker.Snapshot(key)
	require.Equal(t, core.CircuitStateClosed, snapshot.State)
	require.Zero(t, snapshot.SampleCount)
	require.Empty(t, snapshot.ErrorCounts)
	require.Empty(t, scheduler.ClassifyCircuitError(core.AttemptResult{
		StatusCode:         http.StatusTooManyRequests,
		ErrorMessage:       "Too many pending requests, please retry later",
		ErrorCategory:      core.ErrorCategoryOverloadSkip,
		ConcurrencyLimited: true,
	}))
}

func TestCircuitBreakerIgnoresClientAbortedStream(t *testing.T) {
	now := time.Unix(360, 0)
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	}, func() time.Time { return now })
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 16, Group: "vip"}

	breaker.Report(core.AttemptResult{
		Key:               key,
		ChannelID:         16,
		StatusCode:        499,
		StreamInterrupted: true,
		ClientAborted:     true,
	})

	snapshot := breaker.Snapshot(key)
	require.Equal(t, core.CircuitStateClosed, snapshot.State)
	require.Zero(t, snapshot.SampleCount)
	require.Empty(t, snapshot.ErrorCounts)
	require.Empty(t, scheduler.ClassifyCircuitError(core.AttemptResult{
		StatusCode:        499,
		StreamInterrupted: true,
		ClientAborted:     true,
	}))
}

func TestCircuitBreakerErrorPolicyCanCountAuth(t *testing.T) {
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
		},
	}, func() time.Time { return now })
	authKey := core.RuntimeKey{RequestedModel: "openai-codex", ChannelID: 11, Group: "codex"}

	breaker.Report(core.AttemptResult{Key: authKey, ChannelID: 11, StatusCode: http.StatusUnauthorized})
	authSnapshot := breaker.Snapshot(authKey)
	require.Equal(t, core.CircuitStateOpen, authSnapshot.State)
	require.Equal(t, scheduler.CircuitErrorAuth, authSnapshot.OpenReason)
}

func TestCircuitBreakerIgnoresBalanceInsufficient(t *testing.T) {
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Second,
		HalfOpenProbeCount: 1,
		ErrorPolicies: map[string]scheduler.CircuitErrorPolicy{
			scheduler.CircuitErrorQuota: {
				FailureThreshold:   1,
				MinSamples:         1,
				OpenDuration:       time.Second,
				HalfOpenProbeCount: 1,
			},
		},
	}, func() time.Time { return time.Unix(410, 0) })
	quotaKey := core.RuntimeKey{RequestedModel: "deepseek-v4-pro", ChannelID: 12, Group: "codex"}

	breaker.Report(core.AttemptResult{Key: quotaKey, ChannelID: 12, StatusCode: http.StatusForbidden, ErrorCode: "insufficient_user_quota", ErrorCategory: "balance_or_quota", BalanceInsufficient: true})
	quotaSnapshot := breaker.Snapshot(quotaKey)
	require.Equal(t, core.CircuitStateClosed, quotaSnapshot.State)
	require.Zero(t, quotaSnapshot.SampleCount)
	require.Empty(t, quotaSnapshot.ErrorCounts)
	require.Empty(t, scheduler.ClassifyCircuitError(core.AttemptResult{
		StatusCode:          http.StatusForbidden,
		ErrorCode:           "insufficient_user_quota",
		ErrorCategory:       "balance_or_quota",
		BalanceInsufficient: true,
	}))
	require.Empty(t, scheduler.ClassifyCircuitError(core.AttemptResult{
		StatusCode:    http.StatusForbidden,
		ErrorCode:     "insufficient_user_quota",
		ErrorCategory: core.ErrorCategoryUserQuotaExhausted,
	}))
}

func TestCircuitBreakerIgnoresSchedulerExhausted(t *testing.T) {
	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Second,
		HalfOpenProbeCount: 1,
	}, func() time.Time { return time.Unix(420, 0) })
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 14, Group: "codex-plus"}

	breaker.Report(core.AttemptResult{
		Key:           key,
		ChannelID:     14,
		StatusCode:    http.StatusTooManyRequests,
		ErrorCategory: core.ErrorCategorySchedulerExhausted,
		RetryAction:   "stop",
	})

	snapshot := breaker.Snapshot(key)
	require.Equal(t, core.CircuitStateClosed, snapshot.State)
	require.Zero(t, snapshot.SampleCount)
	require.Empty(t, snapshot.ErrorCounts)
	require.Empty(t, scheduler.ClassifyCircuitError(core.AttemptResult{
		StatusCode:    http.StatusTooManyRequests,
		ErrorCategory: core.ErrorCategorySchedulerExhausted,
		RetryAction:   "stop",
	}))
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
