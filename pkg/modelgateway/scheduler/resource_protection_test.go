package scheduler_test

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResourceProtectionDisabledKeepsNormalSelection(t *testing.T) {
	selector := newResourceProtectionTestSelector(t, false, false)

	plan, handled, apiErr := selector.Select(nil, nil, core.GroupSmartPolicy{
		Mode:            core.ModeActive,
		RequestedGroup:  "codex-plus",
		CandidateGroups: []string{"codex-plus"},
		Strategy:        core.StrategySpeedFirst,
	})

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.False(t, plan.ResourceProtectionEnabled)
	require.Empty(t, plan.ResourceProtectionPhase)
}

func TestResourceProtectionPrefersConfiguredPrimaryResource(t *testing.T) {
	selector := newResourceProtectionTestSelector(t, false, false)

	plan, handled, apiErr := selector.Select(nil, nil, resourceProtectionPolicy())

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.True(t, plan.ResourceProtectionEnabled)
	require.Equal(t, core.ResourceProtectionPhasePrimaryHit, plan.ResourceProtectionPhase)
	require.Equal(t, core.ResourceProtectionRolePrimary, plan.ResourceProtectionRole)
	require.Zero(t, plan.QueueWaitMs)
}

func TestResourceProtectionQueuesPrimaryWhenOnlyPrimaryIsSaturated(t *testing.T) {
	selector := newResourceProtectionTestSelector(t, true, false)

	plan, handled, apiErr := selector.Select(nil, nil, resourceProtectionPolicy())

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.Channel.Id)
	require.Equal(t, core.ResourceProtectionPhasePrimarySaturatedWait, plan.ResourceProtectionPhase)
	require.Equal(t, core.ResourceProtectionReasonPrimarySaturated, plan.ResourceProtectionReason)
	require.Equal(t, 3500, plan.QueueWaitMs)
	require.True(t, plan.QueueEnabled)
	require.Equal(t, 7, plan.QueueCapacity)
	require.Equal(t, 7, plan.PrimaryQueueMaxDepth)
}

func TestResourceProtectionAllowsFallbackAfterPrimaryWaitTimeout(t *testing.T) {
	selector := newResourceProtectionTestSelector(t, true, false)
	ctx := newResourceProtectionGinContext()
	core.AllowResourceProtectionFallback(ctx, core.ResourceProtectionReasonPrimaryWaitTimeout)

	plan, handled, apiErr := selector.Select(ctx, nil, resourceProtectionPolicy())

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.Equal(t, core.ResourceProtectionPhaseFallbackAfterTimeout, plan.ResourceProtectionPhase)
	require.Equal(t, core.ResourceProtectionRoleFallback, plan.ResourceProtectionRole)
	require.Equal(t, core.ResourceProtectionReasonPrimaryWaitTimeout, plan.ResourceProtectionReason)
	require.Zero(t, plan.QueueWaitMs)
}

func TestResourceProtectionFallsBackImmediatelyWhenPrimaryFailed(t *testing.T) {
	selector := newResourceProtectionTestSelector(t, false, true)

	plan, handled, apiErr := selector.Select(nil, nil, resourceProtectionPolicy())

	require.Nil(t, apiErr)
	require.True(t, handled)
	require.NotNil(t, plan)
	require.Equal(t, 2, plan.Channel.Id)
	require.Equal(t, core.ResourceProtectionPhasePrimaryFailureFallback, plan.ResourceProtectionPhase)
	require.Equal(t, core.ResourceProtectionReasonPrimaryFailure, plan.ResourceProtectionReason)
	require.Equal(t, core.ResourceProtectionRoleFallback, plan.ResourceProtectionRole)
}

func newResourceProtectionTestSelector(t *testing.T, primarySaturated bool, primaryCircuitOpen bool) *scheduler.DefaultSmartChannelSelector {
	t.Helper()
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	primaryKey := resourceProtectionRuntimeKey(1)
	fallbackKey := resourceProtectionRuntimeKey(2)
	primary := core.RuntimeSnapshot{
		Key:                       primaryKey,
		SuccessRate:               0.98,
		TTFTMs:                    450,
		DurationMs:                2400,
		TokensPerSecond:           80,
		ActiveConcurrency:         0,
		MaxConcurrency:            2,
		EffectiveConcurrencyLimit: 2,
		CostRatio:                 0.05,
		GroupPriorityRatio:        1,
		SampleCount:               20,
	}
	if primarySaturated {
		primary.ActiveConcurrency = 2
		primary.QueueDepth = 1
		primary.QueueCapacity = 4
		primary.QueueTimeoutMs = 1800
	}
	if primaryCircuitOpen {
		primary.CircuitOpen = true
		primary.CircuitState = core.CircuitStateOpen
	}
	store.Put(primary)
	store.Put(core.RuntimeSnapshot{
		Key:                       fallbackKey,
		SuccessRate:               0.99,
		TTFTMs:                    160,
		DurationMs:                900,
		TokensPerSecond:           120,
		ActiveConcurrency:         0,
		MaxConcurrency:            4,
		EffectiveConcurrencyLimit: 4,
		CostRatio:                 1.5,
		GroupPriorityRatio:        1,
		SampleCount:               20,
	})
	return scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{Channel: &model.Channel{Id: 1, Name: "primary-low-cost"}, Group: "codex-plus", RuntimeKey: primaryKey},
			{Channel: &model.Channel{Id: 2, Name: "fallback-high-cost"}, Group: "codex-plus", RuntimeKey: fallbackKey},
		}),
		store,
		scheduler.DefaultScoreWeights(),
	)
}

func resourceProtectionPolicy() core.GroupSmartPolicy {
	return core.GroupSmartPolicy{
		Mode:                      core.ModeActive,
		RequestedGroup:            "codex-plus",
		CandidateGroups:           []string{"codex-plus"},
		Strategy:                  core.StrategyBalanced,
		ResourceProtectionEnabled: true,
		PrimaryChannelIDs:         []int{1},
		PrimaryWaitTimeoutMs:      3500,
		PrimaryQueueMaxDepth:      7,
	}
}

func resourceProtectionRuntimeKey(channelID int) core.RuntimeKey {
	return core.RuntimeKey{
		RequestedModel: "gpt-5.5",
		UpstreamModel:  "gpt-5.5",
		ChannelID:      channelID,
		Group:          "codex-plus",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
}

func newResourceProtectionGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx
}
