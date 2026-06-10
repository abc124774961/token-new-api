package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayprobe "github.com/QuantumNous/new-api/pkg/modelgateway/probe"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelGatewayConfigAPIResponse struct {
	Success bool                       `json:"success"`
	Message string                     `json:"message"`
	Data    ModelGatewayConfigResponse `json:"data"`
}

func setupModelGatewayConfigControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	oldDB := model.DB
	oldOptionMap := common.OptionMap
	model.DB = db
	common.OptionMap = map[string]string{}
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	t.Cleanup(func() {
		modelgatewayprobe.StopDefaultProbeScheduler()
		modelgatewaydynamicbilling.StopDefaultRefresher()
		restoreSetting()
		model.DB = oldDB
		common.OptionMap = oldOptionMap
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestModelGatewayConfigUpdatePersistsSchedulerSetting(t *testing.T) {
	db := setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.Enabled = true
	setting.DefaultMode = scheduler_setting.ModeShadow
	setting.RolloutPercent = 35
	setting.DefaultStrategy = scheduler_setting.StrategySpeedFirst
	setting.QueueDefaultTimeoutMs = 1500
	setting.QueueHighPriorityThreshold = 7
	setting.QueueHighPriorityExtraDepth = 3
	setting.QueueHighPriorityReservedDepth = 2
	setting.QueueAbsoluteMaxDepth = 9
	setting.RuntimeSyncEnabled = true
	setting.RuntimeSyncRedisEnabled = false
	setting.RuntimeSyncNodeID = " node-a "
	setting.RuntimeSyncTTLSeconds = 120
	setting.RuntimeSyncQueueMinIntervalMs = 250
	setting.RuntimeSyncEventPushEnabled = true
	setting.RuntimeSyncEventSubscribeEnabled = true
	setting.ProbeEnabled = true
	setting.ProbeIntervalSeconds = 45
	setting.ProbeWorkerCount = 4
	setting.ProbeTimeoutSeconds = 6
	setting.ProbeMaxPerTick = 7
	setting.ProbeMinChannelIntervalSeconds = 180
	setting.ProbeLowScoreThreshold = 0.66
	setting.ProbeMissingSampleThreshold = 4
	setting.ProbeLongNoSuccessSeconds = 2400
	setting.ProbeRecoverySuccessesRequired = 3
	setting.ProbeFailureAvoidancePriorityEnabled = false
	setting.ProbeRecoverableScoreItems = []string{"completion_rate", "bad", "first_byte_backlog"}
	setting.ProbeSkipRecentRealRequestEnabled = false
	setting.ProbeRecentRealRequestWindowSeconds = 900
	setting.ProbeGoodBaselineEnabled = false
	setting.ProbeGoodBaselineMinSamples = 8
	setting.ProbeGoodBaselineWindowSeconds = 3600
	setting.ProbePromptLibraryEnabled = false
	setting.ProbePromptCategories = []string{"zh", "bad", "long"}
	setting.CostCalculationEnabled = true
	setting.CostCalculationIntervalSeconds = 4
	setting.CostCalculationWorkerCount = 3
	setting.CostCalculationBatchSize = 80
	setting.DynamicBillingCostSource = scheduler_setting.DynamicBillingCostSourceProfit24h
	setting.DynamicBillingApplyMode = scheduler_setting.DynamicBillingApplyModeAuto
	setting.DynamicBillingProfitWindowHours = 24
	setting.DynamicBillingMinTokens = 1200
	setting.DynamicBillingMinRequests = 22
	setting.DynamicBillingMinSuccessRequests = 6
	setting.DynamicBillingMinRatio = 0.02
	setting.DynamicBillingMaxRatio = 1.8
	setting.DynamicBillingMaxStepChange = 0.25
	setting.DynamicBillingWindowSamples = 280
	setting.ProxySameBrandReusePolicy = scheduler_setting.ProxyReusePolicyConfirm
	setting.StickySaveOnSelect = true
	setting.StickyRenewOnSuccess = false
	setting.StickyFailurePolicy = scheduler_setting.StickyFailurePolicyKeep
	setting.CostFirstStickyEscapeEnabled = true
	setting.CostFirstStickyEscapeCostRatio = 0.72
	setting.CostFirstStickyEscapeCacheCostRatio = 0.50
	setting.CostFirstStickyEscapeMaxSpeedDrop = 0.07
	setting.CostFirstStickyEscapeCacheSpeedDrop = 0.04
	setting.CostFirstStickyEscapeMinSamples = 8
	setting.CostFirstStickyEscapeSuccessSlack = 0.03
	setting.CostFirstGuardEnabled = true
	setting.CostFirstGuardMultiple = 1.8
	setting.CostFirstGuardSuccessAdvantage = 0.04
	setting.CostFirstGuardSpeedAdvantage = 0.09
	setting.ChannelPriorityTieBreakEnabled = false
	setting.ChannelPriorityTieBreakScoreDelta = 0.049
	setting.CircuitErrorPolicies = map[string]scheduler_setting.CircuitErrorPolicySetting{
		"unknown": {
			FailureThreshold:   0.9,
			MinSamples:         9,
			OpenSeconds:        9,
			HalfOpenProbeCount: 9,
		},
		"rate_limit": {
			FailureThreshold:   0.25,
			MinSamples:         2,
			OpenSeconds:        45,
			HalfOpenProbeCount: 1,
		},
		" server_error ": {
			FailureThreshold:   0,
			MinSamples:         0,
			OpenSeconds:        0,
			HalfOpenProbeCount: 0,
		},
	}
	setting.GroupPriorityRatio = map[string]float64{"vip": 1.4}
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"vip": {
			Mode:                  scheduler_setting.ModeActive,
			Strategy:              scheduler_setting.StrategyStabilityFirst,
			AutoMode:              scheduler_setting.AutoModeFusion,
			CrossGroupFusion:      true,
			CandidateGroups:       []string{"vip", "default", "vip"},
			CacheAffinityEnabled:  true,
			QueueEnabled:          true,
			QueueHighPriority:     true,
			CircuitBreakerEnabled: true,
		},
	}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.True(t, payload.Data.Setting.Enabled)
	require.Equal(t, 35, payload.Data.Setting.RolloutPercent)
	require.Equal(t, 7, payload.Data.Setting.QueueHighPriorityThreshold)
	require.Equal(t, 3, payload.Data.Setting.QueueHighPriorityExtraDepth)
	require.Equal(t, 2, payload.Data.Setting.QueueHighPriorityReservedDepth)
	require.Equal(t, 9, payload.Data.Setting.QueueAbsoluteMaxDepth)
	require.False(t, payload.Data.Setting.RuntimeSyncRedisEnabled)
	require.Equal(t, "node-a", payload.Data.Setting.RuntimeSyncNodeID)
	require.Equal(t, 120, payload.Data.Setting.RuntimeSyncTTLSeconds)
	require.Equal(t, 250, payload.Data.Setting.RuntimeSyncQueueMinIntervalMs)
	require.True(t, payload.Data.Setting.RuntimeSyncEventPushEnabled)
	require.True(t, payload.Data.Setting.RuntimeSyncEventSubscribeEnabled)
	require.True(t, payload.Data.Setting.ProbeEnabled)
	require.Equal(t, 45, payload.Data.Setting.ProbeIntervalSeconds)
	require.Equal(t, 4, payload.Data.Setting.ProbeWorkerCount)
	require.Equal(t, 6, payload.Data.Setting.ProbeTimeoutSeconds)
	require.Equal(t, 7, payload.Data.Setting.ProbeMaxPerTick)
	require.Equal(t, 180, payload.Data.Setting.ProbeMinChannelIntervalSeconds)
	require.Equal(t, 0.66, payload.Data.Setting.ProbeLowScoreThreshold)
	require.Equal(t, 4, payload.Data.Setting.ProbeMissingSampleThreshold)
	require.Equal(t, 2400, payload.Data.Setting.ProbeLongNoSuccessSeconds)
	require.Equal(t, 3, payload.Data.Setting.ProbeRecoverySuccessesRequired)
	require.False(t, payload.Data.Setting.ProbeFailureAvoidancePriorityEnabled)
	require.Equal(t, []string{"completion_rate", "first_byte_backlog"}, payload.Data.Setting.ProbeRecoverableScoreItems)
	require.False(t, payload.Data.Setting.ProbeSkipRecentRealRequestEnabled)
	require.Equal(t, 900, payload.Data.Setting.ProbeRecentRealRequestWindowSeconds)
	require.False(t, payload.Data.Setting.ProbeGoodBaselineEnabled)
	require.Equal(t, 8, payload.Data.Setting.ProbeGoodBaselineMinSamples)
	require.Equal(t, 3600, payload.Data.Setting.ProbeGoodBaselineWindowSeconds)
	require.False(t, payload.Data.Setting.ProbePromptLibraryEnabled)
	require.Equal(t, []string{"zh", "long"}, payload.Data.Setting.ProbePromptCategories)
	require.True(t, payload.Data.Setting.CostCalculationEnabled)
	require.Equal(t, 4, payload.Data.Setting.CostCalculationIntervalSeconds)
	require.Equal(t, 3, payload.Data.Setting.CostCalculationWorkerCount)
	require.Equal(t, 80, payload.Data.Setting.CostCalculationBatchSize)
	require.Equal(t, scheduler_setting.DynamicBillingCostSourceProfit24h, payload.Data.Setting.DynamicBillingCostSource)
	require.Equal(t, scheduler_setting.DynamicBillingApplyModeAuto, payload.Data.Setting.DynamicBillingApplyMode)
	require.Equal(t, 24, payload.Data.Setting.DynamicBillingProfitWindowHours)
	require.Equal(t, 1200, payload.Data.Setting.DynamicBillingMinTokens)
	require.Equal(t, 22, payload.Data.Setting.DynamicBillingMinRequests)
	require.Equal(t, 6, payload.Data.Setting.DynamicBillingMinSuccessRequests)
	require.Equal(t, 0.02, payload.Data.Setting.DynamicBillingMinRatio)
	require.Equal(t, 1.8, payload.Data.Setting.DynamicBillingMaxRatio)
	require.Equal(t, 0.25, payload.Data.Setting.DynamicBillingMaxStepChange)
	require.Equal(t, 280, payload.Data.Setting.DynamicBillingWindowSamples)
	require.Equal(t, scheduler_setting.ProxyReusePolicyConfirm, payload.Data.Setting.ProxySameBrandReusePolicy)
	require.True(t, payload.Data.Setting.StickySaveOnSelect)
	require.False(t, payload.Data.Setting.StickyRenewOnSuccess)
	require.Equal(t, scheduler_setting.StickyFailurePolicyKeep, payload.Data.Setting.StickyFailurePolicy)
	require.True(t, payload.Data.Setting.CostFirstStickyEscapeEnabled)
	require.Equal(t, 0.72, payload.Data.Setting.CostFirstStickyEscapeCostRatio)
	require.Equal(t, 0.50, payload.Data.Setting.CostFirstStickyEscapeCacheCostRatio)
	require.Equal(t, 0.07, payload.Data.Setting.CostFirstStickyEscapeMaxSpeedDrop)
	require.Equal(t, 0.04, payload.Data.Setting.CostFirstStickyEscapeCacheSpeedDrop)
	require.Equal(t, 8, payload.Data.Setting.CostFirstStickyEscapeMinSamples)
	require.Equal(t, 0.03, payload.Data.Setting.CostFirstStickyEscapeSuccessSlack)
	require.True(t, payload.Data.Setting.CostFirstGuardEnabled)
	require.Equal(t, 1.8, payload.Data.Setting.CostFirstGuardMultiple)
	require.Equal(t, 0.04, payload.Data.Setting.CostFirstGuardSuccessAdvantage)
	require.Equal(t, 0.09, payload.Data.Setting.CostFirstGuardSpeedAdvantage)
	require.False(t, payload.Data.Setting.ChannelPriorityTieBreakEnabled)
	require.Equal(t, 0.049, payload.Data.Setting.ChannelPriorityTieBreakScoreDelta)
	require.Len(t, payload.Data.Setting.CircuitErrorPolicies, 2)
	require.Equal(t, 0.25, payload.Data.Setting.CircuitErrorPolicies["rate_limit"].FailureThreshold)
	require.Equal(t, 2, payload.Data.Setting.CircuitErrorPolicies["rate_limit"].MinSamples)
	require.Equal(t, 45, payload.Data.Setting.CircuitErrorPolicies["rate_limit"].OpenSeconds)
	require.Equal(t, 1, payload.Data.Setting.CircuitErrorPolicies["rate_limit"].HalfOpenProbeCount)
	require.Equal(t, payload.Data.Setting.CircuitFailureThreshold, payload.Data.Setting.CircuitErrorPolicies["server_error"].FailureThreshold)
	require.Equal(t, payload.Data.Setting.CircuitMinSamples, payload.Data.Setting.CircuitErrorPolicies["server_error"].MinSamples)
	require.Equal(t, payload.Data.Setting.CircuitOpenSeconds, payload.Data.Setting.CircuitErrorPolicies["server_error"].OpenSeconds)
	require.Equal(t, payload.Data.Setting.CircuitHalfOpenProbeCount, payload.Data.Setting.CircuitErrorPolicies["server_error"].HalfOpenProbeCount)
	require.Equal(t, []string{"vip", "default"}, payload.Data.Setting.GroupPolicies["vip"].CandidateGroups)
	require.Equal(t, scheduler_setting.BillingRatioModeStatic, payload.Data.Setting.GroupPolicies["vip"].BillingRatioMode)
	require.True(t, payload.Data.Setting.GroupPolicies["vip"].QueueHighPriority)
	require.Equal(t, scheduler_setting.ModeActive, scheduler_setting.GetSetting().GroupPolicies["vip"].Mode)

	var rolloutOption model.Option
	require.NoError(t, db.First(&rolloutOption, "key = ?", "scheduler_setting.rollout_percent").Error)
	require.Equal(t, "35", rolloutOption.Value)
	var policiesOption model.Option
	require.NoError(t, db.First(&policiesOption, "key = ?", "scheduler_setting.group_policies").Error)
	require.Contains(t, policiesOption.Value, `"vip"`)
	require.Contains(t, policiesOption.Value, `"queue_high_priority":true`)
	var queueExtraOption model.Option
	require.NoError(t, db.First(&queueExtraOption, "key = ?", "scheduler_setting.queue_high_priority_extra_depth").Error)
	require.Equal(t, "3", queueExtraOption.Value)
	var runtimeSyncRedisOption model.Option
	require.NoError(t, db.First(&runtimeSyncRedisOption, "key = ?", "scheduler_setting.runtime_sync_redis_enabled").Error)
	require.Equal(t, "false", runtimeSyncRedisOption.Value)
	var runtimeSyncNodeOption model.Option
	require.NoError(t, db.First(&runtimeSyncNodeOption, "key = ?", "scheduler_setting.runtime_sync_node_id").Error)
	require.Equal(t, "node-a", runtimeSyncNodeOption.Value)
	var runtimeSyncThrottleOption model.Option
	require.NoError(t, db.First(&runtimeSyncThrottleOption, "key = ?", "scheduler_setting.runtime_sync_queue_min_interval_ms").Error)
	require.Equal(t, "250", runtimeSyncThrottleOption.Value)
	var runtimeSyncEventSubscribeOption model.Option
	require.NoError(t, db.First(&runtimeSyncEventSubscribeOption, "key = ?", "scheduler_setting.runtime_sync_event_subscribe_enabled").Error)
	require.Equal(t, "true", runtimeSyncEventSubscribeOption.Value)
	var channelPriorityTieBreakEnabledOption model.Option
	require.NoError(t, db.First(&channelPriorityTieBreakEnabledOption, "key = ?", "scheduler_setting.channel_priority_tie_break_enabled").Error)
	require.Equal(t, "false", channelPriorityTieBreakEnabledOption.Value)
	var channelPriorityTieBreakDeltaOption model.Option
	require.NoError(t, db.First(&channelPriorityTieBreakDeltaOption, "key = ?", "scheduler_setting.channel_priority_tie_break_score_delta").Error)
	require.Equal(t, "0.049", channelPriorityTieBreakDeltaOption.Value)
	runtimePolicy := modelgatewayintegration.RuntimePolicySetting()
	require.False(t, runtimePolicy.ChannelPriorityTieBreak.Enabled)
	require.Equal(t, 0.049, runtimePolicy.ChannelPriorityTieBreak.ScoreDelta)
	var probeMaxPerTickOption model.Option
	require.NoError(t, db.First(&probeMaxPerTickOption, "key = ?", "scheduler_setting.probe_max_per_tick").Error)
	require.Equal(t, "7", probeMaxPerTickOption.Value)
	var probeMinIntervalOption model.Option
	require.NoError(t, db.First(&probeMinIntervalOption, "key = ?", "scheduler_setting.probe_min_channel_interval_seconds").Error)
	require.Equal(t, "180", probeMinIntervalOption.Value)
	var probeLowScoreOption model.Option
	require.NoError(t, db.First(&probeLowScoreOption, "key = ?", "scheduler_setting.probe_low_score_threshold").Error)
	require.Equal(t, "0.66", probeLowScoreOption.Value)
	var probeRecoveryOption model.Option
	require.NoError(t, db.First(&probeRecoveryOption, "key = ?", "scheduler_setting.probe_recovery_successes_required").Error)
	require.Equal(t, "3", probeRecoveryOption.Value)
	var costWorkerOption model.Option
	require.NoError(t, db.First(&costWorkerOption, "key = ?", "scheduler_setting.cost_calculation_worker_count").Error)
	require.Equal(t, "3", costWorkerOption.Value)
	var costBatchOption model.Option
	require.NoError(t, db.First(&costBatchOption, "key = ?", "scheduler_setting.cost_calculation_batch_size").Error)
	require.Equal(t, "80", costBatchOption.Value)
	var dynamicBillingWindowSamplesOption model.Option
	require.NoError(t, db.First(&dynamicBillingWindowSamplesOption, "key = ?", "scheduler_setting.dynamic_billing_window_samples").Error)
	require.Equal(t, "280", dynamicBillingWindowSamplesOption.Value)
	var dynamicBillingCostSourceOption model.Option
	require.NoError(t, db.First(&dynamicBillingCostSourceOption, "key = ?", "scheduler_setting.dynamic_billing_cost_source").Error)
	require.Equal(t, scheduler_setting.DynamicBillingCostSourceProfit24h, dynamicBillingCostSourceOption.Value)
	var dynamicBillingApplyModeOption model.Option
	require.NoError(t, db.First(&dynamicBillingApplyModeOption, "key = ?", "scheduler_setting.dynamic_billing_apply_mode").Error)
	require.Equal(t, scheduler_setting.DynamicBillingApplyModeAuto, dynamicBillingApplyModeOption.Value)
	var proxyReusePolicyOption model.Option
	require.NoError(t, db.First(&proxyReusePolicyOption, "key = ?", "scheduler_setting.proxy_same_brand_reuse_policy").Error)
	require.Equal(t, scheduler_setting.ProxyReusePolicyConfirm, proxyReusePolicyOption.Value)
	var circuitErrorPolicyOption model.Option
	require.NoError(t, db.First(&circuitErrorPolicyOption, "key = ?", "scheduler_setting.circuit_error_policies").Error)
	require.Contains(t, circuitErrorPolicyOption.Value, `"rate_limit"`)
	require.Contains(t, circuitErrorPolicyOption.Value, `"server_error"`)
	require.NotContains(t, circuitErrorPolicyOption.Value, `"unknown"`)
	var stickyFailurePolicyOption model.Option
	require.NoError(t, db.First(&stickyFailurePolicyOption, "key = ?", "scheduler_setting.sticky_failure_policy").Error)
	require.Equal(t, scheduler_setting.StickyFailurePolicyKeep, stickyFailurePolicyOption.Value)
	var stickyEscapeCostOption model.Option
	require.NoError(t, db.First(&stickyEscapeCostOption, "key = ?", "scheduler_setting.cost_first_sticky_escape_cost_ratio").Error)
	require.Equal(t, "0.72", stickyEscapeCostOption.Value)
	var stickyEscapeSpeedOption model.Option
	require.NoError(t, db.First(&stickyEscapeSpeedOption, "key = ?", "scheduler_setting.cost_first_sticky_escape_max_speed_score_drop").Error)
	require.Equal(t, "0.07", stickyEscapeSpeedOption.Value)
	var costGuardMultipleOption model.Option
	require.NoError(t, db.First(&costGuardMultipleOption, "key = ?", "scheduler_setting.cost_first_guard_multiple").Error)
	require.Equal(t, "1.8", costGuardMultipleOption.Value)
	var costGuardSpeedOption model.Option
	require.NoError(t, db.First(&costGuardSpeedOption, "key = ?", "scheduler_setting.cost_first_guard_speed_advantage").Error)
	require.Equal(t, "0.09", costGuardSpeedOption.Value)
	require.Equal(t, "35", common.OptionMap["scheduler_setting.rollout_percent"])
}

func TestModelGatewayConfigUpdatePersistsDynamicBillingMode(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.DynamicBillingEnabled = true
	setting.DynamicBillingProfitRate = 0.35
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"codex-plus": {
			Mode:             scheduler_setting.ModeActive,
			Strategy:         scheduler_setting.StrategyBalanced,
			AutoMode:         scheduler_setting.AutoModeFusion,
			BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
		},
	}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.True(t, payload.Data.Setting.DynamicBillingEnabled)
	require.Equal(t, 0.35, payload.Data.Setting.DynamicBillingProfitRate)
	require.Equal(t, scheduler_setting.BillingRatioModeDynamic, payload.Data.Setting.GroupPolicies["codex-plus"].BillingRatioMode)
	require.Equal(t, scheduler_setting.BillingRatioModeDynamic, scheduler_setting.GetSetting().GroupPolicies["codex-plus"].BillingRatioMode)
	require.Positive(t, payload.Data.Setting.DynamicBillingEnabledAt)
	firstEnabledAt := payload.Data.Setting.DynamicBillingEnabledAt

	body, err = common.Marshal(payload.Data.Setting)
	require.NoError(t, err)

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload = decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.Equal(t, firstEnabledAt, payload.Data.Setting.DynamicBillingEnabledAt)
}

func TestModelGatewayConfigUpdateResetsDynamicBillingEnabledAtWhenDynamicPolicyChanges(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	current := scheduler_setting.DefaultSetting()
	current.DynamicBillingEnabled = true
	current.DynamicBillingProfitRate = 0.35
	current.DynamicBillingEnabledAt = 123
	current.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"auto": {
			Mode:             scheduler_setting.ModeActive,
			Strategy:         scheduler_setting.StrategyBalanced,
			AutoMode:         scheduler_setting.AutoModeFusion,
			BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
			CandidateGroups:  []string{"auto"},
		},
	}
	restoreSetting := scheduler_setting.SetSettingForTest(current)
	defer restoreSetting()

	next := current
	next.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"auto": {
			Mode:             scheduler_setting.ModeActive,
			Strategy:         scheduler_setting.StrategyBalanced,
			AutoMode:         scheduler_setting.AutoModeFusion,
			BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
			CandidateGroups:  []string{"auto", "codex-plus"},
		},
	}
	body, err := common.Marshal(next)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.Greater(t, payload.Data.Setting.DynamicBillingEnabledAt, int64(123))
	require.Equal(t, []string{"auto", "codex-plus"}, payload.Data.Setting.GroupPolicies["auto"].CandidateGroups)
}

func TestModelGatewayConfigRejectsInvalidPolicy(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"default": {Mode: "bad-mode"},
	}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "invalid mode")
}

func TestModelGatewayConfigNormalizesResourceProtectionPolicy(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.Enabled = true
	setting.DefaultMode = scheduler_setting.ModeActive
	setting.QueueDefaultTimeoutMs = 4200
	setting.QueueMaxDepthPerChannel = 12
	setting.GroupPolicies = map[string]scheduler_setting.GroupPolicySetting{
		"codex-plus": {
			Mode:                      scheduler_setting.ModeActive,
			ResourceProtectionEnabled: true,
			PrimaryChannelIDs:         []int{8, 8, 0, 9},
			PrimaryWaitTimeoutMs:      0,
			PrimaryQueueMaxDepth:      0,
			FallbackChannelIDs:        []int{18, -1, 18, 19},
		},
	}
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	policy := payload.Data.Setting.GroupPolicies["codex-plus"]
	require.True(t, policy.ResourceProtectionEnabled)
	require.Equal(t, []int{8, 9}, policy.PrimaryChannelIDs)
	require.Equal(t, 4200, policy.PrimaryWaitTimeoutMs)
	require.Equal(t, 12, policy.PrimaryQueueMaxDepth)
	require.Equal(t, []int{18, 19}, policy.FallbackChannelIDs)
}

func TestModelGatewayConfigRejectsInvalidProxyReusePolicy(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	router := gin.New()
	router.PUT("/api/model_gateway/config", UpdateModelGatewayConfig)

	setting := scheduler_setting.DefaultSetting()
	setting.ProxySameBrandReusePolicy = "invalid"
	body, err := common.Marshal(setting)
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/config", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "invalid proxy_same_brand_reuse_policy")
}

func TestModelGatewayConfigResetRestoresDefaults(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		Enabled:        true,
		DefaultMode:    scheduler_setting.ModeActive,
		RolloutPercent: 100,
	})
	defer restoreSetting()

	router := gin.New()
	router.POST("/api/model_gateway/config/reset", ResetModelGatewayConfig)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/config/reset", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.False(t, payload.Data.Setting.Enabled)
	require.Equal(t, scheduler_setting.ModeOff, payload.Data.Setting.DefaultMode)
	require.Equal(t, 0, payload.Data.Setting.RolloutPercent)
	require.Equal(t, scheduler_setting.ProxyReusePolicyWarn, payload.Data.Setting.ProxySameBrandReusePolicy)
	require.True(t, payload.Data.Setting.ChannelPriorityTieBreakEnabled)
	require.Equal(t, 0.05, payload.Data.Setting.ChannelPriorityTieBreakScoreDelta)
}

func TestModelGatewayProbeConfigPatchMergesProbeFields(t *testing.T) {
	setupModelGatewayConfigControllerTestDB(t)
	initial := scheduler_setting.DefaultSetting()
	initial.Enabled = true
	initial.DefaultMode = scheduler_setting.ModeActive
	initial.DefaultStrategy = scheduler_setting.StrategySpeedFirst
	restoreSetting := scheduler_setting.SetSettingForTest(initial)
	defer restoreSetting()

	router := gin.New()
	router.PATCH("/api/model_gateway/config/probe", UpdateModelGatewayProbeConfig)

	body := []byte(`{
		"probe_enabled": false,
		"probe_max_per_tick": 9,
		"probe_low_score_threshold": 0.71,
		"probe_recoverable_score_items": ["stream_interrupted_rate", "bad", "first_byte_backlog"],
		"probe_skip_recent_real_request_enabled": true,
		"probe_recent_real_request_window_seconds": 600,
		"probe_good_baseline_enabled": true,
		"probe_good_baseline_min_samples": 6,
		"probe_good_baseline_window_seconds": 2400,
		"probe_prompt_library_enabled": true,
		"probe_prompt_categories": ["long", "bad", "short"]
	}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/model_gateway/config/probe", bytes.NewReader(body))
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayConfigResponse(t, resp)
	require.True(t, payload.Success, resp.Body.String())
	require.False(t, payload.Data.Setting.ProbeEnabled)
	require.Equal(t, 9, payload.Data.Setting.ProbeMaxPerTick)
	require.Equal(t, 0.71, payload.Data.Setting.ProbeLowScoreThreshold)
	require.Equal(t, []string{"first_byte_backlog", "stream_interrupted_rate"}, payload.Data.Setting.ProbeRecoverableScoreItems)
	require.True(t, payload.Data.Setting.ProbeSkipRecentRealRequestEnabled)
	require.Equal(t, 600, payload.Data.Setting.ProbeRecentRealRequestWindowSeconds)
	require.True(t, payload.Data.Setting.ProbeGoodBaselineEnabled)
	require.Equal(t, 6, payload.Data.Setting.ProbeGoodBaselineMinSamples)
	require.Equal(t, 2400, payload.Data.Setting.ProbeGoodBaselineWindowSeconds)
	require.True(t, payload.Data.Setting.ProbePromptLibraryEnabled)
	require.Equal(t, []string{"short", "long"}, payload.Data.Setting.ProbePromptCategories)
	require.True(t, payload.Data.Setting.Enabled)
	require.Equal(t, scheduler_setting.ModeActive, payload.Data.Setting.DefaultMode)
	require.Equal(t, scheduler_setting.StrategySpeedFirst, payload.Data.Setting.DefaultStrategy)
}

func decodeModelGatewayConfigResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelGatewayConfigAPIResponse {
	t.Helper()
	var payload modelGatewayConfigAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}
