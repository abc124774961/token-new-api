package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type logsAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items []model.Log `json:"items"`
		Total int         `json:"total"`
	} `json:"data"`
}

type logsStatAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Quota int   `json:"quota"`
		Token int64 `json:"token"`
		Rpm   int   `json:"rpm"`
		Tpm   int   `json:"tpm"`
	} `json:"data"`
}

func TestGetLogsStatReturnsFilteredTokenTotal(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.Log{
		UserId:           1001,
		CreatedAt:        now,
		Type:             model.LogTypeConsume,
		Username:         "stat-user",
		TokenName:        "stat-token",
		ModelName:        "gpt-stat",
		Quota:            20,
		PromptTokens:     10,
		CompletionTokens: 5,
		ChannelId:        12,
		Group:            "vip",
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId:           1002,
		CreatedAt:        now,
		Type:             model.LogTypeConsume,
		Username:         "stat-user",
		TokenName:        "stat-token",
		ModelName:        "gpt-stat",
		Quota:            80,
		PromptTokens:     100,
		CompletionTokens: 50,
		ChannelId:        12,
		Group:            "default",
	}).Error)

	router := gin.New()
	router.GET("/api/log/stat", GetLogsStat)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/log/stat?type=2&username=stat-user&token_name=stat-token&model_name=gpt-stat&channel=12&group=vip&start_timestamp=1&end_timestamp="+strconv.FormatInt(now+10, 10), nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var payload logsStatAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, 20, payload.Data.Quota)
	require.Equal(t, int64(15), payload.Data.Token)
	require.Equal(t, 1, payload.Data.Rpm)
	require.Equal(t, 15, payload.Data.Tpm)
}

func TestGetAllLogsAttachesModelGatewayCostSummary(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.Channel{
		Id:   77,
		Name: "codex-upstream",
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId:           1001,
		CreatedAt:        now,
		Type:             model.LogTypeConsume,
		Username:         "cost-user",
		TokenName:        "cost-token",
		ModelName:        "gpt-5.5",
		Quota:            42,
		PromptTokens:     12,
		CompletionTokens: 8,
		ChannelId:        77,
		Group:            "auto",
		RequestId:        "req-log-cost",
		Other:            `{"model_ratio":0.5}`,
	}).Error)
	breakdown, err := common.Marshal(map[string]any{
		"currency":            "USD",
		"cost_coefficient":    1.2,
		"fee_multiplier":      0.8,
		"token_multiplier":    0.4,
		"recharge_multiplier": 2,
		"input": map[string]any{
			"tokens":            12,
			"price_per_million": 0.1,
			"amount":            0.0000012,
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-log-cost",
		ChannelID:         77,
		UpstreamModel:     "mimo-codex",
		UpstreamCostTotal: 0.0000012,
		BreakdownJSON:     string(breakdown),
		CostSource:        "manual",
		CostAccuracy:      "precise",
		CalculatedAt:      now,
	}).Error)

	router := gin.New()
	router.GET("/api/log", GetAllLogs)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/log?type=2&page_size=20", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var payload logsAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "codex-upstream", payload.Data.Items[0].ChannelName)

	other := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(payload.Data.Items[0].Other, &other))
	costSummary, ok := other["model_gateway_cost"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(77), costSummary["channel_id"])
	require.Equal(t, "mimo-codex", costSummary["upstream_model"])
	require.Equal(t, 0.0000012, costSummary["upstream_cost_total"])
	breakdownMap, ok := costSummary["breakdown"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 0.4, breakdownMap["token_multiplier"])
	require.NotContains(t, breakdownMap, "input")
	require.Equal(t, float64(42), costSummary["billing_quota"])
	require.Equal(t, 0.5, other["model_ratio"])
}

func TestGetAllLogsAttachesEstimatedChannelCostWhenSummaryMissing(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.Channel{
		Id:     78,
		Name:   "estimated-cost-channel",
		Models: "gpt-4o",
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:            78,
		UpstreamModel:        "*",
		Source:               "manual",
		Accuracy:             "precise",
		Currency:             "USD",
		PricingMode:          "token",
		InputPerMillion:      2,
		OutputPerMillion:     10,
		TokenMultiplier:      0.5,
		CostCoefficient:      1,
		RechargeMultiplier:   1,
		InputCostMultiplier:  0.5,
		OutputCostMultiplier: 0.5,
		CreatedAt:            now,
		UpdatedAt:            now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId:           1003,
		CreatedAt:        now,
		Type:             model.LogTypeConsume,
		Username:         "estimated-cost-user",
		TokenName:        "estimated-cost-token",
		ModelName:        "gpt-4o",
		Quota:            123,
		PromptTokens:     1000,
		CompletionTokens: 500,
		ChannelId:        78,
		Group:            "auto",
		RequestId:        "req-log-estimated-cost",
		Other:            `{"model_ratio":1.25,"group_ratio":0.5}`,
	}).Error)

	router := gin.New()
	router.GET("/api/log", GetAllLogs)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/log?type=2&page_size=20", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var payload logsAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 1)

	other := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(payload.Data.Items[0].Other, &other))
	costSummary, ok := other["model_gateway_cost"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(78), costSummary["channel_id"])
	require.Equal(t, "gpt-4o", costSummary["upstream_model"])
	require.Equal(t, "manual", costSummary["cost_source"])
	require.Equal(t, "precise", costSummary["cost_accuracy"])
	require.InEpsilon(t, 0.007, costSummary["upstream_cost_total"], 0.000001)
	require.Equal(t, float64(123), costSummary["billing_quota"])

	breakdownMap, ok := costSummary["breakdown"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "USD", breakdownMap["currency"])
	require.Equal(t, 0.5, breakdownMap["token_multiplier"])
	require.NotContains(t, breakdownMap, "input")
	require.NotContains(t, breakdownMap, "output")
}

func TestGetAllLogsFiltersAdminAuditFields(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	matchingOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"permission": "admin:system:roles:update",
			"source":     "database",
			"result":     "completed",
			"summary": map[string]interface{}{
				"target_user_id": 39,
			},
		},
	})
	otherOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"permission": "admin:channel:channel:update",
			"source":     "role_compatibility",
			"result":     "denied",
			"summary": map[string]interface{}{
				"target_user_id": 40,
			},
		},
	})
	require.NoError(t, db.Create(&model.Log{
		UserId:    1,
		CreatedAt: now,
		Type:      model.LogTypeManage,
		Username:  "root-admin",
		Content:   "管理员操作: admin:system:roles:update PUT /api/admin/permissions/users/:id",
		Other:     matchingOther,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId:    2,
		CreatedAt: now,
		Type:      model.LogTypeManage,
		Username:  "channel-admin",
		Content:   "管理员操作: admin:channel:channel:update PUT /api/channel/",
		Other:     otherOther,
	}).Error)

	router := gin.New()
	router.GET("/api/log", GetAllLogs)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/log?type=3&page_size=20&audit_permission=admin:system:roles:update&audit_source=database&audit_result=completed&audit_operator=root-admin&audit_target_user_id=39", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var payload logsAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, 1, payload.Data.Total)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "root-admin", payload.Data.Items[0].Username)
	require.Contains(t, payload.Data.Items[0].Other, "admin:system:roles:update")
}

func TestGetUserLogsDoesNotExposeModelGatewayCostSummary(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.Log{
		UserId:    1002,
		CreatedAt: now,
		Type:      model.LogTypeConsume,
		Username:  "normal-user",
		ModelName: "gpt-5.5",
		ChannelId: 88,
		Group:     "auto",
		RequestId: "req-user-log-cost",
		Other:     `{"admin_info":{"use_channel":[88]},"model_ratio":0.6}`,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-user-log-cost",
		ChannelID:         88,
		UpstreamModel:     "deepseek-codex",
		UpstreamCostTotal: 0.000002,
		CostSource:        "manual",
		CostAccuracy:      "precise",
		CalculatedAt:      now,
	}).Error)

	router := gin.New()
	router.GET("/api/user/log", func(c *gin.Context) {
		c.Set("id", 1002)
		GetUserLogs(c)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/log?type=2&page_size=20", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var payload logsAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 1)
	require.Empty(t, payload.Data.Items[0].ChannelName)

	other := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(payload.Data.Items[0].Other, &other))
	require.NotContains(t, other, "model_gateway_cost")
	require.NotContains(t, other, "admin_info")
	require.Equal(t, 0.6, other["model_ratio"])
}
