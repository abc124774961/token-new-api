package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestPublicHomeStatusUsesFinalRequestOutcome(t *testing.T) {
	now := time.Now()
	rows := []model.ChannelStatusMonitorLogRow{
		{
			Id:        1,
			CreatedAt: now.Unix(),
			Type:      model.LogTypeError,
			ChannelId: 10,
			RequestId: "req-failover-success",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, too many requests",
		},
		{
			Id:        2,
			CreatedAt: now.Add(time.Second).Unix(),
			Type:      model.LogTypeConsume,
			ChannelId: 11,
			RequestId: "req-failover-success",
			UseTime:   2,
			Other:     `{"frt":180}`,
		},
		{
			Id:        3,
			CreatedAt: now.Add(2 * time.Second).Unix(),
			Type:      model.LogTypeError,
			ChannelId: 12,
			RequestId: "req-total-failure",
			Other:     `{"status_code":500}`,
			Content:   "status_code=500, upstream failed",
		},
	}

	response := buildPublicHomeStatusFromRows(7, rows)

	require.EqualValues(t, 7, response.Summary.Days)
	require.EqualValues(t, 2, response.Summary.Requests)
	require.InDelta(t, 50, response.Summary.SuccessRate, 0.001)
	require.EqualValues(t, 180, response.Summary.AvgLatencyMs)
	require.EqualValues(t, 2, response.Summary.ProtectedEvents)
	require.Len(t, response.Daily, 7)

	var today PublicHomeStatusDaily
	for _, item := range response.Daily {
		if item.Date == now.Format("2006-01-02") {
			today = item
		}
	}
	require.EqualValues(t, 2, today.Requests)
	require.InDelta(t, 50, today.SuccessRate, 0.001)
	require.EqualValues(t, 2, today.ProtectedEvents)
}

func TestPublicHomeStatusEmptyKeepsDailyWindow(t *testing.T) {
	response := buildPublicHomeStatusFromRows(30, nil)

	require.EqualValues(t, 30, response.Summary.Days)
	require.EqualValues(t, 0, response.Summary.Requests)
	require.EqualValues(t, 0, response.Summary.SuccessRate)
	require.Len(t, response.Daily, 30)
}
