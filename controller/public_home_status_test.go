package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/stretchr/testify/require"
)

func TestPublicHomeStatusUsesFinalRequestOutcome(t *testing.T) {
	now := time.Now()
	rows := []model.ChannelStatusMonitorLogRow{
		{
			Id:        1,
			CreatedAt: now.Unix(),
			Type:      model.LogTypeError,
			Group:     "codex-pro",
			ChannelId: 10,
			RequestId: "req-failover-success",
			Other:     `{"status_code":429}`,
			Content:   "status_code=429, too many requests",
		},
		{
			Id:        2,
			CreatedAt: now.Add(time.Second).Unix(),
			Type:      model.LogTypeConsume,
			Group:     "codex-pro",
			ChannelId: 11,
			RequestId: "req-failover-success",
			UseTime:   2,
			Other:     `{"frt":180}`,
		},
		{
			Id:        3,
			CreatedAt: now.Add(2 * time.Second).Unix(),
			Type:      model.LogTypeError,
			Group:     "cheap",
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
	require.EqualValues(t, 180, response.Summary.AvgTTFTMs)
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
	require.EqualValues(t, 180, today.AvgTTFTMs)
	require.EqualValues(t, 2, today.ProtectedEvents)
	require.Len(t, response.Groups, 4)
	require.Equal(t, "codex", response.Groups[0].Key)
	require.Equal(t, "Codex 专用", response.Groups[0].Name)
	require.EqualValues(t, 1, response.Groups[0].Summary.Requests)
	require.InDelta(t, 100, response.Groups[0].Summary.SuccessRate, 0.001)
	require.EqualValues(t, 180, response.Groups[0].Summary.AvgTTFTMs)
	require.Equal(t, "value", response.Groups[3].Key)
	require.EqualValues(t, 1, response.Groups[3].Summary.Requests)
	require.InDelta(t, 0, response.Groups[3].Summary.SuccessRate, 0.001)
}

func TestPublicHomeStatusEmptyKeepsDailyWindow(t *testing.T) {
	response := buildPublicHomeStatusFromRows(30, nil)

	require.EqualValues(t, 30, response.Summary.Days)
	require.EqualValues(t, 0, response.Summary.Requests)
	require.EqualValues(t, 0, response.Summary.SuccessRate)
	require.Len(t, response.Daily, 30)
	require.Len(t, response.Groups, 4)
	require.Len(t, response.Groups[0].Daily, 30)
}

func TestPublicHomeModelGatewayStatsOverrideFirstByteLatency(t *testing.T) {
	response := PublicHomeStatusResponse{
		Summary: PublicHomeStatusSummary{
			Requests:     2,
			SuccessRate:  50,
			AvgLatencyMs: 428,
			AvgTTFTMs:    180,
		},
	}

	applyPublicHomeModelGatewayStats(&response, publicHomeModelGatewayStats{
		Requests:     4676,
		Successes:    3914,
		SuccessRate:  83.70402053036784,
		AvgLatencyMs: 6120,
		AvgTTFTMs:    5490,
	})

	require.EqualValues(t, 4676, response.Summary.Requests)
	require.InDelta(t, 83.704, response.Summary.SuccessRate, 0.001)
	require.EqualValues(t, 6120, response.Summary.AvgLatencyMs)
	require.EqualValues(t, 5490, response.Summary.AvgTTFTMs)
}

func TestPublicHomeModelGatewayStatsKeepsExistingWhenEmpty(t *testing.T) {
	response := PublicHomeStatusResponse{
		Summary: PublicHomeStatusSummary{
			Requests:     2,
			SuccessRate:  50,
			AvgLatencyMs: 428,
			AvgTTFTMs:    180,
		},
	}

	applyPublicHomeModelGatewayStats(&response, publicHomeModelGatewayStats{
		AvgTTFTMs: 5490,
	})

	require.EqualValues(t, 2, response.Summary.Requests)
	require.InDelta(t, 50, response.Summary.SuccessRate, 0.001)
	require.EqualValues(t, 428, response.Summary.AvgLatencyMs)
	require.EqualValues(t, 5490, response.Summary.AvgTTFTMs)
}

func TestPublicHomeModelGatewayStatsFromUserRequestsUsesEffectiveRequests(t *testing.T) {
	stats := publicHomeModelGatewayStatsFromUserRequests([]model.ModelGatewayUserRequestSummary{
		{
			RequestId:     "req-success",
			FinalSuccess:  true,
			DurationMs:    6000,
			TTFTMs:        5320,
			SelectedGroup: "codex-plus",
		},
		{
			RequestId:          "req-failure",
			FinalStatusCode:    502,
			FinalErrorCategory: model.ModelGatewayUserRequestErrorUpstream,
			DurationMs:         9000,
			TTFTMs:             7000,
			SelectedGroup:      "codex-plus",
		},
		{
			RequestId:      "req-aborted",
			ClientAborted:  true,
			DurationMs:     12000,
			TTFTMs:         9000,
			RequestedGroup: "codex-plus",
		},
		{
			RequestId:     "req-probe",
			FinalSuccess:  true,
			IsHealthProbe: true,
			DurationMs:    100,
			TTFTMs:        80,
			SelectedGroup: "codex-plus",
		},
	})

	require.EqualValues(t, 2, stats.Requests)
	require.EqualValues(t, 1, stats.Successes)
	require.InDelta(t, 50, stats.SuccessRate, 0.001)
	require.EqualValues(t, 7500, stats.AvgLatencyMs)
	require.EqualValues(t, 6160, stats.AvgTTFTMs)
}

func TestPublicHomeModelGatewayStatsAggregatesFromDB(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := time.Now()

	require.NoError(t, db.Create(&model.Channel{
		Id:     301,
		Type:   1,
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Name:   "codex-enterprise",
		Group:  "codex-enterprise",
		Models: "gpt-5.5",
	}).Error)
	require.NoError(t, db.Create(&[]model.ModelGatewayUserRequestSummary{
		{
			RequestId:     "req-codex-plus-success",
			CompletedAt:   now.Unix(),
			SelectedGroup: "codex-plus",
			FinalSuccess:  true,
			DurationMs:    1000,
			TTFTMs:        300,
		},
		{
			RequestId:     "req-codex-plus-failure",
			CompletedAt:   now.Unix(),
			SelectedGroup: "codex-plus",
			DurationMs:    3000,
			TTFTMs:        900,
		},
		{
			RequestId:      "req-requested-group-fallback",
			CompletedAt:    now.Unix(),
			RequestedGroup: "codex-pro",
			FinalSuccess:   true,
			DurationMs:     5000,
			TTFTMs:         1500,
		},
		{
			RequestId:     "req-channel-derived-codex-group",
			CompletedAt:   now.Unix(),
			SelectedGroup: "codex-enterprise",
			FinalSuccess:  true,
			DurationMs:    7000,
			TTFTMs:        2100,
		},
		{
			RequestId:     "req-other-group",
			CompletedAt:   now.Unix(),
			SelectedGroup: "value",
			FinalSuccess:  true,
			DurationMs:    9000,
			TTFTMs:        2700,
		},
		{
			RequestId:     "req-health-probe",
			CompletedAt:   now.Unix(),
			SelectedGroup: "codex-plus",
			FinalSuccess:  true,
			IsHealthProbe: true,
			DurationMs:    100,
			TTFTMs:        80,
		},
		{
			RequestId:     "req-client-aborted",
			CompletedAt:   now.Unix(),
			SelectedGroup: "codex-plus",
			ClientAborted: true,
			DurationMs:    100,
			TTFTMs:        80,
		},
		{
			RequestId:     "req-too-old",
			CompletedAt:   now.Add(-25 * time.Hour).Unix(),
			SelectedGroup: "codex-plus",
			FinalSuccess:  true,
			DurationMs:    100,
			TTFTMs:        80,
		},
	}).Error)

	stats := buildPublicHomeModelGatewayStats()

	require.EqualValues(t, 4, stats.Requests)
	require.EqualValues(t, 3, stats.Successes)
	require.InDelta(t, 75, stats.SuccessRate, 0.001)
	require.EqualValues(t, 4000, stats.AvgLatencyMs)
	require.EqualValues(t, 1200, stats.AvgTTFTMs)
}

func TestPublicHomeDynamicBillingOnlyExposesDisplayPrice(t *testing.T) {
	ratio_setting.InitRatioSettings()
	db := setupModelGatewayReplayControllerTestDB(t)
	now := time.Now().Unix()
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingMinSamples:     1,
		DynamicBillingRefreshSeconds: 30,
		DynamicBillingMaxAgeSeconds:  300,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
				CandidateGroups:  []string{"codex-plus"},
			},
		},
	})
	defer restoreSetting()

	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-5.4:codex-plus": {
			RequestedModel: "gpt-5.4",
			Group:          "codex-plus",
			Ratio:          0.0693,
			PricePerM:      0.128,
			SampleCount:    8,
			CalculatedAt:   now - 24,
		},
	})
	defer restoreBaselines()

	require.NoError(t, db.Create(&[]model.ModelGatewayUserRequestSummary{
		{
			RequestId:       "req-home-range-low",
			CreatedAt:       now - 120,
			UpdatedAt:       now - 120,
			CompletedAt:     now - 120,
			RequestedModel:  "gpt-5.4",
			RequestedGroup:  "auto",
			SelectedGroup:   "codex-plus",
			FinalChannelID:  4,
			Attempts:        1,
			FinalSuccess:    true,
			FinalStatusCode: 200,
		},
		{
			RequestId:       "req-home-range-high",
			CreatedAt:       now - 60,
			UpdatedAt:       now - 60,
			CompletedAt:     now - 60,
			RequestedModel:  "gpt-5.4",
			RequestedGroup:  "auto",
			SelectedGroup:   "codex-plus",
			FinalChannelID:  4,
			Attempts:        1,
			FinalSuccess:    true,
			FinalStatusCode: 200,
		},
	}).Error)
	lowOther, err := common.Marshal(map[string]any{
		"dynamic_billing_applied":     true,
		"dynamic_billing_group":       "codex-plus",
		"dynamic_billing_ratio":       0.055,
		"dynamic_billing_price_per_m": 0.102,
	})
	require.NoError(t, err)
	highOther, err := common.Marshal(map[string]any{
		"dynamic_billing_applied":     true,
		"dynamic_billing_group":       "codex-plus",
		"dynamic_billing_ratio":       0.083,
		"dynamic_billing_price_per_m": 0.154,
	})
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{
			Type:             model.LogTypeConsume,
			RequestId:        "req-home-range-low",
			CreatedAt:        now - 120,
			ModelName:        "gpt-5.4",
			Group:            "codex-plus",
			PromptTokens:     100,
			CompletionTokens: 100,
			Quota:            100,
			Other:            string(lowOther),
		},
		{
			Type:             model.LogTypeConsume,
			RequestId:        "req-home-range-high",
			CreatedAt:        now - 60,
			ModelName:        "gpt-5.4",
			Group:            "codex-plus",
			PromptTokens:     100,
			CompletionTokens: 100,
			Quota:            100,
			Other:            string(highOther),
		},
	}).Error)

	result := buildPublicHomeDynamicBilling(now)

	require.NotNil(t, result)
	require.True(t, result.Enabled)
	require.Equal(t, "codex-plus", result.Group)
	require.Equal(t, "gpt-5.4", result.Model)
	require.InEpsilon(t, 0.0693, result.CurrentRatio, 0.000001)
	require.InEpsilon(t, 0.069, result.MinRatio7d, 0.000001)
	require.InEpsilon(t, 0.069, result.MaxRatio7d, 0.000001)
	require.Equal(t, modelGatewayDynamicBillingPricePerMillion("gpt-5.4", 0.0693), result.DisplayPricePerM)
	require.EqualValues(t, 24, result.UpdatedSecondsAgo)

	payload, err := common.Marshal(result)
	require.NoError(t, err)
	payloadText := strings.ToLower(string(payload))
	require.NotContains(t, payloadText, "cost")
	require.NotContains(t, payloadText, "profit")
	require.NotContains(t, payloadText, "channel")
}

func TestPublicHomeDynamicBillingTrendAggregatesDisplayBuckets(t *testing.T) {
	ratio_setting.InitRatioSettings()
	db := setupModelGatewayReplayControllerTestDB(t)
	nowTime := time.Date(2026, 5, 29, 12, 30, 0, 0, time.Local)
	now := nowTime.Unix()
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingMinSamples:     1,
		DynamicBillingRefreshSeconds: 30,
		DynamicBillingMaxAgeSeconds:  7 * 24 * 3600,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
				CandidateGroups:  []string{"codex-plus"},
			},
		},
	})
	defer restoreSetting()

	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-5.4:codex-plus": {
			RequestedModel: "gpt-5.4",
			Group:          "codex-plus",
			Ratio:          0.0693,
			PricePerM:      0.128,
			SampleCount:    8,
			CalculatedAt:   now - 24,
		},
	})
	defer restoreBaselines()

	today10 := time.Date(2026, 5, 29, 10, 20, 0, 0, time.Local).Unix()
	today11 := time.Date(2026, 5, 29, 11, 10, 0, 0, time.Local).Unix()
	yesterday18 := time.Date(2026, 5, 28, 18, 15, 0, 0, time.Local).Unix()
	oldDay := time.Date(2026, 5, 24, 9, 0, 0, 0, time.Local).Unix()
	require.NoError(t, db.Create(&[]model.ModelGatewayUserRequestSummary{
		{
			RequestId:      "req-trend-today-a",
			CreatedAt:      today10,
			CompletedAt:    today10,
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			FinalSuccess:   true,
		},
		{
			RequestId:      "req-trend-today-b",
			CreatedAt:      today11,
			CompletedAt:    today11,
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			FinalSuccess:   true,
		},
		{
			RequestId:      "req-trend-yesterday",
			CreatedAt:      yesterday18,
			CompletedAt:    yesterday18,
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			FinalSuccess:   true,
		},
		{
			RequestId:      "req-trend-old",
			CreatedAt:      oldDay,
			CompletedAt:    oldDay,
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			FinalSuccess:   true,
		},
		{
			RequestId:      "req-trend-probe",
			CreatedAt:      today10,
			CompletedAt:    today10,
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			FinalSuccess:   true,
			IsHealthProbe:  true,
		},
	}).Error)

	makeOther := func(ratio float64, extra map[string]any) string {
		payload := map[string]any{
			"dynamic_billing_applied":     true,
			"dynamic_billing_group":       "codex-plus",
			"dynamic_billing_ratio":       ratio,
			"dynamic_billing_price_per_m": ratio * 2,
		}
		for key, value := range extra {
			payload[key] = value
		}
		data, err := common.Marshal(payload)
		require.NoError(t, err)
		return string(data)
	}
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-trend-today-a",
			CreatedAt: today10,
			ModelName: "gpt-5.4",
			Group:     "codex-plus",
			Other:     makeOther(0.05, nil),
		},
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-trend-today-b",
			CreatedAt: today11,
			ModelName: "gpt-5.4",
			Group:     "codex-plus",
			Other:     makeOther(0.07, nil),
		},
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-trend-yesterday",
			CreatedAt: yesterday18,
			ModelName: "gpt-5.4",
			Group:     "codex-plus",
			Other:     makeOther(0.09, nil),
		},
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-trend-old",
			CreatedAt: oldDay,
			ModelName: "gpt-5.4",
			Group:     "codex-plus",
			Other:     makeOther(0.11, nil),
		},
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-trend-probe",
			CreatedAt: today10,
			ModelName: "gpt-5.4",
			Group:     "codex-plus",
			Other:     makeOther(0.5, map[string]any{"is_health_probe": true}),
		},
	}).Error)

	result := buildPublicHomeDynamicBilling(now)

	require.NotNil(t, result)
	require.NotNil(t, result.Trend)
	require.Len(t, result.Trend.Today.Points, 24)
	require.Len(t, result.Trend.Yesterday.Points, 24)
	require.Len(t, result.Trend.SevenDays.Points, 7)
	require.EqualValues(t, 2, result.Trend.Today.SampleCount)
	require.InEpsilon(t, 0.05, result.Trend.Today.MinRatio, 0.000001)
	require.InEpsilon(t, 0.07, result.Trend.Today.MaxRatio, 0.000001)
	require.InEpsilon(t, 0.06, result.Trend.Today.AvgRatio, 0.000001)
	require.InEpsilon(t, 0.09, result.Trend.Yesterday.LatestRatio, 0.000001)
	require.EqualValues(t, 4, result.Trend.SevenDays.SampleCount)

	payload, err := common.Marshal(result.Trend)
	require.NoError(t, err)
	payloadText := strings.ToLower(string(payload))
	require.NotContains(t, payloadText, "cost")
	require.NotContains(t, payloadText, "profit")
	require.NotContains(t, payloadText, "channel")
}
