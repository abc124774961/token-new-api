package controller

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

func setupUserContactControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	common.RedisEnabled = false
	common.UsingSQLite = true
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
	})
	return db
}

func TestUpdateSelfContactProfileDoesNotRequirePassword(t *testing.T) {
	db := setupUserContactControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("old-password")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id:          88,
		Username:    "self-user",
		Password:    hashedPassword,
		DisplayName: "Self User",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 88)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(`{
		"contact_name":"新昵称",
		"contact_email":"contact@example.com",
		"contact_qq":"987654321",
		"contact_other":"wechat:abc"
	}`))

	UpdateSelf(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.True(t, payload.Success, payload.Message)

	var updated model.User
	require.NoError(t, db.First(&updated, 88).Error)
	require.Equal(t, "新昵称", updated.ContactName)
	require.Equal(t, "contact@example.com", updated.ContactEmail)
	require.Equal(t, "987654321", updated.ContactQQ)
	require.Equal(t, "wechat:abc", updated.ContactOther)
	require.Equal(t, "self-user", updated.Username)
	require.True(t, common.ValidatePasswordAndHash("old-password", updated.Password))
}
