package model

import (
	"context"
	"strings"
	"time"
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
	err := LOG_DB.Model(&Log{}).
		WithContext(ctx).
		Select("id, created_at, type, channel_id, request_id, use_time, other, content").
		Where("created_at >= ? AND channel_id IN ? AND type IN ?", startTs, channelIds, []int{LogTypeConsume, LogTypeError}).
		Order("id asc").
		Find(&rows).Error
	return rows, err
}
