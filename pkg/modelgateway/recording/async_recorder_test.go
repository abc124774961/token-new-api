package recording

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAsyncExecutionRecorderRecordsDispatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Record(context.Background(), core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestedGroup: "default",
			ModelName:      "gpt-4.1",
			EndpointType:   constant.EndpointTypeOpenAI,
		},
		Policy: core.GroupSmartPolicy{
			Mode:            core.ModeShadow,
			AutoMode:        core.AutoModeSequential,
			Strategy:        core.StrategyBalanced,
			CandidateGroups: []string{"default"},
		},
		Plan: &core.DispatchPlan{
			Channel:                &model.Channel{Id: 2, Name: "smart"},
			SelectedGroup:          "default",
			RuntimeKey:             core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "mimo-v1", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI},
			ProviderProfile:        "mimo_codex_chat",
			ProxyMode:              "responses_via_chat",
			ScoreTotal:             0.88,
			ScoreBreakdown:         map[string]float64{"completion_rate": 0.9},
			QueueEnabled:           true,
			QueueDepth:             1,
			QueueCapacity:          8,
			StickySource:           "prompt_cache_key",
			StickyKeyFP:            "abc123",
			StickyRetained:         true,
			CacheAffinity:          true,
			SelectedReason:         "weighted_score",
			RequiresCodexImageTool: true,
			RequiredTools: []string{
				core.DispatchRequiredToolCodexImageGeneration,
			},
			CandidateFilterConditions: []string{
				core.DispatchFilterConditionCodexImageGenerationTool,
			},
			Candidates: []core.CandidateExplanation{
				{
					ChannelID:       2,
					ChannelName:     "smart",
					Group:           "default",
					UpstreamModel:   "mimo-v1",
					ProviderProfile: "mimo_codex_chat",
					ProxyMode:       "responses_via_chat",
					RuntimeKey:      core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "mimo-v1", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI},
					Available:       true,
					ScoreTotal:      0.88,
					ScoreBreakdown:  map[string]float64{"completion_rate": 0.9},
					Selected:        true,
				},
			},
		},
		Actual:      &model.Channel{Id: 1, Name: "legacy"},
		ActualGroup: "default",
		Shadow:      true,
		RecordedAt:  time.Now(),
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.First(&record).Error)
	require.True(t, record.Shadow)
	require.True(t, record.SmartHandled)
	require.Equal(t, 2, record.ChannelId)
	require.Equal(t, 1, record.ActualChannelId)
	require.Equal(t, "default", record.SelectedGroup)
	require.Contains(t, record.ScoreBreakdown, "completion_rate")
	require.Contains(t, record.RequestMeta, "mimo_codex_chat")
	require.Contains(t, record.RequestMeta, "responses_via_chat")
	require.Contains(t, record.RequestMeta, "prompt_cache_key")
	require.Contains(t, record.RequestMeta, "candidate_explanations")
	require.Contains(t, record.RequestMeta, "mimo-v1")
	var requestMeta dispatchRequestMeta
	require.NoError(t, common.UnmarshalJsonStr(record.RequestMeta, &requestMeta))
	require.False(t, requestMeta.RequiresCodexImageTool)
	require.Empty(t, requestMeta.RequiredTools)
	require.Empty(t, requestMeta.CandidateFilterConditions)
}

func TestChannelAccountUsageEventFromAttemptUsesCurrentTimeWhenObservedAtUnset(t *testing.T) {
	before := time.Now().Unix()
	event := channelAccountUsageEventFromAttempt(core.AttemptResult{
		RequestID:  "req-no-observed-at",
		ChannelID:  22,
		Success:    true,
		StatusCode: 200,
	})
	after := time.Now().Unix()

	require.GreaterOrEqual(t, event.CompletedAt, before)
	require.LessOrEqual(t, event.CompletedAt, after)
	require.Greater(t, event.CreatedAt, int64(0))
	require.Greater(t, event.UpdatedAt, int64(0))
}

func TestAsyncExecutionRecorderDoesNotDropResultWhenQueueFull(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := &AsyncExecutionRecorder{queue: make(chan event, 1)}
	recorder.offer(event{record: &core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:    "req-queued-dispatch",
			ModelName:    "gpt-5.5",
			EndpointType: constant.EndpointTypeOpenAI,
		},
		RecordedAt: time.Now(),
	}})
	recorder.offer(event{result: &core.AttemptResult{
		RequestID:      "req-overflow-result",
		AttemptIndex:   0,
		ChannelID:      42,
		ChannelName:    "overflow-channel",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
		Success:        true,
		RetryAction:    "complete",
		Duration:       250 * time.Millisecond,
		TTFT:           80 * time.Millisecond,
	}})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-overflow-result").First(&summary).Error
		return err == nil && summary.CompletedAt > 0 && summary.FinalSuccess
	}, 2*time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-overflow-result").First(&summary).Error)
	require.Equal(t, "overflow-channel", summary.FinalChannelName)
	require.Equal(t, int64(250), summary.DurationMs)
	require.Equal(t, int64(80), summary.TTFTMs)
}

func TestAsyncExecutionRecorderRecordsRetryRoutingIntentMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	intent := core.NewFirstByteTimeoutRetryRoutingIntent(11, "slow-first-byte", 0)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:     "req-retry-intent-meta",
		AttemptIndex:  1,
		ChannelID:     12,
		ChannelName:   "healthy-channel",
		SelectedGroup: "default",
		ModelName:     "gpt-5.5",
		Success:       true,
		RetryAction:   "complete",
		Plan: &core.DispatchPlan{
			Channel:                 &model.Channel{Id: 12, Name: "healthy-channel"},
			SelectedGroup:           "default",
			SelectedReason:          "score_items_retry_intent",
			QueueEnabled:            true,
			QueuePriority:           core.RetryRoutingQueuePriority,
			RetryRoutingIntent:      intent,
			RetryIntentApplied:      true,
			RetryQueuePriorityBoost: true,
			Candidates: []core.CandidateExplanation{
				{
					ChannelID:          12,
					ChannelName:        "healthy-channel",
					Available:          true,
					Selected:           true,
					RetryIntentApplied: true,
					RetryIntentReason:  core.RelayAttemptCancelReasonFirstByteTimeout,
					RoutingScoreBreakdown: map[string]float64{
						"retry_intent_recovery": 0.99,
					},
				},
			},
		},
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "req-retry-intent-meta").Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-retry-intent-meta").First(&record).Error)
	require.Contains(t, record.RequestMeta, `"retry_routing_intent"`)
	require.Contains(t, record.RequestMeta, `"retry_intent_applied":true`)
	require.Contains(t, record.RequestMeta, `"retry_queue_priority_boost":true`)
	require.Contains(t, record.RequestMeta, `"queue_priority":1000`)
	require.Contains(t, record.RequestMeta, `"retry_intent_recovery"`)
}

func TestAsyncExecutionRecorderRecordsAttemptFlowMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:                      "req-flow",
		AttemptIndex:                   0,
		ChannelID:                      8,
		RequestedGroup:                 "auto",
		SelectedGroup:                  "vip",
		ModelName:                      "gpt-5.5",
		StatusCode:                     429,
		ErrorCode:                      "bad_response_status_code",
		ErrorType:                      "openai_error",
		ErrorMessage:                   "Too many pending requests, please retry later",
		ErrorCategory:                  "upstream_concurrency_limit",
		RetryAction:                    "switch_channel",
		WillRetry:                      true,
		ConcurrencyLimited:             true,
		ActiveConcurrency:              47,
		ConfiguredConcurrencyLimit:     64,
		LearnedConcurrencyLimit:        46,
		LearnedConcurrencyLimitChanged: true,
		UsedChannels:                   []string{"8"},
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "req-flow").Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-flow").First(&record).Error)
	require.Equal(t, "req-flow", record.RequestId)
	require.Equal(t, 429, record.StatusCode)
	require.Contains(t, record.RequestMeta, "upstream_concurrency_limit")
	require.Contains(t, record.RequestMeta, "switch_channel")
	require.Contains(t, record.RequestMeta, "learned_concurrency_limit")
	require.Contains(t, record.RequestMeta, "Too many pending requests")

	var summaries int64
	require.NoError(t, db.Model(&model.ModelGatewayUserRequestSummary{}).Where("request_id = ?", "req-flow").Count(&summaries).Error)
	require.Equal(t, int64(0), summaries)
}

func TestAsyncExecutionRecorderDoesNotSummarizeRetryingStreamInterruptedAttempt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:         "req-retrying-stream-interrupted",
		AttemptIndex:      0,
		ChannelID:         12,
		ChannelName:       "freeyourtokens-plus",
		RequestedGroup:    "codex-plus",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		StatusCode:        502,
		ErrorCategory:     "stream_interrupted",
		RetryAction:       "switch_channel",
		WillRetry:         true,
		StreamInterrupted: true,
		Duration:          3120 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var records int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).
			Where("request_id = ?", "req-retrying-stream-interrupted").
			Count(&records).Error)
		return records == 1
	}, time.Second, 10*time.Millisecond)

	var summaries int64
	require.NoError(t, db.Model(&model.ModelGatewayUserRequestSummary{}).
		Where("request_id = ?", "req-retrying-stream-interrupted").
		Count(&summaries).Error)
	require.Equal(t, int64(0), summaries)
}

func TestAsyncExecutionRecorderFinalizesMisflaggedStopRetryAttempt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "req-stop-retry-misflag",
		AttemptIndex:   1,
		ChannelID:      42,
		ChannelName:    "self-built-pool",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		StatusCode:     http.StatusInternalServerError,
		ErrorCategory:  core.ErrorCategorySchedulerExhausted,
		RetryAction:    "stop",
		WillRetry:      true,
		Duration:       2 * time.Second,
		TTFT:           4900 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-stop-retry-misflag").First(&summary).Error
		return err == nil && summary.CompletedAt > 0
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-stop-retry-misflag").First(&summary).Error)
	require.False(t, summary.FinalSuccess)
	require.Equal(t, 2, summary.Attempts)
	require.Equal(t, http.StatusInternalServerError, summary.FinalStatusCode)
	require.Equal(t, model.ModelGatewayUserRequestErrorSchedulerExhausted, summary.FinalErrorCategory)
	require.Equal(t, int64(2000), summary.DurationMs)
	require.Equal(t, int64(4900), summary.TTFTMs)
}

func TestAsyncExecutionRecorderAttemptPreservesDispatchScoreExplanation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	plan := &core.DispatchPlan{
		Channel:           &model.Channel{Id: 12, Name: "score-channel"},
		RequestedGroup:    "auto",
		SelectedGroup:     "codex-plus",
		RuntimeKey:        core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 12, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI},
		ProviderProfile:   "mimo_codex_chat",
		ProxyMode:         "responses_via_chat",
		ScoreTotal:        0.952,
		ScoreBreakdown:    map[string]float64{"completion_rate": 1, "ttft_latency": 0.84},
		RoutingScoreTotal: 0.951,
		SelectedReason:    "score_items",
		PolicyMode:        "active",
		AutoMode:          core.AutoModeSequential,
		Strategy:          core.StrategyBalanced,
		QueueEnabled:      true,
		QueueDepth:        1,
		QueueCapacity:     10,
		Candidates: []core.CandidateExplanation{
			{
				ChannelID:      12,
				ChannelName:    "score-channel",
				Group:          "codex-plus",
				UpstreamModel:  "gpt-5.5",
				RuntimeKey:     core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: 12, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI},
				Available:      true,
				Selected:       true,
				ScoreTotal:     0.952,
				ScoreBreakdown: map[string]float64{"completion_rate": 1, "ttft_latency": 0.84},
				ScoreItems: []core.ScoreItem{
					{Key: "completion_rate", Score: 1, Weight: 0.4, WeightedScore: 0.4},
				},
			},
		},
	}
	recorder.Report(context.Background(), core.AttemptResult{
		Plan:           plan,
		RequestID:      "req-score-explanation",
		AttemptIndex:   0,
		ChannelID:      12,
		ChannelName:    "score-channel",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		EndpointType:   constant.EndpointTypeOpenAI,
		Success:        true,
		Duration:       1200 * time.Millisecond,
		TTFT:           240 * time.Millisecond,
		RetryAction:    "complete",
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "req-score-explanation").Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-score-explanation").First(&record).Error)
	require.True(t, record.SmartHandled)
	require.Equal(t, "active", record.PolicyMode)
	require.Equal(t, 0.952, record.ScoreTotal)
	require.Contains(t, record.ScoreBreakdown, "completion_rate")
	require.Contains(t, record.RequestMeta, "candidate_explanations")
	require.Contains(t, record.RequestMeta, "score_items")
	require.Contains(t, record.RequestMeta, "retry_action")
}

func TestAsyncExecutionRecorderRecordsFirstByteTimeoutRetryReason(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:     "req-first-byte-timeout",
		AttemptIndex:  0,
		ChannelID:     11,
		ChannelName:   "slow-first-byte",
		SelectedGroup: "default",
		ModelName:     "gpt-5.5",
		StatusCode:    504,
		ErrorCode:     "channel:response_time_exceeded",
		ErrorCategory: "timeout",
		RetryAction:   "switch_channel",
		RetryReason:   "first_byte_timeout",
		WillRetry:     true,
		Duration:      20 * time.Second,
	})
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:       "req-first-byte-timeout",
		AttemptIndex:    1,
		ChannelID:       12,
		ChannelName:     "healthy-channel",
		SelectedGroup:   "default",
		ModelName:       "gpt-5.5",
		Success:         true,
		RetryAction:     "complete",
		Duration:        900 * time.Millisecond,
		TTFT:            180 * time.Millisecond,
		RequestDuration: 21 * time.Second,
		RequestTTFT:     20*time.Second + 180*time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-first-byte-timeout").First(&summary).Error
		return err == nil && summary.Attempts == 2 && summary.FinalSuccess && summary.Recovered
	}, time.Second, 10*time.Millisecond)

	var firstAttemptRecord model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ? AND attempt_index = ?", "req-first-byte-timeout", 0).First(&firstAttemptRecord).Error)
	require.Equal(t, int64(20000), firstAttemptRecord.DurationMs)
	require.Contains(t, firstAttemptRecord.RequestMeta, `"retry_reason":"first_byte_timeout"`)
	require.Contains(t, firstAttemptRecord.RequestMeta, `"retry_action":"switch_channel"`)
	var finalAttemptRecord model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ? AND attempt_index = ?", "req-first-byte-timeout", 1).First(&finalAttemptRecord).Error)
	require.Equal(t, int64(900), finalAttemptRecord.DurationMs)
	require.Equal(t, int64(180), finalAttemptRecord.TTFTMs)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-first-byte-timeout").First(&summary).Error)
	require.True(t, summary.FinalSuccess)
	require.True(t, summary.Recovered)
	require.False(t, summary.ClientAborted)
	require.Equal(t, 12, summary.FinalChannelID)
	require.Equal(t, int64(21000), summary.DurationMs)
	require.Equal(t, int64(20180), summary.TTFTMs)
}

func TestAsyncExecutionRecorderRecordsAttemptTimingMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:              "req-timing",
		AttemptIndex:           0,
		ChannelID:              4,
		ChannelName:            "timing-channel",
		RequestedGroup:         "codex-plus",
		SelectedGroup:          "codex-plus",
		ModelName:              "gpt-5.4",
		Success:                true,
		RetryAction:            "complete",
		Duration:               24 * time.Second,
		TTFT:                   12 * time.Second,
		QueueWait:              9 * time.Second,
		RelayToFirstByte:       2 * time.Second,
		RelayTotal:             13 * time.Second,
		UpstreamResponseHeader: 1500 * time.Millisecond,
		RequestBodyPrepare:     320 * time.Millisecond,
		RequestBodyBytes:       2 << 20,
		RequestBodyStorage:     "disk",
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "req-timing").Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-timing").First(&record).Error)
	require.Equal(t, int64(24000), record.DurationMs)
	require.Equal(t, int64(12000), record.TTFTMs)
	require.Contains(t, record.RequestMeta, `"timing"`)
	require.Contains(t, record.RequestMeta, `"queue_wait_ms":9000`)
	require.Contains(t, record.RequestMeta, `"relay_to_first_byte_ms":2000`)
	require.Contains(t, record.RequestMeta, `"relay_total_ms":13000`)
	require.Contains(t, record.RequestMeta, `"upstream_response_header_ms":1500`)
	require.Contains(t, record.RequestMeta, `"upstream_first_event_wait_ms":500`)
	require.Contains(t, record.RequestMeta, `"request_body_prepare_ms":320`)
	require.Contains(t, record.RequestMeta, `"pre_first_byte_ms":11000`)
	require.Contains(t, record.RequestMeta, `"post_first_byte_ms":11000`)
	require.Contains(t, record.RequestMeta, `"request_body_bytes":2097152`)
	require.Contains(t, record.RequestMeta, `"request_body_storage":"disk"`)
	require.Contains(t, record.RequestMeta, `"request_body_size_likely_latency":true`)
}

func TestAsyncExecutionRecorderTimingMetaFallbackDoesNotDoubleCountQueueWait(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "req-timing-fallback",
		AttemptIndex:   0,
		ChannelID:      4,
		ChannelName:    "timing-channel",
		RequestedGroup: "codex-plus",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.4",
		Success:        true,
		RetryAction:    "complete",
		Duration:       24 * time.Second,
		TTFT:           12 * time.Second,
		QueueWait:      9 * time.Second,
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "req-timing-fallback").Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-timing-fallback").First(&record).Error)
	require.Contains(t, record.RequestMeta, `"timing"`)
	require.Contains(t, record.RequestMeta, `"queue_wait_ms":9000`)
	require.Contains(t, record.RequestMeta, `"relay_to_first_byte_ms":3000`)
	require.Contains(t, record.RequestMeta, `"relay_total_ms":15000`)
	require.Contains(t, record.RequestMeta, `"pre_first_byte_ms":12000`)
	require.Contains(t, record.RequestMeta, `"post_first_byte_ms":12000`)
	require.NotContains(t, record.RequestMeta, `"pre_first_byte_ms":21000`)
}

func TestAsyncExecutionRecorderSummarizesRecoveredUserRequest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "req-recovered",
		AttemptIndex:   0,
		ChannelID:      8,
		ChannelName:    "rate-limited-channel",
		RequestedGroup: "auto",
		SelectedGroup:  "vip",
		ModelName:      "gpt-5.5",
		StatusCode:     429,
		ErrorCategory:  "upstream_concurrency_limit",
		WillRetry:      true,
		RetryAction:    "switch_channel",
		Duration:       180 * time.Millisecond,
	})
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "req-recovered",
		AttemptIndex:   1,
		ChannelID:      9,
		ChannelName:    "healthy-channel",
		RequestedGroup: "auto",
		SelectedGroup:  "vip",
		ModelName:      "gpt-5.5",
		Success:        true,
		RetryAction:    "complete",
		Duration:       620 * time.Millisecond,
		TTFT:           120 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-recovered").First(&summary).Error
		return err == nil && summary.Attempts == 2 && summary.FinalSuccess && summary.Recovered
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-recovered").First(&summary).Error)
	require.Equal(t, 2, summary.Attempts)
	require.True(t, summary.FinalSuccess)
	require.True(t, summary.Recovered)
	require.Equal(t, int64(620), summary.DurationMs)
	require.Equal(t, int64(120), summary.TTFTMs)
	require.Equal(t, 0, summary.FinalStatusCode)
	require.Empty(t, summary.FinalErrorCategory)
	require.Equal(t, 9, summary.FinalChannelID)
	require.Equal(t, "healthy-channel", summary.FinalChannelName)
	require.Greater(t, summary.CompletedAt, int64(0))
}

func TestAsyncExecutionRecorderSameAttemptIndexCannotMarkRecoveredUserRequest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "req-same-index",
		AttemptIndex:   0,
		ChannelID:      8,
		ChannelName:    "busy-channel",
		RequestedGroup: "auto",
		SelectedGroup:  "vip",
		ModelName:      "gpt-5.5",
		StatusCode:     429,
		ErrorCategory:  "upstream_concurrency_limit",
		WillRetry:      true,
		RetryAction:    "switch_channel",
	})
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "req-same-index",
		AttemptIndex:   0,
		ChannelID:      9,
		ChannelName:    "healthy-channel",
		RequestedGroup: "auto",
		SelectedGroup:  "vip",
		ModelName:      "gpt-5.5",
		Success:        true,
		RetryAction:    "complete",
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-same-index").First(&summary).Error
		return err == nil
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-same-index").First(&summary).Error)
	require.True(t, summary.FinalSuccess)
	require.False(t, summary.Recovered)
	require.Equal(t, 1, summary.Attempts)
	require.Equal(t, 9, summary.FinalChannelID)
}

func TestAsyncExecutionRecorderRecordsHealthProbeUserRequestSummary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:      "probe-1",
		AttemptIndex:   0,
		ChannelID:      8,
		ChannelName:    "probe-channel",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		ModelName:      "gpt-4.1",
		Success:        true,
		StatusCode:     200,
		Duration:       450 * time.Millisecond,
		TTFT:           90 * time.Millisecond,
		IsHealthProbe:  true,
		ProbeReason:    "low_score",
	})

	require.Eventually(t, func() bool {
		var executions int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "probe-1").Count(&executions).Error)
		return executions == 1
	}, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		var summaryCount int64
		require.NoError(t, db.Model(&model.ModelGatewayUserRequestSummary{}).Where("request_id = ?", "probe-1").Count(&summaryCount).Error)
		return summaryCount == 1
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "probe-1").First(&summary).Error)
	require.True(t, summary.FinalSuccess)
	require.True(t, summary.IsHealthProbe)
	require.Equal(t, "low_score", summary.ProbeReason)
	require.Equal(t, "probe-channel", summary.FinalChannelName)
	require.Equal(t, "gpt-4.1", summary.RequestedModel)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "probe-1").First(&record).Error)
	require.Contains(t, record.RequestMeta, "is_health_probe")
	require.Contains(t, record.RequestMeta, "low_score")
}

func TestAsyncExecutionRecorderHealthProbeFailureIsProbeSummary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:     "probe-failed-1",
		AttemptIndex:  0,
		ChannelID:     8,
		ChannelName:   "probe-channel",
		SelectedGroup: "default",
		ModelName:     "gpt-4.1",
		StatusCode:    502,
		ErrorCategory: "upstream_error",
		Duration:      250 * time.Millisecond,
		TTFT:          70 * time.Millisecond,
		IsHealthProbe: true,
		ProbeReason:   "failure_avoidance",
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "probe-failed-1").First(&summary).Error
		return err == nil
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "probe-failed-1").First(&summary).Error)
	require.False(t, summary.FinalSuccess)
	require.True(t, summary.IsHealthProbe)
	require.Equal(t, "failure_avoidance", summary.ProbeReason)
	require.Equal(t, 502, summary.FinalStatusCode)
	require.Equal(t, model.ModelGatewayUserRequestErrorStreamInterrupted, summary.FinalErrorCategory)
	require.False(t, summary.Recovered)
}

func TestAsyncExecutionRecorderPersistsBalanceInsufficientMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:           "req-balance",
		AttemptIndex:        0,
		ChannelID:           8,
		ChannelName:         "balance-channel",
		RequestedGroup:      "default",
		SelectedGroup:       "default",
		ModelName:           "gpt-4.1",
		StatusCode:          403,
		ErrorCode:           "insufficient_user_quota",
		ErrorCategory:       "balance_or_quota",
		ErrorMessage:        "insufficient account balance",
		BalanceInsufficient: true,
		Duration:            120 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var executions int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Where("request_id = ?", "req-balance").Count(&executions).Error)
		return executions == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-balance").First(&record).Error)
	require.Contains(t, record.RequestMeta, "balance_insufficient")

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-balance").First(&summary).Error)
	require.False(t, summary.FinalSuccess)
	require.Equal(t, "balance_or_quota", summary.FinalErrorCategory)
}

func TestAsyncExecutionRecorderSummarizesClientAbortAsUserCanceled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:         "req-client-abort",
		AttemptIndex:      0,
		ChannelID:         8,
		RequestedGroup:    "auto",
		SelectedGroup:     "vip",
		ModelName:         "gpt-5.5",
		StatusCode:        499,
		ErrorCategory:     "client_aborted",
		RetryAction:       "client_aborted",
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          1400 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-client-abort").First(&summary).Error
		return err == nil && summary.StreamInterrupted && summary.ClientAborted && !summary.FinalSuccess
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-client-abort").First(&summary).Error)
	require.Equal(t, 1, summary.Attempts)
	require.False(t, summary.FinalSuccess)
	require.True(t, summary.ClientAborted)
	require.Equal(t, 499, summary.FinalStatusCode)
	require.Equal(t, model.ModelGatewayUserRequestErrorClientAborted, summary.FinalErrorCategory)
	require.Equal(t, int64(1400), summary.DurationMs)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-client-abort").First(&record).Error)
	require.True(t, record.StreamInterrupted)
	require.Contains(t, record.RequestMeta, "client_aborted")
}

func TestAsyncExecutionRecorderPersistsChannelInducedAbortWarningMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:                 "req-channel-induced-abort",
		AttemptIndex:              0,
		ChannelID:                 18,
		RequestedGroup:            "auto",
		SelectedGroup:             "codex-plus",
		ModelName:                 "gpt-5.5",
		StatusCode:                499,
		ErrorCategory:             core.ErrorCategoryChannelInducedClientAbort,
		WarningLevel:              core.WarningLevelWarning,
		WarningFlags:              []string{core.WarningFlagChannelInducedAbort, core.WarningFlagNoEffectiveFirstByte},
		WarningMessage:            "client aborted before effective response",
		ChannelInducedClientAbort: true,
		RetryAction:               "client_aborted",
		StreamInterrupted:         true,
		ClientAborted:             true,
		Duration:                  7 * time.Second,
	})

	require.Eventually(t, func() bool {
		var record model.ModelExecutionRecord
		err := db.Where("request_id = ?", "req-channel-induced-abort").First(&record).Error
		return err == nil && record.RequestMeta != ""
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ?", "req-channel-induced-abort").First(&record).Error)
	require.Equal(t, core.ErrorCategoryChannelInducedClientAbort, record.ErrorCategory)
	require.Contains(t, record.RequestMeta, `"warning_level":"warning"`)
	require.Contains(t, record.RequestMeta, `"channel_induced_client_abort":true`)
	require.Contains(t, record.RequestMeta, core.WarningFlagChannelInducedAbort)
	require.Contains(t, record.RequestMeta, core.WarningFlagNoEffectiveFirstByte)
}

func TestAsyncExecutionRecorderClientAbortOverridesStreamInterruptedCategory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:         "req-client-abort-stream-category",
		AttemptIndex:      0,
		ChannelID:         8,
		RequestedGroup:    "auto",
		SelectedGroup:     "vip",
		ModelName:         "gpt-5.5",
		ErrorCategory:     "stream_interrupted",
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          900 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-client-abort-stream-category").First(&summary).Error
		return err == nil && summary.FinalErrorCategory == model.ModelGatewayUserRequestErrorClientAborted
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-client-abort-stream-category").First(&summary).Error)
	require.True(t, summary.ClientAborted)
	require.Equal(t, model.ModelGatewayUserRequestErrorClientAborted, summary.FinalErrorCategory)
}

func TestAsyncExecutionRecorderDoesNotLetOlderFailedAttemptOverrideLaterClientAbort(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:         "req-switch-then-client-abort",
		AttemptIndex:      1,
		ChannelID:         4,
		ChannelName:       "toioto",
		RequestedGroup:    "codex-plus",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		StatusCode:        499,
		ErrorCategory:     "client_aborted",
		RetryAction:       "client_aborted",
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          3380 * time.Millisecond,
	})
	require.Eventually(t, func() bool {
		var summary model.ModelGatewayUserRequestSummary
		err := db.Where("request_id = ?", "req-switch-then-client-abort").First(&summary).Error
		return err == nil && summary.LastAttemptIndex == 1
	}, time.Second, 10*time.Millisecond)

	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:         "req-switch-then-client-abort",
		AttemptIndex:      0,
		ChannelID:         12,
		ChannelName:       "freeyourtokens-plus",
		RequestedGroup:    "codex-plus",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		StatusCode:        502,
		ErrorCategory:     "stream_interrupted",
		RetryAction:       "switch_channel",
		WillRetry:         true,
		StreamInterrupted: true,
		Duration:          3120 * time.Millisecond,
	})

	require.Eventually(t, func() bool {
		var records int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).
			Where("request_id = ?", "req-switch-then-client-abort").
			Count(&records).Error)
		return records == 2
	}, time.Second, 10*time.Millisecond)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-switch-then-client-abort").First(&summary).Error)
	require.Equal(t, 1, summary.LastAttemptIndex)
	require.Equal(t, 2, summary.Attempts)
	require.False(t, summary.FinalSuccess)
	require.True(t, summary.ClientAborted)
	require.Equal(t, 499, summary.FinalStatusCode)
	require.Equal(t, model.ModelGatewayUserRequestErrorClientAborted, summary.FinalErrorCategory)
	require.Equal(t, 4, summary.FinalChannelID)
	require.Equal(t, "toioto", summary.FinalChannelName)
}
