package scheduler

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	recoverableQualityBaselineAlpha      = 0.08
	recoverableQualityMinBaselineSamples = 5
	recoverableQualityDropRatioThreshold = 0.18
	recoverableQualityItemDropThreshold  = 0.18
	recoverableQualityRecoveredRatio     = 0.95
	scoreAnomalyFastProbeMaxAttempts     = 5
	scoreAnomalyFastProbeInterval        = time.Minute
)

var defaultRecoverableQualityItems = []string{
	scoreItemCompletionRate,
	scoreItemUpstreamErrorRate,
	scoreItemTTFTLatency,
	scoreItemDurationLatency,
	scoreItemFirstByteBacklog,
	scoreItemEmptyOutputRate,
	scoreItemStreamInterruptedRate,
}

type RecoverableQualityEvaluation struct {
	Valid        bool
	Score        float64
	ItemScores   map[string]float64
	MissingItems []string
}

func EvaluateRecoverableQuality(score core.ScoreResult, configuredItems []string) RecoverableQualityEvaluation {
	items := normalizeRecoverableQualityItems(configuredItems)
	if len(items) == 0 {
		return RecoverableQualityEvaluation{}
	}
	evaluation := recoverableQualityFromScoreItems(score.Items, items)
	if evaluation.Valid || len(evaluation.MissingItems) > 0 {
		return evaluation
	}
	return recoverableQualityFromScoreItems(score.RoutingItems, items)
}

func ScoreAnomalyFastProbeInterval() time.Duration {
	return scoreAnomalyFastProbeInterval
}

func ScoreAnomalyFastProbeMaxAttempts() int {
	return scoreAnomalyFastProbeMaxAttempts
}

func ScoreAnomalyFastProbeQuota(maxPerTick int) int {
	if maxPerTick <= 0 {
		maxPerTick = 5
	}
	quota := (maxPerTick*40 + 99) / 100
	if quota < 1 {
		quota = 1
	}
	if quota > 2 {
		quota = 2
	}
	return quota
}

func ScoreAnomalyFastProbePending(snapshot core.RuntimeSnapshot) bool {
	if strings.TrimSpace(snapshot.ProbeTriggerReason) != core.ProbeReasonScoreAnomalyFastProbe &&
		strings.TrimSpace(snapshot.ProbeRecoveryPhase) != core.ProbeRecoveryPhaseFastProbe {
		return false
	}
	if strings.TrimSpace(snapshot.ProbeRecoveryPhase) == core.ProbeRecoveryPhasePendingRealConfirmation {
		return false
	}
	if snapshot.ProbeFastRecoveryAttempts >= scoreAnomalyFastProbeMaxAttempts {
		return false
	}
	return len(snapshot.ProbeAnomalyTriggerItems) > 0
}

func ScoreAnomalyRecovered(snapshot core.RuntimeSnapshot, evaluation RecoverableQualityEvaluation) bool {
	if !evaluation.Valid {
		return false
	}
	if len(snapshot.ProbeAnomalyTriggerItems) == 0 {
		return snapshot.RecoverableQualityBaseline <= 0 ||
			evaluation.Score >= snapshot.RecoverableQualityBaseline*recoverableQualityRecoveredRatio
	}
	for _, item := range snapshot.ProbeAnomalyTriggerItems {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == "recoverable_quality_score" {
			if snapshot.RecoverableQualityBaseline <= 0 ||
				evaluation.Score < snapshot.RecoverableQualityBaseline*recoverableQualityRecoveredRatio {
				return false
			}
			continue
		}
		itemScore, ok := evaluation.ItemScores[item]
		if !ok {
			return false
		}
		if baseline := snapshot.RecoverableQualityItemBaselines[item]; baseline > 0 &&
			baseline-itemScore > recoverableQualityItemDropThreshold {
			return false
		}
	}
	return true
}

func recoverableQualityFromScoreItems(scoreItems []core.ScoreItem, recoverableItems []string) RecoverableQualityEvaluation {
	recoverable := make(map[string]struct{}, len(recoverableItems))
	for _, item := range recoverableItems {
		recoverable[item] = struct{}{}
	}
	itemScores := make(map[string]float64, len(recoverableItems))
	missingSet := map[string]struct{}{}
	totalWeight := 0.0
	weighted := 0.0
	for _, item := range scoreItems {
		key := strings.TrimSpace(item.Key)
		if _, ok := recoverable[key]; !ok || item.Weight <= 0 {
			continue
		}
		if strings.TrimSpace(item.MissingReason) != "" {
			missingSet[key] = struct{}{}
			continue
		}
		score := clampRecoverableQualityScore(item.Score)
		itemScores[key] = score
		totalWeight += item.Weight
		weighted += score * item.Weight
	}
	missing := make([]string, 0, len(missingSet))
	for item := range missingSet {
		missing = append(missing, item)
	}
	sort.Strings(missing)
	if totalWeight <= 0 {
		return RecoverableQualityEvaluation{ItemScores: itemScores, MissingItems: missing}
	}
	return RecoverableQualityEvaluation{
		Valid:        true,
		Score:        round4(weighted / totalWeight),
		ItemScores:   itemScores,
		MissingItems: missing,
	}
}

func normalizeRecoverableQualityItems(items []string) []string {
	if items == nil {
		return append([]string(nil), defaultRecoverableQualityItems...)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if !validRecoverableQualityItem(item) {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return recoverableQualityItemOrder(out[i]) < recoverableQualityItemOrder(out[j])
	})
	return out
}

func validRecoverableQualityItem(item string) bool {
	switch item {
	case scoreItemCompletionRate,
		scoreItemUpstreamErrorRate,
		scoreItemTTFTLatency,
		scoreItemDurationLatency,
		scoreItemFirstByteBacklog,
		scoreItemEmptyOutputRate,
		scoreItemStreamInterruptedRate:
		return true
	default:
		return false
	}
}

func recoverableQualityItemOrder(item string) int {
	for idx, candidate := range defaultRecoverableQualityItems {
		if item == candidate {
			return idx
		}
	}
	return len(defaultRecoverableQualityItems)
}

func clampRecoverableQualityScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
