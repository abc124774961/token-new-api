package scheduler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

type fakeCostProfileProvider struct {
	ratio float64
	ok    bool
}

func (p fakeCostProfileProvider) CostRatio(channelID int, upstreamModel string) (float64, bool) {
	return p.ratio, p.ok
}

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
	require.Zero(t, snapshot.QueueDepth)
	require.Equal(t, 4, snapshot.QueueCapacity)
	require.Equal(t, 1500, snapshot.QueueTimeoutMs)
	require.True(t, snapshot.Cooldown)
	require.True(t, snapshot.FailureAvoidance)
}

func TestRuntimeSnapshotEnricherKeepsSnapshotLimitWhenLearnedLimitUnset(t *testing.T) {
	setting, err := common.Marshal(map[string]any{"max_concurrency_ceiling": 46})
	require.NoError(t, err)
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{
		ActiveConcurrencyByChannel: map[int]int{8: 1},
	}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 8, Setting: common.GetPointer(string(setting))},
		Group:   "codex-plus",
	}, core.RuntimeSnapshot{
		MaxConcurrency: 46,
	}, core.GroupSmartPolicy{})

	require.Equal(t, 1, snapshot.ActiveConcurrency)
	require.Equal(t, 46, snapshot.MaxConcurrency)
	require.Equal(t, 46, snapshot.ConfiguredConcurrencyLimit)
	require.Equal(t, 46, snapshot.LearnedConcurrencyLimit)
	require.Equal(t, 46, snapshot.EffectiveConcurrencyLimit)
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

func TestRuntimeSnapshotEnricherUsesCostProfileProvider(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)
	enricher.WithCostProfileProvider(fakeCostProfileProvider{ratio: 0.42, ok: true})

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
		},
	}, core.RuntimeSnapshot{CostRatio: 1}, core.GroupSmartPolicy{})

	require.Equal(t, 0.42, snapshot.CostRatio)
}

func TestRuntimeSnapshotEnricherIgnoresDeprecatedChannelCostPerMillionAndStaleSnapshotCost(t *testing.T) {
	deprecatedCost := 0.01
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7, CostPerMillion: &deprecatedCost},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
		},
	}, core.RuntimeSnapshot{CostRatio: 1}, core.GroupSmartPolicy{})

	require.Zero(t, snapshot.CostRatio)
	require.NotEqual(t, deprecatedCost, snapshot.CostRatio)
}

func TestRuntimeSnapshotEnricherDoesNotRefreshCostProfileOnRequestPath(t *testing.T) {
	previousDB := model.DB
	model.DB = nil
	defer func() {
		model.DB = previousDB
		modelgatewaycost.InvalidateDefaultProfileCache()
	}()
	modelgatewaycost.InvalidateDefaultProfileCache()

	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
		},
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{})

	require.NotNil(t, snapshot)
}

func TestRuntimeSnapshotEnricherAppliesGroupPriorityRatio(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "codex-plus",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
		},
	}, core.RuntimeSnapshot{GroupPriorityRatio: 0.6}, core.GroupSmartPolicy{
		GroupPriorityRatio: map[string]float64{
			"codex-plus": 1.4,
			"codex-pro":  0.7,
		},
	})

	require.Equal(t, 1.4, snapshot.GroupPriorityRatio)
}

func TestRuntimeSnapshotEnricherAppliesFirstBytePending(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{
		FirstBytePendingByChannel: map[int]*service.ChannelFirstBytePendingStatus{
			7: {
				Pending:       3,
				SlowPending:   2,
				OldestMs:      14000,
				SlowThreshold: 8000,
			},
		},
	}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{})

	require.Equal(t, 3, snapshot.FirstBytePending)
	require.Equal(t, 2, snapshot.SlowFirstBytePending)
	require.Equal(t, 14000.0, snapshot.OldestFirstByteWaitMs)
}
