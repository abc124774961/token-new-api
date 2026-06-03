package scheduler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type resourceProtectionDecision struct {
	Enabled              bool
	Phase                string
	Reason               string
	Role                 string
	PrimaryChannelIDs    []int
	FallbackChannelIDs   []int
	PrimaryWaitTimeoutMs int
	PrimaryQueueMaxDepth int
	primarySet           map[int]struct{}
	fallbackSet          map[int]struct{}
}

func (s *DefaultSmartChannelSelector) resourceProtectionCandidates(c *gin.Context, req core.DispatchRequest, policy core.GroupSmartPolicy, candidates []core.Candidate) ([]core.Candidate, resourceProtectionDecision, *types.NewAPIError) {
	decision := newResourceProtectionDecision(policy)
	if !decision.Enabled || len(candidates) == 0 {
		return candidates, decision, nil
	}
	primaryCandidates, fallbackCandidates := splitResourceProtectionCandidates(candidates, decision)
	if req.ResourceProtectionFallback {
		decision.Phase = core.ResourceProtectionPhaseFallbackAfterTimeout
		decision.Reason = firstNonEmptyString(req.ResourceProtectionReason, core.ResourceProtectionReasonFallbackAfterTimeout)
		decision.Role = core.ResourceProtectionRoleFallback
		if !hasSelectableResourceProtectionCandidate(s, c, policy, fallbackCandidates) {
			return nil, decision, resourceProtectionNoCandidateError(policy, decision)
		}
		return fallbackCandidates, decision, nil
	}
	if len(primaryCandidates) == 0 {
		decision.Phase = core.ResourceProtectionPhaseNoPrimaryFallback
		decision.Reason = core.ResourceProtectionReasonNoPrimaryCandidate
		decision.Role = core.ResourceProtectionRoleFallback
		if !hasSelectableResourceProtectionCandidate(s, c, policy, fallbackCandidates) {
			return nil, decision, resourceProtectionNoCandidateError(policy, decision)
		}
		return fallbackCandidates, decision, nil
	}
	availability := resourceProtectionAvailability(s, c, policy, primaryCandidates)
	if availability.available || availability.saturated {
		decision.Phase = core.ResourceProtectionPhasePrimaryHit
		decision.Reason = core.ResourceProtectionReasonPrimaryAvailable
		decision.Role = core.ResourceProtectionRolePrimary
		return primaryCandidates, decision, nil
	}
	decision.Phase = core.ResourceProtectionPhasePrimaryFailureFallback
	decision.Reason = core.ResourceProtectionReasonPrimaryFailure
	decision.Role = core.ResourceProtectionRoleFallback
	if !hasSelectableResourceProtectionCandidate(s, c, policy, fallbackCandidates) {
		return nil, decision, resourceProtectionNoCandidateError(policy, decision)
	}
	return fallbackCandidates, decision, nil
}

func newResourceProtectionDecision(policy core.GroupSmartPolicy) resourceProtectionDecision {
	decision := resourceProtectionDecision{
		Enabled:              policy.ResourceProtectionEnabled && len(policy.PrimaryChannelIDs) > 0,
		PrimaryChannelIDs:    append([]int(nil), policy.PrimaryChannelIDs...),
		FallbackChannelIDs:   append([]int(nil), policy.FallbackChannelIDs...),
		PrimaryWaitTimeoutMs: policy.PrimaryWaitTimeoutMs,
		PrimaryQueueMaxDepth: policy.PrimaryQueueMaxDepth,
	}
	decision.primarySet = intSet(decision.PrimaryChannelIDs)
	decision.fallbackSet = intSet(decision.FallbackChannelIDs)
	return decision
}

func splitResourceProtectionCandidates(candidates []core.Candidate, decision resourceProtectionDecision) ([]core.Candidate, []core.Candidate) {
	primaryCandidates := make([]core.Candidate, 0, len(candidates))
	fallbackCandidates := make([]core.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		channelID := candidateChannelID(candidate)
		if channelID <= 0 {
			continue
		}
		if _, ok := decision.primarySet[channelID]; ok {
			primaryCandidates = append(primaryCandidates, candidate)
			if len(decision.fallbackSet) == 0 {
				continue
			}
		}
		if len(decision.fallbackSet) > 0 {
			if _, ok := decision.fallbackSet[channelID]; ok {
				fallbackCandidates = append(fallbackCandidates, candidate)
			}
			continue
		}
		if _, ok := decision.primarySet[channelID]; !ok {
			fallbackCandidates = append(fallbackCandidates, candidate)
		}
	}
	return primaryCandidates, fallbackCandidates
}

type resourceProtectionAvailabilityState struct {
	available bool
	saturated bool
}

func resourceProtectionAvailability(s *DefaultSmartChannelSelector, c *gin.Context, policy core.GroupSmartPolicy, candidates []core.Candidate) resourceProtectionAvailabilityState {
	state := resourceProtectionAvailabilityState{}
	for _, candidate := range candidates {
		snapshot := resourceProtectionSnapshotForCandidate(s, candidate, policy)
		if candidateUnavailableReason(c, candidate, snapshot, policy) != "" {
			continue
		}
		if routingConcurrencySaturated(snapshot) {
			state.saturated = true
			continue
		}
		state.available = true
	}
	return state
}

func hasSelectableResourceProtectionCandidate(s *DefaultSmartChannelSelector, c *gin.Context, policy core.GroupSmartPolicy, candidates []core.Candidate) bool {
	availability := resourceProtectionAvailability(s, c, policy, candidates)
	return availability.available || availability.saturated
}

func resourceProtectionSnapshotForCandidate(s *DefaultSmartChannelSelector, candidate core.Candidate, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	if s == nil {
		return defaultSnapshot(candidate)
	}
	snapshot := s.snapshotForCandidate(candidate, policy)
	if s.snapshotEnricher != nil {
		snapshot = s.snapshotEnricher.Enrich(candidate, snapshot, policy)
	}
	return snapshot
}

func (d resourceProtectionDecision) selectedPhase(selectedSaturated bool) string {
	if !d.Enabled {
		return ""
	}
	if d.Role == core.ResourceProtectionRolePrimary && selectedSaturated {
		return core.ResourceProtectionPhasePrimarySaturatedWait
	}
	return d.Phase
}

func (d resourceProtectionDecision) selectedReason(selectedSaturated bool) string {
	if !d.Enabled {
		return ""
	}
	if d.Role == core.ResourceProtectionRolePrimary && selectedSaturated {
		return core.ResourceProtectionReasonPrimarySaturated
	}
	return d.Reason
}

func (d resourceProtectionDecision) queueEnabled(selectedSaturated bool, policy core.GroupSmartPolicy) bool {
	if d.primaryWaitSelected(selectedSaturated) {
		return true
	}
	return policy.QueueEnabled
}

func (d resourceProtectionDecision) primaryWaitSelected(selectedSaturated bool) bool {
	return d.Enabled && d.Role == core.ResourceProtectionRolePrimary && selectedSaturated
}

func (d resourceProtectionDecision) roleFor(candidate core.Candidate) string {
	if !d.Enabled {
		return ""
	}
	channelID := candidateChannelID(candidate)
	if _, ok := d.primarySet[channelID]; ok {
		return core.ResourceProtectionRolePrimary
	}
	if len(d.fallbackSet) > 0 {
		if _, ok := d.fallbackSet[channelID]; ok {
			return core.ResourceProtectionRoleFallback
		}
		return ""
	}
	return core.ResourceProtectionRoleFallback
}

func applyResourceProtectionExplanation(explanation *core.CandidateExplanation, candidate core.Candidate, decision resourceProtectionDecision) {
	if explanation == nil || !decision.Enabled {
		return
	}
	explanation.ResourceProtectionRole = decision.roleFor(candidate)
	explanation.ResourceProtectionReason = decision.Reason
}

func candidateChannelID(candidate core.Candidate) int {
	if candidate.Channel != nil {
		return candidate.Channel.Id
	}
	if candidate.RuntimeKey.ChannelID > 0 {
		return candidate.RuntimeKey.ChannelID
	}
	return 0
}

func resourceProtectionNoCandidateError(policy core.GroupSmartPolicy, decision resourceProtectionDecision) *types.NewAPIError {
	group := strings.TrimSpace(policy.RequestedGroup)
	if group == "" {
		group = strings.TrimSpace(policy.UserGroup)
	}
	reason := decision.Reason
	if reason == "" {
		reason = core.ResourceProtectionReasonNoFallbackCandidate
	}
	return types.NewErrorWithStatusCode(
		fmt.Errorf("group %s has no selectable fallback resource after resource protection (%s)", group, reason),
		types.ErrorCodeGetChannelFailed,
		http.StatusTooManyRequests,
	)
}

func intSet(values []int) map[int]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			out[value] = struct{}{}
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
