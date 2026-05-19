package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorGroupStatsUseRequestOutcome(t *testing.T) {
	rows := []model.ChannelStatusMonitorLogRow{
		{
			Id:        1,
			CreatedAt: 100,
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			ChannelId: 10,
			RequestId: "req-success-after-failover",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, too many requests",
		},
		{
			Id:               2,
			CreatedAt:        101,
			Type:             model.LogTypeConsume,
			Group:            "codex-pro",
			ChannelId:        11,
			RequestId:        "req-success-after-failover",
			UseTime:          3,
			PromptTokens:     12,
			CompletionTokens: 34,
			Other:            `{"frt":250}`,
		},
		{
			Id:        3,
			CreatedAt: 102,
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			ChannelId: 10,
			RequestId: "req-total-failure",
			Other:     `{"status_code":500}`,
			Content:   "status_code=500, upstream failed",
		},
		{
			Id:        4,
			CreatedAt: 103,
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			ChannelId: 11,
			RequestId: "req-total-failure",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, still limited",
		},
	}

	statsIndex := buildChannelMonitorLogStats(rows)
	groupStats := statsIndex.byGroup["codex-pro"]

	require.NotNil(t, groupStats)
	require.EqualValues(t, 2, groupStats.requests)
	require.EqualValues(t, 1, groupStats.successes)
	require.EqualValues(t, 1, groupStats.failures)
	require.EqualValues(t, 1, groupStats.error429)
	require.EqualValues(t, 0, groupStats.error5xx)
	require.EqualValues(t, 250, avgInt64(groupStats.latencySum, groupStats.latencyCount))

	channelStats := statsIndex.byChannelGroup[10]["codex-pro"]
	require.EqualValues(t, 2, channelStats.requests)
	require.EqualValues(t, 2, channelStats.failures)
	require.EqualValues(t, 1, channelStats.error429)
	require.EqualValues(t, 1, channelStats.error5xx)
}

func TestChannelMonitorBalanceErrorsDoNotDeductHealthScore(t *testing.T) {
	channel := &model.Channel{
		Id:       20,
		Name:     "balance-paused",
		Status:   common.ChannelStatusAutoDisabled,
		Group:    "default",
		Models:   "gpt-5.5",
		TestTime: 100,
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": service.ChannelStatusReasonBalanceInsufficient,
	})

	rows := []model.ChannelStatusMonitorLogRow{
		{
			Id:        1,
			CreatedAt: 100,
			Type:      model.LogTypeError,
			Group:     "default",
			ChannelId: 20,
			RequestId: "req-balance",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, insufficient balance",
		},
	}

	response := buildChannelStatusMonitorFromRowsWithChannels(24, []*model.Channel{channel}, rows, nil)

	require.EqualValues(t, 1, response.Summary.TotalChannels)
	require.EqualValues(t, 0, response.Summary.CooldownChannels)
	require.EqualValues(t, 1, response.Summary.HealthyChannels)
	require.EqualValues(t, 0, response.Summary.BadChannels)
	require.Len(t, response.Groups, 1)
	require.Len(t, response.Groups[0].Channels, 1)

	item := response.Groups[0].Channels[0]
	require.False(t, item.Enabled)
	require.Equal(t, service.ChannelStatusReasonBalanceInsufficient, item.PauseType)
	require.EqualValues(t, 1, item.RecentBalanceErrors)
	require.EqualValues(t, 100, item.HealthScore)
	require.Equal(t, "healthy", item.HealthState)
}

func TestChannelMonitorErrorPauseShowsReasonAndRemaining(t *testing.T) {
	until := time.Now().Add(30 * time.Minute).Unix()
	channel := &model.Channel{
		Id:     21,
		Name:   "error-paused",
		Status: common.ChannelStatusAutoDisabled,
		Group:  "default",
		Models: "gpt-5.5",
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": service.ChannelStatusReasonErrorPaused,
		"pause_type":    service.ChannelStatusReasonErrorPaused,
		"pause_reason":  "upstream_error:502:bad_response_status_code",
		"pause_until":   until,
	})

	response := buildChannelStatusMonitorFromRowsWithChannels(24, []*model.Channel{channel}, nil, nil)

	require.Len(t, response.Groups, 1)
	item := response.Groups[0].Channels[0]
	require.False(t, item.Enabled)
	require.Equal(t, service.ChannelStatusReasonErrorPaused, item.PauseType)
	require.Equal(t, "upstream_error:502:bad_response_status_code", item.PauseReason)
	require.Equal(t, until, item.PauseUntil)
	require.Positive(t, item.PauseRemaining)
	require.Less(t, item.HealthScore, 100)
	require.NotEqual(t, "healthy", item.HealthState)
}

func TestChannelMonitorRecentStatusUsesRequestOutcome(t *testing.T) {
	rows := []model.ChannelStatusMonitorRecentLogRow{
		{
			Id:        4,
			CreatedAt: 103,
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			RequestId: "req-total-failure",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, still limited",
		},
		{
			Id:        3,
			CreatedAt: 102,
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			RequestId: "req-total-failure",
			Other:     `{"status_code":500}`,
			Content:   "status_code=500, upstream failed",
		},
		{
			Id:        2,
			CreatedAt: 101,
			Type:      model.LogTypeConsume,
			Group:     "codex-pro",
			RequestId: "req-success-after-failover",
		},
		{
			Id:        1,
			CreatedAt: 100,
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			RequestId: "req-success-after-failover",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, too many requests",
		},
	}

	statuses := buildChannelMonitorRecentStatus(rows, 60)

	require.Equal(t, []string{"success", "rate_limit"}, statuses["codex-pro"])
}

func TestChannelMonitorRecentStatusIsGroupedByUserRequestOutcome(t *testing.T) {
	rows := []model.ChannelStatusMonitorRecentLogRow{
		{
			Id:        1,
			CreatedAt: 100,
			Type:      model.LogTypeError,
			Group:     "codex-subscription",
			ChannelId: 4,
			RequestId: "req-failover-success",
			Other:     `{"status_code":502}`,
			Content:   "status_code=502, bad response",
		},
		{
			Id:        2,
			CreatedAt: 101,
			Type:      model.LogTypeConsume,
			Group:     "codex-subscription",
			ChannelId: 6,
			RequestId: "req-failover-success",
			Other:     `{"stream_status":{"status":"ok"}}`,
		},
		{
			Id:        3,
			CreatedAt: 102,
			Type:      model.LogTypeError,
			Group:     "codex-subscription",
			ChannelId: 4,
			RequestId: "req-final-failure",
			Other:     `{"status_code":500}`,
			Content:   "status_code=500, final upstream error",
		},
		{
			Id:        4,
			CreatedAt: 103,
			Type:      model.LogTypeConsume,
			Group:     "codex-plus",
			ChannelId: 2,
			RequestId: "req-other-group",
		},
	}

	statuses := buildChannelMonitorRecentStatus(rows, 60)

	require.Equal(t, []string{"success", "server_error"}, statuses["codex-subscription"])
	require.Equal(t, []string{"success"}, statuses["codex-plus"])
}

func TestChannelMonitorStreamConsumeErrorIsNotSuccessful(t *testing.T) {
	rows := []model.ChannelStatusMonitorLogRow{
		{
			Id:               1,
			CreatedAt:        100,
			Type:             model.LogTypeConsume,
			Group:            "codex-pro",
			ChannelId:        10,
			RequestId:        "req-stream-error",
			UseTime:          2,
			CompletionTokens: 10,
			Other:            `{"stream_status":{"status":"error","end_reason":"scanner_error","error_count":1}}`,
		},
		{
			Id:               2,
			CreatedAt:        101,
			Type:             model.LogTypeConsume,
			Group:            "codex-pro",
			ChannelId:        10,
			RequestId:        "req-client-gone",
			UseTime:          1,
			CompletionTokens: 5,
			Other:            `{"stream_status":{"status":"client_gone","end_reason":"client_gone"}}`,
		},
	}

	statsIndex := buildChannelMonitorLogStats(rows)
	groupStats := statsIndex.byGroup["codex-pro"]
	channelStats := statsIndex.byChannelGroup[10]["codex-pro"]

	require.NotNil(t, groupStats)
	require.EqualValues(t, 2, groupStats.requests)
	require.EqualValues(t, 1, groupStats.successes)
	require.EqualValues(t, 1, groupStats.failures)

	require.NotNil(t, channelStats)
	require.EqualValues(t, 2, channelStats.requests)
	require.EqualValues(t, 1, channelStats.successes)
	require.EqualValues(t, 1, channelStats.failures)
	require.EqualValues(t, 1, channelStats.streamErrors)
}
