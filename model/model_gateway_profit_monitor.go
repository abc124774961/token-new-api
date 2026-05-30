package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

type ModelGatewayProfitRatioRecommendationFilter struct {
	Window         string
	Dimension      string
	StartTimestamp int64
	EndTimestamp   int64
}

type ModelGatewayProfitCanaryTaskFilter struct {
	RecommendationID int
	Status           string
}

const (
	ModelGatewayProfitResourceTypeAccountPool = "account_pool"
	ModelGatewayProfitResourceTypeProxy       = "proxy"
	ModelGatewayProfitResourceTypeServer      = "server"
	ModelGatewayProfitResourceTypeBandwidth   = "bandwidth"
	ModelGatewayProfitResourceTypeOther       = "other"

	ModelGatewayProfitResourceScopeGlobal  = "global"
	ModelGatewayProfitResourceScopeChannel = "channel"
	ModelGatewayProfitResourceScopeGroup   = "group"
	ModelGatewayProfitResourceScopeModel   = "model"

	ModelGatewayProfitResourceAllocationGlobal  = "global"
	ModelGatewayProfitResourceAllocationRevenue = "revenue"
	ModelGatewayProfitResourceAllocationRequest = "request"

	ModelGatewayProfitRecommendationDecisionPending = "pending"
	ModelGatewayProfitRecommendationDecisionCanary  = "canary"
	ModelGatewayProfitRecommendationDecisionAdopted = "adopted"
	ModelGatewayProfitRecommendationDecisionIgnored = "ignored"

	ModelGatewayProfitCanaryTaskStatusPlanned   = "planned"
	ModelGatewayProfitCanaryTaskStatusRunning   = "running"
	ModelGatewayProfitCanaryTaskStatusCompleted = "completed"
	ModelGatewayProfitCanaryTaskStatusCanceled  = "canceled"
)

type ModelGatewayProfitResourceCost struct {
	Id              int     `json:"id" gorm:"primaryKey"`
	Name            string  `json:"name" gorm:"type:varchar(191);not null;index"`
	ResourceType    string  `json:"resource_type" gorm:"type:varchar(64);index;default:'other'"`
	ScopeType       string  `json:"scope_type" gorm:"type:varchar(32);index;default:'global'"`
	ScopeID         int     `json:"scope_id" gorm:"index;default:0"`
	ScopeKey        string  `json:"scope_key" gorm:"type:varchar(191);index;default:''"`
	AmountUSD       float64 `json:"amount_usd" gorm:"type:decimal(18,8);default:0"`
	PeriodSeconds   int     `json:"period_seconds" gorm:"default:86400"`
	AmortizeStartAt int64   `json:"amortize_start_at" gorm:"bigint;index;default:0"`
	AmortizeEndAt   int64   `json:"amortize_end_at" gorm:"bigint;index;default:0"`
	LossAmountUSD   float64 `json:"loss_amount_usd" gorm:"type:decimal(18,8);default:0"`
	LossRecordedAt  int64   `json:"loss_recorded_at" gorm:"bigint;index;default:0"`
	AllocationMode  string  `json:"allocation_mode" gorm:"type:varchar(32);default:'revenue'"`
	Enabled         bool    `json:"enabled" gorm:"default:true;index"`
	Remark          string  `json:"remark" gorm:"type:varchar(255);default:''"`
	MetadataJSON    string  `json:"metadata_json" gorm:"type:text"`
	CreatedAt       int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt       int64   `json:"updated_at" gorm:"bigint;index"`
	WindowCostUSD   float64 `json:"window_cost_usd,omitempty" gorm:"-"`
	WindowLossUSD   float64 `json:"window_loss_usd,omitempty" gorm:"-"`
	WindowTotalUSD  float64 `json:"window_total_usd,omitempty" gorm:"-"`
}

// ModelGatewayProfitRatioRecommendation stores an auditable dynamic ratio recommendation snapshot.
type ModelGatewayProfitRatioRecommendation struct {
	Id                           int     `json:"id" gorm:"primaryKey"`
	Window                       string  `json:"window" gorm:"type:varchar(32);index;default:'24h'"`
	Dimension                    string  `json:"dimension" gorm:"type:varchar(32);index;default:'group'"`
	StartTimestamp               int64   `json:"start_timestamp" gorm:"bigint;index"`
	EndTimestamp                 int64   `json:"end_timestamp" gorm:"bigint;index"`
	TargetProfitRate             float64 `json:"target_profit_rate" gorm:"type:decimal(18,8);default:0"`
	RevenueUSD                   float64 `json:"revenue_usd" gorm:"type:decimal(18,8);default:0"`
	OperatingCostUSD             float64 `json:"operating_cost_usd" gorm:"type:decimal(18,8);default:0"`
	UpstreamCostUSD              float64 `json:"upstream_cost_usd" gorm:"type:decimal(18,8);default:0"`
	TrafficCostUSD               float64 `json:"traffic_cost_usd" gorm:"type:decimal(18,8);default:0"`
	ServerCostUSD                float64 `json:"server_cost_usd" gorm:"type:decimal(18,8);default:0"`
	ResourceCostUSD              float64 `json:"resource_cost_usd" gorm:"type:decimal(18,8);default:0"`
	CurrentMargin                float64 `json:"current_margin" gorm:"type:decimal(18,8);default:0"`
	RequiredRevenueUSD           float64 `json:"required_revenue_usd" gorm:"type:decimal(18,8);default:0"`
	RevenueGapUSD                float64 `json:"revenue_gap_usd" gorm:"type:decimal(18,8);default:0"`
	CostMultiplier               float64 `json:"cost_multiplier" gorm:"type:decimal(18,8);default:0"`
	CostMarkupMultiplier         float64 `json:"cost_markup_multiplier" gorm:"type:decimal(18,8);default:0"`
	RecommendedRevenueMultiplier float64 `json:"recommended_revenue_multiplier" gorm:"type:decimal(18,8);default:0"`
	RecommendedFloorPerMTokenUSD float64 `json:"recommended_floor_per_m_token_usd" gorm:"type:decimal(18,8);default:0"`
	Confidence                   float64 `json:"confidence" gorm:"type:decimal(18,8);default:0"`
	RiskLevel                    string  `json:"risk_level" gorm:"type:varchar(32);index;default:'medium'"`
	Reason                       string  `json:"reason" gorm:"type:varchar(255);default:''"`
	InputJSON                    string  `json:"input_json" gorm:"type:text"`
	RecommendationJSON           string  `json:"recommendation_json" gorm:"type:text"`
	DecisionStatus               string  `json:"decision_status" gorm:"type:varchar(32);index;default:'pending'"`
	DecisionRemark               string  `json:"decision_remark" gorm:"type:text"`
	PlannedRevenueMultiplier     float64 `json:"planned_revenue_multiplier" gorm:"type:decimal(18,8);default:0"`
	DecisionOperatorID           int     `json:"decision_operator_id" gorm:"index;default:0"`
	DecisionOperatorName         string  `json:"decision_operator_name" gorm:"type:varchar(191);default:''"`
	DecisionUpdatedAt            int64   `json:"decision_updated_at" gorm:"bigint;index;default:0"`
	CreatedAt                    int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt                    int64   `json:"updated_at" gorm:"bigint;index"`
}

// ModelGatewayProfitCanaryTask records the operational follow-up for a ratio recommendation.
type ModelGatewayProfitCanaryTask struct {
	Id                           int      `json:"id" gorm:"primaryKey"`
	RecommendationID             int      `json:"recommendation_id" gorm:"index;default:0"`
	Title                        string   `json:"title" gorm:"type:varchar(191);not null;index"`
	Status                       string   `json:"status" gorm:"type:varchar(32);index;default:'planned'"`
	ScopeType                    string   `json:"scope_type" gorm:"type:varchar(32);index;default:'global'"`
	ScopeID                      int      `json:"scope_id" gorm:"index;default:0"`
	ScopeKey                     string   `json:"scope_key" gorm:"type:varchar(191);index;default:''"`
	BaselineRevenueMultiplier    float64  `json:"baseline_revenue_multiplier" gorm:"type:decimal(18,8);default:0"`
	PlannedRevenueMultiplier     float64  `json:"planned_revenue_multiplier" gorm:"type:decimal(18,8);default:0"`
	RecommendedRevenueMultiplier float64  `json:"recommended_revenue_multiplier" gorm:"type:decimal(18,8);default:0"`
	PlannedStartAt               int64    `json:"planned_start_at" gorm:"bigint;index;default:0"`
	PlannedEndAt                 int64    `json:"planned_end_at" gorm:"bigint;index;default:0"`
	ActualStartAt                int64    `json:"actual_start_at" gorm:"bigint;index;default:0"`
	ActualEndAt                  int64    `json:"actual_end_at" gorm:"bigint;index;default:0"`
	ObservationWindowSeconds     int      `json:"observation_window_seconds" gorm:"default:7200"`
	WatchMetricJSON              string   `json:"watch_metric_json" gorm:"type:text"`
	WatchMetrics                 []string `json:"watch_metrics,omitempty" gorm:"-"`
	ResultSummary                string   `json:"result_summary" gorm:"type:text"`
	CreatedByID                  int      `json:"created_by_id" gorm:"index;default:0"`
	CreatedByName                string   `json:"created_by_name" gorm:"type:varchar(191);default:''"`
	UpdatedByID                  int      `json:"updated_by_id" gorm:"index;default:0"`
	UpdatedByName                string   `json:"updated_by_name" gorm:"type:varchar(191);default:''"`
	CreatedAt                    int64    `json:"created_at" gorm:"bigint;index"`
	UpdatedAt                    int64    `json:"updated_at" gorm:"bigint;index"`
}

func (ModelGatewayProfitResourceCost) TableName() string {
	return "model_gateway_profit_resource_costs"
}

func (ModelGatewayProfitRatioRecommendation) TableName() string {
	return "model_gateway_profit_ratio_recommendations"
}

func (ModelGatewayProfitCanaryTask) TableName() string {
	return "model_gateway_profit_canary_tasks"
}

func (r *ModelGatewayProfitResourceCost) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if r.CreatedAt <= 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.Normalize()
	return nil
}

func (r *ModelGatewayProfitResourceCost) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	r.Normalize()
	return nil
}

func (r *ModelGatewayProfitRatioRecommendation) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if r.CreatedAt <= 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.Normalize()
	return nil
}

func (r *ModelGatewayProfitRatioRecommendation) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	r.Normalize()
	return nil
}

func (r *ModelGatewayProfitCanaryTask) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if r.CreatedAt <= 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.Normalize()
	return nil
}

func (r *ModelGatewayProfitCanaryTask) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	r.Normalize()
	return nil
}

func (r *ModelGatewayProfitResourceCost) Normalize() {
	if r == nil {
		return
	}
	r.Name = strings.TrimSpace(r.Name)
	r.ResourceType = NormalizeModelGatewayProfitResourceType(r.ResourceType)
	r.ScopeType = NormalizeModelGatewayProfitResourceScope(r.ScopeType)
	r.ScopeKey = strings.TrimSpace(r.ScopeKey)
	r.AllocationMode = NormalizeModelGatewayProfitResourceAllocationMode(r.AllocationMode)
	r.Remark = strings.TrimSpace(r.Remark)
	if r.PeriodSeconds <= 0 {
		r.PeriodSeconds = 86400
	}
	if r.AmountUSD < 0 {
		r.AmountUSD = 0
	}
	if r.LossAmountUSD < 0 {
		r.LossAmountUSD = 0
	}
	if r.AmortizeEndAt > 0 && r.AmortizeStartAt > 0 && r.AmortizeEndAt < r.AmortizeStartAt {
		r.AmortizeEndAt = r.AmortizeStartAt
	}
}

func (r *ModelGatewayProfitRatioRecommendation) Normalize() {
	if r == nil {
		return
	}
	r.Window = strings.ToLower(strings.TrimSpace(r.Window))
	if r.Window == "" {
		r.Window = "24h"
	}
	r.Dimension = strings.ToLower(strings.TrimSpace(r.Dimension))
	if r.Dimension == "" {
		r.Dimension = ModelGatewayProfitResourceScopeGroup
	}
	r.RiskLevel = strings.ToLower(strings.TrimSpace(r.RiskLevel))
	if r.RiskLevel == "" {
		r.RiskLevel = "medium"
	}
	r.Reason = strings.TrimSpace(r.Reason)
	if r.TargetProfitRate < 0 {
		r.TargetProfitRate = 0
	}
	if r.TargetProfitRate > 0.95 {
		r.TargetProfitRate = 0.95
	}
	if r.Confidence < 0 {
		r.Confidence = 0
	}
	if r.Confidence > 1 {
		r.Confidence = 1
	}
	r.DecisionStatus = NormalizeModelGatewayProfitRecommendationDecisionStatus(r.DecisionStatus)
	r.DecisionRemark = strings.TrimSpace(r.DecisionRemark)
	r.DecisionOperatorName = strings.TrimSpace(r.DecisionOperatorName)
	if r.PlannedRevenueMultiplier < 0 {
		r.PlannedRevenueMultiplier = 0
	}
	if r.PlannedRevenueMultiplier > 100 {
		r.PlannedRevenueMultiplier = 100
	}
	if r.CostMarkupMultiplier < 0 {
		r.CostMarkupMultiplier = 0
	}
	if r.CostMarkupMultiplier > 100 {
		r.CostMarkupMultiplier = 100
	}
	if r.CostMultiplier < 0 {
		r.CostMultiplier = 0
	}
	if r.CostMultiplier > 100 {
		r.CostMultiplier = 100
	}
}

func (r *ModelGatewayProfitCanaryTask) Normalize() {
	if r == nil {
		return
	}
	r.Title = strings.TrimSpace(r.Title)
	if r.Title == "" {
		r.Title = "canary task"
	}
	r.Status = NormalizeModelGatewayProfitCanaryTaskStatus(r.Status)
	r.ScopeType = NormalizeModelGatewayProfitResourceScope(r.ScopeType)
	r.ScopeKey = strings.TrimSpace(r.ScopeKey)
	r.ResultSummary = strings.TrimSpace(r.ResultSummary)
	r.CreatedByName = strings.TrimSpace(r.CreatedByName)
	r.UpdatedByName = strings.TrimSpace(r.UpdatedByName)
	if r.BaselineRevenueMultiplier < 0 {
		r.BaselineRevenueMultiplier = 0
	}
	if r.PlannedRevenueMultiplier < 0 {
		r.PlannedRevenueMultiplier = 0
	}
	if r.RecommendedRevenueMultiplier < 0 {
		r.RecommendedRevenueMultiplier = 0
	}
	if r.BaselineRevenueMultiplier > 100 {
		r.BaselineRevenueMultiplier = 100
	}
	if r.PlannedRevenueMultiplier > 100 {
		r.PlannedRevenueMultiplier = 100
	}
	if r.RecommendedRevenueMultiplier > 100 {
		r.RecommendedRevenueMultiplier = 100
	}
	if r.ObservationWindowSeconds <= 0 {
		r.ObservationWindowSeconds = 7200
	}
	if r.PlannedEndAt > 0 && r.PlannedStartAt > 0 && r.PlannedEndAt < r.PlannedStartAt {
		r.PlannedEndAt = r.PlannedStartAt
	}
	if r.ActualEndAt > 0 && r.ActualStartAt > 0 && r.ActualEndAt < r.ActualStartAt {
		r.ActualEndAt = r.ActualStartAt
	}
}

func NormalizeModelGatewayProfitResourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModelGatewayProfitResourceTypeAccountPool,
		ModelGatewayProfitResourceTypeProxy,
		ModelGatewayProfitResourceTypeServer,
		ModelGatewayProfitResourceTypeBandwidth:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ModelGatewayProfitResourceTypeOther
	}
}

func NormalizeModelGatewayProfitResourceScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModelGatewayProfitResourceScopeChannel,
		ModelGatewayProfitResourceScopeGroup,
		ModelGatewayProfitResourceScopeModel:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ModelGatewayProfitResourceScopeGlobal
	}
}

func NormalizeModelGatewayProfitResourceAllocationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModelGatewayProfitResourceAllocationGlobal,
		ModelGatewayProfitResourceAllocationRequest:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ModelGatewayProfitResourceAllocationRevenue
	}
}

func NormalizeModelGatewayProfitRecommendationDecisionStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModelGatewayProfitRecommendationDecisionCanary,
		ModelGatewayProfitRecommendationDecisionAdopted,
		ModelGatewayProfitRecommendationDecisionIgnored:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ModelGatewayProfitRecommendationDecisionPending
	}
}

func NormalizeModelGatewayProfitCanaryTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModelGatewayProfitCanaryTaskStatusRunning,
		ModelGatewayProfitCanaryTaskStatusCompleted,
		ModelGatewayProfitCanaryTaskStatusCanceled:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ModelGatewayProfitCanaryTaskStatusPlanned
	}
}

func ListModelGatewayProfitResourceCosts(enabledOnly bool) ([]ModelGatewayProfitResourceCost, error) {
	rows := make([]ModelGatewayProfitResourceCost, 0)
	query := DB.Model(&ModelGatewayProfitResourceCost{})
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("updated_at DESC").Find(&rows).Error
	return rows, err
}

func GetModelGatewayProfitResourceCost(id int) (*ModelGatewayProfitResourceCost, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var row ModelGatewayProfitResourceCost
	if err := DB.First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func CreateModelGatewayProfitRatioRecommendation(row *ModelGatewayProfitRatioRecommendation) error {
	if row == nil {
		return gorm.ErrInvalidData
	}
	row.Normalize()
	return DB.Create(row).Error
}

func GetModelGatewayProfitRatioRecommendation(id int) (*ModelGatewayProfitRatioRecommendation, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var row ModelGatewayProfitRatioRecommendation
	if err := DB.First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func ListModelGatewayProfitRatioRecommendations(limit int, filter ModelGatewayProfitRatioRecommendationFilter) ([]ModelGatewayProfitRatioRecommendation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows := make([]ModelGatewayProfitRatioRecommendation, 0, limit)
	query := DB.Model(&ModelGatewayProfitRatioRecommendation{})
	if window := strings.ToLower(strings.TrimSpace(filter.Window)); window != "" {
		query = query.Where("window = ?", window)
	}
	if dimension := strings.ToLower(strings.TrimSpace(filter.Dimension)); dimension != "" {
		query = query.Where("dimension = ?", dimension)
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("start_timestamp >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("end_timestamp <= ?", filter.EndTimestamp)
	}
	err := query.
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func GetModelGatewayProfitCanaryTask(id int) (*ModelGatewayProfitCanaryTask, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var row ModelGatewayProfitCanaryTask
	if err := DB.First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	hydrateModelGatewayProfitCanaryTaskWatchMetrics(&row)
	return &row, nil
}

func ListModelGatewayProfitCanaryTasks(limit int, filter ModelGatewayProfitCanaryTaskFilter) ([]ModelGatewayProfitCanaryTask, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows := make([]ModelGatewayProfitCanaryTask, 0, limit)
	query := DB.Model(&ModelGatewayProfitCanaryTask{})
	if filter.RecommendationID > 0 {
		query = query.Where("recommendation_id = ?", filter.RecommendationID)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", NormalizeModelGatewayProfitCanaryTaskStatus(status))
	}
	err := query.
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return rows, err
	}
	for i := range rows {
		hydrateModelGatewayProfitCanaryTaskWatchMetrics(&rows[i])
	}
	return rows, nil
}

func hydrateModelGatewayProfitCanaryTaskWatchMetrics(row *ModelGatewayProfitCanaryTask) {
	if row == nil || strings.TrimSpace(row.WatchMetricJSON) == "" {
		return
	}
	var metrics []string
	if err := common.UnmarshalJsonStr(row.WatchMetricJSON, &metrics); err == nil {
		row.WatchMetrics = metrics
	}
}
