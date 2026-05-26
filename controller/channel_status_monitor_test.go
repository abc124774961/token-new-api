package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
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

func TestChannelMonitorRecentUserRequestStatusUsesFinalOutcome(t *testing.T) {
	rows := []model.ModelGatewayUserRequestSummary{
		{
			Id:             1,
			CompletedAt:    100,
			RequestedGroup: "codex-pro",
			FinalSuccess:   true,
		},
		{
			Id:                 2,
			CompletedAt:        101,
			RequestedGroup:     "codex-pro",
			FinalErrorCategory: model.ModelGatewayUserRequestErrorRateLimit,
			FinalStatusCode:    429,
		},
		{
			Id:              3,
			CompletedAt:     102,
			RequestedGroup:  "codex-pro",
			FinalSuccess:    true,
			ExperienceIssue: "malformed_content",
		},
		{
			Id:                 4,
			CompletedAt:        103,
			RequestedGroup:     "codex-plus",
			FinalErrorCategory: model.ModelGatewayUserRequestErrorTimeout,
			FinalStatusCode:    504,
		},
	}

	statuses := buildChannelMonitorRecentUserRequestStatus(rows, 60)

	require.Equal(t, []string{"success", "rate_limit", "experience_issue"}, statuses["codex-pro"])
	require.Equal(t, []string{"timeout"}, statuses["codex-plus"])
}

func TestChannelMonitorRecentUserRequestStatusPrefersSelectedGroup(t *testing.T) {
	rows := []model.ModelGatewayUserRequestSummary{
		{
			Id:             1,
			CompletedAt:    100,
			RequestedGroup: "auto",
			SelectedGroup:  "codex-pro",
			FinalSuccess:   true,
		},
	}

	statuses := buildChannelMonitorRecentUserRequestStatus(rows, 60)

	require.Empty(t, statuses["auto"])
	require.Equal(t, []string{"success"}, statuses["codex-pro"])
}

func TestChannelMonitorUserRequestStatsUseUserPerspective(t *testing.T) {
	rows := []model.ModelGatewayUserRequestSummary{
		{
			Id:             1,
			RequestId:      "req-success",
			CompletedAt:    100,
			SelectedGroup:  "codex-pro",
			FinalChannelID: 10,
			FinalSuccess:   true,
			DurationMs:     1200,
			TTFTMs:         250,
		},
		{
			Id:                 2,
			RequestId:          "req-rate-limit",
			CompletedAt:        101,
			RequestedGroup:     "codex-pro",
			FinalChannelID:     10,
			FinalErrorCategory: model.ModelGatewayUserRequestErrorRateLimit,
			FinalStatusCode:    429,
		},
		{
			Id:                 3,
			RequestId:          "req-cancelled",
			CompletedAt:        102,
			RequestedGroup:     "codex-pro",
			FinalChannelID:     11,
			FinalErrorCategory: model.ModelGatewayUserRequestErrorClientAborted,
			FinalStatusCode:    relayStatusClientClosedRequest,
			ClientAborted:      true,
		},
		{
			Id:              4,
			RequestId:       "req-empty",
			CompletedAt:     103,
			RequestedGroup:  "codex-plus",
			FinalChannelID:  12,
			FinalSuccess:    true,
			EmptyOutput:     true,
			ExperienceIssue: "empty_output",
		},
	}

	stats := buildChannelMonitorUserRequestStats(rows)

	proStats := stats.byGroup["codex-pro"]
	require.NotNil(t, proStats)
	require.EqualValues(t, 3, proStats.requests)
	require.EqualValues(t, 1, proStats.successes)
	require.EqualValues(t, 1, proStats.failures)
	require.EqualValues(t, 1, proStats.clientAborted)
	require.EqualValues(t, 1, proStats.error429)
	require.EqualValues(t, 1200, avgInt64(proStats.latencySum, proStats.latencyCount))
	require.EqualValues(t, 250, avgInt64(proStats.firstRespSum, proStats.firstRespCount))
	require.Equal(t, float64(50), channelMonitorUserSuccessRate(proStats.successes, proStats.requests, proStats.clientAborted, proStats.healthProbes))

	plusStats := stats.byGroup["codex-plus"]
	require.NotNil(t, plusStats)
	require.EqualValues(t, 1, plusStats.requests)
	require.EqualValues(t, 1, plusStats.successes)
	require.EqualValues(t, 1, plusStats.emptyOutputs)
	require.EqualValues(t, 0, plusStats.experienceIssues)
}

func TestChannelMonitorRuntimeDoesNotFillUserRequestHistory(t *testing.T) {
	channel := &model.Channel{
		Id:     30,
		Name:   "runtime-channel",
		Status: common.ChannelStatusEnabled,
		Group:  "codex-pro",
		Models: "gpt-5.5",
	}
	response := buildChannelStatusMonitorFromRowsWithChannels(24, []*model.Channel{channel}, nil, nil)

	applyChannelStatusMonitorRuntimeStatus(&response, modelgatewayobservability.RuntimeStatusResponse{
		Items: []modelgatewayobservability.RuntimeStatusItem{
			{
				ChannelID:    30,
				Group:        "codex-pro",
				HealthStatus: "healthy",
				SampleCount:  2,
				SuccessRate:  1,
				ScoreTotal:   0.9,
			},
		},
	})

	require.Len(t, response.Groups, 1)
	require.Empty(t, response.Groups[0].RecentStatusSource)
	require.Empty(t, response.Groups[0].RecentStatus)
}

func TestChannelMonitorRuntimeRecentStatusDoesNotOverrideUserRequests(t *testing.T) {
	channel := &model.Channel{
		Id:     31,
		Name:   "runtime-channel",
		Status: common.ChannelStatusEnabled,
		Group:  "codex-pro",
		Models: "gpt-5.5",
	}
	response := buildChannelStatusMonitorFromRowsWithChannelsAndUserRequests(24, []*model.Channel{channel}, nil, nil, nil, []model.ModelGatewayUserRequestSummary{
		{
			Id:             1,
			CompletedAt:    100,
			RequestedGroup: "codex-pro",
			FinalSuccess:   true,
		},
	})

	applyChannelStatusMonitorRuntimeStatus(&response, modelgatewayobservability.RuntimeStatusResponse{
		Items: []modelgatewayobservability.RuntimeStatusItem{
			{
				ChannelID:    31,
				Group:        "codex-pro",
				HealthStatus: "degraded",
			},
		},
	})

	require.Len(t, response.Groups, 1)
	require.Equal(t, "user_requests", response.Groups[0].RecentStatusSource)
	require.Equal(t, []string{"success"}, response.Groups[0].RecentStatus)
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
