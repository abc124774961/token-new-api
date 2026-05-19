package scheduler_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	ModeOff    = "off"
	ModeShadow = "shadow"
	ModeActive = "active"

	AutoModeSequential = "auto_sequential"
	AutoModeFusion     = "auto_fusion"

	StrategyBalanced       = "balanced"
	StrategySpeedFirst     = "speed_first"
	StrategyCostFirst      = "cost_first"
	StrategyStabilityFirst = "stability_first"
)

type GroupPolicySetting struct {
	Mode                  string   `json:"mode"`
	Strategy              string   `json:"strategy"`
	AutoMode              string   `json:"auto_mode"`
	CrossGroupFusion      bool     `json:"cross_group_fusion"`
	CandidateGroups       []string `json:"candidate_groups"`
	CacheAffinityEnabled  bool     `json:"cache_affinity_enabled"`
	QueueEnabled          bool     `json:"queue_enabled"`
	CircuitBreakerEnabled bool     `json:"circuit_breaker_enabled"`
}

type SchedulerSetting struct {
	Enabled                      bool                          `json:"enabled"`
	DefaultMode                  string                        `json:"default_mode"`
	RolloutPercent               int                           `json:"rollout_percent"`
	DefaultStrategy              string                        `json:"default_strategy"`
	SnapshotRefreshMs            int                           `json:"snapshot_refresh_ms"`
	StickyTTLSeconds             int                           `json:"sticky_ttl_seconds"`
	StickyKeepScoreRatio         float64                       `json:"sticky_keep_score_ratio"`
	CacheAffinityEnabled         bool                          `json:"cache_affinity_enabled"`
	CacheAffinityKeepScoreRatio  float64                       `json:"cache_affinity_keep_score_ratio"`
	QueueEnabled                 bool                          `json:"queue_enabled"`
	QueueDefaultTimeoutMs        int                           `json:"queue_default_timeout_ms"`
	QueueMaxDepthPerChannel      int                           `json:"queue_max_depth_per_channel"`
	QueueDepthMultiplier         int                           `json:"queue_depth_multiplier"`
	CircuitBreakerEnabled        bool                          `json:"circuit_breaker_enabled"`
	CircuitFailureThreshold      float64                       `json:"circuit_failure_threshold"`
	CircuitMinSamples            int                           `json:"circuit_min_samples"`
	CircuitOpenSeconds           int                           `json:"circuit_open_seconds"`
	CircuitHalfOpenProbeCount    int                           `json:"circuit_half_open_probe_count"`
	CooldownMaxSeconds           int                           `json:"cooldown_max_seconds"`
	SuccessWeight                float64                       `json:"success_weight"`
	SpeedWeight                  float64                       `json:"speed_weight"`
	LoadWeight                   float64                       `json:"load_weight"`
	CostWeight                   float64                       `json:"cost_weight"`
	GroupWeight                  float64                       `json:"group_weight"`
	GroupPriorityRatio           map[string]float64            `json:"group_priority_ratio"`
	GroupPolicies                map[string]GroupPolicySetting `json:"group_policies"`
	FailureFastWindowSeconds     int                           `json:"failure_fast_window_seconds"`
	FailureMainWindowSeconds     int                           `json:"failure_main_window_seconds"`
	FailureFallbackWindowSeconds int                           `json:"failure_fallback_window_seconds"`
}

var schedulerSetting = SchedulerSetting{
	Enabled:                      false,
	DefaultMode:                  ModeOff,
	RolloutPercent:               0,
	DefaultStrategy:              StrategyBalanced,
	SnapshotRefreshMs:            500,
	StickyTTLSeconds:             180,
	StickyKeepScoreRatio:         0.85,
	CacheAffinityEnabled:         true,
	CacheAffinityKeepScoreRatio:  0.75,
	QueueEnabled:                 true,
	QueueDefaultTimeoutMs:        2000,
	QueueMaxDepthPerChannel:      64,
	QueueDepthMultiplier:         2,
	CircuitBreakerEnabled:        true,
	CircuitFailureThreshold:      0.5,
	CircuitMinSamples:            10,
	CircuitOpenSeconds:           30,
	CircuitHalfOpenProbeCount:    3,
	CooldownMaxSeconds:           600,
	SuccessWeight:                0.32,
	SpeedWeight:                  0.28,
	LoadWeight:                   0.20,
	CostWeight:                   0.15,
	GroupWeight:                  0.05,
	GroupPriorityRatio:           map[string]float64{},
	GroupPolicies:                map[string]GroupPolicySetting{},
	FailureFastWindowSeconds:     60,
	FailureMainWindowSeconds:     300,
	FailureFallbackWindowSeconds: 1800,
}

func init() {
	config.GlobalConfig.Register("scheduler_setting", &schedulerSetting)
}

func DefaultSetting() SchedulerSetting {
	return defaultSchedulerSetting()
}

func GetSetting() SchedulerSetting {
	return schedulerSetting
}

func SetSetting(setting SchedulerSetting) {
	schedulerSetting = setting
}

func SetSettingForTest(setting SchedulerSetting) func() {
	old := schedulerSetting
	schedulerSetting = setting
	return func() {
		schedulerSetting = old
	}
}

func defaultSchedulerSetting() SchedulerSetting {
	setting := schedulerSetting
	setting.Enabled = false
	setting.DefaultMode = ModeOff
	setting.RolloutPercent = 0
	setting.DefaultStrategy = StrategyBalanced
	setting.SnapshotRefreshMs = 500
	setting.StickyTTLSeconds = 180
	setting.StickyKeepScoreRatio = 0.85
	setting.CacheAffinityEnabled = true
	setting.CacheAffinityKeepScoreRatio = 0.75
	setting.QueueEnabled = true
	setting.QueueDefaultTimeoutMs = 2000
	setting.QueueMaxDepthPerChannel = 64
	setting.QueueDepthMultiplier = 2
	setting.CircuitBreakerEnabled = true
	setting.CircuitFailureThreshold = 0.5
	setting.CircuitMinSamples = 10
	setting.CircuitOpenSeconds = 30
	setting.CircuitHalfOpenProbeCount = 3
	setting.CooldownMaxSeconds = 600
	setting.SuccessWeight = 0.32
	setting.SpeedWeight = 0.28
	setting.LoadWeight = 0.20
	setting.CostWeight = 0.15
	setting.GroupWeight = 0.05
	setting.GroupPriorityRatio = map[string]float64{}
	setting.GroupPolicies = map[string]GroupPolicySetting{}
	setting.FailureFastWindowSeconds = 60
	setting.FailureMainWindowSeconds = 300
	setting.FailureFallbackWindowSeconds = 1800
	return setting
}
