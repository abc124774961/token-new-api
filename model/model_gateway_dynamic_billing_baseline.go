package model

type ModelGatewayDynamicBillingBaseline struct {
	Id                   int     `json:"id" gorm:"primaryKey"`
	BillingGroup         string  `json:"billing_group" gorm:"type:varchar(64);uniqueIndex;not null"`
	ReferenceModel       string  `json:"reference_model" gorm:"type:varchar(191);default:''"`
	Ratio                float64 `json:"ratio" gorm:"type:decimal(18,8);default:0"`
	ReferencePricePerM   float64 `json:"reference_price_per_m" gorm:"type:decimal(18,8);default:0"`
	SampleCount          int     `json:"sample_count" gorm:"default:0"`
	ModelCount           int     `json:"model_count" gorm:"default:0"`
	WindowStart          int64   `json:"window_start" gorm:"bigint;default:0"`
	WindowEnd            int64   `json:"window_end" gorm:"bigint;default:0"`
	ProfitRate           float64 `json:"profit_rate" gorm:"type:decimal(18,8);default:0"`
	CostSource           string  `json:"cost_source" gorm:"type:varchar(32);index;default:'sample_cost'"`
	ApplyMode            string  `json:"apply_mode" gorm:"type:varchar(32);index;default:''"`
	ApplyReason          string  `json:"apply_reason" gorm:"type:varchar(64);index;default:''"`
	OperatingCostUSD     float64 `json:"operating_cost_usd" gorm:"type:decimal(18,8);default:0"`
	RequiredRevenueUSD   float64 `json:"required_revenue_usd" gorm:"type:decimal(18,8);default:0"`
	BaseQuotaAtRatio1    float64 `json:"base_quota_at_ratio_1" gorm:"column:base_quota_at_ratio_1;type:decimal(18,8);default:0"`
	CostMultiplier       float64 `json:"cost_multiplier" gorm:"type:decimal(18,8);default:0"`
	TargetRatio          float64 `json:"target_ratio" gorm:"type:decimal(18,8);default:0"`
	EffectiveRatio       float64 `json:"effective_ratio" gorm:"type:decimal(18,8);default:0"`
	Clamped              bool    `json:"clamped" gorm:"default:false;index"`
	PendingManualConfirm bool    `json:"pending_manual_confirm" gorm:"default:false;index"`
	FallbackReason       string  `json:"fallback_reason" gorm:"type:varchar(64);index;default:''"`
	RequestCount         int64   `json:"request_count" gorm:"default:0"`
	SuccessRequestCount  int64   `json:"success_request_count" gorm:"default:0"`
	TotalTokens          int64   `json:"total_tokens" gorm:"default:0"`
	TrafficCostUSD       float64 `json:"traffic_cost_usd" gorm:"type:decimal(18,8);default:0"`
	TrafficEstimated     bool    `json:"traffic_estimated" gorm:"default:false;index"`
	TrafficDataReady     bool    `json:"traffic_data_ready" gorm:"default:false;index"`
	ServerCostUSD        float64 `json:"server_cost_usd" gorm:"type:decimal(18,8);default:0"`
	ResourceCostUSD      float64 `json:"resource_cost_usd" gorm:"type:decimal(18,8);default:0"`
	UpstreamCostUSD      float64 `json:"upstream_cost_usd" gorm:"type:decimal(18,8);default:0"`
	CalculatedAt         int64   `json:"calculated_at" gorm:"bigint;index"`
	CreatedAt            int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt            int64   `json:"updated_at" gorm:"bigint;index"`
}

func (ModelGatewayDynamicBillingBaseline) TableName() string {
	return "model_gateway_dynamic_billing_baselines"
}
