package middleware

import (
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

func TestRequireAdminPermissionUsesLegacyRoleCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		role        int
		middleware  gin.HandlerFunc
		wantSuccess bool
	}{
		{
			name:        "admin passes default admin permission",
			role:        common.RoleAdminUser,
			middleware:  RequireAdminPermission(AdminPermissionCommercialSettlementComplete),
			wantSuccess: true,
		},
		{
			name:        "admin fails root permission",
			role:        common.RoleAdminUser,
			middleware:  RequireRootAdminPermission(AdminPermissionSystemSettingsUpdate),
			wantSuccess: false,
		},
		{
			name:        "root passes root permission",
			role:        common.RoleRootUser,
			middleware:  RequireRootAdminPermission(AdminPermissionSystemSettingsUpdate),
			wantSuccess: true,
		},
		{
			name:        "missing role fails default admin permission",
			role:        common.RoleGuestUser,
			middleware:  RequireAdminPermission(AdminPermissionUserUserDanger),
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/guarded", func(c *gin.Context) {
				if tt.role != common.RoleGuestUser {
					c.Set("role", tt.role)
				}
				c.Next()
			}, tt.middleware, func(c *gin.Context) {
				require.NotEmpty(t, c.GetString("admin_permission"))
				common.ApiSuccess(c, gin.H{"ok": true})
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/guarded", nil)
			router.ServeHTTP(recorder, request)

			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
			require.Equal(t, tt.wantSuccess, body["success"])
		})
	}
}

func TestRequireAdminPermissionRecordsAuditLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
	})

	router := gin.New()
	router.POST("/guarded/:id", func(c *gin.Context) {
		c.Set("id", 39)
		c.Set("username", "admin-user")
		c.Set("role", common.RoleAdminUser)
		c.Set("group", "default")
		c.Set(common.RequestIdKey, "audit-request-id")
		c.Next()
	}, RequireAdminPermission(AdminPermissionCommercialSubscriptionUpdate), func(c *gin.Context) {
		var body map[string]interface{}
		require.NoError(t, common.DecodeJson(c.Request.Body, &body))
		require.Equal(t, "T123", body["trade_no"])
		common.ApiSuccess(c, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/guarded/42?dry_run=true&unused=1", strings.NewReader(`{"trade_no":"T123","value":"do-not-record","channel_id":7}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	var log model.Log
	require.NoError(t, db.First(&log).Error)
	require.Equal(t, model.LogTypeManage, log.Type)
	require.Equal(t, 39, log.UserId)
	require.Equal(t, "admin-user", log.Username)
	require.Equal(t, "audit-request-id", log.RequestId)
	require.Contains(t, log.Content, AdminPermissionCommercialSubscriptionUpdate)

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other), log.Other)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, AdminPermissionCommercialSubscriptionUpdate, adminInfo["permission"])
	require.Equal(t, "completed", adminInfo["result"])
	require.Equal(t, "POST", adminInfo["method"])
	require.Equal(t, "/guarded/:id", adminInfo["route"])
	require.Equal(t, "audit-request-id", adminInfo["request_id"])
	require.Equal(t, float64(common.RoleAdminUser), adminInfo["role"])
	target, ok := adminInfo["target"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "42", target["id"])
	queryKeys, ok := adminInfo["query_keys"].([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"dry_run", "unused"}, queryKeys)
	summary, ok := adminInfo["summary"].(map[string]interface{})
	require.True(t, ok)
	bodyKeys, ok := summary["body_keys"].([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"channel_id", "trade_no", "value"}, bodyKeys)
	bodyIdentifiers, ok := summary["body_identifiers"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, float64(7), bodyIdentifiers["channel_id"])
	require.Equal(t, "T123", bodyIdentifiers["trade_no"])
	require.NotContains(t, bodyIdentifiers, "value")
}

func TestAdminPermissionCatalogAndRoleCompatibility(t *testing.T) {
	catalog := GetAdminPermissionCatalog()
	require.Len(t, catalog.RoleTemplates, 5)
	require.Len(t, catalog.MenuPermissions, 26)
	require.Len(t, catalog.DangerousOperationPermissions, 10)
	require.Len(t, catalog.OperationPermissions, 8)

	adminPermissions := GetAdminPermissionsForRole(common.RoleAdminUser)
	require.Contains(t, adminPermissions, AdminPermissionChannelChannelRead)
	require.Contains(t, adminPermissions, AdminPermissionChannelChannelUpdate)
	require.Contains(t, adminPermissions, AdminPermissionCommercialSettlementComplete)
	require.NotContains(t, adminPermissions, AdminPermissionSystemSettingsRead)
	require.NotContains(t, adminPermissions, AdminPermissionModelRatioUpdate)

	rootPermissions := GetAdminPermissionsForRole(common.RoleRootUser)
	require.Equal(t, []string{"*"}, rootPermissions)
	require.Nil(t, GetAdminPermissionsForRole(common.RoleCommonUser))
}

func TestRequireAdminPermissionUsesStoredAssignmentsWhenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AdminRole{}, &model.AdminRolePermission{}, &model.AdminUserRoleBinding{}, &model.AdminUserPermissionOverride{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})

	role, err := model.UpsertAdminRoleWithPermissions(model.AdminRole{
		Key:  "channel-readonly",
		Name: "渠道只读",
	}, []string{AdminPermissionChannelAccountRead})
	require.NoError(t, err)
	require.NoError(t, model.SetAdminUserPermissionAssignment(39, []int{role.Id}, nil, nil))

	router := gin.New()
	router.POST("/guarded", func(c *gin.Context) {
		c.Set("id", 39)
		c.Set("username", "admin-user")
		c.Set("role", common.RoleAdminUser)
		c.Next()
	}, RequireAdminPermission(AdminPermissionChannelChannelUpdate), func(c *gin.Context) {
		common.ApiSuccess(c, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/guarded", nil)
	router.ServeHTTP(recorder, request)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, false, body["success"])

	require.NoError(t, model.SetAdminUserPermissionAssignment(39, []int{role.Id}, []string{AdminPermissionChannelChannelUpdate}, nil))
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/guarded", nil)
	router.ServeHTTP(recorder, request)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
}
