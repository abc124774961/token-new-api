package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	Id               int                                 `json:"id"`
	Name             string                              `json:"name" gorm:"type:varchar(128);not null"`
	Enabled          bool                                `json:"enabled" gorm:"default:true;index"`
	Priority         int                                 `json:"priority" gorm:"type:int;default:0;index"`
	ScopeType        string                              `json:"scope_type" gorm:"type:varchar(32);not null;default:'global';index"`
	ScopeValue       string                              `json:"scope_value" gorm:"type:varchar(128);not null;default:'';index"`
	ScopeID          int                                 `json:"scope_id" gorm:"type:int;default:0;index"`
	ScopeKey         string                              `json:"scope_key" gorm:"type:varchar(191);default:'';index"`
	ScopeName        string                              `json:"scope_name" gorm:"type:varchar(191);default:''"`
	UsingGroups      string                              `json:"using_groups" gorm:"type:text"`
	GroupMultipliers string                              `json:"group_multipliers" gorm:"type:text"`
	Models           string                              `json:"models" gorm:"type:text"`
	Mode             string                              `json:"mode" gorm:"type:varchar(32);not null;default:'multiply'"`
	Multiplier       float64                             `json:"multiplier" gorm:"type:decimal(18,8);not null;default:1"`
	StartAt          int64                               `json:"start_at" gorm:"bigint;default:0;index"`
	EndAt            int64                               `json:"end_at" gorm:"bigint;default:0;index"`
	Description      string                              `json:"description" gorm:"type:varchar(255);default:''"`
	CreatedAt        int64                               `json:"created_at" gorm:"bigint"`
	UpdatedAt        int64                               `json:"updated_at" gorm:"bigint"`
	Targets          []BillingMultiplierPolicyTarget     `json:"targets,omitempty" gorm:"-"`
	GroupPrices      []BillingMultiplierPolicyGroupPrice `json:"group_prices,omitempty" gorm:"-"`
	TargetCount      int                                 `json:"target_count" gorm:"-"`
	UserTargetCount  int                                 `json:"user_target_count" gorm:"-"`
	GroupTargetCount int                                 `json:"group_target_count" gorm:"-"`
	PlanTargetCount  int                                 `json:"plan_target_count" gorm:"-"`
	GroupPriceCount  int                                 `json:"group_price_count" gorm:"-"`
}

type BillingMultiplierGroupConfig struct {
	GroupKey   string  `json:"group_key"`
	Mode       string  `json:"mode"`
	Multiplier float64 `json:"multiplier"`
	Enabled    bool    `json:"enabled"`
}

type BillingMultiplierPolicyTarget struct {
	Id         int    `json:"id"`
	PolicyID   int    `json:"policy_id" gorm:"type:int;not null;index"`
	TargetType string `json:"target_type" gorm:"type:varchar(32);not null;index"`
	TargetID   int    `json:"target_id" gorm:"type:int;default:0;index"`
	TargetKey  string `json:"target_key" gorm:"type:varchar(191);default:'';index"`
	TargetName string `json:"target_name" gorm:"type:varchar(191);default:''"`
	Enabled    bool   `json:"enabled" gorm:"default:true;index"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

type BillingMultiplierPolicyGroupPrice struct {
	Id         int     `json:"id"`
	PolicyID   int     `json:"policy_id" gorm:"type:int;not null;index"`
	UsingGroup string  `json:"using_group" gorm:"type:varchar(191);not null;index"`
	GroupKey   string  `json:"group_key,omitempty" gorm:"-"`
	Mode       string  `json:"mode" gorm:"type:varchar(32);not null;default:'override'"`
	Multiplier float64 `json:"multiplier" gorm:"type:decimal(18,8);not null;default:1"`
	Enabled    bool    `json:"enabled" gorm:"default:true;index"`
	Priority   int     `json:"priority" gorm:"type:int;default:0;index"`
	CreatedAt  int64   `json:"created_at" gorm:"bigint"`
	UpdatedAt  int64   `json:"updated_at" gorm:"bigint"`
}

func (t *BillingMultiplierPolicyTarget) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	t.CreatedAt = now
	t.UpdatedAt = now
	return t.Normalize()
}

func (t *BillingMultiplierPolicyTarget) BeforeUpdate(tx *gorm.DB) error {
	t.UpdatedAt = common.GetTimestamp()
	return t.Normalize()
}

func (t *BillingMultiplierPolicyTarget) Normalize() error {
	t.TargetType = strings.TrimSpace(t.TargetType)
	if t.TargetType == "" {
		t.TargetType = BillingMultiplierScopeGlobal
	}
	if !validBillingMultiplierScope(t.TargetType) {
		return fmt.Errorf("invalid target_type: %s", t.TargetType)
	}
	t.TargetKey = strings.TrimSpace(t.TargetKey)
	t.TargetName = strings.TrimSpace(t.TargetName)
	switch t.TargetType {
	case BillingMultiplierScopeGlobal:
		t.TargetID = 0
		t.TargetKey = ""
	case BillingMultiplierScopeUser, BillingMultiplierScopeSubscriptionPlan:
		if t.TargetID <= 0 {
			if parsed, err := strconv.Atoi(t.TargetKey); err == nil && parsed > 0 {
				t.TargetID = parsed
			}
		}
		if t.TargetID <= 0 {
			return fmt.Errorf("%s target_id is required", t.TargetType)
		}
		t.TargetKey = strconv.Itoa(t.TargetID)
	case BillingMultiplierScopeUserGroup, BillingMultiplierScopeUsingGroup:
		if t.TargetKey == "" {
			return fmt.Errorf("%s target_key is required", t.TargetType)
		}
		t.TargetID = 0
	}
	if t.TargetName == "" {
		t.TargetName = t.TargetKey
	}
	return nil
}

func (p *BillingMultiplierPolicyGroupPrice) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	p.CreatedAt = now
	p.UpdatedAt = now
	return p.Normalize()
}

func (p *BillingMultiplierPolicyGroupPrice) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = common.GetTimestamp()
	return p.Normalize()
}

func (p *BillingMultiplierPolicyGroupPrice) Normalize() error {
	p.UsingGroup = strings.TrimSpace(p.UsingGroup)
	if p.UsingGroup == "" {
		p.UsingGroup = strings.TrimSpace(p.GroupKey)
	}
	p.GroupKey = p.UsingGroup
	if p.UsingGroup == "" {
		return errors.New("using_group is required")
	}
	p.Mode = strings.TrimSpace(p.Mode)
	if p.Mode == "" {
		p.Mode = BillingMultiplierModeOverride
	}
	if !validBillingMultiplierMode(p.Mode) {
		return fmt.Errorf("invalid group price mode: %s", p.Mode)
	}
	if p.Multiplier < 0 {
		return errors.New("group price multiplier must be >= 0")
	}
	return nil
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
	if len(p.Targets) == 0 && p.ScopeType != BillingMultiplierScopeGlobal && billingMultiplierPolicyScopeIdentity(p) == "" {
		return errors.New("scope identity is required")
	}
	if p.Multiplier < 0 {
		return errors.New("multiplier must be >= 0")
	}
	if normalized, err := normalizeBillingMultiplierGroupConfigs(p.GroupMultipliers); err != nil {
		return err
	} else {
		p.GroupMultipliers = normalized
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

type billingMultiplierGroupConfigPayload struct {
	GroupKey   string  `json:"group_key"`
	Mode       string  `json:"mode"`
	Multiplier float64 `json:"multiplier"`
	Enabled    *bool   `json:"enabled"`
}

func normalizeBillingMultiplierGroupConfigs(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var payloads []billingMultiplierGroupConfigPayload
	if err := common.UnmarshalJsonStr(raw, &payloads); err != nil {
		return "", fmt.Errorf("invalid group_multipliers: %w", err)
	}
	if len(payloads) == 0 {
		return "", nil
	}
	seen := make(map[string]struct{}, len(payloads))
	configs := make([]BillingMultiplierGroupConfig, 0, len(payloads))
	for _, payload := range payloads {
		groupKey := strings.TrimSpace(payload.GroupKey)
		if groupKey == "" {
			return "", errors.New("group_multipliers group_key is required")
		}
		seenKey := strings.ToLower(groupKey)
		if _, ok := seen[seenKey]; ok {
			return "", fmt.Errorf("duplicate group_multipliers group_key: %s", groupKey)
		}
		seen[seenKey] = struct{}{}
		mode := strings.TrimSpace(payload.Mode)
		if mode == "" {
			mode = BillingMultiplierModeOverride
		}
		if !validBillingMultiplierMode(mode) {
			return "", fmt.Errorf("invalid group_multipliers mode: %s", mode)
		}
		if payload.Multiplier < 0 {
			return "", errors.New("group_multipliers multiplier must be >= 0")
		}
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		configs = append(configs, BillingMultiplierGroupConfig{
			GroupKey:   groupKey,
			Mode:       mode,
			Multiplier: payload.Multiplier,
			Enabled:    enabled,
		})
	}
	data, err := common.Marshal(configs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func billingMultiplierGroupConfigs(raw string) []BillingMultiplierGroupConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payloads []billingMultiplierGroupConfigPayload
	if err := common.UnmarshalJsonStr(raw, &payloads); err != nil {
		return nil
	}
	configs := make([]BillingMultiplierGroupConfig, 0, len(payloads))
	for _, payload := range payloads {
		groupKey := strings.TrimSpace(payload.GroupKey)
		if groupKey == "" {
			continue
		}
		mode := strings.TrimSpace(payload.Mode)
		if mode == "" || !validBillingMultiplierMode(mode) {
			mode = BillingMultiplierModeOverride
		}
		if payload.Multiplier < 0 {
			continue
		}
		enabled := true
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		configs = append(configs, BillingMultiplierGroupConfig{
			GroupKey:   groupKey,
			Mode:       mode,
			Multiplier: payload.Multiplier,
			Enabled:    enabled,
		})
	}
	return configs
}

func billingMultiplierGroupConfigForUsingGroup(raw string, usingGroup string) (BillingMultiplierGroupConfig, bool, bool) {
	configs := billingMultiplierGroupConfigs(raw)
	if len(configs) == 0 {
		return BillingMultiplierGroupConfig{}, false, false
	}
	usingGroup = strings.TrimSpace(usingGroup)
	if usingGroup == "" {
		return BillingMultiplierGroupConfig{}, false, true
	}
	for _, config := range configs {
		if !config.Enabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(config.GroupKey), usingGroup) {
			if strings.TrimSpace(config.Mode) == "" {
				config.Mode = BillingMultiplierModeOverride
			}
			return config, true, true
		}
	}
	return BillingMultiplierGroupConfig{}, false, true
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

const billingMultiplierPolicyCacheTTL = 15 * time.Second

var (
	billingMultiplierCompiledCache        atomic.Value
	billingMultiplierCompiledCacheMu      sync.Mutex
	billingMultiplierPolicyVersionCounter int64
)

type BillingMultiplierPolicySet struct {
	rules     []compiledBillingMultiplierPolicy
	expiresAt int64
	version   int64
}

type compiledBillingMultiplierPolicy struct {
	policy         BillingMultiplierPolicy
	targets        []compiledBillingMultiplierTarget
	groupPrices    []BillingMultiplierPolicyGroupPrice
	usingGroups    []string
	models         []string
	hasGroupPrices bool
}

type compiledBillingMultiplierTarget struct {
	targetType string
	targetID   int
	targetKey  string
	targetName string
}

func ListBillingMultiplierPolicies() ([]BillingMultiplierPolicy, error) {
	var policies []BillingMultiplierPolicy
	if err := DB.Order("priority desc, id desc").Find(&policies).Error; err != nil {
		return nil, err
	}
	if err := attachBillingMultiplierPolicyRelations(DB, policies); err != nil {
		return nil, err
	}
	return policies, nil
}

func GetBillingMultiplierPolicyByID(id int) (*BillingMultiplierPolicy, error) {
	if id <= 0 {
		return nil, errors.New("invalid policy id")
	}
	var policy BillingMultiplierPolicy
	if err := DB.Where("id = ?", id).First(&policy).Error; err != nil {
		return nil, err
	}
	policies := []BillingMultiplierPolicy{policy}
	if err := attachBillingMultiplierPolicyRelations(DB, policies); err != nil {
		return nil, err
	}
	return &policies[0], nil
}

func CreateBillingMultiplierPolicy(policy *BillingMultiplierPolicy) error {
	if policy == nil {
		return errors.New("policy is nil")
	}
	policy.Id = 0
	if err := prepareBillingMultiplierPolicyForSave(policy); err != nil {
		return err
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(policy).Error; err != nil {
			return err
		}
		return replaceBillingMultiplierPolicyRelationsTx(tx, policy)
	})
	if err == nil {
		InvalidateBillingMultiplierPolicyCache()
	}
	return err
}

func UpdateBillingMultiplierPolicy(id int, policy *BillingMultiplierPolicy) error {
	if id <= 0 {
		return errors.New("invalid policy id")
	}
	if policy == nil {
		return errors.New("policy is nil")
	}
	policy.Id = id
	if err := prepareBillingMultiplierPolicyForSave(policy); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"name":              policy.Name,
		"enabled":           policy.Enabled,
		"priority":          policy.Priority,
		"scope_type":        policy.ScopeType,
		"scope_value":       policy.ScopeValue,
		"scope_id":          policy.ScopeID,
		"scope_key":         policy.ScopeKey,
		"scope_name":        policy.ScopeName,
		"using_groups":      policy.UsingGroups,
		"group_multipliers": policy.GroupMultipliers,
		"models":            policy.Models,
		"mode":              policy.Mode,
		"multiplier":        policy.Multiplier,
		"start_at":          policy.StartAt,
		"end_at":            policy.EndAt,
		"description":       policy.Description,
		"updated_at":        common.GetTimestamp(),
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&BillingMultiplierPolicy{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return replaceBillingMultiplierPolicyRelationsTx(tx, policy)
	})
	if err == nil {
		InvalidateBillingMultiplierPolicyCache()
	}
	return err
}

func DeleteBillingMultiplierPolicy(id int) error {
	if id <= 0 {
		return errors.New("invalid policy id")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("policy_id = ?", id).Delete(&BillingMultiplierPolicyTarget{}).Error; err != nil {
			return err
		}
		if err := tx.Where("policy_id = ?", id).Delete(&BillingMultiplierPolicyGroupPrice{}).Error; err != nil {
			return err
		}
		return tx.Delete(&BillingMultiplierPolicy{}, id).Error
	})
	if err == nil {
		InvalidateBillingMultiplierPolicyCache()
	}
	return err
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
	set := compileBillingMultiplierPolicySet(policies, now, BillingMultiplierPolicyVersion())
	return evaluateBillingMultiplierPolicySet(ctx, set, baseRatio, now)
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

	set, err := billingMultiplierPolicySetForDB(db, now)
	if err != nil {
		common.SysLog("failed to evaluate billing multiplier policies: " + err.Error())
		return types.BillingMultiplierSnapshot{
			BaseGroupRatio:  baseRatio,
			FinalGroupRatio: baseRatio,
			Multiplier:      1,
		}
	}

	return evaluateBillingMultiplierPolicySet(ctx, set, baseRatio, now)
}

func evaluateBillingMultiplierPolicySet(ctx BillingMultiplierContext, set *BillingMultiplierPolicySet, baseRatio float64, now int64) types.BillingMultiplierSnapshot {
	finalRatio := baseRatio
	if set == nil || len(set.rules) == 0 {
		return types.BillingMultiplierSnapshot{
			BaseGroupRatio:  baseRatio,
			FinalGroupRatio: baseRatio,
			Multiplier:      1,
		}
	}
	rules := make([]types.BillingMultiplierRuleSnapshot, 0)
	for _, compiled := range set.rules {
		matchedTarget, ok := billingMultiplierCompiledPolicyTargetMatches(compiled, ctx)
		if !ok || !billingMultiplierCompiledPolicyBaseMatches(compiled, ctx, now) {
			continue
		}
		policy := compiled.policy
		mode := policy.Mode
		multiplier := policy.Multiplier
		usingGroup := ""
		groupMultiplier := false
		if price, ok, hasPrices := billingMultiplierGroupPriceForUsingGroup(compiled, ctx.UsingGroup); hasPrices {
			if !ok {
				continue
			}
			mode = price.Mode
			multiplier = price.Multiplier
			usingGroup = price.UsingGroup
			groupMultiplier = true
		}
		switch mode {
		case BillingMultiplierModeOverride:
			finalRatio = multiplier
		case BillingMultiplierModeMin:
			if finalRatio < multiplier {
				finalRatio = multiplier
			}
		case BillingMultiplierModeMax:
			if finalRatio > multiplier {
				finalRatio = multiplier
			}
		default:
			finalRatio *= multiplier
		}
		rules = append(rules, types.BillingMultiplierRuleSnapshot{
			ID:              policy.Id,
			Name:            policy.Name,
			ScopeType:       matchedTarget.targetType,
			ScopeValue:      billingMultiplierTargetScopeValue(matchedTarget),
			ScopeID:         matchedTarget.targetID,
			ScopeKey:        matchedTarget.targetKey,
			ScopeName:       matchedTarget.targetName,
			UsingGroup:      usingGroup,
			GroupMultiplier: groupMultiplier,
			Mode:            mode,
			Multiplier:      multiplier,
			Priority:        policy.Priority,
			Description:     strings.TrimSpace(policy.Description),
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

func billingMultiplierPolicySetForDB(db *gorm.DB, now int64) (*BillingMultiplierPolicySet, error) {
	if db != DB {
		var policies []BillingMultiplierPolicy
		if err := db.Where("enabled = ?", true).Order("priority desc, id asc").Find(&policies).Error; err != nil {
			return nil, err
		}
		if err := attachBillingMultiplierPolicyRelations(db, policies); err != nil {
			return nil, err
		}
		return compileBillingMultiplierPolicySet(policies, now, BillingMultiplierPolicyVersion()), nil
	}

	version := BillingMultiplierPolicyVersion()
	if cached, ok := billingMultiplierCompiledCache.Load().(*BillingMultiplierPolicySet); ok {
		if cached != nil && cached.version == version && cached.expiresAt > now {
			return cached, nil
		}
	}

	billingMultiplierCompiledCacheMu.Lock()
	defer billingMultiplierCompiledCacheMu.Unlock()

	if cached, ok := billingMultiplierCompiledCache.Load().(*BillingMultiplierPolicySet); ok {
		if cached != nil && cached.version == version && cached.expiresAt > now {
			return cached, nil
		}
	}

	var policies []BillingMultiplierPolicy
	if err := db.Where("enabled = ?", true).Order("priority desc, id asc").Find(&policies).Error; err != nil {
		return nil, err
	}
	if err := attachBillingMultiplierPolicyRelations(db, policies); err != nil {
		return nil, err
	}
	set := compileBillingMultiplierPolicySet(policies, now, version)
	billingMultiplierCompiledCache.Store(set)
	return set, nil
}

func InvalidateBillingMultiplierPolicyCache() {
	atomic.AddInt64(&billingMultiplierPolicyVersionCounter, 1)
}

func BillingMultiplierPolicyVersion() int64 {
	return atomic.LoadInt64(&billingMultiplierPolicyVersionCounter)
}

func compileBillingMultiplierPolicySet(policies []BillingMultiplierPolicy, now int64, version int64) *BillingMultiplierPolicySet {
	rules := make([]compiledBillingMultiplierPolicy, 0, len(policies))
	for _, policy := range policies {
		if err := policy.Normalize(); err != nil {
			common.SysLog("skip invalid billing multiplier policy: " + err.Error())
			continue
		}
		compiled := compiledBillingMultiplierPolicy{
			policy:         policy,
			targets:        compileBillingMultiplierTargets(policy),
			groupPrices:    compileBillingMultiplierGroupPrices(policy),
			usingGroups:    billingMultiplierListValues(policy.UsingGroups),
			models:         billingMultiplierListValues(policy.Models),
			hasGroupPrices: len(policy.GroupPrices) > 0 || len(billingMultiplierGroupConfigs(policy.GroupMultipliers)) > 0,
		}
		if len(compiled.targets) == 0 {
			if len(policy.Targets) > 0 {
				continue
			}
			compiled.targets = []compiledBillingMultiplierTarget{{targetType: BillingMultiplierScopeGlobal}}
		}
		rules = append(rules, compiled)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].policy.Priority == rules[j].policy.Priority {
			return rules[i].policy.Id < rules[j].policy.Id
		}
		return rules[i].policy.Priority > rules[j].policy.Priority
	})
	return &BillingMultiplierPolicySet{
		rules:     rules,
		expiresAt: now + int64(billingMultiplierPolicyCacheTTL.Seconds()),
		version:   version,
	}
}

func billingMultiplierCompiledPolicyBaseMatches(compiled compiledBillingMultiplierPolicy, ctx BillingMultiplierContext, now int64) bool {
	policy := compiled.policy
	if !policy.Enabled {
		return false
	}
	if policy.StartAt > 0 && now < policy.StartAt {
		return false
	}
	if policy.EndAt > 0 && now > policy.EndAt {
		return false
	}
	if compiled.hasGroupPrices {
		if _, ok, _ := billingMultiplierGroupPriceForUsingGroup(compiled, ctx.UsingGroup); !ok {
			return false
		}
	} else if !billingMultiplierStringListMatches(compiled.usingGroups, ctx.UsingGroup) {
		return false
	}
	if !billingMultiplierStringListMatches(compiled.models, ctx.ModelName) {
		return false
	}
	return true
}

func billingMultiplierCompiledPolicyTargetMatches(compiled compiledBillingMultiplierPolicy, ctx BillingMultiplierContext) (compiledBillingMultiplierTarget, bool) {
	for _, target := range compiled.targets {
		switch target.targetType {
		case BillingMultiplierScopeGlobal:
			return target, true
		case BillingMultiplierScopeUser:
			if target.targetID > 0 && target.targetID == ctx.UserID {
				return target, true
			}
		case BillingMultiplierScopeUserGroup:
			if strings.EqualFold(target.targetKey, strings.TrimSpace(ctx.UserGroup)) {
				return target, true
			}
		case BillingMultiplierScopeSubscriptionPlan:
			if ctx.SubscriptionPlanID > 0 && target.targetID == ctx.SubscriptionPlanID {
				return target, true
			}
			for _, planID := range ctx.SubscriptionPlanIDs {
				if target.targetID == planID {
					return target, true
				}
			}
		case BillingMultiplierScopeUsingGroup:
			if strings.EqualFold(target.targetKey, strings.TrimSpace(ctx.UsingGroup)) {
				return target, true
			}
		}
	}
	return compiledBillingMultiplierTarget{}, false
}

func billingMultiplierGroupPriceForUsingGroup(compiled compiledBillingMultiplierPolicy, usingGroup string) (BillingMultiplierPolicyGroupPrice, bool, bool) {
	if !compiled.hasGroupPrices {
		return BillingMultiplierPolicyGroupPrice{}, false, false
	}
	usingGroup = strings.TrimSpace(usingGroup)
	if usingGroup == "" {
		return BillingMultiplierPolicyGroupPrice{}, false, true
	}
	for _, price := range compiled.groupPrices {
		if !price.Enabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(price.UsingGroup), usingGroup) {
			return price, true, true
		}
	}
	return BillingMultiplierPolicyGroupPrice{}, false, true
}

func billingMultiplierTargetScopeValue(target compiledBillingMultiplierTarget) string {
	switch target.targetType {
	case BillingMultiplierScopeUser, BillingMultiplierScopeSubscriptionPlan:
		if target.targetID > 0 {
			return strconv.Itoa(target.targetID)
		}
	}
	return target.targetKey
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
	return billingMultiplierStringListMatches(billingMultiplierListValues(raw), value)
}

func prepareBillingMultiplierPolicyForSave(policy *BillingMultiplierPolicy) error {
	if policy == nil {
		return errors.New("policy is nil")
	}
	if err := policy.Normalize(); err != nil {
		return err
	}
	if len(policy.Targets) == 0 {
		policy.Targets = legacyBillingMultiplierPolicyTargets(*policy)
	}
	if len(policy.Targets) == 0 {
		policy.Targets = []BillingMultiplierPolicyTarget{{TargetType: BillingMultiplierScopeGlobal, Enabled: true}}
	}
	for i := range policy.Targets {
		policy.Targets[i].PolicyID = policy.Id
		if !policy.Targets[i].Enabled && policy.Targets[i].Id == 0 {
			policy.Targets[i].Enabled = true
		}
		if err := policy.Targets[i].Normalize(); err != nil {
			return err
		}
	}
	if len(policy.GroupPrices) == 0 {
		policy.GroupPrices = legacyBillingMultiplierGroupPrices(*policy)
	}
	if len(policy.GroupPrices) > 0 {
		seen := make(map[string]struct{}, len(policy.GroupPrices))
		for i := range policy.GroupPrices {
			policy.GroupPrices[i].PolicyID = policy.Id
			if !policy.GroupPrices[i].Enabled && policy.GroupPrices[i].Id == 0 {
				policy.GroupPrices[i].Enabled = true
			}
			if err := policy.GroupPrices[i].Normalize(); err != nil {
				return err
			}
			seenKey := strings.ToLower(policy.GroupPrices[i].UsingGroup)
			if _, ok := seen[seenKey]; ok {
				return fmt.Errorf("duplicate group price using_group: %s", policy.GroupPrices[i].UsingGroup)
			}
			seen[seenKey] = struct{}{}
		}
		if text, err := groupPricesToLegacyGroupMultipliers(policy.GroupPrices); err != nil {
			return err
		} else {
			policy.GroupMultipliers = text
			policy.UsingGroups = listToJSONTextForBillingMultiplier(groupPriceUsingGroups(policy.GroupPrices))
		}
	}
	return nil
}

func attachBillingMultiplierPolicyRelations(db *gorm.DB, policies []BillingMultiplierPolicy) error {
	if db == nil || len(policies) == 0 {
		return nil
	}
	ids := make([]int, 0, len(policies))
	index := make(map[int]int, len(policies))
	for i := range policies {
		if policies[i].Id <= 0 {
			continue
		}
		ids = append(ids, policies[i].Id)
		index[policies[i].Id] = i
	}
	if len(ids) == 0 {
		return nil
	}
	if db.Migrator().HasTable(&BillingMultiplierPolicyTarget{}) {
		var targets []BillingMultiplierPolicyTarget
		if err := db.Where("policy_id IN ?", ids).Order("id asc").Find(&targets).Error; err != nil {
			return err
		}
		for _, target := range targets {
			if pos, ok := index[target.PolicyID]; ok {
				policies[pos].Targets = append(policies[pos].Targets, target)
			}
		}
	}
	if db.Migrator().HasTable(&BillingMultiplierPolicyGroupPrice{}) {
		var prices []BillingMultiplierPolicyGroupPrice
		if err := db.Where("policy_id IN ?", ids).Order("priority desc, id asc").Find(&prices).Error; err != nil {
			return err
		}
		for _, price := range prices {
			price.GroupKey = price.UsingGroup
			if pos, ok := index[price.PolicyID]; ok {
				policies[pos].GroupPrices = append(policies[pos].GroupPrices, price)
			}
		}
	}
	for i := range policies {
		if len(policies[i].Targets) == 0 {
			policies[i].Targets = legacyBillingMultiplierPolicyTargets(policies[i])
		}
		if len(policies[i].GroupPrices) == 0 {
			policies[i].GroupPrices = legacyBillingMultiplierGroupPrices(policies[i])
		}
		policies[i].fillBillingMultiplierRelationCounts()
	}
	return nil
}

func replaceBillingMultiplierPolicyRelationsTx(tx *gorm.DB, policy *BillingMultiplierPolicy) error {
	if tx == nil || policy == nil || policy.Id <= 0 {
		return nil
	}
	if err := tx.Where("policy_id = ?", policy.Id).Delete(&BillingMultiplierPolicyTarget{}).Error; err != nil {
		return err
	}
	targets := make([]BillingMultiplierPolicyTarget, 0, len(policy.Targets))
	for _, target := range policy.Targets {
		target.PolicyID = policy.Id
		if !target.Enabled && target.Id == 0 {
			target.Enabled = true
		}
		if err := target.Normalize(); err != nil {
			return err
		}
		target.Id = 0
		targets = append(targets, target)
	}
	if len(targets) > 0 {
		if err := tx.Create(&targets).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("policy_id = ?", policy.Id).Delete(&BillingMultiplierPolicyGroupPrice{}).Error; err != nil {
		return err
	}
	prices := make([]BillingMultiplierPolicyGroupPrice, 0, len(policy.GroupPrices))
	for _, price := range policy.GroupPrices {
		price.PolicyID = policy.Id
		if !price.Enabled && price.Id == 0 {
			price.Enabled = true
		}
		if err := price.Normalize(); err != nil {
			return err
		}
		price.Id = 0
		prices = append(prices, price)
	}
	if len(prices) > 0 {
		if err := tx.Create(&prices).Error; err != nil {
			return err
		}
	}
	return nil
}

func (p *BillingMultiplierPolicy) fillBillingMultiplierRelationCounts() {
	p.TargetCount = 0
	p.UserTargetCount = 0
	p.GroupTargetCount = 0
	p.PlanTargetCount = 0
	for _, target := range p.Targets {
		if !target.Enabled {
			continue
		}
		p.TargetCount++
		switch target.TargetType {
		case BillingMultiplierScopeUser:
			p.UserTargetCount++
		case BillingMultiplierScopeUserGroup:
			p.GroupTargetCount++
		case BillingMultiplierScopeSubscriptionPlan:
			p.PlanTargetCount++
		}
	}
	p.GroupPriceCount = 0
	for _, price := range p.GroupPrices {
		if price.Enabled {
			p.GroupPriceCount++
		}
	}
}

func compileBillingMultiplierTargets(policy BillingMultiplierPolicy) []compiledBillingMultiplierTarget {
	targets := policy.Targets
	if len(targets) == 0 {
		targets = legacyBillingMultiplierPolicyTargets(policy)
	}
	compiled := make([]compiledBillingMultiplierTarget, 0, len(targets))
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		if err := target.Normalize(); err != nil {
			continue
		}
		compiled = append(compiled, compiledBillingMultiplierTarget{
			targetType: target.TargetType,
			targetID:   target.TargetID,
			targetKey:  strings.TrimSpace(target.TargetKey),
			targetName: strings.TrimSpace(target.TargetName),
		})
	}
	return compiled
}

func compileBillingMultiplierGroupPrices(policy BillingMultiplierPolicy) []BillingMultiplierPolicyGroupPrice {
	prices := policy.GroupPrices
	if len(prices) == 0 {
		prices = legacyBillingMultiplierGroupPrices(policy)
	}
	compiled := make([]BillingMultiplierPolicyGroupPrice, 0, len(prices))
	for _, price := range prices {
		if !price.Enabled {
			continue
		}
		if err := price.Normalize(); err != nil {
			continue
		}
		compiled = append(compiled, price)
	}
	sort.SliceStable(compiled, func(i, j int) bool {
		if compiled[i].Priority == compiled[j].Priority {
			return compiled[i].Id < compiled[j].Id
		}
		return compiled[i].Priority > compiled[j].Priority
	})
	return compiled
}

func legacyBillingMultiplierPolicyTargets(policy BillingMultiplierPolicy) []BillingMultiplierPolicyTarget {
	target := BillingMultiplierPolicyTarget{
		PolicyID:   policy.Id,
		TargetType: policy.ScopeType,
		TargetID:   policy.ScopeID,
		TargetKey:  policy.ScopeKey,
		TargetName: policy.ScopeName,
		Enabled:    true,
	}
	if target.TargetType == "" {
		target.TargetType = BillingMultiplierScopeGlobal
	}
	if target.TargetKey == "" {
		target.TargetKey = billingMultiplierPolicyScopeIdentity(&policy)
	}
	if target.TargetID <= 0 && (target.TargetType == BillingMultiplierScopeUser || target.TargetType == BillingMultiplierScopeSubscriptionPlan) {
		if parsed, err := strconv.Atoi(billingMultiplierPolicyScopeIdentity(&policy)); err == nil && parsed > 0 {
			target.TargetID = parsed
		}
	}
	if target.TargetType != BillingMultiplierScopeGlobal && target.TargetID <= 0 && target.TargetKey == "" {
		return nil
	}
	if err := target.Normalize(); err != nil {
		return nil
	}
	return []BillingMultiplierPolicyTarget{target}
}

func legacyBillingMultiplierGroupPrices(policy BillingMultiplierPolicy) []BillingMultiplierPolicyGroupPrice {
	configs := billingMultiplierGroupConfigs(policy.GroupMultipliers)
	if len(configs) == 0 {
		return nil
	}
	prices := make([]BillingMultiplierPolicyGroupPrice, 0, len(configs))
	for i, config := range configs {
		price := BillingMultiplierPolicyGroupPrice{
			PolicyID:   policy.Id,
			UsingGroup: strings.TrimSpace(config.GroupKey),
			GroupKey:   strings.TrimSpace(config.GroupKey),
			Mode:       config.Mode,
			Multiplier: config.Multiplier,
			Enabled:    config.Enabled,
			Priority:   len(configs) - i,
		}
		if err := price.Normalize(); err != nil {
			continue
		}
		prices = append(prices, price)
	}
	return prices
}

func groupPricesToLegacyGroupMultipliers(prices []BillingMultiplierPolicyGroupPrice) (string, error) {
	configs := make([]BillingMultiplierGroupConfig, 0, len(prices))
	for _, price := range prices {
		if !price.Enabled {
			continue
		}
		if err := price.Normalize(); err != nil {
			return "", err
		}
		configs = append(configs, BillingMultiplierGroupConfig{
			GroupKey:   price.UsingGroup,
			Mode:       price.Mode,
			Multiplier: price.Multiplier,
			Enabled:    price.Enabled,
		})
	}
	if len(configs) == 0 {
		return "", nil
	}
	data, err := common.Marshal(configs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func groupPriceUsingGroups(prices []BillingMultiplierPolicyGroupPrice) []string {
	groups := make([]string, 0, len(prices))
	for _, price := range prices {
		if price.Enabled && strings.TrimSpace(price.UsingGroup) != "" {
			groups = append(groups, strings.TrimSpace(price.UsingGroup))
		}
	}
	return groups
}

func listToJSONTextForBillingMultiplier(values []string) string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seenKey := strings.ToLower(value)
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return ""
	}
	data, err := common.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(data)
}

func billingMultiplierListValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	values := make([]string, 0)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		values = strings.Split(raw, ",")
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seenKey := strings.ToLower(value)
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func billingMultiplierStringListMatches(values []string, value string) bool {
	if len(values) == 0 {
		return true
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, item := range values {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}
