package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/stretchr/testify/require"
)

func TestMarkAndClearCodexAccountUsageLimitPreservesCapability(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	streamAllowed := true
	channel := &model.Channel{
		Id:     9901,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"access","account_id":"acct"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	changed, err := MarkCodexAccountUsageLimited(channel.Id, 0, "usage limit has been reached")
	require.NoError(t, err)
	require.True(t, changed)

	var saved model.Channel
	require.NoError(t, db.First(&saved, "id = ?", channel.Id).Error)
	capability := saved.ChannelInfo.MultiKeyCapabilities[0]
	require.True(t, capability.HasCodexBackendResponsesStreamAllowed())
	require.True(t, capability.UsageLimitActiveAt(common.GetTimestamp()))
	require.Equal(t, channelcapability.ClassificationAccountUsageLimited, capability.EffectiveClassification())

	changed, err = ClearCodexAccountUsageLimit(channel.Id, 0)
	require.NoError(t, err)
	require.True(t, changed)

	require.NoError(t, db.First(&saved, "id = ?", channel.Id).Error)
	capability = saved.ChannelInfo.MultiKeyCapabilities[0]
	require.True(t, capability.HasCodexBackendResponsesStreamAllowed())
	require.False(t, capability.UsageLimitActiveAt(common.GetTimestamp()))
}

func TestCodexAccountUsageLimitCooldownFromMetadata(t *testing.T) {
	now := common.GetTimestamp()
	tests := []struct {
		name       string
		metadata   string
		wantSource string
		wantMin    int64
		wantMax    int64
	}{
		{
			name:       "retry after seconds",
			metadata:   `{"retry_after_seconds":45,"rate_limit_reset_requests":"2m"}`,
			wantSource: "retry_after_seconds",
			wantMin:    45,
			wantMax:    45,
		},
		{
			name:       "duration reset",
			metadata:   `{"rate_limit_reset_requests":"90s"}`,
			wantSource: "rate_limit_reset_requests",
			wantMin:    90,
			wantMax:    90,
		},
		{
			name:       "unix timestamp reset",
			metadata:   fmt.Sprintf(`{"rate_limit_reset":%d}`, now+120),
			wantSource: "rate_limit_reset",
			wantMin:    119,
			wantMax:    120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seconds, source := CodexAccountUsageLimitCooldownFromMetadata([]byte(tt.metadata), now)
			require.Equal(t, tt.wantSource, source)
			require.GreaterOrEqual(t, seconds, tt.wantMin)
			require.LessOrEqual(t, seconds, tt.wantMax)
		})
	}
}

func TestMarkCodexAccountUsageLimitUsesParsedCooldown(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	streamAllowed := true
	channel := &model.Channel{
		Id:     9902,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"access","account_id":"acct"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	now := common.GetTimestamp()
	changed, err := MarkCodexAccountUsageLimitedWithCooldown(channel.Id, 0, "usage limit has been reached", 75, "retry_after_seconds")
	require.NoError(t, err)
	require.True(t, changed)

	var saved model.Channel
	require.NoError(t, db.First(&saved, "id = ?", channel.Id).Error)
	capability := saved.ChannelInfo.MultiKeyCapabilities[0]
	require.Equal(t, "retry_after_seconds", capability.UsageLimitResetSource)
	require.GreaterOrEqual(t, capability.UsageLimitExpiresAt, now+75)
	require.LessOrEqual(t, capability.UsageLimitExpiresAt, now+76)
}

func TestMarkCodexAccountUsageLimitThrottleStillExtendsCooldown(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	streamAllowed := true
	channel := &model.Channel{
		Id:     9903,
		Type:   constant.ChannelTypeCodex,
		Key:    `{"access_token":"access","account_id":"acct"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}
	require.NoError(t, db.Create(channel).Error)

	changed, err := MarkCodexAccountUsageLimitedWithCooldown(channel.Id, 0, "usage limit has been reached", 30, "retry_after_seconds")
	require.NoError(t, err)
	require.True(t, changed)

	var saved model.Channel
	require.NoError(t, db.First(&saved, "id = ?", channel.Id).Error)
	firstExpiresAt := saved.ChannelInfo.MultiKeyCapabilities[0].UsageLimitExpiresAt

	changed, err = MarkCodexAccountUsageLimitedWithCooldown(channel.Id, 0, "usage limit has been reached", 180, "rate_limit_reset_requests")
	require.NoError(t, err)
	require.True(t, changed)

	require.NoError(t, db.First(&saved, "id = ?", channel.Id).Error)
	capability := saved.ChannelInfo.MultiKeyCapabilities[0]
	require.Greater(t, capability.UsageLimitExpiresAt, firstExpiresAt)
	require.Equal(t, "rate_limit_reset_requests", capability.UsageLimitResetSource)
}
