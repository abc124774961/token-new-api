package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBalanceInsufficientPausedChannelDetection(t *testing.T) {
	channel := &model.Channel{
		Status: common.ChannelStatusAutoDisabled,
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": ChannelStatusReasonBalanceInsufficient,
	})

	require.True(t, IsBalanceInsufficientPausedChannel(channel))

	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": "401 invalid api key",
	})
	require.False(t, IsBalanceInsufficientPausedChannel(channel))

	channel.Status = common.ChannelStatusManuallyDisabled
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": ChannelStatusReasonBalanceInsufficient,
	})
	require.False(t, IsBalanceInsufficientPausedChannel(channel))
}

func TestShouldResumeBalancePausedChannelRequiresEnabledAndThreshold(t *testing.T) {
	originalEnabled := common.ChannelBalanceAutoResumeEnabled
	originalThreshold := common.ChannelBalanceRecoveryThreshold
	common.ChannelBalanceAutoResumeEnabled = true
	common.ChannelBalanceRecoveryThreshold = 1
	t.Cleanup(func() {
		common.ChannelBalanceAutoResumeEnabled = originalEnabled
		common.ChannelBalanceRecoveryThreshold = originalThreshold
	})

	require.False(t, ShouldResumeBalancePausedChannel(1))
	require.True(t, ShouldResumeBalancePausedChannel(1.01))

	common.ChannelBalanceAutoResumeEnabled = false
	require.False(t, ShouldResumeBalancePausedChannel(2))
}
