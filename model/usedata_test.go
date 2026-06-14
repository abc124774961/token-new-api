package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupQuotaDataTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &QuotaData{}))

	oldDB := DB
	oldLogDB := LOG_DB
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGetQuotaDataByUserIdUsesConsumeLogs(t *testing.T) {
	db := setupQuotaDataTestDB(t)
	require.NoError(t, db.Create(&QuotaData{
		UserID:    7,
		Username:  "alice",
		ModelName: "gpt-old",
		CreatedAt: 3600,
		Count:     99,
		Quota:     9900,
		TokenUsed: 990000,
	}).Error)
	require.NoError(t, db.Create(&[]Log{
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        3661,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.5",
			Quota:            120,
			PromptTokens:     1000,
			CompletionTokens: 20,
		},
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        3999,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.5",
			Quota:            80,
			PromptTokens:     700,
			CompletionTokens: 10,
		},
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        4000,
			Type:             LogTypeError,
			ModelName:        "gpt-5.5",
			Quota:            1000,
			PromptTokens:     1000,
			CompletionTokens: 1000,
		},
	}).Error)

	rows, err := GetQuotaDataByUserId(7, 3600, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "gpt-5.5", rows[0].ModelName)
	require.Equal(t, int64(3600), rows[0].CreatedAt)
	require.Equal(t, 2, rows[0].Count)
	require.Equal(t, 200, rows[0].Quota)
	require.Equal(t, 1730, rows[0].TokenUsed)
}

func TestGetQuotaDataGroupByUserUsesConsumeLogs(t *testing.T) {
	db := setupQuotaDataTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        3661,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.5",
			Quota:            120,
			PromptTokens:     1000,
			CompletionTokens: 20,
		},
		{
			UserId:           8,
			Username:         "bob",
			CreatedAt:        3662,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.4",
			Quota:            90,
			PromptTokens:     500,
			CompletionTokens: 5,
		},
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        7300,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.5",
			Quota:            50,
			PromptTokens:     100,
			CompletionTokens: 10,
		},
	}).Error)

	rows, err := GetQuotaDataGroupByUser(3600, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byUser := map[string]*QuotaData{}
	for _, row := range rows {
		byUser[row.Username] = row
	}
	require.Equal(t, 1, byUser["alice"].Count)
	require.Equal(t, 120, byUser["alice"].Quota)
	require.Equal(t, 1020, byUser["alice"].TokenUsed)
	require.Equal(t, 1, byUser["bob"].Count)
	require.Equal(t, 90, byUser["bob"].Quota)
	require.Equal(t, 505, byUser["bob"].TokenUsed)
}
