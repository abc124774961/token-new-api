package probe

import (
	"testing"
	"time"

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
