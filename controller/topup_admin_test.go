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

func setupAdminTopUpControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldQuotaPerUnit := common.QuotaPerUnit
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.TopUp{},
		&model.AdminRole{},
		&model.AdminRolePermission{},
		&model.AdminUserRoleBinding{},
		&model.AdminUserPermissionOverride{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	common.QuotaPerUnit = 1000
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.QuotaPerUnit = oldQuotaPerUnit
		common.RedisEnabled = oldRedisEnabled
	})
	return db
}

func adminTopUpControllerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 100)
		c.Set("username", "finance-admin")
		c.Set("role", common.RoleAdminUser)
		c.Set("group", "default")
		c.Set(common.RequestIdKey, "admin-complete-topup-audit-test")
		c.Next()
	})
	router.GET("/api/user/topup", GetAllTopUps)
	router.POST("/api/user/topup/complete", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSettlementComplete), AdminCompleteTopUp)
	return router
}

func TestGetAllTopUpsIncludesUserDisplayInfo(t *testing.T) {
	db := setupAdminTopUpControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:          78,
		Username:    "settlement-user",
		DisplayName: "Settlement User",
		ContactName: "Settlement Contact",
		ContactQQ:   "123456",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
	}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId:        78,
		Amount:        10,
		Money:         10,
		TradeNo:       "settlement-list-001",
		PaymentMethod: "alipay",
		CreateTime:    1000,
		Status:        common.TopUpStatusSuccess,
	}).Error)

	router := adminTopUpControllerTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/user/topup?p=1&page_size=20", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				UserId          int    `json:"user_id"`
				UserUsername    string `json:"user_username"`
				UserDisplayName string `json:"user_display_name"`
				UserContactName string `json:"user_contact_name"`
				UserContactQQ   string `json:"user_contact_qq"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, 78, payload.Data.Items[0].UserId)
	require.Equal(t, "settlement-user", payload.Data.Items[0].UserUsername)
	require.Equal(t, "Settlement Contact", payload.Data.Items[0].UserDisplayName)
	require.Equal(t, "Settlement Contact", payload.Data.Items[0].UserContactName)
	require.Equal(t, "123456", payload.Data.Items[0].UserContactQQ)
}

func TestAdminCompleteTopUpAuditSummaryIncludesBusinessDelta(t *testing.T) {
	db := setupAdminTopUpControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       77,
		Username: "topup-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    500,
	}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId:          77,
		Amount:          3,
		Money:           3.5,
		TradeNo:         "manual-topup-001",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1000,
		Status:          common.TopUpStatusPending,
	}).Error)

	router := adminTopUpControllerTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/topup/complete", strings.NewReader(`{"trade_no":"manual-topup-001"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	var user model.User
	require.NoError(t, db.First(&user, 77).Error)
	require.Equal(t, 3500, user.Quota)

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "manual-topup-001").First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	require.NotZero(t, topUp.CompleteTime)

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "admin_complete_topup", summary["operation"])
	require.Equal(t, "manual-topup-001", summary["trade_no"])
	require.Equal(t, float64(topUp.Id), summary["topup_id"])
	require.Equal(t, float64(77), summary["target_user_id"])
	require.Equal(t, common.TopUpStatusPending, summary["status_before"])
	require.Equal(t, common.TopUpStatusSuccess, summary["status_after"])
	require.Equal(t, float64(0), summary["complete_time_before"])
	require.NotZero(t, summary["complete_time_after"])
	require.Equal(t, float64(500), summary["quota_before"])
	require.Equal(t, float64(3500), summary["quota_after"])
	require.Equal(t, float64(3000), summary["quota_delta"])
	require.Equal(t, float64(3), summary["amount"])
	require.Equal(t, 3.5, summary["money"])
	require.Equal(t, "alipay", summary["payment_method"])
	require.Equal(t, model.PaymentProviderEpay, summary["payment_provider"])
}
