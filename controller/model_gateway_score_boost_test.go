package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelGatewayScoreBoostAPIResponse struct {
	Success bool                                  `json:"success"`
	Message string                                `json:"message"`
	Data    ModelGatewayChannelScoreBoostResponse `json:"data"`
}

func setupModelGatewayScoreBoostControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	oldDB := model.DB
	oldLOGDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = nil
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLOGDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestUpdateModelGatewayChannelScoreBoostsPersistsWithoutOverwritingSettings(t *testing.T) {
	db := setupModelGatewayScoreBoostControllerTestDB(t)
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		WireAPI: "responses",
		SmartScoreBoosts: map[string]float64{
			"ttft_latency": 0.2,
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Channel{
		Id:            101,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "boosted",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: string(settingsBytes),
	}).Error)

	router := gin.New()
	router.PATCH("/api/model_gateway/channels/:id/score_boosts", UpdateModelGatewayChannelScoreBoosts)
	body, err := common.Marshal(map[string]any{
		"smart_score_boosts": map[string]float64{
			"completion_rate":       0.6,
			"cost":                  2,
			"ttft_latency":          -0.1,
			"group_priority":        0.5,
			"retry_intent_recovery": 0.5,
			"bad":                   0.5,
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/channels/101/score_boosts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var payload modelGatewayScoreBoostAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, map[string]float64{
		"completion_rate": 0.6,
		"cost":            1,
	}, payload.Data.SmartScoreBoosts)

	var channel model.Channel
	require.NoError(t, db.First(&channel, "id = ?", 101).Error)
	settings := channel.GetOtherSettings()
	require.Equal(t, "responses", settings.WireAPI)
	require.Equal(t, map[string]float64{
		"completion_rate": 0.6,
		"cost":            1,
	}, settings.SmartScoreBoosts)
}

func TestUpdateModelGatewayChannelScoreBoostsRejectsMissingPayload(t *testing.T) {
	db := setupModelGatewayScoreBoostControllerTestDB(t)
	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		WireAPI: "responses",
		SmartScoreBoosts: map[string]float64{
			"ttft_latency": 0.2,
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Channel{
		Id:            102,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "boosted",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: string(settingsBytes),
	}).Error)

	router := gin.New()
	router.PATCH("/api/model_gateway/channels/:id/score_boosts", UpdateModelGatewayChannelScoreBoosts)
	req := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/channels/102/score_boosts", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusBadRequest, resp.Code)
	var channel model.Channel
	require.NoError(t, db.First(&channel, "id = ?", 102).Error)
	settings := channel.GetOtherSettings()
	require.Equal(t, "responses", settings.WireAPI)
	require.Equal(t, map[string]float64{
		"ttft_latency": 0.2,
	}, settings.SmartScoreBoosts)
}
