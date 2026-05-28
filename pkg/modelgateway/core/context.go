package core

import "github.com/gin-gonic/gin"

const retryRoutingIntentContextKey = "modelgateway_retry_routing_intent"

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
