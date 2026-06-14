package modelgateway

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type SmartDispatchDeps struct {
	PolicyResolver core.GroupPolicyResolver
	AutoResolver   core.AutoGroupResolver
	Selector       core.SmartChannelSelector
	LegacySelector core.LegacyChannelSelector
	Recorder       core.ExecutionRecorder
}

type SmartDispatchFacade struct {
	policyResolver core.GroupPolicyResolver
	autoResolver   core.AutoGroupResolver
	selector       core.SmartChannelSelector
	legacySelector core.LegacyChannelSelector
	recorder       core.ExecutionRecorder
}

func NewSmartDispatchFacade(deps SmartDispatchDeps) *SmartDispatchFacade {
	f := &SmartDispatchFacade{
		policyResolver: deps.PolicyResolver,
		autoResolver:   deps.AutoResolver,
		selector:       deps.Selector,
		legacySelector: deps.LegacySelector,
		recorder:       deps.Recorder,
	}
	if f.recorder == nil {
		f.recorder = core.NoopRecorder{}
	}
	return f
}

func (f *SmartDispatchFacade) Select(c *gin.Context, param *service.RetryParam) (*core.DispatchPlan, bool, *types.NewAPIError) {
	if f == nil || f.policyResolver == nil || param == nil {
		return nil, false, nil
	}
	req := core.NewDispatchRequestFromGin(c, param)
	basePolicy := f.policyResolver.Resolve(c, &req)
	policy := basePolicy
	if policy.Mode == core.ModeOff || policy.Mode == "" {
		return nil, false, nil
	}
	if policy.Mode == core.ModeShadow {
		return nil, false, nil
	}
	policy = f.resolveCandidateGroups(c, &req, policy)
	if f.selector == nil {
		return nil, false, nil
	}
	tryCandidateGroupFallback := shouldTryCandidateGroupFallback(req, basePolicy, policy)
	tryLossMakingLastFallback := shouldTryLossMakingLastFallback(req, basePolicy, policy)
	if tryCandidateGroupFallback {
		policy.SuppressLossMakingFallback = true
	} else if tryLossMakingLastFallback {
		policy.SuppressLossMakingFallback = true
	}
	plan, handled, apiErr := f.selector.Select(c, param, policy)
	if apiErr == nil && (plan == nil || plan.Channel == nil) && tryCandidateGroupFallback {
		fallbackReq := req
		fallbackReq.CandidateGroupFallback = true
		fallbackPolicy := f.resolveCandidateGroups(c, &fallbackReq, basePolicy)
		fallbackPolicy.SuppressLossMakingFallback = true
		plan, handled, apiErr = f.selector.Select(c, param, fallbackPolicy)
		if apiErr == nil && (plan == nil || plan.Channel == nil) {
			fallbackPolicy.SuppressLossMakingFallback = false
			plan, handled, apiErr = f.selector.Select(c, param, fallbackPolicy)
		}
		policy = fallbackPolicy
	} else if apiErr == nil && (plan == nil || plan.Channel == nil) && tryLossMakingLastFallback {
		policy.SuppressLossMakingFallback = false
		plan, handled, apiErr = f.selector.Select(c, param, policy)
	}
	if handled && plan != nil {
		plan.PolicyMode = policy.Mode
		plan.AutoMode = policy.AutoMode
		plan.BillingRatioMode = policy.BillingRatioMode
		f.recorder.Record(context.Background(), core.DispatchRecord{
			Request:    req,
			Policy:     policy,
			Plan:       plan,
			RecordedAt: time.Now(),
		})
	}
	return plan, handled, apiErr
}

func shouldTryCandidateGroupFallback(req core.DispatchRequest, basePolicy core.GroupSmartPolicy, resolvedPolicy core.GroupSmartPolicy) bool {
	if req.RequestedGroup == "" || req.RequestedGroup == "auto" || req.CandidateGroupFallback {
		return false
	}
	if basePolicy.Mode != core.ModeActive || !basePolicy.CrossGroupFusion || basePolicy.AutoMode == core.AutoModeFusion {
		return false
	}
	if !hasNonPrimaryCandidateGroup(req.RequestedGroup, basePolicy.CandidateGroups) {
		return false
	}
	return len(resolvedPolicy.CandidateGroups) == 1 && resolvedPolicy.CandidateGroups[0] == req.RequestedGroup
}

func shouldTryLossMakingLastFallback(req core.DispatchRequest, basePolicy core.GroupSmartPolicy, resolvedPolicy core.GroupSmartPolicy) bool {
	if basePolicy.Mode != core.ModeActive {
		return false
	}
	if resolvedPolicy.SuppressLossMakingFallback {
		return false
	}
	if len(resolvedPolicy.CandidateGroups) > 1 {
		return true
	}
	if req.Retry > 0 || req.ExtraRetries > 0 || req.ForceNextAutoGroup || req.ResourceProtectionFallback {
		return true
	}
	if req.RetryRoutingIntent != nil && req.RetryRoutingIntent.Active() {
		return true
	}
	return false
}

func hasNonPrimaryCandidateGroup(primary string, groups []string) bool {
	for _, group := range groups {
		if group != "" && group != primary {
			return true
		}
	}
	return false
}

func (f *SmartDispatchFacade) Shadow(c *gin.Context, param *service.RetryParam, actual *model.Channel, actualGroup string) {
	if f == nil || f.policyResolver == nil || f.recorder == nil || param == nil {
		return
	}
	req := core.NewDispatchRequestFromGin(c, param)
	policy := f.policyResolver.Resolve(c, &req)
	if policy.Mode != core.ModeShadow {
		return
	}
	policy = f.resolveCandidateGroups(c, &req, policy)
	shadowPolicy := policy
	shadowPolicy.Mode = core.ModeActive
	var plan *core.DispatchPlan
	if f.selector != nil {
		if suggested, handled, _ := f.selector.Select(c, param, shadowPolicy); handled {
			plan = suggested
			if plan != nil {
				plan.PolicyMode = core.ModeShadow
			}
		}
	}
	f.recorder.Record(context.Background(), core.DispatchRecord{
		Request:     req,
		Policy:      policy,
		Plan:        plan,
		Actual:      actual,
		ActualGroup: actualGroup,
		Shadow:      true,
		RecordedAt:  time.Now(),
	})
}

func (f *SmartDispatchFacade) Report(c *gin.Context, result *core.AttemptResult) {
	if f == nil || f.recorder == nil || result == nil {
		return
	}
	f.updateStickyLifecycle(c, result)
	f.recorder.Report(context.Background(), *result)
}

func (f *SmartDispatchFacade) updateStickyLifecycle(c *gin.Context, result *core.AttemptResult) {
	if f == nil || f.selector == nil || c == nil || result == nil {
		return
	}
	plan, ok := selectedStickyPlanForLifecycle(c)
	if !ok {
		return
	}
	req := core.DispatchRequest{
		RequestedGroup: plan.RequestedGroup,
		UserGroup:      result.RequestedGroup,
		ModelName:      result.ModelName,
		EndpointType:   result.EndpointType,
	}
	if req.RequestedGroup == "" {
		req.RequestedGroup = plan.SelectedGroup
	}
	if req.UserGroup == "" {
		req.UserGroup = req.RequestedGroup
	}
	if req.ModelName == "" {
		req.ModelName = plan.RuntimeKey.RequestedModel
	}
	if req.EndpointType == "" {
		req.EndpointType = plan.RuntimeKey.EndpointType
	}
	if req.ModelName == "" {
		return
	}
	stickyRouter, ok := stickyRouterFromSelector(f.selector)
	if !ok {
		return
	}
	stickyRouter.Report(c, &req, plan, *result)
}

func selectedStickyPlanForLifecycle(c *gin.Context) (*core.DispatchPlan, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get("modelgateway_failed_sticky_plan")
	if !ok {
		return nil, false
	}
	plan, ok := value.(*core.DispatchPlan)
	return plan, ok && plan != nil && plan.StickySource != ""
}

type stickyRouterProvider interface {
	StickyRouter() core.StickyRouter
}

func stickyRouterFromSelector(selector core.SmartChannelSelector) (core.StickyRouter, bool) {
	provider, ok := selector.(stickyRouterProvider)
	if !ok {
		return nil, false
	}
	router := provider.StickyRouter()
	return router, router != nil
}

func (f *SmartDispatchFacade) resolveCandidateGroups(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) core.GroupSmartPolicy {
	if f == nil || f.autoResolver == nil {
		return policy
	}
	plan := f.autoResolver.Resolve(c, req, policy)
	if policy.AutoMode == "" {
		policy.AutoMode = plan.Mode
	}
	if len(plan.CandidateGroups) > 0 {
		policy.CandidateGroups = append([]string(nil), plan.CandidateGroups...)
	}
	return policy
}

var _ core.SmartDispatchFacadeInterface = (*SmartDispatchFacade)(nil)
