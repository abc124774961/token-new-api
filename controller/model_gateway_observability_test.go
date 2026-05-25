package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelGatewayObservabilityAPIResponse struct {
	Success bool                              `json:"success"`
	Message string                            `json:"message"`
	Data    ModelGatewayObservabilityResponse `json:"data"`
}

type modelGatewayTrendExportAPIResponse struct {
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
	Data    ModelGatewayTrendExport `json:"data"`
}

type modelGatewayStickyStoreAPIResponse struct {
	Success bool                            `json:"success"`
	Message string                          `json:"message"`
	Data    ModelGatewayStickyStoreResponse `json:"data"`
}

type modelGatewayStickyClearAPIResponse struct {
	Success bool                            `json:"success"`
	Message string                          `json:"message"`
	Data    ModelGatewayStickyClearResponse `json:"data"`
}

type modelGatewayScoreHistoryAPIResponse struct {
	Success bool                             `json:"success"`
	Message string                           `json:"message"`
	Data    ModelGatewayScoreHistoryResponse `json:"data"`
}

func TestGetModelGatewayObservabilitySummaryAggregatesRecentRecords(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	scoreA, err := common.Marshal(map[string]float64{"success": 0.9, "speed": 0.8})
	require.NoError(t, err)
	scoreB, err := common.Marshal(map[string]float64{"success": 0.3, "speed": 0.4})
	require.NoError(t, err)
	candidates, err := common.Marshal([]string{"default", "vip"})
	require.NoError(t, err)
	requestMeta, err := common.Marshal(map[string]any{
		"provider_profile": "mimo_codex_chat",
		"proxy_mode":       "responses_via_chat",
		"queue_enabled":    true,
		"queue_wait_ms":    640,
		"queue_depth":      2,
		"queue_capacity":   8,
		"sticky_source":    "prompt_cache_key",
		"sticky_retained":  true,
		"cache_affinity":   true,
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:       101,
				ChannelName:     "primary",
				Group:           "vip",
				UpstreamModel:   "gpt-5.5",
				ProviderProfile: "openai_codex",
				ProxyMode:       "native",
				RuntimeKey: core.RuntimeKey{
					RequestedModel:        "gpt-5.5",
					UpstreamModel:         "gpt-5.5",
					ChannelID:             101,
					Group:                 "vip",
					EndpointType:          constant.EndpointTypeOpenAI,
					CapabilityFingerprint: "openai_codex",
				},
				Available:     false,
				RejectReason:  "circuit_open",
				StickyMatched: true,
			},
			{
				ChannelID:       201,
				ChannelName:     "primary-actual",
				Group:           "vip",
				UpstreamModel:   "mimo-v1",
				ProviderProfile: "mimo_codex_chat",
				ProxyMode:       "responses_via_chat",
				RuntimeKey: core.RuntimeKey{
					RequestedModel:        "gpt-5.5",
					UpstreamModel:         "mimo-v1",
					ChannelID:             201,
					Group:                 "vip",
					EndpointType:          constant.EndpointTypeOpenAI,
					CapabilityFingerprint: "mimo_codex_chat",
				},
				Available:      true,
				ScoreTotal:     0.81234,
				ScoreBreakdown: map[string]float64{"success": 0.9, "speed": 0.8},
				Selected:       true,
			},
		},
	})
	require.NoError(t, err)
	requestMetaBrokenSticky, err := common.Marshal(map[string]any{
		"queue_enabled":  true,
		"sticky_source":  "user_sticky",
		"sticky_break":   "score_below_threshold",
		"cache_affinity": false,
	})
	require.NoError(t, err)

	records := []model.ModelExecutionRecord{
		{
			CreatedAt:       now - 20,
			RequestId:       "req-1",
			RequestedGroup:  "auto",
			SelectedGroup:   "vip",
			ActualGroup:     "vip",
			RequestedModel:  "gpt-5.5",
			ChannelId:       101,
			ChannelName:     "primary",
			ActualChannelId: 201,
			EndpointType:    "chat",
			PolicyMode:      "active",
			AutoMode:        "auto_fusion",
			Strategy:        "balanced",
			SmartHandled:    true,
			ScoreTotal:      0.8,
			ScoreBreakdown:  string(scoreA),
			CandidateGroups: string(candidates),
			SelectedReason:  "weighted_score",
			RequestMeta:     string(requestMeta),
		},
		{
			CreatedAt:      now - 18,
			RequestId:      "req-1",
			AttemptIndex:   0,
			RequestedGroup: "auto",
			SelectedGroup:  "vip",
			RequestedModel: "gpt-5.5",
			ChannelId:      201,
			ChannelName:    "primary-actual",
			Success:        true,
			DurationMs:     1200,
			TTFTMs:         240,
		},
		{
			CreatedAt:         now - 16,
			RequestId:         "req-2",
			AttemptIndex:      0,
			RequestedGroup:    "default",
			SelectedGroup:     "default",
			RequestedModel:    "claude-4",
			ChannelId:         202,
			ChannelName:       "fallback",
			Success:           false,
			StatusCode:        http.StatusBadGateway,
			ErrorCode:         "bad_response_status_code",
			ErrorType:         "upstream_error",
			DurationMs:        2200,
			TTFTMs:            500,
			StreamInterrupted: true,
			RequestMeta:       `{"error_category":"stream_interrupted","retry_action":"stop","error_message":"upstream stream interrupted"}`,
		},
		{
			CreatedAt:      now - 14,
			RequestId:      "req-3",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "claude-4",
			ChannelId:      202,
			ChannelName:    "fallback",
			PolicyMode:     "active",
			SmartHandled:   true,
			FallbackUsed:   true,
			ScoreTotal:     0.4,
			ScoreBreakdown: string(scoreB),
			RequestMeta:    string(requestMetaBrokenSticky),
		},
		{
			CreatedAt:      now - int64(48*3600),
			RequestId:      "old",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "old-model",
			ChannelId:      999,
			Success:        true,
			DurationMs:     100,
		},
	}
	require.NoError(t, db.Create(&records).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?hours=24&recent_limit=4&top_n=5", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayObservabilityResponse(t, resp)
	require.True(t, payload.Success)
	require.Equal(t, int64(4), payload.Data.Summary.TotalRecords)
	require.Equal(t, 4, payload.Data.Summary.ScannedRecords)
	require.Equal(t, int64(2), payload.Data.Summary.Dispatches)
	require.Equal(t, int64(2), payload.Data.Summary.Attempts)
	require.Equal(t, int64(1), payload.Data.Summary.Successes)
	require.Equal(t, int64(1), payload.Data.Summary.Failures)
	require.Equal(t, int64(1), payload.Data.Summary.StreamInterrupted)
	require.Equal(t, int64(1), payload.Data.Summary.FallbackUsed)
	require.Equal(t, int64(2), payload.Data.Summary.QueueEnabledDispatches)
	require.Equal(t, int64(1), payload.Data.Summary.QueuedDispatches)
	require.Equal(t, int64(640), payload.Data.Summary.AvgQueueWaitMs)
	require.Equal(t, int64(2), payload.Data.Summary.StickyRoutes)
	require.Equal(t, int64(1), payload.Data.Summary.StickyRetained)
	require.Equal(t, int64(1), payload.Data.Summary.StickyBroken)
	require.Equal(t, int64(1), payload.Data.Summary.CacheAffinityRoutes)
	requireTrendRejectReason(t, payload.Data.Summary.CircuitErrorTypes, scheduler.CircuitErrorStreamInterrupted, 1)
	requireTrendRejectReason(t, payload.Data.Summary.CircuitErrorCounts, scheduler.CircuitErrorStreamInterrupted, 1)
	require.GreaterOrEqual(t, payload.Data.Summary.RiskEvents, int64(1))
	require.GreaterOrEqual(t, payload.Data.Summary.RiskStatusChanges, int64(1))
	require.Equal(t, payload.Data.Summary.RiskEvents, payload.Data.Risk.EventCount)
	require.Equal(t, payload.Data.Summary.RiskEvents, payload.Data.Risk.RiskEventCount)
	require.Equal(t, payload.Data.Summary.RiskStatusChanges, payload.Data.Risk.StatusChanges)
	require.NotEmpty(t, payload.Data.RiskEvents)
	require.NotEmpty(t, payload.Data.Risk.Events)
	require.NotEmpty(t, payload.Data.Risk.RiskEvents)
	require.NotEmpty(t, payload.Data.Risk.RiskTimeline)
	require.Equal(t, 0.5, payload.Data.Summary.SuccessRate)
	require.Equal(t, int64(1700), payload.Data.Summary.AvgDurationMs)
	require.Equal(t, int64(370), payload.Data.Summary.AvgTTFTMs)
	require.Equal(t, 0.6, payload.Data.Summary.AvgScoreTotal)
	require.Equal(t, int64(2), payload.Data.ScoreBreakdown.Samples)
	require.Equal(t, 0.6, payload.Data.ScoreBreakdown.Average["success"])
	require.Equal(t, 0.6, payload.Data.ScoreBreakdown.Average["speed"])
	require.Len(t, payload.Data.RecentRecords, 4)
	require.Equal(t, "dispatch", payload.Data.RecentRecords[0].Kind)
	require.Equal(t, "attempt", payload.Data.RecentRecords[1].Kind)
	require.Equal(t, "stream_interrupted", payload.Data.RecentRecords[1].ErrorCategory)
	require.Equal(t, "stop", payload.Data.RecentRecords[1].RetryAction)
	require.Equal(t, "upstream stream interrupted", payload.Data.RecentRecords[1].ErrorMessage)
	require.Equal(t, []string{"default", "vip"}, payload.Data.RecentRecords[3].CandidateGroups)
	require.Equal(t, "mimo_codex_chat", payload.Data.RecentRecords[3].RequestMeta["provider_profile"])
	require.Equal(t, "responses_via_chat", payload.Data.RecentRecords[3].RequestMeta["proxy_mode"])
	require.Equal(t, true, payload.Data.RecentRecords[3].RequestMeta["queue_enabled"])
	require.True(t, payload.Data.RecentRecords[3].QueueEnabled)
	require.Equal(t, int64(640), payload.Data.RecentRecords[3].QueueWaitMs)
	require.Equal(t, 2, payload.Data.RecentRecords[3].QueueDepth)
	require.Equal(t, 8, payload.Data.RecentRecords[3].QueueCapacity)
	require.Equal(t, "prompt_cache_key", payload.Data.RecentRecords[3].StickySource)
	require.True(t, payload.Data.RecentRecords[3].StickyRetained)
	require.Empty(t, payload.Data.RecentRecords[3].StickyBreak)
	require.True(t, payload.Data.RecentRecords[3].CacheAffinity)
	require.Len(t, payload.Data.RecentRecords[3].CandidateExplanations, 2)
	require.False(t, payload.Data.RecentRecords[3].CandidateExplanations[0].Available)
	require.Equal(t, "circuit_open", payload.Data.RecentRecords[3].CandidateExplanations[0].RejectReason)
	require.True(t, payload.Data.RecentRecords[3].CandidateExplanations[0].StickyMatched)
	require.True(t, payload.Data.RecentRecords[3].CandidateExplanations[1].Available)
	require.True(t, payload.Data.RecentRecords[3].CandidateExplanations[1].Selected)
	require.Equal(t, "mimo_codex_chat", payload.Data.RecentRecords[3].CandidateExplanations[1].ProviderProfile)
	require.Equal(t, "mimo-v1", payload.Data.RecentRecords[3].CandidateExplanations[1].RuntimeKey.UpstreamModel)
	require.Equal(t, 0.8123, payload.Data.RecentRecords[3].CandidateExplanations[1].ScoreTotal)
	require.Equal(t, 0.9, payload.Data.RecentRecords[3].CandidateExplanations[1].ScoreBreakdown["success"])
	require.Equal(t, "score_below_threshold", payload.Data.RecentRecords[0].StickyBreak)
	require.Len(t, payload.Data.Trends, 4)
	trend := requireModelGatewayTrendWithRecords(t, payload.Data.Trends, 4)
	require.Equal(t, int64(2), trend.Dispatches)
	require.Equal(t, int64(2), trend.Attempts)
	require.Equal(t, int64(1), trend.Successes)
	require.Equal(t, int64(1), trend.Failures)
	require.Equal(t, int64(1), trend.StreamInterrupted)
	require.Equal(t, int64(1), trend.FallbackUsed)
	require.Equal(t, 0.5, trend.SuccessRate)
	require.Equal(t, int64(1700), trend.AvgDurationMs)
	require.Equal(t, int64(370), trend.AvgTTFTMs)
	require.Equal(t, int64(2), trend.QueueEnabledDispatches)
	require.Equal(t, int64(1), trend.QueuedDispatches)
	require.Equal(t, int64(640), trend.AvgQueueWaitMs)
	require.Equal(t, int64(640), trend.QueueWaitP50Ms)
	require.Equal(t, int64(640), trend.QueueWaitP90Ms)
	require.Equal(t, int64(640), trend.QueueWaitP95Ms)
	require.Equal(t, int64(2), trend.StickyRoutes)
	require.Equal(t, int64(1), trend.StickyRetained)
	require.Equal(t, int64(1), trend.StickyBroken)
	require.Equal(t, int64(1), trend.CacheAffinityRoutes)
	requireAggregate(t, trend.ByProviderProfile, "mimo_codex_chat", 1, 1, 0)
	requireAggregate(t, trend.ByProviderProfile, "unknown", 1, 0, 1)
	requireAggregate(t, trend.ByProxyMode, "responses_via_chat", 1, 1, 0)
	requireAggregate(t, trend.ByProxyMode, "unknown", 1, 0, 1)
	requireTrendRejectReason(t, trend.RejectReasons, "circuit_open", 1)
	requireTrendRejectReason(t, trend.CircuitErrorTypes, scheduler.CircuitErrorStreamInterrupted, 1)
	requireTrendRejectReason(t, trend.CircuitErrorCounts, scheduler.CircuitErrorStreamInterrupted, 1)
	requireRiskEvent(t, trend.RiskEvents, "trend_bucket", "stream_interrupted", "stream_interrupted", "")
	requireRiskEvent(t, trend.RiskEvents, "trend_bucket", "circuit_error_type", "stream_interrupted", scheduler.CircuitErrorStreamInterrupted)
	requireRiskEvent(t, trend.RiskEvents, "trend_bucket", "fallback_used", "fallback", "")
	requireRiskEvent(t, trend.RiskEvents, "trend_bucket", "reject_reason", "circuit_open", "circuit_open")
	require.NotNil(t, trend.Risk)
	require.GreaterOrEqual(t, trend.Risk.EventCount, int64(1))
	requireTrendRejectReason(t, trend.Risk.TopCircuitErrorTypes, scheduler.CircuitErrorStreamInterrupted, 1)
	requireRiskEvent(t, trend.Risk.RiskEvents, "trend_bucket", "reject_reason", "circuit_open", "circuit_open")
	requireRiskEvent(t, payload.Data.RiskTimeline, "trend_bucket", "stream_interrupted", "stream_interrupted", "")
	requireTrendRejectReason(t, payload.Data.Risk.TopCircuitErrorTypes, scheduler.CircuitErrorStreamInterrupted, 1)

	requireAggregate(t, payload.Data.ByModel, "gpt-5.5", 1, 1, 0)
	requireAggregate(t, payload.Data.ByModel, "claude-4", 1, 0, 1)
	requireAggregate(t, payload.Data.ByGroup, "vip", 1, 1, 0)
	requireAggregate(t, payload.Data.ByGroup, "default", 1, 0, 1)
	requireAggregate(t, payload.Data.ByChannel, "201", 1, 1, 0)
	requireAggregate(t, payload.Data.ByChannel, "202", 1, 0, 1)
	requireAggregate(t, payload.Data.ByProviderProfile, "mimo_codex_chat", 1, 1, 0)
	requireAggregate(t, payload.Data.ByProviderProfile, "unknown", 1, 0, 1)
	requireAggregate(t, payload.Data.ByProxyMode, "responses_via_chat", 1, 1, 0)
	requireAggregate(t, payload.Data.ByProxyMode, "unknown", 1, 0, 1)
	requireObservabilityMetaAggregate(t, payload.Data.ByChannel, "201", 1, 1, 640, 1, 1, 0, 1)
	requireObservabilityMetaAggregate(t, payload.Data.ByChannel, "202", 1, 0, 0, 1, 0, 1, 0)
	requireObservabilityMetaAggregate(t, payload.Data.ByProviderProfile, "mimo_codex_chat", 1, 1, 640, 1, 1, 0, 1)
	requireObservabilityMetaAggregate(t, payload.Data.ByProxyMode, "responses_via_chat", 1, 1, 640, 1, 1, 0, 1)
}

func TestModelGatewayObservabilityNormalizesLowSampleLegacySpeedScores(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	requestMeta, err := common.Marshal(map[string]any{
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:   11,
				ChannelName: "qicun",
				Group:       "codex-plus",
				RuntimeKey: core.RuntimeKey{
					RequestedModel: "gpt-5.5",
					UpstreamModel:  "gpt-5.5",
					ChannelID:      11,
					Group:          "codex-plus",
					EndpointType:   constant.EndpointTypeOpenAIResponse,
				},
				Available:      true,
				ScoreTotal:     0.52,
				ScoreBreakdown: map[string]float64{"success": 1, "speed": 0.03, "cost": 0.782, "group": 0.9},
				SuccessScore:   1,
				SpeedScore:     0.038,
				TTFTMs:         19260,
				DurationMs:     26410,
				SampleCount:    1,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 10,
		RequestId:      "low-sample-speed",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		ChannelId:      4,
		ChannelName:    "toioto",
		ScoreTotal:     0.65,
		SmartHandled:   true,
		PolicyMode:     "active",
		RequestMeta:    string(requestMeta),
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?hours=1&recent_limit=1&top_n=5&scan_limit=10", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayObservabilityResponse(t, resp)
	require.True(t, payload.Success)
	require.Len(t, payload.Data.RecentRecords, 1)
	require.Len(t, payload.Data.RecentRecords[0].CandidateExplanations, 1)
	candidate := payload.Data.RecentRecords[0].CandidateExplanations[0]
	require.Equal(t, 1, candidate.SampleCount)
	require.Equal(t, 19260.0, candidate.TTFTMs)
	require.InEpsilon(t, 0.4476, candidate.SpeedScore, 0.0002)
	require.InEpsilon(t, 0.4166, candidate.ScoreSpeedFactor, 0.0002)
	require.Equal(t, candidate.ScoreSpeedFactor, candidate.ScoreBreakdown["speed"])
}

func TestGetModelGatewayObservabilitySummaryTreatsPending429AsConcurrencyFlow(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 1,
		RequestId:      "req-pending",
		AttemptIndex:   0,
		RequestedGroup: "auto",
		SelectedGroup:  "vip",
		RequestedModel: "gpt-5.5",
		ChannelId:      8,
		ChannelName:    "pending-channel",
		Success:        false,
		StatusCode:     http.StatusTooManyRequests,
		ErrorCode:      string(types.ErrorCodeBadResponseStatusCode),
		ErrorType:      string(types.ErrorTypeOpenAIError),
		DurationMs:     900,
		RequestMeta: `{
			"error_message":"Too many pending requests, please retry later",
			"error_category":"upstream_concurrency_limit",
			"retry_action":"switch_channel",
			"will_retry":true,
			"concurrency_limited":true,
			"active_concurrency":47,
			"configured_concurrency_limit":64,
			"learned_concurrency_limit":46,
			"learned_concurrency_limit_changed":true,
			"used_channels":["8"]
		}`,
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 1,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.RecentRecords, 1)
	record := response.RecentRecords[0]
	require.Equal(t, "upstream_concurrency_limit", record.ErrorCategory)
	require.Equal(t, "switch_channel", record.RetryAction)
	require.True(t, record.WillRetry)
	require.True(t, record.ConcurrencyLimited)
	require.Equal(t, 47, record.ActiveConcurrency)
	require.Equal(t, 64, record.ConfiguredConcurrencyLimit)
	require.Equal(t, 46, record.LearnedConcurrencyLimit)
	require.True(t, record.LearnedConcurrencyLimitChanged)
	require.Equal(t, []string{"8"}, record.UsedChannels)
	require.Empty(t, response.Summary.CircuitErrorTypes)
	require.Empty(t, response.Summary.CircuitErrorCounts)
}

func TestBuildModelGatewayObservabilitySummaryExposesEngineeringErrorMetrics(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	dispatchMeta, err := common.Marshal(map[string]any{
		"queue_enabled": true,
		"queue_wait_ms": 250,
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:             12,
				ChannelName:           "isolated",
				Group:                 "auto",
				UpstreamModel:         "gpt-5.5",
				Available:             false,
				RejectReason:          "config_error_isolated",
				ConfigErrorIsolated:   true,
				IsolationReason:       core.ErrorCategoryAuthConfigError,
				IsolationUntil:        now + 3600,
				AuthConfigErrorCount:  2,
				LastAuthConfigErrorAt: now - 60,
				RuntimeKey: core.RuntimeKey{
					RequestedModel:        "gpt-5.5",
					UpstreamModel:         "gpt-5.5",
					ChannelID:             12,
					Group:                 "auto",
					EndpointType:          constant.EndpointTypeOpenAI,
					CapabilityFingerprint: "openai_codex",
				},
			},
		},
	})
	require.NoError(t, err)
	records := []model.ModelExecutionRecord{
		{
			CreatedAt:      now - 5,
			RequestId:      "engineering-dispatch",
			RequestedGroup: "auto",
			SelectedGroup:  "auto",
			RequestedModel: "gpt-5.5",
			ChannelId:      12,
			SmartHandled:   true,
			PolicyMode:     "active",
			RequestMeta:    string(dispatchMeta),
		},
		{
			CreatedAt:      now - 4,
			RequestId:      "engineering-overload",
			AttemptIndex:   0,
			RequestedGroup: "auto",
			SelectedGroup:  "auto",
			RequestedModel: "gpt-5.5",
			ChannelId:      12,
			StatusCode:     http.StatusTooManyRequests,
			ErrorCategory:  core.ErrorCategoryOverloadSkip,
			DurationMs:     100,
			RequestMeta:    `{"error_category":"overload_skip","retry_action":"switch_channel","concurrency_limited":true}`,
		},
		{
			CreatedAt:      now - 3,
			RequestId:      "engineering-auth",
			AttemptIndex:   0,
			RequestedGroup: "auto",
			SelectedGroup:  "auto",
			RequestedModel: "gpt-5.5",
			ChannelId:      13,
			StatusCode:     http.StatusUnauthorized,
			ErrorCategory:  core.ErrorCategoryAuthConfigError,
			DurationMs:     120,
			RequestMeta:    `{"error_category":"auth_config_error","retry_action":"switch_channel"}`,
		},
		{
			CreatedAt:      now - 2,
			RequestId:      "engineering-unknown",
			AttemptIndex:   0,
			RequestedGroup: "auto",
			SelectedGroup:  "auto",
			RequestedModel: "gpt-5.5",
			ChannelId:      14,
			StatusCode:     http.StatusBadGateway,
			ErrorCategory:  core.ErrorCategoryUnknown,
			DurationMs:     140,
			RequestMeta:    `{"error_category":"unknown","retry_action":"stop"}`,
		},
	}
	require.NoError(t, db.Create(&records).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:              1,
		RecentLimit:        10,
		TopN:               10,
		ScanLimit:          10,
		TrendBucketSeconds: 3600,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), response.Summary.QueueWaitCount)
	require.Equal(t, int64(1), response.Summary.QueuedDispatches)
	require.Equal(t, int64(250), response.Summary.AvgQueueWaitMs)
	require.Equal(t, int64(1), response.Summary.OverloadSkipCount)
	require.Equal(t, int64(1), response.Summary.AuthConfigErrorCount)
	require.Equal(t, int64(1), response.Summary.UnknownErrorCount)
	require.Equal(t, int64(1), response.Summary.ConfigErrorIsolatedCount)

	channel := requireAggregate(t, response.ByChannel, "12", 1, 0, 1)
	require.Equal(t, int64(1), channel.QueueWaitCount)
	require.Equal(t, int64(1), channel.OverloadSkipCount)
	require.Equal(t, int64(1), channel.ConfigErrorIsolatedCount)

	trend := requireModelGatewayTrendWithRecords(t, response.Trends, 4)
	require.Equal(t, int64(1), trend.QueueWaitCount)
	require.Equal(t, int64(1), trend.OverloadSkipCount)
	require.Equal(t, int64(1), trend.AuthConfigErrorCount)
	require.Equal(t, int64(1), trend.UnknownErrorCount)
	require.Equal(t, int64(1), trend.ConfigErrorIsolatedCount)
	require.Len(t, response.RecentRecords, 4)
	require.True(t, response.RecentRecords[3].CandidateExplanations[0].ConfigErrorIsolated)
	require.Equal(t, core.ErrorCategoryAuthConfigError, response.RecentRecords[3].CandidateExplanations[0].IsolationReason)
	require.Equal(t, now+3600, response.RecentRecords[3].CandidateExplanations[0].IsolationUntil)
}

func TestBuildModelGatewayObservabilitySummaryExposesHealthProbeMarker(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 1,
		RequestId:      "mg_probe_test",
		AttemptIndex:   0,
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-4.1",
		ChannelId:      8,
		ChannelName:    "probe-channel",
		Success:        true,
		StatusCode:     http.StatusOK,
		DurationMs:     320,
		TTFTMs:         80,
		RequestMeta: `{
			"is_health_probe":true,
			"probe_reason":"low_score"
		}`,
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 1,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.RecentRecords, 1)
	require.True(t, response.RecentRecords[0].IsHealthProbe)
	require.Equal(t, "low_score", response.RecentRecords[0].ProbeReason)
	require.Empty(t, response.UserRequests.RecentRequests)
}

func TestModelGatewayObservabilityOverlaysCurrentBalanceStatus(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	channel := model.Channel{
		Id:     201,
		Name:   "balance-paused",
		Status: common.ChannelStatusAutoDisabled,
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": "balance_insufficient",
	})
	require.NoError(t, db.Create(&channel).Error)
	requestMeta, err := common.Marshal(map[string]any{
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:   201,
				ChannelName: "balance-paused",
				RuntimeKey: core.RuntimeKey{
					ChannelID:     201,
					Group:         "default",
					UpstreamModel: "gpt-5.5",
				},
				Available:  true,
				ScoreTotal: 0.9,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create([]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 10,
			RequestId:      "balance-dispatch",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      201,
			ChannelName:    "balance-paused",
			PolicyMode:     "active",
			SmartHandled:   true,
			RequestMeta:    string(requestMeta),
		},
		{
			CreatedAt:      now - 9,
			RequestId:      "balance-dispatch",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      201,
			ChannelName:    "balance-paused",
			Success:        true,
			DurationMs:     100,
		},
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 2,
		TopN:        5,
		ScanLimit:   20,
	})

	require.NoError(t, err)
	require.Len(t, response.RecentRecords, 2)
	require.Len(t, response.RecentRecords[1].CandidateExplanations, 1)
	candidate := response.RecentRecords[1].CandidateExplanations[0]
	require.False(t, candidate.Available)
	require.True(t, candidate.BalanceInsufficient)
	require.Equal(t, "balance_insufficient", candidate.StatusReason)
	require.Equal(t, "balance_insufficient", candidate.RejectReason)
	aggregate := requireAggregate(t, response.ByChannel, "201", 1, 1, 0)
	require.True(t, aggregate.BalanceInsufficient)
	require.Equal(t, "balance_insufficient", aggregate.StatusReason)
}

func TestModelGatewayObservabilityOverlaysConfirmedZeroBalanceStatus(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	channel := model.Channel{
		Id:                 202,
		Name:               "zero-balance",
		Status:             common.ChannelStatusEnabled,
		Balance:            0,
		BalanceUpdatedTime: now,
	}
	require.NoError(t, db.Create(&channel).Error)
	requestMeta, err := common.Marshal(map[string]any{
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:   202,
				ChannelName: "zero-balance",
				RuntimeKey: core.RuntimeKey{
					ChannelID:     202,
					Group:         "default",
					UpstreamModel: "gpt-5.5",
				},
				Available:  true,
				ScoreTotal: 0.9,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create([]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 10,
			RequestId:      "zero-balance-dispatch",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      202,
			ChannelName:    "zero-balance",
			PolicyMode:     "active",
			SmartHandled:   true,
			RequestMeta:    string(requestMeta),
		},
		{
			CreatedAt:      now - 9,
			RequestId:      "zero-balance-dispatch",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      202,
			ChannelName:    "zero-balance",
			Success:        true,
			DurationMs:     100,
		},
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 2,
		TopN:        5,
		ScanLimit:   20,
	})

	require.NoError(t, err)
	require.Len(t, response.RecentRecords, 2)
	require.Len(t, response.RecentRecords[1].CandidateExplanations, 1)
	candidate := response.RecentRecords[1].CandidateExplanations[0]
	require.False(t, candidate.Available)
	require.True(t, candidate.BalanceInsufficient)
	require.Equal(t, "balance_insufficient", candidate.StatusReason)
	require.Equal(t, "balance_insufficient", candidate.RejectReason)
	aggregate := requireAggregate(t, response.ByChannel, "202", 1, 1, 0)
	require.True(t, aggregate.BalanceInsufficient)
	require.Equal(t, "balance_insufficient", aggregate.StatusReason)
}

func TestGetModelGatewayObservabilitySummaryIgnoresClientAbortHealthSample(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 2,
			RequestId:      "req-ok",
			AttemptIndex:   0,
			RequestedGroup: "auto",
			SelectedGroup:  "vip",
			RequestedModel: "gpt-5.5",
			ChannelId:      201,
			ChannelName:    "primary",
			Success:        true,
			DurationMs:     1000,
		},
		{
			CreatedAt:         now - 1,
			RequestId:         "req-client-abort",
			AttemptIndex:      0,
			RequestedGroup:    "auto",
			SelectedGroup:     "vip",
			RequestedModel:    "gpt-5.5",
			ChannelId:         201,
			ChannelName:       "primary",
			StatusCode:        499,
			DurationMs:        800,
			StreamInterrupted: true,
			RequestMeta:       `{"client_aborted":true,"error_category":"client_aborted","retry_action":"client_aborted"}`,
		},
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), response.Summary.TotalRecords)
	require.Equal(t, int64(1), response.Summary.Attempts)
	require.Equal(t, int64(1), response.Summary.Successes)
	require.Zero(t, response.Summary.Failures)
	require.Zero(t, response.Summary.StreamInterrupted)
	require.Equal(t, 1.0, response.Summary.SuccessRate)
	requireAggregate(t, response.ByChannel, "201", 1, 1, 0)
	require.Len(t, response.RecentRecords, 2)
	require.True(t, response.RecentRecords[0].ClientAborted)
	require.Empty(t, response.Summary.CircuitErrorTypes)
}

func TestBuildModelGatewayObservabilitySummaryIncludesUserRequests(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"auto":0.25,"default":1,"vip":0.8}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&[]model.ModelGatewayUserRequestSummary{
		{
			CreatedAt:        now - 70,
			UpdatedAt:        now - 65,
			CompletedAt:      now - 60,
			RequestId:        "req-user-success",
			RequestedGroup:   "auto",
			SelectedGroup:    "vip",
			RequestedModel:   "gpt-5.5",
			FinalChannelID:   21,
			FinalChannelName: "healthy-channel",
			Attempts:         2,
			LastAttemptIndex: 1,
			FinalSuccess:     true,
			Recovered:        true,
			DurationMs:       1200,
			TTFTMs:           180,
		},
		{
			CreatedAt:          now - 55,
			UpdatedAt:          now - 50,
			CompletedAt:        now - 45,
			RequestId:          "req-user-failed",
			RequestedGroup:     "auto",
			SelectedGroup:      "vip",
			RequestedModel:     "gpt-5.5",
			Attempts:           2,
			LastAttemptIndex:   1,
			FinalSuccess:       false,
			FinalStatusCode:    http.StatusBadGateway,
			FinalErrorCategory: model.ModelGatewayUserRequestErrorUpstream,
			DurationMs:         2400,
			TTFTMs:             320,
		},
		{
			CreatedAt:        now - 40,
			UpdatedAt:        now - 35,
			CompletedAt:      now - 30,
			RequestId:        "req-user-other",
			RequestedGroup:   "default",
			SelectedGroup:    "default",
			RequestedModel:   "claude-4",
			Attempts:         1,
			LastAttemptIndex: 0,
			FinalSuccess:     true,
			DurationMs:       3600,
			TTFTMs:           520,
		},
		{
			CreatedAt:          now - 20,
			UpdatedAt:          now - 18,
			CompletedAt:        0,
			RequestId:          "req-user-pending",
			RequestedGroup:     "auto",
			SelectedGroup:      "vip",
			RequestedModel:     "gpt-5.5",
			Attempts:           1,
			LastAttemptIndex:   0,
			FinalSuccess:       false,
			FinalStatusCode:    0,
			FinalErrorCategory: "",
			DurationMs:         0,
		},
		{
			CreatedAt:          now - 18,
			UpdatedAt:          now - 17,
			CompletedAt:        now - 16,
			RequestId:          "req-user-aborted",
			RequestedGroup:     "auto",
			SelectedGroup:      "vip",
			RequestedModel:     "gpt-5.5",
			Attempts:           1,
			LastAttemptIndex:   0,
			FinalSuccess:       false,
			FinalStatusCode:    499,
			FinalErrorCategory: model.ModelGatewayUserRequestErrorClientAborted,
			ClientAborted:      true,
			StreamInterrupted:  true,
			DurationMs:         900,
		},
	}).Error)
	consumeOther, err := common.Marshal(map[string]any{
		"model_ratio":                 1.25,
		"group_ratio":                 0.8,
		"completion_ratio":            2.0,
		"cache_tokens":                100,
		"cache_ratio":                 0.5,
		"cache_creation_tokens":       40,
		"cache_creation_ratio":        1.25,
		"billing_source":              "subscription_wallet",
		"subscription_consumed":       1200,
		"wallet_quota_deducted":       388,
		"web_search_call_count":       1,
		"web_search_price":            10.0,
		"image_generation_call":       true,
		"image_generation_call_price": 0.011,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Log{
		CreatedAt:        now - 58,
		Type:             model.LogTypeConsume,
		RequestId:        "req-user-success",
		UserId:           1001,
		Username:         "request_user",
		Quota:            1588,
		PromptTokens:     1200,
		CompletionTokens: 320,
		ChannelId:        21,
		Group:            "vip",
		ModelName:        "gpt-5.5",
		Content:          "按量计费",
		Other:            string(consumeOther),
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)

	userRequests := response.UserRequests
	require.Equal(t, int64(4), userRequests.Summary.TotalRequests)
	require.Equal(t, 4, userRequests.Summary.ScannedRequests)
	require.Equal(t, int64(2), userRequests.Summary.Successes)
	require.Equal(t, int64(1), userRequests.Summary.FinalFailures)
	require.Equal(t, int64(1), userRequests.Summary.ClientAborted)
	require.Equal(t, int64(1), userRequests.Summary.Recovered)
	require.Equal(t, 0.6667, userRequests.Summary.UserSuccessRate)
	require.Equal(t, int64(2400), userRequests.Summary.AvgDurationMs)
	require.Equal(t, int64(3600), userRequests.Summary.P95DurationMs)
	require.Equal(t, int64(340), userRequests.Summary.AvgTTFTMs)
	require.Equal(t, int64(520), userRequests.Summary.P95TTFTMs)
	require.Len(t, userRequests.RecentRequests, 4)
	require.Equal(t, "req-user-aborted", userRequests.RecentRequests[0].RequestID)
	require.True(t, userRequests.RecentRequests[0].ClientAborted)
	require.Equal(t, "client_aborted", userRequests.RecentRequests[0].Status)
	require.Equal(t, "req-user-other", userRequests.RecentRequests[1].RequestID)
	require.Equal(t, "default", userRequests.RecentRequests[1].ActualGroup)
	require.Equal(t, 1.0, userRequests.RecentRequests[1].ActualGroupRatio)
	require.Equal(t, model.ModelGatewayUserRequestErrorUpstream, userRequests.RecentRequests[2].FinalErrorCategory)
	require.Empty(t, userRequests.RecentRequests[1].FinalErrorCategory)
	require.Equal(t, 21, userRequests.RecentRequests[3].FinalChannelID)
	require.Equal(t, "healthy-channel", userRequests.RecentRequests[3].FinalChannelName)
	require.Equal(t, 1001, userRequests.RecentRequests[3].UserID)
	require.Equal(t, "request_user", userRequests.RecentRequests[3].Username)
	require.Equal(t, "vip", userRequests.RecentRequests[3].ActualGroup)
	require.Equal(t, 0.8, userRequests.RecentRequests[3].ActualGroupRatio)
	require.NotNil(t, userRequests.RecentRequests[3].Billing)
	require.Equal(t, 1001, userRequests.RecentRequests[3].Billing.UserID)
	require.Equal(t, "request_user", userRequests.RecentRequests[3].Billing.Username)
	require.Equal(t, 1588, userRequests.RecentRequests[3].Billing.Quota)
	require.Equal(t, 1200, userRequests.RecentRequests[3].Billing.PromptTokens)
	require.Equal(t, 320, userRequests.RecentRequests[3].Billing.CompletionTokens)
	require.Equal(t, 1520, userRequests.RecentRequests[3].Billing.TotalTokens)
	require.Equal(t, 1.25, userRequests.RecentRequests[3].Billing.ModelRatio)
	require.Equal(t, 0.8, userRequests.RecentRequests[3].Billing.GroupRatio)
	require.Equal(t, 2.0, userRequests.RecentRequests[3].Billing.CompletionRatio)
	require.Equal(t, 100, userRequests.RecentRequests[3].Billing.CacheTokens)
	require.Equal(t, 1200, userRequests.RecentRequests[3].Billing.SubscriptionConsumed)
	require.Equal(t, 388, userRequests.RecentRequests[3].Billing.WalletQuotaDeducted)
	require.Equal(t, 1, userRequests.RecentRequests[3].Billing.WebSearchCallCount)
	require.Equal(t, "pending", userRequests.RecentRequests[3].UpstreamCostSource)
	require.Equal(t, "pending", userRequests.RecentRequests[3].UpstreamCostAccuracy)
	requireUserRequestAggregate(t, userRequests.ByModel, "gpt-5.5", 3, 1, 1, 1)
	requireUserRequestAggregate(t, userRequests.ByModel, "claude-4", 1, 1, 0, 0)
	requireUserRequestAggregate(t, userRequests.ByGroup, "vip", 3, 1, 1, 1)
	requireUserRequestAggregate(t, userRequests.ByGroup, "default", 1, 1, 0, 0)
	trend := requireModelGatewayUserRequestTrendWithRequests(t, userRequests.Trends, 4)
	require.Equal(t, int64(340), trend.AvgTTFTMs)
	require.Equal(t, int64(520), trend.P95TTFTMs)
}

func TestBuildModelGatewayObservabilitySummaryIncludesHealthProbeUserRequest(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()

	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		CreatedAt:        now - 10,
		UpdatedAt:        now - 8,
		CompletedAt:      now - 5,
		RequestId:        "req-user-probe",
		RequestedGroup:   "auto",
		SelectedGroup:    "vip",
		RequestedModel:   "gpt-5.5",
		FinalChannelID:   31,
		FinalChannelName: "probe-channel",
		Attempts:         1,
		LastAttemptIndex: 0,
		FinalSuccess:     true,
		DurationMs:       900,
		TTFTMs:           140,
	}).Error)

	require.NoError(t, db.Create(&model.Log{
		CreatedAt:        now - 5,
		Type:             model.LogTypeConsume,
		RequestId:        "req-user-probe",
		UserId:           1,
		Username:         "root",
		Quota:            10,
		PromptTokens:     10,
		CompletionTokens: 5,
		ChannelId:        31,
		Group:            "vip",
		ModelName:        "gpt-5.5",
		Content:          "probe",
		Other:            `{"is_health_probe":true,"probe_reason":"low_score","billing_source":"model_gateway_probe"}`,
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
		ViewMode:    "user_requests",
	})
	require.NoError(t, err)
	require.Len(t, response.UserRequests.RecentRequests, 1)
	require.True(t, response.UserRequests.RecentRequests[0].IsHealthProbe)
	require.Equal(t, "low_score", response.UserRequests.RecentRequests[0].ProbeReason)
}

func TestBuildModelGatewayObservabilitySummaryUsesSelectedGroupRatioWhenBillingGroupIsRequestedGroup(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"auto":0.25,"codex-plus":1.6}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		CreatedAt:        now - 20,
		UpdatedAt:        now - 18,
		CompletedAt:      now - 16,
		RequestId:        "req-auto-to-codex-plus",
		RequestedGroup:   "auto",
		SelectedGroup:    "codex-plus",
		RequestedModel:   "gpt-5.5",
		FinalChannelID:   7,
		FinalChannelName: "dora",
		Attempts:         1,
		FinalSuccess:     true,
		DurationMs:       900,
		TTFTMs:           120,
	}).Error)
	consumeOther, err := common.Marshal(map[string]any{
		"group_ratio": 0.25,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Log{
		CreatedAt:        now - 15,
		Type:             model.LogTypeConsume,
		RequestId:        "req-auto-to-codex-plus",
		Quota:            100,
		PromptTokens:     10,
		CompletionTokens: 5,
		ChannelId:        7,
		Group:            "auto",
		ModelName:        "gpt-5.5",
		Other:            string(consumeOther),
	}).Error)
	breakdown, err := common.Marshal(map[string]any{
		"currency": "USD",
		"input": map[string]any{
			"tokens":            10,
			"price_per_million": 0.05,
			"amount":            0.0000005,
		},
		"output": map[string]any{
			"tokens":            5,
			"price_per_million": 0.1,
			"amount":            0.0000005,
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-auto-to-codex-plus",
		ChannelID:         7,
		UpstreamModel:     "gpt-5.5",
		UpstreamCostTotal: 0.000001,
		BreakdownJSON:     string(breakdown),
		CostSource:        "manual",
		CostAccuracy:      "precise",
		CalculatedAt:      now - 14,
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.UserRequests.RecentRequests, 1)
	record := response.UserRequests.RecentRequests[0]
	require.Equal(t, "auto", record.RequestedGroup)
	require.Equal(t, "codex-plus", record.SelectedGroup)
	require.Equal(t, "codex-plus", record.ActualGroup)
	require.Equal(t, 1.6, record.ActualGroupRatio)
	require.Equal(t, 0.000001, record.ActualChannelCost)
	require.Equal(t, 0.000001, record.UpstreamCostTotal)
	require.Equal(t, "manual", record.UpstreamCostSource)
	require.Equal(t, "precise", record.UpstreamCostAccuracy)
	require.NotEmpty(t, record.UpstreamCostBreakdown)
	require.NotNil(t, record.Billing)
	require.Equal(t, "auto", record.Billing.Group)
	require.Equal(t, 0.25, record.Billing.GroupRatio)
}

func TestModelGatewayUserRequestsExposeDynamicBillingFields(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	other, err := common.Marshal(map[string]any{
		"billing_mode":                       "model_gateway_dynamic",
		"billing_source_detail":              "dynamic_group_ratio",
		"group_ratio":                        0.37,
		"model_ratio":                        2,
		"completion_ratio":                   1,
		"dynamic_billing_applied":            true,
		"dynamic_billing_ratio":              0.37,
		"dynamic_billing_price_per_m":        1.48,
		"dynamic_billing_sample_count":       8,
		"dynamic_billing_static_group_ratio": 0.1,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:        "req-dynamic-summary",
		CreatedAt:        now - 3,
		UpdatedAt:        now - 1,
		CompletedAt:      now - 1,
		RequestedModel:   "gpt-test",
		RequestedGroup:   "auto",
		SelectedGroup:    "codex-plus",
		FinalChannelID:   4,
		FinalChannelName: "toioto",
		Attempts:         1,
		FinalSuccess:     true,
		FinalStatusCode:  200,
		DurationMs:       900,
		TTFTMs:           120,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Type:             model.LogTypeConsume,
		CreatedAt:        now - 1,
		RequestId:        "req-dynamic-summary",
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		ChannelId:        4,
		PromptTokens:     100,
		CompletionTokens: 50,
		Quota:            111,
		Other:            string(other),
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.UserRequests.RecentRequests, 1)
	record := response.UserRequests.RecentRequests[0]
	require.Equal(t, "codex-plus", record.ActualGroup)
	require.Equal(t, 0.37, record.ActualGroupRatio)
	require.NotNil(t, record.Billing)
	require.Equal(t, "model_gateway_dynamic", record.Billing.BillingMode)
	require.True(t, record.Billing.DynamicBillingApplied)
	require.Equal(t, 0.37, record.Billing.DynamicBillingRatio)
	require.Equal(t, 1.48, record.Billing.DynamicBillingPricePerM)
	require.Equal(t, 8, record.Billing.DynamicBillingSampleCount)
}

func TestModelGatewayObservabilitySummaryExposesDynamicBillingOverview(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingProfitRate:     0.2,
		DynamicBillingWindowSamples:  300,
		DynamicBillingMinSamples:     5,
		DynamicBillingRefreshSeconds: 30,
		DynamicBillingMaxAgeSeconds:  300,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
				CandidateGroups:  []string{"codex-plus", "codex-pro"},
			},
		},
	})
	defer restoreSetting()

	now := common.GetTimestamp()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-5.5:codex-plus": {
			RequestedModel: "gpt-5.5",
			Group:          "codex-plus",
			Ratio:          0.37,
			PricePerM:      1.48,
			SampleCount:    8,
			CalculatedAt:   now - 30,
		},
		"gpt-5.4:codex-pro": {
			RequestedModel: "gpt-5.4",
			Group:          "codex-pro",
			Ratio:          0.41,
			PricePerM:      1.66,
			SampleCount:    10,
			CalculatedAt:   now - 20,
		},
	})
	defer restoreBaselines()

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.True(t, response.DynamicBilling.Enabled)
	require.Equal(t, 1, response.DynamicBilling.PolicyCount)
	require.Equal(t, 1, response.DynamicBilling.ActiveCount)
	require.Equal(t, 0, response.DynamicBilling.WaitingCount)
	require.Equal(t, 300, response.DynamicBilling.WindowSamples)
	require.Len(t, response.DynamicBilling.Groups, 1)
	group := response.DynamicBilling.Groups[0]
	expectedCurrentPrice := modelGatewayDynamicBillingPricePerMillion(modelGatewayDynamicBillingDisplayModel, group.CurrentRatio)
	expectedMinPrice := modelGatewayDynamicBillingPricePerMillion(modelGatewayDynamicBillingDisplayModel, group.MinRatio)
	expectedMaxPrice := modelGatewayDynamicBillingPricePerMillion(modelGatewayDynamicBillingDisplayModel, group.MaxRatio)
	expectedAveragePrice := modelGatewayDynamicBillingPricePerMillion(modelGatewayDynamicBillingDisplayModel, group.AverageRatio)
	require.Equal(t, "auto", group.PolicyGroup)
	require.Equal(t, []string{"auto", "codex-plus", "codex-pro"}, group.TargetGroups)
	require.Equal(t, "active", group.Status)
	require.Equal(t, 0.41, group.CurrentRatio)
	require.Equal(t, expectedCurrentPrice, group.CurrentPricePerM)
	require.Equal(t, "codex-pro", group.CurrentTargetGroup)
	require.Equal(t, "gpt-5.4", group.CurrentModel)
	require.Equal(t, "gpt-5.4", group.ReferenceModel)
	require.Equal(t, 0.37, group.MinRatio)
	require.Equal(t, 0.41, group.MaxRatio)
	require.Equal(t, expectedMinPrice, group.MinPricePerM)
	require.Equal(t, expectedMaxPrice, group.MaxPricePerM)
	require.InEpsilon(t, 0.3922222222, group.AverageRatio, 0.000001)
	require.Equal(t, expectedAveragePrice, group.AveragePricePerM)
	require.InEpsilon(t, 0.3922222222, group.BlendedRatio, 0.000001)
	require.Equal(t, expectedAveragePrice, group.BlendedPricePerM)
	require.Equal(t, 18, group.SampleCount)
	require.Equal(t, 18, group.EffectiveSamples)
	require.Equal(t, 2, group.ModelCount)
	require.True(t, response.DynamicBilling7d.Enabled)
	require.Equal(t, 7*24*60, response.DynamicBilling7d.WindowMinutes)
	require.Len(t, response.DynamicBilling7d.Groups, 1)
}

func TestModelGatewayObservabilitySummaryCurrentDynamicBillingPrefersRatioOverPrice(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingProfitRate:     0.2,
		DynamicBillingWindowSamples:  300,
		DynamicBillingMinSamples:     1,
		DynamicBillingRefreshSeconds: 30,
		DynamicBillingMaxAgeSeconds:  300,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
				CandidateGroups:  []string{"codex-pro", "codex-plus"},
			},
		},
	})
	defer restoreSetting()

	now := common.GetTimestamp()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"model-low-ratio-high-price:codex-plus": {
			RequestedModel: "model-low-ratio-high-price",
			Group:          "codex-plus",
			Ratio:          0.2,
			PricePerM:      10,
			SampleCount:    4,
			CalculatedAt:   now,
		},
		"model-high-ratio-low-price:codex-pro": {
			RequestedModel: "model-high-ratio-low-price",
			Group:          "codex-pro",
			Ratio:          0.5,
			PricePerM:      1,
			SampleCount:    3,
			CalculatedAt:   now,
		},
	})
	defer restoreBaselines()

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.DynamicBilling.Groups, 1)
	group := response.DynamicBilling.Groups[0]
	require.Equal(t, 0.5, group.CurrentRatio)
	require.Equal(t, modelGatewayDynamicBillingPricePerMillion(modelGatewayDynamicBillingDisplayModel, 0.5), group.CurrentPricePerM)
	require.Equal(t, "codex-pro", group.CurrentTargetGroup)
	require.Equal(t, "model-high-ratio-low-price", group.CurrentModel)
	require.Equal(t, "gpt-5.4", group.ReferenceModel)
}

func TestModelGatewayObservabilitySummary7dOverviewSkipsSamplesBeforeDynamicBillingEnabledAt(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	now := common.GetTimestamp()
	enabledAt := now - 3600
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingEnabledAt:      enabledAt,
		DynamicBillingProfitRate:     0.2,
		DynamicBillingWindowSamples:  300,
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

	db := model.DB
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:       "req-before-enabled",
		CreatedAt:       enabledAt - 60,
		UpdatedAt:       enabledAt - 60,
		CompletedAt:     enabledAt - 60,
		RequestedModel:  "gpt-test",
		RequestedGroup:  "auto",
		SelectedGroup:   "codex-plus",
		FinalChannelID:  4,
		Attempts:        1,
		FinalSuccess:    true,
		FinalStatusCode: 200,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:       "req-after-enabled",
		CreatedAt:       enabledAt + 60,
		UpdatedAt:       enabledAt + 60,
		CompletedAt:     enabledAt + 60,
		RequestedModel:  "gpt-test",
		RequestedGroup:  "auto",
		SelectedGroup:   "codex-plus",
		FinalChannelID:  4,
		Attempts:        1,
		FinalSuccess:    true,
		FinalStatusCode: 200,
	}).Error)
	beforeOther, err := common.Marshal(map[string]any{
		"dynamic_billing_applied":     true,
		"dynamic_billing_group":       "codex-plus",
		"dynamic_billing_ratio":       9.99,
		"dynamic_billing_price_per_m": 39.96,
	})
	require.NoError(t, err)
	afterOther, err := common.Marshal(map[string]any{
		"dynamic_billing_applied":     true,
		"dynamic_billing_group":       "codex-plus",
		"dynamic_billing_ratio":       1.2,
		"dynamic_billing_price_per_m": 4.8,
	})
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Type:             model.LogTypeConsume,
		RequestId:        "req-before-enabled",
		CreatedAt:        enabledAt - 60,
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            string(beforeOther),
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Type:             model.LogTypeConsume,
		RequestId:        "req-after-enabled",
		CreatedAt:        enabledAt + 60,
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            string(afterOther),
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.DynamicBilling7d.Groups, 1)
	group := response.DynamicBilling7d.Groups[0]
	expectedPrice := modelGatewayDynamicBillingPricePerMillion(modelGatewayDynamicBillingDisplayModel, 1.2)
	require.Equal(t, "auto", group.PolicyGroup)
	require.Equal(t, "codex-plus", group.CurrentTargetGroup)
	require.Equal(t, "gpt-test", group.CurrentModel)
	require.Equal(t, "gpt-5.4", group.ReferenceModel)
	require.Equal(t, 1, group.SampleCount)
	require.Equal(t, "active", group.Status)
	require.InEpsilon(t, 1.2, group.CurrentRatio, 0.000001)
	require.Equal(t, expectedPrice, group.CurrentPricePerM)
	require.InEpsilon(t, 1.2, group.AverageRatio, 0.000001)
	require.Equal(t, expectedPrice, group.AveragePricePerM)
	require.InEpsilon(t, 1.2, group.BlendedRatio, 0.000001)
	require.Equal(t, expectedPrice, group.BlendedPricePerM)
	require.InEpsilon(t, 1.2, group.MinRatio, 0.000001)
	require.InEpsilon(t, 1.2, group.MaxRatio, 0.000001)
	require.Equal(t, expectedPrice, group.MinPricePerM)
	require.Equal(t, expectedPrice, group.MaxPricePerM)
	require.Equal(t, 1, group.EffectiveSamples)
	require.Equal(t, 1, group.ModelCount)
	require.GreaterOrEqual(t, group.LatestCalculatedAt, enabledAt)
}

func TestModelGatewayObservabilitySummary7dOverviewWaitsWithoutAppliedSamplesAfterEnabledAt(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	now := common.GetTimestamp()
	enabledAt := now - 3600
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingEnabledAt:      enabledAt,
		DynamicBillingProfitRate:     0.2,
		DynamicBillingWindowSamples:  300,
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

	require.NoError(t, model.DB.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:       "req-not-applied",
		CreatedAt:       enabledAt + 60,
		UpdatedAt:       enabledAt + 60,
		CompletedAt:     enabledAt + 60,
		RequestedModel:  "gpt-test",
		RequestedGroup:  "auto",
		SelectedGroup:   "codex-plus",
		FinalChannelID:  4,
		Attempts:        1,
		FinalSuccess:    true,
		FinalStatusCode: 200,
	}).Error)

	other, err := common.Marshal(map[string]any{
		"dynamic_billing_applied":     false,
		"dynamic_billing_group":       "codex-plus",
		"dynamic_billing_ratio":       0.8,
		"dynamic_billing_price_per_m": 3.2,
	})
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Type:             model.LogTypeConsume,
		RequestId:        "req-not-applied",
		CreatedAt:        enabledAt + 60,
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            string(other),
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.DynamicBilling7d.Groups, 1)
	group := response.DynamicBilling7d.Groups[0]
	require.Equal(t, "auto", group.PolicyGroup)
	require.Equal(t, "waiting_samples", group.Status)
	require.Zero(t, group.SampleCount)
	require.Zero(t, group.EffectiveSamples)
	require.Zero(t, group.CurrentRatio)
	require.Zero(t, group.CurrentPricePerM)
	require.Zero(t, group.AverageRatio)
	require.Zero(t, group.AveragePricePerM)
	require.Zero(t, group.BlendedRatio)
	require.Zero(t, group.BlendedPricePerM)
	require.Equal(t, 1, response.DynamicBilling7d.WaitingCount)
	require.Zero(t, response.DynamicBilling7d.ActiveCount)
}

func TestModelGatewayObservabilitySummary7dOverviewSkipsHealthProbeAppliedSamples(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	now := common.GetTimestamp()
	enabledAt := now - 3600
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingEnabledAt:      enabledAt,
		DynamicBillingProfitRate:     0.2,
		DynamicBillingWindowSamples:  300,
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

	require.NoError(t, model.DB.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:       "req-health-probe-dynamic",
		CreatedAt:       enabledAt + 60,
		UpdatedAt:       enabledAt + 60,
		CompletedAt:     enabledAt + 60,
		RequestedModel:  "gpt-test",
		RequestedGroup:  "auto",
		SelectedGroup:   "codex-plus",
		FinalChannelID:  4,
		Attempts:        1,
		FinalSuccess:    true,
		FinalStatusCode: 200,
	}).Error)

	other, err := common.Marshal(map[string]any{
		"dynamic_billing_applied":     true,
		"dynamic_billing_group":       "codex-plus",
		"dynamic_billing_ratio":       9.99,
		"dynamic_billing_price_per_m": 39.96,
		"is_health_probe":             true,
		"billing_source":              "model_gateway_probe",
	})
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Type:             model.LogTypeConsume,
		RequestId:        "req-health-probe-dynamic",
		CreatedAt:        enabledAt + 60,
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            string(other),
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.DynamicBilling7d.Groups, 1)
	group := response.DynamicBilling7d.Groups[0]
	require.Equal(t, "auto", group.PolicyGroup)
	require.Equal(t, "waiting_samples", group.Status)
	require.Zero(t, group.SampleCount)
	require.Zero(t, group.CurrentRatio)
	require.Equal(t, 1, response.DynamicBilling7d.WaitingCount)
	require.Zero(t, response.DynamicBilling7d.ActiveCount)
}

func TestModelGatewayObservabilitySummaryDynamicBillingOverviewWaitingWithoutBaseline(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:        true,
		DynamicBillingWindowSamples:  300,
		DynamicBillingMinSamples:     5,
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

	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{})
	defer restoreBaselines()

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.Len(t, response.DynamicBilling.Groups, 1)
	require.Equal(t, "waiting_samples", response.DynamicBilling.Groups[0].Status)
	require.Equal(t, 1, response.DynamicBilling.WaitingCount)
}

func TestModelGatewayObservabilitySummaryDynamicBillingOverviewGlobalDisabled(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	resetModelGatewayObservabilitySummaryCache()
	defer resetModelGatewayObservabilitySummaryCache()

	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled: false,
		GroupPolicies: map[string]scheduler_setting.GroupPolicySetting{
			"auto": {
				BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
				CandidateGroups:  []string{"codex-plus"},
			},
		},
	})
	defer restoreSetting()

	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-5.5:codex-plus": {
			RequestedModel: "gpt-5.5",
			Group:          "codex-plus",
			Ratio:          0.37,
			PricePerM:      1.48,
			SampleCount:    8,
			CalculatedAt:   common.GetTimestamp() - 30,
		},
	})
	defer restoreBaselines()

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		ViewMode:    modelGatewayObservabilityViewUserRequests,
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ScanLimit:   10,
	})
	require.NoError(t, err)
	require.False(t, response.DynamicBilling.Enabled)
	require.Len(t, response.DynamicBilling.Groups, 1)
	require.Equal(t, "global_disabled", response.DynamicBilling.Groups[0].Status)
}

func TestBuildModelGatewayObservabilitySummaryUserRequestViewSkipsExecutionRecords(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 20,
		RequestId:      "req-engineering-only",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-5.4",
		ChannelId:      8,
		Success:        true,
		DurationMs:     100,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		CreatedAt:      now - 10,
		UpdatedAt:      now - 9,
		CompletedAt:    now - 8,
		RequestId:      "req-user-view",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		Attempts:       1,
		FinalSuccess:   true,
		DurationMs:     900,
		TTFTMs:         120,
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ViewMode:    modelGatewayObservabilityViewUserRequests,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), response.Summary.TotalRecords)
	require.Empty(t, response.RecentRecords)
	require.Equal(t, int64(1), response.UserRequests.Summary.TotalRequests)
	require.Equal(t, int64(1), response.UserRequests.Summary.Successes)
	require.Len(t, response.UserRequests.RecentRequests, 1)
	require.Equal(t, "req-user-view", response.UserRequests.RecentRequests[0].RequestID)
}

func TestBuildModelGatewayObservabilitySummaryUserRequestUsesDispatchUserFallback(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.User{
		Id:       2001,
		Username: "dispatch_user",
		Password: "password",
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		CreatedAt:      now - 10,
		UpdatedAt:      now - 9,
		CompletedAt:    now - 8,
		RequestId:      "req-dispatch-user",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		Attempts:       1,
		FinalSuccess:   true,
		DurationMs:     900,
		TTFTMs:         120,
	}).Error)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 9,
		RequestId:      "req-dispatch-user",
		UserId:         2001,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		ChannelId:      8,
		PolicyMode:     "active",
		SmartHandled:   true,
		ScoreTotal:     0.92,
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 5,
		TopN:        5,
		ViewMode:    modelGatewayObservabilityViewUserRequests,
	})
	require.NoError(t, err)
	require.Len(t, response.UserRequests.RecentRequests, 1)
	record := response.UserRequests.RecentRequests[0]
	require.Equal(t, "req-dispatch-user", record.RequestID)
	require.Equal(t, 2001, record.UserID)
	require.Equal(t, "dispatch_user", record.Username)
	require.NotNil(t, record.DispatchRecord)
}

func TestBuildModelGatewayObservabilitySummaryUsesDefaultScanLimit(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 2,
			RequestId:      "req-realtime",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.4",
			ChannelId:      8,
			PolicyMode:     "active",
			SmartHandled:   true,
		},
		{
			CreatedAt:      now - 1,
			RequestId:      "req-realtime",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.4",
			ChannelId:      8,
			Success:        true,
			DurationMs:     7290,
			TTFTMs:         2650,
		},
	}).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       24,
		RecentLimit: 50,
		TopN:        10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), response.Summary.TotalRecords)
	require.Equal(t, 2, response.Summary.ScannedRecords)
	require.Equal(t, int64(1), response.Summary.Dispatches)
	require.Equal(t, int64(1), response.Summary.Attempts)
	require.Equal(t, int64(1), response.Summary.Successes)
	require.Equal(t, 1.0, response.Summary.SuccessRate)
	require.Len(t, response.ByModel, 1)
	require.Equal(t, int64(1), response.ByModel[0].Attempts)
}

func TestBuildModelGatewayObservabilitySummarySkipsTotalCountByDefault(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	records := make([]model.ModelExecutionRecord, 0, 3)
	for idx := 0; idx < 3; idx++ {
		records = append(records, model.ModelExecutionRecord{
			CreatedAt:      now - int64(idx+1),
			RequestId:      fmt.Sprintf("req-fast-summary-%d", idx),
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			Success:        true,
			DurationMs:     100,
		})
	}
	require.NoError(t, db.Create(&records).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 2,
		TopN:        5,
		ScanLimit:   2,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), response.Summary.TotalRecords)
	require.Equal(t, 2, response.Summary.ScannedRecords)
	require.True(t, response.Summary.Truncated)
	require.Equal(t, int64(2), response.Summary.Attempts)
}

func TestBuildModelGatewayObservabilitySummaryIncludesTotalWhenRequested(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	records := make([]model.ModelExecutionRecord, 0, 3)
	for idx := 0; idx < 3; idx++ {
		records = append(records, model.ModelExecutionRecord{
			CreatedAt:      now - int64(idx+1),
			RequestId:      fmt.Sprintf("req-full-summary-%d", idx),
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			Success:        true,
			DurationMs:     100,
		})
	}
	require.NoError(t, db.Create(&records).Error)

	response, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:        1,
		RecentLimit:  2,
		TopN:         5,
		ScanLimit:    2,
		IncludeTotal: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), response.Summary.TotalRecords)
	require.Equal(t, 2, response.Summary.ScannedRecords)
	require.True(t, response.Summary.Truncated)
	require.Equal(t, int64(2), response.Summary.Attempts)
}

func TestModelGatewayObservabilitySummaryCacheKeyNormalizesAndEscapes(t *testing.T) {
	require.Equal(t, 2*time.Second, modelGatewayObservabilitySummaryFreshTTL)

	left := modelGatewayObservabilitySummaryCacheKey(ModelGatewayObservabilityOptions{
		Hours:              0,
		RecentLimit:        500,
		TopN:               0,
		ScanLimit:          0,
		TrendBucketSeconds: 30,
		Model:              " gpt=5&5 ",
		Group:              " vip group ",
		ChannelID:          -10,
		RequestID:          " req=a&b ",
	})
	right := modelGatewayObservabilitySummaryCacheKey(ModelGatewayObservabilityOptions{
		Hours:              1,
		RecentLimit:        modelGatewayObservabilityMaxRecentLimit,
		TopN:               1,
		ScanLimit:          modelGatewayObservabilityDefaultScanLimit,
		TrendBucketSeconds: modelGatewayObservabilityMinTrendBucket,
		Model:              "gpt=5&5",
		Group:              "vip group",
		ChannelID:          0,
		RequestID:          "req=a&b",
	})
	require.Equal(t, right, left)
	require.Contains(t, left, "model=gpt%3D5%265")
	require.Contains(t, left, "request_id=req%3Da%26b")
}

func TestBuildModelGatewayObservabilitySummaryCacheUsesFreshResultByQuery(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 1,
		RequestId:      "req-cache-a",
		RequestedGroup: "vip",
		SelectedGroup:  "vip",
		RequestedModel: "gpt-5.5",
		ChannelId:      42,
		Success:        true,
		DurationMs:     100,
	}).Error)

	options := ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 10,
		TopN:        5,
		ScanLimit:   20,
		Group:       "vip",
	}
	first, err := BuildModelGatewayObservabilitySummary(options)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Summary.Successes)

	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now,
		RequestId:      "req-cache-b",
		RequestedGroup: "vip",
		SelectedGroup:  "vip",
		RequestedModel: "gpt-5.5",
		ChannelId:      42,
		Success:        true,
		DurationMs:     200,
	}).Error)

	cached, err := BuildModelGatewayObservabilitySummary(options)
	require.NoError(t, err)
	require.Equal(t, int64(1), cached.Summary.Successes)

	filtered, err := BuildModelGatewayObservabilitySummary(ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 10,
		TopN:        5,
		ScanLimit:   20,
		Group:       "default",
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), filtered.Summary.Successes)

	InvalidateModelGatewayObservabilitySummaryCacheForRecord(model.ModelExecutionRecord{
		CreatedAt:      now,
		RequestedGroup: "vip",
		SelectedGroup:  "vip",
		ChannelId:      42,
		RequestedModel: "gpt-5.5",
	})
	refreshed, err := BuildModelGatewayObservabilitySummary(options)
	require.NoError(t, err)
	require.Equal(t, int64(2), refreshed.Summary.Successes)
}

func TestInvalidateModelGatewayObservabilitySummaryCacheForRecord(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)
	now := time.Now()
	matching := ModelGatewayObservabilityOptions{
		Hours:       1,
		RecentLimit: 50,
		TopN:        10,
		ScanLimit:   100,
		Model:       "gpt-5.5",
		Group:       "vip",
		ChannelID:   42,
	}
	other := matching
	other.ChannelID = 99
	matchingKey := modelGatewayObservabilitySummaryCacheKey(matching)
	otherKey := modelGatewayObservabilitySummaryCacheKey(other)
	modelGatewayObservabilitySummaryCache.store(matchingKey, normalizeModelGatewayObservabilityOptions(matching), ModelGatewayObservabilityResponse{}, now)
	modelGatewayObservabilitySummaryCache.store(otherKey, normalizeModelGatewayObservabilityOptions(other), ModelGatewayObservabilityResponse{}, now)

	InvalidateModelGatewayObservabilitySummaryCacheForRecord(model.ModelExecutionRecord{
		CreatedAt:      common.GetTimestamp(),
		RequestedModel: "gpt-5.5",
		RequestedGroup: "default",
		SelectedGroup:  "vip",
		ChannelId:      42,
	})

	_, ok := modelGatewayObservabilitySummaryCache.lookupFresh(matchingKey, now)
	require.False(t, ok)
	_, ok = modelGatewayObservabilitySummaryCache.lookupFresh(otherKey, now)
	require.True(t, ok)
}

func TestModelGatewayStickyStoreListsAndClearsByKeyID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps)

	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps)
	require.NotNil(t, runtimeDeps.StickyRouter)
	key := fmt.Sprintf("sensitive-user-session-%d", time.Now().UnixNano())
	store := scheduler.NewMemoryStickyStore(8)
	store.Set(key, scheduler.StickyEntry{
		ChannelID:      42,
		Group:          "vip",
		KeyFingerprint: "session-fp",
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	routerStore := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{Store: store}, nil)
	runtimeDeps.StickyRouter = routerStore

	router := gin.New()
	router.GET("/api/model_gateway/observability/sticky", GetModelGatewayStickyStore)
	router.DELETE("/api/model_gateway/observability/sticky/:key_id", ClearModelGatewayStickyStoreEntry)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/sticky", nil)
	router.ServeHTTP(resp, req)

	var payload modelGatewayStickyStoreAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success, resp.Body.String())
	require.Len(t, payload.Data.Items, 1)
	item := payload.Data.Items[0]
	require.NotEmpty(t, item.KeyID)
	require.Equal(t, 42, item.ChannelID)
	require.Equal(t, "vip", item.Group)
	require.Equal(t, "session-fp", item.KeyFingerprint)
	require.NotContains(t, resp.Body.String(), key)

	clearResp := httptest.NewRecorder()
	clearReq := httptest.NewRequest(http.MethodDelete, "/api/model_gateway/observability/sticky/"+item.KeyID, nil)
	router.ServeHTTP(clearResp, clearReq)

	var clearPayload modelGatewayStickyClearAPIResponse
	require.NoError(t, common.Unmarshal(clearResp.Body.Bytes(), &clearPayload), clearResp.Body.String())
	require.True(t, clearPayload.Success, clearResp.Body.String())
	require.True(t, clearPayload.Data.Cleared)
	require.Empty(t, routerStore.StickyEntries())
}

func TestModelGatewayStickyStoreBulkClearsByGroupAndChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps)

	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps)
	store := scheduler.NewMemoryStickyStore(8)
	store.Set("sticky-vip-42-a", scheduler.StickyEntry{
		ChannelID:      42,
		Group:          "vip",
		KeyFingerprint: "fp-a",
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	store.Set("sticky-vip-42-b", scheduler.StickyEntry{
		ChannelID:      42,
		Group:          "vip",
		KeyFingerprint: "fp-b",
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	store.Set("sticky-vip-43", scheduler.StickyEntry{
		ChannelID:      43,
		Group:          "vip",
		KeyFingerprint: "fp-c",
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	store.Set("sticky-default-42", scheduler.StickyEntry{
		ChannelID:      42,
		Group:          "default",
		KeyFingerprint: "fp-d",
		ExpiresAt:      time.Now().Add(time.Minute),
	})
	routerStore := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{Store: store}, nil)
	runtimeDeps.StickyRouter = routerStore

	router := gin.New()
	router.DELETE("/api/model_gateway/observability/sticky", ClearModelGatewayStickyStore)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/model_gateway/observability/sticky?group=vip&channel_id=42", nil)
	router.ServeHTTP(resp, req)

	var payload modelGatewayStickyClearAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success, resp.Body.String())
	require.True(t, payload.Data.Cleared)
	require.Equal(t, 2, payload.Data.Deleted)
	require.Equal(t, "vip", payload.Data.Group)
	require.Equal(t, 42, payload.Data.ChannelID)

	entries := routerStore.StickyEntries()
	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.False(t, entry.Group == "vip" && entry.ChannelID == 42)
	}
}

func TestModelGatewayStickyStoreBulkClearRequiresFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/model_gateway/observability/sticky", ClearModelGatewayStickyStore)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/model_gateway/observability/sticky", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "missing sticky clear filter")
}

func TestGetModelGatewayObservabilitySummaryAddsRuntimeRiskTimeline(t *testing.T) {
	response := ModelGatewayObservabilityResponse{
		RuntimeStatus: modelgatewayobservability.RuntimeStatusResponse{
			Summary: modelgatewayobservability.RuntimeStatusSummary{
				UpdatedAt: 1710000000,
			},
			Items: []modelgatewayobservability.RuntimeStatusItem{
				{
					RequestedModel:    "gpt-5.5",
					UpstreamModel:     "mimo-v1",
					ChannelID:         101,
					Group:             "vip",
					EndpointType:      string(constant.EndpointTypeOpenAI),
					HealthStatus:      "circuit_open",
					CircuitOpen:       true,
					CircuitOpenReason: scheduler.CircuitErrorRateLimit,
					CircuitErrorCounts: map[string]int{
						scheduler.CircuitErrorRateLimit: 3,
						scheduler.CircuitErrorServer:    1,
					},
				},
				{
					RequestedModel:    "gpt-5.5",
					ChannelID:         102,
					Group:             "vip",
					EndpointType:      string(constant.EndpointTypeOpenAI),
					HealthStatus:      "healthy",
					ActiveConcurrency: 1,
					MaxConcurrency:    4,
				},
				{
					RequestedModel:         "claude-4",
					ChannelID:              103,
					Group:                  "default",
					EndpointType:           string(constant.EndpointTypeOpenAI),
					HealthStatus:           "failure_avoidance",
					FailureAvoidance:       true,
					FailureAvoidanceReason: "upstream_5xx",
				},
			},
		},
	}

	applyModelGatewayRuntimeRiskEvents(&response)

	require.Equal(t, int64(2), response.Summary.RiskEvents)
	require.Equal(t, 2, response.Summary.CurrentRiskRuntimeKeys)
	requireTrendRejectReason(t, response.Summary.CircuitOpenReasons, scheduler.CircuitErrorRateLimit, 1)
	requireTrendRejectReason(t, response.Summary.CircuitErrorTypes, scheduler.CircuitErrorRateLimit, 3)
	requireTrendRejectReason(t, response.Summary.CircuitErrorCounts, scheduler.CircuitErrorRateLimit, 3)
	requireTrendRejectReason(t, response.Summary.CircuitErrorTypes, scheduler.CircuitErrorServer, 1)
	require.Len(t, response.RiskTimeline, 2)
	require.Len(t, response.RiskEvents, 2)
	require.Equal(t, int64(2), response.Risk.EventCount)
	require.Equal(t, 2, response.Risk.CurrentRiskRuntimeKeys)
	requireTrendRejectReason(t, response.Risk.TopCircuitOpenReasons, scheduler.CircuitErrorRateLimit, 1)
	requireTrendRejectReason(t, response.Risk.TopCircuitErrorTypes, scheduler.CircuitErrorRateLimit, 3)
	requireRiskEvent(t, response.RiskTimeline, "runtime_status", "current_runtime_status", "circuit_open", scheduler.CircuitErrorRateLimit)
	requireRiskEvent(t, response.RiskTimeline, "runtime_status", "current_runtime_status", "failure_avoidance", "upstream_5xx")
}

func TestGetModelGatewayObservabilitySummaryFiltersAndReportsParseErrors(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 10,
			RequestId:      "req-good",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			Success:        true,
			DurationMs:     100,
		},
		{
			CreatedAt:      now - 8,
			RequestId:      "req-bad-score",
			RequestedGroup: "vip",
			SelectedGroup:  "vip",
			RequestedModel: "gpt-5.5",
			ChannelId:      11,
			PolicyMode:     "active",
			SmartHandled:   true,
			ScoreBreakdown: "{broken",
		},
		{
			CreatedAt:      now - 6,
			RequestId:      "req-other",
			RequestedGroup: "vip",
			SelectedGroup:  "vip",
			RequestedModel: "claude-4",
			ChannelId:      12,
			Success:        false,
			StatusCode:     http.StatusTooManyRequests,
		},
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?model=gpt-5.5&top_n=1", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayObservabilityResponse(t, resp)
	require.True(t, payload.Success)
	require.Equal(t, int64(2), payload.Data.Summary.TotalRecords)
	require.Equal(t, int64(1), payload.Data.Summary.Attempts)
	require.Equal(t, int64(1), payload.Data.Summary.Successes)
	require.Equal(t, int64(1), payload.Data.Summary.ScoreBreakdownParseErrs)
	require.Equal(t, int64(1), payload.Data.ScoreBreakdown.ParseErrors)
	require.Len(t, payload.Data.ByModel, 1)
	require.Equal(t, "gpt-5.5", payload.Data.ByModel[0].Key)
}

func TestGetModelGatewayObservabilitySummaryUsesCustomTrendBucket(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 2*3600,
			RequestId:      "req-a",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			Success:        true,
			DurationMs:     100,
		},
		{
			CreatedAt:      now - 30*60,
			RequestId:      "req-b",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			Success:        false,
			StatusCode:     http.StatusTooManyRequests,
		},
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?hours=6&trend_bucket_seconds=1800", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayObservabilityResponse(t, resp)
	require.True(t, payload.Success)
	require.Equal(t, int64(1800), payload.Data.Summary.TrendBucketSeconds)
	require.Len(t, payload.Data.Trends, 12)
	require.NotEqual(t, payload.Data.Trends[0].BucketStart, payload.Data.Trends[1].BucketStart)
	require.Equal(t, int64(1800), payload.Data.Trends[0].BucketEnd-payload.Data.Trends[0].BucketStart)
	require.Equal(t, int64(2), payload.Data.Summary.Attempts)
}

func TestGetModelGatewayObservabilitySummaryTrendQueueWaitPercentiles(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	records := make([]model.ModelExecutionRecord, 0, 20)
	for idx := 1; idx <= 20; idx++ {
		requestMeta, err := common.Marshal(map[string]any{
			"queue_wait_ms": idx * 10,
		})
		require.NoError(t, err)
		records = append(records, model.ModelExecutionRecord{
			CreatedAt:      now - int64(idx),
			RequestId:      fmt.Sprintf("queue-percentile-%02d", idx),
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			PolicyMode:     "active",
			SmartHandled:   true,
			RequestMeta:    string(requestMeta),
		})
	}
	require.NoError(t, db.Create(&records).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?hours=1&trend_bucket_seconds=3600", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayObservabilityResponse(t, resp)
	require.True(t, payload.Success)
	trend := requireModelGatewayTrendWithRecords(t, payload.Data.Trends, 20)
	require.Equal(t, int64(20), trend.QueuedDispatches)
	require.Equal(t, int64(105), trend.AvgQueueWaitMs)
	require.Equal(t, int64(100), trend.QueueWaitP50Ms)
	require.Equal(t, int64(180), trend.QueueWaitP90Ms)
	require.Equal(t, int64(190), trend.QueueWaitP95Ms)
}

func TestGetModelGatewayObservabilitySummaryTrendQueueWaitPercentilesEmptySamples(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	requestMeta, err := common.Marshal(map[string]any{
		"queue_enabled": true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 10,
		RequestId:      "queue-empty-sample",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-5.5",
		ChannelId:      10,
		PolicyMode:     "active",
		SmartHandled:   true,
		RequestMeta:    string(requestMeta),
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?hours=1&trend_bucket_seconds=3600", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayObservabilityResponse(t, resp)
	require.True(t, payload.Success)
	trend := requireModelGatewayTrendWithRecords(t, payload.Data.Trends, 1)
	require.Equal(t, int64(1), trend.QueueEnabledDispatches)
	require.Equal(t, int64(0), trend.QueuedDispatches)
	require.Equal(t, int64(0), trend.AvgQueueWaitMs)
	require.Equal(t, int64(0), trend.QueueWaitP50Ms)
	require.Equal(t, int64(0), trend.QueueWaitP90Ms)
	require.Equal(t, int64(0), trend.QueueWaitP95Ms)
}

func TestGetModelGatewayObservabilitySummaryRejectsInvalidTrendBucket(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?trend_bucket_seconds=abc", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "invalid trend_bucket_seconds")
}

func TestGetModelGatewayObservabilitySummaryRejectsInvalidChannelID(t *testing.T) {
	setupModelGatewayReplayControllerTestDB(t)

	router := gin.New()
	router.GET("/api/model_gateway/observability/summary", GetModelGatewayObservabilitySummary)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/summary?channel_id=abc", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "invalid channel_id")
}

func TestExportModelGatewayObservabilityTrendsReturnsPayload(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	requestMeta, err := common.Marshal(map[string]any{
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:     10,
				ChannelName:   "primary",
				Group:         "default",
				UpstreamModel: "gpt-5.5",
				RuntimeKey: core.RuntimeKey{
					RequestedModel: "gpt-5.5",
					UpstreamModel:  "gpt-5.5",
					ChannelID:      10,
					Group:          "default",
					EndpointType:   constant.EndpointTypeOpenAI,
				},
				Available:    false,
				RejectReason: "circuit_open",
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 120,
			RequestId:      "trend-export",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			Success:        true,
			DurationMs:     100,
		},
		{
			CreatedAt:      now - 90,
			RequestId:      "trend-export",
			RequestedGroup: "default",
			SelectedGroup:  "default",
			RequestedModel: "gpt-5.5",
			ChannelId:      10,
			PolicyMode:     "active",
			SmartHandled:   true,
			FallbackUsed:   true,
			RequestMeta:    string(requestMeta),
		},
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/trends/export", ExportModelGatewayObservabilityTrends)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/trends/export?hours=6&trend_bucket_seconds=1800&model=gpt-5.5", nil)
	router.ServeHTTP(resp, req)

	var payload modelGatewayTrendExportAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success)
	require.Equal(t, "modelgateway_observability_trends", payload.Data.Kind)
	require.Equal(t, 6, payload.Data.Filters.Hours)
	require.Equal(t, int64(1800), payload.Data.Filters.TrendBucketSeconds)
	require.Equal(t, "gpt-5.5", payload.Data.Filters.Model)
	require.Equal(t, int64(1), payload.Data.Summary.Attempts)
	require.Equal(t, int64(1), payload.Data.Summary.FallbackUsed)
	require.GreaterOrEqual(t, payload.Data.Preview.NonEmptyBuckets, 1)
	require.GreaterOrEqual(t, payload.Data.Preview.RiskEvents, int64(1))
	require.Equal(t, payload.Data.Preview.RiskEvents, payload.Data.Preview.RiskEventCount)
	require.Equal(t, payload.Data.Preview.RiskEvents, payload.Data.Preview.Risk.EventCount)
	require.NotEmpty(t, payload.Data.Preview.RiskTimeline)
	require.NotEmpty(t, payload.Data.Preview.Risk.RiskTimeline)
	requireTrendRejectReason(t, payload.Data.Preview.TopRejectReasons, "circuit_open", 1)
	requireTrendRejectReason(t, payload.Data.Preview.TopCircuitOpenReasons, "circuit_open", 1)
	requireTrendRejectReason(t, payload.Data.Preview.Risk.TopRejectReasons, "circuit_open", 1)
	requireTrendRejectReason(t, payload.Data.Preview.Risk.TopCircuitOpenReasons, "circuit_open", 1)
	requireRiskStatusCount(t, payload.Data.Preview.TopRiskStatuses, "circuit_open", 1)
	requireRiskStatusCount(t, payload.Data.Preview.Risk.TopRiskStatuses, "circuit_open", 1)
	require.Len(t, payload.Data.Trends, 12)
}

func TestExportModelGatewayObservabilityTrendsDownload(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now - 60,
		RequestId:      "trend-download",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-5.5",
		ChannelId:      10,
		Success:        true,
		DurationMs:     100,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/trends/export", ExportModelGatewayObservabilityTrends)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/trends/export?hours=6&trend_bucket_seconds=1800&download=true", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Header().Get("Content-Type"), "application/json")
	require.Contains(t, resp.Header().Get("Content-Disposition"), "modelgateway-trends-6h-1800s.json")
	require.Contains(t, resp.Body.String(), `"kind":"modelgateway_observability_trends"`)
	require.Contains(t, resp.Body.String(), `"trends"`)
	require.Contains(t, resp.Body.String(), `"preview"`)
	require.Contains(t, resp.Body.String(), `"risk_event_count"`)
	require.NotContains(t, resp.Body.String(), `"success":true`)
}

func TestGetModelGatewayRuntimeStatusReturnsInjectedState(t *testing.T) {
	now := time.Unix(1710000000, 0)
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      808,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        0.91,
		DurationMs:         1000,
		ActiveConcurrency:  2,
		MaxConcurrency:     4,
		QueueCapacity:      8,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        9,
	})
	service := modelgatewayobservability.NewRuntimeStatusService(modelgatewayobservability.RuntimeStatusDeps{
		SnapshotStore: store,
		QueueSnapshot: func() map[int]int {
			return map[int]int{808: 2}
		},
		QueueDetailSnapshot: func() core.RuntimeQueueSnapshot {
			return core.RuntimeQueueSnapshot{
				UpdatedAt: now.Unix(),
				Summary: core.RuntimeQueueSummary{
					UpdatedAt: now.Unix(),
				},
				Channels: []core.RuntimeQueueChannelSnapshot{
					{
						ChannelID:       808,
						QueueDepth:      2,
						QueuedRequests:  2,
						WaitingRequests: 2,
						QueueCapacity:   8,
						NormalDepth:     2,
						Groups: []core.RuntimeQueueGroupSnapshot{
							{ChannelID: 808, Group: "vip", QueueDepth: 2, QueuedRequests: 2, WaitingRequests: 2, NormalDepth: 2},
						},
					},
				},
			}
		},
		Now: func() time.Time { return now },
	})

	router := gin.New()
	router.GET("/api/model_gateway/observability/runtime", func(c *gin.Context) {
		query, err := parseModelGatewayRuntimeStatusQuery(c)
		require.NoError(t, err)
		common.ApiSuccess(c, service.Build(query))
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/runtime?model=gpt-5.5&group=vip&channel_id=808", nil)
	router.ServeHTTP(resp, req)

	payload := decodeModelGatewayRuntimeStatusResponse(t, resp)
	require.True(t, payload.Success)
	require.Equal(t, int64(1710000000), payload.Data.Summary.UpdatedAt)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, 808, payload.Data.Items[0].ChannelID)
	require.Equal(t, 2, payload.Data.Items[0].QueueDepth)
	require.Equal(t, "healthy", payload.Data.Items[0].HealthStatus)
	require.NotNil(t, payload.Data.QueueSnapshot)
	require.Equal(t, 2, payload.Data.QueueSnapshot.Summary.TotalQueued)
	require.Equal(t, 808, payload.Data.QueueSnapshot.Channels[0].ChannelID)
	require.Equal(t, "vip", payload.Data.QueueSnapshot.Groups[0].Group)
}

func TestGetModelGatewayRuntimeStatusRejectsInvalidChannelID(t *testing.T) {
	router := gin.New()
	router.GET("/api/model_gateway/observability/runtime", GetModelGatewayRuntimeStatus)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/runtime?channel_id=abc", nil)
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"success":false`)
	require.Contains(t, resp.Body.String(), "invalid channel_id")
}

func TestGetModelGatewayScoreHistoryReturnsCandidateChanges(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps)
	now := common.GetTimestamp()
	buildMeta := func(score float64, selected bool) string {
		t.Helper()
		body, err := common.Marshal(map[string]any{
			"candidate_explanations": []core.CandidateExplanation{
				{
					ChannelID:     88,
					ChannelName:   "score-channel",
					Group:         "codex-plus",
					UpstreamModel: "gpt-5.5",
					RuntimeKey: core.RuntimeKey{
						RequestedModel:        "gpt-5.5",
						UpstreamModel:         "gpt-5.5",
						ChannelID:             88,
						Group:                 "codex-plus",
						EndpointType:          constant.EndpointTypeOpenAI,
						CapabilityFingerprint: "openai_codex",
					},
					Available:      true,
					Selected:       selected,
					ScoreTotal:     score,
					ScoreBreakdown: map[string]float64{"success": score, "speed": score - 0.1},
					SuccessScore:   score,
					SpeedScore:     score - 0.1,
					SampleCount:    6,
				},
				{
					ChannelID:   99,
					ChannelName: "other-channel",
					Available:   true,
					ScoreTotal:  0.99,
				},
			},
		})
		require.NoError(t, err)
		return string(body)
	}
	require.NoError(t, db.Create(&[]model.ModelExecutionRecord{
		{
			CreatedAt:      now - 20,
			RequestId:      "score-new",
			RequestedGroup: "codex-plus",
			SelectedGroup:  "codex-plus",
			RequestedModel: "gpt-5.5",
			PolicyMode:     "active",
			SmartHandled:   true,
			SelectedReason: "weighted_score",
			RequestMeta:    buildMeta(0.72, true),
		},
		{
			CreatedAt:      now - 80,
			RequestId:      "score-old",
			RequestedGroup: "codex-plus",
			SelectedGroup:  "codex-plus",
			RequestedModel: "gpt-5.5",
			PolicyMode:     "active",
			SmartHandled:   true,
			RequestMeta:    buildMeta(0.61, false),
		},
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/score-history", GetModelGatewayScoreHistory)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/score-history?hours=1&limit=10&channel_id=88&requested_model=gpt-5.5&group=codex-plus", nil)
	router.ServeHTTP(resp, req)

	var payload modelGatewayScoreHistoryAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success)
	require.Equal(t, 88, payload.Data.ChannelID)
	require.Equal(t, "score-channel", payload.Data.ChannelName)
	require.Len(t, payload.Data.Items, 2)
	require.Equal(t, "score-new", payload.Data.Items[0].RequestID)
	require.Equal(t, 0.72, payload.Data.Items[0].ScoreTotal)
	require.Equal(t, 0.11, payload.Data.Items[0].ScoreDelta)
	require.Equal(t, 0.11, payload.Data.Items[0].ScoreBreakdownDelta["success"])
	require.True(t, payload.Data.Items[0].Selected)
	require.Equal(t, "weighted_score", payload.Data.Items[0].SelectedReason)
	require.NotNil(t, payload.Data.Current)
	require.NotNil(t, payload.Data.Previous)
	require.Equal(t, 0.11, payload.Data.ScoreDelta)
}

func TestGetModelGatewayScoreHistoryPrependsRuntimeCurrentScore(t *testing.T) {
	db := setupModelGatewayReplayControllerTestDB(t)
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps)
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	require.NotNil(t, runtimeDeps)
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.5",
		UpstreamModel:         "mimo-v1",
		ChannelID:             88,
		Group:                 "codex-plus",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "mimo_codex_chat|responses_via_chat",
	}
	runtimeDeps.SnapshotStore.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        1,
		SuccessScore:       1,
		SpeedScore:         0.03,
		TTFTMs:             45000,
		DurationMs:         46000,
		CostRatio:          0.05,
		GroupPriorityRatio: 1,
		SampleCount:        4,
	})
	require.NoError(t, db.Create(&model.Channel{Id: 88, Name: "dora", Status: 1}).Error)

	oldMeta, err := common.Marshal(map[string]any{
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:   88,
				ChannelName: "dora",
				Group:       "codex-plus",
				RuntimeKey: core.RuntimeKey{
					RequestedModel: "gpt-5.5",
					ChannelID:      88,
					Group:          "codex-plus",
					EndpointType:   constant.EndpointTypeOpenAI,
				},
				Available:      true,
				ScoreTotal:     0.709,
				ScoreBreakdown: map[string]float64{"success": 0.8, "speed": 0.45},
				SuccessScore:   0.8,
				SpeedScore:     0.45,
				SampleCount:    0,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      common.GetTimestamp() - 30,
		RequestId:      "score-cold",
		RequestedGroup: "codex-plus",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		PolicyMode:     "active",
		SmartHandled:   true,
		RequestMeta:    string(oldMeta),
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/observability/score-history", GetModelGatewayScoreHistory)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/observability/score-history?hours=1&limit=10&channel_id=88&requested_model=gpt-5.5&group=codex-plus", nil)
	router.ServeHTTP(resp, req)

	var payload modelGatewayScoreHistoryAPIResponse
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &payload), resp.Body.String())
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 2)
	require.Equal(t, "runtime_current", payload.Data.Items[0].Source)
	require.Equal(t, "runtime-current", payload.Data.Items[0].RequestID)
	require.Equal(t, 4, payload.Data.Items[0].SampleCount)
	require.Equal(t, 45000.0, payload.Data.Items[0].TTFTMs)
	require.Less(t, payload.Data.Items[0].ScoreTotal, 0.35)
	require.Equal(t, 0.78, payload.Data.Items[0].ScoreBreakdown["ttft_penalty"])
	require.Equal(t, "score-cold", payload.Data.Items[1].RequestID)
	require.NotZero(t, payload.Data.ScoreDelta)
}

func decodeModelGatewayObservabilityResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelGatewayObservabilityAPIResponse {
	t.Helper()
	var payload modelGatewayObservabilityAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

type modelGatewayRuntimeStatusAPIResponse struct {
	Success bool                                            `json:"success"`
	Message string                                          `json:"message"`
	Data    modelgatewayobservability.RuntimeStatusResponse `json:"data"`
}

func decodeModelGatewayRuntimeStatusResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelGatewayRuntimeStatusAPIResponse {
	t.Helper()
	var payload modelGatewayRuntimeStatusAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

func requireAggregate(t *testing.T, items []ModelGatewayObservabilityAggregate, key string, attempts int64, successes int64, failures int64) ModelGatewayObservabilityAggregate {
	t.Helper()
	for _, item := range items {
		if item.Key != key {
			continue
		}
		require.Equal(t, attempts, item.Attempts, fmt.Sprintf("attempts for %s", key))
		require.Equal(t, successes, item.Successes, fmt.Sprintf("successes for %s", key))
		require.Equal(t, failures, item.Failures, fmt.Sprintf("failures for %s", key))
		return item
	}
	require.Failf(t, "aggregate not found", "key=%s items=%+v", key, items)
	return ModelGatewayObservabilityAggregate{}
}

func requireObservabilityMetaAggregate(t *testing.T, items []ModelGatewayObservabilityAggregate, key string, queueEnabled int64, queued int64, avgQueueWaitMs int64, stickyRoutes int64, stickyRetained int64, stickyBroken int64, cacheAffinity int64) {
	t.Helper()
	for _, item := range items {
		if item.Key != key {
			continue
		}
		require.Equal(t, queueEnabled, item.QueueEnabledDispatches, fmt.Sprintf("queue enabled for %s", key))
		require.Equal(t, queued, item.QueuedDispatches, fmt.Sprintf("queued dispatches for %s", key))
		require.Equal(t, avgQueueWaitMs, item.AvgQueueWaitMs, fmt.Sprintf("avg queue wait for %s", key))
		require.Equal(t, stickyRoutes, item.StickyRoutes, fmt.Sprintf("sticky routes for %s", key))
		require.Equal(t, stickyRetained, item.StickyRetained, fmt.Sprintf("sticky retained for %s", key))
		require.Equal(t, stickyBroken, item.StickyBroken, fmt.Sprintf("sticky broken for %s", key))
		require.Equal(t, cacheAffinity, item.CacheAffinityRoutes, fmt.Sprintf("cache affinity for %s", key))
		return
	}
	require.Failf(t, "aggregate not found", "key=%s items=%+v", key, items)
}

func requireModelGatewayTrendWithRecords(t *testing.T, items []ModelGatewayObservabilityTrendPoint, records int64) ModelGatewayObservabilityTrendPoint {
	t.Helper()
	for _, item := range items {
		if item.Records == records {
			require.Greater(t, item.BucketStart, int64(0))
			require.Greater(t, item.BucketEnd, item.BucketStart)
			return item
		}
	}
	require.Failf(t, "trend bucket not found", "records=%d items=%+v", records, items)
	return ModelGatewayObservabilityTrendPoint{}
}

func requireModelGatewayUserRequestTrendWithRequests(t *testing.T, items []ModelGatewayUserRequestTrendPoint, requests int64) ModelGatewayUserRequestTrendPoint {
	t.Helper()
	for _, item := range items {
		if item.Requests == requests {
			require.Greater(t, item.BucketStart, int64(0))
			require.Greater(t, item.BucketEnd, item.BucketStart)
			return item
		}
	}
	require.Failf(t, "user request trend bucket not found", "requests=%d items=%+v", requests, items)
	return ModelGatewayUserRequestTrendPoint{}
}

func requireUserRequestAggregate(t *testing.T, items []ModelGatewayUserRequestAggregate, key string, requests int64, successes int64, finalFailures int64, recovered int64) {
	t.Helper()
	for _, item := range items {
		if item.Key != key {
			continue
		}
		require.Equal(t, requests, item.Requests, fmt.Sprintf("requests for %s", key))
		require.Equal(t, successes, item.Successes, fmt.Sprintf("successes for %s", key))
		require.Equal(t, finalFailures, item.FinalFailures, fmt.Sprintf("final failures for %s", key))
		require.Equal(t, recovered, item.Recovered, fmt.Sprintf("recovered for %s", key))
		if key == "gpt-5.5" || key == "vip" {
			require.Equal(t, int64(250), item.AvgTTFTMs, fmt.Sprintf("avg ttft for %s", key))
			require.Equal(t, int64(320), item.P95TTFTMs, fmt.Sprintf("p95 ttft for %s", key))
		}
		if key == "claude-4" || key == "default" {
			require.Equal(t, int64(520), item.AvgTTFTMs, fmt.Sprintf("avg ttft for %s", key))
			require.Equal(t, int64(520), item.P95TTFTMs, fmt.Sprintf("p95 ttft for %s", key))
		}
		return
	}
	require.Failf(t, "user request aggregate not found", "key=%s items=%+v", key, items)
}

func requireTrendRejectReason(t *testing.T, items []ModelGatewayTrendReasonCount, reason string, count int64) {
	t.Helper()
	for _, item := range items {
		if item.Reason != reason {
			continue
		}
		require.Equal(t, count, item.Count, fmt.Sprintf("reject reason count for %s", reason))
		return
	}
	require.Failf(t, "reject reason not found", "reason=%s items=%+v", reason, items)
}

func requireRiskEvent(t *testing.T, items []ModelGatewayRiskEvent, source string, eventType string, status string, reason string) {
	t.Helper()
	for _, item := range items {
		if item.Source != source || item.EventType != eventType || item.Status != status {
			continue
		}
		if reason != "" && item.Reason != reason {
			continue
		}
		require.NotEmpty(t, item.Severity, fmt.Sprintf("risk severity for %s/%s", source, eventType))
		require.Greater(t, item.Count, int64(0), fmt.Sprintf("risk count for %s/%s", source, eventType))
		return
	}
	require.Failf(t, "risk event not found", "source=%s type=%s status=%s reason=%s items=%+v", source, eventType, status, reason, items)
}

func requireRiskStatusCount(t *testing.T, items []ModelGatewayRiskStatusCount, status string, minCount int64) {
	t.Helper()
	for _, item := range items {
		if item.Status != status {
			continue
		}
		require.GreaterOrEqual(t, item.Count, minCount, fmt.Sprintf("risk status count for %s", status))
		require.NotEmpty(t, item.Severity, fmt.Sprintf("risk status severity for %s", status))
		return
	}
	require.Failf(t, "risk status count not found", "status=%s items=%+v", status, items)
}
