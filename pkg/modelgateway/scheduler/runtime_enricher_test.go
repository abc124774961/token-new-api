package scheduler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSnapshotEnricherAppliesConcurrencyCooldownAndAvoidance(t *testing.T) {
	setting, err := common.Marshal(map[string]any{"max_concurrency": 2})
	require.NoError(t, err)
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{
		ActiveConcurrencyByChannel: map[int]int{7: 2},
		CooldownByChannel:          map[int]bool{7: true},
		FailureAvoidanceByChannel:  map[int]bool{7: true},
	}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7, Setting: common.GetPointer(string(setting))},
		Group:   "default",
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{
		QueueEnabled: true,
	})

	require.Equal(t, 2, snapshot.ActiveConcurrency)
	require.Equal(t, 2, snapshot.MaxConcurrency)
	require.Equal(t, 1, snapshot.QueueDepth)
	require.Equal(t, 4, snapshot.QueueCapacity)
	require.Equal(t, 1500, snapshot.QueueTimeoutMs)
	require.True(t, snapshot.Cooldown)
	require.True(t, snapshot.FailureAvoidance)
}

func TestRuntimeSnapshotEnricherAppliesCircuitBreakerState(t *testing.T) {
	setting, err := common.Marshal(map[string]any{"max_concurrency": 2})
	require.NoError(t, err)
	breaker := scheduler.NewCircuitBreaker(scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         1,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	})
	key := core.RuntimeKey{RequestedModel: "gpt-5-codex", ChannelID: 9, Group: "default"}
	breaker.Report(core.AttemptResult{Key: key, ChannelID: 9, StatusCode: http.StatusBadGateway})
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2).WithCircuitBreaker(breaker)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 9, Setting: common.GetPointer(string(setting))},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
		},
	}, core.RuntimeSnapshot{Key: key}, core.GroupSmartPolicy{
		CircuitBreakerEnabled: true,
	})

	require.Equal(t, core.CircuitStateOpen, snapshot.CircuitState)
	require.True(t, snapshot.CircuitOpen)
}
