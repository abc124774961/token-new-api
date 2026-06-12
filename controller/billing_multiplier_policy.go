package controller

import (
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
