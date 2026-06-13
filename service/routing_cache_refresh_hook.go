package service

import (
	"sync"

	"github.com/QuantumNous/new-api/model"
)

var routingCacheRefreshHookState struct {
	mu   sync.RWMutex
	hook func(reason string, resetProxyClient bool)
}

func RegisterRoutingCacheRefreshHook(hook func(reason string, resetProxyClient bool)) func() {
	routingCacheRefreshHookState.mu.Lock()
	previous := routingCacheRefreshHookState.hook
	routingCacheRefreshHookState.hook = hook
	routingCacheRefreshHookState.mu.Unlock()
	return func() {
		routingCacheRefreshHookState.mu.Lock()
		routingCacheRefreshHookState.hook = previous
		routingCacheRefreshHookState.mu.Unlock()
	}
}

func RefreshRoutingCachesAfterConfigChange(reason string, resetProxyClient bool) {
	routingCacheRefreshHookState.mu.RLock()
	hook := routingCacheRefreshHookState.hook
	routingCacheRefreshHookState.mu.RUnlock()
	if hook != nil {
		hook(reason, resetProxyClient)
		return
	}
	if model.DB != nil {
		model.InitChannelCache()
	}
	if resetProxyClient {
		ResetProxyClientCache()
	}
}
