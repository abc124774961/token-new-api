package core

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func NewDispatchRequestFromGin(c *gin.Context, param *service.RetryParam) DispatchRequest {
	req := DispatchRequest{}
	if param != nil {
		req.RequestedGroup = param.TokenGroup
		req.ModelName = param.ModelName
		req.EndpointType = param.EndpointType
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
	req.ClientSessionKey = SessionRoutingKeyFromGin(c)
	req.UserID = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	req.TokenID = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	req.UserGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	req.CrossGroupRetry = common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry)
	req.ForceNextAutoGroup = common.GetContextKeyBool(c, constant.ContextKeyForceNextAutoGroup)
	req.CurrentAutoGroup = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	req.RequiresResponsesPreviousID = common.GetContextKeyBool(c, constant.ContextKeyResponsesPreviousID)
	if intent, ok := GetRetryRoutingIntent(c); ok {
		req.RetryRoutingIntent = intent.Clone()
	}
	if fallback, ok := GetResourceProtectionFallback(c); ok {
		req.ResourceProtectionFallback = true
		req.ResourceProtectionReason = fallback.Reason
	}
	if index, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex); ok {
		if idx, ok := index.(int); ok {
			req.CurrentAutoGroupIndex = idx
			req.HasCurrentAutoGroupIndex = true
		}
	}
	return req
}

var sessionRoutingBodyKeyPaths = []string{
	"prompt_cache_key",
	"previous_response_id",
	"session_id",
	"sessionId",
	"session.id",
	"conversation_id",
	"conversationId",
	"conversation",
	"conversation.id",
	"chat_id",
	"chatId",
	"chat.id",
	"thread_id",
	"threadId",
	"thread.id",
	"parent_id",
	"parentId",
	"parent.id",
	"metadata.session_id",
	"metadata.sessionId",
	"metadata.session.id",
	"metadata.conversation_id",
	"metadata.conversationId",
	"metadata.conversation.id",
	"metadata.chat_id",
	"metadata.chatId",
	"metadata.chat.id",
	"metadata.thread_id",
	"metadata.threadId",
	"metadata.thread.id",
	"metadata.parent_id",
	"metadata.parentId",
	"metadata.parent.id",
	"extra_body.session_id",
	"extra_body.session.id",
	"extra_body.conversation_id",
	"extra_body.conversation.id",
	"extra_body.thread_id",
	"extra_body.thread.id",
}

var sessionRoutingHeaderKeys = []string{
	"Session_id",
	"Session-Id",
	"X-Session-Id",
	"X-Conversation-Id",
	"X-Thread-Id",
	"X-Chat-Id",
	"X-Parent-Id",
	"X-Codex-Session-Id",
	"X-Codex-Conversation-Id",
	"X-Codex-Thread-Id",
	"Mcp-Session-Id",
}

var sessionRoutingMetadataHeaderPaths = []string{
	"session_id",
	"sessionId",
	"conversation_id",
	"conversationId",
	"thread_id",
	"threadId",
	"chat_id",
	"chatId",
}

func SessionRoutingKeyFromGin(c *gin.Context) string {
	if source, value := SessionRoutingSignalFromGin(c); value != "" {
		return sessionRoutingKeyPart(source, value)
	}
	return ""
}

func SessionRoutingSignalFromGin(c *gin.Context) (string, string) {
	if source, value := sessionRoutingHeaderSignal(c); value != "" {
		return source, value
	}
	if source, value := sessionRoutingBodySignal(c); value != "" {
		return source, value
	}
	return "", ""
}

func sessionRoutingHeaderSignal(c *gin.Context) (string, string) {
	if c == nil || c.Request == nil {
		return "", ""
	}
	if source, value := sessionRoutingMetadataHeaderSignal(c.Request.Header); value != "" {
		return source, value
	}
	for _, header := range sessionRoutingHeaderKeys {
		if value := normalizeSessionRoutingSignalValue(c.GetHeader(header)); value != "" {
			return "header." + strings.ToLower(header), value
		}
	}
	return "", ""
}

func sessionRoutingMetadataHeaderSignal(header http.Header) (string, string) {
	if len(header) == 0 {
		return "", ""
	}
	raw := strings.TrimSpace(header.Get("X-Codex-Turn-Metadata"))
	if raw == "" {
		return "", ""
	}
	for _, path := range sessionRoutingMetadataHeaderPaths {
		result := gjson.Get(raw, path)
		if value := sessionRoutingJSONScalar(result); value != "" {
			return "header.x-codex-turn-metadata." + path, value
		}
	}
	return "", ""
}

func sessionRoutingBodySignal(c *gin.Context) (string, string) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return "", ""
	}
	contentType := c.Request.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "json") {
		return "", ""
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", ""
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return "", ""
	}
	for _, path := range sessionRoutingBodyKeyPaths {
		result := gjson.GetBytes(body, path)
		if value := sessionRoutingJSONScalar(result); value != "" {
			return "body." + path, value
		}
	}
	return "", ""
}

func sessionRoutingJSONScalar(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	switch result.Type {
	case gjson.String, gjson.Number:
		return normalizeSessionRoutingSignalValue(result.String())
	default:
		return ""
	}
}

func normalizeSessionRoutingSignalValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "auto", "none", "null", "undefined", "false", "true", "{}", "[]":
		return ""
	default:
		return value
	}
}

func sessionRoutingKeyPart(source string, value string) string {
	source = strings.TrimSpace(source)
	value = normalizeSessionRoutingSignalValue(value)
	if source == "" || value == "" {
		return ""
	}
	return "session:" + source + ":" + sessionRoutingFingerprint(value)
}

func sessionRoutingFingerprint(value string) string {
	sum := sha1.Sum([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if len(encoded) > 12 {
		return encoded[:12]
	}
	return encoded
}
