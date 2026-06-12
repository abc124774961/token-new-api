package types

import "fmt"

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
	BaseGroupRatio    float64
}

type DynamicBillingSnapshot struct {
	Applied              bool    `json:"applied"`
	FallbackReason       string  `json:"fallback_reason,omitempty"`
	RequestedModel       string  `json:"requested_model,omitempty"`
	Group                string  `json:"group,omitempty"`
	StaticGroupRatio     float64 `json:"static_group_ratio,omitempty"`
	DynamicRatio         float64 `json:"dynamic_ratio,omitempty"`
	PricePerM            float64 `json:"price_per_m,omitempty"`
	ProfitRate           float64 `json:"profit_rate,omitempty"`
	SampleCount          int     `json:"sample_count,omitempty"`
	CalculatedAt         int64   `json:"calculated_at,omitempty"`
	WindowStart          int64   `json:"window_start,omitempty"`
	WindowEnd            int64   `json:"window_end,omitempty"`
	CostSource           string  `json:"cost_source,omitempty"`
	ApplyMode            string  `json:"apply_mode,omitempty"`
	ApplyReason          string  `json:"apply_reason,omitempty"`
	OperatingCostUSD     float64 `json:"operating_cost_usd,omitempty"`
	RequiredRevenueUSD   float64 `json:"required_revenue_usd,omitempty"`
	BaseQuotaAtRatio1    float64 `json:"base_quota_at_ratio_1,omitempty"`
	CostMultiplier       float64 `json:"cost_multiplier,omitempty"`
	TargetRatio          float64 `json:"target_ratio,omitempty"`
	EffectiveRatio       float64 `json:"effective_ratio,omitempty"`
	FixedRatio           float64 `json:"fixed_ratio,omitempty"`
	FixedRatioApplied    bool    `json:"fixed_ratio_applied,omitempty"`
	Clamped              bool    `json:"clamped,omitempty"`
	PendingManualConfirm bool    `json:"pending_manual_confirm,omitempty"`
	RequestCount         int64   `json:"request_count,omitempty"`
	SuccessRequestCount  int64   `json:"success_request_count,omitempty"`
	TotalTokens          int64   `json:"total_tokens,omitempty"`
	TrafficCostUSD       float64 `json:"traffic_cost_usd,omitempty"`
	TrafficEstimated     bool    `json:"traffic_estimated,omitempty"`
	TrafficDataReady     bool    `json:"traffic_data_ready,omitempty"`
	ServerCostUSD        float64 `json:"server_cost_usd,omitempty"`
	ResourceCostUSD      float64 `json:"resource_cost_usd,omitempty"`
	UpstreamCostUSD      float64 `json:"upstream_cost_usd,omitempty"`
}

type FixedPriceMarginGuardInfo struct {
	Applied             bool    `json:"applied"`
	OriginalGroupRatio  float64 `json:"original_group_ratio,omitempty"`
	ProtectedGroupRatio float64 `json:"protected_group_ratio,omitempty"`
	CostUSD             float64 `json:"cost_usd,omitempty"`
	TargetMargin        float64 `json:"target_margin,omitempty"`
	MinRevenueUSD       float64 `json:"min_revenue_usd,omitempty"`
	ProfileID           int     `json:"profile_id,omitempty"`
	ProfileModel        string  `json:"profile_model,omitempty"`
	ProfileSource       string  `json:"profile_source,omitempty"`
	ProfileAccuracy     string  `json:"profile_accuracy,omitempty"`
}

type PriceData struct {
	FreeModel             bool
	ModelPrice            float64
	ModelRatio            float64
	CompletionRatio       float64
	CacheRatio            float64
	CacheCreationRatio    float64
	CacheCreation5mRatio  float64
	CacheCreation1hRatio  float64
	ImageRatio            float64
	AudioRatio            float64
	AudioCompletionRatio  float64
	OtherRatios           map[string]float64
	UsePrice              bool
	Quota                 int // 按次计费的最终额度（MJ / Task）
	QuotaToPreConsume     int // 按量计费的预消耗额度
	QuotaBeforeGroup      float64
	GroupRatioInfo        GroupRatioInfo
	BillingMultiplier     *BillingMultiplierSnapshot
	FixedPriceMarginGuard *FixedPriceMarginGuardInfo
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if p.OtherRatios == nil {
		p.OtherRatios = make(map[string]float64)
	}
	if ratio <= 0 {
		return
	}
	p.OtherRatios[key] = ratio
}

func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
