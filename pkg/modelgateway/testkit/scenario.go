package testkit

import (
	"os"

	"github.com/QuantumNous/new-api/common"
)

type DispatchScenario struct {
	Name             string                   `json:"name"`
	Request          DispatchRequestFixture   `json:"request"`
	Policy           GroupPolicyFixture       `json:"policy"`
	AutoGroups       []string                 `json:"auto_groups"`
	UsableGroups     []string                 `json:"usable_groups"`
	Channels         []ChannelFixture         `json:"channels"`
	RuntimeSnapshots []RuntimeSnapshotFixture `json:"runtime_snapshots"`
	StickyState      *StickyFixture           `json:"sticky_state,omitempty"`
	CacheAffinity    *CacheAffinityFixture    `json:"cache_affinity,omitempty"`
	Expected         DispatchExpected         `json:"expected"`
}

type DispatchRequestFixture struct {
	UserGroup      string `json:"user_group"`
	RequestedGroup string `json:"requested_group"`
	ModelName      string `json:"model_name"`
}

type GroupPolicyFixture struct {
	Mode             string   `json:"mode"`
	Strategy         string   `json:"strategy"`
	AutoMode         string   `json:"auto_mode"`
	CrossGroupFusion bool     `json:"cross_group_fusion"`
	CandidateGroups  []string `json:"candidate_groups"`
	QueueEnabled     bool     `json:"queue_enabled"`
}

type ChannelFixture struct {
	ID    int    `json:"id"`
	Group string `json:"group"`
	Name  string `json:"name"`
}

type RuntimeSnapshotFixture struct {
	ChannelID            int     `json:"channel_id"`
	Group                string  `json:"group"`
	SuccessRate          float64 `json:"success_rate"`
	TTFTMs               float64 `json:"ttft_ms"`
	DurationMs           float64 `json:"duration_ms"`
	TokensPerSecond      float64 `json:"tokens_per_second"`
	ActiveConcurrency    int     `json:"active_concurrency"`
	MaxConcurrency       int     `json:"max_concurrency"`
	QueueDepth           int     `json:"queue_depth"`
	EstimatedQueueWaitMs float64 `json:"estimated_queue_wait_ms"`
	CostRatio            float64 `json:"cost_ratio"`
	GroupPriorityRatio   float64 `json:"group_priority_ratio"`
	CircuitOpen          bool    `json:"circuit_open"`
	Cooldown             bool    `json:"cooldown"`
	FailureAvoidance     bool    `json:"failure_avoidance"`
	SampleCount          int     `json:"sample_count"`
}

type StickyFixture struct {
	ChannelID      int     `json:"channel_id"`
	Group          string  `json:"group"`
	KeyFingerprint string  `json:"key_fp"`
	KeepScoreRatio float64 `json:"keep_score_ratio"`
}

type CacheAffinityFixture struct {
	Enabled          bool    `json:"enabled"`
	ChannelID        int     `json:"channel_id"`
	Group            string  `json:"group"`
	KeyFingerprint   string  `json:"key_fp"`
	KeepScoreRatio   float64 `json:"keep_score_ratio"`
	ExpectRetained   bool    `json:"expect_retained"`
	ExpectBrokenText string  `json:"expect_broken_text"`
}

type DispatchExpected struct {
	Handled           bool                   `json:"handled"`
	SelectedChannelID int                    `json:"selected_channel_id"`
	SelectedGroup     string                 `json:"selected_group"`
	FallbackUsed      bool                   `json:"fallback_used"`
	ContextKeys       map[string]any         `json:"context_keys"`
	ScoreBreakdown    map[string]float64     `json:"score_breakdown"`
	Candidates        []CandidateExpectation `json:"candidates,omitempty"`
	StickyRetained    *bool                  `json:"sticky_retained,omitempty"`
	StickyBreak       string                 `json:"sticky_break,omitempty"`
	CacheAffinity     *bool                  `json:"cache_affinity,omitempty"`
}

type CandidateExpectation struct {
	ChannelID    int     `json:"channel_id"`
	Available    *bool   `json:"available,omitempty"`
	RejectReason string  `json:"reject_reason,omitempty"`
	Selected     *bool   `json:"selected,omitempty"`
	ScoreTotal   float64 `json:"score_total,omitempty"`
}

func LoadDispatchScenario(path string) (*DispatchScenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var scenario DispatchScenario
	if err := common.Unmarshal(data, &scenario); err != nil {
		return nil, err
	}
	return &scenario, nil
}
