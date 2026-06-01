package controller

import (
	"archive/zip"
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelAccountsAPIResponse struct {
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
	Data    ChannelAccountsResponse `json:"data"`
}

type channelAccountRecentRequestsAPIResponse struct {
	Success bool                                 `json:"success"`
	Message string                               `json:"message"`
	Data    ChannelAccountRecentRequestsResponse `json:"data"`
}

type channelAccountRequestReconcileAPIResponse struct {
	Success bool                                   `json:"success"`
	Message string                                 `json:"message"`
	Data    ChannelAccountRequestReconcileResponse `json:"data"`
}

type codexApplicationEnvironmentListAPIResponse struct {
	Success bool                                    `json:"success"`
	Message string                                  `json:"message"`
	Data    CodexApplicationEnvironmentListResponse `json:"data"`
}

func setupChannelAccountControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.ModelExecutionRecord{},
		&model.ModelGatewayUserRequestSummary{},
		&model.ModelGatewayChannelCostProfile{},
		&model.ModelGatewayRequestCostSummary{},
		&model.ModelGatewayRuntimeSnapshot{},
		&model.ModelGatewayScoreEvent{},
		&model.ModelGatewayProxy{},
		&model.ModelGatewayProxyUsage{},
		&model.ChannelAccountUsageEvent{},
		&model.CodexApplicationEnvironment{},
	))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))

	oldDB := model.DB
	model.DB = db
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	modelgatewaycost.InvalidateDefaultProfileCache()
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(func() {
		modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
		modelgatewaycost.InvalidateDefaultProfileCache()
		restoreSetting()
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestListChannelAccountsHidesRawKeysAndCarriesScoreSummary(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	env := model.CodexApplicationEnvironment{
		Id:           7007,
		Name:         "codex-env-test",
		Platform:     "macOS",
		AppVersion:   "0.135.0",
		UserAgent:    "Codex Desktop/0.135.0",
		Originator:   "Codex Desktop",
		SessionID:    "session-test",
		WindowID:     "window-test",
		BetaFeatures: "responses_compact",
		Enabled:      true,
	}
	require.NoError(t, db.Create(&env).Error)
	channel := model.Channel{
		Id:     7,
		Name:   "codex accounts",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-primary\nsk-disabled",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{1: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{1: "auth failed"},
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					ResponsesWrite:       common.GetPointer(true),
					ChatCompletionsWrite: common.GetPointer(true),
					CheckedTime:          1700000003,
					LastEndpoint:         "responses",
					LastMessage:          "ok",
				},
			},
			MultiKeyCodexEnvironmentIDs: map[int]int{0: env.Id},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 7,
		Enabled:   true,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:           7,
		UpstreamModel:       "*",
		Currency:            "USD",
		PricingMode:         "token",
		InputPerMillion:     0.2,
		OutputPerMillion:    0.8,
		CacheReadPerMillion: 0.05,
		EffectiveTime:       common.GetTimestamp() - 1,
		Version:             1,
	}).Error)
	modelgatewaycost.StoreCachedDefaultProfile(model.ModelGatewayChannelCostProfile{
		ChannelID:           7,
		UpstreamModel:       "*",
		Currency:            "USD",
		PricingMode:         "token",
		InputPerMillion:     0.2,
		OutputPerMillion:    0.8,
		CacheReadPerMillion: 0.05,
		EffectiveTime:       common.GetTimestamp() - 1,
		Version:             1,
	})

	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 2)
	runtimeKey := modelgatewayaccount.RuntimeKeyForChannelAccount(modelgatewaycore.RuntimeKey{
		RequestedModel: "gpt-5.4",
		UpstreamModel:  "gpt-5.4",
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}, accounts[0])
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                 runtimeKey,
		SuccessRate:         0.92,
		TTFTMs:              420,
		DurationMs:          1800,
		CostRatio:           0.32,
		CostReferenceRatio:  0.2,
		CostPricingMode:     "token",
		EmptyOutputRate:     0.01,
		ExperienceIssueRate: 0.02,
		SampleCount:         9,
		RealSampleCount30m:  3,
		LastRealSuccessAt:   1700000001,
		LastProbeSuccessAt:  1700000002,
	})

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/7/accounts", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 7, payload.Data.ChannelID)
	require.Equal(t, 2, payload.Data.Total)
	require.Equal(t, 1, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)
	require.Len(t, payload.Data.Items, 2)
	require.NotContains(t, recorder.Body.String(), "sk-primary")
	require.NotContains(t, recorder.Body.String(), "sk-disabled")
	require.NotEmpty(t, payload.Data.Items[0].SubjectShort)
	require.NotEmpty(t, payload.Data.Items[0].CredentialShort)
	require.NotNil(t, payload.Data.Items[0].Capabilities)
	require.Equal(t, int64(1700000003), payload.Data.Items[0].Capabilities.CheckedTime)
	require.True(t, *payload.Data.Items[0].Capabilities.ResponsesWrite)
	require.Equal(t, env.Id, payload.Data.Items[0].CodexEnvironmentID)
	require.NotNil(t, payload.Data.Items[0].CodexEnvironment)
	require.Equal(t, "codex-env-test", payload.Data.Items[0].CodexEnvironment.Name)
	require.Equal(t, "Codex Desktop/0.135.0", payload.Data.Items[0].CodexEnvironment.Headers["User-Agent"])
	require.NotNil(t, payload.Data.Items[0].Score)
	require.Equal(t, 9, payload.Data.Items[0].Score.SampleCount)
	require.Equal(t, "gpt-5.4", payload.Data.Items[0].Score.RuntimeKey.RequestedModel)
	require.Len(t, payload.Data.Items[0].RuntimeKeys, 1)
	require.False(t, payload.Data.Items[1].KeyEnabled)
	require.Equal(t, "auth failed", payload.Data.Items[1].DisabledReason)
	require.Nil(t, payload.Data.Items[1].Score)
}

func TestListCodexApplicationEnvironmentsReturnsHeaderSummary(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	envs := []model.CodexApplicationEnvironment{
		{
			Id:           7101,
			Name:         "codex-env-enabled",
			Platform:     "Windows",
			AppVersion:   "0.135.0",
			UserAgent:    "Codex Desktop/0.135.0",
			Originator:   "Codex Desktop",
			SessionID:    "session-enabled",
			WindowID:     "window-enabled",
			BetaFeatures: "terminal_resize_reflow",
			HeadersJSON:  `{"x-extra-feature":"enabled"}`,
			Enabled:      true,
		},
		{
			Id:         7102,
			Name:       "codex-env-disabled",
			Platform:   "Linux",
			UserAgent:  "Codex CLI/0.45.0",
			Originator: "codex_cli_rs",
			Enabled:    true,
		},
	}
	require.NoError(t, db.Create(&envs).Error)
	require.NoError(t, db.Model(&model.CodexApplicationEnvironment{}).Where("id = ?", 7102).Update("enabled", false).Error)

	router := gin.New()
	router.GET("/api/channel/codex-environments", ListCodexApplicationEnvironments)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/codex-environments", nil)
	router.ServeHTTP(recorder, req)
	payload := decodeCodexApplicationEnvironmentListResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 1, payload.Data.Total)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "codex-env-enabled", payload.Data.Items[0].Name)
	require.Equal(t, "enabled", payload.Data.Items[0].Headers["x-extra-feature"])

	includeRecorder := httptest.NewRecorder()
	includeReq := httptest.NewRequest(http.MethodGet, "/api/channel/codex-environments?include_disabled=true", nil)
	router.ServeHTTP(includeRecorder, includeReq)
	includePayload := decodeCodexApplicationEnvironmentListResponse(t, includeRecorder)
	require.True(t, includePayload.Success, includeRecorder.Body.String())
	require.Equal(t, 2, includePayload.Data.Total)
}

func TestListChannelAccountsIncludesSchedulingExplanation(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	now := common.GetTimestamp()
	channel := model.Channel{
		Id:     38,
		Name:   "codex scheduling",
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"access-a","account_id":"acct-a"}` + "\n" + `{"access_token":"access-b","account_id":"acct-b"}`,
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{1: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{1: "manual disabled"},
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: common.GetPointer(true),
					UsageLimitStatus:                 channelcapability.UsageLimitStatusLimited,
					UsageLimitReason:                 channelcapability.UsageLimitReasonReached,
					UsageLimitMessage:                "usage limit has been reached",
					UsageLimitDetectedTime:           now - 30,
					UsageLimitExpiresAt:              now + 600,
					UsageLimitResetSource:            "retry_after_seconds",
				},
			},
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/38/accounts", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.Items, 2)
	first := payload.Data.Items[0]
	require.NotNil(t, first.Scheduling)
	require.False(t, first.Scheduling.Schedulable)
	require.Equal(t, channelcapability.ClassificationAccountUsageLimited, first.Scheduling.PrimaryReason)
	require.Contains(t, first.Scheduling.BlockingReasons, channelcapability.ClassificationAccountUsageLimited)
	require.Equal(t, now+600, first.Scheduling.RecoveryAt)
	require.Equal(t, "retry_after_seconds", first.Scheduling.RecoverySource)

	second := payload.Data.Items[1]
	require.NotNil(t, second.Scheduling)
	require.False(t, second.Scheduling.Schedulable)
	require.Equal(t, "account_disabled", second.Scheduling.PrimaryReason)
	require.Contains(t, second.Scheduling.BlockingReasons, "account_disabled")
}

func TestBuildChannelAccountSchedulingExplanationKeepsAuthAheadOfProxyExitError(t *testing.T) {
	item := ChannelAccountItem{
		KeyEnabled: true,
		Capabilities: &model.ChannelAccountCapability{
			CapabilityClassification: channelcapability.ClassificationAuthError,
			ProxyLastError:           "invalid character 'P' looking for beginning of value",
		},
	}

	explanation := buildChannelAccountSchedulingExplanation(item)
	require.False(t, explanation.Schedulable)
	require.Equal(t, channelcapability.ClassificationAuthError, explanation.PrimaryReason)
	require.Contains(t, explanation.BlockingReasons, channelcapability.ClassificationAuthError)
	require.NotContains(t, explanation.BlockingReasons, channelcapability.ClassificationProxyError)
}

func TestListChannelAccountsSummaryCountsAvailabilityStates(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     40,
		Name:   "availability summary accounts",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-ok\nsk-disabled\nsk-recovery\nsk-circuit",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           4,
			MultiKeyStatusList:     map[int]int{1: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{1: "manual disabled"},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: 40,
		Enabled:   true,
	}).Error)
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 4)
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	baseKey := modelgatewaycore.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	keyOK := modelgatewayaccount.RuntimeKeyForChannelAccount(baseKey, accounts[0])
	keyRecovery := modelgatewayaccount.RuntimeKeyForChannelAccount(baseKey, accounts[2])
	keyCircuit := modelgatewayaccount.RuntimeKeyForChannelAccount(baseKey, accounts[3])
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                       keyOK,
		SuccessRate:               0.99,
		EffectiveConcurrencyLimit: 10,
		SampleCount:               12,
	})
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                       keyRecovery,
		SuccessRate:               0.98,
		FailureAvoidance:          true,
		ProbeRecoveryPending:      true,
		ProbeTriggerReason:        "timeout_recovery",
		ProbeRecoverySuccessCount: 1,
		ProbeRecoveryRequired:     2,
		EffectiveConcurrencyLimit: 10,
		SampleCount:               10,
	})
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                       keyCircuit,
		SuccessRate:               0.97,
		CircuitOpen:               true,
		CircuitState:              modelgatewaycore.CircuitStateOpen,
		EffectiveConcurrencyLimit: 10,
		SampleCount:               9,
	})

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/40/accounts?page=1&page_size=20", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.Items, 4)
	require.Equal(t, int64(3), payload.Data.Summary.Scored)
	require.Equal(t, int64(1), payload.Data.Summary.SchedulableAccounts)
	require.Equal(t, int64(3), payload.Data.Summary.BlockedAccounts)
	require.Equal(t, int64(1), payload.Data.Summary.RecoveryAccounts)
	require.Equal(t, int64(1), payload.Data.Summary.CircuitOpenAccounts)

	itemsByIndex := map[int]ChannelAccountItem{}
	for _, item := range payload.Data.Items {
		itemsByIndex[item.CredentialIndex] = item
	}
	require.True(t, itemsByIndex[0].Scheduling.Schedulable)
	require.False(t, itemsByIndex[2].Scheduling.Schedulable)
	require.Contains(t, itemsByIndex[2].Scheduling.BlockingReasons, "probe_recovery_pending")
	require.False(t, itemsByIndex[3].Scheduling.Schedulable)
	require.Contains(t, itemsByIndex[3].Scheduling.BlockingReasons, "circuit_open")

	pagedRecorder := httptest.NewRecorder()
	pagedReq := httptest.NewRequest(http.MethodGet, "/api/channel/40/accounts?page=1&page_size=2&sort=credential_index&order=asc", nil)
	router.ServeHTTP(pagedRecorder, pagedReq)
	pagedPayload := decodeChannelAccountsResponse(t, pagedRecorder)
	require.True(t, pagedPayload.Success, pagedRecorder.Body.String())
	require.Len(t, pagedPayload.Data.Items, 2)
	require.Equal(t, int64(3), pagedPayload.Data.Summary.Scored)
	require.Equal(t, int64(1), pagedPayload.Data.Summary.SchedulableAccounts)
	require.Equal(t, int64(3), pagedPayload.Data.Summary.BlockedAccounts)
	require.Equal(t, int64(1), pagedPayload.Data.Summary.RecoveryAccounts)
	require.Equal(t, int64(1), pagedPayload.Data.Summary.CircuitOpenAccounts)
}

func TestListChannelAccountsUsesSingleKeyChannelRuntimeFallback(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     8,
		Name:   "single",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-single",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key: modelgatewaycore.RuntimeKey{
			RequestedModel: "gpt-5.4",
			UpstreamModel:  "gpt-5.4",
			ChannelID:      8,
			Group:          "default",
			EndpointType:   constant.EndpointTypeOpenAI,
		},
		SuccessRate: 0.88,
		SampleCount: 4,
		TTFTMs:      900,
	})

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/8/accounts", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.Items, 1)
	require.NotNil(t, payload.Data.Items[0].Score)
	require.Equal(t, 4, payload.Data.Items[0].Score.SampleCount)
	require.Equal(t, 0.88, payload.Data.Items[0].Score.SuccessRate)
}

func TestListChannelAccountsStatsViewUsesAccountScopedRuntime(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     42,
		Name:   "account scoped runtime",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-account-a\nsk-account-b",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: 42,
		Enabled:   true,
	}).Error)

	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 2)
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	baseKey := modelgatewaycore.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAIResponse,
	}
	keyA := modelgatewayaccount.RuntimeKeyForChannelAccount(baseKey, accounts[0])
	keyB := modelgatewayaccount.RuntimeKeyForChannelAccount(baseKey, accounts[1])
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                       keyA,
		SuccessRate:               0.99,
		ActiveConcurrency:         1,
		EffectiveConcurrencyLimit: 10,
		SampleCount:               8,
	})
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key:                       keyB,
		SuccessRate:               0.98,
		ActiveConcurrency:         0,
		EffectiveConcurrencyLimit: 10,
		SampleCount:               8,
	})
	runtimeDeps.SnapshotStore.Put(modelgatewaycore.RuntimeSnapshot{
		Key: modelgatewaycore.RuntimeKey{
			RequestedModel:  "gpt-5.5",
			UpstreamModel:   "gpt-5.5",
			ChannelID:       42,
			CredentialIndex: 0,
			Group:           "default",
			EndpointType:    constant.EndpointTypeOpenAIResponse,
		},
		SuccessRate:               0.90,
		ActiveConcurrency:         6,
		EffectiveConcurrencyLimit: 10,
		QueueDepth:                6,
		SampleCount:               12,
	})

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/42/accounts?view=stats&page=1&page_size=20", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, channelAccountViewStats, payload.Data.View)
	require.Len(t, payload.Data.Items, 2)
	items := map[int]ChannelAccountItem{}
	for _, item := range payload.Data.Items {
		items[item.CredentialIndex] = item
	}
	require.NotNil(t, items[0].Score)
	require.NotNil(t, items[1].Score)
	require.Equal(t, 0, items[0].Score.ActiveConcurrency)
	require.Equal(t, 0, items[0].Score.QueueDepth)
	require.Equal(t, 0, items[1].Score.ActiveConcurrency)
	require.Equal(t, 0, items[1].Score.QueueDepth)
}

func TestChannelAccountCredentialUIDDoesNotDependOnCredentialIndex(t *testing.T) {
	account := modelgatewayaccount.ChannelAccount{
		ChannelID:       42,
		CredentialIndex: 0,
		AccountIdentity: modelgatewaycore.AccountIdentity{
			Brand:                        "openai",
			CredentialSubjectFingerprint: "abcdef0123456789",
			CredentialFingerprint:        "1234567890abcdef",
		},
		CredentialRef: modelgatewaycore.CredentialRef{
			CredentialIndex:              0,
			CredentialSubjectFingerprint: "abcdef0123456789",
			CredentialFingerprint:        "1234567890abcdef",
		},
	}

	uid := channelAccountCredentialUID(account)
	label := channelAccountCredentialLabel(account)
	account.CredentialIndex = 8
	account.CredentialRef.CredentialIndex = 8

	require.Equal(t, "acct-abcdef01", uid)
	require.Equal(t, uid, channelAccountCredentialUID(account))
	require.Equal(t, "openai-acct-abcdef01", label)
}

func TestChannelAccountCredentialUIDHashesFallbackIdentity(t *testing.T) {
	account := modelgatewayaccount.ChannelAccount{
		ChannelID:       42,
		CredentialIndex: 0,
		AccountIdentity: modelgatewaycore.AccountIdentity{
			Brand:              "codex",
			AccountIdentityKey: "codex:codex:user@example.com",
		},
	}

	uid := channelAccountCredentialUID(account)

	require.Regexp(t, `^acct-[0-9a-f]{8}$`, uid)
	require.NotContains(t, uid, "example")
	require.NotContains(t, uid, "codex:")
}

func TestListChannelAccountsSupportsServerSidePaginationAndStatusFilter(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     33,
		Name:   "paged accounts",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two\nsk-three\nsk-four",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           4,
			MultiKeyStatusList:     map[int]int{1: common.ChannelStatusManuallyDisabled, 3: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{1: "maintenance", 3: "balance_insufficient"},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 33,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/33/accounts?status=disabled&page=2&page_size=1&sort=credential_index&order=asc", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, channelAccountViewManage, payload.Data.View)
	require.Equal(t, 4, payload.Data.Total)
	require.Equal(t, 2, payload.Data.FilteredTotal)
	require.Equal(t, 2, payload.Data.Enabled)
	require.Equal(t, 2, payload.Data.Disabled)
	require.Equal(t, 2, payload.Data.Page)
	require.Equal(t, 1, payload.Data.PageSize)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, 3, payload.Data.Items[0].CredentialIndex)
	require.False(t, payload.Data.Items[0].KeyEnabled)
	require.Equal(t, "balance_insufficient", payload.Data.Items[0].DisabledReason)
	require.NotContains(t, recorder.Body.String(), "sk-one")
	require.NotContains(t, recorder.Body.String(), "sk-four")
}

func TestListChannelAccountsDefaultSortsEnabledAccountsFirst(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     39,
		Name:   "enabled first accounts",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-disabled-zero\nsk-enabled-one\nsk-disabled-two\nsk-enabled-three",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       4,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled, 2: common.ChannelStatusAutoDisabled},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 39,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/39/accounts?page=1&page_size=4", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.Items, 4)
	require.Equal(t, []int{1, 3, 0, 2}, []int{
		payload.Data.Items[0].CredentialIndex,
		payload.Data.Items[1].CredentialIndex,
		payload.Data.Items[2].CredentialIndex,
		payload.Data.Items[3].CredentialIndex,
	})
	require.True(t, payload.Data.Items[0].KeyEnabled)
	require.True(t, payload.Data.Items[1].KeyEnabled)
	require.False(t, payload.Data.Items[2].KeyEnabled)
	require.False(t, payload.Data.Items[3].KeyEnabled)
}

func TestListChannelAccountsStatsViewAggregatesUsageAndExcludesHealthProbes(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     34,
		Name:   "stats accounts",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-stat-one\nsk-stat-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 34,
		Enabled:   true,
	}).Error)
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 2)
	now := time.Now().Unix()
	account := accounts[0]
	require.NoError(t, db.Create(&[]model.ChannelAccountUsageEvent{
		{
			RequestId:          "stats-user-success",
			ChannelID:          34,
			ChannelName:        channel.Name,
			CredentialIndex:    account.CredentialIndex,
			AccountID:          account.AccountIdentity.AccountID,
			AccountIdentityKey: account.AccountIdentity.AccountIdentityKey,
			AccountType:        account.AccountIdentity.AccountType,
			Brand:              account.AccountIdentity.Brand,
			Provider:           account.AccountIdentity.Provider,
			RequestedModel:     "gpt-5.4",
			RequestedGroup:     "default",
			SelectedGroup:      "default",
			EndpointType:       string(constant.EndpointTypeOpenAI),
			CompletedAt:        now - 60,
			Success:            true,
			StatusCode:         http.StatusOK,
			DurationMs:         1200,
			TTFTMs:             320,
			PromptTokens:       100,
			CompletionTokens:   40,
			TotalTokens:        140,
			Quota:              1400,
			UpstreamCostTotal:  0.0025,
			CostSource:         "profile",
			CostAccuracy:       "precise",
			CostCalculatedAt:   now - 50,
		},
		{
			RequestId:          "stats-user-timeout",
			ChannelID:          34,
			ChannelName:        channel.Name,
			CredentialIndex:    account.CredentialIndex,
			AccountID:          account.AccountIdentity.AccountID,
			AccountIdentityKey: account.AccountIdentity.AccountIdentityKey,
			AccountType:        account.AccountIdentity.AccountType,
			Brand:              account.AccountIdentity.Brand,
			Provider:           account.AccountIdentity.Provider,
			RequestedModel:     "gpt-5.4",
			RequestedGroup:     "default",
			SelectedGroup:      "default",
			EndpointType:       string(constant.EndpointTypeOpenAI),
			CompletedAt:        now - 40,
			Success:            false,
			StatusCode:         http.StatusGatewayTimeout,
			ErrorCategory:      model.ModelGatewayUserRequestErrorTimeout,
			DurationMs:         5000,
			TTFTMs:             0,
		},
		{
			RequestId:          "stats-health-probe",
			ChannelID:          34,
			ChannelName:        channel.Name,
			CredentialIndex:    account.CredentialIndex,
			AccountID:          account.AccountIdentity.AccountID,
			AccountIdentityKey: account.AccountIdentity.AccountIdentityKey,
			AccountType:        account.AccountIdentity.AccountType,
			Brand:              account.AccountIdentity.Brand,
			Provider:           account.AccountIdentity.Provider,
			RequestedModel:     "gpt-5.4",
			RequestedGroup:     "default",
			SelectedGroup:      "default",
			EndpointType:       string(constant.EndpointTypeOpenAI),
			CompletedAt:        now - 20,
			Success:            true,
			StatusCode:         http.StatusOK,
			IsHealthProbe:      true,
			DurationMs:         100,
			TTFTMs:             20,
			PromptTokens:       999,
			CompletionTokens:   999,
			TotalTokens:        1998,
			Quota:              19980,
			UpstreamCostTotal:  99,
		},
	}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/34/accounts?view=stats&page=1&page_size=20", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, channelAccountViewStats, payload.Data.View)
	require.True(t, payload.Data.Summary.HealthProbeExcluded)
	require.Equal(t, int64(2), payload.Data.Summary.Today.Requests)
	require.Equal(t, int64(1), payload.Data.Summary.Today.SuccessRequests)
	require.Equal(t, int64(1), payload.Data.Summary.Today.ErrorRequests)
	require.Equal(t, int64(1), payload.Data.Summary.Today.TimeoutRequests)
	require.Equal(t, int64(140), payload.Data.Summary.Today.TotalTokens)
	require.Equal(t, int64(1400), payload.Data.Summary.Today.Quota)
	require.InEpsilon(t, 0.0025, payload.Data.Summary.Today.UpstreamCostTotal, 0.0001)
	require.InEpsilon(t, 0.5, payload.Data.Summary.Today.SuccessRate, 0.0001)
	require.Len(t, payload.Data.Items, 2)
	require.Equal(t, 0, payload.Data.Items[0].CredentialIndex)
	require.NotNil(t, payload.Data.Items[0].Stats)
	require.Equal(t, int64(2), payload.Data.Items[0].Stats.Today.Requests)
	require.Equal(t, model.ModelGatewayUserRequestErrorTimeout, payload.Data.Items[0].Stats.Today.TopErrorCategory)
	require.Equal(t, int64(1), payload.Data.Items[0].Stats.Today.TopErrorCount)
	require.Equal(t, int64(0), payload.Data.Items[1].Stats.Today.Requests)
}

func TestChannelAccountRecentRequestsAndAttributionRefresh(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     35,
		Name:   "recent accounts",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-recent-one\nsk-recent-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:       "req-recent-account",
		AttemptIndex:    2,
		ChannelID:       35,
		CredentialIndex: 1,
		RequestedModel:  "gpt-5.4",
		CompletedAt:     now - 30,
		Success:         false,
		StatusCode:      http.StatusTooManyRequests,
		ErrorCategory:   "rate_limit",
		IsHealthProbe:   true,
		DurationMs:      900,
	}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/accounts/:credential_index/requests", ListChannelAccountRecentRequests)
	router.POST("/api/channel/:id/accounts/:credential_index/refresh-attribution", RefreshChannelAccountUsageAttribution)

	getRecorder := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/channel/35/accounts/1/requests", nil)
	router.ServeHTTP(getRecorder, getReq)
	getPayload := decodeChannelAccountRecentRequestsResponse(t, getRecorder)
	require.True(t, getPayload.Success, getRecorder.Body.String())
	require.Len(t, getPayload.Data.Items, 1)
	require.Equal(t, "req-recent-account", getPayload.Data.Items[0].RequestID)
	require.Equal(t, 2, getPayload.Data.Items[0].AttemptIndex)
	require.Equal(t, 2, getPayload.Data.Items[0].AccountDisplayIndex)
	require.True(t, getPayload.Data.Items[0].IsHealthProbe)
	require.Equal(t, "health_probe", getPayload.Data.Items[0].StatisticsStatus)
	require.Equal(t, "health_probe_excluded", getPayload.Data.Items[0].StatisticsDiagnostic)
	require.False(t, getPayload.Data.Items[0].AttributionComplete)

	refreshRecorder := httptest.NewRecorder()
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/channel/35/accounts/1/refresh-attribution", nil)
	router.ServeHTTP(refreshRecorder, refreshReq)
	refreshPayload := decodeChannelAccountRecentRequestsResponse(t, refreshRecorder)
	require.True(t, refreshPayload.Success, refreshRecorder.Body.String())
	require.NotNil(t, refreshPayload.Data.RefreshResult)
	require.Equal(t, 1, refreshPayload.Data.RefreshResult.Scanned)
	require.Equal(t, 1, refreshPayload.Data.RefreshResult.Updated)
	require.Len(t, refreshPayload.Data.Items, 1)
	require.True(t, refreshPayload.Data.Items[0].AttributionComplete)
	require.NotEmpty(t, refreshPayload.Data.Items[0].AccountIdentityKey)
}

func TestChannelAccountRequestReconcileIncludesUsageSummaryAndSamples(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	now := time.Now().Unix()
	requestID := "req-reconcile-account"
	require.NoError(t, db.Create(&model.Channel{
		Id:     36,
		Name:   "reconcile-channel",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b\nsk-c",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
		},
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:             requestID,
		AttemptIndex:          1,
		ChannelID:             36,
		CredentialIndex:       2,
		AccountIdentityKey:    "account:reconcile",
		CredentialFingerprint: "cred-reconcile",
		RequestedModel:        "gpt-5.5",
		CompletedAt:           now,
		Success:               true,
		StatusCode:            http.StatusOK,
		DurationMs:            1800,
		TTFTMs:                320,
		PromptTokens:          100,
		CompletionTokens:      40,
		TotalTokens:           140,
		Quota:                 1500,
		UpstreamCostTotal:     0.0025,
		CostCalculatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:        requestID,
		CreatedAt:        now,
		UpdatedAt:        now,
		CompletedAt:      now,
		RequestedModel:   "gpt-5.5",
		RequestedGroup:   "default",
		SelectedGroup:    "vip",
		FinalChannelID:   36,
		FinalChannelName: "reconcile-channel",
		Attempts:         2,
		LastAttemptIndex: 1,
		FinalSuccess:     true,
		DurationMs:       1800,
		TTFTMs:           320,
	}).Error)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now,
		RequestId:      requestID,
		AttemptIndex:   1,
		ChannelId:      36,
		ChannelName:    "reconcile-channel",
		Success:        true,
		StatusCode:     http.StatusOK,
		DurationMs:     1800,
		TTFTMs:         320,
		SmartHandled:   true,
		ScoreTotal:     0.91,
		SelectedReason: "score",
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayScoreEvent{
		TraceID:         "trace-reconcile",
		RequestID:       requestID,
		AttemptIndex:    1,
		ChannelID:       36,
		CredentialIndex: 2,
		RequestedModel:  "gpt-5.5",
		Group:           "vip",
		BeforeTotal:     0.8,
		AfterTotal:      0.91,
		Delta:           0.11,
		CreatedAt:       now,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         requestID,
		ChannelID:         36,
		UpstreamModel:     "gpt-5.5",
		UpstreamCostTotal: 0.0025,
		CostSource:        "profile",
		CostAccuracy:      "precise",
		CalculatedAt:      now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/accounts/:credential_index/requests/:request_id/reconcile", GetChannelAccountRequestReconcile)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/36/accounts/2/requests/"+requestID+"/reconcile", nil)
	router.ServeHTTP(recorder, req)
	payload := decodeChannelAccountRequestReconcileResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, requestID, payload.Data.RequestID)
	require.Equal(t, 3, payload.Data.AccountDisplayIndex)
	require.NotNil(t, payload.Data.UsageEvent)
	require.Equal(t, "complete", payload.Data.UsageEvent.StatisticsStatus)
	require.NotNil(t, payload.Data.UserRequest)
	require.Equal(t, 2, payload.Data.UserRequest.Attempts)
	require.Len(t, payload.Data.ExecutionRecords, 1)
	require.Len(t, payload.Data.ScoreEvents, 1)
	require.NotNil(t, payload.Data.CostSummary)
	require.Contains(t, reconcileCheckStatuses(payload.Data.Checks), "usage_event:ok")
	require.Contains(t, reconcileCheckStatuses(payload.Data.Checks), "account_match:ok")
	require.Contains(t, reconcileCheckStatuses(payload.Data.Checks), "samples:ok")
	require.Contains(t, reconcileDiagnosisKeys(payload.Data.Diagnoses), "trace_complete")
}

func TestUpdateChannelAccountStatusDisablesMultiKeyAccount(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     9,
		Name:   "multi manage",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 9,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/status", UpdateChannelAccountStatus)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"enabled":false,"reason":"manual test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/9/accounts/1/status", body)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 1, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)
	require.True(t, payload.Data.Items[0].KeyEnabled)
	require.False(t, payload.Data.Items[1].KeyEnabled)
	require.Equal(t, "manual test", payload.Data.Items[1].DisabledReason)

	updated, err := model.GetChannelById(9, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.NotZero(t, updated.ChannelInfo.MultiKeyDisabledTime[1])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 9).Error)
	require.True(t, ability.Enabled)
	require.NotContains(t, recorder.Body.String(), "sk-one")
	require.NotContains(t, recorder.Body.String(), "sk-two")
}

func TestUpdateChannelAccountStatusEnablesMultiKeyAndRestoresAutoDisabledChannel(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     10,
		Name:   "restore multi",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusAutoDisabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusManuallyDisabled,
				1: common.ChannelStatusManuallyDisabled,
			},
			MultiKeyDisabledReason: map[int]string{
				0: "manual",
				1: "manual",
			},
			MultiKeyDisabledTime: map[int]int64{
				0: 1700000000,
				1: 1700000001,
			},
		},
	}
	channel.SetOtherInfo(map[string]interface{}{"status_reason": channelAccountAllKeysDisabledReason})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 10,
		Enabled:   false,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/status", UpdateChannelAccountStatus)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/10/accounts/0/status", bytes.NewBufferString(`{"action":"enable"}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 1, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)
	require.True(t, payload.Data.Items[0].KeyEnabled)
	require.False(t, payload.Data.Items[1].KeyEnabled)

	updated, err := model.GetChannelById(10, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	_, disabled := updated.ChannelInfo.MultiKeyStatusList[0]
	require.False(t, disabled)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.Empty(t, updated.GetOtherInfo()["status_reason"])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 10).Error)
	require.True(t, ability.Enabled)
}

func TestUpdateChannelAccountStatusMapsSingleKeyToChannelStatus(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     11,
		Name:   "single manage",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-single",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 11,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/status", UpdateChannelAccountStatus)

	disableRecorder := httptest.NewRecorder()
	disableReq := httptest.NewRequest(http.MethodPost, "/api/channel/11/accounts/0/status", bytes.NewBufferString(`{"enabled":false}`))
	router.ServeHTTP(disableRecorder, disableReq)
	disablePayload := decodeChannelAccountsResponse(t, disableRecorder)
	require.True(t, disablePayload.Success, disableRecorder.Body.String())
	require.False(t, disablePayload.Data.Items[0].KeyEnabled)

	disabledChannel, err := model.GetChannelById(11, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, disabledChannel.Status)
	require.Equal(t, channelAccountManualDisabledReason, disabledChannel.GetOtherInfo()["status_reason"])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 11).Error)
	require.False(t, ability.Enabled)

	enableRecorder := httptest.NewRecorder()
	enableReq := httptest.NewRequest(http.MethodPost, "/api/channel/11/accounts/0/status", bytes.NewBufferString(`{"action":"enable"}`))
	router.ServeHTTP(enableRecorder, enableReq)
	enablePayload := decodeChannelAccountsResponse(t, enableRecorder)
	require.True(t, enablePayload.Success, enableRecorder.Body.String())
	require.True(t, enablePayload.Data.Items[0].KeyEnabled)

	enabledChannel, err := model.GetChannelById(11, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, enabledChannel.Status)
	require.Empty(t, enabledChannel.GetOtherInfo()["status_reason"])
	require.NoError(t, db.First(&ability, "channel_id = ?", 11).Error)
	require.True(t, ability.Enabled)
	require.NotContains(t, enableRecorder.Body.String(), "sk-single")
}

func TestUpdateChannelAccountCredentialReplacesOneKeyAndHidesRawCredential(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     33,
		Name:   "credential update",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-old\nsk-keep",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusManuallyDisabled,
			},
			MultiKeyDisabledReason: map[int]string{1: "manual"},
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts/:credential_index", UpdateChannelAccountCredential)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/channel/33/accounts/0",
		bytes.NewBufferString(`{"credential":"sk-new","credential_type":"api_key"}`),
	)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "credential", payload.Data.Operation.Type)
	require.Equal(t, "update", payload.Data.Operation.Action)
	require.Equal(t, 2, payload.Data.Total)
	require.Equal(t, modelgatewaycore.AccountTypeAPIKey, payload.Data.Items[0].AccountIdentity.AccountType)
	require.False(t, payload.Data.Items[1].KeyEnabled)
	require.Equal(t, "manual", payload.Data.Items[1].DisabledReason)
	require.NotContains(t, recorder.Body.String(), "sk-new")
	require.NotContains(t, recorder.Body.String(), "sk-old")
	require.NotContains(t, recorder.Body.String(), "sk-keep")

	updated, err := model.GetChannelById(33, true)
	require.NoError(t, err)
	require.Equal(t, "sk-new\nsk-keep", updated.Key)
	require.Equal(t, modelgatewaycore.AccountTypeAPIKey, updated.ChannelInfo.MultiKeyAccountTypes[0])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
}

func TestUpdateChannelAccountCredentialCanOnlyChangeCodexEnvironment(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	env := model.CodexApplicationEnvironment{
		Id:         7037,
		Name:       "codex-env-update",
		UserAgent:  "Codex Desktop/0.136.0",
		Originator: "Codex Desktop",
		Enabled:    true,
	}
	require.NoError(t, db.Create(&env).Error)
	channel := model.Channel{
		Id:     37,
		Name:   "environment update",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-old\nsk-keep",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts/:credential_index", UpdateChannelAccountCredential)
	recorder := httptest.NewRecorder()
	bodyBytes, err := common.Marshal(map[string]interface{}{
		"codex_environment_id": env.Id,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/37/accounts/0", bytes.NewReader(bodyBytes))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, env.Id, payload.Data.Items[0].CodexEnvironmentID)
	require.NotNil(t, payload.Data.Items[0].CodexEnvironment)
	require.Equal(t, "codex-env-update", payload.Data.Items[0].CodexEnvironment.Name)

	updated, err := model.GetChannelById(37, true)
	require.NoError(t, err)
	require.Equal(t, "sk-old\nsk-keep", updated.Key)
	require.Equal(t, env.Id, updated.ChannelInfo.MultiKeyCodexEnvironmentIDs[0])
}

func TestUpdateChannelAccountCredentialSupportsOAuthJSONType(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     34,
		Name:   "oauth credential update",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-old",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts/:credential_index", UpdateChannelAccountCredential)
	recorder := httptest.NewRecorder()
	jsonCredential := "{\n  \"account_id\": \"acct-34\",\n  \"access_token\": \"access-34\",\n  \"refresh_token\": \"refresh-34\"\n}"
	bodyBytes, err := common.Marshal(map[string]interface{}{
		"credential":      jsonCredential,
		"credential_type": modelgatewaycore.AccountTypeOAuthAccount,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/34/accounts/0", bytes.NewReader(bodyBytes))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, modelgatewaycore.AccountTypeOAuthAccount, payload.Data.Items[0].AccountIdentity.AccountType)
	require.NotContains(t, recorder.Body.String(), "access-34")
	require.NotContains(t, recorder.Body.String(), "refresh-34")

	updated, err := model.GetChannelById(34, true)
	require.NoError(t, err)
	require.JSONEq(t, jsonCredential, updated.Key)
	require.NotContains(t, updated.Key, "\n")
	require.Equal(t, modelgatewaycore.AccountTypeOAuthAccount, updated.ChannelInfo.MultiKeyAccountTypes[0])
}

func TestUpdateChannelAccountCredentialRejectsDuplicateCredential(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     35,
		Name:   "credential duplicate",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts/:credential_index", UpdateChannelAccountCredential)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/channel/35/accounts/0",
		bytes.NewBufferString(`{"credential":"sk-two","credential_type":"api_key"}`),
	)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "账号凭证已存在")
	updated, err := model.GetChannelById(35, true)
	require.NoError(t, err)
	require.Equal(t, "sk-one\nsk-two", updated.Key)
}

func TestUpdateChannelAccountCredentialRejectsInvalidJSONCredentialType(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     36,
		Name:   "credential invalid json",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-old",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts/:credential_index", UpdateChannelAccountCredential)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/channel/36/accounts/0",
		bytes.NewBufferString(`{"credential":"not-json","credential_type":"oauth_account"}`),
	)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "JSON 对象")
	updated, err := model.GetChannelById(36, true)
	require.NoError(t, err)
	require.Equal(t, "sk-old", updated.Key)
}

func TestUpdateChannelAccountsStatusBatchDisablesAndDeduplicatesIndexes(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     12,
		Name:   "batch multi",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two\nsk-three",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 12,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/accounts", UpdateChannelAccountsStatus)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"enabled":false,"credential_indexes":[2,0,2],"reason":"batch manual"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/12/accounts", body)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "status", payload.Data.Operation.Type)
	require.Equal(t, "disable", payload.Data.Operation.Action)
	require.Equal(t, 3, payload.Data.Operation.Requested)
	require.Equal(t, 2, payload.Data.Operation.Affected)
	require.Equal(t, 1, payload.Data.Enabled)
	require.Equal(t, 2, payload.Data.Disabled)
	itemsByIndex := map[int]ChannelAccountItem{}
	for _, item := range payload.Data.Items {
		itemsByIndex[item.CredentialIndex] = item
	}
	require.False(t, itemsByIndex[0].KeyEnabled)
	require.True(t, itemsByIndex[1].KeyEnabled)
	require.False(t, itemsByIndex[2].KeyEnabled)

	updated, err := model.GetChannelById(12, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[2])
	require.Equal(t, "batch manual", updated.ChannelInfo.MultiKeyDisabledReason[0])
	require.Equal(t, "batch manual", updated.ChannelInfo.MultiKeyDisabledReason[2])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 12).Error)
	require.True(t, ability.Enabled)
	require.NotContains(t, recorder.Body.String(), "sk-one")
	require.NotContains(t, recorder.Body.String(), "sk-two")
	require.NotContains(t, recorder.Body.String(), "sk-three")
}

func TestUpdateChannelAccountsStatusBatchAllDisabledAutoDisablesAndRestores(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     13,
		Name:   "batch all",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 13,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/accounts", UpdateChannelAccountsStatus)

	disableRecorder := httptest.NewRecorder()
	disableReq := httptest.NewRequest(http.MethodPost, "/api/channel/13/accounts", bytes.NewBufferString(`{"action":"disable","credential_indexes":[0,1]}`))
	router.ServeHTTP(disableRecorder, disableReq)
	disablePayload := decodeChannelAccountsResponse(t, disableRecorder)
	require.True(t, disablePayload.Success, disableRecorder.Body.String())
	require.NotNil(t, disablePayload.Data.Operation)
	require.Equal(t, "status", disablePayload.Data.Operation.Type)
	require.Equal(t, "disable", disablePayload.Data.Operation.Action)
	require.Equal(t, 2, disablePayload.Data.Operation.Affected)
	require.True(t, disablePayload.Data.Operation.ChannelDisabled)
	require.Equal(t, 0, disablePayload.Data.Enabled)
	require.Equal(t, 2, disablePayload.Data.Disabled)

	disabledChannel, err := model.GetChannelById(13, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, disabledChannel.Status)
	require.Equal(t, channelAccountAllKeysDisabledReason, disabledChannel.GetOtherInfo()["status_reason"])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 13).Error)
	require.False(t, ability.Enabled)

	enableRecorder := httptest.NewRecorder()
	enableReq := httptest.NewRequest(http.MethodPost, "/api/channel/13/accounts", bytes.NewBufferString(`{"action":"enable","credential_indexes":[1]}`))
	router.ServeHTTP(enableRecorder, enableReq)
	enablePayload := decodeChannelAccountsResponse(t, enableRecorder)
	require.True(t, enablePayload.Success, enableRecorder.Body.String())
	require.NotNil(t, enablePayload.Data.Operation)
	require.Equal(t, "enable", enablePayload.Data.Operation.Action)
	require.Equal(t, 1, enablePayload.Data.Operation.Affected)
	require.True(t, enablePayload.Data.Operation.ChannelRestored)
	require.Equal(t, 1, enablePayload.Data.Enabled)
	require.Equal(t, 1, enablePayload.Data.Disabled)

	enabledChannel, err := model.GetChannelById(13, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, enabledChannel.Status)
	require.Empty(t, enabledChannel.GetOtherInfo()["status_reason"])
	require.NoError(t, db.First(&ability, "channel_id = ?", 13).Error)
	require.True(t, ability.Enabled)
}

func TestImportChannelAccountsAppendsOnlyNewAndKeepsAllDisabledChannel(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     14,
		Name:   "import multi",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one",
		Status: common.ChannelStatusAutoDisabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusManuallyDisabled,
			},
			MultiKeyDisabledReason: map[int]string{0: "manual"},
			MultiKeyDisabledTime:   map[int]int64{0: 1700000000},
		},
	}
	channel.SetOtherInfo(map[string]interface{}{"status_reason": channelAccountAllKeysDisabledReason})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 14,
		Enabled:   false,
	}).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"credentials":"sk-one\nsk-two\nsk-two\nsk-three","only_new":true}`)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/14/accounts", body)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "import", payload.Data.Operation.Type)
	require.Equal(t, 4, payload.Data.Operation.TotalInput)
	require.Equal(t, 2, payload.Data.Operation.Added)
	require.Equal(t, 2, payload.Data.Operation.Skipped)
	require.Equal(t, 1, payload.Data.Operation.SkippedExisting)
	require.Equal(t, 1, payload.Data.Operation.SkippedDuplicate)
	require.False(t, payload.Data.Operation.ChannelRestored)
	require.Equal(t, 3, payload.Data.Total)
	require.Equal(t, 0, payload.Data.Enabled)
	require.Equal(t, 3, payload.Data.Disabled)
	require.NotContains(t, recorder.Body.String(), "sk-one")
	require.NotContains(t, recorder.Body.String(), "sk-two")
	require.NotContains(t, recorder.Body.String(), "sk-three")

	updated, err := model.GetChannelById(14, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.Status)
	require.True(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, 3, updated.ChannelInfo.MultiKeySize)
	require.Equal(t, constant.MultiKeyModeRandom, updated.ChannelInfo.MultiKeyMode)
	require.Equal(t, "sk-one\nsk-two\nsk-three", updated.Key)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[2])
	require.Equal(t, channelAccountAllKeysDisabledReason, updated.GetOtherInfo()["status_reason"])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 14).Error)
	require.False(t, ability.Enabled)
}

func TestImportChannelAccountsDisablesSingleImportedAccount(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     140,
		Name:   "import single disabled",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 140,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channel/140/accounts", bytes.NewBufferString(`{"credentials":"sk-new","only_new":true}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "import", payload.Data.Operation.Type)
	require.Equal(t, 1, payload.Data.Operation.Added)
	require.False(t, payload.Data.Operation.ChannelRestored)
	require.Equal(t, 1, payload.Data.Total)
	require.Equal(t, 0, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)

	updated, err := model.GetChannelById(140, true)
	require.NoError(t, err)
	require.Equal(t, "sk-new", updated.Key)
	require.False(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.Status)
	require.Equal(t, channelAccountManualDisabledReason, updated.GetOtherInfo()["status_reason"])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 140).Error)
	require.False(t, ability.Enabled)
}

func TestImportChannelAccountsRejectsDuplicateWithoutOnlyNew(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     15,
		Name:   "duplicate import",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"credentials":"sk-one\nsk-two","only_new":false}`)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/15/accounts", body)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "账号凭证已存在")

	updated, err := model.GetChannelById(15, true)
	require.NoError(t, err)
	require.Equal(t, "sk-one", updated.Key)
	require.False(t, updated.ChannelInfo.IsMultiKey)
}

func TestImportChannelAccountsKeepsFormattedJSONCredentialTogether(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     18,
		Name:   "json import",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	jsonCredential := "{\n  \"account_id\": \"acct-json\",\n  \"refresh_token\": \"rt-json\"\n}"
	bodyBytes, err := common.Marshal(map[string]interface{}{
		"credentials": jsonCredential,
		"only_new":    true,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/18/accounts", bytes.NewReader(bodyBytes))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 1, payload.Data.Operation.Added)
	require.Equal(t, 2, payload.Data.Total)

	updated, err := model.GetChannelById(18, true)
	require.NoError(t, err)
	keys := updated.GetKeys()
	require.Len(t, keys, 2)
	require.Equal(t, "sk-one", keys[0])
	require.JSONEq(t, jsonCredential, keys[1])
	require.NotContains(t, keys[1], "\n")
}

func TestImportChannelAccountsExpandsJSONCredentialArray(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     19,
		Name:   "json array import",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	bodyBytes, err := common.Marshal(map[string]interface{}{
		"credentials": `["sk-two", {"account_id": "acct-json", "refresh_token": "rt-json"}]`,
		"only_new":    true,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/19/accounts", bytes.NewReader(bodyBytes))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 2, payload.Data.Operation.Added)
	require.Equal(t, 3, payload.Data.Total)

	updated, err := model.GetChannelById(19, true)
	require.NoError(t, err)
	keys := updated.GetKeys()
	require.Len(t, keys, 3)
	require.Equal(t, "sk-two", keys[1])
	require.JSONEq(t, `{"account_id":"acct-json","refresh_token":"rt-json"}`, keys[2])
}

func TestImportChannelAccountsAcceptsCardCredentialExportLine(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     74,
		Name:   "card export import",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.5",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: 74,
		Enabled:   true,
	}).Error)

	line := "User@Example.com----pass-123----acct-card----refresh-token"
	bodyBytes, err := common.Marshal(map[string]interface{}{
		"credentials": line,
		"only_new":    true,
	})
	require.NoError(t, err)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channel/74/accounts", bytes.NewReader(bodyBytes))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 1, payload.Data.Operation.Added)
	require.Equal(t, 1, payload.Data.Total)
	require.Equal(t, 0, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)

	updated, err := model.GetChannelById(74, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.Status)
	keys := updated.GetKeys()
	require.Len(t, keys, 1)
	require.JSONEq(t, `{"account_id":"acct-card","chatgpt_account_id":"acct-card","email":"user@example.com","password":"pass-123","refresh_token":"refresh-token","type":"codex"}`, keys[0])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 74).Error)
	require.False(t, ability.Enabled)
}

func TestImportChannelAccountsAcceptsSub2APIAccountExport(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     72,
		Name:   "sub2api import",
		Type:   constant.ChannelTypeCodex,
		Key:    "",
		Status: common.ChannelStatusAutoDisabled,
		Models: "gpt-5",
		Group:  "default",
	}
	channel.SetOtherInfo(map[string]interface{}{"status_reason": channelAccountAllKeysDisabledReason})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5",
		ChannelId: 72,
		Enabled:   false,
	}).Error)

	payload := map[string]interface{}{
		"type":        "sub2api-data",
		"version":     1,
		"exported_at": "2026-05-29T12:00:46.820Z",
		"proxies":     []interface{}{},
		"accounts": []interface{}{
			map[string]interface{}{
				"name":     "first@example.com",
				"platform": "openai",
				"type":     "oauth",
				"credentials": map[string]interface{}{
					"access_token":       "access-one",
					"refresh_token":      "refresh-one",
					"id_token":           "id-one",
					"email":              "first@example.com",
					"account_id":         "acct-one",
					"chatgpt_account_id": "acct-one",
					"chatgpt_user_id":    "user-one",
					"expires_at":         "2026-06-08T11:51:36Z",
				},
				"concurrency":           10,
				"priority":              1,
				"rate_multiplier":       1,
				"auto_pause_on_expired": true,
			},
			map[string]interface{}{
				"name":     "second@example.com",
				"platform": "openai",
				"type":     "oauth",
				"credentials": map[string]interface{}{
					"access_token":       "access-two",
					"refresh_token":      "refresh-two",
					"id_token":           "id-two",
					"email":              "second@example.com",
					"account_id":         "acct-two",
					"chatgpt_account_id": "acct-two",
					"chatgpt_user_id":    "user-two",
					"expires_at":         "2026-06-08T11:53:12Z",
				},
				"concurrency":           10,
				"priority":              1,
				"rate_multiplier":       1,
				"auto_pause_on_expired": true,
			},
		},
	}
	sub2apiBytes, err := common.Marshal(payload)
	require.NoError(t, err)
	bodyBytes, err := common.Marshal(map[string]interface{}{
		"credentials": string(sub2apiBytes),
		"only_new":    true,
	})
	require.NoError(t, err)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channel/72/accounts", bytes.NewReader(bodyBytes))
	router.ServeHTTP(recorder, req)

	response := decodeChannelAccountsResponse(t, recorder)
	require.True(t, response.Success, recorder.Body.String())
	require.Equal(t, 2, response.Data.Operation.Added)
	require.Equal(t, 2, response.Data.Total)
	require.Equal(t, 0, response.Data.Enabled)
	require.Equal(t, 2, response.Data.Disabled)
	require.False(t, response.Data.Operation.ChannelRestored)
	require.NotContains(t, recorder.Body.String(), "access-one")
	require.NotContains(t, recorder.Body.String(), "refresh-two")

	updated, err := model.GetChannelById(72, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.Status)
	require.Equal(t, channelAccountAllKeysDisabledReason, updated.GetOtherInfo()["status_reason"])
	keys := updated.GetKeys()
	require.Len(t, keys, 2)
	require.JSONEq(t, `{"access_token":"access-one","refresh_token":"refresh-one","id_token":"id-one","email":"first@example.com","account_id":"acct-one","chatgpt_account_id":"acct-one","chatgpt_user_id":"user-one","expires_at":"2026-06-08T11:51:36Z"}`, keys[0])
	require.JSONEq(t, `{"access_token":"access-two","refresh_token":"refresh-two","id_token":"id-two","email":"second@example.com","account_id":"acct-two","chatgpt_account_id":"acct-two","chatgpt_user_id":"user-two","expires_at":"2026-06-08T11:53:12Z"}`, keys[1])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])

	fileChannel := model.Channel{
		Id:     73,
		Name:   "sub2api file import",
		Type:   constant.ChannelTypeCodex,
		Key:    "",
		Status: common.ChannelStatusAutoDisabled,
		Models: "gpt-5",
		Group:  "default",
	}
	fileChannel.SetOtherInfo(map[string]interface{}{"status_reason": channelAccountAllKeysDisabledReason})
	require.NoError(t, db.Create(&fileChannel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5",
		ChannelId: 73,
		Enabled:   false,
	}).Error)

	fileBody, contentType := buildChannelAccountImportMultipart(t, "sub2api-export.json", sub2apiBytes, true)
	fileRecorder := httptest.NewRecorder()
	fileReq := httptest.NewRequest(http.MethodPut, "/api/channel/73/accounts", fileBody)
	fileReq.Header.Set("Content-Type", contentType)
	router.ServeHTTP(fileRecorder, fileReq)

	fileResponse := decodeChannelAccountsResponse(t, fileRecorder)
	require.True(t, fileResponse.Success, fileRecorder.Body.String())
	require.Equal(t, 2, fileResponse.Data.Operation.Added)
	require.Equal(t, 2, fileResponse.Data.Total)
	require.Equal(t, 0, fileResponse.Data.Enabled)
	require.Equal(t, 2, fileResponse.Data.Disabled)
	require.False(t, fileResponse.Data.Operation.ChannelRestored)
	updatedFromFile, err := model.GetChannelById(73, true)
	require.NoError(t, err)
	require.Len(t, updatedFromFile.GetKeys(), 2)
	require.Equal(t, common.ChannelStatusAutoDisabled, updatedFromFile.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updatedFromFile.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updatedFromFile.ChannelInfo.MultiKeyStatusList[1])
}

func TestImportChannelAccountsRefreshesAccountCandidateIndex(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     20,
		Name:   "index refresh",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps)
	require.NotNil(t, runtimeDeps.AccountCandidateIndex)
	before := runtimeDeps.AccountCandidateIndex.Index().Stats()
	require.Equal(t, 1, before.Candidates)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channel/20/accounts", bytes.NewBufferString(`{"credentials":"sk-two","only_new":true}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	after := runtimeDeps.AccountCandidateIndex.Index().Stats()
	require.Equal(t, 1, after.Candidates)
}

func TestImportChannelAccountsAcceptsXAutoNewAPIZip(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     71,
		Name:   "xauto codex pool",
		Type:   constant.ChannelTypeCodex,
		Key:    "",
		Status: common.ChannelStatusAutoDisabled,
		Models: "gpt-5",
		Group:  "default",
	}
	channel.SetOtherInfo(map[string]interface{}{"status_reason": channelAccountAllKeysDisabledReason})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5",
		ChannelId: 71,
		Enabled:   false,
	}).Error)

	archiveBytes := buildXAutoNewAPIArchive(t, []string{
		`{"access_token":"access-one","refresh_token":"refresh-one","account_id":"acct-one"}`,
		`{"access_token":"access-two","refresh_token":"refresh-two","account_id":"acct-two"}`,
		`{"access_token":"access-two","refresh_token":"refresh-two","account_id":"acct-two"}`,
	})
	body, contentType := buildChannelAccountImportMultipart(t, "xauto-package-newapi.zip", archiveBytes, true)

	router := gin.New()
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channel/71/accounts", body)
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "import", payload.Data.Operation.Type)
	require.Equal(t, 3, payload.Data.Operation.TotalInput)
	require.Equal(t, 2, payload.Data.Operation.Added)
	require.Equal(t, 1, payload.Data.Operation.SkippedDuplicate)
	require.False(t, payload.Data.Operation.ChannelRestored)
	require.Equal(t, 2, payload.Data.Total)
	require.Equal(t, 0, payload.Data.Enabled)
	require.Equal(t, 2, payload.Data.Disabled)
	require.NotContains(t, recorder.Body.String(), "access-one")
	require.NotContains(t, recorder.Body.String(), "refresh-one")

	updated, err := model.GetChannelById(71, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.Status)
	require.True(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, 2, updated.ChannelInfo.MultiKeySize)
	keys := updated.GetKeys()
	require.Len(t, keys, 2)
	require.JSONEq(t, `{"access_token":"access-one","refresh_token":"refresh-one","account_id":"acct-one"}`, keys[0])
	require.JSONEq(t, `{"access_token":"access-two","refresh_token":"refresh-two","account_id":"acct-two"}`, keys[1])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.Equal(t, channelAccountAllKeysDisabledReason, updated.GetOtherInfo()["status_reason"])

	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 71).Error)
	require.False(t, ability.Enabled)
}

func TestDeleteChannelAccountsReindexesStatusAndKeepsRawKeysHidden(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     16,
		Name:   "delete multi",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two\nsk-three\nsk-four",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         4,
			MultiKeyPollingIndex: 3,
			MultiKeyStatusList: map[int]int{
				2: common.ChannelStatusManuallyDisabled,
				3: common.ChannelStatusAutoDisabled,
			},
			MultiKeyDisabledReason: map[int]string{
				2: "manual",
				3: "auto",
			},
			MultiKeyDisabledTime: map[int]int64{
				2: 1700000002,
				3: 1700000003,
			},
			MultiKeyProxyIDs: map[int]int{
				0: 91,
				2: 92,
				3: 93,
			},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 16,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.DELETE("/api/channel/:id/accounts", DeleteChannelAccounts)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"credential_indexes":[1,3]}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/channel/16/accounts", body)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "delete", payload.Data.Operation.Type)
	require.Equal(t, 2, payload.Data.Operation.Requested)
	require.Equal(t, 2, payload.Data.Operation.Deleted)
	require.Equal(t, 2, payload.Data.Total)
	require.Equal(t, 1, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)
	require.NotContains(t, recorder.Body.String(), "sk-one")
	require.NotContains(t, recorder.Body.String(), "sk-two")
	require.NotContains(t, recorder.Body.String(), "sk-three")
	require.NotContains(t, recorder.Body.String(), "sk-four")

	updated, err := model.GetChannelById(16, true)
	require.NoError(t, err)
	require.Equal(t, "sk-one\nsk-three", updated.Key)
	require.True(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, 2, updated.ChannelInfo.MultiKeySize)
	require.Equal(t, 0, updated.ChannelInfo.MultiKeyPollingIndex)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[1])
	require.Equal(t, "manual", updated.ChannelInfo.MultiKeyDisabledReason[1])
	require.Equal(t, int64(1700000002), updated.ChannelInfo.MultiKeyDisabledTime[1])
	require.Equal(t, 91, updated.ChannelInfo.MultiKeyProxyIDs[0])
	require.Equal(t, 92, updated.ChannelInfo.MultiKeyProxyIDs[1])
	_, deletedProxyBinding := updated.ChannelInfo.MultiKeyProxyIDs[2]
	require.False(t, deletedProxyBinding)
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 16).Error)
	require.True(t, ability.Enabled)
}

func TestDeleteChannelAccountsReindexesAccountTypes(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     37,
		Name:   "delete account types",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two\nsk-three",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         3,
			MultiKeyAccountTypes: map[int]string{0: modelgatewaycore.AccountTypeAPIKey, 2: modelgatewaycore.AccountTypeOAuthAccount},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 37,
		Enabled:   true,
	}).Error)

	router := gin.New()
	router.DELETE("/api/channel/:id/accounts", DeleteChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/channel/37/accounts", bytes.NewBufferString(`{"credential_indexes":[1]}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	updated, err := model.GetChannelById(37, true)
	require.NoError(t, err)
	require.Equal(t, map[int]string{
		0: modelgatewaycore.AccountTypeAPIKey,
		1: modelgatewaycore.AccountTypeOAuthAccount,
	}, updated.ChannelInfo.MultiKeyAccountTypes)
}

func TestUpdateChannelAccountProxyBindsAndClearsProxy(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     21,
		Name:   "proxy bind",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       501,
		Name:     "socks exit",
		Protocol: "socks5",
		Address:  "user:pass@127.0.0.1:1080",
		Enabled:  true,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/proxy", UpdateChannelAccountProxy)
	bindRecorder := httptest.NewRecorder()
	bindReq := httptest.NewRequest(http.MethodPost, "/api/channel/21/accounts/1/proxy", bytes.NewBufferString(`{"proxy_id":501}`))
	router.ServeHTTP(bindRecorder, bindReq)

	bindPayload := decodeChannelAccountsResponse(t, bindRecorder)
	require.True(t, bindPayload.Success, bindRecorder.Body.String())
	require.NotNil(t, bindPayload.Data.Operation)
	require.Equal(t, "proxy", bindPayload.Data.Operation.Type)
	require.Equal(t, "bind", bindPayload.Data.Operation.Action)
	require.Nil(t, bindPayload.Data.Items[0].Proxy)
	require.NotNil(t, bindPayload.Data.Items[1].Proxy)
	require.Equal(t, 501, bindPayload.Data.Items[1].Proxy.ID)
	require.Equal(t, "socks5://127.0.0.1:1080", bindPayload.Data.Items[1].Proxy.MaskedAddress)
	require.Nil(t, bindPayload.Data.Items[1].Proxy.ReuseRisks)
	require.NotContains(t, bindRecorder.Body.String(), "user:pass")
	require.NotContains(t, bindRecorder.Body.String(), "sk-one")
	require.NotContains(t, bindRecorder.Body.String(), "sk-two")

	updated, err := model.GetChannelById(21, true)
	require.NoError(t, err)
	require.Equal(t, 501, updated.ChannelInfo.MultiKeyProxyIDs[1])
	var usage model.ModelGatewayProxyUsage
	require.NoError(t, db.First(&usage, "proxy_id = ? AND channel_id = ?", 501, 21).Error)
	require.Equal(t, "openai", usage.Brand)
	require.Equal(t, model.ModelGatewayProxyUsageStatusBound, usage.LastStatus)

	clearRecorder := httptest.NewRecorder()
	clearReq := httptest.NewRequest(http.MethodPost, "/api/channel/21/accounts/1/proxy", bytes.NewBufferString(`{"proxy_id":0}`))
	router.ServeHTTP(clearRecorder, clearReq)
	clearPayload := decodeChannelAccountsResponse(t, clearRecorder)
	require.True(t, clearPayload.Success, clearRecorder.Body.String())
	require.Equal(t, "clear", clearPayload.Data.Operation.Action)
	require.Nil(t, clearPayload.Data.Items[1].Proxy)

	updated, err = model.GetChannelById(21, true)
	require.NoError(t, err)
	require.Nil(t, updated.ChannelInfo.MultiKeyProxyIDs)
}

func TestUpdateChannelAccountsProxyBindsAndClearsSelectedAccounts(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     23,
		Name:   "proxy batch bind",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two\nsk-three",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       502,
		Name:     "batch socks exit",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/account-proxies", UpdateChannelAccountsProxy)
	bindRecorder := httptest.NewRecorder()
	bindReq := httptest.NewRequest(http.MethodPost, "/api/channel/23/account-proxies", bytes.NewBufferString(`{"proxy_id":502,"credential_indexes":[2,0,2]}`))
	router.ServeHTTP(bindRecorder, bindReq)

	bindPayload := decodeChannelAccountsResponse(t, bindRecorder)
	require.True(t, bindPayload.Success, bindRecorder.Body.String())
	require.NotNil(t, bindPayload.Data.Operation)
	require.Equal(t, "proxy", bindPayload.Data.Operation.Type)
	require.Equal(t, "bind", bindPayload.Data.Operation.Action)
	require.Equal(t, 2, bindPayload.Data.Operation.Requested)
	require.Equal(t, 2, bindPayload.Data.Operation.Affected)
	require.NotNil(t, bindPayload.Data.Items[0].Proxy)
	require.Nil(t, bindPayload.Data.Items[1].Proxy)
	require.NotNil(t, bindPayload.Data.Items[2].Proxy)
	require.Equal(t, 502, bindPayload.Data.Items[0].Proxy.ID)
	require.Equal(t, 502, bindPayload.Data.Items[2].Proxy.ID)

	updated, err := model.GetChannelById(23, true)
	require.NoError(t, err)
	require.Equal(t, map[int]int{0: 502, 2: 502}, updated.ChannelInfo.MultiKeyProxyIDs)

	var usageCount int64
	require.NoError(t, db.Model(&model.ModelGatewayProxyUsage{}).Where("proxy_id = ? AND channel_id = ?", 502, 23).Count(&usageCount).Error)
	require.Equal(t, int64(2), usageCount)

	clearRecorder := httptest.NewRecorder()
	clearReq := httptest.NewRequest(http.MethodPost, "/api/channel/23/account-proxies", bytes.NewBufferString(`{"proxy_id":0,"credential_indexes":[0,2]}`))
	router.ServeHTTP(clearRecorder, clearReq)
	clearPayload := decodeChannelAccountsResponse(t, clearRecorder)
	require.True(t, clearPayload.Success, clearRecorder.Body.String())
	require.Equal(t, "clear", clearPayload.Data.Operation.Action)
	require.Equal(t, 2, clearPayload.Data.Operation.Affected)
	require.Nil(t, clearPayload.Data.Items[0].Proxy)
	require.Nil(t, clearPayload.Data.Items[2].Proxy)

	updated, err = model.GetChannelById(23, true)
	require.NoError(t, err)
	require.Nil(t, updated.ChannelInfo.MultiKeyProxyIDs)
}

func TestUpdateChannelAccountsProxyConfirmPolicyRequiresAllowReuseRisk(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     24,
		Name:   "proxy batch confirm",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       503,
		Name:     "batch confirm socks",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	setting := scheduler_setting.DefaultSetting()
	setting.ProxySameBrandReusePolicy = scheduler_setting.ProxyReusePolicyConfirm
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/channel/:id/account-proxies", UpdateChannelAccountsProxy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/24/account-proxies", bytes.NewBufferString(`{"proxy_id":503,"credential_indexes":[0,1]}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "请确认后继续绑定")
	updated, err := model.GetChannelById(24, true)
	require.NoError(t, err)
	require.Nil(t, updated.ChannelInfo.MultiKeyProxyIDs)

	confirmedRecorder := httptest.NewRecorder()
	confirmedReq := httptest.NewRequest(http.MethodPost, "/api/channel/24/account-proxies", bytes.NewBufferString(`{"proxy_id":503,"credential_indexes":[0,1],"allow_reuse_risk":true}`))
	router.ServeHTTP(confirmedRecorder, confirmedReq)
	confirmedPayload := decodeChannelAccountsResponse(t, confirmedRecorder)
	require.True(t, confirmedPayload.Success, confirmedRecorder.Body.String())
	require.Equal(t, "bind", confirmedPayload.Data.Operation.Action)

	updated, err = model.GetChannelById(24, true)
	require.NoError(t, err)
	require.Equal(t, map[int]int{0: 503, 1: 503}, updated.ChannelInfo.MultiKeyProxyIDs)
}

func TestListChannelAccountsIncludesProxyReuseRisk(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     22,
		Name:   "proxy risk account",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyProxyIDs:   map[int]int{0: 502, 1: 502},
			MultiKeyStatusList: map[int]int{},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       502,
		Name:     "shared socks",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 2)
	for _, account := range accounts {
		require.NoError(t, model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
			ProxyID:                      502,
			ChannelID:                    channel.Id,
			ResourceID:                   account.ResourceRef.ResourceID,
			ResourceType:                 account.ResourceRef.ResourceType,
			AccountID:                    account.AccountIdentity.AccountID,
			AccountType:                  account.AccountIdentity.AccountType,
			Brand:                        account.AccountIdentity.Brand,
			Provider:                     account.AccountIdentity.Provider,
			CredentialIndex:              account.CredentialIndex,
			CredentialSubjectFingerprint: account.AccountIdentity.CredentialSubjectFingerprint,
			CredentialFingerprint:        account.AccountIdentity.CredentialFingerprint,
			LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
		}))
	}

	router := gin.New()
	router.GET("/api/channel/:id/accounts", ListChannelAccounts)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/22/accounts", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.Items, 2)
	require.NotNil(t, payload.Data.Items[0].Proxy)
	require.Len(t, payload.Data.Items[0].Proxy.ReuseRisks, 1)
	require.Equal(t, "openai", payload.Data.Items[0].Proxy.ReuseRisks[0].Brand)
	require.Equal(t, 2, payload.Data.Items[0].Proxy.ReuseRisks[0].DistinctSubjectCount)
}

func TestUpdateChannelAccountProxyWarnPolicyAllowsSameBrandReuse(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	createProxyReusePolicyFixture(t, db)

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/proxy", UpdateChannelAccountProxy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/31/accounts/0/proxy", bytes.NewBufferString(`{"proxy_id":601}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "bind", payload.Data.Operation.Action)
	updated, err := model.GetChannelById(31, true)
	require.NoError(t, err)
	require.Equal(t, 601, updated.ChannelInfo.MultiKeyProxyIDs[0])
}

func TestUpdateChannelAccountProxyConfirmPolicyRequiresAllowReuseRisk(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	createProxyReusePolicyFixture(t, db)
	setting := scheduler_setting.DefaultSetting()
	setting.ProxySameBrandReusePolicy = scheduler_setting.ProxyReusePolicyConfirm
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/proxy", UpdateChannelAccountProxy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/31/accounts/0/proxy", bytes.NewBufferString(`{"proxy_id":601}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "请确认后继续绑定")
	updated, err := model.GetChannelById(31, true)
	require.NoError(t, err)
	require.Nil(t, updated.ChannelInfo.MultiKeyProxyIDs)
}

func TestUpdateChannelAccountProxyConfirmPolicyAllowsConfirmedReuseRisk(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	createProxyReusePolicyFixture(t, db)
	setting := scheduler_setting.DefaultSetting()
	setting.ProxySameBrandReusePolicy = scheduler_setting.ProxyReusePolicyConfirm
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/proxy", UpdateChannelAccountProxy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/31/accounts/0/proxy", bytes.NewBufferString(`{"proxy_id":601,"allow_reuse_risk":true}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "bind", payload.Data.Operation.Action)
	updated, err := model.GetChannelById(31, true)
	require.NoError(t, err)
	require.Equal(t, 601, updated.ChannelInfo.MultiKeyProxyIDs[0])
}

func TestUpdateChannelAccountProxyBlockPolicyRejectsSameBrandReuse(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	createProxyReusePolicyFixture(t, db)
	setting := scheduler_setting.DefaultSetting()
	setting.ProxySameBrandReusePolicy = scheduler_setting.ProxyReusePolicyBlock
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/proxy", UpdateChannelAccountProxy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/31/accounts/0/proxy", bytes.NewBufferString(`{"proxy_id":601,"allow_reuse_risk":true}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "请选择其它代理")
	updated, err := model.GetChannelById(31, true)
	require.NoError(t, err)
	require.Nil(t, updated.ChannelInfo.MultiKeyProxyIDs)
}

func TestUpdateChannelAccountProxyPolicyAllowsSameSubjectAndDifferentBrand(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     32,
		Name:   "policy same subject",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       602,
		Name:     "shared subject socks",
		Protocol: "socks5",
		Address:  "127.0.0.1:1081",
		Enabled:  true,
	}).Error)
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&channel)
	require.Len(t, accounts, 1)
	require.NoError(t, model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
		ProxyID:                      602,
		ChannelID:                    99,
		AccountID:                    accounts[0].AccountIdentity.AccountID,
		AccountType:                  accounts[0].AccountIdentity.AccountType,
		Brand:                        accounts[0].AccountIdentity.Brand,
		Provider:                     accounts[0].AccountIdentity.Provider,
		CredentialSubjectFingerprint: accounts[0].AccountIdentity.CredentialSubjectFingerprint,
		CredentialFingerprint:        accounts[0].AccountIdentity.CredentialFingerprint,
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
	}))
	require.NoError(t, model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
		ProxyID:                      602,
		ChannelID:                    100,
		AccountID:                    "anthropic:claude:subject",
		AccountType:                  modelgatewaycore.AccountTypeAPIKey,
		Brand:                        "claude",
		Provider:                     "anthropic",
		CredentialSubjectFingerprint: "other-brand-subject",
		CredentialFingerprint:        "other-brand-credential",
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
	}))
	setting := scheduler_setting.DefaultSetting()
	setting.ProxySameBrandReusePolicy = scheduler_setting.ProxyReusePolicyBlock
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/channel/:id/accounts/:credential_index/proxy", UpdateChannelAccountProxy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/32/accounts/0/proxy", bytes.NewBufferString(`{"proxy_id":602}`))
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "bind", payload.Data.Operation.Action)
	updated, err := model.GetChannelById(32, true)
	require.NoError(t, err)
	require.Equal(t, 602, updated.ChannelInfo.MultiKeyProxyIDs[0])
}

func TestDeleteChannelAccountsAllowsDeletingAllAccountsAndAutoDisablesChannel(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	channel := model.Channel{
		Id:     17,
		Name:   "delete all",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyProxyIDs:     map[int]int{0: 701, 1: 702},
			MultiKeyAccountTypes: map[int]string{0: modelgatewaycore.AccountTypeAPIKey, 1: modelgatewaycore.AccountTypeOAuthAccount},
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: 17,
		Enabled:   true,
	}).Error)
	require.NoError(t, model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
		ProxyID:         701,
		ChannelID:       17,
		ResourceID:      "openai",
		ResourceType:    "channel",
		AccountID:       "sk-one",
		AccountType:     modelgatewaycore.AccountTypeAPIKey,
		Brand:           "openai",
		Provider:        "openai",
		CredentialIndex: 0,
		LastStatus:      model.ModelGatewayProxyUsageStatusBound,
	}))
	require.NoError(t, model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
		ProxyID:         702,
		ChannelID:       17,
		ResourceID:      "openai",
		ResourceType:    "channel",
		AccountID:       "sk-two",
		AccountType:     modelgatewaycore.AccountTypeOAuthAccount,
		Brand:           "openai",
		Provider:        "openai",
		CredentialIndex: 1,
		LastStatus:      model.ModelGatewayProxyUsageStatusBound,
	}))

	router := gin.New()
	router.DELETE("/api/channel/:id/accounts", DeleteChannelAccounts)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"credential_indexes":[0,1]}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/channel/17/accounts", body)
	router.ServeHTTP(recorder, req)

	payload := decodeChannelAccountsResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.NotNil(t, payload.Data.Operation)
	require.Equal(t, "delete", payload.Data.Operation.Type)
	require.Equal(t, 2, payload.Data.Operation.Deleted)
	require.True(t, payload.Data.Operation.ChannelDisabled)
	require.Equal(t, 0, payload.Data.Total)
	require.Empty(t, payload.Data.Items)

	updated, err := model.GetChannelById(17, true)
	require.NoError(t, err)
	require.Empty(t, updated.Key)
	require.Equal(t, common.ChannelStatusAutoDisabled, updated.Status)
	require.Equal(t, channelAccountAllKeysDisabledReason, updated.GetOtherInfo()["status_reason"])
	require.False(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, 0, updated.ChannelInfo.MultiKeySize)
	require.Nil(t, updated.ChannelInfo.MultiKeyStatusList)
	require.Nil(t, updated.ChannelInfo.MultiKeyProxyIDs)
	require.Nil(t, updated.ChannelInfo.MultiKeyAccountTypes)

	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 17).Error)
	require.False(t, ability.Enabled)
	var usageCount int64
	require.NoError(t, db.Model(&model.ModelGatewayProxyUsage{}).Where("channel_id = ?", 17).Count(&usageCount).Error)
	require.Equal(t, int64(0), usageCount)
}

func createProxyReusePolicyFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	channel := model.Channel{
		Id:     31,
		Name:   "policy target",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-target",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
		},
	}
	existingChannel := model.Channel{
		Id:     30,
		Name:   "policy existing",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-existing",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&existingChannel).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       601,
		Name:     "shared policy socks",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(&existingChannel)
	require.Len(t, accounts, 1)
	require.NoError(t, model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
		ProxyID:                      601,
		ChannelID:                    existingChannel.Id,
		ResourceID:                   accounts[0].ResourceRef.ResourceID,
		ResourceType:                 accounts[0].ResourceRef.ResourceType,
		AccountID:                    accounts[0].AccountIdentity.AccountID,
		AccountType:                  accounts[0].AccountIdentity.AccountType,
		Brand:                        accounts[0].AccountIdentity.Brand,
		Provider:                     accounts[0].AccountIdentity.Provider,
		CredentialIndex:              accounts[0].CredentialIndex,
		CredentialSubjectFingerprint: accounts[0].AccountIdentity.CredentialSubjectFingerprint,
		CredentialFingerprint:        accounts[0].AccountIdentity.CredentialFingerprint,
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
	}))
}

func buildXAutoNewAPIArchive(t *testing.T, credentials []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	manifest, err := common.Marshal(map[string]interface{}{
		"type":         "newapi-channel-files",
		"version":      1,
		"channel_type": constant.ChannelTypeCodex,
	})
	require.NoError(t, err)
	manifestFile, err := writer.Create("newapi/manifest.json")
	require.NoError(t, err)
	_, err = manifestFile.Write(manifest)
	require.NoError(t, err)

	for index, credential := range credentials {
		payload, err := common.Marshal(map[string]interface{}{
			"mode": "single",
			"channel": map[string]interface{}{
				"name":     "XAutoJS Codex",
				"type":     constant.ChannelTypeCodex,
				"key":      credential,
				"models":   "gpt-5",
				"group":    "default",
				"status":   common.ChannelStatusEnabled,
				"auto_ban": 1,
			},
		})
		require.NoError(t, err)
		file, err := writer.Create(fmt.Sprintf("newapi/%03d-xauto@example.com.json", index+1))
		require.NoError(t, err)
		_, err = file.Write(payload)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

func buildChannelAccountImportMultipart(t *testing.T, filename string, fileBytes []byte, onlyNew bool) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("only_new", strconv.FormatBool(onlyNew)))
	fileWriter, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = fileWriter.Write(fileBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}

func decodeChannelAccountsResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelAccountsAPIResponse {
	t.Helper()
	var payload channelAccountsAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func decodeChannelAccountRecentRequestsResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelAccountRecentRequestsAPIResponse {
	t.Helper()
	var payload channelAccountRecentRequestsAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func decodeChannelAccountRequestReconcileResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelAccountRequestReconcileAPIResponse {
	t.Helper()
	var payload channelAccountRequestReconcileAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func decodeCodexApplicationEnvironmentListResponse(t *testing.T, recorder *httptest.ResponseRecorder) codexApplicationEnvironmentListAPIResponse {
	t.Helper()
	var payload codexApplicationEnvironmentListAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func reconcileCheckStatuses(checks []ChannelAccountRequestReconcileCheck) []string {
	values := make([]string, 0, len(checks))
	for _, check := range checks {
		values = append(values, check.Key+":"+check.Status)
	}
	return values
}

func reconcileDiagnosisKeys(diagnoses []ChannelAccountRequestReconcileDiagnosis) []string {
	values := make([]string, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
		values = append(values, diagnosis.Key)
	}
	return values
}
