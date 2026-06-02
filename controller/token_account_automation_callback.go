package controller

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
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
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
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
