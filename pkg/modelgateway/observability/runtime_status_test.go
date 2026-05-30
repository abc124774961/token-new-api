package observability_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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

func TestRuntimeStatusServiceCarriesAccountScopeFields(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.4",
		UpstreamModel:         "gpt-5.4",
		ChannelID:             501,
		ResourceID:            "platform:channel:501",
		ResourceType:          core.ResourceTypePlatformOwned,
		AccountID:             "openai:openai:acct-1",
		AccountType:           core.AccountTypeOAuthAccount,
		Brand:                 "openai",
		Provider:              "openai",
		CredentialIndex:       2,
		CredentialSubjectFP:   "subject-fp",
		CredentialFP:          "credential-fp",
		Group:                 "default",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "openai_codex",
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        1,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        3,
	})

	service := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{
		SnapshotStore: store,
		ChannelProvider: func(channelID int) (*model.Channel, bool) {
			if channelID == 501 {
				return &model.Channel{Id: channelID, Name: "OpenAI pooled channel"}, true
			}
			return nil, false
		},
	})
	response := service.Build(observability.RuntimeStatusQuery{ChannelID: 501, Limit: 10})

	require.Len(t, response.Items, 1)
	item := response.Items[0]
	require.Equal(t, "OpenAI pooled channel", item.ChannelName)
	require.Equal(t, key.ResourceID, item.ResourceID)
	require.Equal(t, key.ResourceType, item.ResourceType)
	require.Equal(t, key.AccountID, item.AccountID)
	require.Equal(t, key.AccountType, item.AccountType)
	require.Equal(t, key.Brand, item.Brand)
	require.Equal(t, key.Provider, item.Provider)
	require.Equal(t, key.CredentialIndex, item.CredentialIndex)
	require.Equal(t, key.CredentialSubjectFP, item.CredentialSubjectFP)
	require.Equal(t, key.CredentialFP, item.CredentialFP)
}

func TestRuntimeStatusServiceStrictAccountScopeDoesNotApplyChannelLiveStateOrQueue(t *testing.T) {
	now := time.Unix(1710000100, 0)
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	keyA := core.RuntimeKey{
		RequestedModel:      "gpt-5.5",
		UpstreamModel:       "gpt-5.5",
		ChannelID:           601,
		AccountID:           "acct-a",
		CredentialIndex:     0,
		CredentialSubjectFP: "subject-a",
		CredentialFP:        "credential-a",
		Group:               "default",
		EndpointType:        constant.EndpointTypeOpenAIResponse,
	}
	keyB := core.RuntimeKey{
		RequestedModel:      "gpt-5.5",
		UpstreamModel:       "gpt-5.5",
		ChannelID:           601,
		AccountID:           "acct-b",
		CredentialIndex:     1,
		CredentialSubjectFP: "subject-b",
		CredentialFP:        "credential-b",
		Group:               "default",
		EndpointType:        constant.EndpointTypeOpenAIResponse,
	}
	store.Put(core.RuntimeSnapshot{Key: keyA, SuccessRate: 0.99, ActiveConcurrency: 1, MaxConcurrency: 10, SampleCount: 8})
	store.Put(core.RuntimeSnapshot{Key: keyB, SuccessRate: 0.98, ActiveConcurrency: 0, MaxConcurrency: 10, SampleCount: 8})
	store.Put(core.RuntimeSnapshot{
		Key: core.RuntimeKey{
			RequestedModel:  "gpt-5.5",
			UpstreamModel:   "gpt-5.5",
			ChannelID:       601,
			CredentialIndex: 0,
			Group:           "default",
			EndpointType:    constant.EndpointTypeOpenAIResponse,
		},
		SuccessRate:       0.90,
		ActiveConcurrency: 6,
		QueueDepth:        6,
		SampleCount:       12,
	})

	service := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{
		SnapshotStore: store,
		QueueSnapshot: func() map[int]int {
			return map[int]int{601: 9}
		},
		QueueDetailSnapshot: func() core.RuntimeQueueSnapshot {
			return core.RuntimeQueueSnapshot{
				UpdatedAt: now.Unix(),
				Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Unix(), TotalQueued: 9},
				Channels: []core.RuntimeQueueChannelSnapshot{
					{ChannelID: 601, QueueDepth: 9, QueuedRequests: 9, WaitingRequests: 9, QueueCapacity: 20},
				},
				RuntimeKeys: []core.RuntimeQueueKeySnapshot{
					{RuntimeKey: keyA, RequestedModel: keyA.RequestedModel, UpstreamModel: keyA.UpstreamModel, ChannelID: keyA.ChannelID, Group: keyA.Group, EndpointType: string(keyA.EndpointType), QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2, NormalDepth: 2},
				},
			}
		},
		StateProvider: fakeRuntimeStateProvider{
			active: map[int]int{601: 9},
		},
		Now: func() time.Time { return now },
	})

	response := service.Build(observability.RuntimeStatusQuery{
		ChannelID:          601,
		Limit:              10,
		AccountIDs:         []string{"acct-a", "acct-b"},
		CredentialIndexes:  []int{0, 1},
		StrictAccountScope: true,
	})

	require.Len(t, response.Items, 2)
	byAccount := map[string]observability.RuntimeStatusItem{}
	for _, item := range response.Items {
		byAccount[item.AccountID] = item
	}
	require.Equal(t, 0, byAccount["acct-a"].ActiveConcurrency)
	require.Equal(t, 2, byAccount["acct-a"].QueueDepth)
	require.Equal(t, 0, byAccount["acct-b"].ActiveConcurrency)
	require.Equal(t, 0, byAccount["acct-b"].QueueDepth)
	require.Equal(t, 0, response.Summary.ActiveConcurrency)
	require.Equal(t, 2, response.Summary.QueuedRequests)
	require.NotNil(t, response.QueueSnapshot)
	require.Empty(t, response.QueueSnapshot.Channels)
	require.Len(t, response.QueueSnapshot.RuntimeKeys, 1)
	require.Equal(t, 2, response.QueueSnapshot.Summary.TotalQueued)
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
		Key:                       keyOpen,
		SuccessRate:               0.2,
		DurationMs:                1800,
		ActiveConcurrency:         4,
		MaxConcurrency:            4,
		QueueDepth:                1,
		QueueCapacity:             8,
		CostRatio:                 0.8,
		GroupPriorityRatio:        1.1,
		CircuitState:              core.CircuitStateClosed,
		SampleCount:               6,
		ProbeRecoveryPending:      true,
		ProbeRecoverySuccessCount: 1,
		ProbeRecoveryRequired:     2,
		ProbeTriggerReason:        "failure_avoidance",
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
		QueueDetailSnapshot: func() core.RuntimeQueueSnapshot {
			return core.RuntimeQueueSnapshot{
				UpdatedAt: now.Unix(),
				Summary: core.RuntimeQueueSummary{
					UpdatedAt: now.Unix(),
				},
				Channels: []core.RuntimeQueueChannelSnapshot{
					{
						ChannelID:            101,
						QueueDepth:           2,
						QueuedRequests:       2,
						WaitingRequests:      2,
						QueueCapacity:        8,
						HighPriorityDepth:    1,
						NormalDepth:          1,
						HighPriorityCapacity: 4,
						NormalCapacity:       4,
						Groups: []core.RuntimeQueueGroupSnapshot{
							{ChannelID: 101, Group: "vip", QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2, HighPriorityDepth: 1, NormalDepth: 1},
						},
					},
					{
						ChannelID:       202,
						QueueDepth:      3,
						QueuedRequests:  3,
						WaitingRequests: 3,
						QueueCapacity:   8,
						NormalDepth:     3,
						Groups: []core.RuntimeQueueGroupSnapshot{
							{ChannelID: 202, Group: "vip", QueueDepth: 3, QueuedRequests: 3, WaitingRequests: 3, NormalDepth: 3},
						},
					},
					{ChannelID: 404, QueueDepth: 1, QueuedRequests: 1, WaitingRequests: 1, QueueCapacity: 8},
				},
				RuntimeKeys: []core.RuntimeQueueKeySnapshot{
					{
						RuntimeKey:        keyHealthy,
						RequestedModel:    keyHealthy.RequestedModel,
						UpstreamModel:     keyHealthy.UpstreamModel,
						ChannelID:         keyHealthy.ChannelID,
						Group:             keyHealthy.Group,
						EndpointType:      string(keyHealthy.EndpointType),
						QueueDepth:        2,
						QueuedRequests:    2,
						WaitingRequests:   2,
						HighPriorityDepth: 1,
						NormalDepth:       1,
					},
					{
						RuntimeKey:      keyOpen,
						RequestedModel:  keyOpen.RequestedModel,
						UpstreamModel:   keyOpen.UpstreamModel,
						ChannelID:       keyOpen.ChannelID,
						Group:           keyOpen.Group,
						EndpointType:    string(keyOpen.EndpointType),
						QueueDepth:      3,
						QueuedRequests:  3,
						WaitingRequests: 3,
						NormalDepth:     3,
					},
				},
				RejectReasons: []core.RuntimeQueueReasonCount{{Reason: "max_depth_reached", Count: 2}},
				Nodes: []core.RuntimeQueueNodeSnapshot{
					{
						NodeID:    "node-a",
						UpdatedAt: now.Unix(),
						Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Unix(), TotalQueued: 2},
						Channels: []core.RuntimeQueueChannelSnapshot{
							{
								ChannelID:       101,
								QueueDepth:      2,
								QueuedRequests:  2,
								WaitingRequests: 2,
								QueueCapacity:   8,
								Groups: []core.RuntimeQueueGroupSnapshot{
									{ChannelID: 101, Group: "vip", QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2},
								},
							},
						},
						RuntimeKeys: []core.RuntimeQueueKeySnapshot{
							{RuntimeKey: keyHealthy, RequestedModel: keyHealthy.RequestedModel, ChannelID: keyHealthy.ChannelID, Group: keyHealthy.Group, QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2},
						},
					},
					{
						NodeID:    "node-b",
						UpdatedAt: now.Add(-time.Second).Unix(),
						Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Add(-time.Second).Unix(), TotalQueued: 3},
						Channels: []core.RuntimeQueueChannelSnapshot{
							{
								ChannelID:       202,
								QueueDepth:      3,
								QueuedRequests:  3,
								WaitingRequests: 3,
								QueueCapacity:   8,
								Groups: []core.RuntimeQueueGroupSnapshot{
									{ChannelID: 202, Group: "vip", QueueDepth: 3, QueuedRequests: 3, WaitingRequests: 3},
								},
							},
						},
						RuntimeKeys: []core.RuntimeQueueKeySnapshot{
							{RuntimeKey: keyOpen, RequestedModel: keyOpen.RequestedModel, ChannelID: keyOpen.ChannelID, Group: keyOpen.Group, QueueDepth: 3, QueuedRequests: 3, WaitingRequests: 3},
						},
					},
					{
						NodeID:    "node-c",
						UpdatedAt: now.Unix(),
						Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Unix(), TotalQueued: 1},
						Channels: []core.RuntimeQueueChannelSnapshot{
							{
								ChannelID:       404,
								QueueDepth:      1,
								QueuedRequests:  1,
								WaitingRequests: 1,
								QueueCapacity:   8,
							},
						},
					},
				},
			}
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
	require.Equal(t, 1, response.Summary.ProbeRecoveryPendingChannels)
	require.Equal(t, 1, response.Summary.HighPressureChannels)
	require.Zero(t, response.Summary.SaturatedChannels)
	require.NotNil(t, response.QueueSnapshot)
	require.Equal(t, 5, response.QueueSnapshot.Summary.TotalQueued)
	require.Equal(t, 2, response.QueueSnapshot.Summary.QueueChannels)
	require.Equal(t, 1, response.QueueSnapshot.Summary.HighPriorityDepth)
	require.Equal(t, 4, response.QueueSnapshot.Summary.NormalDepth)
	require.Equal(t, 2, response.QueueSnapshot.Summary.QueueNodes)
	require.Len(t, response.QueueSnapshot.Channels, 2)
	require.Len(t, response.QueueSnapshot.RuntimeKeys, 2)
	require.Len(t, response.QueueSnapshot.Groups, 1)
	require.Len(t, response.QueueSnapshot.Nodes, 2)
	require.Equal(t, "node-b", response.QueueSnapshot.Nodes[0].NodeID)
	require.Equal(t, 3, response.QueueSnapshot.Nodes[0].Summary.TotalQueued)
	require.Equal(t, "node-a", response.QueueSnapshot.Nodes[1].NodeID)
	require.Equal(t, 2, response.QueueSnapshot.Nodes[1].Summary.TotalQueued)
	require.Equal(t, "vip", response.QueueSnapshot.Groups[0].Group)
	require.NotEmpty(t, response.QueueSnapshot.RejectReasons)
	require.NotEmpty(t, response.QueueSnapshot.Cooldowns)

	openItem := response.Items[0]
	require.Equal(t, 202, openItem.ChannelID)
	require.True(t, openItem.CircuitOpen)
	require.Equal(t, "open", openItem.CircuitState)
	require.Equal(t, int64(1710000060), openItem.CircuitOpenUntil)
	require.Equal(t, 3, openItem.QueueDepth)
	require.True(t, openItem.FailureAvoidance)
	require.Equal(t, "upstream_5xx", openItem.FailureAvoidanceReason)
	require.True(t, openItem.ProbeRecoveryPending)
	require.Equal(t, 1, openItem.ProbeRecoverySuccessCount)
	require.Equal(t, 2, openItem.ProbeRecoveryRequired)
	require.Equal(t, "failure_avoidance", openItem.ProbeTriggerReason)
	require.Greater(t, openItem.ScoreTotal, 0.70)
	require.Less(t, openItem.RoutingScoreTotal, openItem.ScoreTotal)
	require.Equal(t, 0.2, openItem.ScoreBreakdown["completion_rate"])
	require.Equal(t, 1.0, openItem.ScoreBreakdown["upstream_error_rate"])
	require.Equal(t, 1.0, openItem.ScoreBreakdown["stream_interrupted_rate"])
	require.Equal(t, "circuit_open", openItem.HealthStatus)
	require.True(t, openItem.CostReferenceMissing)
	require.Zero(t, openItem.ScoreBreakdown["cost"])

	healthyItem := response.Items[1]
	require.Equal(t, 101, healthyItem.ChannelID)
	require.Equal(t, 3, healthyItem.ActiveConcurrency)
	require.Equal(t, 2, healthyItem.QueueDepth)
	require.True(t, healthyItem.Cooldown)
	require.Equal(t, "cooldown", healthyItem.HealthStatus)
	require.True(t, healthyItem.CostReferenceMissing)
	require.Zero(t, healthyItem.ScoreBreakdown["cost"])
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
	require.Equal(t, "queued", response.Items[0].HealthStatus)

	empty := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{}).Build(observability.RuntimeStatusQuery{})
	require.Empty(t, empty.Items)
	require.Zero(t, empty.Summary.RuntimeKeys)
}

func TestRuntimeStatusServiceUsesInjectedGroupPolicyForScore(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      707,
		Group:          "cost-plus",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        1,
		TTFTMs:             400,
		DurationMs:         1600,
		TokensPerSecond:    70,
		CostRatio:          0.40,
		CostReferenceRatio: 0.10,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})

	service := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{
		SnapshotStore: store,
		PolicyForGroup: func(group string) core.GroupSmartPolicy {
			require.Equal(t, "cost-plus", group)
			return core.GroupSmartPolicy{
				Strategy:        core.StrategyCostFirst,
				AutoMode:        core.AutoModeFusion,
				CandidateGroups: []string{"cost-plus", "backup"},
				GroupPriorityRatio: map[string]float64{
					"cost-plus": 1,
					"backup":    1,
				},
			}
		},
		Now: func() time.Time { return time.Unix(200, 0) },
	})

	response := service.Build(observability.RuntimeStatusQuery{Group: "cost-plus"})
	require.Len(t, response.Items, 1)
	item := response.Items[0]
	require.Contains(t, item.ScoreBreakdown, "cost")
	require.InEpsilon(t, 0.1386, item.ScoreBreakdown["cost"], 0.0001)
}

func TestRuntimeStatusServiceFiltersMultiNodeQueueSnapshotByChannel(t *testing.T) {
	now := time.Unix(1710000300, 0)
	keyA := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "openai-codex",
		ChannelID:      901,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	keyB := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "mimo",
		ChannelID:      902,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	keyC := core.RuntimeKey{
		RequestedModel: "deepseek-v4-pro",
		UpstreamModel:  "deepseek-v4-pro",
		ChannelID:      903,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	store.Put(core.RuntimeSnapshot{Key: keyA, SuccessRate: 0.99, QueueDepth: 2, QueueCapacity: 8, SampleCount: 4})
	store.Put(core.RuntimeSnapshot{Key: keyB, SuccessRate: 0.95, QueueDepth: 4, QueueCapacity: 8, SampleCount: 4})
	store.Put(core.RuntimeSnapshot{Key: keyC, SuccessRate: 0.9, QueueDepth: 6, QueueCapacity: 8, SampleCount: 4})

	service := observability.NewRuntimeStatusService(observability.RuntimeStatusDeps{
		SnapshotStore: store,
		QueueSnapshot: func() map[int]int {
			return map[int]int{901: 2, 902: 4, 903: 6}
		},
		QueueDetailSnapshot: func() core.RuntimeQueueSnapshot {
			return core.RuntimeQueueSnapshot{
				UpdatedAt: now.Unix(),
				Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Unix()},
				Channels: []core.RuntimeQueueChannelSnapshot{
					{
						ChannelID:       901,
						QueueDepth:      2,
						QueuedRequests:  2,
						WaitingRequests: 2,
						QueueCapacity:   8,
						Groups: []core.RuntimeQueueGroupSnapshot{
							{ChannelID: 901, Group: "vip", QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2},
						},
					},
					{
						ChannelID:       902,
						QueueDepth:      4,
						QueuedRequests:  4,
						WaitingRequests: 4,
						QueueCapacity:   8,
						Groups: []core.RuntimeQueueGroupSnapshot{
							{ChannelID: 902, Group: "vip", QueueDepth: 4, QueuedRequests: 4, WaitingRequests: 4},
						},
					},
					{
						ChannelID:       903,
						QueueDepth:      6,
						QueuedRequests:  6,
						WaitingRequests: 6,
						QueueCapacity:   8,
						Groups: []core.RuntimeQueueGroupSnapshot{
							{ChannelID: 903, Group: "default", QueueDepth: 6, QueuedRequests: 6, WaitingRequests: 6},
						},
					},
				},
				RuntimeKeys: []core.RuntimeQueueKeySnapshot{
					{RuntimeKey: keyA, RequestedModel: keyA.RequestedModel, UpstreamModel: keyA.UpstreamModel, ChannelID: keyA.ChannelID, Group: keyA.Group, EndpointType: string(keyA.EndpointType), QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2},
					{RuntimeKey: keyB, RequestedModel: keyB.RequestedModel, UpstreamModel: keyB.UpstreamModel, ChannelID: keyB.ChannelID, Group: keyB.Group, EndpointType: string(keyB.EndpointType), QueueDepth: 4, QueuedRequests: 4, WaitingRequests: 4},
					{RuntimeKey: keyC, RequestedModel: keyC.RequestedModel, UpstreamModel: keyC.UpstreamModel, ChannelID: keyC.ChannelID, Group: keyC.Group, EndpointType: string(keyC.EndpointType), QueueDepth: 6, QueuedRequests: 6, WaitingRequests: 6},
				},
				Nodes: []core.RuntimeQueueNodeSnapshot{
					{
						NodeID:    "node-a",
						UpdatedAt: now.Unix(),
						Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Unix()},
						Channels: []core.RuntimeQueueChannelSnapshot{
							{ChannelID: 901, QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2, QueueCapacity: 8, Groups: []core.RuntimeQueueGroupSnapshot{{ChannelID: 901, Group: "vip", QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2}}},
							{ChannelID: 903, QueueDepth: 6, QueuedRequests: 6, WaitingRequests: 6, QueueCapacity: 8, Groups: []core.RuntimeQueueGroupSnapshot{{ChannelID: 903, Group: "default", QueueDepth: 6, QueuedRequests: 6, WaitingRequests: 6}}},
						},
						RuntimeKeys: []core.RuntimeQueueKeySnapshot{
							{RuntimeKey: keyA, RequestedModel: keyA.RequestedModel, ChannelID: keyA.ChannelID, Group: keyA.Group, QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2},
							{RuntimeKey: keyC, RequestedModel: keyC.RequestedModel, ChannelID: keyC.ChannelID, Group: keyC.Group, QueueDepth: 6, QueuedRequests: 6, WaitingRequests: 6},
						},
					},
					{
						NodeID:    "node-b",
						UpdatedAt: now.Add(-time.Second).Unix(),
						Summary:   core.RuntimeQueueSummary{UpdatedAt: now.Add(-time.Second).Unix()},
						Channels: []core.RuntimeQueueChannelSnapshot{
							{ChannelID: 902, QueueDepth: 4, QueuedRequests: 4, WaitingRequests: 4, QueueCapacity: 8, Groups: []core.RuntimeQueueGroupSnapshot{{ChannelID: 902, Group: "vip", QueueDepth: 4, QueuedRequests: 4, WaitingRequests: 4}}},
						},
						RuntimeKeys: []core.RuntimeQueueKeySnapshot{
							{RuntimeKey: keyB, RequestedModel: keyB.RequestedModel, ChannelID: keyB.ChannelID, Group: keyB.Group, QueueDepth: 4, QueuedRequests: 4, WaitingRequests: 4},
						},
					},
				},
			}
		},
		Now: func() time.Time { return now },
	})

	response := service.Build(observability.RuntimeStatusQuery{ChannelID: 902, Limit: 10})

	require.Len(t, response.Items, 1)
	require.Equal(t, 902, response.Items[0].ChannelID)
	require.Equal(t, 4, response.Items[0].QueueDepth)
	require.NotNil(t, response.QueueSnapshot)
	require.Equal(t, 4, response.QueueSnapshot.Summary.TotalQueued)
	require.Equal(t, 1, response.QueueSnapshot.Summary.QueueChannels)
	require.Equal(t, 1, response.QueueSnapshot.Summary.QueueNodes)
	require.Len(t, response.QueueSnapshot.Channels, 1)
	require.Equal(t, 902, response.QueueSnapshot.Channels[0].ChannelID)
	require.Len(t, response.QueueSnapshot.RuntimeKeys, 1)
	require.Equal(t, keyB, response.QueueSnapshot.RuntimeKeys[0].RuntimeKey)
	require.Len(t, response.QueueSnapshot.Nodes, 1)
	require.Equal(t, "node-b", response.QueueSnapshot.Nodes[0].NodeID)
	require.Equal(t, 4, response.QueueSnapshot.Nodes[0].Summary.TotalQueued)
	require.Len(t, response.QueueSnapshot.Nodes[0].RuntimeKeys, 1)
	require.Equal(t, keyB, response.QueueSnapshot.Nodes[0].RuntimeKeys[0].RuntimeKey)
}
