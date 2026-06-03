package core

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const retryRoutingIntentContextKey = "modelgateway_retry_routing_intent"
const resourceProtectionFallbackContextKey = "modelgateway_resource_protection_fallback"

type ResourceProtectionFallbackContext struct {
	Reason string
}

func SetRetryRoutingIntent(c *gin.Context, intent *RetryRoutingIntent) {
	if c == nil || intent == nil || !intent.Active() {
		return
	}
	c.Set(retryRoutingIntentContextKey, intent.Clone())
}

func ClearRetryRoutingIntent(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(retryRoutingIntentContextKey, nil)
}

func GetRetryRoutingIntent(c *gin.Context) (*RetryRoutingIntent, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(retryRoutingIntentContextKey)
	if !ok || value == nil {
		return nil, false
	}
	intent, ok := value.(*RetryRoutingIntent)
	return intent, ok && intent != nil && intent.Active()
}

func AllowResourceProtectionFallback(c *gin.Context, reason string) {
	if c == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = ResourceProtectionReasonPrimaryWaitTimeout
	}
	c.Set(resourceProtectionFallbackContextKey, ResourceProtectionFallbackContext{Reason: reason})
}

func ClearResourceProtectionFallback(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(resourceProtectionFallbackContextKey, nil)
}

func GetResourceProtectionFallback(c *gin.Context) (ResourceProtectionFallbackContext, bool) {
	if c == nil {
		return ResourceProtectionFallbackContext{}, false
	}
	value, ok := c.Get(resourceProtectionFallbackContextKey)
	if !ok || value == nil {
		return ResourceProtectionFallbackContext{}, false
	}
	ctx, ok := value.(ResourceProtectionFallbackContext)
	if !ok {
		return ResourceProtectionFallbackContext{}, false
	}
	ctx.Reason = strings.TrimSpace(ctx.Reason)
	if ctx.Reason == "" {
		ctx.Reason = ResourceProtectionReasonPrimaryWaitTimeout
	}
	return ctx, true
}
