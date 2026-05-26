package recording

import (
	"context"
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
			ScoreBreakdown:         map[string]float64{"success": 0.9},
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
					ScoreBreakdown:  map[string]float64{"success": 0.9},
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
	require.Contains(t, record.ScoreBreakdown, "success")
	require.Contains(t, record.RequestMeta, "mimo_codex_chat")
	require.Contains(t, record.RequestMeta, "responses_via_chat")
	require.Contains(t, record.RequestMeta, "prompt_cache_key")
	require.Contains(t, record.RequestMeta, "candidate_explanations")
	require.Contains(t, record.RequestMeta, "mimo-v1")
	var requestMeta dispatchRequestMeta
	require.NoError(t, common.UnmarshalJsonStr(record.RequestMeta, &requestMeta))
	require.True(t, requestMeta.RequiresCodexImageTool)
	require.Equal(t, []string{core.DispatchRequiredToolCodexImageGeneration}, requestMeta.RequiredTools)
	require.Equal(t, []string{core.DispatchFilterConditionCodexImageGenerationTool}, requestMeta.CandidateFilterConditions)
}

func TestAsyncExecutionRecorderRecordsAttemptFlowMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
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

func TestAsyncExecutionRecorderRecordsAttemptTimingMeta(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Report(context.Background(), core.AttemptResult{
		RequestID:        "req-timing",
		AttemptIndex:     0,
		ChannelID:        4,
		ChannelName:      "timing-channel",
		RequestedGroup:   "codex-plus",
		SelectedGroup:    "codex-plus",
		ModelName:        "gpt-5.4",
		Success:          true,
		RetryAction:      "complete",
		Duration:         24 * time.Second,
		TTFT:             12 * time.Second,
		QueueWait:        9 * time.Second,
		RelayToFirstByte: 2 * time.Second,
		RelayTotal:       13 * time.Second,
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
	require.Contains(t, record.RequestMeta, `"pre_first_byte_ms":11000`)
	require.Contains(t, record.RequestMeta, `"post_first_byte_ms":11000`)
}

func TestAsyncExecutionRecorderTimingMetaFallbackDoesNotDoubleCountQueueWait(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
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

func TestAsyncExecutionRecorderClientAbortOverridesStreamInterruptedCategory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
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
