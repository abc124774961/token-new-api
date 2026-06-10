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
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayprobe "github.com/QuantumNous/new-api/pkg/modelgateway/probe"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	modelgatewayupstreamerror "github.com/QuantumNous/new-api/pkg/modelgateway/upstreamerror"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
)

type ModelGatewayConfigResponse struct {
	Setting                   scheduler_setting.SchedulerSetting         `json:"setting"`
	Defaults                  scheduler_setting.SchedulerSetting         `json:"defaults"`
	Modes                     []string                                   `json:"modes"`
	Strategies                []string                                   `json:"strategies"`
	AutoModes                 []string                                   `json:"auto_modes"`
	DynamicBillingBaselines   []modelgatewaydynamicbilling.RatioBaseline `json:"dynamic_billing_baselines"`
	UpstreamErrorKinds        []string                                   `json:"upstream_error_kinds"`
	UpstreamErrorActions      []string                                   `json:"upstream_error_actions"`
	UpstreamErrorRuleDefaults []scheduler_setting.UpstreamErrorRule      `json:"upstream_error_rule_defaults"`
}

type UpdateModelGatewayProbeConfigRequest struct {
	ProbeEnabled                         *bool    `json:"probe_enabled,omitempty"`
	ProbeIntervalSeconds                 *int     `json:"probe_interval_seconds,omitempty"`
	ProbeWorkerCount                     *int     `json:"probe_worker_count,omitempty"`
	ProbeTimeoutSeconds                  *int     `json:"probe_timeout_seconds,omitempty"`
	ProbeMaxPerTick                      *int     `json:"probe_max_per_tick,omitempty"`
	ProbeMinChannelIntervalSeconds       *int     `json:"probe_min_channel_interval_seconds,omitempty"`
	ProbeLowScoreThreshold               *float64 `json:"probe_low_score_threshold,omitempty"`
	ProbeMissingSampleThreshold          *int     `json:"probe_missing_sample_threshold,omitempty"`
	ProbeLongNoSuccessSeconds            *int     `json:"probe_long_no_success_seconds,omitempty"`
	ProbeRecoverySuccessesRequired       *int     `json:"probe_recovery_successes_required,omitempty"`
	ProbeFailureAvoidancePriorityEnabled *bool    `json:"probe_failure_avoidance_priority_enabled,omitempty"`
	ProbeRecoverableScoreItems           []string `json:"probe_recoverable_score_items,omitempty"`
	ProbeSkipRecentRealRequestEnabled    *bool    `json:"probe_skip_recent_real_request_enabled,omitempty"`
	ProbeRecentRealRequestWindowSeconds  *int     `json:"probe_recent_real_request_window_seconds,omitempty"`
	ProbeGoodBaselineEnabled             *bool    `json:"probe_good_baseline_enabled,omitempty"`
	ProbeGoodBaselineMinSamples          *int     `json:"probe_good_baseline_min_samples,omitempty"`
	ProbeGoodBaselineWindowSeconds       *int     `json:"probe_good_baseline_window_seconds,omitempty"`
	ProbePromptLibraryEnabled            *bool    `json:"probe_prompt_library_enabled,omitempty"`
	ProbePromptCategories                []string `json:"probe_prompt_categories,omitempty"`
}

func GetModelGatewayConfig(c *gin.Context) {
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func UpdateModelGatewayConfig(c *gin.Context) {
	before := scheduler_setting.GetSetting()
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
	normalized.DynamicBillingEnabledAt = nextModelGatewayDynamicBillingEnabledAt(before, normalized)
	if err := persistModelGatewaySchedulerSetting(normalized); err != nil {
		common.ApiError(c, err)
		return
	}
	invalidatePublicHomeDynamicBillingCache()
	modelgatewayprobe.RegisterRelayInvoker(Relay)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	modelgatewayintegration.SyncRuntimeEventSubscriberLifecycle()
	modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	modelgatewaydynamicbilling.SyncDefaultRefresherLifecycle()
	modelgatewayupstreamerror.SyncDefaultManager()
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func UpdateModelGatewayProbeConfig(c *gin.Context) {
	before := scheduler_setting.GetSetting()
	setting := before
	var request UpdateModelGatewayProbeConfigRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	applyModelGatewayProbeConfigRequest(&setting, request)
	normalized, err := normalizeModelGatewaySchedulerSetting(setting)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	normalized.DynamicBillingEnabledAt = before.DynamicBillingEnabledAt
	if err := persistModelGatewaySchedulerSetting(normalized); err != nil {
		common.ApiError(c, err)
		return
	}
	modelgatewayprobe.RegisterRelayInvoker(Relay)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	modelgatewayintegration.SyncRuntimeEventSubscriberLifecycle()
	modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	modelgatewaydynamicbilling.SyncDefaultRefresherLifecycle()
	modelgatewayupstreamerror.SyncDefaultManager()
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func applyModelGatewayProbeConfigRequest(setting *scheduler_setting.SchedulerSetting, request UpdateModelGatewayProbeConfigRequest) {
	if setting == nil {
		return
	}
	if request.ProbeEnabled != nil {
		setting.ProbeEnabled = *request.ProbeEnabled
	}
	if request.ProbeIntervalSeconds != nil {
		setting.ProbeIntervalSeconds = *request.ProbeIntervalSeconds
	}
	if request.ProbeWorkerCount != nil {
		setting.ProbeWorkerCount = *request.ProbeWorkerCount
	}
	if request.ProbeTimeoutSeconds != nil {
		setting.ProbeTimeoutSeconds = *request.ProbeTimeoutSeconds
	}
	if request.ProbeMaxPerTick != nil {
		setting.ProbeMaxPerTick = *request.ProbeMaxPerTick
	}
	if request.ProbeMinChannelIntervalSeconds != nil {
		setting.ProbeMinChannelIntervalSeconds = *request.ProbeMinChannelIntervalSeconds
	}
	if request.ProbeLowScoreThreshold != nil {
		setting.ProbeLowScoreThreshold = *request.ProbeLowScoreThreshold
	}
	if request.ProbeMissingSampleThreshold != nil {
		setting.ProbeMissingSampleThreshold = *request.ProbeMissingSampleThreshold
	}
	if request.ProbeLongNoSuccessSeconds != nil {
		setting.ProbeLongNoSuccessSeconds = *request.ProbeLongNoSuccessSeconds
	}
	if request.ProbeRecoverySuccessesRequired != nil {
		setting.ProbeRecoverySuccessesRequired = *request.ProbeRecoverySuccessesRequired
	}
	if request.ProbeFailureAvoidancePriorityEnabled != nil {
		setting.ProbeFailureAvoidancePriorityEnabled = *request.ProbeFailureAvoidancePriorityEnabled
	}
	if request.ProbeRecoverableScoreItems != nil {
		setting.ProbeRecoverableScoreItems = request.ProbeRecoverableScoreItems
	}
	if request.ProbeSkipRecentRealRequestEnabled != nil {
		setting.ProbeSkipRecentRealRequestEnabled = *request.ProbeSkipRecentRealRequestEnabled
	}
	if request.ProbeRecentRealRequestWindowSeconds != nil {
		setting.ProbeRecentRealRequestWindowSeconds = *request.ProbeRecentRealRequestWindowSeconds
	}
	if request.ProbeGoodBaselineEnabled != nil {
		setting.ProbeGoodBaselineEnabled = *request.ProbeGoodBaselineEnabled
	}
	if request.ProbeGoodBaselineMinSamples != nil {
		setting.ProbeGoodBaselineMinSamples = *request.ProbeGoodBaselineMinSamples
	}
	if request.ProbeGoodBaselineWindowSeconds != nil {
		setting.ProbeGoodBaselineWindowSeconds = *request.ProbeGoodBaselineWindowSeconds
	}
	if request.ProbePromptLibraryEnabled != nil {
		setting.ProbePromptLibraryEnabled = *request.ProbePromptLibraryEnabled
	}
	if request.ProbePromptCategories != nil {
		if len(request.ProbePromptCategories) == 0 {
			setting.ProbePromptCategories = []string{modelgatewayprobe.PromptCategoryShort}
		} else {
			setting.ProbePromptCategories = request.ProbePromptCategories
		}
	}
}

func ResetModelGatewayConfig(c *gin.Context) {
	if strings.EqualFold(strings.TrimSpace(c.Query("scope")), "upstream_error_rules") {
		setting := scheduler_setting.GetSetting()
		setting.UpstreamErrorClassificationEnabled = scheduler_setting.DefaultSetting().UpstreamErrorClassificationEnabled
		setting.UpstreamErrorRuleVersion = scheduler_setting.UpstreamErrorRuleVersion
		setting.UpstreamErrorRules = scheduler_setting.DefaultUpstreamErrorRules()
		normalized, err := normalizeModelGatewaySchedulerSetting(setting)
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		if err := persistModelGatewaySchedulerSetting(normalized); err != nil {
			common.ApiError(c, err)
			return
		}
		modelgatewayupstreamerror.SyncDefaultManager()
		common.ApiSuccess(c, buildModelGatewayConfigResponse())
		return
	}
	setting := scheduler_setting.DefaultSetting()
	setting.DynamicBillingEnabledAt = 0
	if err := persistModelGatewaySchedulerSetting(setting); err != nil {
		common.ApiError(c, err)
		return
	}
	invalidatePublicHomeDynamicBillingCache()
	modelgatewayprobe.RegisterRelayInvoker(Relay)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	modelgatewayintegration.SyncRuntimeEventSubscriberLifecycle()
	modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	modelgatewaydynamicbilling.SyncDefaultRefresherLifecycle()
	modelgatewayupstreamerror.SyncDefaultManager()
	common.ApiSuccess(c, buildModelGatewayConfigResponse())
}

func buildModelGatewayConfigResponse() ModelGatewayConfigResponse {
	return ModelGatewayConfigResponse{
		Setting:                   scheduler_setting.GetSetting(),
		Defaults:                  scheduler_setting.DefaultSetting(),
		Modes:                     []string{scheduler_setting.ModeOff, scheduler_setting.ModeShadow, scheduler_setting.ModeActive},
		Strategies:                []string{scheduler_setting.StrategyBalanced, scheduler_setting.StrategySpeedFirst, scheduler_setting.StrategyCostFirst, scheduler_setting.StrategyStabilityFirst},
		AutoModes:                 []string{scheduler_setting.AutoModeSequential, scheduler_setting.AutoModeFusion},
		DynamicBillingBaselines:   modelgatewaydynamicbilling.DefaultBaselineSnapshots(),
		UpstreamErrorKinds:        scheduler_setting.UpstreamErrorKinds(),
		UpstreamErrorActions:      scheduler_setting.UpstreamErrorActions(),
		UpstreamErrorRuleDefaults: scheduler_setting.DefaultUpstreamErrorRules(),
	}
}

func nextModelGatewayDynamicBillingEnabledAt(before scheduler_setting.SchedulerSetting, after scheduler_setting.SchedulerSetting) int64 {
	if !after.DynamicBillingEnabled {
		return 0
	}
	if !before.DynamicBillingEnabled || before.DynamicBillingEnabledAt <= 0 {
		return common.GetTimestamp()
	}
	if modelGatewayDynamicBillingPolicySignature(before) != modelGatewayDynamicBillingPolicySignature(after) {
		return common.GetTimestamp()
	}
	return before.DynamicBillingEnabledAt
}

func modelGatewayDynamicBillingPolicySignature(setting scheduler_setting.SchedulerSetting) string {
	if !setting.DynamicBillingEnabled || len(setting.GroupPolicies) == 0 {
		return ""
	}
	parts := []string{
		"source=" + setting.DynamicBillingCostSource,
		"apply=" + setting.DynamicBillingApplyMode,
		"profit=" + strconv.FormatFloat(setting.DynamicBillingProfitRate, 'f', -1, 64),
		"profit_window=" + strconv.Itoa(setting.DynamicBillingProfitWindowHours),
		"min_ratio=" + strconv.FormatFloat(setting.DynamicBillingMinRatio, 'f', -1, 64),
		"max_ratio=" + strconv.FormatFloat(setting.DynamicBillingMaxRatio, 'f', -1, 64),
		"max_step=" + strconv.FormatFloat(setting.DynamicBillingMaxStepChange, 'f', -1, 64),
	}
	for group, policy := range setting.GroupPolicies {
		if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
			continue
		}
		targets := append([]string(nil), policy.CandidateGroups...)
		targets = uniqueTrimmedModelGatewayStrings(targets)
		sort.Strings(targets)
		parts = append(parts, strings.TrimSpace(group)+"=>"+strings.Join(targets, ","))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
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
	setting.CostFirstStickyEscapeCostRatio = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstStickyEscapeCostRatio, defaults.CostFirstStickyEscapeCostRatio), 0.01, 1)
	setting.CostFirstStickyEscapeCacheCostRatio = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstStickyEscapeCacheCostRatio, defaults.CostFirstStickyEscapeCacheCostRatio), 0.01, 1)
	setting.CostFirstStickyEscapeMaxSpeedDrop = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstStickyEscapeMaxSpeedDrop, defaults.CostFirstStickyEscapeMaxSpeedDrop), 0, 1)
	setting.CostFirstStickyEscapeCacheSpeedDrop = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstStickyEscapeCacheSpeedDrop, defaults.CostFirstStickyEscapeCacheSpeedDrop), 0, 1)
	setting.CostFirstStickyEscapeMinSamples = normalizeModelGatewayConfigMin(setting.CostFirstStickyEscapeMinSamples, 1, defaults.CostFirstStickyEscapeMinSamples)
	setting.CostFirstStickyEscapeSuccessSlack = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstStickyEscapeSuccessSlack, defaults.CostFirstStickyEscapeSuccessSlack), 0, 1)
	setting.CostFirstGuardMultiple = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstGuardMultiple, defaults.CostFirstGuardMultiple), 1.01, 100)
	setting.CostFirstGuardSuccessAdvantage = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstGuardSuccessAdvantage, defaults.CostFirstGuardSuccessAdvantage), 0, 1)
	setting.CostFirstGuardSpeedAdvantage = clampModelGatewayConfigFloat(defaultFloat(setting.CostFirstGuardSpeedAdvantage, defaults.CostFirstGuardSpeedAdvantage), 0, 1)
	setting.ChannelPriorityTieBreakScoreDelta = clampModelGatewayConfigFloat(defaultFloat(setting.ChannelPriorityTieBreakScoreDelta, defaults.ChannelPriorityTieBreakScoreDelta), 0.000001, 1)
	if setting.UpstreamErrorRuleVersion <= 0 && setting.UpstreamErrorRules == nil {
		setting.UpstreamErrorClassificationEnabled = defaults.UpstreamErrorClassificationEnabled
	}
	upstreamRules, err := normalizeModelGatewayUpstreamErrorRules(setting.UpstreamErrorRules, setting.UpstreamErrorRuleVersion)
	if err != nil {
		return scheduler_setting.SchedulerSetting{}, err
	}
	setting.UpstreamErrorRuleVersion = scheduler_setting.UpstreamErrorRuleVersion
	setting.UpstreamErrorRules = upstreamRules
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
	setting.ProbeLowScoreThreshold = clampModelGatewayConfigFloat(defaultFloat(setting.ProbeLowScoreThreshold, defaults.ProbeLowScoreThreshold), 0.01, 1)
	setting.ProbeMissingSampleThreshold = normalizeModelGatewayConfigMin(setting.ProbeMissingSampleThreshold, 1, defaults.ProbeMissingSampleThreshold)
	setting.ProbeLongNoSuccessSeconds = normalizeModelGatewayConfigMin(setting.ProbeLongNoSuccessSeconds, 1, defaults.ProbeLongNoSuccessSeconds)
	setting.ProbeRecoverySuccessesRequired = normalizeModelGatewayConfigMin(setting.ProbeRecoverySuccessesRequired, 1, defaults.ProbeRecoverySuccessesRequired)
	setting.ProbeRecoverableScoreItems = modelgatewayprobe.NormalizeRecoverableScoreItems(defaultNilStringSlice(setting.ProbeRecoverableScoreItems, defaults.ProbeRecoverableScoreItems))
	setting.ProbeRecentRealRequestWindowSeconds = normalizeModelGatewayConfigMin(setting.ProbeRecentRealRequestWindowSeconds, 1, defaults.ProbeRecentRealRequestWindowSeconds)
	setting.ProbeGoodBaselineMinSamples = normalizeModelGatewayConfigMin(setting.ProbeGoodBaselineMinSamples, 1, defaultInt(defaults.ProbeGoodBaselineMinSamples, defaults.ProbeMissingSampleThreshold))
	setting.ProbeGoodBaselineWindowSeconds = normalizeModelGatewayConfigMin(setting.ProbeGoodBaselineWindowSeconds, 1, defaults.ProbeGoodBaselineWindowSeconds)
	setting.ProbePromptCategories = modelgatewayprobe.NormalizePromptCategories(defaultNilStringSlice(setting.ProbePromptCategories, defaults.ProbePromptCategories))
	setting.RelayTotalTimeoutSeconds = normalizeModelGatewayConfigMin(setting.RelayTotalTimeoutSeconds, 1, defaults.RelayTotalTimeoutSeconds)
	setting.ChannelTimeoutDegradeWindowSeconds = normalizeModelGatewayConfigMin(setting.ChannelTimeoutDegradeWindowSeconds, 1, defaults.ChannelTimeoutDegradeWindowSeconds)
	setting.ChannelTimeoutDegradeMinSamples = normalizeModelGatewayConfigMin(setting.ChannelTimeoutDegradeMinSamples, 1, defaults.ChannelTimeoutDegradeMinSamples)
	setting.ChannelTimeoutDegradeThreshold = clampModelGatewayConfigFloat(defaultFloat(setting.ChannelTimeoutDegradeThreshold, defaults.ChannelTimeoutDegradeThreshold), 0.01, 1)
	setting.ChannelTimeoutDegradeConsecutive = normalizeModelGatewayConfigMin(setting.ChannelTimeoutDegradeConsecutive, 1, defaults.ChannelTimeoutDegradeConsecutive)
	setting.ChannelTimeoutRecoveryProbeSuccesses = normalizeModelGatewayConfigMin(setting.ChannelTimeoutRecoveryProbeSuccesses, 1, defaults.ChannelTimeoutRecoveryProbeSuccesses)
	setting.CostCalculationIntervalSeconds = normalizeModelGatewayConfigMin(setting.CostCalculationIntervalSeconds, 1, defaults.CostCalculationIntervalSeconds)
	setting.CostCalculationWorkerCount = normalizeModelGatewayConfigMin(setting.CostCalculationWorkerCount, 1, defaults.CostCalculationWorkerCount)
	setting.CostCalculationBatchSize = normalizeModelGatewayConfigMin(setting.CostCalculationBatchSize, 1, defaults.CostCalculationBatchSize)
	setting.DynamicBillingProfitRate = clampModelGatewayConfigFloat(defaultFloat(setting.DynamicBillingProfitRate, defaults.DynamicBillingProfitRate), 0, 0.95)
	setting.DynamicBillingWindowSamples = normalizeModelGatewayConfigMin(setting.DynamicBillingWindowSamples, 1, defaults.DynamicBillingWindowSamples)
	setting.DynamicBillingWindowMinutes = normalizeModelGatewayConfigMin(setting.DynamicBillingWindowMinutes, 1, defaults.DynamicBillingWindowMinutes)
	setting.DynamicBillingMinSamples = normalizeModelGatewayConfigMin(setting.DynamicBillingMinSamples, 1, defaults.DynamicBillingMinSamples)
	setting.DynamicBillingRefreshSeconds = normalizeModelGatewayConfigMin(setting.DynamicBillingRefreshSeconds, 1, defaults.DynamicBillingRefreshSeconds)
	setting.DynamicBillingMaxAgeSeconds = normalizeModelGatewayConfigMin(setting.DynamicBillingMaxAgeSeconds, 1, defaults.DynamicBillingMaxAgeSeconds)
	if setting.DynamicBillingCostSource == "" {
		setting.DynamicBillingCostSource = defaults.DynamicBillingCostSource
	}
	if !validModelGatewayConfigValue(setting.DynamicBillingCostSource, modelGatewayConfigDynamicBillingCostSources()) {
		return scheduler_setting.SchedulerSetting{}, errors.New("invalid dynamic_billing_cost_source")
	}
	if setting.DynamicBillingApplyMode == "" {
		setting.DynamicBillingApplyMode = defaults.DynamicBillingApplyMode
	}
	if !validModelGatewayConfigValue(setting.DynamicBillingApplyMode, modelGatewayConfigDynamicBillingApplyModes()) {
		return scheduler_setting.SchedulerSetting{}, errors.New("invalid dynamic_billing_apply_mode")
	}
	setting.DynamicBillingProfitWindowHours = normalizeModelGatewayConfigMin(setting.DynamicBillingProfitWindowHours, 1, defaults.DynamicBillingProfitWindowHours)
	setting.DynamicBillingMinTokens = normalizeModelGatewayConfigMin(setting.DynamicBillingMinTokens, 1, defaults.DynamicBillingMinTokens)
	setting.DynamicBillingMinRequests = normalizeModelGatewayConfigMin(setting.DynamicBillingMinRequests, 1, defaults.DynamicBillingMinRequests)
	setting.DynamicBillingMinSuccessRequests = normalizeModelGatewayConfigMin(setting.DynamicBillingMinSuccessRequests, 1, defaults.DynamicBillingMinSuccessRequests)
	setting.DynamicBillingMinRatio = clampModelGatewayConfigFloat(defaultFloat(setting.DynamicBillingMinRatio, defaults.DynamicBillingMinRatio), 0.000001, 100)
	setting.DynamicBillingMaxRatio = clampModelGatewayConfigFloat(defaultFloat(setting.DynamicBillingMaxRatio, defaults.DynamicBillingMaxRatio), setting.DynamicBillingMinRatio, 100)
	setting.DynamicBillingMaxStepChange = clampModelGatewayConfigFloat(defaultFloat(setting.DynamicBillingMaxStepChange, defaults.DynamicBillingMaxStepChange), 0.01, 10)
	setting.FailureFastWindowSeconds = normalizeModelGatewayConfigMin(setting.FailureFastWindowSeconds, 1, defaults.FailureFastWindowSeconds)
	setting.FailureMainWindowSeconds = normalizeModelGatewayConfigMin(setting.FailureMainWindowSeconds, 1, defaults.FailureMainWindowSeconds)
	setting.FailureFallbackWindowSeconds = normalizeModelGatewayConfigMin(setting.FailureFallbackWindowSeconds, 1, defaults.FailureFallbackWindowSeconds)
	if setting.ProxySameBrandReusePolicy == "" {
		setting.ProxySameBrandReusePolicy = defaults.ProxySameBrandReusePolicy
	}
	if !validModelGatewayConfigValue(setting.ProxySameBrandReusePolicy, modelGatewayConfigProxyReusePolicies()) {
		return scheduler_setting.SchedulerSetting{}, errors.New("invalid proxy_same_brand_reuse_policy")
	}

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
		if policy.BillingRatioMode == "" {
			policy.BillingRatioMode = scheduler_setting.BillingRatioModeStatic
		}
		if policy.BillingRatioMode != scheduler_setting.BillingRatioModeStatic && policy.BillingRatioMode != scheduler_setting.BillingRatioModeDynamic {
			return nil, fmt.Errorf("invalid billing_ratio_mode for group %s", group)
		}
		policy.CandidateGroups = uniqueTrimmedModelGatewayStrings(policy.CandidateGroups)
		policy.PrimaryChannelIDs = uniquePositiveModelGatewayInts(policy.PrimaryChannelIDs)
		policy.FallbackChannelIDs = uniquePositiveModelGatewayInts(policy.FallbackChannelIDs)
		if policy.PrimaryWaitTimeoutMs < 0 {
			policy.PrimaryWaitTimeoutMs = 0
		}
		if policy.PrimaryQueueMaxDepth < 0 {
			policy.PrimaryQueueMaxDepth = 0
		}
		if policy.ResourceProtectionEnabled {
			if len(policy.PrimaryChannelIDs) == 0 {
				return nil, fmt.Errorf("primary_channel_ids is required for group %s when resource protection is enabled", group)
			}
			if policy.PrimaryWaitTimeoutMs <= 0 {
				policy.PrimaryWaitTimeoutMs = defaultInt(setting.QueueDefaultTimeoutMs, 3000)
			}
			policy.PrimaryWaitTimeoutMs = normalizeModelGatewayConfigMin(policy.PrimaryWaitTimeoutMs, 1, 3000)
			policy.PrimaryWaitTimeoutMs = normalizeModelGatewayConfigMax(policy.PrimaryWaitTimeoutMs, 600000)
			if policy.PrimaryQueueMaxDepth <= 0 {
				policy.PrimaryQueueMaxDepth = defaultInt(setting.QueueMaxDepthPerChannel, 64)
			}
			policy.PrimaryQueueMaxDepth = normalizeModelGatewayConfigMin(policy.PrimaryQueueMaxDepth, 1, 64)
			policy.PrimaryQueueMaxDepth = normalizeModelGatewayConfigMax(policy.PrimaryQueueMaxDepth, 10000)
		}
		result[group] = policy
	}
	return result, nil
}

func uniquePositiveModelGatewayInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
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

func normalizeModelGatewayUpstreamErrorRules(src []scheduler_setting.UpstreamErrorRule, version int) ([]scheduler_setting.UpstreamErrorRule, error) {
	if src == nil && version <= 0 {
		return scheduler_setting.DefaultUpstreamErrorRules(), nil
	}
	if src == nil {
		return scheduler_setting.DefaultUpstreamErrorRules(), nil
	}
	result := make([]scheduler_setting.UpstreamErrorRule, 0, len(src))
	seenIDs := make(map[string]struct{}, len(src))
	for index, rule := range src {
		rule.ID = normalizeModelGatewayUpstreamRuleID(rule.ID, index)
		if _, ok := seenIDs[rule.ID]; ok {
			return nil, fmt.Errorf("duplicate upstream_error_rule id %s", rule.ID)
		}
		seenIDs[rule.ID] = struct{}{}
		rule.Kind = strings.ToLower(strings.TrimSpace(rule.Kind))
		if !modelgatewayupstreamerror.IsKnownKind(rule.Kind) {
			return nil, fmt.Errorf("invalid upstream_error_rule kind %s", rule.Kind)
		}
		rule.SchedulerAction = strings.ToLower(strings.TrimSpace(rule.SchedulerAction))
		if rule.SchedulerAction == "" {
			rule.SchedulerAction = defaultModelGatewayUpstreamErrorAction(rule.Kind)
		}
		if !modelgatewayupstreamerror.IsKnownAction(rule.SchedulerAction) {
			return nil, fmt.Errorf("invalid upstream_error_rule scheduler_action %s", rule.SchedulerAction)
		}
		rule.Priority = clampModelGatewayConfigInt(rule.Priority, 0, 10000)
		rule.StatusCodes = uniqueModelGatewayHTTPStatusCodes(rule.StatusCodes)
		rule.Keywords = normalizeModelGatewayUpstreamErrorKeywords(rule.Keywords)
		rule.AvoidanceSeconds = normalizeModelGatewayConfigNonNegative(rule.AvoidanceSeconds)
		rule.AvoidanceSeconds = normalizeModelGatewayConfigMax(rule.AvoidanceSeconds, 86400)
		rule.Description = strings.TrimSpace(rule.Description)
		if len(rule.Description) > 512 {
			rule.Description = rule.Description[:512]
		}
		if len(rule.StatusCodes) == 0 && upstreamErrorKeywordRuleEmpty(rule.Keywords) {
			return nil, fmt.Errorf("upstream_error_rule %s must define status_codes or keywords", rule.ID)
		}
		result = append(result, rule)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Priority == result[j].Priority {
			return result[i].ID < result[j].ID
		}
		return result[i].Priority > result[j].Priority
	})
	return result, nil
}

func normalizeModelGatewayUpstreamRuleID(id string, index int) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return fmt.Sprintf("rule_%d", index+1)
	}
	var builder strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
			continue
		}
		if builder.Len() == 0 || builder.String()[builder.Len()-1] != '_' {
			builder.WriteByte('_')
		}
	}
	normalized := strings.Trim(builder.String(), "_.-")
	if normalized == "" {
		return fmt.Sprintf("rule_%d", index+1)
	}
	if len(normalized) > 80 {
		normalized = normalized[:80]
	}
	return normalized
}

func defaultModelGatewayUpstreamErrorAction(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case scheduler_setting.UpstreamErrorKindRequestLimit,
		scheduler_setting.UpstreamErrorKindPolicySafety:
		return scheduler_setting.UpstreamErrorActionStop
	default:
		return scheduler_setting.UpstreamErrorActionSwitchChannel
	}
}

func uniqueModelGatewayHTTPStatusCodes(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value < 100 || value > 599 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func normalizeModelGatewayUpstreamErrorKeywords(src scheduler_setting.UpstreamErrorKeywordRule) scheduler_setting.UpstreamErrorKeywordRule {
	return scheduler_setting.UpstreamErrorKeywordRule{
		Code:     uniqueTrimmedModelGatewayStringsLimited(src.Code, 64, 128),
		Type:     uniqueTrimmedModelGatewayStringsLimited(src.Type, 64, 128),
		Message:  uniqueTrimmedModelGatewayStringsLimited(src.Message, 64, 256),
		Metadata: uniqueTrimmedModelGatewayStringsLimited(src.Metadata, 64, 256),
		Header:   uniqueTrimmedModelGatewayStringsLimited(src.Header, 64, 128),
	}
}

func uniqueTrimmedModelGatewayStringsLimited(values []string, maxItems int, maxLen int) []string {
	if len(values) == 0 || maxItems <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > maxLen {
			value = value[:maxLen]
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) >= maxItems {
			break
		}
	}
	return result
}

func upstreamErrorKeywordRuleEmpty(keywords scheduler_setting.UpstreamErrorKeywordRule) bool {
	return len(keywords.Code) == 0 &&
		len(keywords.Type) == 0 &&
		len(keywords.Message) == 0 &&
		len(keywords.Metadata) == 0 &&
		len(keywords.Header) == 0
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
	probeRecoverableScoreItems, err := common.Marshal(setting.ProbeRecoverableScoreItems)
	if err != nil {
		return nil, err
	}
	probePromptCategories, err := common.Marshal(setting.ProbePromptCategories)
	if err != nil {
		return nil, err
	}
	upstreamErrorRules, err := common.Marshal(setting.UpstreamErrorRules)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"enabled":                                             strconv.FormatBool(setting.Enabled),
		"default_mode":                                        setting.DefaultMode,
		"rollout_percent":                                     strconv.Itoa(setting.RolloutPercent),
		"default_strategy":                                    setting.DefaultStrategy,
		"snapshot_refresh_ms":                                 strconv.Itoa(setting.SnapshotRefreshMs),
		"sticky_ttl_seconds":                                  strconv.Itoa(setting.StickyTTLSeconds),
		"sticky_keep_score_ratio":                             strconv.FormatFloat(setting.StickyKeepScoreRatio, 'f', -1, 64),
		"sticky_save_on_select":                               strconv.FormatBool(setting.StickySaveOnSelect),
		"sticky_renew_on_success":                             strconv.FormatBool(setting.StickyRenewOnSuccess),
		"sticky_failure_policy":                               setting.StickyFailurePolicy,
		"cache_affinity_enabled":                              strconv.FormatBool(setting.CacheAffinityEnabled),
		"cache_affinity_keep_score_ratio":                     strconv.FormatFloat(setting.CacheAffinityKeepScoreRatio, 'f', -1, 64),
		"cost_first_sticky_escape_enabled":                    strconv.FormatBool(setting.CostFirstStickyEscapeEnabled),
		"cost_first_sticky_escape_cost_ratio":                 strconv.FormatFloat(setting.CostFirstStickyEscapeCostRatio, 'f', -1, 64),
		"cost_first_sticky_escape_cache_cost_ratio":           strconv.FormatFloat(setting.CostFirstStickyEscapeCacheCostRatio, 'f', -1, 64),
		"cost_first_sticky_escape_max_speed_score_drop":       strconv.FormatFloat(setting.CostFirstStickyEscapeMaxSpeedDrop, 'f', -1, 64),
		"cost_first_sticky_escape_cache_max_speed_score_drop": strconv.FormatFloat(setting.CostFirstStickyEscapeCacheSpeedDrop, 'f', -1, 64),
		"cost_first_sticky_escape_min_samples":                strconv.Itoa(setting.CostFirstStickyEscapeMinSamples),
		"cost_first_sticky_escape_success_slack":              strconv.FormatFloat(setting.CostFirstStickyEscapeSuccessSlack, 'f', -1, 64),
		"cost_first_guard_enabled":                            strconv.FormatBool(setting.CostFirstGuardEnabled),
		"cost_first_guard_multiple":                           strconv.FormatFloat(setting.CostFirstGuardMultiple, 'f', -1, 64),
		"cost_first_guard_success_advantage":                  strconv.FormatFloat(setting.CostFirstGuardSuccessAdvantage, 'f', -1, 64),
		"cost_first_guard_speed_advantage":                    strconv.FormatFloat(setting.CostFirstGuardSpeedAdvantage, 'f', -1, 64),
		"channel_priority_tie_break_enabled":                  strconv.FormatBool(setting.ChannelPriorityTieBreakEnabled),
		"channel_priority_tie_break_score_delta":              strconv.FormatFloat(setting.ChannelPriorityTieBreakScoreDelta, 'f', -1, 64),
		"upstream_error_classification_enabled":               strconv.FormatBool(setting.UpstreamErrorClassificationEnabled),
		"upstream_error_rule_version":                         strconv.Itoa(setting.UpstreamErrorRuleVersion),
		"upstream_error_rules":                                string(upstreamErrorRules),
		"queue_enabled":                                       strconv.FormatBool(setting.QueueEnabled),
		"queue_default_timeout_ms":                            strconv.Itoa(setting.QueueDefaultTimeoutMs),
		"queue_max_depth_per_channel":                         strconv.Itoa(setting.QueueMaxDepthPerChannel),
		"queue_depth_multiplier":                              strconv.Itoa(setting.QueueDepthMultiplier),
		"queue_high_priority_threshold":                       strconv.Itoa(setting.QueueHighPriorityThreshold),
		"queue_high_priority_extra_depth":                     strconv.Itoa(setting.QueueHighPriorityExtraDepth),
		"queue_high_priority_reserved_depth":                  strconv.Itoa(setting.QueueHighPriorityReservedDepth),
		"queue_absolute_max_depth":                            strconv.Itoa(setting.QueueAbsoluteMaxDepth),
		"circuit_breaker_enabled":                             strconv.FormatBool(setting.CircuitBreakerEnabled),
		"circuit_failure_threshold":                           strconv.FormatFloat(setting.CircuitFailureThreshold, 'f', -1, 64),
		"circuit_min_samples":                                 strconv.Itoa(setting.CircuitMinSamples),
		"circuit_open_seconds":                                strconv.Itoa(setting.CircuitOpenSeconds),
		"circuit_half_open_probe_count":                       strconv.Itoa(setting.CircuitHalfOpenProbeCount),
		"circuit_error_policies":                              string(circuitErrorPolicies),
		"cooldown_max_seconds":                                strconv.Itoa(setting.CooldownMaxSeconds),
		"runtime_sync_enabled":                                strconv.FormatBool(setting.RuntimeSyncEnabled),
		"runtime_sync_redis_enabled":                          strconv.FormatBool(setting.RuntimeSyncRedisEnabled),
		"runtime_sync_node_id":                                setting.RuntimeSyncNodeID,
		"runtime_sync_ttl_seconds":                            strconv.Itoa(setting.RuntimeSyncTTLSeconds),
		"runtime_sync_queue_min_interval_ms":                  strconv.Itoa(setting.RuntimeSyncQueueMinIntervalMs),
		"runtime_sync_event_push_enabled":                     strconv.FormatBool(setting.RuntimeSyncEventPushEnabled),
		"runtime_sync_event_subscribe_enabled":                strconv.FormatBool(setting.RuntimeSyncEventSubscribeEnabled),
		"probe_enabled":                                       strconv.FormatBool(setting.ProbeEnabled),
		"probe_interval_seconds":                              strconv.Itoa(setting.ProbeIntervalSeconds),
		"probe_worker_count":                                  strconv.Itoa(setting.ProbeWorkerCount),
		"probe_timeout_seconds":                               strconv.Itoa(setting.ProbeTimeoutSeconds),
		"probe_max_per_tick":                                  strconv.Itoa(setting.ProbeMaxPerTick),
		"probe_min_channel_interval_seconds":                  strconv.Itoa(setting.ProbeMinChannelIntervalSeconds),
		"probe_low_score_threshold":                           strconv.FormatFloat(setting.ProbeLowScoreThreshold, 'f', -1, 64),
		"probe_missing_sample_threshold":                      strconv.Itoa(setting.ProbeMissingSampleThreshold),
		"probe_long_no_success_seconds":                       strconv.Itoa(setting.ProbeLongNoSuccessSeconds),
		"probe_recovery_successes_required":                   strconv.Itoa(setting.ProbeRecoverySuccessesRequired),
		"probe_failure_avoidance_priority_enabled":            strconv.FormatBool(setting.ProbeFailureAvoidancePriorityEnabled),
		"probe_recoverable_score_items":                       string(probeRecoverableScoreItems),
		"probe_skip_recent_real_request_enabled":              strconv.FormatBool(setting.ProbeSkipRecentRealRequestEnabled),
		"probe_recent_real_request_window_seconds":            strconv.Itoa(setting.ProbeRecentRealRequestWindowSeconds),
		"probe_good_baseline_enabled":                         strconv.FormatBool(setting.ProbeGoodBaselineEnabled),
		"probe_good_baseline_min_samples":                     strconv.Itoa(setting.ProbeGoodBaselineMinSamples),
		"probe_good_baseline_window_seconds":                  strconv.Itoa(setting.ProbeGoodBaselineWindowSeconds),
		"probe_prompt_library_enabled":                        strconv.FormatBool(setting.ProbePromptLibraryEnabled),
		"probe_prompt_categories":                             string(probePromptCategories),
		"relay_total_timeout_enabled":                         strconv.FormatBool(setting.RelayTotalTimeoutEnabled),
		"relay_total_timeout_seconds":                         strconv.Itoa(setting.RelayTotalTimeoutSeconds),
		"channel_timeout_degrade_enabled":                     strconv.FormatBool(setting.ChannelTimeoutDegradeEnabled),
		"channel_timeout_degrade_window_seconds":              strconv.Itoa(setting.ChannelTimeoutDegradeWindowSeconds),
		"channel_timeout_degrade_min_samples":                 strconv.Itoa(setting.ChannelTimeoutDegradeMinSamples),
		"channel_timeout_degrade_threshold":                   strconv.FormatFloat(setting.ChannelTimeoutDegradeThreshold, 'f', -1, 64),
		"channel_timeout_degrade_consecutive":                 strconv.Itoa(setting.ChannelTimeoutDegradeConsecutive),
		"channel_timeout_recovery_probe_successes":            strconv.Itoa(setting.ChannelTimeoutRecoveryProbeSuccesses),
		"cost_calculation_enabled":                            strconv.FormatBool(setting.CostCalculationEnabled),
		"cost_calculation_interval_seconds":                   strconv.Itoa(setting.CostCalculationIntervalSeconds),
		"cost_calculation_worker_count":                       strconv.Itoa(setting.CostCalculationWorkerCount),
		"cost_calculation_batch_size":                         strconv.Itoa(setting.CostCalculationBatchSize),
		"dynamic_billing_enabled":                             strconv.FormatBool(setting.DynamicBillingEnabled),
		"dynamic_billing_profit_rate":                         strconv.FormatFloat(setting.DynamicBillingProfitRate, 'f', -1, 64),
		"dynamic_billing_cost_source":                         setting.DynamicBillingCostSource,
		"dynamic_billing_profit_window_hours":                 strconv.Itoa(setting.DynamicBillingProfitWindowHours),
		"dynamic_billing_min_tokens":                          strconv.Itoa(setting.DynamicBillingMinTokens),
		"dynamic_billing_min_requests":                        strconv.Itoa(setting.DynamicBillingMinRequests),
		"dynamic_billing_min_success_requests":                strconv.Itoa(setting.DynamicBillingMinSuccessRequests),
		"dynamic_billing_min_ratio":                           strconv.FormatFloat(setting.DynamicBillingMinRatio, 'f', -1, 64),
		"dynamic_billing_max_ratio":                           strconv.FormatFloat(setting.DynamicBillingMaxRatio, 'f', -1, 64),
		"dynamic_billing_max_step_change":                     strconv.FormatFloat(setting.DynamicBillingMaxStepChange, 'f', -1, 64),
		"dynamic_billing_apply_mode":                          setting.DynamicBillingApplyMode,
		"dynamic_billing_window_samples":                      strconv.Itoa(setting.DynamicBillingWindowSamples),
		"dynamic_billing_window_minutes":                      strconv.Itoa(setting.DynamicBillingWindowMinutes),
		"dynamic_billing_min_samples":                         strconv.Itoa(setting.DynamicBillingMinSamples),
		"dynamic_billing_refresh_seconds":                     strconv.Itoa(setting.DynamicBillingRefreshSeconds),
		"dynamic_billing_max_age_seconds":                     strconv.Itoa(setting.DynamicBillingMaxAgeSeconds),
		"dynamic_billing_enabled_at":                          strconv.FormatInt(setting.DynamicBillingEnabledAt, 10),
		"success_weight":                                      strconv.FormatFloat(setting.SuccessWeight, 'f', -1, 64),
		"speed_weight":                                        strconv.FormatFloat(setting.SpeedWeight, 'f', -1, 64),
		"load_weight":                                         strconv.FormatFloat(setting.LoadWeight, 'f', -1, 64),
		"cost_weight":                                         strconv.FormatFloat(setting.CostWeight, 'f', -1, 64),
		"group_weight":                                        strconv.FormatFloat(setting.GroupWeight, 'f', -1, 64),
		"group_priority_ratio":                                string(groupPriorityRatio),
		"group_policies":                                      string(groupPolicies),
		"failure_fast_window_seconds":                         strconv.Itoa(setting.FailureFastWindowSeconds),
		"failure_main_window_seconds":                         strconv.Itoa(setting.FailureMainWindowSeconds),
		"failure_fallback_window_seconds":                     strconv.Itoa(setting.FailureFallbackWindowSeconds),
		"proxy_same_brand_reuse_policy":                       setting.ProxySameBrandReusePolicy,
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

func modelGatewayConfigProxyReusePolicies() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.ProxyReusePolicyWarn:    {},
		scheduler_setting.ProxyReusePolicyConfirm: {},
		scheduler_setting.ProxyReusePolicyBlock:   {},
	}
}

func modelGatewayConfigDynamicBillingCostSources() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.DynamicBillingCostSourceSampleCost: {},
		scheduler_setting.DynamicBillingCostSourceProfit24h:  {},
	}
}

func modelGatewayConfigDynamicBillingApplyModes() map[string]struct{} {
	return map[string]struct{}{
		scheduler_setting.DynamicBillingApplyModeObserve: {},
		scheduler_setting.DynamicBillingApplyModeManual:  {},
		scheduler_setting.DynamicBillingApplyModeAuto:    {},
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

func normalizeModelGatewayConfigMax(value int, maxValue int) int {
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func defaultInt(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultNilStringSlice(value []string, fallback []string) []string {
	if value == nil {
		return append([]string(nil), fallback...)
	}
	return append([]string(nil), value...)
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
