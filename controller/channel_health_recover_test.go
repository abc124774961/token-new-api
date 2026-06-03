package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelResponseSurfacesRuntimeAccountBalanceInsufficient(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)

	channel := model.Channel{
		Type:   1,
		Name:   "runtime-balance-visible",
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
	}
	require.NoError(t, db.Create(&channel).Error)

	service.MarkChannelRuntimeBalanceInsufficient(service.ChannelRuntimeIdentity{
		ChannelID: channel.Id,
		AccountID: "acct-a",
	})
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficientForChannel(channel.Id)
	})

	resp := buildChannelResponseWithDisplays(&channel, nil, nil, service.RuntimeBalanceInsufficientCountForChannel(channel.Id))
	require.NotNil(t, resp)
	require.True(t, resp.BalanceInsufficient)
	require.Equal(t, 1, resp.RuntimeBalanceInsufficientCount)
}

func TestRecoverChannelHealthClearsRuntimeAndMultiKeyBalanceState(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	clearRuntimeBalanceInsufficientForControllerTest(t)

	channel := model.Channel{
		Type:   1,
		Name:   "recover-balance",
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusAutoDisabled,
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:             true,
			MultiKeySize:           2,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{0: service.ChannelStatusReasonBalanceInsufficient},
			MultiKeyDisabledTime:   map[int]int64{0: common.GetTimestamp()},
		},
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": channelBalanceAllAccountsDisabledReason,
		"status_time":   common.GetTimestamp(),
	})
	require.NoError(t, db.Create(&channel).Error)

	identity := service.ChannelRuntimeIdentity{
		ChannelID:          channel.Id,
		AccountID:          "acct-a",
		CredentialIndex:    0,
		CredentialIndexSet: true,
	}
	service.MarkChannelRuntimeBalanceInsufficient(identity)
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficientForChannel(channel.Id)
	})

	router := gin.New()
	router.POST("/api/channel/:id/recover_health", RecoverChannelHealth)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/"+strconv.Itoa(channel.Id)+"/recover_health", nil)
	router.ServeHTTP(resp, req)

	var payload struct {
		Success bool                         `json:"success"`
		Message string                       `json:"message"`
		Data    ChannelHealthRecoverResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success, payload.Message)
	require.Equal(t, 1, payload.Data.RuntimeBalanceCleared)
	require.Equal(t, 1, payload.Data.MultiKeyBalanceCleared)
	require.True(t, payload.Data.StatusUpdated)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.ChannelInfo.MultiKeyStatusList, 0)
	require.NotContains(t, updated.ChannelInfo.MultiKeyDisabledReason, 0)
	require.False(t, service.IsRuntimeBalanceInsufficientIdentity(identity))
}

func clearRuntimeBalanceInsufficientForControllerTest(t *testing.T) {
	t.Helper()
	service.ClearChannelBalanceInsufficientForChannel(1)
	service.ClearChannelBalanceInsufficientForChannel(2)
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficientForChannel(1)
		service.ClearChannelBalanceInsufficientForChannel(2)
	})
}
