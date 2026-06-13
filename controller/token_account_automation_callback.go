package controller

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const tokenAccountAutomationCallbackTokenEnv = "TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN"

func TokenAccountAutomationCredentialWriteback(c *gin.Context) {
	if !requireTokenAccountAutomationCallbackToken(c) {
		return
	}
	var req service.TokenAccountAutomationCredentialWritebackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	result, err := service.WritebackTokenAccountAutomationCredential(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	clearChannelAccountRuntimeBlocks(result.ChannelID, result.CredentialIndex, true)
	modelgatewayintegration.RefreshDefaultRoutingCaches(modelgatewayintegration.RoutingCacheRefreshOptions{
		Reason:           "token_account_automation_writeback",
		ResetProxyClient: true,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func TokenAccountAutomationAccountProfile(c *gin.Context) {
	if !requireTokenAccountAutomationCallbackToken(c) {
		return
	}
	channelID, err := strconv.Atoi(strings.TrimSpace(c.Query("channel_id")))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel_id is required"})
		return
	}
	credentialIndex, err := strconv.Atoi(strings.TrimSpace(c.Query("credential_index")))
	if err != nil || credentialIndex < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "credential_index is required"})
		return
	}
	profile, err := buildTokenAccountAutomationAccountProfile(channelID, credentialIndex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

func TokenAccountAutomationArchiveInvalidAccount(c *gin.Context) {
	tokenAccountAutomationArchiveAccount(c, model.ChannelAccountPoolInvalid)
}

func TokenAccountAutomationArchiveDiscardedAccount(c *gin.Context) {
	tokenAccountAutomationArchiveAccount(c, model.ChannelAccountPoolDiscarded)
}

func TokenAccountAutomationListInvalidAccounts(c *gin.Context) {
	if !requireTokenAccountAutomationCallbackToken(c) {
		return
	}
	query := parseChannelAccountPoolQuery(c)
	records, total, err := model.ListChannelInvalidAccounts(query)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	items := make([]ChannelAccountPoolItem, 0, len(records))
	for _, record := range records {
		items = append(items, buildChannelAccountPoolItem(model.ChannelAccountPoolInvalid, record.ChannelAccountArchiveFields))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ChannelAccountPoolResponse{
		Page:          query.Page,
		PageSize:      query.PageSize,
		Total:         total,
		FilteredTotal: total,
		Channels:      listChannelAccountChannelItemsLite(),
		Items:         items,
	}})
}

func TokenAccountAutomationReauthorizeInvalidAccount(c *gin.Context) {
	if !requireTokenAccountAutomationCallbackToken(c) {
		return
	}
	poolID, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	var request RestoreChannelInvalidAccountRequest
	if c.Request.Body != nil {
		err := common.DecodeJson(c.Request.Body, &request)
		if err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	if !service.TokenAccountAutomationConfigured() {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "账号自动化服务未配置"})
		return
	}
	restored, err := restoreChannelInvalidAccountForReauthorization(poolID, request.ChannelID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	event := buildTokenAccountAutomationManualReauthorizationEvent(restored.Record, restored.ChannelID, restored.CredentialIndex, restored.PoolID, request.Reason)
	automation, err := service.EnqueueTokenAccountAutomationAuthInvalid(c.Request.Context(), event)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	modelgatewayintegration.RefreshDefaultRoutingCaches(modelgatewayintegration.RoutingCacheRefreshOptions{
		Reason: "token_account_automation_reauthorize",
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"operation":  restored.Operation,
		"automation": automation,
	}})
}

func tokenAccountAutomationArchiveAccount(c *gin.Context, pool string) {
	if !requireTokenAccountAutomationCallbackToken(c) {
		return
	}
	var req ArchiveChannelAccountsRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	operation, err := archiveChannelAccounts(pool, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	modelgatewayintegration.RefreshDefaultRoutingCaches(modelgatewayintegration.RoutingCacheRefreshOptions{
		Reason:           "token_account_automation_archive",
		ResetProxyClient: true,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"operation": operation}})
}

func requireTokenAccountAutomationCallbackToken(c *gin.Context) bool {
	expected := strings.TrimSpace(common.GetEnvOrDefaultString(tokenAccountAutomationCallbackTokenEnv, ""))
	if expected == "" {
		expected = strings.TrimSpace(common.GetEnvOrDefaultString("TOKEN_ACCOUNT_AUTOMATION_API_TOKEN", ""))
	}
	if expected == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "token account automation callback token is not configured"})
		return false
	}
	actual := strings.TrimSpace(c.GetHeader("Authorization"))
	actual = strings.TrimPrefix(actual, "Bearer ")
	if actual == "" {
		actual = strings.TrimSpace(c.GetHeader("X-Automation-Token"))
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid token"})
		return false
	}
	return true
}
