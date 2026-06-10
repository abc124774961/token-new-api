package scheduler_setting

import (
	"sync"

	"github.com/QuantumNous/new-api/setting/config"
)

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

	DynamicBillingCostSourceSampleCost = "sample_cost"
	DynamicBillingCostSourceProfit24h  = "profit_24h"

	DynamicBillingApplyModeObserve = "observe"
	DynamicBillingApplyModeManual  = "manual"
	DynamicBillingApplyModeAuto    = "auto"

	ProxyReusePolicyWarn    = "warn"
	ProxyReusePolicyConfirm = "confirm"
	ProxyReusePolicyBlock   = "block"
)

type GroupPolicySetting struct {
	Mode                      string   `json:"mode"`
	Strategy                  string   `json:"strategy"`
	AutoMode                  string   `json:"auto_mode"`
	CrossGroupFusion          bool     `json:"cross_group_fusion"`
	CandidateGroups           []string `json:"candidate_groups"`
	BillingRatioMode          string   `json:"billing_ratio_mode"`
	CacheAffinityEnabled      bool     `json:"cache_affinity_enabled"`
	QueueEnabled              bool     `json:"queue_enabled"`
	QueueHighPriority         bool     `json:"queue_high_priority"`
	CircuitBreakerEnabled     bool     `json:"circuit_breaker_enabled"`
	ResourceProtectionEnabled bool     `json:"resource_protection_enabled"`
	PrimaryChannelIDs         []int    `json:"primary_channel_ids"`
	PrimaryWaitTimeoutMs      int      `json:"primary_wait_timeout_ms"`
	PrimaryQueueMaxDepth      int      `json:"primary_queue_max_depth"`
	FallbackChannelIDs        []int    `json:"fallback_channel_ids"`
}

type CircuitErrorPolicySetting struct {
	FailureThreshold   float64 `json:"failure_threshold"`
	MinSamples         int     `json:"min_samples"`
	OpenSeconds        int     `json:"open_seconds"`
	HalfOpenProbeCount int     `json:"half_open_probe_count"`
}

type SchedulerSetting struct {
	Enabled                              bool                                 `json:"enabled"`
	DefaultMode                          string                               `json:"default_mode"`
	RolloutPercent                       int                                  `json:"rollout_percent"`
	DefaultStrategy                      string                               `json:"default_strategy"`
	SnapshotRefreshMs                    int                                  `json:"snapshot_refresh_ms"`
	StickyTTLSeconds                     int                                  `json:"sticky_ttl_seconds"`
	StickyKeepScoreRatio                 float64                              `json:"sticky_keep_score_ratio"`
	StickySaveOnSelect                   bool                                 `json:"sticky_save_on_select"`
	StickyRenewOnSuccess                 bool                                 `json:"sticky_renew_on_success"`
	StickyFailurePolicy                  string                               `json:"sticky_failure_policy"`
	CacheAffinityEnabled                 bool                                 `json:"cache_affinity_enabled"`
	CacheAffinityKeepScoreRatio          float64                              `json:"cache_affinity_keep_score_ratio"`
	CostFirstStickyEscapeEnabled         bool                                 `json:"cost_first_sticky_escape_enabled"`
	CostFirstStickyEscapeCostRatio       float64                              `json:"cost_first_sticky_escape_cost_ratio"`
	CostFirstStickyEscapeCacheCostRatio  float64                              `json:"cost_first_sticky_escape_cache_cost_ratio"`
	CostFirstStickyEscapeMaxSpeedDrop    float64                              `json:"cost_first_sticky_escape_max_speed_score_drop"`
	CostFirstStickyEscapeCacheSpeedDrop  float64                              `json:"cost_first_sticky_escape_cache_max_speed_score_drop"`
	CostFirstStickyEscapeMinSamples      int                                  `json:"cost_first_sticky_escape_min_samples"`
	CostFirstStickyEscapeSuccessSlack    float64                              `json:"cost_first_sticky_escape_success_slack"`
	CostFirstGuardEnabled                bool                                 `json:"cost_first_guard_enabled"`
	CostFirstGuardMultiple               float64                              `json:"cost_first_guard_multiple"`
	CostFirstGuardSuccessAdvantage       float64                              `json:"cost_first_guard_success_advantage"`
	CostFirstGuardSpeedAdvantage         float64                              `json:"cost_first_guard_speed_advantage"`
	ChannelPriorityTieBreakEnabled       bool                                 `json:"channel_priority_tie_break_enabled"`
	ChannelPriorityTieBreakScoreDelta    float64                              `json:"channel_priority_tie_break_score_delta"`
	QueueEnabled                         bool                                 `json:"queue_enabled"`
	QueueDefaultTimeoutMs                int                                  `json:"queue_default_timeout_ms"`
	QueueMaxDepthPerChannel              int                                  `json:"queue_max_depth_per_channel"`
	QueueDepthMultiplier                 int                                  `json:"queue_depth_multiplier"`
	QueueHighPriorityThreshold           int                                  `json:"queue_high_priority_threshold"`
	QueueHighPriorityExtraDepth          int                                  `json:"queue_high_priority_extra_depth"`
	QueueHighPriorityReservedDepth       int                                  `json:"queue_high_priority_reserved_depth"`
	QueueAbsoluteMaxDepth                int                                  `json:"queue_absolute_max_depth"`
	CircuitBreakerEnabled                bool                                 `json:"circuit_breaker_enabled"`
	CircuitFailureThreshold              float64                              `json:"circuit_failure_threshold"`
	CircuitMinSamples                    int                                  `json:"circuit_min_samples"`
	CircuitOpenSeconds                   int                                  `json:"circuit_open_seconds"`
	CircuitHalfOpenProbeCount            int                                  `json:"circuit_half_open_probe_count"`
	CircuitErrorPolicies                 map[string]CircuitErrorPolicySetting `json:"circuit_error_policies"`
	CooldownMaxSeconds                   int                                  `json:"cooldown_max_seconds"`
	RuntimeSyncEnabled                   bool                                 `json:"runtime_sync_enabled"`
	RuntimeSyncRedisEnabled              bool                                 `json:"runtime_sync_redis_enabled"`
	RuntimeSyncNodeID                    string                               `json:"runtime_sync_node_id"`
	RuntimeSyncTTLSeconds                int                                  `json:"runtime_sync_ttl_seconds"`
	RuntimeSyncQueueMinIntervalMs        int                                  `json:"runtime_sync_queue_min_interval_ms"`
	RuntimeSyncEventPushEnabled          bool                                 `json:"runtime_sync_event_push_enabled"`
	RuntimeSyncEventSubscribeEnabled     bool                                 `json:"runtime_sync_event_subscribe_enabled"`
	ProbeEnabled                         bool                                 `json:"probe_enabled"`
	ProbeIntervalSeconds                 int                                  `json:"probe_interval_seconds"`
	ProbeWorkerCount                     int                                  `json:"probe_worker_count"`
	ProbeTimeoutSeconds                  int                                  `json:"probe_timeout_seconds"`
	ProbeMaxPerTick                      int                                  `json:"probe_max_per_tick"`
	ProbeMinChannelIntervalSeconds       int                                  `json:"probe_min_channel_interval_seconds"`
	ProbeLowScoreThreshold               float64                              `json:"probe_low_score_threshold"`
	ProbeMissingSampleThreshold          int                                  `json:"probe_missing_sample_threshold"`
	ProbeLongNoSuccessSeconds            int                                  `json:"probe_long_no_success_seconds"`
	ProbeRecoverySuccessesRequired       int                                  `json:"probe_recovery_successes_required"`
	ProbeFailureAvoidancePriorityEnabled bool                                 `json:"probe_failure_avoidance_priority_enabled"`
	ProbeRecoverableScoreItems           []string                             `json:"probe_recoverable_score_items"`
	ProbeSkipRecentRealRequestEnabled    bool                                 `json:"probe_skip_recent_real_request_enabled"`
	ProbeRecentRealRequestWindowSeconds  int                                  `json:"probe_recent_real_request_window_seconds"`
	ProbeGoodBaselineEnabled             bool                                 `json:"probe_good_baseline_enabled"`
	ProbeGoodBaselineMinSamples          int                                  `json:"probe_good_baseline_min_samples"`
	ProbeGoodBaselineWindowSeconds       int                                  `json:"probe_good_baseline_window_seconds"`
	ProbePromptLibraryEnabled            bool                                 `json:"probe_prompt_library_enabled"`
	ProbePromptCategories                []string                             `json:"probe_prompt_categories"`
	RelayTotalTimeoutEnabled             bool                                 `json:"relay_total_timeout_enabled"`
	RelayTotalTimeoutSeconds             int                                  `json:"relay_total_timeout_seconds"`
	ChannelTimeoutDegradeEnabled         bool                                 `json:"channel_timeout_degrade_enabled"`
	ChannelTimeoutDegradeWindowSeconds   int                                  `json:"channel_timeout_degrade_window_seconds"`
	ChannelTimeoutDegradeMinSamples      int                                  `json:"channel_timeout_degrade_min_samples"`
	ChannelTimeoutDegradeThreshold       float64                              `json:"channel_timeout_degrade_threshold"`
	ChannelTimeoutDegradeConsecutive     int                                  `json:"channel_timeout_degrade_consecutive"`
	ChannelTimeoutRecoveryProbeSuccesses int                                  `json:"channel_timeout_recovery_probe_successes"`
	CostCalculationEnabled               bool                                 `json:"cost_calculation_enabled"`
	CostCalculationIntervalSeconds       int                                  `json:"cost_calculation_interval_seconds"`
	CostCalculationWorkerCount           int                                  `json:"cost_calculation_worker_count"`
	CostCalculationBatchSize             int                                  `json:"cost_calculation_batch_size"`
	DynamicBillingEnabled                bool                                 `json:"dynamic_billing_enabled"`
	DynamicBillingProfitRate             float64                              `json:"dynamic_billing_profit_rate"`
	DynamicBillingWindowSamples          int                                  `json:"dynamic_billing_window_samples"`
	DynamicBillingWindowMinutes          int                                  `json:"dynamic_billing_window_minutes"`
	DynamicBillingMinSamples             int                                  `json:"dynamic_billing_min_samples"`
	DynamicBillingRefreshSeconds         int                                  `json:"dynamic_billing_refresh_seconds"`
	DynamicBillingMaxAgeSeconds          int                                  `json:"dynamic_billing_max_age_seconds"`
	DynamicBillingEnabledAt              int64                                `json:"dynamic_billing_enabled_at"`
	DynamicBillingCostSource             string                               `json:"dynamic_billing_cost_source"`
	DynamicBillingProfitWindowHours      int                                  `json:"dynamic_billing_profit_window_hours"`
	DynamicBillingMinTokens              int                                  `json:"dynamic_billing_min_tokens"`
	DynamicBillingMinRequests            int                                  `json:"dynamic_billing_min_requests"`
	DynamicBillingMinSuccessRequests     int                                  `json:"dynamic_billing_min_success_requests"`
	DynamicBillingMinRatio               float64                              `json:"dynamic_billing_min_ratio"`
	DynamicBillingMaxRatio               float64                              `json:"dynamic_billing_max_ratio"`
	DynamicBillingMaxStepChange          float64                              `json:"dynamic_billing_max_step_change"`
	DynamicBillingApplyMode              string                               `json:"dynamic_billing_apply_mode"`
	SuccessWeight                        float64                              `json:"success_weight"`
	SpeedWeight                          float64                              `json:"speed_weight"`
	LoadWeight                           float64                              `json:"load_weight"`
	CostWeight                           float64                              `json:"cost_weight"`
	GroupWeight                          float64                              `json:"group_weight"`
	GroupPriorityRatio                   map[string]float64                   `json:"group_priority_ratio"`
	GroupPolicies                        map[string]GroupPolicySetting        `json:"group_policies"`
	FailureFastWindowSeconds             int                                  `json:"failure_fast_window_seconds"`
	FailureMainWindowSeconds             int                                  `json:"failure_main_window_seconds"`
	FailureFallbackWindowSeconds         int                                  `json:"failure_fallback_window_seconds"`
	ProxySameBrandReusePolicy            string                               `json:"proxy_same_brand_reuse_policy"`
}

var schedulerSetting = SchedulerSetting{
	Enabled:                              false,
	DefaultMode:                          ModeOff,
	RolloutPercent:                       0,
	DefaultStrategy:                      StrategyBalanced,
	SnapshotRefreshMs:                    500,
	StickyTTLSeconds:                     180,
	StickyKeepScoreRatio:                 0.85,
	StickySaveOnSelect:                   false,
	StickyRenewOnSuccess:                 true,
	StickyFailurePolicy:                  StickyFailurePolicyClear,
	CacheAffinityEnabled:                 true,
	CacheAffinityKeepScoreRatio:          0.75,
	CostFirstStickyEscapeEnabled:         true,
	CostFirstStickyEscapeCostRatio:       0.75,
	CostFirstStickyEscapeCacheCostRatio:  0.55,
	CostFirstStickyEscapeMaxSpeedDrop:    0.06,
	CostFirstStickyEscapeCacheSpeedDrop:  0.03,
	CostFirstStickyEscapeMinSamples:      5,
	CostFirstStickyEscapeSuccessSlack:    0.02,
	CostFirstGuardEnabled:                true,
	CostFirstGuardMultiple:               1.8,
	CostFirstGuardSuccessAdvantage:       0.03,
	CostFirstGuardSpeedAdvantage:         0.08,
	ChannelPriorityTieBreakEnabled:       true,
	ChannelPriorityTieBreakScoreDelta:    0.05,
	QueueEnabled:                         true,
	QueueDefaultTimeoutMs:                2000,
	QueueMaxDepthPerChannel:              64,
	QueueDepthMultiplier:                 2,
	QueueHighPriorityThreshold:           0,
	QueueHighPriorityExtraDepth:          0,
	QueueHighPriorityReservedDepth:       0,
	QueueAbsoluteMaxDepth:                0,
	CircuitBreakerEnabled:                true,
	CircuitFailureThreshold:              0.5,
	CircuitMinSamples:                    10,
	CircuitOpenSeconds:                   30,
	CircuitHalfOpenProbeCount:            3,
	CircuitErrorPolicies:                 map[string]CircuitErrorPolicySetting{},
	CooldownMaxSeconds:                   600,
	RuntimeSyncEnabled:                   true,
	RuntimeSyncRedisEnabled:              true,
	RuntimeSyncNodeID:                    "",
	RuntimeSyncTTLSeconds:                90,
	RuntimeSyncQueueMinIntervalMs:        500,
	RuntimeSyncEventPushEnabled:          false,
	RuntimeSyncEventSubscribeEnabled:     false,
	ProbeEnabled:                         true,
	ProbeIntervalSeconds:                 60,
	ProbeWorkerCount:                     2,
	ProbeTimeoutSeconds:                  8,
	ProbeMaxPerTick:                      5,
	ProbeMinChannelIntervalSeconds:       300,
	ProbeLowScoreThreshold:               0.62,
	ProbeMissingSampleThreshold:          3,
	ProbeLongNoSuccessSeconds:            1800,
	ProbeRecoverySuccessesRequired:       2,
	ProbeFailureAvoidancePriorityEnabled: true,
	ProbeRecoverableScoreItems:           []string{"completion_rate", "upstream_error_rate", "ttft_latency", "duration_latency", "first_byte_backlog", "empty_output_rate", "stream_interrupted_rate"},
	ProbeSkipRecentRealRequestEnabled:    true,
	ProbeRecentRealRequestWindowSeconds:  1800,
	ProbeGoodBaselineEnabled:             true,
	ProbeGoodBaselineMinSamples:          3,
	ProbeGoodBaselineWindowSeconds:       86400,
	ProbePromptLibraryEnabled:            true,
	ProbePromptCategories:                []string{"short", "zh", "medium", "long"},
	RelayTotalTimeoutEnabled:             true,
	RelayTotalTimeoutSeconds:             180,
	ChannelTimeoutDegradeEnabled:         true,
	ChannelTimeoutDegradeWindowSeconds:   600,
	ChannelTimeoutDegradeMinSamples:      5,
	ChannelTimeoutDegradeThreshold:       0.4,
	ChannelTimeoutDegradeConsecutive:     3,
	ChannelTimeoutRecoveryProbeSuccesses: 2,
	CostCalculationEnabled:               true,
	CostCalculationIntervalSeconds:       5,
	CostCalculationWorkerCount:           2,
	CostCalculationBatchSize:             100,
	DynamicBillingEnabled:                false,
	DynamicBillingProfitRate:             0.20,
	DynamicBillingWindowSamples:          300,
	DynamicBillingWindowMinutes:          60,
	DynamicBillingMinSamples:             5,
	DynamicBillingRefreshSeconds:         30,
	DynamicBillingMaxAgeSeconds:          300,
	DynamicBillingCostSource:             DynamicBillingCostSourceProfit24h,
	DynamicBillingProfitWindowHours:      24,
	DynamicBillingMinTokens:              1000,
	DynamicBillingMinRequests:            20,
	DynamicBillingMinSuccessRequests:     5,
	DynamicBillingMinRatio:               0.01,
	DynamicBillingMaxRatio:               2.0,
	DynamicBillingMaxStepChange:          0.30,
	DynamicBillingApplyMode:              DynamicBillingApplyModeAuto,
	SuccessWeight:                        0.32,
	SpeedWeight:                          0.28,
	LoadWeight:                           0.20,
	CostWeight:                           0.15,
	GroupWeight:                          0.05,
	GroupPriorityRatio:                   map[string]float64{},
	GroupPolicies:                        map[string]GroupPolicySetting{},
	FailureFastWindowSeconds:             60,
	FailureMainWindowSeconds:             300,
	FailureFallbackWindowSeconds:         1800,
	ProxySameBrandReusePolicy:            ProxyReusePolicyWarn,
}

var (
	changeHooksMu sync.RWMutex
	changeHooks   []func(before SchedulerSetting, after SchedulerSetting)
)

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
	old := schedulerSetting
	schedulerSetting = setting
	notifyChangeHooks(old, schedulerSetting)
}

func SetSettingForTest(setting SchedulerSetting) func() {
	old := schedulerSetting
	schedulerSetting = setting
	return func() {
		schedulerSetting = old
	}
}

func AddChangeHook(hook func(before SchedulerSetting, after SchedulerSetting)) {
	if hook == nil {
		return
	}
	changeHooksMu.Lock()
	changeHooks = append(changeHooks, hook)
	changeHooksMu.Unlock()
}

func NotifyChange(before SchedulerSetting) {
	notifyChangeHooks(before, schedulerSetting)
}

func notifyChangeHooks(before SchedulerSetting, after SchedulerSetting) {
	changeHooksMu.RLock()
	hooks := append([]func(before SchedulerSetting, after SchedulerSetting){}, changeHooks...)
	changeHooksMu.RUnlock()
	for _, hook := range hooks {
		hook(before, after)
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
	setting.CostFirstStickyEscapeEnabled = true
	setting.CostFirstStickyEscapeCostRatio = 0.75
	setting.CostFirstStickyEscapeCacheCostRatio = 0.55
	setting.CostFirstStickyEscapeMaxSpeedDrop = 0.06
	setting.CostFirstStickyEscapeCacheSpeedDrop = 0.03
	setting.CostFirstStickyEscapeMinSamples = 5
	setting.CostFirstStickyEscapeSuccessSlack = 0.02
	setting.CostFirstGuardEnabled = true
	setting.CostFirstGuardMultiple = 1.8
	setting.CostFirstGuardSuccessAdvantage = 0.03
	setting.CostFirstGuardSpeedAdvantage = 0.08
	setting.ChannelPriorityTieBreakEnabled = true
	setting.ChannelPriorityTieBreakScoreDelta = 0.05
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
	setting.ProbeLowScoreThreshold = 0.62
	setting.ProbeMissingSampleThreshold = 3
	setting.ProbeLongNoSuccessSeconds = 1800
	setting.ProbeRecoverySuccessesRequired = 2
	setting.ProbeFailureAvoidancePriorityEnabled = true
	setting.ProbeRecoverableScoreItems = []string{"completion_rate", "upstream_error_rate", "ttft_latency", "duration_latency", "first_byte_backlog", "empty_output_rate", "stream_interrupted_rate"}
	setting.ProbeSkipRecentRealRequestEnabled = true
	setting.ProbeRecentRealRequestWindowSeconds = 1800
	setting.ProbeGoodBaselineEnabled = true
	setting.ProbeGoodBaselineMinSamples = 3
	setting.ProbeGoodBaselineWindowSeconds = 86400
	setting.ProbePromptLibraryEnabled = true
	setting.ProbePromptCategories = []string{"short", "zh", "medium", "long"}
	setting.RelayTotalTimeoutEnabled = true
	setting.RelayTotalTimeoutSeconds = 180
	setting.ChannelTimeoutDegradeEnabled = true
	setting.ChannelTimeoutDegradeWindowSeconds = 600
	setting.ChannelTimeoutDegradeMinSamples = 5
	setting.ChannelTimeoutDegradeThreshold = 0.4
	setting.ChannelTimeoutDegradeConsecutive = 3
	setting.ChannelTimeoutRecoveryProbeSuccesses = 2
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
	setting.DynamicBillingCostSource = DynamicBillingCostSourceProfit24h
	setting.DynamicBillingProfitWindowHours = 24
	setting.DynamicBillingMinTokens = 1000
	setting.DynamicBillingMinRequests = 20
	setting.DynamicBillingMinSuccessRequests = 5
	setting.DynamicBillingMinRatio = 0.01
	setting.DynamicBillingMaxRatio = 2.0
	setting.DynamicBillingMaxStepChange = 0.30
	setting.DynamicBillingApplyMode = DynamicBillingApplyModeAuto
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
	setting.ProxySameBrandReusePolicy = ProxyReusePolicyWarn
	return setting
}
