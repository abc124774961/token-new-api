package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ModelGatewayTrafficMetric stores hourly traffic bytes for real relay requests.
type ModelGatewayTrafficMetric struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	ModelName     string `json:"model_name" gorm:"size:128;uniqueIndex:idx_mg_traffic_bucket,priority:1"`
	Group         string `json:"group" gorm:"column:group;size:64;uniqueIndex:idx_mg_traffic_bucket,priority:2"`
	ChannelID     int    `json:"channel_id" gorm:"uniqueIndex:idx_mg_traffic_bucket,priority:3;index"`
	ProxyID       int    `json:"proxy_id" gorm:"uniqueIndex:idx_mg_traffic_bucket,priority:4;index"`
	BucketTs      int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_mg_traffic_bucket,priority:5;index"`
	RequestCount  int64  `json:"request_count" gorm:"default:0"`
	RequestBytes  int64  `json:"request_bytes" gorm:"default:0"`
	ResponseBytes int64  `json:"response_bytes" gorm:"default:0"`
	TotalBytes    int64  `json:"total_bytes" gorm:"default:0"`
}

func (ModelGatewayTrafficMetric) TableName() string {
	return "model_gateway_traffic_metrics"
}

func UpsertModelGatewayTrafficMetric(metric *ModelGatewayTrafficMetric) error {
	if metric == nil || metric.RequestCount == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "model_name"},
			{Name: "group"},
			{Name: "channel_id"},
			{Name: "proxy_id"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count":  gorm.Expr("request_count + ?", metric.RequestCount),
			"request_bytes":  gorm.Expr("request_bytes + ?", metric.RequestBytes),
			"response_bytes": gorm.Expr("response_bytes + ?", metric.ResponseBytes),
			"total_bytes":    gorm.Expr("total_bytes + ?", metric.TotalBytes),
		}),
	}).Create(metric).Error
}

func ListModelGatewayTrafficMetrics(startTs int64, endTs int64) ([]ModelGatewayTrafficMetric, error) {
	rows := make([]ModelGatewayTrafficMetric, 0)
	if DB == nil {
		return rows, nil
	}
	err := DB.Model(&ModelGatewayTrafficMetric{}).
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Find(&rows).Error
	return rows, err
}

func DeleteModelGatewayTrafficMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 || DB == nil {
		return nil
	}
	return DB.Where("bucket_ts < ?", cutoffTs).Delete(&ModelGatewayTrafficMetric{}).Error
}
