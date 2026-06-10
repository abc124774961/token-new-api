package userrequest

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTrackerStartSnapshotsProcessingWithoutDatabase(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	now := time.Now()
	tracker.Start(core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      "req-live",
			UserID:         42,
			RequestedGroup: "auto",
			ModelName:      "gpt-5.5",
		},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: now,
	})

	items := tracker.Snapshot(5, Filters{Model: "gpt-5.5", Group: "codex-plus"})
	require.Len(t, items, 1)
	require.Equal(t, "req-live", items[0].RequestID)
	require.Equal(t, 42, items[0].UserID)
	require.Equal(t, StatusProcessing, items[0].Status)
	require.Equal(t, now.Unix(), items[0].CreatedAt)
	require.Equal(t, now.Unix(), items[0].UpdatedAt)
	require.Zero(t, items[0].CompletedAt)
}

func TestTrackerObserveFirstByteUpdatesProcessingRecord(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	now := time.Now()
	tracker.Start(core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      "req-first-byte-live",
			RequestedGroup: "auto",
			ModelName:      "gpt-5.5",
		},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: now,
	})

	tracker.ObserveFirstByte(FirstByteObservation{
		RequestID:  "req-first-byte-live",
		ObservedAt: now.Add(5 * time.Second),
		TTFT:       1200 * time.Millisecond,
	})

	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, now.Add(5*time.Second).Unix(), items[0].UpdatedAt)
	require.Equal(t, int64(1200), items[0].TTFTMs)
	require.Equal(t, StatusProcessing, items[0].Status)
	require.Len(t, events, 2)
	require.Equal(t, EventUpdated, events[1].Kind)
	require.Equal(t, int64(1200), events[1].Record.TTFTMs)
}

func TestTrackerSnapshotSortsProcessingByCreatedAtNotUpdatedAt(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	now := time.Now()
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-older", ModelName: "gpt-5.5"},
		RecordedAt: now,
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-newer", ModelName: "gpt-5.5"},
		RecordedAt: now.Add(20 * time.Second),
	})
	tracker.ObserveFirstByte(FirstByteObservation{
		RequestID:  "req-older",
		ObservedAt: now.Add(40 * time.Second),
		TTFT:       time.Second,
	})

	items := tracker.Snapshot(10, Filters{})
	require.Len(t, items, 2)
	require.Equal(t, "req-newer", items[0].RequestID)
	require.Equal(t, "req-older", items[1].RequestID)
	require.Equal(t, now.Add(40*time.Second).Unix(), items[1].UpdatedAt)
}

func TestTrackerAppliesEarlyFirstByteWhenStartArrivesLater(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	now := time.Now()

	tracker.ObserveFirstByte(FirstByteObservation{
		RequestID:  "req-early-first-byte",
		ObservedAt: now.Add(2 * time.Second),
		TTFT:       800 * time.Millisecond,
	})
	tracker.Start(core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      "req-early-first-byte",
			RequestedGroup: "auto",
			ModelName:      "gpt-5.5",
		},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: now,
	})

	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, int64(800), items[0].TTFTMs)
	require.Equal(t, now.Add(2*time.Second).Unix(), items[0].UpdatedAt)
}

func TestTrackerObserveUpstreamCompletedMovesToSettling(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	now := time.Now()
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-upstream-done", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: now,
	})

	tracker.ObserveUpstreamCompleted(UpstreamCompletedObservation{
		RequestID:  "req-upstream-done",
		ObservedAt: now.Add(7 * time.Second),
		Duration:   7 * time.Second,
	})

	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, StatusSettling, items[0].Status)
	require.Equal(t, now.Add(7*time.Second).Unix(), items[0].CompletedAt)
	require.Equal(t, int64(7000), items[0].DurationMs)
	require.True(t, items[0].FinalSuccess)
	require.Len(t, events, 2)
	require.Equal(t, EventUpdated, events[1].Kind)
}

func TestTrackerSnapshotConvertsStaleSettlingToBillingPendingFinal(t *testing.T) {
	tracker := NewTracker(10, time.Hour)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	startedAt := time.Now().Add(-3 * time.Minute)
	completedAt := time.Now().Add(-2 * time.Minute)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-stale-settling", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: startedAt,
	})
	tracker.ObserveUpstreamCompleted(UpstreamCompletedObservation{
		RequestID:  "req-stale-settling",
		ObservedAt: completedAt,
		Duration:   completedAt.Sub(startedAt),
	})

	items := tracker.Snapshot(5, Filters{})

	require.Len(t, items, 1)
	require.Equal(t, "req-stale-settling", items[0].RequestID)
	require.Equal(t, StatusSettlementTimeout, items[0].Status)
	require.True(t, items[0].FinalSuccess)
	require.Equal(t, completedAt.Unix(), items[0].CompletedAt)
	require.Zero(t, items[0].FinalStatusCode)
	require.Empty(t, items[0].FinalErrorCategory)
	require.Len(t, events, 3)
	require.Equal(t, EventFinished, events[2].Kind)
	require.Equal(t, StatusSettlementTimeout, events[2].Record.Status)
}

func TestTrackerRecentSettlingStaysPending(t *testing.T) {
	tracker := NewTracker(10, time.Hour)
	startedAt := time.Now().Add(-20 * time.Second)
	completedAt := time.Now().Add(-5 * time.Second)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-recent-settling", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: startedAt,
	})
	tracker.ObserveUpstreamCompleted(UpstreamCompletedObservation{
		RequestID:  "req-recent-settling",
		ObservedAt: completedAt,
		Duration:   completedAt.Sub(startedAt),
	})

	items := tracker.Snapshot(5, Filters{})

	require.Len(t, items, 1)
	require.Equal(t, StatusSettling, items[0].Status)
}

func TestTrackerLateFinishReplacesStaleSettlingPlaceholder(t *testing.T) {
	tracker := NewTracker(10, time.Hour)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	startedAt := time.Now().Add(-3 * time.Minute)
	completedAt := time.Now().Add(-2 * time.Minute)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-late-settling", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: startedAt,
	})
	tracker.ObserveUpstreamCompleted(UpstreamCompletedObservation{
		RequestID:  "req-late-settling",
		ObservedAt: completedAt,
		Duration:   completedAt.Sub(startedAt),
	})
	require.Len(t, tracker.SweepStale(), 1)

	tracker.Finish(core.AttemptResult{
		RequestID:      "req-late-settling",
		AttemptIndex:   0,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		Success:        true,
	}, &model.ModelGatewayUserRequestSummary{
		Id:             19,
		CreatedAt:      startedAt.Unix(),
		CompletedAt:    completedAt.Unix(),
		RequestId:      "req-late-settling",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		Attempts:       1,
		FinalSuccess:   true,
		DurationMs:     completedAt.Sub(startedAt).Milliseconds(),
	})

	require.Empty(t, tracker.Snapshot(5, Filters{}))
	require.Equal(t, EventFinished, events[len(events)-1].Kind)
	require.Equal(t, StatusSuccess, events[len(events)-1].Record.Status)
	require.Equal(t, 19, events[len(events)-1].Record.ID)
}

func TestTrackerAppliesEarlyUpstreamCompletedWhenStartArrivesLater(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	now := time.Now()

	tracker.ObserveUpstreamCompleted(UpstreamCompletedObservation{
		RequestID:  "req-early-upstream-done",
		ObservedAt: now.Add(3 * time.Second),
		Duration:   3 * time.Second,
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-early-upstream-done", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: now,
	})

	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, StatusSettling, items[0].Status)
	require.Equal(t, now.Add(3*time.Second).Unix(), items[0].CompletedAt)
	require.Equal(t, int64(3000), items[0].DurationMs)
}

func TestTrackerFinishKeepsUpstreamCompletionDuration(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	startedAt := time.Unix(1000, 0)
	completedAt := startedAt.Add(4 * time.Second)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-finish-after-settle", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: startedAt,
	})
	tracker.ObserveUpstreamCompleted(UpstreamCompletedObservation{
		RequestID:  "req-finish-after-settle",
		ObservedAt: completedAt,
		Duration:   4 * time.Second,
	})

	tracker.Finish(core.AttemptResult{
		RequestID:      "req-finish-after-settle",
		AttemptIndex:   0,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		Success:        true,
	}, &model.ModelGatewayUserRequestSummary{
		Id:             11,
		CreatedAt:      startedAt.Unix(),
		CompletedAt:    startedAt.Add(12 * time.Second).Unix(),
		RequestId:      "req-finish-after-settle",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		Attempts:       1,
		FinalSuccess:   true,
		DurationMs:     12000,
		TTFTMs:         500,
	})

	require.Empty(t, tracker.Snapshot(5, Filters{}))
	require.Len(t, events, 3)
	require.Equal(t, EventFinished, events[2].Kind)
	require.Equal(t, StatusSuccess, events[2].Record.Status)
	require.Equal(t, completedAt.Unix(), events[2].Record.CompletedAt)
	require.Equal(t, int64(4000), events[2].Record.DurationMs)
}

func TestTrackerFinishRemovesProcessingAndPublishesFinalSummary(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-final", UserID: 88, RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: time.Unix(100, 0),
	})

	tracker.Finish(core.AttemptResult{
		RequestID:      "req-final",
		AttemptIndex:   1,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		Success:        true,
	}, &model.ModelGatewayUserRequestSummary{
		Id:               7,
		CreatedAt:        100,
		CompletedAt:      105,
		RequestId:        "req-final",
		RequestedGroup:   "auto",
		SelectedGroup:    "codex-plus",
		FinalChannelID:   18,
		FinalChannelName: "codex-fast",
		RequestedModel:   "gpt-5.5",
		Attempts:         2,
		FinalSuccess:     true,
		Recovered:        true,
		DurationMs:       5000,
		TTFTMs:           300,
	})

	require.Empty(t, tracker.Snapshot(5, Filters{}))
	require.Len(t, events, 2)
	require.Equal(t, EventStarted, events[0].Kind)
	require.Equal(t, EventFinished, events[1].Kind)
	require.Equal(t, StatusSuccess, events[1].Record.Status)
	require.True(t, events[1].Record.Recovered)
	require.Equal(t, 88, events[1].Record.UserID)
	require.Equal(t, 2, events[1].Record.Attempts)
	require.Equal(t, 18, events[1].Record.FinalChannelID)
	require.Equal(t, "codex-fast", events[1].Record.FinalChannelName)
	require.Equal(t, int64(5000), events[1].Record.DurationMs)
}

func TestTrackerFinishClientAbortRemovesProcessingAndPublishesFailedFinal(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-abort", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: time.Unix(200, 0),
	})

	tracker.Finish(core.AttemptResult{
		RequestID:         "req-abort",
		AttemptIndex:      0,
		RequestedGroup:    "auto",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		StatusCode:        499,
		ErrorCategory:     "client_aborted",
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          3 * time.Second,
	}, nil)

	require.Empty(t, tracker.Snapshot(5, Filters{}))
	require.Len(t, events, 2)
	require.Equal(t, EventFinished, events[1].Kind)
	require.Equal(t, "client_aborted", events[1].Record.Status)
	require.False(t, events[1].Record.FinalSuccess)
	require.True(t, events[1].Record.ClientAborted)
	require.Equal(t, "client_aborted", events[1].Record.FinalErrorCategory)
	require.Equal(t, int64(3000), events[1].Record.DurationMs)
	require.Equal(t, 1, events[1].Record.Attempts)
}

func TestTrackerRetryingStreamInterruptedAttemptStaysProcessing(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-retry-stream", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: time.Now(),
	})

	tracker.Finish(core.AttemptResult{
		RequestID:         "req-retry-stream",
		AttemptIndex:      0,
		RequestedGroup:    "auto",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		ChannelID:         12,
		ChannelName:       "freeyourtokens-plus",
		StatusCode:        502,
		ErrorCategory:     "stream_interrupted",
		RetryAction:       "switch_channel",
		WillRetry:         true,
		StreamInterrupted: true,
	}, nil)

	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, "req-retry-stream", items[0].RequestID)
	require.Equal(t, StatusProcessing, items[0].Status)
	require.Equal(t, 1, items[0].Attempts)
	require.Equal(t, 12, items[0].FinalChannelID)
	require.Len(t, events, 2)
	require.Equal(t, EventStarted, events[1].Kind)
	require.Equal(t, StatusProcessing, events[1].Record.Status)
}

func TestTrackerFinalizesMisflaggedStopRetryAttempt(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-stop-retry", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: time.Now(),
	})

	tracker.Finish(core.AttemptResult{
		RequestID:      "req-stop-retry",
		AttemptIndex:   1,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		ChannelID:      42,
		ChannelName:    "self-built-pool",
		StatusCode:     http.StatusInternalServerError,
		ErrorCategory:  core.ErrorCategorySchedulerExhausted,
		RetryAction:    "stop",
		WillRetry:      true,
		Duration:       2 * time.Second,
		TTFT:           4900 * time.Millisecond,
	}, nil)

	require.Empty(t, tracker.Snapshot(5, Filters{}))
	require.Len(t, events, 2)
	require.Equal(t, EventFinished, events[1].Kind)
	require.Equal(t, StatusFailed, events[1].Record.Status)
	require.Equal(t, "req-stop-retry", events[1].Record.RequestID)
	require.Equal(t, model.ModelGatewayUserRequestErrorSchedulerExhausted, events[1].Record.FinalErrorCategory)
	require.Equal(t, int64(2000), events[1].Record.DurationMs)
	require.Equal(t, int64(4900), events[1].Record.TTFTMs)
}

func TestTrackerLateStartDoesNotReopenFinishedRequest(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})

	tracker.Finish(core.AttemptResult{
		RequestID:         "req-race",
		AttemptIndex:      0,
		RequestedGroup:    "auto",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		StatusCode:        499,
		ErrorCategory:     "client_aborted",
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          120 * time.Millisecond,
	}, nil)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-race", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: time.Now(),
	})

	require.Empty(t, tracker.Snapshot(5, Filters{}))
	require.Len(t, events, 1)
	require.Equal(t, EventFinished, events[0].Kind)
	require.Equal(t, "client_aborted", events[0].Record.Status)
	require.True(t, events[0].Record.ClientAborted)
}

func TestTrackerClientAbortOverridesStreamInterruptedCategory(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})

	tracker.Finish(core.AttemptResult{
		RequestID:         "req-abort-stream-category",
		AttemptIndex:      0,
		RequestedGroup:    "auto",
		SelectedGroup:     "codex-plus",
		ModelName:         "gpt-5.5",
		ErrorCategory:     "stream_interrupted",
		StreamInterrupted: true,
		ClientAborted:     true,
		Duration:          2 * time.Second,
	}, nil)

	require.Len(t, events, 1)
	require.Equal(t, "client_aborted", events[0].Record.Status)
	require.True(t, events[0].Record.ClientAborted)
	require.Equal(t, "client_aborted", events[0].Record.FinalErrorCategory)
}

func TestTrackerFinishWithoutPendingCarriesResultUserID(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	var finished Event
	tracker.AddObserver(func(event Event) {
		if event.Kind == EventFinished {
			finished = event
		}
	})

	tracker.Finish(core.AttemptResult{
		RequestID:      "req-result-user",
		UserID:         99,
		AttemptIndex:   0,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		Success:        true,
		Duration:       time.Second,
	}, nil)

	require.Equal(t, EventFinished, finished.Kind)
	require.Equal(t, "req-result-user", finished.Record.RequestID)
	require.Equal(t, 99, finished.Record.UserID)
	require.Equal(t, StatusSuccess, finished.Record.Status)
}

func TestTrackerPrunesExpiredProcessingRequests(t *testing.T) {
	tracker := NewTracker(10, time.Second)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-old", ModelName: "gpt-5.5"},
		RecordedAt: time.Now().Add(-2 * time.Second),
	})

	require.Empty(t, tracker.Snapshot(5, Filters{}))
}

func TestTrackerSweepStaleProcessingPublishesTimeoutFinal(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 1, 1, 0)
	defer restore()

	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	createdAt := time.Now().Add(-35 * time.Second)
	tracker.Start(core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      "req-stale",
			UserID:         77,
			RequestedGroup: "auto",
			ModelName:      "gpt-5.5",
		},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: createdAt,
	})

	records := tracker.SweepStale()

	require.Len(t, records, 1)
	require.Equal(t, "req-stale", records[0].RequestID)
	require.Equal(t, StatusFailed, records[0].Status)
	require.Equal(t, http.StatusGatewayTimeout, records[0].FinalStatusCode)
	require.Equal(t, model.ModelGatewayUserRequestErrorTimeout, records[0].FinalErrorCategory)
	require.Greater(t, records[0].CompletedAt, int64(0))
	require.GreaterOrEqual(t, records[0].DurationMs, int64(30000))
	require.Len(t, events, 2)
	require.Equal(t, EventFinished, events[1].Kind)
	require.Equal(t, StatusFailed, events[1].Record.Status)

	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, StatusFailed, items[0].Status)
	require.Equal(t, model.ModelGatewayUserRequestErrorTimeout, items[0].FinalErrorCategory)
}

func TestTrackerSweepStaleProcessingPersistsTimeoutSummary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayUserRequestSummary{}))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()
	restore := setTrackerTimeoutsForTest(t, true, 1, 1, 0)
	defer restore()

	tracker := NewTracker(10, time.Minute)
	tracker.Start(core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      "req-stale-persisted",
			RequestedGroup: "auto",
			ModelName:      "gpt-5.5",
		},
		Plan: &core.DispatchPlan{
			SelectedGroup: "codex-plus",
			Channel:       &model.Channel{Id: 42, Name: "stale-channel"},
		},
		RecordedAt: time.Now().Add(-35 * time.Second),
	})

	records := tracker.SweepStale()

	require.Len(t, records, 1)
	require.Greater(t, records[0].ID, 0)
	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", "req-stale-persisted").First(&summary).Error)
	require.Greater(t, summary.CompletedAt, int64(0))
	require.Equal(t, http.StatusGatewayTimeout, summary.FinalStatusCode)
	require.Equal(t, model.ModelGatewayUserRequestErrorTimeout, summary.FinalErrorCategory)
	require.Equal(t, "gpt-5.5", summary.RequestedModel)
	require.Equal(t, "codex-plus", summary.SelectedGroup)
	require.Equal(t, 42, summary.FinalChannelID)
	require.GreaterOrEqual(t, summary.DurationMs, int64(30000))
}

func TestTrackerSnapshotConvertsStaleProcessingToTimeoutFinal(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 1, 1, 0)
	defer restore()

	tracker := NewTracker(10, time.Minute)
	events := make([]Event, 0)
	tracker.AddObserver(func(event Event) {
		events = append(events, event)
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-stale-snapshot", ModelName: "gpt-5.5"},
		RecordedAt: time.Now().Add(-35 * time.Second),
	})

	items := tracker.Snapshot(5, Filters{})

	require.Len(t, items, 1)
	require.Equal(t, "req-stale-snapshot", items[0].RequestID)
	require.Equal(t, StatusFailed, items[0].Status)
	require.Equal(t, model.ModelGatewayUserRequestErrorTimeout, items[0].FinalErrorCategory)
	require.Len(t, events, 2)
	require.Equal(t, EventFinished, events[1].Kind)
}

func TestTrackerRecentActivityKeepsLongRunningStreamProcessing(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 1, 1, 0)
	defer restore()

	tracker := NewTracker(10, time.Minute)
	createdAt := time.Now().Add(-35 * time.Second)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-active", ModelName: "gpt-5.5"},
		RecordedAt: createdAt,
	})
	tracker.ObserveActivity(ActivityObservation{
		RequestID:  "req-active",
		ObservedAt: time.Now().Add(-5 * time.Second),
	})

	require.Empty(t, tracker.SweepStale())
	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, StatusProcessing, items[0].Status)
}

func TestTrackerStaleProcessingWithoutFirstByteUsesFirstByteTimeout(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 180, 900, 0)
	defer restore()

	tracker := NewTracker(10, time.Minute)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-stale-first-byte", ModelName: "gpt-5.5"},
		RecordedAt: time.Now().Add(-55 * time.Second),
	})

	records := tracker.SweepStale()

	require.Len(t, records, 1)
	require.Equal(t, "req-stale-first-byte", records[0].RequestID)
	require.Equal(t, StatusFailed, records[0].Status)
	require.Equal(t, model.ModelGatewayUserRequestErrorTimeout, records[0].FinalErrorCategory)
	require.GreaterOrEqual(t, records[0].DurationMs, int64(50000))
}

func TestTrackerImageRequestWithoutFirstByteUsesLongRunningTimeout(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 180, 900, 0)
	defer restore()

	tracker := NewTracker(10, time.Hour)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-image-long-running", ModelName: "gpt-image-2"},
		RecordedAt: time.Now().Add(-55 * time.Second),
	})

	require.Empty(t, tracker.SweepStale())
	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, StatusProcessing, items[0].Status)
	require.Zero(t, items[0].FinalStatusCode)
	require.Empty(t, items[0].FinalErrorCategory)
}

func TestTrackerStaleProcessingWithFirstByteKeepsStreamingTimeout(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 180, 900, 0)
	defer restore()

	tracker := NewTracker(10, time.Hour)
	createdAt := time.Now().Add(-55 * time.Second)
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-stream-after-first-byte", ModelName: "gpt-5.5"},
		RecordedAt: createdAt,
	})
	tracker.ObserveFirstByte(FirstByteObservation{
		RequestID:  "req-stream-after-first-byte",
		ObservedAt: createdAt.Add(2 * time.Second),
		TTFT:       2 * time.Second,
	})

	require.Empty(t, tracker.SweepStale())
	items := tracker.Snapshot(5, Filters{})
	require.Len(t, items, 1)
	require.Equal(t, StatusProcessing, items[0].Status)
	require.Equal(t, int64(2000), items[0].TTFTMs)
}

func TestFinalizeStaleActiveSummarySkipsTrackedPending(t *testing.T) {
	restore := setTrackerTimeoutsForTest(t, true, 1, 1, 0)
	defer restore()
	oldDefault := DefaultTracker
	DefaultTracker = NewTracker(10, time.Minute)
	defer func() {
		DefaultTracker = oldDefault
	}()

	createdAt := time.Now().Add(-35 * time.Second)
	DefaultTracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-tracked-active", ModelName: "gpt-5.5"},
		RecordedAt: createdAt,
	})

	summary := model.ModelGatewayUserRequestSummary{
		RequestId:      "req-tracked-active",
		CreatedAt:      createdAt.Unix(),
		UpdatedAt:      createdAt.Unix(),
		RequestedModel: "gpt-5.5",
	}
	updated, finalized := FinalizeStaleActiveSummary(summary, time.Now())

	require.False(t, finalized)
	require.Zero(t, updated.CompletedAt)
	require.Zero(t, updated.FinalStatusCode)
}

func TestTrackerCarriesExperienceIssueOnSuccessfulRequest(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	var finished Event
	tracker.AddObserver(func(event Event) {
		if event.Kind == EventFinished {
			finished = event
		}
	})
	tracker.Start(core.DispatchRecord{
		Request:    core.DispatchRequest{RequestID: "req-empty", RequestedGroup: "auto", ModelName: "gpt-5.5"},
		Plan:       &core.DispatchPlan{SelectedGroup: "codex-plus"},
		RecordedAt: time.Unix(300, 0),
	})

	tracker.Finish(core.AttemptResult{
		RequestID:       "req-empty",
		AttemptIndex:    0,
		RequestedGroup:  "auto",
		SelectedGroup:   "codex-plus",
		ModelName:       "gpt-5.5",
		Success:         true,
		EmptyOutput:     true,
		ExperienceIssue: "empty_output",
		Duration:        4 * time.Second,
	}, &model.ModelGatewayUserRequestSummary{
		Id:              9,
		CreatedAt:       300,
		CompletedAt:     304,
		RequestId:       "req-empty",
		RequestedGroup:  "auto",
		SelectedGroup:   "codex-plus",
		RequestedModel:  "gpt-5.5",
		Attempts:        1,
		FinalSuccess:    true,
		EmptyOutput:     true,
		ExperienceIssue: "empty_output",
		DurationMs:      4000,
	})

	require.Equal(t, StatusSuccess, finished.Record.Status)
	require.True(t, finished.Record.EmptyOutput)
	require.Equal(t, "empty_output", finished.Record.ExperienceIssue)
}

func TestTrackerCarriesHealthProbeMarker(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	var finished Event
	tracker.AddObserver(func(event Event) {
		if event.Kind == EventFinished {
			finished = event
		}
	})

	tracker.Finish(core.AttemptResult{
		RequestID:      "req-probe-finish",
		AttemptIndex:   0,
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		ModelName:      "gpt-5.5",
		Success:        true,
		IsHealthProbe:  true,
		ProbeReason:    "low_score",
		Duration:       time.Second,
		TTFT:           200 * time.Millisecond,
	}, &model.ModelGatewayUserRequestSummary{
		Id:             11,
		CreatedAt:      500,
		CompletedAt:    501,
		RequestId:      "req-probe-finish",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		Attempts:       1,
		FinalSuccess:   true,
		DurationMs:     1000,
		TTFTMs:         200,
	})

	require.True(t, finished.Record.IsHealthProbe)
	require.Equal(t, "low_score", finished.Record.ProbeReason)
	require.Equal(t, StatusProbe, finished.Record.Status)
}

func TestTrackerStartCarriesHealthProbeMarker(t *testing.T) {
	tracker := NewTracker(10, time.Minute)
	now := time.Now()
	tracker.Start(core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      "req-probe-live",
			RequestedGroup: "auto",
			ModelName:      "gpt-5.5",
		},
		Plan: &core.DispatchPlan{
			SelectedGroup: "codex-plus",
			IsHealthProbe: true,
			ProbeReason:   "low_score",
		},
		RecordedAt: now,
	})

	items := tracker.Snapshot(5, Filters{Model: "gpt-5.5", Group: "codex-plus"})
	require.Len(t, items, 1)
	require.Equal(t, "req-probe-live", items[0].RequestID)
	require.True(t, items[0].IsHealthProbe)
	require.Equal(t, "low_score", items[0].ProbeReason)
	require.Equal(t, StatusProcessing, items[0].Status)
}

func TestFiltersMatchHealthProbeRecordsByDefault(t *testing.T) {
	record := Record{
		RequestID:      "req-probe",
		RequestedModel: "gpt-5.5",
		IsHealthProbe:  true,
		Status:         StatusProcessing,
	}

	require.True(t, Filters{}.Match(record))
}

func setTrackerTimeoutsForTest(t *testing.T, totalEnabled bool, totalSeconds int, streamingSeconds int, relaySeconds int) func() {
	t.Helper()
	oldStreamingTimeout := constant.StreamingTimeout
	oldRelayTimeout := common.RelayTimeout
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		RelayTotalTimeoutEnabled: totalEnabled,
		RelayTotalTimeoutSeconds: totalSeconds,
	})
	constant.StreamingTimeout = streamingSeconds
	common.RelayTimeout = relaySeconds
	return func() {
		constant.StreamingTimeout = oldStreamingTimeout
		common.RelayTimeout = oldRelayTimeout
		restoreSetting()
	}
}
