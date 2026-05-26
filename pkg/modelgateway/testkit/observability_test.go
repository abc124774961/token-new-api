package testkit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelExecutionObservationRecordsCoverSummaryDimensions(t *testing.T) {
	records, err := ModelExecutionObservationRecords()
	require.NoError(t, err)
	require.Len(t, records, 6)

	summary, err := SummarizeModelExecutionObservationRecords(records)
	require.NoError(t, err)
	require.Equal(t, 6, summary.Total)
	require.Equal(t, 3, summary.Successes)
	require.Equal(t, 3, summary.Failures)
	require.InDelta(t, 0.5, summary.SuccessRate, 0.0001)
	require.InDelta(t, 1250, summary.AvgDurationMs, 0.0001)
	require.InDelta(t, 160, summary.AvgTTFTMs, 0.0001)
	require.Equal(t, 2, summary.Fallbacks)
	require.InDelta(t, float64(2)/float64(6), summary.FallbackRate, 0.0001)
	require.Equal(t, 1, summary.StreamInterrupted)
	require.Equal(t, 5, summary.ActiveRecords)
	require.Equal(t, 1, summary.ShadowRecords)

	require.InDelta(t, 0.7066, summary.ScoreBreakdown["completion_rate"], 0.0001)
	require.InDelta(t, 0.675, summary.ScoreBreakdown["ttft_latency"], 0.0001)
	require.InDelta(t, 0.6833, summary.ScoreBreakdown["concurrency_load"], 0.0001)
	require.InDelta(t, 0.7416, summary.ScoreBreakdown["cost"], 0.0001)
	require.InDelta(t, 0.925, summary.ScoreBreakdown["group_priority"], 0.0001)

	require.Contains(t, summary.Models, "gpt-4.1")
	require.Contains(t, summary.Models, "claude-3.7-sonnet")
	require.Contains(t, summary.Models, "gemini-2.5-pro")
	require.InDelta(t, float64(2)/float64(3), summary.Models["gpt-4.1"].SuccessRate, 0.0001)
	require.Equal(t, 1, summary.Models["gpt-4.1"].Fallbacks)
	require.Equal(t, 1, summary.Models["claude-3.7-sonnet"].StreamInterrupted)

	require.Contains(t, summary.Groups, "default")
	require.Contains(t, summary.Groups, "vip")
	require.Contains(t, summary.Groups, "canary")
	require.Contains(t, summary.Groups, "research")
	require.Equal(t, 2, summary.Groups["vip"].Fallbacks)
	require.Equal(t, 1, summary.Groups["canary"].Successes)
	require.InDelta(t, 120, summary.Groups["default"].AvgTTFTMs, 0.0001)

	require.Contains(t, summary.Channels, 101)
	require.Contains(t, summary.Channels, 102)
	require.Contains(t, summary.Channels, 103)
	require.Contains(t, summary.Channels, 104)
	require.Contains(t, summary.Channels, 105)
	require.Equal(t, 2, summary.Channels[101].Total)
	require.Equal(t, 1, summary.Channels[102].Fallbacks)
	require.Equal(t, 1, summary.Channels[103].StreamInterrupted)

	requireRecordDimension(t, records, func(record model.ModelExecutionRecord) bool { return record.Success }, "success")
	requireRecordDimension(t, records, func(record model.ModelExecutionRecord) bool { return !record.Success }, "failure")
	requireRecordDimension(t, records, func(record model.ModelExecutionRecord) bool { return record.StreamInterrupted }, "stream interrupted")
	requireRecordDimension(t, records, func(record model.ModelExecutionRecord) bool { return record.PolicyMode == core.ModeActive }, "active")
	requireRecordDimension(t, records, func(record model.ModelExecutionRecord) bool { return record.Shadow }, "shadow")
}

func TestSeedModelExecutionObservationRecordsUsesSharedMemorySQLite(t *testing.T) {
	db := setupObservationFixtureDB(t)
	seeded, err := SeedModelExecutionObservationRecords(db)
	require.NoError(t, err)
	require.Len(t, seeded, 6)

	var records []model.ModelExecutionRecord
	require.NoError(t, db.Order("created_at ASC").Find(&records).Error)
	require.Len(t, records, 6)

	summary, err := SummarizeModelExecutionObservationRecords(records)
	require.NoError(t, err)
	require.Equal(t, 6, summary.Total)
	require.InDelta(t, 0.5, summary.SuccessRate, 0.0001)
	require.Equal(t, 2, summary.Groups["vip"].Total)
	require.Equal(t, 1, summary.Groups["research"].Successes)
	require.InDelta(t, 2000, summary.Groups["vip"].AvgDurationMs, 0.0001)
}

func TestModelExecutionObservationSummaryRejectsInvalidScoreBreakdown(t *testing.T) {
	records, err := ModelExecutionObservationRecords()
	require.NoError(t, err)
	records[0].ScoreBreakdown = "{bad"

	_, err = SummarizeModelExecutionObservationRecords(records)
	require.Error(t, err)
	require.Contains(t, err.Error(), "obs-success-001")
	require.Contains(t, err.Error(), "score_breakdown")
}

func setupObservationFixtureDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func requireRecordDimension(t *testing.T, records []model.ModelExecutionRecord, match func(model.ModelExecutionRecord) bool, label string) {
	t.Helper()

	for _, record := range records {
		if match(record) {
			return
		}
	}
	require.Failf(t, "missing observation record dimension", "expected at least one %s record", label)
}
