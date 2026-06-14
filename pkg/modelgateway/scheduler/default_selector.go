package scheduler

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const maxCandidateExplanations = 32

const (
	channelPriorityTieBreakReason            = "channel_priority_tie_break"
	channelPriorityTieBreakDefaultScoreDelta = 0.05
	channelPriorityTieBreakScoreEpsilon      = 0.000000001
	channelPriorityTieBreakMaxScoreDelta     = 1
	costFirstStickyEscapeCostRatio           = 0.75
	costFirstStickyEscapeCacheCostRatio      = 0.55
	costFirstStickyEscapeMaxSpeedDrop        = 0.06
	costFirstStickyEscapeCacheSpeedDrop      = 0.03
	costFirstStickyEscapeMinSamples          = 5
	costFirstStickyEscapeSuccessSlack        = 0.02
	negativeCurrentGroupMarginReason         = "negative_current_group_margin"
	negativeMarginFallbackReason             = "negative_margin_fallback"
	failureRecoveryProbeSelectedReason       = "failure_recovery_probe"
	failureRecoveryProbeInFlightReason       = "failure_recovery_probe_inflight"
	FailureRecoveryProbeSelectedReason       = failureRecoveryProbeSelectedReason
)

type DefaultSmartChannelSelector struct {
	candidateBuilder              core.CandidatePoolBuilder
	snapshotStore                 core.RuntimeSnapshotStore
	scoreWeights                  core.ScoreWeights
	snapshotEnricher              core.RuntimeSnapshotEnricher
	stickyRouter                  core.StickyRouter
	costBaselineProvider          core.CostBaselineProvider
	scoringService                *CandidateScoringService
	stickyEscapeConfig            CostFirstStickyEscapeConfig
	costGuardConfig               CostFirstGuardConfig
	channelPriorityTieBreakConfig ChannelPriorityTieBreakConfig
	emptyOutputSwitch             *ClientEmptyOutputSwitchTracker
}

type ChannelPriorityTieBreakConfig struct {
	Enabled    bool
	ScoreDelta float64
}

type CostFirstStickyEscapeConfig struct {
	Enabled        bool
	CostRatio      float64
	CacheCostRatio float64
	MaxSpeedDrop   float64
	CacheSpeedDrop float64
	MinSamples     int
	SuccessSlack   float64
}

type CostFirstGuardConfig struct {
	Enabled          bool
	Multiple         float64
	SuccessAdvantage float64
	SpeedAdvantage   float64
}

type stickySaveOnSelectRouter interface {
	SaveOnSelect() bool
}

type candidateEvaluation struct {
	candidate      core.Candidate
	snapshot       core.RuntimeSnapshot
	score          core.ScoreResult
	stickyMatched  bool
	rejectReason   string
	negativeMargin bool
}

type costFirstGuardResult struct {
	baseline         candidateEvaluation
	decision         *core.CostGuardDecision
	switchToBaseline bool
}

type CandidateScoreEvaluation struct {
	Snapshot    core.RuntimeSnapshot
	Score       core.ScoreResult
	Explanation core.CandidateExplanation
}

func NewDefaultSmartChannelSelector(candidateBuilder core.CandidatePoolBuilder, snapshotStore core.RuntimeSnapshotStore, weights core.ScoreWeights) *DefaultSmartChannelSelector {
	return &DefaultSmartChannelSelector{
		candidateBuilder:              candidateBuilder,
		snapshotStore:                 snapshotStore,
		scoreWeights:                  weights,
		stickyEscapeConfig:            DefaultCostFirstStickyEscapeConfig(),
		costGuardConfig:               DefaultCostFirstGuardConfig(),
		channelPriorityTieBreakConfig: DefaultChannelPriorityTieBreakConfig(),
	}
}

func DefaultChannelPriorityTieBreakConfig() ChannelPriorityTieBreakConfig {
	return ChannelPriorityTieBreakConfig{
		Enabled:    true,
		ScoreDelta: channelPriorityTieBreakDefaultScoreDelta,
	}
}

func (c ChannelPriorityTieBreakConfig) normalized() ChannelPriorityTieBreakConfig {
	if c.ScoreDelta <= 0 {
		c.ScoreDelta = channelPriorityTieBreakDefaultScoreDelta
	}
	if c.ScoreDelta > channelPriorityTieBreakMaxScoreDelta {
		c.ScoreDelta = channelPriorityTieBreakMaxScoreDelta
	}
	return c
}

func DefaultCostFirstStickyEscapeConfig() CostFirstStickyEscapeConfig {
	return CostFirstStickyEscapeConfig{
		Enabled:        true,
		CostRatio:      costFirstStickyEscapeCostRatio,
		CacheCostRatio: costFirstStickyEscapeCacheCostRatio,
		MaxSpeedDrop:   costFirstStickyEscapeMaxSpeedDrop,
		CacheSpeedDrop: costFirstStickyEscapeCacheSpeedDrop,
		MinSamples:     costFirstStickyEscapeMinSamples,
		SuccessSlack:   costFirstStickyEscapeSuccessSlack,
	}
}

func DefaultCostFirstGuardConfig() CostFirstGuardConfig {
	return CostFirstGuardConfig{
		Enabled:          true,
		Multiple:         costFirstGuardMultiple,
		SuccessAdvantage: costFirstGuardSuccessAdvantage,
		SpeedAdvantage:   costFirstGuardSpeedAdvantage,
	}
}

func (c CostFirstGuardConfig) normalized() CostFirstGuardConfig {
	if c.Multiple <= 1 {
		c.Multiple = costFirstGuardMultiple
	}
	if c.SuccessAdvantage < 0 {
		c.SuccessAdvantage = 0
	}
	if c.SpeedAdvantage < 0 {
		c.SpeedAdvantage = 0
	}
	return c
}

func (c CostFirstStickyEscapeConfig) normalized() CostFirstStickyEscapeConfig {
	if c.CostRatio <= 0 {
		c.CostRatio = costFirstStickyEscapeCostRatio
	}
	if c.CacheCostRatio <= 0 {
		c.CacheCostRatio = costFirstStickyEscapeCacheCostRatio
	}
	if c.MaxSpeedDrop < 0 {
		c.MaxSpeedDrop = 0
	}
	if c.CacheSpeedDrop < 0 {
		c.CacheSpeedDrop = 0
	}
	if c.MinSamples <= 0 {
		c.MinSamples = costFirstStickyEscapeMinSamples
	}
	if c.SuccessSlack < 0 {
		c.SuccessSlack = 0
	}
	return c
}

func (s *DefaultSmartChannelSelector) WithCostFirstGuardConfig(config CostFirstGuardConfig) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.costGuardConfig = config.normalized()
	return s
}

func (s *DefaultSmartChannelSelector) WithChannelPriorityTieBreakConfig(config ChannelPriorityTieBreakConfig) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.channelPriorityTieBreakConfig = config.normalized()
	return s
}

func (s *DefaultSmartChannelSelector) WithClientEmptyOutputSwitchTracker(tracker *ClientEmptyOutputSwitchTracker) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.emptyOutputSwitch = tracker
	return s
}

func (s *DefaultSmartChannelSelector) WithCostFirstStickyEscapeConfig(config CostFirstStickyEscapeConfig) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.stickyEscapeConfig = config.normalized()
	return s
}

func (s *DefaultSmartChannelSelector) WithRuntimeSnapshotEnricher(enricher core.RuntimeSnapshotEnricher) *DefaultSmartChannelSelector {
	if s == nil {
		return nil
	}
	s.snapshotEnricher = enricher
	s.scoringService = nil
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
	s.scoringService = nil
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
	retryIntent := req.RetryRoutingIntent
	if policy.RequestedGroup == "" {
		policy.RequestedGroup = req.RequestedGroup
	}
	if policy.UserID <= 0 {
		policy.UserID = req.UserID
	}
	if policy.UserGroup == "" {
		policy.UserGroup = req.UserGroup
	}
	candidates := s.candidateBuilder.Build(&req, policy)
	if len(candidates) == 0 {
		return nil, false, nil
	}
	var resourceDecision resourceProtectionDecision
	var resourceErr *types.NewAPIError
	candidates, resourceDecision, resourceErr = s.resourceProtectionCandidates(c, req, policy, candidates)
	if resourceErr != nil {
		return nil, true, resourceErr
	}
	if len(candidates) == 0 {
		return nil, false, nil
	}
	stickyRoute, hasSticky := s.stickyRoute(c, &req, policy)
	var stickyCandidate core.Candidate
	var stickySnapshot core.RuntimeSnapshot
	var stickyScore core.ScoreResult
	stickySaturated := false
	var stickyBreak string
	var stickyDecision *core.StickyDecision
	stickyFound := false
	availableFound := false
	saturatedFound := false
	costReferenceRatio := 0.0
	explanations := make([]core.CandidateExplanation, 0, minInt(len(candidates), maxCandidateExplanations))
	evaluations := make([]candidateEvaluation, 0, len(candidates))
	availableEvaluations := make([]candidateEvaluation, 0, len(candidates))
	saturatedEvaluations := make([]candidateEvaluation, 0, len(candidates))
	negativeAvailableEvaluations := make([]candidateEvaluation, 0, len(candidates))
	negativeSaturatedEvaluations := make([]candidateEvaluation, 0, len(candidates))
	for _, candidate := range candidates {
		snapshot := s.snapshotForCandidate(candidate, policy)
		if s.snapshotEnricher != nil {
			snapshot = s.snapshotEnricher.Enrich(candidate, snapshot, policy)
		}
		stickyMatched := hasSticky && isStickyCandidate(candidate, stickyRoute)
		negativeMargin := negativeCurrentGroupMargin(snapshot)
		rejectReason := s.candidateUnavailableReason(c, req, candidate, snapshot, policy)
		if rejectReason == ClientEmptyOutputSwitchReason {
			markClientEmptyOutputSwitchChannelSkipped(c, candidate, snapshot)
		}
		if stickyMatched && stickyBreak == "" && rejectReason != "" {
			stickyBreak = rejectReason
		} else if stickyMatched && stickyBreak == "" && negativeMargin {
			stickyBreak = negativeCurrentGroupMarginReason
		}
		referenceCandidate := snapshot.CostReferenceRatio
		if referenceCandidate <= 0 && !(hasSticky && stickyRoute.CacheAware) {
			referenceCandidate = snapshot.CostRatio
		}
		if snapshot.CostRatio > 0 && referenceCandidate > 0 && (costReferenceRatio <= 0 || referenceCandidate < costReferenceRatio) {
			costReferenceRatio = referenceCandidate
		}
		evaluations = append(evaluations, candidateEvaluation{
			candidate:      candidate,
			snapshot:       snapshot,
			stickyMatched:  stickyMatched,
			rejectReason:   rejectReason,
			negativeMargin: negativeMargin,
		})
	}
	for _, evaluation := range evaluations {
		candidate := evaluation.candidate
		snapshot := evaluation.snapshot
		if reference, ok := s.costReferenceForCandidate(candidate, snapshot, policy, costReferenceRatio); ok {
			snapshot.CostReferenceRatio = reference
		}
		scored := s.scorePreparedCandidate(candidate, snapshot, policy, evaluation.stickyMatched && !evaluation.negativeMargin, retryIntent)
		explanation := scored.Explanation
		applyResourceProtectionExplanation(&explanation, candidate, resourceDecision)
		score := scored.Score
		explanation.NegativeCurrentGroupMargin = evaluation.negativeMargin
		if evaluation.rejectReason != "" {
			explanation.RejectReason = evaluation.rejectReason
			if evaluation.rejectReason == ClientEmptyOutputSwitchReason && s.emptyOutputSwitch != nil {
				if avoidance, ok := s.emptyOutputSwitch.AvoidanceInfo(req, candidate, snapshot); ok {
					explanation.ClientEmptyOutputSessionKey = avoidance.Scope.SessionKey
					explanation.ClientEmptyOutputAvoidUntil = avoidance.Until
					explanation.ClientEmptyOutputRemainingSeconds = avoidance.RemainingSeconds
				}
			}
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
			stickyFound = !evaluation.negativeMargin
		}
		if routingConcurrencySaturated(snapshot) {
			evaluation.snapshot = snapshot
			evaluation.score = score
			if evaluation.negativeMargin {
				negativeSaturatedEvaluations = append(negativeSaturatedEvaluations, evaluation)
			} else {
				saturatedEvaluations = append(saturatedEvaluations, evaluation)
				saturatedFound = true
			}
			continue
		}
		evaluation.snapshot = snapshot
		evaluation.score = score
		if evaluation.negativeMargin {
			negativeAvailableEvaluations = append(negativeAvailableEvaluations, evaluation)
		} else {
			availableEvaluations = append(availableEvaluations, evaluation)
			availableFound = true
		}
	}
	var bestCandidate core.Candidate
	var bestSnapshot core.RuntimeSnapshot
	var bestScore core.ScoreResult
	var prioritySelectedEvaluation candidateEvaluation
	priorityTieBreakUsed := false
	var costGuardDecision *core.CostGuardDecision
	selectedSaturated := false
	negativeMarginFallbackSelected := false
	if availableFound {
		selectedEvaluation, tieBreakUsed := s.selectCandidateByRoutingScoreAndChannelPriority(availableEvaluations)
		bestCandidate = selectedEvaluation.candidate
		bestSnapshot = selectedEvaluation.snapshot
		bestScore = selectedEvaluation.score
		prioritySelectedEvaluation = selectedEvaluation
		priorityTieBreakUsed = tieBreakUsed
	} else if saturatedFound {
		selectedEvaluation, tieBreakUsed := s.selectCandidateByRoutingScoreAndChannelPriority(saturatedEvaluations)
		bestCandidate = selectedEvaluation.candidate
		bestSnapshot = selectedEvaluation.snapshot
		bestScore = selectedEvaluation.score
		prioritySelectedEvaluation = selectedEvaluation
		priorityTieBreakUsed = tieBreakUsed
		selectedSaturated = true
	} else if len(negativeAvailableEvaluations) > 0 {
		selectedEvaluation, tieBreakUsed := s.selectNegativeMarginFallbackCandidate(negativeAvailableEvaluations)
		bestCandidate = selectedEvaluation.candidate
		bestSnapshot = selectedEvaluation.snapshot
		bestScore = selectedEvaluation.score
		prioritySelectedEvaluation = selectedEvaluation
		priorityTieBreakUsed = tieBreakUsed
		negativeMarginFallbackSelected = true
	} else if len(negativeSaturatedEvaluations) > 0 {
		selectedEvaluation, tieBreakUsed := s.selectNegativeMarginFallbackCandidate(negativeSaturatedEvaluations)
		bestCandidate = selectedEvaluation.candidate
		bestSnapshot = selectedEvaluation.snapshot
		bestScore = selectedEvaluation.score
		prioritySelectedEvaluation = selectedEvaluation
		priorityTieBreakUsed = tieBreakUsed
		selectedSaturated = true
		negativeMarginFallbackSelected = true
	} else {
		return nil, false, nil
	}
	if bestCandidate.Channel == nil {
		return nil, false, nil
	}
	if !hasSticky {
		if guard, ok := s.costFirstGuardCandidate(policy, bestCandidate, bestSnapshot, bestScore, availableEvaluations, retryIntent); ok {
			costGuardDecision = guard.decision
			if guard.switchToBaseline {
				bestCandidate = guard.baseline.candidate
				bestSnapshot = guard.baseline.snapshot
				bestScore = guard.baseline.score
				selectedSaturated = false
				bestScore.Reason = guard.decision.Reason
			}
		}
	}
	if hasSticky && !stickyFound {
		if guard, ok := s.costFirstGuardCandidate(policy, bestCandidate, bestSnapshot, bestScore, availableEvaluations, retryIntent); ok {
			costGuardDecision = guard.decision
			if guard.switchToBaseline {
				bestCandidate = guard.baseline.candidate
				bestSnapshot = guard.baseline.snapshot
				bestScore = guard.baseline.score
				selectedSaturated = false
				bestScore.Reason = guard.decision.Reason
			}
		}
	}
	if hasSticky {
		if stickyFound {
			keepRatio := stickyRoute.KeepScoreRatio
			if keepRatio <= 0 {
				keepRatio = defaultStickyKeepScoreRatio
			}
			if stickySaturated && availableFound {
				stickyBreak = "concurrency_saturated"
			} else if escape, decision, ok := s.costFirstStickyEscapeCandidate(
				policy,
				stickyRoute,
				stickyCandidate,
				stickySnapshot,
				stickyScore,
				availableEvaluations,
			); ok {
				bestCandidate = escape.candidate
				bestSnapshot = escape.snapshot
				bestScore = escape.score
				selectedSaturated = false
				stickyBreak = "cost_first_cheaper_speed_acceptable"
				stickyDecision = decision
			} else if policy.Strategy == core.StrategyCostFirst {
				stickyDecision = decision
				if guard, ok := s.costFirstGuardCandidate(policy, stickyCandidate, stickySnapshot, stickyScore, availableEvaluations, retryIntent); ok && guard.switchToBaseline {
					costGuardDecision = guard.decision
					bestCandidate = guard.baseline.candidate
					bestSnapshot = guard.baseline.snapshot
					bestScore = guard.baseline.score
					selectedSaturated = false
					bestScore.Reason = guard.decision.Reason
					stickyBreak = "cost_first_guard_baseline_selected"
					stickyDecision = stickyEscapeDecision(
						"cost_first_guard_baseline_selected",
						"switch",
						stickyRoute,
						s.stickyEscapeConfig.normalized(),
						stickySnapshot,
						stickyEscapeCost(stickySnapshot),
						0,
						guard.baseline,
						costFirstEvaluationCost(guard.baseline),
						0,
						1/guard.decision.CostGuardMultiple,
						0,
					)
				} else {
					bestCandidate = stickyCandidate
					bestSnapshot = stickySnapshot
					bestScore = stickyScore
					selectedSaturated = stickySaturated
					bestScore.Reason = stickyRoute.Source + "_retained"
					stickyBreak = ""
				}
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
	finalEvaluation := candidateEvaluation{candidate: bestCandidate, snapshot: bestSnapshot, score: bestScore}
	if priorityTieBreakUsed &&
		candidateEvaluationMatches(finalEvaluation, prioritySelectedEvaluation) &&
		channelPriorityTieBreakCanOwnReason(bestScore.Reason) {
		bestScore.Reason = channelPriorityTieBreakReason
		finalEvaluation.score = bestScore
		if selectedSaturated {
			markChannelPriorityTieBreakCandidateExplanations(explanations, finalEvaluation, saturatedEvaluations, s.channelPriorityTieBreakConfig.normalized())
		} else {
			markChannelPriorityTieBreakCandidateExplanations(explanations, finalEvaluation, availableEvaluations, s.channelPriorityTieBreakConfig.normalized())
		}
	}
	if negativeMarginFallbackSelected {
		bestScore.Reason = negativeMarginFallbackReason
		finalEvaluation.score = bestScore
	} else {
		markNegativeMarginSkippedCandidateExplanations(explanations, append(negativeAvailableEvaluations, negativeSaturatedEvaluations...))
	}
	if failureAvoidanceCanUseBusinessProbe(bestSnapshot) {
		bestScore.Reason = failureRecoveryProbeSelectedReason
		finalEvaluation.score = bestScore
	}
	if !reserveCandidateAccountRateLimit(bestCandidate) {
		return s.Select(c, param, policy)
	}
	markSelectedCandidateExplanation(explanations, bestCandidate, bestSnapshot)
	markCostGuardCandidateExplanations(explanations, costGuardDecision)
	if bestSnapshot.CircuitState == core.CircuitStateHalfOpen && s.snapshotEnricher != nil {
		if !s.snapshotEnricher.ReserveCircuitProbe(bestSnapshot.Key) {
			return nil, false, nil
		}
	}
	stickySaveSuppressed := negativeCurrentGroupMargin(bestSnapshot)
	stickySuppressionReason := ""
	if stickySaveSuppressed {
		stickySuppressionReason = negativeCurrentGroupMarginReason
	}
	plan := &core.DispatchPlan{
		Channel:                     bestCandidate.Channel,
		ResourceRef:                 bestCandidate.ResourceRef,
		AccountIdentity:             bestCandidate.AccountIdentity,
		CredentialRef:               bestCandidate.CredentialRef,
		ProxyRef:                    bestCandidate.ProxyRef,
		SelectedGroup:               bestCandidate.Group,
		RequestedGroup:              req.RequestedGroup,
		RuntimeKey:                  bestSnapshot.Key,
		ProviderProfile:             bestCandidate.ProviderProfile,
		ProxyMode:                   bestCandidate.ProxyMode,
		ScoreTotal:                  bestScore.Total,
		ScoreBreakdown:              bestScore.Breakdown,
		RoutingScoreTotal:           bestScore.RoutingTotal,
		RoutingScoreBreakdown:       bestScore.RoutingBreakdown,
		QueueWaitMs:                 selectedQueueWaitMs(bestSnapshot, policy, selectedSaturated, resourceDecision),
		QueueEnabled:                resourceDecision.queueEnabled(selectedSaturated, policy),
		QueueDepth:                  bestSnapshot.QueueDepth,
		QueueCapacity:               selectedQueueCapacity(bestSnapshot, resourceDecision, selectedSaturated),
		QueuePriority:               queuePriorityForRetryIntent(policy.QueuePriority, retryIntent),
		SelectedReason:              bestScore.Reason,
		StickySource:                stickyRoute.Source,
		StickyKeyFP:                 stickyRoute.KeyFingerprint,
		StickyRetained:              hasSticky && stickyBreak == "",
		StickyBreak:                 stickyBreak,
		StickyDecision:              stickyDecision,
		CacheAffinity:               hasSticky && stickyRoute.CacheAware,
		StickySaveSuppressed:        stickySaveSuppressed,
		StickySuppressionReason:     stickySuppressionReason,
		PolicyMode:                  policy.Mode,
		AutoMode:                    policy.AutoMode,
		Strategy:                    policy.Strategy,
		RequiresCodexImageTool:      req.RequiresCodexImageTool,
		RequiresResponsesPreviousID: req.RequiresResponsesPreviousID,
		RequiredTools:               requiredToolsForDispatchRequest(req),
		CandidateFilterConditions:   candidateFilterConditionsForDispatchRequest(req),
		Candidates:                  explanations,
		PoolLevel:                   bestCandidate.PoolLevel,
		RetryRoutingIntent:          retryIntent.Clone(),
		RetryIntentApplied:          retryIntent != nil && retryIntent.Active(),
		RetryQueuePriorityBoost:     retryIntent != nil && retryIntent.QueuePriorityBoost,
		CostGuardDecision:           costGuardDecision,
		ResourceProtectionEnabled:   resourceDecision.Enabled,
		ResourceProtectionPhase:     resourceDecision.selectedPhase(selectedSaturated),
		ResourceProtectionReason:    resourceDecision.selectedReason(selectedSaturated),
		ResourceProtectionRole:      resourceDecision.Role,
		PrimaryChannelIDs:           append([]int(nil), resourceDecision.PrimaryChannelIDs...),
		FallbackChannelIDs:          append([]int(nil), resourceDecision.FallbackChannelIDs...),
		PrimaryWaitTimeoutMs:        resourceDecision.PrimaryWaitTimeoutMs,
		PrimaryQueueMaxDepth:        resourceDecision.PrimaryQueueMaxDepth,
	}
	if negativeMarginFallbackSelected {
		plan.FallbackUsed = true
	}
	if s.shouldSaveStickyOnSelect() && !plan.StickySaveSuppressed {
		s.stickyRouter.Save(c, &req, plan)
	}
	if plan.StickySource != "" && plan.SelectedReason == "score_items" && plan.StickyBreak != "" {
		plan.SelectedReason = "score_items_sticky_broken"
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
	return s.scorePreparedCandidate(candidate, snapshot, policy, false, nil)
}

func (s *DefaultSmartChannelSelector) scorePreparedCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, stickyMatched bool, retryIntent *core.RetryRoutingIntent) CandidateScoreEvaluation {
	return scoringServiceForSelector(s).EvaluatePreparedCandidate(candidate, snapshot, policy, scoringContextForPolicy(s, policy, candidate, retryIntent), stickyMatched)
}

func scoringServiceForSelector(selector *DefaultSmartChannelSelector) *CandidateScoringService {
	if selector == nil {
		return NewCandidateScoringService()
	}
	if selector.scoringService != nil {
		return selector.scoringService
	}
	selector.scoringService = NewCandidateScoringService().WithCostBaselineProvider(selector.costBaselineProvider)
	return selector.scoringService
}

func scoringContextForPolicy(selector *DefaultSmartChannelSelector, policy core.GroupSmartPolicy, candidate core.Candidate, retryIntent *core.RetryRoutingIntent) ScoringContext {
	ctx := ScoringContext{
		RequestedModel:         strings.TrimSpace(candidate.RuntimeKey.RequestedModel),
		EndpointType:           candidate.RuntimeKey.EndpointType,
		CandidateGroups:        append([]string(nil), policy.CandidateGroups...),
		AutoMode:               policy.AutoMode,
		Strategy:               policy.Strategy,
		RequiresCodexImageTool: candidate.RequiresCodexImageTool,
		ScoreWeights:           scoreWeightsForSelector(selector),
		RetryRoutingIntent:     retryIntent.Clone(),
		ExplainEnabled:         true,
	}
	if ctx.RequestedModel == "" {
		ctx.RequestedModel = strings.TrimSpace(candidate.RuntimeKey.UpstreamModel)
	}
	return ctx
}

func queuePriorityForRetryIntent(base int, intent *core.RetryRoutingIntent) int {
	if intent == nil || !intent.Active() || !intent.QueuePriorityBoost {
		return base
	}
	priority := intent.QueuePriority
	if priority <= 0 {
		priority = core.RetryRoutingQueuePriority
	}
	if priority > base {
		return priority
	}
	return base
}

func scoreWeightsForSelector(selector *DefaultSmartChannelSelector) core.ScoreWeights {
	if selector == nil {
		return DefaultScoreWeights()
	}
	if selector.scoreWeights.Success != 0 ||
		selector.scoreWeights.Speed != 0 ||
		selector.scoreWeights.Load != 0 ||
		selector.scoreWeights.Cost != 0 ||
		selector.scoreWeights.Group != 0 {
		return selector.scoreWeights
	}
	return DefaultScoreWeights()
}

func requiredToolsForDispatchRequest(req core.DispatchRequest) []string {
	if req.RequiresCodexImageTool {
		return []string{core.DispatchRequiredToolCodexImageGeneration}
	}
	return nil
}

func candidateFilterConditionsForDispatchRequest(req core.DispatchRequest) []string {
	conditions := make([]string, 0, 2)
	if req.RequiresCodexImageTool {
		conditions = append(conditions, core.DispatchFilterConditionCodexImageGenerationTool)
	}
	if req.RequiresResponsesPreviousID {
		conditions = append(conditions, core.DispatchFilterConditionResponsesPreviousID)
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

func selectedQueueWaitMs(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy, selectedSaturated bool, resourceDecision resourceProtectionDecision) int {
	if !selectedSaturated {
		return 0
	}
	if resourceDecision.primaryWaitSelected(selectedSaturated) {
		if resourceDecision.PrimaryWaitTimeoutMs > 0 {
			return resourceDecision.PrimaryWaitTimeoutMs
		}
		if snapshot.QueueTimeoutMs > 0 {
			return snapshot.QueueTimeoutMs
		}
		return defaultQueueTimeoutMs
	}
	if !policy.QueueEnabled {
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

func selectedQueueCapacity(snapshot core.RuntimeSnapshot, resourceDecision resourceProtectionDecision, selectedSaturated bool) int {
	if resourceDecision.primaryWaitSelected(selectedSaturated) && resourceDecision.PrimaryQueueMaxDepth > 0 {
		return resourceDecision.PrimaryQueueMaxDepth
	}
	return snapshot.QueueCapacity
}

func (s *DefaultSmartChannelSelector) selectCandidateByRoutingScoreAndChannelPriority(evaluations []candidateEvaluation) (candidateEvaluation, bool) {
	if len(evaluations) == 0 {
		return candidateEvaluation{}, false
	}
	scoreOnlyBest := evaluations[0]
	maxRoutingScore := evaluations[0].score.RoutingTotal
	for _, evaluation := range evaluations[1:] {
		if evaluation.score.RoutingTotal > maxRoutingScore+channelPriorityTieBreakScoreEpsilon {
			scoreOnlyBest = evaluation
			maxRoutingScore = evaluation.score.RoutingTotal
		}
	}
	config := s.channelPriorityTieBreakConfig.normalized()
	if !config.Enabled {
		return scoreOnlyBest, false
	}
	selected := scoreOnlyBest
	for _, evaluation := range evaluations {
		if maxRoutingScore-evaluation.score.RoutingTotal > config.ScoreDelta+channelPriorityTieBreakScoreEpsilon {
			continue
		}
		selectedPriority := candidateChannelPriority(selected.candidate)
		evaluationPriority := candidateChannelPriority(evaluation.candidate)
		if evaluationPriority > selectedPriority {
			selected = evaluation
			continue
		}
		if evaluationPriority == selectedPriority &&
			evaluation.score.RoutingTotal > selected.score.RoutingTotal+channelPriorityTieBreakScoreEpsilon {
			selected = evaluation
		}
	}
	return selected, !candidateEvaluationMatches(selected, scoreOnlyBest)
}

func (s *DefaultSmartChannelSelector) selectNegativeMarginFallbackCandidate(evaluations []candidateEvaluation) (candidateEvaluation, bool) {
	if len(evaluations) == 0 {
		return candidateEvaluation{}, false
	}
	best := evaluations[0]
	bestLossMultiple := negativeMarginLossMultiple(best.snapshot)
	tieBreakUsed := false
	for _, evaluation := range evaluations[1:] {
		lossMultiple := negativeMarginLossMultiple(evaluation.snapshot)
		if bestLossMultiple <= 0 || (lossMultiple > 0 && lossMultiple < bestLossMultiple-channelPriorityTieBreakScoreEpsilon) {
			best = evaluation
			bestLossMultiple = lossMultiple
			tieBreakUsed = false
			continue
		}
		if lossMultiple <= 0 || lossMultiple > bestLossMultiple+channelPriorityTieBreakScoreEpsilon {
			continue
		}
		selected, used := s.selectCandidateByRoutingScoreAndChannelPriority([]candidateEvaluation{best, evaluation})
		if !candidateEvaluationMatches(selected, best) {
			best = selected
		}
		tieBreakUsed = tieBreakUsed || used
	}
	return best, tieBreakUsed
}

func candidateChannelPriority(candidate core.Candidate) int64 {
	if candidate.Channel == nil {
		return 0
	}
	return candidate.Channel.GetPriority()
}

func candidateEvaluationMatches(left candidateEvaluation, right candidateEvaluation) bool {
	leftKey := candidateExplanationRuntimeKey(left.candidate, left.snapshot)
	rightKey := candidateExplanationRuntimeKey(right.candidate, right.snapshot)
	if leftKey != rightKey {
		return false
	}
	return candidateEvaluationChannelID(left) == candidateEvaluationChannelID(right)
}

func candidateEvaluationChannelID(evaluation candidateEvaluation) int {
	if channelID := candidateChannelID(evaluation.candidate); channelID > 0 {
		return channelID
	}
	if evaluation.snapshot.Key.ChannelID != 0 {
		return evaluation.snapshot.Key.ChannelID
	}
	return evaluation.candidate.RuntimeKey.ChannelID
}

func channelPriorityTieBreakCanOwnReason(reason string) bool {
	return reason == "" || reason == "score_items"
}

func markChannelPriorityTieBreakCandidateExplanations(explanations []core.CandidateExplanation, selected candidateEvaluation, evaluations []candidateEvaluation, config ChannelPriorityTieBreakConfig) {
	config = config.normalized()
	if !config.Enabled || len(explanations) == 0 || len(evaluations) == 0 {
		return
	}
	selectedPriority := candidateChannelPriority(selected.candidate)
	if selectedPriority <= 0 {
		return
	}
	maxRoutingScore := evaluations[0].score.RoutingTotal
	for _, evaluation := range evaluations[1:] {
		if evaluation.score.RoutingTotal > maxRoutingScore+channelPriorityTieBreakScoreEpsilon {
			maxRoutingScore = evaluation.score.RoutingTotal
		}
	}
	for _, evaluation := range evaluations {
		if candidateEvaluationMatches(evaluation, selected) {
			continue
		}
		if maxRoutingScore-evaluation.score.RoutingTotal > config.ScoreDelta+channelPriorityTieBreakScoreEpsilon {
			continue
		}
		if candidateChannelPriority(evaluation.candidate) >= selectedPriority {
			continue
		}
		markCandidateExplanationSelectionSkipReason(explanations, evaluation, channelPriorityTieBreakReason)
	}
}

func markNegativeMarginSkippedCandidateExplanations(explanations []core.CandidateExplanation, evaluations []candidateEvaluation) {
	if len(explanations) == 0 || len(evaluations) == 0 {
		return
	}
	for _, evaluation := range evaluations {
		markCandidateExplanationSelectionSkipReasonWithOverwrite(explanations, evaluation, negativeCurrentGroupMarginReason, true)
	}
}

func markCandidateExplanationSelectionSkipReason(explanations []core.CandidateExplanation, evaluation candidateEvaluation, reason string) {
	markCandidateExplanationSelectionSkipReasonWithOverwrite(explanations, evaluation, reason, false)
}

func markCandidateExplanationSelectionSkipReasonWithOverwrite(explanations []core.CandidateExplanation, evaluation candidateEvaluation, reason string, overwrite bool) {
	channelID := candidateEvaluationChannelID(evaluation)
	selectedKey := candidateExplanationRuntimeKey(evaluation.candidate, evaluation.snapshot)
	for idx := range explanations {
		if explanations[idx].ChannelID != channelID {
			continue
		}
		if selectedKey.Group != "" && explanations[idx].RuntimeKey.Group != "" && explanations[idx].RuntimeKey.Group != selectedKey.Group {
			continue
		}
		if selectedKey.RequestedModel != "" && explanations[idx].RuntimeKey.RequestedModel != "" && explanations[idx].RuntimeKey.RequestedModel != selectedKey.RequestedModel {
			continue
		}
		if selectedKey.EndpointType != "" && explanations[idx].RuntimeKey.EndpointType != "" && explanations[idx].RuntimeKey.EndpointType != selectedKey.EndpointType {
			continue
		}
		if selectedKey.AccountID != "" && explanations[idx].RuntimeKey.AccountID != "" && explanations[idx].RuntimeKey.AccountID != selectedKey.AccountID {
			continue
		}
		if selectedKey.CredentialFP != "" && explanations[idx].RuntimeKey.CredentialFP != "" && explanations[idx].RuntimeKey.CredentialFP != selectedKey.CredentialFP {
			continue
		}
		if overwrite || explanations[idx].SelectionSkipReason == "" {
			explanations[idx].SelectionSkipReason = reason
		}
		return
	}
}

func candidateExplanation(candidate core.Candidate, snapshot core.RuntimeSnapshot, stickyMatched bool) core.CandidateExplanation {
	explanation := core.CandidateExplanation{
		Group:                             candidate.Group,
		UpstreamModel:                     candidate.UpstreamModel,
		ProviderProfile:                   candidate.ProviderProfile,
		ProxyMode:                         candidate.ProxyMode,
		ResourceID:                        candidate.ResourceRef.ResourceID,
		ResourceType:                      candidate.ResourceRef.ResourceType,
		AccountID:                         candidate.AccountIdentity.AccountID,
		AccountType:                       candidate.AccountIdentity.AccountType,
		Brand:                             candidate.AccountIdentity.Brand,
		Provider:                          candidate.AccountIdentity.Provider,
		CredentialIndex:                   candidate.CredentialRef.CredentialIndex,
		CredentialSubjectFP:               candidate.CredentialRef.CredentialSubjectFingerprint,
		CredentialFP:                      candidate.CredentialRef.CredentialFingerprint,
		ProxyID:                           candidate.ProxyRef.ProxyID,
		PoolLevel:                         candidate.PoolLevel,
		RuntimeKey:                        candidateExplanationRuntimeKey(candidate, snapshot),
		SuccessRate:                       snapshot.SuccessRate,
		TTFTMs:                            snapshot.TTFTMs,
		DurationMs:                        snapshot.DurationMs,
		TokensPerSecond:                   snapshot.TokensPerSecond,
		SampleCount:                       snapshot.SampleCount,
		ActiveConcurrency:                 snapshot.ActiveConcurrency,
		MaxConcurrency:                    snapshot.MaxConcurrency,
		ConfiguredConcurrencyLimit:        snapshot.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:           snapshot.LearnedConcurrencyLimit,
		EffectiveConcurrencyLimit:         snapshot.EffectiveConcurrencyLimit,
		QueueDepth:                        snapshot.QueueDepth,
		QueueCapacity:                     snapshot.QueueCapacity,
		EstimatedQueueWaitMs:              snapshot.EstimatedQueueWaitMs,
		FirstBytePending:                  snapshot.FirstBytePending,
		SlowFirstBytePending:              snapshot.SlowFirstBytePending,
		OldestFirstByteWaitMs:             snapshot.OldestFirstByteWaitMs,
		CostRatio:                         snapshot.CostRatio,
		CostReferenceRatio:                snapshot.CostReferenceRatio,
		CostPricingMode:                   snapshot.CostPricingMode,
		GroupPriorityRatio:                snapshot.GroupPriorityRatio,
		RevenueRatio:                      snapshot.RevenueRatio,
		NegativeCurrentGroupMargin:        negativeCurrentGroupMargin(snapshot),
		CircuitState:                      snapshot.CircuitState,
		CircuitOpen:                       snapshot.CircuitOpen,
		CircuitOpenUntil:                  snapshot.CircuitOpenUntil,
		CircuitOpenReason:                 snapshot.CircuitOpenReason,
		CircuitFailureCount:               snapshot.CircuitFailureCount,
		CircuitFailureRate:                snapshot.CircuitFailureRate,
		CircuitSampleCount:                snapshot.CircuitSampleCount,
		CircuitErrorCounts:                copyCircuitErrorCounts(snapshot.CircuitErrorCounts),
		CircuitHalfOpenProbeUsed:          snapshot.CircuitHalfOpenProbeUsed,
		CircuitHalfOpenProbeMax:           snapshot.CircuitHalfOpenProbeMax,
		EmptyOutputRate:                   snapshot.EmptyOutputRate,
		ExperienceIssueRate:               snapshot.ExperienceIssueRate,
		RecoverableQualityScore:           snapshot.RecoverableQualityScore,
		RecoverableQualityBaseline:        snapshot.RecoverableQualityBaseline,
		RecoverableQualityBaselineSamples: snapshot.RecoverableQualityBaselineSamples,
		RecoverableQualityDropRatio:       snapshot.RecoverableQualityDropRatio,
		ProbeRecoveryPending:              snapshot.ProbeRecoveryPending,
		ProbeRecoverySuccessCount:         snapshot.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:             snapshot.ProbeRecoveryRequired,
		ProbeTriggerReason:                snapshot.ProbeTriggerReason,
		ProbeRecoveryPhase:                snapshot.ProbeRecoveryPhase,
		ProbeFastRecoveryAttempts:         snapshot.ProbeFastRecoveryAttempts,
		ProbeAnomalyTriggerItems:          append([]string(nil), snapshot.ProbeAnomalyTriggerItems...),
		ConfigErrorIsolated:               snapshot.ConfigErrorIsolated,
		IsolationReason:                   snapshot.IsolationReason,
		IsolationUntil:                    snapshot.IsolationUntil,
		AuthConfigErrorCount:              snapshot.AuthConfigErrorCount,
		LastAuthConfigErrorAt:             snapshot.LastAuthConfigErrorAt,
		StickyMatched:                     stickyMatched,
		ScoreSampleSource:                 snapshot.SampleSource,
		MatchedRuntimeKey:                 snapshot.MatchedRuntimeKey,
	}
	if candidate.Channel != nil {
		explanation.ChannelID = candidate.Channel.Id
		explanation.ChannelName = candidate.Channel.Name
		explanation.ChannelStatus = candidate.Channel.Status
		explanation.ChannelPriority = candidate.Channel.GetPriority()
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
	return explanation
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
	if key.ResourceID == "" {
		key.ResourceID = candidate.RuntimeKey.ResourceID
	}
	if key.ResourceType == "" {
		key.ResourceType = candidate.RuntimeKey.ResourceType
	}
	if key.AccountID == "" {
		key.AccountID = candidate.RuntimeKey.AccountID
	}
	if key.AccountType == "" {
		key.AccountType = candidate.RuntimeKey.AccountType
	}
	if key.Brand == "" {
		key.Brand = candidate.RuntimeKey.Brand
	}
	if key.Provider == "" {
		key.Provider = candidate.RuntimeKey.Provider
	}
	if key.CredentialIndex == 0 {
		key.CredentialIndex = candidate.RuntimeKey.CredentialIndex
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = candidate.RuntimeKey.CredentialSubjectFP
	}
	if key.CredentialFP == "" {
		key.CredentialFP = candidate.RuntimeKey.CredentialFP
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
	selectedKey := candidateExplanationRuntimeKey(candidate, snapshot)
	for idx := range explanations {
		if explanations[idx].ChannelID != channelID {
			continue
		}
		if selectedKey.Group != "" && explanations[idx].RuntimeKey.Group != "" && explanations[idx].RuntimeKey.Group != selectedKey.Group {
			continue
		}
		if selectedKey.RequestedModel != "" && explanations[idx].RuntimeKey.RequestedModel != "" && explanations[idx].RuntimeKey.RequestedModel != selectedKey.RequestedModel {
			continue
		}
		if selectedKey.EndpointType != "" && explanations[idx].RuntimeKey.EndpointType != "" && explanations[idx].RuntimeKey.EndpointType != selectedKey.EndpointType {
			continue
		}
		if selectedKey.AccountID != "" && explanations[idx].RuntimeKey.AccountID != "" && explanations[idx].RuntimeKey.AccountID != selectedKey.AccountID {
			continue
		}
		if selectedKey.CredentialFP != "" && explanations[idx].RuntimeKey.CredentialFP != "" && explanations[idx].RuntimeKey.CredentialFP != selectedKey.CredentialFP {
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

func markCostGuardCandidateExplanations(explanations []core.CandidateExplanation, decision *core.CostGuardDecision) {
	if decision == nil {
		return
	}
	for idx := range explanations {
		switch explanations[idx].ChannelID {
		case decision.BaselineChannelID:
			explanations[idx].CostGuardDecision = decision.Decision
			explanations[idx].CostGuardReason = decision.Reason
		case decision.CandidateChannelID:
			explanations[idx].CostGuardDecision = decision.Decision
			explanations[idx].CostGuardReason = decision.Reason
		}
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
	snapshot.RevenueRatio = dynamic.RevenueRatio
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
	snapshot.RecoverableQualityScore = dynamic.RecoverableQualityScore
	snapshot.RecoverableQualityBaseline = dynamic.RecoverableQualityBaseline
	snapshot.RecoverableQualityBaselineSamples = dynamic.RecoverableQualityBaselineSamples
	snapshot.RecoverableQualityDropRatio = dynamic.RecoverableQualityDropRatio
	snapshot.ProbeRecoveryPending = dynamic.ProbeRecoveryPending
	snapshot.ProbeRecoverySuccessCount = dynamic.ProbeRecoverySuccessCount
	snapshot.ProbeRecoveryRequired = dynamic.ProbeRecoveryRequired
	snapshot.ProbeTriggerReason = dynamic.ProbeTriggerReason
	snapshot.ProbeRecoveryPhase = dynamic.ProbeRecoveryPhase
	snapshot.ProbeFastRecoveryAttempts = dynamic.ProbeFastRecoveryAttempts
	snapshot.ProbeAnomalyTriggerItems = append([]string(nil), dynamic.ProbeAnomalyTriggerItems...)
	snapshot.ConfigErrorIsolated = false
	snapshot.IsolationReason = ""
	snapshot.IsolationUntil = 0
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

func (s *DefaultSmartChannelSelector) costFirstStickyEscapeCandidate(policy core.GroupSmartPolicy, route core.StickyRoute, stickyCandidate core.Candidate, stickySnapshot core.RuntimeSnapshot, stickyScore core.ScoreResult, candidates []candidateEvaluation) (candidateEvaluation, *core.StickyDecision, bool) {
	if policy.Strategy != core.StrategyCostFirst {
		return candidateEvaluation{}, nil, false
	}
	config := DefaultCostFirstStickyEscapeConfig()
	if s != nil {
		config = s.stickyEscapeConfig.normalized()
	}
	costRatio := config.CostRatio
	speedDrop := config.MaxSpeedDrop
	if route.CacheAware {
		costRatio = config.CacheCostRatio
		speedDrop = config.CacheSpeedDrop
	}
	if !config.Enabled {
		return candidateEvaluation{}, stickyEscapeRetainDecision("cost_first_sticky_escape_disabled", route, config, stickySnapshot, 0, 0, costRatio, speedDrop), false
	}
	stickyCost := stickyEscapeCost(stickySnapshot)
	if stickyCost <= 0 {
		return candidateEvaluation{}, stickyEscapeRetainDecision("cost_first_sticky_escape_sticky_cost_missing", route, config, stickySnapshot, 0, 0, costRatio, speedDrop), false
	}
	if stickySnapshot.SampleCount < config.MinSamples {
		return candidateEvaluation{}, stickyEscapeRetainDecision("cost_first_sticky_escape_sticky_samples_insufficient", route, config, stickySnapshot, stickyCost, 0, costRatio, speedDrop), false
	}
	stickySpeed, ok := stickySpeedScore(stickyScore)
	if !ok {
		return candidateEvaluation{}, stickyEscapeRetainDecision("cost_first_sticky_escape_sticky_speed_missing", route, config, stickySnapshot, stickyCost, 0, costRatio, speedDrop), false
	}
	var best candidateEvaluation
	bestCost := 0.0
	var bestDecision *core.StickyDecision
	var retainDecision *core.StickyDecision
	retainPriority := -1
	retainCost := 0.0
	found := false
	for _, candidate := range candidates {
		if sameRoutingCandidate(stickyCandidate, candidate.candidate) {
			continue
		}
		candidateCost := stickyEscapeCost(candidate.snapshot)
		if candidateCost <= 0 {
			continue
		}
		if candidateCost > stickyCost*costRatio {
			retainDecision, retainPriority, retainCost = stickyEscapePreferredRetainDecision(
				retainDecision,
				retainPriority,
				retainCost,
				10,
				"cost_first_sticky_escape_cost_gap_insufficient",
				route,
				config,
				stickySnapshot,
				stickyCost,
				stickySpeed,
				candidate,
				candidateCost,
				0,
				costRatio,
				speedDrop,
			)
			continue
		}
		if candidate.snapshot.SampleCount < config.MinSamples {
			retainDecision, retainPriority, retainCost = stickyEscapePreferredRetainDecision(
				retainDecision,
				retainPriority,
				retainCost,
				20,
				"cost_first_sticky_escape_candidate_samples_insufficient",
				route,
				config,
				stickySnapshot,
				stickyCost,
				stickySpeed,
				candidate,
				candidateCost,
				0,
				costRatio,
				speedDrop,
			)
			continue
		}
		if candidate.snapshot.SuccessRate+config.SuccessSlack < stickySnapshot.SuccessRate {
			retainDecision, retainPriority, retainCost = stickyEscapePreferredRetainDecision(
				retainDecision,
				retainPriority,
				retainCost,
				30,
				"cost_first_sticky_escape_success_guard_failed",
				route,
				config,
				stickySnapshot,
				stickyCost,
				stickySpeed,
				candidate,
				candidateCost,
				0,
				costRatio,
				speedDrop,
			)
			continue
		}
		candidateSpeed, ok := stickySpeedScore(candidate.score)
		if !ok {
			retainDecision, retainPriority, retainCost = stickyEscapePreferredRetainDecision(
				retainDecision,
				retainPriority,
				retainCost,
				40,
				"cost_first_sticky_escape_candidate_speed_missing",
				route,
				config,
				stickySnapshot,
				stickyCost,
				stickySpeed,
				candidate,
				candidateCost,
				0,
				costRatio,
				speedDrop,
			)
			continue
		}
		if candidateSpeed+speedDrop < stickySpeed {
			retainDecision, retainPriority, retainCost = stickyEscapePreferredRetainDecision(
				retainDecision,
				retainPriority,
				retainCost,
				50,
				"cost_first_sticky_escape_speed_drop_exceeded",
				route,
				config,
				stickySnapshot,
				stickyCost,
				stickySpeed,
				candidate,
				candidateCost,
				candidateSpeed,
				costRatio,
				speedDrop,
			)
			continue
		}
		if !found || candidateCost < bestCost {
			best = candidate
			bestCost = candidateCost
			bestDecision = stickyEscapeDecision(
				"cost_first_cheaper_speed_acceptable",
				"switch",
				route,
				config,
				stickySnapshot,
				stickyCost,
				stickySpeed,
				candidate,
				candidateCost,
				candidateSpeed,
				costRatio,
				speedDrop,
			)
			found = true
		}
	}
	if !found && retainDecision == nil {
		retainDecision = stickyEscapeRetainDecision("cost_first_sticky_escape_no_candidate", route, config, stickySnapshot, stickyCost, stickySpeed, costRatio, speedDrop)
	}
	if found {
		return best, bestDecision, true
	}
	return candidateEvaluation{}, retainDecision, false
}

func (s *DefaultSmartChannelSelector) costFirstGuardCandidate(policy core.GroupSmartPolicy, selectedCandidate core.Candidate, selectedSnapshot core.RuntimeSnapshot, selectedScore core.ScoreResult, candidates []candidateEvaluation, retryIntent *core.RetryRoutingIntent) (costFirstGuardResult, bool) {
	if policy.Strategy != core.StrategyCostFirst || len(candidates) == 0 {
		return costFirstGuardResult{}, false
	}
	config := DefaultCostFirstGuardConfig()
	if s != nil {
		config = s.costGuardConfig.normalized()
	}
	if !config.Enabled {
		return costFirstGuardResult{}, false
	}
	selected := candidateEvaluation{candidate: selectedCandidate, snapshot: selectedSnapshot, score: selectedScore}
	selectedCost := costFirstEvaluationCost(selected)
	if selectedCost <= 0 {
		return costFirstGuardResult{}, false
	}
	baseline, ok := costFirstGuardBaselineCandidate(candidates)
	if !ok || sameRoutingCandidate(selectedCandidate, baseline.candidate) {
		return costFirstGuardResult{}, false
	}
	baselineCost := costFirstEvaluationCost(baseline)
	if baselineCost <= 0 {
		return costFirstGuardResult{}, false
	}
	multiple := selectedCost / baselineCost
	if multiple <= config.Multiple {
		return costFirstGuardResult{}, false
	}
	decision := costFirstGuardDecision("cost_first_guard_baseline_selected", "baseline", config, baseline, selected, multiple, retryIntent)
	if costFirstGuardAllowsExpensiveCandidate(config, baseline, selected, retryIntent) {
		decision.Reason = "cost_first_guard_quality_override"
		decision.Decision = "override"
		return costFirstGuardResult{baseline: baseline, decision: decision}, true
	}
	return costFirstGuardResult{baseline: baseline, decision: decision, switchToBaseline: true}, true
}

func costFirstGuardBaselineCandidate(candidates []candidateEvaluation) (candidateEvaluation, bool) {
	var best candidateEvaluation
	bestCost := 0.0
	found := false
	for _, candidate := range candidates {
		cost := costFirstEvaluationCost(candidate)
		if cost <= 0 {
			continue
		}
		if !found || cost < bestCost || (cost == bestCost && candidate.score.RoutingTotal > best.score.RoutingTotal) {
			best = candidate
			bestCost = cost
			found = true
		}
	}
	return best, found
}

func costFirstGuardAllowsExpensiveCandidate(config CostFirstGuardConfig, baseline candidateEvaluation, candidate candidateEvaluation, retryIntent *core.RetryRoutingIntent) bool {
	if costFirstGuardBaselineRisk(baseline.snapshot) {
		return true
	}
	successDelta := candidate.snapshot.SuccessRate - baseline.snapshot.SuccessRate
	if successDelta >= config.SuccessAdvantage {
		return true
	}
	speedDelta := costFirstGuardSpeedScore(candidate) - costFirstGuardSpeedScore(baseline)
	if speedDelta >= config.SpeedAdvantage {
		return true
	}
	if retryIntent != nil && retryIntent.FirstByteRecovery() && retryIntent.FailedChannelID == baselineChannelID(baseline) && (successDelta > 0 || speedDelta > 0) {
		return true
	}
	return false
}

func costFirstGuardDecision(reason string, decision string, config CostFirstGuardConfig, baseline candidateEvaluation, candidate candidateEvaluation, multiple float64, retryIntent *core.RetryRoutingIntent) *core.CostGuardDecision {
	baselineChannelID, baselineChannelName := costFirstGuardChannelMeta(baseline)
	candidateChannelID, candidateChannelName := costFirstGuardChannelMeta(candidate)
	return &core.CostGuardDecision{
		Reason:               reason,
		Decision:             decision,
		BaselineChannelID:    baselineChannelID,
		BaselineChannelName:  baselineChannelName,
		CandidateChannelID:   candidateChannelID,
		CandidateChannelName: candidateChannelName,
		BaselineCost:         costFirstEvaluationCost(baseline),
		CandidateCost:        costFirstEvaluationCost(candidate),
		CostMultiple:         multiple,
		CostGuardMultiple:    config.Multiple,
		SuccessDelta:         candidate.snapshot.SuccessRate - baseline.snapshot.SuccessRate,
		SuccessAdvantage:     config.SuccessAdvantage,
		SpeedScoreDelta:      costFirstGuardSpeedScore(candidate) - costFirstGuardSpeedScore(baseline),
		SpeedAdvantage:       config.SpeedAdvantage,
		BaselineRisk:         costFirstGuardBaselineRisk(baseline.snapshot),
		RetryIntent:          retryIntent != nil && retryIntent.Active(),
		BaselineSampleCount:  baseline.snapshot.SampleCount,
		CandidateSampleCount: candidate.snapshot.SampleCount,
	}
}

func costFirstGuardChannelMeta(candidate candidateEvaluation) (int, string) {
	if candidate.candidate.Channel != nil {
		return candidate.candidate.Channel.Id, candidate.candidate.Channel.Name
	}
	return candidate.snapshot.Key.ChannelID, ""
}

func costFirstEvaluationCost(candidate candidateEvaluation) float64 {
	return stickyEscapeCost(candidate.snapshot)
}

func baselineChannelID(candidate candidateEvaluation) int {
	channelID, _ := costFirstGuardChannelMeta(candidate)
	return channelID
}

func costFirstGuardBaselineRisk(snapshot core.RuntimeSnapshot) bool {
	return snapshot.CircuitOpen ||
		snapshot.Cooldown ||
		snapshot.FailureAvoidance ||
		snapshot.ConfigErrorIsolated ||
		snapshot.FirstBytePending > 0 ||
		snapshot.SlowFirstBytePending > 0 ||
		snapshot.QueueDepth > 0
}

func costFirstGuardSpeedScore(candidate candidateEvaluation) float64 {
	if score, ok := stickySpeedScore(candidate.score); ok {
		return score
	}
	score := 0.0
	parts := 0.0
	if candidate.snapshot.TTFTMs > 0 {
		score += progressiveTTFTLatencyScore(candidate.snapshot.TTFTMs, 800, 20000)
		parts++
	}
	if candidate.snapshot.DurationMs > 0 {
		score += inverseLatencyScore(candidate.snapshot.DurationMs, 3000, 90000)
		parts++
	}
	if candidate.snapshot.TokensPerSecond > 0 {
		score += throughputScore(candidate.snapshot.TokensPerSecond, 5, 80)
		parts++
	}
	if parts <= 0 {
		return 0
	}
	return score / parts
}

func stickyEscapePreferredRetainDecision(current *core.StickyDecision, currentPriority int, currentCost float64, priority int, reason string, route core.StickyRoute, config CostFirstStickyEscapeConfig, stickySnapshot core.RuntimeSnapshot, stickyCost float64, stickySpeed float64, candidate candidateEvaluation, candidateCost float64, candidateSpeed float64, costThreshold float64, maxSpeedDrop float64) (*core.StickyDecision, int, float64) {
	if current != nil && (priority < currentPriority || (priority == currentPriority && (currentCost <= 0 || candidateCost >= currentCost))) {
		return current, currentPriority, currentCost
	}
	return stickyEscapeDecision(
		reason,
		"retain",
		route,
		config,
		stickySnapshot,
		stickyCost,
		stickySpeed,
		candidate,
		candidateCost,
		candidateSpeed,
		costThreshold,
		maxSpeedDrop,
	), priority, candidateCost
}

func stickyEscapeRetainDecision(reason string, route core.StickyRoute, config CostFirstStickyEscapeConfig, stickySnapshot core.RuntimeSnapshot, stickyCost float64, stickySpeed float64, costThreshold float64, maxSpeedDrop float64) *core.StickyDecision {
	return &core.StickyDecision{
		Reason:            reason,
		StickyCost:        stickyCost,
		CostThreshold:     costThreshold,
		StickySpeedScore:  stickySpeed,
		MaxSpeedScoreDrop: maxSpeedDrop,
		StickySuccessRate: stickySnapshot.SuccessRate,
		SuccessSlack:      config.SuccessSlack,
		StickySampleCount: stickySnapshot.SampleCount,
		MinSamples:        config.MinSamples,
		CacheAware:        route.CacheAware,
		Decision:          "retain",
	}
}

func stickyEscapeDecision(reason, decision string, route core.StickyRoute, config CostFirstStickyEscapeConfig, stickySnapshot core.RuntimeSnapshot, stickyCost float64, stickySpeed float64, candidate candidateEvaluation, candidateCost float64, candidateSpeed float64, costThreshold float64, maxSpeedDrop float64) *core.StickyDecision {
	channelID := 0
	channelName := ""
	if candidate.candidate.Channel != nil {
		channelID = candidate.candidate.Channel.Id
		channelName = candidate.candidate.Channel.Name
	}
	if channelID == 0 {
		channelID = candidate.snapshot.Key.ChannelID
	}
	costRatio := 0.0
	if stickyCost > 0 {
		costRatio = candidateCost / stickyCost
	}
	return &core.StickyDecision{
		Reason:               reason,
		CandidateChannelID:   channelID,
		CandidateChannelName: channelName,
		StickyCost:           stickyCost,
		CandidateCost:        candidateCost,
		CostRatio:            costRatio,
		CostThreshold:        costThreshold,
		StickySpeedScore:     stickySpeed,
		CandidateSpeedScore:  candidateSpeed,
		SpeedScoreDelta:      candidateSpeed - stickySpeed,
		MaxSpeedScoreDrop:    maxSpeedDrop,
		StickySuccessRate:    stickySnapshot.SuccessRate,
		CandidateSuccessRate: candidate.snapshot.SuccessRate,
		SuccessSlack:         config.SuccessSlack,
		StickySampleCount:    stickySnapshot.SampleCount,
		CandidateSampleCount: candidate.snapshot.SampleCount,
		MinSamples:           config.MinSamples,
		CacheAware:           route.CacheAware,
		Decision:             decision,
	}
}

func stickySpeedScore(score core.ScoreResult) (float64, bool) {
	total := 0.0
	count := 0.0
	for _, item := range score.Items {
		switch item.Key {
		case scoreItemTTFTLatency, scoreItemDurationLatency:
			if item.MissingReason != "" || item.SampleCount <= 0 {
				continue
			}
			total += item.Score
			count++
		case scoreItemThroughput:
			if item.MissingReason != "" || item.SampleCount <= 0 {
				continue
			}
			total += item.Score
			count++
		}
	}
	if count <= 0 {
		return 0, false
	}
	return total / count, true
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

func negativeCurrentGroupMargin(snapshot core.RuntimeSnapshot) bool {
	if snapshot.CostRatio <= 0 || snapshot.RevenueRatio <= 0 {
		return false
	}
	if strings.TrimSpace(snapshot.CostPricingMode) == "request" {
		return false
	}
	return snapshot.CostRatio > snapshot.RevenueRatio
}

func negativeMarginLossMultiple(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.CostRatio <= 0 {
		return 0
	}
	if snapshot.RevenueRatio <= 0 {
		return snapshot.CostRatio
	}
	return snapshot.CostRatio / snapshot.RevenueRatio
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

func (s *DefaultSmartChannelSelector) candidateUnavailableReason(c *gin.Context, req core.DispatchRequest, candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) string {
	if reason := candidateUnavailableReason(c, candidate, snapshot, policy); reason != "" {
		if reason == "routing_slot_reserved" && s != nil && s.emptyOutputSwitch != nil {
			if switchReason := s.emptyOutputSwitch.AvoidanceReason(req, candidate, snapshot); switchReason != "" {
				return switchReason
			}
		}
		return reason
	}
	if s == nil || s.emptyOutputSwitch == nil {
		return ""
	}
	return s.emptyOutputSwitch.AvoidanceReason(req, candidate, snapshot)
}

func markClientEmptyOutputSwitchChannelSkipped(c *gin.Context, candidate core.Candidate, snapshot core.RuntimeSnapshot) {
	if c == nil {
		return
	}
	channelID := candidateChannelID(candidate)
	if channelID <= 0 {
		channelID = firstPositiveInt(snapshot.Key.ChannelID, candidate.RuntimeKey.ChannelID)
	}
	service.MarkChannelSelectionSkipped(c, channelID)
}

func candidateUnavailableReason(c *gin.Context, candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) string {
	if candidate.RequiresCodexImageTool && !candidateSupportsCodexImageGenerationTool(candidate) {
		return "codex_image_generation_tool_unsupported"
	}
	identity := serviceRuntimeIdentityFromCandidate(candidate, snapshot)
	if snapshot.ConfigErrorIsolated {
		return "config_error_isolated"
	}
	if service.IsChannelRuntimeBalanceSkipped(c, identity) {
		return service.ChannelStatusReasonBalanceInsufficient
	}
	if candidate.Channel != nil && service.IsKnownBalanceInsufficientChannel(candidate.Channel) {
		return service.ChannelStatusReasonBalanceInsufficient
	}
	if service.IsRuntimeBalanceInsufficientIdentity(identity) {
		return service.ChannelStatusReasonBalanceInsufficient
	}
	if candidate.Channel != nil && candidate.Channel.Status != 0 && candidate.Channel.Status != common.ChannelStatusEnabled {
		return "channel_disabled"
	}
	if candidateAccountRateLimitRejected(candidate) {
		return AccountRateLimitRejectReason
	}
	if service.IsChannelRuntimeSelectionSkipped(c, identity) {
		if service.IsChannelRuntimeAttempted(c, identity) {
			return "already_failed_in_request"
		}
		return "routing_slot_reserved"
	}
	if snapshot.CircuitOpen {
		return "circuit_open"
	}
	if snapshot.Cooldown {
		return "cooldown"
	}
	if snapshot.FailureAvoidance {
		if failureAvoidanceCanUseBusinessProbe(snapshot) {
			if routingConcurrencySaturated(snapshot) {
				return failureRecoveryProbeInFlightReason
			}
			return ""
		}
		if strings.TrimSpace(snapshot.ProbeTriggerReason) == service.ChannelTimeoutRecoveryReason {
			return service.ChannelTimeoutRecoveryReason
		}
		if strings.TrimSpace(snapshot.ProbeTriggerReason) == service.ChannelOverloadRecoveryReason {
			return service.ChannelOverloadRecoveryReason
		}
		return "failure_avoidance"
	}
	return ""
}

func candidateSupportsCodexImageGenerationTool(candidate core.Candidate) bool {
	if !service.ChannelSupportsCodexImageGenerationTool(candidate.Channel) {
		return false
	}
	if candidate.Channel == nil || len(candidate.Channel.ChannelInfo.MultiKeyCapabilities) == 0 {
		return true
	}
	index := candidate.CredentialRef.CredentialIndex
	capability, ok := candidate.Channel.ChannelInfo.MultiKeyCapabilities[index]
	if !ok {
		return true
	}
	if service.ChannelAccountCapabilityUsageLimited(capability) ||
		strings.TrimSpace(capability.CapabilityClassification) == channelcapability.ClassificationAuthError {
		return false
	}
	return capability.CodexImageGenerationTool == nil || *capability.CodexImageGenerationTool
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
