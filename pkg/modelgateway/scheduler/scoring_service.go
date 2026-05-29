package scheduler

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
)

type ScoringContext struct {
	RequestedModel         string
	EndpointType           constant.EndpointType
	CandidateGroups        []string
	AutoMode               string
	Strategy               string
	RequiresCodexImageTool bool
	ScoreWeights           core.ScoreWeights
	RetryRoutingIntent     *core.RetryRoutingIntent
	Now                    time.Time
	ExplainEnabled         bool
}

type CandidateScoringService struct {
	snapshotEnricher     core.RuntimeSnapshotEnricher
	costBaselineProvider core.CostBaselineProvider
}

type scoreLatencyView struct {
	TTFTMs                 float64
	DurationMs             float64
	TokensPerSecond        float64
	TTFTStabilityPenalty   float64
	LatencySamples         int
	ThroughputSamples      int
	UsesScoreStatsLatency  bool
	UsesRuntimeLatencyView bool
	TTFTSource             string
	DurationSource         string
	ThroughputSource       string
}

func NewCandidateScoringService() *CandidateScoringService {
	return &CandidateScoringService{}
}

func (s *CandidateScoringService) WithRuntimeSnapshotEnricher(enricher core.RuntimeSnapshotEnricher) *CandidateScoringService {
	if s == nil {
		return nil
	}
	s.snapshotEnricher = enricher
	return s
}

func (s *CandidateScoringService) WithCostBaselineProvider(provider core.CostBaselineProvider) *CandidateScoringService {
	if s == nil {
		return nil
	}
	s.costBaselineProvider = provider
	return s
}

func (s *CandidateScoringService) EvaluateCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, ctx ScoringContext) CandidateScoreEvaluation {
	if s == nil {
		s = NewCandidateScoringService()
	}
	snapshot = normalizeRuntimeSnapshot(snapshot)
	if s.snapshotEnricher != nil && candidate.Channel != nil {
		snapshot = s.snapshotEnricher.Enrich(candidate, snapshot, policy)
	}
	if reference, ok := s.costReferenceForCandidate(candidate, snapshot, policy, ctx, snapshot.CostReferenceRatio); ok {
		snapshot.CostReferenceRatio = reference
	}
	explanation := candidateExplanation(candidate, snapshot, false)
	score := s.score(candidate, snapshot, policy, ctx)
	explanation.ScoreTotal = score.Total
	explanation.ScoreBreakdown = score.Breakdown
	explanation.ScoreItems = score.Items
	explanation.RoutingScoreTotal = score.RoutingTotal
	explanation.RoutingScoreBreakdown = score.RoutingBreakdown
	explanation.RoutingScoreItems = score.RoutingItems
	applyScoreItemDisplayMetricsToExplanation(&explanation, score.Items)
	explanation.StateTags = score.StateTags
	explanation.CostReferenceMissing = score.CostReferenceMissing
	applyRetryIntentToExplanation(&explanation, ctx.RetryRoutingIntent)
	return CandidateScoreEvaluation{Snapshot: snapshot, Score: score, Explanation: explanation}
}

func (s *CandidateScoringService) EvaluatePreparedCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, ctx ScoringContext, stickyMatched bool) CandidateScoreEvaluation {
	if s == nil {
		s = NewCandidateScoringService()
	}
	explanation := candidateExplanation(candidate, snapshot, stickyMatched)
	score := s.score(candidate, snapshot, policy, ctx)
	explanation.ScoreTotal = score.Total
	explanation.ScoreBreakdown = score.Breakdown
	explanation.ScoreItems = score.Items
	explanation.RoutingScoreTotal = score.RoutingTotal
	explanation.RoutingScoreBreakdown = score.RoutingBreakdown
	explanation.RoutingScoreItems = score.RoutingItems
	applyScoreItemDisplayMetricsToExplanation(&explanation, score.Items)
	explanation.StateTags = score.StateTags
	explanation.CostReferenceMissing = score.CostReferenceMissing
	applyRetryIntentToExplanation(&explanation, ctx.RetryRoutingIntent)
	return CandidateScoreEvaluation{Snapshot: snapshot, Score: score, Explanation: explanation}
}

func applyRetryIntentToExplanation(explanation *core.CandidateExplanation, intent *core.RetryRoutingIntent) {
	if explanation == nil || intent == nil || !intent.Active() {
		return
	}
	explanation.RetryIntentApplied = true
	explanation.RetryIntentReason = intent.Reason
}

func (s *CandidateScoringService) BuildScoreItems(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, ctx ScoringContext, routing bool) []core.ScoreItem {
	strategy := policy.Strategy
	if strategy == "" {
		strategy = ctx.Strategy
	}
	profile := StrategyProfileFor(strategy).WithScoreWeights(ctx.ScoreWeights)
	stats := scoreStatsFromSnapshot(snapshot)
	latency := scoreLatencyViewFromSnapshot(snapshot, stats)
	loadScore := stableConcurrencyLoadItemScore(snapshot)
	queueScore := 1.0
	firstByteScore := 1.0
	if routing {
		loadScore = routingConcurrencyLoadItemScoreForStrategy(snapshot, strategy)
		queueScore = queuePressureScore(snapshot, strategy)
		firstByteScore = firstByteBacklogScore(snapshot, strategy)
	}
	values := []core.ScoreItem{
		scoreItem(scoreItemCompletionRate, "完成率分", scoreCategorySample, scoreRateSuccessRawValue(stats.Rates["completion"], "completion_rate"), "评分窗口", scoreRateValue(stats.Rates["completion"], completionRateScore(snapshot)), profile.Weights[scoreItemCompletionRate], scoreRateSampleCount(stats.Rates["completion"], snapshot.SampleCount), "completed / total", ""),
		scoreItem(scoreItemUpstreamErrorRate, "上游错误率分", scoreCategorySample, scoreRateEventRawValue(stats.Rates["upstream_error"], "upstream_error_rate"), "评分窗口", scoreRateValue(stats.Rates["upstream_error"], 1), profile.Weights[scoreItemUpstreamErrorRate], scoreRateSampleCount(stats.Rates["upstream_error"], snapshot.SampleCount), "1 - upstream_error_rate", ""),
		scoreItem(scoreItemTTFTLatency, "首包速度分", scoreCategorySample, rawTTFTRawValue(latency), "评分窗口", ttftScoreItemValue(latency), profile.Weights[scoreItemTTFTLatency], latency.SampleCount(snapshot.SampleCount), "recency_weighted_p50_progressive_score(ttft, decay=2.2, half_life=16, stability_penalty)", ""),
		scoreItem(scoreItemDurationLatency, "完整耗时分", scoreCategorySample, rawDurationRawValue(latency), "评分窗口", durationScoreItemValue(latency), profile.Weights[scoreItemDurationLatency], latency.SampleCount(snapshot.SampleCount), "inverse_latency_score(duration, 3000ms, 90000ms)", ""),
		scoreItem(scoreItemThroughput, "吞吐速度分", scoreCategorySample, rawThroughputRawValue(latency), "评分窗口", throughputScoreItemValue(latency), profile.Weights[scoreItemThroughput], latency.ThroughputSampleCount(snapshot.SampleCount), "throughput_score(tps, 5, 80)", ""),
		scoreItem(scoreItemEmptyOutputRate, "空输出率分", scoreCategorySample, scoreRateEventRawValue(stats.Rates["empty_output"], "empty_output_rate"), "评分窗口", scoreRateValue(stats.Rates["empty_output"], clamp01(1-clamp01(snapshot.EmptyOutputRate))), profile.Weights[scoreItemEmptyOutputRate], scoreRateSampleCount(stats.Rates["empty_output"], snapshot.SampleCount), "1 - empty_output_rate", ""),
		scoreItem(scoreItemStreamInterruptedRate, "流中断率分", scoreCategorySample, scoreRateEventRawValue(stats.Rates["stream_interrupted"], "stream_interrupted_rate"), "评分窗口", scoreRateValue(stats.Rates["stream_interrupted"], 1), profile.Weights[scoreItemStreamInterruptedRate], scoreRateSampleCount(stats.Rates["stream_interrupted"], snapshot.SampleCount), "1 - stream_interrupted_rate", ""),
		scoreItem(scoreItemConcurrencyLoad, "并发负载分", scoreCategoryPressure, concurrencyRawValue(snapshot), "实时", loadScore, profile.Weights[scoreItemConcurrencyLoad], snapshot.SampleCount, "1 - load_penalty(active/effective_limit)", ""),
		scoreItem(scoreItemQueuePressure, "队列压力分", scoreCategoryPressure, queueRawValue(snapshot), "实时", queueScore, profile.Weights[scoreItemQueuePressure], snapshot.SampleCount, "1 - queue_penalty(depth, wait)", ""),
		scoreItem(scoreItemFirstByteBacklog, "首包积压分", scoreCategoryPressure, firstByteBacklogRawValue(snapshot), "实时", firstByteScore, profile.Weights[scoreItemFirstByteBacklog], snapshot.SampleCount, "1 - first_byte_pending_penalty", ""),
		scoreItem(scoreItemCost, "成本分", scoreCategoryFormula, costRawValue(snapshot), "配置", costScoreItemValue(snapshot, profile), profile.Weights[scoreItemCost], snapshot.SampleCount, costScoreFormula(profile), ""),
		scoreItem(scoreItemGroupPriority, "分组分", scoreCategoryFormula, groupRawValue(snapshot), "配置", groupPriorityItemScoreForStrategy(snapshot, policy, strategy), profile.Weights[scoreItemGroupPriority], snapshot.SampleCount, "group priority formula", ""),
	}
	if routing {
		values = append(values, retryIntentScoreItem(snapshot, ctx))
	}
	annotateScoreItems(values, stats, snapshot, latency, profile)
	markSampleMissingScoreItems(values, stats, snapshot, latency)
	if snapshot.CostRatio <= 0 || snapshot.CostReferenceRatio <= 0 {
		for idx := range values {
			if values[idx].Key == scoreItemCost {
				values[idx].MissingReason = "cost_reference_missing"
				values[idx].Score = 0
				values[idx].WeightedScore = 0
			}
		}
	}
	return normalizeScoreItems(values)
}

func retryIntentScoreItem(snapshot core.RuntimeSnapshot, ctx ScoringContext) core.ScoreItem {
	intent := ctx.RetryRoutingIntent
	if intent == nil || !intent.FirstByteRecovery() {
		return scoreItem(scoreItemRetryIntentRecovery, "重试恢复分", scoreCategoryFormula, "inactive", "本次重试", 0, 0, snapshot.SampleCount, "retry_recovery_score", "")
	}
	successScore := completionRateScore(snapshot)
	ttftScore := 0.55
	if snapshot.TTFTMs > 0 {
		ttftScore = progressiveTTFTLatencyScore(snapshot.TTFTMs, 800, 12000)
	}
	backlogScore := firstByteBacklogScore(snapshot, core.StrategySpeedFirst)
	score := 0.0
	weightParts := 0.0
	if intent.PreferHighSuccess {
		score += successScore * 0.30
		weightParts += 0.30
	}
	if intent.PreferLowTTFT {
		score += ttftScore * 0.45
		weightParts += 0.45
	}
	score += backlogScore * 0.25
	weightParts += 0.25
	if weightParts > 0 {
		score = score / weightParts
	}
	item := scoreItem(
		scoreItemRetryIntentRecovery,
		"重试恢复分",
		scoreCategoryFormula,
		retryIntentRawValue(snapshot),
		"本次重试",
		score,
		0.60,
		snapshot.SampleCount,
		"weighted(success_rate, ttft_latency, first_byte_backlog)",
		intent.Reason,
	)
	item.Source = "retry_intent"
	item.RawUnit = "recovery"
	return item
}

func retryIntentRawValue(snapshot core.RuntimeSnapshot) string {
	return fmt.Sprintf("success=%.4f ttft=%.0fms first_byte_pending=%d", completionRateScore(snapshot), snapshot.TTFTMs, snapshot.FirstBytePending)
}

func scoreLatencyViewFromSnapshot(snapshot core.RuntimeSnapshot, stats ScoreStats) scoreLatencyView {
	view := scoreLatencyView{
		TTFTMs:               snapshot.TTFTMs,
		DurationMs:           snapshot.DurationMs,
		TokensPerSecond:      snapshot.TokensPerSecond,
		TTFTStabilityPenalty: 1,
		TTFTSource:           scoreItemSourceForSnapshotValue(snapshot.TTFTMs),
		DurationSource:       scoreItemSourceForSnapshotValue(snapshot.DurationMs),
		ThroughputSource:     scoreItemSourceForSnapshotValue(snapshot.TokensPerSecond),
	}
	if len(snapshot.RecentLatencySamples) > 0 {
		if weightedTTFTMs, stabilityPenalty, sampleCount := recencyWeightedTTFTLatency(snapshot.RecentLatencySamples); weightedTTFTMs > 0 {
			view.TTFTMs = weightedTTFTMs
			view.TTFTStabilityPenalty = stabilityPenalty
			view.TTFTSource = scoreItemSourceRuntimeLatencySamples
			view.LatencySamples = maxIntValue(view.LatencySamples, sampleCount)
			view.UsesRuntimeLatencyView = true
		}
	}
	if len(stats.Latency.TTFTMs) > 0 {
		if view.TTFTMs <= 0 || view.TTFTSource == scoreItemSourceSnapshotFallback || view.TTFTSource == scoreItemSourceSampleMissing {
			view.TTFTMs = trimmedMeanFloat64(stats.Latency.TTFTMs)
			view.LatencySamples = maxIntValue(view.LatencySamples, len(stats.Latency.TTFTMs))
			view.UsesScoreStatsLatency = true
			view.TTFTSource = scoreItemSourceScoreStatsLatency
		}
	}
	if len(stats.Latency.DurationMs) > 0 {
		view.DurationMs = trimmedMeanFloat64(stats.Latency.DurationMs)
		view.LatencySamples = maxIntValue(view.LatencySamples, len(stats.Latency.DurationMs))
		view.UsesScoreStatsLatency = true
		view.DurationSource = scoreItemSourceScoreStatsLatency
	}
	if len(stats.Latency.TokensPerSecond) > 0 {
		view.TokensPerSecond = trimmedMeanFloat64(stats.Latency.TokensPerSecond)
		view.ThroughputSamples = len(stats.Latency.TokensPerSecond)
		view.UsesScoreStatsLatency = true
		view.ThroughputSource = scoreItemSourceScoreStatsLatency
	}
	if !view.UsesScoreStatsLatency && !view.UsesRuntimeLatencyView && len(snapshot.RecentLatencySamples) > 0 {
		durationMs, ttftMs, _ := runtimeLatencyStats(snapshot.RecentLatencySamples)
		if ttftMs > 0 {
			view.TTFTMs = ttftMs
			view.TTFTSource = scoreItemSourceRuntimeLatencySamples
		}
		if durationMs > 0 {
			view.DurationMs = durationMs
			view.DurationSource = scoreItemSourceRuntimeLatencySamples
		}
		view.LatencySamples = len(snapshot.RecentLatencySamples)
		view.UsesRuntimeLatencyView = true
	}
	if view.LatencySamples <= 0 && snapshot.SampleCount > 0 && (view.TTFTMs > 0 || view.DurationMs > 0) {
		view.LatencySamples = snapshot.SampleCount
	}
	if view.ThroughputSamples <= 0 && view.TokensPerSecond > 0 {
		view.ThroughputSamples = snapshot.SampleCount
	}
	return view
}

func (v scoreLatencyView) SampleCount(fallback int) int {
	if v.LatencySamples > 0 {
		return v.LatencySamples
	}
	return fallback
}

func (v scoreLatencyView) ThroughputSampleCount(fallback int) int {
	if v.ThroughputSamples > 0 {
		return v.ThroughputSamples
	}
	if v.TokensPerSecond > 0 {
		return fallback
	}
	return 0
}

func maxIntValue(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func (s *CandidateScoringService) score(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, ctx ScoringContext) core.ScoreResult {
	strategy := policy.Strategy
	if strategy == "" {
		strategy = ctx.Strategy
	}
	items := s.BuildScoreItems(snapshot, policy, ctx, false)
	routingItems := s.BuildScoreItems(snapshot, policy, ctx, true)
	breakdown := scoreBreakdownFromItems(items)
	routingBreakdown := scoreBreakdownFromItems(routingItems)
	stateTags := stateTagsForSnapshot(snapshot)
	reason := "score_items"
	if ctx.RetryRoutingIntent != nil && ctx.RetryRoutingIntent.Active() {
		stateTags = append(stateTags, "retry_intent:"+ctx.RetryRoutingIntent.Strategy)
		reason = "score_items_retry_intent"
	}
	return core.ScoreResult{
		Total:                scoreTotalFromItems(items, snapshot, strategy, false),
		Breakdown:            breakdown,
		Items:                items,
		RoutingTotal:         scoreTotalFromItems(routingItems, snapshot, strategy, true),
		RoutingBreakdown:     routingBreakdown,
		RoutingItems:         routingItems,
		StateTags:            stateTags,
		CostReferenceMissing: snapshot.CostRatio <= 0 || snapshot.CostReferenceRatio <= 0,
		Reason:               reason,
	}
}

func (s *CandidateScoringService) costReferenceForCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, ctx ScoringContext, fallbackReference float64) (float64, bool) {
	if s == nil || s.costBaselineProvider == nil || snapshot.CostRatio <= 0 {
		if fallbackReference > 0 {
			return fallbackReference, true
		}
		return 0, false
	}
	groups := policy.CandidateGroups
	if len(groups) == 0 {
		groups = ctx.CandidateGroups
	}
	if len(groups) == 0 {
		group := strings.TrimSpace(candidate.Group)
		if group == "" {
			group = strings.TrimSpace(snapshot.Key.Group)
		}
		if group != "" {
			groups = []string{group}
		}
	}
	requestedModel := strings.TrimSpace(snapshot.Key.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(ctx.RequestedModel)
	}
	endpointType := strings.TrimSpace(string(snapshot.Key.EndpointType))
	if endpointType == "" {
		endpointType = strings.TrimSpace(string(ctx.EndpointType))
	}
	best := 0.0
	for _, group := range groups {
		scope := core.CostBaselineScope{
			RequestedModel:         requestedModel,
			Group:                  strings.TrimSpace(group),
			EndpointType:           endpointType,
			RequiresCodexImageTool: candidate.RequiresCodexImageTool || ctx.RequiresCodexImageTool,
		}
		if value, ok := s.costBaselineProvider.Baseline(scope); ok && value > 0 && (best <= 0 || value < best) {
			best = value
		}
	}
	if best > 0 {
		return best, true
	}
	if fallbackReference > 0 {
		return fallbackReference, true
	}
	return 0, false
}

func scoreItem(key, name, category, raw, window string, score, weight float64, sampleCount int, formula, reason string) core.ScoreItem {
	score = clamp01(score)
	return core.ScoreItem{
		Key:           key,
		Name:          name,
		Category:      category,
		RawValue:      raw,
		Window:        window,
		Score:         round4(score),
		Weight:        weight,
		WeightedScore: round4(score * weight),
		SampleCount:   sampleCount,
		Formula:       formula,
		Reason:        reason,
	}
}

const (
	scoreItemSourceScoreStatsLatency     = "score_stats_latency"
	scoreItemSourceRuntimeLatencySamples = "runtime_latency_samples"
	scoreItemSourceSnapshotFallback      = "snapshot_fallback"
	scoreItemSourceSampleMissing         = "sample_missing"
	scoreItemSourceScoreStatsRate        = "score_stats_rate"
	scoreItemSourceConfig                = "config"
	scoreItemSourceRealtime              = "realtime"
)

func annotateScoreItems(items []core.ScoreItem, stats ScoreStats, snapshot core.RuntimeSnapshot, latency scoreLatencyView, profile StrategyProfile) {
	for idx := range items {
		item := &items[idx]
		switch item.Key {
		case scoreItemCompletionRate:
			scoreItemSetRaw(item, scoreRateRawNumber(stats.Rates["completion"], completionRateScore(snapshot)), "ratio", scoreRateSource(stats.Rates["completion"], snapshot.SampleCount))
		case scoreItemUpstreamErrorRate:
			scoreItemSetRaw(item, 1-scoreRateRawNumber(stats.Rates["upstream_error"], 1), "ratio", scoreRateSource(stats.Rates["upstream_error"], snapshot.SampleCount))
		case scoreItemTTFTLatency:
			scoreItemSetRaw(item, latency.TTFTMs, "ms", scoreItemSourceForValue(latency.TTFTMs, latency.TTFTSource))
			item.FormulaParameters = ttftFormulaParameters(latency)
		case scoreItemDurationLatency:
			scoreItemSetRaw(item, latency.DurationMs, "ms", scoreItemSourceForValue(latency.DurationMs, latency.DurationSource))
		case scoreItemThroughput:
			scoreItemSetRaw(item, latency.TokensPerSecond, "tps", scoreItemSourceForValue(latency.TokensPerSecond, latency.ThroughputSource))
		case scoreItemEmptyOutputRate:
			scoreItemSetRaw(item, 1-scoreRateRawNumber(stats.Rates["empty_output"], clamp01(1-clamp01(snapshot.EmptyOutputRate))), "ratio", scoreRateSource(stats.Rates["empty_output"], snapshot.SampleCount))
		case scoreItemStreamInterruptedRate:
			scoreItemSetRaw(item, 1-scoreRateRawNumber(stats.Rates["stream_interrupted"], 1), "ratio", scoreRateSource(stats.Rates["stream_interrupted"], snapshot.SampleCount))
		case scoreItemConcurrencyLoad:
			scoreItemSetRaw(item, float64(snapshot.ActiveConcurrency), "concurrency", scoreItemSourceRealtime)
		case scoreItemQueuePressure:
			scoreItemSetRaw(item, float64(snapshot.QueueDepth), "queue_depth", scoreItemSourceRealtime)
		case scoreItemFirstByteBacklog:
			scoreItemSetRaw(item, float64(snapshot.FirstBytePending), "pending", scoreItemSourceRealtime)
		case scoreItemCost:
			scoreItemSetRaw(item, snapshot.CostRatio, costScoreItemUnit(snapshot), scoreItemSourceConfig)
			if snapshot.CostReferenceRatio > 0 {
				item.ReferenceNumber = scoreItemFloat(snapshot.CostReferenceRatio)
				item.ReferenceUnit = costScoreItemUnit(snapshot)
			}
			item.FormulaParameters = costFormulaParameters(snapshot, profile)
			if profile.CostScoreMode != "" {
				item.Reason = profile.CostScoreMode
			}
		case scoreItemGroupPriority:
			scoreItemSetRaw(item, snapshot.GroupPriorityRatio, "ratio", scoreItemSourceConfig)
		case scoreItemRetryIntentRecovery:
			if item.Source == "" {
				item.Source = "retry_intent"
			}
		}
	}
}

func scoreItemSetRaw(item *core.ScoreItem, value float64, unit string, source string) {
	if item == nil {
		return
	}
	if source != scoreItemSourceSampleMissing && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
		item.RawNumber = scoreItemFloat(value)
	}
	item.RawUnit = unit
	item.Source = source
}

func scoreItemFloat(value float64) *float64 {
	raw := value
	return &raw
}

func scoreItemSourceForSnapshotValue(value float64) string {
	if value > 0 {
		return scoreItemSourceSnapshotFallback
	}
	return scoreItemSourceSampleMissing
}

func scoreItemSourceForValue(value float64, source string) string {
	if value <= 0 {
		return scoreItemSourceSampleMissing
	}
	if strings.TrimSpace(source) != "" {
		return source
	}
	return scoreItemSourceSnapshotFallback
}

func scoreRateSource(stats ScoreRateStats, fallbackSamples int) string {
	if stats.Total > 0 {
		return scoreItemSourceScoreStatsRate
	}
	if fallbackSamples > 0 {
		return scoreItemSourceSnapshotFallback
	}
	return scoreItemSourceSampleMissing
}

func scoreRateRawNumber(stats ScoreRateStats, fallback float64) float64 {
	if stats.Total <= 0 {
		return clamp01(fallback)
	}
	if stats.EWMA > 0 {
		return clamp01(stats.EWMA)
	}
	return clamp01(float64(stats.Success) / float64(stats.Total))
}

func costScoreItemUnit(snapshot core.RuntimeSnapshot) string {
	if strings.TrimSpace(snapshot.CostPricingMode) == "request" {
		return "request"
	}
	return "per_million_tokens"
}

func applyScoreItemDisplayMetricsToExplanation(explanation *core.CandidateExplanation, items []core.ScoreItem) {
	if explanation == nil {
		return
	}
	if value, ok := scoreItemRawNumberByKey(items, scoreItemTTFTLatency); ok {
		explanation.TTFTMs = value
	}
	if value, ok := scoreItemRawNumberByKey(items, scoreItemDurationLatency); ok {
		explanation.DurationMs = value
	}
	if value, ok := scoreItemRawNumberByKey(items, scoreItemThroughput); ok {
		explanation.TokensPerSecond = value
	}
}

func scoreItemRawNumberByKey(items []core.ScoreItem, key string) (float64, bool) {
	for _, item := range items {
		if item.Key != key || item.RawNumber == nil || *item.RawNumber <= 0 {
			continue
		}
		return *item.RawNumber, true
	}
	return 0, false
}

func normalizeScoreItems(items []core.ScoreItem) []core.ScoreItem {
	totalWeight := 0.0
	for _, item := range items {
		if item.MissingReason != "" || item.Weight <= 0 {
			continue
		}
		totalWeight += item.Weight
	}
	if totalWeight <= 0 {
		return items
	}
	out := make([]core.ScoreItem, 0, len(items))
	for _, item := range items {
		if item.MissingReason != "" || item.Weight <= 0 {
			item.Weight = 0
			item.WeightedScore = 0
			out = append(out, item)
			continue
		}
		item.Weight = item.Weight / totalWeight
		out = append(out, item)
	}
	applyMinimumNormalizedScoreItemWeight(out, scoreItemTTFTLatency, scoreItemTTFTLatencyMinWeight)
	for idx := range out {
		if out[idx].MissingReason != "" || out[idx].Weight <= 0 {
			continue
		}
		out[idx].Weight = round4(out[idx].Weight)
		out[idx].WeightedScore = round4(out[idx].Score * out[idx].Weight)
	}
	return out
}

func applyMinimumNormalizedScoreItemWeight(items []core.ScoreItem, key string, target float64) {
	if len(items) == 0 || strings.TrimSpace(key) == "" || target <= 0 || target >= 1 {
		return
	}
	target = clamp01(target)
	targetIdx := -1
	current := 0.0
	for idx, item := range items {
		if item.Key != key || item.MissingReason != "" || item.Weight <= 0 {
			continue
		}
		targetIdx = idx
		current = item.Weight
		break
	}
	if targetIdx < 0 || current >= target {
		return
	}
	otherTotal := 1 - current
	if otherTotal <= 0 {
		items[targetIdx].Weight = target
		return
	}
	scale := (1 - target) / otherTotal
	for idx := range items {
		if items[idx].MissingReason != "" || items[idx].Weight <= 0 {
			continue
		}
		if idx == targetIdx {
			items[idx].Weight = target
			continue
		}
		items[idx].Weight *= scale
	}
}

func markSampleMissingScoreItems(items []core.ScoreItem, stats ScoreStats, snapshot core.RuntimeSnapshot, latency scoreLatencyView) {
	markIndividualSampleMissingScoreItems(items, stats, snapshot, latency)
	if snapshot.SampleCount > 0 || stats.Samples > 0 {
		return
	}
	missingKeys := map[string]struct{}{
		scoreItemCompletionRate:        {},
		scoreItemUpstreamErrorRate:     {},
		scoreItemTTFTLatency:           {},
		scoreItemDurationLatency:       {},
		scoreItemThroughput:            {},
		scoreItemEmptyOutputRate:       {},
		scoreItemStreamInterruptedRate: {},
	}
	for idx := range items {
		if _, ok := missingKeys[items[idx].Key]; !ok {
			continue
		}
		items[idx].MissingReason = "sample_missing"
		items[idx].Score = 0
		items[idx].WeightedScore = 0
		if items[idx].RawValue == "" {
			items[idx].RawValue = "sample_missing"
		}
	}
}

func markIndividualSampleMissingScoreItems(items []core.ScoreItem, stats ScoreStats, snapshot core.RuntimeSnapshot, latency scoreLatencyView) {
	missing := map[string]bool{}
	if stats.Rates["completion"].Total <= 0 && snapshot.SampleCount <= 0 {
		missing[scoreItemCompletionRate] = true
	}
	if stats.Rates["upstream_error"].Total <= 0 && snapshot.SampleCount <= 0 {
		missing[scoreItemUpstreamErrorRate] = true
	}
	if latency.TTFTMs <= 0 {
		missing[scoreItemTTFTLatency] = true
	}
	if latency.DurationMs <= 0 {
		missing[scoreItemDurationLatency] = true
	}
	if latency.TokensPerSecond <= 0 {
		missing[scoreItemThroughput] = true
	}
	if stats.Rates["empty_output"].Total <= 0 && snapshot.SampleCount <= 0 {
		missing[scoreItemEmptyOutputRate] = true
	}
	if stats.Rates["stream_interrupted"].Total <= 0 && snapshot.SampleCount <= 0 {
		missing[scoreItemStreamInterruptedRate] = true
	}
	for idx := range items {
		if !missing[items[idx].Key] {
			continue
		}
		items[idx].MissingReason = "sample_missing"
		items[idx].Score = 0
		items[idx].WeightedScore = 0
		if items[idx].RawValue == "" {
			items[idx].RawValue = "sample_missing"
		}
	}
}

func scoreBreakdownFromItems(items []core.ScoreItem) map[string]float64 {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]float64, len(items))
	for _, item := range items {
		if item.MissingReason != "" || item.Weight <= 0 {
			continue
		}
		out[item.Key] = round4(item.Score)
	}
	return out
}

func scoreTotalFromItems(items []core.ScoreItem, snapshot core.RuntimeSnapshot, strategy string, routing bool) float64 {
	total := 0.0
	for _, item := range items {
		if item.MissingReason != "" || item.Weight <= 0 {
			continue
		}
		total += item.Score * item.Weight
	}
	return round4(clamp01(total))
}

func completionRateScore(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.SampleCount <= 0 {
		return 0.55
	}
	return clamp01(snapshot.SuccessRate)
}

func ttftScoreItemValue(latency scoreLatencyView) float64 {
	if latency.TTFTMs <= 0 {
		return 0
	}
	penalty := latency.TTFTStabilityPenalty
	if penalty <= 0 {
		penalty = 1
	}
	return progressiveTTFTLatencyScore(latency.TTFTMs, 800, 20000) * clamp01(penalty)
}

func durationScoreItemValue(latency scoreLatencyView) float64 {
	if latency.DurationMs <= 0 {
		return 0
	}
	return inverseLatencyScore(latency.DurationMs, 3000, 90000)
}

func throughputScoreItemValue(latency scoreLatencyView) float64 {
	return throughputScore(latency.TokensPerSecond, 5, 80)
}

func concurrencyLoadScore(snapshot core.RuntimeSnapshot, strategy string) float64 {
	limit := routingConcurrencyLimit(snapshot)
	if limit <= 0 {
		return routingConcurrencyLoadItemScoreWithoutKnownLimit(snapshot)
	}
	ratio := float64(snapshot.ActiveConcurrency) / float64(limit)
	return clamp01(1 - softLoadPenaltyForStrategy(ratio, strategy))
}

func queuePressureScore(snapshot core.RuntimeSnapshot, strategy string) float64 {
	score := 1.0
	if snapshot.QueueDepth > 0 {
		if strategy == core.StrategyCostFirst {
			score -= math.Min(float64(snapshot.QueueDepth)*0.03, 0.18)
		} else {
			score -= math.Min(float64(snapshot.QueueDepth)*0.05, 0.24)
		}
	}
	if snapshot.EstimatedQueueWaitMs > 0 {
		denominator := 12000.0
		if strategy == core.StrategyCostFirst {
			denominator = 18000
		}
		score -= math.Min(snapshot.EstimatedQueueWaitMs/denominator, 0.24)
	}
	return clamp01(score)
}

func firstByteBacklogScore(snapshot core.RuntimeSnapshot, strategy string) float64 {
	return clamp01(1 - firstByteBacklogPenaltyForStrategy(snapshot, strategy))
}

func costScoreItemValue(snapshot core.RuntimeSnapshot, profile StrategyProfile) float64 {
	if snapshot.CostRatio <= 0 || snapshot.CostReferenceRatio <= 0 {
		return 0
	}
	if profile.CostScoreMode == costScoreModeLogMultiple {
		zeroPoint := profile.CostZeroPoint
		if zeroPoint <= 1 {
			zeroPoint = costFirstLogCostZeroMultiple
		}
		multiple := snapshot.CostRatio / snapshot.CostReferenceRatio
		if multiple <= 1 {
			return 1
		}
		score := 1 - math.Log(multiple)/math.Log(zeroPoint)
		return clamp01(score)
	}
	score := clamp01(snapshot.CostReferenceRatio / snapshot.CostRatio)
	if profile.CostPower > 1 {
		score = math.Pow(score, profile.CostPower)
	}
	return clamp01(score)
}

func costScoreFormula(profile StrategyProfile) string {
	if profile.CostScoreMode == costScoreModeLogMultiple {
		return "1 - log(current_cost / min_cost) / log(zero_multiple)"
	}
	return "min_cost / current_cost"
}

func costFormulaParameters(snapshot core.RuntimeSnapshot, profile StrategyProfile) map[string]float64 {
	params := map[string]float64{}
	if profile.CostPower > 0 {
		params["cost_power"] = profile.CostPower
	}
	if profile.CostScoreMode == costScoreModeLogMultiple {
		zeroPoint := profile.CostZeroPoint
		if zeroPoint <= 1 {
			zeroPoint = costFirstLogCostZeroMultiple
		}
		params["cost_zero_multiple"] = zeroPoint
		if snapshot.CostRatio > 0 && snapshot.CostReferenceRatio > 0 {
			params["cost_multiple"] = snapshot.CostRatio / snapshot.CostReferenceRatio
		}
	}
	return params
}

func ttftFormulaParameters(latency scoreLatencyView) map[string]float64 {
	params := map[string]float64{
		"decay":     progressiveTTFTLatencyDecay,
		"half_life": ttftRecencyHalfLifeSamples,
	}
	if latency.TTFTStabilityPenalty > 0 && latency.TTFTStabilityPenalty < 1 {
		params["stability_penalty"] = latency.TTFTStabilityPenalty
	}
	return params
}

func stateTagsForSnapshot(snapshot core.RuntimeSnapshot) []string {
	tags := make([]string, 0, 4)
	if snapshot.CircuitOpen {
		tags = append(tags, "circuit_open")
	}
	if snapshot.Cooldown {
		tags = append(tags, "cooldown")
	}
	if snapshot.FailureAvoidance {
		tags = append(tags, "failure_avoidance")
	}
	if snapshot.FailureAvoidance && strings.TrimSpace(snapshot.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason {
		tags = append(tags, service.ChannelTimeoutRecoveryReason)
	}
	if snapshot.ProbeRecoveryPending {
		tags = append(tags, "probe_recovery_pending")
	}
	if snapshot.ConfigErrorIsolated {
		tags = append(tags, "config_error_isolated")
	}
	return tags
}

func rawTTFTRawValue(latency scoreLatencyView) string {
	if latency.TTFTMs <= 0 {
		return "sample_missing"
	}
	return fmt.Sprintf("%.0fms", latency.TTFTMs)
}

func rawDurationRawValue(latency scoreLatencyView) string {
	if latency.DurationMs <= 0 {
		return "sample_missing"
	}
	return fmt.Sprintf("%.0fms", latency.DurationMs)
}

func rawThroughputRawValue(latency scoreLatencyView) string {
	if latency.TokensPerSecond <= 0 {
		return "sample_missing"
	}
	return fmt.Sprintf("%.2f tps", latency.TokensPerSecond)
}

func concurrencyRawValue(snapshot core.RuntimeSnapshot) string {
	limit := routingConcurrencyLimit(snapshot)
	if limit <= 0 {
		return fmt.Sprintf("%d/unknown", snapshot.ActiveConcurrency)
	}
	return fmt.Sprintf("%d/%d", snapshot.ActiveConcurrency, limit)
}

func queueRawValue(snapshot core.RuntimeSnapshot) string {
	return fmt.Sprintf("depth=%d wait=%.0fms", snapshot.QueueDepth, snapshot.EstimatedQueueWaitMs)
}

func firstByteBacklogRawValue(snapshot core.RuntimeSnapshot) string {
	return fmt.Sprintf("pending=%d slow=%d oldest=%.0fms", snapshot.FirstBytePending, snapshot.SlowFirstBytePending, snapshot.OldestFirstByteWaitMs)
}

func costRawValue(snapshot core.RuntimeSnapshot) string {
	if snapshot.CostRatio <= 0 {
		return "cost_missing"
	}
	if snapshot.CostReferenceRatio <= 0 {
		return fmt.Sprintf("current=%.4f reference=missing", snapshot.CostRatio)
	}
	return fmt.Sprintf("current=%.4f reference=%.4f", snapshot.CostRatio, snapshot.CostReferenceRatio)
}

func groupRawValue(snapshot core.RuntimeSnapshot) string {
	if snapshot.GroupPriorityRatio <= 0 {
		return "group_priority_missing"
	}
	return fmt.Sprintf("%.4f", snapshot.GroupPriorityRatio)
}

func scoreRateValue(stats ScoreRateStats, fallback float64) float64 {
	if stats.Total <= 0 {
		return fallback
	}
	if stats.EWMA > 0 {
		return clamp01(stats.EWMA)
	}
	return clamp01(float64(stats.Success) / float64(stats.Total))
}

func scoreRateSampleCount(stats ScoreRateStats, fallback int) int {
	if stats.Total > 0 {
		return stats.Total
	}
	return fallback
}

func scoreRateSuccessRawValue(stats ScoreRateStats, label string) string {
	if stats.Total <= 0 {
		return label
	}
	return fmt.Sprintf("%d/%d", stats.Success, stats.Total)
}

func scoreRateEventRawValue(stats ScoreRateStats, label string) string {
	if stats.Total <= 0 {
		return label
	}
	return fmt.Sprintf("%d/%d", stats.Count, stats.Total)
}
