package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

func setupAdminPerformanceAuditControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
	return db
}

func adminPerformanceAuditControllerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 100)
		c.Set("username", "system-admin")
		c.Set("role", common.RoleRootUser)
		c.Set("group", "default")
		c.Set(common.RequestIdKey, "admin-performance-audit-test")
		c.Next()
	})
	router.DELETE("/api/performance/logs", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), CleanupLogFiles)
	router.DELETE("/api/log/", middleware.RequireAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), DeleteHistoryLogs)
	return router
}

func writeAdminAuditLogFile(t *testing.T, dir string, name string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func TestCleanupLogFilesAuditSummaryIncludesDeletionResult(t *testing.T) {
	db := setupAdminPerformanceAuditControllerTestDB(t)
	oldLogDir := *common.LogDir
	logDir := t.TempDir()
	*common.LogDir = logDir
	t.Cleanup(func() {
		*common.LogDir = oldLogDir
	})
	writeAdminAuditLogFile(t, logDir, "oneapi-20260101000000.log", "old-a")
	writeAdminAuditLogFile(t, logDir, "oneapi-20260102000000.log", "old-bb")
	writeAdminAuditLogFile(t, logDir, "oneapi-20260103000000.log", "new")

	router := adminPerformanceAuditControllerTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/performance/logs?mode=by_count&value=1", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, float64(2), data["deleted_count"])
	require.Equal(t, float64(len("old-a")+len("old-bb")), data["freed_bytes"])

	_, err := os.Stat(filepath.Join(logDir, "oneapi-20260103000000.log"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(logDir, "oneapi-20260101000000.log"))
	require.True(t, os.IsNotExist(err))

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "cleanup_log_files", summary["operation"])
	require.Equal(t, "by_count", summary["mode"])
	require.Equal(t, float64(1), summary["value"])
	require.Equal(t, float64(3), summary["file_count_before"])
	require.Equal(t, float64(1), summary["file_count_after"])
	require.Equal(t, float64(2), summary["candidate_delete_count"])
	require.Equal(t, float64(2), summary["deleted_count"])
	require.Equal(t, float64(len("old-a")+len("old-bb")), summary["freed_bytes"])
	require.Equal(t, float64(0), summary["failed_count"])
}

func TestDeleteHistoryLogsAuditSummaryIncludesDeletedCount(t *testing.T) {
	db := setupAdminPerformanceAuditControllerTestDB(t)
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: 100, Type: model.LogTypeConsume, Content: "old-1"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: 200, Type: model.LogTypeConsume, Content: "old-2"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: 3000, Type: model.LogTypeConsume, Content: "new"}).Error)

	router := adminPerformanceAuditControllerTestRouter()
	recorder := httptest.NewRecorder()
	target := int64(1000)
	request := httptest.NewRequest(http.MethodDelete, "/api/log/?target_timestamp="+strconv.FormatInt(target, 10), nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])
	require.Equal(t, float64(2), payload["data"])

	var oldCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("created_at < ?", target).Count(&oldCount).Error)
	require.Equal(t, int64(0), oldCount)

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "delete_history_logs", summary["operation"])
	require.Equal(t, float64(target), summary["target_timestamp"])
	require.Equal(t, float64(100), summary["batch_limit"])
	require.Equal(t, float64(2), summary["matched_count_before"])
	require.Equal(t, float64(2), summary["deleted_count"])
	require.Equal(t, float64(0), summary["remaining_count_after"])
}
