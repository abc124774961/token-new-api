package policy

import (
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
)

type DefaultGroupPolicyResolver struct {
	settings core.SchedulerSettingsProvider
}

func NewDefaultGroupPolicyResolver(settings core.SchedulerSettingsProvider) *DefaultGroupPolicyResolver {
	return &DefaultGroupPolicyResolver{settings: settings}
}

func (r *DefaultGroupPolicyResolver) Resolve(c *gin.Context, req *core.DispatchRequest) core.GroupSmartPolicy {
	if req == nil {
		req = &core.DispatchRequest{}
	}
	settings := core.SchedulerSettings{
		DefaultMode:     core.ModeOff,
		DefaultStrategy: core.StrategyBalanced,
	}
	if r != nil && r.settings != nil {
		settings = r.settings.Get()
	}
	if !settings.Enabled {
		return core.GroupSmartPolicy{
			RequestedGroup: req.RequestedGroup,
			UserGroup:      req.UserGroup,
			Mode:           core.ModeOff,
			Strategy:       normalizeStrategy(settings.DefaultStrategy),
			AutoMode:       core.AutoModeSequential,
		}
	}
	policySetting, ok := settings.GroupPolicies[req.RequestedGroup]
	if !ok {
		return core.GroupSmartPolicy{
			RequestedGroup: req.RequestedGroup,
			UserGroup:      req.UserGroup,
			Mode:           normalizeMode(settings.DefaultMode),
			Strategy:       normalizeStrategy(settings.DefaultStrategy),
			AutoMode:       core.AutoModeSequential,
		}
	}
	return core.GroupSmartPolicy{
		RequestedGroup:        req.RequestedGroup,
		UserGroup:             req.UserGroup,
		Mode:                  normalizeMode(policySetting.Mode),
		Strategy:              normalizeStrategyWithDefault(policySetting.Strategy, settings.DefaultStrategy),
		AutoMode:              normalizeAutoMode(policySetting.AutoMode),
		CrossGroupFusion:      policySetting.CrossGroupFusion,
		CandidateGroups:       append([]string(nil), policySetting.CandidateGroups...),
		CacheAffinityEnabled:  policySetting.CacheAffinityEnabled,
		QueueEnabled:          policySetting.QueueEnabled,
		QueueHighPriority:     policySetting.QueueHighPriority,
		QueuePriority:         queuePriorityForPolicy(req.RequestedGroup, policySetting, settings),
		CircuitBreakerEnabled: policySetting.CircuitBreakerEnabled,
	}
}

func queuePriorityForPolicy(group string, policySetting core.GroupPolicySetting, settings core.SchedulerSettings) int {
	if policySetting.QueueHighPriority {
		threshold := settings.QueueFairness.HighPriorityThreshold
		if threshold <= 0 {
			return 1
		}
		return threshold
	}
	return 0
}

func normalizeMode(mode string) string {
	switch mode {
	case core.ModeShadow, core.ModeActive:
		return mode
	default:
		return core.ModeOff
	}
}

func normalizeAutoMode(mode string) string {
	if mode == core.AutoModeFusion {
		return core.AutoModeFusion
	}
	return core.AutoModeSequential
}

func normalizeStrategy(strategy string) string {
	return normalizeStrategyWithDefault(strategy, core.StrategyBalanced)
}

func normalizeStrategyWithDefault(strategy, fallback string) string {
	switch strategy {
	case core.StrategyBalanced, core.StrategySpeedFirst, core.StrategyCostFirst, core.StrategyStabilityFirst:
		return strategy
	}
	if fallback == "" {
		return core.StrategyBalanced
	}
	return normalizeStrategy(fallback)
}
