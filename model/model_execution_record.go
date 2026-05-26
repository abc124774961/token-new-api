package model

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type ModelExecutionRecord struct {
	Id                int     `json:"id" gorm:"primaryKey"`
	CreatedAt         int64   `json:"created_at" gorm:"bigint;index:idx_model_execution_records_created_at"`
	RequestId         string  `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	AttemptIndex      int     `json:"attempt_index" gorm:"default:0"`
	UserId            int     `json:"user_id" gorm:"index;default:0"`
	TokenId           int     `json:"token_id" gorm:"index;default:0"`
	RequestedGroup    string  `json:"requested_group" gorm:"type:varchar(64);index;default:''"`
	SelectedGroup     string  `json:"selected_group" gorm:"type:varchar(64);index;default:''"`
	ActualGroup       string  `json:"actual_group" gorm:"type:varchar(64);index;default:''"`
	RequestedModel    string  `json:"requested_model" gorm:"type:varchar(191);index;default:''"`
	ChannelId         int     `json:"channel_id" gorm:"index;default:0"`
	ChannelName       string  `json:"channel_name" gorm:"type:varchar(191);default:''"`
	ActualChannelId   int     `json:"actual_channel_id" gorm:"index;default:0"`
	ActualChannelName string  `json:"actual_channel_name" gorm:"type:varchar(191);default:''"`
	EndpointType      string  `json:"endpoint_type" gorm:"type:varchar(64);index;default:''"`
	PolicyMode        string  `json:"policy_mode" gorm:"type:varchar(32);index;default:''"`
	AutoMode          string  `json:"auto_mode" gorm:"type:varchar(32);default:''"`
	Strategy          string  `json:"strategy" gorm:"type:varchar(32);default:''"`
	Shadow            bool    `json:"shadow" gorm:"default:false;index"`
	SmartHandled      bool    `json:"smart_handled" gorm:"default:false;index"`
	FallbackUsed      bool    `json:"fallback_used" gorm:"default:false;index"`
	Success           bool    `json:"success" gorm:"default:false;index"`
	StatusCode        int     `json:"status_code" gorm:"default:0;index"`
	ErrorCode         string  `json:"error_code" gorm:"type:varchar(128);default:''"`
	ErrorType         string  `json:"error_type" gorm:"type:varchar(64);default:''"`
	ErrorCategory     string  `json:"error_category" gorm:"type:varchar(64);index;default:''"`
	DurationMs        int64   `json:"duration_ms" gorm:"default:0"`
	TTFTMs            int64   `json:"ttft_ms" gorm:"default:0"`
	StreamInterrupted bool    `json:"stream_interrupted" gorm:"default:false;index"`
	ScoreTotal        float64 `json:"score_total" gorm:"default:0"`
	ScoreBreakdown    string  `json:"score_breakdown" gorm:"type:text"`
	CandidateGroups   string  `json:"candidate_groups" gorm:"type:text"`
	SelectedReason    string  `json:"selected_reason" gorm:"type:varchar(191);default:''"`
	RequestMeta       string  `json:"request_meta" gorm:"-:migration"`
}

type ModelExecutionObserver func(record ModelExecutionRecord)

var (
	modelExecutionObserverMu sync.RWMutex
	modelExecutionObservers  []ModelExecutionObserver
)

func AddModelExecutionObserver(observer ModelExecutionObserver) {
	if observer == nil {
		return
	}
	modelExecutionObserverMu.Lock()
	defer modelExecutionObserverMu.Unlock()
	modelExecutionObservers = append(modelExecutionObservers, observer)
}

func RecordModelExecution(record *ModelExecutionRecord) {
	if record == nil || DB == nil {
		return
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = common.GetTimestamp()
	}
	if err := DB.Create(record).Error; err != nil {
		if isMissingModelExecutionRequestMetaColumnError(err) {
			if migrateErr := EnsureModelExecutionRecordRequestMetaCapacity(DB); migrateErr != nil {
				err = fmt.Errorf("%w; failed to ensure request_meta capacity: %v", err, migrateErr)
			} else if retryErr := DB.Create(record).Error; retryErr == nil {
				notifyModelExecutionObservers(*record)
				return
			} else {
				err = retryErr
			}
		}
		common.SysLog(fmt.Sprintf(
			"failed to record model execution: request_id=%s channel_id=%d request_meta_bytes=%d error=%v",
			record.RequestId,
			record.ChannelId,
			len(record.RequestMeta),
			err,
		))
		return
	}
	notifyModelExecutionObservers(*record)
}

func isMissingModelExecutionRequestMetaColumnError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "request_meta") &&
		(strings.Contains(message, "no column") ||
			strings.Contains(message, "unknown column") ||
			strings.Contains(message, "does not exist"))
}

func notifyModelExecutionObservers(record ModelExecutionRecord) {
	modelExecutionObserverMu.RLock()
	observers := append([]ModelExecutionObserver(nil), modelExecutionObservers...)
	modelExecutionObserverMu.RUnlock()
	for _, observer := range observers {
		observer(record)
	}
}
