package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveRoutingGroupsForDerivedCodexGroups(t *testing.T) {
	tests := []struct {
		name  string
		group string
		want  []string
	}{
		{
			name:  "plus discount with hyphen",
			group: "codex-plus-特惠",
			want:  []string{"codex-plus-特惠", "codex-plus"},
		},
		{
			name:  "plus typo compatibility",
			group: "code-plus-vip3",
			want:  []string{"code-plus-vip3", "codex-plus"},
		},
		{
			name:  "pro discount without hyphen",
			group: "codex-pro特惠",
			want:  []string{"codex-pro特惠", "codex-pro"},
		},
		{
			name:  "pro mixed case",
			group: "Codex-Pro-VIP",
			want:  []string{"Codex-Pro-VIP", "codex-pro"},
		},
		{
			name:  "base group has no fallback",
			group: "codex-plus",
			want:  []string{"codex-plus"},
		},
		{
			name:  "auto group has no fallback",
			group: "auto",
			want:  []string{"auto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, EffectiveRoutingGroups(tt.group))
		})
	}
}
