package controller

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

	unsupported := false
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
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification:         channelcapability.ClassificationAccountUsageLimited,
					UsageLimitStatus:                 channelcapability.UsageLimitStatusLimited,
					UsageLimitReason:                 channelcapability.UsageLimitReasonReached,
					UsageLimitMessage:                "usage limit reached",
					UsageLimitDetectedTime:           common.GetTimestamp(),
					UsageLimitExpiresAt:              common.GetTimestamp() + 3600,
					UsageLimitResetSource:            "retry_after_seconds",
					CodexBackendResponsesStreamWrite: &unsupported,
					LastEndpoint:                     "codex",
					LastMessage:                      "usage limit reached",
				},
				1: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
					ProxyLastError:           "proxy failed",
					LastEndpoint:             "auth",
					LastMessage:              "token invalid",
				},
			},
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
	service.BeginChannelRuntimeFirstByteWait(nil, identity, "recover-health-req", 0)
	require.NotNil(t, service.GetChannelRuntimeFailureAvoidanceStatus(identity))
	require.NotNil(t, service.GetChannelRuntimeFirstBytePendingStatus(identity))

	persistedKey := modelgatewaycore.RuntimeKey{
		RequestedModel: "gpt-6.1",
		UpstreamModel:  "gpt-6.1",
		ChannelID:      channel.Id,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	persistedKeyBytes, err := common.Marshal(persistedKey)
	require.NoError(t, err)
	persistedHash := runtimeSnapshotPersistenceHashForControllerTest(t, persistedKey)
	require.NoError(t, db.Create(&model.ModelGatewayRuntimeSnapshot{
		RuntimeKeyHash:                    persistedHash,
		RuntimeKey:                        string(persistedKeyBytes),
		ChannelID:                         channel.Id,
		SampleCount:                       20,
		SuccessRate:                       0.05,
		TTFTMs:                            30000,
		DurationMs:                        120000,
		EmptyOutputRate:                   0.75,
		ExperienceIssueRate:               0.60,
		RecoverableQualityScore:           0.1,
		RecoverableQualityBaseline:        0.9,
		RecoverableQualityBaselineSamples: 5,
		RecoverableQualityDropRatio:       0.8,
		ProbeRecoveryPending:              true,
		ProbeRecoverySuccessCount:         1,
		ProbeRecoveryRequired:             3,
		ProbeTriggerReason:                modelgatewaycore.ProbeReasonScoreAnomalyFastProbe,
		ProbeRecoveryPhase:                modelgatewaycore.ProbeRecoveryPhaseFastProbe,
		ProbeFastRecoveryAttempts:         2,
		LastRealFailureAt:                 common.GetTimestamp(),
		ConfigErrorIsolated:               true,
		IsolationReason:                   modelgatewaycore.ErrorCategoryAuthConfigError,
		IsolationUntil:                    common.GetTimestamp() + 3600,
		AuthConfigErrorCount:              2,
		LastAuthConfigErrorAt:             common.GetTimestamp(),
	}).Error)

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
	require.Equal(t, 1, payload.Data.PersistedRuntimeSnapshotsReset)
	require.Equal(t, 1, payload.Data.FirstBytePendingCleared)
	require.True(t, payload.Data.ConfigIsolationCleared)
	require.True(t, payload.Data.StatusUpdated)
	require.Equal(t, 2, payload.Data.AccountSchedulingBlocksCleared)
	require.Equal(t, 1, payload.Data.AccountUsageLimitCleared)
	require.Equal(t, 2, payload.Data.AccountCapabilityBlocksCleared)
	require.Equal(t, 1, payload.Data.AccountProxyErrorsCleared)
	require.Equal(t, 1, payload.Data.AccountNegativeCapabilitiesReset)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.ChannelInfo.MultiKeyStatusList, 0)
	require.NotContains(t, updated.ChannelInfo.MultiKeyDisabledReason, 0)
	require.False(t, service.IsRuntimeBalanceInsufficientIdentity(identity))
	require.Nil(t, service.GetChannelRuntimeFailureAvoidanceStatus(identity))
	require.Nil(t, service.GetChannelRuntimeFirstBytePendingStatus(identity))
	require.Nil(t, service.GetChannelConfigIsolationStatus(service.NewChannelRuntimeConfigIsolationKey(identity, "gpt-5.5", "default", constant.EndpointTypeOpenAI)))
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[0].CapabilityClassification)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[0].UsageLimitStatus)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[0].UsageLimitReason)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[0].UsageLimitMessage)
	require.Zero(t, updated.ChannelInfo.MultiKeyCapabilities[0].UsageLimitExpiresAt)
	require.Nil(t, updated.ChannelInfo.MultiKeyCapabilities[0].CodexBackendResponsesStreamWrite)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[1].CapabilityClassification)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[1].ProxyLastError)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[1].LastEndpoint)
	require.Empty(t, updated.ChannelInfo.MultiKeyCapabilities[1].LastMessage)

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

	persistedSnapshot, ok := runtimeDeps.SnapshotStore.Get(persistedKey)
	require.True(t, ok)
	require.Equal(t, 1.0, persistedSnapshot.SuccessRate)
	require.False(t, persistedSnapshot.ProbeRecoveryPending)
	require.Empty(t, persistedSnapshot.ProbeTriggerReason)
	require.Zero(t, persistedSnapshot.LastRealFailureAt)

	var persistedRow model.ModelGatewayRuntimeSnapshot
	require.NoError(t, db.Where("runtime_key_hash = ?", persistedHash).First(&persistedRow).Error)
	require.Equal(t, 1.0, persistedRow.SuccessRate)
	require.Empty(t, persistedRow.ScoreStatsJSON)
	require.NotEmpty(t, persistedRow.LatencySamples)
	require.False(t, persistedRow.ProbeRecoveryPending)
	require.Empty(t, persistedRow.ProbeTriggerReason)
	require.Empty(t, persistedRow.ProbeRecoveryPhase)
	require.Zero(t, persistedRow.ProbeFastRecoveryAttempts)
	require.False(t, persistedRow.ConfigErrorIsolated)
	require.Empty(t, persistedRow.IsolationReason)
	require.Zero(t, persistedRow.IsolationUntil)
	require.Zero(t, persistedRow.AuthConfigErrorCount)
	require.Zero(t, persistedRow.LastAuthConfigErrorAt)
	require.Zero(t, persistedRow.LastRealFailureAt)

	queue := BuildModelGatewayHealthCheckQueue(modelgatewayobservability.RuntimeStatusQuery{
		Model:     "gpt-5.5",
		Group:     "default",
		ChannelID: channel.Id,
	}, modelGatewayHealthCheckQueueTypeAll)
	require.Zero(t, queue.Summary.PendingCount)
	require.Empty(t, queue.Items)
}

func TestRecoverChannelHealthRestoresAutoDisabledErrorPauseAndAbility(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)

	channel := model.Channel{
		Type:   1,
		Name:   "recover-error-pause",
		Key:    "sk-error-pause",
		Status: common.ChannelStatusAutoDisabled,
		Group:  "default",
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": service.ChannelStatusReasonErrorPaused,
		"status_time":   common.GetTimestamp(),
		"pause_until":   common.GetTimestamp() + 3600,
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)

	payload := postRecoverChannelHealthForTest(t, channel.Id)
	require.True(t, payload.Success, payload.Message)
	require.True(t, payload.Data.StatusUpdated)
	require.Equal(t, common.ChannelStatusEnabled, payload.Data.ChannelStatus)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Empty(t, updated.GetOtherInfo()["status_reason"])
	require.Empty(t, updated.GetOtherInfo()["pause_until"])
	require.True(t, channelRecoverTestAbilityEnabled(t, db, channel.Id))
}

func TestRecoverChannelHealthClearsRequestDisabledAccountsButKeepsConfigDisabledAccounts(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)

	channel := model.Channel{
		Type:   1,
		Name:   "recover-account-status",
		Key:    "sk-a\nsk-b\nsk-c",
		Status: common.ChannelStatusAutoDisabled,
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         3,
			MultiKeyStatusList:   map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusManuallyDisabled, 2: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledTime: map[int]int64{0: common.GetTimestamp(), 1: common.GetTimestamp(), 2: common.GetTimestamp()},
			MultiKeyDisabledReason: map[int]string{
				0: "temporary upstream failure",
				1: "manual",
				2: channelAccountAuthReauthorizationPendingReason,
			},
		},
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": channelAccountAllKeysDisabledReason,
		"status_time":   common.GetTimestamp(),
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)

	payload := postRecoverChannelHealthForTest(t, channel.Id)
	require.True(t, payload.Success, payload.Message)
	require.True(t, payload.Data.StatusUpdated)
	require.Equal(t, 1, payload.Data.AccountStatusBlocksCleared)
	require.Equal(t, common.ChannelStatusEnabled, payload.Data.ChannelStatus)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.ChannelInfo.MultiKeyStatusList, 0)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.ChannelInfo.MultiKeyStatusList[2])
	require.Equal(t, "manual", updated.ChannelInfo.MultiKeyDisabledReason[1])
	require.Equal(t, channelAccountAuthReauthorizationPendingReason, updated.ChannelInfo.MultiKeyDisabledReason[2])
	require.True(t, channelRecoverTestAbilityEnabled(t, db, channel.Id))
}

func TestRecoverChannelHealthDoesNotEnableManualDisabledChannel(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)

	channel := model.Channel{
		Type:   1,
		Name:   "recover-manual-disabled",
		Key:    "sk-manual",
		Status: common.ChannelStatusManuallyDisabled,
		Group:  "default",
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": channelAccountManualDisabledReason,
		"status_time":   common.GetTimestamp(),
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)

	payload := postRecoverChannelHealthForTest(t, channel.Id)
	require.True(t, payload.Success, payload.Message)
	require.False(t, payload.Data.StatusUpdated)
	require.Equal(t, common.ChannelStatusManuallyDisabled, payload.Data.ChannelStatus)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.Status)
	require.False(t, channelRecoverTestAbilityEnabled(t, db, channel.Id))
}

func postRecoverChannelHealthForTest(t *testing.T, channelID int) struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Data    ChannelHealthRecoverResponse `json:"data"`
} {
	t.Helper()
	router := gin.New()
	router.POST("/api/channel/:id/recover_health", RecoverChannelHealth)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(channelID)+"/recover_health", nil)
	router.ServeHTTP(resp, req)

	var payload struct {
		Success bool                         `json:"success"`
		Message string                       `json:"message"`
		Data    ChannelHealthRecoverResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	return payload
}

func channelRecoverTestAbilityEnabled(t *testing.T, db *gorm.DB, channelID int) bool {
	t.Helper()
	var ability model.Ability
	require.NoError(t, db.Where("channel_id = ?", channelID).First(&ability).Error)
	return ability.Enabled
}

func runtimeSnapshotPersistenceHashForControllerTest(t *testing.T, key modelgatewaycore.RuntimeKey) string {
	t.Helper()
	data, err := common.Marshal(key)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
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
