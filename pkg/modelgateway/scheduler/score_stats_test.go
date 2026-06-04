package scheduler

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/stretchr/testify/require"
)

func TestScoreSampleDecisionSkipsSchedulerExhausted(t *testing.T) {
	decision := scoreSampleDecision(core.AttemptResult{
		StatusCode:    http.StatusTooManyRequests,
		ErrorCategory: core.ErrorCategorySchedulerExhausted,
		RetryAction:   "stop",
	})

	require.False(t, decision.ScoreSample)
	require.False(t, decision.CircuitSample)
	require.False(t, decision.RealUserMetric)
	require.Equal(t, core.ErrorCategorySchedulerExhausted, decision.SkipReason)
	require.Equal(t, core.ErrorCategorySchedulerExhausted, decision.Reason)
}
