package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAdminOptionAuditControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldOptionMap := common.OptionMap
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	return db
}

func adminOptionAuditControllerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 100)
		c.Set("username", "system-admin")
		c.Set("role", common.RoleRootUser)
		c.Set("group", "default")
		c.Set(common.RequestIdKey, "admin-option-audit-test")
		c.Next()
	})
	router.PUT("/api/option/", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), UpdateOption)
	return router
}

func seedAuditOption(t *testing.T, db *gorm.DB, key string, value string) {
	t.Helper()
	require.NoError(t, db.Create(&model.Option{Key: key, Value: value}).Error)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
}

func TestUpdateOptionAuditSummaryIncludesBeforeAfterState(t *testing.T) {
	db := setupAdminOptionAuditControllerTestDB(t)
	seedAuditOption(t, db, "AuditFeatureEnabled", "false")

	router := adminOptionAuditControllerTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"AuditFeatureEnabled","value":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "AuditFeatureEnabled").Error)
	require.Equal(t, "true", option.Value)

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "update_option", summary["operation"])
	require.Equal(t, "AuditFeatureEnabled", summary["option_key"])
	require.Equal(t, "bool", summary["request_value_type"])
	require.Equal(t, false, summary["sensitive_option"])
	require.Equal(t, true, summary["value_changed"])
	require.Equal(t, true, summary["value_fingerprint_changed"])
	require.Equal(t, true, summary["before_exists"])
	require.Equal(t, true, summary["after_exists"])
	require.Equal(t, "bool", summary["before_value_kind"])
	require.Equal(t, "bool", summary["after_value_kind"])
	require.Equal(t, false, summary["before_bool_value"])
	require.Equal(t, true, summary["after_bool_value"])
	require.Equal(t, float64(5), summary["before_value_length"])
	require.Equal(t, float64(4), summary["after_value_length"])
}

func TestUpdateOptionAuditSummaryMasksSensitiveValue(t *testing.T) {
	db := setupAdminOptionAuditControllerTestDB(t)
	seedAuditOption(t, db, "ProviderSecret", "old-secret-value")

	router := adminOptionAuditControllerTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"ProviderSecret","value":"new-secret-value"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "update_option", summary["operation"])
	require.Equal(t, "ProviderSecret", summary["option_key"])
	require.Equal(t, true, summary["sensitive_option"])
	require.Equal(t, true, summary["value_changed"])
	require.Equal(t, float64(len("old-secret-value")), summary["before_value_length"])
	require.Equal(t, float64(len("new-secret-value")), summary["after_value_length"])
	require.NotEmpty(t, summary["before_value_fingerprint"])
	require.NotEmpty(t, summary["after_value_fingerprint"])
	require.NotContains(t, summary, "before_bool_value")
	require.NotContains(t, summary, "after_bool_value")

	var log model.Log
	require.NoError(t, db.Order("id desc").First(&log).Error)
	require.NotContains(t, log.Other, "old-secret-value")
	require.NotContains(t, log.Other, "new-secret-value")
}
