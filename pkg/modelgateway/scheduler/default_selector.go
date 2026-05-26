package scheduler

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const maxCandidateExplanations = 32

const (
	costFirstStickyEscapeCostRatio       = 0.75
	costFirstStickyEscapeScoreDelta      = 0.03
	costFirstStickyEscapeCacheCostRatio  = 0.55
	costFirstStickyEscapeCacheScoreDelta = 0.08
	costFirstStickyEscapeMinSamples      = 5
	costFirstStickyEscapeSuccessSlack    = 0.02
)

type DefaultSmartChannelSelector struct {
	candidateBuilder     core.CandidatePoolBuilder
	snapshotStore        core.RuntimeSnapshotStore
	scorerFactory        core.ScoreCalculatorFactory
	snapshotEnricher     core.RuntimeSnapshotEnricher
	stickyRouter         core.StickyRouter
	costBaselineProvider core.CostBaselineProvider
}

type stickySaveOnSelectRouter interface {
	SaveOnSelect() bool
}

type candidateEvaluation struct {
	candidate     core.Candidate
	snapshot      core.RuntimeSnapshot
	stickyMatched bool
	rejectReason  string
}

type CandidateScoreEvaluation struct {
	Snapshot    core.RuntimeSnapshot
	Score       core.ScoreResult
	Explanation core.CandidateExplanation
}

func NewDefaultSmartChannelSelector(candidateBuilder core.CandidatePoolBuilder, snapshotStore core.RuntimeSnapshotStore, scorerFactory core.ScoreCalculatorFactory) *DefaultSmartChannelSelector {
	if scorerFactory == nil {
		scorerFactory = NewScoreCalculatorFactory(DefaultScoreWeights())
	}
	return &DefaultSmartChannelSelector{
		candidateBuilder: candidateBuilder,
		snapshotStore:    snapshotStore,
		scorerFactory:    scorerFactory,
	}
}

func (s *DefaultSmartChannelSelector) WithRuntimeSnapshotEnricher(enricher core.RuntimeSnapshotEnricher) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.snapshotEnricher = enricher
	return s
}

func (s *DefaultSmartChannelSelector) WithStickyRouter(router core.StickyRouter) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.stickyRouter = router
	return s
}

func (s *DefaultSmartChannelSelector) WithCostBaselineProvider(provider core.CostBaselineProvider) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.costBaselineProvider = provider
	return s
}

func (s *DefaultSmartChannelSelector) StickyRouter() core.StickyRouter {
	if s == nil {
		return nil
	}
	return s.stickyRouter
}

func (s *DefaultSmartChannelSelector) Select(c *gin.Context, param *service.RetryParam, policy core.GroupSmartPolicy) (*core.DispatchPlan, bool, *types.NewAPIError) {
	if s == nil || s.candidateBuilder == nil || !policy.IsActive() {
		return nil, false, nil
	}
	req := core.NewDispatchRequestFromGin(c, param)
	if policy.RequestedGroup == "" {
		policy.RequestedGroup = req.RequestedGroup
	}
	if policy.UserGroup == "" {
		policy.UserGroup = req.UserGroup
	}
	candidates := s.candidateBuilder.Build(&req, policy)
	if len(candidates) == 0 {
		return nil, false, nil
	}
	stickyRoute, hasSticky := s.stickyRoute(c, &req, policy)
	var bestAvailableCandidate core.Candidate
	var bestAvailableSnapshot core.RuntimeSnapshot
	var bestAvailableScore core.ScoreResult
	var bestSaturatedCandidate core.Candidate
	var bestSaturatedSnapshot core.RuntimeSnapshot
	var bestSaturatedScore core.ScoreResult
	var stickyCandidate core.Candidate
	var stickySnapshot core.RuntimeSnapshot
	var stickyScore core.ScoreResult
	stickySaturated := false
	var stickyBreak string
	stickyFound := false
	availableFound := false
	saturatedFound := false
	costReferenceRatio := 0.0
	explanations := make([]core.CandidateExplanation, 0, minInt(len(candidates), maxCandidateExplanations))
	evaluations := make([]candidateEvaluation, 0, len(candidates))
	for _, candidate := range candidates {
		snapshot := s.snapshotForCandidate(candidate, policy)
		if s.snapshotEnricher != nil {
			snapshot = s.snapshotEnricher.Enrich(candidate, snapshot, policy)
		}
		stickyMatched := hasSticky && isStickyCandidate(candidate, stickyRoute)
		rejectReason := candidateUnavailableReason(c, candidate, snapshot, policy)
		if stickyMatched && stickyBreak == "" && rejectReason != "" {
			stickyBreak = rejectReason
		}
		referenceCandidate := snapshot.CostReferenceRatio
		if referenceCandidate <= 0 && !(hasSticky && stickyRoute.CacheAware) {
			referenceCandidate = snapshot.CostRatio
		}
		if snapshot.CostRatio > 0 && referenceCandidate > 0 && (costReferenceRatio <= 0 || referenceCandidate < costReferenceRatio) {
			costReferenceRatio = referenceCandidate
		}
		evaluations = append(evaluations, candidateEvaluation{
			candidate:     candidate,
			snapshot:      snapshot,
			stickyMatched: stickyMatched,
			rejectReason:  rejectReason,
		})
	}
	for _, evaluation := range evaluations {
		candidate := evaluation.candidate
		snapshot := evaluation.snapshot
		if reference, ok := s.costReferenceForCandidate(candidate, snapshot, policy, costReferenceRatio); ok {
			snapshot.CostReferenceRatio = reference
		}
		scored := s.scorePreparedCandidate(candidate, snapshot, policy, evaluation.stickyMatched)
		explanation := scored.Explanation
		score := scored.Score
		if evaluation.rejectReason != "" {
			explanation.RejectReason = evaluation.rejectReason
			appendCandidateExplanation(&explanations, explanation)
			continue
		}
		explanation.Available = true
		if routingConcurrencySaturated(snapshot) {
			explanation.SelectionSkipReason = "concurrency_saturated"
		}
		appendCandidateExplanation(&explanations, explanation)
		if hasSticky && evaluation.stickyMatched {
			stickyCandidate = candidate
			stickySnapshot = snapshot
			stickyScore = score
			stickySaturated = routingConcurrencySaturated(snapshot)
			stickyFound = true
		}
		if routingConcurrencySaturated(snapshot) {
			if !saturatedFound || score.RoutingTotal > bestSaturatedScore.RoutingTotal {
				bestSaturatedCandidate = candidate
				bestSaturatedSnapshot = snapshot
				bestSaturatedScore = score
				saturatedFound = true
			}
			continue
		}
		if !availableFound || score.RoutingTotal > bestAvailableScore.RoutingTotal {
			bestAvailableCandidate = candidate
			bestAvailableSnapshot = snapshot
			bestAvailableScore = score
			availableFound = true
		}
	}
	var bestCandidate core.Candidate
	var bestSnapshot core.RuntimeSnapshot
	var bestScore core.ScoreResult
	selectedSaturated := false
	if availableFound {
		bestCandidate = bestAvailableCandidate
		bestSnapshot = bestAvailableSnapshot
		bestScore = bestAvailableScore
	} else if saturatedFound {
		bestCandidate = bestSaturatedCandidate
		bestSnapshot = bestSaturatedSnapshot
		bestScore = bestSaturatedScore
		selectedSaturated = true
	} else {
		return nil, false, nil
	}
	if bestCandidate.Channel == nil {
		return nil, false, nil
	}
	if hasSticky {
		if stickyFound {
			keepRatio := stickyRoute.KeepScoreRatio
			if keepRatio <= 0 {
				keepRatio = defaultStickyKeepScoreRatio
			}
			if stickySaturated && availableFound {
				stickyBreak = "concurrency_saturated"
			} else if shouldBreakStickyForCostFirst(
				policy,
				stickyRoute,
				stickyCandidate,
				stickySnapshot,
				stickyScore,
				bestCandidate,
				bestSnapshot,
				bestScore,
			) {
				stickyBreak = "cost_first_cheaper_higher_score"
			} else if stickyScore.Total >= bestScore.Total*keepRatio {
				bestCandidate = stickyCandidate
				bestSnapshot = stickySnapshot
				bestScore = stickyScore
				selectedSaturated = stickySaturated
				bestScore.Reason = stickyRoute.Source + "_retained"
				stickyBreak = ""
			} else if stickyBreak == "" {
				stickyBreak = "score_below_threshold"
			}
		} else if stickyBreak == "" {
			stickyBreak = "candidate_not_found"
		}
	}
	markSelectedCandidateExplanation(explanations, bestCandidate, bestSnapshot)
	if bestSnapshot.CircuitState == core.CircuitStateHalfOpen && s.snapshotEnricher != nil {
		if !s.snapshotEnricher.ReserveCircuitProbe(bestSnapshot.Key) {
			return nil, false, nil
		}
	}
	plan := &core.DispatchPlan{
		Channel:                   bestCandidate.Channel,
		SelectedGroup:             bestCandidate.Group,
		RequestedGroup:            req.RequestedGroup,
		RuntimeKey:                bestSnapshot.Key,
		ProviderProfile:           bestCandidate.ProviderProfile,
		ProxyMode:                 bestCandidate.ProxyMode,
		ScoreTotal:                bestScore.Total,
		ScoreBreakdown:            bestScore.Breakdown,
		RoutingScoreTotal:         bestScore.RoutingTotal,
		RoutingScoreBreakdown:     bestScore.RoutingBreakdown,
		QueueWaitMs:               selectedQueueWaitMs(bestSnapshot, policy, selectedSaturated),
		QueueEnabled:              policy.QueueEnabled,
		QueueDepth:                bestSnapshot.QueueDepth,
		QueueCapacity:             bestSnapshot.QueueCapacity,
		QueuePriority:             policy.QueuePriority,
		SelectedReason:            bestScore.Reason,
		StickySource:              stickyRoute.Source,
		StickyKeyFP:               stickyRoute.KeyFingerprint,
		StickyRetained:            hasSticky && stickyBreak == "",
		StickyBreak:               stickyBreak,
		CacheAffinity:             hasSticky && stickyRoute.CacheAware,
		PolicyMode:                policy.Mode,
		AutoMode:                  policy.AutoMode,
		RequiresCodexImageTool:    req.RequiresCodexImageTool,
		RequiredTools:             requiredToolsForDispatchRequest(req),
		CandidateFilterConditions: candidateFilterConditionsForDispatchRequest(req),
		Candidates:                explanations,
	}
	if s.shouldSaveStickyOnSelect() {
		s.stickyRouter.Save(c, &req, plan)
	}
	if plan.StickySource != "" && plan.SelectedReason == "weighted_score" && plan.StickyBreak != "" {
		plan.SelectedReason = "weighted_score_sticky_broken"
	}
	return plan, true, nil
}

func (s *DefaultSmartChannelSelector) ScoreCandidate(candidate core.Candidate, policy core.GroupSmartPolicy) CandidateScoreEvaluation {
	snapshot := defaultSnapshot(candidate)
	if s != nil {
		snapshot = s.snapshotForCandidate(candidate, policy)
		if s.snapshotEnricher != nil {
			snapshot = s.snapshotEnricher.Enrich(candidate, snapshot, policy)
		}
		fallbackReference := snapshot.CostReferenceRatio
		if fallbackReference <= 0 {
			fallbackReference = snapshot.CostRatio
		}
		if reference, ok := s.costReferenceForCandidate(candidate, snapshot, policy, fallbackReference); ok {
			snapshot.CostReferenceRatio = reference
		}
	}
	return s.scorePreparedCandidate(candidate, snapshot, policy, false)
}

func (s *DefaultSmartChannelSelector) scorePreparedCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, stickyMatched bool) CandidateScoreEvaluation {
	explanation := candidateExplanation(candidate, snapshot, stickyMatched)
	score := scoreCalculatorFactory(s).ForStrategy(policy.Strategy).Score(candidate, snapshot, policy)
	explanation.ScoreTotal = score.Total
	explanation.ScoreBreakdown = score.Breakdown
	explanation.RoutingScoreTotal = score.RoutingTotal
	explanation.RoutingScoreBreakdown = score.RoutingBreakdown
	applyScoredMetricsToCandidateExplanation(&explanation, score)
	return CandidateScoreEvaluation{
		Snapshot:    snapshot,
		Score:       score,
		Explanation: explanation,
	}
}

func scoreCalculatorFactory(selector *DefaultSmartChannelSelector) core.ScoreCalculatorFactory {
	if selector != nil && selector.scorerFactory != nil {
		return selector.scorerFactory
	}
	return NewScoreCalculatorFactory(DefaultScoreWeights())
}

func requiredToolsForDispatchRequest(req core.DispatchRequest) []string {
	tools := make([]string, 0, 1)
	if req.RequiresCodexImageTool {
		tools = append(tools, core.DispatchRequiredToolCodexImageGeneration)
	}
	return tools
}

func candidateFilterConditionsForDispatchRequest(req core.DispatchRequest) []string {
	conditions := make([]string, 0, 1)
	if req.RequiresCodexImageTool {
		conditions = append(conditions, core.DispatchFilterConditionCodexImageGenerationTool)
	}
	return conditions
}

func (s *DefaultSmartChannelSelector) costReferenceForCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, fallbackReference float64) (float64, bool) {
	if s == nil || s.costBaselineProvider == nil || snapshot.CostRatio <= 0 {
		if fallbackReference > 0 {
			return fallbackReference, true
		}
		if snapshot.CostReferenceRatio > 0 {
			return snapshot.CostReferenceRatio, true
		}
		return 0, false
	}
	groups := policy.CandidateGroups
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
	endpointType := strings.TrimSpace(string(snapshot.Key.EndpointType))
	best := 0.0
	for _, group := range groups {
		scope := core.CostBaselineScope{
			RequestedModel:         requestedModel,
			Group:                  group,
			EndpointType:           endpointType,
			RequiresCodexImageTool: candidate.RequiresCodexImageTool,
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
	if snapshot.CostReferenceRatio > 0 {
		return snapshot.CostReferenceRatio, true
	}
	return 0, false
}

func routingConcurrencySaturated(snapshot core.RuntimeSnapshot) bool {
	limit := routingConcurrencyLimit(snapshot)
	return limit > 0 && snapshot.ActiveConcurrency >= limit
}

func selectedQueueWaitMs(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, selectedSaturated bool) int {
	if !selectedSaturated || !policy.QueueEnabled {
		return 0
	}
	if snapshot.QueueCapacity > 0 && snapshot.QueueDepth >= snapshot.QueueCapacity {
		return 0
	}
	if snapshot.QueueTimeoutMs > 0 {
		return snapshot.QueueTimeoutMs
	}
	return defaultQueueTimeoutMs
}

func candidateExplanation(candidate core.Candidate, snapshot core.RuntimeSnapshot, stickyMatched bool) core.CandidateExplanation {
	explanation := core.CandidateExplanation{
		Group:                      candidate.Group,
		UpstreamModel:              candidate.UpstreamModel,
		ProviderProfile:            candidate.ProviderProfile,
		ProxyMode:                  candidate.ProxyMode,
		RuntimeKey:                 candidateExplanationRuntimeKey(candidate, snapshot),
		SuccessRate:                snapshot.SuccessRate,
		TTFTMs:                     snapshot.TTFTMs,
		DurationMs:                 snapshot.DurationMs,
		TokensPerSecond:            snapshot.TokensPerSecond,
		SampleCount:                snapshot.SampleCount,
		ActiveConcurrency:          snapshot.ActiveConcurrency,
		MaxConcurrency:             snapshot.MaxConcurrency,
		ConfiguredConcurrencyLimit: snapshot.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:    snapshot.LearnedConcurrencyLimit,
		EffectiveConcurrencyLimit:  snapshot.EffectiveConcurrencyLimit,
		QueueDepth:                 snapshot.QueueDepth,
		QueueCapacity:              snapshot.QueueCapacity,
		EstimatedQueueWaitMs:       snapshot.EstimatedQueueWaitMs,
		FirstBytePending:           snapshot.FirstBytePending,
		SlowFirstBytePending:       snapshot.SlowFirstBytePending,
		OldestFirstByteWaitMs:      snapshot.OldestFirstByteWaitMs,
		CostRatio:                  snapshot.CostRatio,
		CostReferenceRatio:         snapshot.CostReferenceRatio,
		CostPricingMode:            snapshot.CostPricingMode,
		GroupPriorityRatio:         snapshot.GroupPriorityRatio,
		SuccessScore:               snapshot.SuccessScore,
		SpeedScore:                 snapshot.SpeedScore,
		ExperienceScore:            snapshot.ExperienceScore,
		EmptyOutputRate:            snapshot.EmptyOutputRate,
		ExperienceIssueRate:        snapshot.ExperienceIssueRate,
		HealthScoreAverage:         snapshot.HealthScoreAverage,
		ProbeRecoveryPending:       snapshot.ProbeRecoveryPending,
		ProbeRecoverySuccessCount:  snapshot.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:      snapshot.ProbeRecoveryRequired,
		ProbeTriggerReason:         snapshot.ProbeTriggerReason,
		ConfigErrorIsolated:        snapshot.ConfigErrorIsolated,
		IsolationReason:            snapshot.IsolationReason,
		IsolationUntil:             snapshot.IsolationUntil,
		AuthConfigErrorCount:       snapshot.AuthConfigErrorCount,
		LastAuthConfigErrorAt:      snapshot.LastAuthConfigErrorAt,
		StickyMatched:              stickyMatched,
		ScoreSampleSource:          snapshot.SampleSource,
		MatchedRuntimeKey:          snapshot.MatchedRuntimeKey,
	}
	if candidate.Channel != nil {
		explanation.ChannelID = candidate.Channel.Id
		explanation.ChannelName = candidate.Channel.Name
		explanation.ChannelStatus = candidate.Channel.Status
		explanation.StatusReason = service.ChannelStatusReason(candidate.Channel)
		explanation.BalanceInsufficient = service.IsKnownBalanceInsufficientChannel(candidate.Channel)
	}
	if explanation.ChannelID == 0 {
		explanation.ChannelID = explanation.RuntimeKey.ChannelID
	}
	if explanation.Group == "" {
		explanation.Group = explanation.RuntimeKey.Group
	}
	if explanation.UpstreamModel == "" {
		explanation.UpstreamModel = explanation.RuntimeKey.UpstreamModel
	}
	explanation.LoadScore = healthLoadScore(snapshot)
	explanation.CostScore = costScore(snapshot)
	explanation.GroupScore = groupScore(snapshot)
	if snapshot.SampleCount > 0 && explanation.SuccessScore <= 0 {
		explanation.SuccessScore = successScore(snapshot)
	}
	if snapshot.SampleCount > 0 {
		if speedScore := displayedSpeedScore(snapshot); speedScore > 0 {
			explanation.SpeedScore = speedScore
		}
	}
	if explanation.ExperienceScore <= 0 {
		explanation.ExperienceScore = experienceScore(snapshot)
	}
	return explanation
}

func applyScoredMetricsToCandidateExplanation(explanation *core.CandidateExplanation, score core.ScoreResult) {
	if explanation == nil || len(score.Breakdown) == 0 {
		return
	}
	if value, ok := score.Breakdown[breakdownSuccess]; ok {
		explanation.SuccessScore = value
	}
	if value, ok := score.Breakdown[breakdownSpeed]; ok {
		explanation.ScoreSpeedFactor = value
	} else if explanation.SampleCount <= 0 {
		explanation.SpeedScore = 0
		explanation.ScoreSpeedFactor = 0
	}
	if value, ok := score.Breakdown[breakdownLoad]; ok {
		explanation.LoadScore = value
	}
	if value, ok := score.Breakdown[breakdownCost]; ok {
		explanation.CostScore = value
	}
	if value, ok := score.Breakdown[breakdownGroup]; ok {
		explanation.GroupScore = value
	}
	if value, ok := score.Breakdown["experience"]; ok {
		explanation.ExperienceScore = value
	}
}

func candidateExplanationRuntimeKey(candidate core.Candidate, snapshot core.RuntimeSnapshot) core.RuntimeKey {
	key := snapshot.Key
	if key.RequestedModel == "" {
		key.RequestedModel = candidate.RuntimeKey.RequestedModel
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = candidate.RuntimeKey.UpstreamModel
	}
	if key.ChannelID == 0 {
		key.ChannelID = candidate.RuntimeKey.ChannelID
	}
	if key.Group == "" {
		key.Group = candidate.RuntimeKey.Group
	}
	if key.EndpointType == "" {
		key.EndpointType = candidate.RuntimeKey.EndpointType
	}
	if key.CapabilityFingerprint == "" {
		key.CapabilityFingerprint = candidate.RuntimeKey.CapabilityFingerprint
	}
	return normalizeRuntimeKey(key)
}

func appendCandidateExplanation(explanations *[]core.CandidateExplanation, explanation core.CandidateExplanation) {
	if explanations == nil || len(*explanations) >= maxCandidateExplanations {
		return
	}
	*explanations = append(*explanations, explanation)
}

func markSelectedCandidateExplanation(explanations []core.CandidateExplanation, candidate core.Candidate, snapshot core.RuntimeSnapshot) {
	channelID := 0
	if candidate.Channel != nil {
		channelID = candidate.Channel.Id
	}
	if channelID == 0 {
		channelID = snapshot.Key.ChannelID
	}
	for idx := range explanations {
		if explanations[idx].ChannelID != channelID {
			continue
		}
		if snapshot.Key.Group != "" && explanations[idx].RuntimeKey.Group != "" && explanations[idx].RuntimeKey.Group != snapshot.Key.Group {
			continue
		}
		if snapshot.Key.RequestedModel != "" && explanations[idx].RuntimeKey.RequestedModel != "" && explanations[idx].RuntimeKey.RequestedModel != snapshot.Key.RequestedModel {
			continue
		}
		if snapshot.Key.EndpointType != "" && explanations[idx].RuntimeKey.EndpointType != "" && explanations[idx].RuntimeKey.EndpointType != snapshot.Key.EndpointType {
			continue
		}
		explanations[idx].Selected = true
		return
	}
}

func (s *DefaultSmartChannelSelector) snapshotForCandidate(candidate core.Candidate, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	base := defaultSnapshot(candidate)
	if s.snapshotEnricher != nil {
		base = s.snapshotEnricher.Enrich(candidate, base, policy)
	}
	base = normalizeRuntimeSnapshot(base)
	if s.snapshotStore == nil {
		return base
	}
	if snapshot, ok := s.snapshotStore.Get(base.Key); ok {
		snapshot = normalizeRuntimeSnapshot(snapshot)
		if s.snapshotEnricher != nil {
			snapshot = mergeRuntimeSnapshotDynamicFields(snapshot, base)
		}
		snapshot.MatchedRuntimeKey = snapshot.Key
		snapshot.SampleSource = "exact"
		return snapshot
	}
	legacyKey := normalizeRuntimeKey(candidate.RuntimeKey)
	if legacyKey != base.Key {
		if snapshot, ok := s.snapshotStore.Get(legacyKey); ok {
			snapshot = normalizeRuntimeSnapshot(snapshot)
			if s.snapshotEnricher != nil {
				snapshot = mergeRuntimeSnapshotDynamicFields(snapshot, base)
			}
			snapshot.MatchedRuntimeKey = snapshot.Key
			snapshot.SampleSource = "legacy_exact"
			return snapshot
		}
	}
	if snapshot, ok := s.fallbackSnapshotForCandidate(candidate); ok {
		return snapshot
	}
	return base
}

func defaultSnapshot(candidate core.Candidate) core.RuntimeSnapshot {
	key := candidate.RuntimeKey
	if key.ChannelID == 0 && candidate.Channel != nil {
		key.ChannelID = candidate.Channel.Id
	}
	if key.Group == "" {
		key.Group = candidate.Group
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = candidate.UpstreamModel
	}
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	return core.RuntimeSnapshot{
		Key:                normalizeRuntimeKey(key),
		CostRatio:          0,
		GroupPriorityRatio: 1,
		SampleSource:       "none",
	}
}

func (s *DefaultSmartChannelSelector) fallbackSnapshotForCandidate(candidate core.Candidate) (core.RuntimeSnapshot, bool) {
	if s == nil || s.snapshotStore == nil {
		return core.RuntimeSnapshot{}, false
	}
	base := defaultSnapshot(candidate)
	best, ok := mostRelevantSnapshot(base.Key, s.snapshotStore.ListCandidates(&core.DispatchRequest{
		ModelName: base.Key.RequestedModel,
	}))
	if !ok {
		return core.RuntimeSnapshot{}, false
	}
	best = normalizeRuntimeSnapshot(best)
	matchedKey := best.Key
	if s.snapshotEnricher != nil {
		best = mergeRuntimeSnapshotDynamicFields(best, base)
	}
	best.Key = base.Key
	if best.Key.ChannelID == 0 {
		best.Key.ChannelID = matchedKey.ChannelID
	}
	if best.Key.RequestedModel == "" {
		best.Key.RequestedModel = matchedKey.RequestedModel
	}
	if best.Key.UpstreamModel == "" {
		best.Key.UpstreamModel = matchedKey.UpstreamModel
	}
	if best.Key.Group == "" {
		best.Key.Group = matchedKey.Group
	}
	if best.Key.EndpointType == "" {
		best.Key.EndpointType = matchedKey.EndpointType
	}
	if best.Key.CapabilityFingerprint == "" {
		best.Key.CapabilityFingerprint = matchedKey.CapabilityFingerprint
	}
	best.MatchedRuntimeKey = matchedKey
	best.SampleSource = "similar"
	return best, true
}

func mergeRuntimeSnapshotDynamicFields(snapshot core.RuntimeSnapshot, dynamic core.RuntimeSnapshot) core.RuntimeSnapshot {
	snapshot.Key = dynamic.Key
	snapshot.CostRatio = dynamic.CostRatio
	snapshot.CostReferenceRatio = dynamic.CostReferenceRatio
	snapshot.CostPricingMode = dynamic.CostPricingMode
	if dynamic.GroupPriorityRatio > 0 {
		snapshot.GroupPriorityRatio = dynamic.GroupPriorityRatio
	}
	snapshot.ActiveConcurrency = dynamic.ActiveConcurrency
	snapshot.MaxConcurrency = dynamic.MaxConcurrency
	snapshot.ConfiguredConcurrencyLimit = dynamic.ConfiguredConcurrencyLimit
	snapshot.LearnedConcurrencyLimit = dynamic.LearnedConcurrencyLimit
	snapshot.EffectiveConcurrencyLimit = dynamic.EffectiveConcurrencyLimit
	snapshot.QueueDepth = dynamic.QueueDepth
	snapshot.QueueCapacity = dynamic.QueueCapacity
	snapshot.QueueTimeoutMs = dynamic.QueueTimeoutMs
	snapshot.EstimatedQueueWaitMs = dynamic.EstimatedQueueWaitMs
	snapshot.FirstBytePending = dynamic.FirstBytePending
	snapshot.SlowFirstBytePending = dynamic.SlowFirstBytePending
	snapshot.OldestFirstByteWaitMs = dynamic.OldestFirstByteWaitMs
	snapshot.CircuitState = dynamic.CircuitState
	snapshot.CircuitOpen = dynamic.CircuitOpen
	snapshot.Cooldown = dynamic.Cooldown
	snapshot.FailureAvoidance = dynamic.FailureAvoidance
	snapshot.HealthScoreAverage = dynamic.HealthScoreAverage
	snapshot.ProbeRecoveryPending = dynamic.ProbeRecoveryPending
	snapshot.ProbeRecoverySuccessCount = dynamic.ProbeRecoverySuccessCount
	snapshot.ProbeRecoveryRequired = dynamic.ProbeRecoveryRequired
	snapshot.ProbeTriggerReason = dynamic.ProbeTriggerReason
	snapshot.ConfigErrorIsolated = dynamic.ConfigErrorIsolated
	snapshot.IsolationReason = dynamic.IsolationReason
	snapshot.IsolationUntil = dynamic.IsolationUntil
	snapshot.AuthConfigErrorCount = dynamic.AuthConfigErrorCount
	snapshot.LastAuthConfigErrorAt = dynamic.LastAuthConfigErrorAt
	return snapshot
}

func mostRelevantSnapshot(key core.RuntimeKey, snapshots []core.RuntimeSnapshot) (core.RuntimeSnapshot, bool) {
	var best core.RuntimeSnapshot
	bestScore := -1
	for _, snapshot := range snapshots {
		snapshot = normalizeRuntimeSnapshot(snapshot)
		if snapshot.SampleCount <= 0 || snapshot.Key.ChannelID <= 0 {
			continue
		}
		if key.ChannelID > 0 && snapshot.Key.ChannelID != key.ChannelID {
			continue
		}
		score := 0
		if snapshot.Key.RequestedModel != "" && key.RequestedModel != "" && snapshot.Key.RequestedModel == key.RequestedModel {
			score += 32
		}
		if snapshot.Key.UpstreamModel != "" && key.UpstreamModel != "" && snapshot.Key.UpstreamModel == key.UpstreamModel {
			score += 16
		}
		if snapshot.Key.EndpointType != "" && key.EndpointType != "" && snapshot.Key.EndpointType == key.EndpointType {
			score += 12
		}
		if snapshot.Key.CapabilityFingerprint != "" && key.CapabilityFingerprint != "" && snapshot.Key.CapabilityFingerprint == key.CapabilityFingerprint {
			score += 8
		}
		if snapshot.Key.CapabilityFingerprint != "" && key.CapabilityFingerprint != "" && runtimeFingerprintOverlaps(snapshot.Key.CapabilityFingerprint, key.CapabilityFingerprint) {
			score += 4
		}
		if snapshot.Key.Group != "" && key.Group != "" && snapshot.Key.Group == key.Group {
			score += 4
		}
		if score == 0 {
			continue
		}
		if score > bestScore || (score == bestScore && snapshot.SampleCount > best.SampleCount) {
			best = snapshot
			bestScore = score
		}
	}
	return best, bestScore >= 0
}

func runtimeFingerprintOverlaps(left string, right string) bool {
	leftParts := runtimeFingerprintSet(left)
	if len(leftParts) == 0 {
		return false
	}
	for part := range runtimeFingerprintSet(right) {
		if _, ok := leftParts[part]; ok {
			return true
		}
	}
	return false
}

func runtimeFingerprintSet(value string) map[string]struct{} {
	parts := strings.Split(value, "|")
	out := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = struct{}{}
		}
	}
	return out
}

func (s *DefaultSmartChannelSelector) stickyRoute(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) (core.StickyRoute, bool) {
	if s == nil || s.stickyRouter == nil {
		return core.StickyRoute{}, false
	}
	return s.stickyRouter.Route(c, req, policy)
}

func (s *DefaultSmartChannelSelector) shouldSaveStickyOnSelect() bool {
	if s == nil || s.stickyRouter == nil {
		return false
	}
	if router, ok := s.stickyRouter.(stickySaveOnSelectRouter); ok {
		return router.SaveOnSelect()
	}
	return true
}

func isStickyCandidate(candidate core.Candidate, route core.StickyRoute) bool {
	if candidate.Channel == nil || route.ChannelID <= 0 {
		return false
	}
	if candidate.Channel.Id != route.ChannelID {
		return false
	}
	if route.Group != "" && candidate.Group != route.Group {
		return false
	}
	return true
}

func shouldBreakStickyForCostFirst(policy core.GroupSmartPolicy, route core.StickyRoute, stickyCandidate core.Candidate, stickySnapshot core.RuntimeSnapshot, stickyScore core.ScoreResult, bestCandidate core.Candidate, bestSnapshot core.RuntimeSnapshot, bestScore core.ScoreResult) bool {
	if policy.Strategy != core.StrategyCostFirst {
		return false
	}
	if sameRoutingCandidate(stickyCandidate, bestCandidate) {
		return false
	}
	stickyCost := stickyEscapeCost(stickySnapshot)
	bestCost := stickyEscapeCost(bestSnapshot)
	if stickyCost <= 0 || bestCost <= 0 {
		return false
	}
	if stickySnapshot.SampleCount < costFirstStickyEscapeMinSamples || bestSnapshot.SampleCount < costFirstStickyEscapeMinSamples {
		return false
	}
	costRatio := costFirstStickyEscapeCostRatio
	scoreDelta := costFirstStickyEscapeScoreDelta
	if route.CacheAware {
		costRatio = costFirstStickyEscapeCacheCostRatio
		scoreDelta = costFirstStickyEscapeCacheScoreDelta
	}
	if bestCost > stickyCost*costRatio {
		return false
	}
	if bestScore.RoutingTotal < stickyScore.RoutingTotal+scoreDelta {
		return false
	}
	if bestScore.Total < stickyScore.Total+scoreDelta/2 {
		return false
	}
	if bestSnapshot.SuccessRate+costFirstStickyEscapeSuccessSlack < stickySnapshot.SuccessRate {
		return false
	}
	return true
}

func stickyEscapeCost(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CostRatio <= 0 {
		return 0
	}
	groupRatio := snapshot.GroupPriorityRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}
	return snapshot.CostRatio * groupRatio
}

func sameRoutingCandidate(left core.Candidate, right core.Candidate) bool {
	leftChannelID := 0
	if left.Channel != nil {
		leftChannelID = left.Channel.Id
	}
	rightChannelID := 0
	if right.Channel != nil {
		rightChannelID = right.Channel.Id
	}
	if leftChannelID != rightChannelID {
		return false
	}
	if left.Group != "" && right.Group != "" && left.Group != right.Group {
		return false
	}
	if left.RuntimeKey.Group != "" && right.RuntimeKey.Group != "" && left.RuntimeKey.Group != right.RuntimeKey.Group {
		return false
	}
	return true
}

func candidateUnavailableReason(c *gin.Context, candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) string {
	if snapshot.ConfigErrorIsolated {
		return "config_error_isolated"
	}
	if candidate.Channel != nil && service.IsChannelBalanceSkipped(c, candidate.Channel.Id) {
		return service.ChannelStatusReasonBalanceInsufficient
	}
	if candidate.Channel != nil && service.IsKnownBalanceInsufficientChannel(candidate.Channel) {
		return service.ChannelStatusReasonBalanceInsufficient
	}
	if candidate.Channel != nil && service.IsRuntimeBalanceInsufficientChannelID(candidate.Channel.Id) {
		return service.ChannelStatusReasonBalanceInsufficient
	}
	if candidate.Channel != nil && service.IsChannelSelectionSkipped(c, candidate.Channel.Id) {
		return "routing_slot_reserved"
	}
	if snapshot.CircuitOpen {
		return "circuit_open"
	}
	if snapshot.Cooldown {
		return "cooldown"
	}
	if snapshot.FailureAvoidance {
		return "failure_avoidance"
	}
	return ""
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewGetChannelFailedError(err error) *types.NewAPIError {
	if err == nil {
		err = errors.New("smart channel selector failed")
	}
	return types.NewError(err, types.ErrorCodeGetChannelFailed)
}

var _ core.SmartChannelSelector = (*DefaultSmartChannelSelector)(nil)
