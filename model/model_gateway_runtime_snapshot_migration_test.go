package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeduplicateRuntimeSnapshotHashesKeepsLatestRow(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Migrator().DropTable(&ModelGatewayRuntimeSnapshot{}))
	t.Cleanup(func() {
		require.NoError(t, DB.Migrator().DropTable(&ModelGatewayRuntimeSnapshot{}))
		require.NoError(t, DB.AutoMigrate(&ModelGatewayRuntimeSnapshot{}))
	})
	require.NoError(t, DB.Exec(`CREATE TABLE model_gateway_runtime_snapshots (
		id integer primary key autoincrement,
		runtime_key_hash text,
		runtime_key text,
		updated_at integer,
		sample_count integer
	)`).Error)
	for _, row := range []map[string]any{
		{"runtime_key_hash": "dup-hash", "runtime_key": `{"channel_id":1}`, "updated_at": 100, "sample_count": 1},
		{"runtime_key_hash": "dup-hash", "runtime_key": `{"channel_id":2}`, "updated_at": 300, "sample_count": 3},
		{"runtime_key_hash": "dup-hash", "runtime_key": `{"channel_id":3}`, "updated_at": 200, "sample_count": 2},
		{"runtime_key_hash": "", "runtime_key": `{"channel_id":4}`, "updated_at": 400, "sample_count": 4},
		{"runtime_key_hash": "unique-hash", "runtime_key": `{"channel_id":5}`, "updated_at": 500, "sample_count": 5},
	} {
		require.NoError(t, DB.Table("model_gateway_runtime_snapshots").Create(&row).Error)
	}

	require.NoError(t, deduplicateRuntimeSnapshotHashes())

	var remaining []ModelGatewayRuntimeSnapshot
	require.NoError(t, DB.Order("runtime_key_hash ASC, updated_at DESC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	require.Equal(t, "dup-hash", remaining[0].RuntimeKeyHash)
	require.Equal(t, int64(300), remaining[0].UpdatedAt)
	require.Equal(t, 3, remaining[0].SampleCount)
	require.Equal(t, "unique-hash", remaining[1].RuntimeKeyHash)
	require.Equal(t, int64(500), remaining[1].UpdatedAt)
}
