package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDeleteOldModelExecutionRecordsDeletesOnlyBeforeCutoff(t *testing.T) {
	db := setupModelExecutionRecordTestDB(t)
	now := int64(1_700_000_000)

	require.NoError(t, db.Create(&[]ModelExecutionRecord{
		{CreatedAt: now - 300, RequestId: "old-1"},
		{CreatedAt: now - 200, RequestId: "old-2"},
		{CreatedAt: now - 100, RequestId: "new-1"},
	}).Error)

	deleted, err := DeleteOldModelExecutionRecords(context.Background(), now-150, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var remaining []string
	require.NoError(t, db.Model(&ModelExecutionRecord{}).Order("request_id asc").Pluck("request_id", &remaining).Error)
	require.Equal(t, []string{"new-1", "old-2"}, remaining)
}

func setupModelExecutionRecordTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelExecutionRecord{}))
	require.NoError(t, EnsureModelExecutionRecordRequestMetaCapacity(db))

	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
