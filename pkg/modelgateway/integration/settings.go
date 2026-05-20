package integration

import (
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type SchedulerSettingsProvider struct{}

type RuntimePolicySettings struct {
	QueueTimeoutMs                   int
	QueueMaxDepth                    int
	QueueDepthMultiplier             int
	QueueFairness                    core.QueueFairnessPolicySetting
	CircuitFailureThreshold          float64
	CircuitMinSamples                int
	CircuitOpenSeconds               int
	CircuitHalfOpenProbeCount        int
	CircuitErrorPolicies             map[string]scheduler.CircuitErrorPolicy
	StickyTTLSeconds                 int
	StickyKeepScoreRatio             float64
	StickySaveOnSelect               bool
	StickyRenewOnSuccess             bool
	StickyFailurePolicy              string
	CacheAffinityKeepScoreRatio      float64
	CacheAffinityEnabled             bool
	RuntimeSyncEnabled               bool
	RuntimeSyncRedisEnabled          bool
	RuntimeSyncNodeID                string
	RuntimeSyncTTLSeconds            int
	RuntimeSyncQueueMinInterval      int
	RuntimeSyncEventPushEnabled      bool
	RuntimeSyncEventSubscribeEnabled bool
}

func NewSchedulerSettingsProvider() *SchedulerSettingsProvider {
	return &SchedulerSettingsProvider{}
}

func RuntimePolicySetting() RuntimePolicySettings {
	setting := scheduler_setting.GetSetting()
	return RuntimePolicySettings{
		QueueTimeoutMs:       setting.QueueDefaultTimeoutMs,
		QueueMaxDepth:        setting.QueueMaxDepthPerChannel,
		QueueDepthMultiplier: setting.QueueDepthMultiplier,
		QueueFairness: core.QueueFairnessPolicySetting{
			HighPriorityGroups:        queueHighPriorityGroups(setting.GroupPolicies),
			HighPriorityThreshold:     setting.QueueHighPriorityThreshold,
			HighPriorityExtraDepth:    setting.QueueHighPriorityExtraDepth,
			HighPriorityReservedDepth: setting.QueueHighPriorityReservedDepth,
			AbsoluteMaxDepth:          setting.QueueAbsoluteMaxDepth,
		},
		CircuitFailureThreshold:          setting.CircuitFailureThreshold,
		CircuitMinSamples:                setting.CircuitMinSamples,
		CircuitOpenSeconds:               setting.CircuitOpenSeconds,
		CircuitHalfOpenProbeCount:        setting.CircuitHalfOpenProbeCount,
		CircuitErrorPolicies:             circuitErrorPolicies(setting.CircuitErrorPolicies),
		StickyTTLSeconds:                 setting.StickyTTLSeconds,
		StickyKeepScoreRatio:             setting.StickyKeepScoreRatio,
		StickySaveOnSelect:               setting.StickySaveOnSelect,
		StickyRenewOnSuccess:             setting.StickyRenewOnSuccess,
		StickyFailurePolicy:              setting.StickyFailurePolicy,
		CacheAffinityKeepScoreRatio:      setting.CacheAffinityKeepScoreRatio,
		CacheAffinityEnabled:             setting.CacheAffinityEnabled,
		RuntimeSyncEnabled:               setting.RuntimeSyncEnabled,
		RuntimeSyncRedisEnabled:          setting.RuntimeSyncRedisEnabled,
		RuntimeSyncNodeID:                setting.RuntimeSyncNodeID,
		RuntimeSyncTTLSeconds:            setting.RuntimeSyncTTLSeconds,
		RuntimeSyncQueueMinInterval:      setting.RuntimeSyncQueueMinIntervalMs,
		RuntimeSyncEventPushEnabled:      setting.RuntimeSyncEventPushEnabled,
		RuntimeSyncEventSubscribeEnabled: setting.RuntimeSyncEventSubscribeEnabled,
	}
}

func (p *SchedulerSettingsProvider) Get() core.SchedulerSettings {
	setting := scheduler_setting.GetSetting()
	policies := make(map[string]core.GroupPolicySetting, len(setting.GroupPolicies))
	for group, policy := range setting.GroupPolicies {
		policies[group] = core.GroupPolicySetting{
			Mode:                  policy.Mode,
			Strategy:              policy.Strategy,
			AutoMode:              policy.AutoMode,
			CrossGroupFusion:      policy.CrossGroupFusion,
			CandidateGroups:       append([]string(nil), policy.CandidateGroups...),
			CacheAffinityEnabled:  policy.CacheAffinityEnabled,
			QueueEnabled:          policy.QueueEnabled,
			QueueHighPriority:     policy.QueueHighPriority,
			CircuitBreakerEnabled: policy.CircuitBreakerEnabled,
		}
	}
	return core.SchedulerSettings{
		Enabled:              setting.Enabled,
		DefaultMode:          setting.DefaultMode,
		RolloutPercent:       setting.RolloutPercent,
		DefaultStrategy:      setting.DefaultStrategy,
		CacheAffinityEnabled: setting.CacheAffinityEnabled,
		QueueEnabled:         setting.QueueEnabled,
		QueueFairness: core.QueueFairnessPolicySetting{
			HighPriorityGroups:        queueHighPriorityGroups(setting.GroupPolicies),
			HighPriorityThreshold:     setting.QueueHighPriorityThreshold,
			HighPriorityExtraDepth:    setting.QueueHighPriorityExtraDepth,
			HighPriorityReservedDepth: setting.QueueHighPriorityReservedDepth,
			AbsoluteMaxDepth:          setting.QueueAbsoluteMaxDepth,
		},
		CircuitBreakerEnabled: setting.CircuitBreakerEnabled,
		GroupPolicies:         policies,
	}
}

func circuitErrorPolicies(policies map[string]scheduler_setting.CircuitErrorPolicySetting) map[string]scheduler.CircuitErrorPolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make(map[string]scheduler.CircuitErrorPolicy, len(policies))
	for kind, policy := range policies {
		out[kind] = scheduler.CircuitErrorPolicy{
			FailureThreshold:   policy.FailureThreshold,
			MinSamples:         policy.MinSamples,
			OpenDuration:       time.Duration(policy.OpenSeconds) * time.Second,
			HalfOpenProbeCount: policy.HalfOpenProbeCount,
		}
	}
	return out
}

func queueHighPriorityGroups(policies map[string]scheduler_setting.GroupPolicySetting) []string {
	if len(policies) == 0 {
		return nil
	}
	groups := make([]string, 0)
	for group, policy := range policies {
		if policy.QueueHighPriority {
			groups = append(groups, group)
		}
	}
	return groups
}
