package scheduler

import (
	"errors"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const maxCandidateExplanations = 32

type DefaultSmartChannelSelector struct {
	candidateBuilder core.CandidatePoolBuilder
	snapshotStore    core.RuntimeSnapshotStore
	scorerFactory    core.ScoreCalculatorFactory
	snapshotEnricher core.RuntimeSnapshotEnricher
	stickyRouter     core.StickyRouter
}

type stickySaveOnSelectRouter interface {
	SaveOnSelect() bool
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
	scorer := s.scorerFactory.ForStrategy(policy.Strategy)
	stickyRoute, hasSticky := s.stickyRoute(c, &req, policy)
	var bestCandidate core.Candidate
	var bestSnapshot core.RuntimeSnapshot
	var bestScore core.ScoreResult
	var stickyCandidate core.Candidate
	var stickySnapshot core.RuntimeSnapshot
	var stickyScore core.ScoreResult
	var stickyBreak string
	stickyFound := false
	found := false
	explanations := make([]core.CandidateExplanation, 0, minInt(len(candidates), maxCandidateExplanations))
	for _, candidate := range candidates {
		snapshot := s.snapshotForCandidate(candidate)
		if s.snapshotEnricher != nil {
			snapshot = s.snapshotEnricher.Enrich(candidate, snapshot, policy)
		}
		stickyMatched := hasSticky && isStickyCandidate(candidate, stickyRoute)
		explanation := candidateExplanation(candidate, snapshot, stickyMatched)
		rejectReason := candidateUnavailableReason(c, candidate, snapshot, policy)
		if stickyMatched && stickyBreak == "" && rejectReason != "" {
			stickyBreak = rejectReason
		}
		if rejectReason != "" {
			explanation.RejectReason = rejectReason
			appendCandidateExplanation(&explanations, explanation)
			continue
		}
		score := scorer.Score(candidate, snapshot, policy)
		explanation.Available = true
		explanation.ScoreTotal = score.Total
		explanation.ScoreBreakdown = score.Breakdown
		appendCandidateExplanation(&explanations, explanation)
		if hasSticky && stickyMatched {
			stickyCandidate = candidate
			stickySnapshot = snapshot
			stickyScore = score
			stickyFound = true
		}
		if !found || score.Total > bestScore.Total {
			bestCandidate = candidate
			bestSnapshot = snapshot
			bestScore = score
			found = true
		}
	}
	if !found || bestCandidate.Channel == nil {
		return nil, false, nil
	}
	if hasSticky {
		if stickyFound {
			keepRatio := stickyRoute.KeepScoreRatio
			if keepRatio <= 0 {
				keepRatio = defaultStickyKeepScoreRatio
			}
			if stickyScore.Total >= bestScore.Total*keepRatio {
				bestCandidate = stickyCandidate
				bestSnapshot = stickySnapshot
				bestScore = stickyScore
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
		Channel:         bestCandidate.Channel,
		SelectedGroup:   bestCandidate.Group,
		RequestedGroup:  req.RequestedGroup,
		RuntimeKey:      bestSnapshot.Key,
		ProviderProfile: bestCandidate.ProviderProfile,
		ProxyMode:       bestCandidate.ProxyMode,
		ScoreTotal:      bestScore.Total,
		ScoreBreakdown:  bestScore.Breakdown,
		QueueWaitMs:     selectedQueueWaitMs(bestSnapshot, policy),
		QueueEnabled:    policy.QueueEnabled,
		QueueDepth:      bestSnapshot.QueueDepth,
		QueueCapacity:   bestSnapshot.QueueCapacity,
		QueuePriority:   policy.QueuePriority,
		SelectedReason:  bestScore.Reason,
		StickySource:    stickyRoute.Source,
		StickyKeyFP:     stickyRoute.KeyFingerprint,
		StickyRetained:  hasSticky && stickyBreak == "",
		StickyBreak:     stickyBreak,
		CacheAffinity:   hasSticky && stickyRoute.CacheAware,
		PolicyMode:      policy.Mode,
		AutoMode:        policy.AutoMode,
		Candidates:      explanations,
	}
	if s.shouldSaveStickyOnSelect() {
		s.stickyRouter.Save(c, &req, plan)
	}
	if plan.StickySource != "" && plan.SelectedReason == "weighted_score" && plan.StickyBreak != "" {
		plan.SelectedReason = "weighted_score_sticky_broken"
	}
	return plan, true, nil
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
		CostRatio:                  snapshot.CostRatio,
		GroupPriorityRatio:         snapshot.GroupPriorityRatio,
		SuccessScore:               snapshot.SuccessScore,
		SpeedScore:                 snapshot.SpeedScore,
		ExperienceScore:            snapshot.ExperienceScore,
		EmptyOutputRate:            snapshot.EmptyOutputRate,
		ExperienceIssueRate:        snapshot.ExperienceIssueRate,
		StickyMatched:              stickyMatched,
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
	explanation.LoadScore = loadScore(snapshot)
	explanation.CostScore = costScore(snapshot)
	explanation.GroupScore = groupScore(snapshot)
	if explanation.SuccessScore <= 0 {
		explanation.SuccessScore = successScore(snapshot)
	}
	if explanation.SpeedScore <= 0 {
		explanation.SpeedScore = speedScore(snapshot)
	}
	if explanation.ExperienceScore <= 0 {
		explanation.ExperienceScore = experienceScore(snapshot)
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
	return key
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

func (s *DefaultSmartChannelSelector) snapshotForCandidate(candidate core.Candidate) core.RuntimeSnapshot {
	if s.snapshotStore == nil {
		return defaultSnapshot(candidate)
	}
	if snapshot, ok := s.snapshotStore.Get(candidate.RuntimeKey); ok {
		return snapshot
	}
	return defaultSnapshot(candidate)
}

func defaultSnapshot(candidate core.Candidate) core.RuntimeSnapshot {
	return core.RuntimeSnapshot{
		Key:                candidate.RuntimeKey,
		SuccessRate:        0.80,
		CostRatio:          0,
		GroupPriorityRatio: 1,
	}
}

func queueCanWait(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) bool {
	if !policy.QueueEnabled {
		return false
	}
	if snapshot.QueueCapacity <= 0 {
		return false
	}
	if snapshot.QueueDepth >= snapshot.QueueCapacity {
		return false
	}
	if snapshot.QueueTimeoutMs <= 0 {
		return false
	}
	if snapshot.EstimatedQueueWaitMs <= 0 {
		return true
	}
	return snapshot.EstimatedQueueWaitMs <= float64(snapshot.QueueTimeoutMs)
}

func selectedQueueWaitMs(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) int {
	if snapshot.MaxConcurrency <= 0 || snapshot.ActiveConcurrency < snapshot.MaxConcurrency || !queueCanWait(snapshot, policy) {
		return 0
	}
	if snapshot.EstimatedQueueWaitMs > 0 {
		return int(snapshot.EstimatedQueueWaitMs)
	}
	return snapshot.QueueTimeoutMs
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

func candidateUnavailableReason(c *gin.Context, candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) string {
	if candidate.Channel != nil && service.IsChannelConcurrencySkipped(c, candidate.Channel.Id) {
		return "local_concurrency_full"
	}
	if candidate.Channel != nil && service.IsKnownBalanceInsufficientChannel(candidate.Channel) {
		return service.ChannelStatusReasonBalanceInsufficient
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
	limit := snapshot.EffectiveConcurrencyLimit
	if limit <= 0 {
		limit = snapshot.MaxConcurrency
	}
	if limit > 0 && snapshot.ActiveConcurrency >= limit && !queueCanWait(snapshot, policy) {
		if snapshot.LearnedConcurrencyLimit > 0 && snapshot.ConfiguredConcurrencyLimit > snapshot.LearnedConcurrencyLimit {
			return "learned_concurrency_full"
		}
		return "concurrency_full"
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
