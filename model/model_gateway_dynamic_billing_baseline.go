package model

type ModelGatewayDynamicBillingBaseline struct {
	Id                 int     `json:"id" gorm:"primaryKey"`
	BillingGroup       string  `json:"billing_group" gorm:"type:varchar(64);uniqueIndex;not null"`
	ReferenceModel     string  `json:"reference_model" gorm:"type:varchar(191);default:''"`
	Ratio              float64 `json:"ratio" gorm:"type:decimal(18,8);default:0"`
	ReferencePricePerM float64 `json:"reference_price_per_m" gorm:"type:decimal(18,8);default:0"`
	SampleCount        int     `json:"sample_count" gorm:"default:0"`
	ModelCount         int     `json:"model_count" gorm:"default:0"`
	WindowStart        int64   `json:"window_start" gorm:"bigint;default:0"`
	WindowEnd          int64   `json:"window_end" gorm:"bigint;default:0"`
	ProfitRate         float64 `json:"profit_rate" gorm:"type:decimal(18,8);default:0"`
	CalculatedAt       int64   `json:"calculated_at" gorm:"bigint;index"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt          int64   `json:"updated_at" gorm:"bigint;index"`
}

func (ModelGatewayDynamicBillingBaseline) TableName() string {
	return "model_gateway_dynamic_billing_baselines"
}
