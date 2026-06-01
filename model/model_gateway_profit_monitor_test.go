package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestProfitRatioRecommendationFilterQuotesWindowColumnForMySQL(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
	})
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		rows := make([]ModelGatewayProfitRatioRecommendation, 0)
		return applyModelGatewayProfitRatioRecommendationFilter(
			tx.Model(&ModelGatewayProfitRatioRecommendation{}),
			ModelGatewayProfitRatioRecommendationFilter{
				Window:    "24H",
				Dimension: "Group",
			},
		).Order("created_at DESC, id DESC").Limit(8).Find(&rows)
	})

	require.Contains(t, sql, "WHERE `window` = '24h'")
	require.NotContains(t, strings.ToLower(sql), "where window =")
	require.Contains(t, sql, "`dimension` = 'group'")
}
