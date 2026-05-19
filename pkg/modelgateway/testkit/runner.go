package testkit

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func RunDispatchScenario(t *testing.T, path string) {
	t.Helper()
	scenario, err := LoadDispatchScenario(path)
	require.NoError(t, err)
	runDispatchScenarioObject(t, scenario)
}

func runDispatchScenarioObject(t *testing.T, scenario *DispatchScenario) {
	t.Helper()
	require.NotNil(t, scenario)
	plan, handled, err := ExecuteDispatchScenario(scenario)
	require.NoError(t, err)
	require.Equal(t, scenario.Expected.Handled, handled, "scenario %s handled mismatch", scenario.Name)
	if !scenario.Expected.Handled {
		require.Nil(t, plan)
		return
	}
	require.NotNil(t, plan)
	require.NotNil(t, plan.Channel)
	require.Equal(t, scenario.Expected.SelectedChannelID, plan.Channel.Id)
	require.Equal(t, scenario.Expected.SelectedGroup, plan.SelectedGroup)
	assertDispatchPlanExpectations(t, scenario, plan)
}

func ExecuteDispatchScenario(scenario *DispatchScenario) (*core.DispatchPlan, bool, error) {
	if scenario == nil {
		return nil, false, fmt.Errorf("dispatch scenario is nil")
	}
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, scenario.Request.UserGroup)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 1)
	h := NewScenarioHarnessWithContext(scenario, ctx)
	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: scenario.Request.RequestedGroup,
		ModelName:  scenario.Request.ModelName,
	}
	plan, handled, apiErr := h.Facade.Select(ctx, param)
	if apiErr != nil {
		return plan, handled, fmt.Errorf("dispatch scenario %s failed: %s", scenario.Name, apiErr.Error())
	}
	return plan, handled, nil
}

func assertDispatchPlanExpectations(t *testing.T, scenario *DispatchScenario, plan *core.DispatchPlan) {
	t.Helper()
	if scenario.Expected.StickyRetained != nil {
		require.Equal(t, *scenario.Expected.StickyRetained, plan.StickyRetained)
	}
	if scenario.Expected.StickyBreak != "" {
		require.Equal(t, scenario.Expected.StickyBreak, plan.StickyBreak)
	}
	if scenario.Expected.CacheAffinity != nil {
		require.Equal(t, *scenario.Expected.CacheAffinity, plan.CacheAffinity)
	}
	for _, expected := range scenario.Expected.Candidates {
		actual, ok := findCandidateExplanation(plan.Candidates, expected.ChannelID)
		require.True(t, ok, "scenario %s missing candidate explanation for channel %d", scenario.Name, expected.ChannelID)
		if expected.Available != nil {
			require.Equal(t, *expected.Available, actual.Available, "scenario %s candidate %d available mismatch", scenario.Name, expected.ChannelID)
		}
		if expected.RejectReason != "" {
			require.Equal(t, expected.RejectReason, actual.RejectReason, "scenario %s candidate %d reject reason mismatch", scenario.Name, expected.ChannelID)
		}
		if expected.Selected != nil {
			require.Equal(t, *expected.Selected, actual.Selected, "scenario %s candidate %d selected mismatch", scenario.Name, expected.ChannelID)
		}
		if expected.ScoreTotal > 0 {
			require.InDelta(t, expected.ScoreTotal, actual.ScoreTotal, 0.0001, "scenario %s candidate %d score mismatch", scenario.Name, expected.ChannelID)
		}
	}
}

func findCandidateExplanation(candidates []core.CandidateExplanation, channelID int) (core.CandidateExplanation, bool) {
	for _, candidate := range candidates {
		if candidate.ChannelID == channelID {
			return candidate, true
		}
	}
	return core.CandidateExplanation{}, false
}

func DispatchScenarioPaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "testdata", "dispatch", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	return paths
}
