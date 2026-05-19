package integration

import (
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type SchedulerSettingsProvider struct{}

type RuntimePolicySettings struct {
	QueueTimeoutMs              int
	QueueMaxDepth               int
	QueueDepthMultiplier        int
	CircuitFailureThreshold     float64
	CircuitMinSamples           int
	CircuitOpenSeconds          int
	CircuitHalfOpenProbeCount   int
	StickyTTLSeconds            int
	StickyKeepScoreRatio        float64
	CacheAffinityKeepScoreRatio float64
	CacheAffinityEnabled        bool
}

func NewSchedulerSettingsProvider() *SchedulerSettingsProvider {
	return &SchedulerSettingsProvider{}
}

func RuntimePolicySetting() RuntimePolicySettings {
	setting := scheduler_setting.GetSetting()
	return RuntimePolicySettings{
		QueueTimeoutMs:              setting.QueueDefaultTimeoutMs,
		QueueMaxDepth:               setting.QueueMaxDepthPerChannel,
		QueueDepthMultiplier:        setting.QueueDepthMultiplier,
		CircuitFailureThreshold:     setting.CircuitFailureThreshold,
		CircuitMinSamples:           setting.CircuitMinSamples,
		CircuitOpenSeconds:          setting.CircuitOpenSeconds,
		CircuitHalfOpenProbeCount:   setting.CircuitHalfOpenProbeCount,
		StickyTTLSeconds:            setting.StickyTTLSeconds,
		StickyKeepScoreRatio:        setting.StickyKeepScoreRatio,
		CacheAffinityKeepScoreRatio: setting.CacheAffinityKeepScoreRatio,
		CacheAffinityEnabled:        setting.CacheAffinityEnabled,
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
			CircuitBreakerEnabled: policy.CircuitBreakerEnabled,
		}
	}
	return core.SchedulerSettings{
		Enabled:               setting.Enabled,
		DefaultMode:           setting.DefaultMode,
		RolloutPercent:        setting.RolloutPercent,
		DefaultStrategy:       setting.DefaultStrategy,
		CacheAffinityEnabled:  setting.CacheAffinityEnabled,
		QueueEnabled:          setting.QueueEnabled,
		CircuitBreakerEnabled: setting.CircuitBreakerEnabled,
		GroupPolicies:         policies,
	}
}
