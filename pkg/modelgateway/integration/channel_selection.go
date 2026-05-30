package integration

import (
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
	if w.Facade != nil {
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
			service.MarkChannelSelectionSkipped(c, plan.Channel.Id)
			w.mu.Unlock()
			common.SysLog(fmt.Sprintf("model gateway smart selection skipped stale candidate: channel_id=%d reason=%s", plan.Channel.Id, reason))
			RefreshDefaultAccountCandidateIndex()
			continue
		}
		if !service.ReserveChannelSelectionRouting(c, plan.Channel.Id) {
			service.MarkChannelSelectionSkipped(c, plan.Channel.Id)
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
	if _, apiErr := modelgatewaycredential.ResolveChannelCredential(plan.Channel, plan.CredentialRef); apiErr != nil {
		return false, apiErr.Error()
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
	return ref.ResourceID != "" || ref.AccountID != "" || ref.CredentialFingerprint != ""
}
