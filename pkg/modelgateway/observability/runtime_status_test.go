package observability_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

type fakeRuntimeStateProvider struct {
	active    map[int]int
	cooldown  map[int]*service.ChannelConcurrencyControlStatus
	avoidance map[int]*service.ChannelFailureAvoidanceStatus
}

func (p fakeRuntimeStateProvider) ActiveConcurrency(channelID int) int {
	return p.active[channelID]
}

func (p fakeRuntimeStateProvider) ConcurrencyCooldownStatus(channelID int) *service.ChannelConcurrencyControlStatus {
	return p.cooldown[channelID]
}

func (p fakeRuntimeStateProvider) FailureAvoidanceStatus(channelID int) *service.ChannelFailureAvoidanceStatus {
	return p.avoidance[channelID]
}

func TestRuntimeStatusServiceMergesSnapshotCircuitQueueAndLiveState(t *testing.T) {
	now := time.Unix(1710000000, 0)
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	keyHealthy := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "gpt-5.5",
		ChannelID:             101,
		Group:                 "vip",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "openai_codex",
	}
	keyOpen := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo-v1",
		ChannelID:      202,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	keyOther := core.RuntimeKey{
		RequestedModel: "deepseek-v4-pro",
		ChannelID:      303,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{
		Key:                keyHealthy,
		SuccessRate:        0.96,
		TTFTMs:             120,
		DurationMs:         900,
		TokensPerSecond:    48,
		ActiveConcurrency:  2,
		MaxConcurrency:     8,
		QueueCapacity:      16,
		QueueTimeoutMs:     1500,
		CostRatio:          1.2,
		GroupPriorityRatio: 1,
		CircuitState:       core.CircuitStateClosed,
		SampleCount:        24,
	})
	store.Put(core.RuntimeSnapshot{
		Key:                keyOpen,
		SuccessRate:        0.2,
		DurationMs:         1800,
		ActiveConcurrency:  4,
		MaxConcurrency:     4,
		QueueDepth:         1,
		QueueCapacity:      8,
		CostRatio:          0.8,
		GroupPriorityRatio: 1.1,
		CircuitState:       core.CircuitStateClosed,
		SampleCount:        6,
	})
	store.Put(core.RuntimeSnapshot{Key: keyOther, SuccessRate: 0.99, SampleCount: 3})

	breaker := scheduler.NewCircuitBreakerForTest(scheduler.CircuitBreakerOptions{
		FailureThreshold:   0.5,
		MinSamples:         2,
		OpenDuration:       time.Minute,
		HalfOpenProbeCount: 1,
	}, func() time.Time { return now })
	breaker.Report(core.AttemptResult{Key: keyOpen, ChannelID: keyOpen.ChannelID, StatusCode: http.StatusInternalServerError})
	breaker.Report(core.AttemptResult{Key: keyOpen, ChannelID: keyOpen.ChannelID, StatusCode: http.StatusBadGateway})

	service := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{
		SnapshotStore: store,
		Circuit:       breaker,
		QueueSnapshot: func() map[int]int {
			return map[int]int{101: 2, 202: 3, 404: 1}
		},
		StateProvider: fakeRuntimeStateProvider{
			active: map[int]int{101: 3},
			cooldown: map[int]*service.ChannelConcurrencyControlStatus{
				101: {
					Active:       true,
					Reason:       "too many requests",
					RemainingSec: 30,
					FailureCount: 2,
				},
			},
			avoidance: map[int]*service.ChannelFailureAvoidanceStatus{
				202: {
					Active:       true,
					Reason:       "upstream_5xx",
					RemainingSec: 12,
					FailureCount: 1,
				},
			},
		},
		Now: func() time.Time { return now },
	})

	response := service.Build(observability.RuntimeStatusQuery{Model: "gpt-5.5", Group: "vip", Limit: 10})

	require.Equal(t, int64(1710000000), response.Summary.UpdatedAt)
	require.Len(t, response.Items, 2)
	require.Equal(t, 2, response.Summary.RuntimeKeys)
	require.Equal(t, 2, response.Summary.Channels)
	require.Equal(t, 7, response.Summary.ActiveConcurrency)
	require.Equal(t, 5, response.Summary.QueuedRequests)
	require.Equal(t, 2, response.Summary.QueueChannels)
	require.Equal(t, 3, response.Summary.MaxQueueDepth)
	require.Equal(t, 1, response.Summary.CircuitOpen)
	require.Equal(t, 1, response.Summary.CooldownChannels)
	require.Equal(t, 1, response.Summary.FailureAvoidanceChannels)
	require.Equal(t, 1, response.Summary.SaturatedChannels)

	openItem := response.Items[0]
	require.Equal(t, 202, openItem.ChannelID)
	require.True(t, openItem.CircuitOpen)
	require.Equal(t, "open", openItem.CircuitState)
	require.Equal(t, int64(1710000060), openItem.CircuitOpenUntil)
	require.Equal(t, 3, openItem.QueueDepth)
	require.True(t, openItem.FailureAvoidance)
	require.Equal(t, "upstream_5xx", openItem.FailureAvoidanceReason)
	require.Equal(t, "circuit_open", openItem.HealthStatus)

	healthyItem := response.Items[1]
	require.Equal(t, 101, healthyItem.ChannelID)
	require.Equal(t, 3, healthyItem.ActiveConcurrency)
	require.Equal(t, 2, healthyItem.QueueDepth)
	require.True(t, healthyItem.Cooldown)
	require.Equal(t, "cooldown", healthyItem.HealthStatus)
}

func TestRuntimeStatusServiceFiltersAndHandlesMissingDeps(t *testing.T) {
	service := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{
		QueueSnapshot: func() map[int]int {
			return map[int]int{55: 2, 66: 1}
		},
		Now: func() time.Time { return time.Unix(100, 0) },
	})

	response := service.Build(observability.RuntimeStatusQuery{ChannelID: 55})

	require.Equal(t, int64(100), response.Summary.UpdatedAt)
	require.Len(t, response.Items, 1)
	require.Equal(t, 55, response.Items[0].ChannelID)
	require.Equal(t, 2, response.Items[0].QueueDepth)
	require.Equal(t, "healthy", response.Items[0].HealthStatus)

	empty := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{}).Build(observability.RuntimeStatusQuery{})
	require.Empty(t, empty.Items)
	require.Zero(t, empty.Summary.RuntimeKeys)
}
