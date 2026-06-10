package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientEmptyOutputSwitchAvoidsRepeatedEmptyOutputChannel(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, ctx, nil)

	require.Equal(t, 32, plan.Channel.Id)
	rejected := candidateExplanationByChannel(t, plan.Candidates, 31)
	require.Equal(t, scheduler.ClientEmptyOutputSwitchReason, rejected.RejectReason)
	require.NotEmpty(t, rejected.ClientEmptyOutputSessionKey)
	require.Greater(t, rejected.ClientEmptyOutputAvoidUntil, now.Unix())
	require.Greater(t, rejected.ClientEmptyOutputRemainingSeconds, int64(0))
	selected := candidateExplanationByChannel(t, plan.Candidates, 32)
	require.True(t, selected.Available)
}

func TestClientEmptyOutputSwitchRequiresThreeSamplesWithinWindow(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second)

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, ctx, nil)

	require.Equal(t, 31, plan.Channel.Id)
	require.Empty(t, candidateExplanationByChannel(t, plan.Candidates, 31).RejectReason)
	require.Equal(t, 2, tracker.CountForTest(clientEmptyOutputSwitchRequestForContext(t, ctx), clientEmptyOutputSwitchCandidate(31), clientEmptyOutputSwitchSnapshot(31, 0.99)))
}

func TestClientEmptyOutputSwitchMarksSkippedWhenNoPeerCandidate(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })

	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	store.Put(clientEmptyOutputSwitchSnapshot(31, 0.99))
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{clientEmptyOutputSwitchCandidate(31)}),
		store,
		core.ScoreWeights{Success: 1},
	).WithClientEmptyOutputSwitchTracker(tracker)

	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		TokenGroup:   "vip",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
	}, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "vip",
		UserGroup:       "vip",
		CandidateGroups: []string{"vip"},
		Strategy:        core.StrategyBalanced,
	})

	require.Nil(t, apiErr)
	require.False(t, handled)
	require.Nil(t, plan)
	require.True(t, service.IsChannelSelectionSkipped(ctx, 31))
}

func TestClientEmptyOutputSwitchBreaksStickyRoute(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })

	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)
	req := clientEmptyOutputSwitchRequestForContext(t, ctx)
	sticky := scheduler.NewMemoryStickyRouter(scheduler.StickyRouterOptions{}, nil)
	sticky.Save(ctx, &req, &core.DispatchPlan{
		Channel:       &model.Channel{Id: 31, Name: "empty-output-a", Status: common.ChannelStatusEnabled},
		SelectedGroup: "vip",
		RuntimeKey:    clientEmptyOutputSwitchRuntimeKey(31),
	})

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, sticky, ctx, nil)

	require.Equal(t, 32, plan.Channel.Id)
	require.False(t, plan.StickyRetained)
	require.Equal(t, scheduler.ClientEmptyOutputSwitchReason, plan.StickyBreak)
	rejected := candidateExplanationByChannel(t, plan.Candidates, 31)
	require.Equal(t, scheduler.ClientEmptyOutputSwitchReason, rejected.RejectReason)
}

func TestClientEmptyOutputSwitchClearsAfterNonEmptySuccess(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)
	tracker.Report(context.Background(), core.AttemptResult{
		ClientSessionKey: core.SessionRoutingKeyFromGin(ctx),
		UserID:           7001,
		TokenID:          7101,
		ChannelID:        31,
		ModelName:        "gpt-5.5",
		SelectedGroup:    "vip",
		EndpointType:     constant.EndpointTypeOpenAI,
		Success:          true,
		ObservedAt:       now,
	})

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, ctx, nil)

	require.Equal(t, 31, plan.Channel.Id)
	require.Empty(t, candidateExplanationByChannel(t, plan.Candidates, 31).RejectReason)
}

func TestClientEmptyOutputSwitchExpiresAfterAvoidanceTTL(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)
	now = now.Add(11 * time.Minute)

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, ctx, nil)

	require.Equal(t, 31, plan.Channel.Id)
	require.Empty(t, candidateExplanationByChannel(t, plan.Candidates, 31).RejectReason)
}

func TestClientEmptyOutputSwitchIsSessionScoped(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	emptySession := newClientEmptyOutputSwitchContextForSession(t, "client-empty-output-switch-a")
	otherSession := newClientEmptyOutputSwitchContextForSession(t, "client-empty-output-switch-b")
	recordClientEmptyOutputSwitchSamples(t, tracker, emptySession, now, -20*time.Second, -10*time.Second, -5*time.Second)

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, otherSession, nil)

	require.Equal(t, 31, plan.Channel.Id)
	require.Empty(t, candidateExplanationByChannel(t, plan.Candidates, 31).RejectReason)
}

func TestClientEmptyOutputSwitchClearRemovesAvoidance(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)

	cleared := tracker.Clear(scheduler.ClientEmptyOutputSwitchScope{
		SessionKey:     core.SessionRoutingKeyFromGin(ctx),
		ChannelID:      31,
		RequestedModel: "gpt-5.5",
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAI,
	})
	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, ctx, nil)

	require.True(t, cleared)
	require.Equal(t, 31, plan.Channel.Id)
	require.Empty(t, candidateExplanationByChannel(t, plan.Candidates, 31).RejectReason)
}

func TestClientEmptyOutputSwitchFallsBackFromResourceProtectedPrimary(t *testing.T) {
	now := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)
	tracker := scheduler.NewClientEmptyOutputSwitchTracker(scheduler.ClientEmptyOutputSwitchConfig{}).
		WithNowForTest(func() time.Time { return now })
	ctx := newClientEmptyOutputSwitchContext(t)
	recordClientEmptyOutputSwitchSamples(t, tracker, ctx, now, -20*time.Second, -10*time.Second, -5*time.Second)

	plan := selectClientEmptyOutputSwitchPlan(t, tracker, nil, ctx, func(policy *core.GroupSmartPolicy) {
		policy.ResourceProtectionEnabled = true
		policy.PrimaryChannelIDs = []int{31}
		policy.FallbackChannelIDs = []int{32}
	})

	require.Equal(t, 32, plan.Channel.Id)
	require.Equal(t, core.ResourceProtectionRoleFallback, plan.ResourceProtectionRole)
	require.Equal(t, core.ResourceProtectionPhasePrimaryFailureFallback, plan.ResourceProtectionPhase)
}

func recordClientEmptyOutputSwitchSamples(t *testing.T, tracker *scheduler.ClientEmptyOutputSwitchTracker, ctx *gin.Context, now time.Time, offsets ...time.Duration) {
	t.Helper()
	sessionKey := core.SessionRoutingKeyFromGin(ctx)
	require.NotEmpty(t, sessionKey)
	for _, offset := range offsets {
		tracker.Report(context.Background(), core.AttemptResult{
			ClientSessionKey: sessionKey,
			UserID:           7001,
			TokenID:          7101,
			ChannelID:        31,
			ModelName:        "gpt-5.5",
			SelectedGroup:    "vip",
			EndpointType:     constant.EndpointTypeOpenAI,
			Success:          true,
			EmptyOutput:      true,
			ObservedAt:       now.Add(offset),
		})
	}
}

func selectClientEmptyOutputSwitchPlan(
	t *testing.T,
	tracker *scheduler.ClientEmptyOutputSwitchTracker,
	sticky core.StickyRouter,
	ctx *gin.Context,
	adjustPolicy func(*core.GroupSmartPolicy),
) *core.DispatchPlan {
	t.Helper()
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	store.Put(clientEmptyOutputSwitchSnapshot(31, 0.99))
	store.Put(clientEmptyOutputSwitchSnapshot(32, 0.90))
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			clientEmptyOutputSwitchCandidate(31),
			clientEmptyOutputSwitchCandidate(32),
		}),
		store,
		core.ScoreWeights{Success: 1},
	).WithClientEmptyOutputSwitchTracker(tracker)
	if sticky != nil {
		selector = selector.WithStickyRouter(sticky)
	}
	policy := core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "vip",
		UserGroup:       "vip",
		CandidateGroups: []string{"vip"},
		Strategy:        core.StrategyBalanced,
	}
	if adjustPolicy != nil {
		adjustPolicy(&policy)
	}
	plan, handled, apiErr := selector.Select(ctx, &service.RetryParam{
		TokenGroup:   "vip",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
	}, policy)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	return plan
}

func newClientEmptyOutputSwitchContext(t *testing.T) *gin.Context {
	t.Helper()
	return newClientEmptyOutputSwitchContextForSession(t, "client-empty-output-switch")
}

func newClientEmptyOutputSwitchContextForSession(t *testing.T, sessionID string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx := newStickyRequestContext(t, `{"session_id":"`+sessionID+`"}`, nil)
	common.SetContextKey(ctx, constant.ContextKeyUserId, 7001)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 7101)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "vip")
	return ctx
}

func clientEmptyOutputSwitchRequestForContext(t *testing.T, ctx *gin.Context) core.DispatchRequest {
	t.Helper()
	req := core.DispatchRequest{
		ClientSessionKey: core.SessionRoutingKeyFromGin(ctx),
		UserID:           7001,
		TokenID:          7101,
		RequestedGroup:   "vip",
		UserGroup:        "vip",
		ModelName:        "gpt-5.5",
		EndpointType:     constant.EndpointTypeOpenAI,
	}
	require.NotEmpty(t, req.ClientSessionKey)
	return req
}

func clientEmptyOutputSwitchCandidate(channelID int) core.Candidate {
	return core.Candidate{
		Channel:       &model.Channel{Id: channelID, Name: clientEmptyOutputSwitchChannelName(channelID), Status: common.ChannelStatusEnabled},
		Group:         "vip",
		UpstreamModel: "gpt-5.5",
		RuntimeKey:    clientEmptyOutputSwitchRuntimeKey(channelID),
	}
}

func clientEmptyOutputSwitchSnapshot(channelID int, successRate float64) core.RuntimeSnapshot {
	return core.RuntimeSnapshot{
		Key:                clientEmptyOutputSwitchRuntimeKey(channelID),
		SuccessRate:        successRate,
		TTFTMs:             300,
		DurationMs:         1200,
		TokensPerSecond:    80,
		SampleCount:        20,
		CostRatio:          1,
		GroupPriorityRatio: 1,
	}
}

func clientEmptyOutputSwitchRuntimeKey(channelID int) core.RuntimeKey {
	return core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      channelID,
		Group:          "vip",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
}

func clientEmptyOutputSwitchChannelName(channelID int) string {
	if channelID == 31 {
		return "empty-output-a"
	}
	return "empty-output-b"
}
