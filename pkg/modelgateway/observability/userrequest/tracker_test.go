package userrequest

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, EventStarted, events[1].Kind)
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
