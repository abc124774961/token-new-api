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
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelGatewayConfigAPIResponse struct {
	Success bool                       `json:"success"`
	Message string                     `json:"message"`
	Data    ModelGatewayConfigResponse `json:"data"`
}

func setupModelGatewayConfigControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	oldDB := model.DB
	oldOptionMap := common.OptionMap
	model.DB = db
	common.OptionMap = map[string]string{}
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	t.Cleanup(func() {
		restoreSetting()
		model.DB = oldDB
		common.OptionMap = oldOptionMap
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestModelGatewayConfigUpdatePersistsSchedulerSetting(t *testing.T) {
	db := setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.Enabled = true
	setting.DefaultMode = scheduler_setting.ModeShadow
	setting.RolloutPercent = 35
	setting.DefaultStrategy = scheduler_setting.StrategySpeedFirst
	setting.QueueDefaultTimeoutMs = 1500
	setting.GroupPriorityRatio = map[string]float64{"vip": 1.4}
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"vip": {
			Mode:                  scheduler_setting.ModeActive,
			Strategy:              scheduler_setting.StrategyStabilityFirst,
			AutoMode:              scheduler_setting.AutoModeFusion,
			CrossGroupFusion:      true,
			CandidateGroups:       []string{"vip", "default", "vip"},
			CacheAffinityEnabled:  true,
			QueueEnabled:          true,
			CircuitBreakerEnabled: true,
		},
	}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.True(t, payload.Data.Setting.Enabled)
	require.Equal(t, 35, payload.Data.Setting.RolloutPercent)
	require.Equal(t, []string{"vip", "default"}, payload.Data.Setting.GroupPolicies["vip"].CandidateGroups)
	require.Equal(t, scheduler_setting.ModeActive, scheduler_setting.GetSetting().GroupPolicies["vip"].Mode)

	var rolloutOption model.Option
	require.NoError(t, db.First(&rolloutOption, "key = ?", "scheduler_setting.rollout_percent").Error)
	require.Equal(t, "35", rolloutOption.Value)
	var policiesOption model.Option
	require.NoError(t, db.First(&policiesOption, "key = ?", "scheduler_setting.group_policies").Error)
	require.Contains(t, policiesOption.Value, `"vip"`)
	require.Equal(t, "35", common.OptionMap["scheduler_setting.rollout_percent"])
}

func TestModelGatewayConfigRejectsInvalidPolicy(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"default": {Mode: "bad-mode"},
	}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "invalid mode")
}

func TestModelGatewayConfigResetRestoresDefaults(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		Enabled:        true,
		DefaultMode:    scheduler_setting.ModeActive,
		RolloutPercent: 100,
	})
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/model_gateway/config/reset", ResetModelGatewayConfig)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/config/reset", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.False(t, payload.Data.Setting.Enabled)
	require.Equal(t, scheduler_setting.ModeOff, payload.Data.Setting.DefaultMode)
	require.Equal(t, 0, payload.Data.Setting.RolloutPercent)
}

func decodeModelGatewayConfigResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelGatewayConfigAPIResponse {
	t.Helper()
	var payload modelGatewayConfigAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}
