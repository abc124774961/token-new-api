package model

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModelGatewayUserRequestErrorRateLimit         = "rate_limit"
	ModelGatewayUserRequestErrorTimeout           = "timeout"
	ModelGatewayUserRequestErrorUpstream          = "upstream_error"
	ModelGatewayUserRequestErrorStreamInterrupted = "stream_interrupted"
	ModelGatewayUserRequestErrorServer            = "server_error"
	ModelGatewayUserRequestErrorClientAborted     = "client_aborted"
)

type ModelGatewayUserRequestSummary struct {
	Id                 int    `json:"id" gorm:"primaryKey"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	CreatedAt          int64  `json:"created_at" gorm:"bigint;index:idx_mg_user_req_created_at"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint;index"`
	CompletedAt        int64  `json:"completed_at" gorm:"bigint;index:idx_mg_user_req_completed_at;index:idx_mg_user_req_model_completed,priority:2;index:idx_mg_user_req_group_completed,priority:2"`
	RequestedModel     string `json:"requested_model" gorm:"type:varchar(191);index:idx_mg_user_req_model_completed,priority:1;default:''"`
	RequestedGroup     string `json:"requested_group" gorm:"type:varchar(64);index:idx_mg_user_req_group_completed,priority:1;default:''"`
	SelectedGroup      string `json:"selected_group" gorm:"type:varchar(64);index;default:''"`
	FinalChannelID     int    `json:"final_channel_id" gorm:"default:0;index"`
	FinalChannelName   string `json:"final_channel_name" gorm:"type:varchar(191);default:''"`
	Attempts           int    `json:"attempts" gorm:"default:0"`
	LastAttemptIndex   int    `json:"last_attempt_index" gorm:"default:0"`
	FinalSuccess       bool   `json:"final_success" gorm:"default:false;index"`
	Recovered          bool   `json:"recovered" gorm:"default:false;index"`
	FinalStatusCode    int    `json:"final_status_code" gorm:"default:0"`
	FinalErrorCategory string `json:"final_error_category" gorm:"type:varchar(64);index;default:''"`
	EmptyOutput        bool   `json:"empty_output" gorm:"default:false;index"`
	ExperienceIssue    string `json:"experience_issue" gorm:"type:varchar(64);index;default:''"`
	StreamInterrupted  bool   `json:"stream_interrupted" gorm:"default:false;index"`
	ClientAborted      bool   `json:"client_aborted" gorm:"default:false;index"`
	DurationMs         int64  `json:"duration_ms" gorm:"default:0"`
	TTFTMs             int64  `json:"ttft_ms" gorm:"default:0"`
}

func (ModelGatewayUserRequestSummary) TableName() string {
	return "model_gateway_user_request_summaries"
}

type ModelGatewayUserRequestAttempt struct {
	CreatedAt         int64
	RequestId         string
	AttemptIndex      int
	RequestedGroup    string
	SelectedGroup     string
	ChannelID         int
	ChannelName       string
	RequestedModel    string
	Success           bool
	StatusCode        int
	ErrorCode         string
	ErrorType         string
	ErrorCategory     string
	DurationMs        int64
	TTFTMs            int64
	StreamInterrupted bool
	WillRetry         bool
	ClientAborted     bool
	EmptyOutput       bool
	ExperienceIssue   string
}

func RecordModelGatewayUserRequestAttempt(attempt ModelGatewayUserRequestAttempt) *ModelGatewayUserRequestSummary {
	if DB == nil {
		return nil
	}
	if strings.TrimSpace(attempt.RequestId) == "" {
		return nil
	}
	if attempt.CreatedAt == 0 {
		attempt.CreatedAt = common.GetTimestamp()
	}
	if !modelGatewayUserRequestAttemptFinalized(attempt) {
		return nil
	}
	summary, err := upsertModelGatewayUserRequestAttempt(attempt)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to upsert model gateway user request summary: request_id=%s error=%v", attempt.RequestId, err))
		return nil
	}
	return summary
}

func upsertModelGatewayUserRequestAttempt(attempt ModelGatewayUserRequestAttempt) (*ModelGatewayUserRequestSummary, error) {
	summary := modelGatewayUserRequestSummaryFromAttempt(attempt)
	create := *summary
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&create)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		return &create, nil
	}

	if attempt.AttemptIndex <= 0 {
		var updated ModelGatewayUserRequestSummary
		err := DB.Transaction(func(tx *gorm.DB) error {
			var existing ModelGatewayUserRequestSummary
			if err := tx.Where("request_id = ?", attempt.RequestId).First(&existing).Error; err != nil {
				return err
			}
			updates := modelGatewayUserRequestSummaryUpdates(existing, attempt)
			if err := tx.Model(&ModelGatewayUserRequestSummary{}).
				Where("request_id = ?", attempt.RequestId).
				Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("request_id = ?", attempt.RequestId).First(&updated).Error
		})
		if err != nil {
			return nil, err
		}
		return &updated, nil
	}

	var updated ModelGatewayUserRequestSummary
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing ModelGatewayUserRequestSummary
		if err := tx.Where("request_id = ?", attempt.RequestId).First(&existing).Error; err != nil {
			return err
		}
		if attempt.AttemptIndex <= existing.LastAttemptIndex {
			updated = existing
			return nil
		}
		updates := modelGatewayUserRequestSummaryUpdates(existing, attempt)
		if err := tx.Model(&ModelGatewayUserRequestSummary{}).
			Where("request_id = ?", attempt.RequestId).
			Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("request_id = ?", attempt.RequestId).First(&updated).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func modelGatewayUserRequestSummaryFromAttempt(attempt ModelGatewayUserRequestAttempt) *ModelGatewayUserRequestSummary {
	finalized := modelGatewayUserRequestAttemptFinalized(attempt)
	clientAborted := finalized && modelGatewayUserRequestAttemptClientAborted(attempt)
	success := finalized && attempt.Success && !attempt.StreamInterrupted && !clientAborted
	errorCategory := NormalizeModelGatewayUserRequestErrorCategory(
		attempt.ErrorCategory,
		attempt.ErrorCode,
		attempt.ErrorType,
		attempt.StatusCode,
		attempt.StreamInterrupted,
		success,
	)
	if clientAborted {
		errorCategory = ModelGatewayUserRequestErrorClientAborted
	}
	if !finalized {
		errorCategory = ""
	}
	return &ModelGatewayUserRequestSummary{
		RequestId:          strings.TrimSpace(attempt.RequestId),
		CreatedAt:          attempt.CreatedAt,
		UpdatedAt:          attempt.CreatedAt,
		CompletedAt:        modelGatewayUserRequestFinalCompletedAt(attempt, finalized),
		RequestedModel:     strings.TrimSpace(attempt.RequestedModel),
		RequestedGroup:     strings.TrimSpace(attempt.RequestedGroup),
		SelectedGroup:      strings.TrimSpace(attempt.SelectedGroup),
		FinalChannelID:     attempt.ChannelID,
		FinalChannelName:   strings.TrimSpace(attempt.ChannelName),
		Attempts:           attempt.AttemptIndex + 1,
		LastAttemptIndex:   attempt.AttemptIndex,
		FinalSuccess:       success,
		Recovered:          success && attempt.AttemptIndex > 0,
		FinalStatusCode:    modelGatewayUserRequestFinalStatusCode(attempt, finalized, success),
		FinalErrorCategory: errorCategory,
		EmptyOutput:        finalized && attempt.EmptyOutput,
		ExperienceIssue:    modelGatewayUserRequestExperienceIssue(attempt, finalized),
		StreamInterrupted:  finalized && attempt.StreamInterrupted,
		ClientAborted:      clientAborted,
		DurationMs:         modelGatewayUserRequestFinalDuration(attempt, finalized),
		TTFTMs:             modelGatewayUserRequestFinalTTFT(attempt, finalized),
	}
}

func modelGatewayUserRequestSummaryUpdates(existing ModelGatewayUserRequestSummary, attempt ModelGatewayUserRequestAttempt) map[string]any {
	finalized := modelGatewayUserRequestAttemptFinalized(attempt)
	clientAborted := finalized && modelGatewayUserRequestAttemptClientAborted(attempt)
	success := finalized && attempt.Success && !attempt.StreamInterrupted && !clientAborted
	hadFinalFailure := existing.FinalStatusCode > 0 ||
		strings.TrimSpace(existing.FinalErrorCategory) != "" ||
		existing.StreamInterrupted
	errorCategory := NormalizeModelGatewayUserRequestErrorCategory(
		attempt.ErrorCategory,
		attempt.ErrorCode,
		attempt.ErrorType,
		attempt.StatusCode,
		attempt.StreamInterrupted,
		success,
	)
	if clientAborted {
		errorCategory = ModelGatewayUserRequestErrorClientAborted
	}
	recovered := existing.Recovered || (success && (existing.Attempts > 0 && !existing.FinalSuccess || hadFinalFailure))
	finalSuccess := existing.FinalSuccess || success
	if !finalSuccess || !finalized {
		recovered = false
	}

	updates := map[string]any{
		"updated_at":         attempt.CreatedAt,
		"attempts":           existing.Attempts + 1,
		"last_attempt_index": attempt.AttemptIndex,
		"final_success":      finalSuccess,
		"recovered":          recovered,
	}
	shouldUpdateFinalMetrics := finalized && (success || !existing.FinalSuccess)
	if shouldUpdateFinalMetrics {
		updates["completed_at"] = attempt.CreatedAt
		updates["duration_ms"] = attempt.DurationMs
		updates["ttft_ms"] = attempt.TTFTMs
		updates["empty_output"] = attempt.EmptyOutput
		updates["experience_issue"] = modelGatewayUserRequestExperienceIssue(attempt, finalized)
	}
	if strings.TrimSpace(existing.RequestedModel) == "" && strings.TrimSpace(attempt.RequestedModel) != "" {
		updates["requested_model"] = strings.TrimSpace(attempt.RequestedModel)
	}
	if strings.TrimSpace(existing.RequestedGroup) == "" && strings.TrimSpace(attempt.RequestedGroup) != "" {
		updates["requested_group"] = strings.TrimSpace(attempt.RequestedGroup)
	}
	if strings.TrimSpace(attempt.SelectedGroup) != "" {
		updates["selected_group"] = strings.TrimSpace(attempt.SelectedGroup)
	}
	if attempt.ChannelID > 0 && (shouldUpdateFinalMetrics || existing.FinalChannelID == 0) {
		updates["final_channel_id"] = attempt.ChannelID
	}
	if strings.TrimSpace(attempt.ChannelName) != "" && (shouldUpdateFinalMetrics || strings.TrimSpace(existing.FinalChannelName) == "") {
		updates["final_channel_name"] = strings.TrimSpace(attempt.ChannelName)
	}
	if !finalized {
		return updates
	}
	if success {
		updates["final_status_code"] = 0
		updates["final_error_category"] = ""
		updates["empty_output"] = attempt.EmptyOutput
		updates["experience_issue"] = modelGatewayUserRequestExperienceIssue(attempt, finalized)
		updates["stream_interrupted"] = false
		updates["client_aborted"] = false
		return updates
	}
	if !finalSuccess {
		updates["final_status_code"] = attempt.StatusCode
		updates["final_error_category"] = errorCategory
		updates["stream_interrupted"] = attempt.StreamInterrupted
		updates["client_aborted"] = clientAborted
	}
	return updates
}

func modelGatewayUserRequestExperienceIssue(attempt ModelGatewayUserRequestAttempt, finalized bool) string {
	if !finalized {
		return ""
	}
	if issue := strings.ToLower(strings.TrimSpace(attempt.ExperienceIssue)); issue != "" {
		return issue
	}
	if attempt.EmptyOutput {
		return "empty_output"
	}
	return ""
}

func modelGatewayUserRequestAttemptFinalized(attempt ModelGatewayUserRequestAttempt) bool {
	return !attempt.WillRetry || attempt.Success || attempt.StreamInterrupted
}

func modelGatewayUserRequestFinalStatusCode(attempt ModelGatewayUserRequestAttempt, finalized bool, success bool) int {
	if !finalized || success {
		return 0
	}
	return attempt.StatusCode
}

func modelGatewayUserRequestAttemptClientAborted(attempt ModelGatewayUserRequestAttempt) bool {
	category := strings.ToLower(strings.TrimSpace(attempt.ErrorCategory))
	return attempt.ClientAborted ||
		attempt.StatusCode == 499 ||
		strings.Contains(category, "client_aborted") ||
		strings.Contains(category, "client_abort") ||
		strings.Contains(category, "client_gone")
}

func modelGatewayUserRequestFinalCompletedAt(attempt ModelGatewayUserRequestAttempt, finalized bool) int64 {
	if !finalized {
		return 0
	}
	return attempt.CreatedAt
}

func modelGatewayUserRequestFinalDuration(attempt ModelGatewayUserRequestAttempt, finalized bool) int64 {
	if !finalized {
		return 0
	}
	return attempt.DurationMs
}

func modelGatewayUserRequestFinalTTFT(attempt ModelGatewayUserRequestAttempt, finalized bool) int64 {
	if !finalized {
		return 0
	}
	return attempt.TTFTMs
}

func NormalizeModelGatewayUserRequestErrorCategory(category string, errorCode string, errorType string, statusCode int, streamInterrupted bool, success bool) string {
	if success {
		return ""
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(category)), "client_abort") ||
		strings.Contains(strings.ToLower(strings.TrimSpace(category)), "client_gone") ||
		statusCode == 499 {
		return ModelGatewayUserRequestErrorClientAborted
	}
	if streamInterrupted {
		return ModelGatewayUserRequestErrorStreamInterrupted
	}
	normalizedCategory := strings.ToLower(strings.TrimSpace(category))
	normalizedCode := strings.ToLower(strings.TrimSpace(errorCode))
	normalizedType := strings.ToLower(strings.TrimSpace(errorType))
	switch {
	case strings.Contains(normalizedCategory, "stream"):
		return ModelGatewayUserRequestErrorStreamInterrupted
	case strings.Contains(normalizedCategory, "timeout") || strings.Contains(normalizedCode, "timeout") || statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		return ModelGatewayUserRequestErrorTimeout
	case strings.Contains(normalizedCategory, "rate_limit") ||
		strings.Contains(normalizedCategory, "concurrency_limit") ||
		strings.Contains(normalizedCategory, "quota") ||
		statusCode == http.StatusTooManyRequests:
		return ModelGatewayUserRequestErrorRateLimit
	case strings.Contains(normalizedCategory, "server_error"):
		return ModelGatewayUserRequestErrorServer
	case strings.Contains(normalizedCategory, "upstream") || strings.Contains(normalizedType, "upstream"):
		return ModelGatewayUserRequestErrorUpstream
	case statusCode >= http.StatusInternalServerError:
		return ModelGatewayUserRequestErrorServer
	default:
		return ModelGatewayUserRequestErrorUpstream
	}
}
