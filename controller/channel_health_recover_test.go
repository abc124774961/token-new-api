package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelResponseSurfacesRuntimeAccountBalanceInsufficient(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)

	channel := model.Channel{
		Type:   1,
		Name:   "runtime-balance-visible",
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	service.MarkChannelRuntimeBalanceInsufficient(service.ChannelRuntimeIdentity{
		ChannelID: channel.Id,
		AccountID: "acct-a",
	})
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficientForChannel(channel.Id)
	})

	resp := buildChannelResponseWithDisplays(&channel, nil, nil, service.RuntimeBalanceInsufficientCountForChannel(channel.Id))
	require.NotNil(t, resp)
	require.True(t, resp.BalanceInsufficient)
	require.Equal(t, 1, resp.RuntimeBalanceInsufficientCount)
}

func TestRecoverChannelHealthClearsRuntimeAndMultiKeyBalanceState(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps)

	channel := model.Channel{
		Type:   1,
		Name:   "recover-balance",
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusAutoDisabled,
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{0: service.ChannelStatusReasonBalanceInsufficient},
			MultiKeyDisabledTime:   map[int]int64{0: common.GetTimestamp()},
		},
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": channelBalanceAllAccountsDisabledReason,
		"status_time":   common.GetTimestamp(),
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)

	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps)
	require.NotNil(t, runtimeDeps.SnapshotStore)
	require.NotNil(t, runtimeDeps.CircuitBreaker)
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 2)
	runtimeKey := modelgatewayaccount.RuntimeKeyForChannelAccount(modelgatewaycore.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}, accounts[0])
	identity := modelGatewayRuntimeIdentityFromCoreKey(runtimeKey)
	service.MarkChannelRuntimeBalanceInsufficient(identity)
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficientForChannel(channel.Id)
		service.ClearChannelFailureAvoidance(channel.Id)
		service.ClearChannelConfigIsolationForChannel(channel.Id)
	})
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                               runtimeKey,
		ScoreStatsJSON:                    `{"version":1,"samples":50,"rates":{"completion":{"success":2,"total":50},"empty_output":{"success":45,"total":50}},"latency":{"ttft_ms":[20000],"duration_ms":[90000]}}`,
		RecentLatencySamples:              []modelgatewaycore.RuntimeLatencySample{{ObservedAt: 1710000000, TTFTMs: 20000, DurationMs: 90000}},
		SuccessRate:                       0.04,
		TTFTMs:                            20000,
		DurationMs:                        90000,
		TokensPerSecond:                   1,
		EmptyOutputRate:                   0.90,
		ExperienceIssueRate:               0.80,
		ActiveConcurrency:                 3,
		QueueDepth:                        4,
		EstimatedQueueWaitMs:              3000,
		FirstBytePending:                  2,
		SlowFirstBytePending:              1,
		OldestFirstByteWaitMs:             12000,
		CostRatio:                         2,
		CostReferenceRatio:                0.5,
		GroupPriorityRatio:                0.2,
		CircuitState:                      modelgatewaycore.CircuitStateOpen,
		CircuitOpen:                       true,
		CircuitOpenReason:                 scheduler.CircuitErrorServer,
		CircuitFailureCount:               5,
		CircuitFailureRate:                1,
		CircuitSampleCount:                5,
		CircuitErrorCounts:                map[string]int{scheduler.CircuitErrorServer: 5},
		Cooldown:                          true,
		FailureAvoidance:                  true,
		RecoverableQualityScore:           0.2,
		RecoverableQualityBaseline:        0.9,
		RecoverableQualityBaselineSamples: 5,
		RecoverableQualityDropRatio:       0.7,
		RecoverableQualityItemBaselines:   map[string]float64{"ttft_latency": 0.95},
		ProbeRecoveryPending:              true,
		ProbeRecoverySuccessCount:         1,
		ProbeRecoveryRequired:             2,
		ProbeTriggerReason:                modelgatewaycore.ProbeReasonScoreAnomalyFastProbe,
		ProbeRecoveryPhase:                modelgatewaycore.ProbeRecoveryPhaseFastProbe,
		ProbeFastRecoveryAttempts:         3,
		ProbeAnomalyTriggerItems:          []string{"ttft_latency"},
		ConfigErrorIsolated:               true,
		IsolationReason:                   modelgatewaycore.ErrorCategoryAuthConfigError,
		IsolationUntil:                    common.GetTimestamp() + 3600,
		AuthConfigErrorCount:              2,
		LastAuthConfigErrorAt:             common.GetTimestamp(),
		SampleCount:                       50,
		LastRealFailureAt:                 1710000000,
	})
	service.RecordChannelRuntimeTimeoutRecovery(identity, nil)
	service.RecordChannelConfigAuthError(service.NewChannelRuntimeConfigIsolationKey(identity, "gpt-5.5", "default", constant.EndpointTypeOpenAI), modelgatewaycore.ErrorCategoryAuthConfigError)
	service.RecordChannelConfigAuthError(service.NewChannelRuntimeConfigIsolationKey(identity, "gpt-5.5", "default", constant.EndpointTypeOpenAI), modelgatewaycore.ErrorCategoryAuthConfigError)
	require.NotNil(t, service.GetChannelRuntimeFailureAvoidanceStatus(identity))

	router := gin.New()
	router.POST("/api/channel/:id/recover_health", RecoverChannelHealth)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(channel.Id)+"/recover_health", nil)
	router.ServeHTTP(resp, req)

	var payload struct {
		Success bool                         `json:"success"`
		Message string                       `json:"message"`
		Data    ChannelHealthRecoverResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success, payload.Message)
	require.Equal(t, 1, payload.Data.RuntimeBalanceCleared)
	require.Equal(t, 1, payload.Data.MultiKeyBalanceCleared)
	require.Equal(t, 1, payload.Data.RuntimeHealthSnapshotsReset)
	require.True(t, payload.Data.ConfigIsolationCleared)
	require.True(t, payload.Data.StatusUpdated)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.ChannelInfo.MultiKeyStatusList, 0)
	require.NotContains(t, updated.ChannelInfo.MultiKeyDisabledReason, 0)
	require.False(t, service.IsRuntimeBalanceInsufficientIdentity(identity))
	require.Nil(t, service.GetChannelRuntimeFailureAvoidanceStatus(identity))
	require.Nil(t, service.GetChannelConfigIsolationStatus(service.NewChannelRuntimeConfigIsolationKey(identity, "gpt-5.5", "default", constant.EndpointTypeOpenAI)))

	snapshot, ok := runtimeDeps.SnapshotStore.Get(runtimeKey)
	require.True(t, ok)
	require.Equal(t, 50, snapshot.SampleCount)
	require.Equal(t, 1.0, snapshot.SuccessRate)
	require.Equal(t, 800.0, snapshot.TTFTMs)
	require.Equal(t, 3000.0, snapshot.DurationMs)
	require.Equal(t, 80.0, snapshot.TokensPerSecond)
	require.Empty(t, snapshot.ScoreStatsJSON)
	require.Len(t, snapshot.RecentLatencySamples, 3)
	require.Zero(t, snapshot.EmptyOutputRate)
	require.Zero(t, snapshot.ExperienceIssueRate)
	require.Zero(t, snapshot.ActiveConcurrency)
	require.Zero(t, snapshot.QueueDepth)
	require.Zero(t, snapshot.EstimatedQueueWaitMs)
	require.Zero(t, snapshot.FirstBytePending)
	require.Zero(t, snapshot.SlowFirstBytePending)
	require.Zero(t, snapshot.OldestFirstByteWaitMs)
	require.Equal(t, snapshot.CostRatio, snapshot.CostReferenceRatio)
	require.Equal(t, 1.0, snapshot.GroupPriorityRatio)
	require.False(t, snapshot.CircuitOpen)
	require.False(t, snapshot.Cooldown)
	require.False(t, snapshot.FailureAvoidance)
	require.False(t, snapshot.ProbeRecoveryPending)
	require.Empty(t, snapshot.ProbeTriggerReason)
	require.Empty(t, snapshot.ProbeRecoveryPhase)
	require.Empty(t, snapshot.ProbeAnomalyTriggerItems)
	require.Zero(t, snapshot.ProbeFastRecoveryAttempts)
	require.False(t, snapshot.ConfigErrorIsolated)
	require.Empty(t, snapshot.IsolationReason)
	require.Zero(t, snapshot.IsolationUntil)
	require.Zero(t, snapshot.AuthConfigErrorCount)
	require.Zero(t, snapshot.LastAuthConfigErrorAt)
	require.Zero(t, snapshot.LastRealFailureAt)
	require.NotZero(t, snapshot.LastRealSuccessAt)
	assertRecoveredChannelScoresAllOne(t, &channel, snapshot)

	queue := BuildModelGatewayHealthCheckQueue(modelgatewayobservability.RuntimeStatusQuery{
		Model:     "gpt-5.5",
		Group:     "default",
		ChannelID: channel.Id,
	}, modelGatewayHealthCheckQueueTypeAll)
	require.Zero(t, queue.Summary.PendingCount)
	require.Empty(t, queue.Items)
}

func assertRecoveredChannelScoresAllOne(t *testing.T, channel *model.Channel, snapshot modelgatewaycore.RuntimeSnapshot) {
	t.Helper()
	evaluation := scheduler.NewCandidateScoringService().EvaluateCandidate(
		modelgatewaycore.Candidate{
			Channel:       channel,
			Group:         snapshot.Key.Group,
			UpstreamModel: snapshot.Key.UpstreamModel,
			RuntimeKey:    snapshot.Key,
		},
		snapshot,
		modelgatewaycore.GroupSmartPolicy{
			Strategy:           modelgatewaycore.StrategyBalanced,
			GroupPriorityRatio: map[string]float64{snapshot.Key.Group: 1},
		},
		scheduler.ScoringContext{
			RequestedModel: snapshot.Key.RequestedModel,
			EndpointType:   snapshot.Key.EndpointType,
			Strategy:       modelgatewaycore.StrategyBalanced,
		},
	)
	require.Equal(t, 1.0, evaluation.Score.Total)
	require.Equal(t, 1.0, evaluation.Score.RoutingTotal)
	for _, item := range evaluation.Score.Items {
		if item.Weight <= 0 || item.MissingReason != "" {
			continue
		}
		require.Equalf(t, 1.0, item.Score, "score item %s should reset to 1", item.Key)
	}
	for _, item := range evaluation.Score.RoutingItems {
		if item.Weight <= 0 || item.MissingReason != "" {
			continue
		}
		require.Equalf(t, 1.0, item.Score, "routing score item %s should reset to 1", item.Key)
	}
}

func clearRuntimeBalanceInsufficientForControllerTest(t *testing.T) {
	t.Helper()
	service.ClearChannelBalanceInsufficientForChannel(1)
	service.ClearChannelBalanceInsufficientForChannel(2)
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficientForChannel(1)
		service.ClearChannelBalanceInsufficientForChannel(2)
	})
}
