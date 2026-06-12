package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingMultiplierControllerTest(t *testing.T) *gin.Engine {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.BillingMultiplierPolicy{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	router := gin.New()
	router.POST("/preview", PreviewBillingMultiplierPolicy)
	router.POST("/policies", CreateBillingMultiplierPolicy)
	return router
}

func TestPreviewBillingMultiplierPolicyDoesNotPersistTemporaryPolicy(t *testing.T) {
	router := setupBillingMultiplierControllerTest(t)
	body := []byte(`{
		"user_group":"vip",
		"using_group":"codex-plus",
		"model_name":"gpt-test",
		"base_group_ratio":1,
		"policy":{
			"name":"preview only",
			"enabled":true,
			"scope_type":"user_group",
			"scope_value":"vip",
			"mode":"multiply",
			"multiplier":0.8
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"final_group_ratio":0.8`)
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingMultiplierPolicy{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestPreviewBillingMultiplierPolicyReplacesEditingPolicy(t *testing.T) {
	router := setupBillingMultiplierControllerTest(t)
	require.NoError(t, model.DB.Create(&model.BillingMultiplierPolicy{
		Name:       "old",
		Enabled:    true,
		Priority:   10,
		ScopeType:  model.BillingMultiplierScopeUserGroup,
		ScopeValue: "vip",
		Mode:       model.BillingMultiplierModeMultiply,
		Multiplier: 0.5,
	}).Error)

	body := []byte(`{
		"user_group":"vip",
		"base_group_ratio":1,
		"policy":{
			"id":1,
			"name":"edited",
			"enabled":true,
			"priority":10,
			"scope_type":"user_group",
			"scope_value":"vip",
			"mode":"multiply",
			"multiplier":0.8
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"final_group_ratio":0.8`)
	require.NotContains(t, recorder.Body.String(), `"final_group_ratio":0.4`)
}
