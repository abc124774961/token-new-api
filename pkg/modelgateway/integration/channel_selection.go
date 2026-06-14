package integration

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycredential "github.com/QuantumNous/new-api/pkg/modelgateway/credential"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type SelectionResult struct {
	Channel      *model.Channel
	Group        string
	Plan         *core.DispatchPlan
	SmartHandled bool
	FallbackUsed bool
}

type ChannelSelectionWrapper struct {
	Facade core.SmartDispatchFacadeInterface
	Legacy core.LegacyChannelSelector
	mu     sync.Mutex
}

const (
	routingCacheRefreshRetriedContextKey = "modelgateway_routing_cache_refresh_retried"
	staleSmartCandidateLogInterval       = 30 * time.Second
)

var (
	staleSmartCandidateLogMu   sync.Mutex
	staleSmartCandidateLastLog = map[string]time.Time{}
)

func NewChannelSelectionWrapper(facade core.SmartDispatchFacadeInterface, legacy core.LegacyChannelSelector) *ChannelSelectionWrapper {
	return &ChannelSelectionWrapper{
		Facade: facade,
		Legacy: legacy,
	}
}

func (w *ChannelSelectionWrapper) Select(c *gin.Context, param *service.RetryParam) (*SelectionResult, *types.NewAPIError) {
	if w == nil {
		return nil, types.NewError(errors.New("channel selection wrapper is nil"), types.ErrorCodeGetChannelFailed)
	}
	if result, apiErr := w.SelectSmartOnly(c, param); apiErr != nil || result != nil {
		return result, apiErr
	}
	if w.Legacy == nil {
		return nil, types.NewError(errors.New("legacy channel selector is nil"), types.ErrorCodeGetChannelFailed)
	}
	channel, group, err := w.Legacy.Select(param)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeGetChannelFailed)
	}
	if channel == nil && markSelectionMissRefreshRetried(c) {
		RefreshDefaultRoutingCachesForSelectionMiss("selection_miss")
		if result, apiErr := w.SelectSmartOnly(c, param); apiErr != nil || result != nil {
			return result, apiErr
		}
		channel, group, err = w.Legacy.Select(param)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeGetChannelFailed)
		}
	}
	if w.Facade != nil && channel != nil {
		scheduler.WithStickyRoutingDisabled(c, func() {
			w.Facade.Shadow(c, param, channel, group)
		})
	}
	core.ClearRetryRoutingIntent(c)
	return &SelectionResult{
		Channel:      channel,
		Group:        group,
		FallbackUsed: true,
	}, nil
}

func (w *ChannelSelectionWrapper) SelectSmartOnly(c *gin.Context, param *service.RetryParam) (*SelectionResult, *types.NewAPIError) {
	if w == nil {
		return nil, types.NewError(errors.New("channel selection wrapper is nil"), types.ErrorCodeGetChannelFailed)
	}
	ClearSelectedPlan(c)
	ClearFailedStickyPlan(c)
	if w.Facade == nil {
		return nil, nil
	}
	for attempts := 0; attempts < 4; attempts++ {
		w.mu.Lock()
		plan, handled, apiErr := w.Facade.Select(c, param)
		if apiErr != nil {
			w.mu.Unlock()
			return nil, apiErr
		}
		if !handled || plan == nil || plan.Channel == nil {
			w.mu.Unlock()
			return nil, nil
		}
		if ok, reason := validateSelectedSmartPlan(plan); !ok {
			markSelectedSmartPlanSkipped(c, plan, reason)
			w.mu.Unlock()
			if shouldLogStaleSmartCandidate(plan.Channel.Id, reason) {
				common.SysLog(fmt.Sprintf("model gateway smart selection skipped stale candidate: channel_id=%d reason=%s", plan.Channel.Id, reason))
			}
			RefreshDefaultRoutingCachesForSelectionMiss("stale_smart_candidate")
			continue
		}
		identity := serviceRuntimeIdentityFromPlan(plan)
		if plan.SelectedReason == scheduler.FailureRecoveryProbeSelectedReason && !service.ReserveChannelFailureRecoveryProbe(identity, 0) {
			service.MarkChannelRuntimeSelectionSkipped(c, identity)
			w.mu.Unlock()
			continue
		}
		if !service.ReserveChannelRuntimeSelectionRouting(c, identity) {
			service.MarkChannelRuntimeSelectionSkipped(c, identity)
			w.mu.Unlock()
			continue
		}
		w.mu.Unlock()
		SetSelectedPlan(c, plan)
		core.ClearRetryRoutingIntent(c)
		if plan.StickySource != "" {
			SetFailedStickyPlan(c, plan)
		}
		if plan.CacheAffinity {
			service.MarkChannelAffinitySelection(c, service.ChannelAffinitySelectionInfo{
				SelectedGroup:                plan.SelectedGroup,
				SelectedChannelID:            plan.Channel.Id,
				Retained:                     plan.StickyRetained,
				Broken:                       !plan.StickyRetained,
				BreakReason:                  plan.StickyBreak,
				StickySource:                 plan.StickySource,
				SelectedReason:               plan.SelectedReason,
				AccountID:                    plan.AccountIdentity.AccountID,
				AccountType:                  plan.AccountIdentity.AccountType,
				AccountIdentityKey:           plan.AccountIdentity.AccountIdentityKey,
				CredentialIndex:              plan.CredentialRef.CredentialIndex,
				HasCredentialIndex:           true,
				CredentialSubjectFingerprint: plan.CredentialRef.CredentialSubjectFingerprint,
				CredentialFingerprint:        plan.CredentialRef.CredentialFingerprint,
				ResourceID:                   plan.ResourceRef.ResourceID,
				ResourceType:                 plan.ResourceRef.ResourceType,
				ProxyID:                      plan.ProxyRef.ProxyID,
				PoolLevel:                    plan.PoolLevel,
			})
		}
		return &SelectionResult{
			Channel:      plan.Channel,
			Group:        plan.SelectedGroup,
			Plan:         plan,
			SmartHandled: true,
		}, nil
	}
	return nil, nil
}

func markSelectionMissRefreshRetried(c *gin.Context) bool {
	if c == nil {
		return true
	}
	if _, ok := c.Get(routingCacheRefreshRetriedContextKey); ok {
		return false
	}
	c.Set(routingCacheRefreshRetriedContextKey, true)
	return true
}

func shouldLogStaleSmartCandidate(channelID int, reason string) bool {
	now := time.Now()
	key := fmt.Sprintf("%d:%s", channelID, reason)
	staleSmartCandidateLogMu.Lock()
	defer staleSmartCandidateLogMu.Unlock()
	if last, ok := staleSmartCandidateLastLog[key]; ok && now.Sub(last) < staleSmartCandidateLogInterval {
		return false
	}
	staleSmartCandidateLastLog[key] = now
	return true
}

func markSelectedSmartPlanSkipped(c *gin.Context, plan *core.DispatchPlan, reason string) {
	if plan == nil || plan.Channel == nil {
		return
	}
	switch reason {
	case "channel_disabled":
		service.MarkChannelSelectionSkipped(c, plan.Channel.Id)
	default:
		service.MarkChannelRuntimeSelectionSkipped(c, serviceRuntimeIdentityFromPlan(plan))
	}
}

func serviceRuntimeIdentityFromPlan(plan *core.DispatchPlan) service.ChannelRuntimeIdentity {
	if plan == nil {
		return service.ChannelRuntimeIdentity{}
	}
	key := plan.RuntimeKey
	if key.ChannelID <= 0 && plan.Channel != nil {
		key.ChannelID = plan.Channel.Id
	}
	identity := service.ChannelRuntimeIdentity{
		ChannelID:           key.ChannelID,
		RequestedModel:      key.RequestedModel,
		SelectedGroup:       key.Group,
		EndpointType:        key.EndpointType,
		AccountID:           key.AccountID,
		CredentialIndex:     key.CredentialIndex,
		CredentialSubjectFP: key.CredentialSubjectFP,
		CredentialFP:        key.CredentialFP,
	}
	if identity.AccountID == "" {
		identity.AccountID = plan.AccountIdentity.AccountID
	}
	if identity.AccountID == "" {
		identity.AccountID = plan.CredentialRef.AccountID
	}
	if identity.CredentialSubjectFP == "" {
		identity.CredentialSubjectFP = plan.AccountIdentity.CredentialSubjectFingerprint
	}
	if identity.CredentialSubjectFP == "" {
		identity.CredentialSubjectFP = plan.CredentialRef.CredentialSubjectFingerprint
	}
	if identity.CredentialFP == "" {
		identity.CredentialFP = plan.AccountIdentity.CredentialFingerprint
	}
	if identity.CredentialFP == "" {
		identity.CredentialFP = plan.CredentialRef.CredentialFingerprint
	}
	if identity.CredentialIndex == 0 && plan.AccountIdentity.CredentialIndex != 0 {
		identity.CredentialIndex = plan.AccountIdentity.CredentialIndex
	}
	if identity.CredentialIndex == 0 && plan.CredentialRef.CredentialIndex != 0 {
		identity.CredentialIndex = plan.CredentialRef.CredentialIndex
	}
	if key.AccountID != "" || key.CredentialSubjectFP != "" || key.CredentialFP != "" ||
		plan.AccountIdentity.AccountID != "" || plan.AccountIdentity.AccountIdentityKey != "" ||
		plan.AccountIdentity.AccountUniqueKey != "" || plan.AccountIdentity.CredentialSubjectFingerprint != "" ||
		plan.AccountIdentity.CredentialFingerprint != "" || dispatchPlanHasCredentialRef(plan) {
		if plan.CredentialRef.CredentialIndex != 0 || identity.CredentialIndex == 0 {
			identity.CredentialIndex = plan.CredentialRef.CredentialIndex
		}
		identity.CredentialIndexSet = true
	}
	return identity.Normalize()
}

func RuntimeIdentityFromPlan(plan *core.DispatchPlan) service.ChannelRuntimeIdentity {
	return serviceRuntimeIdentityFromPlan(plan)
}

func validateSelectedSmartPlan(plan *core.DispatchPlan) (bool, string) {
	if plan == nil || plan.Channel == nil {
		return true, ""
	}
	if common.MemoryCacheEnabled || model.DB != nil {
		if current, err := model.CacheGetChannel(plan.Channel.Id); err == nil && current != nil {
			plan.Channel = current
		}
	}
	if channelDisabledForSmartSelection(plan.Channel) {
		return false, "channel_disabled"
	}
	if !dispatchPlanHasCredentialRef(plan) {
		return true, ""
	}
	resolved, apiErr := modelgatewaycredential.ResolveChannelCredential(plan.Channel, plan.CredentialRef)
	if apiErr != nil {
		return false, apiErr.Error()
	}
	if reason := selectedPlanSchedulingRejectReason(plan, resolved); reason != "" {
		return false, reason
	}
	return true, ""
}

func channelDisabledForSmartSelection(channel *model.Channel) bool {
	return channel != nil && channel.Status != 0 && channel.Status != common.ChannelStatusEnabled
}

func dispatchPlanHasCredentialRef(plan *core.DispatchPlan) bool {
	if plan == nil {
		return false
	}
	ref := plan.CredentialRef
	return ref.ResourceID != "" || ref.AccountID != "" || ref.CredentialIndex != 0 ||
		ref.CredentialSubjectFingerprint != "" || ref.CredentialFingerprint != "" || ref.Resolver != ""
}

func selectedPlanSchedulingRejectReason(plan *core.DispatchPlan, resolved modelgatewaycredential.ResolvedCredential) string {
	if plan == nil || plan.Channel == nil || !selectedPlanUsesCodexUsageLimitedCredential(plan.Channel, resolved) {
		return ""
	}
	capability, ok := plan.Channel.ChannelInfo.AccountCapability(resolved.CredentialIndex)
	if !ok || service.ChannelAccountCapabilityAllowsScheduling(capability) {
		return ""
	}
	if reason := service.AccountUsageLimitedRejectReason(capability); reason != "" {
		return reason
	}
	if reason := capability.EffectiveClassification(); reason != "" {
		return reason
	}
	return "account_unavailable"
}

func selectedPlanUsesCodexUsageLimitedCredential(channel *model.Channel, resolved modelgatewaycredential.ResolvedCredential) bool {
	if channel == nil {
		return false
	}
	switch channel.Type {
	case constant.ChannelTypeCodex:
		return true
	case constant.ChannelTypeOpenAI:
		return codexauth.IsOAuthJSONCredential(resolved.Key) || channel.GetOtherSettings().UsesCodexCompatibilityMode()
	default:
		return false
	}
}
