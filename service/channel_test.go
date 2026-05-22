package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
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

func TestKnownBalanceInsufficientChannelDetectionUsesConfirmedBalance(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	channel := &model.Channel{
		Id:                 411,
		Status:             common.ChannelStatusEnabled,
		Balance:            0,
		BalanceUpdatedTime: common.GetTimestamp(),
	}

	require.True(t, IsConfirmedBalanceInsufficientChannel(channel))
	require.True(t, IsKnownBalanceInsufficientChannel(channel))
	require.Equal(t, ChannelStatusReasonBalanceInsufficient, ChannelStatusReason(channel))

	channel.Balance = 0.01
	require.False(t, IsConfirmedBalanceInsufficientChannel(channel))
	require.False(t, IsKnownBalanceInsufficientChannel(channel))

	channel.Balance = 0
	channel.BalanceUpdatedTime = 0
	require.False(t, IsConfirmedBalanceInsufficientChannel(channel))
	require.False(t, IsKnownBalanceInsufficientChannel(channel))

	MarkChannelBalanceInsufficient(channel.Id)
	require.True(t, IsKnownBalanceInsufficientChannel(channel))
	require.Equal(t, ChannelStatusReasonBalanceInsufficient, ChannelStatusReason(channel))
	ClearChannelBalanceInsufficient(channel.Id)
	require.False(t, IsKnownBalanceInsufficientChannel(channel))
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

func TestBalanceInsufficientErrorUsesBalancePauseInsteadOfHealthDisable(t *testing.T) {
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	err := types.NewOpenAIError(
		errors.New("insufficient balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	require.True(t, IsBalanceInsufficientError(err))
	require.True(t, ShouldDisableChannelForBalance(err))
	require.False(t, ShouldDisableChannel(err))
}

func TestEnglishAccountBalanceMessageIsBalanceInsufficient(t *testing.T) {
	err := types.NewOpenAIError(
		errors.New(`{"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}`),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)

	require.True(t, IsBalanceInsufficientError(err))
}

func TestInsufficientUserQuotaOnlyPausesChannelWhenRetryable(t *testing.T) {
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	})

	upstreamErr := types.WithOpenAIError(types.OpenAIError{
		Message: "user quota is not enough",
		Type:    "insufficient_quota",
		Code:    string(types.ErrorCodeInsufficientUserQuota),
	}, http.StatusForbidden)
	require.True(t, IsBalanceInsufficientError(upstreamErr))
	require.True(t, ShouldDisableChannelForBalance(upstreamErr))
	require.False(t, ShouldDisableChannel(upstreamErr))

	localErr := types.NewErrorWithStatusCode(
		errors.New("用户额度不足"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)
	require.True(t, IsBalanceInsufficientError(localErr))
	require.False(t, ShouldDisableChannelForBalance(localErr))
	require.False(t, ShouldDisableChannel(localErr))
}

func TestShouldResumeErrorPausedChannelWaitsForPauseUntilAndSuccess(t *testing.T) {
	now := time.Now().Unix()
	channel := &model.Channel{
		Status: common.ChannelStatusAutoDisabled,
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": ChannelStatusReasonErrorPaused,
		"pause_until":   now + 60,
	})

	require.False(t, ShouldResumeErrorPausedChannel(channel, nil))

	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": ChannelStatusReasonErrorPaused,
		"pause_until":   now - 1,
	})
	require.True(t, ShouldResumeErrorPausedChannel(channel, nil))

	err := types.NewOpenAIError(errors.New("still failing"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	require.False(t, ShouldResumeErrorPausedChannel(channel, err))
}
