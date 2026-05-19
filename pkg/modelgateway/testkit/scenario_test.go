package testkit

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPortableLoadDispatchScenario(t *testing.T) {
	scenario, err := LoadDispatchScenario(filepath.Join("..", "testdata", "dispatch", "group_off.json"))
	require.NoError(t, err)
	require.Equal(t, "group_off", scenario.Name)
	require.Equal(t, "default", scenario.Request.RequestedGroup)
	require.False(t, scenario.Expected.Handled)
}

func TestPortableFakeRuntimeSnapshotStore(t *testing.T) {
	store := NewFakeRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", ChannelID: 9, Group: "default"}
	store.Put(core.RuntimeSnapshot{Key: key, SuccessRate: 0.9})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 0.9, snapshot.SuccessRate)
	require.Len(t, store.ListCandidates(&core.DispatchRequest{ModelName: "gpt-4.1"}), 1)
	require.Empty(t, store.ListCandidates(&core.DispatchRequest{ModelName: "other"}))
}

func TestDispatchScenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range DispatchScenarioPaths(t) {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			RunDispatchScenario(t, path)
		})
	}
}
