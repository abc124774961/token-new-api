package scheduler_test

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeSnapshotPersistenceFlushAndRestore(t *testing.T) {
	db := setupRuntimeSnapshotPersistenceDB(t)
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.4",
		UpstreamModel:         "gpt-5.4",
		ChannelID:             42,
		Group:                 "vip",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "openai_codex|native",
	}
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	store.Put(core.RuntimeSnapshot{
		Key: key,
		RecentLatencySamples: []core.RuntimeLatencySample{
			{ObservedAt: 100, TTFTMs: 500, DurationMs: 2200},
			{ObservedAt: 101, TTFTMs: 700, DurationMs: 2400},
			{ObservedAt: 102, TTFTMs: 840, DurationMs: 2600},
		},
		SuccessRate:           0.96,
		TTFTMs:                680,
		DurationMs:            2400,
		SuccessScore:          0.92,
		SpeedScore:            0.84,
		ExperienceScore:       0.98,
		EmptyOutputRate:       0.01,
		ExperienceIssueRate:   0.02,
		ConfigErrorIsolated:   true,
		IsolationReason:       core.ErrorCategoryAuthConfigError,
		IsolationUntil:        1770000000,
		AuthConfigErrorCount:  2,
		LastAuthConfigErrorAt: 1769999900,
		SampleCount:           37,
	})

	persistence := scheduler.NewRuntimeSnapshotPersistence(store, scheduler.RuntimeSnapshotPersistenceOptions{Batch: 2})
	require.NoError(t, persistence.Flush(context.Background()))

	var count int64
	require.NoError(t, db.Model(&model.ModelGatewayRuntimeSnapshot{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	restored := scheduler.NewMemoryRuntimeSnapshotStore()
	restorer := scheduler.NewRuntimeSnapshotPersistence(restored, scheduler.RuntimeSnapshotPersistenceOptions{})
	require.NoError(t, restorer.Restore(context.Background()))

	snapshot, ok := restored.Get(key)
	require.True(t, ok)
	require.Equal(t, 37, snapshot.SampleCount)
	require.Equal(t, 0.96, snapshot.SuccessRate)
	require.Equal(t, 700.0, snapshot.TTFTMs)
	require.Greater(t, snapshot.SpeedScore, 0.99)
	require.Len(t, snapshot.RecentLatencySamples, 3)
	require.True(t, snapshot.ConfigErrorIsolated)
	require.Equal(t, core.ErrorCategoryAuthConfigError, snapshot.IsolationReason)
	require.EqualValues(t, 1770000000, snapshot.IsolationUntil)
	require.Equal(t, 2, snapshot.AuthConfigErrorCount)
	require.EqualValues(t, 1769999900, snapshot.LastAuthConfigErrorAt)
	require.Zero(t, snapshot.ActiveConcurrency)
	require.Zero(t, snapshot.QueueDepth)
}

func TestRuntimeSnapshotPersistenceNormalizesAndCoalescesLegacyEndpointRows(t *testing.T) {
	db := setupRuntimeSnapshotPersistenceDB(t)
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "gpt-5.5",
		ChannelID:             8,
		Group:                 "codex-plus",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "openai_codex|native_responses",
	}
	legacyKey := key
	legacyKey.EndpointType = ""
	legacyBytes, err := common.Marshal(legacyKey)
	require.NoError(t, err)
	currentBytes, err := common.Marshal(key)
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]model.ModelGatewayRuntimeSnapshot{
		{
			RuntimeKeyHash:        "legacy-hash",
			RuntimeKey:            string(legacyBytes),
			UpdatedAt:             100,
			RequestedModel:        legacyKey.RequestedModel,
			UpstreamModel:         legacyKey.UpstreamModel,
			ChannelID:             legacyKey.ChannelID,
			Group:                 legacyKey.Group,
			EndpointType:          "",
			CapabilityFingerprint: legacyKey.CapabilityFingerprint,
			SampleCount:           10,
			SuccessRate:           1,
			TTFTMs:                3000,
			DurationMs:            4200,
			SuccessScore:          1,
			SpeedScore:            0.90,
			ExperienceScore:       1,
		},
		{
			RuntimeKeyHash:        runtimeSnapshotPersistenceTestHash("current"),
			RuntimeKey:            string(currentBytes),
			UpdatedAt:             120,
			RequestedModel:        key.RequestedModel,
			UpstreamModel:         key.UpstreamModel,
			ChannelID:             key.ChannelID,
			Group:                 key.Group,
			EndpointType:          string(key.EndpointType),
			CapabilityFingerprint: key.CapabilityFingerprint,
			SampleCount:           30,
			SuccessRate:           0.90,
			TTFTMs:                9000,
			DurationMs:            12000,
			SuccessScore:          0.80,
			SpeedScore:            0.40,
			ExperienceScore:       0.95,
		},
	}).Error)

	restored := scheduler.NewMemoryRuntimeSnapshotStore()
	restorer := scheduler.NewRuntimeSnapshotPersistence(restored, scheduler.RuntimeSnapshotPersistenceOptions{})
	require.NoError(t, restorer.Restore(context.Background()))

	snapshots := restored.ListCandidates(nil)
	require.Len(t, snapshots, 1)
	snapshot := snapshots[0]
	require.Equal(t, key, snapshot.Key)
	require.Equal(t, 40, snapshot.SampleCount)
	require.InEpsilon(t, 0.925, snapshot.SuccessRate, 0.001)
	require.Zero(t, snapshot.TTFTMs)
	require.Zero(t, snapshot.DurationMs)
	require.Zero(t, snapshot.SpeedScore)
}

func TestRuntimeSnapshotPersistenceCoalescesLatencySamplesWithTrimmedStats(t *testing.T) {
	db := setupRuntimeSnapshotPersistenceDB(t)
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "gpt-5.5",
		ChannelID:             9,
		Group:                 "codex-plus",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "openai_codex|native_responses",
	}
	keyBytes, err := common.Marshal(key)
	require.NoError(t, err)
	leftSamples := runtimeSnapshotPersistenceMustMarshal(t, []core.RuntimeLatencySample{
		{ObservedAt: 100, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 101, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 102, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 103, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 104, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 105, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 106, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 107, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 108, TTFTMs: 45000, DurationMs: 80000},
		{ObservedAt: 109, TTFTMs: 30, DurationMs: 80},
	})
	rightSamples := runtimeSnapshotPersistenceMustMarshal(t, []core.RuntimeLatencySample{
		{ObservedAt: 110, TTFTMs: 900, DurationMs: 2000},
		{ObservedAt: 111, TTFTMs: 900, DurationMs: 2000},
	})
	require.NoError(t, db.Create(&[]model.ModelGatewayRuntimeSnapshot{
		{
			RuntimeKeyHash:        "sample-left",
			RuntimeKey:            string(keyBytes),
			UpdatedAt:             100,
			RequestedModel:        key.RequestedModel,
			UpstreamModel:         key.UpstreamModel,
			ChannelID:             key.ChannelID,
			Group:                 key.Group,
			EndpointType:          string(key.EndpointType),
			CapabilityFingerprint: key.CapabilityFingerprint,
			LatencySamples:        leftSamples,
			SampleCount:           10,
			SuccessRate:           1,
			TTFTMs:                45000,
			SpeedScore:            0.002,
			SuccessScore:          1,
			ExperienceScore:       1,
		},
		{
			RuntimeKeyHash:        "sample-right",
			RuntimeKey:            string(keyBytes),
			UpdatedAt:             120,
			RequestedModel:        key.RequestedModel,
			UpstreamModel:         key.UpstreamModel,
			ChannelID:             key.ChannelID,
			Group:                 key.Group,
			EndpointType:          string(key.EndpointType),
			CapabilityFingerprint: key.CapabilityFingerprint,
			LatencySamples:        rightSamples,
			SampleCount:           2,
			SuccessRate:           1,
			TTFTMs:                900,
			SpeedScore:            0.995,
			SuccessScore:          1,
			ExperienceScore:       1,
		},
	}).Error)

	restored := scheduler.NewMemoryRuntimeSnapshotStore()
	restorer := scheduler.NewRuntimeSnapshotPersistence(restored, scheduler.RuntimeSnapshotPersistenceOptions{})
	require.NoError(t, restorer.Restore(context.Background()))

	snapshot, ok := restored.Get(key)
	require.True(t, ok)
	require.Equal(t, 12, snapshot.SampleCount)
	require.Len(t, snapshot.RecentLatencySamples, 12)
	require.InEpsilon(t, 900.0, snapshot.TTFTMs, 0.001)
	require.Greater(t, snapshot.SpeedScore, 0.98)
}

func setupRuntimeSnapshotPersistenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayRuntimeSnapshot{}))
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

func runtimeSnapshotPersistenceTestHash(value string) string {
	return "test-" + value
}

func runtimeSnapshotPersistenceMustMarshal(t *testing.T, v any) string {
	t.Helper()
	bytes, err := common.Marshal(v)
	require.NoError(t, err)
	return string(bytes)
}
