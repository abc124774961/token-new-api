package scheduler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
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
	require.Zero(t, snapshot.TTFTMs)
	require.Zero(t, snapshot.TTFTMs)
	require.Equal(t, core.CircuitStateOpen, snapshot.CircuitState)
	require.True(t, snapshot.CircuitOpen)
}

func TestRuntimeHealthMonitorSkipsOverloadSkip429(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	breaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	})
	monitor := scheduler.NewRuntimeHealthMonitor(store, breaker)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 83, Group: "default"}

	for i := 0; i < 5; i++ {
		monitor.Report(context.Background(), core.AttemptResult{
			Key:           key,
			ChannelID:     83,
			StatusCode:    http.StatusTooManyRequests,
			ErrorCode:     "rate_limit",
			ErrorCategory: core.ErrorCategoryOverloadSkip,
			Duration:      80 * time.Millisecond,
			TTFT:          20 * time.Millisecond,
		})
	}

	_, ok := store.Get(key)
	require.False(t, ok)
	circuit := breaker.Snapshot(key)
	require.Equal(t, core.CircuitStateClosed, circuit.State)
	require.Zero(t, circuit.SampleCount)
	require.Empty(t, circuit.ErrorCounts)
}

func TestRuntimeHealthMonitorSkipsBalanceInsufficientForSuccessRate(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	breaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
		FailureThreshold:   1,
		MinSamples:         1,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
		ErrorPolicies: map[string]scheduler.CircuitErrorPolicy{
			scheduler.CircuitErrorQuota: {
				FailureThreshold:   1,
				MinSamples:         1,
				OpenDuration:       time.Minute,
				HalfOpenProbeCount: 1,
			},
		},
	})
	monitor := scheduler.NewRuntimeHealthMonitor(store, breaker)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 84, Group: "default"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:                 key,
		ChannelID:           84,
		StatusCode:          http.StatusForbidden,
		ErrorCode:           "insufficient_user_quota",
		ErrorMessage:        "insufficient account balance",
		ErrorCategory:       "balance_or_quota",
		BalanceInsufficient: true,
		Duration:            80 * time.Millisecond,
		TTFT:                20 * time.Millisecond,
	})

	_, ok := store.Get(key)
	require.False(t, ok)
	circuit := breaker.Snapshot(key)
	require.Equal(t, core.CircuitStateClosed, circuit.State)
	require.Zero(t, circuit.SampleCount)
	require.Empty(t, circuit.ErrorCounts)
	require.True(t, service.IsRuntimeBalanceInsufficientChannelID(84))
	service.ClearChannelBalanceInsufficient(84)
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

func TestRuntimeHealthMonitorClearsLongNoSuccessReasonOnRealSuccess(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-5.4", ChannelID: 78, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        4,
		SuccessRate:        0.5,
		ProbeTriggerReason: "long_no_success",
	})
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)

	monitor.Report(context.Background(), core.AttemptResult{
		Key:        key,
		ChannelID:  78,
		Success:    true,
		ObservedAt: time.Unix(1710000100, 0),
		Duration:   300 * time.Millisecond,
		TTFT:       120 * time.Millisecond,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Empty(t, snapshot.ProbeTriggerReason)
	require.Equal(t, int64(1710000100), snapshot.LastRealSuccessAt)
}

func TestRuntimeHealthMonitorContinuesRestoredSnapshotSamples(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "standard-openai", ChannelID: 82, Group: "default"}
	store.Put(core.RuntimeSnapshot{
		Key:                 key,
		SampleCount:         20,
		SuccessRate:         0.90,
		DurationMs:          900,
		TTFTMs:              300,
		EmptyOutputRate:     0,
		ExperienceIssueRate: 0,
	})
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)

	monitor.Report(context.Background(), core.AttemptResult{
		Key:       key,
		ChannelID: 82,
		Success:   true,
		Duration:  500 * time.Millisecond,
		TTFT:      180 * time.Millisecond,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 21, snapshot.SampleCount)
	require.InEpsilon(t, float64(19)/float64(21), snapshot.SuccessRate, 0.001)
	require.Greater(t, snapshot.TTFTMs, 0.70)
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
	require.InEpsilon(t, 0.2, snapshot.EmptyOutputRate, 0.000001)
	require.Zero(t, snapshot.ExperienceIssueRate)
	require.True(t, snapshot.EmptyOutputRate > 0)
	require.True(t, snapshot.EmptyOutputRate < 1)
	require.Equal(t, 1.0, snapshot.SuccessRate)

	stats := scoreStatsForHealthMonitorTest(t, snapshot)
	require.Equal(t, 1, stats.Rates["empty_output"].Count)
	require.Equal(t, 1, stats.Rates["upstream_error"].Success)
	require.Equal(t, 1, stats.Rates["stream_interrupted"].Success)
}

func TestRuntimeHealthMonitorScoreStatsKeepFlatErrorDimensionsSeparate(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 108, Group: "default"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:               key,
		ChannelID:         108,
		Success:           true,
		StreamInterrupted: true,
		ErrorCategory:     core.ErrorCategoryStreamInterrupted,
		Duration:          2 * time.Second,
		TTFT:              400 * time.Millisecond,
	})
	monitor.Report(context.Background(), core.AttemptResult{
		Key:           key,
		ChannelID:     108,
		Success:       false,
		ErrorCategory: core.ErrorCategoryUpstreamError,
		Duration:      1200 * time.Millisecond,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	stats := scoreStatsForHealthMonitorTest(t, snapshot)
	require.Equal(t, 2, stats.Rates["completion"].Total)
	require.Equal(t, 0, stats.Rates["completion"].Success)
	require.Equal(t, 2, stats.Rates["completion"].Count)
	require.Equal(t, 2, stats.Rates["upstream_error"].Total)
	require.Equal(t, 1, stats.Rates["upstream_error"].Count)
	require.Equal(t, 1, stats.Rates["upstream_error"].Success)
	require.Equal(t, 2, stats.Rates["stream_interrupted"].Total)
	require.Equal(t, 1, stats.Rates["stream_interrupted"].Count)
	require.Equal(t, 1, stats.Rates["stream_interrupted"].Success)
}

func TestRuntimeHealthMonitorTracksNonEmptyExperienceIssueSeparately(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 89, Group: "default"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:             key,
		ChannelID:       89,
		Success:         true,
		Duration:        2 * time.Second,
		TTFT:            700 * time.Millisecond,
		ExperienceIssue: "malformed_content",
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 1, snapshot.SampleCount)
	require.Zero(t, snapshot.EmptyOutputRate)
	require.InEpsilon(t, 0.2, snapshot.ExperienceIssueRate, 0.000001)
	require.True(t, snapshot.ExperienceIssueRate > 0)
	require.True(t, snapshot.ExperienceIssueRate < 1)
}

type scoreStatsForHealthMonitor struct {
	Rates map[string]struct {
		Success int `json:"success,omitempty"`
		Count   int `json:"count,omitempty"`
		Total   int `json:"total,omitempty"`
	} `json:"rates,omitempty"`
}

func scoreStatsForHealthMonitorTest(t *testing.T, snapshot core.RuntimeSnapshot) scoreStatsForHealthMonitor {
	t.Helper()
	var stats scoreStatsForHealthMonitor
	require.NoError(t, common.UnmarshalJsonStr(snapshot.ScoreStatsJSON, &stats))
	require.NotNil(t, stats.Rates)
	return stats
}

func TestRuntimeHealthMonitorExperienceRatesRecoverAfterNormalSamples(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 90, Group: "default"}

	for i := 0; i < 4; i++ {
		monitor.Report(context.Background(), core.AttemptResult{
			Key:             key,
			ChannelID:       90,
			Success:         true,
			Duration:        2 * time.Second,
			TTFT:            700 * time.Millisecond,
			ExperienceIssue: "malformed_content",
		})
	}
	snapshot, ok := store.Get(key)
	require.True(t, ok)
	raisedIssueRate := snapshot.ExperienceIssueRate
	require.Greater(t, raisedIssueRate, 0.5)

	for i := 0; i < 6; i++ {
		monitor.Report(context.Background(), core.AttemptResult{
			Key:       key,
			ChannelID: 90,
			Success:   true,
			Duration:  2 * time.Second,
			TTFT:      700 * time.Millisecond,
		})
	}
	snapshot, ok = store.Get(key)
	require.True(t, ok)
	require.Less(t, snapshot.ExperienceIssueRate, raisedIssueRate)
	require.Greater(t, snapshot.ExperienceIssueRate, 0.10)
}

func TestRuntimeHealthMonitorFastTracksSlowTTFTRegression(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 99, Group: "codex-plus"}

	for i := 0; i < 20; i++ {
		monitor.Report(context.Background(), core.AttemptResult{
			Key:        key,
			ChannelID:  99,
			Success:    true,
			ObservedAt: time.Unix(int64(100+i), 0),
			Duration:   2 * time.Second,
			TTFT:       500 * time.Millisecond,
		})
	}
	monitor.Report(context.Background(), core.AttemptResult{
		Key:        key,
		ChannelID:  99,
		Success:    true,
		ObservedAt: time.Unix(200, 0),
		Duration:   70 * time.Second,
		TTFT:       36 * time.Second,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 21, snapshot.SampleCount)
	require.InEpsilon(t, 500.0, snapshot.TTFTMs, 0.001)
	require.Greater(t, snapshot.TTFTMs, 0.95)
}

func TestRuntimeHealthMonitorSmallLatencyWindowIgnoresSingleOutlier(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 104, Group: "codex-plus"}

	for i, ttft := range []time.Duration{900 * time.Millisecond, 920 * time.Millisecond, 28 * time.Second} {
		monitor.Report(context.Background(), core.AttemptResult{
			Key:        key,
			ChannelID:  104,
			Success:    true,
			ObservedAt: time.Unix(int64(100+i), 0),
			Duration:   ttft + 1200*time.Millisecond,
			TTFT:       ttft,
		})
	}

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 3, snapshot.SampleCount)
	require.Len(t, snapshot.RecentLatencySamples, 3)
	require.InEpsilon(t, 920.0, snapshot.TTFTMs, 0.001)
	require.Greater(t, snapshot.TTFTMs, 0.98)
}

func TestRuntimeHealthMonitorSlowSuccessfulTTFTDoesNotCreateFailureAvoidance(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 6
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(100)
	})

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 100, Group: "auto"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:           key,
		RequestID:     "req-slow",
		ChannelID:     100,
		ChannelName:   "dora",
		SelectedGroup: "auto",
		ModelName:     "gpt-5.5",
		Success:       true,
		Duration:      46 * time.Second,
		TTFT:          45 * time.Second,
	})

	require.Nil(t, service.GetChannelFailureAvoidanceStatus(100))
	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 1, snapshot.SampleCount)
	require.GreaterOrEqual(t, snapshot.TTFTMs, 45000.0)
	require.Equal(t, 1.0, snapshot.SuccessRate)
}

func TestRuntimeHealthMonitorUsesTrimmedLatencyWindow(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 102, Group: "codex-plus"}

	for i := 0; i < 18; i++ {
		monitor.Report(context.Background(), core.AttemptResult{
			Key:        key,
			ChannelID:  102,
			Success:    true,
			ObservedAt: time.Unix(int64(100+i), 0),
			Duration:   2 * time.Second,
			TTFT:       900 * time.Millisecond,
		})
	}
	monitor.Report(context.Background(), core.AttemptResult{
		Key:        key,
		ChannelID:  102,
		Success:    true,
		ObservedAt: time.Unix(200, 0),
		Duration:   80 * time.Second,
		TTFT:       45 * time.Second,
	})
	monitor.Report(context.Background(), core.AttemptResult{
		Key:        key,
		ChannelID:  102,
		Success:    true,
		ObservedAt: time.Unix(201, 0),
		Duration:   80 * time.Millisecond,
		TTFT:       30 * time.Millisecond,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 20, snapshot.SampleCount)
	require.Len(t, snapshot.RecentLatencySamples, 20)
	require.InEpsilon(t, 900.0, snapshot.TTFTMs, 0.001)
	require.Greater(t, snapshot.TTFTMs, 0.98)
}

func TestRuntimeHealthMonitorClearsAvoidanceAfterTwoFastProbeSuccesses(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 6
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(101)
	})

	service.RecordChannelFailureAvoidance(101, "upstream_error:502:bad_response_status_code")
	require.NotNil(t, service.GetChannelFailureAvoidanceStatus(101))

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", ChannelID: 101, Group: "auto"}

	monitor.Report(context.Background(), core.AttemptResult{
		Key:           key,
		ChannelID:     101,
		Success:       true,
		Duration:      2 * time.Second,
		TTFT:          600 * time.Millisecond,
		IsHealthProbe: true,
		ProbeReason:   "failure_avoidance",
	})

	require.NotNil(t, service.GetChannelFailureAvoidanceStatus(101))
	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.True(t, snapshot.ProbeRecoveryPending)
	require.Equal(t, 1, snapshot.ProbeRecoverySuccessCount)
	require.Equal(t, 2, snapshot.ProbeRecoveryRequired)

	monitor.Report(context.Background(), core.AttemptResult{
		Key:           key,
		ChannelID:     101,
		Success:       true,
		Duration:      1800 * time.Millisecond,
		TTFT:          500 * time.Millisecond,
		IsHealthProbe: true,
		ProbeReason:   "failure_avoidance",
	})

	require.Nil(t, service.GetChannelFailureAvoidanceStatus(101))
	snapshot, ok = store.Get(key)
	require.True(t, ok)
	require.False(t, snapshot.ProbeRecoveryPending)
	require.Equal(t, 2, snapshot.ProbeRecoverySuccessCount)
}
