package probe

import (
	"sort"
	"strings"
)

const (
	ScoreItemCompletionRate        = "completion_rate"
	ScoreItemUpstreamErrorRate     = "upstream_error_rate"
	ScoreItemTTFTLatency           = "ttft_latency"
	ScoreItemDurationLatency       = "duration_latency"
	ScoreItemFirstByteBacklog      = "first_byte_backlog"
	ScoreItemEmptyOutputRate       = "empty_output_rate"
	ScoreItemStreamInterruptedRate = "stream_interrupted_rate"

	PromptCategoryShort  = "short"
	PromptCategoryZH     = "zh"
	PromptCategoryMedium = "medium"
	PromptCategoryLong   = "long"
)

var defaultRecoverableScoreItems = []string{
	ScoreItemCompletionRate,
	ScoreItemUpstreamErrorRate,
	ScoreItemTTFTLatency,
	ScoreItemDurationLatency,
	ScoreItemFirstByteBacklog,
	ScoreItemEmptyOutputRate,
	ScoreItemStreamInterruptedRate,
}

var defaultPromptCategories = []string{
	PromptCategoryShort,
	PromptCategoryZH,
	PromptCategoryMedium,
	PromptCategoryLong,
}

func DefaultRecoverableScoreItems() []string {
	return append([]string(nil), defaultRecoverableScoreItems...)
}

func DefaultPromptCategories() []string {
	return append([]string(nil), defaultPromptCategories...)
}

func NormalizeRecoverableScoreItems(items []string) []string {
	if items == nil {
		return append([]string(nil), defaultRecoverableScoreItems...)
	}
	return normalizeProbeStringSet(items, defaultRecoverableScoreItems, false, validRecoverableScoreItem)
}

func NormalizePromptCategories(categories []string) []string {
	if categories == nil {
		return append([]string(nil), defaultPromptCategories...)
	}
	out := normalizeProbeStringSet(categories, defaultPromptCategories, false, validPromptCategory)
	if len(out) == 0 {
		return []string{PromptCategoryShort}
	}
	return out
}

func validRecoverableScoreItem(item string) bool {
	switch strings.TrimSpace(item) {
	case ScoreItemCompletionRate,
		ScoreItemUpstreamErrorRate,
		ScoreItemTTFTLatency,
		ScoreItemDurationLatency,
		ScoreItemFirstByteBacklog,
		ScoreItemEmptyOutputRate,
		ScoreItemStreamInterruptedRate:
		return true
	default:
		return false
	}
}

func validPromptCategory(category string) bool {
	switch strings.TrimSpace(category) {
	case PromptCategoryShort,
		PromptCategoryZH,
		PromptCategoryMedium,
		PromptCategoryLong:
		return true
	default:
		return false
	}
}

func normalizeProbeStringSet(values []string, defaults []string, fallbackWhenEmpty bool, valid func(string) bool) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !valid(value) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 && fallbackWhenEmpty && len(defaults) > 0 {
		out = append([]string(nil), defaults...)
	}
	sort.SliceStable(out, func(i int, j int) bool {
		return probeStringSetOrder(out[i], defaults) < probeStringSetOrder(out[j], defaults)
	})
	return out
}

func probeStringSetOrder(value string, defaults []string) int {
	for idx, item := range defaults {
		if item == value {
			return idx
		}
	}
	return len(defaults)
}
