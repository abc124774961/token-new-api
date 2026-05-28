package model

type ChannelBalanceMonitorEvent struct {
	Id                 int     `json:"id"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint;index"`
	Scope              string  `json:"scope" gorm:"type:varchar(32);index;default:'account'"`
	ChannelID          int     `json:"channel_id" gorm:"index"`
	ChannelName        string  `json:"channel_name" gorm:"type:varchar(191)"`
	ChannelType        int     `json:"channel_type" gorm:"index"`
	Group              string  `json:"group" gorm:"type:varchar(64);index"`
	CredentialIndex    int     `json:"credential_index" gorm:"default:0;index"`
	AccountID          string  `json:"account_id" gorm:"type:varchar(191);index;default:''"`
	AccountIdentityKey string  `json:"account_identity_key" gorm:"type:varchar(191);index;default:''"`
	Balance            float64 `json:"balance"`
	Threshold          float64 `json:"threshold"`
	EventType          string  `json:"event_type" gorm:"type:varchar(64);index"`
	Field              string  `json:"field" gorm:"type:varchar(64);index;default:''"`
	ModelName          string  `json:"model_name" gorm:"type:varchar(191);index;default:''"`
	AutoApplied        bool    `json:"auto_applied" gorm:"default:false"`
	Error              string  `json:"error" gorm:"type:text"`
	Details            string  `json:"details" gorm:"type:text"`
}
