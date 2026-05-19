package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

const (
	ChannelFailureEventTypeAvoidance = "failure_avoidance"
	ChannelFailureEventTypePaused    = "failure_paused"
)

type ChannelFailureEvent struct {
	Id               int    `json:"id" gorm:"primaryKey"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_channel_failure_events_created_at"`
	ChannelId        int    `json:"channel_id" gorm:"index:idx_channel_failure_events_channel_created,priority:1;index:idx_channel_failure_events_reason"`
	ChannelName      string `json:"channel_name" gorm:"type:varchar(191);default:''"`
	ChannelType      int    `json:"channel_type" gorm:"default:0"`
	Group            string `json:"group" gorm:"type:varchar(64);index"`
	ModelName        string `json:"model_name" gorm:"type:varchar(191);index;default:''"`
	RequestId        string `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	EventType        string `json:"event_type" gorm:"type:varchar(32);index;default:''"`
	Reason           string `json:"reason" gorm:"type:varchar(128);index:idx_channel_failure_events_reason"`
	ErrorType        string `json:"error_type" gorm:"type:varchar(64);default:''"`
	ErrorCode        string `json:"error_code" gorm:"type:varchar(128);default:''"`
	StatusCode       int    `json:"status_code" gorm:"default:0;index"`
	FailureCount     int    `json:"failure_count" gorm:"default:0"`
	AttemptIndex     int    `json:"attempt_index" gorm:"default:0"`
	RemainingSeconds int64  `json:"remaining_seconds" gorm:"default:0"`
	Until            int64  `json:"until" gorm:"bigint;default:0"`
	AutoPaused       bool   `json:"auto_paused" gorm:"default:false;index"`
	FinalFailure     bool   `json:"final_failure" gorm:"default:false;index"`
	UsedChannels     string `json:"used_channels" gorm:"type:varchar(255);default:''"`
	Message          string `json:"message" gorm:"type:text"`
	Metadata         string `json:"metadata" gorm:"type:text"`
}

type RecordChannelFailureEventParams struct {
	ChannelId        int
	ChannelName      string
	ChannelType      int
	Group            string
	ModelName        string
	RequestId        string
	EventType        string
	Reason           string
	ErrorType        string
	ErrorCode        string
	StatusCode       int
	FailureCount     int
	AttemptIndex     int
	RemainingSeconds int64
	Until            int64
	AutoPaused       bool
	FinalFailure     bool
	UsedChannels     string
	Message          string
	Metadata         string
}

func RecordChannelFailureEvent(params RecordChannelFailureEventParams) {
	if params.ChannelId <= 0 {
		return
	}
	if DB == nil {
		return
	}
	if params.EventType == "" {
		params.EventType = ChannelFailureEventTypeAvoidance
	}
	event := &ChannelFailureEvent{
		CreatedAt:        common.GetTimestamp(),
		ChannelId:        params.ChannelId,
		ChannelName:      params.ChannelName,
		ChannelType:      params.ChannelType,
		Group:            params.Group,
		ModelName:        params.ModelName,
		RequestId:        params.RequestId,
		EventType:        params.EventType,
		Reason:           params.Reason,
		ErrorType:        params.ErrorType,
		ErrorCode:        params.ErrorCode,
		StatusCode:       params.StatusCode,
		FailureCount:     params.FailureCount,
		AttemptIndex:     params.AttemptIndex,
		RemainingSeconds: params.RemainingSeconds,
		Until:            params.Until,
		AutoPaused:       params.AutoPaused,
		FinalFailure:     params.FinalFailure,
		UsedChannels:     params.UsedChannels,
		Message:          params.Message,
		Metadata:         params.Metadata,
	}
	if err := DB.Create(event).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to record channel failure event: channel_id=%d, reason=%s, error=%v", params.ChannelId, params.Reason, err))
	}
}
