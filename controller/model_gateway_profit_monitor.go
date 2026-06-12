package controller

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewaytraffic "github.com/QuantumNous/new-api/pkg/modelgateway/traffic"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	modelGatewayProfitMonitorConfigOptionKey        = "model_gateway.profit_monitor.config"
	modelGatewayProfitRecommendationMinRequests     = 20
	modelGatewayProfitRecommendationMinSuccesses    = 5
	modelGatewayProfitRecommendationMinTotalTokens  = 1000
	modelGatewayProfitRecommendationReasonOK        = "ok"
	modelGatewayProfitRecommendationReasonNoCost    = "no_cost_data"
	modelGatewayProfitRecommendationReasonNoRevenue = "insufficient_revenue_data"
	modelGatewayProfitRecommendationReasonDisabled  = "recommendation_disabled"
	modelGatewayProfitRecommendationReasonLowSample = "insufficient_sample"
)

var modelGatewayProfitCanaryDefaultWatchMetrics = []string{
	"gross_margin",
	"success_rate",
	"traffic_cost_usd",
	"resource_cost_usd",
	"revenue_gap_usd",
	"request_count",
}

type ModelGatewayProfitMonitorConfig struct {
	Enabled                        bool    `json:"enabled"`
	ServerDailyCostUSD             float64 `json:"server_daily_cost_usd"`
	TrafficCostPerGBUSD            float64 `json:"traffic_cost_per_gb_usd"`
	TrafficEstimationEnabled       bool    `json:"traffic_estimation_enabled"`
	TrafficEstimatedBytesPerToken  int     `json:"traffic_estimated_bytes_per_token"`
	ResourceCostEnabled            bool    `json:"resource_cost_enabled"`
	TargetProfitRate               float64 `json:"target_profit_rate"`
	DynamicRatioMinLimit           float64 `json:"dynamic_ratio_min_limit"`
	DynamicRatioMaxLimit           float64 `json:"dynamic_ratio_max_limit"`
	DynamicRatioMaxLimitUpdatedAt  int64   `json:"dynamic_ratio_max_limit_updated_at,omitempty"`
	DynamicRatioFixedValue         float64 `json:"dynamic_ratio_fixed_value"`
	DynamicRatioRecommendationMode string  `json:"dynamic_ratio_recommendation_mode"`
}

type UpdateModelGatewayProfitMonitorConfigRequest struct {
	Enabled                        *bool    `json:"enabled,omitempty"`
	ServerDailyCostUSD             *float64 `json:"server_daily_cost_usd,omitempty"`
	TrafficCostPerGBUSD            *float64 `json:"traffic_cost_per_gb_usd,omitempty"`
	TrafficEstimationEnabled       *bool    `json:"traffic_estimation_enabled,omitempty"`
	TrafficEstimatedBytesPerToken  *int     `json:"traffic_estimated_bytes_per_token,omitempty"`
	ResourceCostEnabled            *bool    `json:"resource_cost_enabled,omitempty"`
	TargetProfitRate               *float64 `json:"target_profit_rate,omitempty"`
	DynamicRatioMinLimit           *float64 `json:"dynamic_ratio_min_limit,omitempty"`
	DynamicRatioMaxLimit           *float64 `json:"dynamic_ratio_max_limit,omitempty"`
	DynamicRatioFixedValue         *float64 `json:"dynamic_ratio_fixed_value,omitempty"`
	DynamicRatioRecommendationMode string   `json:"dynamic_ratio_recommendation_mode,omitempty"`
}

type ModelGatewayProfitMonitorResponse struct {
	Window              string                                `json:"window"`
	Dimension           string                                `json:"dimension"`
	BreakdownDimension  string                                `json:"breakdown_dimension"`
	StartTimestamp      int64                                 `json:"start_timestamp"`
	EndTimestamp        int64                                 `json:"end_timestamp"`
	Config              ModelGatewayProfitMonitorConfig       `json:"config"`
	Summary             ModelGatewayProfitMonitorSummary      `json:"summary"`
	Breakdown           []ModelGatewayProfitMonitorBreakdown  `json:"breakdown"`
	DynamicRatioGroups  []ModelGatewayProfitDynamicRatioGroup `json:"dynamic_ratio_groups"`
	DynamicRatioSummary ModelGatewayProfitDynamicRatioSummary `json:"dynamic_ratio_summary"`
	Anomalies           []ModelGatewayProfitAnomaly           `json:"anomalies"`
	Resources           ModelGatewayProfitResourceSummary     `json:"resources"`
	Recommendation      ModelGatewayProfitRecommendation      `json:"recommendation"`
}

type ModelGatewayProfitTrafficResponse struct {
	Window         string                                  `json:"window"`
	Dimension      string                                  `json:"dimension"`
	StartTimestamp int64                                   `json:"start_timestamp"`
	EndTimestamp   int64                                   `json:"end_timestamp"`
	Config         ModelGatewayProfitMonitorConfig         `json:"config"`
	Summary        ModelGatewayProfitTrafficSummary        `json:"summary"`
	Breakdown      []ModelGatewayProfitTrafficBreakdown    `json:"breakdown"`
	Series         []ModelGatewayProfitTrafficBucketSeries `json:"series"`
}

type ModelGatewayProfitRecommendationSnapshotInput struct {
	Window         string                            `json:"window"`
	Dimension      string                            `json:"dimension"`
	ScopeType      string                            `json:"scope_type,omitempty"`
	ScopeID        int                               `json:"scope_id,omitempty"`
	ScopeKey       string                            `json:"scope_key,omitempty"`
	ScopeName      string                            `json:"scope_name,omitempty"`
	StartTimestamp int64                             `json:"start_timestamp"`
	EndTimestamp   int64                             `json:"end_timestamp"`
	Config         ModelGatewayProfitMonitorConfig   `json:"config"`
	Summary        ModelGatewayProfitMonitorSummary  `json:"summary"`
	Resources      ModelGatewayProfitResourceSummary `json:"resources"`
	Recommendation ModelGatewayProfitRecommendation  `json:"recommendation"`
}

type ModelGatewayProfitRecommendationPackage struct {
	Mode                 string             `json:"mode"`
	GeneratedBy          string             `json:"generated_by"`
	ScopeType            string             `json:"scope_type,omitempty"`
	ScopeID              int                `json:"scope_id,omitempty"`
	ScopeKey             string             `json:"scope_key,omitempty"`
	ScopeName            string             `json:"scope_name,omitempty"`
	RiskLevel            string             `json:"risk_level"`
	Confidence           float64            `json:"confidence"`
	ReasonCode           string             `json:"reason_code"`
	ConstraintCodes      []string           `json:"constraint_codes"`
	SuggestedActionCodes []string           `json:"suggested_action_codes"`
	Metrics              map[string]float64 `json:"metrics"`
}

type ModelGatewayProfitMonitorSummary struct {
	Requests                   int64   `json:"requests"`
	SuccessRequests            int64   `json:"success_requests"`
	SuccessRate                float64 `json:"success_rate"`
	PromptTokens               int64   `json:"prompt_tokens"`
	CompletionTokens           int64   `json:"completion_tokens"`
	TotalTokens                int64   `json:"total_tokens"`
	BillingQuota               int64   `json:"billing_quota"`
	RevenueUSD                 float64 `json:"revenue_usd"`
	UpstreamCostUSD            float64 `json:"upstream_cost_usd"`
	TrafficRequestBytes        int64   `json:"traffic_request_bytes"`
	TrafficResponseBytes       int64   `json:"traffic_response_bytes"`
	TrafficBytes               int64   `json:"traffic_bytes"`
	TrafficCostUSD             float64 `json:"traffic_cost_usd"`
	TrafficEstimated           bool    `json:"traffic_estimated"`
	TrafficDataReady           bool    `json:"traffic_data_ready"`
	ServerCostUSD              float64 `json:"server_cost_usd"`
	ResourceAmortizedCostUSD   float64 `json:"resource_amortized_cost_usd"`
	ResourceLossCostUSD        float64 `json:"resource_loss_cost_usd"`
	OperatingCostUSD           float64 `json:"operating_cost_usd"`
	ProfitUSD                  float64 `json:"profit_usd"`
	GrossMargin                float64 `json:"gross_margin"`
	UpstreamOnlyProfitUSD      float64 `json:"upstream_only_profit_usd"`
	UpstreamOnlyMargin         float64 `json:"upstream_only_margin"`
	CostCoverageFloorUSD       float64 `json:"cost_coverage_floor_usd"`
	CostCoverageFloorPerMToken float64 `json:"cost_coverage_floor_per_m_token"`
}

type ModelGatewayProfitTrafficSummary struct {
	RequestCount     int64   `json:"request_count"`
	RequestBytes     int64   `json:"request_bytes"`
	ResponseBytes    int64   `json:"response_bytes"`
	TotalBytes       int64   `json:"total_bytes"`
	TrafficCostUSD   float64 `json:"traffic_cost_usd"`
	TrafficCostPerGB float64 `json:"traffic_cost_per_gb"`
	DataReady        bool    `json:"data_ready"`
}

type ModelGatewayProfitTrafficBreakdown struct {
	DimensionID    int     `json:"dimension_id"`
	DimensionKey   string  `json:"dimension_key"`
	DimensionName  string  `json:"dimension_name"`
	RequestCount   int64   `json:"request_count"`
	RequestBytes   int64   `json:"request_bytes"`
	ResponseBytes  int64   `json:"response_bytes"`
	TotalBytes     int64   `json:"total_bytes"`
	TrafficCostUSD float64 `json:"traffic_cost_usd"`
	Share          float64 `json:"share"`
}

type ModelGatewayProfitTrafficBucketSeries struct {
	BucketTs       int64   `json:"bucket_ts"`
	RequestCount   int64   `json:"request_count"`
	RequestBytes   int64   `json:"request_bytes"`
	ResponseBytes  int64   `json:"response_bytes"`
	TotalBytes     int64   `json:"total_bytes"`
	TrafficCostUSD float64 `json:"traffic_cost_usd"`
}

type ModelGatewayProfitMonitorBreakdown struct {
	DimensionID               int     `json:"dimension_id"`
	DimensionKey              string  `json:"dimension_key"`
	DimensionName             string  `json:"dimension_name"`
	Requests                  int64   `json:"requests"`
	SuccessRequests           int64   `json:"success_requests"`
	SuccessRate               float64 `json:"success_rate"`
	TotalTokens               int64   `json:"total_tokens"`
	BillingQuota              int64   `json:"billing_quota"`
	RevenueUSD                float64 `json:"revenue_usd"`
	UpstreamCostUSD           float64 `json:"upstream_cost_usd"`
	TrafficBytes              int64   `json:"traffic_bytes"`
	TrafficCostUSD            float64 `json:"traffic_cost_usd"`
	AllocatedOperatingCostUSD float64 `json:"allocated_operating_cost_usd"`
	ProfitUSD                 float64 `json:"profit_usd"`
	GrossMargin               float64 `json:"gross_margin"`
}

type ModelGatewayProfitDynamicRatioGroup struct {
	Group                    string  `json:"group"`
	BillingRatioMode         string  `json:"billing_ratio_mode"`
	StaticRatio              float64 `json:"static_ratio"`
	TargetRatio              float64 `json:"target_ratio"`
	EffectiveRatio           float64 `json:"effective_ratio"`
	DynamicRatio             float64 `json:"dynamic_ratio"`
	ActualRatio              float64 `json:"actual_ratio"`
	Applied                  bool    `json:"applied"`
	FallbackReason           string  `json:"fallback_reason"`
	ApplyReason              string  `json:"apply_reason"`
	Clamped                  bool    `json:"clamped"`
	PendingManualConfirm     bool    `json:"pending_manual_confirm"`
	SampleCount              int     `json:"sample_count"`
	ModelCount               int     `json:"model_count"`
	RequestCount             int64   `json:"request_count"`
	SuccessRequestCount      int64   `json:"success_request_count"`
	TotalTokens              int64   `json:"total_tokens"`
	CurrentRevenueUSD        float64 `json:"current_revenue_usd"`
	RequiredRevenueUSD       float64 `json:"required_revenue_usd"`
	RevenueGapUSD            float64 `json:"revenue_gap_usd"`
	UpstreamCostUSD          float64 `json:"upstream_cost_usd"`
	TrafficCostUSD           float64 `json:"traffic_cost_usd"`
	ServerCostUSD            float64 `json:"server_cost_usd"`
	ResourceCostUSD          float64 `json:"resource_cost_usd"`
	OperatingCostUSD         float64 `json:"operating_cost_usd"`
	DynamicRatioLimitMin     float64 `json:"dynamic_ratio_limit_min"`
	DynamicRatioLimitMax     float64 `json:"dynamic_ratio_limit_max"`
	DynamicRatioFixedValue   float64 `json:"dynamic_ratio_fixed_value"`
	DynamicRatioFixedApplied bool    `json:"dynamic_ratio_fixed_applied"`
	BaseQuotaAtRatio1        float64 `json:"base_quota_at_ratio_1"`
	CostMultiplier           float64 `json:"cost_multiplier"`
	CostMarkupMultiplier     float64 `json:"cost_markup_multiplier"`
	ReferenceModel           string  `json:"reference_model"`
	CostSource               string  `json:"cost_source"`
	ApplyMode                string  `json:"apply_mode"`
	ProfitRate               float64 `json:"profit_rate"`
	TrafficEstimated         bool    `json:"traffic_estimated"`
	TrafficDataReady         bool    `json:"traffic_data_ready"`
	WindowStart              int64   `json:"window_start"`
	WindowEnd                int64   `json:"window_end"`
	CalculatedAt             int64   `json:"calculated_at"`
	UpdatedAt                int64   `json:"updated_at"`
}

type ModelGatewayProfitDynamicRatioSummary struct {
	TotalGroups                int     `json:"total_groups"`
	ActiveGroups               int     `json:"active_groups"`
	FallbackGroups             int     `json:"fallback_groups"`
	ClampedGroups              int     `json:"clamped_groups"`
	PendingManualConfirmGroups int     `json:"pending_manual_confirm_groups"`
	OperatingCostUSD           float64 `json:"operating_cost_usd"`
	RequiredRevenueUSD         float64 `json:"required_revenue_usd"`
	RevenueGapUSD              float64 `json:"revenue_gap_usd"`
}

type ModelGatewayProfitAnomaly struct {
	ChannelID         int     `json:"channel_id"`
	ChannelName       string  `json:"channel_name"`
	RequestedModel    string  `json:"requested_model"`
	Requests          int64   `json:"requests"`
	TotalTokens       int64   `json:"total_tokens"`
	BillingQuota      int64   `json:"billing_quota"`
	RevenueUSD        float64 `json:"revenue_usd"`
	UpstreamCostUSD   float64 `json:"upstream_cost_usd"`
	ProfitUSD         float64 `json:"profit_usd"`
	GrossMargin       float64 `json:"gross_margin"`
	ProfitDragUSD     float64 `json:"profit_drag_usd"`
	TargetGrossMargin float64 `json:"target_gross_margin"`
}

type ModelGatewayProfitResourceSummary struct {
	EnabledCount     int                                    `json:"enabled_count"`
	AmortizedCostUSD float64                                `json:"amortized_cost_usd"`
	LossCostUSD      float64                                `json:"loss_cost_usd"`
	TotalCostUSD     float64                                `json:"total_cost_usd"`
	Items            []model.ModelGatewayProfitResourceCost `json:"items"`
}

type ModelGatewayProfitRecommendation struct {
	TargetProfitRate               float64 `json:"target_profit_rate"`
	RequiredRevenueUSD             float64 `json:"required_revenue_usd"`
	RevenueGapUSD                  float64 `json:"revenue_gap_usd"`
	CostMultiplier                 float64 `json:"cost_multiplier"`
	CostMarkupMultiplier           float64 `json:"cost_markup_multiplier"`
	RecommendedRevenueMultiplier   float64 `json:"recommended_revenue_multiplier"`
	RecommendedFloorPerMTokenUSD   float64 `json:"recommended_floor_per_m_token_usd"`
	BaseQuotaAtRatio1              float64 `json:"base_quota_at_ratio_1"`
	MinimumRevenuePerMBaseQuotaUSD float64 `json:"minimum_revenue_per_m_base_quota_usd"`
	SuggestedDynamicRatio          float64 `json:"suggested_dynamic_ratio"`
	SuggestedDynamicRatioRaw       float64 `json:"suggested_dynamic_ratio_raw"`
	DynamicRatioLimitMin           float64 `json:"dynamic_ratio_limit_min"`
	DynamicRatioLimitMax           float64 `json:"dynamic_ratio_limit_max"`
	DynamicRatioFixedValue         float64 `json:"dynamic_ratio_fixed_value"`
	DynamicRatioFixedApplied       bool    `json:"dynamic_ratio_fixed_applied"`
	DynamicRatioLimitApplied       bool    `json:"dynamic_ratio_limit_applied"`
	DynamicRatioLimitReason        string  `json:"dynamic_ratio_limit_reason"`
	CurrentEffectiveDynamicRatio   float64 `json:"current_effective_dynamic_ratio"`
	DynamicBillingApplied          bool    `json:"dynamic_billing_applied"`
	CanRecommend                   bool    `json:"can_recommend"`
	Reason                         string  `json:"reason"`
}

type modelGatewayProfitUsageAggregate struct {
	Requests         int64   `gorm:"column:requests"`
	SuccessRequests  int64   `gorm:"column:success_requests"`
	PromptTokens     int64   `gorm:"column:prompt_tokens"`
	CompletionTokens int64   `gorm:"column:completion_tokens"`
	TotalTokens      int64   `gorm:"column:total_tokens"`
	BillingQuota     int64   `gorm:"column:billing_quota"`
	UpstreamCostUSD  float64 `gorm:"column:upstream_cost_usd"`
}

type modelGatewayProfitBreakdownRow struct {
	DimensionID     int     `gorm:"column:dimension_id"`
	DimensionKey    string  `gorm:"column:dimension_key"`
	DimensionName   string  `gorm:"column:dimension_name"`
	Requests        int64   `gorm:"column:requests"`
	SuccessRequests int64   `gorm:"column:success_requests"`
	TotalTokens     int64   `gorm:"column:total_tokens"`
	BillingQuota    int64   `gorm:"column:billing_quota"`
	UpstreamCostUSD float64 `gorm:"column:upstream_cost_usd"`
}

type modelGatewayProfitAnomalyRow struct {
	ChannelID       int     `gorm:"column:channel_id"`
	ChannelName     string  `gorm:"column:channel_name"`
	RequestedModel  string  `gorm:"column:requested_model"`
	Requests        int64   `gorm:"column:requests"`
	TotalTokens     int64   `gorm:"column:total_tokens"`
	BillingQuota    int64   `gorm:"column:billing_quota"`
	UpstreamCostUSD float64 `gorm:"column:upstream_cost_usd"`
}

type UpsertModelGatewayProfitResourceRequest struct {
	Name            *string                `json:"name,omitempty"`
	ResourceType    *string                `json:"resource_type,omitempty"`
	ScopeType       *string                `json:"scope_type,omitempty"`
	ScopeID         *int                   `json:"scope_id,omitempty"`
	ScopeKey        *string                `json:"scope_key,omitempty"`
	AmountUSD       *float64               `json:"amount_usd,omitempty"`
	PeriodSeconds   *int                   `json:"period_seconds,omitempty"`
	AmortizeStartAt *int64                 `json:"amortize_start_at,omitempty"`
	AmortizeEndAt   *int64                 `json:"amortize_end_at,omitempty"`
	LossAmountUSD   *float64               `json:"loss_amount_usd,omitempty"`
	LossRecordedAt  *int64                 `json:"loss_recorded_at,omitempty"`
	AllocationMode  *string                `json:"allocation_mode,omitempty"`
	Enabled         *bool                  `json:"enabled,omitempty"`
	Remark          *string                `json:"remark,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type CreateModelGatewayProfitRecommendationRequest struct {
	ScopeType string `json:"scope_type,omitempty"`
	ScopeID   int    `json:"scope_id,omitempty"`
	ScopeKey  string `json:"scope_key,omitempty"`
	ScopeName string `json:"scope_name,omitempty"`
}

type UpdateModelGatewayProfitRecommendationDecisionRequest struct {
	DecisionStatus           string   `json:"decision_status,omitempty"`
	DecisionRemark           *string  `json:"decision_remark,omitempty"`
	PlannedRevenueMultiplier *float64 `json:"planned_revenue_multiplier,omitempty"`
}

type UpsertModelGatewayProfitCanaryTaskRequest struct {
	RecommendationID             *int     `json:"recommendation_id,omitempty"`
	Title                        *string  `json:"title,omitempty"`
	Status                       string   `json:"status,omitempty"`
	ScopeType                    *string  `json:"scope_type,omitempty"`
	ScopeID                      *int     `json:"scope_id,omitempty"`
	ScopeKey                     *string  `json:"scope_key,omitempty"`
	BaselineRevenueMultiplier    *float64 `json:"baseline_revenue_multiplier,omitempty"`
	PlannedRevenueMultiplier     *float64 `json:"planned_revenue_multiplier,omitempty"`
	RecommendedRevenueMultiplier *float64 `json:"recommended_revenue_multiplier,omitempty"`
	PlannedStartAt               *int64   `json:"planned_start_at,omitempty"`
	PlannedEndAt                 *int64   `json:"planned_end_at,omitempty"`
	ActualStartAt                *int64   `json:"actual_start_at,omitempty"`
	ActualEndAt                  *int64   `json:"actual_end_at,omitempty"`
	ObservationWindowSeconds     *int     `json:"observation_window_seconds,omitempty"`
	WatchMetrics                 []string `json:"watch_metrics,omitempty"`
	ResultSummary                *string  `json:"result_summary,omitempty"`
}

func GetModelGatewayProfitMonitorConfig(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"config":   effectiveModelGatewayProfitMonitorRecommendationConfig(loadModelGatewayProfitMonitorConfig()),
		"defaults": defaultModelGatewayProfitMonitorConfig(),
		"resource_types": []string{
			model.ModelGatewayProfitResourceTypeAccountPool,
			model.ModelGatewayProfitResourceTypeProxy,
			model.ModelGatewayProfitResourceTypeServer,
			model.ModelGatewayProfitResourceTypeBandwidth,
			model.ModelGatewayProfitResourceTypeOther,
		},
		"scope_types": []string{
			model.ModelGatewayProfitResourceScopeGlobal,
			model.ModelGatewayProfitResourceScopeChannel,
			model.ModelGatewayProfitResourceScopeGroup,
			model.ModelGatewayProfitResourceScopeModel,
		},
		"allocation_modes": []string{
			model.ModelGatewayProfitResourceAllocationRevenue,
			model.ModelGatewayProfitResourceAllocationRequest,
			model.ModelGatewayProfitResourceAllocationGlobal,
		},
	})
}

func UpdateModelGatewayProfitMonitorConfig(c *gin.Context) {
	config := loadModelGatewayProfitMonitorConfig()
	previous := normalizeModelGatewayProfitMonitorConfig(config)
	var request UpdateModelGatewayProfitMonitorConfigRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if request.Enabled != nil {
		config.Enabled = *request.Enabled
	}
	if request.ServerDailyCostUSD != nil {
		config.ServerDailyCostUSD = *request.ServerDailyCostUSD
	}
	if request.TrafficCostPerGBUSD != nil {
		config.TrafficCostPerGBUSD = *request.TrafficCostPerGBUSD
	}
	if request.TrafficEstimationEnabled != nil {
		config.TrafficEstimationEnabled = *request.TrafficEstimationEnabled
	}
	if request.TrafficEstimatedBytesPerToken != nil {
		config.TrafficEstimatedBytesPerToken = *request.TrafficEstimatedBytesPerToken
	}
	if request.ResourceCostEnabled != nil {
		config.ResourceCostEnabled = *request.ResourceCostEnabled
	}
	if request.TargetProfitRate != nil {
		config.TargetProfitRate = *request.TargetProfitRate
	}
	if request.DynamicRatioMinLimit != nil {
		config.DynamicRatioMinLimit = *request.DynamicRatioMinLimit
	}
	if request.DynamicRatioMaxLimit != nil {
		config.DynamicRatioMaxLimit = *request.DynamicRatioMaxLimit
	}
	if request.DynamicRatioFixedValue != nil {
		config.DynamicRatioFixedValue = *request.DynamicRatioFixedValue
	}
	if strings.TrimSpace(request.DynamicRatioRecommendationMode) != "" {
		config.DynamicRatioRecommendationMode = strings.TrimSpace(request.DynamicRatioRecommendationMode)
	}
	config = normalizeModelGatewayProfitMonitorConfig(config)
	if request.DynamicRatioMaxLimit != nil && (config.DynamicRatioMaxLimit != previous.DynamicRatioMaxLimit || config.DynamicRatioMaxLimitUpdatedAt <= 0) {
		config.DynamicRatioMaxLimitUpdatedAt = common.GetTimestamp()
	}
	if err := saveModelGatewayProfitMonitorConfig(config); err != nil {
		common.ApiError(c, err)
		return
	}
	invalidatePublicHomeDynamicBillingCache()
	if err := modelgatewaydynamicbilling.RefreshDefaultNow(); err != nil {
		common.SysLog("model gateway dynamic billing refresh after profit monitor config update failed: " + err.Error())
	}
	common.ApiSuccess(c, gin.H{
		"config": config,
	})
}

func GetModelGatewayProfitMonitorSummary(c *gin.Context) {
	config := loadModelGatewayProfitMonitorConfig()
	window, startTimestamp, endTimestamp := parseModelGatewayProfitMonitorWindow(c)
	breakdownDimension := parseModelGatewayProfitMonitorBreakdownDimension(c)

	payload, err := buildModelGatewayProfitMonitorResponse(window, startTimestamp, endTimestamp, breakdownDimension, config)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, payload)
}

func CreateModelGatewayProfitMonitorRecommendation(c *gin.Context) {
	config := loadModelGatewayProfitMonitorConfig()
	window, startTimestamp, endTimestamp := parseModelGatewayProfitMonitorWindow(c)
	breakdownDimension := parseModelGatewayProfitMonitorBreakdownDimension(c)
	request, err := parseModelGatewayProfitRecommendationRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}

	payload, err := buildModelGatewayProfitMonitorResponse(window, startTimestamp, endTimestamp, breakdownDimension, config)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	snapshot, err := buildModelGatewayProfitRecommendationSnapshot(payload, request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreateModelGatewayProfitRatioRecommendation(&snapshot); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, snapshot)
}

func ListModelGatewayProfitMonitorRecommendations(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	filter := model.ModelGatewayProfitRatioRecommendationFilter{
		Window:    strings.ToLower(strings.TrimSpace(c.Query("window"))),
		Dimension: normalizeModelGatewayProfitMonitorDimension(c.Query("dimension")),
	}
	filter.StartTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	filter.EndTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if strings.TrimSpace(c.Query("dimension")) == "" {
		filter.Dimension = ""
	}
	rows, err := model.ListModelGatewayProfitRatioRecommendations(limit, filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func UpdateModelGatewayProfitMonitorRecommendationDecision(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	row, err := model.GetModelGatewayProfitRatioRecommendation(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	beforeStatus := row.DecisionStatus
	beforeMultiplier := row.PlannedRevenueMultiplier
	beforeRemark := row.DecisionRemark

	var request UpdateModelGatewayProfitRecommendationDecisionRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if strings.TrimSpace(request.DecisionStatus) != "" {
		normalizedStatus := model.NormalizeModelGatewayProfitRecommendationDecisionStatus(request.DecisionStatus)
		if !strings.EqualFold(strings.TrimSpace(request.DecisionStatus), normalizedStatus) {
			common.ApiErrorMsg(c, "无效的决策状态")
			return
		}
		row.DecisionStatus = normalizedStatus
	}
	if request.DecisionRemark != nil {
		row.DecisionRemark = strings.TrimSpace(*request.DecisionRemark)
	}
	if request.PlannedRevenueMultiplier != nil {
		row.PlannedRevenueMultiplier = *request.PlannedRevenueMultiplier
	}
	row.DecisionOperatorID = c.GetInt("id")
	row.DecisionOperatorName = strings.TrimSpace(c.GetString("username"))
	row.DecisionUpdatedAt = common.GetTimestamp()
	row.Normalize()

	if err := model.DB.Save(row).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	setModelGatewayProfitRecommendationDecisionAuditSummary(c, row, beforeStatus, beforeMultiplier, beforeRemark)
	recordModelGatewayProfitRecommendationDecisionLog(c, row, beforeStatus, beforeMultiplier, beforeRemark)
	common.ApiSuccess(c, row)
}

func ListModelGatewayProfitMonitorCanaryTasks(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	recommendationID, _ := strconv.Atoi(c.Query("recommendation_id"))
	filter := model.ModelGatewayProfitCanaryTaskFilter{
		RecommendationID: recommendationID,
		Status:           strings.TrimSpace(c.Query("status")),
	}
	rows, err := model.ListModelGatewayProfitCanaryTasks(limit, filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func CreateModelGatewayProfitMonitorCanaryTask(c *gin.Context) {
	var request UpsertModelGatewayProfitCanaryTaskRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	row := model.ModelGatewayProfitCanaryTask{
		Status:                   model.ModelGatewayProfitCanaryTaskStatusPlanned,
		ObservationWindowSeconds: 7200,
	}
	if err := applyModelGatewayProfitCanaryTaskRequest(&row, request, true, c); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DB.Create(&row).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	hydrateModelGatewayProfitCanaryTaskResponse(&row)
	recordModelGatewayProfitCanaryTaskLog(c, &row, "create")
	common.ApiSuccess(c, row)
}

func UpdateModelGatewayProfitMonitorCanaryTask(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	row, err := model.GetModelGatewayProfitCanaryTask(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request UpsertModelGatewayProfitCanaryTaskRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if err := applyModelGatewayProfitCanaryTaskRequest(row, request, false, c); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DB.Save(row).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	hydrateModelGatewayProfitCanaryTaskResponse(row)
	recordModelGatewayProfitCanaryTaskLog(c, row, "update")
	common.ApiSuccess(c, row)
}

func buildModelGatewayProfitMonitorResponse(window string, startTimestamp int64, endTimestamp int64, breakdownDimension string, config ModelGatewayProfitMonitorConfig) (ModelGatewayProfitMonitorResponse, error) {
	config = effectiveModelGatewayProfitMonitorRecommendationConfig(config)
	breakdownDimension = normalizeModelGatewayProfitMonitorDimension(breakdownDimension)
	summary, err := queryModelGatewayProfitMonitorSummary(startTimestamp, endTimestamp, config)
	if err != nil {
		return ModelGatewayProfitMonitorResponse{}, err
	}
	resources, err := buildModelGatewayProfitResourceSummary(startTimestamp, endTimestamp, config.ResourceCostEnabled)
	if err != nil {
		return ModelGatewayProfitMonitorResponse{}, err
	}
	summary.ResourceAmortizedCostUSD = resources.AmortizedCostUSD
	summary.ResourceLossCostUSD = resources.LossCostUSD
	summary.OperatingCostUSD = summary.UpstreamCostUSD + summary.TrafficCostUSD + summary.ServerCostUSD + resources.TotalCostUSD
	summary.ProfitUSD = summary.RevenueUSD - summary.OperatingCostUSD
	summary.GrossMargin = ratioOrZero(summary.ProfitUSD, summary.RevenueUSD)
	summary.UpstreamOnlyProfitUSD = summary.RevenueUSD - summary.UpstreamCostUSD
	summary.UpstreamOnlyMargin = ratioOrZero(summary.UpstreamOnlyProfitUSD, summary.RevenueUSD)
	summary.CostCoverageFloorUSD = summary.OperatingCostUSD
	if summary.TotalTokens > 0 {
		summary.CostCoverageFloorPerMToken = summary.OperatingCostUSD / float64(summary.TotalTokens) * 1_000_000
	}

	breakdown, err := queryModelGatewayProfitMonitorBreakdown(startTimestamp, endTimestamp, breakdownDimension)
	if err != nil {
		return ModelGatewayProfitMonitorResponse{}, err
	}
	hasTrafficBreakdown, err := applyModelGatewayProfitMonitorTrafficBreakdown(breakdown, startTimestamp, endTimestamp, breakdownDimension, summary)
	if err != nil {
		return ModelGatewayProfitMonitorResponse{}, err
	}
	allocateModelGatewayProfitMonitorBreakdownCosts(breakdown, summary, resources, config, breakdownDimension, hasTrafficBreakdown)
	dynamicRatioGroups, dynamicRatioSummary := buildModelGatewayProfitDynamicRatioGroups(config)
	anomalies, err := queryModelGatewayProfitAnomalies(startTimestamp, endTimestamp, 0.40)
	if err != nil {
		return ModelGatewayProfitMonitorResponse{}, err
	}

	recommendation := buildModelGatewayProfitRecommendation(summary, config)
	enrichModelGatewayProfitRecommendationWithDynamicBilling(&recommendation, config)

	return ModelGatewayProfitMonitorResponse{
		Window:              window,
		Dimension:           breakdownDimension,
		BreakdownDimension:  breakdownDimension,
		StartTimestamp:      startTimestamp,
		EndTimestamp:        endTimestamp,
		Config:              config,
		Summary:             summary,
		Breakdown:           breakdown,
		DynamicRatioGroups:  dynamicRatioGroups,
		DynamicRatioSummary: dynamicRatioSummary,
		Anomalies:           anomalies,
		Resources:           resources,
		Recommendation:      recommendation,
	}, nil
}

func GetModelGatewayProfitMonitorTraffic(c *gin.Context) {
	config := loadModelGatewayProfitMonitorConfig()
	window, startTimestamp, endTimestamp := parseModelGatewayProfitMonitorWindow(c)
	dimension := normalizeModelGatewayProfitMonitorDimension(c.Query("dimension"))

	trafficSummary, err := modelgatewaytraffic.QuerySummary(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	trafficBreakdown, err := modelgatewaytraffic.QueryBreakdown(startTimestamp, endTimestamp, dimension)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	trafficSeries, err := modelgatewaytraffic.QuerySeries(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	summary := ModelGatewayProfitTrafficSummary{
		RequestCount:     trafficSummary.RequestCount,
		RequestBytes:     trafficSummary.RequestBytes,
		ResponseBytes:    trafficSummary.ResponseBytes,
		TotalBytes:       trafficSummary.TotalBytes,
		TrafficCostUSD:   modelGatewayProfitTrafficCostUSD(trafficSummary.TotalBytes, config),
		TrafficCostPerGB: config.TrafficCostPerGBUSD,
		DataReady:        trafficSummary.TotalBytes > 0,
	}
	breakdown := make([]ModelGatewayProfitTrafficBreakdown, 0, len(trafficBreakdown))
	for _, row := range trafficBreakdown {
		breakdown = append(breakdown, ModelGatewayProfitTrafficBreakdown{
			DimensionID:    row.DimensionID,
			DimensionKey:   row.DimensionKey,
			DimensionName:  modelGatewayProfitTrafficDimensionName(row.DimensionID, row.DimensionKey, dimension),
			RequestCount:   row.RequestCount,
			RequestBytes:   row.RequestBytes,
			ResponseBytes:  row.ResponseBytes,
			TotalBytes:     row.TotalBytes,
			TrafficCostUSD: modelGatewayProfitTrafficCostUSD(row.TotalBytes, config),
			Share:          ratioOrZero(float64(row.TotalBytes), float64(trafficSummary.TotalBytes)),
		})
	}
	series := make([]ModelGatewayProfitTrafficBucketSeries, 0, len(trafficSeries))
	for _, row := range trafficSeries {
		series = append(series, ModelGatewayProfitTrafficBucketSeries{
			BucketTs:       row.BucketTs,
			RequestCount:   row.RequestCount,
			RequestBytes:   row.RequestBytes,
			ResponseBytes:  row.ResponseBytes,
			TotalBytes:     row.TotalBytes,
			TrafficCostUSD: modelGatewayProfitTrafficCostUSD(row.TotalBytes, config),
		})
	}

	common.ApiSuccess(c, ModelGatewayProfitTrafficResponse{
		Window:         window,
		Dimension:      dimension,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		Config:         config,
		Summary:        summary,
		Breakdown:      breakdown,
		Series:         series,
	})
}

func ListModelGatewayProfitMonitorResources(c *gin.Context) {
	enabledOnly := strings.EqualFold(c.Query("enabled_only"), "true")
	rows, err := model.ListModelGatewayProfitResourceCosts(enabledOnly)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func CreateModelGatewayProfitMonitorResource(c *gin.Context) {
	var request UpsertModelGatewayProfitResourceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	row := model.ModelGatewayProfitResourceCost{
		Enabled: true,
	}
	if err := applyModelGatewayProfitResourceRequest(&row, request, true); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DB.Create(&row).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func UpdateModelGatewayProfitMonitorResource(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	row, err := model.GetModelGatewayProfitResourceCost(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request UpsertModelGatewayProfitResourceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if err := applyModelGatewayProfitResourceRequest(row, request, false); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DB.Save(row).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func DeleteModelGatewayProfitMonitorResource(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的资源成本 ID")
		return
	}
	if err := model.DB.Delete(&model.ModelGatewayProfitResourceCost{}, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func defaultModelGatewayProfitMonitorConfig() ModelGatewayProfitMonitorConfig {
	return ModelGatewayProfitMonitorConfig{
		Enabled:                        true,
		ServerDailyCostUSD:             0,
		TrafficCostPerGBUSD:            0,
		TrafficEstimationEnabled:       false,
		TrafficEstimatedBytesPerToken:  0,
		ResourceCostEnabled:            true,
		TargetProfitRate:               0.20,
		DynamicRatioMinLimit:           0,
		DynamicRatioMaxLimit:           0,
		DynamicRatioFixedValue:         0,
		DynamicRatioRecommendationMode: "observe",
	}
}

func loadModelGatewayProfitMonitorConfig() ModelGatewayProfitMonitorConfig {
	config := defaultModelGatewayProfitMonitorConfig()
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[modelGatewayProfitMonitorConfigOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(raw) == "" {
		return config
	}
	if err := common.UnmarshalJsonStr(raw, &config); err != nil {
		return defaultModelGatewayProfitMonitorConfig()
	}
	return normalizeModelGatewayProfitMonitorConfig(config)
}

func saveModelGatewayProfitMonitorConfig(config ModelGatewayProfitMonitorConfig) error {
	payload, err := common.Marshal(normalizeModelGatewayProfitMonitorConfig(config))
	if err != nil {
		return err
	}
	if err := model.UpdateOption(modelGatewayProfitMonitorConfigOptionKey, string(payload)); err != nil {
		return err
	}
	invalidatePublicHomeDynamicBillingCache()
	return nil
}

func normalizeModelGatewayProfitMonitorConfig(config ModelGatewayProfitMonitorConfig) ModelGatewayProfitMonitorConfig {
	if config.ServerDailyCostUSD < 0 {
		config.ServerDailyCostUSD = 0
	}
	if config.TrafficCostPerGBUSD < 0 {
		config.TrafficCostPerGBUSD = 0
	}
	if config.TrafficEstimatedBytesPerToken < 0 {
		config.TrafficEstimatedBytesPerToken = 0
	}
	if config.TargetProfitRate < 0 {
		config.TargetProfitRate = 0
	}
	if config.TargetProfitRate > 1 && config.TargetProfitRate <= 95 {
		config.TargetProfitRate = config.TargetProfitRate / 100
	}
	if config.TargetProfitRate > 0.95 {
		config.TargetProfitRate = 0.95
	}
	if config.DynamicRatioMinLimit < 0 {
		config.DynamicRatioMinLimit = 0
	}
	if config.DynamicRatioMaxLimit < 0 {
		config.DynamicRatioMaxLimit = 0
	}
	if config.DynamicRatioFixedValue < 0 {
		config.DynamicRatioFixedValue = 0
	}
	if config.DynamicRatioMinLimit > 100 {
		config.DynamicRatioMinLimit = 100
	}
	if config.DynamicRatioMaxLimit > 100 {
		config.DynamicRatioMaxLimit = 100
	}
	if config.DynamicRatioMaxLimitUpdatedAt < 0 {
		config.DynamicRatioMaxLimitUpdatedAt = 0
	}
	if config.DynamicRatioFixedValue > 100 {
		config.DynamicRatioFixedValue = 100
	}
	if config.DynamicRatioMaxLimit > 0 && config.DynamicRatioMinLimit > config.DynamicRatioMaxLimit {
		config.DynamicRatioMaxLimit = config.DynamicRatioMinLimit
	}
	switch strings.TrimSpace(config.DynamicRatioRecommendationMode) {
	case "off", "observe", "ai":
	default:
		config.DynamicRatioRecommendationMode = "observe"
	}
	return config
}

func effectiveModelGatewayProfitMonitorRecommendationConfig(config ModelGatewayProfitMonitorConfig) ModelGatewayProfitMonitorConfig {
	config = normalizeModelGatewayProfitMonitorConfig(config)
	setting := scheduler_setting.GetSetting()
	if strings.TrimSpace(setting.DynamicBillingCostSource) == scheduler_setting.DynamicBillingCostSourceProfit24h && setting.DynamicBillingProfitRate >= 0 {
		config.TargetProfitRate = setting.DynamicBillingProfitRate
	}
	return normalizeModelGatewayProfitMonitorConfig(config)
}

func parseModelGatewayProfitMonitorWindow(c *gin.Context) (string, int64, int64) {
	now := common.GetTimestamp()
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if endTimestamp <= 0 {
		endTimestamp = now
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	window := strings.ToLower(strings.TrimSpace(c.Query("window")))
	if window == "" {
		window = "24h"
	}
	if startTimestamp <= 0 {
		switch window {
		case "1h":
			startTimestamp = endTimestamp - 3600
		case "7d":
			startTimestamp = endTimestamp - 7*86400
		case "30d":
			startTimestamp = endTimestamp - 30*86400
		default:
			window = "24h"
			startTimestamp = endTimestamp - 86400
		}
	} else {
		window = "custom"
	}
	if startTimestamp > endTimestamp {
		startTimestamp = endTimestamp - 86400
		window = "24h"
	}
	return window, startTimestamp, endTimestamp
}

func normalizeModelGatewayProfitMonitorDimension(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "channel", "model", "proxy":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "group"
	}
}

func parseModelGatewayProfitMonitorBreakdownDimension(c *gin.Context) string {
	if c == nil {
		return "channel"
	}
	if value := strings.TrimSpace(c.Query("breakdown_dimension")); value != "" {
		return normalizeModelGatewayProfitMonitorDimension(value)
	}
	if value := strings.TrimSpace(c.Query("dimension")); value != "" {
		return normalizeModelGatewayProfitMonitorDimension(value)
	}
	return "channel"
}

func parseModelGatewayProfitRecommendationRequest(c *gin.Context) (CreateModelGatewayProfitRecommendationRequest, error) {
	request := CreateModelGatewayProfitRecommendationRequest{}
	if c == nil {
		return request, nil
	}
	if c.Request != nil && c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := common.DecodeJson(c.Request.Body, &request); err != nil {
			return request, err
		}
	}
	if value := strings.TrimSpace(c.Query("scope_type")); value != "" {
		request.ScopeType = value
	}
	if value := strings.TrimSpace(c.Query("scope_key")); value != "" {
		request.ScopeKey = value
	}
	if value := strings.TrimSpace(c.Query("scope_name")); value != "" {
		request.ScopeName = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(c.Query("scope_id"))); err == nil && value > 0 {
		request.ScopeID = value
	}
	request.ScopeType = model.NormalizeModelGatewayProfitResourceScope(request.ScopeType)
	request.ScopeKey = strings.TrimSpace(request.ScopeKey)
	request.ScopeName = strings.TrimSpace(request.ScopeName)
	return request, nil
}

func queryModelGatewayProfitMonitorSummary(startTimestamp int64, endTimestamp int64, config ModelGatewayProfitMonitorConfig) (ModelGatewayProfitMonitorSummary, error) {
	var aggregate modelGatewayProfitUsageAggregate
	if model.DB == nil {
		return ModelGatewayProfitMonitorSummary{}, nil
	}
	query := modelGatewayProfitUsageBaseQuery(startTimestamp, endTimestamp)
	err := query.Select(modelGatewayProfitAggregateSelect(), modelGatewayProfitAggregateSelectArgs()...).
		Scan(&aggregate).Error
	if err != nil {
		return ModelGatewayProfitMonitorSummary{}, err
	}
	summary := ModelGatewayProfitMonitorSummary{
		Requests:         aggregate.Requests,
		SuccessRequests:  aggregate.SuccessRequests,
		PromptTokens:     aggregate.PromptTokens,
		CompletionTokens: aggregate.CompletionTokens,
		TotalTokens:      aggregate.TotalTokens,
		BillingQuota:     aggregate.BillingQuota,
		UpstreamCostUSD:  aggregate.UpstreamCostUSD,
	}
	summary.SuccessRate = ratioOrZero(float64(summary.SuccessRequests), float64(summary.Requests))
	if common.QuotaPerUnit > 0 {
		summary.RevenueUSD = float64(summary.BillingQuota) / common.QuotaPerUnit
	}
	summary.ServerCostUSD = config.ServerDailyCostUSD * math.Max(0, float64(endTimestamp-startTimestamp)) / 86400
	trafficSummary, err := modelgatewaytraffic.QuerySummary(startTimestamp, endTimestamp)
	if err != nil {
		return ModelGatewayProfitMonitorSummary{}, err
	}
	if trafficSummary.TotalBytes > 0 {
		summary.TrafficDataReady = true
		summary.TrafficRequestBytes = trafficSummary.RequestBytes
		summary.TrafficResponseBytes = trafficSummary.ResponseBytes
		summary.TrafficBytes = trafficSummary.TotalBytes
		summary.TrafficCostUSD = modelGatewayProfitTrafficCostUSD(summary.TrafficBytes, config)
	} else if config.TrafficEstimationEnabled && config.TrafficEstimatedBytesPerToken > 0 {
		summary.TrafficEstimated = true
		summary.TrafficDataReady = true
		summary.TrafficBytes = summary.TotalTokens * int64(config.TrafficEstimatedBytesPerToken)
		summary.TrafficCostUSD = modelGatewayProfitTrafficCostUSD(summary.TrafficBytes, config)
	}
	return summary, nil
}

func queryModelGatewayProfitMonitorBreakdown(startTimestamp int64, endTimestamp int64, dimension string) ([]ModelGatewayProfitMonitorBreakdown, error) {
	if model.DB == nil {
		return nil, nil
	}
	rows := make([]modelGatewayProfitBreakdownRow, 0)
	query := modelGatewayProfitUsageBaseQuery(startTimestamp, endTimestamp)
	selectPrefix, groupExpr := modelGatewayProfitBreakdownSelectParts(dimension)
	err := query.Select(selectPrefix+", "+modelGatewayProfitAggregateSelect(), modelGatewayProfitAggregateSelectArgs()...).
		Group(groupExpr).
		Order("revenue_quota DESC").
		Limit(100).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]ModelGatewayProfitMonitorBreakdown, 0, len(rows))
	for _, row := range rows {
		item := ModelGatewayProfitMonitorBreakdown{
			DimensionID:     row.DimensionID,
			DimensionKey:    strings.TrimSpace(row.DimensionKey),
			DimensionName:   strings.TrimSpace(row.DimensionName),
			Requests:        row.Requests,
			SuccessRequests: row.SuccessRequests,
			TotalTokens:     row.TotalTokens,
			BillingQuota:    row.BillingQuota,
			UpstreamCostUSD: row.UpstreamCostUSD,
		}
		if item.DimensionName == "" {
			item.DimensionName = item.DimensionKey
		}
		if item.DimensionName == "" && item.DimensionID > 0 {
			item.DimensionName = strconv.Itoa(item.DimensionID)
		}
		if item.DimensionName == "" {
			item.DimensionName = "未归类"
		}
		if common.QuotaPerUnit > 0 {
			item.RevenueUSD = float64(item.BillingQuota) / common.QuotaPerUnit
		}
		item.SuccessRate = ratioOrZero(float64(item.SuccessRequests), float64(item.Requests))
		out = append(out, item)
	}
	return out, nil
}

func queryModelGatewayProfitAnomalies(startTimestamp int64, endTimestamp int64, targetGrossMargin float64) ([]ModelGatewayProfitAnomaly, error) {
	if model.DB == nil {
		return nil, nil
	}
	targetGrossMargin = modelgatewaydynamicbilling.SanitizeTargetGrossMargin(targetGrossMargin)
	rows := make([]modelGatewayProfitAnomalyRow, 0)
	query := modelGatewayProfitUsageBaseQuery(startTimestamp, endTimestamp)
	err := query.Select(
		"channel_id, MAX(channel_name) AS channel_name, requested_model, COUNT(*) AS requests, "+
			"COALESCE(SUM(total_tokens), 0) AS total_tokens, COALESCE(SUM(quota), 0) AS billing_quota, "+
			"COALESCE(SUM(upstream_cost_total), 0) AS upstream_cost_usd",
	).
		Where("success = ?", true).
		Group("channel_id, requested_model").
		Order("upstream_cost_usd DESC").
		Limit(50).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]ModelGatewayProfitAnomaly, 0, len(rows))
	for _, row := range rows {
		item := ModelGatewayProfitAnomaly{
			ChannelID:         row.ChannelID,
			ChannelName:       strings.TrimSpace(row.ChannelName),
			RequestedModel:    strings.TrimSpace(row.RequestedModel),
			Requests:          row.Requests,
			TotalTokens:       row.TotalTokens,
			BillingQuota:      row.BillingQuota,
			UpstreamCostUSD:   row.UpstreamCostUSD,
			TargetGrossMargin: targetGrossMargin,
		}
		if item.ChannelName == "" && item.ChannelID > 0 {
			item.ChannelName = "#" + strconv.Itoa(item.ChannelID)
		}
		if item.RequestedModel == "" {
			item.RequestedModel = "未归类"
		}
		if common.QuotaPerUnit > 0 {
			item.RevenueUSD = float64(item.BillingQuota) / common.QuotaPerUnit
		}
		item.ProfitUSD = item.RevenueUSD - item.UpstreamCostUSD
		item.GrossMargin = ratioOrZero(item.ProfitUSD, item.RevenueUSD)
		requiredRevenue := modelgatewaydynamicbilling.RequiredRevenueForGrossMargin(item.UpstreamCostUSD, targetGrossMargin)
		if requiredRevenue > item.RevenueUSD {
			item.ProfitDragUSD = requiredRevenue - item.RevenueUSD
		}
		if item.ProfitUSD < 0 || item.ProfitDragUSD > 0 {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ProfitDragUSD != out[j].ProfitDragUSD {
			return out[i].ProfitDragUSD > out[j].ProfitDragUSD
		}
		return out[i].UpstreamCostUSD > out[j].UpstreamCostUSD
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

func modelGatewayProfitUsageBaseQuery(startTimestamp int64, endTimestamp int64) *gorm.DB {
	return model.DB.Model(&model.ChannelAccountUsageEvent{}).
		Where("is_health_probe = ?", false).
		Where(
			"((completed_at >= ? AND completed_at <= ?) OR (completed_at <= ? AND updated_at >= ? AND updated_at <= ?) OR (completed_at <= ? AND updated_at <= ? AND created_at >= ? AND created_at <= ?))",
			startTimestamp, endTimestamp,
			0, startTimestamp, endTimestamp,
			0, 0, startTimestamp, endTimestamp,
		).
		Where("(quota <> ? OR upstream_cost_total > ? OR total_tokens > ? OR success = ? OR status_code <> ? OR error_category <> ?)", 0, 0, 0, true, 0, "")
}

func modelGatewayProfitAggregateSelect() string {
	return "COUNT(*) AS requests, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN 1 ELSE 0 END), 0) AS success_requests, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN prompt_tokens ELSE 0 END), 0) AS prompt_tokens, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN completion_tokens ELSE 0 END), 0) AS completion_tokens, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN total_tokens ELSE 0 END), 0) AS total_tokens, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN quota ELSE 0 END), 0) AS billing_quota, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN quota ELSE 0 END), 0) AS revenue_quota, " +
		"COALESCE(SUM(CASE WHEN success = ? THEN upstream_cost_total ELSE 0 END), 0) AS upstream_cost_usd"
}

func modelGatewayProfitAggregateSelectArgs() []interface{} {
	return []interface{}{true, true, true, true, true, true, true}
}

func modelGatewayProfitBreakdownSelectParts(dimension string) (string, string) {
	switch dimension {
	case "channel":
		return "channel_id AS dimension_id, '' AS dimension_key, MAX(channel_name) AS dimension_name", "channel_id"
	case "model":
		return "0 AS dimension_id, requested_model AS dimension_key, requested_model AS dimension_name", "requested_model"
	case "proxy":
		return "proxy_id AS dimension_id, '' AS dimension_key, '' AS dimension_name", "proxy_id"
	default:
		groupExpr := "COALESCE(NULLIF(selected_group, ''), requested_group, '')"
		return "0 AS dimension_id, " + groupExpr + " AS dimension_key, " + groupExpr + " AS dimension_name", groupExpr
	}
}

func buildModelGatewayProfitResourceSummary(startTimestamp int64, endTimestamp int64, enabled bool) (ModelGatewayProfitResourceSummary, error) {
	if model.DB == nil || !enabled {
		return ModelGatewayProfitResourceSummary{Items: []model.ModelGatewayProfitResourceCost{}}, nil
	}
	rows, err := model.ListModelGatewayProfitResourceCosts(true)
	if err != nil {
		return ModelGatewayProfitResourceSummary{}, err
	}
	summary := ModelGatewayProfitResourceSummary{
		Items: make([]model.ModelGatewayProfitResourceCost, 0, len(rows)),
	}
	for _, row := range rows {
		row.Normalize()
		row.WindowCostUSD = modelGatewayProfitResourceWindowCost(row, startTimestamp, endTimestamp)
		row.WindowLossUSD = modelGatewayProfitResourceWindowLoss(row, startTimestamp, endTimestamp)
		row.WindowTotalUSD = row.WindowCostUSD + row.WindowLossUSD
		if row.Enabled {
			summary.EnabledCount++
		}
		summary.AmortizedCostUSD += row.WindowCostUSD
		summary.LossCostUSD += row.WindowLossUSD
		summary.TotalCostUSD += row.WindowTotalUSD
		summary.Items = append(summary.Items, row)
	}
	return summary, nil
}

func modelGatewayProfitResourceWindowCost(resource model.ModelGatewayProfitResourceCost, startTimestamp int64, endTimestamp int64) float64 {
	if !resource.Enabled || resource.AmountUSD <= 0 || resource.PeriodSeconds <= 0 || endTimestamp <= startTimestamp {
		return 0
	}
	activeStart := resource.AmortizeStartAt
	if activeStart <= 0 {
		activeStart = resource.CreatedAt
	}
	if activeStart <= 0 {
		activeStart = startTimestamp
	}
	activeEnd := resource.AmortizeEndAt
	if activeEnd <= 0 {
		activeEnd = endTimestamp
	}
	overlapStart := maxProfitMonitorInt64(startTimestamp, activeStart)
	overlapEnd := minProfitMonitorInt64(endTimestamp, activeEnd)
	if overlapEnd <= overlapStart {
		return 0
	}
	return resource.AmountUSD * float64(overlapEnd-overlapStart) / float64(resource.PeriodSeconds)
}

func modelGatewayProfitResourceWindowLoss(resource model.ModelGatewayProfitResourceCost, startTimestamp int64, endTimestamp int64) float64 {
	if !resource.Enabled || resource.LossAmountUSD <= 0 || resource.LossRecordedAt <= 0 {
		return 0
	}
	if resource.LossRecordedAt < startTimestamp || resource.LossRecordedAt > endTimestamp {
		return 0
	}
	return resource.LossAmountUSD
}

func applyModelGatewayProfitMonitorTrafficBreakdown(rows []ModelGatewayProfitMonitorBreakdown, startTimestamp int64, endTimestamp int64, dimension string, summary ModelGatewayProfitMonitorSummary) (bool, error) {
	if len(rows) == 0 || summary.TrafficBytes <= 0 || summary.TrafficEstimated {
		return false, nil
	}
	trafficRows, err := modelgatewaytraffic.QueryBreakdown(startTimestamp, endTimestamp, dimension)
	if err != nil {
		return false, err
	}
	if len(trafficRows) == 0 {
		return false, nil
	}
	trafficByKey := make(map[string]modelgatewaytraffic.DimensionSummary, len(trafficRows))
	for _, row := range trafficRows {
		trafficByKey[modelGatewayProfitTrafficBreakdownKey(row.DimensionID, row.DimensionKey, dimension)] = row
	}
	matched := false
	for i := range rows {
		key := modelGatewayProfitBreakdownLookupKey(rows[i], dimension)
		trafficRow, ok := trafficByKey[key]
		if !ok || trafficRow.TotalBytes <= 0 {
			continue
		}
		rows[i].TrafficBytes = trafficRow.TotalBytes
		rows[i].TrafficCostUSD = summary.TrafficCostUSD * ratioOrZero(float64(trafficRow.TotalBytes), float64(summary.TrafficBytes))
		matched = true
	}
	return matched, nil
}

func modelGatewayProfitBreakdownLookupKey(row ModelGatewayProfitMonitorBreakdown, dimension string) string {
	return modelGatewayProfitTrafficBreakdownKey(row.DimensionID, row.DimensionKey, dimension)
}

func modelGatewayProfitTrafficBreakdownKey(id int, key string, dimension string) string {
	switch dimension {
	case "channel", "proxy":
		return strconv.Itoa(id)
	default:
		return strings.TrimSpace(key)
	}
}

func modelGatewayProfitTrafficCostUSD(bytes int64, config ModelGatewayProfitMonitorConfig) float64 {
	if bytes <= 0 || config.TrafficCostPerGBUSD <= 0 {
		return 0
	}
	return float64(bytes) / 1024 / 1024 / 1024 * config.TrafficCostPerGBUSD
}

func modelGatewayProfitTrafficDimensionName(id int, key string, dimension string) string {
	key = strings.TrimSpace(key)
	switch dimension {
	case "channel", "proxy":
		if id > 0 {
			return "#" + strconv.Itoa(id)
		}
		if key != "" && key != "0" {
			return "#" + key
		}
		return "未归类"
	default:
		if key != "" {
			return key
		}
		return "未归类"
	}
}

func allocateModelGatewayProfitMonitorBreakdownCosts(rows []ModelGatewayProfitMonitorBreakdown, summary ModelGatewayProfitMonitorSummary, resources ModelGatewayProfitResourceSummary, config ModelGatewayProfitMonitorConfig, dimension string, hasTrafficBreakdown bool) {
	if len(rows) == 0 {
		return
	}
	globalExtra := summary.ServerCostUSD
	if !hasTrafficBreakdown {
		globalExtra += summary.TrafficCostUSD
	}
	resourceAllocations := make([]float64, len(rows))
	for _, resource := range resources.Items {
		amount := resource.WindowTotalUSD
		if amount <= 0 {
			continue
		}
		matched := false
		for i := range rows {
			if modelGatewayProfitResourceMatchesBreakdown(resource, rows[i], dimension) {
				resourceAllocations[i] += amount
				matched = true
			}
		}
		if matched {
			continue
		}
		for i := range rows {
			resourceAllocations[i] += amount * modelGatewayProfitAllocationRatio(rows[i], summary, resource.AllocationMode)
		}
	}
	for i := range rows {
		rows[i].AllocatedOperatingCostUSD = rows[i].UpstreamCostUSD +
			rows[i].TrafficCostUSD +
			globalExtra*modelGatewayProfitAllocationRatio(rows[i], summary, model.ModelGatewayProfitResourceAllocationRevenue) +
			resourceAllocations[i]
		rows[i].ProfitUSD = rows[i].RevenueUSD - rows[i].AllocatedOperatingCostUSD
		rows[i].GrossMargin = ratioOrZero(rows[i].ProfitUSD, rows[i].RevenueUSD)
	}
	_ = config
}

func modelGatewayProfitResourceMatchesBreakdown(resource model.ModelGatewayProfitResourceCost, row ModelGatewayProfitMonitorBreakdown, dimension string) bool {
	scope := model.NormalizeModelGatewayProfitResourceScope(resource.ScopeType)
	if scope == model.ModelGatewayProfitResourceScopeGlobal || scope != dimension {
		return false
	}
	scopeKey := strings.TrimSpace(resource.ScopeKey)
	switch scope {
	case model.ModelGatewayProfitResourceScopeChannel:
		if resource.ScopeID > 0 && row.DimensionID == resource.ScopeID {
			return true
		}
		return scopeKey != "" && (scopeKey == row.DimensionKey || scopeKey == row.DimensionName)
	case model.ModelGatewayProfitResourceScopeGroup, model.ModelGatewayProfitResourceScopeModel:
		return scopeKey != "" && (scopeKey == row.DimensionKey || scopeKey == row.DimensionName)
	default:
		return false
	}
}

func modelGatewayProfitAllocationRatio(row ModelGatewayProfitMonitorBreakdown, summary ModelGatewayProfitMonitorSummary, allocationMode string) float64 {
	switch model.NormalizeModelGatewayProfitResourceAllocationMode(allocationMode) {
	case model.ModelGatewayProfitResourceAllocationRequest:
		return ratioOrZero(float64(row.SuccessRequests), float64(summary.SuccessRequests))
	default:
		ratio := ratioOrZero(row.RevenueUSD, summary.RevenueUSD)
		if ratio <= 0 {
			ratio = ratioOrZero(float64(row.SuccessRequests), float64(summary.SuccessRequests))
		}
		return ratio
	}
}

func buildModelGatewayProfitDynamicRatioGroups(config ModelGatewayProfitMonitorConfig) ([]ModelGatewayProfitDynamicRatioGroup, ModelGatewayProfitDynamicRatioSummary) {
	setting := scheduler_setting.GetSetting()
	ratioMap := ratio_setting.GetGroupRatioCopy()
	baselines := modelgatewaydynamicbilling.DefaultBaselineSnapshots()
	baselineByGroup := make(map[string]modelgatewaydynamicbilling.RatioBaseline, len(baselines))
	groupNames := make(map[string]string)
	for group := range ratioMap {
		trimmed := strings.TrimSpace(group)
		if trimmed != "" {
			groupNames[strings.ToLower(trimmed)] = trimmed
		}
	}
	for group := range setting.GroupPolicies {
		trimmed := strings.TrimSpace(group)
		if trimmed != "" {
			groupNames[strings.ToLower(trimmed)] = trimmed
		}
	}
	for _, baseline := range baselines {
		group := strings.TrimSpace(baseline.Group)
		if group == "" {
			continue
		}
		key := strings.ToLower(group)
		baselineByGroup[key] = baseline
		groupNames[key] = group
	}

	groups := make([]string, 0, len(groupNames))
	for _, group := range groupNames {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	provider := modelgatewaydynamicbilling.DefaultRatioProvider()
	now := common.GetTimestamp()
	items := make([]ModelGatewayProfitDynamicRatioGroup, 0, len(groups))
	summary := ModelGatewayProfitDynamicRatioSummary{}
	dynamicRatioLimitMin, dynamicRatioLimitMax := modelGatewayProfitEffectiveDynamicRatioLimits(config)
	for _, group := range groups {
		key := strings.ToLower(strings.TrimSpace(group))
		baseline := baselineByGroup[key]
		staticRatio := modelGatewayProfitStaticGroupRatio(group, ratioMap)
		billingMode := scheduler_setting.BillingRatioModeStatic
		if policy, ok := modelGatewayProfitGroupPolicy(setting.GroupPolicies, group); ok {
			billingMode = strings.TrimSpace(policy.BillingRatioMode)
		}
		if billingMode != scheduler_setting.BillingRatioModeDynamic {
			billingMode = scheduler_setting.BillingRatioModeStatic
		}
		referenceModel := strings.TrimSpace(baseline.ReferenceModel)
		if referenceModel == "" {
			referenceModel = strings.TrimSpace(baseline.RequestedModel)
		}
		applied := modelgatewaydynamicbilling.Apply(modelgatewaydynamicbilling.ApplyInput{
			RequestedModel:   referenceModel,
			Group:            group,
			StaticGroupRatio: staticRatio,
			Mode:             billingMode,
			Setting:          setting,
			Provider:         provider,
			Now:              now,
		})
		actualRatio := staticRatio
		if applied.Applied && applied.DynamicRatio > 0 {
			actualRatio = applied.DynamicRatio
		}
		profitRate := baseline.ProfitRate
		if profitRate <= 0 {
			profitRate = config.TargetProfitRate
		}
		profitRate = modelgatewaydynamicbilling.SanitizeTargetGrossMargin(profitRate)
		requiredRevenue := baseline.RequiredRevenueUSD
		if requiredRevenue <= 0 && baseline.OperatingCostUSD > 0 {
			requiredRevenue = modelgatewaydynamicbilling.RequiredRevenueForGrossMargin(baseline.OperatingCostUSD, profitRate)
		}
		currentRevenue := 0.0
		if baseline.BaseQuotaAtRatio1 > 0 && common.QuotaPerUnit > 0 && actualRatio > 0 {
			currentRevenue = actualRatio * baseline.BaseQuotaAtRatio1 / common.QuotaPerUnit
		}
		dynamicRatio := baseline.Ratio
		if dynamicRatio <= 0 {
			dynamicRatio = applied.DynamicRatio
		}
		effectiveRatio := baseline.EffectiveRatio
		if effectiveRatio <= 0 {
			effectiveRatio = dynamicRatio
		}
		fallbackReason := strings.TrimSpace(applied.FallbackReason)
		if fallbackReason == "" {
			fallbackReason = strings.TrimSpace(baseline.FallbackReason)
		}
		applyReason := strings.TrimSpace(applied.ApplyReason)
		if applyReason == "" {
			applyReason = strings.TrimSpace(baseline.ApplyReason)
		}
		item := ModelGatewayProfitDynamicRatioGroup{
			Group:                    group,
			BillingRatioMode:         billingMode,
			StaticRatio:              staticRatio,
			TargetRatio:              baseline.TargetRatio,
			EffectiveRatio:           effectiveRatio,
			DynamicRatio:             dynamicRatio,
			ActualRatio:              actualRatio,
			Applied:                  applied.Applied,
			FallbackReason:           fallbackReason,
			ApplyReason:              applyReason,
			Clamped:                  baseline.Clamped,
			PendingManualConfirm:     baseline.PendingManualConfirm,
			DynamicRatioFixedValue:   baseline.FixedRatio,
			DynamicRatioFixedApplied: baseline.FixedRatioApplied,
			SampleCount:              baseline.SampleCount,
			ModelCount:               baseline.ModelCount,
			RequestCount:             baseline.RequestCount,
			SuccessRequestCount:      baseline.SuccessRequestCount,
			TotalTokens:              baseline.TotalTokens,
			CurrentRevenueUSD:        currentRevenue,
			RequiredRevenueUSD:       requiredRevenue,
			RevenueGapUSD:            requiredRevenue - currentRevenue,
			UpstreamCostUSD:          baseline.UpstreamCostUSD,
			TrafficCostUSD:           baseline.TrafficCostUSD,
			ServerCostUSD:            baseline.ServerCostUSD,
			ResourceCostUSD:          baseline.ResourceCostUSD,
			OperatingCostUSD:         baseline.OperatingCostUSD,
			DynamicRatioLimitMin:     dynamicRatioLimitMin,
			DynamicRatioLimitMax:     dynamicRatioLimitMax,
			BaseQuotaAtRatio1:        baseline.BaseQuotaAtRatio1,
			CostMultiplier:           baseline.CostMultiplier,
			CostMarkupMultiplier:     modelgatewaydynamicbilling.RevenueMultiplierForGrossMargin(profitRate),
			ReferenceModel:           referenceModel,
			CostSource:               strings.TrimSpace(baseline.CostSource),
			ApplyMode:                strings.TrimSpace(applied.ApplyMode),
			ProfitRate:               profitRate,
			TrafficEstimated:         baseline.TrafficEstimated,
			TrafficDataReady:         baseline.TrafficDataReady,
			WindowStart:              baseline.WindowStart,
			WindowEnd:                baseline.WindowEnd,
			CalculatedAt:             baseline.CalculatedAt,
			UpdatedAt:                baseline.CalculatedAt,
		}
		items = append(items, item)
		summary.TotalGroups++
		if item.Applied {
			summary.ActiveGroups++
		} else if item.FallbackReason != "" && item.FallbackReason != modelgatewaydynamicbilling.FallbackStaticMode {
			summary.FallbackGroups++
		}
		if item.Clamped {
			summary.ClampedGroups++
		}
		if item.PendingManualConfirm {
			summary.PendingManualConfirmGroups++
		}
		summary.OperatingCostUSD += item.OperatingCostUSD
		summary.RequiredRevenueUSD += item.RequiredRevenueUSD
		summary.RevenueGapUSD += item.RevenueGapUSD
	}
	sort.SliceStable(items, func(i, j int) bool {
		if math.Abs(items[i].RevenueGapUSD-items[j].RevenueGapUSD) > 0.0000001 {
			return items[i].RevenueGapUSD > items[j].RevenueGapUSD
		}
		if math.Abs(items[i].OperatingCostUSD-items[j].OperatingCostUSD) > 0.0000001 {
			return items[i].OperatingCostUSD > items[j].OperatingCostUSD
		}
		return strings.TrimSpace(items[i].Group) < strings.TrimSpace(items[j].Group)
	})
	return items, summary
}

func modelGatewayProfitStaticGroupRatio(group string, ratioMap map[string]float64) float64 {
	group = strings.TrimSpace(group)
	if group == "" {
		return 1
	}
	if ratio, ok := ratioMap[group]; ok && ratio > 0 {
		return ratio
	}
	lowerGroup := strings.ToLower(group)
	for key, ratio := range ratioMap {
		if strings.EqualFold(strings.TrimSpace(key), lowerGroup) && ratio > 0 {
			return ratio
		}
	}
	return 1
}

func modelGatewayProfitGroupPolicy(policies map[string]scheduler_setting.GroupPolicySetting, group string) (scheduler_setting.GroupPolicySetting, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return scheduler_setting.GroupPolicySetting{}, false
	}
	if policy, ok := policies[group]; ok {
		return policy, true
	}
	lowerGroup := strings.ToLower(group)
	if policy, ok := policies[lowerGroup]; ok {
		return policy, true
	}
	for key, policy := range policies {
		if strings.EqualFold(strings.TrimSpace(key), group) {
			return policy, true
		}
	}
	return scheduler_setting.GroupPolicySetting{}, false
}

func modelGatewayProfitDynamicRatioGroupByScope(payload ModelGatewayProfitMonitorResponse, request CreateModelGatewayProfitRecommendationRequest) (ModelGatewayProfitDynamicRatioGroup, bool) {
	if request.ScopeType != model.ModelGatewayProfitResourceScopeGroup {
		return ModelGatewayProfitDynamicRatioGroup{}, false
	}
	scopeKey := strings.TrimSpace(request.ScopeKey)
	if scopeKey == "" {
		return ModelGatewayProfitDynamicRatioGroup{}, false
	}
	for _, item := range payload.DynamicRatioGroups {
		if strings.EqualFold(strings.TrimSpace(item.Group), scopeKey) {
			return item, true
		}
	}
	return ModelGatewayProfitDynamicRatioGroup{}, false
}

func buildModelGatewayProfitRecommendation(summary ModelGatewayProfitMonitorSummary, config ModelGatewayProfitMonitorConfig) ModelGatewayProfitRecommendation {
	result := ModelGatewayProfitRecommendation{
		TargetProfitRate: config.TargetProfitRate,
	}
	if config.DynamicRatioRecommendationMode == "off" {
		result.Reason = modelGatewayProfitRecommendationReasonDisabled
		return result
	}
	costUSD := summary.OperatingCostUSD
	if costUSD <= 0 {
		costUSD = summary.UpstreamCostUSD
	}
	if costUSD <= 0 {
		result.Reason = modelGatewayProfitRecommendationReasonNoCost
		return result
	}
	target := modelgatewaydynamicbilling.SanitizeTargetGrossMargin(config.TargetProfitRate)
	requiredRevenue := modelgatewaydynamicbilling.RequiredRevenueForGrossMargin(costUSD, target)
	result.RequiredRevenueUSD = requiredRevenue
	result.RevenueGapUSD = requiredRevenue - summary.RevenueUSD
	result.CostMarkupMultiplier = modelgatewaydynamicbilling.RevenueMultiplierForGrossMargin(target)
	if summary.RevenueUSD > 0 {
		result.RecommendedRevenueMultiplier = requiredRevenue / summary.RevenueUSD
	}
	if summary.TotalTokens > 0 {
		result.RecommendedFloorPerMTokenUSD = requiredRevenue / float64(summary.TotalTokens) * 1_000_000
	}
	if summary.RevenueUSD <= 0 {
		result.Reason = modelGatewayProfitRecommendationReasonNoRevenue
		return result
	}
	if !modelGatewayProfitRecommendationHasEnoughSamples(summary) {
		result.Reason = modelGatewayProfitRecommendationReasonLowSample
		return result
	}
	result.CanRecommend = true
	if result.CanRecommend {
		result.Reason = modelGatewayProfitRecommendationReasonOK
	} else {
		result.Reason = modelGatewayProfitRecommendationReasonNoRevenue
	}
	return result
}

func enrichModelGatewayProfitRecommendationWithDynamicBilling(recommendation *ModelGatewayProfitRecommendation, config ModelGatewayProfitMonitorConfig) {
	if recommendation == nil {
		return
	}
	currentRevenue := recommendation.RequiredRevenueUSD - recommendation.RevenueGapUSD
	baselines := modelgatewaydynamicbilling.DefaultBaselineSnapshots()
	if len(baselines) == 0 {
		return
	}
	baseQuota := 0.0
	requiredRevenue := 0.0
	costMultiplierSum := 0.0
	costMultiplierWeight := 0.0
	totalTokens := int64(0)
	currentRatio := 0.0
	applied := false
	for _, baseline := range baselines {
		if baseline.CostSource != "profit_24h" {
			continue
		}
		baseQuota += baseline.BaseQuotaAtRatio1
		requiredRevenue += baseline.RequiredRevenueUSD
		totalTokens += baseline.TotalTokens
		if baseline.CostMultiplier > 0 && baseline.BaseQuotaAtRatio1 > 0 {
			costMultiplierSum += baseline.CostMultiplier * baseline.BaseQuotaAtRatio1
			costMultiplierWeight += baseline.BaseQuotaAtRatio1
		}
		if baseline.EffectiveRatio > currentRatio {
			currentRatio = baseline.EffectiveRatio
		}
		if (baseline.FallbackReason == "" || modelgatewaydynamicbilling.IsAutoAppliedLegacyFallback(baseline.FallbackReason)) && baseline.Ratio > 0 {
			applied = true
		}
	}
	if baseQuota <= 0 {
		return
	}
	recommendation.BaseQuotaAtRatio1 = baseQuota
	if costMultiplierWeight > 0 {
		recommendation.CostMultiplier = costMultiplierSum / costMultiplierWeight
		targetRequiredRevenue := requiredRevenue
		if targetRequiredRevenue <= 0 {
			targetRequiredRevenue = recommendation.RequiredRevenueUSD
		}
		if targetRequiredRevenue > 0 {
			recommendation.SuggestedDynamicRatio = targetRequiredRevenue * common.QuotaPerUnit / baseQuota
		} else {
			recommendation.SuggestedDynamicRatio = recommendation.CostMultiplier *
				modelgatewaydynamicbilling.RevenueMultiplierForGrossMargin(recommendation.TargetProfitRate)
		}
		applyModelGatewayProfitDynamicRatioLimit(recommendation, config)
	} else if requiredRevenue > 0 {
		recommendation.SuggestedDynamicRatio = requiredRevenue * common.QuotaPerUnit / baseQuota
		applyModelGatewayProfitDynamicRatioLimit(recommendation, config)
	}
	if recommendation.SuggestedDynamicRatio > 0 {
		recommendation.RequiredRevenueUSD = recommendation.SuggestedDynamicRatio * baseQuota / common.QuotaPerUnit
		if currentRevenue > 0 {
			recommendation.RevenueGapUSD = recommendation.RequiredRevenueUSD - currentRevenue
			recommendation.RecommendedRevenueMultiplier = recommendation.RequiredRevenueUSD / currentRevenue
		}
		if totalTokens > 0 {
			recommendation.RecommendedFloorPerMTokenUSD = recommendation.RequiredRevenueUSD / float64(totalTokens) * 1_000_000
		}
		recommendation.MinimumRevenuePerMBaseQuotaUSD = recommendation.SuggestedDynamicRatio * 1_000_000 / common.QuotaPerUnit
	}
	recommendation.CurrentEffectiveDynamicRatio = currentRatio
	recommendation.DynamicBillingApplied = applied
}

func applyModelGatewayProfitDynamicRatioLimit(recommendation *ModelGatewayProfitRecommendation, config ModelGatewayProfitMonitorConfig) {
	if recommendation == nil || recommendation.SuggestedDynamicRatio <= 0 {
		return
	}
	minLimit, maxLimit := modelGatewayProfitEffectiveDynamicRatioLimits(config)
	rawRatio := recommendation.SuggestedDynamicRatio
	limitedRatio := rawRatio
	reason := ""
	fixedRatio := normalizeModelGatewayProfitMonitorConfig(config).DynamicRatioFixedValue
	if fixedRatio > 0 {
		limitedRatio = fixedRatio
		reason = "fixed_ratio"
	}
	if fixedRatio <= 0 && minLimit > 0 && limitedRatio < minLimit {
		limitedRatio = minLimit
		reason = "min_limit"
	}
	if fixedRatio <= 0 && maxLimit > 0 && limitedRatio > maxLimit {
		limitedRatio = maxLimit
		reason = "max_limit"
	}
	recommendation.SuggestedDynamicRatioRaw = rawRatio
	recommendation.DynamicRatioLimitMin = minLimit
	recommendation.DynamicRatioLimitMax = maxLimit
	recommendation.DynamicRatioFixedValue = fixedRatio
	recommendation.DynamicRatioFixedApplied = fixedRatio > 0
	if math.Abs(limitedRatio-rawRatio) > 0.0000001 {
		recommendation.SuggestedDynamicRatio = limitedRatio
		recommendation.DynamicRatioLimitApplied = true
		recommendation.DynamicRatioLimitReason = reason
	} else if fixedRatio > 0 {
		recommendation.DynamicRatioLimitReason = reason
	}
}

func modelGatewayProfitEffectiveDynamicRatioLimits(config ModelGatewayProfitMonitorConfig) (float64, float64) {
	config = normalizeModelGatewayProfitMonitorConfig(config)
	setting := scheduler_setting.GetSetting()
	minLimit := config.DynamicRatioMinLimit
	if setting.DynamicBillingMinRatio > minLimit {
		minLimit = setting.DynamicBillingMinRatio
	}
	maxLimit := config.DynamicRatioMaxLimit
	if setting.DynamicBillingMaxRatio > 0 && (maxLimit <= 0 || setting.DynamicBillingMaxRatio < maxLimit) {
		maxLimit = setting.DynamicBillingMaxRatio
	}
	if maxLimit > 0 && minLimit > maxLimit {
		maxLimit = minLimit
	}
	return minLimit, maxLimit
}

func modelGatewayProfitRecommendationHasEnoughSamples(summary ModelGatewayProfitMonitorSummary) bool {
	return summary.Requests >= modelGatewayProfitRecommendationMinRequests &&
		summary.SuccessRequests >= modelGatewayProfitRecommendationMinSuccesses &&
		summary.TotalTokens >= modelGatewayProfitRecommendationMinTotalTokens
}

func buildModelGatewayProfitRecommendationSnapshot(payload ModelGatewayProfitMonitorResponse, request CreateModelGatewayProfitRecommendationRequest) (model.ModelGatewayProfitRatioRecommendation, error) {
	request = normalizeModelGatewayProfitRecommendationScopeRequest(request)
	if item, ok := modelGatewayProfitDynamicRatioGroupByScope(payload, request); ok {
		return buildModelGatewayProfitGroupRecommendationSnapshot(payload, request, item)
	}
	summary := payload.Summary
	recommendation := payload.Recommendation
	confidence, riskLevel := modelGatewayProfitRecommendationConfidence(summary, recommendation)
	reasonCode := modelGatewayProfitRecommendationReasonCode(summary, recommendation, riskLevel)
	resourceCost := summary.ResourceAmortizedCostUSD + summary.ResourceLossCostUSD
	input := ModelGatewayProfitRecommendationSnapshotInput{
		Window:         payload.Window,
		Dimension:      payload.Dimension,
		ScopeType:      request.ScopeType,
		ScopeID:        request.ScopeID,
		ScopeKey:       request.ScopeKey,
		ScopeName:      request.ScopeName,
		StartTimestamp: payload.StartTimestamp,
		EndTimestamp:   payload.EndTimestamp,
		Config:         payload.Config,
		Summary:        summary,
		Resources:      payload.Resources,
		Recommendation: recommendation,
	}
	packagePayload := ModelGatewayProfitRecommendationPackage{
		Mode:                 payload.Config.DynamicRatioRecommendationMode,
		GeneratedBy:          "profit_monitor",
		ScopeType:            request.ScopeType,
		ScopeID:              request.ScopeID,
		ScopeKey:             request.ScopeKey,
		ScopeName:            request.ScopeName,
		RiskLevel:            riskLevel,
		Confidence:           confidence,
		ReasonCode:           reasonCode,
		ConstraintCodes:      modelGatewayProfitRecommendationConstraintCodes(),
		SuggestedActionCodes: modelGatewayProfitRecommendationActionCodes(summary, recommendation, riskLevel),
		Metrics: map[string]float64{
			"current_margin":                    summary.GrossMargin,
			"target_profit_rate":                recommendation.TargetProfitRate,
			"revenue_usd":                       summary.RevenueUSD,
			"operating_cost_usd":                summary.OperatingCostUSD,
			"required_revenue_usd":              recommendation.RequiredRevenueUSD,
			"revenue_gap_usd":                   recommendation.RevenueGapUSD,
			"cost_multiplier":                   recommendation.CostMultiplier,
			"cost_markup_multiplier":            recommendation.CostMarkupMultiplier,
			"recommended_revenue_multiplier":    recommendation.RecommendedRevenueMultiplier,
			"recommended_floor_per_m_token_usd": recommendation.RecommendedFloorPerMTokenUSD,
			"suggested_dynamic_ratio":           recommendation.SuggestedDynamicRatio,
			"suggested_dynamic_ratio_raw":       recommendation.SuggestedDynamicRatioRaw,
			"dynamic_ratio_limit_min":           recommendation.DynamicRatioLimitMin,
			"dynamic_ratio_limit_max":           recommendation.DynamicRatioLimitMax,
			"dynamic_ratio_fixed_value":         recommendation.DynamicRatioFixedValue,
			"dynamic_ratio_fixed_applied":       boolToProfitMonitorFloat(recommendation.DynamicRatioFixedApplied),
			"dynamic_ratio_limit_applied":       boolToProfitMonitorFloat(recommendation.DynamicRatioLimitApplied),
			"traffic_cost_usd":                  summary.TrafficCostUSD,
			"resource_cost_usd":                 resourceCost,
			"server_cost_usd":                   summary.ServerCostUSD,
			"upstream_cost_usd":                 summary.UpstreamCostUSD,
			"traffic_real_data_ready":           boolToProfitMonitorFloat(summary.TrafficDataReady && !summary.TrafficEstimated),
			"request_count":                     float64(summary.Requests),
			"success_rate":                      summary.SuccessRate,
			"confidence":                        confidence,
		},
	}
	inputJSON, err := common.Marshal(input)
	if err != nil {
		return model.ModelGatewayProfitRatioRecommendation{}, err
	}
	recommendationJSON, err := common.Marshal(packagePayload)
	if err != nil {
		return model.ModelGatewayProfitRatioRecommendation{}, err
	}
	return model.ModelGatewayProfitRatioRecommendation{
		Window:                       payload.Window,
		Dimension:                    payload.Dimension,
		ScopeType:                    request.ScopeType,
		ScopeID:                      request.ScopeID,
		ScopeKey:                     request.ScopeKey,
		ScopeName:                    request.ScopeName,
		StartTimestamp:               payload.StartTimestamp,
		EndTimestamp:                 payload.EndTimestamp,
		TargetProfitRate:             recommendation.TargetProfitRate,
		RevenueUSD:                   summary.RevenueUSD,
		OperatingCostUSD:             summary.OperatingCostUSD,
		UpstreamCostUSD:              summary.UpstreamCostUSD,
		TrafficCostUSD:               summary.TrafficCostUSD,
		ServerCostUSD:                summary.ServerCostUSD,
		ResourceCostUSD:              resourceCost,
		CurrentMargin:                summary.GrossMargin,
		RequiredRevenueUSD:           recommendation.RequiredRevenueUSD,
		RevenueGapUSD:                recommendation.RevenueGapUSD,
		CostMultiplier:               recommendation.CostMultiplier,
		CostMarkupMultiplier:         recommendation.CostMarkupMultiplier,
		RecommendedRevenueMultiplier: recommendation.RecommendedRevenueMultiplier,
		RecommendedFloorPerMTokenUSD: recommendation.RecommendedFloorPerMTokenUSD,
		Confidence:                   confidence,
		RiskLevel:                    riskLevel,
		Reason:                       reasonCode,
		InputJSON:                    string(inputJSON),
		RecommendationJSON:           string(recommendationJSON),
	}, nil
}

func buildModelGatewayProfitGroupRecommendationSnapshot(payload ModelGatewayProfitMonitorResponse, request CreateModelGatewayProfitRecommendationRequest, item ModelGatewayProfitDynamicRatioGroup) (model.ModelGatewayProfitRatioRecommendation, error) {
	if request.ScopeKey == "" {
		request.ScopeKey = item.Group
	}
	if request.ScopeName == "" {
		request.ScopeName = item.Group
	}
	summary := ModelGatewayProfitMonitorSummary{
		Requests:                 item.RequestCount,
		SuccessRequests:          item.SuccessRequestCount,
		SuccessRate:              ratioOrZero(float64(item.SuccessRequestCount), float64(item.RequestCount)),
		TotalTokens:              item.TotalTokens,
		RevenueUSD:               item.CurrentRevenueUSD,
		UpstreamCostUSD:          item.UpstreamCostUSD,
		TrafficCostUSD:           item.TrafficCostUSD,
		TrafficEstimated:         item.TrafficEstimated,
		TrafficDataReady:         item.TrafficDataReady,
		ServerCostUSD:            item.ServerCostUSD,
		ResourceAmortizedCostUSD: item.ResourceCostUSD,
		OperatingCostUSD:         item.OperatingCostUSD,
	}
	summary.ProfitUSD = summary.RevenueUSD - summary.OperatingCostUSD
	summary.GrossMargin = ratioOrZero(summary.ProfitUSD, summary.RevenueUSD)
	summary.UpstreamOnlyProfitUSD = summary.RevenueUSD - summary.UpstreamCostUSD
	summary.UpstreamOnlyMargin = ratioOrZero(summary.UpstreamOnlyProfitUSD, summary.RevenueUSD)
	recommendation := ModelGatewayProfitRecommendation{
		TargetProfitRate:               item.ProfitRate,
		RequiredRevenueUSD:             item.RequiredRevenueUSD,
		RevenueGapUSD:                  item.RevenueGapUSD,
		CostMultiplier:                 item.CostMultiplier,
		CostMarkupMultiplier:           item.CostMarkupMultiplier,
		BaseQuotaAtRatio1:              item.BaseQuotaAtRatio1,
		SuggestedDynamicRatio:          item.EffectiveRatio,
		SuggestedDynamicRatioRaw:       item.TargetRatio,
		CurrentEffectiveDynamicRatio:   item.ActualRatio,
		DynamicBillingApplied:          item.Applied,
		DynamicRatioFixedValue:         item.DynamicRatioFixedValue,
		DynamicRatioFixedApplied:       item.DynamicRatioFixedApplied,
		DynamicRatioLimitApplied:       item.Clamped || item.DynamicRatioFixedApplied,
		DynamicRatioLimitReason:        modelGatewayProfitDynamicRatioLimitReason(item),
		MinimumRevenuePerMBaseQuotaUSD: ratioOrZero(item.RequiredRevenueUSD, item.BaseQuotaAtRatio1) * 1_000_000,
		CanRecommend:                   item.RequiredRevenueUSD > 0 && item.BaseQuotaAtRatio1 > 0,
		Reason:                         modelGatewayProfitRecommendationReasonOK,
	}
	if recommendation.TargetProfitRate <= 0 {
		recommendation.TargetProfitRate = payload.Config.TargetProfitRate
	}
	recommendation.TargetProfitRate = modelgatewaydynamicbilling.SanitizeTargetGrossMargin(recommendation.TargetProfitRate)
	recommendation.RecommendedRevenueMultiplier = ratioOrZero(item.RequiredRevenueUSD, item.CurrentRevenueUSD)
	if item.TotalTokens > 0 {
		recommendation.RecommendedFloorPerMTokenUSD = item.RequiredRevenueUSD / float64(item.TotalTokens) * 1_000_000
	}
	if !recommendation.CanRecommend {
		recommendation.Reason = modelGatewayProfitRecommendationReasonNoRevenue
	} else if item.FallbackReason == modelgatewaydynamicbilling.FallbackNoCostData {
		recommendation.Reason = modelGatewayProfitRecommendationReasonNoCost
	}
	confidence, riskLevel := modelGatewayProfitRecommendationConfidence(summary, recommendation)
	reasonCode := modelGatewayProfitRecommendationReasonCode(summary, recommendation, riskLevel)
	input := ModelGatewayProfitRecommendationSnapshotInput{
		Window:         payload.Window,
		Dimension:      model.ModelGatewayProfitResourceScopeGroup,
		ScopeType:      request.ScopeType,
		ScopeID:        request.ScopeID,
		ScopeKey:       request.ScopeKey,
		ScopeName:      request.ScopeName,
		StartTimestamp: payload.StartTimestamp,
		EndTimestamp:   payload.EndTimestamp,
		Config:         payload.Config,
		Summary:        summary,
		Resources:      payload.Resources,
		Recommendation: recommendation,
	}
	packagePayload := ModelGatewayProfitRecommendationPackage{
		Mode:                 payload.Config.DynamicRatioRecommendationMode,
		GeneratedBy:          "profit_monitor",
		ScopeType:            request.ScopeType,
		ScopeID:              request.ScopeID,
		ScopeKey:             request.ScopeKey,
		ScopeName:            request.ScopeName,
		RiskLevel:            riskLevel,
		Confidence:           confidence,
		ReasonCode:           reasonCode,
		ConstraintCodes:      modelGatewayProfitRecommendationConstraintCodes(),
		SuggestedActionCodes: modelGatewayProfitRecommendationActionCodes(summary, recommendation, riskLevel),
		Metrics: map[string]float64{
			"current_margin":                    summary.GrossMargin,
			"target_profit_rate":                recommendation.TargetProfitRate,
			"revenue_usd":                       summary.RevenueUSD,
			"operating_cost_usd":                summary.OperatingCostUSD,
			"required_revenue_usd":              recommendation.RequiredRevenueUSD,
			"revenue_gap_usd":                   recommendation.RevenueGapUSD,
			"cost_multiplier":                   recommendation.CostMultiplier,
			"cost_markup_multiplier":            recommendation.CostMarkupMultiplier,
			"recommended_revenue_multiplier":    recommendation.RecommendedRevenueMultiplier,
			"recommended_floor_per_m_token_usd": recommendation.RecommendedFloorPerMTokenUSD,
			"suggested_dynamic_ratio":           recommendation.SuggestedDynamicRatio,
			"suggested_dynamic_ratio_raw":       recommendation.SuggestedDynamicRatioRaw,
			"current_effective_dynamic_ratio":   recommendation.CurrentEffectiveDynamicRatio,
			"dynamic_ratio_fixed_value":         recommendation.DynamicRatioFixedValue,
			"dynamic_ratio_fixed_applied":       boolToProfitMonitorFloat(recommendation.DynamicRatioFixedApplied),
			"dynamic_ratio_limit_applied":       boolToProfitMonitorFloat(recommendation.DynamicRatioLimitApplied),
			"request_count":                     float64(summary.Requests),
			"success_rate":                      summary.SuccessRate,
			"confidence":                        confidence,
		},
	}
	inputJSON, err := common.Marshal(input)
	if err != nil {
		return model.ModelGatewayProfitRatioRecommendation{}, err
	}
	recommendationJSON, err := common.Marshal(packagePayload)
	if err != nil {
		return model.ModelGatewayProfitRatioRecommendation{}, err
	}
	return model.ModelGatewayProfitRatioRecommendation{
		Window:                       payload.Window,
		Dimension:                    model.ModelGatewayProfitResourceScopeGroup,
		ScopeType:                    request.ScopeType,
		ScopeID:                      request.ScopeID,
		ScopeKey:                     request.ScopeKey,
		ScopeName:                    request.ScopeName,
		StartTimestamp:               payload.StartTimestamp,
		EndTimestamp:                 payload.EndTimestamp,
		TargetProfitRate:             recommendation.TargetProfitRate,
		RevenueUSD:                   summary.RevenueUSD,
		OperatingCostUSD:             summary.OperatingCostUSD,
		UpstreamCostUSD:              summary.UpstreamCostUSD,
		TrafficCostUSD:               summary.TrafficCostUSD,
		ServerCostUSD:                summary.ServerCostUSD,
		ResourceCostUSD:              summary.ResourceAmortizedCostUSD,
		CurrentMargin:                summary.GrossMargin,
		RequiredRevenueUSD:           recommendation.RequiredRevenueUSD,
		RevenueGapUSD:                recommendation.RevenueGapUSD,
		CostMultiplier:               recommendation.CostMultiplier,
		CostMarkupMultiplier:         recommendation.CostMarkupMultiplier,
		RecommendedRevenueMultiplier: recommendation.RecommendedRevenueMultiplier,
		RecommendedFloorPerMTokenUSD: recommendation.RecommendedFloorPerMTokenUSD,
		Confidence:                   confidence,
		RiskLevel:                    riskLevel,
		Reason:                       reasonCode,
		InputJSON:                    string(inputJSON),
		RecommendationJSON:           string(recommendationJSON),
	}, nil
}

func normalizeModelGatewayProfitRecommendationScopeRequest(request CreateModelGatewayProfitRecommendationRequest) CreateModelGatewayProfitRecommendationRequest {
	request.ScopeType = model.NormalizeModelGatewayProfitResourceScope(request.ScopeType)
	request.ScopeKey = strings.TrimSpace(request.ScopeKey)
	request.ScopeName = strings.TrimSpace(request.ScopeName)
	return request
}

func modelGatewayProfitDynamicRatioLimitReason(item ModelGatewayProfitDynamicRatioGroup) string {
	if item.DynamicRatioFixedApplied {
		return "fixed_ratio"
	}
	if !item.Clamped {
		return ""
	}
	if item.TargetRatio > 0 && item.EffectiveRatio > 0 {
		if item.EffectiveRatio < item.TargetRatio {
			return "max_limit"
		}
		if item.EffectiveRatio > item.TargetRatio {
			return "min_limit"
		}
	}
	return "ratio_limit"
}

func modelGatewayProfitRecommendationConfidence(summary ModelGatewayProfitMonitorSummary, recommendation ModelGatewayProfitRecommendation) (float64, string) {
	if !recommendation.CanRecommend {
		return 0, "insufficient_data"
	}
	confidence := 0.88
	if summary.Requests < 20 {
		confidence -= 0.18
	} else if summary.Requests < 100 {
		confidence -= 0.08
	}
	if summary.TrafficEstimated || !summary.TrafficDataReady {
		confidence -= 0.08
	}
	if summary.SuccessRate > 0 && summary.SuccessRate < 0.8 {
		confidence -= 0.08
	}
	if summary.ResourceAmortizedCostUSD+summary.ResourceLossCostUSD <= 0 {
		confidence -= 0.04
	}
	riskLevel := "low"
	if recommendation.RevenueGapUSD > 0 {
		riskLevel = "medium"
	}
	if summary.GrossMargin < 0 || recommendation.RecommendedRevenueMultiplier >= 1.5 {
		riskLevel = "high"
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return confidence, riskLevel
}

func modelGatewayProfitRecommendationReasonCode(summary ModelGatewayProfitMonitorSummary, recommendation ModelGatewayProfitRecommendation, riskLevel string) string {
	switch recommendation.Reason {
	case modelGatewayProfitRecommendationReasonDisabled,
		modelGatewayProfitRecommendationReasonNoCost,
		modelGatewayProfitRecommendationReasonNoRevenue,
		modelGatewayProfitRecommendationReasonLowSample:
		return recommendation.Reason
	}
	if recommendation.RevenueGapUSD <= 0 {
		return "target_covered"
	}
	if riskLevel == "high" {
		return "high_gap"
	}
	if summary.TrafficEstimated || !summary.TrafficDataReady {
		return "traffic_estimated"
	}
	return "below_target"
}

func modelGatewayProfitRecommendationConstraintCodes() []string {
	return []string{
		"snapshot_only",
		"insufficient_data_no_direct_adjust",
		"billing_expression_remains_source",
	}
}

func modelGatewayProfitRecommendationActionCodes(summary ModelGatewayProfitMonitorSummary, recommendation ModelGatewayProfitRecommendation, riskLevel string) []string {
	actions := make([]string, 0, 4)
	if recommendation.CanRecommend && recommendation.RecommendedRevenueMultiplier > 1 {
		actions = append(actions, "gray_raise_dynamic_ratio")
	} else {
		actions = append(actions, "keep_observing")
	}
	if summary.TrafficEstimated || !summary.TrafficDataReady {
		actions = append(actions, "complete_real_traffic_data")
	}
	if summary.ResourceAmortizedCostUSD+summary.ResourceLossCostUSD <= 0 {
		actions = append(actions, "add_resource_cost_ledger")
	}
	if riskLevel == "high" {
		actions = append(actions, "check_cost_anomalies")
	}
	return actions
}

func compactModelGatewayProfitDecisionRemark(value string) (string, bool) {
	value = strings.TrimSpace(value)
	const limit = 200
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}

func setModelGatewayProfitRecommendationDecisionAuditSummary(c *gin.Context, row *model.ModelGatewayProfitRatioRecommendation, beforeStatus string, beforeMultiplier float64, beforeRemark string) {
	if row == nil {
		return
	}
	beforeRemarkText, beforeRemarkTruncated := compactModelGatewayProfitDecisionRemark(beforeRemark)
	afterRemarkText, afterRemarkTruncated := compactModelGatewayProfitDecisionRemark(row.DecisionRemark)
	middleware.SetAdminAuditSummary(c, "operation", "update_profit_recommendation_decision")
	middleware.SetAdminAuditSummary(c, "recommendation_id", row.Id)
	middleware.SetAdminAuditSummary(c, "decision_status_before", beforeStatus)
	middleware.SetAdminAuditSummary(c, "decision_status_after", row.DecisionStatus)
	middleware.SetAdminAuditSummary(c, "decision_status_changed", beforeStatus != row.DecisionStatus)
	middleware.SetAdminAuditSummary(c, "planned_revenue_multiplier_before", beforeMultiplier)
	middleware.SetAdminAuditSummary(c, "planned_revenue_multiplier_after", row.PlannedRevenueMultiplier)
	middleware.SetAdminAuditSummary(c, "planned_revenue_multiplier_changed", beforeMultiplier != row.PlannedRevenueMultiplier)
	middleware.SetAdminAuditSummary(c, "decision_remark_before", beforeRemarkText)
	middleware.SetAdminAuditSummary(c, "decision_remark_after", afterRemarkText)
	middleware.SetAdminAuditSummary(c, "decision_remark_before_length", len([]rune(strings.TrimSpace(beforeRemark))))
	middleware.SetAdminAuditSummary(c, "decision_remark_after_length", len([]rune(strings.TrimSpace(row.DecisionRemark))))
	middleware.SetAdminAuditSummary(c, "decision_remark_changed", strings.TrimSpace(beforeRemark) != strings.TrimSpace(row.DecisionRemark))
	middleware.SetAdminAuditSummary(c, "decision_remark_before_truncated", beforeRemarkTruncated)
	middleware.SetAdminAuditSummary(c, "decision_remark_after_truncated", afterRemarkTruncated)
	middleware.SetAdminAuditSummary(c, "recommendation_window", row.Window)
	middleware.SetAdminAuditSummary(c, "recommendation_dimension", row.Dimension)
	middleware.SetAdminAuditSummary(c, "recommendation_scope_type", row.ScopeType)
	middleware.SetAdminAuditSummary(c, "recommendation_scope_id", row.ScopeID)
	middleware.SetAdminAuditSummary(c, "recommendation_scope_key", row.ScopeKey)
	middleware.SetAdminAuditSummary(c, "recommendation_scope_name", row.ScopeName)
	middleware.SetAdminAuditSummary(c, "recommendation_risk_level", row.RiskLevel)
	middleware.SetAdminAuditSummary(c, "recommendation_reason", row.Reason)
	middleware.SetAdminAuditSummary(c, "recommended_revenue_multiplier", row.RecommendedRevenueMultiplier)
	middleware.SetAdminAuditSummary(c, "recommended_floor_per_m_token_usd", row.RecommendedFloorPerMTokenUSD)
	middleware.SetAdminAuditSummary(c, "cost_multiplier", row.CostMultiplier)
	middleware.SetAdminAuditSummary(c, "cost_markup_multiplier", row.CostMarkupMultiplier)
	middleware.SetAdminAuditSummary(c, "decision_operator_id", row.DecisionOperatorID)
	middleware.SetAdminAuditSummary(c, "decision_operator_name", row.DecisionOperatorName)
	middleware.SetAdminAuditSummary(c, "decision_updated_at", row.DecisionUpdatedAt)
}

func recordModelGatewayProfitRecommendationDecisionLog(c *gin.Context, row *model.ModelGatewayProfitRatioRecommendation, beforeStatus string, beforeMultiplier float64, beforeRemark string) {
	if model.LOG_DB == nil || row == nil {
		return
	}
	userID := c.GetInt("id")
	model.RecordLogWithAdminInfo(userID, model.LogTypeManage, "更新盈利监控建议快照决策", map[string]interface{}{
		"recommendation_id":                 row.Id,
		"old_decision_status":               beforeStatus,
		"new_decision_status":               row.DecisionStatus,
		"old_planned_revenue_multiplier":    beforeMultiplier,
		"new_planned_revenue_multiplier":    row.PlannedRevenueMultiplier,
		"old_decision_remark":               beforeRemark,
		"new_decision_remark":               row.DecisionRemark,
		"cost_multiplier":                   row.CostMultiplier,
		"cost_markup_multiplier":            row.CostMarkupMultiplier,
		"recommended_revenue_multiplier":    row.RecommendedRevenueMultiplier,
		"recommended_floor_per_m_token_usd": row.RecommendedFloorPerMTokenUSD,
		"recommendation_window":             row.Window,
		"recommendation_dimension":          row.Dimension,
		"recommendation_scope_type":         row.ScopeType,
		"recommendation_scope_id":           row.ScopeID,
		"recommendation_scope_key":          row.ScopeKey,
		"recommendation_scope_name":         row.ScopeName,
		"recommendation_risk_level":         row.RiskLevel,
		"recommendation_reason":             row.Reason,
		"decision_operator_id":              row.DecisionOperatorID,
		"decision_operator_name":            row.DecisionOperatorName,
		"decision_updated_at":               row.DecisionUpdatedAt,
	})
}

func applyModelGatewayProfitCanaryTaskRequest(row *model.ModelGatewayProfitCanaryTask, request UpsertModelGatewayProfitCanaryTaskRequest, creating bool, c *gin.Context) error {
	if row == nil {
		return errors.New("灰度任务不存在")
	}
	var recommendation *model.ModelGatewayProfitRatioRecommendation
	if request.RecommendationID != nil {
		row.RecommendationID = *request.RecommendationID
	}
	if row.RecommendationID > 0 {
		if found, err := model.GetModelGatewayProfitRatioRecommendation(row.RecommendationID); err == nil {
			recommendation = found
		}
	}
	if request.Title != nil {
		row.Title = strings.TrimSpace(*request.Title)
	}
	if creating && row.Title == "" && recommendation != nil {
		row.Title = "盈利建议 #" + strconv.Itoa(recommendation.Id) + " 灰度任务"
	}
	if strings.TrimSpace(request.Status) != "" {
		normalizedStatus := model.NormalizeModelGatewayProfitCanaryTaskStatus(request.Status)
		if !strings.EqualFold(strings.TrimSpace(request.Status), normalizedStatus) {
			return errors.New("无效的灰度任务状态")
		}
		row.Status = normalizedStatus
	}
	if request.ScopeType != nil {
		normalizedScope := model.NormalizeModelGatewayProfitResourceScope(*request.ScopeType)
		if strings.TrimSpace(*request.ScopeType) != "" && !strings.EqualFold(strings.TrimSpace(*request.ScopeType), normalizedScope) {
			return errors.New("无效的灰度范围")
		}
		row.ScopeType = normalizedScope
	} else if creating && recommendation != nil {
		if recommendation.ScopeType != "" && recommendation.ScopeType != model.ModelGatewayProfitResourceScopeGlobal {
			row.ScopeType = recommendation.ScopeType
			row.ScopeID = recommendation.ScopeID
			row.ScopeKey = recommendation.ScopeKey
		} else {
			switch recommendation.Dimension {
			case model.ModelGatewayProfitResourceScopeChannel,
				model.ModelGatewayProfitResourceScopeGroup,
				model.ModelGatewayProfitResourceScopeModel:
				row.ScopeType = recommendation.Dimension
			}
		}
	}
	if request.ScopeID != nil {
		row.ScopeID = *request.ScopeID
	}
	if request.ScopeKey != nil {
		row.ScopeKey = strings.TrimSpace(*request.ScopeKey)
	}
	if request.BaselineRevenueMultiplier != nil {
		row.BaselineRevenueMultiplier = *request.BaselineRevenueMultiplier
	}
	if request.PlannedRevenueMultiplier != nil {
		row.PlannedRevenueMultiplier = *request.PlannedRevenueMultiplier
	}
	if request.RecommendedRevenueMultiplier != nil {
		row.RecommendedRevenueMultiplier = *request.RecommendedRevenueMultiplier
	}
	if creating && recommendation != nil {
		if row.RecommendedRevenueMultiplier <= 0 {
			row.RecommendedRevenueMultiplier = recommendation.RecommendedRevenueMultiplier
		}
		if row.PlannedRevenueMultiplier <= 0 {
			if recommendation.PlannedRevenueMultiplier > 0 {
				row.PlannedRevenueMultiplier = recommendation.PlannedRevenueMultiplier
			} else {
				row.PlannedRevenueMultiplier = recommendation.RecommendedRevenueMultiplier
			}
		}
	}
	if request.PlannedStartAt != nil {
		row.PlannedStartAt = *request.PlannedStartAt
	}
	if request.PlannedEndAt != nil {
		row.PlannedEndAt = *request.PlannedEndAt
	}
	if request.ActualStartAt != nil {
		row.ActualStartAt = *request.ActualStartAt
	}
	if request.ActualEndAt != nil {
		row.ActualEndAt = *request.ActualEndAt
	}
	if request.ObservationWindowSeconds != nil {
		row.ObservationWindowSeconds = *request.ObservationWindowSeconds
	}
	if request.WatchMetrics != nil {
		metrics := normalizeModelGatewayProfitCanaryWatchMetrics(request.WatchMetrics)
		payload, err := common.Marshal(metrics)
		if err != nil {
			return err
		}
		row.WatchMetricJSON = string(payload)
		row.WatchMetrics = metrics
	} else if creating {
		payload, err := common.Marshal(modelGatewayProfitCanaryDefaultWatchMetrics)
		if err != nil {
			return err
		}
		row.WatchMetricJSON = string(payload)
		row.WatchMetrics = append([]string{}, modelGatewayProfitCanaryDefaultWatchMetrics...)
	}
	if request.ResultSummary != nil {
		row.ResultSummary = strings.TrimSpace(*request.ResultSummary)
	}

	now := common.GetTimestamp()
	if c != nil {
		row.UpdatedByID = c.GetInt("id")
		row.UpdatedByName = strings.TrimSpace(c.GetString("username"))
		if creating {
			row.CreatedByID = row.UpdatedByID
			row.CreatedByName = row.UpdatedByName
		}
	}
	if row.Status == model.ModelGatewayProfitCanaryTaskStatusRunning && row.ActualStartAt <= 0 {
		row.ActualStartAt = now
	}
	if (row.Status == model.ModelGatewayProfitCanaryTaskStatusCompleted || row.Status == model.ModelGatewayProfitCanaryTaskStatusCanceled) && row.ActualEndAt <= 0 {
		row.ActualEndAt = now
	}
	row.Normalize()
	if row.Title == "" {
		return errors.New("灰度任务标题不能为空")
	}
	return nil
}

func normalizeModelGatewayProfitCanaryWatchMetrics(values []string) []string {
	seen := map[string]struct{}{}
	metrics := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if !isAllowedModelGatewayProfitCanaryWatchMetric(normalized) {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		metrics = append(metrics, normalized)
	}
	if len(metrics) == 0 {
		metrics = append(metrics, modelGatewayProfitCanaryDefaultWatchMetrics...)
	}
	return metrics
}

func isAllowedModelGatewayProfitCanaryWatchMetric(value string) bool {
	switch value {
	case "gross_margin",
		"success_rate",
		"traffic_cost_usd",
		"resource_cost_usd",
		"revenue_gap_usd",
		"request_count",
		"upstream_cost_usd",
		"operating_cost_usd":
		return true
	default:
		return false
	}
}

func hydrateModelGatewayProfitCanaryTaskResponse(row *model.ModelGatewayProfitCanaryTask) {
	if row == nil {
		return
	}
	if strings.TrimSpace(row.WatchMetricJSON) == "" {
		return
	}
	var metrics []string
	if err := common.UnmarshalJsonStr(row.WatchMetricJSON, &metrics); err == nil {
		row.WatchMetrics = metrics
	}
}

func recordModelGatewayProfitCanaryTaskLog(c *gin.Context, row *model.ModelGatewayProfitCanaryTask, action string) {
	if model.LOG_DB == nil || row == nil {
		return
	}
	userID := 0
	if c != nil {
		userID = c.GetInt("id")
	}
	model.RecordLogWithAdminInfo(userID, model.LogTypeManage, "更新盈利监控灰度任务", map[string]interface{}{
		"action":                         action,
		"canary_task_id":                 row.Id,
		"recommendation_id":              row.RecommendationID,
		"title":                          row.Title,
		"status":                         row.Status,
		"scope_type":                     row.ScopeType,
		"scope_id":                       row.ScopeID,
		"scope_key":                      row.ScopeKey,
		"baseline_revenue_multiplier":    row.BaselineRevenueMultiplier,
		"planned_revenue_multiplier":     row.PlannedRevenueMultiplier,
		"recommended_revenue_multiplier": row.RecommendedRevenueMultiplier,
		"planned_start_at":               row.PlannedStartAt,
		"planned_end_at":                 row.PlannedEndAt,
		"actual_start_at":                row.ActualStartAt,
		"actual_end_at":                  row.ActualEndAt,
		"observation_window_seconds":     row.ObservationWindowSeconds,
		"watch_metrics":                  row.WatchMetrics,
	})
}

func applyModelGatewayProfitResourceRequest(row *model.ModelGatewayProfitResourceCost, request UpsertModelGatewayProfitResourceRequest, creating bool) error {
	if row == nil {
		return errors.New("资源成本不存在")
	}
	if request.Name != nil {
		row.Name = strings.TrimSpace(*request.Name)
	}
	if request.ResourceType != nil {
		row.ResourceType = *request.ResourceType
	}
	if request.ScopeType != nil {
		row.ScopeType = *request.ScopeType
	}
	if request.ScopeID != nil {
		row.ScopeID = *request.ScopeID
	}
	if request.ScopeKey != nil {
		row.ScopeKey = *request.ScopeKey
	}
	if request.AmountUSD != nil {
		row.AmountUSD = *request.AmountUSD
	}
	if request.PeriodSeconds != nil {
		row.PeriodSeconds = *request.PeriodSeconds
	}
	if request.AmortizeStartAt != nil {
		row.AmortizeStartAt = *request.AmortizeStartAt
	}
	if request.AmortizeEndAt != nil {
		row.AmortizeEndAt = *request.AmortizeEndAt
	}
	if request.LossAmountUSD != nil {
		row.LossAmountUSD = *request.LossAmountUSD
	}
	if request.LossRecordedAt != nil {
		row.LossRecordedAt = *request.LossRecordedAt
	}
	if request.AllocationMode != nil {
		row.AllocationMode = *request.AllocationMode
	}
	if request.Enabled != nil {
		row.Enabled = *request.Enabled
	}
	if request.Remark != nil {
		row.Remark = *request.Remark
	}
	if request.Metadata != nil {
		payload, err := common.Marshal(request.Metadata)
		if err != nil {
			return err
		}
		row.MetadataJSON = string(payload)
	}
	row.Normalize()
	if creating && row.Name == "" {
		return errors.New("资源名称不能为空")
	}
	if row.Name == "" {
		return errors.New("资源名称不能为空")
	}
	return nil
}

func ratioOrZero(numerator float64, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	value := numerator / denominator
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func boolToProfitMonitorFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func minProfitMonitorInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxProfitMonitorInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
