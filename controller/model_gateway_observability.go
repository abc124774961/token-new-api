package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	modelgatewayprobe "github.com/QuantumNous/new-api/pkg/modelgateway/probe"
	modelgatewayscheduler "github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	modelGatewayObservabilityDefaultHours       = 24
	modelGatewayObservabilityMaxHours           = 24 * 30
	modelGatewayObservabilityDefaultRecentLimit = 50
	modelGatewayObservabilityMaxRecentLimit     = 200
	modelGatewayObservabilityDefaultTopN        = 10
	modelGatewayObservabilityMaxTopN            = 50
	modelGatewayObservabilityDefaultScanLimit   = 5000
	modelGatewayObservabilityMaxScanLimit       = 20000
	modelGatewayObservabilityMinTrendBucket     = 60
	modelGatewayObservabilityMaxTrendBucket     = 24 * 3600
	modelGatewayObservabilityMaxTrendBuckets    = 360
	modelGatewayObservabilityTrendTopN          = 5
	modelGatewayObservabilityRiskTimelineLimit  = 100
	modelGatewayTrendExportPreviewLimit         = 20
	modelGatewayRuntimeStatusDefaultLimit       = 200
	modelGatewayRuntimeStatusMaxLimit           = 1000
	modelGatewayObservabilitySummaryFreshTTL    = 2 * time.Second
	modelGatewayObservabilitySummaryStaleTTL    = 30 * time.Second
	modelGatewayObservabilitySummaryMaxCache    = 128
	// Keep batch size below SQLite's default bind parameter ceiling.
	modelGatewayDynamicBillingAppliedSummaryBatchSize = 500
	modelGatewayObservabilityViewUserRequests         = "user_requests"
	modelGatewayDynamicBillingDisplayModel            = "gpt-5.4"
	modelGatewayHealthCheckQueueTypeAll               = "all"
	modelGatewayHealthCheckQueueTypeLowScore          = "low_score"
	modelGatewayHealthCheckQueueTypeLowTraffic        = "low_traffic"
	modelGatewayHealthCheckQueueTypeRecovery          = "recovery"
	modelGatewayHealthCheckQueueTypeIsolated          = "isolated"
	modelGatewayHealthCheckQueueTypeCircuitHalfOpen   = "circuit_half_open"
	modelGatewayHealthCheckQueueTypeScoreAnomaly      = modelgatewaycore.ProbeReasonScoreAnomalyFastProbe
	modelGatewayHealthCheckSuccessRateThreshold       = 0.80
	modelGatewayHealthCheckOutputRateThreshold        = 0.02
)

type ModelGatewayObservabilityResponse struct {
	Summary           ModelGatewayObservabilitySummary                `json:"summary"`
	UserRequests      ModelGatewayUserRequestObservabilityResponse    `json:"user_requests"`
	DynamicBilling    ModelGatewayDynamicBillingOverview              `json:"dynamic_billing_overview"`
	DynamicBilling7d  ModelGatewayDynamicBillingOverview              `json:"dynamic_billing_7d_overview"`
	Trends            []ModelGatewayObservabilityTrendPoint           `json:"trends"`
	Risk              ModelGatewayRiskSnapshot                        `json:"risk"`
	RiskTimeline      []ModelGatewayRiskEvent                         `json:"risk_timeline"`
	RiskEvents        []ModelGatewayRiskEvent                         `json:"risk_events"`
	ByModel           []ModelGatewayObservabilityAggregate            `json:"by_model"`
	ByGroup           []ModelGatewayObservabilityAggregate            `json:"by_group"`
	ByChannel         []ModelGatewayObservabilityAggregate            `json:"by_channel"`
	ByProviderProfile []ModelGatewayObservabilityAggregate            `json:"by_provider_profile"`
	ByProxyMode       []ModelGatewayObservabilityAggregate            `json:"by_proxy_mode"`
	RecentRecords     []ModelGatewayObservabilityRecord               `json:"recent_records"`
	ScoreBreakdown    ModelGatewayObservabilityScoreBreakdown         `json:"score_breakdown"`
	RuntimeStatus     modelgatewayobservability.RuntimeStatusResponse `json:"runtime_status"`
	Partial           bool                                            `json:"partial,omitempty"`
}

type ModelGatewayDynamicBillingOverview struct {
	Enabled            bool                                      `json:"enabled"`
	ProfitRate         float64                                   `json:"profit_rate,omitempty"`
	WindowSamples      int                                       `json:"window_samples,omitempty"`
	WindowMinutes      int                                       `json:"window_minutes,omitempty"`
	MinSamples         int                                       `json:"min_samples,omitempty"`
	RefreshSeconds     int                                       `json:"refresh_seconds,omitempty"`
	MaxAgeSeconds      int                                       `json:"max_age_seconds,omitempty"`
	CostSource         string                                    `json:"cost_source,omitempty"`
	ApplyMode          string                                    `json:"apply_mode,omitempty"`
	ProfitWindowHours  int                                       `json:"profit_window_hours,omitempty"`
	MinTokens          int                                       `json:"min_tokens,omitempty"`
	MinRequests        int                                       `json:"min_requests,omitempty"`
	MinSuccessRequests int                                       `json:"min_success_requests,omitempty"`
	MinRatio           float64                                   `json:"min_ratio,omitempty"`
	MaxRatio           float64                                   `json:"max_ratio,omitempty"`
	MaxStepChange      float64                                   `json:"max_step_change,omitempty"`
	PolicyCount        int                                       `json:"policy_count"`
	ActiveCount        int                                       `json:"active_count"`
	WaitingCount       int                                       `json:"waiting_count"`
	Groups             []ModelGatewayDynamicBillingGroupOverview `json:"groups"`
}

type ModelGatewayDynamicBillingGroupOverview struct {
	PolicyGroup          string   `json:"policy_group"`
	DisplayGroup         string   `json:"display_group,omitempty"`
	TargetGroups         []string `json:"target_groups,omitempty"`
	Status               string   `json:"status"`
	CurrentRatio         float64  `json:"current_ratio,omitempty"`
	CurrentPricePerM     float64  `json:"current_price_per_m,omitempty"`
	AverageRatio         float64  `json:"average_ratio,omitempty"`
	AveragePricePerM     float64  `json:"average_price_per_m,omitempty"`
	BlendedRatio         float64  `json:"blended_ratio,omitempty"`
	BlendedPricePerM     float64  `json:"blended_price_per_m,omitempty"`
	CurrentTargetGroup   string   `json:"current_target_group,omitempty"`
	CurrentModel         string   `json:"current_model,omitempty"`
	ReferenceModel       string   `json:"reference_model,omitempty"`
	MinRatio             float64  `json:"min_ratio,omitempty"`
	MaxRatio             float64  `json:"max_ratio,omitempty"`
	MinPricePerM         float64  `json:"min_price_per_m,omitempty"`
	MaxPricePerM         float64  `json:"max_price_per_m,omitempty"`
	SampleCount          int      `json:"sample_count,omitempty"`
	EffectiveSamples     int      `json:"effective_sample_count,omitempty"`
	ModelCount           int      `json:"model_count,omitempty"`
	LatestCalculatedAt   int64    `json:"latest_calculated_at,omitempty"`
	CostSource           string   `json:"cost_source,omitempty"`
	ApplyMode            string   `json:"apply_mode,omitempty"`
	ApplyReason          string   `json:"apply_reason,omitempty"`
	OperatingCostUSD     float64  `json:"operating_cost_usd,omitempty"`
	RequiredRevenueUSD   float64  `json:"required_revenue_usd,omitempty"`
	BaseQuotaAtRatio1    float64  `json:"base_quota_at_ratio_1,omitempty"`
	CostMultiplier       float64  `json:"cost_multiplier,omitempty"`
	TargetRatio          float64  `json:"target_ratio,omitempty"`
	EffectiveRatio       float64  `json:"effective_ratio,omitempty"`
	FixedRatio           float64  `json:"fixed_ratio,omitempty"`
	FixedRatioApplied    bool     `json:"fixed_ratio_applied,omitempty"`
	Clamped              bool     `json:"clamped,omitempty"`
	PendingManualConfirm bool     `json:"pending_manual_confirm,omitempty"`
	FallbackReason       string   `json:"fallback_reason,omitempty"`
	RequestCount         int64    `json:"request_count,omitempty"`
	SuccessRequestCount  int64    `json:"success_request_count,omitempty"`
	TotalTokens          int64    `json:"total_tokens,omitempty"`
	TrafficCostUSD       float64  `json:"traffic_cost_usd,omitempty"`
	TrafficEstimated     bool     `json:"traffic_estimated,omitempty"`
	TrafficDataReady     bool     `json:"traffic_data_ready,omitempty"`
	ServerCostUSD        float64  `json:"server_cost_usd,omitempty"`
	ResourceCostUSD      float64  `json:"resource_cost_usd,omitempty"`
	UpstreamCostUSD      float64  `json:"upstream_cost_usd,omitempty"`
}

type ModelGatewayStickyStoreResponse struct {
	Items []ModelGatewayStickyStoreItem `json:"items"`
	Total int                           `json:"total"`
}

type ModelGatewayStickyStoreItem struct {
	KeyID          string `json:"key_id"`
	ChannelID      int    `json:"channel_id"`
	Group          string `json:"group,omitempty"`
	KeyFingerprint string `json:"key_fingerprint,omitempty"`
	ExpiresAt      int64  `json:"expires_at,omitempty"`
	TTLSeconds     int64  `json:"ttl_seconds,omitempty"`
}

type ModelGatewayStickyClearResponse struct {
	KeyID     string `json:"key_id,omitempty"`
	Group     string `json:"group,omitempty"`
	ChannelID int    `json:"channel_id,omitempty"`
	Cleared   bool   `json:"cleared"`
	Deleted   int    `json:"deleted"`
}

type ModelGatewayRuntimeCircuitClearRequest struct {
	ChannelID             int                    `json:"channel_id"`
	RuntimeKey            ModelGatewayRuntimeKey `json:"runtime_key,omitempty"`
	ClearFailureAvoidance *bool                  `json:"clear_failure_avoidance,omitempty"`
}

type ModelGatewayRuntimeCircuitClearResponse struct {
	ChannelID               int                      `json:"channel_id"`
	RuntimeCircuitsCleared  int                      `json:"runtime_circuits_cleared"`
	RuntimeSnapshotsUpdated int                      `json:"runtime_snapshots_updated"`
	FailureAvoidanceCleared int                      `json:"failure_avoidance_cleared"`
	MatchedRuntimeKeys      []ModelGatewayRuntimeKey `json:"matched_runtime_keys,omitempty"`
	ClearFailureAvoidance   bool                     `json:"clear_failure_avoidance"`
}

type ModelGatewayClientEmptyOutputAvoidanceClearRequest struct {
	SessionKey     string                `json:"session_key"`
	ChannelID      int                   `json:"channel_id"`
	RequestedModel string                `json:"requested_model"`
	Group          string                `json:"group"`
	EndpointType   constant.EndpointType `json:"endpoint_type,omitempty"`
}

type ModelGatewayClientEmptyOutputAvoidanceClearResponse struct {
	Cleared bool                                               `json:"cleared"`
	Scope   modelgatewayscheduler.ClientEmptyOutputSwitchScope `json:"scope"`
}

type ModelGatewayHealthCheckProbeRequest struct {
	ChannelID          int                    `json:"channel_id"`
	RuntimeKey         ModelGatewayRuntimeKey `json:"runtime_key,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	TriggerScoreItems  []string               `json:"trigger_score_items,omitempty"`
	RequestedModel     string                 `json:"requested_model,omitempty"`
	Group              string                 `json:"group,omitempty"`
	SkipRecentRealGate bool                   `json:"skip_recent_real_gate,omitempty"`
}

type ModelGatewayHealthCheckProbeResponse struct {
	ProbeID           string                 `json:"probe_id"`
	ChannelID         int                    `json:"channel_id"`
	ChannelName       string                 `json:"channel_name,omitempty"`
	RuntimeKey        ModelGatewayRuntimeKey `json:"runtime_key,omitempty"`
	Reason            string                 `json:"reason,omitempty"`
	Success           bool                   `json:"success"`
	Skipped           bool                   `json:"skipped,omitempty"`
	SkipReason        string                 `json:"skip_reason,omitempty"`
	StatusCode        int                    `json:"status_code,omitempty"`
	DurationMs        int64                  `json:"duration_ms,omitempty"`
	TTFTMs            int64                  `json:"ttft_ms,omitempty"`
	Quota             int                    `json:"quota,omitempty"`
	Error             string                 `json:"error,omitempty"`
	TriggerScoreItems []string               `json:"trigger_score_items,omitempty"`
	PromptType        string                 `json:"prompt_type,omitempty"`
}

type ModelGatewayHealthCheckQueueResponse struct {
	Summary        ModelGatewayHealthCheckQueueSummary            `json:"summary"`
	Thresholds     ModelGatewayHealthCheckQueueThresholds         `json:"thresholds"`
	RuntimeSummary modelgatewayobservability.RuntimeStatusSummary `json:"runtime_summary"`
	Items          []ModelGatewayHealthCheckQueueItem             `json:"items"`
	ReasonCounts   []ModelGatewayHealthCheckQueueReasonCount      `json:"reason_counts,omitempty"`
	GeneratedAt    int64                                          `json:"generated_at"`
	QueueSnapshot  *modelgatewaycore.RuntimeQueueSnapshot         `json:"queue_snapshot,omitempty"`
}

type ModelGatewayHealthCheckQueueSummary struct {
	UpdatedAt                          int64  `json:"updated_at"`
	PendingCount                       int    `json:"pending_count"`
	ReturnedCount                      int    `json:"returned_count"`
	LowScoreCount                      int    `json:"low_score_count"`
	LowTrafficCount                    int    `json:"low_traffic_count"`
	RecoveryCount                      int    `json:"recovery_count"`
	ScoreAnomalyCount                  int    `json:"score_anomaly_count"`
	IsolatedCount                      int    `json:"isolated_count"`
	RuntimeKeys                        int    `json:"runtime_keys"`
	Channels                           int    `json:"channels"`
	ActiveConcurrency                  int    `json:"active_concurrency"`
	QueuedRequests                     int    `json:"queued_requests"`
	ProbeEnabled                       bool   `json:"probe_enabled"`
	SchedulerRunning                   bool   `json:"scheduler_running"`
	SchedulerMasterNode                bool   `json:"scheduler_master_node"`
	SchedulerRelayInvokerRegistered    bool   `json:"scheduler_relay_invoker_registered"`
	ProbeIntervalSeconds               int64  `json:"probe_interval_seconds,omitempty"`
	LastSchedulerProbeAt               int64  `json:"last_scheduler_probe_at,omitempty"`
	NextSchedulerProbeAt               int64  `json:"next_scheduler_probe_at,omitempty"`
	NextSchedulerProbeRemainingSeconds int64  `json:"next_scheduler_probe_remaining_seconds,omitempty"`
	FilteredQueueType                  string `json:"filtered_queue_type,omitempty"`
}

type ModelGatewayHealthCheckQueueThresholds struct {
	LowScore            float64 `json:"low_score"`
	MissingSamples      int     `json:"missing_samples"`
	SuccessRate         float64 `json:"success_rate"`
	EmptyOutputRate     float64 `json:"empty_output_rate"`
	ExperienceIssueRate float64 `json:"experience_issue_rate"`
}

type ModelGatewayHealthCheckQueueReason struct {
	Key      string `json:"key"`
	Severity string `json:"severity"`
	Priority int    `json:"priority"`
	Label    string `json:"label,omitempty"`
}

type ModelGatewayHealthCheckQueueReasonCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type ModelGatewayHealthCheckQueueItem struct {
	modelgatewayobservability.RuntimeStatusItem
	Reasons                   []ModelGatewayHealthCheckQueueReason `json:"reasons"`
	ProbeTriggerScoreItems    []string                             `json:"probe_trigger_score_items,omitempty"`
	ProbeSkipReason           string                               `json:"probe_skip_reason,omitempty"`
	LastRealAttemptAt         int64                                `json:"last_real_attempt_at,omitempty"`
	NextProbeAt               int64                                `json:"next_probe_at,omitempty"`
	NextProbeRemainingSeconds int64                                `json:"next_probe_remaining_seconds"`
	Priority                  int                                  `json:"priority"`
	QueueType                 string                               `json:"queue_type"`
	RowKey                    string                               `json:"row_key"`
}

type ModelGatewayScoreHistoryResponse struct {
	ChannelID    int                            `json:"channel_id"`
	ChannelName  string                         `json:"channel_name,omitempty"`
	RuntimeKey   ModelGatewayRuntimeKey         `json:"runtime_key,omitempty"`
	Hours        int                            `json:"hours"`
	Limit        int                            `json:"limit"`
	TotalMatched int                            `json:"total_matched"`
	Truncated    bool                           `json:"truncated"`
	Items        []ModelGatewayScoreHistoryItem `json:"items"`
	Current      *ModelGatewayScoreHistoryItem  `json:"current,omitempty"`
	Previous     *ModelGatewayScoreHistoryItem  `json:"previous,omitempty"`
	ScoreDelta   float64                        `json:"score_delta,omitempty"`
	MetricDeltas map[string]float64             `json:"metric_deltas,omitempty"`
	GeneratedAt  int64                          `json:"generated_at"`
}

type ModelGatewayScoreHistoryItem struct {
	ID                                int                                    `json:"id"`
	Source                            string                                 `json:"source,omitempty"`
	CreatedAt                         int64                                  `json:"created_at"`
	RequestID                         string                                 `json:"request_id"`
	RequestedModel                    string                                 `json:"requested_model,omitempty"`
	RequestedGroup                    string                                 `json:"requested_group,omitempty"`
	SelectedGroup                     string                                 `json:"selected_group,omitempty"`
	ChannelID                         int                                    `json:"channel_id"`
	ChannelName                       string                                 `json:"channel_name,omitempty"`
	RuntimeKey                        ModelGatewayRuntimeKey                 `json:"runtime_key,omitempty"`
	PoolLevel                         string                                 `json:"pool_level,omitempty"`
	SwitchReason                      string                                 `json:"switch_reason,omitempty"`
	FailureScope                      string                                 `json:"failure_scope,omitempty"`
	Selected                          bool                                   `json:"selected,omitempty"`
	SelectedReason                    string                                 `json:"selected_reason,omitempty"`
	Available                         bool                                   `json:"available"`
	RejectReason                      string                                 `json:"reject_reason,omitempty"`
	ClientEmptyOutputSessionKey       string                                 `json:"client_empty_output_session_key,omitempty"`
	ClientEmptyOutputAvoidUntil       int64                                  `json:"client_empty_output_avoid_until,omitempty"`
	ClientEmptyOutputRemainingSeconds int64                                  `json:"client_empty_output_remaining_seconds,omitempty"`
	StatusReason                      string                                 `json:"status_reason,omitempty"`
	ScoreTotal                        float64                                `json:"score_total,omitempty"`
	ScoreDelta                        float64                                `json:"score_delta,omitempty"`
	ScoreBreakdown                    map[string]float64                     `json:"score_breakdown,omitempty"`
	ScoreItems                        []modelgatewaycore.ScoreItem           `json:"score_items,omitempty"`
	ScoreItemDeltas                   []modelgatewaycore.ScoreAdjustmentItem `json:"score_item_deltas,omitempty"`
	ScoreBreakdownDelta               map[string]float64                     `json:"score_breakdown_delta,omitempty"`
	RoutingScoreTotal                 float64                                `json:"routing_score_total,omitempty"`
	RoutingScoreBreakdown             map[string]float64                     `json:"routing_score_breakdown,omitempty"`
	RoutingScoreItems                 []modelgatewaycore.ScoreItem           `json:"routing_score_items,omitempty"`
	CostReferenceMissing              bool                                   `json:"cost_reference_missing,omitempty"`
	SampleCount                       int                                    `json:"sample_count,omitempty"`
	TTFTMs                            float64                                `json:"ttft_ms,omitempty"`
	DurationMs                        float64                                `json:"duration_ms,omitempty"`
	ActiveConcurrency                 int                                    `json:"active_concurrency,omitempty"`
	EffectiveConcurrency              int                                    `json:"effective_concurrency_limit,omitempty"`
	IsHealthProbe                     bool                                   `json:"is_health_probe,omitempty"`
	ScoreSampleSource                 string                                 `json:"score_sample_source,omitempty"`
}

type modelGatewayScoreHistoryOptions struct {
	Hours      int
	Limit      int
	ChannelID  int
	RuntimeKey ModelGatewayRuntimeKey
}

type ModelGatewayScoreEventsResponse struct {
	Items     []model.ModelGatewayScoreEvent `json:"items"`
	Limit     int                            `json:"limit"`
	Generated int64                          `json:"generated_at"`
}

type ModelGatewayTrendExport struct {
	Kind      string                                `json:"kind"`
	Filters   ModelGatewayTrendExportFilters        `json:"filters"`
	Summary   ModelGatewayObservabilitySummary      `json:"summary"`
	Trends    []ModelGatewayObservabilityTrendPoint `json:"trends"`
	Preview   ModelGatewayTrendExportPreview        `json:"preview"`
	Generated int64                                 `json:"generated"`
}

type ModelGatewayTrendExportFilters struct {
	Hours              int    `json:"hours"`
	TrendBucketSeconds int64  `json:"trend_bucket_seconds"`
	Model              string `json:"model,omitempty"`
	Group              string `json:"group,omitempty"`
	ChannelID          int    `json:"channel_id,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
}

type ModelGatewayTrendExportPreview struct {
	BucketCount            int                            `json:"bucket_count"`
	NonEmptyBuckets        int                            `json:"non_empty_buckets"`
	RiskEvents             int64                          `json:"risk_events"`
	RiskEventCount         int64                          `json:"risk_event_count"`
	RiskStatusChanges      int64                          `json:"risk_status_changes"`
	CurrentRiskRuntimeKeys int                            `json:"current_risk_runtime_keys"`
	Risk                   ModelGatewayRiskSnapshot       `json:"risk"`
	RiskTimeline           []ModelGatewayRiskEvent        `json:"risk_timeline,omitempty"`
	TopRiskStatuses        []ModelGatewayRiskStatusCount  `json:"top_risk_statuses,omitempty"`
	TopRejectReasons       []ModelGatewayTrendReasonCount `json:"top_reject_reasons,omitempty"`
	TopCircuitOpenReasons  []ModelGatewayTrendReasonCount `json:"top_circuit_open_reasons,omitempty"`
	TopCircuitErrorTypes   []ModelGatewayTrendReasonCount `json:"top_circuit_error_types,omitempty"`
}

type ModelGatewayObservabilitySummary struct {
	WindowHours                               int                            `json:"window_hours"`
	TrendBucketSeconds                        int64                          `json:"trend_bucket_seconds"`
	StartTime                                 int64                          `json:"start_time"`
	EndTime                                   int64                          `json:"end_time"`
	TotalRecords                              int64                          `json:"total_records"`
	ScannedRecords                            int                            `json:"scanned_records"`
	Truncated                                 bool                           `json:"truncated"`
	Dispatches                                int64                          `json:"dispatches"`
	Attempts                                  int64                          `json:"attempts"`
	Successes                                 int64                          `json:"successes"`
	Failures                                  int64                          `json:"failures"`
	StreamInterrupted                         int64                          `json:"stream_interrupted"`
	FallbackUsed                              int64                          `json:"fallback_used"`
	SuccessRate                               float64                        `json:"success_rate"`
	AvgDurationMs                             int64                          `json:"avg_duration_ms"`
	AvgTTFTMs                                 int64                          `json:"avg_ttft_ms"`
	AvgScoreTotal                             float64                        `json:"avg_score_total"`
	QueueEnabledDispatches                    int64                          `json:"queue_enabled_dispatches"`
	QueuedDispatches                          int64                          `json:"queued_dispatches"`
	QueueWaitCount                            int64                          `json:"queue_wait_count"`
	AvgQueueWaitMs                            int64                          `json:"avg_queue_wait_ms"`
	ResourceProtectionDispatches              int64                          `json:"resource_protection_dispatches"`
	ResourceProtectionPrimaryHits             int64                          `json:"resource_protection_primary_hits"`
	ResourceProtectionPrimaryWaits            int64                          `json:"resource_protection_primary_waits"`
	ResourceProtectionFallbacks               int64                          `json:"resource_protection_fallbacks"`
	ResourceProtectionWaitTimeoutFallbacks    int64                          `json:"resource_protection_wait_timeout_fallbacks"`
	ResourceProtectionPrimaryFailureFallbacks int64                          `json:"resource_protection_primary_failure_fallbacks"`
	ResourceProtectionQueueDepth              int                            `json:"resource_protection_queue_depth,omitempty"`
	ResourceProtectionMaxQueueDepth           int                            `json:"resource_protection_max_queue_depth,omitempty"`
	ResourceProtectionQueueCapacity           int                            `json:"resource_protection_queue_capacity,omitempty"`
	ResourceProtectionAvgWaitMs               int64                          `json:"resource_protection_avg_wait_ms"`
	ResourceProtectionFallbackCostShare       float64                        `json:"resource_protection_fallback_cost_share,omitempty"`
	StickyRoutes                              int64                          `json:"sticky_routes"`
	StickyRetained                            int64                          `json:"sticky_retained"`
	StickyBroken                              int64                          `json:"sticky_broken"`
	CacheAffinityRoutes                       int64                          `json:"cache_affinity_routes"`
	OverloadSkipCount                         int64                          `json:"overload_skip_count"`
	AuthConfigErrorCount                      int64                          `json:"auth_config_error_count"`
	UnknownErrorCount                         int64                          `json:"unknown_error_count"`
	ConfigErrorIsolatedCount                  int64                          `json:"config_error_isolated_count"`
	ScoreBreakdownSamples                     int64                          `json:"score_breakdown_samples"`
	ScoreBreakdownParseErrs                   int64                          `json:"score_breakdown_parse_errors"`
	RiskEvents                                int64                          `json:"risk_events"`
	RiskStatusChanges                         int64                          `json:"risk_status_changes"`
	CurrentRiskRuntimeKeys                    int                            `json:"current_risk_runtime_keys"`
	CircuitOpenReasons                        []ModelGatewayTrendReasonCount `json:"circuit_open_reasons,omitempty"`
	CircuitErrorCounts                        []ModelGatewayTrendReasonCount `json:"circuit_error_counts,omitempty"`
	CircuitErrorTypes                         []ModelGatewayTrendReasonCount `json:"circuit_error_types,omitempty"`
}

type ModelGatewayUserRequestObservabilityResponse struct {
	Summary        ModelGatewayUserRequestSummary      `json:"summary"`
	Trends         []ModelGatewayUserRequestTrendPoint `json:"trends"`
	ByModel        []ModelGatewayUserRequestAggregate  `json:"by_model"`
	ByGroup        []ModelGatewayUserRequestAggregate  `json:"by_group"`
	RecentRequests []ModelGatewayUserRequestRecord     `json:"recent_requests"`
}

type ModelGatewayUserRequestSummary struct {
	WindowHours        int     `json:"window_hours"`
	TrendBucketSeconds int64   `json:"trend_bucket_seconds"`
	StartTime          int64   `json:"start_time"`
	EndTime            int64   `json:"end_time"`
	TotalRequests      int64   `json:"total_requests"`
	UserRequests       int64   `json:"user_requests"`
	ScannedRequests    int     `json:"scanned_requests"`
	Truncated          bool    `json:"truncated"`
	Successes          int64   `json:"successes"`
	FinalFailures      int64   `json:"final_failures"`
	UserQuotaExhausted int64   `json:"user_quota_exhausted"`
	ClientAborted      int64   `json:"client_aborted"`
	HealthProbes       int64   `json:"health_probes"`
	Recovered          int64   `json:"recovered"`
	EmptyOutputs       int64   `json:"empty_outputs"`
	ExperienceIssues   int64   `json:"experience_issues"`
	UserSuccessRate    float64 `json:"user_success_rate"`
	AvgDurationMs      int64   `json:"avg_duration_ms"`
	P95DurationMs      int64   `json:"p95_duration_ms"`
	AvgTTFTMs          int64   `json:"avg_ttft_ms"`
	P95TTFTMs          int64   `json:"p95_ttft_ms"`
}

type ModelGatewayUserRequestTrendPoint struct {
	BucketStart        int64   `json:"bucket_start"`
	BucketEnd          int64   `json:"bucket_end"`
	Requests           int64   `json:"requests"`
	UserRequests       int64   `json:"user_requests"`
	Successes          int64   `json:"successes"`
	FinalFailures      int64   `json:"final_failures"`
	UserQuotaExhausted int64   `json:"user_quota_exhausted"`
	ClientAborted      int64   `json:"client_aborted"`
	HealthProbes       int64   `json:"health_probes"`
	Recovered          int64   `json:"recovered"`
	EmptyOutputs       int64   `json:"empty_outputs"`
	ExperienceIssues   int64   `json:"experience_issues"`
	UserSuccessRate    float64 `json:"user_success_rate"`
	AvgDurationMs      int64   `json:"avg_duration_ms"`
	P95DurationMs      int64   `json:"p95_duration_ms"`
	AvgTTFTMs          int64   `json:"avg_ttft_ms"`
	P95TTFTMs          int64   `json:"p95_ttft_ms"`
}

type ModelGatewayUserRequestAggregate struct {
	Key                string  `json:"key"`
	Requests           int64   `json:"requests"`
	UserRequests       int64   `json:"user_requests"`
	Successes          int64   `json:"successes"`
	FinalFailures      int64   `json:"final_failures"`
	UserQuotaExhausted int64   `json:"user_quota_exhausted"`
	ClientAborted      int64   `json:"client_aborted"`
	HealthProbes       int64   `json:"health_probes"`
	Recovered          int64   `json:"recovered"`
	EmptyOutputs       int64   `json:"empty_outputs"`
	ExperienceIssues   int64   `json:"experience_issues"`
	UserSuccessRate    float64 `json:"user_success_rate"`
	AvgDurationMs      int64   `json:"avg_duration_ms"`
	P95DurationMs      int64   `json:"p95_duration_ms"`
	AvgTTFTMs          int64   `json:"avg_ttft_ms"`
	P95TTFTMs          int64   `json:"p95_ttft_ms"`
	LastRequestAt      int64   `json:"last_request_at"`
}

type ModelGatewayUserRequestRecord struct {
	ID                        int                                 `json:"id"`
	CreatedAt                 int64                               `json:"created_at"`
	UpdatedAt                 int64                               `json:"updated_at"`
	CompletedAt               int64                               `json:"completed_at"`
	RequestID                 string                              `json:"request_id"`
	UserID                    int                                 `json:"user_id,omitempty"`
	Username                  string                              `json:"username,omitempty"`
	RequestedModel            string                              `json:"requested_model"`
	RequestedGroup            string                              `json:"requested_group"`
	SelectedGroup             string                              `json:"selected_group,omitempty"`
	ActualGroup               string                              `json:"actual_group,omitempty"`
	ActualGroupRatio          float64                             `json:"actual_group_ratio,omitempty"`
	FinalChannelID            int                                 `json:"final_channel_id,omitempty"`
	FinalChannelName          string                              `json:"final_channel_name,omitempty"`
	ActualChannelCost         float64                             `json:"actual_channel_cost,omitempty"`
	UpstreamCostTotal         float64                             `json:"upstream_cost_total,omitempty"`
	UpstreamCostModel         string                              `json:"upstream_cost_model,omitempty"`
	UpstreamCostBreakdown     map[string]interface{}              `json:"upstream_cost_breakdown,omitempty"`
	UpstreamCostSource        string                              `json:"upstream_cost_source,omitempty"`
	UpstreamCostAccuracy      string                              `json:"upstream_cost_accuracy,omitempty"`
	Attempts                  int                                 `json:"attempts"`
	FinalSuccess              bool                                `json:"final_success"`
	Recovered                 bool                                `json:"recovered"`
	FinalStatusCode           int                                 `json:"final_status_code,omitempty"`
	FinalErrorCategory        string                              `json:"final_error_category,omitempty"`
	WarningLevel              string                              `json:"warning_level,omitempty"`
	WarningFlags              []string                            `json:"warning_flags,omitempty"`
	WarningMessage            string                              `json:"warning_message,omitempty"`
	ChannelInducedClientAbort bool                                `json:"channel_induced_client_abort,omitempty"`
	EmptyOutput               bool                                `json:"empty_output,omitempty"`
	ExperienceIssue           string                              `json:"experience_issue,omitempty"`
	ClientAborted             bool                                `json:"client_aborted,omitempty"`
	IsHealthProbe             bool                                `json:"is_health_probe,omitempty"`
	ProbeReason               string                              `json:"probe_reason,omitempty"`
	DurationMs                int64                               `json:"duration_ms,omitempty"`
	TTFTMs                    int64                               `json:"ttft_ms,omitempty"`
	Status                    string                              `json:"status,omitempty"`
	Billing                   *ModelGatewayUserRequestBillingInfo `json:"billing,omitempty"`
	DispatchRecord            *ModelGatewayObservabilityRecord    `json:"dispatch_record,omitempty"`
	AttemptRecords            []ModelGatewayObservabilityRecord   `json:"attempt_records,omitempty"`
}

type ModelGatewayUserRequestBillingInfo struct {
	LogID                     int     `json:"log_id,omitempty"`
	UserID                    int     `json:"user_id,omitempty"`
	Username                  string  `json:"username,omitempty"`
	CreatedAt                 int64   `json:"created_at,omitempty"`
	Quota                     int     `json:"quota"`
	PromptTokens              int     `json:"prompt_tokens,omitempty"`
	CompletionTokens          int     `json:"completion_tokens,omitempty"`
	TotalTokens               int     `json:"total_tokens,omitempty"`
	ChannelID                 int     `json:"channel_id,omitempty"`
	Group                     string  `json:"group,omitempty"`
	ModelName                 string  `json:"model_name,omitempty"`
	Content                   string  `json:"content,omitempty"`
	ModelRatio                float64 `json:"model_ratio,omitempty"`
	GroupRatio                float64 `json:"group_ratio,omitempty"`
	UserGroupRatio            float64 `json:"user_group_ratio,omitempty"`
	CompletionRatio           float64 `json:"completion_ratio,omitempty"`
	CacheTokens               int     `json:"cache_tokens,omitempty"`
	CacheRatio                float64 `json:"cache_ratio,omitempty"`
	CacheCreationTokens       int     `json:"cache_creation_tokens,omitempty"`
	CacheCreationRatio        float64 `json:"cache_creation_ratio,omitempty"`
	CacheCreationTokens5m     int     `json:"cache_creation_tokens_5m,omitempty"`
	CacheCreationRatio5m      float64 `json:"cache_creation_ratio_5m,omitempty"`
	CacheCreationTokens1h     int     `json:"cache_creation_tokens_1h,omitempty"`
	CacheCreationRatio1h      float64 `json:"cache_creation_ratio_1h,omitempty"`
	CacheWriteTokens          int     `json:"cache_write_tokens,omitempty"`
	ModelPrice                float64 `json:"model_price,omitempty"`
	BillingMode               string  `json:"billing_mode,omitempty"`
	BillingSource             string  `json:"billing_source,omitempty"`
	BillingSubtype            string  `json:"billing_subtype,omitempty"`
	DynamicBillingApplied     bool    `json:"dynamic_billing_applied,omitempty"`
	DynamicBillingFallback    bool    `json:"dynamic_billing_fallback,omitempty"`
	DynamicFallbackReason     string  `json:"dynamic_fallback_reason,omitempty"`
	DynamicApplyReason        string  `json:"dynamic_billing_apply_reason,omitempty"`
	DynamicBillingRatio       float64 `json:"dynamic_billing_ratio,omitempty"`
	DynamicBillingPricePerM   float64 `json:"dynamic_billing_price_per_m,omitempty"`
	DynamicBillingSampleCount int     `json:"dynamic_billing_sample_count,omitempty"`
	SubscriptionConsumed      int     `json:"subscription_consumed,omitempty"`
	WalletQuotaDeducted       int     `json:"wallet_quota_deducted,omitempty"`
	WebSearchCallCount        int     `json:"web_search_call_count,omitempty"`
	WebSearchPrice            float64 `json:"web_search_price,omitempty"`
	FileSearchCallCount       int     `json:"file_search_call_count,omitempty"`
	FileSearchPrice           float64 `json:"file_search_price,omitempty"`
	AudioInputTokenCount      int     `json:"audio_input_token_count,omitempty"`
	AudioInputPrice           float64 `json:"audio_input_price,omitempty"`
	ImageTokens               int     `json:"image_tokens,omitempty"`
	ImageRatio                float64 `json:"image_ratio,omitempty"`
	ImageGenerationCallCount  int     `json:"image_generation_call_count,omitempty"`
	ImageGenerationCallPrice  float64 `json:"image_generation_call_price,omitempty"`
	ImageGenerationQuality    string  `json:"image_generation_call_quality,omitempty"`
	ImageGenerationSize       string  `json:"image_generation_call_size,omitempty"`
	UsageSemantic             string  `json:"usage_semantic,omitempty"`
	IsHealthProbe             bool    `json:"is_health_probe,omitempty"`
	ProbeReason               string  `json:"probe_reason,omitempty"`
}

type ModelGatewayObservabilityTrendPoint struct {
	BucketStart                               int64                                `json:"bucket_start"`
	BucketEnd                                 int64                                `json:"bucket_end"`
	Records                                   int64                                `json:"records"`
	Dispatches                                int64                                `json:"dispatches"`
	Attempts                                  int64                                `json:"attempts"`
	Successes                                 int64                                `json:"successes"`
	Failures                                  int64                                `json:"failures"`
	StreamInterrupted                         int64                                `json:"stream_interrupted"`
	FallbackUsed                              int64                                `json:"fallback_used"`
	SuccessRate                               float64                              `json:"success_rate"`
	AvgDurationMs                             int64                                `json:"avg_duration_ms"`
	AvgTTFTMs                                 int64                                `json:"avg_ttft_ms"`
	QueueEnabledDispatches                    int64                                `json:"queue_enabled_dispatches"`
	QueuedDispatches                          int64                                `json:"queued_dispatches"`
	QueueWaitCount                            int64                                `json:"queue_wait_count"`
	AvgQueueWaitMs                            int64                                `json:"avg_queue_wait_ms"`
	QueueWaitP50Ms                            int64                                `json:"queue_wait_p50_ms"`
	QueueWaitP90Ms                            int64                                `json:"queue_wait_p90_ms"`
	QueueWaitP95Ms                            int64                                `json:"queue_wait_p95_ms"`
	ResourceProtectionDispatches              int64                                `json:"resource_protection_dispatches"`
	ResourceProtectionPrimaryHits             int64                                `json:"resource_protection_primary_hits"`
	ResourceProtectionPrimaryWaits            int64                                `json:"resource_protection_primary_waits"`
	ResourceProtectionFallbacks               int64                                `json:"resource_protection_fallbacks"`
	ResourceProtectionWaitTimeoutFallbacks    int64                                `json:"resource_protection_wait_timeout_fallbacks"`
	ResourceProtectionPrimaryFailureFallbacks int64                                `json:"resource_protection_primary_failure_fallbacks"`
	ResourceProtectionQueueDepth              int                                  `json:"resource_protection_queue_depth,omitempty"`
	ResourceProtectionMaxQueueDepth           int                                  `json:"resource_protection_max_queue_depth,omitempty"`
	ResourceProtectionQueueCapacity           int                                  `json:"resource_protection_queue_capacity,omitempty"`
	ResourceProtectionAvgWaitMs               int64                                `json:"resource_protection_avg_wait_ms"`
	ResourceProtectionFallbackCostShare       float64                              `json:"resource_protection_fallback_cost_share,omitempty"`
	StickyRoutes                              int64                                `json:"sticky_routes"`
	StickyRetained                            int64                                `json:"sticky_retained"`
	StickyBroken                              int64                                `json:"sticky_broken"`
	CacheAffinityRoutes                       int64                                `json:"cache_affinity_routes"`
	OverloadSkipCount                         int64                                `json:"overload_skip_count"`
	AuthConfigErrorCount                      int64                                `json:"auth_config_error_count"`
	UnknownErrorCount                         int64                                `json:"unknown_error_count"`
	ConfigErrorIsolatedCount                  int64                                `json:"config_error_isolated_count"`
	ByProviderProfile                         []ModelGatewayObservabilityAggregate `json:"by_provider_profile,omitempty"`
	ByProxyMode                               []ModelGatewayObservabilityAggregate `json:"by_proxy_mode,omitempty"`
	RejectReasons                             []ModelGatewayTrendReasonCount       `json:"reject_reasons,omitempty"`
	CircuitOpenReasons                        []ModelGatewayTrendReasonCount       `json:"circuit_open_reasons,omitempty"`
	CircuitErrorCounts                        []ModelGatewayTrendReasonCount       `json:"circuit_error_counts,omitempty"`
	CircuitErrorTypes                         []ModelGatewayTrendReasonCount       `json:"circuit_error_types,omitempty"`
	Risk                                      *ModelGatewayRiskSnapshot            `json:"risk,omitempty"`
	RiskEvents                                []ModelGatewayRiskEvent              `json:"risk_events,omitempty"`
}

type ModelGatewayTrendReasonCount struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type ModelGatewayRiskStatusCount struct {
	Status   string `json:"status"`
	Severity string `json:"severity,omitempty"`
	Count    int64  `json:"count"`
}

type ModelGatewayRiskEvent struct {
	Timestamp       int64  `json:"timestamp,omitempty"`
	BucketStart     int64  `json:"bucket_start,omitempty"`
	BucketEnd       int64  `json:"bucket_end,omitempty"`
	Source          string `json:"source"`
	EventType       string `json:"event_type"`
	Severity        string `json:"severity"`
	Status          string `json:"status"`
	PreviousStatus  string `json:"previous_status,omitempty"`
	CurrentStatus   string `json:"current_status,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Count           int64  `json:"count"`
	RequestedModel  string `json:"requested_model,omitempty"`
	UpstreamModel   string `json:"upstream_model,omitempty"`
	Group           string `json:"group,omitempty"`
	ChannelID       int    `json:"channel_id,omitempty"`
	EndpointType    string `json:"endpoint_type,omitempty"`
	ProviderProfile string `json:"provider_profile,omitempty"`
	ProxyMode       string `json:"proxy_mode,omitempty"`
}

type ModelGatewayRiskSnapshot struct {
	EventCount             int64                          `json:"event_count"`
	RiskEventCount         int64                          `json:"risk_event_count"`
	StatusChanges          int64                          `json:"status_changes"`
	RiskStatusChanges      int64                          `json:"risk_status_changes"`
	CurrentRuntimeKeys     int                            `json:"current_runtime_keys"`
	CurrentRiskRuntimeKeys int                            `json:"current_risk_runtime_keys"`
	Events                 []ModelGatewayRiskEvent        `json:"events"`
	RiskEvents             []ModelGatewayRiskEvent        `json:"risk_events"`
	Timeline               []ModelGatewayRiskEvent        `json:"timeline"`
	RiskTimeline           []ModelGatewayRiskEvent        `json:"risk_timeline"`
	TopStatuses            []ModelGatewayRiskStatusCount  `json:"top_statuses,omitempty"`
	TopRiskStatuses        []ModelGatewayRiskStatusCount  `json:"top_risk_statuses,omitempty"`
	TopRejectReasons       []ModelGatewayTrendReasonCount `json:"top_reject_reasons,omitempty"`
	TopCircuitOpenReasons  []ModelGatewayTrendReasonCount `json:"top_circuit_open_reasons,omitempty"`
	TopCircuitErrorTypes   []ModelGatewayTrendReasonCount `json:"top_circuit_error_types,omitempty"`
}

type ModelGatewayObservabilityAggregate struct {
	Key                                       string             `json:"key"`
	Name                                      string             `json:"name,omitempty"`
	ChannelID                                 int                `json:"channel_id,omitempty"`
	ChannelStatus                             int                `json:"channel_status,omitempty"`
	StatusReason                              string             `json:"status_reason,omitempty"`
	BalanceInsufficient                       bool               `json:"balance_insufficient,omitempty"`
	Records                                   int64              `json:"records"`
	Dispatches                                int64              `json:"dispatches"`
	Attempts                                  int64              `json:"attempts"`
	Successes                                 int64              `json:"successes"`
	Failures                                  int64              `json:"failures"`
	StreamInterrupted                         int64              `json:"stream_interrupted"`
	FallbackUsed                              int64              `json:"fallback_used"`
	SuccessRate                               float64            `json:"success_rate"`
	AvgDurationMs                             int64              `json:"avg_duration_ms"`
	AvgTTFTMs                                 int64              `json:"avg_ttft_ms"`
	AvgScoreTotal                             float64            `json:"avg_score_total"`
	QueueEnabledDispatches                    int64              `json:"queue_enabled_dispatches"`
	QueuedDispatches                          int64              `json:"queued_dispatches"`
	QueueWaitCount                            int64              `json:"queue_wait_count"`
	AvgQueueWaitMs                            int64              `json:"avg_queue_wait_ms"`
	ResourceProtectionDispatches              int64              `json:"resource_protection_dispatches"`
	ResourceProtectionPrimaryHits             int64              `json:"resource_protection_primary_hits"`
	ResourceProtectionPrimaryWaits            int64              `json:"resource_protection_primary_waits"`
	ResourceProtectionFallbacks               int64              `json:"resource_protection_fallbacks"`
	ResourceProtectionWaitTimeoutFallbacks    int64              `json:"resource_protection_wait_timeout_fallbacks"`
	ResourceProtectionPrimaryFailureFallbacks int64              `json:"resource_protection_primary_failure_fallbacks"`
	ResourceProtectionQueueDepth              int                `json:"resource_protection_queue_depth,omitempty"`
	ResourceProtectionMaxQueueDepth           int                `json:"resource_protection_max_queue_depth,omitempty"`
	ResourceProtectionQueueCapacity           int                `json:"resource_protection_queue_capacity,omitempty"`
	ResourceProtectionAvgWaitMs               int64              `json:"resource_protection_avg_wait_ms"`
	ResourceProtectionFallbackCostShare       float64            `json:"resource_protection_fallback_cost_share,omitempty"`
	StickyRoutes                              int64              `json:"sticky_routes"`
	StickyRetained                            int64              `json:"sticky_retained"`
	StickyBroken                              int64              `json:"sticky_broken"`
	CacheAffinityRoutes                       int64              `json:"cache_affinity_routes"`
	OverloadSkipCount                         int64              `json:"overload_skip_count"`
	AuthConfigErrorCount                      int64              `json:"auth_config_error_count"`
	UnknownErrorCount                         int64              `json:"unknown_error_count"`
	ConfigErrorIsolatedCount                  int64              `json:"config_error_isolated_count"`
	ScoreBreakdown                            map[string]float64 `json:"score_breakdown,omitempty"`
	ScoreBreakdownSamples                     int64              `json:"score_breakdown_samples"`
	ScoreBreakdownParseErrs                   int64              `json:"score_breakdown_parse_errors"`
	LastRecordAt                              int64              `json:"last_record_at"`
}

type ModelGatewayObservabilityScoreBreakdown struct {
	Samples     int64              `json:"samples"`
	Average     map[string]float64 `json:"average"`
	ParseErrors int64              `json:"parse_errors"`
}

type ModelGatewayObservabilityRecord struct {
	ID                             int                                `json:"id"`
	Kind                           string                             `json:"kind"`
	CreatedAt                      int64                              `json:"created_at"`
	RequestID                      string                             `json:"request_id"`
	AttemptIndex                   int                                `json:"attempt_index"`
	RequestedGroup                 string                             `json:"requested_group"`
	SelectedGroup                  string                             `json:"selected_group"`
	ActualGroup                    string                             `json:"actual_group,omitempty"`
	RequestedModel                 string                             `json:"requested_model"`
	ChannelID                      int                                `json:"channel_id"`
	ChannelName                    string                             `json:"channel_name,omitempty"`
	ActualChannelID                int                                `json:"actual_channel_id,omitempty"`
	ActualChannelName              string                             `json:"actual_channel_name,omitempty"`
	EndpointType                   string                             `json:"endpoint_type,omitempty"`
	PolicyMode                     string                             `json:"policy_mode,omitempty"`
	AutoMode                       string                             `json:"auto_mode,omitempty"`
	Strategy                       string                             `json:"strategy,omitempty"`
	Shadow                         bool                               `json:"shadow"`
	SmartHandled                   bool                               `json:"smart_handled"`
	FallbackUsed                   bool                               `json:"fallback_used"`
	Success                        bool                               `json:"success"`
	StatusCode                     int                                `json:"status_code,omitempty"`
	ErrorCode                      string                             `json:"error_code,omitempty"`
	ErrorType                      string                             `json:"error_type,omitempty"`
	DurationMs                     int64                              `json:"duration_ms,omitempty"`
	TTFTMs                         int64                              `json:"ttft_ms,omitempty"`
	StreamInterrupted              bool                               `json:"stream_interrupted"`
	ScoreTotal                     float64                            `json:"score_total,omitempty"`
	ScoreBreakdown                 map[string]float64                 `json:"score_breakdown,omitempty"`
	CandidateGroups                []string                           `json:"candidate_groups,omitempty"`
	CandidateExplanations          []ModelGatewayCandidateExplanation `json:"candidate_explanations,omitempty"`
	SelectedReason                 string                             `json:"selected_reason,omitempty"`
	QueueEnabled                   bool                               `json:"queue_enabled,omitempty"`
	QueueWaitMs                    int64                              `json:"queue_wait_ms,omitempty"`
	QueueDepth                     int                                `json:"queue_depth,omitempty"`
	QueueCapacity                  int                                `json:"queue_capacity,omitempty"`
	ResourceProtectionEnabled      bool                               `json:"resource_protection_enabled,omitempty"`
	ResourceProtectionPhase        string                             `json:"resource_protection_phase,omitempty"`
	ResourceProtectionReason       string                             `json:"resource_protection_reason,omitempty"`
	ResourceProtectionRole         string                             `json:"resource_protection_role,omitempty"`
	PrimaryChannelIDs              []int                              `json:"primary_channel_ids,omitempty"`
	FallbackChannelIDs             []int                              `json:"fallback_channel_ids,omitempty"`
	PrimaryWaitTimeoutMs           int                                `json:"primary_wait_timeout_ms,omitempty"`
	PrimaryQueueMaxDepth           int                                `json:"primary_queue_max_depth,omitempty"`
	StickySource                   string                             `json:"sticky_source,omitempty"`
	StickyRetained                 bool                               `json:"sticky_retained,omitempty"`
	StickyBreak                    string                             `json:"sticky_break,omitempty"`
	StickyDecision                 *modelgatewaycore.StickyDecision   `json:"sticky_decision,omitempty"`
	CacheAffinity                  bool                               `json:"cache_affinity,omitempty"`
	ErrorMessage                   string                             `json:"error_message,omitempty"`
	ErrorCategory                  string                             `json:"error_category,omitempty"`
	WarningLevel                   string                             `json:"warning_level,omitempty"`
	WarningFlags                   []string                           `json:"warning_flags,omitempty"`
	WarningMessage                 string                             `json:"warning_message,omitempty"`
	ChannelInducedClientAbort      bool                               `json:"channel_induced_client_abort,omitempty"`
	RetryAction                    string                             `json:"retry_action,omitempty"`
	RetryReason                    string                             `json:"retry_reason,omitempty"`
	WillRetry                      bool                               `json:"will_retry,omitempty"`
	ClientAborted                  bool                               `json:"client_aborted,omitempty"`
	ConcurrencyLimited             bool                               `json:"concurrency_limited,omitempty"`
	BalanceInsufficient            bool                               `json:"balance_insufficient,omitempty"`
	ActiveConcurrency              int                                `json:"active_concurrency,omitempty"`
	ConfiguredConcurrencyLimit     int                                `json:"configured_concurrency_limit,omitempty"`
	LearnedConcurrencyLimit        int                                `json:"learned_concurrency_limit,omitempty"`
	LearnedConcurrencyLimitChanged bool                               `json:"learned_concurrency_limit_changed,omitempty"`
	EmptyOutput                    bool                               `json:"empty_output,omitempty"`
	ExperienceIssue                string                             `json:"experience_issue,omitempty"`
	UsedChannels                   []string                           `json:"used_channels,omitempty"`
	IsHealthProbe                  bool                               `json:"is_health_probe,omitempty"`
	ProbeReason                    string                             `json:"probe_reason,omitempty"`
	RequestMeta                    map[string]any                     `json:"request_meta,omitempty"`
	ScoreBreakdownError            bool                               `json:"score_breakdown_error,omitempty"`
	CandidateGroupsError           bool                               `json:"candidate_groups_error,omitempty"`
	RequestMetaError               bool                               `json:"request_meta_error,omitempty"`
}

type ModelGatewayCandidateExplanation struct {
	ChannelID                         int                          `json:"channel_id"`
	ChannelName                       string                       `json:"channel_name,omitempty"`
	ResourceID                        string                       `json:"resource_id,omitempty"`
	ResourceType                      string                       `json:"resource_type,omitempty"`
	AccountID                         string                       `json:"account_id,omitempty"`
	AccountType                       string                       `json:"account_type,omitempty"`
	Brand                             string                       `json:"brand,omitempty"`
	Provider                          string                       `json:"provider,omitempty"`
	CredentialIndex                   int                          `json:"credential_index,omitempty"`
	CredentialSubjectFP               string                       `json:"credential_subject_fingerprint,omitempty"`
	CredentialFP                      string                       `json:"credential_fingerprint,omitempty"`
	CredentialUID                     string                       `json:"credential_uid,omitempty"`
	CredentialLabel                   string                       `json:"credential_label,omitempty"`
	PoolLevel                         string                       `json:"pool_level,omitempty"`
	SwitchReason                      string                       `json:"switch_reason,omitempty"`
	FailureScope                      string                       `json:"failure_scope,omitempty"`
	Group                             string                       `json:"group,omitempty"`
	UpstreamModel                     string                       `json:"upstream_model,omitempty"`
	ProviderProfile                   string                       `json:"provider_profile,omitempty"`
	ProxyMode                         string                       `json:"proxy_mode,omitempty"`
	RuntimeKey                        ModelGatewayRuntimeKey       `json:"runtime_key"`
	Available                         bool                         `json:"available"`
	RejectReason                      string                       `json:"reject_reason,omitempty"`
	ClientEmptyOutputSessionKey       string                       `json:"client_empty_output_session_key,omitempty"`
	ClientEmptyOutputAvoidUntil       int64                        `json:"client_empty_output_avoid_until,omitempty"`
	ClientEmptyOutputRemainingSeconds int64                        `json:"client_empty_output_remaining_seconds,omitempty"`
	SelectionSkipReason               string                       `json:"selection_skip_reason,omitempty"`
	ChannelStatus                     int                          `json:"channel_status,omitempty"`
	ChannelPriority                   int64                        `json:"channel_priority,omitempty"`
	StatusReason                      string                       `json:"status_reason,omitempty"`
	BalanceInsufficient               bool                         `json:"balance_insufficient,omitempty"`
	ScoreTotal                        float64                      `json:"score_total,omitempty"`
	ScoreBreakdown                    map[string]float64           `json:"score_breakdown,omitempty"`
	ScoreItems                        []modelgatewaycore.ScoreItem `json:"score_items,omitempty"`
	RoutingScoreTotal                 float64                      `json:"routing_score_total,omitempty"`
	RoutingScoreBreakdown             map[string]float64           `json:"routing_score_breakdown,omitempty"`
	RoutingScoreItems                 []modelgatewaycore.ScoreItem `json:"routing_score_items,omitempty"`
	StateTags                         []string                     `json:"state_tags,omitempty"`
	CostReferenceMissing              bool                         `json:"cost_reference_missing,omitempty"`
	SuccessRate                       float64                      `json:"success_rate,omitempty"`
	TTFTMs                            float64                      `json:"ttft_ms,omitempty"`
	DurationMs                        float64                      `json:"duration_ms,omitempty"`
	TokensPerSecond                   float64                      `json:"tokens_per_second,omitempty"`
	SampleCount                       int                          `json:"sample_count,omitempty"`
	ActiveConcurrency                 int                          `json:"active_concurrency,omitempty"`
	MaxConcurrency                    int                          `json:"max_concurrency,omitempty"`
	ConfiguredConcurrencyLimit        int                          `json:"configured_concurrency_limit,omitempty"`
	LearnedConcurrencyLimit           int                          `json:"learned_concurrency_limit,omitempty"`
	EffectiveConcurrencyLimit         int                          `json:"effective_concurrency_limit,omitempty"`
	QueueDepth                        int                          `json:"queue_depth,omitempty"`
	QueueCapacity                     int                          `json:"queue_capacity,omitempty"`
	EstimatedQueueWaitMs              float64                      `json:"estimated_queue_wait_ms,omitempty"`
	ResourceProtectionRole            string                       `json:"resource_protection_role,omitempty"`
	ResourceProtectionReason          string                       `json:"resource_protection_reason,omitempty"`
	CostRatio                         float64                      `json:"cost_ratio,omitempty"`
	CostReferenceRatio                float64                      `json:"cost_reference_ratio,omitempty"`
	CostPricingMode                   string                       `json:"cost_pricing_mode,omitempty"`
	GroupPriorityRatio                float64                      `json:"group_priority_ratio,omitempty"`
	CircuitState                      string                       `json:"circuit_state,omitempty"`
	CircuitOpen                       bool                         `json:"circuit_open,omitempty"`
	CircuitOpenUntil                  int64                        `json:"circuit_open_until,omitempty"`
	CircuitOpenReason                 string                       `json:"circuit_open_reason,omitempty"`
	CircuitFailureCount               int                          `json:"circuit_failure_count,omitempty"`
	CircuitFailureRate                float64                      `json:"circuit_failure_rate,omitempty"`
	CircuitSampleCount                int                          `json:"circuit_sample_count,omitempty"`
	CircuitErrorCounts                map[string]int               `json:"circuit_error_counts,omitempty"`
	CircuitHalfOpenProbeUsed          int                          `json:"circuit_half_open_probe_used,omitempty"`
	CircuitHalfOpenProbeMax           int                          `json:"circuit_half_open_probe_max,omitempty"`
	EmptyOutputRate                   float64                      `json:"empty_output_rate,omitempty"`
	ExperienceIssueRate               float64                      `json:"experience_issue_rate,omitempty"`
	ProbeRecoveryPending              bool                         `json:"probe_recovery_pending,omitempty"`
	ProbeRecoverySuccessCount         int                          `json:"probe_recovery_success_count,omitempty"`
	ProbeRecoveryRequired             int                          `json:"probe_recovery_required,omitempty"`
	ProbeTriggerReason                string                       `json:"probe_trigger_reason,omitempty"`
	ConfigErrorIsolated               bool                         `json:"config_error_isolated,omitempty"`
	IsolationReason                   string                       `json:"isolation_reason,omitempty"`
	IsolationUntil                    int64                        `json:"isolation_until,omitempty"`
	AuthConfigErrorCount              int                          `json:"auth_config_error_count,omitempty"`
	LastAuthConfigErrorAt             int64                        `json:"last_auth_config_error_at,omitempty"`
	StickyMatched                     bool                         `json:"sticky_matched,omitempty"`
	Selected                          bool                         `json:"selected,omitempty"`
	ScoreSampleSource                 string                       `json:"score_sample_source,omitempty"`
	MatchedRuntimeKey                 ModelGatewayRuntimeKey       `json:"matched_runtime_key,omitempty"`
	RequestBodyBytes                  int64                        `json:"request_body_bytes,omitempty"`
	RequestBodyStorage                string                       `json:"request_body_storage,omitempty"`
	RequestBodySizeLikelyLatency      bool                         `json:"request_body_size_likely_latency,omitempty"`
	RequestBodyPrepareMs              int64                        `json:"request_body_prepare_ms,omitempty"`
	UpstreamResponseHeaderMs          int64                        `json:"upstream_response_header_ms,omitempty"`
	UpstreamFirstEventWaitMs          int64                        `json:"upstream_first_event_wait_ms,omitempty"`
}

type ModelGatewayRuntimeKey struct {
	RequestedModel        string `json:"requested_model,omitempty"`
	UpstreamModel         string `json:"upstream_model,omitempty"`
	ChannelID             int    `json:"channel_id,omitempty"`
	ResourceID            string `json:"resource_id,omitempty"`
	ResourceType          string `json:"resource_type,omitempty"`
	AccountID             string `json:"account_id,omitempty"`
	AccountType           string `json:"account_type,omitempty"`
	Brand                 string `json:"brand,omitempty"`
	Provider              string `json:"provider,omitempty"`
	CredentialIndex       int    `json:"credential_index,omitempty"`
	CredentialIndexSet    bool   `json:"-"`
	CredentialSubjectFP   string `json:"credential_subject_fingerprint,omitempty"`
	CredentialFP          string `json:"credential_fingerprint,omitempty"`
	Group                 string `json:"group,omitempty"`
	EndpointType          string `json:"endpoint_type,omitempty"`
	CapabilityFingerprint string `json:"capability_fingerprint,omitempty"`
}

type ModelGatewayObservabilityOptions struct {
	Hours              int
	RecentLimit        int
	TopN               int
	ScanLimit          int
	TrendBucketSeconds int64
	ViewMode           string
	Model              string
	Group              string
	ChannelID          int
	RequestID          string
	IncludeTotal       bool
	HealthProbeOnly    bool
	Lite               bool
	IncludeDispatch    bool
	RecentOnly         bool
}

type modelGatewayObservabilityAccumulator struct {
	ModelGatewayObservabilityAggregate
	durationSum                    int64
	durationSamples                int64
	ttftSum                        int64
	ttftSamples                    int64
	scoreTotalSum                  float64
	scoreTotalSamples              int64
	queueWaitSum                   int64
	queueWaitSamples               int64
	queueWaitValues                []int64
	resourceProtectionWaitSum      int64
	resourceProtectionWaitSamples  int64
	resourceProtectionCostSum      float64
	resourceProtectionFallbackCost float64
	scoreSums                      map[string]float64
	configIsolationKeys            map[string]struct{}
}

type modelGatewayObservabilityTrendAccumulator struct {
	*modelGatewayObservabilityAccumulator
	ProviderProfileAccumulators map[string]*modelGatewayObservabilityAccumulator
	ProxyModeAccumulators       map[string]*modelGatewayObservabilityAccumulator
	RejectReasons               map[string]int64
	CircuitOpenReasons          map[string]int64
	CircuitErrorTypes           map[string]int64
}

type modelGatewayObservabilitySummaryCacheEntry struct {
	options    ModelGatewayObservabilityOptions
	response   ModelGatewayObservabilityResponse
	createdAt  time.Time
	expiresAt  time.Time
	staleUntil time.Time
}

type modelGatewayObservabilitySummaryCacheStore struct {
	mu      sync.Mutex
	group   singleflight.Group
	entries map[string]modelGatewayObservabilitySummaryCacheEntry
}

var modelGatewayObservabilitySummaryCache = &modelGatewayObservabilitySummaryCacheStore{
	entries: make(map[string]modelGatewayObservabilitySummaryCacheEntry),
}

func resetModelGatewayObservabilitySummaryCache() {
	modelGatewayObservabilitySummaryCache = &modelGatewayObservabilitySummaryCacheStore{
		entries: make(map[string]modelGatewayObservabilitySummaryCacheEntry),
	}
}

func GetModelGatewayObservabilitySummary(c *gin.Context) {
	options, err := parseModelGatewayObservabilityOptions(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	response, err := BuildModelGatewayObservabilitySummary(options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func ExportModelGatewayObservabilityTrends(c *gin.Context) {
	options, err := parseModelGatewayObservabilityOptions(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	options.IncludeTotal = true
	response, err := BuildModelGatewayObservabilitySummary(options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	export := ModelGatewayTrendExport{
		Kind: "modelgateway_observability_trends",
		Filters: ModelGatewayTrendExportFilters{
			Hours:              response.Summary.WindowHours,
			TrendBucketSeconds: response.Summary.TrendBucketSeconds,
			Model:              options.Model,
			Group:              options.Group,
			ChannelID:          options.ChannelID,
			RequestID:          options.RequestID,
		},
		Summary:   response.Summary,
		Trends:    response.Trends,
		Preview:   buildModelGatewayTrendExportPreview(response),
		Generated: common.GetTimestamp(),
	}

	if c.Query("download") == "true" {
		data, err := common.Marshal(export)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filename := fmt.Sprintf("modelgateway-trends-%dh-%ds.json", export.Filters.Hours, export.Filters.TrendBucketSeconds)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/json; charset=utf-8", append(data, '\n'))
		return
	}

	common.ApiSuccess(c, export)
}

func GetModelGatewayRuntimeStatus(c *gin.Context) {
	query, err := parseModelGatewayRuntimeStatusQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, defaultModelGatewayRuntimeStatusService().Build(query))
}

func GetModelGatewayHealthCheckQueue(c *gin.Context) {
	query, err := parseModelGatewayRuntimeStatusQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if strings.TrimSpace(c.Query("limit")) == "" {
		query.Limit = modelGatewayRuntimeStatusMaxLimit
	}
	queueType, err := parseModelGatewayHealthCheckQueueType(c.Query("queue_type"))
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	response := BuildModelGatewayHealthCheckQueue(query, queueType)
	if !parseModelGatewayObservabilityBool(c.Query("include_queue_snapshot")) {
		response.QueueSnapshot = nil
	}
	common.ApiSuccess(c, response)
}

func RunModelGatewayHealthCheckProbe(c *gin.Context) {
	var request ModelGatewayHealthCheckProbeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if request.ChannelID <= 0 {
		common.ApiErrorMsg(c, "missing channel_id")
		return
	}
	channel, err := model.GetChannelById(request.ChannelID, true)
	if err != nil || channel == nil {
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiErrorMsg(c, "channel not found")
		return
	}
	runtimeKey := request.RuntimeKey
	if runtimeKey.ChannelID == 0 {
		runtimeKey.ChannelID = request.ChannelID
	}
	if runtimeKey.CredentialIndex > 0 {
		runtimeKey.CredentialIndexSet = true
	}
	if runtimeKey.ChannelID != request.ChannelID {
		common.ApiErrorMsg(c, "runtime_key channel_id must match channel_id")
		return
	}
	coreKey := modelGatewayRuntimeKeyToCore(runtimeKey, request.ChannelID)
	coreKey = modelGatewayHealthCheckProbeCompleteRuntimeKey(coreKey, request, channel)
	reason := modelGatewayHealthCheckProbeReason(request.Reason, coreKey)
	triggerItems := append([]string(nil), request.TriggerScoreItems...)
	result, err := modelgatewayprobe.RunImmediateProbe(c.Request.Context(), modelgatewayprobe.ImmediateProbeOptions{
		Channel:           channel,
		RuntimeKey:        coreKey,
		Model:             firstNonEmptyTrimmed(request.RequestedModel, coreKey.RequestedModel, coreKey.UpstreamModel),
		Group:             firstNonEmptyTrimmed(request.Group, coreKey.Group),
		Reason:            reason,
		TriggerScoreItems: triggerItems,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	errorMessage := ""
	if result.NewAPIError != nil {
		errorMessage = result.NewAPIError.ErrorWithStatusCode()
	} else if result.Err != nil {
		errorMessage = result.Err.Error()
	}
	responseChannelID := request.ChannelID
	responseChannelName := channel.Name
	if result.Channel != nil {
		responseChannelID = result.Channel.Id
		responseChannelName = result.Channel.Name
	}
	common.ApiSuccess(c, ModelGatewayHealthCheckProbeResponse{
		ProbeID:           result.ProbeID,
		ChannelID:         responseChannelID,
		ChannelName:       responseChannelName,
		RuntimeKey:        modelGatewayRuntimeKeyFromCore(result.AttemptRuntimeKey()),
		Reason:            result.Reason,
		Success:           result.Success,
		Skipped:           result.Skipped,
		SkipReason:        result.SkipReason,
		StatusCode:        result.StatusCode,
		DurationMs:        result.Duration.Milliseconds(),
		TTFTMs:            result.TTFT.Milliseconds(),
		Quota:             result.Quota,
		Error:             common.MaskSensitiveInfo(errorMessage),
		TriggerScoreItems: triggerItems,
		PromptType:        result.PromptType,
	})
}

func GetModelGatewayScoreHistory(c *gin.Context) {
	options, err := parseModelGatewayScoreHistoryOptions(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	response, err := BuildModelGatewayScoreHistory(options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func GetModelGatewayScoreEvents(c *gin.Context) {
	if model.DB == nil {
		common.ApiError(c, errors.New("database is not initialized"))
		return
	}
	limit := normalizeModelGatewayObservabilityInt(c.Query("limit"), 50, 1, 200)
	query := model.DB.Model(&model.ModelGatewayScoreEvent{})
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		channelID, err := strconv.Atoi(raw)
		if err != nil || channelID <= 0 {
			common.ApiErrorMsg(c, "invalid channel_id")
			return
		}
		query = query.Where("channel_id = ?", channelID)
	}
	if value := strings.TrimSpace(c.Query("requested_model")); value != "" {
		query = query.Where("requested_model = ?", value)
	}
	if value := strings.TrimSpace(c.Query("group")); value != "" {
		query = query.Where(&model.ModelGatewayScoreEvent{Group: value})
	}
	if value := strings.TrimSpace(c.Query("request_id")); value != "" {
		query = query.Where("request_id = ?", value)
	}
	if value := strings.TrimSpace(c.Query("is_health_probe")); value != "" {
		query = query.Where("is_health_probe = ?", value == "true" || value == "1")
	}
	if from := normalizeModelGatewayObservabilityInt(c.Query("from"), 0, 0, math.MaxInt32); from > 0 {
		query = query.Where("created_at >= ?", from)
	}
	if to := normalizeModelGatewayObservabilityInt(c.Query("to"), 0, 0, math.MaxInt32); to > 0 {
		query = query.Where("created_at <= ?", to)
	}
	var rows []model.ModelGatewayScoreEvent
	if err := query.Order("created_at desc, id desc").Limit(limit).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ModelGatewayScoreEventsResponse{
		Items:     rows,
		Limit:     limit,
		Generated: common.GetTimestamp(),
	})
}

func GetModelGatewayStickyStore(c *gin.Context) {
	limit := normalizeModelGatewayObservabilityInt(c.Query("limit"), 100, 1, 500)
	items := modelGatewayStickyStoreItems(limit)
	common.ApiSuccess(c, ModelGatewayStickyStoreResponse{
		Items: items,
		Total: len(items),
	})
}

func ClearModelGatewayStickyStoreEntry(c *gin.Context) {
	keyID := strings.TrimSpace(c.Param("key_id"))
	if keyID == "" {
		common.ApiErrorMsg(c, "missing sticky key id")
		return
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	deleted := 0
	if runtimeDeps != nil && runtimeDeps.StickyRouter != nil && runtimeDeps.StickyRouter.ClearStickyEntryByID(keyID) {
		deleted = 1
	}
	common.ApiSuccess(c, ModelGatewayStickyClearResponse{
		KeyID:   keyID,
		Cleared: deleted > 0,
		Deleted: deleted,
	})
}

func ClearModelGatewayStickyStore(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			common.ApiErrorMsg(c, "invalid channel_id")
			return
		}
		channelID = parsed
	}
	if group == "" && channelID <= 0 {
		common.ApiErrorMsg(c, "missing sticky clear filter")
		return
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	deleted := 0
	if runtimeDeps != nil && runtimeDeps.StickyRouter != nil {
		deleted = runtimeDeps.StickyRouter.ClearStickyEntries(modelgatewayscheduler.StickyClearFilter{
			Group:     group,
			ChannelID: channelID,
		})
	}
	common.ApiSuccess(c, ModelGatewayStickyClearResponse{
		Group:     group,
		ChannelID: channelID,
		Cleared:   deleted > 0,
		Deleted:   deleted,
	})
}

func ClearModelGatewayRuntimeCircuit(c *gin.Context) {
	var request ModelGatewayRuntimeCircuitClearRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if request.ChannelID <= 0 {
		common.ApiErrorMsg(c, "missing channel_id")
		return
	}
	if _, err := model.GetChannelById(request.ChannelID, false); err != nil {
		common.ApiError(c, err)
		return
	}
	clearFailureAvoidance := true
	if request.ClearFailureAvoidance != nil {
		clearFailureAvoidance = *request.ClearFailureAvoidance
	}
	filter := request.RuntimeKey
	if filter.ChannelID == 0 {
		filter.ChannelID = request.ChannelID
	}
	if filter.CredentialIndex > 0 {
		filter.CredentialIndexSet = true
	}
	if filter.ChannelID != request.ChannelID {
		common.ApiErrorMsg(c, "runtime_key channel_id must match channel_id")
		return
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	matchedKeys := modelGatewayRuntimeCircuitClearKeys(runtimeDeps, request.ChannelID, filter)
	circuitsCleared := 0
	if runtimeDeps != nil && runtimeDeps.CircuitBreaker != nil {
		if modelGatewayRuntimeKeyIsEmpty(filter) || modelGatewayRuntimeKeyOnlyChannel(filter, request.ChannelID) {
			circuitsCleared = runtimeDeps.CircuitBreaker.ResetChannel(request.ChannelID)
		} else {
			for _, key := range matchedKeys {
				if runtimeDeps.CircuitBreaker.Reset(key) {
					circuitsCleared++
				}
			}
		}
	}
	snapshotsUpdated := modelGatewayClearRuntimeCircuitSnapshots(runtimeDeps, matchedKeys, clearFailureAvoidance)
	failureAvoidanceCleared := 0
	if clearFailureAvoidance {
		if modelGatewayRuntimeKeyIsEmpty(filter) || modelGatewayRuntimeKeyOnlyChannel(filter, request.ChannelID) {
			service.ClearChannelFailureAvoidance(request.ChannelID)
			failureAvoidanceCleared = 1
		} else {
			for _, key := range matchedKeys {
				service.ClearChannelRuntimeFailureAvoidance(modelGatewayRuntimeIdentityFromCoreKey(key))
				failureAvoidanceCleared++
			}
		}
	}
	common.ApiSuccess(c, ModelGatewayRuntimeCircuitClearResponse{
		ChannelID:               request.ChannelID,
		RuntimeCircuitsCleared:  circuitsCleared,
		RuntimeSnapshotsUpdated: snapshotsUpdated,
		FailureAvoidanceCleared: failureAvoidanceCleared,
		MatchedRuntimeKeys:      modelGatewayRuntimeKeysFromCore(matchedKeys),
		ClearFailureAvoidance:   clearFailureAvoidance,
	})
}

func ClearModelGatewayClientEmptyOutputAvoidance(c *gin.Context) {
	var request ModelGatewayClientEmptyOutputAvoidanceClearRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	sessionKey := strings.TrimSpace(request.SessionKey)
	requestedModel := strings.TrimSpace(request.RequestedModel)
	group := strings.TrimSpace(request.Group)
	if sessionKey == "" {
		common.ApiErrorMsg(c, "missing session_key")
		return
	}
	if request.ChannelID <= 0 {
		common.ApiErrorMsg(c, "missing channel_id")
		return
	}
	if requestedModel == "" {
		common.ApiErrorMsg(c, "missing requested_model")
		return
	}
	if group == "" {
		common.ApiErrorMsg(c, "missing group")
		return
	}
	endpointType := request.EndpointType
	if endpointType == "" {
		endpointType = constant.EndpointTypeOpenAI
	}
	scope := modelgatewayscheduler.ClientEmptyOutputSwitchScope{
		SessionKey:     sessionKey,
		ChannelID:      request.ChannelID,
		RequestedModel: requestedModel,
		Group:          group,
		EndpointType:   endpointType,
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	cleared := false
	if runtimeDeps != nil && runtimeDeps.ClientEmptyOutputSwitchTracker != nil {
		cleared = runtimeDeps.ClientEmptyOutputSwitchTracker.Clear(scope)
	}
	common.ApiSuccess(c, ModelGatewayClientEmptyOutputAvoidanceClearResponse{
		Cleared: cleared,
		Scope:   scope,
	})
}

func parseModelGatewayObservabilityOptions(c *gin.Context) (ModelGatewayObservabilityOptions, error) {
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return ModelGatewayObservabilityOptions{}, errors.New("invalid channel_id")
		}
		channelID = parsed
	}
	trendBucketSeconds, err := parseModelGatewayTrendBucketSeconds(c.Query("trend_bucket_seconds"))
	if err != nil {
		return ModelGatewayObservabilityOptions{}, err
	}

	return ModelGatewayObservabilityOptions{
		Hours:              normalizeModelGatewayObservabilityInt(c.Query("hours"), modelGatewayObservabilityDefaultHours, 1, modelGatewayObservabilityMaxHours),
		RecentLimit:        normalizeModelGatewayObservabilityInt(c.Query("recent_limit"), modelGatewayObservabilityDefaultRecentLimit, 0, modelGatewayObservabilityMaxRecentLimit),
		TopN:               normalizeModelGatewayObservabilityInt(c.Query("top_n"), modelGatewayObservabilityDefaultTopN, 1, modelGatewayObservabilityMaxTopN),
		ScanLimit:          normalizeModelGatewayObservabilityInt(c.Query("scan_limit"), modelGatewayObservabilityDefaultScanLimit, 1, modelGatewayObservabilityMaxScanLimit),
		TrendBucketSeconds: trendBucketSeconds,
		ViewMode:           strings.TrimSpace(c.Query("view_mode")),
		Model:              strings.TrimSpace(c.Query("model")),
		Group:              strings.TrimSpace(c.Query("group")),
		ChannelID:          channelID,
		RequestID:          strings.TrimSpace(c.Query("request_id")),
		IncludeTotal:       parseModelGatewayObservabilityBool(c.Query("include_total")),
		HealthProbeOnly:    parseModelGatewayObservabilityBool(c.Query("health_probe_only")),
		Lite:               parseModelGatewayObservabilityBool(c.Query("lite")),
		IncludeDispatch:    parseModelGatewayObservabilityBool(c.Query("include_dispatch")),
		RecentOnly:         parseModelGatewayObservabilityBool(c.Query("recent_only")),
	}, nil
}

func parseModelGatewayRuntimeStatusQuery(c *gin.Context) (modelgatewayobservability.RuntimeStatusQuery, error) {
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return modelgatewayobservability.RuntimeStatusQuery{}, errors.New("invalid channel_id")
		}
		channelID = parsed
	}
	return modelgatewayobservability.RuntimeStatusQuery{
		Model:     strings.TrimSpace(c.Query("model")),
		Group:     strings.TrimSpace(c.Query("group")),
		ChannelID: channelID,
		Limit:     normalizeModelGatewayObservabilityInt(c.Query("limit"), modelGatewayRuntimeStatusDefaultLimit, 1, modelGatewayRuntimeStatusMaxLimit),
	}, nil
}

func parseModelGatewayHealthCheckQueueType(raw string) (string, error) {
	queueType := strings.ToLower(strings.TrimSpace(raw))
	if queueType == "" {
		queueType = modelGatewayHealthCheckQueueTypeAll
	}
	switch queueType {
	case modelGatewayHealthCheckQueueTypeAll,
		modelGatewayHealthCheckQueueTypeLowScore,
		modelGatewayHealthCheckQueueTypeLowTraffic,
		modelGatewayHealthCheckQueueTypeRecovery,
		modelGatewayHealthCheckQueueTypeIsolated,
		modelGatewayHealthCheckQueueTypeCircuitHalfOpen,
		modelGatewayHealthCheckQueueTypeScoreAnomaly:
		return queueType, nil
	default:
		return "", errors.New("invalid queue_type")
	}
}

func parseModelGatewayScoreHistoryOptions(c *gin.Context) (modelGatewayScoreHistoryOptions, error) {
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return modelGatewayScoreHistoryOptions{}, errors.New("invalid channel_id")
		}
		channelID = parsed
	}
	if channelID <= 0 {
		return modelGatewayScoreHistoryOptions{}, errors.New("missing channel_id")
	}
	return modelGatewayScoreHistoryOptions{
		Hours:     normalizeModelGatewayObservabilityInt(c.Query("hours"), modelGatewayObservabilityDefaultHours, 1, modelGatewayObservabilityMaxHours),
		Limit:     normalizeModelGatewayObservabilityInt(c.Query("limit"), 50, 1, 200),
		ChannelID: channelID,
		RuntimeKey: ModelGatewayRuntimeKey{
			RequestedModel:        strings.TrimSpace(c.Query("requested_model")),
			UpstreamModel:         strings.TrimSpace(c.Query("upstream_model")),
			ChannelID:             channelID,
			ResourceID:            strings.TrimSpace(c.Query("resource_id")),
			ResourceType:          strings.TrimSpace(c.Query("resource_type")),
			AccountID:             strings.TrimSpace(c.Query("account_id")),
			AccountType:           strings.TrimSpace(c.Query("account_type")),
			Brand:                 strings.TrimSpace(c.Query("brand")),
			Provider:              strings.TrimSpace(c.Query("provider")),
			CredentialIndex:       normalizeModelGatewayObservabilityInt(c.Query("credential_index"), -1, -1, math.MaxInt32),
			CredentialIndexSet:    strings.TrimSpace(c.Query("credential_index")) != "",
			CredentialSubjectFP:   strings.TrimSpace(c.Query("credential_subject_fingerprint")),
			CredentialFP:          strings.TrimSpace(c.Query("credential_fingerprint")),
			Group:                 strings.TrimSpace(c.Query("group")),
			EndpointType:          strings.TrimSpace(c.Query("endpoint_type")),
			CapabilityFingerprint: strings.TrimSpace(c.Query("capability_fingerprint")),
		},
	}, nil
}

func BuildModelGatewayScoreHistory(options modelGatewayScoreHistoryOptions) (ModelGatewayScoreHistoryResponse, error) {
	if model.DB == nil {
		return ModelGatewayScoreHistoryResponse{}, errors.New("database is not initialized")
	}
	options.Hours = clampModelGatewayObservabilityValue(options.Hours, 1, modelGatewayObservabilityMaxHours)
	options.Limit = clampModelGatewayObservabilityValue(options.Limit, 1, 200)
	if options.ChannelID <= 0 {
		return ModelGatewayScoreHistoryResponse{}, errors.New("missing channel_id")
	}

	endTime := common.GetTimestamp()
	startTime := endTime - int64(options.Hours*3600)
	scanLimit := minModelGatewayObservabilityInt(modelGatewayObservabilityMaxScanLimit, maxModelGatewayObservabilityInt(options.Limit*40, 500))
	records := make([]model.ModelExecutionRecord, 0)
	base := model.DB.Model(&model.ModelExecutionRecord{}).
		Where("created_at >= ?", startTime).
		Where("request_meta <> ''")
	if options.RuntimeKey.RequestedModel != "" {
		base = base.Where("requested_model = ?", options.RuntimeKey.RequestedModel)
	}
	if err := base.Order("created_at desc, id desc").
		Limit(scanLimit).
		Find(&records).Error; err != nil {
		return ModelGatewayScoreHistoryResponse{}, err
	}

	response := ModelGatewayScoreHistoryResponse{
		ChannelID:   options.ChannelID,
		RuntimeKey:  options.RuntimeKey,
		Hours:       options.Hours,
		Limit:       options.Limit,
		Items:       make([]ModelGatewayScoreHistoryItem, 0, minModelGatewayObservabilityInt(options.Limit, len(records))),
		GeneratedAt: endTime,
	}
	for _, record := range records {
		requestMeta, err := parseModelGatewayRequestMeta(record.RequestMeta)
		if err != nil {
			continue
		}
		candidates := modelGatewayCandidateExplanationsFromRequestMeta(requestMeta)
		for _, candidate := range candidates {
			if !modelGatewayScoreHistoryCandidateMatches(candidate, options) {
				continue
			}
			if candidate.ScoreTotal <= 0 {
				continue
			}
			item := modelGatewayScoreHistoryItem(record, candidate)
			response.TotalMatched++
			if len(response.Items) >= options.Limit {
				response.Truncated = true
				break
			}
			response.Items = append(response.Items, item)
			if response.ChannelName == "" {
				response.ChannelName = item.ChannelName
			}
			if modelGatewayRuntimeKeyIsEmpty(response.RuntimeKey) {
				response.RuntimeKey = item.RuntimeKey
			}
			break
		}
	}

	applyModelGatewayScoreHistoryRuntimeCurrent(&response, options)
	applyModelGatewayScoreHistoryDeltas(&response)
	return response, nil
}

func modelGatewayStickyStoreItems(limit int) []ModelGatewayStickyStoreItem {
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	if runtimeDeps == nil || runtimeDeps.StickyRouter == nil {
		return []ModelGatewayStickyStoreItem{}
	}
	entries := runtimeDeps.StickyRouter.StickyEntries()
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	items := make([]ModelGatewayStickyStoreItem, 0, len(entries))
	now := time.Now()
	for _, entry := range entries {
		items = append(items, modelGatewayStickyStoreItem(entry, now))
	}
	return items
}

func modelGatewayStickyStoreItem(entry modelgatewayscheduler.StickyStoreEntry, now time.Time) ModelGatewayStickyStoreItem {
	item := ModelGatewayStickyStoreItem{
		KeyID:          entry.KeyID,
		ChannelID:      entry.ChannelID,
		Group:          entry.Group,
		KeyFingerprint: entry.KeyFingerprint,
	}
	if !entry.ExpiresAt.IsZero() {
		item.ExpiresAt = entry.ExpiresAt.Unix()
		ttl := int64(time.Until(entry.ExpiresAt).Seconds())
		if !now.IsZero() {
			ttl = int64(entry.ExpiresAt.Sub(now).Seconds())
		}
		if ttl < 0 {
			ttl = 0
		}
		item.TTLSeconds = ttl
	}
	return item
}

func normalizeModelGatewayObservabilityInt(raw string, fallback int, minValue int, maxValue int) int {
	parsed := fallback
	if strings.TrimSpace(raw) != "" {
		if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			parsed = value
		}
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func parseModelGatewayTrendBucketSeconds(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "auto") {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid trend_bucket_seconds")
	}
	return parsed, nil
}

func parseModelGatewayObservabilityBool(raw string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && parsed
}

func normalizeModelGatewayObservabilityOptions(options ModelGatewayObservabilityOptions) ModelGatewayObservabilityOptions {
	options.Hours = clampModelGatewayObservabilityValue(options.Hours, 1, modelGatewayObservabilityMaxHours)
	options.RecentLimit = clampModelGatewayObservabilityValue(options.RecentLimit, 0, modelGatewayObservabilityMaxRecentLimit)
	options.TopN = clampModelGatewayObservabilityValue(options.TopN, 1, modelGatewayObservabilityMaxTopN)
	options.ScanLimit = normalizeModelGatewayObservabilityScanLimit(options.ScanLimit)
	options.TrendBucketSeconds = modelGatewayObservabilityTrendBucketSeconds(options.Hours, options.TrendBucketSeconds)
	options.ViewMode = normalizeModelGatewayObservabilityViewMode(options.ViewMode)
	options.Model = strings.TrimSpace(options.Model)
	options.Group = strings.TrimSpace(options.Group)
	options.RequestID = strings.TrimSpace(options.RequestID)
	if options.ChannelID < 0 {
		options.ChannelID = 0
	}
	if options.RecentOnly {
		options.Lite = true
	}
	return options
}

func normalizeModelGatewayObservabilityViewMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case modelGatewayObservabilityViewUserRequests:
		return mode
	default:
		return ""
	}
}

func modelGatewayObservabilitySummaryCacheKey(options ModelGatewayObservabilityOptions) string {
	options = normalizeModelGatewayObservabilityOptions(options)
	values := url.Values{}
	values.Set("hours", strconv.Itoa(options.Hours))
	values.Set("recent_limit", strconv.Itoa(options.RecentLimit))
	values.Set("top_n", strconv.Itoa(options.TopN))
	values.Set("scan_limit", strconv.Itoa(options.ScanLimit))
	values.Set("trend_bucket_seconds", strconv.FormatInt(options.TrendBucketSeconds, 10))
	values.Set("view_mode", options.ViewMode)
	values.Set("model", options.Model)
	values.Set("group", options.Group)
	values.Set("channel_id", strconv.Itoa(options.ChannelID))
	values.Set("request_id", options.RequestID)
	values.Set("include_total", strconv.FormatBool(options.IncludeTotal))
	values.Set("probe_only", strconv.FormatBool(options.HealthProbeOnly))
	values.Set("lite", strconv.FormatBool(options.Lite))
	values.Set("include_dispatch", strconv.FormatBool(options.IncludeDispatch))
	values.Set("recent_only", strconv.FormatBool(options.RecentOnly))
	return values.Encode()
}

func refreshModelGatewayObservabilitySummaryCache(key string, options ModelGatewayObservabilityOptions) {
	_, _, _ = modelGatewayObservabilitySummaryCache.group.Do(key, func() (any, error) {
		response, err := buildModelGatewayObservabilitySummaryUncached(options)
		if err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
		modelGatewayObservabilitySummaryCache.store(key, options, response, time.Now())
		return response, nil
	})
}

func (store *modelGatewayObservabilitySummaryCacheStore) lookupFresh(key string, now time.Time) (ModelGatewayObservabilityResponse, bool) {
	if store == nil {
		return ModelGatewayObservabilityResponse{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[key]
	if !ok || now.After(entry.expiresAt) {
		return ModelGatewayObservabilityResponse{}, false
	}
	return entry.response, true
}

func (store *modelGatewayObservabilitySummaryCacheStore) lookupStale(key string, now time.Time) (ModelGatewayObservabilityResponse, bool) {
	if store == nil {
		return ModelGatewayObservabilityResponse{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[key]
	if !ok || now.After(entry.staleUntil) {
		return ModelGatewayObservabilityResponse{}, false
	}
	return entry.response, true
}

func (store *modelGatewayObservabilitySummaryCacheStore) store(key string, options ModelGatewayObservabilityOptions, response ModelGatewayObservabilityResponse, now time.Time) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.entries == nil {
		store.entries = make(map[string]modelGatewayObservabilitySummaryCacheEntry)
	}
	store.entries[key] = modelGatewayObservabilitySummaryCacheEntry{
		options:    options,
		response:   response,
		createdAt:  now,
		expiresAt:  now.Add(modelGatewayObservabilitySummaryFreshTTL),
		staleUntil: now.Add(modelGatewayObservabilitySummaryStaleTTL),
	}
	store.pruneLocked(now)
}

func (store *modelGatewayObservabilitySummaryCacheStore) pruneLocked(now time.Time) {
	if len(store.entries) <= modelGatewayObservabilitySummaryMaxCache {
		return
	}
	for key, entry := range store.entries {
		if now.After(entry.staleUntil) {
			delete(store.entries, key)
		}
	}
	for len(store.entries) > modelGatewayObservabilitySummaryMaxCache {
		var oldestKey string
		var oldestTime time.Time
		for key, entry := range store.entries {
			if oldestKey == "" || entry.createdAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.createdAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(store.entries, oldestKey)
	}
}

func InvalidateModelGatewayObservabilitySummaryCacheForRecord(record model.ModelExecutionRecord) {
	if modelGatewayObservabilitySummaryCache == nil {
		return
	}
	modelGatewayObservabilitySummaryCache.invalidateForRecord(record)
}

func InvalidateModelGatewayObservabilitySummaryCacheForUserRequest(record ModelGatewayUserRequestRecord) {
	if modelGatewayObservabilitySummaryCache == nil {
		return
	}
	modelGatewayObservabilitySummaryCache.invalidateForUserRequest(record)
}

func (store *modelGatewayObservabilitySummaryCacheStore) invalidateForRecord(record model.ModelExecutionRecord) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for key, entry := range store.entries {
		if entry.options.matchesRecord(record) {
			delete(store.entries, key)
		}
	}
}

func (store *modelGatewayObservabilitySummaryCacheStore) invalidateForUserRequest(record ModelGatewayUserRequestRecord) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for key, entry := range store.entries {
		if entry.options.matchesUserRequestRecord(record) {
			delete(store.entries, key)
		}
	}
}

func (options ModelGatewayObservabilityOptions) matchesRecord(record model.ModelExecutionRecord) bool {
	options = normalizeModelGatewayObservabilityOptions(options)
	if options.Model != "" && options.Model != record.RequestedModel {
		return false
	}
	if options.Group != "" && options.Group != record.RequestedGroup && options.Group != record.SelectedGroup && options.Group != record.ActualGroup {
		return false
	}
	if options.ChannelID > 0 && options.ChannelID != record.ChannelId && options.ChannelID != record.ActualChannelId {
		return false
	}
	if options.RequestID != "" && options.RequestID != record.RequestId {
		return false
	}
	if options.Hours > 0 {
		cutoff := time.Now().Add(-time.Duration(options.Hours) * time.Hour).Unix()
		return record.CreatedAt >= cutoff
	}
	return true
}

func (options ModelGatewayObservabilityOptions) matchesUserRequestRecord(record ModelGatewayUserRequestRecord) bool {
	options = normalizeModelGatewayObservabilityOptions(options)
	if options.Model != "" && options.Model != record.RequestedModel {
		return false
	}
	if options.Group != "" && options.Group != record.RequestedGroup && options.Group != record.SelectedGroup && options.Group != record.ActualGroup {
		return false
	}
	if options.ChannelID > 0 && options.ChannelID != record.FinalChannelID {
		return false
	}
	if options.RequestID != "" && options.RequestID != record.RequestID {
		return false
	}
	if options.HealthProbeOnly && !record.IsHealthProbe {
		return false
	}
	if options.Hours > 0 {
		recordTime := record.CompletedAt
		if recordTime <= 0 {
			recordTime = record.CreatedAt
		}
		cutoff := time.Now().Add(-time.Duration(options.Hours) * time.Hour).Unix()
		return recordTime >= cutoff
	}
	return true
}

func BuildModelGatewayObservabilitySummary(options ModelGatewayObservabilityOptions) (ModelGatewayObservabilityResponse, error) {
	if model.DB == nil {
		return ModelGatewayObservabilityResponse{}, errors.New("database is not initialized")
	}
	options = normalizeModelGatewayObservabilityOptions(options)
	key := modelGatewayObservabilitySummaryCacheKey(options)
	now := time.Now()
	if response, ok := modelGatewayObservabilitySummaryCache.lookupFresh(key, now); ok {
		applyModelGatewayCurrentChannelStatus(&response)
		return response, nil
	}
	if response, ok := modelGatewayObservabilitySummaryCache.lookupStale(key, now); ok {
		go refreshModelGatewayObservabilitySummaryCache(key, options)
		applyModelGatewayCurrentChannelStatus(&response)
		return response, nil
	}

	value, err, _ := modelGatewayObservabilitySummaryCache.group.Do(key, func() (any, error) {
		response, err := buildModelGatewayObservabilitySummaryUncached(options)
		if err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
		modelGatewayObservabilitySummaryCache.store(key, options, response, time.Now())
		return response, nil
	})
	if err != nil {
		if response, ok := modelGatewayObservabilitySummaryCache.lookupStale(key, time.Now()); ok {
			applyModelGatewayCurrentChannelStatus(&response)
			return response, nil
		}
		return ModelGatewayObservabilityResponse{}, err
	}
	response, _ := value.(ModelGatewayObservabilityResponse)
	applyModelGatewayCurrentChannelStatus(&response)
	return response, nil
}

func buildModelGatewayObservabilitySummaryUncached(options ModelGatewayObservabilityOptions) (ModelGatewayObservabilityResponse, error) {
	options.Hours = clampModelGatewayObservabilityValue(options.Hours, 1, modelGatewayObservabilityMaxHours)
	options.RecentLimit = clampModelGatewayObservabilityValue(options.RecentLimit, 0, modelGatewayObservabilityMaxRecentLimit)
	options.TopN = clampModelGatewayObservabilityValue(options.TopN, 1, modelGatewayObservabilityMaxTopN)
	options.ScanLimit = normalizeModelGatewayObservabilityScanLimit(options.ScanLimit)
	options.TrendBucketSeconds = modelGatewayObservabilityTrendBucketSeconds(options.Hours, options.TrendBucketSeconds)

	endTime := common.GetTimestamp()
	startTime := endTime - int64(options.Hours*3600)
	if options.ViewMode == modelGatewayObservabilityViewUserRequests {
		userRequests, err := buildModelGatewayUserRequestObservability(startTime, endTime, options)
		if err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
		response := ModelGatewayObservabilityResponse{
			Summary: ModelGatewayObservabilitySummary{
				WindowHours:        options.Hours,
				TrendBucketSeconds: options.TrendBucketSeconds,
				StartTime:          startTime,
				EndTime:            endTime,
			},
			UserRequests: userRequests,
			Partial:      options.RecentOnly,
		}
		if shouldIncludeModelGatewayDynamicBillingOverview(options) {
			response.DynamicBilling = buildModelGatewayDynamicBillingOverviewForDisplay(time.Now().Unix(), 0)
			response.DynamicBilling7d = buildModelGatewayDynamicBillingOverviewForDisplay(time.Now().Unix(), 7*24*60)
		}
		return response, nil
	}

	base := model.DB.Model(&model.ModelExecutionRecord{}).Where("created_at >= ?", startTime)
	base = applyModelGatewayObservabilityFilters(base, options)

	records := make([]model.ModelExecutionRecord, 0)
	queryLimit := options.ScanLimit
	var totalRecords int64
	if options.IncludeTotal {
		if err := base.Count(&totalRecords).Error; err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
	} else {
		queryLimit = options.ScanLimit + 1
	}
	if err := base.Order("created_at desc, id desc").Limit(queryLimit).Find(&records).Error; err != nil {
		return ModelGatewayObservabilityResponse{}, err
	}
	if !options.IncludeTotal {
		totalRecords = int64(len(records))
		if len(records) > options.ScanLimit {
			records = records[:options.ScanLimit]
		}
	}

	response := buildModelGatewayObservabilityFromRecords(records, totalRecords, startTime, endTime, options)
	userRequests, err := buildModelGatewayUserRequestObservability(startTime, endTime, options)
	if err != nil {
		return ModelGatewayObservabilityResponse{}, err
	}
	response.UserRequests = userRequests
	if shouldIncludeModelGatewayDynamicBillingOverview(options) {
		response.DynamicBilling = buildModelGatewayDynamicBillingOverviewForDisplay(time.Now().Unix(), 0)
		response.DynamicBilling7d = buildModelGatewayDynamicBillingOverviewForDisplay(time.Now().Unix(), 7*24*60)
	}
	response.RuntimeStatus = defaultModelGatewayRuntimeStatusService().Build(modelgatewayobservability.RuntimeStatusQuery{
		Model:     options.Model,
		Group:     options.Group,
		ChannelID: options.ChannelID,
		Limit:     modelGatewayRuntimeStatusDefaultLimit,
	})
	applyModelGatewayRuntimeRiskEvents(&response)
	return response, nil
}

func shouldIncludeModelGatewayDynamicBillingOverview(options ModelGatewayObservabilityOptions) bool {
	return !options.HealthProbeOnly && !options.RecentOnly
}

func buildModelGatewayDynamicBillingOverview(now int64, windowMinutesOverride int) ModelGatewayDynamicBillingOverview {
	return buildModelGatewayDynamicBillingOverviewWithOptions(now, windowMinutesOverride, modelGatewayDynamicBillingOverviewOptions{})
}

type modelGatewayDynamicBillingOverviewOptions struct {
	DisplayReferenceModel string
}

func buildModelGatewayDynamicBillingOverviewForDisplay(now int64, windowMinutesOverride int) ModelGatewayDynamicBillingOverview {
	return buildModelGatewayDynamicBillingOverviewWithOptions(now, windowMinutesOverride, modelGatewayDynamicBillingOverviewOptions{
		DisplayReferenceModel: modelGatewayDynamicBillingDisplayModel,
	})
}

func buildModelGatewayDynamicBillingOverviewWithOptions(now int64, windowMinutesOverride int, options modelGatewayDynamicBillingOverviewOptions) ModelGatewayDynamicBillingOverview {
	setting := scheduler_setting.GetSetting()
	windowSamples := setting.DynamicBillingWindowSamples
	if windowSamples <= 0 {
		windowSamples = scheduler_setting.DefaultSetting().DynamicBillingWindowSamples
	}
	overview := ModelGatewayDynamicBillingOverview{
		Enabled:            setting.DynamicBillingEnabled,
		ProfitRate:         setting.DynamicBillingProfitRate,
		WindowSamples:      windowSamples,
		MinSamples:         setting.DynamicBillingMinSamples,
		RefreshSeconds:     setting.DynamicBillingRefreshSeconds,
		MaxAgeSeconds:      setting.DynamicBillingMaxAgeSeconds,
		CostSource:         setting.DynamicBillingCostSource,
		ApplyMode:          setting.DynamicBillingApplyMode,
		ProfitWindowHours:  setting.DynamicBillingProfitWindowHours,
		MinTokens:          setting.DynamicBillingMinTokens,
		MinRequests:        setting.DynamicBillingMinRequests,
		MinSuccessRequests: setting.DynamicBillingMinSuccessRequests,
		MinRatio:           setting.DynamicBillingMinRatio,
		MaxRatio:           setting.DynamicBillingMaxRatio,
		MaxStepChange:      setting.DynamicBillingMaxStepChange,
		Groups:             make([]ModelGatewayDynamicBillingGroupOverview, 0),
	}
	if windowMinutesOverride > 0 {
		overview.WindowSamples = 0
		overview.WindowMinutes = windowMinutesOverride
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	if len(setting.GroupPolicies) == 0 {
		return overview
	}
	if windowMinutesOverride > 0 {
		appliedOverview, err := buildModelGatewayDynamicBillingAppliedOverview(now, windowMinutesOverride, setting, overview)
		if err != nil {
			common.SysLog(fmt.Sprintf("model gateway dynamic billing applied overview refresh (%d minutes) failed: %v", windowMinutesOverride, err))
			skeleton := buildModelGatewayDynamicBillingAppliedSkeleton(setting, overview)
			normalizeModelGatewayDynamicBillingOverviewDisplay(&skeleton, options.DisplayReferenceModel)
			return skeleton
		}
		normalizeModelGatewayDynamicBillingOverviewDisplay(&appliedOverview, options.DisplayReferenceModel)
		return appliedOverview
	}
	baselines := modelgatewaydynamicbilling.DefaultBaselineSnapshots()
	groupedBaselines := make(map[string][]modelgatewaydynamicbilling.RatioBaseline)
	for _, baseline := range baselines {
		group := strings.TrimSpace(baseline.Group)
		if group == "" {
			continue
		}
		groupedBaselines[group] = append(groupedBaselines[group], baseline)
	}
	groups := make([]ModelGatewayDynamicBillingGroupOverview, 0)
	isActiveBaseline := func(baseline modelgatewaydynamicbilling.RatioBaseline) bool {
		if baseline.Ratio <= 0 {
			return false
		}
		if baseline.FixedRatioApplied && baseline.FixedRatio > 0 {
			return true
		}
		if strings.TrimSpace(baseline.FallbackReason) != "" && !modelgatewaydynamicbilling.IsAutoAppliedLegacyFallback(baseline.FallbackReason) {
			return false
		}
		if setting.DynamicBillingMinSamples > 0 && baseline.SampleCount < setting.DynamicBillingMinSamples {
			return false
		}
		if baseline.CostSource == scheduler_setting.DynamicBillingCostSourceProfit24h {
			if baseline.RequestCount < int64(setting.DynamicBillingMinRequests) ||
				baseline.SuccessRequestCount < int64(setting.DynamicBillingMinSuccessRequests) ||
				baseline.TotalTokens < int64(setting.DynamicBillingMinTokens) ||
				baseline.BaseQuotaAtRatio1 <= 0 {
				return false
			}
			if setting.DynamicBillingApplyMode == scheduler_setting.DynamicBillingApplyModeObserve {
				return false
			}
		}
		if setting.DynamicBillingMaxAgeSeconds > 0 && (baseline.CalculatedAt <= 0 || now-baseline.CalculatedAt > int64(setting.DynamicBillingMaxAgeSeconds)) {
			return false
		}
		return true
	}
	shouldReplaceCurrentBaseline := func(current modelgatewaydynamicbilling.RatioBaseline, currentActive bool, candidate modelgatewaydynamicbilling.RatioBaseline, candidateActive bool, hasCurrent bool) bool {
		if !hasCurrent {
			return true
		}
		if candidateActive != currentActive {
			return candidateActive
		}
		if candidate.CalculatedAt != current.CalculatedAt {
			return candidate.CalculatedAt > current.CalculatedAt
		}
		if candidate.Ratio != current.Ratio {
			return candidate.Ratio > current.Ratio
		}
		if candidate.SampleCount != current.SampleCount {
			return candidate.SampleCount > current.SampleCount
		}
		if candidate.Group != current.Group {
			return candidate.Group < current.Group
		}
		return candidate.RequestedModel < current.RequestedModel
	}
	for groupName, policy := range setting.GroupPolicies {
		if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
			continue
		}
		targetGroups := normalizeModelGatewayDynamicTargetGroups(groupName, policy.CandidateGroups)
		item := ModelGatewayDynamicBillingGroupOverview{
			PolicyGroup:  strings.TrimSpace(groupName),
			TargetGroups: targetGroups,
			Status:       "waiting_samples",
		}
		overview.PolicyCount++
		if !overview.Enabled {
			item.Status = "global_disabled"
			groups = append(groups, item)
			continue
		}
		modelSet := make(map[string]struct{})
		latestCalculatedAt := int64(0)
		minRatio := 0.0
		maxRatio := 0.0
		minPrice := 0.0
		maxPrice := 0.0
		sampleCount := 0
		averageRatioSum := 0.0
		averageRatioWeight := 0
		averagePriceSum := 0.0
		averagePriceWeight := 0
		blendedRatioSum := 0.0
		blendedRatioWeight := 0
		blendedPriceSum := 0.0
		blendedPriceWeight := 0
		active := false
		expiredOnly := true
		currentBaseline := modelgatewaydynamicbilling.RatioBaseline{}
		currentBaselineActive := false
		hasCurrentBaseline := false
		for _, targetGroup := range targetGroups {
			for _, baseline := range groupedBaselines[targetGroup] {
				baselineWeight := baseline.SampleCount
				if baselineWeight <= 0 {
					baselineWeight = 1
				}
				if baseline.Ratio > 0 {
					if minRatio <= 0 || baseline.Ratio < minRatio {
						minRatio = baseline.Ratio
					}
					if baseline.Ratio > maxRatio {
						maxRatio = baseline.Ratio
					}
					averageRatioSum += baseline.Ratio * float64(baselineWeight)
					averageRatioWeight += baselineWeight
				}
				if baseline.PricePerM > 0 {
					if minPrice <= 0 || baseline.PricePerM < minPrice {
						minPrice = baseline.PricePerM
					}
					if baseline.PricePerM > maxPrice {
						maxPrice = baseline.PricePerM
					}
					averagePriceSum += baseline.PricePerM * float64(baselineWeight)
					averagePriceWeight += baselineWeight
				}
				if baseline.SampleCount > 0 {
					sampleCount += baseline.SampleCount
				}
				if baseline.RequestedModel != "" {
					modelSet[baseline.RequestedModel] = struct{}{}
				}
				if baseline.CalculatedAt > latestCalculatedAt {
					latestCalculatedAt = baseline.CalculatedAt
				}
				baselineActive := isActiveBaseline(baseline)
				if baseline.Ratio > 0 && shouldReplaceCurrentBaseline(currentBaseline, currentBaselineActive, baseline, baselineActive, hasCurrentBaseline) {
					currentBaseline = baseline
					currentBaselineActive = baselineActive
					hasCurrentBaseline = true
				}
				if baseline.Ratio <= 0 {
					continue
				}
				if !baselineActive {
					isExpired := setting.DynamicBillingMaxAgeSeconds > 0 && (baseline.CalculatedAt <= 0 || now-baseline.CalculatedAt > int64(setting.DynamicBillingMaxAgeSeconds))
					if !isExpired {
						expiredOnly = false
					}
					continue
				}
				active = true
				expiredOnly = false
				if baselineWeight > 0 {
					blendedRatioSum += baseline.Ratio * float64(baselineWeight)
					blendedRatioWeight += baselineWeight
					if baseline.PricePerM > 0 {
						blendedPriceSum += baseline.PricePerM * float64(baselineWeight)
						blendedPriceWeight += baselineWeight
					}
				}
			}
		}
		item.MinRatio = minRatio
		item.MaxRatio = maxRatio
		item.MinPricePerM = minPrice
		item.MaxPricePerM = maxPrice
		if averageRatioWeight > 0 {
			item.AverageRatio = averageRatioSum / float64(averageRatioWeight)
		}
		if averagePriceWeight > 0 {
			item.AveragePricePerM = averagePriceSum / float64(averagePriceWeight)
		}
		item.SampleCount = sampleCount
		item.ModelCount = len(modelSet)
		item.LatestCalculatedAt = latestCalculatedAt
		item.EffectiveSamples = blendedRatioWeight
		if blendedRatioWeight > 0 {
			item.BlendedRatio = blendedRatioSum / float64(blendedRatioWeight)
		}
		if blendedPriceWeight > 0 {
			item.BlendedPricePerM = blendedPriceSum / float64(blendedPriceWeight)
		}
		if hasCurrentBaseline {
			item.CurrentRatio = currentBaseline.Ratio
			item.CurrentPricePerM = currentBaseline.PricePerM
			item.CurrentTargetGroup = currentBaseline.Group
			item.CurrentModel = currentBaseline.RequestedModel
			item.ReferenceModel = firstNonEmptyTrimmed(currentBaseline.ReferenceModel, currentBaseline.RequestedModel)
			item.CostSource = currentBaseline.CostSource
			item.ApplyMode = currentBaseline.ApplyMode
			item.ApplyReason = currentBaseline.ApplyReason
			item.OperatingCostUSD = currentBaseline.OperatingCostUSD
			item.RequiredRevenueUSD = currentBaseline.RequiredRevenueUSD
			item.BaseQuotaAtRatio1 = currentBaseline.BaseQuotaAtRatio1
			item.CostMultiplier = currentBaseline.CostMultiplier
			item.TargetRatio = currentBaseline.TargetRatio
			item.EffectiveRatio = currentBaseline.EffectiveRatio
			item.FixedRatio = currentBaseline.FixedRatio
			item.FixedRatioApplied = currentBaseline.FixedRatioApplied
			item.Clamped = currentBaseline.Clamped
			item.PendingManualConfirm = currentBaseline.PendingManualConfirm && !modelgatewaydynamicbilling.IsAutoAppliedLegacyFallback(currentBaseline.FallbackReason)
			item.FallbackReason = currentBaseline.FallbackReason
			item.RequestCount = currentBaseline.RequestCount
			item.SuccessRequestCount = currentBaseline.SuccessRequestCount
			item.TotalTokens = currentBaseline.TotalTokens
			item.TrafficCostUSD = currentBaseline.TrafficCostUSD
			item.TrafficEstimated = currentBaseline.TrafficEstimated
			item.TrafficDataReady = currentBaseline.TrafficDataReady
			item.ServerCostUSD = currentBaseline.ServerCostUSD
			item.ResourceCostUSD = currentBaseline.ResourceCostUSD
			item.UpstreamCostUSD = currentBaseline.UpstreamCostUSD
		}
		switch {
		case active:
			item.Status = "active"
			overview.ActiveCount++
		case hasCurrentBaseline && modelgatewaydynamicbilling.IsAutoAppliedLegacyFallback(currentBaseline.FallbackReason):
			item.Status = "active"
			overview.ActiveCount++
		case hasCurrentBaseline && strings.TrimSpace(currentBaseline.FallbackReason) != "":
			item.Status = currentBaseline.FallbackReason
			overview.WaitingCount++
		case latestCalculatedAt > 0 && expiredOnly:
			item.Status = "expired"
			overview.WaitingCount++
		default:
			item.Status = "waiting_samples"
			overview.WaitingCount++
		}
		groups = append(groups, item)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].PolicyGroup < groups[j].PolicyGroup
	})
	overview.Groups = groups
	normalizeModelGatewayDynamicBillingOverviewDisplay(&overview, options.DisplayReferenceModel)
	return overview
}

func normalizeModelGatewayDynamicBillingOverviewDisplay(overview *ModelGatewayDynamicBillingOverview, referenceModel string) {
	if overview == nil {
		return
	}
	for idx := range overview.Groups {
		overview.Groups[idx].DisplayGroup = resolveModelGatewayDynamicBillingDisplayGroup(overview.Groups[idx])
	}
	referenceModel = strings.TrimSpace(referenceModel)
	if referenceModel == "" {
		return
	}
	for idx := range overview.Groups {
		normalizeModelGatewayDynamicBillingGroupDisplay(&overview.Groups[idx], referenceModel)
	}
}

func normalizeModelGatewayDynamicBillingGroupDisplay(group *ModelGatewayDynamicBillingGroupOverview, referenceModel string) {
	if group == nil {
		return
	}
	group.ReferenceModel = referenceModel
	group.CurrentPricePerM = modelGatewayDynamicBillingPricePerMillion(referenceModel, group.CurrentRatio)
	group.AveragePricePerM = modelGatewayDynamicBillingPricePerMillion(referenceModel, group.AverageRatio)
	group.BlendedPricePerM = modelGatewayDynamicBillingPricePerMillion(referenceModel, group.BlendedRatio)
	group.MinPricePerM = modelGatewayDynamicBillingPricePerMillion(referenceModel, group.MinRatio)
	group.MaxPricePerM = modelGatewayDynamicBillingPricePerMillion(referenceModel, group.MaxRatio)
}

func resolveModelGatewayDynamicBillingDisplayGroup(group ModelGatewayDynamicBillingGroupOverview) string {
	currentGroup := strings.TrimSpace(group.CurrentTargetGroup)
	if currentGroup != "" && !strings.EqualFold(currentGroup, "auto") {
		return currentGroup
	}
	for _, targetGroup := range group.TargetGroups {
		targetGroup = strings.TrimSpace(targetGroup)
		if targetGroup == "" || strings.EqualFold(targetGroup, "auto") {
			continue
		}
		return targetGroup
	}
	policyGroup := strings.TrimSpace(group.PolicyGroup)
	if policyGroup != "" && !strings.EqualFold(policyGroup, "auto") {
		return policyGroup
	}
	if currentGroup != "" {
		return currentGroup
	}
	return policyGroup
}

func modelGatewayDynamicBillingPricePerMillion(modelName string, ratio float64) float64 {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || ratio <= 0 {
		return 0
	}
	value, usePrice, ok := ratio_setting.GetModelRatioOrPrice(modelName)
	if !ok || value <= 0 {
		return 0
	}
	if usePrice {
		return value * ratio
	}
	return value * 2 * ratio
}

func buildModelGatewayDynamicBillingAppliedSkeleton(setting scheduler_setting.SchedulerSetting, overview ModelGatewayDynamicBillingOverview) ModelGatewayDynamicBillingOverview {
	groups := make([]ModelGatewayDynamicBillingGroupOverview, 0)
	for groupName, policy := range setting.GroupPolicies {
		if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
			continue
		}
		status := "waiting_samples"
		if !overview.Enabled {
			status = "global_disabled"
		}
		groups = append(groups, ModelGatewayDynamicBillingGroupOverview{
			PolicyGroup:  strings.TrimSpace(groupName),
			TargetGroups: normalizeModelGatewayDynamicTargetGroups(groupName, policy.CandidateGroups),
			Status:       status,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].PolicyGroup < groups[j].PolicyGroup
	})
	overview.PolicyCount = len(groups)
	if overview.Enabled {
		overview.WaitingCount = len(groups)
	}
	overview.Groups = groups
	return overview
}

type modelGatewayDynamicBillingAppliedAccumulator struct {
	LatestCalculatedAt int64
	SampleCount        int
	ModelSet           map[string]struct{}
	CurrentRatio       float64
	CurrentPricePerM   float64
	CurrentTargetGroup string
	CurrentModel       string
	MinRatio           float64
	MaxRatio           float64
	MinPricePerM       float64
	MaxPricePerM       float64
	AverageRatioSum    float64
	AverageRatioCount  int
	AveragePriceSum    float64
	AveragePriceCount  int
}

type modelGatewayDynamicBillingAppliedLog struct {
	RequestId string
	CreatedAt int64
	ModelName string
	Group     string
	Other     string
}

func buildModelGatewayDynamicBillingAppliedOverview(now int64, windowMinutes int, setting scheduler_setting.SchedulerSetting, overview ModelGatewayDynamicBillingOverview) (ModelGatewayDynamicBillingOverview, error) {
	groups := make([]ModelGatewayDynamicBillingGroupOverview, 0)
	accumulators := make(map[string]*modelGatewayDynamicBillingAppliedAccumulator)
	policyTargets := make(map[string]map[string]struct{})

	for groupName, policy := range setting.GroupPolicies {
		if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
			continue
		}
		targetGroups := normalizeModelGatewayDynamicTargetGroups(groupName, policy.CandidateGroups)
		item := ModelGatewayDynamicBillingGroupOverview{
			PolicyGroup:  strings.TrimSpace(groupName),
			TargetGroups: targetGroups,
			Status:       "waiting_samples",
		}
		overview.PolicyCount++
		groups = append(groups, item)
		if !overview.Enabled {
			continue
		}
		targetSet := make(map[string]struct{}, len(targetGroups))
		for _, targetGroup := range targetGroups {
			targetSet[targetGroup] = struct{}{}
		}
		policyTargets[item.PolicyGroup] = targetSet
		accumulators[item.PolicyGroup] = &modelGatewayDynamicBillingAppliedAccumulator{
			ModelSet: make(map[string]struct{}),
		}
	}
	if len(groups) == 0 {
		overview.Groups = groups
		return overview, nil
	}
	if !overview.Enabled {
		for idx := range groups {
			groups[idx].Status = "global_disabled"
		}
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].PolicyGroup < groups[j].PolicyGroup
		})
		overview.Groups = groups
		return overview, nil
	}
	startTime := now - int64(windowMinutes*60)
	if setting.DynamicBillingEnabledAt > 0 && setting.DynamicBillingEnabledAt > startTime {
		startTime = setting.DynamicBillingEnabledAt
	}
	if startTime < 0 {
		startTime = 0
	}

	appliedLogs, err := loadModelGatewayDynamicBillingAppliedLogs(startTime)
	if err != nil {
		return overview, err
	}
	if len(appliedLogs) == 0 {
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].PolicyGroup < groups[j].PolicyGroup
		})
		overview.WaitingCount = len(groups)
		overview.Groups = groups
		return overview, nil
	}
	summaryByRequestID, err := loadModelGatewayDynamicBillingAppliedSummaries(appliedLogs)
	if err != nil {
		return overview, err
	}
	for _, logRow := range appliedLogs {
		other := make(map[string]interface{})
		if err := common.UnmarshalJsonStr(logRow.Other, &other); err != nil {
			continue
		}
		if skipModelGatewayDynamicBillingAppliedLog(other) {
			continue
		}
		if !modelGatewayBillingBool(other, "dynamic_billing_applied") {
			continue
		}
		ratio := modelGatewayBillingFloat(other, "dynamic_billing_ratio")
		if ratio <= 0 {
			continue
		}
		pricePerM := modelGatewayBillingFloat(other, "dynamic_billing_price_per_m")
		summary := summaryByRequestID[strings.TrimSpace(logRow.RequestId)]
		targetGroup := firstNonEmptyTrimmed(
			modelGatewayBillingString(other, "dynamic_billing_group"),
			summary.SelectedGroup,
			logRow.Group,
		)
		policyGroup := resolveModelGatewayDynamicPolicyGroup(
			strings.TrimSpace(summary.RequestedGroup),
			targetGroup,
			policyTargets,
		)
		if policyGroup == "" {
			continue
		}
		accumulator := accumulators[policyGroup]
		if accumulator == nil {
			continue
		}
		accumulator.SampleCount++
		if modelName := strings.TrimSpace(logRow.ModelName); modelName != "" {
			accumulator.ModelSet[modelName] = struct{}{}
		}
		if logRow.CreatedAt > accumulator.LatestCalculatedAt ||
			(logRow.CreatedAt == accumulator.LatestCalculatedAt && ratio > accumulator.CurrentRatio) ||
			(logRow.CreatedAt == accumulator.LatestCalculatedAt && ratio == accumulator.CurrentRatio && pricePerM > accumulator.CurrentPricePerM) {
			accumulator.LatestCalculatedAt = logRow.CreatedAt
			accumulator.CurrentRatio = ratio
			accumulator.CurrentPricePerM = pricePerM
			accumulator.CurrentTargetGroup = targetGroup
			accumulator.CurrentModel = strings.TrimSpace(logRow.ModelName)
		}
		if accumulator.MinRatio <= 0 || ratio < accumulator.MinRatio {
			accumulator.MinRatio = ratio
		}
		if ratio > accumulator.MaxRatio {
			accumulator.MaxRatio = ratio
		}
		accumulator.AverageRatioSum += ratio
		accumulator.AverageRatioCount++
		if pricePerM > 0 {
			if accumulator.MinPricePerM <= 0 || pricePerM < accumulator.MinPricePerM {
				accumulator.MinPricePerM = pricePerM
			}
			if pricePerM > accumulator.MaxPricePerM {
				accumulator.MaxPricePerM = pricePerM
			}
			accumulator.AveragePriceSum += pricePerM
			accumulator.AveragePriceCount++
		}
	}

	for idx := range groups {
		accumulator := accumulators[groups[idx].PolicyGroup]
		if accumulator == nil || accumulator.SampleCount == 0 {
			groups[idx].Status = "waiting_samples"
			overview.WaitingCount++
			continue
		}
		groups[idx].Status = "active"
		groups[idx].CurrentRatio = accumulator.CurrentRatio
		groups[idx].CurrentPricePerM = accumulator.CurrentPricePerM
		groups[idx].CurrentTargetGroup = accumulator.CurrentTargetGroup
		groups[idx].CurrentModel = accumulator.CurrentModel
		groups[idx].ReferenceModel = accumulator.CurrentModel
		groups[idx].MinRatio = accumulator.MinRatio
		groups[idx].MaxRatio = accumulator.MaxRatio
		groups[idx].MinPricePerM = accumulator.MinPricePerM
		groups[idx].MaxPricePerM = accumulator.MaxPricePerM
		groups[idx].SampleCount = accumulator.SampleCount
		groups[idx].ModelCount = len(accumulator.ModelSet)
		groups[idx].LatestCalculatedAt = accumulator.LatestCalculatedAt
		if accumulator.AverageRatioCount > 0 {
			groups[idx].AverageRatio = accumulator.AverageRatioSum / float64(accumulator.AverageRatioCount)
		}
		if accumulator.AveragePriceCount > 0 {
			groups[idx].AveragePricePerM = accumulator.AveragePriceSum / float64(accumulator.AveragePriceCount)
		}
		if accumulator.SampleCount > 0 {
			groups[idx].BlendedRatio = groups[idx].AverageRatio
			groups[idx].BlendedPricePerM = groups[idx].AveragePricePerM
			groups[idx].EffectiveSamples = accumulator.SampleCount
		}
		overview.ActiveCount++
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].PolicyGroup < groups[j].PolicyGroup
	})
	overview.Groups = groups
	return overview, nil
}

func loadModelGatewayDynamicBillingAppliedLogs(startTime int64) ([]modelGatewayDynamicBillingAppliedLog, error) {
	rows := make([]model.Log, 0)
	query := model.LOG_DB.Model(&model.Log{}).
		Select("request_id", "created_at", "model_name", "group", "other").
		Where("type = ? AND created_at >= ? AND request_id <> ''", model.LogTypeConsume, startTime)
	query = query.Where("other LIKE ?", `%"dynamic_billing_applied":true%`)
	if err := query.Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	logs := make([]modelGatewayDynamicBillingAppliedLog, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, modelGatewayDynamicBillingAppliedLog{
			RequestId: row.RequestId,
			CreatedAt: row.CreatedAt,
			ModelName: row.ModelName,
			Group:     row.Group,
			Other:     row.Other,
		})
	}
	return logs, nil
}

func skipModelGatewayDynamicBillingAppliedLog(other map[string]interface{}) bool {
	if len(other) == 0 {
		return false
	}
	if modelGatewayBillingBool(other, "is_health_probe") {
		return true
	}
	if strings.TrimSpace(modelGatewayBillingString(other, "billing_source")) == "model_gateway_probe" {
		return true
	}
	return strings.TrimSpace(modelGatewayBillingString(other, "source")) == "health_probe"
}

func loadModelGatewayDynamicBillingAppliedSummaries(logs []modelGatewayDynamicBillingAppliedLog) (map[string]model.ModelGatewayUserRequestSummary, error) {
	requestIDs := make([]string, 0, len(logs))
	requestIDSeen := make(map[string]struct{}, len(logs))
	for _, logRow := range logs {
		requestID := strings.TrimSpace(logRow.RequestId)
		if requestID == "" {
			continue
		}
		if _, exists := requestIDSeen[requestID]; exists {
			continue
		}
		requestIDSeen[requestID] = struct{}{}
		requestIDs = append(requestIDs, requestID)
	}
	summaryByRequestID := make(map[string]model.ModelGatewayUserRequestSummary, len(requestIDs))
	if len(requestIDs) == 0 {
		return summaryByRequestID, nil
	}
	for start := 0; start < len(requestIDs); start += modelGatewayDynamicBillingAppliedSummaryBatchSize {
		end := start + modelGatewayDynamicBillingAppliedSummaryBatchSize
		if end > len(requestIDs) {
			end = len(requestIDs)
		}
		batchRequestIDs := requestIDs[start:end]
		summaries := make([]model.ModelGatewayUserRequestSummary, 0, len(batchRequestIDs))
		if err := model.DB.Model(&model.ModelGatewayUserRequestSummary{}).
			Select("request_id", "requested_group", "selected_group").
			Where("request_id IN ?", batchRequestIDs).
			Find(&summaries).Error; err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			summaryByRequestID[strings.TrimSpace(summary.RequestId)] = summary
		}
	}
	return summaryByRequestID, nil
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resolveModelGatewayDynamicPolicyGroup(requestedGroup string, targetGroup string, policyTargets map[string]map[string]struct{}) string {
	targetGroup = strings.TrimSpace(targetGroup)
	if targetGroup != "" {
		if _, ok := policyTargets[targetGroup]; ok {
			return targetGroup
		}
	}
	resolved := ""
	for policyGroup, targets := range policyTargets {
		if _, ok := targets[targetGroup]; !ok {
			continue
		}
		if resolved != "" && resolved != policyGroup {
			return ""
		}
		resolved = policyGroup
	}
	if resolved != "" {
		return resolved
	}
	requestedGroup = strings.TrimSpace(requestedGroup)
	if _, ok := policyTargets[requestedGroup]; ok {
		return requestedGroup
	}
	return ""
}

func normalizeModelGatewayDynamicTargetGroups(policyGroup string, candidateGroups []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(candidateGroups)+1)
	appendGroup := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	appendGroup(policyGroup)
	for _, candidate := range candidateGroups {
		appendGroup(candidate)
	}
	sort.Strings(result)
	return result
}

func defaultModelGatewayRuntimeStatusService() *modelgatewayobservability.RuntimeStatusService {
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	deps := modelgatewayobservability.RuntimeStatusDeps{
		QueueSnapshot: func() map[int]int {
			return currentRelayQueueManager().Snapshot()
		},
		QueueDetailSnapshot: func() modelgatewaycore.RuntimeQueueSnapshot {
			local := currentRelayQueueManager().DetailedSnapshot()
			if runtimeDeps == nil || runtimeDeps.QueueSnapshotSyncer == nil {
				return local
			}
			return runtimeDeps.QueueSnapshotSyncer.SyncAndAggregate(local)
		},
	}
	if runtimeDeps != nil {
		deps.SnapshotStore = runtimeDeps.SnapshotStore
		deps.Circuit = runtimeDeps.CircuitBreaker
		deps.CostBaselineProvider = runtimeDeps.CostBaselineCache
	}
	deps.ScoreWeights = modelgatewayintegration.RuntimePolicySetting().ScoreWeights
	deps.PolicyForGroup = modelGatewayRuntimePolicyForGroup
	return modelgatewayobservability.NewRuntimeStatusService(deps)
}

func modelGatewayRuntimePolicyForGroup(group string) modelgatewaycore.GroupSmartPolicy {
	setting := scheduler_setting.GetSetting()
	group = strings.TrimSpace(group)
	strategy := strings.TrimSpace(setting.DefaultStrategy)
	if strategy == "" {
		strategy = modelgatewaycore.StrategyBalanced
	}
	policy := modelgatewaycore.GroupSmartPolicy{
		RequestedGroup:        group,
		UserGroup:             group,
		Mode:                  modelgatewaycore.ModeActive,
		Strategy:              strategy,
		AutoMode:              modelgatewaycore.AutoModeSequential,
		CandidateGroups:       []string{group},
		QueueEnabled:          setting.QueueEnabled,
		CircuitBreakerEnabled: setting.CircuitBreakerEnabled,
		GroupPriorityRatio:    copyModelGatewayGroupPriorityRatio(setting.GroupPriorityRatio),
	}
	if groupPolicy, ok := setting.GroupPolicies[group]; ok {
		if strings.TrimSpace(groupPolicy.Mode) != "" {
			policy.Mode = strings.TrimSpace(groupPolicy.Mode)
		}
		if strings.TrimSpace(groupPolicy.Strategy) != "" {
			policy.Strategy = strings.TrimSpace(groupPolicy.Strategy)
		}
		if strings.TrimSpace(groupPolicy.AutoMode) != "" {
			policy.AutoMode = strings.TrimSpace(groupPolicy.AutoMode)
		}
		if len(groupPolicy.CandidateGroups) > 0 {
			policy.CandidateGroups = append([]string(nil), groupPolicy.CandidateGroups...)
		}
		policy.CrossGroupFusion = groupPolicy.CrossGroupFusion
		policy.BillingRatioMode = groupPolicy.BillingRatioMode
		policy.CacheAffinityEnabled = groupPolicy.CacheAffinityEnabled
		policy.QueueEnabled = groupPolicy.QueueEnabled
		policy.QueueHighPriority = groupPolicy.QueueHighPriority
		policy.CircuitBreakerEnabled = groupPolicy.CircuitBreakerEnabled
		policy.ResourceProtectionEnabled = groupPolicy.ResourceProtectionEnabled
		policy.PrimaryChannelIDs = append([]int(nil), groupPolicy.PrimaryChannelIDs...)
		policy.PrimaryWaitTimeoutMs = groupPolicy.PrimaryWaitTimeoutMs
		policy.PrimaryQueueMaxDepth = groupPolicy.PrimaryQueueMaxDepth
		policy.FallbackChannelIDs = append([]int(nil), groupPolicy.FallbackChannelIDs...)
	}
	if len(policy.CandidateGroups) == 0 && group != "" {
		policy.CandidateGroups = []string{group}
	}
	if strings.TrimSpace(policy.Strategy) == "" {
		policy.Strategy = modelgatewaycore.StrategyBalanced
	}
	if strings.TrimSpace(policy.AutoMode) == "" {
		policy.AutoMode = modelgatewaycore.AutoModeSequential
	}
	return policy
}

func copyModelGatewayGroupPriorityRatio(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for group, ratio := range values {
		group = strings.TrimSpace(group)
		if group != "" && ratio > 0 {
			out[group] = ratio
		}
	}
	return out
}

func BuildModelGatewayHealthCheckQueue(query modelgatewayobservability.RuntimeStatusQuery, queueType string) ModelGatewayHealthCheckQueueResponse {
	queueType, err := parseModelGatewayHealthCheckQueueType(queueType)
	if err != nil {
		queueType = modelGatewayHealthCheckQueueTypeAll
	}
	runtimeStatus := defaultModelGatewayRuntimeStatusService().Build(query)
	thresholds := modelGatewayHealthCheckQueueThresholds()
	schedulerStatus := modelgatewayprobe.DefaultProbeSchedulerStatus()
	items := make([]ModelGatewayHealthCheckQueueItem, 0)
	reasonCounts := map[string]int{}
	summary := ModelGatewayHealthCheckQueueSummary{
		UpdatedAt:                          runtimeStatus.Summary.UpdatedAt,
		RuntimeKeys:                        runtimeStatus.Summary.RuntimeKeys,
		Channels:                           runtimeStatus.Summary.Channels,
		ActiveConcurrency:                  runtimeStatus.Summary.ActiveConcurrency,
		QueuedRequests:                     runtimeStatus.Summary.QueuedRequests,
		ProbeEnabled:                       schedulerStatus.Enabled,
		SchedulerRunning:                   schedulerStatus.Running,
		SchedulerMasterNode:                schedulerStatus.MasterNode,
		SchedulerRelayInvokerRegistered:    schedulerStatus.RelayInvokerRegistered,
		ProbeIntervalSeconds:               schedulerStatus.ProbeIntervalSeconds,
		LastSchedulerProbeAt:               schedulerStatus.LastTickAt,
		NextSchedulerProbeAt:               schedulerStatus.NextTickAt,
		NextSchedulerProbeRemainingSeconds: schedulerStatus.RemainingSeconds,
		FilteredQueueType:                  queueType,
	}
	for _, runtimeItem := range runtimeStatus.Items {
		if !modelGatewayHealthCheckRuntimeItemProbeEligible(runtimeItem) {
			continue
		}
		reasons := modelGatewayHealthCheckQueueReasons(runtimeItem, thresholds)
		if len(reasons) == 0 {
			continue
		}
		summary.PendingCount++
		modelGatewayHealthCheckQueueAccumulateSummary(&summary, runtimeItem, reasons)
		for _, reason := range reasons {
			reasonCounts[reason.Key]++
		}
		itemQueueType := modelGatewayHealthCheckQueueItemType(runtimeItem, reasons)
		if !modelGatewayHealthCheckQueueItemMatchesType(itemQueueType, runtimeItem, reasons, queueType) {
			continue
		}
		probeSkipReason := modelGatewayHealthCheckProbeSkipReason(runtimeItem, thresholds)
		nextProbeAt, nextProbeRemaining := modelGatewayHealthCheckNextProbeSchedule(runtimeItem)
		items = append(items, ModelGatewayHealthCheckQueueItem{
			RuntimeStatusItem:         runtimeItem,
			Reasons:                   reasons,
			ProbeTriggerScoreItems:    modelGatewayHealthCheckTriggerScoreItems(runtimeItem, thresholds),
			ProbeSkipReason:           probeSkipReason,
			LastRealAttemptAt:         runtimeItem.LastRealAttemptAt,
			NextProbeAt:               nextProbeAt,
			NextProbeRemainingSeconds: nextProbeRemaining,
			Priority:                  reasons[0].Priority,
			QueueType:                 itemQueueType,
			RowKey:                    modelGatewayHealthCheckRuntimeRowKey(runtimeItem),
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		leftScore := items[i].ScoreTotal
		rightScore := items[j].ScoreTotal
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		return items[i].RowKey < items[j].RowKey
	})
	summary.ReturnedCount = len(items)
	return ModelGatewayHealthCheckQueueResponse{
		Summary:        summary,
		Thresholds:     thresholds,
		RuntimeSummary: runtimeStatus.Summary,
		Items:          items,
		ReasonCounts:   modelGatewayHealthCheckQueueReasonCounts(reasonCounts),
		GeneratedAt:    common.GetTimestamp(),
		QueueSnapshot:  runtimeStatus.QueueSnapshot,
	}
}

func modelGatewayHealthCheckRuntimeItemProbeEligible(item modelgatewayobservability.RuntimeStatusItem) bool {
	if item.ChannelID <= 0 {
		return false
	}
	if model.DB == nil && !common.MemoryCacheEnabled {
		return true
	}
	channel, err := model.CacheGetChannel(item.ChannelID)
	if err != nil || channel == nil {
		return false
	}
	key := modelGatewayRuntimeKeyToCore(modelGatewayRuntimeKeyFromStatusItem(item), item.ChannelID)
	if key.RequestedModel == "" {
		key.RequestedModel = strings.TrimSpace(item.RequestedModel)
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = strings.TrimSpace(item.UpstreamModel)
	}
	if key.Group == "" {
		key.Group = strings.TrimSpace(item.Group)
	}
	return modelgatewayprobe.ProbeRuntimeKeyEligible(channel, key)
}

func modelGatewayHealthCheckQueueThresholds() ModelGatewayHealthCheckQueueThresholds {
	setting := scheduler_setting.GetSetting()
	lowScore := setting.ProbeLowScoreThreshold
	if lowScore <= 0 {
		lowScore = 0.62
	}
	missingSamples := setting.ProbeMissingSampleThreshold
	if missingSamples <= 0 {
		missingSamples = 3
	}
	return ModelGatewayHealthCheckQueueThresholds{
		LowScore:            lowScore,
		MissingSamples:      missingSamples,
		SuccessRate:         modelGatewayHealthCheckSuccessRateThreshold,
		EmptyOutputRate:     modelGatewayHealthCheckOutputRateThreshold,
		ExperienceIssueRate: modelGatewayHealthCheckOutputRateThreshold,
	}
}

func modelGatewayHealthCheckQueueReasons(item modelgatewayobservability.RuntimeStatusItem, thresholds ModelGatewayHealthCheckQueueThresholds) []ModelGatewayHealthCheckQueueReason {
	reasons := make([]ModelGatewayHealthCheckQueueReason, 0, 8)
	addReason := func(key string, priority int, severity string) {
		reasons = append(reasons, ModelGatewayHealthCheckQueueReason{
			Key:      key,
			Priority: priority,
			Severity: severity,
			Label:    modelGatewayHealthCheckReasonLabel(key),
		})
	}
	triggerScoreItems := modelGatewayHealthCheckTriggerScoreItems(item, thresholds)
	scoreAnomaly := strings.TrimSpace(item.ProbeTriggerReason) == modelgatewaycore.ProbeReasonScoreAnomalyFastProbe ||
		strings.TrimSpace(item.ProbeRecoveryPhase) == modelgatewaycore.ProbeRecoveryPhaseFastProbe ||
		strings.TrimSpace(item.ProbeRecoveryPhase) == modelgatewaycore.ProbeRecoveryPhasePendingRealConfirmation
	if item.ConfigErrorIsolated {
		addReason("config_error", 100, "critical")
	}
	if item.CircuitOpen {
		addReason("circuit_open", 96, "critical")
	}
	if strings.TrimSpace(item.CircuitState) == string(modelgatewaycore.CircuitStateHalfOpen) {
		addReason(modelGatewayHealthCheckQueueTypeCircuitHalfOpen, 94, "warning")
	}
	if item.Cooldown {
		addReason("cooldown", 90, "warning")
	}
	if item.FailureAvoidance {
		failureAvoidanceReason := strings.TrimSpace(item.FailureAvoidanceReason)
		probeTriggerReason := strings.TrimSpace(item.ProbeTriggerReason)
		if failureAvoidanceReason == service.ChannelTimeoutRecoveryReason ||
			probeTriggerReason == service.ChannelTimeoutRecoveryReason {
			addReason(service.ChannelTimeoutRecoveryReason, 92, "warning")
		} else if failureAvoidanceReason == service.ChannelOverloadRecoveryReason ||
			probeTriggerReason == service.ChannelOverloadRecoveryReason {
			addReason(service.ChannelOverloadRecoveryReason, 92, "warning")
		} else if failureAvoidanceReason == service.ChannelAuthConfigRecoveryReason ||
			probeTriggerReason == service.ChannelAuthConfigRecoveryReason {
			addReason(service.ChannelAuthConfigRecoveryReason, 92, "warning")
		} else {
			addReason("failure_avoidance", 88, "warning")
		}
	}
	if item.ProbeRecoveryPending {
		addReason("probe_recovery_pending", 78, "info")
	}
	if scoreAnomaly {
		addReason(modelGatewayHealthCheckQueueTypeScoreAnomaly, 74, "warning")
	}
	if len(triggerScoreItems) > 0 && !scoreAnomaly {
		addReason("low_score", 72, "warning")
	}
	if len(reasons) == 0 {
		return nil
	}
	if item.SampleCount < thresholds.MissingSamples {
		addReason("missing_samples", 48, "neutral")
	}
	if item.SuccessRate > 0 && item.SuccessRate < thresholds.SuccessRate {
		addReason("success_rate", 66, "warning")
	}
	if item.EmptyOutputRate > thresholds.EmptyOutputRate {
		addReason("empty_output", 62, "warning")
	}
	if item.ExperienceIssueRate > thresholds.ExperienceIssueRate {
		addReason("experience_issue", 62, "warning")
	}
	sort.SliceStable(reasons, func(i int, j int) bool {
		return reasons[i].Priority > reasons[j].Priority
	})
	return reasons
}

func modelGatewayHealthCheckReasonLabel(key string) string {
	switch strings.TrimSpace(key) {
	case "config_error":
		return "配置异常"
	case "circuit_open":
		return "熔断中"
	case modelGatewayHealthCheckQueueTypeCircuitHalfOpen:
		return "熔断半开"
	case "cooldown":
		return "冷却中"
	case service.ChannelTimeoutRecoveryReason:
		return "频繁超时降级中"
	case service.ChannelOverloadRecoveryReason:
		return "429 过载恢复中"
	case service.ChannelAuthConfigRecoveryReason:
		return "配置异常恢复中"
	case "failure_avoidance":
		return "近期失败恢复中"
	case "probe_recovery_pending":
		return "恢复确认中"
	case modelGatewayHealthCheckQueueTypeScoreAnomaly:
		return "分数异常快速恢复"
	case "low_score":
		return "低分待体检"
	case "low_traffic":
		return "近期缺少真实请求"
	case "missing_samples":
		return "缺少样本"
	case "success_rate":
		return "成功率偏低"
	case "empty_output":
		return "空输出偏高"
	case "experience_issue":
		return "体验异常偏高"
	default:
		return key
	}
}

func modelGatewayHealthCheckTriggerScoreItems(item modelgatewayobservability.RuntimeStatusItem, thresholds ModelGatewayHealthCheckQueueThresholds) []string {
	if len(item.ProbeAnomalyTriggerItems) > 0 &&
		(strings.TrimSpace(item.ProbeTriggerReason) == modelgatewaycore.ProbeReasonScoreAnomalyFastProbe ||
			strings.TrimSpace(item.ProbeRecoveryPhase) != "") {
		return append([]string(nil), item.ProbeAnomalyTriggerItems...)
	}
	if !(item.ScoreTotal > 0 && item.ScoreTotal < thresholds.LowScore) {
		return nil
	}
	setting := scheduler_setting.GetSetting()
	enabled := modelgatewayprobe.NormalizeRecoverableScoreItems(setting.ProbeRecoverableScoreItems)
	enabledSet := make(map[string]struct{}, len(enabled))
	for _, key := range enabled {
		enabledSet[key] = struct{}{}
	}
	out := make([]string, 0)
	seen := map[string]struct{}{}
	addItems := func(items []modelgatewaycore.ScoreItem) {
		for _, scoreItem := range items {
			if scoreItem.MissingReason != "" || scoreItem.Weight <= 0 {
				continue
			}
			if _, ok := enabledSet[scoreItem.Key]; !ok {
				continue
			}
			if scoreItem.Score < thresholds.LowScore {
				if _, exists := seen[scoreItem.Key]; exists {
					continue
				}
				seen[scoreItem.Key] = struct{}{}
				out = append(out, scoreItem.Key)
			}
		}
	}
	addItems(item.ScoreItems)
	addItems(item.RoutingScoreItems)
	return out
}

func modelGatewayHealthCheckProbeSkipReason(item modelgatewayobservability.RuntimeStatusItem, thresholds ModelGatewayHealthCheckQueueThresholds) string {
	setting := scheduler_setting.GetSetting()
	if !setting.ProbeSkipRecentRealRequestEnabled || item.LastRealAttemptAt <= 0 {
		return ""
	}
	if strings.TrimSpace(item.CircuitState) == string(modelgatewaycore.CircuitStateHalfOpen) ||
		strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason ||
		strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelOverloadRecoveryReason ||
		strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelAuthConfigRecoveryReason ||
		strings.TrimSpace(item.ProbeTriggerReason) == modelgatewaycore.ProbeReasonScoreAnomalyFastProbe ||
		strings.TrimSpace(item.ProbeRecoveryPhase) == modelgatewaycore.ProbeRecoveryPhaseFastProbe {
		return ""
	}
	window := setting.ProbeRecentRealRequestWindowSeconds
	if window <= 0 {
		window = 1800
	}
	if item.LastRealAttemptAt >= common.GetTimestamp()-int64(window) && len(modelGatewayHealthCheckTriggerScoreItems(item, thresholds)) > 0 {
		return "recent_real_request"
	}
	return ""
}

func modelGatewayHealthCheckNextProbeSchedule(item modelgatewayobservability.RuntimeStatusItem) (int64, int64) {
	setting := scheduler_setting.GetSetting()
	now := common.GetTimestamp()
	nextAt := now
	minInterval := setting.ProbeMinChannelIntervalSeconds
	if minInterval <= 0 {
		minInterval = 300
	}
	if strings.TrimSpace(item.ProbeTriggerReason) == modelgatewaycore.ProbeReasonScoreAnomalyFastProbe ||
		strings.TrimSpace(item.ProbeRecoveryPhase) == modelgatewaycore.ProbeRecoveryPhaseFastProbe {
		fastInterval := int64(modelgatewayscheduler.ScoreAnomalyFastProbeInterval().Seconds())
		if fastInterval > 0 {
			minInterval = int(fastInterval)
		}
	}
	if item.LastProbeAt > 0 {
		nextAt = item.LastProbeAt + int64(minInterval)
	}
	if strings.TrimSpace(item.CircuitState) == string(modelgatewaycore.CircuitStateOpen) && item.CircuitOpenUntil > now && item.CircuitOpenUntil > nextAt {
		nextAt = item.CircuitOpenUntil
	}
	bypassRecentRequestSkip := strings.TrimSpace(item.CircuitState) == string(modelgatewaycore.CircuitStateHalfOpen) ||
		strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason ||
		strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelOverloadRecoveryReason ||
		strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelAuthConfigRecoveryReason ||
		strings.TrimSpace(item.ProbeTriggerReason) == modelgatewaycore.ProbeReasonScoreAnomalyFastProbe ||
		strings.TrimSpace(item.ProbeRecoveryPhase) == modelgatewaycore.ProbeRecoveryPhaseFastProbe
	if !bypassRecentRequestSkip && setting.ProbeSkipRecentRealRequestEnabled && item.LastRealAttemptAt > 0 {
		window := setting.ProbeRecentRealRequestWindowSeconds
		if window <= 0 {
			window = 1800
		}
		recentRequestNextAt := item.LastRealAttemptAt + int64(window)
		if recentRequestNextAt > nextAt && item.LastRealAttemptAt >= now-int64(window) {
			nextAt = recentRequestNextAt
		}
	}
	remaining := nextAt - now
	if remaining <= 0 {
		return now, 0
	}
	return nextAt, remaining
}

func modelGatewayHealthCheckQueueAccumulateSummary(summary *ModelGatewayHealthCheckQueueSummary, item modelgatewayobservability.RuntimeStatusItem, reasons []ModelGatewayHealthCheckQueueReason) {
	if summary == nil {
		return
	}
	if modelGatewayHealthCheckReasonsContain(reasons, "low_score") {
		summary.LowScoreCount++
	}
	if modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeScoreAnomaly) {
		summary.ScoreAnomalyCount++
	}
	if item.ProbeRecoveryPending || item.FailureAvoidance || modelGatewayHealthCheckReasonsContain(reasons, "failure_avoidance") || modelGatewayHealthCheckReasonsContain(reasons, service.ChannelTimeoutRecoveryReason) || modelGatewayHealthCheckReasonsContain(reasons, service.ChannelOverloadRecoveryReason) || modelGatewayHealthCheckReasonsContain(reasons, service.ChannelAuthConfigRecoveryReason) || modelGatewayHealthCheckReasonsContain(reasons, "probe_recovery_pending") || modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeCircuitHalfOpen) {
		summary.RecoveryCount++
	}
	if item.CircuitOpen || item.Cooldown || item.ConfigErrorIsolated || modelGatewayHealthCheckReasonsContain(reasons, "circuit_open") || modelGatewayHealthCheckReasonsContain(reasons, "cooldown") || modelGatewayHealthCheckReasonsContain(reasons, "config_error") {
		summary.IsolatedCount++
	}
}

func modelGatewayHealthCheckQueueItemType(item modelgatewayobservability.RuntimeStatusItem, reasons []ModelGatewayHealthCheckQueueReason) string {
	if item.CircuitOpen || item.Cooldown || item.ConfigErrorIsolated ||
		modelGatewayHealthCheckReasonsContain(reasons, "circuit_open") ||
		modelGatewayHealthCheckReasonsContain(reasons, "cooldown") ||
		modelGatewayHealthCheckReasonsContain(reasons, "config_error") {
		return modelGatewayHealthCheckQueueTypeIsolated
	}
	if modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeCircuitHalfOpen) {
		return modelGatewayHealthCheckQueueTypeCircuitHalfOpen
	}
	if modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeScoreAnomaly) {
		return modelGatewayHealthCheckQueueTypeScoreAnomaly
	}
	if item.ProbeRecoveryPending || item.FailureAvoidance ||
		modelGatewayHealthCheckReasonsContain(reasons, "failure_avoidance") ||
		modelGatewayHealthCheckReasonsContain(reasons, service.ChannelTimeoutRecoveryReason) ||
		modelGatewayHealthCheckReasonsContain(reasons, service.ChannelOverloadRecoveryReason) ||
		modelGatewayHealthCheckReasonsContain(reasons, service.ChannelAuthConfigRecoveryReason) ||
		modelGatewayHealthCheckReasonsContain(reasons, "probe_recovery_pending") ||
		modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeCircuitHalfOpen) {
		return modelGatewayHealthCheckQueueTypeRecovery
	}
	if modelGatewayHealthCheckReasonsContain(reasons, "low_score") {
		return modelGatewayHealthCheckQueueTypeLowScore
	}
	if len(reasons) > 0 {
		return reasons[0].Key
	}
	return ""
}

func modelGatewayHealthCheckQueueItemMatchesType(itemQueueType string, item modelgatewayobservability.RuntimeStatusItem, reasons []ModelGatewayHealthCheckQueueReason, queueType string) bool {
	switch queueType {
	case "", modelGatewayHealthCheckQueueTypeAll:
		return true
	case modelGatewayHealthCheckQueueTypeLowScore:
		return modelGatewayHealthCheckReasonsContain(reasons, "low_score")
	case modelGatewayHealthCheckQueueTypeLowTraffic:
		return false
	case modelGatewayHealthCheckQueueTypeRecovery:
		return itemQueueType == modelGatewayHealthCheckQueueTypeRecovery || item.ProbeRecoveryPending || item.FailureAvoidance || modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeCircuitHalfOpen)
	case modelGatewayHealthCheckQueueTypeIsolated:
		return itemQueueType == modelGatewayHealthCheckQueueTypeIsolated || item.CircuitOpen || item.Cooldown || item.ConfigErrorIsolated
	case modelGatewayHealthCheckQueueTypeCircuitHalfOpen:
		return modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeCircuitHalfOpen)
	case modelGatewayHealthCheckQueueTypeScoreAnomaly:
		return modelGatewayHealthCheckReasonsContain(reasons, modelGatewayHealthCheckQueueTypeScoreAnomaly)
	default:
		return true
	}
}

func modelGatewayHealthCheckReasonsContain(reasons []ModelGatewayHealthCheckQueueReason, key string) bool {
	for _, reason := range reasons {
		if reason.Key == key {
			return true
		}
	}
	return false
}

func modelGatewayHealthCheckQueueReasonCounts(counts map[string]int) []ModelGatewayHealthCheckQueueReasonCount {
	items := make([]ModelGatewayHealthCheckQueueReasonCount, 0, len(counts))
	for key, count := range counts {
		items = append(items, ModelGatewayHealthCheckQueueReasonCount{Key: key, Count: count})
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Key < items[j].Key
	})
	return items
}

func modelGatewayHealthCheckRuntimeRowKey(item modelgatewayobservability.RuntimeStatusItem) string {
	parts := []string{
		nonEmptyTrimmed(item.RequestedModel, "model"),
		nonEmptyTrimmed(item.UpstreamModel, "upstream"),
		strconv.Itoa(item.ChannelID),
		nonEmptyTrimmed(item.ResourceID, "resource"),
		nonEmptyTrimmed(item.AccountID, "account"),
		strconv.Itoa(item.CredentialIndex),
		nonEmptyTrimmed(item.CredentialSubjectFP, "subject"),
		nonEmptyTrimmed(item.CredentialFP, "credential"),
		nonEmptyTrimmed(item.Group, "group"),
		nonEmptyTrimmed(item.EndpointType, "endpoint"),
		nonEmptyTrimmed(item.CapabilityFingerprint, "capability"),
	}
	return strings.Join(parts, ":")
}

func modelGatewayHealthCheckRuntimeRowKeyFromRuntimeKey(key ModelGatewayRuntimeKey) string {
	parts := []string{
		nonEmptyTrimmed(key.RequestedModel, "model"),
		nonEmptyTrimmed(key.UpstreamModel, "upstream"),
		strconv.Itoa(key.ChannelID),
		nonEmptyTrimmed(key.ResourceID, "resource"),
		nonEmptyTrimmed(key.AccountID, "account"),
		strconv.Itoa(key.CredentialIndex),
		nonEmptyTrimmed(key.CredentialSubjectFP, "subject"),
		nonEmptyTrimmed(key.CredentialFP, "credential"),
		nonEmptyTrimmed(key.Group, "group"),
		nonEmptyTrimmed(key.EndpointType, "endpoint"),
		nonEmptyTrimmed(key.CapabilityFingerprint, "capability"),
	}
	return strings.Join(parts, ":")
}

func nonEmptyTrimmed(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return fallback
}

func applyModelGatewayObservabilityFilters(tx *gorm.DB, options ModelGatewayObservabilityOptions) *gorm.DB {
	if options.Model != "" {
		tx = tx.Where("requested_model = ?", options.Model)
	}
	if options.Group != "" {
		tx = tx.Where("(requested_group = ? OR selected_group = ? OR actual_group = ?)", options.Group, options.Group, options.Group)
	}
	if options.ChannelID > 0 {
		tx = tx.Where("(channel_id = ? OR actual_channel_id = ?)", options.ChannelID, options.ChannelID)
	}
	if options.RequestID != "" {
		tx = tx.Where("request_id = ?", options.RequestID)
	}
	if options.HealthProbeOnly {
		tx = tx.Where("is_health_probe = ?", true)
	}
	return tx
}

func applyModelGatewayUserRequestObservabilityFilters(tx *gorm.DB, options ModelGatewayObservabilityOptions) *gorm.DB {
	if options.Model != "" {
		tx = tx.Where("requested_model = ?", options.Model)
	}
	if options.Group != "" {
		tx = tx.Where("(requested_group = ? OR selected_group = ?)", options.Group, options.Group)
	}
	if options.ChannelID > 0 {
		tx = tx.Where("final_channel_id = ?", options.ChannelID)
	}
	if options.RequestID != "" {
		tx = tx.Where("request_id = ?", options.RequestID)
	}
	if options.HealthProbeOnly {
		tx = tx.Where("is_health_probe = ?", true)
	}
	return tx
}

func buildModelGatewayUserRequestObservability(startTime int64, endTime int64, options ModelGatewayObservabilityOptions) (ModelGatewayUserRequestObservabilityResponse, error) {
	base := model.DB.Model(&model.ModelGatewayUserRequestSummary{}).
		Where("completed_at >= ? AND completed_at > 0", startTime)
	base = applyModelGatewayUserRequestObservabilityFilters(base, options)

	userRequests := make([]model.ModelGatewayUserRequestSummary, 0)
	queryLimit := options.ScanLimit
	var totalRequests int64
	if options.RecentOnly {
		queryLimit = options.RecentLimit
		if queryLimit <= 0 {
			queryLimit = modelGatewayObservabilityDefaultRecentLimit
		}
	} else if options.IncludeTotal {
		if err := base.Count(&totalRequests).Error; err != nil {
			return ModelGatewayUserRequestObservabilityResponse{}, err
		}
	} else {
		queryLimit = options.ScanLimit + 1
	}
	if err := base.Order("completed_at desc, created_at desc, id desc").Limit(queryLimit).Find(&userRequests).Error; err != nil {
		return ModelGatewayUserRequestObservabilityResponse{}, err
	}
	if options.RecentOnly || !options.IncludeTotal {
		totalRequests = int64(len(userRequests))
		if !options.RecentOnly && len(userRequests) > options.ScanLimit {
			userRequests = userRequests[:options.ScanLimit]
		}
	}
	reconcileModelGatewayUserRequestClientAbortSummaries(userRequests)

	return buildModelGatewayUserRequestObservabilityFromSummaries(userRequests, totalRequests, startTime, endTime, options), nil
}

func reconcileModelGatewayUserRequestClientAbortSummaries(userRequests []model.ModelGatewayUserRequestSummary) {
	if len(userRequests) == 0 || model.DB == nil {
		return
	}
	requestIDs := make([]string, 0)
	seen := make(map[string]bool)
	for _, userRequest := range userRequests {
		requestID := strings.TrimSpace(userRequest.RequestId)
		if requestID == "" || seen[requestID] || !modelGatewayUserRequestNeedsClientAbortReconcile(userRequest) {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	clientAbortAttempts := loadModelGatewayUserRequestClientAbortAttempts(requestIDs)
	if len(clientAbortAttempts) == 0 {
		return
	}
	for idx := range userRequests {
		attempt, ok := clientAbortAttempts[strings.TrimSpace(userRequests[idx].RequestId)]
		if !ok || attempt.AttemptIndex < userRequests[idx].LastAttemptIndex {
			continue
		}
		applyModelGatewayUserRequestClientAbortReconcile(&userRequests[idx], attempt)
		persistModelGatewayUserRequestClientAbortReconcile(userRequests[idx])
	}
}

func modelGatewayUserRequestNeedsClientAbortReconcile(userRequest model.ModelGatewayUserRequestSummary) bool {
	if userRequest.FinalSuccess || modelGatewayUserRequestClientAborted(userRequest) {
		return false
	}
	category := strings.ToLower(strings.TrimSpace(userRequest.FinalErrorCategory))
	return userRequest.StreamInterrupted ||
		category == model.ModelGatewayUserRequestErrorStreamInterrupted ||
		strings.Contains(category, "stream")
}

func loadModelGatewayUserRequestClientAbortAttempts(requestIDs []string) map[string]model.ModelExecutionRecord {
	const batchSize = 500
	result := make(map[string]model.ModelExecutionRecord)
	for start := 0; start < len(requestIDs); start += batchSize {
		end := start + batchSize
		if end > len(requestIDs) {
			end = len(requestIDs)
		}
		records := make([]model.ModelExecutionRecord, 0)
		if err := model.DB.
			Model(&model.ModelExecutionRecord{}).
			Select("id, created_at, request_id, attempt_index, channel_id, channel_name, success, status_code, error_code, error_type, error_category, stream_interrupted, duration_ms, ttft_ms, request_meta").
			Where("request_id IN ?", requestIDs[start:end]).
			Order("request_id asc, attempt_index desc, created_at desc, id desc").
			Find(&records).Error; err != nil {
			common.SysLog(fmt.Sprintf("failed to reconcile model gateway user request client abort summaries: %v", err))
			continue
		}
		for _, record := range records {
			requestID := strings.TrimSpace(record.RequestId)
			if requestID == "" || !isModelGatewayAttemptRecord(record) {
				continue
			}
			current, exists := result[requestID]
			if !exists || modelGatewayClientAbortAttemptBetter(record, current) {
				result[requestID] = record
			}
		}
	}
	for requestID, record := range result {
		if !modelGatewayExecutionRecordClientAborted(record) {
			delete(result, requestID)
		}
	}
	return result
}

func modelGatewayClientAbortAttemptBetter(left model.ModelExecutionRecord, right model.ModelExecutionRecord) bool {
	if left.AttemptIndex != right.AttemptIndex {
		return left.AttemptIndex > right.AttemptIndex
	}
	if left.CreatedAt != right.CreatedAt {
		return left.CreatedAt > right.CreatedAt
	}
	return left.Id > right.Id
}

func modelGatewayExecutionRecordClientAborted(record model.ModelExecutionRecord) bool {
	category := strings.ToLower(strings.TrimSpace(record.ErrorCategory))
	if record.StatusCode == relayStatusClientClosedRequest ||
		category == model.ModelGatewayUserRequestErrorClientAborted ||
		strings.Contains(category, "client_abort") ||
		strings.Contains(category, "client_gone") {
		return true
	}
	requestMeta, err := parseModelGatewayRequestMeta(record.RequestMeta)
	if err != nil {
		return false
	}
	attemptMeta := modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta)
	category = strings.ToLower(strings.TrimSpace(attemptMeta.ErrorCategory))
	return attemptMeta.ClientAborted ||
		category == model.ModelGatewayUserRequestErrorClientAborted ||
		strings.Contains(category, "client_abort") ||
		strings.Contains(category, "client_gone")
}

func applyModelGatewayUserRequestClientAbortReconcile(userRequest *model.ModelGatewayUserRequestSummary, attempt model.ModelExecutionRecord) {
	if userRequest == nil {
		return
	}
	completedAt := attempt.CreatedAt
	if completedAt <= 0 {
		completedAt = userRequest.CompletedAt
	}
	if completedAt <= 0 {
		completedAt = common.GetTimestamp()
	}
	userRequest.UpdatedAt = common.GetTimestamp()
	userRequest.CompletedAt = completedAt
	userRequest.FinalSuccess = false
	userRequest.Recovered = false
	userRequest.FinalStatusCode = relayStatusClientClosedRequest
	userRequest.FinalErrorCategory = model.ModelGatewayUserRequestErrorClientAborted
	userRequest.ClientAborted = true
	userRequest.StreamInterrupted = attempt.StreamInterrupted
	userRequest.EmptyOutput = false
	userRequest.ExperienceIssue = ""
	if attempt.ChannelId > 0 {
		userRequest.FinalChannelID = attempt.ChannelId
	}
	if strings.TrimSpace(attempt.ChannelName) != "" {
		userRequest.FinalChannelName = strings.TrimSpace(attempt.ChannelName)
	}
	if attempt.AttemptIndex > userRequest.LastAttemptIndex {
		userRequest.LastAttemptIndex = attempt.AttemptIndex
	}
	if attempts := attempt.AttemptIndex + 1; attempts > userRequest.Attempts {
		userRequest.Attempts = attempts
	}
	if attempt.DurationMs > 0 {
		userRequest.DurationMs = attempt.DurationMs
	}
	if attempt.TTFTMs > 0 {
		userRequest.TTFTMs = attempt.TTFTMs
	}
}

func persistModelGatewayUserRequestClientAbortReconcile(userRequest model.ModelGatewayUserRequestSummary) {
	if model.DB == nil || strings.TrimSpace(userRequest.RequestId) == "" {
		return
	}
	query := model.DB.Model(&model.ModelGatewayUserRequestSummary{})
	if userRequest.Id > 0 {
		query = query.Where("id = ?", userRequest.Id)
	} else {
		query = query.Where("request_id = ?", userRequest.RequestId)
	}
	if err := query.Updates(map[string]any{
		"updated_at":           userRequest.UpdatedAt,
		"completed_at":         userRequest.CompletedAt,
		"final_channel_id":     userRequest.FinalChannelID,
		"final_channel_name":   userRequest.FinalChannelName,
		"attempts":             userRequest.Attempts,
		"last_attempt_index":   userRequest.LastAttemptIndex,
		"final_success":        false,
		"recovered":            false,
		"final_status_code":    relayStatusClientClosedRequest,
		"final_error_category": model.ModelGatewayUserRequestErrorClientAborted,
		"empty_output":         false,
		"experience_issue":     "",
		"stream_interrupted":   userRequest.StreamInterrupted,
		"client_aborted":       true,
		"duration_ms":          userRequest.DurationMs,
		"ttft_ms":              userRequest.TTFTMs,
	}).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to persist model gateway user request client abort reconcile: request_id=%s error=%v", userRequest.RequestId, err))
	}
}

type modelGatewayUserRequestAccumulator struct {
	ModelGatewayUserRequestAggregate
	durationSum        int64
	durationSamples    int64
	durationValues     []int64
	ttftSum            int64
	ttftSamples        int64
	ttftValues         []int64
	userQuotaExhausted int64
}

type modelGatewayUserRequestTrendAccumulator struct {
	modelGatewayUserRequestAccumulator
}

func buildModelGatewayUserRequestObservabilityFromSummaries(userRequests []model.ModelGatewayUserRequestSummary, totalRequests int64, startTime int64, endTime int64, options ModelGatewayObservabilityOptions) ModelGatewayUserRequestObservabilityResponse {
	response := ModelGatewayUserRequestObservabilityResponse{
		Summary: ModelGatewayUserRequestSummary{
			WindowHours:        options.Hours,
			TrendBucketSeconds: options.TrendBucketSeconds,
			StartTime:          startTime,
			EndTime:            endTime,
			TotalRequests:      totalRequests,
			ScannedRequests:    len(userRequests),
			Truncated:          totalRequests > int64(len(userRequests)),
		},
		Trends:         make([]ModelGatewayUserRequestTrendPoint, 0),
		ByModel:        make([]ModelGatewayUserRequestAggregate, 0),
		ByGroup:        make([]ModelGatewayUserRequestAggregate, 0),
		RecentRequests: make([]ModelGatewayUserRequestRecord, 0, minModelGatewayObservabilityInt(options.RecentLimit, len(userRequests))),
	}
	totalAccumulator := newModelGatewayUserRequestAccumulator("all")
	modelAccumulators := make(map[string]*modelGatewayUserRequestAccumulator)
	groupAccumulators := make(map[string]*modelGatewayUserRequestAccumulator)
	trendAccumulators := make(map[int64]*modelGatewayUserRequestTrendAccumulator)

	for idx, userRequest := range userRequests {
		isHealthProbe := userRequest.IsHealthProbe
		applyModelGatewayUserRequestAccumulator(totalAccumulator, userRequest, isHealthProbe)
		if !options.RecentOnly {
			applyModelGatewayUserRequestAccumulator(
				modelGatewayUserRequestAccumulatorFor(modelAccumulators, modelGatewayUserRequestModelKey(userRequest)),
				userRequest,
				isHealthProbe,
			)
			applyModelGatewayUserRequestAccumulator(
				modelGatewayUserRequestAccumulatorFor(groupAccumulators, modelGatewayUserRequestGroupKey(userRequest)),
				userRequest,
				isHealthProbe,
			)
			if bucketStart, ok := modelGatewayObservabilityTrendBucketStart(userRequest.CompletedAt, startTime, endTime, options.TrendBucketSeconds); ok {
				applyModelGatewayUserRequestAccumulator(
					&modelGatewayUserRequestTrendAccumulatorFor(trendAccumulators, bucketStart).modelGatewayUserRequestAccumulator,
					userRequest,
					isHealthProbe,
				)
			}
		}
		if idx < options.RecentLimit {
			response.RecentRequests = append(response.RecentRequests, modelGatewayUserRequestRecord(userRequest))
		}
	}
	attachModelGatewayUserRequestWarnings(response.RecentRequests)
	if !options.Lite {
		attachModelGatewayUserRequestBilling(response.RecentRequests)
		attachModelGatewayUserRequestCosts(response.RecentRequests)
	}

	response.Summary = modelGatewayUserRequestSummaryFromAccumulator(response.Summary, totalAccumulator)
	if !options.RecentOnly {
		response.ByModel = finalizeModelGatewayUserRequestAggregates(modelAccumulators, options.TopN)
		response.ByGroup = finalizeModelGatewayUserRequestAggregates(groupAccumulators, options.TopN)
		response.Trends = finalizeModelGatewayUserRequestTrends(trendAccumulators, startTime, endTime, options.TrendBucketSeconds)
	}
	if !options.Lite || options.IncludeDispatch {
		attachModelGatewayUserRequestDispatchRecords(response.RecentRequests)
		attachModelGatewayUserRequestAttemptRecords(response.RecentRequests)
	}
	attachModelGatewayUserRequestWarningDispatchFallback(response.RecentRequests)
	normalizeModelGatewayUserRequestBusinessRecords(response.RecentRequests)
	normalizeModelGatewayUserRequestHealthProbeRecords(response.RecentRequests)
	if !options.Lite {
		attachModelGatewayUserRequestExecutionUsers(response.RecentRequests)
		attachModelGatewayUserRequestUsernames(response.RecentRequests)
	}
	return response
}

func attachModelGatewayUserRequestCosts(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.DB == nil {
		return
	}
	requestIDs := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	summaries := make([]model.ModelGatewayRequestCostSummary, 0, len(requestIDs))
	if err := model.DB.
		Where("request_id IN ?", requestIDs).
		Find(&summaries).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request costs: %v", err))
		return
	}

	summaryByRequestID := make(map[string]model.ModelGatewayRequestCostSummary, len(summaries))
	for _, summary := range summaries {
		requestID := strings.TrimSpace(summary.RequestId)
		if requestID == "" {
			continue
		}
		summaryByRequestID[requestID] = summary
	}
	for idx := range records {
		summary, ok := summaryByRequestID[strings.TrimSpace(records[idx].RequestID)]
		if !ok {
			records[idx].UpstreamCostSource = modelgatewaycost.SourcePending
			records[idx].UpstreamCostAccuracy = modelgatewaycost.AccuracyPending
			continue
		}
		records[idx].UpstreamCostTotal = summary.UpstreamCostTotal
		records[idx].ActualChannelCost = summary.UpstreamCostTotal
		records[idx].UpstreamCostModel = strings.TrimSpace(summary.UpstreamModel)
		records[idx].UpstreamCostSource = strings.TrimSpace(summary.CostSource)
		records[idx].UpstreamCostAccuracy = strings.TrimSpace(summary.CostAccuracy)
		if records[idx].UpstreamCostSource == "" {
			records[idx].UpstreamCostSource = modelgatewaycost.SourcePending
		}
		if records[idx].UpstreamCostAccuracy == "" {
			records[idx].UpstreamCostAccuracy = modelgatewaycost.AccuracyPending
		}
		if strings.TrimSpace(summary.BreakdownJSON) != "" {
			breakdown := map[string]interface{}{}
			if err := common.UnmarshalJsonStr(summary.BreakdownJSON, &breakdown); err == nil {
				records[idx].UpstreamCostBreakdown = breakdown
			}
		}
	}
}

func attachModelGatewayUserRequestWarnings(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.DB == nil {
		return
	}
	requestIDs := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	executionRecords := make([]model.ModelExecutionRecord, 0, len(requestIDs))
	if err := model.DB.
		Model(&model.ModelExecutionRecord{}).
		Select("id, created_at, request_id, attempt_index, request_meta").
		Where("request_id IN ?", requestIDs).
		Order("request_id asc, attempt_index desc, created_at desc, id desc").
		Find(&executionRecords).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request warnings: %v", err))
		return
	}

	type warningRecord struct {
		attempt modelGatewayObservabilityAttemptMeta
		record  model.ModelExecutionRecord
	}
	warningsByRequestID := make(map[string]warningRecord, len(executionRecords))
	for _, record := range executionRecords {
		requestID := strings.TrimSpace(record.RequestId)
		if requestID == "" || strings.TrimSpace(record.RequestMeta) == "" {
			continue
		}
		requestMeta, err := parseModelGatewayRequestMeta(record.RequestMeta)
		if err != nil {
			continue
		}
		attemptMeta := modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta)
		if !modelGatewayObservabilityAttemptMetaHasWarning(attemptMeta) {
			continue
		}
		current, exists := warningsByRequestID[requestID]
		if exists && !modelGatewayWarningRecordBetter(record, current.record) {
			continue
		}
		warningsByRequestID[requestID] = warningRecord{attempt: attemptMeta, record: record}
	}
	for idx := range records {
		warning, ok := warningsByRequestID[strings.TrimSpace(records[idx].RequestID)]
		if !ok {
			continue
		}
		records[idx].WarningLevel = warning.attempt.WarningLevel
		records[idx].WarningFlags = append([]string(nil), warning.attempt.WarningFlags...)
		records[idx].WarningMessage = warning.attempt.WarningMessage
		records[idx].ChannelInducedClientAbort = warning.attempt.ChannelInducedClientAbort
	}
}

func modelGatewayObservabilityAttemptMetaHasWarning(meta modelGatewayObservabilityAttemptMeta) bool {
	return strings.TrimSpace(meta.WarningLevel) != "" ||
		len(meta.WarningFlags) > 0 ||
		strings.TrimSpace(meta.WarningMessage) != "" ||
		meta.ChannelInducedClientAbort
}

func modelGatewayWarningRecordBetter(left model.ModelExecutionRecord, right model.ModelExecutionRecord) bool {
	if left.AttemptIndex != right.AttemptIndex {
		return left.AttemptIndex > right.AttemptIndex
	}
	if left.CreatedAt != right.CreatedAt {
		return left.CreatedAt > right.CreatedAt
	}
	return left.Id > right.Id
}

func attachModelGatewayUserRequestDispatchRecords(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.DB == nil {
		return
	}
	requestIDs := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	dispatchRecords := make([]model.ModelExecutionRecord, 0, len(requestIDs))
	if err := model.DB.
		Where("request_id IN ? AND (smart_handled = ? OR policy_mode <> '' OR score_total > 0 OR score_breakdown <> '' OR candidate_groups <> '')", requestIDs, true).
		Order("created_at desc, id desc").
		Find(&dispatchRecords).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request dispatch records: %v", err))
		return
	}
	if len(dispatchRecords) == 0 {
		return
	}

	type dispatchRecordWithUser struct {
		record ModelGatewayObservabilityRecord
		userID int
	}
	dispatchByRequestID := make(map[string]dispatchRecordWithUser, len(dispatchRecords))
	for _, record := range dispatchRecords {
		requestID := strings.TrimSpace(record.RequestId)
		if requestID == "" {
			continue
		}
		next := ModelGatewayObservabilityRecordFromModelRecord(record)
		current, exists := dispatchByRequestID[requestID]
		if exists && !modelGatewayDispatchRecordBetterForUserRequest(next, current.record) {
			continue
		}
		dispatchByRequestID[requestID] = dispatchRecordWithUser{
			record: next,
			userID: record.UserId,
		}
	}
	for idx := range records {
		dispatch, ok := dispatchByRequestID[strings.TrimSpace(records[idx].RequestID)]
		if !ok {
			continue
		}
		records[idx].DispatchRecord = &dispatch.record
		if !records[idx].IsHealthProbe && dispatch.record.IsHealthProbe {
			records[idx].IsHealthProbe = true
		}
		if records[idx].ProbeReason == "" && strings.TrimSpace(dispatch.record.ProbeReason) != "" {
			records[idx].ProbeReason = strings.TrimSpace(dispatch.record.ProbeReason)
		}
		if records[idx].UserID == 0 && dispatch.userID > 0 {
			records[idx].UserID = dispatch.userID
		}
	}
}

func attachModelGatewayUserRequestAttemptRecords(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.DB == nil {
		return
	}
	requestIDs := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	executionRecords := make([]model.ModelExecutionRecord, 0, len(requestIDs)*2)
	if err := model.DB.
		Where("request_id IN ?", requestIDs).
		Order("request_id asc, attempt_index asc, created_at asc, id asc").
		Find(&executionRecords).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request attempt records: %v", err))
		return
	}

	attemptsByRequestID := make(map[string][]ModelGatewayObservabilityRecord, len(requestIDs))
	userByRequestID := make(map[string]int, len(requestIDs))
	for _, record := range executionRecords {
		if !isModelGatewayAttemptRecord(record) {
			continue
		}
		requestID := strings.TrimSpace(record.RequestId)
		if requestID == "" {
			continue
		}
		attemptsByRequestID[requestID] = append(attemptsByRequestID[requestID], ModelGatewayObservabilityRecordFromModelRecord(record))
		if record.UserId > 0 && userByRequestID[requestID] == 0 {
			userByRequestID[requestID] = record.UserId
		}
	}
	for idx := range records {
		requestID := strings.TrimSpace(records[idx].RequestID)
		if attempts := attemptsByRequestID[requestID]; len(attempts) > 0 {
			records[idx].AttemptRecords = attempts
		}
		if records[idx].UserID == 0 && userByRequestID[requestID] > 0 {
			records[idx].UserID = userByRequestID[requestID]
		}
	}
}

func attachModelGatewayUserRequestWarningDispatchFallback(records []ModelGatewayUserRequestRecord) {
	for idx := range records {
		if modelGatewayUserRequestRecordHasWarning(records[idx]) || records[idx].DispatchRecord == nil {
			continue
		}
		dispatch := records[idx].DispatchRecord
		if !modelGatewayObservabilityRecordHasWarning(*dispatch) {
			continue
		}
		records[idx].WarningLevel = dispatch.WarningLevel
		records[idx].WarningFlags = append([]string(nil), dispatch.WarningFlags...)
		records[idx].WarningMessage = dispatch.WarningMessage
		records[idx].ChannelInducedClientAbort = dispatch.ChannelInducedClientAbort
	}
}

func modelGatewayUserRequestRecordHasWarning(record ModelGatewayUserRequestRecord) bool {
	return strings.TrimSpace(record.WarningLevel) != "" ||
		len(record.WarningFlags) > 0 ||
		strings.TrimSpace(record.WarningMessage) != "" ||
		record.ChannelInducedClientAbort
}

func modelGatewayObservabilityRecordHasWarning(record ModelGatewayObservabilityRecord) bool {
	return strings.TrimSpace(record.WarningLevel) != "" ||
		len(record.WarningFlags) > 0 ||
		strings.TrimSpace(record.WarningMessage) != "" ||
		record.ChannelInducedClientAbort
}

func AttachModelGatewayUserRequestDispatchRecords(records []ModelGatewayUserRequestRecord) {
	attachModelGatewayUserRequestDispatchRecords(records)
}

func AttachModelGatewayUserRequestRealtimeDetails(records []ModelGatewayUserRequestRecord) {
	attachModelGatewayUserRequestBilling(records)
	attachModelGatewayUserRequestCosts(records)
	attachModelGatewayUserRequestDispatchRecords(records)
	attachModelGatewayUserRequestAttemptRecords(records)
	attachModelGatewayUserRequestWarningDispatchFallback(records)
	normalizeModelGatewayUserRequestBusinessRecords(records)
	normalizeModelGatewayUserRequestHealthProbeRecords(records)
	attachModelGatewayUserRequestExecutionUsers(records)
	attachModelGatewayUserRequestUsernames(records)
}

func normalizeModelGatewayUserRequestBusinessRecords(records []ModelGatewayUserRequestRecord) {
	for idx := range records {
		if !modelGatewayUserRequestRecordUserQuotaExhausted(records[idx]) {
			continue
		}
		records[idx].FinalErrorCategory = model.ModelGatewayUserRequestErrorUserQuotaExhausted
		if strings.TrimSpace(records[idx].Status) == "" || records[idx].Status == "failed" {
			records[idx].Status = model.ModelGatewayUserRequestErrorUserQuotaExhausted
		}
		if records[idx].UpstreamCostSource == "" {
			records[idx].UpstreamCostSource = "not_applicable"
		}
		if records[idx].UpstreamCostAccuracy == "" {
			records[idx].UpstreamCostAccuracy = "not_applicable"
		}
	}
}

func modelGatewayUserRequestRecordUserQuotaExhausted(record ModelGatewayUserRequestRecord) bool {
	category := strings.ToLower(strings.TrimSpace(record.FinalErrorCategory))
	if category == model.ModelGatewayUserRequestErrorUserQuotaExhausted || strings.Contains(category, "user_quota") {
		return true
	}
	if record.DispatchRecord == nil {
		return false
	}
	dispatch := record.DispatchRecord
	dispatchCategory := strings.ToLower(strings.TrimSpace(dispatch.ErrorCategory))
	dispatchCode := strings.ToLower(strings.TrimSpace(dispatch.ErrorCode))
	return dispatchCategory == modelgatewaycore.ErrorCategoryUserQuotaExhausted ||
		strings.Contains(dispatchCategory, "user_quota") ||
		(dispatchCode == strings.ToLower(string(types.ErrorCodeInsufficientUserQuota)) && !dispatch.BalanceInsufficient)
}

func modelGatewayDispatchRecordBetterForUserRequest(left ModelGatewayObservabilityRecord, right ModelGatewayObservabilityRecord) bool {
	leftCandidates := len(left.CandidateExplanations)
	rightCandidates := len(right.CandidateExplanations)
	if leftCandidates != rightCandidates {
		return leftCandidates > rightCandidates
	}
	if left.Kind != right.Kind {
		return left.Kind == "dispatch"
	}
	if left.ScoreTotal != right.ScoreTotal {
		return left.ScoreTotal > right.ScoreTotal
	}
	return left.CreatedAt > right.CreatedAt
}

func normalizeModelGatewayUserRequestHealthProbeRecords(records []ModelGatewayUserRequestRecord) {
	for idx := range records {
		if !records[idx].IsHealthProbe {
			continue
		}
		if records[idx].ProbeReason == "" && records[idx].DispatchRecord != nil {
			records[idx].ProbeReason = strings.TrimSpace(records[idx].DispatchRecord.ProbeReason)
		}
		records[idx].Status = modelGatewayUserRequestStatus(records[idx].FinalSuccess, records[idx].ClientAborted, true)
	}
}

func attachModelGatewayUserRequestExecutionUsers(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.DB == nil {
		return
	}
	requestIDs := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if record.UserID > 0 {
			continue
		}
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	type executionUserRow struct {
		RequestID string `gorm:"column:request_id"`
		UserID    int    `gorm:"column:user_id"`
	}
	rows := make([]executionUserRow, 0, len(requestIDs))
	if err := model.DB.
		Model(&model.ModelExecutionRecord{}).
		Select("request_id", "user_id").
		Where("request_id IN ? AND user_id > 0", requestIDs).
		Order("created_at desc, id desc").
		Find(&rows).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request execution users: %v", err))
		return
	}
	userByRequestID := make(map[string]int, len(rows))
	for _, row := range rows {
		requestID := strings.TrimSpace(row.RequestID)
		if requestID == "" || row.UserID <= 0 {
			continue
		}
		if _, exists := userByRequestID[requestID]; exists {
			continue
		}
		userByRequestID[requestID] = row.UserID
	}
	for idx := range records {
		if records[idx].UserID > 0 {
			continue
		}
		if userID := userByRequestID[strings.TrimSpace(records[idx].RequestID)]; userID > 0 {
			records[idx].UserID = userID
		}
	}
}

func attachModelGatewayUserRequestUsernames(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.DB == nil {
		return
	}
	userIDs := make([]int, 0, len(records))
	seen := make(map[int]bool, len(records))
	for idx := range records {
		records[idx].Username = strings.TrimSpace(records[idx].Username)
		if records[idx].Billing != nil {
			records[idx].Billing.Username = strings.TrimSpace(records[idx].Billing.Username)
			if records[idx].UserID == 0 && records[idx].Billing.UserID > 0 {
				records[idx].UserID = records[idx].Billing.UserID
			}
			if records[idx].Username == "" && records[idx].Billing.Username != "" {
				records[idx].Username = records[idx].Billing.Username
			}
		}
		if records[idx].UserID <= 0 || records[idx].Username != "" || seen[records[idx].UserID] {
			continue
		}
		seen[records[idx].UserID] = true
		userIDs = append(userIDs, records[idx].UserID)
	}
	if len(userIDs) == 0 {
		return
	}

	users := make([]model.User, 0, len(userIDs))
	if err := model.DB.
		Model(&model.User{}).
		Select("id", "username").
		Where("id IN ?", userIDs).
		Find(&users).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request users: %v", err))
		return
	}
	usernameByID := make(map[int]string, len(users))
	for _, user := range users {
		username := strings.TrimSpace(user.Username)
		if user.Id > 0 && username != "" {
			usernameByID[user.Id] = username
		}
	}
	for idx := range records {
		if records[idx].UserID <= 0 || records[idx].Username != "" {
			continue
		}
		records[idx].Username = usernameByID[records[idx].UserID]
		if records[idx].Billing != nil && records[idx].Billing.Username == "" && records[idx].Billing.UserID == records[idx].UserID {
			records[idx].Billing.Username = records[idx].Username
		}
	}
}

func attachModelGatewayUserRequestBilling(records []ModelGatewayUserRequestRecord) {
	if len(records) == 0 || model.LOG_DB == nil {
		return
	}
	requestIDs := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		seen[requestID] = true
		requestIDs = append(requestIDs, requestID)
	}
	if len(requestIDs) == 0 {
		return
	}

	logs := make([]model.Log, 0, len(requestIDs))
	if err := model.LOG_DB.
		Where("request_id IN ? AND type = ?", requestIDs, model.LogTypeConsume).
		Order("created_at desc, id desc").
		Find(&logs).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load model gateway user request billing logs: %v", err))
		return
	}
	if len(logs) == 0 {
		return
	}

	billingByRequestID := make(map[string]*ModelGatewayUserRequestBillingInfo, len(logs))
	for _, log := range logs {
		requestID := strings.TrimSpace(log.RequestId)
		if requestID == "" {
			continue
		}
		if _, exists := billingByRequestID[requestID]; exists {
			continue
		}
		billingByRequestID[requestID] = modelGatewayUserRequestBillingInfoFromLog(log)
	}
	for idx := range records {
		billing := billingByRequestID[strings.TrimSpace(records[idx].RequestID)]
		records[idx].Billing = billing
		if billing != nil {
			records[idx].UserID = billing.UserID
			records[idx].Username = strings.TrimSpace(billing.Username)
			if !records[idx].IsHealthProbe && billing.IsHealthProbe {
				records[idx].IsHealthProbe = true
			}
			if records[idx].ProbeReason == "" && strings.TrimSpace(billing.ProbeReason) != "" {
				records[idx].ProbeReason = strings.TrimSpace(billing.ProbeReason)
			}
		}
		applyModelGatewayUserRequestActualGroup(&records[idx], billing)
	}
}

func modelGatewayUserRequestBillingInfoFromLog(log model.Log) *ModelGatewayUserRequestBillingInfo {
	other := make(map[string]interface{})
	if strings.TrimSpace(log.Other) != "" {
		if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
			other = map[string]interface{}{}
		}
	}

	info := &ModelGatewayUserRequestBillingInfo{
		LogID:                     log.Id,
		UserID:                    log.UserId,
		Username:                  strings.TrimSpace(log.Username),
		CreatedAt:                 log.CreatedAt,
		Quota:                     log.Quota,
		PromptTokens:              log.PromptTokens,
		CompletionTokens:          log.CompletionTokens,
		TotalTokens:               log.PromptTokens + log.CompletionTokens,
		ChannelID:                 log.ChannelId,
		Group:                     log.Group,
		ModelName:                 log.ModelName,
		Content:                   log.Content,
		ModelRatio:                modelGatewayBillingFloat(other, "model_ratio"),
		GroupRatio:                modelGatewayBillingFloat(other, "group_ratio"),
		UserGroupRatio:            modelGatewayBillingFloat(other, "user_group_ratio"),
		CompletionRatio:           modelGatewayBillingFloat(other, "completion_ratio"),
		CacheTokens:               modelGatewayBillingInt(other, "cache_tokens"),
		CacheRatio:                modelGatewayBillingFloat(other, "cache_ratio"),
		CacheCreationTokens:       modelGatewayBillingInt(other, "cache_creation_tokens"),
		CacheCreationRatio:        modelGatewayBillingFloat(other, "cache_creation_ratio"),
		CacheCreationTokens5m:     modelGatewayBillingInt(other, "cache_creation_tokens_5m"),
		CacheCreationRatio5m:      modelGatewayBillingFloat(other, "cache_creation_ratio_5m"),
		CacheCreationTokens1h:     modelGatewayBillingInt(other, "cache_creation_tokens_1h"),
		CacheCreationRatio1h:      modelGatewayBillingFloat(other, "cache_creation_ratio_1h"),
		CacheWriteTokens:          modelGatewayBillingInt(other, "cache_write_tokens"),
		ModelPrice:                modelGatewayBillingFloat(other, "model_price"),
		BillingMode:               modelGatewayBillingString(other, "billing_mode"),
		BillingSource:             modelGatewayBillingString(other, "billing_source"),
		BillingSubtype:            modelGatewayBillingString(other, "billing_subtype"),
		DynamicBillingApplied:     modelGatewayBillingBool(other, "dynamic_billing_applied"),
		DynamicBillingFallback:    modelGatewayBillingBool(other, "dynamic_billing_fallback"),
		DynamicFallbackReason:     modelGatewayBillingString(other, "dynamic_fallback_reason"),
		DynamicApplyReason:        modelGatewayBillingString(other, "dynamic_billing_apply_reason"),
		DynamicBillingRatio:       modelGatewayBillingFloat(other, "dynamic_billing_ratio"),
		DynamicBillingPricePerM:   modelGatewayBillingFloat(other, "dynamic_billing_price_per_m"),
		DynamicBillingSampleCount: modelGatewayBillingInt(other, "dynamic_billing_sample_count"),
		SubscriptionConsumed:      modelGatewayBillingInt(other, "subscription_consumed"),
		WalletQuotaDeducted:       modelGatewayBillingInt(other, "wallet_quota_deducted"),
		WebSearchCallCount:        modelGatewayBillingInt(other, "web_search_call_count"),
		WebSearchPrice:            modelGatewayBillingFloat(other, "web_search_price"),
		FileSearchCallCount:       modelGatewayBillingInt(other, "file_search_call_count"),
		FileSearchPrice:           modelGatewayBillingFloat(other, "file_search_price"),
		AudioInputTokenCount:      modelGatewayBillingInt(other, "audio_input_token_count"),
		AudioInputPrice:           modelGatewayBillingFloat(other, "audio_input_price"),
		ImageTokens:               modelGatewayBillingInt(other, "image_output"),
		ImageRatio:                modelGatewayBillingFloat(other, "image_ratio"),
		ImageGenerationCallCount:  modelGatewayBillingInt(other, "image_generation_call_count"),
		ImageGenerationCallPrice:  modelGatewayBillingFloat(other, "image_generation_call_price"),
		ImageGenerationQuality:    modelGatewayBillingString(other, "image_generation_call_quality"),
		ImageGenerationSize:       modelGatewayBillingString(other, "image_generation_call_size"),
		UsageSemantic:             modelGatewayBillingString(other, "usage_semantic"),
		IsHealthProbe:             modelGatewayBillingBool(other, "is_health_probe"),
		ProbeReason:               modelGatewayBillingString(other, "probe_reason"),
	}
	if info.CacheWriteTokens == 0 && info.CacheCreationTokens > 0 {
		info.CacheWriteTokens = info.CacheCreationTokens
	}
	if info.ImageGenerationCallCount == 0 && info.ImageGenerationCallPrice > 0 {
		info.ImageGenerationCallCount = 1
	}
	return info
}

func applyModelGatewayUserRequestActualGroup(record *ModelGatewayUserRequestRecord, billing *ModelGatewayUserRequestBillingInfo) {
	if record == nil {
		return
	}
	actualGroup := strings.TrimSpace(record.ActualGroup)
	if actualGroup == "" {
		actualGroup = modelGatewayUserRequestActualGroup(record.RequestedGroup, record.SelectedGroup)
	}
	if actualGroup == "" && billing != nil {
		actualGroup = strings.TrimSpace(billing.Group)
	}
	record.ActualGroup = actualGroup
	if actualGroup == "" {
		record.ActualGroupRatio = 0
		return
	}
	if billing != nil && billing.GroupRatio > 0 && strings.TrimSpace(billing.Group) == actualGroup {
		record.ActualGroupRatio = billing.GroupRatio
		return
	}
	if record.ActualGroupRatio > 0 && (billing == nil || strings.TrimSpace(billing.Group) == "" || strings.TrimSpace(billing.Group) == actualGroup) {
		return
	}
	if record.CompletedAt <= 0 {
		record.ActualGroupRatio = 0
		return
	}
	record.ActualGroupRatio = ratio_setting.GetGroupRatio(actualGroup)
}

func modelGatewayBillingFloat(values map[string]interface{}, key string) float64 {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint64:
		return float64(typed)
	case uint32:
		return float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func modelGatewayBillingInt(values map[string]interface{}, key string) int {
	value := modelGatewayBillingFloat(values, key)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return int(math.Round(value))
}

func modelGatewayBillingString(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(common.Interface2String(value))
}

func modelGatewayBillingBool(values map[string]interface{}, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	}
	return false
}

func newModelGatewayUserRequestAccumulator(key string) *modelGatewayUserRequestAccumulator {
	return &modelGatewayUserRequestAccumulator{
		ModelGatewayUserRequestAggregate: ModelGatewayUserRequestAggregate{
			Key: key,
		},
	}
}

func modelGatewayUserRequestAccumulatorFor(accumulators map[string]*modelGatewayUserRequestAccumulator, key string) *modelGatewayUserRequestAccumulator {
	if key == "" {
		key = "unknown"
	}
	if existing, ok := accumulators[key]; ok {
		return existing
	}
	accumulator := newModelGatewayUserRequestAccumulator(key)
	accumulators[key] = accumulator
	return accumulator
}

func modelGatewayUserRequestTrendAccumulatorFor(accumulators map[int64]*modelGatewayUserRequestTrendAccumulator, bucketStart int64) *modelGatewayUserRequestTrendAccumulator {
	if existing, ok := accumulators[bucketStart]; ok {
		return existing
	}
	accumulator := &modelGatewayUserRequestTrendAccumulator{
		modelGatewayUserRequestAccumulator: *newModelGatewayUserRequestAccumulator(strconv.FormatInt(bucketStart, 10)),
	}
	accumulators[bucketStart] = accumulator
	return accumulator
}

func applyModelGatewayUserRequestAccumulator(accumulator *modelGatewayUserRequestAccumulator, userRequest model.ModelGatewayUserRequestSummary, isHealthProbe bool) {
	if accumulator == nil {
		return
	}
	accumulator.Requests++
	if isHealthProbe {
		accumulator.HealthProbes++
	} else if userRequest.Recovered {
		accumulator.Recovered++
	}
	clientAborted := modelGatewayUserRequestClientAborted(userRequest)
	userQuotaExhausted := modelGatewayUserRequestUserQuotaExhausted(userRequest)
	if userQuotaExhausted && !isHealthProbe {
		accumulator.userQuotaExhausted++
		accumulator.UserQuotaExhausted++
	}
	if clientAborted {
		if !isHealthProbe {
			accumulator.ClientAborted++
		}
	} else if userRequest.FinalSuccess && !isHealthProbe {
		accumulator.Successes++
	} else if !isHealthProbe && !userQuotaExhausted {
		accumulator.FinalFailures++
	}
	if userRequest.EmptyOutput && !clientAborted && !userQuotaExhausted && !isHealthProbe {
		accumulator.EmptyOutputs++
	}
	if strings.TrimSpace(userRequest.ExperienceIssue) != "" && !clientAborted && !userQuotaExhausted && !isHealthProbe {
		accumulator.ExperienceIssues++
	}
	if userRequest.DurationMs > 0 && !clientAborted && !userQuotaExhausted {
		accumulator.durationSum += userRequest.DurationMs
		accumulator.durationSamples++
		accumulator.durationValues = append(accumulator.durationValues, userRequest.DurationMs)
	}
	if userRequest.TTFTMs > 0 && !clientAborted && !userQuotaExhausted {
		accumulator.ttftSum += userRequest.TTFTMs
		accumulator.ttftSamples++
		accumulator.ttftValues = append(accumulator.ttftValues, userRequest.TTFTMs)
	}
	if userRequest.CompletedAt > accumulator.LastRequestAt {
		accumulator.LastRequestAt = userRequest.CompletedAt
	}
}

func modelGatewayUserRequestSummaryFromAccumulator(summary ModelGatewayUserRequestSummary, accumulator *modelGatewayUserRequestAccumulator) ModelGatewayUserRequestSummary {
	if accumulator == nil {
		return summary
	}
	userRequests := accumulator.Requests - accumulator.HealthProbes
	if userRequests < 0 {
		userRequests = 0
	}
	userCompletedRequests := userRequests - accumulator.ClientAborted - accumulator.userQuotaExhausted
	if userCompletedRequests < 0 {
		userCompletedRequests = 0
	}
	summary.Successes = accumulator.Successes
	summary.FinalFailures = accumulator.FinalFailures
	summary.UserQuotaExhausted = accumulator.userQuotaExhausted
	summary.ClientAborted = accumulator.ClientAborted
	summary.HealthProbes = accumulator.HealthProbes
	summary.UserRequests = userRequests
	summary.Recovered = accumulator.Recovered
	summary.EmptyOutputs = accumulator.EmptyOutputs
	summary.ExperienceIssues = accumulator.ExperienceIssues
	summary.UserSuccessRate = successRateModelGatewayObservability(accumulator.Successes, userCompletedRequests)
	summary.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
	summary.P95DurationMs = percentileModelGatewayObservabilityInt64(accumulator.durationValues, 0.95)
	summary.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
	summary.P95TTFTMs = percentileModelGatewayObservabilityInt64(accumulator.ttftValues, 0.95)
	return summary
}

func finalizeModelGatewayUserRequestAggregates(accumulators map[string]*modelGatewayUserRequestAccumulator, topN int) []ModelGatewayUserRequestAggregate {
	items := make([]ModelGatewayUserRequestAggregate, 0, len(accumulators))
	for _, accumulator := range accumulators {
		item := accumulator.ModelGatewayUserRequestAggregate
		item.UserRequests = item.Requests - item.HealthProbes
		if item.UserRequests < 0 {
			item.UserRequests = 0
		}
		userCompletedRequests := item.UserRequests - item.ClientAborted - item.UserQuotaExhausted
		if userCompletedRequests < 0 {
			userCompletedRequests = 0
		}
		item.UserSuccessRate = successRateModelGatewayObservability(item.Successes, userCompletedRequests)
		item.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
		item.P95DurationMs = percentileModelGatewayObservabilityInt64(accumulator.durationValues, 0.95)
		item.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
		item.P95TTFTMs = percentileModelGatewayObservabilityInt64(accumulator.ttftValues, 0.95)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UserSuccessRate != items[j].UserSuccessRate {
			return items[i].UserSuccessRate > items[j].UserSuccessRate
		}
		if items[i].Requests != items[j].Requests {
			return items[i].Requests > items[j].Requests
		}
		if items[i].FinalFailures != items[j].FinalFailures {
			return items[i].FinalFailures < items[j].FinalFailures
		}
		return items[i].Key < items[j].Key
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func finalizeModelGatewayUserRequestTrends(accumulators map[int64]*modelGatewayUserRequestTrendAccumulator, startTime int64, endTime int64, bucketSeconds int64) []ModelGatewayUserRequestTrendPoint {
	if startTime <= 0 || endTime <= startTime || bucketSeconds <= 0 {
		return []ModelGatewayUserRequestTrendPoint{}
	}
	bucketCount := int((endTime - startTime + bucketSeconds - 1) / bucketSeconds)
	if bucketCount <= 0 {
		return []ModelGatewayUserRequestTrendPoint{}
	}
	items := make([]ModelGatewayUserRequestTrendPoint, 0, bucketCount)
	for idx := 0; idx < bucketCount; idx++ {
		bucketStart := startTime + int64(idx)*bucketSeconds
		bucketEnd := bucketStart + bucketSeconds
		if bucketEnd > endTime {
			bucketEnd = endTime
		}
		items = append(items, modelGatewayUserRequestTrendPointFromAccumulator(bucketStart, bucketEnd, accumulators[bucketStart]))
	}
	return items
}

func modelGatewayUserRequestTrendPointFromAccumulator(bucketStart int64, bucketEnd int64, accumulator *modelGatewayUserRequestTrendAccumulator) ModelGatewayUserRequestTrendPoint {
	point := ModelGatewayUserRequestTrendPoint{
		BucketStart: bucketStart,
		BucketEnd:   bucketEnd,
	}
	if accumulator == nil {
		return point
	}
	point.Requests = accumulator.Requests
	point.UserRequests = accumulator.Requests - accumulator.HealthProbes
	if point.UserRequests < 0 {
		point.UserRequests = 0
	}
	point.Successes = accumulator.Successes
	point.FinalFailures = accumulator.FinalFailures
	point.UserQuotaExhausted = accumulator.UserQuotaExhausted
	point.ClientAborted = accumulator.ClientAborted
	point.HealthProbes = accumulator.HealthProbes
	point.Recovered = accumulator.Recovered
	point.EmptyOutputs = accumulator.EmptyOutputs
	point.ExperienceIssues = accumulator.ExperienceIssues
	userCompletedRequests := point.UserRequests - point.ClientAborted - point.UserQuotaExhausted
	if userCompletedRequests < 0 {
		userCompletedRequests = 0
	}
	point.UserSuccessRate = successRateModelGatewayObservability(accumulator.Successes, userCompletedRequests)
	point.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
	point.P95DurationMs = percentileModelGatewayObservabilityInt64(accumulator.durationValues, 0.95)
	point.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
	point.P95TTFTMs = percentileModelGatewayObservabilityInt64(accumulator.ttftValues, 0.95)
	return point
}

func ModelGatewayUserRequestRecordFromSummary(userRequest model.ModelGatewayUserRequestSummary) ModelGatewayUserRequestRecord {
	clientAborted := modelGatewayUserRequestClientAborted(userRequest)
	actualGroup := modelGatewayUserRequestActualGroup(userRequest.RequestedGroup, userRequest.SelectedGroup)
	actualGroupRatio := 0.0
	if userRequest.CompletedAt > 0 && actualGroup != "" {
		actualGroupRatio = ratio_setting.GetGroupRatio(actualGroup)
	}
	return ModelGatewayUserRequestRecord{
		ID:                 userRequest.Id,
		CreatedAt:          userRequest.CreatedAt,
		UpdatedAt:          modelGatewayUserRequestUpdatedAt(userRequest),
		CompletedAt:        userRequest.CompletedAt,
		RequestID:          userRequest.RequestId,
		RequestedModel:     userRequest.RequestedModel,
		RequestedGroup:     userRequest.RequestedGroup,
		SelectedGroup:      userRequest.SelectedGroup,
		ActualGroup:        actualGroup,
		ActualGroupRatio:   actualGroupRatio,
		FinalChannelID:     userRequest.FinalChannelID,
		FinalChannelName:   userRequest.FinalChannelName,
		Attempts:           userRequest.Attempts,
		FinalSuccess:       userRequest.FinalSuccess,
		Recovered:          userRequest.Recovered,
		FinalStatusCode:    userRequest.FinalStatusCode,
		FinalErrorCategory: userRequest.FinalErrorCategory,
		WarningLevel:       "",
		WarningFlags:       nil,
		WarningMessage:     "",
		EmptyOutput:        userRequest.EmptyOutput,
		ExperienceIssue:    userRequest.ExperienceIssue,
		ClientAborted:      clientAborted,
		IsHealthProbe:      userRequest.IsHealthProbe,
		ProbeReason:        strings.TrimSpace(userRequest.ProbeReason),
		DurationMs:         userRequest.DurationMs,
		TTFTMs:             userRequest.TTFTMs,
		Status:             modelGatewayUserRequestStatus(userRequest.FinalSuccess, clientAborted, userRequest.IsHealthProbe),
	}
}

func modelGatewayUserRequestActualGroup(requestedGroup string, selectedGroup string) string {
	if group := strings.TrimSpace(selectedGroup); group != "" {
		return group
	}
	if group := strings.TrimSpace(requestedGroup); group != "" {
		return group
	}
	return ""
}

func modelGatewayUserRequestRecord(userRequest model.ModelGatewayUserRequestSummary) ModelGatewayUserRequestRecord {
	return ModelGatewayUserRequestRecordFromSummary(userRequest)
}

func modelGatewayUserRequestUpdatedAt(userRequest model.ModelGatewayUserRequestSummary) int64 {
	if userRequest.UpdatedAt > 0 {
		return userRequest.UpdatedAt
	}
	if userRequest.CompletedAt > 0 {
		return userRequest.CompletedAt
	}
	return userRequest.CreatedAt
}

func modelGatewayUserRequestStatus(success bool, clientAborted bool, healthProbe bool) string {
	if clientAborted {
		return "client_aborted"
	}
	if healthProbe {
		if success {
			return "health_probe"
		}
		return "health_probe_failed"
	}
	if success {
		return "success"
	}
	return "failed"
}

func modelGatewayUserRequestClientAborted(userRequest model.ModelGatewayUserRequestSummary) bool {
	category := strings.ToLower(strings.TrimSpace(userRequest.FinalErrorCategory))
	return userRequest.ClientAborted ||
		userRequest.FinalStatusCode == relayStatusClientClosedRequest ||
		category == model.ModelGatewayUserRequestErrorClientAborted ||
		strings.Contains(category, "client_abort") ||
		strings.Contains(category, "client_gone")
}

func modelGatewayUserRequestUserQuotaExhausted(userRequest model.ModelGatewayUserRequestSummary) bool {
	category := strings.ToLower(strings.TrimSpace(userRequest.FinalErrorCategory))
	return category == model.ModelGatewayUserRequestErrorUserQuotaExhausted ||
		strings.Contains(category, "user_quota")
}

func modelGatewayUserRequestModelKey(userRequest model.ModelGatewayUserRequestSummary) string {
	if userRequest.RequestedModel != "" {
		return userRequest.RequestedModel
	}
	return "unknown"
}

func modelGatewayUserRequestGroupKey(userRequest model.ModelGatewayUserRequestSummary) string {
	if userRequest.SelectedGroup != "" {
		return userRequest.SelectedGroup
	}
	if userRequest.RequestedGroup != "" {
		return userRequest.RequestedGroup
	}
	return "unknown"
}

func buildModelGatewayObservabilityFromRecords(records []model.ModelExecutionRecord, totalRecords int64, startTime int64, endTime int64, options ModelGatewayObservabilityOptions) ModelGatewayObservabilityResponse {
	response := ModelGatewayObservabilityResponse{
		Summary: ModelGatewayObservabilitySummary{
			WindowHours:        options.Hours,
			TrendBucketSeconds: options.TrendBucketSeconds,
			StartTime:          startTime,
			EndTime:            endTime,
			TotalRecords:       totalRecords,
			ScannedRecords:     len(records),
			Truncated:          totalRecords > int64(len(records)),
		},
		ByModel:           make([]ModelGatewayObservabilityAggregate, 0),
		ByGroup:           make([]ModelGatewayObservabilityAggregate, 0),
		ByChannel:         make([]ModelGatewayObservabilityAggregate, 0),
		ByProviderProfile: make([]ModelGatewayObservabilityAggregate, 0),
		ByProxyMode:       make([]ModelGatewayObservabilityAggregate, 0),
		Trends:            make([]ModelGatewayObservabilityTrendPoint, 0),
		RiskTimeline:      make([]ModelGatewayRiskEvent, 0),
		RecentRecords:     make([]ModelGatewayObservabilityRecord, 0, minModelGatewayObservabilityInt(options.RecentLimit, len(records))),
		ScoreBreakdown: ModelGatewayObservabilityScoreBreakdown{
			Average: make(map[string]float64),
		},
	}

	modelAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	groupAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	channelAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	providerProfileAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	proxyModeAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	totalAccumulator := newModelGatewayObservabilityAccumulator("all", "", 0)
	trendBucketSeconds := options.TrendBucketSeconds
	trendAccumulators := make(map[int64]*modelGatewayObservabilityTrendAccumulator)
	dispatchMetaByRequestID := buildModelGatewayDispatchMetaIndex(records)

	for idx, record := range records {
		scoreBreakdown, scoreErr := parseModelGatewayScoreBreakdown(record.ScoreBreakdown)
		candidateGroups, candidateErr := parseModelGatewayCandidateGroups(record.CandidateGroups)
		requestMeta, requestMetaErr := parseModelGatewayRequestMeta(record.RequestMeta)
		applyModelGatewayObservabilityRecord(totalAccumulator, record, scoreBreakdown, requestMeta, scoreErr != nil)
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(modelAccumulators, modelGatewayObservabilityModelKey(record), "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(groupAccumulators, modelGatewayObservabilityGroupKey(record), "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)

		channelID, channelName := modelGatewayObservabilityChannel(record)
		channelKey := strconv.Itoa(channelID)
		if channelID == 0 {
			channelKey = "unknown"
		}
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(channelAccumulators, channelKey, channelName, channelID), record, scoreBreakdown, requestMeta, scoreErr != nil)

		profileProxyMeta := modelGatewayProfileProxyMetaForRecord(record, requestMeta, dispatchMetaByRequestID)
		providerProfileKey := profileProxyMeta.ProviderProfile
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(providerProfileAccumulators, providerProfileKey, "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)
		proxyModeKey := profileProxyMeta.ProxyMode
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(proxyModeAccumulators, proxyModeKey, "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)
		if bucketStart, ok := modelGatewayObservabilityTrendBucketStart(record.CreatedAt, startTime, endTime, trendBucketSeconds); ok {
			applyModelGatewayObservabilityTrendRecord(modelGatewayObservabilityTrendAccumulatorFor(trendAccumulators, bucketStart), record, scoreBreakdown, requestMeta, scoreErr != nil, profileProxyMeta)
		}

		if idx < options.RecentLimit {
			recent := modelGatewayObservabilityRecentRecord(record, scoreBreakdown, candidateGroups, requestMeta)
			enrichModelGatewayRecentTimingMeta(&recent)
			recent.ScoreBreakdownError = scoreErr != nil
			recent.CandidateGroupsError = candidateErr != nil
			recent.RequestMetaError = requestMetaErr != nil
			response.RecentRecords = append(response.RecentRecords, recent)
		}
	}

	response.Summary = modelGatewayObservabilitySummaryFromAccumulator(response.Summary, totalAccumulator)
	scoreBreakdownAverage := averageModelGatewayObservabilityScoreBreakdown(totalAccumulator)
	if scoreBreakdownAverage == nil {
		scoreBreakdownAverage = map[string]float64{}
	}
	response.ScoreBreakdown = ModelGatewayObservabilityScoreBreakdown{
		Samples:     totalAccumulator.ScoreBreakdownSamples,
		Average:     scoreBreakdownAverage,
		ParseErrors: totalAccumulator.ScoreBreakdownParseErrs,
	}
	response.ByModel = finalizeModelGatewayObservabilityAggregates(modelAccumulators, options.TopN)
	response.ByGroup = finalizeModelGatewayObservabilityAggregates(groupAccumulators, options.TopN)
	response.ByChannel = finalizeModelGatewayObservabilityAggregates(channelAccumulators, options.TopN)
	response.ByProviderProfile = finalizeModelGatewayObservabilityAggregates(providerProfileAccumulators, options.TopN)
	response.ByProxyMode = finalizeModelGatewayObservabilityAggregates(proxyModeAccumulators, options.TopN)
	response.Trends = finalizeModelGatewayObservabilityTrends(trendAccumulators, startTime, endTime, trendBucketSeconds)
	response.Summary.CircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(modelGatewayCircuitOpenReasonCountsFromTrends(response.Trends), modelGatewayObservabilityTrendTopN)
	response.Summary.CircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(modelGatewayCircuitErrorTypeCountsFromTrends(response.Trends), modelGatewayObservabilityTrendTopN)
	response.Summary.CircuitErrorCounts = response.Summary.CircuitErrorTypes
	applyModelGatewayCurrentChannelStatus(&response)
	applyModelGatewayTrendRiskTimeline(&response)
	return response
}

func modelGatewayObservabilitySummaryFromAccumulator(summary ModelGatewayObservabilitySummary, accumulator *modelGatewayObservabilityAccumulator) ModelGatewayObservabilitySummary {
	summary.Dispatches = accumulator.Dispatches
	summary.Attempts = accumulator.Attempts
	summary.Successes = accumulator.Successes
	summary.Failures = accumulator.Failures
	summary.StreamInterrupted = accumulator.StreamInterrupted
	summary.FallbackUsed = accumulator.FallbackUsed
	summary.SuccessRate = successRateModelGatewayObservability(accumulator.Successes, accumulator.Attempts)
	summary.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
	summary.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
	summary.AvgScoreTotal = averageFloat64(accumulator.scoreTotalSum, accumulator.scoreTotalSamples)
	summary.QueueEnabledDispatches = accumulator.QueueEnabledDispatches
	summary.QueuedDispatches = accumulator.QueuedDispatches
	summary.QueueWaitCount = accumulator.QueueWaitCount
	summary.AvgQueueWaitMs = averageInt64(accumulator.queueWaitSum, accumulator.queueWaitSamples)
	summary.ResourceProtectionDispatches = accumulator.ResourceProtectionDispatches
	summary.ResourceProtectionPrimaryHits = accumulator.ResourceProtectionPrimaryHits
	summary.ResourceProtectionPrimaryWaits = accumulator.ResourceProtectionPrimaryWaits
	summary.ResourceProtectionFallbacks = accumulator.ResourceProtectionFallbacks
	summary.ResourceProtectionWaitTimeoutFallbacks = accumulator.ResourceProtectionWaitTimeoutFallbacks
	summary.ResourceProtectionPrimaryFailureFallbacks = accumulator.ResourceProtectionPrimaryFailureFallbacks
	summary.ResourceProtectionQueueDepth = accumulator.ResourceProtectionQueueDepth
	summary.ResourceProtectionMaxQueueDepth = accumulator.ResourceProtectionMaxQueueDepth
	summary.ResourceProtectionQueueCapacity = accumulator.ResourceProtectionQueueCapacity
	summary.ResourceProtectionAvgWaitMs = averageInt64(accumulator.resourceProtectionWaitSum, accumulator.resourceProtectionWaitSamples)
	summary.ResourceProtectionFallbackCostShare = modelGatewayResourceProtectionFallbackCostShare(accumulator)
	summary.StickyRoutes = accumulator.StickyRoutes
	summary.StickyRetained = accumulator.StickyRetained
	summary.StickyBroken = accumulator.StickyBroken
	summary.CacheAffinityRoutes = accumulator.CacheAffinityRoutes
	summary.OverloadSkipCount = accumulator.OverloadSkipCount
	summary.AuthConfigErrorCount = accumulator.AuthConfigErrorCount
	summary.UnknownErrorCount = accumulator.UnknownErrorCount
	summary.ConfigErrorIsolatedCount = accumulator.ConfigErrorIsolatedCount
	summary.ScoreBreakdownSamples = accumulator.ScoreBreakdownSamples
	summary.ScoreBreakdownParseErrs = accumulator.ScoreBreakdownParseErrs
	return summary
}

func modelGatewayObservabilityRecentRecord(record model.ModelExecutionRecord, scoreBreakdown map[string]float64, candidateGroups []string, requestMeta map[string]any) ModelGatewayObservabilityRecord {
	meta := modelGatewayObservabilityMetaFromRequestMeta(requestMeta)
	attemptMeta := modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta)
	candidateExplanations := modelGatewayCandidateExplanationsFromRequestMeta(requestMeta)
	return ModelGatewayObservabilityRecord{
		ID:                             record.Id,
		Kind:                           modelGatewayObservabilityRecordKind(record),
		CreatedAt:                      record.CreatedAt,
		RequestID:                      record.RequestId,
		AttemptIndex:                   record.AttemptIndex,
		RequestedGroup:                 record.RequestedGroup,
		SelectedGroup:                  record.SelectedGroup,
		ActualGroup:                    record.ActualGroup,
		RequestedModel:                 record.RequestedModel,
		ChannelID:                      record.ChannelId,
		ChannelName:                    record.ChannelName,
		ActualChannelID:                record.ActualChannelId,
		ActualChannelName:              record.ActualChannelName,
		EndpointType:                   record.EndpointType,
		PolicyMode:                     record.PolicyMode,
		AutoMode:                       record.AutoMode,
		Strategy:                       record.Strategy,
		Shadow:                         record.Shadow,
		SmartHandled:                   record.SmartHandled,
		FallbackUsed:                   record.FallbackUsed,
		Success:                        record.Success,
		StatusCode:                     record.StatusCode,
		ErrorCode:                      record.ErrorCode,
		ErrorType:                      record.ErrorType,
		DurationMs:                     record.DurationMs,
		TTFTMs:                         record.TTFTMs,
		StreamInterrupted:              record.StreamInterrupted,
		ScoreTotal:                     roundModelGatewayObservabilityFloat(record.ScoreTotal),
		ScoreBreakdown:                 scoreBreakdown,
		CandidateGroups:                candidateGroups,
		CandidateExplanations:          candidateExplanations,
		SelectedReason:                 record.SelectedReason,
		QueueEnabled:                   meta.QueueEnabled,
		QueueWaitMs:                    meta.QueueWaitMs,
		QueueDepth:                     meta.QueueDepth,
		QueueCapacity:                  meta.QueueCapacity,
		ResourceProtectionEnabled:      meta.ResourceProtectionEnabled,
		ResourceProtectionPhase:        meta.ResourceProtectionPhase,
		ResourceProtectionReason:       meta.ResourceProtectionReason,
		ResourceProtectionRole:         meta.ResourceProtectionRole,
		PrimaryChannelIDs:              append([]int(nil), meta.PrimaryChannelIDs...),
		FallbackChannelIDs:             append([]int(nil), meta.FallbackChannelIDs...),
		PrimaryWaitTimeoutMs:           meta.PrimaryWaitTimeoutMs,
		PrimaryQueueMaxDepth:           meta.PrimaryQueueMaxDepth,
		StickySource:                   meta.StickySource,
		StickyRetained:                 meta.StickyRetained,
		StickyBreak:                    meta.StickyBreak,
		StickyDecision:                 meta.StickyDecision,
		CacheAffinity:                  meta.CacheAffinity,
		ErrorMessage:                   attemptMeta.ErrorMessage,
		ErrorCategory:                  attemptMeta.ErrorCategory,
		WarningLevel:                   attemptMeta.WarningLevel,
		WarningFlags:                   append([]string(nil), attemptMeta.WarningFlags...),
		WarningMessage:                 attemptMeta.WarningMessage,
		ChannelInducedClientAbort:      attemptMeta.ChannelInducedClientAbort,
		RetryAction:                    attemptMeta.RetryAction,
		RetryReason:                    attemptMeta.RetryReason,
		WillRetry:                      attemptMeta.WillRetry,
		ClientAborted:                  attemptMeta.ClientAborted,
		ConcurrencyLimited:             attemptMeta.ConcurrencyLimited,
		BalanceInsufficient:            attemptMeta.BalanceInsufficient,
		EmptyOutput:                    attemptMeta.EmptyOutput,
		ExperienceIssue:                attemptMeta.ExperienceIssue,
		ActiveConcurrency:              attemptMeta.ActiveConcurrency,
		ConfiguredConcurrencyLimit:     attemptMeta.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:        attemptMeta.LearnedConcurrencyLimit,
		LearnedConcurrencyLimitChanged: attemptMeta.LearnedConcurrencyLimitChanged,
		UsedChannels:                   attemptMeta.UsedChannels,
		IsHealthProbe:                  attemptMeta.IsHealthProbe,
		ProbeReason:                    attemptMeta.ProbeReason,
		RequestMeta:                    requestMeta,
	}
}

func enrichModelGatewayRecentTimingMeta(record *ModelGatewayObservabilityRecord) {
	if record == nil || len(record.RequestMeta) == 0 {
		return
	}
	timing := modelGatewayObservabilityTimingMetaFromRequestMeta(record.RequestMeta)
	if timing.RequestBodyBytes <= 0 &&
		timing.UpstreamResponseHeaderMs <= 0 &&
		timing.UpstreamFirstEventWaitMs <= 0 &&
		timing.RequestBodyPrepareMs <= 0 {
		return
	}
	for idx := range record.CandidateExplanations {
		candidate := &record.CandidateExplanations[idx]
		if !candidate.Selected {
			continue
		}
		candidate.RequestBodyBytes = timing.RequestBodyBytes
		candidate.RequestBodyStorage = timing.RequestBodyStorage
		candidate.RequestBodySizeLikelyLatency = timing.RequestBodySizeLikelyLatency
		candidate.RequestBodyPrepareMs = timing.RequestBodyPrepareMs
		candidate.UpstreamResponseHeaderMs = timing.UpstreamResponseHeaderMs
		candidate.UpstreamFirstEventWaitMs = timing.UpstreamFirstEventWaitMs
		return
	}
}

func ModelGatewayObservabilityRecordFromModelRecord(record model.ModelExecutionRecord) ModelGatewayObservabilityRecord {
	scoreBreakdown, scoreErr := parseModelGatewayScoreBreakdown(record.ScoreBreakdown)
	candidateGroups, candidateErr := parseModelGatewayCandidateGroups(record.CandidateGroups)
	requestMeta, requestMetaErr := parseModelGatewayRequestMeta(record.RequestMeta)
	recent := modelGatewayObservabilityRecentRecord(record, scoreBreakdown, candidateGroups, requestMeta)
	enrichModelGatewayRecentTimingMeta(&recent)
	recent.ScoreBreakdownError = scoreErr != nil
	recent.CandidateGroupsError = candidateErr != nil
	recent.RequestMetaError = requestMetaErr != nil
	return recent
}

func applyModelGatewayObservabilityRecord(accumulator *modelGatewayObservabilityAccumulator, record model.ModelExecutionRecord, scoreBreakdown map[string]float64, requestMeta map[string]any, scoreErr bool) {
	accumulator.Records++
	if record.CreatedAt > accumulator.LastRecordAt {
		accumulator.LastRecordAt = record.CreatedAt
	}
	if isModelGatewayDispatchRecord(record) {
		accumulator.Dispatches++
		applyModelGatewayObservabilityDispatchMeta(accumulator, record, requestMeta)
	}
	if record.FallbackUsed {
		accumulator.FallbackUsed++
	}
	if isModelGatewayAttemptRecord(record) {
		attemptMeta := modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta)
		if !attemptMeta.ClientAborted {
			accumulator.Attempts++
			if record.Success {
				accumulator.Successes++
			} else {
				accumulator.Failures++
				applyModelGatewayErrorCategoryCounts(accumulator, record, attemptMeta)
			}
			if record.StreamInterrupted {
				accumulator.StreamInterrupted++
			}
			if record.DurationMs > 0 {
				accumulator.durationSum += record.DurationMs
				accumulator.durationSamples++
			}
			if record.TTFTMs > 0 {
				accumulator.ttftSum += record.TTFTMs
				accumulator.ttftSamples++
			}
			applyModelGatewayResourceProtectionAttemptMeta(accumulator, requestMeta, attemptMeta)
		}
	}
	if hasModelGatewayScoreTotalSample(record) {
		accumulator.scoreTotalSum += record.ScoreTotal
		accumulator.scoreTotalSamples++
	}
	for _, candidate := range modelGatewayCandidateExplanationsFromRequestMeta(requestMeta) {
		if !candidate.ConfigErrorIsolated {
			continue
		}
		key := modelGatewayConfigIsolationCandidateKey(candidate)
		if key == "" {
			continue
		}
		if accumulator.configIsolationKeys == nil {
			accumulator.configIsolationKeys = make(map[string]struct{})
		}
		if _, exists := accumulator.configIsolationKeys[key]; exists {
			continue
		}
		accumulator.configIsolationKeys[key] = struct{}{}
		accumulator.ConfigErrorIsolatedCount++
	}
	if scoreErr {
		accumulator.ScoreBreakdownParseErrs++
		return
	}
	if len(scoreBreakdown) == 0 {
		return
	}
	if accumulator.scoreSums == nil {
		accumulator.scoreSums = make(map[string]float64, len(scoreBreakdown))
	}
	for key, value := range scoreBreakdown {
		accumulator.scoreSums[key] += value
	}
	accumulator.ScoreBreakdownSamples++
}

func applyModelGatewayErrorCategoryCounts(accumulator *modelGatewayObservabilityAccumulator, record model.ModelExecutionRecord, attemptMeta modelGatewayObservabilityAttemptMeta) {
	if accumulator == nil {
		return
	}
	category := strings.TrimSpace(record.ErrorCategory)
	if category == "" {
		category = strings.TrimSpace(attemptMeta.ErrorCategory)
	}
	if category == "" {
		category = modelgatewaycore.ErrorCategoryUnknown
	}
	switch category {
	case modelgatewaycore.ErrorCategoryOverloadSkip:
		accumulator.OverloadSkipCount++
	case modelgatewaycore.ErrorCategoryAuthConfigError:
		accumulator.AuthConfigErrorCount++
	case modelgatewaycore.ErrorCategoryUnknown:
		accumulator.UnknownErrorCount++
	}
}

func modelGatewayConfigIsolationCandidateKey(candidate ModelGatewayCandidateExplanation) string {
	if !candidate.ConfigErrorIsolated {
		return ""
	}
	key := candidate.RuntimeKey
	if key.ChannelID <= 0 {
		key.ChannelID = candidate.ChannelID
	}
	if key.ResourceID == "" {
		key.ResourceID = candidate.ResourceID
	}
	if key.ResourceType == "" {
		key.ResourceType = candidate.ResourceType
	}
	if key.AccountID == "" {
		key.AccountID = candidate.AccountID
	}
	if key.AccountType == "" {
		key.AccountType = candidate.AccountType
	}
	if key.Brand == "" {
		key.Brand = candidate.Brand
	}
	if key.Provider == "" {
		key.Provider = candidate.Provider
	}
	if key.CredentialIndex == 0 {
		key.CredentialIndex = candidate.CredentialIndex
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = candidate.CredentialSubjectFP
	}
	if key.CredentialFP == "" {
		key.CredentialFP = candidate.CredentialFP
	}
	if key.Group == "" {
		key.Group = candidate.Group
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = candidate.UpstreamModel
	}
	if key.ChannelID <= 0 &&
		key.RequestedModel == "" &&
		key.UpstreamModel == "" &&
		key.ResourceID == "" &&
		key.AccountID == "" &&
		key.CredentialSubjectFP == "" &&
		key.CredentialFP == "" &&
		key.Group == "" &&
		key.EndpointType == "" &&
		key.CapabilityFingerprint == "" {
		return ""
	}
	return fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%d|%s|%s|%s",
		key.ChannelID,
		key.RequestedModel,
		key.UpstreamModel,
		key.ResourceID,
		key.AccountID,
		key.CredentialSubjectFP,
		key.CredentialFP,
		key.CredentialIndex,
		key.Group,
		key.EndpointType,
		key.CapabilityFingerprint,
	)
}

type modelGatewayObservabilityDispatchMeta struct {
	QueueEnabled              bool
	QueueWaitMs               int64
	QueueWaitSet              bool
	QueueDepth                int
	QueueCapacity             int
	ResourceProtectionEnabled bool
	ResourceProtectionPhase   string
	ResourceProtectionReason  string
	ResourceProtectionRole    string
	PrimaryChannelIDs         []int
	FallbackChannelIDs        []int
	PrimaryWaitTimeoutMs      int
	PrimaryQueueMaxDepth      int
	StickySource              string
	StickyRetained            bool
	StickyBreak               string
	StickyDecision            *modelgatewaycore.StickyDecision
	CacheAffinity             bool
}

type modelGatewayObservabilityAttemptMeta struct {
	ErrorMessage                   string
	ErrorCategory                  string
	WarningLevel                   string
	WarningFlags                   []string
	WarningMessage                 string
	ChannelInducedClientAbort      bool
	RetryAction                    string
	RetryReason                    string
	WillRetry                      bool
	ClientAborted                  bool
	ConcurrencyLimited             bool
	BalanceInsufficient            bool
	EmptyOutput                    bool
	ExperienceIssue                string
	ActiveConcurrency              int
	ConfiguredConcurrencyLimit     int
	LearnedConcurrencyLimit        int
	LearnedConcurrencyLimitChanged bool
	UsedChannels                   []string
	IsHealthProbe                  bool
	ProbeReason                    string
}

type modelGatewayObservabilityTimingMeta struct {
	QueueWaitMs                  int64
	RelayToFirstByteMs           int64
	RelayTotalMs                 int64
	UpstreamResponseHeaderMs     int64
	UpstreamFirstEventWaitMs     int64
	PreFirstByteMs               int64
	PostFirstByteMs              int64
	RequestBodyPrepareMs         int64
	RequestBodyBytes             int64
	RequestBodyStorage           string
	RequestBodySizeLikelyLatency bool
}

type modelGatewayProfileProxyMeta struct {
	ProviderProfile string
	ProxyMode       string
}

func buildModelGatewayDispatchMetaIndex(records []model.ModelExecutionRecord) map[string]modelGatewayProfileProxyMeta {
	index := make(map[string]modelGatewayProfileProxyMeta)
	for _, record := range records {
		if strings.TrimSpace(record.RequestId) == "" || !isModelGatewayDispatchRecord(record) {
			continue
		}
		requestMeta, err := parseModelGatewayRequestMeta(record.RequestMeta)
		if err != nil {
			continue
		}
		meta := modelGatewayProfileProxyMeta{
			ProviderProfile: modelGatewayObservabilityProviderProfileKey(requestMeta),
			ProxyMode:       modelGatewayObservabilityProxyModeKey(requestMeta),
		}
		if meta.ProviderProfile == "unknown" && meta.ProxyMode == "unknown" {
			continue
		}
		index[record.RequestId] = meta
	}
	return index
}

func applyModelGatewayObservabilityDispatchMeta(accumulator *modelGatewayObservabilityAccumulator, record model.ModelExecutionRecord, requestMeta map[string]any) {
	if accumulator == nil {
		return
	}
	meta := modelGatewayObservabilityMetaFromRequestMeta(requestMeta)
	if meta.QueueEnabled {
		accumulator.QueueEnabledDispatches++
	}
	if meta.QueueWaitMs > 0 {
		accumulator.QueuedDispatches++
	}
	if meta.QueueWaitSet {
		accumulator.QueueWaitCount++
		accumulator.queueWaitSum += meta.QueueWaitMs
		accumulator.queueWaitSamples++
		accumulator.queueWaitValues = append(accumulator.queueWaitValues, meta.QueueWaitMs)
	}
	applyModelGatewayResourceProtectionDispatchMeta(accumulator, record, meta, requestMeta)
	if meta.StickySource != "" {
		accumulator.StickyRoutes++
	}
	if meta.StickyRetained {
		accumulator.StickyRetained++
	}
	if meta.StickyBreak != "" {
		accumulator.StickyBroken++
	}
	if meta.CacheAffinity {
		accumulator.CacheAffinityRoutes++
	}
}

func applyModelGatewayResourceProtectionDispatchMeta(accumulator *modelGatewayObservabilityAccumulator, record model.ModelExecutionRecord, meta modelGatewayObservabilityDispatchMeta, requestMeta map[string]any) {
	if accumulator == nil || !meta.ResourceProtectionEnabled {
		return
	}
	accumulator.ResourceProtectionDispatches++
	switch meta.ResourceProtectionPhase {
	case modelgatewaycore.ResourceProtectionPhasePrimaryHit:
		accumulator.ResourceProtectionPrimaryHits++
	case modelgatewaycore.ResourceProtectionPhasePrimarySaturatedWait:
		accumulator.ResourceProtectionPrimaryWaits++
	case modelgatewaycore.ResourceProtectionPhaseFallbackAfterTimeout:
		accumulator.ResourceProtectionFallbacks++
		accumulator.ResourceProtectionWaitTimeoutFallbacks++
	case modelgatewaycore.ResourceProtectionPhasePrimaryFailureFallback:
		accumulator.ResourceProtectionFallbacks++
		accumulator.ResourceProtectionPrimaryFailureFallbacks++
	case modelgatewaycore.ResourceProtectionPhaseNoPrimaryFallback:
		accumulator.ResourceProtectionFallbacks++
	default:
		if meta.ResourceProtectionRole == modelgatewaycore.ResourceProtectionRoleFallback {
			accumulator.ResourceProtectionFallbacks++
		}
	}
	if meta.ResourceProtectionPhase == modelgatewaycore.ResourceProtectionPhasePrimarySaturatedWait || meta.QueueDepth > 0 {
		accumulator.ResourceProtectionQueueDepth = meta.QueueDepth
		if meta.QueueDepth > accumulator.ResourceProtectionMaxQueueDepth {
			accumulator.ResourceProtectionMaxQueueDepth = meta.QueueDepth
		}
		if meta.QueueCapacity > 0 {
			accumulator.ResourceProtectionQueueCapacity = meta.QueueCapacity
		}
	}
	if costRatio := modelGatewaySelectedDispatchCostRatio(record, requestMeta); costRatio > 0 {
		accumulator.resourceProtectionCostSum += costRatio
		if meta.ResourceProtectionRole == modelgatewaycore.ResourceProtectionRoleFallback {
			accumulator.resourceProtectionFallbackCost += costRatio
		}
	}
}

func applyModelGatewayResourceProtectionAttemptMeta(accumulator *modelGatewayObservabilityAccumulator, requestMeta map[string]any, attemptMeta modelGatewayObservabilityAttemptMeta) {
	if accumulator == nil {
		return
	}
	meta := modelGatewayObservabilityMetaFromRequestMeta(requestMeta)
	if !meta.ResourceProtectionEnabled {
		return
	}
	timing := modelGatewayObservabilityTimingMetaFromRequestMeta(requestMeta)
	if timing.QueueWaitMs <= 0 {
		return
	}
	if meta.ResourceProtectionRole == modelgatewaycore.ResourceProtectionRolePrimary ||
		meta.ResourceProtectionPhase == modelgatewaycore.ResourceProtectionPhaseFallbackAfterTimeout ||
		attemptMeta.RetryAction == "resource_protection_fallback" {
		accumulator.resourceProtectionWaitSum += timing.QueueWaitMs
		accumulator.resourceProtectionWaitSamples++
	}
}

func modelGatewaySelectedDispatchCostRatio(record model.ModelExecutionRecord, requestMeta map[string]any) float64 {
	channelID := record.ActualChannelId
	if channelID <= 0 {
		channelID = record.ChannelId
	}
	candidates := modelGatewayCandidateExplanationsFromRequestMeta(requestMeta)
	if len(candidates) == 0 {
		return 0
	}
	if channelID > 0 {
		for _, candidate := range candidates {
			if candidate.ChannelID == channelID && candidate.CostRatio > 0 {
				return candidate.CostRatio
			}
		}
	}
	if len(candidates) == 1 && candidates[0].CostRatio > 0 {
		return candidates[0].CostRatio
	}
	return 0
}

func modelGatewayObservabilityMetaFromRequestMeta(requestMeta map[string]any) modelGatewayObservabilityDispatchMeta {
	if len(requestMeta) == 0 {
		return modelGatewayObservabilityDispatchMeta{}
	}
	queueWaitMs, queueWaitSet := modelGatewayObservabilityMetaNonNegativeInt64(requestMeta["queue_wait_ms"])
	return modelGatewayObservabilityDispatchMeta{
		QueueEnabled:              modelGatewayObservabilityMetaBool(requestMeta["queue_enabled"]),
		QueueWaitMs:               queueWaitMs,
		QueueWaitSet:              queueWaitSet,
		QueueDepth:                int(modelGatewayObservabilityMetaInt64(requestMeta["queue_depth"])),
		QueueCapacity:             int(modelGatewayObservabilityMetaInt64(requestMeta["queue_capacity"])),
		ResourceProtectionEnabled: modelGatewayObservabilityMetaBool(requestMeta["resource_protection_enabled"]),
		ResourceProtectionPhase:   strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["resource_protection_phase"])),
		ResourceProtectionReason:  strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["resource_protection_reason"])),
		ResourceProtectionRole:    strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["resource_protection_role"])),
		PrimaryChannelIDs:         modelGatewayObservabilityIntSlice(requestMeta["primary_channel_ids"]),
		FallbackChannelIDs:        modelGatewayObservabilityIntSlice(requestMeta["fallback_channel_ids"]),
		PrimaryWaitTimeoutMs:      int(modelGatewayObservabilityMetaInt64(requestMeta["primary_wait_timeout_ms"])),
		PrimaryQueueMaxDepth:      int(modelGatewayObservabilityMetaInt64(requestMeta["primary_queue_max_depth"])),
		StickySource:              strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["sticky_source"])),
		StickyRetained:            modelGatewayObservabilityMetaBool(requestMeta["sticky_retained"]),
		StickyBreak:               strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["sticky_break"])),
		StickyDecision:            modelGatewayStickyDecisionFromRequestMeta(requestMeta),
		CacheAffinity:             modelGatewayObservabilityMetaBool(requestMeta["cache_affinity"]),
	}
}

func modelGatewayStickyDecisionFromRequestMeta(requestMeta map[string]any) *modelgatewaycore.StickyDecision {
	if len(requestMeta) == 0 || requestMeta["sticky_decision"] == nil {
		return nil
	}
	bytes, err := common.Marshal(requestMeta["sticky_decision"])
	if err != nil {
		return nil
	}
	var decision modelgatewaycore.StickyDecision
	if err := common.Unmarshal(bytes, &decision); err != nil {
		return nil
	}
	if decision.Reason == "" && decision.Decision == "" {
		return nil
	}
	return &decision
}

func modelGatewayScoreItemsDelta(current []modelgatewaycore.ScoreItem, previous []modelgatewaycore.ScoreItem) []modelgatewaycore.ScoreAdjustmentItem {
	if len(current) == 0 && len(previous) == 0 {
		return nil
	}
	previousByKey := make(map[string]modelgatewaycore.ScoreItem, len(previous))
	for _, item := range previous {
		previousByKey[item.Key] = item
	}
	out := make([]modelgatewaycore.ScoreAdjustmentItem, 0, len(current))
	for _, item := range current {
		previousItem := previousByKey[item.Key]
		delta := roundModelGatewayObservabilityFloat(item.Score - previousItem.Score)
		weightedDelta := roundModelGatewayObservabilityFloat(item.WeightedScore - previousItem.WeightedScore)
		if delta == 0 && weightedDelta == 0 {
			continue
		}
		out = append(out, modelgatewaycore.ScoreAdjustmentItem{
			Key:             item.Key,
			Name:            item.Name,
			BeforeScore:     previousItem.Score,
			AfterScore:      item.Score,
			Delta:           delta,
			Weight:          item.Weight,
			WeightedDelta:   weightedDelta,
			BeforeRawValue:  previousItem.RawValue,
			AfterRawValue:   item.RawValue,
			BeforeRawNumber: previousItem.RawNumber,
			AfterRawNumber:  item.RawNumber,
			RawUnit:         item.RawUnit,
			Reason:          item.Reason,
		})
	}
	return out
}

type modelGatewayCurrentChannelStatus struct {
	Status              int
	StatusReason        string
	BalanceInsufficient bool
}

func applyModelGatewayCurrentChannelStatus(response *ModelGatewayObservabilityResponse) {
	if response == nil {
		return
	}
	channelIDs := make(map[int]struct{})
	for _, item := range response.ByChannel {
		if item.ChannelID > 0 {
			channelIDs[item.ChannelID] = struct{}{}
		}
	}
	for _, record := range response.RecentRecords {
		for _, candidate := range record.CandidateExplanations {
			id := modelGatewayCandidateChannelID(candidate)
			if id > 0 {
				channelIDs[id] = struct{}{}
			}
		}
	}
	if len(channelIDs) == 0 {
		return
	}
	statuses := modelGatewayCurrentChannelStatuses(channelIDs)
	if len(statuses) == 0 {
		return
	}
	for idx := range response.ByChannel {
		status, ok := statuses[response.ByChannel[idx].ChannelID]
		if !ok {
			continue
		}
		response.ByChannel[idx].ChannelStatus = status.Status
		response.ByChannel[idx].StatusReason = status.StatusReason
		response.ByChannel[idx].BalanceInsufficient = status.BalanceInsufficient
	}
	for recordIdx := range response.RecentRecords {
		for candidateIdx := range response.RecentRecords[recordIdx].CandidateExplanations {
			candidate := &response.RecentRecords[recordIdx].CandidateExplanations[candidateIdx]
			status, ok := statuses[modelGatewayCandidateChannelID(*candidate)]
			if !ok {
				continue
			}
			applyModelGatewayCandidateChannelStatus(candidate, status)
		}
	}
}

func modelGatewayCurrentChannelStatuses(channelIDs map[int]struct{}) map[int]modelGatewayCurrentChannelStatus {
	ids := make([]int, 0, len(channelIDs))
	for id := range channelIDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	channels, err := model.GetChannelsByIds(ids)
	if err != nil || len(channels) == 0 {
		return nil
	}
	statuses := make(map[int]modelGatewayCurrentChannelStatus, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		statuses[channel.Id] = modelGatewayCurrentChannelStatus{
			Status:              channel.Status,
			StatusReason:        service.ChannelStatusReason(channel),
			BalanceInsufficient: service.IsKnownBalanceInsufficientChannel(channel),
		}
	}
	return statuses
}

func modelGatewayCandidateChannelID(candidate ModelGatewayCandidateExplanation) int {
	if candidate.ChannelID > 0 {
		return candidate.ChannelID
	}
	return candidate.RuntimeKey.ChannelID
}

func modelGatewayScoreHistoryCandidateMatches(candidate ModelGatewayCandidateExplanation, options modelGatewayScoreHistoryOptions) bool {
	if modelGatewayCandidateChannelID(candidate) != options.ChannelID {
		return false
	}
	return modelGatewayRuntimeKeyMatchesFilter(candidate.RuntimeKey, options.RuntimeKey)
}

func applyModelGatewayScoreHistoryRuntimeCurrent(response *ModelGatewayScoreHistoryResponse, options modelGatewayScoreHistoryOptions) {
	if response == nil || options.ChannelID <= 0 {
		return
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	if runtimeDeps == nil || runtimeDeps.SnapshotStore == nil {
		return
	}
	query := modelgatewayobservability.RuntimeStatusQuery{
		Model:     options.RuntimeKey.RequestedModel,
		Group:     options.RuntimeKey.Group,
		ChannelID: options.ChannelID,
		Limit:     modelGatewayRuntimeStatusMaxLimit,
	}
	status := defaultModelGatewayRuntimeStatusService().Build(query)
	var best *modelgatewayobservability.RuntimeStatusItem
	for idx := range status.Items {
		item := status.Items[idx]
		if !modelGatewayRuntimeStatusItemMatchesScoreHistory(item, options.RuntimeKey) {
			continue
		}
		if best == nil || modelGatewayRuntimeStatusItemBetterForScoreHistory(item, *best, options.RuntimeKey) {
			best = &item
		}
	}
	if best == nil || best.ChannelID <= 0 || best.SampleCount <= 0 || best.ScoreTotal <= 0 {
		return
	}
	runtimeItem := modelGatewayScoreHistoryItemFromRuntime(*best, response.GeneratedAt)
	if response.ChannelName == "" {
		response.ChannelName = runtimeItem.ChannelName
	}
	if modelGatewayRuntimeKeyIsEmpty(response.RuntimeKey) || modelGatewayRuntimeKeyIsEmpty(options.RuntimeKey) {
		response.RuntimeKey = runtimeItem.RuntimeKey
	}
	response.Items = append([]ModelGatewayScoreHistoryItem{runtimeItem}, response.Items...)
	if len(response.Items) > response.Limit {
		response.Items = response.Items[:response.Limit]
		response.Truncated = true
	}
}

func modelGatewayRuntimeStatusItemMatchesScoreHistory(item modelgatewayobservability.RuntimeStatusItem, filter ModelGatewayRuntimeKey) bool {
	if filter.UpstreamModel != "" && item.UpstreamModel != filter.UpstreamModel {
		return false
	}
	if filter.EndpointType != "" && item.EndpointType != filter.EndpointType {
		return false
	}
	if filter.CapabilityFingerprint != "" && item.CapabilityFingerprint != filter.CapabilityFingerprint {
		return false
	}
	if filter.ResourceID != "" && item.ResourceID != filter.ResourceID {
		return false
	}
	if filter.ResourceType != "" && item.ResourceType != filter.ResourceType {
		return false
	}
	if filter.AccountID != "" && item.AccountID != filter.AccountID {
		return false
	}
	if filter.AccountType != "" && item.AccountType != filter.AccountType {
		return false
	}
	if filter.Brand != "" && item.Brand != filter.Brand {
		return false
	}
	if filter.Provider != "" && item.Provider != filter.Provider {
		return false
	}
	if filter.CredentialIndexSet && item.CredentialIndex != filter.CredentialIndex {
		return false
	}
	if filter.CredentialSubjectFP != "" && item.CredentialSubjectFP != filter.CredentialSubjectFP {
		return false
	}
	if filter.CredentialFP != "" && item.CredentialFP != filter.CredentialFP {
		return false
	}
	return true
}

func modelGatewayRuntimeStatusItemBetterForScoreHistory(left modelgatewayobservability.RuntimeStatusItem, right modelgatewayobservability.RuntimeStatusItem, filter ModelGatewayRuntimeKey) bool {
	leftExact := modelGatewayRuntimeStatusItemExactForScoreHistory(left, filter)
	rightExact := modelGatewayRuntimeStatusItemExactForScoreHistory(right, filter)
	if leftExact != rightExact {
		return leftExact
	}
	if left.SampleCount != right.SampleCount {
		return left.SampleCount > right.SampleCount
	}
	leftUpdated := modelGatewayRuntimeStatusItemUpdatedAt(left)
	rightUpdated := modelGatewayRuntimeStatusItemUpdatedAt(right)
	if leftUpdated != rightUpdated {
		return leftUpdated > rightUpdated
	}
	return left.ScoreTotal > right.ScoreTotal
}

func modelGatewayRuntimeStatusItemExactForScoreHistory(item modelgatewayobservability.RuntimeStatusItem, filter ModelGatewayRuntimeKey) bool {
	if filter.RequestedModel != "" && item.RequestedModel != filter.RequestedModel {
		return false
	}
	if filter.UpstreamModel != "" && item.UpstreamModel != filter.UpstreamModel {
		return false
	}
	if filter.Group != "" && item.Group != filter.Group {
		return false
	}
	if filter.EndpointType != "" && item.EndpointType != filter.EndpointType {
		return false
	}
	if filter.CapabilityFingerprint != "" && item.CapabilityFingerprint != filter.CapabilityFingerprint {
		return false
	}
	if filter.ResourceID != "" && item.ResourceID != filter.ResourceID {
		return false
	}
	if filter.ResourceType != "" && item.ResourceType != filter.ResourceType {
		return false
	}
	if filter.AccountID != "" && item.AccountID != filter.AccountID {
		return false
	}
	if filter.AccountType != "" && item.AccountType != filter.AccountType {
		return false
	}
	if filter.Brand != "" && item.Brand != filter.Brand {
		return false
	}
	if filter.Provider != "" && item.Provider != filter.Provider {
		return false
	}
	if filter.CredentialIndexSet && item.CredentialIndex != filter.CredentialIndex {
		return false
	}
	if filter.CredentialSubjectFP != "" && item.CredentialSubjectFP != filter.CredentialSubjectFP {
		return false
	}
	if filter.CredentialFP != "" && item.CredentialFP != filter.CredentialFP {
		return false
	}
	return true
}

func modelGatewayRuntimeStatusItemUpdatedAt(item modelgatewayobservability.RuntimeStatusItem) int64 {
	updatedAt := item.LastRealAttemptAt
	if item.LastProbeAt > updatedAt {
		updatedAt = item.LastProbeAt
	}
	if item.LastRealSuccessAt > updatedAt {
		updatedAt = item.LastRealSuccessAt
	}
	if item.LastRealFailureAt > updatedAt {
		updatedAt = item.LastRealFailureAt
	}
	return updatedAt
}

func modelGatewayScoreHistoryItemFromRuntime(item modelgatewayobservability.RuntimeStatusItem, generatedAt int64) ModelGatewayScoreHistoryItem {
	return ModelGatewayScoreHistoryItem{
		Source:                "runtime_current",
		CreatedAt:             generatedAt,
		RequestID:             "runtime-current",
		RequestedModel:        item.RequestedModel,
		RequestedGroup:        item.Group,
		SelectedGroup:         item.Group,
		ChannelID:             item.ChannelID,
		ChannelName:           modelGatewayRuntimeChannelName(item.ChannelID),
		RuntimeKey:            modelGatewayRuntimeKeyFromStatusItem(item),
		Available:             item.HealthStatus != "circuit_open",
		RejectReason:          modelGatewayRuntimeStatusRejectReason(item),
		ScoreTotal:            roundModelGatewayObservabilityFloat(item.ScoreTotal),
		ScoreBreakdown:        roundModelGatewayScoreMap(item.ScoreBreakdown),
		ScoreItems:            item.ScoreItems,
		RoutingScoreTotal:     roundModelGatewayObservabilityFloat(item.RoutingScoreTotal),
		RoutingScoreBreakdown: roundModelGatewayScoreMap(item.RoutingScoreBreakdown),
		RoutingScoreItems:     item.RoutingScoreItems,
		CostReferenceMissing:  item.CostReferenceMissing,
		SampleCount:           item.SampleCount,
		TTFTMs:                roundModelGatewayObservabilityFloat(item.TTFTMs),
		DurationMs:            roundModelGatewayObservabilityFloat(item.DurationMs),
		ActiveConcurrency:     item.ActiveConcurrency,
		EffectiveConcurrency:  item.MaxConcurrency,
	}
}

func modelGatewayRuntimeKeyFromStatusItem(item modelgatewayobservability.RuntimeStatusItem) ModelGatewayRuntimeKey {
	return ModelGatewayRuntimeKey{
		RequestedModel:        item.RequestedModel,
		UpstreamModel:         item.UpstreamModel,
		ChannelID:             item.ChannelID,
		ResourceID:            item.ResourceID,
		ResourceType:          item.ResourceType,
		AccountID:             item.AccountID,
		AccountType:           item.AccountType,
		Brand:                 item.Brand,
		Provider:              item.Provider,
		CredentialIndex:       item.CredentialIndex,
		CredentialSubjectFP:   item.CredentialSubjectFP,
		CredentialFP:          item.CredentialFP,
		Group:                 item.Group,
		EndpointType:          item.EndpointType,
		CapabilityFingerprint: item.CapabilityFingerprint,
	}
}

func modelGatewayRuntimeStatusRejectReason(item modelgatewayobservability.RuntimeStatusItem) string {
	switch item.HealthStatus {
	case "circuit_open":
		return "circuit_open"
	case "cooldown":
		return "concurrency_cooldown"
	case "failure_avoidance":
		if strings.TrimSpace(item.FailureAvoidanceReason) == service.ChannelTimeoutRecoveryReason ||
			strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason {
			return service.ChannelTimeoutRecoveryReason
		}
		if strings.TrimSpace(item.FailureAvoidanceReason) == service.ChannelOverloadRecoveryReason ||
			strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelOverloadRecoveryReason {
			return service.ChannelOverloadRecoveryReason
		}
		return "failure_avoidance"
	case "high_pressure":
		return "concurrency_pressure"
	default:
		return ""
	}
}

func modelGatewayRuntimeChannelName(channelID int) string {
	if channelID <= 0 {
		return ""
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil || channel == nil {
		return ""
	}
	return channel.Name
}

func modelGatewayRuntimeKeyMatchesFilter(candidate ModelGatewayRuntimeKey, filter ModelGatewayRuntimeKey) bool {
	if filter.RequestedModel != "" && filter.RequestedModel != candidate.RequestedModel {
		return false
	}
	if filter.UpstreamModel != "" && filter.UpstreamModel != candidate.UpstreamModel {
		return false
	}
	if filter.Group != "" && filter.Group != candidate.Group {
		return false
	}
	if filter.EndpointType != "" && filter.EndpointType != candidate.EndpointType {
		return false
	}
	if filter.CapabilityFingerprint != "" && filter.CapabilityFingerprint != candidate.CapabilityFingerprint {
		return false
	}
	if filter.ResourceID != "" && filter.ResourceID != candidate.ResourceID {
		return false
	}
	if filter.ResourceType != "" && filter.ResourceType != candidate.ResourceType {
		return false
	}
	if filter.AccountID != "" && filter.AccountID != candidate.AccountID {
		return false
	}
	if filter.AccountType != "" && filter.AccountType != candidate.AccountType {
		return false
	}
	if filter.Brand != "" && filter.Brand != candidate.Brand {
		return false
	}
	if filter.Provider != "" && filter.Provider != candidate.Provider {
		return false
	}
	if filter.CredentialIndexSet && filter.CredentialIndex != candidate.CredentialIndex {
		return false
	}
	if filter.CredentialSubjectFP != "" && filter.CredentialSubjectFP != candidate.CredentialSubjectFP {
		return false
	}
	if filter.CredentialFP != "" && filter.CredentialFP != candidate.CredentialFP {
		return false
	}
	return true
}

func modelGatewayRuntimeCircuitClearKeys(runtimeDeps *modelgatewayintegration.DefaultRuntimeObservability, channelID int, filter ModelGatewayRuntimeKey) []modelgatewaycore.RuntimeKey {
	keys := map[modelgatewaycore.RuntimeKey]struct{}{}
	addKey := func(key modelgatewaycore.RuntimeKey) {
		key = normalizeModelGatewayCoreRuntimeKey(key)
		if key.ChannelID != channelID {
			return
		}
		if !modelGatewayRuntimeKeyOnlyChannel(filter, channelID) && !modelGatewayRuntimeKeyMatchesFilter(modelGatewayRuntimeKeyFromCore(key), filter) {
			return
		}
		keys[key] = struct{}{}
	}
	if runtimeDeps != nil {
		if runtimeDeps.CircuitBreaker != nil {
			for _, snapshot := range runtimeDeps.CircuitBreaker.ListSnapshots() {
				addKey(snapshot.Key)
			}
		}
		if runtimeDeps.SnapshotStore != nil {
			for _, snapshot := range runtimeDeps.SnapshotStore.ListCandidates(nil) {
				addKey(snapshot.Key)
			}
		}
	}
	if len(keys) == 0 && !modelGatewayRuntimeKeyOnlyChannel(filter, channelID) {
		addKey(modelGatewayRuntimeKeyToCore(filter, channelID))
	}
	out := make([]modelgatewaycore.RuntimeKey, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.SliceStable(out, func(i int, j int) bool {
		left := modelGatewayRuntimeKeyFromCore(out[i])
		right := modelGatewayRuntimeKeyFromCore(out[j])
		return modelGatewayHealthCheckRuntimeRowKeyFromRuntimeKey(left) < modelGatewayHealthCheckRuntimeRowKeyFromRuntimeKey(right)
	})
	return out
}

func modelGatewayClearRuntimeCircuitSnapshots(runtimeDeps *modelgatewayintegration.DefaultRuntimeObservability, keys []modelgatewaycore.RuntimeKey, clearFailureAvoidance bool) int {
	if runtimeDeps == nil || runtimeDeps.SnapshotStore == nil || len(keys) == 0 {
		return 0
	}
	updated := 0
	for _, key := range keys {
		snapshot, ok := runtimeDeps.SnapshotStore.Get(key)
		if !ok {
			continue
		}
		snapshot.CircuitState = modelgatewaycore.CircuitStateClosed
		snapshot.CircuitOpen = false
		snapshot.CircuitOpenUntil = 0
		snapshot.CircuitOpenReason = ""
		snapshot.CircuitFailureCount = 0
		snapshot.CircuitFailureRate = 0
		snapshot.CircuitSampleCount = 0
		snapshot.CircuitErrorCounts = nil
		snapshot.CircuitHalfOpenProbeUsed = 0
		snapshot.CircuitHalfOpenProbeMax = 0
		if clearFailureAvoidance {
			wasRecovery := snapshot.FailureAvoidance ||
				strings.TrimSpace(snapshot.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason ||
				strings.TrimSpace(snapshot.ProbeTriggerReason) == service.ChannelOverloadRecoveryReason ||
				strings.TrimSpace(snapshot.ProbeTriggerReason) == "failure_avoidance"
			snapshot.FailureAvoidance = false
			if wasRecovery {
				snapshot.ProbeRecoveryPending = false
				snapshot.ProbeRecoverySuccessCount = 0
				snapshot.ProbeRecoveryRequired = 0
				snapshot.ProbeTriggerReason = ""
			}
		}
		runtimeDeps.SnapshotStore.Put(snapshot)
		updated++
	}
	return updated
}

func modelGatewayClearRuntimeCooldownSnapshots(runtimeDeps *modelgatewayintegration.DefaultRuntimeObservability, keys []modelgatewaycore.RuntimeKey) int {
	if runtimeDeps == nil || runtimeDeps.SnapshotStore == nil || len(keys) == 0 {
		return 0
	}
	updated := 0
	for _, key := range keys {
		snapshot, ok := runtimeDeps.SnapshotStore.Get(key)
		if !ok || !snapshot.Cooldown {
			continue
		}
		snapshot.Cooldown = false
		if strings.TrimSpace(snapshot.ProbeTriggerReason) == "cooldown" {
			snapshot.ProbeRecoveryPending = false
			snapshot.ProbeRecoverySuccessCount = 0
			snapshot.ProbeRecoveryRequired = 0
			snapshot.ProbeTriggerReason = ""
		}
		runtimeDeps.SnapshotStore.Put(snapshot)
		updated++
	}
	return updated
}

func modelGatewayRecoverRuntimeHealthSnapshots(runtimeDeps *modelgatewayintegration.DefaultRuntimeObservability, keys []modelgatewaycore.RuntimeKey) int {
	if runtimeDeps == nil || runtimeDeps.SnapshotStore == nil || len(keys) == 0 {
		return 0
	}
	now := time.Now().Unix()
	updated := 0
	for _, key := range keys {
		snapshot, ok := runtimeDeps.SnapshotStore.Get(key)
		if !ok {
			continue
		}
		if snapshot.SampleCount < 3 {
			snapshot.SampleCount = 3
		}
		snapshot.ScoreStatsJSON = ""
		snapshot.RecentLatencySamples = nil
		snapshot.SuccessRate = 1
		snapshot.TTFTMs = 800
		snapshot.DurationMs = 3000
		snapshot.TokensPerSecond = 80
		snapshot.EmptyOutputRate = 0
		snapshot.ExperienceIssueRate = 0
		snapshot.QueueDepth = 0
		snapshot.EstimatedQueueWaitMs = 0
		snapshot.FirstBytePending = 0
		snapshot.SlowFirstBytePending = 0
		snapshot.OldestFirstByteWaitMs = 0
		snapshot.CircuitState = modelgatewaycore.CircuitStateClosed
		snapshot.CircuitOpen = false
		snapshot.CircuitOpenUntil = 0
		snapshot.CircuitOpenReason = ""
		snapshot.CircuitFailureCount = 0
		snapshot.CircuitFailureRate = 0
		snapshot.CircuitSampleCount = 0
		snapshot.CircuitErrorCounts = nil
		snapshot.CircuitHalfOpenProbeUsed = 0
		snapshot.CircuitHalfOpenProbeMax = 0
		snapshot.Cooldown = false
		snapshot.FailureAvoidance = false
		snapshot.RecoverableQualityScore = 1
		snapshot.RecoverableQualityBaseline = 0
		snapshot.RecoverableQualityBaselineSamples = 0
		snapshot.RecoverableQualityDropRatio = 0
		snapshot.RecoverableQualityItemBaselines = nil
		snapshot.ProbeRecoveryPending = false
		snapshot.ProbeRecoverySuccessCount = 0
		snapshot.ProbeRecoveryRequired = 0
		snapshot.ProbeTriggerReason = ""
		snapshot.ProbeRecoveryPhase = ""
		snapshot.ProbeFastRecoveryAttempts = 0
		snapshot.ProbeAnomalyTriggerItems = nil
		snapshot.ConfigErrorIsolated = false
		snapshot.IsolationReason = ""
		snapshot.IsolationUntil = 0
		snapshot.AuthConfigErrorCount = 0
		snapshot.LastAuthConfigErrorAt = 0
		snapshot.LastRealSuccessAt = now
		snapshot.LastRealFailureAt = 0
		if snapshot.LastRealAttemptAt <= 0 {
			snapshot.LastRealAttemptAt = now
		}
		if snapshot.RealSampleCount30m <= 0 {
			snapshot.RealSampleCount30m = 1
		}
		if snapshot.LastProbeSuccessAt <= 0 {
			snapshot.LastProbeSuccessAt = now
		}
		runtimeDeps.SnapshotStore.Put(snapshot)
		updated++
	}
	return updated
}

func modelGatewayRuntimeKeyOnlyChannel(key ModelGatewayRuntimeKey, channelID int) bool {
	key.ChannelID = channelID
	return key.RequestedModel == "" &&
		key.UpstreamModel == "" &&
		key.ChannelID == channelID &&
		key.ResourceID == "" &&
		key.ResourceType == "" &&
		key.AccountID == "" &&
		key.AccountType == "" &&
		key.Brand == "" &&
		key.Provider == "" &&
		!key.CredentialIndexSet &&
		key.CredentialIndex == 0 &&
		key.CredentialSubjectFP == "" &&
		key.CredentialFP == "" &&
		key.Group == "" &&
		key.EndpointType == "" &&
		key.CapabilityFingerprint == ""
}

func modelGatewayRuntimeKeyIsEmpty(key ModelGatewayRuntimeKey) bool {
	return key.RequestedModel == "" &&
		key.UpstreamModel == "" &&
		key.ChannelID == 0 &&
		key.ResourceID == "" &&
		key.ResourceType == "" &&
		key.AccountID == "" &&
		key.AccountType == "" &&
		key.Brand == "" &&
		key.Provider == "" &&
		!key.CredentialIndexSet &&
		key.CredentialSubjectFP == "" &&
		key.CredentialFP == "" &&
		key.Group == "" &&
		key.EndpointType == "" &&
		key.CapabilityFingerprint == ""
}

func modelGatewayRuntimeKeyToCore(key ModelGatewayRuntimeKey, channelID int) modelgatewaycore.RuntimeKey {
	if key.ChannelID <= 0 {
		key.ChannelID = channelID
	}
	return normalizeModelGatewayCoreRuntimeKey(modelgatewaycore.RuntimeKey{
		RequestedModel:        key.RequestedModel,
		UpstreamModel:         key.UpstreamModel,
		ChannelID:             key.ChannelID,
		ResourceID:            key.ResourceID,
		ResourceType:          key.ResourceType,
		AccountID:             key.AccountID,
		AccountType:           key.AccountType,
		Brand:                 key.Brand,
		Provider:              key.Provider,
		CredentialIndex:       key.CredentialIndex,
		CredentialSubjectFP:   key.CredentialSubjectFP,
		CredentialFP:          key.CredentialFP,
		Group:                 key.Group,
		EndpointType:          constant.EndpointType(key.EndpointType),
		CapabilityFingerprint: key.CapabilityFingerprint,
	})
}

func modelGatewayHealthCheckProbeCompleteRuntimeKey(key modelgatewaycore.RuntimeKey, request ModelGatewayHealthCheckProbeRequest, channel *model.Channel) modelgatewaycore.RuntimeKey {
	key = normalizeModelGatewayCoreRuntimeKey(key)
	if key.ChannelID <= 0 && request.ChannelID > 0 {
		key.ChannelID = request.ChannelID
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	if runtimeDeps != nil && runtimeDeps.SnapshotStore != nil {
		if snapshot, ok := runtimeDeps.SnapshotStore.Get(key); ok {
			key = modelGatewayMergeRuntimeKey(key, snapshot.Key)
		} else {
			for _, snapshot := range runtimeDeps.SnapshotStore.ListCandidates(nil) {
				if snapshot.Key.ChannelID != key.ChannelID {
					continue
				}
				if modelGatewayRuntimeKeyMatchesFilter(modelGatewayRuntimeKeyFromCore(snapshot.Key), modelGatewayRuntimeKeyFromCore(key)) {
					key = modelGatewayMergeRuntimeKey(key, snapshot.Key)
					break
				}
			}
		}
	}
	if strings.TrimSpace(request.RequestedModel) != "" {
		key.RequestedModel = strings.TrimSpace(request.RequestedModel)
	}
	if strings.TrimSpace(request.Group) != "" {
		key.Group = strings.TrimSpace(request.Group)
	}
	if key.RequestedModel == "" && key.UpstreamModel != "" {
		key.RequestedModel = key.UpstreamModel
	}
	if key.UpstreamModel == "" && channel != nil && key.RequestedModel != "" {
		key.UpstreamModel = channel.ResolveMappedModelName(key.RequestedModel)
	}
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	return normalizeModelGatewayCoreRuntimeKey(key)
}

func modelGatewayMergeRuntimeKey(base modelgatewaycore.RuntimeKey, fallback modelgatewaycore.RuntimeKey) modelgatewaycore.RuntimeKey {
	if base.RequestedModel == "" {
		base.RequestedModel = fallback.RequestedModel
	}
	if base.UpstreamModel == "" {
		base.UpstreamModel = fallback.UpstreamModel
	}
	if base.ChannelID == 0 {
		base.ChannelID = fallback.ChannelID
	}
	if base.ResourceID == "" {
		base.ResourceID = fallback.ResourceID
	}
	if base.ResourceType == "" {
		base.ResourceType = fallback.ResourceType
	}
	if base.AccountID == "" {
		base.AccountID = fallback.AccountID
	}
	if base.AccountType == "" {
		base.AccountType = fallback.AccountType
	}
	if base.Brand == "" {
		base.Brand = fallback.Brand
	}
	if base.Provider == "" {
		base.Provider = fallback.Provider
	}
	if base.CredentialIndex == 0 {
		base.CredentialIndex = fallback.CredentialIndex
	}
	if base.CredentialSubjectFP == "" {
		base.CredentialSubjectFP = fallback.CredentialSubjectFP
	}
	if base.CredentialFP == "" {
		base.CredentialFP = fallback.CredentialFP
	}
	if base.Group == "" {
		base.Group = fallback.Group
	}
	if base.EndpointType == "" {
		base.EndpointType = fallback.EndpointType
	}
	if base.CapabilityFingerprint == "" {
		base.CapabilityFingerprint = fallback.CapabilityFingerprint
	}
	return base
}

func modelGatewayHealthCheckProbeReason(requested string, key modelgatewaycore.RuntimeKey) string {
	if reason := modelgatewayprobe.NormalizeProbeReason(requested); reason != "" {
		return reason
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	if runtimeDeps != nil && runtimeDeps.SnapshotStore != nil {
		if snapshot, ok := runtimeDeps.SnapshotStore.Get(key); ok {
			if reason := modelgatewayprobe.NormalizeProbeReason(snapshot.ProbeTriggerReason); reason != "" {
				return reason
			}
			if snapshot.CircuitState == modelgatewaycore.CircuitStateHalfOpen {
				return "circuit_half_open"
			}
			if snapshot.FailureAvoidance {
				return "failure_avoidance"
			}
			if snapshot.ProbeRecoveryPending {
				return "low_score"
			}
		}
	}
	return "low_score"
}

func normalizeModelGatewayCoreRuntimeKey(key modelgatewaycore.RuntimeKey) modelgatewaycore.RuntimeKey {
	key.RequestedModel = strings.TrimSpace(key.RequestedModel)
	key.UpstreamModel = strings.TrimSpace(key.UpstreamModel)
	key.ResourceID = strings.TrimSpace(key.ResourceID)
	key.ResourceType = strings.TrimSpace(key.ResourceType)
	key.AccountID = strings.TrimSpace(key.AccountID)
	key.AccountType = strings.TrimSpace(key.AccountType)
	key.Brand = strings.TrimSpace(key.Brand)
	key.Provider = strings.TrimSpace(key.Provider)
	key.CredentialSubjectFP = strings.TrimSpace(key.CredentialSubjectFP)
	key.CredentialFP = strings.TrimSpace(key.CredentialFP)
	key.Group = strings.TrimSpace(key.Group)
	key.EndpointType = constant.EndpointType(strings.TrimSpace(string(key.EndpointType)))
	key.CapabilityFingerprint = strings.TrimSpace(key.CapabilityFingerprint)
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	return key
}

func modelGatewayRuntimeIdentityFromCoreKey(key modelgatewaycore.RuntimeKey) service.ChannelRuntimeIdentity {
	key = normalizeModelGatewayCoreRuntimeKey(key)
	return service.NormalizeChannelRuntimeIdentity(service.ChannelRuntimeIdentity{
		ChannelID:           key.ChannelID,
		RequestedModel:      key.RequestedModel,
		SelectedGroup:       key.Group,
		EndpointType:        key.EndpointType,
		AccountID:           key.AccountID,
		CredentialIndex:     key.CredentialIndex,
		CredentialIndexSet:  key.CredentialIndex > 0,
		CredentialSubjectFP: key.CredentialSubjectFP,
		CredentialFP:        key.CredentialFP,
	})
}

func modelGatewayRuntimeKeysFromCore(keys []modelgatewaycore.RuntimeKey) []ModelGatewayRuntimeKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]ModelGatewayRuntimeKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, modelGatewayRuntimeKeyFromCore(normalizeModelGatewayCoreRuntimeKey(key)))
	}
	return out
}

func modelGatewayScoreHistoryItem(record model.ModelExecutionRecord, candidate ModelGatewayCandidateExplanation) ModelGatewayScoreHistoryItem {
	requestMeta, _ := parseModelGatewayRequestMeta(record.RequestMeta)
	attemptMeta := modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta)
	return ModelGatewayScoreHistoryItem{
		ID:                                record.Id,
		CreatedAt:                         record.CreatedAt,
		RequestID:                         record.RequestId,
		RequestedModel:                    record.RequestedModel,
		RequestedGroup:                    record.RequestedGroup,
		SelectedGroup:                     record.SelectedGroup,
		ChannelID:                         modelGatewayCandidateChannelID(candidate),
		ChannelName:                       candidate.ChannelName,
		RuntimeKey:                        candidate.RuntimeKey,
		PoolLevel:                         candidate.PoolLevel,
		SwitchReason:                      candidate.SwitchReason,
		FailureScope:                      candidate.FailureScope,
		Selected:                          candidate.Selected,
		SelectedReason:                    record.SelectedReason,
		Available:                         candidate.Available,
		RejectReason:                      candidate.RejectReason,
		ClientEmptyOutputSessionKey:       candidate.ClientEmptyOutputSessionKey,
		ClientEmptyOutputAvoidUntil:       candidate.ClientEmptyOutputAvoidUntil,
		ClientEmptyOutputRemainingSeconds: candidate.ClientEmptyOutputRemainingSeconds,
		StatusReason:                      candidate.StatusReason,
		ScoreTotal:                        candidate.ScoreTotal,
		ScoreBreakdown:                    candidate.ScoreBreakdown,
		ScoreItems:                        candidate.ScoreItems,
		RoutingScoreTotal:                 candidate.RoutingScoreTotal,
		RoutingScoreBreakdown:             candidate.RoutingScoreBreakdown,
		RoutingScoreItems:                 candidate.RoutingScoreItems,
		CostReferenceMissing:              candidate.CostReferenceMissing,
		SampleCount:                       candidate.SampleCount,
		TTFTMs:                            candidate.TTFTMs,
		DurationMs:                        candidate.DurationMs,
		ActiveConcurrency:                 candidate.ActiveConcurrency,
		EffectiveConcurrency:              candidate.EffectiveConcurrencyLimit,
		IsHealthProbe:                     attemptMeta.IsHealthProbe,
		ScoreSampleSource:                 candidate.ScoreSampleSource,
	}
}

func applyModelGatewayScoreHistoryDeltas(response *ModelGatewayScoreHistoryResponse) {
	if response == nil || len(response.Items) == 0 {
		return
	}
	for idx := range response.Items {
		if idx+1 >= len(response.Items) {
			continue
		}
		current := &response.Items[idx]
		previous := response.Items[idx+1]
		current.ScoreDelta = roundModelGatewayObservabilityFloat(current.ScoreTotal - previous.ScoreTotal)
		current.ScoreBreakdownDelta = modelGatewayScoreMapDelta(current.ScoreBreakdown, previous.ScoreBreakdown)
		current.ScoreItemDeltas = modelGatewayScoreItemsDelta(current.ScoreItems, previous.ScoreItems)
	}
	response.Current = &response.Items[0]
	if len(response.Items) > 1 {
		response.Previous = &response.Items[1]
		response.ScoreDelta = response.Items[0].ScoreDelta
		response.MetricDeltas = response.Items[0].ScoreBreakdownDelta
	}
}

func modelGatewayScoreMapDelta(current map[string]float64, previous map[string]float64) map[string]float64 {
	if len(current) == 0 && len(previous) == 0 {
		return nil
	}
	keys := make(map[string]struct{}, len(current)+len(previous))
	for key := range current {
		keys[key] = struct{}{}
	}
	for key := range previous {
		keys[key] = struct{}{}
	}
	out := make(map[string]float64, len(keys))
	for key := range keys {
		out[key] = roundModelGatewayObservabilityFloat(current[key] - previous[key])
	}
	return out
}

func applyModelGatewayCandidateChannelStatus(candidate *ModelGatewayCandidateExplanation, status modelGatewayCurrentChannelStatus) {
	if candidate == nil {
		return
	}
	candidate.ChannelStatus = status.Status
	candidate.StatusReason = status.StatusReason
	if !status.BalanceInsufficient {
		candidate.BalanceInsufficient = false
		return
	}
	candidate.BalanceInsufficient = true
	candidate.Available = false
	if strings.TrimSpace(candidate.RejectReason) == "" {
		candidate.RejectReason = service.ChannelStatusReasonBalanceInsufficient
	}
}

func modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta map[string]any) modelGatewayObservabilityAttemptMeta {
	if len(requestMeta) == 0 {
		return modelGatewayObservabilityAttemptMeta{}
	}
	return modelGatewayObservabilityAttemptMeta{
		ErrorMessage:                   strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["error_message"])),
		ErrorCategory:                  strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["error_category"])),
		WarningLevel:                   strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["warning_level"])),
		WarningFlags:                   modelGatewayObservabilityStringSlice(requestMeta["warning_flags"]),
		WarningMessage:                 strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["warning_message"])),
		ChannelInducedClientAbort:      modelGatewayObservabilityMetaBool(requestMeta["channel_induced_client_abort"]),
		RetryAction:                    strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["retry_action"])),
		RetryReason:                    strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["retry_reason"])),
		WillRetry:                      modelGatewayObservabilityMetaBool(requestMeta["will_retry"]),
		ClientAborted:                  modelGatewayObservabilityMetaBool(requestMeta["client_aborted"]),
		ConcurrencyLimited:             modelGatewayObservabilityMetaBool(requestMeta["concurrency_limited"]),
		BalanceInsufficient:            modelGatewayObservabilityMetaBool(requestMeta["balance_insufficient"]),
		EmptyOutput:                    modelGatewayObservabilityMetaBool(requestMeta["empty_output"]),
		ExperienceIssue:                strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["experience_issue"])),
		ActiveConcurrency:              int(modelGatewayObservabilityMetaInt64(requestMeta["active_concurrency"])),
		ConfiguredConcurrencyLimit:     int(modelGatewayObservabilityMetaInt64(requestMeta["configured_concurrency_limit"])),
		LearnedConcurrencyLimit:        int(modelGatewayObservabilityMetaInt64(requestMeta["learned_concurrency_limit"])),
		LearnedConcurrencyLimitChanged: modelGatewayObservabilityMetaBool(requestMeta["learned_concurrency_limit_changed"]),
		UsedChannels:                   modelGatewayObservabilityStringSlice(requestMeta["used_channels"]),
		IsHealthProbe:                  modelGatewayObservabilityMetaBool(requestMeta["is_health_probe"]),
		ProbeReason:                    strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["probe_reason"])),
	}
}

func modelGatewayObservabilityTimingMetaFromRequestMeta(requestMeta map[string]any) modelGatewayObservabilityTimingMeta {
	timingRaw, ok := requestMeta["timing"]
	if !ok || timingRaw == nil {
		return modelGatewayObservabilityTimingMeta{}
	}
	timing, ok := timingRaw.(map[string]any)
	if !ok {
		return modelGatewayObservabilityTimingMeta{}
	}
	return modelGatewayObservabilityTimingMeta{
		QueueWaitMs:                  modelGatewayObservabilityMetaInt64(timing["queue_wait_ms"]),
		RelayToFirstByteMs:           modelGatewayObservabilityMetaInt64(timing["relay_to_first_byte_ms"]),
		RelayTotalMs:                 modelGatewayObservabilityMetaInt64(timing["relay_total_ms"]),
		UpstreamResponseHeaderMs:     modelGatewayObservabilityMetaInt64(timing["upstream_response_header_ms"]),
		UpstreamFirstEventWaitMs:     modelGatewayObservabilityMetaInt64(timing["upstream_first_event_wait_ms"]),
		PreFirstByteMs:               modelGatewayObservabilityMetaInt64(timing["pre_first_byte_ms"]),
		PostFirstByteMs:              modelGatewayObservabilityMetaInt64(timing["post_first_byte_ms"]),
		RequestBodyPrepareMs:         modelGatewayObservabilityMetaInt64(timing["request_body_prepare_ms"]),
		RequestBodyBytes:             modelGatewayObservabilityMetaInt64(timing["request_body_bytes"]),
		RequestBodyStorage:           strings.TrimSpace(modelGatewayObservabilityMetaString(timing["request_body_storage"])),
		RequestBodySizeLikelyLatency: modelGatewayObservabilityMetaBool(timing["request_body_size_likely_latency"]),
	}
}

func modelGatewayCandidateExplanationsFromRequestMeta(requestMeta map[string]any) []ModelGatewayCandidateExplanation {
	if len(requestMeta) == 0 {
		return nil
	}
	raw, ok := requestMeta["candidate_explanations"]
	if !ok || raw == nil {
		return nil
	}
	bytes, err := common.Marshal(raw)
	if err != nil {
		return nil
	}
	candidates := make([]modelgatewaycore.CandidateExplanation, 0)
	if err := common.Unmarshal(bytes, &candidates); err != nil {
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	out := make([]ModelGatewayCandidateExplanation, 0, len(candidates))
	for _, candidate := range candidates {
		item := ModelGatewayCandidateExplanation{
			ChannelID:                         candidate.ChannelID,
			ChannelName:                       candidate.ChannelName,
			ResourceID:                        strings.TrimSpace(candidate.ResourceID),
			ResourceType:                      strings.TrimSpace(candidate.ResourceType),
			AccountID:                         strings.TrimSpace(candidate.AccountID),
			AccountType:                       strings.TrimSpace(candidate.AccountType),
			Brand:                             strings.TrimSpace(candidate.Brand),
			Provider:                          strings.TrimSpace(candidate.Provider),
			CredentialIndex:                   candidate.CredentialIndex,
			CredentialSubjectFP:               strings.TrimSpace(candidate.CredentialSubjectFP),
			CredentialFP:                      strings.TrimSpace(candidate.CredentialFP),
			PoolLevel:                         strings.TrimSpace(candidate.PoolLevel),
			SwitchReason:                      strings.TrimSpace(candidate.SwitchReason),
			FailureScope:                      strings.TrimSpace(candidate.FailureScope),
			Group:                             candidate.Group,
			UpstreamModel:                     candidate.UpstreamModel,
			ProviderProfile:                   candidate.ProviderProfile,
			ProxyMode:                         candidate.ProxyMode,
			RuntimeKey:                        modelGatewayRuntimeKeyFromCore(candidate.RuntimeKey),
			Available:                         candidate.Available,
			RejectReason:                      candidate.RejectReason,
			ClientEmptyOutputSessionKey:       candidate.ClientEmptyOutputSessionKey,
			ClientEmptyOutputAvoidUntil:       candidate.ClientEmptyOutputAvoidUntil,
			ClientEmptyOutputRemainingSeconds: candidate.ClientEmptyOutputRemainingSeconds,
			SelectionSkipReason:               candidate.SelectionSkipReason,
			ChannelStatus:                     candidate.ChannelStatus,
			ChannelPriority:                   candidate.ChannelPriority,
			StatusReason:                      candidate.StatusReason,
			BalanceInsufficient:               candidate.BalanceInsufficient,
			ScoreTotal:                        roundModelGatewayObservabilityFloat(candidate.ScoreTotal),
			ScoreBreakdown:                    roundModelGatewayScoreMap(candidate.ScoreBreakdown),
			ScoreItems:                        candidate.ScoreItems,
			RoutingScoreTotal:                 roundModelGatewayObservabilityFloat(candidate.RoutingScoreTotal),
			RoutingScoreBreakdown:             roundModelGatewayScoreMap(candidate.RoutingScoreBreakdown),
			RoutingScoreItems:                 candidate.RoutingScoreItems,
			StateTags:                         append([]string(nil), candidate.StateTags...),
			CostReferenceMissing:              candidate.CostReferenceMissing,
			SuccessRate:                       roundModelGatewayObservabilityFloat(candidate.SuccessRate),
			TTFTMs:                            roundModelGatewayObservabilityFloat(candidate.TTFTMs),
			DurationMs:                        roundModelGatewayObservabilityFloat(candidate.DurationMs),
			TokensPerSecond:                   roundModelGatewayObservabilityFloat(candidate.TokensPerSecond),
			SampleCount:                       candidate.SampleCount,
			ActiveConcurrency:                 candidate.ActiveConcurrency,
			MaxConcurrency:                    candidate.MaxConcurrency,
			ConfiguredConcurrencyLimit:        candidate.ConfiguredConcurrencyLimit,
			LearnedConcurrencyLimit:           candidate.LearnedConcurrencyLimit,
			EffectiveConcurrencyLimit:         candidate.EffectiveConcurrencyLimit,
			QueueDepth:                        candidate.QueueDepth,
			QueueCapacity:                     candidate.QueueCapacity,
			EstimatedQueueWaitMs:              roundModelGatewayObservabilityFloat(candidate.EstimatedQueueWaitMs),
			ResourceProtectionRole:            strings.TrimSpace(candidate.ResourceProtectionRole),
			ResourceProtectionReason:          strings.TrimSpace(candidate.ResourceProtectionReason),
			CostRatio:                         roundModelGatewayObservabilityFloat(candidate.CostRatio),
			CostReferenceRatio:                roundModelGatewayObservabilityFloat(candidate.CostReferenceRatio),
			CostPricingMode:                   strings.TrimSpace(candidate.CostPricingMode),
			GroupPriorityRatio:                roundModelGatewayObservabilityFloat(candidate.GroupPriorityRatio),
			CircuitState:                      strings.TrimSpace(string(candidate.CircuitState)),
			CircuitOpen:                       candidate.CircuitOpen,
			CircuitOpenUntil:                  candidate.CircuitOpenUntil,
			CircuitOpenReason:                 strings.TrimSpace(candidate.CircuitOpenReason),
			CircuitFailureCount:               candidate.CircuitFailureCount,
			CircuitFailureRate:                roundModelGatewayObservabilityFloat(candidate.CircuitFailureRate),
			CircuitSampleCount:                candidate.CircuitSampleCount,
			CircuitErrorCounts:                copyModelGatewayCircuitErrorCounts(candidate.CircuitErrorCounts),
			CircuitHalfOpenProbeUsed:          candidate.CircuitHalfOpenProbeUsed,
			CircuitHalfOpenProbeMax:           candidate.CircuitHalfOpenProbeMax,
			EmptyOutputRate:                   roundModelGatewayObservabilityFloat(candidate.EmptyOutputRate),
			ExperienceIssueRate:               roundModelGatewayObservabilityFloat(candidate.ExperienceIssueRate),
			ProbeRecoveryPending:              candidate.ProbeRecoveryPending,
			ProbeRecoverySuccessCount:         candidate.ProbeRecoverySuccessCount,
			ProbeRecoveryRequired:             candidate.ProbeRecoveryRequired,
			ProbeTriggerReason:                strings.TrimSpace(candidate.ProbeTriggerReason),
			ConfigErrorIsolated:               candidate.ConfigErrorIsolated,
			IsolationReason:                   strings.TrimSpace(candidate.IsolationReason),
			IsolationUntil:                    candidate.IsolationUntil,
			AuthConfigErrorCount:              candidate.AuthConfigErrorCount,
			LastAuthConfigErrorAt:             candidate.LastAuthConfigErrorAt,
			StickyMatched:                     candidate.StickyMatched,
			Selected:                          candidate.Selected,
			ScoreSampleSource:                 candidate.ScoreSampleSource,
			MatchedRuntimeKey:                 modelGatewayRuntimeKeyFromCore(candidate.MatchedRuntimeKey),
		}
		item.CredentialUID, item.CredentialLabel = modelGatewayCandidateCredentialDisplay(item)
		out = append(out, item)
	}
	return out
}

func modelGatewayCandidateCredentialDisplay(candidate ModelGatewayCandidateExplanation) (string, string) {
	runtimeKey := candidate.RuntimeKey
	subjectFP := firstModelGatewayObservabilityString(candidate.CredentialSubjectFP, runtimeKey.CredentialSubjectFP)
	credentialFP := firstModelGatewayObservabilityString(candidate.CredentialFP, runtimeKey.CredentialFP)
	accountID := firstModelGatewayObservabilityString(candidate.AccountID, runtimeKey.AccountID)
	resourceID := firstModelGatewayObservabilityString(candidate.ResourceID, runtimeKey.ResourceID)
	channelID := candidate.ChannelID
	if channelID <= 0 {
		channelID = runtimeKey.ChannelID
	}
	uid := modelgatewayaccount.CredentialUIDFromParts(modelgatewayaccount.CredentialUIDParts{
		CredentialSubjectFingerprint: subjectFP,
		CredentialFingerprint:        credentialFP,
		AccountID:                    accountID,
		CredentialResourceID:         resourceID,
		ResourceID:                   resourceID,
		ChannelID:                    channelID,
	})
	if uid == "" {
		return "", ""
	}
	brand := firstModelGatewayObservabilityString(candidate.Brand, runtimeKey.Brand)
	if brand == "" {
		return uid, uid
	}
	return uid, strings.ToLower(brand) + "-" + uid
}

func firstModelGatewayObservabilityString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func modelGatewayRejectReasonsFromRequestMeta(requestMeta map[string]any) []string {
	if len(requestMeta) == 0 {
		return nil
	}
	raw, ok := requestMeta["candidate_explanations"]
	if !ok || raw == nil {
		return nil
	}
	bytes, err := common.Marshal(raw)
	if err != nil {
		return nil
	}
	candidates := make([]modelgatewaycore.CandidateExplanation, 0)
	if err := common.Unmarshal(bytes, &candidates); err != nil {
		return nil
	}
	reasons := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		reason := strings.TrimSpace(candidate.RejectReason)
		if candidate.Available || reason == "" {
			continue
		}
		reasons = append(reasons, reason)
	}
	return reasons
}

func modelGatewayRuntimeKeyFromCore(key modelgatewaycore.RuntimeKey) ModelGatewayRuntimeKey {
	return ModelGatewayRuntimeKey{
		RequestedModel:        key.RequestedModel,
		UpstreamModel:         key.UpstreamModel,
		ChannelID:             key.ChannelID,
		ResourceID:            key.ResourceID,
		ResourceType:          key.ResourceType,
		AccountID:             key.AccountID,
		AccountType:           key.AccountType,
		Brand:                 key.Brand,
		Provider:              key.Provider,
		CredentialIndex:       key.CredentialIndex,
		CredentialSubjectFP:   key.CredentialSubjectFP,
		CredentialFP:          key.CredentialFP,
		Group:                 key.Group,
		EndpointType:          string(key.EndpointType),
		CapabilityFingerprint: key.CapabilityFingerprint,
	}
}

func copyModelGatewayCircuitErrorCounts(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func roundModelGatewayScoreMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	rounded := make(map[string]float64, len(values))
	for key, value := range values {
		rounded[key] = roundModelGatewayObservabilityFloat(value)
	}
	return rounded
}

func modelGatewayObservabilityMetaBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func modelGatewayObservabilityMetaInt64(value any) int64 {
	parsed, ok := modelGatewayObservabilityMetaInt64Value(value)
	if !ok {
		return 0
	}
	return parsed
}

func modelGatewayObservabilityMetaNonNegativeInt64(value any) (int64, bool) {
	parsed, ok := modelGatewayObservabilityMetaInt64Value(value)
	if !ok || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func modelGatewayObservabilityMetaInt64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return math.MaxInt64, true
		}
		return int64(typed), true
	case float32:
		return int64(math.Round(float64(typed))), true
	case float64:
		return int64(math.Round(typed)), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed, true
		}
		parsedFloat, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return int64(math.Round(parsedFloat)), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func modelGatewayObservabilityMetaString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func modelGatewayObservabilityStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func modelGatewayObservabilityIntSlice(value any) []int {
	switch typed := value.(type) {
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		result := make([]int, 0, len(typed))
		for _, item := range typed {
			result = append(result, int(item))
		}
		return result
	case []any:
		result := make([]int, 0, len(typed))
		for _, item := range typed {
			value := int(modelGatewayObservabilityMetaInt64(item))
			if value > 0 {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func finalizeModelGatewayObservabilityAggregates(accumulators map[string]*modelGatewayObservabilityAccumulator, topN int) []ModelGatewayObservabilityAggregate {
	items := make([]ModelGatewayObservabilityAggregate, 0, len(accumulators))
	for _, accumulator := range accumulators {
		item := accumulator.ModelGatewayObservabilityAggregate
		item.SuccessRate = successRateModelGatewayObservability(item.Successes, item.Attempts)
		item.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
		item.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
		item.AvgScoreTotal = averageFloat64(accumulator.scoreTotalSum, accumulator.scoreTotalSamples)
		item.QueueWaitCount = accumulator.QueueWaitCount
		item.AvgQueueWaitMs = averageInt64(accumulator.queueWaitSum, accumulator.queueWaitSamples)
		item.ResourceProtectionAvgWaitMs = averageInt64(accumulator.resourceProtectionWaitSum, accumulator.resourceProtectionWaitSamples)
		item.ResourceProtectionFallbackCostShare = modelGatewayResourceProtectionFallbackCostShare(accumulator)
		item.ScoreBreakdown = averageModelGatewayObservabilityScoreBreakdown(accumulator)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Attempts != items[j].Attempts {
			return items[i].Attempts > items[j].Attempts
		}
		if items[i].Failures != items[j].Failures {
			return items[i].Failures > items[j].Failures
		}
		if items[i].Records != items[j].Records {
			return items[i].Records > items[j].Records
		}
		return items[i].Key < items[j].Key
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func modelGatewayResourceProtectionFallbackCostShare(accumulator *modelGatewayObservabilityAccumulator) float64 {
	if accumulator == nil || accumulator.resourceProtectionCostSum <= 0 {
		return 0
	}
	share := accumulator.resourceProtectionFallbackCost / accumulator.resourceProtectionCostSum
	if share < 0 {
		share = 0
	}
	if share > 1 {
		share = 1
	}
	return roundModelGatewayObservabilityFloat(share)
}

func finalizeModelGatewayObservabilityTrends(accumulators map[int64]*modelGatewayObservabilityTrendAccumulator, startTime int64, endTime int64, bucketSeconds int64) []ModelGatewayObservabilityTrendPoint {
	if startTime <= 0 || endTime <= startTime || bucketSeconds <= 0 {
		return []ModelGatewayObservabilityTrendPoint{}
	}
	bucketCount := int((endTime - startTime + bucketSeconds - 1) / bucketSeconds)
	if bucketCount <= 0 {
		return []ModelGatewayObservabilityTrendPoint{}
	}
	items := make([]ModelGatewayObservabilityTrendPoint, 0, bucketCount)
	for idx := 0; idx < bucketCount; idx++ {
		bucketStart := startTime + int64(idx)*bucketSeconds
		bucketEnd := bucketStart + bucketSeconds
		if bucketEnd > endTime {
			bucketEnd = endTime
		}
		items = append(items, modelGatewayObservabilityTrendPointFromAccumulator(bucketStart, bucketEnd, accumulators[bucketStart]))
	}
	return items
}

func modelGatewayObservabilityTrendPointFromAccumulator(bucketStart int64, bucketEnd int64, accumulator *modelGatewayObservabilityTrendAccumulator) ModelGatewayObservabilityTrendPoint {
	point := ModelGatewayObservabilityTrendPoint{
		BucketStart: bucketStart,
		BucketEnd:   bucketEnd,
	}
	if accumulator == nil {
		return point
	}
	point.Records = accumulator.Records
	point.Dispatches = accumulator.Dispatches
	point.Attempts = accumulator.Attempts
	point.Successes = accumulator.Successes
	point.Failures = accumulator.Failures
	point.StreamInterrupted = accumulator.StreamInterrupted
	point.FallbackUsed = accumulator.FallbackUsed
	point.SuccessRate = successRateModelGatewayObservability(accumulator.Successes, accumulator.Attempts)
	point.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
	point.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
	point.QueueEnabledDispatches = accumulator.QueueEnabledDispatches
	point.QueuedDispatches = accumulator.QueuedDispatches
	point.QueueWaitCount = accumulator.QueueWaitCount
	point.AvgQueueWaitMs = averageInt64(accumulator.queueWaitSum, accumulator.queueWaitSamples)
	point.QueueWaitP50Ms = percentileModelGatewayObservabilityInt64(accumulator.queueWaitValues, 0.50)
	point.QueueWaitP90Ms = percentileModelGatewayObservabilityInt64(accumulator.queueWaitValues, 0.90)
	point.QueueWaitP95Ms = percentileModelGatewayObservabilityInt64(accumulator.queueWaitValues, 0.95)
	point.ResourceProtectionDispatches = accumulator.ResourceProtectionDispatches
	point.ResourceProtectionPrimaryHits = accumulator.ResourceProtectionPrimaryHits
	point.ResourceProtectionPrimaryWaits = accumulator.ResourceProtectionPrimaryWaits
	point.ResourceProtectionFallbacks = accumulator.ResourceProtectionFallbacks
	point.ResourceProtectionWaitTimeoutFallbacks = accumulator.ResourceProtectionWaitTimeoutFallbacks
	point.ResourceProtectionPrimaryFailureFallbacks = accumulator.ResourceProtectionPrimaryFailureFallbacks
	point.ResourceProtectionQueueDepth = accumulator.ResourceProtectionQueueDepth
	point.ResourceProtectionMaxQueueDepth = accumulator.ResourceProtectionMaxQueueDepth
	point.ResourceProtectionQueueCapacity = accumulator.ResourceProtectionQueueCapacity
	point.ResourceProtectionAvgWaitMs = averageInt64(accumulator.resourceProtectionWaitSum, accumulator.resourceProtectionWaitSamples)
	point.ResourceProtectionFallbackCostShare = modelGatewayResourceProtectionFallbackCostShare(accumulator.modelGatewayObservabilityAccumulator)
	point.StickyRoutes = accumulator.StickyRoutes
	point.StickyRetained = accumulator.StickyRetained
	point.StickyBroken = accumulator.StickyBroken
	point.CacheAffinityRoutes = accumulator.CacheAffinityRoutes
	point.OverloadSkipCount = accumulator.OverloadSkipCount
	point.AuthConfigErrorCount = accumulator.AuthConfigErrorCount
	point.UnknownErrorCount = accumulator.UnknownErrorCount
	point.ConfigErrorIsolatedCount = accumulator.ConfigErrorIsolatedCount
	point.ByProviderProfile = finalizeModelGatewayObservabilityAggregates(accumulator.ProviderProfileAccumulators, modelGatewayObservabilityTrendTopN)
	point.ByProxyMode = finalizeModelGatewayObservabilityAggregates(accumulator.ProxyModeAccumulators, modelGatewayObservabilityTrendTopN)
	point.RejectReasons = finalizeModelGatewayTrendReasonCounts(accumulator.RejectReasons, modelGatewayObservabilityTrendTopN)
	point.CircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(accumulator.CircuitOpenReasons, modelGatewayObservabilityTrendTopN)
	point.CircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(accumulator.CircuitErrorTypes, modelGatewayObservabilityTrendTopN)
	point.CircuitErrorCounts = point.CircuitErrorTypes
	return point
}

func applyModelGatewayTrendRiskTimeline(response *ModelGatewayObservabilityResponse) {
	if response == nil {
		return
	}
	timeline := make([]ModelGatewayRiskEvent, 0)
	previousStatus := "healthy"
	for idx := range response.Trends {
		point := &response.Trends[idx]
		if point.Records <= 0 {
			continue
		}
		currentStatus, currentSeverity := modelGatewayTrendRiskStatus(*point)
		if currentStatus != previousStatus {
			event := ModelGatewayRiskEvent{
				Timestamp:      point.BucketStart,
				BucketStart:    point.BucketStart,
				BucketEnd:      point.BucketEnd,
				Source:         "trend_bucket",
				EventType:      "risk_status_change",
				Severity:       currentSeverity,
				Status:         currentStatus,
				PreviousStatus: previousStatus,
				CurrentStatus:  currentStatus,
				Count:          1,
			}
			point.RiskEvents = append(point.RiskEvents, event)
			timeline = append(timeline, event)
			response.Summary.RiskStatusChanges++
		}
		previousStatus = currentStatus
		events := modelGatewayTrendDetailRiskEvents(*point)
		point.RiskEvents = append(point.RiskEvents, events...)
		timeline = append(timeline, events...)
		if len(point.RiskEvents) > 0 {
			point.RiskEvents = finalizeModelGatewayRiskTimeline(point.RiskEvents, 0)
			risk := buildModelGatewayRiskSnapshot(
				point.RiskEvents,
				int64(len(point.RiskEvents)),
				modelGatewayRiskStatusChangesFromEvents(point.RiskEvents),
				0,
				modelGatewayTrendRejectReasonCounts(point.RejectReasons),
				modelGatewayTrendReasonCountsToMap(point.CircuitOpenReasons),
				modelGatewayTrendReasonCountsToMap(point.CircuitErrorTypes),
			)
			point.Risk = &risk
		}
	}
	response.Summary.RiskEvents += int64(len(timeline))
	response.RiskTimeline = finalizeModelGatewayRiskTimeline(append(response.RiskTimeline, timeline...), modelGatewayObservabilityRiskTimelineLimit)
	syncModelGatewayRiskCompatibility(response)
}

func applyModelGatewayRuntimeRiskEvents(response *ModelGatewayObservabilityResponse) {
	if response == nil || len(response.RuntimeStatus.Items) == 0 {
		return
	}
	timestamp := response.RuntimeStatus.Summary.UpdatedAt
	if timestamp <= 0 {
		timestamp = common.GetTimestamp()
	}
	circuitOpenReasons := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitOpenReasons)
	circuitErrorTypes := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitErrorTypes)
	circuitSummaryChanged := false
	events := make([]ModelGatewayRiskEvent, 0)
	for _, item := range response.RuntimeStatus.Items {
		if reason := strings.TrimSpace(item.CircuitOpenReason); reason != "" {
			circuitOpenReasons[reason]++
			circuitSummaryChanged = true
		}
		for kind, count := range item.CircuitErrorCounts {
			kind = strings.TrimSpace(kind)
			if kind == "" || count <= 0 {
				continue
			}
			circuitErrorTypes[kind] += int64(count)
			circuitSummaryChanged = true
		}
		status := strings.TrimSpace(item.HealthStatus)
		if status == "" || status == "healthy" {
			continue
		}
		response.Summary.CurrentRiskRuntimeKeys++
		events = append(events, ModelGatewayRiskEvent{
			Timestamp:      timestamp,
			Source:         "runtime_status",
			EventType:      "current_runtime_status",
			Severity:       modelGatewayRiskSeverityForStatus(status),
			Status:         status,
			CurrentStatus:  status,
			Reason:         modelGatewayRuntimeRiskReason(item),
			Count:          1,
			RequestedModel: item.RequestedModel,
			UpstreamModel:  item.UpstreamModel,
			Group:          item.Group,
			ChannelID:      item.ChannelID,
			EndpointType:   item.EndpointType,
		})
	}
	if circuitSummaryChanged {
		response.Summary.CircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(circuitOpenReasons, modelGatewayObservabilityTrendTopN)
		response.Summary.CircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(circuitErrorTypes, modelGatewayObservabilityTrendTopN)
		response.Summary.CircuitErrorCounts = response.Summary.CircuitErrorTypes
	}
	if len(events) == 0 {
		if circuitSummaryChanged {
			syncModelGatewayRiskCompatibility(response)
		}
		return
	}
	response.Summary.RiskEvents += int64(len(events))
	response.RiskTimeline = finalizeModelGatewayRiskTimeline(append(response.RiskTimeline, events...), modelGatewayObservabilityRiskTimelineLimit)
	syncModelGatewayRiskCompatibility(response)
}

func modelGatewayTrendRiskStatus(point ModelGatewayObservabilityTrendPoint) (string, string) {
	switch {
	case point.StreamInterrupted > 0:
		return "stream_interrupted", "critical"
	case point.Attempts > 0 && point.Failures > 0 && point.SuccessRate < 0.5:
		return "failure_spike", "critical"
	case point.FallbackUsed > 0:
		return "fallback", "warning"
	case len(point.RejectReasons) > 0:
		return "candidate_rejected", "warning"
	case point.QueueWaitP95Ms > 0 || point.QueuedDispatches > 0:
		return "queued", "warning"
	case point.StickyBroken > 0:
		return "sticky_broken", "info"
	case point.Attempts > 0 && point.SuccessRate > 0 && point.SuccessRate < 0.8:
		return "degraded", "warning"
	default:
		return "healthy", "info"
	}
}

func modelGatewayTrendDetailRiskEvents(point ModelGatewayObservabilityTrendPoint) []ModelGatewayRiskEvent {
	events := make([]ModelGatewayRiskEvent, 0)
	base := ModelGatewayRiskEvent{
		Timestamp:   point.BucketStart,
		BucketStart: point.BucketStart,
		BucketEnd:   point.BucketEnd,
		Source:      "trend_bucket",
	}
	if point.StreamInterrupted > 0 {
		event := base
		event.EventType = "stream_interrupted"
		event.Severity = "critical"
		event.Status = "stream_interrupted"
		event.Count = point.StreamInterrupted
		events = append(events, event)
	}
	if point.FallbackUsed > 0 {
		event := base
		event.EventType = "fallback_used"
		event.Severity = "warning"
		event.Status = "fallback"
		event.Count = point.FallbackUsed
		events = append(events, event)
	}
	if point.Failures > 0 {
		event := base
		event.EventType = "attempt_failure"
		event.Severity = modelGatewayRiskSeverityForStatus("degraded")
		event.Status = "degraded"
		event.Count = point.Failures
		events = append(events, event)
	}
	if point.QueueWaitP95Ms > 0 || point.QueuedDispatches > 0 {
		event := base
		event.EventType = "queue_wait"
		event.Severity = "warning"
		event.Status = "queued"
		event.Count = point.QueuedDispatches
		if event.Count <= 0 {
			event.Count = 1
		}
		events = append(events, event)
	}
	if point.StickyBroken > 0 {
		event := base
		event.EventType = "sticky_break"
		event.Severity = "info"
		event.Status = "sticky_broken"
		event.Count = point.StickyBroken
		events = append(events, event)
	}
	for _, reason := range point.RejectReasons {
		if reason.Count <= 0 || strings.TrimSpace(reason.Reason) == "" {
			continue
		}
		event := base
		event.EventType = "reject_reason"
		event.Status = modelGatewayRiskStatusForRejectReason(reason.Reason)
		event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		event.Reason = reason.Reason
		event.Count = reason.Count
		events = append(events, event)
	}
	for _, reason := range point.CircuitOpenReasons {
		if reason.Count <= 0 || strings.TrimSpace(reason.Reason) == "" {
			continue
		}
		event := base
		event.EventType = "circuit_open_reason"
		event.Status = "circuit_open"
		event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		event.Reason = reason.Reason
		event.Count = reason.Count
		events = append(events, event)
	}
	for _, kind := range point.CircuitErrorTypes {
		if kind.Count <= 0 || strings.TrimSpace(kind.Reason) == "" {
			continue
		}
		event := base
		event.EventType = "circuit_error_type"
		event.Status = modelGatewayRiskStatusForCircuitErrorType(kind.Reason)
		event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		event.Reason = kind.Reason
		event.Count = kind.Count
		events = append(events, event)
	}
	sortModelGatewayRiskEvents(events)
	return events
}

func buildModelGatewayTrendExportPreview(response ModelGatewayObservabilityResponse) ModelGatewayTrendExportPreview {
	preview := ModelGatewayTrendExportPreview{
		BucketCount:            len(response.Trends),
		RiskEvents:             response.Summary.RiskEvents,
		RiskEventCount:         response.Summary.RiskEvents,
		RiskStatusChanges:      response.Summary.RiskStatusChanges,
		CurrentRiskRuntimeKeys: response.Summary.CurrentRiskRuntimeKeys,
	}
	rejectReasons := make(map[string]int64)
	circuitOpenReasons := make(map[string]int64)
	circuitErrorTypes := make(map[string]int64)
	for _, trend := range response.Trends {
		if trend.Records > 0 {
			preview.NonEmptyBuckets++
		}
		for _, reason := range trend.RejectReasons {
			rejectReasons[reason.Reason] += reason.Count
		}
		for _, reason := range trend.CircuitOpenReasons {
			circuitOpenReasons[reason.Reason] += reason.Count
		}
		for _, kind := range trend.CircuitErrorTypes {
			circuitErrorTypes[kind.Reason] += kind.Count
		}
	}
	for _, reason := range response.Summary.CircuitOpenReasons {
		if _, ok := circuitOpenReasons[reason.Reason]; !ok {
			circuitOpenReasons[reason.Reason] = reason.Count
		}
	}
	for _, kind := range response.Summary.CircuitErrorTypes {
		if _, ok := circuitErrorTypes[kind.Reason]; !ok {
			circuitErrorTypes[kind.Reason] = kind.Count
		}
	}
	preview.RiskTimeline = limitModelGatewayRiskEvents(response.RiskTimeline, modelGatewayTrendExportPreviewLimit)
	preview.TopRiskStatuses = finalizeModelGatewayRiskStatusCounts(response.RiskTimeline, modelGatewayObservabilityTrendTopN)
	preview.TopRejectReasons = finalizeModelGatewayTrendReasonCounts(rejectReasons, modelGatewayObservabilityTrendTopN)
	preview.TopCircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(circuitOpenReasons, modelGatewayObservabilityTrendTopN)
	preview.TopCircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(circuitErrorTypes, modelGatewayObservabilityTrendTopN)
	preview.Risk = buildModelGatewayRiskSnapshot(
		preview.RiskTimeline,
		preview.RiskEvents,
		preview.RiskStatusChanges,
		preview.CurrentRiskRuntimeKeys,
		rejectReasons,
		circuitOpenReasons,
		circuitErrorTypes,
	)
	return preview
}

func syncModelGatewayRiskCompatibility(response *ModelGatewayObservabilityResponse) {
	if response == nil {
		return
	}
	response.RiskTimeline = finalizeModelGatewayRiskTimeline(response.RiskTimeline, modelGatewayObservabilityRiskTimelineLimit)
	response.RiskEvents = limitModelGatewayRiskEvents(response.RiskTimeline, 0)
	if response.RiskEvents == nil {
		response.RiskEvents = []ModelGatewayRiskEvent{}
	}
	response.Risk = buildModelGatewayRiskSnapshot(
		response.RiskTimeline,
		response.Summary.RiskEvents,
		response.Summary.RiskStatusChanges,
		response.Summary.CurrentRiskRuntimeKeys,
		modelGatewayRejectReasonCountsFromTrends(response.Trends),
		modelGatewayCircuitOpenReasonCountsFromSummaryOrTrends(*response),
		modelGatewayCircuitErrorTypeCountsFromSummaryOrTrends(*response),
	)
}

func buildModelGatewayRiskSnapshot(events []ModelGatewayRiskEvent, eventCount int64, statusChanges int64, currentRuntimeKeys int, rejectReasons map[string]int64, circuitOpenReasons map[string]int64, circuitErrorTypes map[string]int64) ModelGatewayRiskSnapshot {
	timeline := limitModelGatewayRiskEvents(events, 0)
	if timeline == nil {
		timeline = []ModelGatewayRiskEvent{}
	}
	if eventCount <= 0 && len(timeline) > 0 {
		eventCount = int64(len(timeline))
	}
	topStatuses := finalizeModelGatewayRiskStatusCounts(timeline, modelGatewayObservabilityTrendTopN)
	topRejectReasons := finalizeModelGatewayTrendReasonCounts(rejectReasons, modelGatewayObservabilityTrendTopN)
	topCircuitOpenReasons := finalizeModelGatewayTrendReasonCounts(circuitOpenReasons, modelGatewayObservabilityTrendTopN)
	topCircuitErrorTypes := finalizeModelGatewayTrendReasonCounts(circuitErrorTypes, modelGatewayObservabilityTrendTopN)
	return ModelGatewayRiskSnapshot{
		EventCount:             eventCount,
		RiskEventCount:         eventCount,
		StatusChanges:          statusChanges,
		RiskStatusChanges:      statusChanges,
		CurrentRuntimeKeys:     currentRuntimeKeys,
		CurrentRiskRuntimeKeys: currentRuntimeKeys,
		Events:                 timeline,
		RiskEvents:             timeline,
		Timeline:               timeline,
		RiskTimeline:           timeline,
		TopStatuses:            topStatuses,
		TopRiskStatuses:        topStatuses,
		TopRejectReasons:       topRejectReasons,
		TopCircuitOpenReasons:  topCircuitOpenReasons,
		TopCircuitErrorTypes:   topCircuitErrorTypes,
	}
}

func modelGatewayRiskStatusChangesFromEvents(events []ModelGatewayRiskEvent) int64 {
	var count int64
	for _, event := range events {
		if event.EventType == "risk_status_change" {
			count++
		}
	}
	return count
}

func modelGatewayRejectReasonCountsFromTrends(trends []ModelGatewayObservabilityTrendPoint) map[string]int64 {
	counts := make(map[string]int64)
	for _, trend := range trends {
		for _, reason := range trend.RejectReasons {
			if strings.TrimSpace(reason.Reason) == "" || reason.Count <= 0 {
				continue
			}
			counts[reason.Reason] += reason.Count
		}
	}
	return counts
}

func modelGatewayCircuitOpenReasonCountsFromTrends(trends []ModelGatewayObservabilityTrendPoint) map[string]int64 {
	counts := make(map[string]int64)
	for _, trend := range trends {
		for _, reason := range trend.CircuitOpenReasons {
			if strings.TrimSpace(reason.Reason) == "" || reason.Count <= 0 {
				continue
			}
			counts[reason.Reason] += reason.Count
		}
	}
	return counts
}

func modelGatewayCircuitErrorTypeCountsFromTrends(trends []ModelGatewayObservabilityTrendPoint) map[string]int64 {
	counts := make(map[string]int64)
	for _, trend := range trends {
		for _, kind := range trend.CircuitErrorTypes {
			if strings.TrimSpace(kind.Reason) == "" || kind.Count <= 0 {
				continue
			}
			counts[kind.Reason] += kind.Count
		}
	}
	return counts
}

func modelGatewayCircuitOpenReasonCountsFromSummaryOrTrends(response ModelGatewayObservabilityResponse) map[string]int64 {
	counts := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitOpenReasons)
	if len(counts) == 0 {
		return modelGatewayCircuitOpenReasonCountsFromTrends(response.Trends)
	}
	return counts
}

func modelGatewayCircuitErrorTypeCountsFromSummaryOrTrends(response ModelGatewayObservabilityResponse) map[string]int64 {
	counts := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitErrorTypes)
	if len(counts) == 0 {
		return modelGatewayCircuitErrorTypeCountsFromTrends(response.Trends)
	}
	return counts
}

func modelGatewayTrendRejectReasonCounts(reasons []ModelGatewayTrendReasonCount) map[string]int64 {
	return modelGatewayTrendReasonCountsToMap(reasons)
}

func modelGatewayTrendReasonCountsToMap(reasons []ModelGatewayTrendReasonCount) map[string]int64 {
	counts := make(map[string]int64)
	for _, reason := range reasons {
		if strings.TrimSpace(reason.Reason) == "" || reason.Count <= 0 {
			continue
		}
		counts[reason.Reason] += reason.Count
	}
	return counts
}

func modelGatewayRuntimeRiskReason(item modelgatewayobservability.RuntimeStatusItem) string {
	switch item.HealthStatus {
	case "circuit_open":
		if reason := strings.TrimSpace(item.CircuitOpenReason); reason != "" {
			return reason
		}
		return "circuit_open"
	case "cooldown":
		return strings.TrimSpace(item.CooldownReason)
	case "failure_avoidance":
		if strings.TrimSpace(item.FailureAvoidanceReason) == service.ChannelTimeoutRecoveryReason ||
			strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason {
			return service.ChannelTimeoutRecoveryReason
		}
		if strings.TrimSpace(item.FailureAvoidanceReason) == service.ChannelOverloadRecoveryReason ||
			strings.TrimSpace(item.ProbeTriggerReason) == service.ChannelOverloadRecoveryReason {
			return service.ChannelOverloadRecoveryReason
		}
		return strings.TrimSpace(item.FailureAvoidanceReason)
	case "queued":
		return "queue_depth"
	case "high_pressure":
		return "concurrency_pressure"
	case "degraded":
		return "success_rate_below_threshold"
	default:
		return ""
	}
}

func modelGatewayRiskStatusForRejectReason(reason string) string {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case strings.Contains(normalized, "circuit"):
		return "circuit_open"
	case strings.Contains(normalized, "cooldown"):
		return "cooldown"
	case strings.Contains(normalized, "timeout_recovery"):
		return "failure_avoidance"
	case strings.Contains(normalized, "overload_recovery"):
		return "failure_avoidance"
	case strings.Contains(normalized, "avoid"):
		return "failure_avoidance"
	case strings.Contains(normalized, "concurrency") || strings.Contains(normalized, "queue"):
		return "queued"
	case strings.Contains(normalized, "fallback"):
		return "fallback"
	default:
		return "candidate_rejected"
	}
}

func modelGatewayRiskStatusForCircuitErrorType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case modelgatewayscheduler.CircuitErrorStreamInterrupted:
		return "stream_interrupted"
	case modelgatewayscheduler.CircuitErrorRateLimit, modelgatewayscheduler.CircuitErrorServer, modelgatewayscheduler.CircuitErrorUpstream:
		return "circuit_error"
	default:
		return "candidate_rejected"
	}
}

func modelGatewayRiskSeverityForStatus(status string) string {
	switch status {
	case "stream_interrupted", "failure_spike", "circuit_open":
		return "critical"
	case "fallback", "candidate_rejected", "queued", "high_pressure", "cooldown", "failure_avoidance", "degraded", "circuit_error":
		return "warning"
	default:
		return "info"
	}
}

func finalizeModelGatewayRiskTimeline(events []ModelGatewayRiskEvent, limit int) []ModelGatewayRiskEvent {
	if len(events) == 0 {
		return []ModelGatewayRiskEvent{}
	}
	filtered := make([]ModelGatewayRiskEvent, 0, len(events))
	for _, event := range events {
		if event.Count <= 0 {
			event.Count = 1
		}
		if strings.TrimSpace(event.Status) == "" || strings.TrimSpace(event.EventType) == "" {
			continue
		}
		if strings.TrimSpace(event.Severity) == "" {
			event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		}
		filtered = append(filtered, event)
	}
	sortModelGatewayRiskEvents(filtered)
	return limitModelGatewayRiskEvents(filtered, limit)
}

func limitModelGatewayRiskEvents(events []ModelGatewayRiskEvent, limit int) []ModelGatewayRiskEvent {
	if len(events) == 0 {
		return nil
	}
	if limit > 0 && len(events) > limit {
		return append([]ModelGatewayRiskEvent(nil), events[:limit]...)
	}
	return append([]ModelGatewayRiskEvent(nil), events...)
}

func sortModelGatewayRiskEvents(events []ModelGatewayRiskEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp != events[j].Timestamp {
			return events[i].Timestamp > events[j].Timestamp
		}
		leftSeverity := modelGatewayRiskSeverityRank(events[i].Severity)
		rightSeverity := modelGatewayRiskSeverityRank(events[j].Severity)
		if leftSeverity != rightSeverity {
			return leftSeverity > rightSeverity
		}
		if events[i].Count != events[j].Count {
			return events[i].Count > events[j].Count
		}
		if events[i].Status != events[j].Status {
			return events[i].Status < events[j].Status
		}
		return events[i].EventType < events[j].EventType
	})
}

func finalizeModelGatewayRiskStatusCounts(events []ModelGatewayRiskEvent, topN int) []ModelGatewayRiskStatusCount {
	if len(events) == 0 {
		return nil
	}
	counts := make(map[string]int64)
	severities := make(map[string]string)
	for _, event := range events {
		status := strings.TrimSpace(event.Status)
		if status == "" {
			continue
		}
		count := event.Count
		if count <= 0 {
			count = 1
		}
		counts[status] += count
		severities[status] = modelGatewayRiskSeverityForStatus(status)
		if event.Severity != "" && modelGatewayRiskSeverityRank(event.Severity) > modelGatewayRiskSeverityRank(severities[status]) {
			severities[status] = event.Severity
		}
	}
	items := make([]ModelGatewayRiskStatusCount, 0, len(counts))
	for status, count := range counts {
		if count <= 0 {
			continue
		}
		items = append(items, ModelGatewayRiskStatusCount{
			Status:   status,
			Severity: severities[status],
			Count:    count,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		leftSeverity := modelGatewayRiskSeverityRank(items[i].Severity)
		rightSeverity := modelGatewayRiskSeverityRank(items[j].Severity)
		if leftSeverity != rightSeverity {
			return leftSeverity > rightSeverity
		}
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Status < items[j].Status
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func modelGatewayRiskSeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func modelGatewayObservabilityAccumulatorFor(accumulators map[string]*modelGatewayObservabilityAccumulator, key string, name string, channelID int) *modelGatewayObservabilityAccumulator {
	if key == "" {
		key = "unknown"
	}
	if existing, ok := accumulators[key]; ok {
		if existing.Name == "" && name != "" {
			existing.Name = name
		}
		if existing.ChannelID == 0 && channelID > 0 {
			existing.ChannelID = channelID
		}
		return existing
	}
	accumulator := newModelGatewayObservabilityAccumulator(key, name, channelID)
	accumulators[key] = accumulator
	return accumulator
}

func modelGatewayObservabilityTrendAccumulatorFor(accumulators map[int64]*modelGatewayObservabilityTrendAccumulator, bucketStart int64) *modelGatewayObservabilityTrendAccumulator {
	if existing, ok := accumulators[bucketStart]; ok {
		return existing
	}
	accumulator := &modelGatewayObservabilityTrendAccumulator{
		modelGatewayObservabilityAccumulator: newModelGatewayObservabilityAccumulator(strconv.FormatInt(bucketStart, 10), "", 0),
		ProviderProfileAccumulators:          make(map[string]*modelGatewayObservabilityAccumulator),
		ProxyModeAccumulators:                make(map[string]*modelGatewayObservabilityAccumulator),
		RejectReasons:                        make(map[string]int64),
		CircuitOpenReasons:                   make(map[string]int64),
		CircuitErrorTypes:                    make(map[string]int64),
	}
	accumulators[bucketStart] = accumulator
	return accumulator
}

func applyModelGatewayObservabilityTrendRecord(accumulator *modelGatewayObservabilityTrendAccumulator, record model.ModelExecutionRecord, scoreBreakdown map[string]float64, requestMeta map[string]any, scoreErr bool, profileProxyMeta modelGatewayProfileProxyMeta) {
	if accumulator == nil {
		return
	}
	applyModelGatewayObservabilityRecord(accumulator.modelGatewayObservabilityAccumulator, record, scoreBreakdown, requestMeta, scoreErr)
	applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(accumulator.ProviderProfileAccumulators, profileProxyMeta.ProviderProfile, "", 0), record, scoreBreakdown, requestMeta, scoreErr)
	applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(accumulator.ProxyModeAccumulators, profileProxyMeta.ProxyMode, "", 0), record, scoreBreakdown, requestMeta, scoreErr)
	for _, reason := range modelGatewayRejectReasonsFromRequestMeta(requestMeta) {
		accumulator.RejectReasons[reason]++
		if modelGatewayRiskStatusForRejectReason(reason) == "circuit_open" {
			accumulator.CircuitOpenReasons[reason]++
		}
	}
	if kind := modelGatewayCircuitErrorTypeFromRecord(record); kind != "" {
		accumulator.CircuitErrorTypes[kind]++
	}
}

func modelGatewayCircuitErrorTypeFromRecord(record model.ModelExecutionRecord) string {
	if !isModelGatewayAttemptRecord(record) || record.Success {
		return ""
	}
	requestMeta, _ := parseModelGatewayRequestMeta(record.RequestMeta)
	attemptMeta := modelGatewayObservabilityAttemptMetaFromRequestMeta(requestMeta)
	if attemptMeta.ClientAborted || attemptMeta.BalanceInsufficient {
		return ""
	}
	result := modelgatewaycore.AttemptResult{
		ChannelID:           record.ChannelId,
		RequestedGroup:      record.RequestedGroup,
		SelectedGroup:       record.SelectedGroup,
		ModelName:           record.RequestedModel,
		EndpointType:        constantEndpointTypeFromString(record.EndpointType),
		Success:             record.Success,
		StatusCode:          record.StatusCode,
		ErrorCode:           record.ErrorCode,
		ErrorType:           record.ErrorType,
		ErrorMessage:        attemptMeta.ErrorMessage,
		ConcurrencyLimited:  attemptMeta.ConcurrencyLimited,
		BalanceInsufficient: attemptMeta.BalanceInsufficient,
		StreamInterrupted:   record.StreamInterrupted,
	}
	kind := modelgatewayscheduler.ClassifyCircuitError(result)
	if kind == modelgatewayscheduler.CircuitErrorConcurrencyLimit {
		return ""
	}
	return kind
}

func constantEndpointTypeFromString(value string) constant.EndpointType {
	return constant.EndpointType(strings.TrimSpace(value))
}

func finalizeModelGatewayTrendReasonCounts(counts map[string]int64, topN int) []ModelGatewayTrendReasonCount {
	if len(counts) == 0 {
		return nil
	}
	items := make([]ModelGatewayTrendReasonCount, 0, len(counts))
	for reason, count := range counts {
		if strings.TrimSpace(reason) == "" || count <= 0 {
			continue
		}
		items = append(items, ModelGatewayTrendReasonCount{
			Reason: reason,
			Count:  count,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Reason < items[j].Reason
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func newModelGatewayObservabilityAccumulator(key string, name string, channelID int) *modelGatewayObservabilityAccumulator {
	return &modelGatewayObservabilityAccumulator{
		ModelGatewayObservabilityAggregate: ModelGatewayObservabilityAggregate{
			Key:       key,
			Name:      name,
			ChannelID: channelID,
		},
	}
}

func parseModelGatewayScoreBreakdown(raw string) (map[string]float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	values := make(map[string]float64)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func parseModelGatewayCandidateGroups(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	values := make([]string, 0)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func parseModelGatewayRequestMeta(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	values := make(map[string]any)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func modelGatewayObservabilityTrendBucketSeconds(hours int, overrideSeconds int64) int64 {
	if overrideSeconds > 0 {
		return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, overrideSeconds)
	}
	if hours <= 6 {
		return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, 3600)
	}
	if hours <= 72 {
		return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, 6*3600)
	}
	return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, 24*3600)
}

func modelGatewayObservabilityNormalizeTrendBucketSeconds(hours int, bucketSeconds int64) int64 {
	if bucketSeconds < modelGatewayObservabilityMinTrendBucket {
		bucketSeconds = modelGatewayObservabilityMinTrendBucket
	}
	if bucketSeconds > modelGatewayObservabilityMaxTrendBucket {
		bucketSeconds = modelGatewayObservabilityMaxTrendBucket
	}
	windowSeconds := int64(hours) * 3600
	if windowSeconds <= 0 {
		return bucketSeconds
	}
	minBucketForWindow := (windowSeconds + modelGatewayObservabilityMaxTrendBuckets - 1) / modelGatewayObservabilityMaxTrendBuckets
	if bucketSeconds < minBucketForWindow {
		bucketSeconds = minBucketForWindow
	}
	return bucketSeconds
}

func modelGatewayObservabilityTrendBucketStart(createdAt int64, startTime int64, endTime int64, bucketSeconds int64) (int64, bool) {
	if createdAt < startTime || endTime <= startTime || bucketSeconds <= 0 {
		return 0, false
	}
	if createdAt >= endTime {
		createdAt = endTime - 1
	}
	if createdAt < startTime {
		return 0, false
	}
	return startTime + ((createdAt-startTime)/bucketSeconds)*bucketSeconds, true
}

func averageModelGatewayObservabilityScoreBreakdown(accumulator *modelGatewayObservabilityAccumulator) map[string]float64 {
	if accumulator == nil || accumulator.ScoreBreakdownSamples == 0 || len(accumulator.scoreSums) == 0 {
		return nil
	}
	average := make(map[string]float64, len(accumulator.scoreSums))
	for key, value := range accumulator.scoreSums {
		average[key] = roundModelGatewayObservabilityFloat(value / float64(accumulator.ScoreBreakdownSamples))
	}
	return average
}

func modelGatewayObservabilityModelKey(record model.ModelExecutionRecord) string {
	if record.RequestedModel != "" {
		return record.RequestedModel
	}
	return "unknown"
}

func modelGatewayObservabilityGroupKey(record model.ModelExecutionRecord) string {
	if record.ActualGroup != "" {
		return record.ActualGroup
	}
	if record.SelectedGroup != "" {
		return record.SelectedGroup
	}
	if record.RequestedGroup != "" {
		return record.RequestedGroup
	}
	return "unknown"
}

func modelGatewayObservabilityChannel(record model.ModelExecutionRecord) (int, string) {
	if record.ActualChannelId > 0 {
		return record.ActualChannelId, record.ActualChannelName
	}
	if record.ChannelId > 0 {
		return record.ChannelId, record.ChannelName
	}
	return 0, ""
}

func modelGatewayObservabilityProviderProfileKey(requestMeta map[string]any) string {
	key := strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["provider_profile"]))
	if key == "" {
		return "unknown"
	}
	return key
}

func modelGatewayObservabilityProxyModeKey(requestMeta map[string]any) string {
	key := strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["proxy_mode"]))
	if key == "" {
		return "unknown"
	}
	return key
}

func modelGatewayProfileProxyMetaForRecord(record model.ModelExecutionRecord, requestMeta map[string]any, dispatchMetaByRequestID map[string]modelGatewayProfileProxyMeta) modelGatewayProfileProxyMeta {
	meta := modelGatewayProfileProxyMeta{
		ProviderProfile: modelGatewayObservabilityProviderProfileKey(requestMeta),
		ProxyMode:       modelGatewayObservabilityProxyModeKey(requestMeta),
	}
	if meta.ProviderProfile != "unknown" || meta.ProxyMode != "unknown" {
		return meta
	}
	if strings.TrimSpace(record.RequestId) == "" {
		return meta
	}
	if dispatchMeta, ok := dispatchMetaByRequestID[record.RequestId]; ok {
		if meta.ProviderProfile == "unknown" {
			meta.ProviderProfile = dispatchMeta.ProviderProfile
		}
		if meta.ProxyMode == "unknown" {
			meta.ProxyMode = dispatchMeta.ProxyMode
		}
	}
	return meta
}

func modelGatewayObservabilityRecordKind(record model.ModelExecutionRecord) string {
	if isModelGatewayAttemptRecord(record) {
		return "attempt"
	}
	if isModelGatewayDispatchRecord(record) {
		return "dispatch"
	}
	return "record"
}

func isModelGatewayDispatchRecord(record model.ModelExecutionRecord) bool {
	if isModelGatewayAttemptRecord(record) {
		return false
	}
	return record.SmartHandled ||
		record.PolicyMode != "" ||
		record.AutoMode != "" ||
		record.Strategy != "" ||
		record.ScoreTotal > 0 ||
		strings.TrimSpace(record.ScoreBreakdown) != "" ||
		strings.TrimSpace(record.CandidateGroups) != ""
}

func isModelGatewayAttemptRecord(record model.ModelExecutionRecord) bool {
	return record.Success ||
		record.StreamInterrupted ||
		record.StatusCode != 0 ||
		record.ErrorCode != "" ||
		record.ErrorType != "" ||
		record.DurationMs != 0 ||
		record.TTFTMs != 0
}

func hasModelGatewayScoreTotalSample(record model.ModelExecutionRecord) bool {
	return record.ScoreTotal != 0 ||
		strings.TrimSpace(record.ScoreBreakdown) != "" ||
		record.SelectedReason != ""
}

func successRateModelGatewayObservability(successes int64, attempts int64) float64 {
	if attempts <= 0 {
		return 0
	}
	if successes < 0 {
		successes = 0
	}
	if successes > attempts {
		successes = attempts
	}
	return roundModelGatewayObservabilityFloat(float64(successes) / float64(attempts))
}

func averageInt64(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return int64(math.Round(float64(sum) / float64(count)))
}

func percentileModelGatewayObservabilityInt64(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sortedValues := append([]int64(nil), values...)
	sort.Slice(sortedValues, func(i, j int) bool {
		return sortedValues[i] < sortedValues[j]
	})
	if percentile <= 0 {
		return sortedValues[0]
	}
	if percentile >= 1 {
		return sortedValues[len(sortedValues)-1]
	}
	index := int(math.Ceil(percentile*float64(len(sortedValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

func averageFloat64(sum float64, count int64) float64 {
	if count <= 0 {
		return 0
	}
	return roundModelGatewayObservabilityFloat(sum / float64(count))
}

func roundModelGatewayObservabilityFloat(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func clampModelGatewayObservabilityFloat(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampModelGatewayObservabilityValue(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeModelGatewayObservabilityScanLimit(value int) int {
	if value <= 0 {
		return modelGatewayObservabilityDefaultScanLimit
	}
	return clampModelGatewayObservabilityValue(value, 1, modelGatewayObservabilityMaxScanLimit)
}

func minModelGatewayObservabilityInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxModelGatewayObservabilityInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
