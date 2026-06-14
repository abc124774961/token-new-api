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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

type fakeCostProfileProvider struct {
	ratio           float64
	ratiosByChannel map[int]float64
	reference       float64
	ok              bool
}

type fakeRuntimeStateProviderWithAccountAvoidance struct {
	*testkit.FakeRuntimeStateProvider
	avoidanceByAccount map[string]*service.ChannelFailureAvoidanceStatus
}

func (p *fakeRuntimeStateProviderWithAccountAvoidance) FailureAvoidanceStatusForIdentity(identity service.ChannelRuntimeIdentity) *service.ChannelFailureAvoidanceStatus {
	if p == nil {
		return nil
	}
	return p.avoidanceByAccount[service.ChannelRuntimeConcurrencyScopeKey(identity)]
}

func (p *fakeRuntimeStateProviderWithAccountAvoidance) FirstBytePendingStatusForIdentity(identity service.ChannelRuntimeIdentity) *service.ChannelFirstBytePendingStatus {
	return nil
}

func (p fakeCostProfileProvider) CostRatio(channelID int, upstreamModel string) (float64, bool) {
	if p.ratiosByChannel != nil {
		ratio, ok := p.ratiosByChannel[channelID]
		return ratio, ok
	}
	return p.ratio, p.ok
}

func (p fakeCostProfileProvider) CostReferenceRatio(upstreamModel string, pricingMode string) (float64, bool) {
	if !p.ok || p.reference <= 0 {
		return 0, false
	}
	return p.reference, true
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

func TestRuntimeSnapshotEnricherAppliesAccountMaxConcurrency(t *testing.T) {
	identity := service.ChannelRuntimeIdentity{
		ChannelID:           77,
		AccountID:           "acct-a",
		CredentialIndex:     1,
		CredentialIndexSet:  true,
		CredentialSubjectFP: "subject-a",
		CredentialFP:        "credential-a",
	}
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{
		ActiveConcurrencyByChannel: map[int]int{77: 12},
		ActiveConcurrencyByAccount: map[string]int{
			service.ChannelRuntimeConcurrencyScopeKey(identity): 3,
		},
	}, 1500, 64, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{
			Id:      77,
			Setting: common.GetPointer(`{"max_concurrency":40}`),
			ChannelInfo: model.ChannelInfo{
				IsMultiKey:             true,
				MultiKeySize:           2,
				MultiKeyMaxConcurrency: map[int]int{1: 4},
			},
		},
		AccountIdentity: core.AccountIdentity{
			AccountID:                    identity.AccountID,
			CredentialIndex:              identity.CredentialIndex,
			CredentialSubjectFingerprint: identity.CredentialSubjectFP,
			CredentialFingerprint:        identity.CredentialFP,
		},
		CredentialRef: core.CredentialRef{
			AccountID:                    identity.AccountID,
			CredentialIndex:              identity.CredentialIndex,
			CredentialSubjectFingerprint: identity.CredentialSubjectFP,
			CredentialFingerprint:        identity.CredentialFP,
			Resolver:                     "channel_key",
		},
		Group: "codex-plus",
	}, core.RuntimeSnapshot{
		MaxConcurrency: 40,
	}, core.GroupSmartPolicy{QueueEnabled: true})

	require.Equal(t, 3, snapshot.ActiveConcurrency)
	require.Equal(t, 4, snapshot.MaxConcurrency)
	require.Equal(t, 4, snapshot.ConfiguredConcurrencyLimit)
	require.Equal(t, 4, snapshot.LearnedConcurrencyLimit)
	require.Equal(t, 4, snapshot.EffectiveConcurrencyLimit)
	require.Equal(t, 8, snapshot.QueueCapacity)
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
	enricher.WithCostProfileProvider(fakeCostProfileProvider{ratio: 0.42, reference: 0.21, ok: true})

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
		},
	}, core.RuntimeSnapshot{CostRatio: 1}, core.GroupSmartPolicy{})

	require.Equal(t, 0.42, snapshot.CostRatio)
	require.Equal(t, 0.21, snapshot.CostReferenceRatio)
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

func TestRuntimeSnapshotEnricherAppliesRevenueRatioFromBillingRatios(t *testing.T) {
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-revenue-test":2}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
	})

	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "codex-plus",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-revenue-test",
		},
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{
		GroupRevenueRatio: map[string]float64{
			"codex-plus": 0.25,
		},
	})

	require.Equal(t, 1.0, snapshot.RevenueRatio)
}

func TestRuntimeSnapshotEnricherAppliesUserBillingMultiplierToRevenueRatio(t *testing.T) {
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-user-revenue-test":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
	})

	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2).
		WithBillingMultiplierEvaluator(func(ctx model.BillingMultiplierContext) types.BillingMultiplierSnapshot {
			require.Equal(t, 901, ctx.UserID)
			require.Equal(t, "default", ctx.UserGroup)
			require.Equal(t, "codex-plus", ctx.UsingGroup)
			require.Equal(t, "gpt-user-revenue-test", ctx.ModelName)
			return types.BillingMultiplierSnapshot{
				Applied:         true,
				BaseGroupRatio:  ctx.BaseGroupRatio,
				FinalGroupRatio: ctx.BaseGroupRatio * 0.5,
				Multiplier:      0.5,
			}
		})

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "codex-plus",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-user-revenue-test",
		},
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{
		UserID:    901,
		UserGroup: "default",
		GroupRevenueRatio: map[string]float64{
			"codex-plus": 1,
		},
	})

	require.Equal(t, 1.0, snapshot.RevenueRatio)
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

func TestRuntimeSnapshotEnricherMarksTimeoutRecoveryPending(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{
		FailureAvoidanceStatusByChannel: map[int]*service.ChannelFailureAvoidanceStatus{
			7: {
				Active:                true,
				Reason:                service.ChannelTimeoutRecoveryReason,
				ProbeRecoveryRequired: true,
				FailureCount:          3,
			},
		},
	}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{})

	require.True(t, snapshot.FailureAvoidance)
	require.True(t, snapshot.ProbeRecoveryPending)
	require.Equal(t, service.ChannelTimeoutRecoveryReason, snapshot.ProbeTriggerReason)
	require.Equal(t, 2, snapshot.ProbeRecoveryRequired)
	require.Equal(t, 1, snapshot.EffectiveConcurrencyLimit)
}

func TestRuntimeSnapshotEnricherClearsStaleFailureAvoidanceSnapshot(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
	}, core.RuntimeSnapshot{
		FailureAvoidance:          true,
		ProbeRecoveryPending:      true,
		ProbeRecoverySuccessCount: 1,
		ProbeRecoveryRequired:     2,
		ProbeTriggerReason:        service.ChannelOverloadRecoveryReason,
		ProbeRecoveryPhase:        "probe",
		ProbeFastRecoveryAttempts: 1,
		ProbeAnomalyTriggerItems:  []string{"upstream_error_rate"},
	}, core.GroupSmartPolicy{})

	require.False(t, snapshot.FailureAvoidance)
	require.False(t, snapshot.ProbeRecoveryPending)
	require.Zero(t, snapshot.ProbeRecoverySuccessCount)
	require.Zero(t, snapshot.ProbeRecoveryRequired)
	require.Empty(t, snapshot.ProbeTriggerReason)
	require.Empty(t, snapshot.ProbeRecoveryPhase)
	require.Zero(t, snapshot.ProbeFastRecoveryAttempts)
	require.Empty(t, snapshot.ProbeAnomalyTriggerItems)
}

func TestRuntimeSnapshotEnricherFallsBackToChannelAvoidanceForAccountCandidate(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&fakeRuntimeStateProviderWithAccountAvoidance{
		FakeRuntimeStateProvider: &testkit.FakeRuntimeStateProvider{
			FailureAvoidanceStatusByChannel: map[int]*service.ChannelFailureAvoidanceStatus{
				7: {
					Active:                true,
					Reason:                service.ChannelOverloadRecoveryReason,
					ProbeRecoveryRequired: true,
					FailureCount:          1,
				},
			},
		},
	}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel:  "gpt-5.5",
			ChannelID:       7,
			Group:           "default",
			AccountID:       "acct-a",
			CredentialIndex: 0,
		},
	}, core.RuntimeSnapshot{}, core.GroupSmartPolicy{})

	require.True(t, snapshot.FailureAvoidance)
	require.True(t, snapshot.ProbeRecoveryPending)
	require.Equal(t, service.ChannelOverloadRecoveryReason, snapshot.ProbeTriggerReason)
}

func TestRuntimeSnapshotEnricherClearsLegacyConfigIsolation(t *testing.T) {
	enricher := scheduler.NewRuntimeSnapshotEnricher(&testkit.FakeRuntimeStateProvider{}, 1500, 8, 2)

	snapshot := enricher.Enrich(core.Candidate{
		Channel: &model.Channel{Id: 7},
		Group:   "default",
		RuntimeKey: core.RuntimeKey{
			RequestedModel: "gpt-5-codex",
			EndpointType:   "openai",
		},
	}, core.RuntimeSnapshot{
		ConfigErrorIsolated:   true,
		IsolationReason:       "auth_config_error",
		IsolationUntil:        1770000000,
		AuthConfigErrorCount:  2,
		LastAuthConfigErrorAt: 1769999900,
	}, core.GroupSmartPolicy{})

	require.False(t, snapshot.ConfigErrorIsolated)
	require.Empty(t, snapshot.IsolationReason)
	require.Zero(t, snapshot.IsolationUntil)
	require.Equal(t, 2, snapshot.AuthConfigErrorCount)
	require.EqualValues(t, 1769999900, snapshot.LastAuthConfigErrorAt)
}
