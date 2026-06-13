package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

const routingCacheSelectionMissRefreshInterval = 2 * time.Second

type RoutingCacheRefreshOptions struct {
	Reason           string
	ResetProxyClient bool
	Force            bool
}

var (
	routingCacheRefreshMu              sync.Mutex
	routingCacheSelectionMissRefreshMu sync.Mutex
	routingCacheSelectionMissLast      time.Time
)

func init() {
	service.RegisterRoutingCacheRefreshHook(func(reason string, resetProxyClient bool) {
		RefreshDefaultRoutingCaches(RoutingCacheRefreshOptions{
			Reason:           reason,
			ResetProxyClient: resetProxyClient,
		})
	})
}

func RefreshDefaultRoutingCaches(options RoutingCacheRefreshOptions) {
	reason := strings.TrimSpace(options.Reason)
	if reason == "" {
		reason = "unspecified"
	}
	started := time.Now()

	routingCacheRefreshMu.Lock()
	defer routingCacheRefreshMu.Unlock()

	common.SysLog(fmt.Sprintf("model gateway routing cache refresh started: reason=%s reset_proxy_client=%t force=%t", reason, options.ResetProxyClient, options.Force))
	runRoutingCacheRefreshStep("channel_cache", func() error {
		if model.DB == nil {
			return nil
		}
		model.InitChannelCache()
		return nil
	})

	if options.ResetProxyClient {
		runRoutingCacheRefreshStep("proxy_client_cache", func() error {
			service.ResetProxyClientCache()
			return nil
		})
	}

	runtimeDeps := CurrentDefaultRuntimeObservabilityDeps()
	if runtimeDeps != nil && runtimeDeps.AccountCandidateIndex != nil {
		runRoutingCacheRefreshStep("account_candidate_index", func() error {
			runtimeDeps.AccountCandidateIndex.Refresh()
			return nil
		})
	}
	if runtimeDeps != nil && runtimeDeps.CostBaselineCache != nil {
		runRoutingCacheRefreshStep("cost_baseline_cache", func() error {
			return runtimeDeps.CostBaselineCache.Refresh(context.Background())
		})
	}

	common.SysLog(fmt.Sprintf("model gateway routing cache refresh finished: reason=%s duration_ms=%d", reason, time.Since(started).Milliseconds()))
}

func RefreshDefaultRoutingCachesForSelectionMiss(reason string) bool {
	now := time.Now()
	routingCacheSelectionMissRefreshMu.Lock()
	if !routingCacheSelectionMissLast.IsZero() && now.Sub(routingCacheSelectionMissLast) < routingCacheSelectionMissRefreshInterval {
		routingCacheSelectionMissRefreshMu.Unlock()
		return false
	}
	routingCacheSelectionMissLast = now
	routingCacheSelectionMissRefreshMu.Unlock()

	RefreshDefaultRoutingCaches(RoutingCacheRefreshOptions{
		Reason: strings.TrimSpace(reason),
		Force:  true,
	})
	return true
}

func ResetRoutingCacheSelectionMissRefreshForTest() {
	routingCacheSelectionMissRefreshMu.Lock()
	defer routingCacheSelectionMissRefreshMu.Unlock()
	routingCacheSelectionMissLast = time.Time{}
}

func runRoutingCacheRefreshStep(name string, step func() error) {
	if step == nil {
		return
	}
	started := time.Now()
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("model gateway routing cache refresh step panic: step=%s panic=%v duration_ms=%d", name, r, time.Since(started).Milliseconds()))
		}
	}()
	if err := step(); err != nil {
		common.SysLog(fmt.Sprintf("model gateway routing cache refresh step failed: step=%s error=%v duration_ms=%d", name, err, time.Since(started).Milliseconds()))
	}
}
