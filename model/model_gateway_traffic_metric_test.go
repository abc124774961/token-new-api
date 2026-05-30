package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelGatewayTrafficMetricTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelGatewayTrafficMetric{}))

	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		initCol()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestUpsertModelGatewayTrafficMetricAccumulatesBytes(t *testing.T) {
	setupModelGatewayTrafficMetricTestDB(t)
	first := &ModelGatewayTrafficMetric{
		ModelName:     "gpt-test",
		Group:         "default",
		ChannelID:     7,
		ProxyID:       2,
		BucketTs:      3600,
		RequestCount:  1,
		RequestBytes:  120,
		ResponseBytes: 880,
		TotalBytes:    1000,
	}
	second := *first
	second.RequestCount = 2
	second.RequestBytes = 300
	second.ResponseBytes = 700
	second.TotalBytes = 1000

	require.NoError(t, UpsertModelGatewayTrafficMetric(first))
	require.NoError(t, UpsertModelGatewayTrafficMetric(&second))

	rows, err := ListModelGatewayTrafficMetrics(0, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(3), rows[0].RequestCount)
	require.Equal(t, int64(420), rows[0].RequestBytes)
	require.Equal(t, int64(1580), rows[0].ResponseBytes)
	require.Equal(t, int64(2000), rows[0].TotalBytes)
}
