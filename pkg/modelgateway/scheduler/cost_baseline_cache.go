package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
)

const defaultCostBaselineRefreshInterval = 500 * time.Millisecond

type CostBaselineCache struct {
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}

	mu        sync.RWMutex
	baselines map[core.CostBaselineScope]float64

	startOnce sync.Once
	closeOnce sync.Once
}

func NewCostBaselineCache(interval time.Duration) *CostBaselineCache {
	if interval <= 0 {
		interval = defaultCostBaselineRefreshInterval
	}
	return &CostBaselineCache{
		interval:  interval,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		baselines: make(map[core.CostBaselineScope]float64),
	}
}

func (c *CostBaselineCache) Start(ctx context.Context) {
	if c == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.startOnce.Do(func() {
		go c.run(ctx)
	})
}

func (c *CostBaselineCache) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		close(c.stop)
		<-c.done
	})
}

func (c *CostBaselineCache) Baseline(scope core.CostBaselineScope) (float64, bool) {
	if c == nil {
		return 0, false
	}
	scope = normalizeCostBaselineScope(scope)
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.baselines[scope]
	return value, ok && value > 0
}

func (c *CostBaselineCache) Refresh(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if model.DB == nil {
		return nil
	}
	bindings, err := model.ListEnabledChannelBindings()
	if err != nil {
		return err
	}
	next := buildCostBaselines(bindings)
	c.mu.Lock()
	c.baselines = next
	c.mu.Unlock()
	return nil
}

func (c *CostBaselineCache) run(ctx context.Context) {
	defer close(c.done)
	if err := c.Refresh(ctx); err != nil {
		common.SysLog(fmt.Sprintf("model gateway cost baseline refresh failed: %v", err))
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stop:
			return
		case <-ticker.C:
			if err := c.Refresh(ctx); err != nil {
				common.SysLog(fmt.Sprintf("model gateway cost baseline refresh failed: %v", err))
			}
		}
	}
}

func buildCostBaselines(bindings []model.EnabledChannelBinding) map[core.CostBaselineScope]float64 {
	if len(bindings) == 0 {
		return map[core.CostBaselineScope]float64{}
	}
	out := make(map[core.CostBaselineScope]float64)
	for _, binding := range bindings {
		channel := binding.Channel
		if channel == nil {
			continue
		}
		requestedModel := strings.TrimSpace(binding.Model)
		group := strings.TrimSpace(binding.Group)
		if requestedModel == "" || group == "" {
			continue
		}
		upstreamModel := channel.ResolveMappedModelName(requestedModel)
		ratio, ok := modelgatewaycost.CostRatioFromProfileForModel(modelgatewaycost.LookupCachedDefaultProfile(channel.Id, upstreamModel), upstreamModel)
		if !ok || ratio <= 0 {
			continue
		}
		endpointScopes := supportedEndpointScopes(channel, requestedModel)
		for _, endpointType := range endpointScopes {
			base := core.CostBaselineScope{
				RequestedModel: requestedModel,
				Group:          group,
				EndpointType:   endpointType,
			}
			storeCostBaseline(out, base, ratio)
		}
	}
	return out
}

func supportedEndpointScopes(channel *model.Channel, modelName string) []string {
	result := make([]string, 0, 4)
	add := func(endpointType constant.EndpointType) {
		value := strings.TrimSpace(string(endpointType))
		if value == "" {
			return
		}
		for _, existing := range result {
			if existing == value {
				return
			}
		}
		result = append(result, value)
	}
	for _, endpointType := range channelcapability.SupportedEndpointTypes(channel.Type, modelName, channel.GetOtherSettings()) {
		add(endpointType)
	}
	return result
}

func storeCostBaseline(target map[core.CostBaselineScope]float64, scope core.CostBaselineScope, ratio float64) {
	scope = normalizeCostBaselineScope(scope)
	if ratio <= 0 || scope.RequestedModel == "" || scope.Group == "" || scope.EndpointType == "" {
		return
	}
	current := target[scope]
	if current <= 0 || ratio < current {
		target[scope] = ratio
	}
}

func normalizeCostBaselineScope(scope core.CostBaselineScope) core.CostBaselineScope {
	scope.RequestedModel = strings.TrimSpace(scope.RequestedModel)
	scope.Group = strings.TrimSpace(scope.Group)
	scope.EndpointType = strings.TrimSpace(scope.EndpointType)
	if scope.EndpointType == "" {
		scope.EndpointType = string(constant.EndpointTypeOpenAI)
	}
	return scope
}

var _ core.CostBaselineProvider = (*CostBaselineCache)(nil)
