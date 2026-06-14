package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingMultiplierPolicyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&BillingMultiplierPolicy{}))
	return db
}

func TestEvaluateBillingMultiplierAppliesPriorityModes(t *testing.T) {
	db := setupBillingMultiplierPolicyTestDB(t)
	require.NoError(t, db.Create(&BillingMultiplierPolicy{
		Name:       "vip discount",
		Enabled:    true,
		Priority:   10,
		ScopeType:  BillingMultiplierScopeUserGroup,
		ScopeValue: "vip",
		Mode:       BillingMultiplierModeMultiply,
		Multiplier: 0.8,
	}).Error)
	require.NoError(t, db.Create(&BillingMultiplierPolicy{
		Name:       "user override",
		Enabled:    true,
		Priority:   20,
		ScopeType:  BillingMultiplierScopeUser,
		ScopeValue: "12",
		Mode:       BillingMultiplierModeOverride,
		Multiplier: 0.5,
	}).Error)
	require.NoError(t, db.Create(&BillingMultiplierPolicy{
		Name:       "floor",
		Enabled:    true,
		Priority:   0,
		ScopeType:  BillingMultiplierScopeGlobal,
		Mode:       BillingMultiplierModeMin,
		Multiplier: 0.6,
	}).Error)

	snapshot := evaluateBillingMultiplierWithDB(db, BillingMultiplierContext{
		UserID:         12,
		UserGroup:      "vip",
		UsingGroup:     "codex-plus",
		ModelName:      "gpt-test",
		BaseGroupRatio: 1.5,
	})

	require.True(t, snapshot.Applied)
	require.InEpsilon(t, 0.6, snapshot.FinalGroupRatio, 0.0001)
	require.Len(t, snapshot.Rules, 3)
	require.Equal(t, "user override", snapshot.Rules[0].Name)
}

func TestEvaluateBillingMultiplierFiltersByModelGroupAndSubscription(t *testing.T) {
	db := setupBillingMultiplierPolicyTestDB(t)
	require.NoError(t, db.Create(&BillingMultiplierPolicy{
		Name:        "plan codex",
		Enabled:     true,
		Priority:    10,
		ScopeType:   BillingMultiplierScopeSubscriptionPlan,
		ScopeValue:  "3",
		UsingGroups: `["codex-plus"]`,
		Models:      `["gpt-test"]`,
		Mode:        BillingMultiplierModeMultiply,
		Multiplier:  0.7,
	}).Error)

	miss := evaluateBillingMultiplierWithDB(db, BillingMultiplierContext{
		UserID:              12,
		UserGroup:           "vip",
		UsingGroup:          "default",
		ModelName:           "gpt-test",
		SubscriptionPlanIDs: []int{3},
		BaseGroupRatio:      1,
	})
	require.False(t, miss.Applied)
	require.Equal(t, 1.0, miss.FinalGroupRatio)

	hit := evaluateBillingMultiplierWithDB(db, BillingMultiplierContext{
		UserID:              12,
		UserGroup:           "vip",
		UsingGroup:          "codex-plus",
		ModelName:           "gpt-test",
		SubscriptionPlanIDs: []int{2, 3},
		BaseGroupRatio:      1,
	})
	require.True(t, hit.Applied)
	require.InEpsilon(t, 0.7, hit.FinalGroupRatio, 0.0001)
}

func TestEvaluateBillingMultiplierUserScopeUsesStableID(t *testing.T) {
	db := setupBillingMultiplierPolicyTestDB(t)
	require.NoError(t, db.Create(&BillingMultiplierPolicy{
		Name:       "user id discount",
		Enabled:    true,
		Priority:   10,
		ScopeType:  BillingMultiplierScopeUser,
		ScopeID:    84,
		ScopeName:  "old-name",
		Mode:       BillingMultiplierModeMultiply,
		Multiplier: 0.6,
	}).Error)

	hit := evaluateBillingMultiplierWithDB(db, BillingMultiplierContext{
		UserID:         84,
		UserGroup:      "renamed-vip",
		UsingGroup:     "codex-pro",
		ModelName:      "gpt-test",
		BaseGroupRatio: 1,
	})

	require.True(t, hit.Applied)
	require.InEpsilon(t, 0.6, hit.FinalGroupRatio, 0.0001)
	require.Equal(t, 84, hit.Rules[0].ScopeID)
	require.Equal(t, "old-name", hit.Rules[0].ScopeName)
}

func TestBillingMultiplierPolicyNormalizesLegacyIDScopeValue(t *testing.T) {
	policy := BillingMultiplierPolicy{
		Name:       "legacy plan",
		Enabled:    true,
		ScopeType:  BillingMultiplierScopeSubscriptionPlan,
		ScopeValue: "7",
		Mode:       BillingMultiplierModeMultiply,
		Multiplier: 0.8,
	}

	require.NoError(t, policy.Normalize())
	require.Equal(t, 7, policy.ScopeID)
	require.Equal(t, "7", policy.ScopeValue)
}

func TestBillingMultiplierPolicyNormalizesBlankNameWithFallback(t *testing.T) {
	policy := BillingMultiplierPolicy{
		Enabled:    true,
		ScopeType:  BillingMultiplierScopeUserGroup,
		ScopeValue: "vip",
		Mode:       BillingMultiplierModeMultiply,
		Multiplier: 0.8,
	}

	require.NoError(t, policy.Normalize())
	require.Equal(t, "user group vip multiplier rule", policy.Name)
}
