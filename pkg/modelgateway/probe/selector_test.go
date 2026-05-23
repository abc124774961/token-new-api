package probe

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProbeSelectorSkipsWhenNoRecentRealTraffic(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:               key,
		SampleCount:       5,
		SuccessRate:       0.2,
		SuccessScore:      0.2,
		LastRealAttemptAt: time.Now().Unix(),
	})
	seedProbeSelectorChannel(t, db, 1, "low-score", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorSelectsLowScoreRuntimeWithRecentTraffic(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-low-score", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "low-score", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        8,
		SuccessRate:        0.4,
		SuccessScore:       0.3,
		SpeedScore:         0.5,
		ExperienceScore:    0.6,
		LastRealAttemptAt:  now.Unix(),
		RealSampleCount30m: 2,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second, LowScoreThreshold: 0.7})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, reasonLowScore, candidates[0].Reason)
	require.Equal(t, key, candidates[0].Key)
	require.Equal(t, "default", candidates[0].Group)
}

func TestProbeSelectorSelectsLowTrafficOnlyForRecentScopes(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-traffic", "gpt-4.1", "codex-pro", "codex-pro", now.Unix())
	target := seedProbeSelectorChannel(t, db, 1, "target", "codex-pro", "gpt-4.1", 1)
	_ = seedProbeSelectorChannel(t, db, 2, "other-model", "codex-pro", "gpt-5.5", 1)
	_ = seedProbeSelectorChannel(t, db, 3, "other-group", "codex-plus", "gpt-4.1", 1)
	model.InitChannelCache()

	selector := NewProbeSelector(scheduler.NewMemoryRuntimeSnapshotStore(), nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, reasonLowTraffic, candidates[0].Reason)
	require.Equal(t, target.Id, candidates[0].Channel.Id)
	require.Equal(t, "gpt-4.1", candidates[0].Model)
	require.Equal(t, "codex-pro", candidates[0].Group)
}

func TestProbeSelectorRateLimitsByRuntimeKey(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-rate-limit", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "target", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        5,
		SuccessRate:        0.3,
		SuccessScore:       0.3,
		LastRealAttemptAt:  now.Unix(),
		RealSampleCount30m: 1,
		LastProbeAt:        now.Add(-time.Minute).Unix(),
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: 5 * time.Minute, LowScoreThreshold: 0.7})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestRuntimeHealthMonitorDoesNotUpdateRealAccessForProbe(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	realAt := time.Now().Add(-time.Minute)
	probeAt := time.Now()

	monitor.Report(nil, core.AttemptResult{
		Key:        key,
		ChannelID:  1,
		ModelName:  "gpt-4.1",
		Success:    false,
		ObservedAt: realAt,
	})
	monitor.Report(nil, core.AttemptResult{
		Key:           key,
		ChannelID:     1,
		ModelName:     "gpt-4.1",
		Success:       true,
		ObservedAt:    probeAt,
		IsHealthProbe: true,
		ProbeReason:   reasonLowTraffic,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, realAt.Unix(), snapshot.LastRealAttemptAt)
	require.Zero(t, snapshot.LastRealSuccessAt)
	require.Equal(t, realAt.Unix(), snapshot.LastRealFailureAt)
	require.Equal(t, probeAt.Unix(), snapshot.LastProbeAt)
	require.Equal(t, probeAt.Unix(), snapshot.LastProbeSuccessAt)
	require.Equal(t, 1, snapshot.RealSampleCount30m)
	require.Equal(t, 2, snapshot.SampleCount)
}

func setupProbeSelectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelGatewayUserRequestSummary{}))

	oldDB := model.DB
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		if oldMemoryCache && oldDB != nil {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedProbeSelectorChannel(t *testing.T, db *gorm.DB, id int, name string, group string, modelName string, priority int64) *model.Channel {
	t.Helper()
	weight := uint(100)
	channel := &model.Channel{
		Id:          id,
		Type:        constant.ChannelTypeOpenAI,
		Name:        name,
		Key:         fmt.Sprintf("sk-%d", id),
		Status:      common.ChannelStatusEnabled,
		Group:       group,
		Models:      modelName,
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: int64(id),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}

func seedProbeSelectorRecentRequest(t *testing.T, db *gorm.DB, requestID string, modelName string, requestedGroup string, selectedGroup string, completedAt int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:      requestID,
		CreatedAt:      completedAt,
		UpdatedAt:      completedAt,
		CompletedAt:    completedAt,
		RequestedModel: modelName,
		RequestedGroup: requestedGroup,
		SelectedGroup:  selectedGroup,
		FinalChannelID: 1,
		FinalSuccess:   true,
	}).Error)
}
