package controller

import (
	"net/http"
	"net/http/httptest"
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

func setupAdminPermissionControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.AdminRole{},
		&model.AdminRolePermission{},
		&model.AdminUserRoleBinding{},
		&model.AdminUserPermissionOverride{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
	return db
}

func adminPermissionControllerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 100)
		c.Set("username", "root-admin")
		c.Set("role", common.RoleRootUser)
		c.Set("group", "default")
		c.Set(common.RequestIdKey, "admin-permission-audit-test")
		c.Next()
	})
	router.PUT("/api/admin/permissions/roles/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), UpdateAdminPermissionRole)
	router.PUT("/api/admin/permissions/users/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), UpdateAdminUserPermissionAssignment)
	return router
}

func latestAdminPermissionAuditSummary(t *testing.T, db *gorm.DB) map[string]interface{} {
	t.Helper()
	var log model.Log
	require.NoError(t, db.Order("id desc").First(&log).Error)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other), log.Other)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	summary, ok := adminInfo["summary"].(map[string]interface{})
	require.True(t, ok)
	return summary
}

func TestUpdateAdminPermissionRoleAuditSummaryIncludesPermissionDelta(t *testing.T) {
	db := setupAdminPermissionControllerTestDB(t)
	role, err := model.UpsertAdminRoleWithPermissions(model.AdminRole{
		Key:         "channel-admin",
		Name:        "渠道管理员",
		Code:        "Channel",
		Description: "before",
		Status:      common.UserStatusEnabled,
		SortOrder:   10,
	}, []string{
		middleware.AdminPermissionChannelAccountRead,
		middleware.AdminPermissionChannelChannelRead,
	})
	require.NoError(t, err)

	router := adminPermissionControllerTestRouter()
	body := `{
		"key":"channel-admin",
		"name":"渠道管理员",
		"code":"Channel",
		"description":"after",
		"status":1,
		"sort_order":20,
		"permissions":[
			"admin:channel:channel:read",
			"admin:channel:health:read"
		]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/permissions/roles/"+strconv.Itoa(role.Id), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "update_admin_role", summary["operation"])
	require.Equal(t, float64(role.Id), summary["role_id"])
	require.Equal(t, "channel-admin", summary["role_key"])
	require.Equal(t, "渠道管理员", summary["role_name"])
	require.Equal(t, float64(1), summary["status_before"])
	require.Equal(t, float64(1), summary["status_after"])
	require.Equal(t, float64(2), summary["permission_count_before"])
	require.Equal(t, float64(2), summary["permission_count_after"])
	require.Equal(t, float64(1), summary["permission_added_count"])
	require.Equal(t, float64(1), summary["permission_removed_count"])
}

func TestUpdateAdminUserPermissionAssignmentAuditSummaryIncludesDelta(t *testing.T) {
	db := setupAdminPermissionControllerTestDB(t)
	target := model.User{
		Id:       77,
		Username: "target-admin",
		Password: "password",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&target).Error)
	channelRole, err := model.UpsertAdminRoleWithPermissions(model.AdminRole{
		Key:    "channel-role",
		Name:   "渠道角色",
		Status: common.UserStatusEnabled,
	}, []string{middleware.AdminPermissionChannelChannelRead})
	require.NoError(t, err)
	modelRole, err := model.UpsertAdminRoleWithPermissions(model.AdminRole{
		Key:    "model-role",
		Name:   "模型角色",
		Status: common.UserStatusEnabled,
	}, []string{middleware.AdminPermissionModelGatewayRead})
	require.NoError(t, err)
	require.NoError(t, model.SetAdminUserPermissionAssignment(
		target.Id,
		[]int{channelRole.Id},
		[]string{middleware.AdminPermissionCommercialSettlementRead},
		[]string{middleware.AdminPermissionSystemRolesRead},
	))

	router := adminPermissionControllerTestRouter()
	body := `{
		"role_ids":[` + strconv.Itoa(modelRole.Id) + `],
		"allow_permissions":[
			"admin:commercial:profit:read",
			"admin:commercial:settlement:read"
		],
		"deny_permissions":["admin:channel:channel:read"]
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/permissions/users/77", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "update_admin_user_permissions", summary["operation"])
	require.Equal(t, float64(77), summary["target_user_id"])
	require.Equal(t, float64(1), summary["role_count_before"])
	require.Equal(t, float64(1), summary["role_count_after"])
	require.Equal(t, float64(1), summary["role_added_count"])
	require.Equal(t, float64(1), summary["role_removed_count"])
	require.Equal(t, float64(1), summary["allow_permission_count_before"])
	require.Equal(t, float64(2), summary["allow_permission_count_after"])
	require.Equal(t, float64(1), summary["allow_permission_added_count"])
	require.Equal(t, float64(0), summary["allow_permission_removed_count"])
	require.Equal(t, float64(1), summary["deny_permission_count_before"])
	require.Equal(t, float64(1), summary["deny_permission_count_after"])
	require.Equal(t, float64(1), summary["deny_permission_added_count"])
	require.Equal(t, float64(1), summary["deny_permission_removed_count"])
	require.Equal(t, float64(2), summary["effective_permission_count_before"])
	require.Equal(t, float64(3), summary["effective_permission_count_after"])
	require.Equal(t, float64(2), summary["effective_permission_added_count"])
	require.Equal(t, float64(1), summary["effective_permission_removed_count"])
}
