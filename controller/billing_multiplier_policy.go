package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type BillingMultiplierPolicyRequest struct {
	Policy model.BillingMultiplierPolicy `json:"policy"`
}

type BillingMultiplierPreviewRequest struct {
	UserID             int                            `json:"user_id"`
	UserGroup          string                         `json:"user_group"`
	UsingGroup         string                         `json:"using_group"`
	ModelName          string                         `json:"model_name"`
	SubscriptionPlanID int                            `json:"subscription_plan_id"`
	BaseGroupRatio     float64                        `json:"base_group_ratio"`
	Policy             *model.BillingMultiplierPolicy `json:"policy"`
}

func ListBillingMultiplierPolicies(c *gin.Context) {
	policies, err := model.ListBillingMultiplierPolicies()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, policies)
}

func CreateBillingMultiplierPolicy(c *gin.Context) {
	var req BillingMultiplierPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := completeBillingMultiplierPolicyRelation(&req.Policy); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreateBillingMultiplierPolicy(&req.Policy); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, req.Policy)
}

func UpdateBillingMultiplierPolicy(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req BillingMultiplierPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := completeBillingMultiplierPolicyRelation(&req.Policy); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateBillingMultiplierPolicy(id, &req.Policy); err != nil {
		common.ApiError(c, err)
		return
	}
	policy, err := model.GetBillingMultiplierPolicyByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, policy)
}

func DeleteBillingMultiplierPolicy(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := model.DeleteBillingMultiplierPolicy(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func PreviewBillingMultiplierPolicy(c *gin.Context) {
	var req BillingMultiplierPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if req.UserID > 0 {
		if strings.TrimSpace(req.UserGroup) == "" {
			if group, err := model.GetUserGroup(req.UserID, false); err == nil {
				req.UserGroup = group
			}
		}
	}
	if req.BaseGroupRatio <= 0 {
		req.BaseGroupRatio = 1
	}
	evalCtx := model.BillingMultiplierContext{
		UserID:             req.UserID,
		UserGroup:          req.UserGroup,
		UsingGroup:         req.UsingGroup,
		ModelName:          req.ModelName,
		SubscriptionPlanID: req.SubscriptionPlanID,
		BaseGroupRatio:     req.BaseGroupRatio,
	}
	if req.Policy == nil {
		common.ApiSuccess(c, model.EvaluateBillingMultiplier(evalCtx))
		return
	}
	policies, err := model.ListBillingMultiplierPolicies()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	policy := *req.Policy
	if err := completeBillingMultiplierPolicyRelation(&policy); err != nil {
		common.ApiError(c, err)
		return
	}
	filtered := make([]model.BillingMultiplierPolicy, 0, len(policies)+1)
	for _, existing := range policies {
		if policy.Id > 0 && existing.Id == policy.Id {
			continue
		}
		filtered = append(filtered, existing)
	}
	if policy.Id == 0 {
		policy.Id = -1
	}
	filtered = append(filtered, policy)
	snapshot := model.EvaluateBillingMultiplierWithPolicies(evalCtx, filtered)
	common.ApiSuccess(c, snapshot)
}

func completeBillingMultiplierPolicyRelation(policy *model.BillingMultiplierPolicy) error {
	if policy == nil {
		return nil
	}
	if err := policy.Normalize(); err != nil {
		return err
	}
	if len(policy.Targets) == 0 {
		policy.Targets = []model.BillingMultiplierPolicyTarget{{
			TargetType: policy.ScopeType,
			TargetID:   policy.ScopeID,
			TargetKey:  policy.ScopeKey,
			TargetName: policy.ScopeName,
			Enabled:    true,
		}}
	}
	for i := range policy.Targets {
		if !policy.Targets[i].Enabled && policy.Targets[i].Id == 0 {
			policy.Targets[i].Enabled = true
		}
		if err := completeBillingMultiplierPolicyTargetRelation(&policy.Targets[i]); err != nil {
			return err
		}
	}
	syncBillingMultiplierLegacyScope(policy)
	for i := range policy.GroupPrices {
		if !policy.GroupPrices[i].Enabled && policy.GroupPrices[i].Id == 0 {
			policy.GroupPrices[i].Enabled = true
		}
		if err := policy.GroupPrices[i].Normalize(); err != nil {
			return err
		}
	}
	return nil
}

func completeBillingMultiplierPolicyTargetRelation(target *model.BillingMultiplierPolicyTarget) error {
	if target == nil {
		return nil
	}
	if err := target.Normalize(); err != nil {
		return err
	}
	switch target.TargetType {
	case model.BillingMultiplierScopeGlobal:
		target.TargetName = ""
	case model.BillingMultiplierScopeUser:
		if target.TargetID <= 0 {
			return errors.New("user target_id is required")
		}
		user, err := model.GetUserById(target.TargetID, false)
		if err != nil {
			return err
		}
		target.TargetName = strings.TrimSpace(user.DisplayName)
		if target.TargetName == "" {
			target.TargetName = strings.TrimSpace(user.Username)
		}
	case model.BillingMultiplierScopeSubscriptionPlan:
		if target.TargetID <= 0 {
			return errors.New("subscription plan target_id is required")
		}
		plan, err := model.GetSubscriptionPlanById(target.TargetID)
		if err != nil {
			return err
		}
		target.TargetName = strings.TrimSpace(plan.Title)
	case model.BillingMultiplierScopeUserGroup, model.BillingMultiplierScopeUsingGroup:
		if strings.TrimSpace(target.TargetName) == "" {
			target.TargetName = target.TargetKey
		}
	}
	return nil
}

func syncBillingMultiplierLegacyScope(policy *model.BillingMultiplierPolicy) {
	if policy == nil {
		return
	}
	policy.ScopeType = model.BillingMultiplierScopeGlobal
	policy.ScopeValue = ""
	policy.ScopeID = 0
	policy.ScopeKey = ""
	policy.ScopeName = ""
	if len(policy.Targets) == 0 {
		return
	}
	first := policy.Targets[0]
	policy.ScopeType = first.TargetType
	policy.ScopeID = first.TargetID
	policy.ScopeKey = first.TargetKey
	policy.ScopeName = first.TargetName
	switch first.TargetType {
	case model.BillingMultiplierScopeUser, model.BillingMultiplierScopeSubscriptionPlan:
		if first.TargetID > 0 {
			policy.ScopeValue = strconv.Itoa(first.TargetID)
		}
	case model.BillingMultiplierScopeUserGroup, model.BillingMultiplierScopeUsingGroup:
		policy.ScopeValue = first.TargetKey
	}
}
