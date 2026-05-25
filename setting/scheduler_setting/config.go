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

	StickyFailurePolicyKeep  = "keep"
	StickyFailurePolicyClear = "clear"

	BillingRatioModeStatic  = "static"
	BillingRatioModeDynamic = "dynamic"
)

type GroupPolicySetting struct {
	Mode                  string   `json:"mode"`
	Strategy              string   `json:"strategy"`
	AutoMode              string   `json:"auto_mode"`
	CrossGroupFusion      bool     `json:"cross_group_fusion"`
	CandidateGroups       []string `json:"candidate_groups"`
	BillingRatioMode      string   `json:"billing_ratio_mode"`
	CacheAffinityEnabled  bool     `json:"cache_affinity_enabled"`
	QueueEnabled          bool     `json:"queue_enabled"`
	QueueHighPriority     bool     `json:"queue_high_priority"`
	CircuitBreakerEnabled bool     `json:"circuit_breaker_enabled"`
}

type CircuitErrorPolicySetting struct {
	FailureThreshold   float64 `json:"failure_threshold"`
	MinSamples         int     `json:"min_samples"`
	OpenSeconds        int     `json:"open_seconds"`
	HalfOpenProbeCount int     `json:"half_open_probe_count"`
}

type SchedulerSetting struct {
	Enabled                          bool                                 `json:"enabled"`
	DefaultMode                      string                               `json:"default_mode"`
	RolloutPercent                   int                                  `json:"rollout_percent"`
	DefaultStrategy                  string                               `json:"default_strategy"`
	SnapshotRefreshMs                int                                  `json:"snapshot_refresh_ms"`
	StickyTTLSeconds                 int                                  `json:"sticky_ttl_seconds"`
	StickyKeepScoreRatio             float64                              `json:"sticky_keep_score_ratio"`
	StickySaveOnSelect               bool                                 `json:"sticky_save_on_select"`
	StickyRenewOnSuccess             bool                                 `json:"sticky_renew_on_success"`
	StickyFailurePolicy              string                               `json:"sticky_failure_policy"`
	CacheAffinityEnabled             bool                                 `json:"cache_affinity_enabled"`
	CacheAffinityKeepScoreRatio      float64                              `json:"cache_affinity_keep_score_ratio"`
	QueueEnabled                     bool                                 `json:"queue_enabled"`
	QueueDefaultTimeoutMs            int                                  `json:"queue_default_timeout_ms"`
	QueueMaxDepthPerChannel          int                                  `json:"queue_max_depth_per_channel"`
	QueueDepthMultiplier             int                                  `json:"queue_depth_multiplier"`
	QueueHighPriorityThreshold       int                                  `json:"queue_high_priority_threshold"`
	QueueHighPriorityExtraDepth      int                                  `json:"queue_high_priority_extra_depth"`
	QueueHighPriorityReservedDepth   int                                  `json:"queue_high_priority_reserved_depth"`
	QueueAbsoluteMaxDepth            int                                  `json:"queue_absolute_max_depth"`
	CircuitBreakerEnabled            bool                                 `json:"circuit_breaker_enabled"`
	CircuitFailureThreshold          float64                              `json:"circuit_failure_threshold"`
	CircuitMinSamples                int                                  `json:"circuit_min_samples"`
	CircuitOpenSeconds               int                                  `json:"circuit_open_seconds"`
	CircuitHalfOpenProbeCount        int                                  `json:"circuit_half_open_probe_count"`
	CircuitErrorPolicies             map[string]CircuitErrorPolicySetting `json:"circuit_error_policies"`
	CooldownMaxSeconds               int                                  `json:"cooldown_max_seconds"`
	RuntimeSyncEnabled               bool                                 `json:"runtime_sync_enabled"`
	RuntimeSyncRedisEnabled          bool                                 `json:"runtime_sync_redis_enabled"`
	RuntimeSyncNodeID                string                               `json:"runtime_sync_node_id"`
	RuntimeSyncTTLSeconds            int                                  `json:"runtime_sync_ttl_seconds"`
	RuntimeSyncQueueMinIntervalMs    int                                  `json:"runtime_sync_queue_min_interval_ms"`
	RuntimeSyncEventPushEnabled      bool                                 `json:"runtime_sync_event_push_enabled"`
	RuntimeSyncEventSubscribeEnabled bool                                 `json:"runtime_sync_event_subscribe_enabled"`
	ProbeEnabled                     bool                                 `json:"probe_enabled"`
	ProbeIntervalSeconds             int                                  `json:"probe_interval_seconds"`
	ProbeWorkerCount                 int                                  `json:"probe_worker_count"`
	ProbeTimeoutSeconds              int                                  `json:"probe_timeout_seconds"`
	ProbeMaxPerTick                  int                                  `json:"probe_max_per_tick"`
	ProbeMinChannelIntervalSeconds   int                                  `json:"probe_min_channel_interval_seconds"`
	CostCalculationEnabled           bool                                 `json:"cost_calculation_enabled"`
	CostCalculationIntervalSeconds   int                                  `json:"cost_calculation_interval_seconds"`
	CostCalculationWorkerCount       int                                  `json:"cost_calculation_worker_count"`
	CostCalculationBatchSize         int                                  `json:"cost_calculation_batch_size"`
	DynamicBillingEnabled            bool                                 `json:"dynamic_billing_enabled"`
	DynamicBillingProfitRate         float64                              `json:"dynamic_billing_profit_rate"`
	DynamicBillingWindowSamples      int                                  `json:"dynamic_billing_window_samples"`
	DynamicBillingWindowMinutes      int                                  `json:"dynamic_billing_window_minutes"`
	DynamicBillingMinSamples         int                                  `json:"dynamic_billing_min_samples"`
	DynamicBillingRefreshSeconds     int                                  `json:"dynamic_billing_refresh_seconds"`
	DynamicBillingMaxAgeSeconds      int                                  `json:"dynamic_billing_max_age_seconds"`
	DynamicBillingEnabledAt          int64                                `json:"dynamic_billing_enabled_at"`
	SuccessWeight                    float64                              `json:"success_weight"`
	SpeedWeight                      float64                              `json:"speed_weight"`
	LoadWeight                       float64                              `json:"load_weight"`
	CostWeight                       float64                              `json:"cost_weight"`
	GroupWeight                      float64                              `json:"group_weight"`
	GroupPriorityRatio               map[string]float64                   `json:"group_priority_ratio"`
	GroupPolicies                    map[string]GroupPolicySetting        `json:"group_policies"`
	FailureFastWindowSeconds         int                                  `json:"failure_fast_window_seconds"`
	FailureMainWindowSeconds         int                                  `json:"failure_main_window_seconds"`
	FailureFallbackWindowSeconds     int                                  `json:"failure_fallback_window_seconds"`
}

var schedulerSetting = SchedulerSetting{
	Enabled:                          false,
	DefaultMode:                      ModeOff,
	RolloutPercent:                   0,
	DefaultStrategy:                  StrategyBalanced,
	SnapshotRefreshMs:                500,
	StickyTTLSeconds:                 180,
	StickyKeepScoreRatio:             0.85,
	StickySaveOnSelect:               false,
	StickyRenewOnSuccess:             true,
	StickyFailurePolicy:              StickyFailurePolicyClear,
	CacheAffinityEnabled:             true,
	CacheAffinityKeepScoreRatio:      0.75,
	QueueEnabled:                     true,
	QueueDefaultTimeoutMs:            2000,
	QueueMaxDepthPerChannel:          64,
	QueueDepthMultiplier:             2,
	QueueHighPriorityThreshold:       0,
	QueueHighPriorityExtraDepth:      0,
	QueueHighPriorityReservedDepth:   0,
	QueueAbsoluteMaxDepth:            0,
	CircuitBreakerEnabled:            true,
	CircuitFailureThreshold:          0.5,
	CircuitMinSamples:                10,
	CircuitOpenSeconds:               30,
	CircuitHalfOpenProbeCount:        3,
	CircuitErrorPolicies:             map[string]CircuitErrorPolicySetting{},
	CooldownMaxSeconds:               600,
	RuntimeSyncEnabled:               true,
	RuntimeSyncRedisEnabled:          true,
	RuntimeSyncNodeID:                "",
	RuntimeSyncTTLSeconds:            90,
	RuntimeSyncQueueMinIntervalMs:    500,
	RuntimeSyncEventPushEnabled:      false,
	RuntimeSyncEventSubscribeEnabled: false,
	ProbeEnabled:                     true,
	ProbeIntervalSeconds:             60,
	ProbeWorkerCount:                 2,
	ProbeTimeoutSeconds:              8,
	ProbeMaxPerTick:                  5,
	ProbeMinChannelIntervalSeconds:   300,
	CostCalculationEnabled:           true,
	CostCalculationIntervalSeconds:   5,
	CostCalculationWorkerCount:       2,
	CostCalculationBatchSize:         100,
	DynamicBillingEnabled:            false,
	DynamicBillingProfitRate:         0.20,
	DynamicBillingWindowSamples:      300,
	DynamicBillingWindowMinutes:      60,
	DynamicBillingMinSamples:         5,
	DynamicBillingRefreshSeconds:     30,
	DynamicBillingMaxAgeSeconds:      300,
	SuccessWeight:                    0.32,
	SpeedWeight:                      0.28,
	LoadWeight:                       0.20,
	CostWeight:                       0.15,
	GroupWeight:                      0.05,
	GroupPriorityRatio:               map[string]float64{},
	GroupPolicies:                    map[string]GroupPolicySetting{},
	FailureFastWindowSeconds:         60,
	FailureMainWindowSeconds:         300,
	FailureFallbackWindowSeconds:     1800,
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
	setting.StickySaveOnSelect = false
	setting.StickyRenewOnSuccess = true
	setting.StickyFailurePolicy = StickyFailurePolicyClear
	setting.CacheAffinityEnabled = true
	setting.CacheAffinityKeepScoreRatio = 0.75
	setting.QueueEnabled = true
	setting.QueueDefaultTimeoutMs = 2000
	setting.QueueMaxDepthPerChannel = 64
	setting.QueueDepthMultiplier = 2
	setting.QueueHighPriorityThreshold = 0
	setting.QueueHighPriorityExtraDepth = 0
	setting.QueueHighPriorityReservedDepth = 0
	setting.QueueAbsoluteMaxDepth = 0
	setting.CircuitBreakerEnabled = true
	setting.CircuitFailureThreshold = 0.5
	setting.CircuitMinSamples = 10
	setting.CircuitOpenSeconds = 30
	setting.CircuitHalfOpenProbeCount = 3
	setting.CircuitErrorPolicies = map[string]CircuitErrorPolicySetting{}
	setting.CooldownMaxSeconds = 600
	setting.RuntimeSyncEnabled = true
	setting.RuntimeSyncRedisEnabled = true
	setting.RuntimeSyncNodeID = ""
	setting.RuntimeSyncTTLSeconds = 90
	setting.RuntimeSyncQueueMinIntervalMs = 500
	setting.RuntimeSyncEventPushEnabled = false
	setting.RuntimeSyncEventSubscribeEnabled = false
	setting.ProbeEnabled = true
	setting.ProbeIntervalSeconds = 60
	setting.ProbeWorkerCount = 2
	setting.ProbeTimeoutSeconds = 8
	setting.ProbeMaxPerTick = 5
	setting.ProbeMinChannelIntervalSeconds = 300
	setting.CostCalculationEnabled = true
	setting.CostCalculationIntervalSeconds = 5
	setting.CostCalculationWorkerCount = 2
	setting.CostCalculationBatchSize = 100
	setting.DynamicBillingEnabled = false
	setting.DynamicBillingProfitRate = 0.20
	setting.DynamicBillingWindowSamples = 300
	setting.DynamicBillingWindowMinutes = 60
	setting.DynamicBillingMinSamples = 5
	setting.DynamicBillingRefreshSeconds = 30
	setting.DynamicBillingMaxAgeSeconds = 300
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
