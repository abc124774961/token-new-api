package realtime

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability/userrequest"
	"github.com/QuantumNous/new-api/pkg/realtime"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type captureSubscriber struct {
	messages []realtime.ServerMessage
}

func (s *captureSubscriber) Send(message realtime.ServerMessage) bool {
	s.messages = append(s.messages, message)
	return true
}

func setupRealtimeTopicTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestMergeUserRequestRealtimeRecordsPrefersTerminalAndLimits(t *testing.T) {
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
			CompletedAt:    105,
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
	require.Equal(t, "req-pending", merged[0].RequestID)
	require.Equal(t, 12, merged[0].FinalChannelID)
	require.Equal(t, "live-channel", merged[0].FinalChannelName)
	require.Equal(t, "req-overlap", merged[1].RequestID)
	require.Equal(t, "success", merged[1].Status)
	require.Equal(t, int64(105), merged[1].CompletedAt)
}

func TestMergeUserRequestRealtimeRecordsTreatsExplicitFailedWithoutCompletedAtAsTerminal(t *testing.T) {
	completed := []controller.ModelGatewayUserRequestRecord{
		{
			RequestID:          "req-quota",
			CreatedAt:          100,
			FinalStatusCode:    403,
			FinalErrorCategory: "user_quota_exhausted",
			Status:             "user_quota_exhausted",
		},
	}
	pending := []userrequest.Record{
		{
			RequestID: "req-quota",
			CreatedAt: 120,
			Status:    userrequest.StatusProcessing,
		},
	}

	merged := mergeUserRequestRealtimeRecords(completed, pending, 10)

	require.Len(t, merged, 1)
	require.Equal(t, "req-quota", merged[0].RequestID)
	require.Equal(t, "user_quota_exhausted", merged[0].Status)
	require.Equal(t, 403, merged[0].FinalStatusCode)
}

func TestMergeUserRequestRealtimeRecordsTreatsSettlingAsTerminal(t *testing.T) {
	completed := []controller.ModelGatewayUserRequestRecord{
		{
			RequestID: "req-settling",
			CreatedAt: 100,
			Status:    userrequest.StatusSettling,
		},
	}
	pending := []userrequest.Record{
		{
			RequestID: "req-settling",
			CreatedAt: 120,
			Status:    userrequest.StatusProcessing,
		},
	}

	merged := mergeUserRequestRealtimeRecords(completed, pending, 10)

	require.Len(t, merged, 1)
	require.Equal(t, userrequest.StatusSettling, merged[0].Status)
}

func TestMergeUserRequestRealtimeRecordsTreatsSettlementTimeoutAsTerminal(t *testing.T) {
	completed := []controller.ModelGatewayUserRequestRecord{
		{
			RequestID: "req-settlement-timeout",
			CreatedAt: 100,
			Status:    userrequest.StatusSettlementTimeout,
		},
	}
	pending := []userrequest.Record{
		{
			RequestID: "req-settlement-timeout",
			CreatedAt: 120,
			Status:    userrequest.StatusProcessing,
		},
	}

	merged := mergeUserRequestRealtimeRecords(completed, pending, 10)

	require.Len(t, merged, 1)
	require.Equal(t, userrequest.StatusSettlementTimeout, merged[0].Status)
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
	require.True(t, params.matchesUserRequest(controller.ModelGatewayUserRequestRecord{
		CreatedAt:      now,
		RequestID:      "req-live",
		RequestedModel: "gpt-5.5",
		SelectedGroup:  "codex-plus",
		FinalChannelID: 12,
		IsHealthProbe:  true,
		Status:         "processing",
	}))
}

func TestParseParamsCarriesRecentOnlyLightOptions(t *testing.T) {
	parsed := parseParams(map[string]any{
		"lite":             true,
		"include_dispatch": "true",
		"recent_only":      float64(1),
	})

	require.True(t, parsed.Lite)
	require.True(t, parsed.IncludeDispatch)
	require.True(t, parsed.RecentOnly)
	require.Contains(t, parsed.key(), "lite=true")
	require.Contains(t, parsed.key(), "include_dispatch=true")
	require.Contains(t, parsed.key(), "recent_only=true")

	options := parsed.toControllerOptions()
	require.True(t, options.Lite)
	require.True(t, options.IncludeDispatch)
	require.True(t, options.RecentOnly)
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
			UserID:         42,
			RequestedModel: "gpt-5.5",
			RequestedGroup: "auto",
			SelectedGroup:  "codex-plus",
			IsHealthProbe:  true,
			ProbeReason:    "low_score",
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
	require.Equal(t, 42, delta.UserRequestsRecent[0].UserID)
	require.Equal(t, "processing", delta.UserRequestsRecent[0].Status)
	require.Equal(t, "codex-plus", delta.UserRequestsRecent[0].ActualGroup)
	require.Zero(t, delta.UserRequestsRecent[0].ActualGroupRatio)
	require.True(t, delta.UserRequestsRecent[0].IsHealthProbe)
	require.Equal(t, "low_score", delta.UserRequestsRecent[0].ProbeReason)
}

func TestMergeUserRequestRealtimeRecordsSortsByProcessingCreatedAndCompletedAt(t *testing.T) {
	merged := mergeUserRequestRealtimeRecords(
		[]controller.ModelGatewayUserRequestRecord{
			{
				RequestID:   "req-completed-newer-created",
				CreatedAt:   130,
				UpdatedAt:   10,
				CompletedAt: 100,
				Status:      "success",
			},
			{
				RequestID:   "req-completed-later-finished",
				CreatedAt:   100,
				UpdatedAt:   140,
				CompletedAt: 140,
				Status:      "success",
			},
		},
		[]userrequest.Record{
			{
				RequestID: "req-processing-newer-created",
				CreatedAt: 120,
				UpdatedAt: 125,
				TTFTMs:    1200,
				Status:    userrequest.StatusProcessing,
			},
			{
				RequestID: "req-processing-older-created",
				CreatedAt: 110,
				UpdatedAt: 999,
				Status:    userrequest.StatusProcessing,
			},
		},
		10,
	)

	require.Len(t, merged, 4)
	require.Equal(t, "req-processing-newer-created", merged[0].RequestID)
	require.Equal(t, int64(125), merged[0].UpdatedAt)
	require.Equal(t, int64(1200), merged[0].TTFTMs)
	require.Equal(t, "req-processing-older-created", merged[1].RequestID)
	require.Equal(t, int64(999), merged[1].UpdatedAt)
	require.Equal(t, "req-completed-later-finished", merged[2].RequestID)
	require.Equal(t, "req-completed-newer-created", merged[3].RequestID)
}

func TestTopicPublishesHealthProbeUserRequestDeltaByDefault(t *testing.T) {
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
			RequestID:      "req-ws-probe-visible",
			RequestedModel: "gpt-5.5",
			IsHealthProbe:  true,
			ProbeReason:    "low_score",
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
	require.Equal(t, "req-ws-probe-visible", delta.UserRequestsRecent[0].RequestID)
	require.True(t, delta.UserRequestsRecent[0].IsHealthProbe)
	require.Equal(t, "low_score", delta.UserRequestsRecent[0].ProbeReason)
}

func TestTopicFinishedUserRequestDeltaCarriesDispatchCandidates(t *testing.T) {
	db := setupRealtimeTopicTestDB(t)
	now := time.Now().Unix()
	requestMeta, err := common.Marshal(map[string]any{
		"candidate_explanations": []core.CandidateExplanation{
			{
				ChannelID:     12,
				ChannelName:   "score-channel",
				Group:         "codex-plus",
				UpstreamModel: "gpt-5.5",
				RuntimeKey: core.RuntimeKey{
					RequestedModel: "gpt-5.5",
					UpstreamModel:  "gpt-5.5",
					ChannelID:      12,
					Group:          "codex-plus",
					EndpointType:   constant.EndpointTypeOpenAI,
				},
				Available:      true,
				Selected:       true,
				ScoreTotal:     0.94,
				ScoreBreakdown: map[string]float64{"completion_rate": 1, "ttft_latency": 0.82},
				ScoreItems:     []core.ScoreItem{{Key: "completion_rate", Score: 1, Weight: 0.4, WeightedScore: 0.4}},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      now,
		RequestId:      "req-ws-finished-score",
		RequestedGroup: "auto",
		SelectedGroup:  "codex-plus",
		RequestedModel: "gpt-5.5",
		ChannelId:      12,
		ChannelName:    "score-channel",
		PolicyMode:     "active",
		SmartHandled:   true,
		ScoreTotal:     0.94,
		ScoreBreakdown: `{"completion_rate":1,"ttft_latency":0.82}`,
		RequestMeta:    string(requestMeta),
	}).Error)

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
		Kind: userrequest.EventFinished,
		Record: userrequest.Record{
			CreatedAt:        now,
			CompletedAt:      now,
			RequestID:        "req-ws-finished-score",
			RequestedModel:   "gpt-5.5",
			RequestedGroup:   "auto",
			SelectedGroup:    "codex-plus",
			FinalChannelID:   12,
			FinalChannelName: "score-channel",
			Attempts:         1,
			FinalSuccess:     true,
			Status:           userrequest.StatusSuccess,
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
	record := delta.UserRequestsRecent[0]
	require.NotNil(t, record.DispatchRecord)
	require.Len(t, record.DispatchRecord.CandidateExplanations, 1)
	require.Equal(t, 0.94, record.DispatchRecord.CandidateExplanations[0].ScoreTotal)
	require.Len(t, record.DispatchRecord.CandidateExplanations[0].ScoreItems, 1)
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

func TestUserRequestViewExecutionRecordMarksSnapshotPending(t *testing.T) {
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
	require.Eventually(t, func() bool {
		topic.mu.Lock()
		defer topic.mu.Unlock()
		return len(topic.pending) > 0
	}, time.Second, 10*time.Millisecond)
}
