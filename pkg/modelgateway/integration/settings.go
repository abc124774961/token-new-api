package integration

import (
	"os"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type SchedulerSettingsProvider struct{}

type RuntimePolicySettings struct {
	SnapshotRefreshMs                int
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
	CostFirstStickyEscape            scheduler.CostFirstStickyEscapeConfig
	RuntimeSyncEnabled               bool
	RuntimeSyncRedisEnabled          bool
	RuntimeSyncNodeID                string
	RuntimeSyncTTLSeconds            int
	RuntimeSyncQueueMinInterval      int
	RuntimeSyncEventPushEnabled      bool
	RuntimeSyncEventSubscribeEnabled bool
	AccountCandidateIndexEnabled     bool
	AccountCandidateIndexShadowLog   bool
	AccountCandidateIndexRefreshMs   int
	ScoreWeights                     core.ScoreWeights
}

func NewSchedulerSettingsProvider() *SchedulerSettingsProvider {
	return &SchedulerSettingsProvider{}
}

func RuntimePolicySetting() RuntimePolicySettings {
	setting := scheduler_setting.GetSetting()
	return RuntimePolicySettings{
		SnapshotRefreshMs:    setting.SnapshotRefreshMs,
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
		CircuitFailureThreshold:     setting.CircuitFailureThreshold,
		CircuitMinSamples:           setting.CircuitMinSamples,
		CircuitOpenSeconds:          setting.CircuitOpenSeconds,
		CircuitHalfOpenProbeCount:   setting.CircuitHalfOpenProbeCount,
		CircuitErrorPolicies:        circuitErrorPolicies(setting.CircuitErrorPolicies),
		StickyTTLSeconds:            setting.StickyTTLSeconds,
		StickyKeepScoreRatio:        setting.StickyKeepScoreRatio,
		StickySaveOnSelect:          setting.StickySaveOnSelect,
		StickyRenewOnSuccess:        setting.StickyRenewOnSuccess,
		StickyFailurePolicy:         setting.StickyFailurePolicy,
		CacheAffinityKeepScoreRatio: setting.CacheAffinityKeepScoreRatio,
		CacheAffinityEnabled:        setting.CacheAffinityEnabled,
		CostFirstStickyEscape: scheduler.CostFirstStickyEscapeConfig{
			Enabled:        setting.CostFirstStickyEscapeEnabled,
			CostRatio:      setting.CostFirstStickyEscapeCostRatio,
			CacheCostRatio: setting.CostFirstStickyEscapeCacheCostRatio,
			MaxSpeedDrop:   setting.CostFirstStickyEscapeMaxSpeedDrop,
			CacheSpeedDrop: setting.CostFirstStickyEscapeCacheSpeedDrop,
			MinSamples:     setting.CostFirstStickyEscapeMinSamples,
			SuccessSlack:   setting.CostFirstStickyEscapeSuccessSlack,
		},
		RuntimeSyncEnabled:               setting.RuntimeSyncEnabled,
		RuntimeSyncRedisEnabled:          setting.RuntimeSyncRedisEnabled,
		RuntimeSyncNodeID:                setting.RuntimeSyncNodeID,
		RuntimeSyncTTLSeconds:            setting.RuntimeSyncTTLSeconds,
		RuntimeSyncQueueMinInterval:      setting.RuntimeSyncQueueMinIntervalMs,
		RuntimeSyncEventPushEnabled:      setting.RuntimeSyncEventPushEnabled,
		RuntimeSyncEventSubscribeEnabled: setting.RuntimeSyncEventSubscribeEnabled,
		AccountCandidateIndexEnabled:     boolEnv("MODEL_GATEWAY_ACCOUNT_CANDIDATE_INDEX_ENABLED", true),
		AccountCandidateIndexShadowLog:   boolEnv("MODEL_GATEWAY_ACCOUNT_CANDIDATE_INDEX_SHADOW_LOG", false),
		AccountCandidateIndexRefreshMs:   intEnv("MODEL_GATEWAY_ACCOUNT_CANDIDATE_INDEX_REFRESH_MS", 30000),
		ScoreWeights: core.ScoreWeights{
			Success: setting.SuccessWeight,
			Speed:   setting.SpeedWeight,
			Load:    setting.LoadWeight,
			Cost:    setting.CostWeight,
			Group:   setting.GroupWeight,
		},
	}
}

func boolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func intEnv(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
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
			BillingRatioMode:      policy.BillingRatioMode,
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
		GroupPriorityRatio:    copyFloatMap(setting.GroupPriorityRatio),
		GroupPolicies:         policies,
	}
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
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
