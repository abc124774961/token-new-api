package controller

import (
	"bytes"
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

type channelAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		ChannelIDs []int `json:"channel_ids"`
	} `json:"data"`
}

type updateChannelAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestAddEmptyCodexAccountPoolChannelAutoDisablesUntilImport(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	router := gin.New()
	router.POST("/api/channel/", AddChannel)
	router.PUT("/api/channel/", UpdateChannel)
	router.PUT("/api/channel/:id/accounts", ImportChannelAccounts)

	addBody, err := common.Marshal(gin.H{
		"mode": "single",
		"channel": gin.H{
			"name":     "empty codex",
			"type":     constant.ChannelTypeCodex,
			"key":      "",
			"models":   "codex-mini-latest",
			"group":    "default",
			"status":   common.ChannelStatusEnabled,
			"auto_ban": 1,
		},
	})
	require.NoError(t, err)
	addRecorder := httptest.NewRecorder()
	router.ServeHTTP(addRecorder, httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(addBody)))

	var addPayload channelAPIResponse
	require.NoError(t, common.Unmarshal(addRecorder.Body.Bytes(), &addPayload))
	require.True(t, addPayload.Success, addRecorder.Body.String())
	require.Len(t, addPayload.Data.ChannelIDs, 1)
	channelID := addPayload.Data.ChannelIDs[0]

	channel, err := model.GetChannelById(channelID, true)
	require.NoError(t, err)
	require.Equal(t, constant.ChannelTypeCodex, channel.Type)
	require.Empty(t, channel.Key)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
	require.Equal(t, channelAccountAllKeysDisabledReason, channel.GetOtherInfo()["status_reason"])
	require.False(t, channel.ChannelInfo.IsMultiKey)
	require.Equal(t, 0, channel.ChannelInfo.MultiKeySize)

	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", channelID).Error)
	require.False(t, ability.Enabled)

	enableBody, err := common.Marshal(gin.H{
		"id":     channelID,
		"status": common.ChannelStatusEnabled,
	})
	require.NoError(t, err)
	enableRecorder := httptest.NewRecorder()
	router.ServeHTTP(enableRecorder, httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(enableBody)))

	var enablePayload updateChannelAPIResponse
	require.NoError(t, common.Unmarshal(enableRecorder.Body.Bytes(), &enablePayload))
	require.False(t, enablePayload.Success, enableRecorder.Body.String())
	require.Contains(t, enablePayload.Message, "导入")

	channel, err = model.GetChannelById(channelID, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)

	credential := `{
		"access_token": "codex-access-token",
		"account_id": "acct_empty_pool",
		"email": "codex@example.com"
	}`
	importBody, err := common.Marshal(gin.H{
		"credentials": credential,
		"only_new":    true,
	})
	require.NoError(t, err)
	importRecorder := httptest.NewRecorder()
	router.ServeHTTP(importRecorder, httptest.NewRequest(http.MethodPut, "/api/channel/"+strconv.Itoa(channelID)+"/accounts", bytes.NewReader(importBody)))

	importPayload := decodeChannelAccountsResponse(t, importRecorder)
	require.True(t, importPayload.Success, importRecorder.Body.String())
	require.NotNil(t, importPayload.Data.Operation)
	require.True(t, importPayload.Data.Operation.ChannelRestored)
	require.Equal(t, 1, importPayload.Data.Total)
	require.Equal(t, 1, importPayload.Data.Enabled)

	channel, err = model.GetChannelById(channelID, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.NotEmpty(t, channel.Key)
	require.True(t, strings.Contains(channel.Key, `"access_token":"codex-access-token"`))
	require.Empty(t, channel.GetOtherInfo()["status_reason"])
	require.False(t, channel.ChannelInfo.IsMultiKey)
	require.Equal(t, 1, channel.ChannelInfo.MultiKeySize)
	require.NoError(t, db.First(&ability, "channel_id = ?", channelID).Error)
	require.True(t, ability.Enabled)
}

func TestAddSingleChannelAllowsEmptyKeyAndAutoDisables(t *testing.T) {
	db := setupChannelAccountControllerTestDB(t)
	router := gin.New()
	router.POST("/api/channel/", AddChannel)

	addBody, err := common.Marshal(gin.H{
		"mode": "single",
		"channel": gin.H{
			"name":     "empty openai",
			"type":     constant.ChannelTypeOpenAI,
			"key":      "",
			"models":   "gpt-5.5",
			"group":    "default",
			"status":   common.ChannelStatusEnabled,
			"auto_ban": 1,
		},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(addBody)))

	var payload channelAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.ChannelIDs, 1)

	channel, err := model.GetChannelById(payload.Data.ChannelIDs[0], true)
	require.NoError(t, err)
	require.Equal(t, constant.ChannelTypeOpenAI, channel.Type)
	require.Empty(t, channel.Key)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
	require.Equal(t, channelAccountAllKeysDisabledReason, channel.GetOtherInfo()["status_reason"])

	var ability model.Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", channel.Id).Error)
	require.False(t, ability.Enabled)
}

func TestAddBatchChannelStillRejectsEmptyKeys(t *testing.T) {
	setupChannelAccountControllerTestDB(t)
	router := gin.New()
	router.POST("/api/channel/", AddChannel)

	addBody, err := common.Marshal(gin.H{
		"mode": "batch",
		"channel": gin.H{
			"name":     "empty batch",
			"type":     constant.ChannelTypeOpenAI,
			"key":      "\n  \n",
			"models":   "gpt-5.5",
			"group":    "default",
			"status":   common.ChannelStatusEnabled,
			"auto_ban": 1,
		},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(addBody)))

	var payload channelAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "请填写渠道密钥")
}
