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
	policy := f.policyResolver.Resolve(c, &req)
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
	plan, handled, apiErr := f.selector.Select(c, param, policy)
	if handled && plan != nil {
		plan.PolicyMode = policy.Mode
		plan.AutoMode = policy.AutoMode
	}
	return plan, handled, apiErr
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
	f.recorder.Report(context.Background(), *result)
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
