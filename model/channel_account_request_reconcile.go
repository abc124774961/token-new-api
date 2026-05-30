package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

func GetChannelAccountUsageEventByRequestId(requestId string) (*ChannelAccountUsageEvent, error) {
	if DB == nil {
		return nil, nil
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil, nil
	}
	var row ChannelAccountUsageEvent
	err := DB.Model(&ChannelAccountUsageEvent{}).Where("request_id = ?", requestId).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func GetModelGatewayUserRequestSummaryByRequestId(requestId string) (*ModelGatewayUserRequestSummary, error) {
	if DB == nil {
		return nil, nil
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil, nil
	}
	var row ModelGatewayUserRequestSummary
	err := DB.Model(&ModelGatewayUserRequestSummary{}).Where("request_id = ?", requestId).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func QueryModelExecutionRecordsByRequestId(requestId string, limit int) ([]ModelExecutionRecord, error) {
	if DB == nil {
		return nil, nil
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows := make([]ModelExecutionRecord, 0, limit)
	err := DB.Model(&ModelExecutionRecord{}).
		Omit("RequestMeta", "ScoreBreakdown", "CandidateGroups").
		Where("request_id = ?", requestId).
		Order("attempt_index ASC").
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func QueryModelGatewayScoreEventsByRequestId(requestId string, limit int) ([]ModelGatewayScoreEvent, error) {
	if DB == nil {
		return nil, nil
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows := make([]ModelGatewayScoreEvent, 0, limit)
	err := DB.Model(&ModelGatewayScoreEvent{}).
		Omit("SampleDecisionJSON", "ChangedItemsJSON", "ContextJSON").
		Where("request_id = ?", requestId).
		Order("attempt_index ASC").
		Order("created_at ASC").
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func GetModelGatewayRequestCostSummaryByRequestId(requestId string) (*ModelGatewayRequestCostSummary, error) {
	if DB == nil {
		return nil, nil
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil, nil
	}
	var row ModelGatewayRequestCostSummary
	err := DB.Model(&ModelGatewayRequestCostSummary{}).
		Omit("BreakdownJSON").
		Where("request_id = ?", requestId).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}
