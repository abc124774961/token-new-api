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
		&model.ModelGatewayChannelCostProfile{},
		&model.ModelGatewayRuntimeSnapshot{},
		&model.ModelGatewayProxy{},
		&model.ModelGatewayProxyUsage{},
		&model.ChannelAccountUsageEvent{},
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
	require.NotNil(t, payload.Data.Items[0].Score)
	require.Equal(t, 9, payload.Data.Items[0].Score.SampleCount)
	require.Equal(t, "gpt-5.4", payload.Data.Items[0].Score.RuntimeKey.RequestedModel)
	require.Len(t, payload.Data.Items[0].RuntimeKeys, 1)
	require.False(t, payload.Data.Items[1].KeyEnabled)
	require.Equal(t, "auth failed", payload.Data.Items[1].DisabledReason)
	require.Nil(t, payload.Data.Items[1].Score)
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
	require.False(t, payload.Data.Items[0].KeyEnabled)
	require.True(t, payload.Data.Items[1].KeyEnabled)
	require.False(t, payload.Data.Items[2].KeyEnabled)

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

func TestImportChannelAccountsAppendsOnlyNewAndRestoresAllDisabledChannel(t *testing.T) {
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
	require.True(t, payload.Data.Operation.ChannelRestored)
	require.Equal(t, 3, payload.Data.Total)
	require.Equal(t, 2, payload.Data.Enabled)
	require.Equal(t, 1, payload.Data.Disabled)
	require.NotContains(t, recorder.Body.String(), "sk-one")
	require.NotContains(t, recorder.Body.String(), "sk-two")
	require.NotContains(t, recorder.Body.String(), "sk-three")

	updated, err := model.GetChannelById(14, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.True(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, 3, updated.ChannelInfo.MultiKeySize)
	require.Equal(t, constant.MultiKeyModeRandom, updated.ChannelInfo.MultiKeyMode)
	require.Equal(t, "sk-one\nsk-two\nsk-three", updated.Key)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.ChannelInfo.MultiKeyStatusList[0])
	require.Empty(t, updated.GetOtherInfo()["status_reason"])
	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 14).Error)
	require.True(t, ability.Enabled)
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
	require.Equal(t, 2, response.Data.Enabled)
	require.True(t, response.Data.Operation.ChannelRestored)
	require.NotContains(t, recorder.Body.String(), "access-one")
	require.NotContains(t, recorder.Body.String(), "refresh-two")

	updated, err := model.GetChannelById(72, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	keys := updated.GetKeys()
	require.Len(t, keys, 2)
	require.JSONEq(t, `{"access_token":"access-one","refresh_token":"refresh-one","id_token":"id-one","email":"first@example.com","account_id":"acct-one","chatgpt_account_id":"acct-one","chatgpt_user_id":"user-one","expires_at":"2026-06-08T11:51:36Z"}`, keys[0])
	require.JSONEq(t, `{"access_token":"access-two","refresh_token":"refresh-two","id_token":"id-two","email":"second@example.com","account_id":"acct-two","chatgpt_account_id":"acct-two","chatgpt_user_id":"user-two","expires_at":"2026-06-08T11:53:12Z"}`, keys[1])

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
	updatedFromFile, err := model.GetChannelById(73, true)
	require.NoError(t, err)
	require.Len(t, updatedFromFile.GetKeys(), 2)
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
	require.Equal(t, 2, after.Candidates)
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
	require.True(t, payload.Data.Operation.ChannelRestored)
	require.Equal(t, 2, payload.Data.Total)
	require.Equal(t, 2, payload.Data.Enabled)
	require.NotContains(t, recorder.Body.String(), "access-one")
	require.NotContains(t, recorder.Body.String(), "refresh-one")

	updated, err := model.GetChannelById(71, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.True(t, updated.ChannelInfo.IsMultiKey)
	require.Equal(t, 2, updated.ChannelInfo.MultiKeySize)
	keys := updated.GetKeys()
	require.Len(t, keys, 2)
	require.JSONEq(t, `{"access_token":"access-one","refresh_token":"refresh-one","account_id":"acct-one"}`, keys[0])
	require.JSONEq(t, `{"access_token":"access-two","refresh_token":"refresh-two","account_id":"acct-two"}`, keys[1])

	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", 71).Error)
	require.True(t, ability.Enabled)
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
