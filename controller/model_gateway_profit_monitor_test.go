package controller

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelGatewayProfitMonitorAPIResponse struct {
	Success bool                              `json:"success"`
	Message string                            `json:"message"`
	Data    ModelGatewayProfitMonitorResponse `json:"data"`
}

type modelGatewayProfitMonitorConfigAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Config ModelGatewayProfitMonitorConfig `json:"config"`
	} `json:"data"`
}

func setupModelGatewayProfitMonitorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.ChannelAccountUsageEvent{},
		&model.ModelGatewayProfitResourceCost{},
		&model.ModelGatewayProfitRatioRecommendation{},
		&model.ModelGatewayProfitCanaryTask{},
		&model.ModelGatewayTrafficMetric{},
	))

	oldDB := model.DB
	oldOptionMap := common.OptionMap
	oldQuotaPerUnit := common.QuotaPerUnit
	model.DB = db
	common.OptionMap = map[string]string{}
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		model.DB = oldDB
		common.OptionMap = oldOptionMap
		common.QuotaPerUnit = oldQuotaPerUnit
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestModelGatewayProfitMonitorConfigPatchPersistsNormalizedConfig(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	router := gin.New()
	router.PATCH("/api/model_gateway/profit_monitor/config", UpdateModelGatewayProfitMonitorConfig)

	body, err := common.Marshal(UpdateModelGatewayProfitMonitorConfigRequest{
		ServerDailyCostUSD:            floatPtr(-1),
		TargetProfitRate:              floatPtr(120),
		DynamicRatioMinLimit:          floatPtr(3),
		DynamicRatioMaxLimit:          floatPtr(2),
		DynamicRatioFixedValue:        floatPtr(101),
		TrafficEstimationEnabled:      boolPtr(true),
		TrafficEstimatedBytesPerToken: intPtr(14),
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/profit_monitor/config", bytes.NewReader(body))
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitMonitorConfigAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, 0.0, payload.Data.Config.ServerDailyCostUSD)
	require.Equal(t, 0.95, payload.Data.Config.TargetProfitRate)
	require.Equal(t, 3.0, payload.Data.Config.DynamicRatioMinLimit)
	require.Equal(t, 3.0, payload.Data.Config.DynamicRatioMaxLimit)
	require.Equal(t, 100.0, payload.Data.Config.DynamicRatioFixedValue)
	require.True(t, payload.Data.Config.TrafficEstimationEnabled)
	require.Equal(t, 14, payload.Data.Config.TrafficEstimatedBytesPerToken)

	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", modelGatewayProfitMonitorConfigOptionKey).Error)
	require.Contains(t, option.Value, `"target_profit_rate":0.95`)
	require.Contains(t, option.Value, `"dynamic_ratio_min_limit":3`)
	require.Contains(t, option.Value, `"dynamic_ratio_fixed_value":100`)
}

func TestModelGatewayProfitMonitorSummaryExcludesHealthProbeAndAddsResourceCost(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	require.NoError(t, saveModelGatewayProfitMonitorConfig(ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		ServerDailyCostUSD:             24,
		ResourceCostEnabled:            true,
		TargetProfitRate:               0.2,
		DynamicRatioRecommendationMode: "observe",
	}))
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:         "req-real",
		ChannelID:         12,
		ChannelName:       "real channel",
		RequestedModel:    "gpt-test",
		RequestedGroup:    "default",
		SelectedGroup:     "default",
		CompletedAt:       120,
		Success:           true,
		TotalTokens:       1000,
		Quota:             500000,
		UpstreamCostTotal: 0.3,
		IsHealthProbe:     false,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:         "req-probe",
		ChannelID:         12,
		ChannelName:       "probe channel",
		RequestedModel:    "gpt-test",
		RequestedGroup:    "default",
		SelectedGroup:     "default",
		CompletedAt:       130,
		Success:           true,
		TotalTokens:       1000,
		Quota:             500000,
		UpstreamCostTotal: 9,
		IsHealthProbe:     true,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:         "req-failed-real",
		ChannelID:         12,
		ChannelName:       "real channel",
		RequestedModel:    "gpt-test",
		RequestedGroup:    "default",
		SelectedGroup:     "default",
		CompletedAt:       140,
		Success:           false,
		StatusCode:        500,
		ErrorCategory:     "upstream_error",
		TotalTokens:       2000,
		Quota:             900000,
		UpstreamCostTotal: 8,
		IsHealthProbe:     false,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProfitResourceCost{
		Name:            "account batch",
		ResourceType:    model.ModelGatewayProfitResourceTypeAccountPool,
		AmountUSD:       3.6,
		PeriodSeconds:   3600,
		AmortizeStartAt: 100,
		AmortizeEndAt:   3700,
		Enabled:         true,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/profit_monitor/summary", GetModelGatewayProfitMonitorSummary)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/profit_monitor/summary?start_timestamp=100&end_timestamp=3700&breakdown_dimension=channel", nil)
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitMonitorAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, int64(2), payload.Data.Summary.Requests)
	require.Equal(t, int64(1), payload.Data.Summary.SuccessRequests)
	require.InEpsilon(t, 0.5, payload.Data.Summary.SuccessRate, 0.0001)
	require.Equal(t, int64(500000), payload.Data.Summary.BillingQuota)
	require.Equal(t, int64(1000), payload.Data.Summary.TotalTokens)
	require.InEpsilon(t, 1.0, payload.Data.Summary.RevenueUSD, 0.0001)
	require.InEpsilon(t, 0.3, payload.Data.Summary.UpstreamCostUSD, 0.0001)
	require.InEpsilon(t, 1.0, payload.Data.Summary.ServerCostUSD, 0.0001)
	require.InEpsilon(t, 3.6, payload.Data.Summary.ResourceAmortizedCostUSD, 0.0001)
	require.InEpsilon(t, 4.9, payload.Data.Summary.OperatingCostUSD, 0.0001)
	require.Len(t, payload.Data.Breakdown, 1)
	require.Equal(t, "channel", payload.Data.BreakdownDimension)
	require.Equal(t, 12, payload.Data.Breakdown[0].DimensionID)
	require.Equal(t, int64(2), payload.Data.Breakdown[0].Requests)
	require.Equal(t, int64(1), payload.Data.Breakdown[0].SuccessRequests)
	require.Equal(t, int64(500000), payload.Data.Breakdown[0].BillingQuota)
	require.InEpsilon(t, 0.3, payload.Data.Breakdown[0].UpstreamCostUSD, 0.0001)
}

func TestModelGatewayProfitMonitorSummaryPrefersRealTrafficBytes(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	require.NoError(t, saveModelGatewayProfitMonitorConfig(ModelGatewayProfitMonitorConfig{
		Enabled:                       true,
		TrafficCostPerGBUSD:           1,
		TrafficEstimationEnabled:      true,
		TrafficEstimatedBytesPerToken: 999,
		ResourceCostEnabled:           true,
		TargetProfitRate:              0.2,
	}))
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:         "req-real-traffic",
		ChannelID:         12,
		ChannelName:       "real channel",
		RequestedModel:    "gpt-test",
		RequestedGroup:    "default",
		SelectedGroup:     "default",
		CompletedAt:       120,
		Success:           true,
		TotalTokens:       1000,
		Quota:             500000,
		UpstreamCostTotal: 0.3,
		IsHealthProbe:     false,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayTrafficMetric{
		ModelName:     "gpt-test",
		Group:         "default",
		ChannelID:     12,
		BucketTs:      0,
		RequestCount:  1,
		RequestBytes:  512,
		ResponseBytes: 1536,
		TotalBytes:    2048,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/profit_monitor/summary", GetModelGatewayProfitMonitorSummary)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/profit_monitor/summary?start_timestamp=100&end_timestamp=3700&dimension=channel", nil)
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitMonitorAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.False(t, payload.Data.Summary.TrafficEstimated)
	require.True(t, payload.Data.Summary.TrafficDataReady)
	require.Equal(t, int64(512), payload.Data.Summary.TrafficRequestBytes)
	require.Equal(t, int64(1536), payload.Data.Summary.TrafficResponseBytes)
	require.Equal(t, int64(2048), payload.Data.Summary.TrafficBytes)
	require.InEpsilon(t, float64(2048)/1024/1024/1024, payload.Data.Summary.TrafficCostUSD, 0.0001)
	require.Len(t, payload.Data.Breakdown, 1)
	require.Equal(t, int64(2048), payload.Data.Breakdown[0].TrafficBytes)
	require.InEpsilon(t, payload.Data.Summary.TrafficCostUSD, payload.Data.Breakdown[0].TrafficCostUSD, 0.0001)
}

type modelGatewayProfitTrafficAPIResponse struct {
	Success bool                              `json:"success"`
	Message string                            `json:"message"`
	Data    ModelGatewayProfitTrafficResponse `json:"data"`
}

type modelGatewayProfitRecommendationAPIResponse struct {
	Success bool                                        `json:"success"`
	Message string                                      `json:"message"`
	Data    model.ModelGatewayProfitRatioRecommendation `json:"data"`
}

type modelGatewayProfitRecommendationListAPIResponse struct {
	Success bool                                          `json:"success"`
	Message string                                        `json:"message"`
	Data    []model.ModelGatewayProfitRatioRecommendation `json:"data"`
}

type modelGatewayProfitCanaryTaskAPIResponse struct {
	Success bool                               `json:"success"`
	Message string                             `json:"message"`
	Data    model.ModelGatewayProfitCanaryTask `json:"data"`
}

type modelGatewayProfitCanaryTaskListAPIResponse struct {
	Success bool                                 `json:"success"`
	Message string                               `json:"message"`
	Data    []model.ModelGatewayProfitCanaryTask `json:"data"`
}

func TestModelGatewayProfitMonitorTrafficEndpointReturnsSeriesAndBreakdown(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	require.NoError(t, saveModelGatewayProfitMonitorConfig(ModelGatewayProfitMonitorConfig{
		Enabled:             true,
		TrafficCostPerGBUSD: 2,
		ResourceCostEnabled: true,
		TargetProfitRate:    0.2,
	}))
	require.NoError(t, db.Create(&model.ModelGatewayTrafficMetric{
		ModelName:     "gpt-test",
		Group:         "default",
		ChannelID:     12,
		BucketTs:      0,
		RequestCount:  2,
		RequestBytes:  1024,
		ResponseBytes: 3072,
		TotalBytes:    4096,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayTrafficMetric{
		ModelName:     "gpt-test",
		Group:         "vip",
		ChannelID:     13,
		BucketTs:      3600,
		RequestCount:  1,
		RequestBytes:  512,
		ResponseBytes: 512,
		TotalBytes:    1024,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/profit_monitor/traffic", GetModelGatewayProfitMonitorTraffic)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/profit_monitor/traffic?start_timestamp=100&end_timestamp=3700&dimension=group", nil)
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitTrafficAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.True(t, payload.Data.Summary.DataReady)
	require.Equal(t, int64(3), payload.Data.Summary.RequestCount)
	require.Equal(t, int64(1536), payload.Data.Summary.RequestBytes)
	require.Equal(t, int64(3584), payload.Data.Summary.ResponseBytes)
	require.Equal(t, int64(5120), payload.Data.Summary.TotalBytes)
	require.InEpsilon(t, float64(5120)/1024/1024/1024*2, payload.Data.Summary.TrafficCostUSD, 0.0001)
	require.Len(t, payload.Data.Series, 2)
	require.Len(t, payload.Data.Breakdown, 2)
	require.Equal(t, "default", payload.Data.Breakdown[0].DimensionName)
	require.InEpsilon(t, 0.8, payload.Data.Breakdown[0].Share, 0.0001)
}

func TestModelGatewayProfitMonitorDynamicRatioGroupsArePerGroup(t *testing.T) {
	_ = setupModelGatewayProfitMonitorTestDB(t)
	seedProfitMonitorDynamicBillingBaselines(t, scheduler_setting.DynamicBillingApplyModeAuto)

	groups, summary := buildModelGatewayProfitDynamicRatioGroups(ModelGatewayProfitMonitorConfig{
		TargetProfitRate: 0.2,
	})
	plus := findProfitMonitorDynamicRatioGroup(t, groups, "codex-plus")
	pro := findProfitMonitorDynamicRatioGroup(t, groups, "codex-pro")

	require.GreaterOrEqual(t, summary.TotalGroups, 2)
	require.True(t, plus.Applied)
	require.True(t, pro.Applied)
	require.InEpsilon(t, 0.20, plus.ActualRatio, 0.0001)
	require.InEpsilon(t, 0.55, pro.ActualRatio, 0.0001)
	require.NotEqual(t, plus.TargetRatio, pro.TargetRatio)
	require.Greater(t, pro.RevenueGapUSD, plus.RevenueGapUSD)
	require.Equal(t, "codex-pro", groups[0].Group)
}

func TestModelGatewayProfitMonitorDynamicRatioGroupPolicyMatchesCaseInsensitive(t *testing.T) {
	_ = setupModelGatewayProfitMonitorTestDB(t)
	setting := scheduler_setting.DefaultSetting()
	setting.DynamicBillingEnabled = true
	setting.DynamicBillingCostSource = scheduler_setting.DynamicBillingCostSourceProfit24h
	setting.DynamicBillingApplyMode = scheduler_setting.DynamicBillingApplyModeAuto
	setting.DynamicBillingMinSamples = 1
	setting.DynamicBillingMinRequests = 1
	setting.DynamicBillingMinSuccessRequests = 1
	setting.DynamicBillingMinTokens = 1
	setting.DynamicBillingMaxAgeSeconds = 3600
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"CODEX-PLUS": {
			BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
		},
	}
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restoreSetting)

	now := common.GetTimestamp()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"codex-plus": {
			Group:               "codex-plus",
			ReferenceModel:      "gpt-test",
			Ratio:               0.33,
			TargetRatio:         0.33,
			EffectiveRatio:      0.33,
			SampleCount:         1,
			CalculatedAt:        now,
			ProfitRate:          0.2,
			CostSource:          scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:           scheduler_setting.DynamicBillingApplyModeAuto,
			OperatingCostUSD:    0.20,
			RequiredRevenueUSD:  0.25,
			BaseQuotaAtRatio1:   float64(common.QuotaPerUnit),
			RequestCount:        1,
			SuccessRequestCount: 1,
			TotalTokens:         1000,
		},
	})
	t.Cleanup(restoreBaselines)

	groups, _ := buildModelGatewayProfitDynamicRatioGroups(ModelGatewayProfitMonitorConfig{
		TargetProfitRate: 0.2,
	})
	plus := findProfitMonitorDynamicRatioGroup(t, groups, "codex-plus")

	require.Equal(t, scheduler_setting.BillingRatioModeDynamic, plus.BillingRatioMode)
	require.True(t, plus.Applied)
	require.InEpsilon(t, 0.33, plus.ActualRatio, 0.0001)
}

func TestModelGatewayProfitMonitorDynamicRatioGroupShowsObserveFallback(t *testing.T) {
	_ = setupModelGatewayProfitMonitorTestDB(t)
	seedProfitMonitorDynamicBillingBaselines(t, scheduler_setting.DynamicBillingApplyModeObserve)

	groups, _ := buildModelGatewayProfitDynamicRatioGroups(ModelGatewayProfitMonitorConfig{
		TargetProfitRate: 0.2,
	})
	plus := findProfitMonitorDynamicRatioGroup(t, groups, "codex-plus")

	require.False(t, plus.Applied)
	require.Equal(t, modelgatewaydynamicbilling.FallbackObserveMode, plus.FallbackReason)
	require.InEpsilon(t, 1.0, plus.ActualRatio, 0.0001)
	require.InEpsilon(t, 0.20, plus.DynamicRatio, 0.0001)
}

func TestModelGatewayProfitMonitorGroupRecommendationScopePersistsAndCanaryInherits(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	seedProfitMonitorDynamicBillingBaselines(t, scheduler_setting.DynamicBillingApplyModeAuto)

	router := gin.New()
	router.POST("/api/model_gateway/profit_monitor/recommendations", CreateModelGatewayProfitMonitorRecommendation)
	router.POST("/api/model_gateway/profit_monitor/canary_tasks", func(c *gin.Context) {
		c.Set("id", 9)
		c.Set("username", "tester")
		CreateModelGatewayProfitMonitorCanaryTask(c)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/profit_monitor/recommendations?window=24h&breakdown_dimension=channel&scope_type=group&scope_key=codex-pro", nil)
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitRecommendationAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.NotZero(t, payload.Data.Id)
	require.Equal(t, model.ModelGatewayProfitResourceScopeGroup, payload.Data.Dimension)
	require.Equal(t, model.ModelGatewayProfitResourceScopeGroup, payload.Data.ScopeType)
	require.Equal(t, "codex-pro", payload.Data.ScopeKey)
	require.Greater(t, payload.Data.RecommendedRevenueMultiplier, 1.0)
	require.Contains(t, payload.Data.InputJSON, `"scope_key":"codex-pro"`)

	body, err := common.Marshal(UpsertModelGatewayProfitCanaryTaskRequest{
		RecommendationID: intPtr(payload.Data.Id),
		Title:            stringPtr("codex-pro group canary"),
	})
	require.NoError(t, err)
	canaryRecorder := httptest.NewRecorder()
	canaryReq := httptest.NewRequest(http.MethodPost, "/api/model_gateway/profit_monitor/canary_tasks", bytes.NewReader(body))
	router.ServeHTTP(canaryRecorder, canaryReq)

	var canaryPayload modelGatewayProfitCanaryTaskAPIResponse
	require.NoError(t, common.Unmarshal(canaryRecorder.Body.Bytes(), &canaryPayload))
	require.True(t, canaryPayload.Success, canaryRecorder.Body.String())
	require.Equal(t, model.ModelGatewayProfitResourceScopeGroup, canaryPayload.Data.ScopeType)
	require.Equal(t, "codex-pro", canaryPayload.Data.ScopeKey)

	var row model.ModelGatewayProfitRatioRecommendation
	require.NoError(t, db.First(&row, "id = ?", payload.Data.Id).Error)
	require.Equal(t, "codex-pro", row.ScopeKey)
}

func TestModelGatewayProfitMonitorRecommendationSnapshotPersistsAndLists(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	require.NoError(t, saveModelGatewayProfitMonitorConfig(ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		ServerDailyCostUSD:             480,
		ResourceCostEnabled:            true,
		TargetProfitRate:               0.2,
		DynamicRatioRecommendationMode: "ai",
	}))
	for i := 0; i < 20; i++ {
		require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
			RequestId:         fmt.Sprintf("req-recommendation-%d", i),
			ChannelID:         12,
			ChannelName:       "real channel",
			RequestedModel:    "gpt-test",
			RequestedGroup:    "default",
			SelectedGroup:     "default",
			CompletedAt:       120 + int64(i),
			Success:           true,
			TotalTokens:       1000,
			Quota:             500000,
			UpstreamCostTotal: 0.3,
			IsHealthProbe:     false,
		}).Error)
	}
	require.NoError(t, db.Create(&model.ModelGatewayProfitResourceCost{
		Name:            "account loss pool",
		ResourceType:    model.ModelGatewayProfitResourceTypeAccountPool,
		AmountUSD:       1.2,
		PeriodSeconds:   3600,
		AmortizeStartAt: 100,
		AmortizeEndAt:   3700,
		Enabled:         true,
	}).Error)
	configBefore := common.OptionMap[modelGatewayProfitMonitorConfigOptionKey]

	router := gin.New()
	router.POST("/api/model_gateway/profit_monitor/recommendations", CreateModelGatewayProfitMonitorRecommendation)
	router.GET("/api/model_gateway/profit_monitor/recommendations", ListModelGatewayProfitMonitorRecommendations)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/profit_monitor/recommendations?start_timestamp=100&end_timestamp=3700&dimension=channel", nil)
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitRecommendationAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.NotZero(t, payload.Data.Id)
	require.Equal(t, "custom", payload.Data.Window)
	require.Equal(t, "channel", payload.Data.Dimension)
	require.InEpsilon(t, 20.0, payload.Data.RevenueUSD, 0.0001)
	require.InEpsilon(t, 27.2, payload.Data.OperatingCostUSD, 0.0001)
	require.InEpsilon(t, 34.0, payload.Data.RequiredRevenueUSD, 0.0001)
	require.InEpsilon(t, 1.25, payload.Data.CostMarkupMultiplier, 0.0001)
	require.InEpsilon(t, 1.7, payload.Data.RecommendedRevenueMultiplier, 0.0001)
	require.Equal(t, "high", payload.Data.RiskLevel)
	require.Equal(t, "high_gap", payload.Data.Reason)
	require.Contains(t, payload.Data.InputJSON, `"summary"`)
	require.Contains(t, payload.Data.RecommendationJSON, `"cost_markup_multiplier":1.25`)
	require.Contains(t, payload.Data.RecommendationJSON, `"reason_code":"high_gap"`)
	require.Contains(t, payload.Data.RecommendationJSON, `"constraint_codes"`)
	require.Equal(t, configBefore, common.OptionMap[modelGatewayProfitMonitorConfigOptionKey])
	require.NoError(t, db.Create(&model.ModelGatewayProfitRatioRecommendation{
		Window:    "custom",
		Dimension: "group",
		Reason:    "target_covered",
	}).Error)

	listRecorder := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/model_gateway/profit_monitor/recommendations?limit=5&window=custom&dimension=channel", nil)
	router.ServeHTTP(listRecorder, listReq)

	var listPayload modelGatewayProfitRecommendationListAPIResponse
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	require.True(t, listPayload.Success, listRecorder.Body.String())
	require.Len(t, listPayload.Data, 1)
	require.Equal(t, payload.Data.Id, listPayload.Data[0].Id)
}

func TestModelGatewayProfitMonitorRecommendationDecisionPatchPersistsAuditFields(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProfitRatioRecommendation{
		Window:                       "24h",
		Dimension:                    "group",
		Reason:                       "below_target",
		RecommendedRevenueMultiplier: 1.45,
		InputJSON:                    `{"summary":{"requests":20}}`,
		RecommendationJSON:           `{"reason_code":"below_target"}`,
	}).Error)

	router := gin.New()
	router.PATCH("/api/model_gateway/profit_monitor/recommendations/:id/decision", func(c *gin.Context) {
		c.Set("id", 42)
		c.Set("username", "ops-admin")
		UpdateModelGatewayProfitMonitorRecommendationDecision(c)
	})

	body, err := common.Marshal(UpdateModelGatewayProfitRecommendationDecisionRequest{
		DecisionStatus:           model.ModelGatewayProfitRecommendationDecisionCanary,
		DecisionRemark:           stringPtr("先灰度 default 分组 2 小时"),
		PlannedRevenueMultiplier: floatPtr(1.25),
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/profit_monitor/recommendations/1/decision", bytes.NewReader(body))
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitRecommendationAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, model.ModelGatewayProfitRecommendationDecisionCanary, payload.Data.DecisionStatus)
	require.Equal(t, "先灰度 default 分组 2 小时", payload.Data.DecisionRemark)
	require.InEpsilon(t, 1.25, payload.Data.PlannedRevenueMultiplier, 0.0001)
	require.Equal(t, 42, payload.Data.DecisionOperatorID)
	require.Equal(t, "ops-admin", payload.Data.DecisionOperatorName)
	require.NotZero(t, payload.Data.DecisionUpdatedAt)
	require.Equal(t, `{"summary":{"requests":20}}`, payload.Data.InputJSON)
	require.Equal(t, `{"reason_code":"below_target"}`, payload.Data.RecommendationJSON)

	invalidBody, err := common.Marshal(UpdateModelGatewayProfitRecommendationDecisionRequest{
		DecisionStatus: "shipped",
	})
	require.NoError(t, err)
	invalidRecorder := httptest.NewRecorder()
	invalidReq := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/profit_monitor/recommendations/1/decision", bytes.NewReader(invalidBody))
	router.ServeHTTP(invalidRecorder, invalidReq)
	require.Contains(t, invalidRecorder.Body.String(), "无效的决策状态")
}

func TestModelGatewayProfitMonitorCanaryTaskCreateUpdateAndList(t *testing.T) {
	db := setupModelGatewayProfitMonitorTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProfitRatioRecommendation{
		Window:                       "24h",
		Dimension:                    "group",
		Reason:                       "below_target",
		RecommendedRevenueMultiplier: 1.45,
		PlannedRevenueMultiplier:     1.25,
		DecisionStatus:               model.ModelGatewayProfitRecommendationDecisionCanary,
	}).Error)

	router := gin.New()
	router.POST("/api/model_gateway/profit_monitor/canary_tasks", func(c *gin.Context) {
		c.Set("id", 42)
		c.Set("username", "ops-admin")
		CreateModelGatewayProfitMonitorCanaryTask(c)
	})
	router.PATCH("/api/model_gateway/profit_monitor/canary_tasks/:id", func(c *gin.Context) {
		c.Set("id", 43)
		c.Set("username", "reviewer")
		UpdateModelGatewayProfitMonitorCanaryTask(c)
	})
	router.GET("/api/model_gateway/profit_monitor/canary_tasks", ListModelGatewayProfitMonitorCanaryTasks)

	createBody, err := common.Marshal(UpsertModelGatewayProfitCanaryTaskRequest{
		RecommendationID:         intPtr(1),
		Title:                    stringPtr("default 分组灰度"),
		ScopeType:                stringPtr(model.ModelGatewayProfitResourceScopeGroup),
		ScopeKey:                 stringPtr("default"),
		ObservationWindowSeconds: intPtr(3600),
		WatchMetrics: []string{
			"gross_margin",
			"success_rate",
			"invalid_metric",
			"gross_margin",
		},
	})
	require.NoError(t, err)

	createRecorder := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/model_gateway/profit_monitor/canary_tasks", bytes.NewReader(createBody))
	router.ServeHTTP(createRecorder, createReq)

	var createPayload modelGatewayProfitCanaryTaskAPIResponse
	require.NoError(t, common.Unmarshal(createRecorder.Body.Bytes(), &createPayload))
	require.True(t, createPayload.Success, createRecorder.Body.String())
	require.NotZero(t, createPayload.Data.Id)
	require.Equal(t, model.ModelGatewayProfitCanaryTaskStatusPlanned, createPayload.Data.Status)
	require.Equal(t, model.ModelGatewayProfitResourceScopeGroup, createPayload.Data.ScopeType)
	require.Equal(t, "default", createPayload.Data.ScopeKey)
	require.InEpsilon(t, 1.25, createPayload.Data.PlannedRevenueMultiplier, 0.0001)
	require.InEpsilon(t, 1.45, createPayload.Data.RecommendedRevenueMultiplier, 0.0001)
	require.Equal(t, []string{"gross_margin", "success_rate"}, createPayload.Data.WatchMetrics)
	require.Equal(t, 42, createPayload.Data.CreatedByID)
	require.Equal(t, "ops-admin", createPayload.Data.CreatedByName)

	updateBody, err := common.Marshal(UpsertModelGatewayProfitCanaryTaskRequest{
		Status:        model.ModelGatewayProfitCanaryTaskStatusCompleted,
		ResultSummary: stringPtr("毛利率恢复，保留后续观察"),
	})
	require.NoError(t, err)
	updateRecorder := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/profit_monitor/canary_tasks/1", bytes.NewReader(updateBody))
	router.ServeHTTP(updateRecorder, updateReq)

	var updatePayload modelGatewayProfitCanaryTaskAPIResponse
	require.NoError(t, common.Unmarshal(updateRecorder.Body.Bytes(), &updatePayload))
	require.True(t, updatePayload.Success, updateRecorder.Body.String())
	require.Equal(t, model.ModelGatewayProfitCanaryTaskStatusCompleted, updatePayload.Data.Status)
	require.Equal(t, "毛利率恢复，保留后续观察", updatePayload.Data.ResultSummary)
	require.NotZero(t, updatePayload.Data.ActualEndAt)
	require.Equal(t, 43, updatePayload.Data.UpdatedByID)
	require.Equal(t, "reviewer", updatePayload.Data.UpdatedByName)

	listRecorder := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/model_gateway/profit_monitor/canary_tasks?status=completed", nil)
	router.ServeHTTP(listRecorder, listReq)

	var listPayload modelGatewayProfitCanaryTaskListAPIResponse
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	require.True(t, listPayload.Success, listRecorder.Body.String())
	require.Len(t, listPayload.Data, 1)
	require.Equal(t, model.ModelGatewayProfitCanaryTaskStatusCompleted, listPayload.Data[0].Status)
	require.Equal(t, []string{"gross_margin", "success_rate"}, listPayload.Data[0].WatchMetrics)
}

func TestModelGatewayProfitMonitorRecommendationRequiresMinimumSamples(t *testing.T) {
	recommendation := buildModelGatewayProfitRecommendation(ModelGatewayProfitMonitorSummary{
		Requests:         1,
		SuccessRequests:  1,
		TotalTokens:      1000,
		RevenueUSD:       1,
		UpstreamCostUSD:  2,
		OperatingCostUSD: 2,
	}, ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		TargetProfitRate:               0.2,
		DynamicRatioRecommendationMode: "observe",
	})

	require.False(t, recommendation.CanRecommend)
	require.Equal(t, modelGatewayProfitRecommendationReasonLowSample, recommendation.Reason)
	require.InEpsilon(t, 2.5, recommendation.RequiredRevenueUSD, 0.0001)
	require.InEpsilon(t, 1.25, recommendation.CostMarkupMultiplier, 0.0001)
	require.InEpsilon(t, 2.5, recommendation.RecommendedRevenueMultiplier, 0.0001)
}

func TestModelGatewayProfitRecommendationUsesWeightedCostMultiplierAndMonitorMarkup(t *testing.T) {
	restore := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"codex-plus": {
			Group:              "codex-plus",
			Ratio:              0.048,
			CostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			BaseQuotaAtRatio1:  100 * common.QuotaPerUnit,
			CostMultiplier:     0.03,
			RequiredRevenueUSD: 3 / 0.99,
			EffectiveRatio:     0.048,
			TotalTokens:        10_000_000,
		},
		"codex-plus-small": {
			Group:              "codex-plus-small",
			Ratio:              0.16,
			CostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			BaseQuotaAtRatio1:  common.QuotaPerUnit,
			CostMultiplier:     0.1,
			RequiredRevenueUSD: 0.1 / 0.99,
			EffectiveRatio:     0.16,
			TotalTokens:        10_000,
		},
	})
	defer restore()

	recommendation := buildModelGatewayProfitRecommendation(ModelGatewayProfitMonitorSummary{
		Requests:        100,
		SuccessRequests: 100,
		TotalTokens:     10_010_000,
		RevenueUSD:      10,
		UpstreamCostUSD: 3.1,
	}, ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		TargetProfitRate:               0.01,
		DynamicRatioRecommendationMode: "observe",
	})
	enrichModelGatewayProfitRecommendationWithDynamicBilling(&recommendation, ModelGatewayProfitMonitorConfig{
		TargetProfitRate:               0.01,
		DynamicRatioRecommendationMode: "observe",
	})

	expectedCostMultiplier := (0.03*100 + 0.1) / 101
	expectedSuggestedRatio := expectedCostMultiplier / 0.99
	expectedRequiredRevenue := expectedSuggestedRatio * 101 * common.QuotaPerUnit / common.QuotaPerUnit
	require.InEpsilon(t, expectedCostMultiplier, recommendation.CostMultiplier, 0.000001)
	require.InEpsilon(t, expectedSuggestedRatio, recommendation.SuggestedDynamicRatio, 0.000001)
	require.InEpsilon(t, expectedRequiredRevenue, recommendation.RequiredRevenueUSD, 0.000001)
	unweightedRatio := (0.03 + 0.1) / 2 / 0.99
	require.Greater(t, math.Abs(unweightedRatio-recommendation.SuggestedDynamicRatio), 0.01)
}

func TestModelGatewayProfitRecommendationUsesDynamicBillingProfitRate(t *testing.T) {
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingCostSource: scheduler_setting.DynamicBillingCostSourceProfit24h,
		DynamicBillingProfitRate: 0.6,
	})
	defer restoreSetting()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"codex-plus": {
			Group:              "codex-plus",
			Ratio:              0.0287,
			CostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			BaseQuotaAtRatio1:  100 * common.QuotaPerUnit,
			CostMultiplier:     0.0284,
			RequiredRevenueUSD: 2.84 / 0.4,
			EffectiveRatio:     0.0287,
			TotalTokens:        10_000_000,
		},
	})
	defer restoreBaselines()

	config := effectiveModelGatewayProfitMonitorRecommendationConfig(ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		TargetProfitRate:               0.0095,
		DynamicRatioRecommendationMode: "observe",
	})
	recommendation := buildModelGatewayProfitRecommendation(ModelGatewayProfitMonitorSummary{
		Requests:        100,
		SuccessRequests: 100,
		TotalTokens:     10_000_000,
		RevenueUSD:      10,
		UpstreamCostUSD: 2.84,
	}, config)
	enrichModelGatewayProfitRecommendationWithDynamicBilling(&recommendation, config)

	require.InEpsilon(t, 0.6, recommendation.TargetProfitRate, 0.000001)
	require.InEpsilon(t, 2.5, recommendation.CostMarkupMultiplier, 0.000001)
	require.InEpsilon(t, 0.0284*2.5, recommendation.SuggestedDynamicRatio, 0.000001)
}

func TestModelGatewayProfitRecommendationAppliesDynamicRatioMaxLimit(t *testing.T) {
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingMinRatio: 0.01,
		DynamicBillingMaxRatio: 1,
	})
	defer restoreSetting()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"codex-plus": {
			Group:              "codex-plus",
			Ratio:              0.071,
			CostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			BaseQuotaAtRatio1:  100 * common.QuotaPerUnit,
			CostMultiplier:     0.0284,
			RequiredRevenueUSD: 7.1,
			EffectiveRatio:     0.071,
			TotalTokens:        10_000_000,
		},
	})
	defer restoreBaselines()

	config := ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		TargetProfitRate:               0.2,
		DynamicRatioMaxLimit:           0.05,
		DynamicRatioRecommendationMode: "observe",
	}
	recommendation := buildModelGatewayProfitRecommendation(ModelGatewayProfitMonitorSummary{
		Requests:        100,
		SuccessRequests: 100,
		TotalTokens:     10_000_000,
		RevenueUSD:      4,
		UpstreamCostUSD: 2.84,
	}, config)
	enrichModelGatewayProfitRecommendationWithDynamicBilling(&recommendation, config)

	require.InEpsilon(t, 0.071, recommendation.SuggestedDynamicRatioRaw, 0.000001)
	require.InEpsilon(t, 0.05, recommendation.SuggestedDynamicRatio, 0.000001)
	require.InEpsilon(t, 0.01, recommendation.DynamicRatioLimitMin, 0.000001)
	require.InEpsilon(t, 0.05, recommendation.DynamicRatioLimitMax, 0.000001)
	require.True(t, recommendation.DynamicRatioLimitApplied)
	require.Equal(t, "max_limit", recommendation.DynamicRatioLimitReason)
	require.InEpsilon(t, 5, recommendation.RequiredRevenueUSD, 0.000001)
	require.InEpsilon(t, 1, recommendation.RevenueGapUSD, 0.000001)
	require.InEpsilon(t, 1.25, recommendation.RecommendedRevenueMultiplier, 0.000001)
}

func TestModelGatewayProfitRecommendationAppliesFixedDynamicRatio(t *testing.T) {
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingMinRatio: 0.01,
		DynamicBillingMaxRatio: 0.05,
	})
	defer restoreSetting()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"codex-plus": {
			Group:              "codex-plus",
			Ratio:              0.071,
			CostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			BaseQuotaAtRatio1:  100 * common.QuotaPerUnit,
			CostMultiplier:     0.0284,
			RequiredRevenueUSD: 7.1,
			EffectiveRatio:     0.071,
			TotalTokens:        10_000_000,
		},
	})
	defer restoreBaselines()

	config := ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		TargetProfitRate:               0.2,
		DynamicRatioMaxLimit:           0.05,
		DynamicRatioFixedValue:         0.0666,
		DynamicRatioRecommendationMode: "observe",
	}
	recommendation := buildModelGatewayProfitRecommendation(ModelGatewayProfitMonitorSummary{
		Requests:        100,
		SuccessRequests: 100,
		TotalTokens:     10_000_000,
		RevenueUSD:      4,
		UpstreamCostUSD: 2.84,
	}, config)
	enrichModelGatewayProfitRecommendationWithDynamicBilling(&recommendation, config)

	require.InEpsilon(t, 0.071, recommendation.SuggestedDynamicRatioRaw, 0.000001)
	require.InEpsilon(t, 0.0666, recommendation.SuggestedDynamicRatio, 0.000001)
	require.InEpsilon(t, 0.0666, recommendation.DynamicRatioFixedValue, 0.000001)
	require.True(t, recommendation.DynamicRatioFixedApplied)
	require.True(t, recommendation.DynamicRatioLimitApplied)
	require.Equal(t, "fixed_ratio", recommendation.DynamicRatioLimitReason)
}

func TestModelGatewayProfitMonitorConfigAcceptsPercentInput(t *testing.T) {
	config := normalizeModelGatewayProfitMonitorConfig(ModelGatewayProfitMonitorConfig{
		TargetProfitRate:               60,
		DynamicRatioRecommendationMode: "observe",
	})

	require.InEpsilon(t, 0.6, config.TargetProfitRate, 0.000001)
}

func seedProfitMonitorDynamicBillingBaselines(t *testing.T, applyMode string) {
	t.Helper()
	setting := scheduler_setting.DefaultSetting()
	setting.DynamicBillingEnabled = true
	setting.DynamicBillingCostSource = scheduler_setting.DynamicBillingCostSourceProfit24h
	setting.DynamicBillingApplyMode = applyMode
	setting.DynamicBillingMinSamples = 1
	setting.DynamicBillingMinRequests = 1
	setting.DynamicBillingMinSuccessRequests = 1
	setting.DynamicBillingMinTokens = 1
	setting.DynamicBillingMaxAgeSeconds = 3600
	setting.DynamicBillingMinRatio = 0.01
	setting.DynamicBillingMaxRatio = 2
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"codex-plus": {
			BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
		},
		"codex-pro": {
			BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
		},
	}
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restoreSetting)
	now := common.GetTimestamp()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"codex-plus": {
			Group:               "codex-plus",
			ReferenceModel:      "gpt-test",
			Ratio:               0.20,
			TargetRatio:         0.30,
			EffectiveRatio:      0.20,
			Clamped:             true,
			SampleCount:         8,
			ModelCount:          1,
			CalculatedAt:        now,
			WindowStart:         now - 3600,
			WindowEnd:           now,
			ProfitRate:          0.2,
			CostSource:          scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:           applyMode,
			OperatingCostUSD:    0.24,
			RequiredRevenueUSD:  0.30,
			BaseQuotaAtRatio1:   float64(common.QuotaPerUnit),
			CostMultiplier:      0.24,
			RequestCount:        20,
			SuccessRequestCount: 20,
			TotalTokens:         10_000,
			UpstreamCostUSD:     0.18,
			TrafficCostUSD:      0.01,
			ServerCostUSD:       0.02,
			ResourceCostUSD:     0.03,
			TrafficDataReady:    true,
		},
		"codex-pro": {
			Group:               "codex-pro",
			ReferenceModel:      "gpt-test",
			Ratio:               0.55,
			TargetRatio:         0.70,
			EffectiveRatio:      0.55,
			Clamped:             true,
			SampleCount:         12,
			ModelCount:          1,
			CalculatedAt:        now,
			WindowStart:         now - 3600,
			WindowEnd:           now,
			ProfitRate:          0.2,
			CostSource:          scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:           applyMode,
			OperatingCostUSD:    0.56,
			RequiredRevenueUSD:  0.70,
			BaseQuotaAtRatio1:   float64(common.QuotaPerUnit),
			CostMultiplier:      0.56,
			RequestCount:        24,
			SuccessRequestCount: 24,
			TotalTokens:         20_000,
			UpstreamCostUSD:     0.42,
			TrafficCostUSD:      0.02,
			ServerCostUSD:       0.04,
			ResourceCostUSD:     0.08,
			TrafficDataReady:    true,
		},
	})
	t.Cleanup(restoreBaselines)
}

func findProfitMonitorDynamicRatioGroup(t *testing.T, groups []ModelGatewayProfitDynamicRatioGroup, name string) ModelGatewayProfitDynamicRatioGroup {
	t.Helper()
	for _, group := range groups {
		if group.Group == name {
			return group
		}
	}
	require.Failf(t, "missing dynamic ratio group", "group=%s", name)
	return ModelGatewayProfitDynamicRatioGroup{}
}

func boolPtr(value bool) *bool {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
