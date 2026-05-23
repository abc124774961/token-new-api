package model

type ModelGatewayChannelCostProfile struct {
	Id                     int     `json:"id" gorm:"primaryKey"`
	ChannelID              int     `json:"channel_id" gorm:"uniqueIndex:idx_mg_channel_cost_profile_versioned,priority:1;index;not null"`
	UpstreamModel          string  `json:"upstream_model" gorm:"type:varchar(191);uniqueIndex:idx_mg_channel_cost_profile_versioned,priority:2;index;default:'*'"`
	EffectiveTime          int64   `json:"effective_time" gorm:"bigint;uniqueIndex:idx_mg_channel_cost_profile_versioned,priority:3;index;default:0"`
	Version                int     `json:"version" gorm:"uniqueIndex:idx_mg_channel_cost_profile_versioned,priority:4;default:1"`
	PricingMode            string  `json:"pricing_mode" gorm:"type:varchar(32);default:'token'"`
	Currency               string  `json:"currency" gorm:"type:varchar(16);default:'USD'"`
	InputPerMillion        float64 `json:"input_per_million" gorm:"type:decimal(18,8);default:0"`
	OutputPerMillion       float64 `json:"output_per_million" gorm:"type:decimal(18,8);default:0"`
	CacheReadPerMillion    float64 `json:"cache_read_per_million" gorm:"type:decimal(18,8);default:0"`
	CacheWritePerMillion   float64 `json:"cache_write_per_million" gorm:"type:decimal(18,8);default:0"`
	CacheWrite5mPerMillion float64 `json:"cache_write_5m_per_million" gorm:"type:decimal(18,8);default:0"`
	CacheWrite1hPerMillion float64 `json:"cache_write_1h_per_million" gorm:"type:decimal(18,8);default:0"`
	ImageInputPerMillion   float64 `json:"image_input_per_million" gorm:"type:decimal(18,8);default:0"`
	ImageOutputPerMillion  float64 `json:"image_output_per_million" gorm:"type:decimal(18,8);default:0"`
	AudioInputPerMillion   float64 `json:"audio_input_per_million" gorm:"type:decimal(18,8);default:0"`
	AudioOutputPerMillion  float64 `json:"audio_output_per_million" gorm:"type:decimal(18,8);default:0"`
	RequestPrice           float64 `json:"request_price" gorm:"type:decimal(18,8);default:0"`
	ToolPricesJSON         string  `json:"tool_prices_json" gorm:"type:text"`
	Source                 string  `json:"source" gorm:"type:varchar(32);index;default:'manual'"`
	Accuracy               string  `json:"accuracy" gorm:"type:varchar(32);index;default:'precise'"`
	CostCoefficient        float64 `json:"cost_coefficient" gorm:"type:decimal(18,8);default:1"`
	TokenMultiplier        float64 `json:"token_multiplier" gorm:"type:decimal(18,8);default:1"`
	InputCostMultiplier    float64 `json:"input_cost_multiplier" gorm:"type:decimal(18,8);default:1"`
	OutputCostMultiplier   float64 `json:"output_cost_multiplier" gorm:"type:decimal(18,8);default:1"`
	CacheReadMultiplier    float64 `json:"cache_read_multiplier" gorm:"type:decimal(18,8);default:1"`
	CacheWriteMultiplier   float64 `json:"cache_write_multiplier" gorm:"type:decimal(18,8);default:1"`
	RequestCostMultiplier  float64 `json:"request_cost_multiplier" gorm:"type:decimal(18,8);default:1"`
	RechargeMultiplier     float64 `json:"recharge_multiplier" gorm:"type:decimal(18,8);default:1"`
	SourceURL              string  `json:"source_url" gorm:"type:varchar(512);default:''"`
	MetadataJSON           string  `json:"metadata_json" gorm:"type:text"`
	SyncedAt               int64   `json:"synced_at" gorm:"bigint;default:0"`
	CreatedAt              int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt              int64   `json:"updated_at" gorm:"bigint;index"`
}

func (ModelGatewayChannelCostProfile) TableName() string {
	return "model_gateway_channel_cost_profiles"
}

type ModelGatewayRequestCostSummary struct {
	Id                int     `json:"id" gorm:"primaryKey"`
	RequestId         string  `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	ChannelID         int     `json:"channel_id" gorm:"index;default:0"`
	UpstreamModel     string  `json:"upstream_model" gorm:"type:varchar(191);index;default:''"`
	UpstreamCostTotal float64 `json:"upstream_cost_total" gorm:"type:decimal(18,12);default:0"`
	BreakdownJSON     string  `json:"breakdown_json" gorm:"type:text"`
	CostSource        string  `json:"cost_source" gorm:"type:varchar(32);index;default:'pending'"`
	CostAccuracy      string  `json:"cost_accuracy" gorm:"type:varchar(32);index;default:'pending'"`
	CalculatedAt      int64   `json:"calculated_at" gorm:"bigint;index"`
	CreatedAt         int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt         int64   `json:"updated_at" gorm:"bigint;index"`
}

func (ModelGatewayRequestCostSummary) TableName() string {
	return "model_gateway_request_cost_summaries"
}
