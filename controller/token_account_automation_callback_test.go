package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenAccountAutomationCallbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
	return db
}

func TestTokenAccountAutomationCredentialWritebackRequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(tokenAccountAutomationCallbackTokenEnv, "callback-token")

	router := gin.New()
	router.POST("/api/internal/token-account-automation/credential", TokenAccountAutomationCredentialWriteback)

	req := httptest.NewRequest(http.MethodPost, "/api/internal/token-account-automation/credential", bytes.NewBufferString(`{}`))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestTokenAccountAutomationCredentialWritebackEndpointUpdatesCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTokenAccountAutomationCallbackDB(t)
	t.Setenv(tokenAccountAutomationCallbackTokenEnv, "callback-token")
	channel := model.Channel{
		Id:     92001,
		Name:   "automation-callback",
		Status: common.ChannelStatusEnabled,
		Key:    "old-a\nold-b",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	router := gin.New()
	router.POST("/api/internal/token-account-automation/credential", TokenAccountAutomationCredentialWriteback)
	body, err := common.Marshal(map[string]any{
		"channel_id":       channel.Id,
		"credential_index": 1,
		"credential_type":  modelgatewaycore.AccountTypeAPIKey,
		"credential":       "new-b",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/token-account-automation/credential", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer callback-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			ChannelID       int    `json:"channel_id"`
			CredentialIndex int    `json:"credential_index"`
			AccountType     string `json:"account_type"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, channel.Id, payload.Data.ChannelID)
	require.Equal(t, 1, payload.Data.CredentialIndex)
	require.Equal(t, modelgatewaycore.AccountTypeAPIKey, payload.Data.AccountType)

	var updated model.Channel
	require.NoError(t, db.First(&updated, "id = ?", channel.Id).Error)
	require.Equal(t, []string{"old-a", "new-b"}, updated.GetKeys())
}
