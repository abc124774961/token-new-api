package integration

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
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
	plan, handled, apiErr := w.Facade.Select(c, param)
	if apiErr != nil {
		return nil, apiErr
	}
	if !handled || plan == nil || plan.Channel == nil {
		return nil, nil
	}
	SetSelectedPlan(c, plan)
	if plan.StickySource != "" {
		SetFailedStickyPlan(c, plan)
	}
	if plan.CacheAffinity {
		service.MarkChannelAffinityUsed(c, plan.SelectedGroup, plan.Channel.Id)
	}
	return &SelectionResult{
		Channel:      plan.Channel,
		Group:        plan.SelectedGroup,
		Plan:         plan,
		SmartHandled: true,
	}, nil
}
