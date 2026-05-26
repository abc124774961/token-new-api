package model

type ModelGatewayScoreEvent struct {
	Id                 int     `json:"id" gorm:"primaryKey"`
	TraceID            string  `json:"trace_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	RequestID          string  `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	AttemptIndex       int     `json:"attempt_index" gorm:"default:0"`
	ChannelID          int     `json:"channel_id" gorm:"index;default:0"`
	RequestedModel     string  `json:"requested_model" gorm:"type:varchar(191);index;default:''"`
	UpstreamModel      string  `json:"upstream_model" gorm:"type:varchar(191);default:''"`
	Group              string  `json:"group" gorm:"column:group;type:varchar(64);index;default:''"`
	EndpointType       string  `json:"endpoint_type" gorm:"type:varchar(64);index;default:''"`
	IsHealthProbe      bool    `json:"is_health_probe" gorm:"default:false;index"`
	Strategy           string  `json:"strategy" gorm:"type:varchar(32);default:''"`
	AutoMode           string  `json:"auto_mode" gorm:"type:varchar(32);default:''"`
	BeforeTotal        float64 `json:"before_total" gorm:"default:0"`
	AfterTotal         float64 `json:"after_total" gorm:"default:0"`
	Delta              float64 `json:"delta" gorm:"default:0"`
	SampleDecisionJSON string  `json:"sample_decision_json" gorm:"type:text"`
	ChangedItemsJSON   string  `json:"changed_items_json" gorm:"type:text"`
	ContextJSON        string  `json:"context_json" gorm:"type:text"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;index"`
}

func (ModelGatewayScoreEvent) TableName() string {
	return "model_gateway_score_events"
}
