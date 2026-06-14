package scheduler

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

func RecoverRuntimeSnapshotHealth(snapshot core.RuntimeSnapshot, now int64) core.RuntimeSnapshot {
	if now <= 0 {
		now = time.Now().Unix()
	}
	if snapshot.SampleCount < 3 {
		snapshot.SampleCount = 3
	}
	snapshot.ScoreStatsJSON = ""
	snapshot.RecentLatencySamples = RecoveredHealthLatencySamples(now)
	snapshot.SuccessRate = 1
	snapshot.TTFTMs = 800
	snapshot.DurationMs = 3000
	snapshot.TokensPerSecond = 80
	snapshot.EmptyOutputRate = 0
	snapshot.ExperienceIssueRate = 0
	snapshot.ActiveConcurrency = 0
	snapshot.QueueDepth = 0
	snapshot.EstimatedQueueWaitMs = 0
	snapshot.FirstBytePending = 0
	snapshot.SlowFirstBytePending = 0
	snapshot.OldestFirstByteWaitMs = 0
	if snapshot.CostRatio > 0 {
		snapshot.CostReferenceRatio = snapshot.CostRatio
	}
	snapshot.GroupPriorityRatio = 1
	snapshot.CircuitState = core.CircuitStateClosed
	snapshot.CircuitOpen = false
	snapshot.CircuitOpenUntil = 0
	snapshot.CircuitOpenReason = ""
	snapshot.CircuitFailureCount = 0
	snapshot.CircuitFailureRate = 0
	snapshot.CircuitSampleCount = 0
	snapshot.CircuitErrorCounts = nil
	snapshot.CircuitHalfOpenProbeUsed = 0
	snapshot.CircuitHalfOpenProbeMax = 0
	snapshot.Cooldown = false
	snapshot.FailureAvoidance = false
	snapshot.RecoverableQualityScore = 1
	snapshot.RecoverableQualityBaseline = 0
	snapshot.RecoverableQualityBaselineSamples = 0
	snapshot.RecoverableQualityDropRatio = 0
	snapshot.RecoverableQualityItemBaselines = nil
	snapshot.ProbeRecoveryPending = false
	snapshot.ProbeRecoverySuccessCount = 0
	snapshot.ProbeRecoveryRequired = 0
	snapshot.ProbeTriggerReason = ""
	snapshot.ProbeRecoveryPhase = ""
	snapshot.ProbeFastRecoveryAttempts = 0
	snapshot.ProbeAnomalyTriggerItems = nil
	snapshot.ConfigErrorIsolated = false
	snapshot.IsolationReason = ""
	snapshot.IsolationUntil = 0
	snapshot.AuthConfigErrorCount = 0
	snapshot.LastAuthConfigErrorAt = 0
	snapshot.LastRealSuccessAt = now
	snapshot.LastRealFailureAt = 0
	if snapshot.LastRealAttemptAt <= 0 {
		snapshot.LastRealAttemptAt = now
	}
	if snapshot.RealSampleCount30m <= 0 {
		snapshot.RealSampleCount30m = 1
	}
	if snapshot.LastProbeSuccessAt <= 0 {
		snapshot.LastProbeSuccessAt = now
	}
	return snapshot
}

func RecoveredHealthLatencySamples(now int64) []core.RuntimeLatencySample {
	if now <= 0 {
		now = common.GetTimestamp()
	}
	return []core.RuntimeLatencySample{
		{ObservedAt: now - 2, TTFTMs: 800, DurationMs: 3000},
		{ObservedAt: now - 1, TTFTMs: 800, DurationMs: 3000},
		{ObservedAt: now, TTFTMs: 800, DurationMs: 3000},
	}
}
