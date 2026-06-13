package policy_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/stretchr/testify/require"
)

func TestGroupPolicyResolverDefaultsToOffWhenDisabled(t *testing.T) {
	resolver := policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{
		Settings: core.SchedulerSettings{
			Enabled:         false,
			DefaultMode:     core.ModeActive,
			DefaultStrategy: core.StrategySpeedFirst,
		},
	})
	p := resolver.Resolve(nil, &core.DispatchRequest{RequestedGroup: "default", UserGroup: "vip"})
	require.Equal(t, core.ModeOff, p.Mode)
	require.Equal(t, core.StrategySpeedFirst, p.Strategy)
	require.Equal(t, core.AutoModeSequential, p.AutoMode)
}

func TestGroupPolicyResolverCarriesGroupPriorityRatio(t *testing.T) {
	resolver := policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{
		Settings: core.SchedulerSettings{
			Enabled:         true,
			DefaultMode:     core.ModeActive,
			DefaultStrategy: core.StrategyCostFirst,
			GroupPriorityRatio: map[string]float64{
				"codex-plus": 1.4,
				"codex-pro":  0.7,
			},
			GroupPolicies: map[string]core.GroupPolicySetting{
				"auto": {
					Mode:             core.ModeActive,
					Strategy:         core.StrategyCostFirst,
					AutoMode:         core.AutoModeFusion,
					CandidateGroups:  []string{"codex-plus", "codex-pro"},
					CrossGroupFusion: true,
				},
			},
		},
	})

	p := resolver.Resolve(nil, &core.DispatchRequest{RequestedGroup: "auto", UserGroup: "vip"})

	require.Equal(t, core.ModeActive, p.Mode)
	require.Equal(t, core.StrategyCostFirst, p.Strategy)
	require.Equal(t, 1.4, p.GroupPriorityRatio["codex-plus"])
	require.Equal(t, 0.7, p.GroupPriorityRatio["codex-pro"])
}

func TestGroupPolicyResolverCarriesResourceProtectionSettings(t *testing.T) {
	resolver := policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{
		Settings: core.SchedulerSettings{
			Enabled:         true,
			DefaultMode:     core.ModeActive,
			DefaultStrategy: core.StrategyBalanced,
			GroupPolicies: map[string]core.GroupPolicySetting{
				"codex-plus": {
					Mode:                      core.ModeActive,
					ResourceProtectionEnabled: true,
					PrimaryChannelIDs:         []int{11, 12},
					PrimaryWaitTimeoutMs:      4500,
					PrimaryQueueMaxDepth:      9,
					FallbackChannelIDs:        []int{21},
				},
			},
		},
	})

	p := resolver.Resolve(nil, &core.DispatchRequest{RequestedGroup: "codex-plus", UserGroup: "vip"})

	require.True(t, p.ResourceProtectionEnabled)
	require.Equal(t, []int{11, 12}, p.PrimaryChannelIDs)
	require.Equal(t, 4500, p.PrimaryWaitTimeoutMs)
	require.Equal(t, 9, p.PrimaryQueueMaxDepth)
	require.Equal(t, []int{21}, p.FallbackChannelIDs)
}

func TestAutoGroupResolverSequentialUsesUserAutoGroups(t *testing.T) {
	groupService := &testkit.FakeGroupPermissionService{
		UsableGroups: map[string]map[string]string{
			"vip": {"default": "default", "fast": "fast"},
		},
		AutoGroups: map[string][]string{
			"vip": {"default", "fast"},
		},
	}
	resolver := policy.NewDefaultAutoGroupResolver(groupService)
	plan := resolver.Resolve(nil, &core.DispatchRequest{
		RequestedGroup: "auto",
		UserGroup:      "vip",
	}, core.GroupSmartPolicy{AutoMode: core.AutoModeSequential})

	require.Equal(t, []string{"default", "fast"}, plan.CandidateGroups)
	require.Equal(t, "default", plan.CurrentGroup)
	require.Equal(t, 0, plan.StartIndex)
}

func TestAutoGroupResolverCrossGroupFusionFiltersUsableGroups(t *testing.T) {
	groupService := &testkit.FakeGroupPermissionService{
		UsableGroups: map[string]map[string]string{
			"vip": {"default": "default", "fast": "fast"},
		},
	}
	resolver := policy.NewDefaultAutoGroupResolver(groupService)
	plan := resolver.Resolve(nil, &core.DispatchRequest{
		RequestedGroup: "vip",
		UserGroup:      "vip",
	}, core.GroupSmartPolicy{
		CrossGroupFusion: true,
		CandidateGroups:  []string{"default", "fast", "forbidden"},
		AutoMode:         core.AutoModeSequential,
	})

	require.Equal(t, []string{"default", "fast"}, plan.CandidateGroups)
}

func TestAutoGroupResolverUsesEffectiveRoutingGroupsForFixedGroup(t *testing.T) {
	groupService := &testkit.FakeGroupPermissionService{
		EffectiveGroups: map[string][]string{
			"codex-plus-特惠": {"codex-plus-特惠", "codex-plus"},
		},
	}
	resolver := policy.NewDefaultAutoGroupResolver(groupService)

	plan := resolver.Resolve(nil, &core.DispatchRequest{
		RequestedGroup: "codex-plus-特惠",
		UserGroup:      "vip",
	}, core.GroupSmartPolicy{AutoMode: core.AutoModeSequential})

	require.Equal(t, []string{"codex-plus-特惠", "codex-plus"}, plan.CandidateGroups)
}
