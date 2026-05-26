package model

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

const channelStatusMonitorQueryTimeout = 5 * time.Second

type ChannelStatusMonitorLogRow struct {
	Id               int    `gorm:"column:id"`
	CreatedAt        int64  `gorm:"column:created_at"`
	Type             int    `gorm:"column:type"`
	Group            string `gorm:"column:group"`
	ChannelId        int    `gorm:"column:channel_id"`
	RequestId        string `gorm:"column:request_id"`
	UseTime          int    `gorm:"column:use_time"`
	PromptTokens     int    `gorm:"column:prompt_tokens"`
	CompletionTokens int    `gorm:"column:completion_tokens"`
	Other            string `gorm:"column:other"`
	Content          string `gorm:"column:content"`
}

type ChannelStatusMonitorRecentLogRow struct {
	Id        int    `gorm:"column:id"`
	CreatedAt int64  `gorm:"column:created_at"`
	Type      int    `gorm:"column:type"`
	ChannelId int    `gorm:"column:channel_id"`
	Group     string `gorm:"column:group"`
	RequestId string `gorm:"column:request_id"`
	Other     string `gorm:"column:other"`
	Content   string `gorm:"column:content"`
}

func GetChannelStatusMonitorLogs(startTs int64, channelIds []int, limit int) ([]ChannelStatusMonitorLogRow, error) {
	if startTs <= 0 {
		startTs = time.Now().Add(-24 * time.Hour).Unix()
	}
	if len(channelIds) == 0 {
		return []ChannelStatusMonitorLogRow{}, nil
	}
	rows := make([]ChannelStatusMonitorLogRow, 0)
	groupSelectCol := logGroupCol
	if strings.Contains(groupSelectCol, "`") {
		groupSelectCol = groupSelectCol + " as `group`"
	} else {
		groupSelectCol = groupSelectCol + " as \"group\""
	}
	ctx, cancel := context.WithTimeout(context.Background(), channelStatusMonitorQueryTimeout)
	defer cancel()
	tx := LOG_DB.Model(&Log{}).
		WithContext(ctx).
		Select("id, created_at, type, "+groupSelectCol+", channel_id, request_id, use_time, prompt_tokens, completion_tokens, other").
		Where("created_at >= ? AND channel_id IN ? AND type IN ?", startTs, channelIds, []int{LogTypeConsume, LogTypeError}).
		Order("id desc")
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	err := tx.Find(&rows).Error
	return rows, err
}

func GetChannelStatusMonitorRecentLogs(channelIds []int, limit int) ([]ChannelStatusMonitorRecentLogRow, error) {
	if limit <= 0 {
		limit = 60
	}
	if len(channelIds) == 0 {
		return []ChannelStatusMonitorRecentLogRow{}, nil
	}
	rows := make([]ChannelStatusMonitorRecentLogRow, 0, limit)
	groupSelectCol := logGroupCol
	if strings.Contains(groupSelectCol, "`") {
		groupSelectCol = groupSelectCol + " as `group`"
	} else {
		groupSelectCol = groupSelectCol + " as \"group\""
	}
	ctx, cancel := context.WithTimeout(context.Background(), channelStatusMonitorQueryTimeout)
	defer cancel()
	err := LOG_DB.Model(&Log{}).
		WithContext(ctx).
		Select("id, created_at, type, channel_id, "+groupSelectCol+", request_id, other, content").
		Where("channel_id IN ? AND type IN ?", channelIds, []int{LogTypeConsume, LogTypeError}).
		Order("id desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func GetChannelStatusMonitorRecentLogsByGroups(groupNames []string, perGroupLimit int) ([]ChannelStatusMonitorRecentLogRow, error) {
	if perGroupLimit <= 0 {
		perGroupLimit = 60
	}
	groupNames = uniqueNormalizedMonitorGroups(groupNames)
	if len(groupNames) == 0 {
		return []ChannelStatusMonitorRecentLogRow{}, nil
	}
	rows := make([]ChannelStatusMonitorRecentLogRow, 0, len(groupNames)*perGroupLimit)
	groupSelectCol := logGroupCol
	if strings.Contains(groupSelectCol, "`") {
		groupSelectCol = groupSelectCol + " as `group`"
	} else {
		groupSelectCol = groupSelectCol + " as \"group\""
	}
	ctx, cancel := context.WithTimeout(context.Background(), channelStatusMonitorQueryTimeout)
	defer cancel()
	for _, groupName := range groupNames {
		requestIds := make([]string, 0, perGroupLimit)
		requestQuery := LOG_DB.Model(&Log{}).
			WithContext(ctx).
			Where("type IN ? AND request_id <> ?", []int{LogTypeConsume, LogTypeError}, "").
			Group("request_id").
			Order("MAX(id) desc").
			Limit(perGroupLimit)
		requestQuery = applyChannelStatusMonitorGroupFilter(requestQuery, groupName)
		if err := requestQuery.Pluck("request_id", &requestIds).Error; err != nil {
			return nil, err
		}

		groupRows := make([]ChannelStatusMonitorRecentLogRow, 0, perGroupLimit)
		tx := LOG_DB.Model(&Log{}).
			WithContext(ctx).
			Select("id, created_at, type, channel_id, "+groupSelectCol+", request_id, other, content").
			Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
			Order("id desc")
		tx = applyChannelStatusMonitorGroupFilter(tx, groupName)
		if len(requestIds) > 0 {
			tx = tx.Where("request_id IN ?", requestIds)
		} else {
			tx = tx.Limit(perGroupLimit)
		}
		if err := tx.Find(&groupRows).Error; err != nil {
			return nil, err
		}
		rows = append(rows, groupRows...)
	}
	return rows, nil
}

func GetChannelStatusMonitorUserRequests(startTs int64, groupNames []string, limit int) ([]ModelGatewayUserRequestSummary, error) {
	if startTs <= 0 {
		startTs = time.Now().Add(-24 * time.Hour).Unix()
	}
	groupNames = uniqueNormalizedMonitorGroups(groupNames)
	if len(groupNames) == 0 || DB == nil {
		return []ModelGatewayUserRequestSummary{}, nil
	}
	rows := make([]ModelGatewayUserRequestSummary, 0)
	ctx, cancel := context.WithTimeout(context.Background(), channelStatusMonitorQueryTimeout)
	defer cancel()
	tx := DB.Model(&ModelGatewayUserRequestSummary{}).
		WithContext(ctx).
		Select("id, request_id, completed_at, requested_group, selected_group, final_channel_id, final_channel_name, final_success, recovered, final_status_code, final_error_category, empty_output, experience_issue, stream_interrupted, client_aborted, is_health_probe, probe_reason, duration_ms, ttft_ms").
		Where("completed_at >= ? AND completed_at > 0", startTs).
		Order("completed_at desc, id desc")
	tx = applyChannelStatusMonitorUserRequestGroupsFilter(tx, groupNames)
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	err := tx.Find(&rows).Error
	return rows, err
}

func GetChannelStatusMonitorRecentUserRequestsByGroups(groupNames []string, perGroupLimit int) ([]ModelGatewayUserRequestSummary, error) {
	if perGroupLimit <= 0 {
		perGroupLimit = 60
	}
	groupNames = uniqueNormalizedMonitorGroups(groupNames)
	if len(groupNames) == 0 || DB == nil {
		return []ModelGatewayUserRequestSummary{}, nil
	}
	rows := make([]ModelGatewayUserRequestSummary, 0, len(groupNames)*perGroupLimit)
	ctx, cancel := context.WithTimeout(context.Background(), channelStatusMonitorQueryTimeout)
	defer cancel()
	for _, groupName := range groupNames {
		groupRows := make([]ModelGatewayUserRequestSummary, 0, perGroupLimit)
		tx := DB.Model(&ModelGatewayUserRequestSummary{}).
			WithContext(ctx).
			Select("id, request_id, completed_at, requested_group, selected_group, final_success, final_status_code, final_error_category, empty_output, experience_issue, stream_interrupted, client_aborted, is_health_probe, probe_reason").
			Where("completed_at > 0").
			Where("is_health_probe = ?", false).
			Order("completed_at desc, id desc").
			Limit(perGroupLimit)
		tx = applyChannelStatusMonitorUserRequestGroupFilter(tx, groupName)
		if err := tx.Find(&groupRows).Error; err != nil {
			return nil, err
		}
		rows = append(rows, groupRows...)
	}
	return rows, nil
}

func applyChannelStatusMonitorGroupFilter(tx *gorm.DB, groupName string) *gorm.DB {
	if groupName == "default" {
		return tx.Where("("+logGroupCol+" = ? OR "+logGroupCol+" = ?)", groupName, "")
	}
	return tx.Where(logGroupCol+" = ?", groupName)
}

func applyChannelStatusMonitorUserRequestGroupFilter(tx *gorm.DB, groupName string) *gorm.DB {
	if groupName == "default" {
		return tx.Where("((selected_group <> ? AND selected_group = ?) OR (selected_group = ? AND (requested_group = ? OR requested_group = ?)))", "", groupName, "", groupName, "")
	}
	return tx.Where("((selected_group <> ? AND selected_group = ?) OR (selected_group = ? AND requested_group = ?))", "", groupName, "", groupName)
}

func applyChannelStatusMonitorUserRequestGroupsFilter(tx *gorm.DB, groupNames []string) *gorm.DB {
	groupNames = uniqueNormalizedMonitorGroups(groupNames)
	if len(groupNames) == 0 {
		return tx
	}
	hasDefault := false
	for _, groupName := range groupNames {
		if groupName == "default" {
			hasDefault = true
			break
		}
	}
	if hasDefault {
		return tx.Where("(selected_group IN ? OR (selected_group = ? AND requested_group IN ?) OR (selected_group = ? AND requested_group = ?))", groupNames, "", groupNames, "", "")
	}
	return tx.Where("(selected_group IN ? OR (selected_group = ? AND requested_group IN ?))", groupNames, "", groupNames)
}

func uniqueNormalizedMonitorGroups(groupNames []string) []string {
	seen := make(map[string]struct{}, len(groupNames))
	result := make([]string, 0, len(groupNames))
	for _, groupName := range groupNames {
		groupName = strings.TrimSpace(groupName)
		if groupName == "" {
			groupName = "default"
		}
		if _, ok := seen[groupName]; ok {
			continue
		}
		seen[groupName] = struct{}{}
		result = append(result, groupName)
	}
	return result
}

func GetPublicHomeStatusLogs(startTs int64, channelIds []int) ([]ChannelStatusMonitorLogRow, error) {
	if startTs <= 0 {
		startTs = time.Now().Add(-30 * 24 * time.Hour).Unix()
	}
	if len(channelIds) == 0 {
		return []ChannelStatusMonitorLogRow{}, nil
	}
	rows := make([]ChannelStatusMonitorLogRow, 0)
	ctx, cancel := context.WithTimeout(context.Background(), channelStatusMonitorQueryTimeout)
	defer cancel()
	groupSelectCol := logGroupCol
	if strings.Contains(groupSelectCol, "`") {
		groupSelectCol = groupSelectCol + " as `group`"
	} else {
		groupSelectCol = groupSelectCol + " as \"group\""
	}
	err := LOG_DB.Model(&Log{}).
		WithContext(ctx).
		Select("id, created_at, type, "+groupSelectCol+", channel_id, request_id, use_time, other, content").
		Where("created_at >= ? AND channel_id IN ? AND type IN ?", startTs, channelIds, []int{LogTypeConsume, LogTypeError}).
		Order("id asc").
		Find(&rows).Error
	return rows, err
}
