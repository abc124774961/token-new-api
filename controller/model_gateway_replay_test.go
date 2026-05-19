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
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}))

	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
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

func TestSafeReplayFilenamePart(t *testing.T) {
	require.Equal(t, "req_abc_1", safeReplayFilenamePart("req/abc?1"))
	require.Equal(t, "unknown", safeReplayFilenamePart("///"))
	require.Len(t, safeReplayFilenamePart(strings.Repeat("a", 100)), 80)
}
