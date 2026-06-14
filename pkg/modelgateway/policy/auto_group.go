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
		configuredGroups := filterUsableGroups(policy.CandidateGroups, req.UserGroup, r.groupService)
		if policy.AutoMode == core.AutoModeFusion {
			plan.CandidateGroups = configuredGroups
		} else if fixedGroupCandidateFallbackAllowed(req) {
			plan.CandidateGroups = fallbackCandidateGroups(req.RequestedGroup, configuredGroups)
		} else if req.RequestedGroup != "" {
			plan.CandidateGroups = []string{req.RequestedGroup}
		}
	} else if req.RequestedGroup != "" {
		plan.CandidateGroups = []string{req.RequestedGroup}
	}
	if req.RequestedGroup != "" && req.RequestedGroup != "auto" && len(plan.CandidateGroups) == 0 {
		plan.CandidateGroups = []string{req.RequestedGroup}
	}
	return plan
}

func fixedGroupCandidateFallbackAllowed(req *core.DispatchRequest) bool {
	if req == nil || req.RequestedGroup == "" || req.RequestedGroup == "auto" {
		return false
	}
	return req.CandidateGroupFallback ||
		req.ForceNextAutoGroup ||
		req.ResourceProtectionFallback ||
		req.Retry > 0 ||
		req.ExtraRetries > 0
}

func fallbackCandidateGroups(requestedGroup string, groups []string) []string {
	out := make([]string, 0, len(groups)+1)
	seen := make(map[string]struct{}, len(groups)+1)
	appendGroup := func(group string) {
		if group == "" {
			return
		}
		if _, ok := seen[group]; ok {
			return
		}
		seen[group] = struct{}{}
		out = append(out, group)
	}
	appendGroup(requestedGroup)
	for _, group := range groups {
		appendGroup(group)
	}
	return out
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

func effectiveRoutingGroups(group string, groupService core.GroupPermissionService) []string {
	if group == "" {
		return nil
	}
	if groupService == nil {
		return []string{group}
	}
	groups := groupService.EffectiveRoutingGroups(group)
	if len(groups) == 0 {
		return []string{group}
	}
	out := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, candidate := range groups {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return []string{group}
	}
	return out
}
