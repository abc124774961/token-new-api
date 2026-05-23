package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayprobe "github.com/QuantumNous/new-api/pkg/modelgateway/probe"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
)

type ModelGatewayConfigResponse struct {
	Setting    scheduler_setting.SchedulerSetting `json:"setting"`
	Defaults   scheduler_setting.SchedulerSetting `json:"defaults"`
	Modes      []string                           `json:"modes"`
	Strategies []string                           `json:"strategies"`
	AutoModes  []string                           `json:"auto_modes"`
}

func GetModelGatewayConfig(c *gin.Context) {
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func UpdateModelGatewayConfig(c *gin.Context) {
	var setting scheduler_setting.SchedulerSetting
	if err := common.DecodeJson(c.Request.Body, &setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	normalized, err := normalizeModelGatewaySchedulerSetting(setting)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := persistModelGatewaySchedulerSetting(normalized); err != nil {
		common.ApiError(c, err)
		return
	}
	modelgatewayprobe.RegisterRelayInvoker(Relay)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	modelgatewayintegration.SyncRuntimeEventSubscriberLifecycle()
	modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func ResetModelGatewayConfig(c *gin.Context) {
	setting := scheduler_setting.DefaultSetting()
	if err := persistModelGatewaySchedulerSetting(setting); err != nil {
		common.ApiError(c, err)
		return
	}
	modelgatewayprobe.RegisterRelayInvoker(Relay)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	modelgatewayintegration.SyncRuntimeEventSubscriberLifecycle()
	modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func buildModelGatewayConfigResponse() ModelGatewayConfigResponse {
	return ModelGatewayConfigResponse{
		Setting:    scheduler_setting.GetSetting(),
		Defaults:   scheduler_setting.DefaultSetting(),
		Modes:      []string{scheduler_setting.ModeOff, scheduler_setting.ModeShadow, scheduler_setting.ModeActive},
		Strategies: []string{scheduler_setting.StrategyBalanced, scheduler_setting.StrategySpeedFirst, scheduler_setting.StrategyCostFirst, scheduler_setting.StrategyStabilityFirst},
		AutoModes:  []string{scheduler_setting.AutoModeSequential, scheduler_setting.AutoModeFusion},
	}
}

func normalizeModelGatewaySchedulerSetting(setting scheduler_setting.SchedulerSetting) (scheduler_setting.SchedulerSetting, error) {
	defaults := scheduler_setting.DefaultSetting()
	if setting.DefaultMode == "" {
		setting.DefaultMode = defaults.DefaultMode
	}
	if !validModelGatewayConfigValue(setting.DefaultMode, modelGatewayConfigModes()) {
		return scheduler_setting.SchedulerSetting{}, errors.New("invalid default_mode")
	}
	if setting.DefaultStrategy == "" {
		setting.DefaultStrategy = defaults.DefaultStrategy
	}
	if !validModelGatewayConfigValue(setting.DefaultStrategy, modelGatewayConfigStrategies()) {
		return scheduler_setting.SchedulerSetting{}, errors.New("invalid default_strategy")
	}
	setting.RolloutPercent = clampModelGatewayConfigInt(setting.RolloutPercent, 0, 100)
	setting.SnapshotRefreshMs = normalizeModelGatewayConfigMin(setting.SnapshotRefreshMs, 100, defaults.SnapshotRefreshMs)
	setting.StickyTTLSeconds = normalizeModelGatewayConfigMin(setting.StickyTTLSeconds, 1, defaults.StickyTTLSeconds)
	setting.StickyKeepScoreRatio = clampModelGatewayConfigFloat(defaultFloat(setting.StickyKeepScoreRatio, defaults.StickyKeepScoreRatio), 0.01, 1)
	if setting.StickyFailurePolicy == "" {
		setting.StickyFailurePolicy = defaults.StickyFailurePolicy
	}
	if !validModelGatewayConfigValue(setting.StickyFailurePolicy, modelGatewayConfigStickyFailurePolicies()) {
		return scheduler_setting.SchedulerSetting{}, errors.New("invalid sticky_failure_policy")
	}
	setting.CacheAffinityKeepScoreRatio = clampModelGatewayConfigFloat(defaultFloat(setting.CacheAffinityKeepScoreRatio, defaults.CacheAffinityKeepScoreRatio), 0.01, 1)
	setting.QueueDefaultTimeoutMs = normalizeModelGatewayConfigMin(setting.QueueDefaultTimeoutMs, 1, defaults.QueueDefaultTimeoutMs)
	setting.QueueMaxDepthPerChannel = normalizeModelGatewayConfigMin(setting.QueueMaxDepthPerChannel, 1, defaults.QueueMaxDepthPerChannel)
	setting.QueueDepthMultiplier = normalizeModelGatewayConfigMin(setting.QueueDepthMultiplier, 1, defaults.QueueDepthMultiplier)
	setting.QueueHighPriorityThreshold = normalizeModelGatewayConfigNonNegative(setting.QueueHighPriorityThreshold)
	setting.QueueHighPriorityExtraDepth = normalizeModelGatewayConfigNonNegative(setting.QueueHighPriorityExtraDepth)
	setting.QueueHighPriorityReservedDepth = normalizeModelGatewayConfigNonNegative(setting.QueueHighPriorityReservedDepth)
	setting.QueueAbsoluteMaxDepth = normalizeModelGatewayConfigNonNegative(setting.QueueAbsoluteMaxDepth)
	setting.CircuitFailureThreshold = clampModelGatewayConfigFloat(defaultFloat(setting.CircuitFailureThreshold, defaults.CircuitFailureThreshold), 0.01, 1)
	setting.CircuitMinSamples = normalizeModelGatewayConfigMin(setting.CircuitMinSamples, 1, defaults.CircuitMinSamples)
	setting.CircuitOpenSeconds = normalizeModelGatewayConfigMin(setting.CircuitOpenSeconds, 1, defaults.CircuitOpenSeconds)
	setting.CircuitHalfOpenProbeCount = normalizeModelGatewayConfigMin(setting.CircuitHalfOpenProbeCount, 1, defaults.CircuitHalfOpenProbeCount)
	setting.CircuitErrorPolicies = normalizeModelGatewayCircuitErrorPolicies(setting.CircuitErrorPolicies, setting)
	setting.CooldownMaxSeconds = normalizeModelGatewayConfigMin(setting.CooldownMaxSeconds, 1, defaults.CooldownMaxSeconds)
	setting.RuntimeSyncNodeID = strings.TrimSpace(setting.RuntimeSyncNodeID)
	setting.RuntimeSyncTTLSeconds = normalizeModelGatewayConfigMin(setting.RuntimeSyncTTLSeconds, 1, defaults.RuntimeSyncTTLSeconds)
	setting.RuntimeSyncQueueMinIntervalMs = normalizeModelGatewayConfigNonNegative(setting.RuntimeSyncQueueMinIntervalMs)
	setting.ProbeIntervalSeconds = normalizeModelGatewayConfigMin(setting.ProbeIntervalSeconds, 10, defaults.ProbeIntervalSeconds)
	setting.ProbeWorkerCount = normalizeModelGatewayConfigMin(setting.ProbeWorkerCount, 1, defaults.ProbeWorkerCount)
	setting.ProbeTimeoutSeconds = normalizeModelGatewayConfigMin(setting.ProbeTimeoutSeconds, 1, defaults.ProbeTimeoutSeconds)
	setting.ProbeMaxPerTick = normalizeModelGatewayConfigMin(setting.ProbeMaxPerTick, 1, defaults.ProbeMaxPerTick)
	setting.ProbeMinChannelIntervalSeconds = normalizeModelGatewayConfigMin(setting.ProbeMinChannelIntervalSeconds, 10, defaults.ProbeMinChannelIntervalSeconds)
	setting.CostCalculationIntervalSeconds = normalizeModelGatewayConfigMin(setting.CostCalculationIntervalSeconds, 1, defaults.CostCalculationIntervalSeconds)
	setting.CostCalculationWorkerCount = normalizeModelGatewayConfigMin(setting.CostCalculationWorkerCount, 1, defaults.CostCalculationWorkerCount)
	setting.CostCalculationBatchSize = normalizeModelGatewayConfigMin(setting.CostCalculationBatchSize, 1, defaults.CostCalculationBatchSize)
	setting.FailureFastWindowSeconds = normalizeModelGatewayConfigMin(setting.FailureFastWindowSeconds, 1, defaults.FailureFastWindowSeconds)
	setting.FailureMainWindowSeconds = normalizeModelGatewayConfigMin(setting.FailureMainWindowSeconds, 1, defaults.FailureMainWindowSeconds)
	setting.FailureFallbackWindowSeconds = normalizeModelGatewayConfigMin(setting.FailureFallbackWindowSeconds, 1, defaults.FailureFallbackWindowSeconds)

	weights := []float64{setting.SuccessWeight, setting.SpeedWeight, setting.LoadWeight, setting.CostWeight, setting.GroupWeight}
	for _, weight := range weights {
		if weight < 0 {
			return scheduler_setting.SchedulerSetting{}, errors.New("score weights cannot be negative")
		}
	}
	if setting.SuccessWeight+setting.SpeedWeight+setting.LoadWeight+setting.CostWeight+setting.GroupWeight <= 0 {
		setting.SuccessWeight = defaults.SuccessWeight
		setting.SpeedWeight = defaults.SpeedWeight
		setting.LoadWeight = defaults.LoadWeight
		setting.CostWeight = defaults.CostWeight
		setting.GroupWeight = defaults.GroupWeight
	}

	setting.GroupPriorityRatio = normalizeModelGatewayGroupPriorityRatio(setting.GroupPriorityRatio)
	policies, err := normalizeModelGatewayGroupPolicies(setting.GroupPolicies, setting)
	if err != nil {
		return scheduler_setting.SchedulerSetting{}, err
	}
	setting.GroupPolicies = policies
	return setting, nil
}

func normalizeModelGatewayGroupPriorityRatio(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return map[string]float64{}
	}
	result := make(map[string]float64, len(src))
	for group, ratio := range src {
		group = strings.TrimSpace(group)
		if group == "" || ratio <= 0 {
			continue
		}
		result[group] = clampModelGatewayConfigFloat(ratio, 0.01, 10)
	}
	return result
}

func normalizeModelGatewayGroupPolicies(src map[string]scheduler_setting.GroupPolicySetting, setting scheduler_setting.SchedulerSetting) (map[string]scheduler_setting.GroupPolicySetting, error) {
	if len(src) == 0 {
		return map[string]scheduler_setting.GroupPolicySetting{}, nil
	}
	result := make(map[string]scheduler_setting.GroupPolicySetting, len(src))
	for group, policy := range src {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if policy.Mode == "" {
			policy.Mode = setting.DefaultMode
		}
		if !validModelGatewayConfigValue(policy.Mode, modelGatewayConfigModes()) {
			return nil, fmt.Errorf("invalid mode for group %s", group)
		}
		if policy.Strategy == "" {
			policy.Strategy = setting.DefaultStrategy
		}
		if !validModelGatewayConfigValue(policy.Strategy, modelGatewayConfigStrategies()) {
			return nil, fmt.Errorf("invalid strategy for group %s", group)
		}
		if policy.AutoMode == "" {
			policy.AutoMode = scheduler_setting.AutoModeSequential
		}
		if !validModelGatewayConfigValue(policy.AutoMode, modelGatewayConfigAutoModes()) {
			return nil, fmt.Errorf("invalid auto_mode for group %s", group)
		}
		policy.CandidateGroups = uniqueTrimmedModelGatewayStrings(policy.CandidateGroups)
		result[group] = policy
	}
	return result, nil
}

func normalizeModelGatewayCircuitErrorPolicies(src map[string]scheduler_setting.CircuitErrorPolicySetting, setting scheduler_setting.SchedulerSetting) map[string]scheduler_setting.CircuitErrorPolicySetting {
	if len(src) == 0 {
		return map[string]scheduler_setting.CircuitErrorPolicySetting{}
	}
	result := make(map[string]scheduler_setting.CircuitErrorPolicySetting, len(src))
	for kind, policy := range src {
		kind = normalizeModelGatewayCircuitErrorKind(kind)
		if kind == "" {
			continue
		}
		policy.FailureThreshold = clampModelGatewayConfigFloat(defaultFloat(policy.FailureThreshold, setting.CircuitFailureThreshold), 0.01, 1)
		policy.MinSamples = normalizeModelGatewayConfigMin(policy.MinSamples, 1, setting.CircuitMinSamples)
		policy.OpenSeconds = normalizeModelGatewayConfigMin(policy.OpenSeconds, 1, setting.CircuitOpenSeconds)
		policy.HalfOpenProbeCount = normalizeModelGatewayConfigMin(policy.HalfOpenProbeCount, 1, setting.CircuitHalfOpenProbeCount)
		result[kind] = policy
	}
	return result
}

func normalizeModelGatewayCircuitErrorKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case scheduler.CircuitErrorStreamInterrupted,
		scheduler.CircuitErrorRateLimit,
		scheduler.CircuitErrorAuth,
		scheduler.CircuitErrorQuota,
		scheduler.CircuitErrorServer,
		scheduler.CircuitErrorUpstream:
		return kind
	default:
		return ""
	}
}

func persistModelGatewaySchedulerSetting(setting scheduler_setting.SchedulerSetting) error {
	values, err := modelGatewaySchedulerSettingOptionMap(setting)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := model.UpdateOption("scheduler_setting."+key, values[key]); err != nil {
			return err
		}
	}
	scheduler_setting.SetSetting(setting)
	resetRelayQueueManager()
	return nil
}

func modelGatewaySchedulerSettingOptionMap(setting scheduler_setting.SchedulerSetting) (map[string]string, error) {
	groupPriorityRatio, err := common.Marshal(setting.GroupPriorityRatio)
	if err != nil {
		return nil, err
	}
	groupPolicies, err := common.Marshal(setting.GroupPolicies)
	if err != nil {
		return nil, err
	}
	circuitErrorPolicies, err := common.Marshal(setting.CircuitErrorPolicies)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"enabled":                              strconv.FormatBool(setting.Enabled),
		"default_mode":                         setting.DefaultMode,
		"rollout_percent":                      strconv.Itoa(setting.RolloutPercent),
		"default_strategy":                     setting.DefaultStrategy,
		"snapshot_refresh_ms":                  strconv.Itoa(setting.SnapshotRefreshMs),
		"sticky_ttl_seconds":                   strconv.Itoa(setting.StickyTTLSeconds),
		"sticky_keep_score_ratio":              strconv.FormatFloat(setting.StickyKeepScoreRatio, 'f', -1, 64),
		"sticky_save_on_select":                strconv.FormatBool(setting.StickySaveOnSelect),
		"sticky_renew_on_success":              strconv.FormatBool(setting.StickyRenewOnSuccess),
		"sticky_failure_policy":                setting.StickyFailurePolicy,
		"cache_affinity_enabled":               strconv.FormatBool(setting.CacheAffinityEnabled),
		"cache_affinity_keep_score_ratio":      strconv.FormatFloat(setting.CacheAffinityKeepScoreRatio, 'f', -1, 64),
		"queue_enabled":                        strconv.FormatBool(setting.QueueEnabled),
		"queue_default_timeout_ms":             strconv.Itoa(setting.QueueDefaultTimeoutMs),
		"queue_max_depth_per_channel":          strconv.Itoa(setting.QueueMaxDepthPerChannel),
		"queue_depth_multiplier":               strconv.Itoa(setting.QueueDepthMultiplier),
		"queue_high_priority_threshold":        strconv.Itoa(setting.QueueHighPriorityThreshold),
		"queue_high_priority_extra_depth":      strconv.Itoa(setting.QueueHighPriorityExtraDepth),
		"queue_high_priority_reserved_depth":   strconv.Itoa(setting.QueueHighPriorityReservedDepth),
		"queue_absolute_max_depth":             strconv.Itoa(setting.QueueAbsoluteMaxDepth),
		"circuit_breaker_enabled":              strconv.FormatBool(setting.CircuitBreakerEnabled),
		"circuit_failure_threshold":            strconv.FormatFloat(setting.CircuitFailureThreshold, 'f', -1, 64),
		"circuit_min_samples":                  strconv.Itoa(setting.CircuitMinSamples),
		"circuit_open_seconds":                 strconv.Itoa(setting.CircuitOpenSeconds),
		"circuit_half_open_probe_count":        strconv.Itoa(setting.CircuitHalfOpenProbeCount),
		"circuit_error_policies":               string(circuitErrorPolicies),
		"cooldown_max_seconds":                 strconv.Itoa(setting.CooldownMaxSeconds),
		"runtime_sync_enabled":                 strconv.FormatBool(setting.RuntimeSyncEnabled),
		"runtime_sync_redis_enabled":           strconv.FormatBool(setting.RuntimeSyncRedisEnabled),
		"runtime_sync_node_id":                 setting.RuntimeSyncNodeID,
		"runtime_sync_ttl_seconds":             strconv.Itoa(setting.RuntimeSyncTTLSeconds),
		"runtime_sync_queue_min_interval_ms":   strconv.Itoa(setting.RuntimeSyncQueueMinIntervalMs),
		"runtime_sync_event_push_enabled":      strconv.FormatBool(setting.RuntimeSyncEventPushEnabled),
		"runtime_sync_event_subscribe_enabled": strconv.FormatBool(setting.RuntimeSyncEventSubscribeEnabled),
		"probe_enabled":                        strconv.FormatBool(setting.ProbeEnabled),
		"probe_interval_seconds":               strconv.Itoa(setting.ProbeIntervalSeconds),
		"probe_worker_count":                   strconv.Itoa(setting.ProbeWorkerCount),
		"probe_timeout_seconds":                strconv.Itoa(setting.ProbeTimeoutSeconds),
		"probe_max_per_tick":                   strconv.Itoa(setting.ProbeMaxPerTick),
		"probe_min_channel_interval_seconds":   strconv.Itoa(setting.ProbeMinChannelIntervalSeconds),
		"cost_calculation_enabled":             strconv.FormatBool(setting.CostCalculationEnabled),
		"cost_calculation_interval_seconds":    strconv.Itoa(setting.CostCalculationIntervalSeconds),
		"cost_calculation_worker_count":        strconv.Itoa(setting.CostCalculationWorkerCount),
		"cost_calculation_batch_size":          strconv.Itoa(setting.CostCalculationBatchSize),
		"success_weight":                       strconv.FormatFloat(setting.SuccessWeight, 'f', -1, 64),
		"speed_weight":                         strconv.FormatFloat(setting.SpeedWeight, 'f', -1, 64),
		"load_weight":                          strconv.FormatFloat(setting.LoadWeight, 'f', -1, 64),
		"cost_weight":                          strconv.FormatFloat(setting.CostWeight, 'f', -1, 64),
		"group_weight":                         strconv.FormatFloat(setting.GroupWeight, 'f', -1, 64),
		"group_priority_ratio":                 string(groupPriorityRatio),
		"group_policies":                       string(groupPolicies),
		"failure_fast_window_seconds":          strconv.Itoa(setting.FailureFastWindowSeconds),
		"failure_main_window_seconds":          strconv.Itoa(setting.FailureMainWindowSeconds),
		"failure_fallback_window_seconds":      strconv.Itoa(setting.FailureFallbackWindowSeconds),
	}, nil
}

func modelGatewayConfigModes() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.ModeOff:    {},
		scheduler_setting.ModeShadow: {},
		scheduler_setting.ModeActive: {},
	}
}

func modelGatewayConfigStrategies() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.StrategyBalanced:       {},
		scheduler_setting.StrategySpeedFirst:     {},
		scheduler_setting.StrategyCostFirst:      {},
		scheduler_setting.StrategyStabilityFirst: {},
	}
}

func modelGatewayConfigAutoModes() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.AutoModeSequential: {},
		scheduler_setting.AutoModeFusion:     {},
	}
}

func modelGatewayConfigStickyFailurePolicies() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.StickyFailurePolicyKeep:  {},
		scheduler_setting.StickyFailurePolicyClear: {},
	}
}

func validModelGatewayConfigValue(value string, allowed map[string]struct{}) bool {
	_, ok := allowed[value]
	return ok
}

func normalizeModelGatewayConfigMin(value int, minValue int, fallback int) int {
	if value < minValue {
		return fallback
	}
	return value
}

func normalizeModelGatewayConfigNonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func clampModelGatewayConfigInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampModelGatewayConfigFloat(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func defaultFloat(value float64, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func uniqueTrimmedModelGatewayStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
