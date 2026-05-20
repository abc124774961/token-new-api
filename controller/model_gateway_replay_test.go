package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelGatewayReplayControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.Log{}, &model.User{}))

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	resetModelGatewayObservabilitySummaryCache()
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		resetModelGatewayObservabilitySummaryCache()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestExportModelGatewayReplayReturnsSanitizedArtifact(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:       10,
		RequestId:       "req-api",
		UserId:          9,
		TokenId:         10,
		RequestedGroup:  "auto",
		SelectedGroup:   "vip",
		RequestedModel:  "mimo-v1",
		ChannelId:       101,
		ChannelName:     "https://upstream.example.com/v1?api_key=secret",
		EndpointType:    string(constant.EndpointTypeOpenAIResponse),
		PolicyMode:      core.ModeActive,
		AutoMode:        core.AutoModeFusion,
		Strategy:        core.StrategyBalanced,
		SmartHandled:    true,
		CandidateGroups: `["default","vip"]`,
		RequestMeta:     `{"prompt_tokens":100,"pre_consumed_quota":200}`,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/replay/export", ExportModelGatewayReplay)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/replay/export?request_id=req-api&stable_ids=true", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":true`)
	require.Contains(t, resp.Body.String(), `"count":1`)
	require.Contains(t, resp.Body.String(), `"request_id":"req-api"`)
	require.NotContains(t, resp.Body.String(), `"records":[{"request_id":"req-api"`)
	require.NotContains(t, resp.Body.String(), `"user_id":9`)
	require.NotContains(t, resp.Body.String(), "api_key")
	require.NotContains(t, resp.Body.String(), "prompt_tokens")
	require.Contains(t, resp.Body.String(), `"channel_id":1`)
}

func TestExportModelGatewayReplayDownload(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      10,
		RequestId:      "req/download",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-5.5",
		ChannelId:      8,
		EndpointType:   string(constant.EndpointTypeOpenAI),
		PolicyMode:     core.ModeActive,
		Strategy:       core.StrategyBalanced,
		SmartHandled:   true,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/replay/export", ExportModelGatewayReplay)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/replay/export?request_id=req/download&download=true", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Header().Get("Content-Type"), "application/json")
	require.Contains(t, resp.Header().Get("Content-Disposition"), "modelgateway-replay-req_download.json")
	require.Contains(t, resp.Body.String(), `"kind": "modelgateway_replay"`)
	require.NotContains(t, resp.Body.String(), `"request_id"`)
	require.NotContains(t, resp.Body.String(), `"success":true`)
}

func TestExportModelGatewayReplayRequiresRequestID(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)

	router := gin.New()
	router.GET("/api/model_gateway/replay/export", ExportModelGatewayReplay)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/replay/export", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "request_id is required")
}

func TestExportModelGatewayReplayBatchReturnsManifestAndSanitizedArtifacts(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:       100,
			RequestId:       "req-batch-1",
			UserId:          99,
			TokenId:         100,
			RequestedGroup:  "auto",
			SelectedGroup:   "vip",
			RequestedModel:  "mimo-v1",
			ChannelId:       101,
			ChannelName:     "https://upstream.example.com/v1?api_key=secret",
			EndpointType:    string(constant.EndpointTypeOpenAIResponse),
			PolicyMode:      core.ModeActive,
			AutoMode:        core.AutoModeFusion,
			Strategy:        core.StrategyBalanced,
			SmartHandled:    true,
			Success:         false,
			ErrorType:       "stream",
			CandidateGroups: `["default","vip"]`,
			RequestMeta:     `{"prompt_tokens":100,"pre_consumed_quota":200}`,
		},
		{
			CreatedAt:      110,
			RequestId:      "req-batch-2",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      202,
			EndpointType:   string(constant.EndpointTypeOpenAI),
			PolicyMode:     core.ModeActive,
			Strategy:       core.StrategyBalanced,
			SmartHandled:   true,
			Success:        true,
		},
		{
			CreatedAt:      120,
			RequestId:      "req-dispatch-only",
			RequestedGroup: "auto",
			SelectedGroup:  "vip",
			RequestedModel: "mimo-v1",
			ChannelId:      103,
			EndpointType:   string(constant.EndpointTypeOpenAIResponse),
			PolicyMode:     core.ModeActive,
			AutoMode:       core.AutoModeFusion,
			Strategy:       core.StrategyBalanced,
			SmartHandled:   true,
		},
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/replay/export/batch", ExportModelGatewayReplayBatch)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/replay/export/batch?start_time=1&end_time=200&model=mimo-v1&error_type=stream&stable_ids=true", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":true`)
	require.Contains(t, resp.Body.String(), `"kind":"modelgateway_replay_batch"`)
	require.Contains(t, resp.Body.String(), `"artifact_count":1`)
	require.Contains(t, resp.Body.String(), `"request_id":"req-batch-1"`)
	require.NotContains(t, resp.Body.String(), `"user_id":99`)
	require.NotContains(t, resp.Body.String(), "api_key")
	require.NotContains(t, resp.Body.String(), "prompt_tokens")
	require.NotContains(t, resp.Body.String(), "req-batch-2")
	require.NotContains(t, resp.Body.String(), "req-dispatch-only")

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Contains(t, logs[0].Content, "replay 批量样本")
	require.NotContains(t, logs[0].Other, "req-batch-1")
	require.Contains(t, logs[0].Other, "request_hash")
}

func TestExportModelGatewayReplayBatchRequestIDListReportsMissingAndDownload(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      10,
		RequestId:      "req-present",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-5.5",
		ChannelId:      8,
		EndpointType:   string(constant.EndpointTypeOpenAI),
		PolicyMode:     core.ModeActive,
		Strategy:       core.StrategyBalanced,
		SmartHandled:   true,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/replay/export/batch", ExportModelGatewayReplayBatch)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/replay/export/batch?request_ids=req-missing,req-present,req-present&download=true", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Header().Get("Content-Disposition"), "modelgateway-replay-batch-")
	require.Contains(t, resp.Body.String(), `"kind":"modelgateway_replay_batch"`)
	require.Contains(t, resp.Body.String(), `"failed_count":1`)
	require.Contains(t, resp.Body.String(), `"request_id":"req-missing"`)
	require.Contains(t, resp.Body.String(), `"error":"no model execution records found`)
	require.Contains(t, resp.Body.String(), `"request_id":"req-present"`)
	require.Contains(t, resp.Body.String(), `"artifact_count":1`)
}

func TestExportModelGatewayReplayBatchFiltersByChannelID(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      10,
			RequestId:      "req-channel-primary",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      8,
			EndpointType:   string(constant.EndpointTypeOpenAI),
			PolicyMode:     core.ModeActive,
			Strategy:       core.StrategyBalanced,
			SmartHandled:   true,
		},
		{
			CreatedAt:         11,
			RequestId:         "req-channel-actual",
			RequestedGroup:    "default",
			SelectedGroup:     "default",
			RequestedModel:    "gpt-5.5",
			ChannelId:         7,
			ActualChannelId:   8,
			ActualChannelName: "actual-channel",
			EndpointType:      string(constant.EndpointTypeOpenAI),
			PolicyMode:        core.ModeActive,
			Strategy:          core.StrategyBalanced,
			SmartHandled:      true,
		},
		{
			CreatedAt:      12,
			RequestId:      "req-channel-other",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      9,
			EndpointType:   string(constant.EndpointTypeOpenAI),
			PolicyMode:     core.ModeActive,
			Strategy:       core.StrategyBalanced,
			SmartHandled:   true,
		},
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/replay/export/batch", ExportModelGatewayReplayBatch)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/replay/export/batch?start_time=1&end_time=20&channel_id=8&stable_ids=true", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"channel_id":8`)
	require.Contains(t, resp.Body.String(), `"artifact_count":2`)
	require.Contains(t, resp.Body.String(), `"request_id":"req-channel-primary"`)
	require.Contains(t, resp.Body.String(), `"request_id":"req-channel-actual"`)
	require.NotContains(t, resp.Body.String(), "req-channel-other")
}

func TestSafeReplayFilenamePart(t *testing.T) {
	require.Equal(t, "req_abc_1", safeReplayFilenamePart("req/abc?1"))
	require.Equal(t, "unknown", safeReplayFilenamePart("///"))
	require.Len(t, safeReplayFilenamePart(strings.Repeat("a", 100)), 80)
}
