package realtime

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability/userrequest"
	"github.com/QuantumNous/new-api/pkg/realtime"
	"github.com/stretchr/testify/require"
)

type captureSubscriber struct {
	messages []realtime.ServerMessage
}

func (s *captureSubscriber) Send(message realtime.ServerMessage) bool {
	s.messages = append(s.messages, message)
	return true
}

func TestMergeUserRequestRealtimeRecordsPrefersPendingAndLimits(t *testing.T) {
	completed := []controller.ModelGatewayUserRequestRecord{
		{
			ID:             1,
			RequestID:      "req-complete",
			CreatedAt:      90,
			CompletedAt:    100,
			RequestedModel: "gpt-5.5",
			FinalSuccess:   true,
			Status:         "success",
		},
		{
			ID:             2,
			RequestID:      "req-overlap",
			CreatedAt:      80,
			CompletedAt:    95,
			RequestedModel: "gpt-5.5",
			FinalSuccess:   true,
			Status:         "success",
		},
	}
	pending := []userrequest.Record{
		{
			RequestID:        "req-pending",
			CreatedAt:        110,
			RequestedModel:   "gpt-5.5",
			FinalChannelID:   12,
			FinalChannelName: "live-channel",
			Status:           userrequest.StatusProcessing,
		},
		{
			RequestID:      "req-overlap",
			CreatedAt:      120,
			RequestedModel: "gpt-5.5",
			Status:         userrequest.StatusProcessing,
		},
	}

	merged := mergeUserRequestRealtimeRecords(completed, pending, 2)

	require.Len(t, merged, 2)
	require.Equal(t, "req-overlap", merged[0].RequestID)
	require.Equal(t, "processing", merged[0].Status)
	require.Equal(t, "req-pending", merged[1].RequestID)
	require.Equal(t, 12, merged[1].FinalChannelID)
	require.Equal(t, "live-channel", merged[1].FinalChannelName)
}

func TestParamsMatchesUserRequest(t *testing.T) {
	now := time.Now().Unix()
	params := params{
		Hours:     1,
		ViewMode:  "user_requests",
		Model:     "gpt-5.5",
		Group:     "codex-plus",
		RequestID: "req-live",
		ChannelID: 12,
	}

	require.True(t, params.matchesUserRequest(controller.ModelGatewayUserRequestRecord{
		CreatedAt:      now,
		RequestID:      "req-live",
		RequestedModel: "gpt-5.5",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		FinalChannelID: 12,
		Status:         "processing",
	}))
	require.False(t, params.matchesUserRequest(controller.ModelGatewayUserRequestRecord{
		CreatedAt:      now,
		RequestID:      "req-other",
		RequestedModel: "gpt-5.5",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		FinalChannelID: 13,
		Status:         "processing",
	}))
}

func TestTopicPublishesProcessingUserRequestDelta(t *testing.T) {
	topic := NewTopic()
	defer topic.Close()
	subscriber := &captureSubscriber{}
	topic.Subscribe(subscriber, realtime.Subscription{
		ID:    "sub-user",
		Topic: TopicName,
		Params: map[string]any{
			"view_mode":    "user_requests",
			"hours":        1,
			"recent_limit": 10,
		},
	})
	require.Eventually(t, func() bool {
		return len(subscriber.messages) > 0
	}, time.Second, 10*time.Millisecond)
	subscriber.messages = nil

	topic.PublishUserRequest(userrequest.Event{
		Kind: userrequest.EventStarted,
		Record: userrequest.Record{
			CreatedAt:      time.Now().Unix(),
			RequestID:      "req-ws-processing",
			RequestedModel: "gpt-5.5",
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			Status:         userrequest.StatusProcessing,
		},
	})

	require.Eventually(t, func() bool {
		return len(subscriber.messages) == 1
	}, time.Second, 10*time.Millisecond)
	message := subscriber.messages[0]
	require.Equal(t, realtime.MessageTypeDelta, message.Type)
	delta, ok := message.Data.(Delta)
	require.True(t, ok)
	require.Len(t, delta.UserRequestsRecent, 1)
	require.Equal(t, "req-ws-processing", delta.UserRequestsRecent[0].RequestID)
	require.Equal(t, "processing", delta.UserRequestsRecent[0].Status)
}

func TestProcessingUserRequestDeltaDoesNotMarkSnapshotPending(t *testing.T) {
	topic := NewTopic()
	defer topic.Close()
	subscriber := &captureSubscriber{}
	topic.Subscribe(subscriber, realtime.Subscription{
		ID:    "sub-user",
		Topic: TopicName,
		Params: map[string]any{
			"view_mode":    "user_requests",
			"hours":        1,
			"recent_limit": 10,
		},
	})
	require.Eventually(t, func() bool {
		return len(subscriber.messages) > 0
	}, time.Second, 10*time.Millisecond)
	subscriber.messages = nil

	topic.PublishUserRequest(userrequest.Event{
		Kind: userrequest.EventStarted,
		Record: userrequest.Record{
			CreatedAt:      time.Now().Unix(),
			RequestID:      "req-ws-processing",
			RequestedModel: "gpt-5.5",
			Status:         userrequest.StatusProcessing,
		},
	})

	require.Eventually(t, func() bool {
		return len(subscriber.messages) == 1
	}, time.Second, 10*time.Millisecond)
	require.Empty(t, topic.pending)
}

func TestUserRequestViewIgnoresExecutionRecordRealtime(t *testing.T) {
	topic := NewTopic()
	defer topic.Close()
	subscriber := &captureSubscriber{}
	topic.Subscribe(subscriber, realtime.Subscription{
		ID:    "sub-user",
		Topic: TopicName,
		Params: map[string]any{
			"view_mode":    "user_requests",
			"hours":        1,
			"recent_limit": 10,
		},
	})
	require.Eventually(t, func() bool {
		return len(subscriber.messages) > 0
	}, time.Second, 10*time.Millisecond)
	subscriber.messages = nil

	topic.Publish(model.ModelExecutionRecord{
		CreatedAt:      time.Now().Unix(),
		RequestId:      "req-attempt",
		RequestedModel: "gpt-5.5",
		RequestedGroup: "auto",
	})

	require.Never(t, func() bool {
		return len(subscriber.messages) > 0
	}, 50*time.Millisecond, 10*time.Millisecond)
	require.Empty(t, topic.pending)
}
