package observabilitypolicy

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type Policy struct {
	PerformanceModeEnabled    bool
	DiagnosticLevel           string
	ClientRequestTraceEnabled bool
	ScoreEventEnabled         bool
	CandidateDetailEnabled    bool
}

func Current() Policy {
	setting := scheduler_setting.GetSetting()
	policy := Policy{
		PerformanceModeEnabled:    setting.ObservabilityPerformanceModeEnabled,
		DiagnosticLevel:           scheduler_setting.NormalizeObservabilityDiagnosticLevel(setting.ObservabilityDiagnosticLevel),
		ClientRequestTraceEnabled: setting.ObservabilityClientRequestTraceEnabled,
		ScoreEventEnabled:         setting.ObservabilityScoreEventEnabled,
		CandidateDetailEnabled:    setting.ObservabilityCandidateDetailEnabled,
	}
	if shouldDefaultPerformanceMode(setting) {
		policy.PerformanceModeEnabled = true
	}
	return policy
}

func FullDiagnosticsEnabled() bool {
	return Current().FullDiagnosticsEnabled()
}

func CandidateDetailEnabled() bool {
	return Current().CandidateDetailEnabledForRuntime()
}

func ScoreEventEnabled() bool {
	return Current().ScoreEventEnabledForRuntime()
}

func ClientRequestTraceEnabled() bool {
	return Current().ClientRequestTraceEnabledForRuntime()
}

func ClientRequestTraceExplicitEnabled() bool {
	return Current().ClientRequestTraceEnabled
}

func ShouldPersistDispatchRecord() bool {
	policy := Current()
	return policy.CandidateDetailEnabledForRuntime()
}

func CompactAttemptDiagnosticsEnabled() bool {
	return !FullDiagnosticsEnabled()
}

func (p Policy) FullDiagnosticsEnabled() bool {
	return !p.PerformanceModeEnabled ||
		p.DiagnosticLevel == scheduler_setting.ObservabilityDiagnosticLevelFull
}

func (p Policy) MinimalDiagnosticsEnabled() bool {
	return p.PerformanceModeEnabled &&
		p.DiagnosticLevel == scheduler_setting.ObservabilityDiagnosticLevelMinimal
}

func (p Policy) CandidateDetailEnabledForRuntime() bool {
	if p.MinimalDiagnosticsEnabled() {
		return false
	}
	return p.FullDiagnosticsEnabled() || p.CandidateDetailEnabled
}

func (p Policy) ScoreEventEnabledForRuntime() bool {
	if p.MinimalDiagnosticsEnabled() {
		return false
	}
	return p.FullDiagnosticsEnabled() || p.ScoreEventEnabled
}

func (p Policy) ClientRequestTraceEnabledForRuntime() bool {
	if p.MinimalDiagnosticsEnabled() {
		return false
	}
	return p.FullDiagnosticsEnabled() || p.ClientRequestTraceEnabled
}

func shouldDefaultPerformanceMode(setting scheduler_setting.SchedulerSetting) bool {
	return strings.TrimSpace(setting.ObservabilityDiagnosticLevel) == "" &&
		!setting.ObservabilityPerformanceModeEnabled &&
		!setting.ObservabilityClientRequestTraceEnabled &&
		!setting.ObservabilityScoreEventEnabled &&
		!setting.ObservabilityCandidateDetailEnabled
}
