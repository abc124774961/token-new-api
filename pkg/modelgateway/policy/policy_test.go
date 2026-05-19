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
