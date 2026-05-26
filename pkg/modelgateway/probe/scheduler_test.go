package probe

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRecentRealUserTrafficActiveUsesThirtyMinuteWindow(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	active, err := recentRealUserTrafficActive()
	require.NoError(t, err)
	require.False(t, active)

	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-old", "gpt-4.1", "default", "default", now.Add(-31*time.Minute).Unix())
	active, err = recentRealUserTrafficActive()
	require.NoError(t, err)
	require.False(t, active)

	seedProbeSelectorRecentRequest(t, db, "req-recent", "gpt-4.1", "default", "default", now.Add(-29*time.Minute).Unix())
	active, err = recentRealUserTrafficActive()
	require.NoError(t, err)
	require.True(t, active)
}

func TestProbeSchedulerLifecycleStopsWhenDisabled(t *testing.T) {
	StopDefaultProbeScheduler()
	t.Cleanup(StopDefaultProbeScheduler)

	setting := scheduler_setting.DefaultSetting()
	setting.ProbeEnabled = true
	restoreSetting := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restoreSetting)

	RegisterRelayInvoker(func(c *gin.Context, relayFormat types.RelayFormat) {})
	t.Cleanup(func() { RegisterRelayInvoker(nil) })

	scheduler := SyncDefaultProbeSchedulerLifecycle()
	require.NotNil(t, scheduler)
	require.True(t, DefaultProbeSchedulerRunning())

	before := scheduler_setting.GetSetting()
	disabled := before
	disabled.ProbeEnabled = false
	scheduler_setting.SetSetting(disabled)
	SyncDefaultProbeSchedulerLifecycle()

	require.False(t, DefaultProbeSchedulerRunning())
}
