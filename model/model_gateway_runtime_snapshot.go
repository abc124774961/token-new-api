package model

type ModelGatewayRuntimeSnapshot struct {
	Id                    int     `json:"id" gorm:"primaryKey"`
	RuntimeKeyHash        string  `json:"runtime_key_hash" gorm:"type:varchar(64);uniqueIndex;not null"`
	RuntimeKey            string  `json:"runtime_key" gorm:"type:text"`
	UpdatedAt             int64   `json:"updated_at" gorm:"bigint;index"`
	RequestedModel        string  `json:"requested_model" gorm:"type:varchar(191);index;default:''"`
	UpstreamModel         string  `json:"upstream_model" gorm:"type:varchar(191);default:''"`
	ChannelID             int     `json:"channel_id" gorm:"index;default:0"`
	Group                 string  `json:"group" gorm:"column:group;type:varchar(64);index;default:''"`
	EndpointType          string  `json:"endpoint_type" gorm:"type:varchar(64);index;default:''"`
	CapabilityFingerprint string  `json:"capability_fingerprint" gorm:"type:text"`
	LatencySamples        string  `json:"latency_samples" gorm:"type:text"`
	SampleCount           int     `json:"sample_count" gorm:"default:0"`
	SuccessRate           float64 `json:"success_rate" gorm:"default:0"`
	TTFTMs                float64 `json:"ttft_ms" gorm:"default:0"`
	DurationMs            float64 `json:"duration_ms" gorm:"default:0"`
	TokensPerSecond       float64 `json:"tokens_per_second" gorm:"default:0"`
	SuccessScore          float64 `json:"success_score" gorm:"default:0"`
	SpeedScore            float64 `json:"speed_score" gorm:"default:0"`
	ExperienceScore       float64 `json:"experience_score" gorm:"default:0"`
	EmptyOutputRate       float64 `json:"empty_output_rate" gorm:"default:0"`
	ExperienceIssueRate   float64 `json:"experience_issue_rate" gorm:"default:0"`
	LastRealAttemptAt     int64   `json:"last_real_attempt_at" gorm:"bigint;index;default:0"`
	LastRealSuccessAt     int64   `json:"last_real_success_at" gorm:"bigint;default:0"`
	LastRealFailureAt     int64   `json:"last_real_failure_at" gorm:"bigint;default:0"`
	RealSampleCount30m    int     `json:"real_sample_count_30m" gorm:"column:real_sample_count_30m;default:0"`
	LastProbeAt           int64   `json:"last_probe_at" gorm:"bigint;index;default:0"`
	LastProbeSuccessAt    int64   `json:"last_probe_success_at" gorm:"bigint;default:0"`
	ConfigErrorIsolated   bool    `json:"config_error_isolated" gorm:"default:false;index"`
	IsolationReason       string  `json:"isolation_reason" gorm:"type:varchar(64);default:''"`
	IsolationUntil        int64   `json:"isolation_until" gorm:"bigint;index;default:0"`
	AuthConfigErrorCount  int     `json:"auth_config_error_count" gorm:"default:0"`
	LastAuthConfigErrorAt int64   `json:"last_auth_config_error_at" gorm:"bigint;default:0"`
}

func (ModelGatewayRuntimeSnapshot) TableName() string {
	return "model_gateway_runtime_snapshots"
}
