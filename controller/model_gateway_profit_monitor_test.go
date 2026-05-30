package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
		TargetProfitRate:              floatPtr(1.2),
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
	require.True(t, payload.Data.Config.TrafficEstimationEnabled)
	require.Equal(t, 14, payload.Data.Config.TrafficEstimatedBytesPerToken)

	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", modelGatewayProfitMonitorConfigOptionKey).Error)
	require.Contains(t, option.Value, `"target_profit_rate":0.95`)
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
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/profit_monitor/summary?start_timestamp=100&end_timestamp=3700&dimension=channel", nil)
	router.ServeHTTP(recorder, req)

	var payload modelGatewayProfitMonitorAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, int64(1), payload.Data.Summary.Requests)
	require.Equal(t, int64(500000), payload.Data.Summary.BillingQuota)
	require.InEpsilon(t, 1.0, payload.Data.Summary.RevenueUSD, 0.0001)
	require.InEpsilon(t, 0.3, payload.Data.Summary.UpstreamCostUSD, 0.0001)
	require.InEpsilon(t, 1.0, payload.Data.Summary.ServerCostUSD, 0.0001)
	require.InEpsilon(t, 3.6, payload.Data.Summary.ResourceAmortizedCostUSD, 0.0001)
	require.InEpsilon(t, 4.9, payload.Data.Summary.OperatingCostUSD, 0.0001)
	require.Len(t, payload.Data.Breakdown, 1)
	require.Equal(t, 12, payload.Data.Breakdown[0].DimensionID)
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
	require.InEpsilon(t, 1.7, payload.Data.RecommendedRevenueMultiplier, 0.0001)
	require.Equal(t, "high", payload.Data.RiskLevel)
	require.Equal(t, "high_gap", payload.Data.Reason)
	require.Contains(t, payload.Data.InputJSON, `"summary"`)
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
		OperatingCostUSD: 2,
	}, ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		TargetProfitRate:               0.2,
		DynamicRatioRecommendationMode: "observe",
	})

	require.False(t, recommendation.CanRecommend)
	require.Equal(t, modelGatewayProfitRecommendationReasonLowSample, recommendation.Reason)
	require.InEpsilon(t, 2.5, recommendation.RequiredRevenueUSD, 0.0001)
	require.InEpsilon(t, 2.5, recommendation.RecommendedRevenueMultiplier, 0.0001)
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
