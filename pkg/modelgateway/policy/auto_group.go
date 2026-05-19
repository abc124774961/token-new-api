package policy

import (
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
)

type DefaultAutoGroupResolver struct {
	groupService core.GroupPermissionService
}

func NewDefaultAutoGroupResolver(groupService core.GroupPermissionService) *DefaultAutoGroupResolver {
	return &DefaultAutoGroupResolver{groupService: groupService}
}

func (r *DefaultAutoGroupResolver) Resolve(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) core.AutoGroupPlan {
	if req == nil {
		req = &core.DispatchRequest{}
	}
	plan := core.AutoGroupPlan{
		RequestedGroup:  req.RequestedGroup,
		UserGroup:       req.UserGroup,
		CurrentGroup:    req.CurrentAutoGroup,
		StartIndex:      req.CurrentAutoGroupIndex,
		CrossGroupRetry: req.CrossGroupRetry,
		ForceNextGroup:  req.ForceNextAutoGroup,
		Mode:            policy.AutoMode,
	}
	if plan.Mode == "" {
		plan.Mode = core.AutoModeSequential
	}
	if req.RequestedGroup == "auto" {
		if r != nil && r.groupService != nil {
			plan.CandidateGroups = append([]string(nil), r.groupService.GetUserAutoGroup(req.UserGroup)...)
		}
		if !req.HasCurrentAutoGroupIndex {
			plan.StartIndex = autoGroupIndex(plan.CandidateGroups, req.CurrentAutoGroup)
		}
		if plan.CurrentGroup == "" && len(plan.CandidateGroups) > 0 && plan.StartIndex >= 0 && plan.StartIndex < len(plan.CandidateGroups) {
			plan.CurrentGroup = plan.CandidateGroups[plan.StartIndex]
		}
		return plan
	}
	if policy.CrossGroupFusion {
		plan.CandidateGroups = filterUsableGroups(policy.CandidateGroups, req.UserGroup, r.groupService)
	} else if req.RequestedGroup != "" {
		plan.CandidateGroups = []string{req.RequestedGroup}
	}
	return plan
}

func autoGroupIndex(groups []string, current string) int {
	if current == "" {
		return 0
	}
	for i, group := range groups {
		if group == current {
			return i
		}
	}
	return 0
}

func filterUsableGroups(groups []string, userGroup string, groupService core.GroupPermissionService) []string {
	if len(groups) == 0 {
		return nil
	}
	if groupService == nil {
		return append([]string(nil), groups...)
	}
	usable := groupService.GetUserUsableGroups(userGroup)
	filtered := make([]string, 0, len(groups))
	for _, group := range groups {
		if _, ok := usable[group]; ok {
			filtered = append(filtered, group)
		}
	}
	return filtered
}
