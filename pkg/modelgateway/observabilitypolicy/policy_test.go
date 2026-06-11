package observabilitypolicy

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/stretchr/testify/require"
)

func TestDefaultPolicyKeepsPerformanceModeErrorsOnly(t *testing.T) {
	restore := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	defer restore()

	policy := Current()

	require.True(t, policy.PerformanceModeEnabled)
	require.Equal(t, scheduler_setting.ObservabilityDiagnosticLevelErrorsOnly, policy.DiagnosticLevel)
	require.False(t, FullDiagnosticsEnabled())
	require.False(t, CandidateDetailEnabled())
	require.False(t, ScoreEventEnabled())
	require.False(t, ClientRequestTraceEnabled())
	require.False(t, ShouldPersistDispatchRecord())
	require.True(t, CompactAttemptDiagnosticsEnabled())
}

func TestFullPolicyRestoresDiagnosticRecorders(t *testing.T) {
	setting := scheduler_setting.DefaultSetting()
	setting.ObservabilityPerformanceModeEnabled = false
	setting.ObservabilityDiagnosticLevel = scheduler_setting.ObservabilityDiagnosticLevelErrorsOnly
	restore := scheduler_setting.SetSettingForTest(setting)
	defer restore()

	require.True(t, FullDiagnosticsEnabled())
	require.True(t, CandidateDetailEnabled())
	require.True(t, ScoreEventEnabled())
	require.True(t, ClientRequestTraceEnabled())
	require.True(t, ShouldPersistDispatchRecord())
	require.False(t, CompactAttemptDiagnosticsEnabled())
}

func TestMinimalPolicySuppressesOptionalSwitches(t *testing.T) {
	setting := scheduler_setting.DefaultSetting()
	setting.ObservabilityDiagnosticLevel = scheduler_setting.ObservabilityDiagnosticLevelMinimal
	setting.ObservabilityCandidateDetailEnabled = true
	setting.ObservabilityScoreEventEnabled = true
	setting.ObservabilityClientRequestTraceEnabled = true
	restore := scheduler_setting.SetSettingForTest(setting)
	defer restore()

	policy := Current()

	require.True(t, policy.MinimalDiagnosticsEnabled())
	require.False(t, FullDiagnosticsEnabled())
	require.False(t, CandidateDetailEnabled())
	require.False(t, ScoreEventEnabled())
	require.False(t, ClientRequestTraceEnabled())
	require.False(t, ShouldPersistDispatchRecord())
	require.True(t, CompactAttemptDiagnosticsEnabled())
}
