package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"gorm.io/gorm"
)

const (
	BillingMultiplierScopeGlobal           = "global"
	BillingMultiplierScopeUser             = "user"
	BillingMultiplierScopeUserGroup        = "user_group"
	BillingMultiplierScopeSubscriptionPlan = "subscription_plan"
	BillingMultiplierScopeUsingGroup       = "using_group"

	BillingMultiplierModeMultiply = "multiply"
	BillingMultiplierModeOverride = "override"
	BillingMultiplierModeMin      = "min"
	BillingMultiplierModeMax      = "max"
)

type BillingMultiplierPolicy struct {
	Id          int     `json:"id"`
	Name        string  `json:"name" gorm:"type:varchar(128);not null"`
	Enabled     bool    `json:"enabled" gorm:"default:true;index"`
	Priority    int     `json:"priority" gorm:"type:int;default:0;index"`
	ScopeType   string  `json:"scope_type" gorm:"type:varchar(32);not null;default:'global';index"`
	ScopeValue  string  `json:"scope_value" gorm:"type:varchar(128);not null;default:'';index"`
	ScopeID     int     `json:"scope_id" gorm:"type:int;default:0;index"`
	ScopeKey    string  `json:"scope_key" gorm:"type:varchar(191);default:'';index"`
	ScopeName   string  `json:"scope_name" gorm:"type:varchar(191);default:''"`
	UsingGroups string  `json:"using_groups" gorm:"type:text"`
	Models      string  `json:"models" gorm:"type:text"`
	Mode        string  `json:"mode" gorm:"type:varchar(32);not null;default:'multiply'"`
	Multiplier  float64 `json:"multiplier" gorm:"type:decimal(18,8);not null;default:1"`
	StartAt     int64   `json:"start_at" gorm:"bigint;default:0;index"`
	EndAt       int64   `json:"end_at" gorm:"bigint;default:0;index"`
	Description string  `json:"description" gorm:"type:varchar(255);default:''"`
	CreatedAt   int64   `json:"created_at" gorm:"bigint"`
	UpdatedAt   int64   `json:"updated_at" gorm:"bigint"`
}

func (p *BillingMultiplierPolicy) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return p.Normalize()
}

func (p *BillingMultiplierPolicy) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = common.GetTimestamp()
	return p.Normalize()
}

func (p *BillingMultiplierPolicy) Normalize() error {
	p.Name = strings.TrimSpace(p.Name)
	p.ScopeType = strings.TrimSpace(p.ScopeType)
	if p.ScopeType == "" {
		p.ScopeType = BillingMultiplierScopeGlobal
	}
	p.ScopeValue = strings.TrimSpace(p.ScopeValue)
	p.ScopeKey = strings.TrimSpace(p.ScopeKey)
	p.ScopeName = strings.TrimSpace(p.ScopeName)
	p.Mode = strings.TrimSpace(p.Mode)
	if p.Mode == "" {
		p.Mode = BillingMultiplierModeMultiply
	}
	if !validBillingMultiplierScope(p.ScopeType) {
		return fmt.Errorf("invalid scope_type: %s", p.ScopeType)
	}
	if !validBillingMultiplierMode(p.Mode) {
		return fmt.Errorf("invalid mode: %s", p.Mode)
	}
	p.normalizeScopeIdentity()
	if p.Name == "" {
		p.Name = p.fallbackName()
	}
	if p.ScopeType != BillingMultiplierScopeGlobal && billingMultiplierPolicyScopeIdentity(p) == "" {
		return errors.New("scope identity is required")
	}
	if p.Multiplier < 0 {
		return errors.New("multiplier must be >= 0")
	}
	if p.EndAt > 0 && p.StartAt > 0 && p.EndAt < p.StartAt {
		return errors.New("end_at must be greater than start_at")
	}
	return nil
}

func (p *BillingMultiplierPolicy) normalizeScopeIdentity() {
	switch p.ScopeType {
	case BillingMultiplierScopeGlobal:
		p.ScopeValue = ""
		p.ScopeID = 0
		p.ScopeKey = ""
	case BillingMultiplierScopeUser, BillingMultiplierScopeSubscriptionPlan:
		if p.ScopeID <= 0 {
			if parsed, err := strconv.Atoi(strings.TrimSpace(p.ScopeValue)); err == nil && parsed > 0 {
				p.ScopeID = parsed
			}
		}
		if p.ScopeID > 0 {
			p.ScopeValue = strconv.Itoa(p.ScopeID)
		}
		p.ScopeKey = ""
	case BillingMultiplierScopeUserGroup, BillingMultiplierScopeUsingGroup:
		if p.ScopeKey == "" {
			p.ScopeKey = p.ScopeValue
		}
		p.ScopeKey = strings.TrimSpace(p.ScopeKey)
		if p.ScopeKey != "" {
			p.ScopeValue = p.ScopeKey
		}
		p.ScopeID = 0
	}
}

func (p *BillingMultiplierPolicy) fallbackName() string {
	target := "global"
	identity := billingMultiplierPolicyScopeIdentity(p)
	switch p.ScopeType {
	case BillingMultiplierScopeUser:
		target = "user"
		if identity != "" {
			target = "user #" + identity
		}
	case BillingMultiplierScopeSubscriptionPlan:
		target = "subscription plan"
		if identity != "" {
			target = "subscription plan #" + identity
		}
	case BillingMultiplierScopeUserGroup:
		target = "user group"
		if identity != "" {
			target = "user group " + identity
		}
	case BillingMultiplierScopeUsingGroup:
		target = "using group"
		if identity != "" {
			target = "using group " + identity
		}
	}
	return target + " multiplier rule"
}

func validBillingMultiplierScope(scope string) bool {
	switch scope {
	case BillingMultiplierScopeGlobal,
		BillingMultiplierScopeUser,
		BillingMultiplierScopeUserGroup,
		BillingMultiplierScopeSubscriptionPlan,
		BillingMultiplierScopeUsingGroup:
		return true
	default:
		return false
	}
}

func validBillingMultiplierMode(mode string) bool {
	switch mode {
	case BillingMultiplierModeMultiply,
		BillingMultiplierModeOverride,
		BillingMultiplierModeMin,
		BillingMultiplierModeMax:
		return true
	default:
		return false
	}
}

type BillingMultiplierContext struct {
	UserID              int
	UserGroup           string
	UsingGroup          string
	ModelName           string
	SubscriptionPlanID  int
	SubscriptionPlanIDs []int
	BaseGroupRatio      float64
	Now                 int64
}

func ListBillingMultiplierPolicies() ([]BillingMultiplierPolicy, error) {
	var policies []BillingMultiplierPolicy
	err := DB.Order("priority desc, id desc").Find(&policies).Error
	return policies, err
}

func GetBillingMultiplierPolicyByID(id int) (*BillingMultiplierPolicy, error) {
	if id <= 0 {
		return nil, errors.New("invalid policy id")
	}
	var policy BillingMultiplierPolicy
	if err := DB.Where("id = ?", id).First(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func CreateBillingMultiplierPolicy(policy *BillingMultiplierPolicy) error {
	if policy == nil {
		return errors.New("policy is nil")
	}
	policy.Id = 0
	return DB.Create(policy).Error
}

func UpdateBillingMultiplierPolicy(id int, policy *BillingMultiplierPolicy) error {
	if id <= 0 {
		return errors.New("invalid policy id")
	}
	if policy == nil {
		return errors.New("policy is nil")
	}
	policy.Id = id
	if err := policy.Normalize(); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"name":         policy.Name,
		"enabled":      policy.Enabled,
		"priority":     policy.Priority,
		"scope_type":   policy.ScopeType,
		"scope_value":  policy.ScopeValue,
		"scope_id":     policy.ScopeID,
		"scope_key":    policy.ScopeKey,
		"scope_name":   policy.ScopeName,
		"using_groups": policy.UsingGroups,
		"models":       policy.Models,
		"mode":         policy.Mode,
		"multiplier":   policy.Multiplier,
		"start_at":     policy.StartAt,
		"end_at":       policy.EndAt,
		"description":  policy.Description,
		"updated_at":   common.GetTimestamp(),
	}
	return DB.Model(&BillingMultiplierPolicy{}).Where("id = ?", id).Updates(updates).Error
}

func DeleteBillingMultiplierPolicy(id int) error {
	if id <= 0 {
		return errors.New("invalid policy id")
	}
	return DB.Delete(&BillingMultiplierPolicy{}, id).Error
}

func EvaluateBillingMultiplier(ctx BillingMultiplierContext) types.BillingMultiplierSnapshot {
	return evaluateBillingMultiplierWithDB(DB, ctx)
}

func EvaluateBillingMultiplierWithPolicies(ctx BillingMultiplierContext, policies []BillingMultiplierPolicy) types.BillingMultiplierSnapshot {
	baseRatio := ctx.BaseGroupRatio
	if baseRatio < 0 {
		baseRatio = 0
	}
	now := ctx.Now
	if now <= 0 {
		now = common.GetTimestamp()
	}
	return evaluateBillingMultiplierPolicies(ctx, policies, baseRatio, now)
}

func evaluateBillingMultiplierWithDB(db *gorm.DB, ctx BillingMultiplierContext) types.BillingMultiplierSnapshot {
	baseRatio := ctx.BaseGroupRatio
	if baseRatio < 0 {
		baseRatio = 0
	}
	now := ctx.Now
	if now <= 0 {
		now = common.GetTimestamp()
	}
	if db == nil {
		return types.BillingMultiplierSnapshot{
			BaseGroupRatio:  baseRatio,
			FinalGroupRatio: baseRatio,
			Multiplier:      1,
		}
	}

	var policies []BillingMultiplierPolicy
	if err := db.Where("enabled = ?", true).
		Order("priority desc, id asc").
		Find(&policies).Error; err != nil {
		common.SysLog("failed to evaluate billing multiplier policies: " + err.Error())
		return types.BillingMultiplierSnapshot{
			BaseGroupRatio:  baseRatio,
			FinalGroupRatio: baseRatio,
			Multiplier:      1,
		}
	}

	return evaluateBillingMultiplierPolicies(ctx, policies, baseRatio, now)
}

func evaluateBillingMultiplierPolicies(ctx BillingMultiplierContext, policies []BillingMultiplierPolicy, baseRatio float64, now int64) types.BillingMultiplierSnapshot {
	matched := make([]BillingMultiplierPolicy, 0)
	for _, policy := range policies {
		if !billingMultiplierPolicyMatches(policy, ctx, now) {
			continue
		}
		matched = append(matched, policy)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Priority == matched[j].Priority {
			return matched[i].Id < matched[j].Id
		}
		return matched[i].Priority > matched[j].Priority
	})

	finalRatio := baseRatio
	rules := make([]types.BillingMultiplierRuleSnapshot, 0, len(matched))
	for _, policy := range matched {
		switch policy.Mode {
		case BillingMultiplierModeOverride:
			finalRatio = policy.Multiplier
		case BillingMultiplierModeMin:
			if finalRatio < policy.Multiplier {
				finalRatio = policy.Multiplier
			}
		case BillingMultiplierModeMax:
			if finalRatio > policy.Multiplier {
				finalRatio = policy.Multiplier
			}
		default:
			finalRatio *= policy.Multiplier
		}
		rules = append(rules, types.BillingMultiplierRuleSnapshot{
			ID:          policy.Id,
			Name:        policy.Name,
			ScopeType:   policy.ScopeType,
			ScopeValue:  policy.ScopeValue,
			ScopeID:     policy.ScopeID,
			ScopeKey:    policy.ScopeKey,
			ScopeName:   policy.ScopeName,
			Mode:        policy.Mode,
			Multiplier:  policy.Multiplier,
			Priority:    policy.Priority,
			Description: strings.TrimSpace(policy.Description),
		})
	}
	if finalRatio < 0 {
		finalRatio = 0
	}
	multiplier := 1.0
	if baseRatio != 0 {
		multiplier = finalRatio / baseRatio
	} else if finalRatio != 0 {
		multiplier = finalRatio
	}

	return types.BillingMultiplierSnapshot{
		Applied:         len(rules) > 0,
		BaseGroupRatio:  baseRatio,
		FinalGroupRatio: finalRatio,
		Multiplier:      multiplier,
		Rules:           rules,
	}
}

func billingMultiplierPolicyMatches(policy BillingMultiplierPolicy, ctx BillingMultiplierContext, now int64) bool {
	if !policy.Enabled {
		return false
	}
	if policy.StartAt > 0 && now < policy.StartAt {
		return false
	}
	if policy.EndAt > 0 && now > policy.EndAt {
		return false
	}
	if !billingMultiplierListMatches(policy.UsingGroups, ctx.UsingGroup) {
		return false
	}
	if !billingMultiplierListMatches(policy.Models, ctx.ModelName) {
		return false
	}
	switch policy.ScopeType {
	case BillingMultiplierScopeGlobal:
		return true
	case BillingMultiplierScopeUser:
		if policy.ScopeID > 0 {
			return policy.ScopeID == ctx.UserID
		}
		return billingMultiplierPolicyScopeIdentity(&policy) == strconv.Itoa(ctx.UserID)
	case BillingMultiplierScopeUserGroup:
		return strings.EqualFold(billingMultiplierPolicyScopeIdentity(&policy), strings.TrimSpace(ctx.UserGroup))
	case BillingMultiplierScopeSubscriptionPlan:
		if ctx.SubscriptionPlanID <= 0 && len(ctx.SubscriptionPlanIDs) == 0 {
			return false
		}
		if policy.ScopeID > 0 && policy.ScopeID == ctx.SubscriptionPlanID {
			return true
		}
		identity := billingMultiplierPolicyScopeIdentity(&policy)
		if identity == strconv.Itoa(ctx.SubscriptionPlanID) {
			return true
		}
		for _, planID := range ctx.SubscriptionPlanIDs {
			if (policy.ScopeID > 0 && policy.ScopeID == planID) || identity == strconv.Itoa(planID) {
				return true
			}
		}
		return false
	case BillingMultiplierScopeUsingGroup:
		return strings.EqualFold(billingMultiplierPolicyScopeIdentity(&policy), strings.TrimSpace(ctx.UsingGroup))
	default:
		return false
	}
}

func billingMultiplierPolicyScopeIdentity(policy *BillingMultiplierPolicy) string {
	if policy == nil {
		return ""
	}
	switch policy.ScopeType {
	case BillingMultiplierScopeUser, BillingMultiplierScopeSubscriptionPlan:
		if policy.ScopeID > 0 {
			return strconv.Itoa(policy.ScopeID)
		}
	case BillingMultiplierScopeUserGroup, BillingMultiplierScopeUsingGroup:
		if strings.TrimSpace(policy.ScopeKey) != "" {
			return strings.TrimSpace(policy.ScopeKey)
		}
	}
	return strings.TrimSpace(policy.ScopeValue)
}

func billingMultiplierListMatches(raw string, value string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	values := make([]string, 0)
	if err := common.UnmarshalJsonStr(raw, &values); err == nil {
		if len(values) == 0 {
			return true
		}
		for _, item := range values {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return true
			}
		}
		return false
	}
	for _, item := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}
