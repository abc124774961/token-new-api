package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestListElectronBrowserAccountsRequiresTokenAndReturnsProxyRules(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	t.Setenv(tokenAccountAutomationCallbackTokenEnv, "callback-token")
	require.NoError(t, db.Create(&model.Channel{
		Id:     41,
		Name:   "codex desktop",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:       true,
			MultiKeySize:     2,
			MultiKeyProxyIDs: map[int]int{1: 901},
		},
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       901,
		Name:     "desktop socks",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Username: "user",
		Password: "pass",
		Enabled:  true,
	}).Error)

	router := gin.New()
	router.GET("/api/internal/electron-browser/accounts", ListElectronBrowserAccounts)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/internal/electron-browser/accounts", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/internal/electron-browser/accounts?page_size=20", nil)
	req.Header.Set("Authorization", "Bearer callback-token")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload struct {
		Success bool                            `json:"success"`
		Data    ElectronBrowserAccountsResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 2)
	require.Equal(t, "channel-41-credential-0", payload.Data.Items[0].ProfileKey)
	require.Equal(t, "https://chatgpt.com/", payload.Data.Items[0].OpenURL)
	require.Empty(t, payload.Data.Items[0].ProxyRules)
	require.Equal(t, "socks5://user:pass@127.0.0.1:1080", payload.Data.Items[1].ProxyRules)
	require.NotNil(t, payload.Data.Items[1].Proxy)
	require.Equal(t, "socks5://127.0.0.1:1080", payload.Data.Items[1].Proxy.MaskedAddress)
}

func TestListElectronBrowserAccountsAggregatesBeyondChannelAccountPageLimit(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	t.Setenv(tokenAccountAutomationCallbackTokenEnv, "callback-token")
	keys := make([]string, 0, channelAccountMaxPageSize+20)
	for i := 0; i < channelAccountMaxPageSize+20; i++ {
		keys = append(keys, "sk-page-test-"+strconv.Itoa(i))
	}
	require.NoError(t, db.Create(&model.Channel{
		Id:     42,
		Name:   "openai desktop bulk",
		Type:   constant.ChannelTypeOpenAI,
		Key:    strings.Join(keys, "\n"),
		Status: common.ChannelStatusEnabled,
		Models: "gpt-5.4",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: len(keys),
		},
	}).Error)

	router := gin.New()
	router.GET("/api/internal/electron-browser/accounts", ListElectronBrowserAccounts)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/internal/electron-browser/accounts?page_size=120", nil)
	req.Header.Set("Authorization", "Bearer callback-token")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload struct {
		Success bool                            `json:"success"`
		Data    ElectronBrowserAccountsResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, 120, payload.Data.Total)
	require.Len(t, payload.Data.Items, channelAccountMaxPageSize+20)
	require.Equal(t, "channel-42-credential-0", payload.Data.Items[0].ProfileKey)
	require.Equal(t, "channel-42-credential-119", payload.Data.Items[119].ProfileKey)
}
