package core

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func NewDispatchRequestFromGin(c *gin.Context, param *service.RetryParam) DispatchRequest {
	req := DispatchRequest{}
	if param != nil {
		req.RequestedGroup = param.TokenGroup
		req.ModelName = param.ModelName
		req.EndpointType = param.EndpointType
		req.RequiresCodexImageTool = param.RequiresCodexImageTool
		req.Retry = param.GetRetry()
		req.ExtraRetries = param.GetExtraRetries()
	}
	if req.EndpointType == "" {
		req.EndpointType = constant.EndpointTypeOpenAI
	}
	if c == nil {
		return req
	}
	req.RequestID = c.GetString(common.RequestIdKey)
	req.UserID = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	req.TokenID = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	req.UserGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	req.CrossGroupRetry = common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry)
	req.ForceNextAutoGroup = common.GetContextKeyBool(c, constant.ContextKeyForceNextAutoGroup)
	req.CurrentAutoGroup = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	if intent, ok := GetRetryRoutingIntent(c); ok {
		req.RetryRoutingIntent = intent.Clone()
	}
	if index, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex); ok {
		if idx, ok := index.(int); ok {
			req.CurrentAutoGroupIndex = idx
			req.HasCurrentAutoGroupIndex = true
		}
	}
	return req
}
