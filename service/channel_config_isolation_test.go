package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestConfigIsolationOneFailureDoesNotIsolate(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	key := NewChannelConfigIsolationKey(1001, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	status := manager.RecordAuthError(key, "upstream 401")

	require.NotNil(t, status)
	require.False(t, status.Active)
	require.Equal(t, "upstream 401", status.Reason)
	require.Equal(t, 1, status.FailureCount)
	require.Equal(t, now.Unix(), status.LastErrorAt)
	require.Zero(t, status.Until)
	require.Zero(t, status.RemainingSec)
	require.False(t, manager.IsIsolated(key))
}

func TestConfigIsolationTwoFailuresRemainNonBlocking(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	key := NewChannelConfigIsolationKey(1002, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	require.False(t, manager.RecordAuthError(key, "first 403").Active)
	status := manager.RecordAuthError(key, "second 403")

	require.NotNil(t, status)
	require.False(t, status.Active)
	require.Equal(t, "second 403", status.Reason)
	require.Equal(t, 2, status.FailureCount)
	require.Equal(t, now.Unix(), status.LastErrorAt)
	require.Zero(t, status.Until)
	require.Zero(t, status.RemainingSec)
	require.False(t, manager.IsIsolated(key))
}

func TestConfigIsolationSuccessClearsKey(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	key := NewChannelConfigIsolationKey(1003, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	manager.RecordAuthError(key, "401")
	manager.RecordAuthError(key, "401")
	require.False(t, manager.IsIsolated(key))
	require.NotNil(t, manager.GetStatus(key))

	manager.RecordSuccess(key)

	require.Nil(t, manager.GetStatus(key))
	require.False(t, manager.IsIsolated(key))
}

func TestConfigIsolationAuthTrackingDoesNotUseTTL(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	key := NewChannelConfigIsolationKey(1004, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	manager.RecordAuthError(key, "401")
	manager.RecordAuthError(key, "403")
	require.False(t, manager.IsIsolated(key))

	now = now.Add(time.Hour + time.Second)

	require.NotNil(t, manager.GetStatus(key))
	require.False(t, manager.IsIsolated(key))
}

func TestConfigIsolationKeysAreIndependent(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	baseKey := NewChannelConfigIsolationKey(1005, "gpt-4o", "default", constant.EndpointTypeOpenAI)
	otherModelKey := NewChannelConfigIsolationKey(1005, "gpt-4.1", "default", constant.EndpointTypeOpenAI)
	otherGroupKey := NewChannelConfigIsolationKey(1005, "gpt-4o", "vip", constant.EndpointTypeOpenAI)
	otherEndpointKey := NewChannelConfigIsolationKey(1005, "gpt-4o", "default", constant.EndpointTypeOpenAIResponse)
	otherChannelKey := NewChannelConfigIsolationKey(1006, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	manager.RecordAuthError(baseKey, "401")
	manager.RecordAuthError(baseKey, "403")

	status := manager.GetStatus(baseKey)
	require.NotNil(t, status)
	require.Equal(t, 2, status.FailureCount)
	require.False(t, manager.IsIsolated(baseKey))
	require.False(t, manager.IsIsolated(otherModelKey))
	require.False(t, manager.IsIsolated(otherGroupKey))
	require.False(t, manager.IsIsolated(otherEndpointKey))
	require.False(t, manager.IsIsolated(otherChannelKey))
}

func TestConfigIsolationClearForChannel(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	channelKey := NewChannelConfigIsolationKey(1007, "gpt-4o", "default", constant.EndpointTypeOpenAI)
	channelPeerKey := NewChannelConfigIsolationKey(1007, "gpt-4.1", "default", constant.EndpointTypeOpenAI)
	otherChannelKey := NewChannelConfigIsolationKey(1008, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	for _, key := range []ChannelConfigIsolationKey{channelKey, channelPeerKey, otherChannelKey} {
		manager.RecordAuthError(key, "401")
		manager.RecordAuthError(key, "403")
		require.NotNil(t, manager.GetStatus(key))
		require.False(t, manager.IsIsolated(key))
	}

	manager.ClearForChannel(1007)

	require.Nil(t, manager.GetStatus(channelKey))
	require.Nil(t, manager.GetStatus(channelPeerKey))
	require.NotNil(t, manager.GetStatus(otherChannelKey))
	require.False(t, manager.IsIsolated(otherChannelKey))
}

func TestConfigIsolationConcurrentRecordsAreSafe(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	manager := newChannelConfigIsolationManager(time.Hour, 2, func() time.Time { return now })
	key := NewChannelConfigIsolationKey(1009, "gpt-4o", "default", constant.EndpointTypeOpenAI)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.RecordAuthError(key, "401")
		}()
	}
	wg.Wait()

	status := manager.GetStatus(key)
	require.NotNil(t, status)
	require.False(t, status.Active)
	require.Equal(t, 32, status.FailureCount)
}
