package integration_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedRuntimeSnapshotsFromExecutionRecords(t *testing.T) {
	db := setupRuntimeSnapshotSeedDB(t)
	channel := model.Channel{
		Id:     42,
		Name:   "seed-channel",
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Group:  "vip",
		Models: "gpt-5.4",
	}
	require.NoError(t, db.Create(&channel).Error)
	concurrencyMeta, err := common.Marshal(map[string]any{"concurrency_limited": true})
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      100,
			RequestId:      "seed-success",
			AttemptIndex:   0,
			ChannelId:      42,
			RequestedGroup: "auto",
			SelectedGroup:  "vip",
			RequestedModel: "gpt-5.4",
			EndpointType:   string(constant.EndpointTypeOpenAI),
			Success:        true,
			DurationMs:     1200,
			TTFTMs:         300,
		},
		{
			CreatedAt:      110,
			RequestId:      "seed-concurrency",
			AttemptIndex:   1,
			ChannelId:      42,
			RequestedGroup: "auto",
			SelectedGroup:  "vip",
			RequestedModel: "gpt-5.4",
			EndpointType:   string(constant.EndpointTypeOpenAI),
			StatusCode:     429,
			RequestMeta:    string(concurrencyMeta),
		},
	}).Error)

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	seeded, err := integration.SeedRuntimeSnapshotsFromExecutionRecords(context.Background(), store, 100)
	require.NoError(t, err)
	require.Equal(t, 1, seeded)

	snapshots := store.ListCandidates(nil)
	require.Len(t, snapshots, 1)
	require.Equal(t, 42, snapshots[0].Key.ChannelID)
	require.Equal(t, "gpt-5.4", snapshots[0].Key.RequestedModel)
	require.Equal(t, "vip", snapshots[0].Key.Group)
	require.Equal(t, 1, snapshots[0].SampleCount)
	require.Equal(t, 1.0, snapshots[0].SuccessRate)
	require.Equal(t, 300.0, snapshots[0].TTFTMs)
	require.Contains(t, snapshots[0].Key.CapabilityFingerprint, "standard_openai_compatible")
}

func setupRuntimeSnapshotSeedDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ModelExecutionRecord{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
