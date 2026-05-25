package core

import (
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

const (
	ModeOff    = "off"
	ModeShadow = "shadow"
	ModeActive = "active"

	AutoModeSequential = "auto_sequential"
	AutoModeFusion     = "auto_fusion"

	StrategyBalanced       = "balanced"
	StrategySpeedFirst     = "speed_first"
	StrategyCostFirst      = "cost_first"
	StrategyStabilityFirst = "stability_first"
)

type DispatchRequest struct {
	RequestID                string
	UserID                   int
	TokenID                  int
	RequestedGroup           string
	UserGroup                string
	ModelName                string
	EndpointType             constant.EndpointType
	RequiresCodexImageTool   bool
	Retry                    int
	ExtraRetries             int
	CrossGroupRetry          bool
	ForceNextAutoGroup       bool
	CurrentAutoGroup         string
	CurrentAutoGroupIndex    int
	HasCurrentAutoGroupIndex bool
}

type GroupSmartPolicy struct {
	RequestedGroup        string
	UserGroup             string
	Mode                  string
	Strategy              string
	AutoMode              string
	CrossGroupFusion      bool
	CandidateGroups       []string
	BillingRatioMode      string
	CacheAffinityEnabled  bool
	QueueEnabled          bool
	QueueHighPriority     bool
	QueuePriority         int
	CircuitBreakerEnabled bool
	GroupPriorityRatio    map[string]float64
}

func (p GroupSmartPolicy) IsActive() bool {
	return p.Mode == ModeActive
}

func (p GroupSmartPolicy) IsShadow() bool {
	return p.Mode == ModeShadow
}

type AutoGroupPlan struct {
	RequestedGroup  string
	UserGroup       string
	CandidateGroups []string
	CurrentGroup    string
	StartIndex      int
	CrossGroupRetry bool
	ForceNextGroup  bool
	Mode            string
}

type DispatchPlan struct {
	Channel               *model.Channel
	SelectedGroup         string
	RequestedGroup        string
	BillingRatioMode      string
	RuntimeKey            RuntimeKey
	ProviderProfile       string
	ProxyMode             string
	ScoreTotal            float64
	ScoreBreakdown        map[string]float64
	RoutingScoreTotal     float64
	RoutingScoreBreakdown map[string]float64
	QueueWaitMs           int
	QueueEnabled          bool
	QueueDepth            int
	QueueCapacity         int
	QueuePriority         int
	SelectedReason        string
	StickySource          string
	StickyKeyFP           string
	StickyRetained        bool
	StickyBreak           string
	CacheAffinity         bool
	FallbackUsed          bool
	PolicyMode            string
	AutoMode              string
	Candidates            []CandidateExplanation
}

type RuntimeKey struct {
	RequestedModel        string                `json:"requested_model,omitempty"`
	UpstreamModel         string                `json:"upstream_model,omitempty"`
	ChannelID             int                   `json:"channel_id,omitempty"`
	Group                 string                `json:"group,omitempty"`
	EndpointType          constant.EndpointType `json:"endpoint_type,omitempty"`
	CapabilityFingerprint string                `json:"capability_fingerprint,omitempty"`
}

type RuntimeSnapshot struct {
	Key                        RuntimeKey
	MatchedRuntimeKey          RuntimeKey
	SampleSource               string
	RecentLatencySamples       []RuntimeLatencySample
	SuccessRate                float64
	TTFTMs                     float64
	DurationMs                 float64
	TokensPerSecond            float64
	SuccessScore               float64
	SpeedScore                 float64
	ExperienceScore            float64
	EmptyOutputRate            float64
	ExperienceIssueRate        float64
	ActiveConcurrency          int
	MaxConcurrency             int
	ConfiguredConcurrencyLimit int
	LearnedConcurrencyLimit    int
	EffectiveConcurrencyLimit  int
	QueueDepth                 int
	QueueCapacity              int
	QueueTimeoutMs             int
	EstimatedQueueWaitMs       float64
	FirstBytePending           int
	SlowFirstBytePending       int
	OldestFirstByteWaitMs      float64
	CostRatio                  float64
	CostReferenceRatio         float64
	CostPricingMode            string
	GroupPriorityRatio         float64
	CircuitState               CircuitState
	CircuitOpen                bool
	Cooldown                   bool
	FailureAvoidance           bool
	ConfigErrorIsolated        bool
	IsolationReason            string
	IsolationUntil             int64
	AuthConfigErrorCount       int
	LastAuthConfigErrorAt      int64
	SampleCount                int
	LastRealAttemptAt          int64
	LastRealSuccessAt          int64
	LastRealFailureAt          int64
	RealSampleCount30m         int
	LastProbeAt                int64
	LastProbeSuccessAt         int64
}

type RuntimeLatencySample struct {
	ObservedAt int64   `json:"observed_at,omitempty"`
	DurationMs float64 `json:"duration_ms,omitempty"`
	TTFTMs     float64 `json:"ttft_ms,omitempty"`
}

type RuntimeQueueSnapshot struct {
	UpdatedAt     int64                         `json:"updated_at,omitempty"`
	NodeID        string                        `json:"node_id,omitempty"`
	Summary       RuntimeQueueSummary           `json:"summary"`
	Channels      []RuntimeQueueChannelSnapshot `json:"channels,omitempty"`
	RuntimeKeys   []RuntimeQueueKeySnapshot     `json:"runtime_keys,omitempty"`
	Groups        []RuntimeQueueGroupSnapshot   `json:"groups,omitempty"`
	RejectReasons []RuntimeQueueReasonCount     `json:"reject_reasons,omitempty"`
	Cooldowns     []RuntimeQueueCooldownHint    `json:"cooldowns,omitempty"`
	Nodes         []RuntimeQueueNodeSnapshot    `json:"nodes,omitempty"`
}

type RuntimeQueueNodeSnapshot struct {
	NodeID        string                        `json:"node_id"`
	UpdatedAt     int64                         `json:"updated_at,omitempty"`
	Summary       RuntimeQueueSummary           `json:"summary"`
	Channels      []RuntimeQueueChannelSnapshot `json:"channels,omitempty"`
	RuntimeKeys   []RuntimeQueueKeySnapshot     `json:"runtime_keys,omitempty"`
	Groups        []RuntimeQueueGroupSnapshot   `json:"groups,omitempty"`
	RejectReasons []RuntimeQueueReasonCount     `json:"reject_reasons,omitempty"`
	Cooldowns     []RuntimeQueueCooldownHint    `json:"cooldowns,omitempty"`
}

type RuntimeQueueSummary struct {
	UpdatedAt            int64 `json:"updated_at,omitempty"`
	TotalQueued          int   `json:"total_queued"`
	TotalDepth           int   `json:"total_depth"`
	TotalCapacity        int   `json:"total_capacity,omitempty"`
	Waiting              int   `json:"waiting"`
	QueuedRequests       int   `json:"queued_requests"`
	WaitingRequests      int   `json:"waiting_requests"`
	QueueChannels        int   `json:"queue_channels"`
	QueueGroups          int   `json:"queue_groups,omitempty"`
	QueueCapacity        int   `json:"queue_capacity,omitempty"`
	MaxQueueDepth        int   `json:"max_queue_depth,omitempty"`
	HighPriorityDepth    int   `json:"high_priority_depth,omitempty"`
	NormalDepth          int   `json:"normal_depth,omitempty"`
	HighPriorityCapacity int   `json:"high_priority_capacity,omitempty"`
	NormalCapacity       int   `json:"normal_capacity,omitempty"`
	QueueNodes           int   `json:"queue_nodes,omitempty"`
}

type RuntimeQueueChannelSnapshot struct {
	ChannelID            int                         `json:"channel_id"`
	QueueDepth           int                         `json:"queue_depth"`
	QueuedRequests       int                         `json:"queued_requests"`
	WaitingRequests      int                         `json:"waiting_requests"`
	QueueCapacity        int                         `json:"queue_capacity,omitempty"`
	MaxQueueDepth        int                         `json:"max_queue_depth,omitempty"`
	HighPriorityDepth    int                         `json:"high_priority_depth,omitempty"`
	NormalDepth          int                         `json:"normal_depth,omitempty"`
	HighPriorityCapacity int                         `json:"high_priority_capacity,omitempty"`
	NormalCapacity       int                         `json:"normal_capacity,omitempty"`
	Groups               []RuntimeQueueGroupSnapshot `json:"groups,omitempty"`
	RejectReason         string                      `json:"reject_reason,omitempty"`
	RejectCount          int                         `json:"reject_count,omitempty"`
}

type RuntimeQueueKeySnapshot struct {
	RuntimeKey            RuntimeKey `json:"runtime_key"`
	RequestedModel        string     `json:"requested_model,omitempty"`
	UpstreamModel         string     `json:"upstream_model,omitempty"`
	ChannelID             int        `json:"channel_id,omitempty"`
	Group                 string     `json:"group,omitempty"`
	EndpointType          string     `json:"endpoint_type,omitempty"`
	CapabilityFingerprint string     `json:"capability_fingerprint,omitempty"`
	QueueDepth            int        `json:"queue_depth"`
	QueuedRequests        int        `json:"queued_requests"`
	WaitingRequests       int        `json:"waiting_requests"`
	HighPriorityDepth     int        `json:"high_priority_depth,omitempty"`
	NormalDepth           int        `json:"normal_depth,omitempty"`
}

type RuntimeQueueGroupSnapshot struct {
	ChannelID         int    `json:"channel_id,omitempty"`
	Group             string `json:"group,omitempty"`
	QueueDepth        int    `json:"queue_depth"`
	QueuedRequests    int    `json:"queued_requests"`
	WaitingRequests   int    `json:"waiting_requests"`
	HighPriorityDepth int    `json:"high_priority_depth,omitempty"`
	NormalDepth       int    `json:"normal_depth,omitempty"`
}

type RuntimeQueueReasonCount struct {
	Reason    string `json:"reason"`
	Count     int    `json:"count"`
	ChannelID int    `json:"channel_id,omitempty"`
	Group     string `json:"group,omitempty"`
}

type RuntimeQueueCooldownHint struct {
	ChannelID                        int    `json:"channel_id,omitempty"`
	Group                            string `json:"group,omitempty"`
	Reason                           string `json:"reason,omitempty"`
	CooldownRemainingSeconds         int64  `json:"cooldown_remaining_seconds,omitempty"`
	FailureAvoidanceRemainingSeconds int64  `json:"failure_avoidance_remaining_seconds,omitempty"`
}

type CircuitState string

const (
	CircuitStateClosed   CircuitState = "closed"
	CircuitStateOpen     CircuitState = "open"
	CircuitStateHalfOpen CircuitState = "half_open"
)

type CircuitSnapshot struct {
	Key               RuntimeKey
	State             CircuitState
	FailureCount      int
	SuccessCount      int
	SampleCount       int
	FailureRate       float64
	OpenReason        string
	ErrorCounts       map[string]int
	OpenUntil         time.Time
	HalfOpenProbeUsed int
	HalfOpenProbeMax  int
}

type Candidate struct {
	Channel                *model.Channel
	Group                  string
	UpstreamModel          string
	ProviderProfile        string
	ProxyMode              string
	RequiresCodexImageTool bool
	RuntimeKey             RuntimeKey
}

type CandidateExplanation struct {
	ChannelID                  int                `json:"channel_id"`
	ChannelName                string             `json:"channel_name,omitempty"`
	Group                      string             `json:"group,omitempty"`
	UpstreamModel              string             `json:"upstream_model,omitempty"`
	ProviderProfile            string             `json:"provider_profile,omitempty"`
	ProxyMode                  string             `json:"proxy_mode,omitempty"`
	RuntimeKey                 RuntimeKey         `json:"runtime_key"`
	Available                  bool               `json:"available"`
	RejectReason               string             `json:"reject_reason,omitempty"`
	SelectionSkipReason        string             `json:"selection_skip_reason,omitempty"`
	ChannelStatus              int                `json:"channel_status,omitempty"`
	StatusReason               string             `json:"status_reason,omitempty"`
	BalanceInsufficient        bool               `json:"balance_insufficient,omitempty"`
	ScoreTotal                 float64            `json:"score_total,omitempty"`
	ScoreBreakdown             map[string]float64 `json:"score_breakdown,omitempty"`
	RoutingScoreTotal          float64            `json:"routing_score_total,omitempty"`
	RoutingScoreBreakdown      map[string]float64 `json:"routing_score_breakdown,omitempty"`
	SuccessRate                float64            `json:"success_rate,omitempty"`
	TTFTMs                     float64            `json:"ttft_ms,omitempty"`
	DurationMs                 float64            `json:"duration_ms,omitempty"`
	TokensPerSecond            float64            `json:"tokens_per_second,omitempty"`
	SampleCount                int                `json:"sample_count,omitempty"`
	ActiveConcurrency          int                `json:"active_concurrency,omitempty"`
	MaxConcurrency             int                `json:"max_concurrency,omitempty"`
	ConfiguredConcurrencyLimit int                `json:"configured_concurrency_limit,omitempty"`
	LearnedConcurrencyLimit    int                `json:"learned_concurrency_limit,omitempty"`
	EffectiveConcurrencyLimit  int                `json:"effective_concurrency_limit,omitempty"`
	QueueDepth                 int                `json:"queue_depth,omitempty"`
	QueueCapacity              int                `json:"queue_capacity,omitempty"`
	EstimatedQueueWaitMs       float64            `json:"estimated_queue_wait_ms,omitempty"`
	FirstBytePending           int                `json:"first_byte_pending,omitempty"`
	SlowFirstBytePending       int                `json:"slow_first_byte_pending,omitempty"`
	OldestFirstByteWaitMs      float64            `json:"oldest_first_byte_wait_ms,omitempty"`
	CostRatio                  float64            `json:"cost_ratio,omitempty"`
	CostReferenceRatio         float64            `json:"cost_reference_ratio,omitempty"`
	CostPricingMode            string             `json:"cost_pricing_mode,omitempty"`
	GroupPriorityRatio         float64            `json:"group_priority_ratio,omitempty"`
	SuccessScore               float64            `json:"success_score,omitempty"`
	SpeedScore                 float64            `json:"speed_score,omitempty"`
	ScoreSpeedFactor           float64            `json:"score_speed_factor,omitempty"`
	LoadScore                  float64            `json:"load_score,omitempty"`
	CostScore                  float64            `json:"cost_score,omitempty"`
	GroupScore                 float64            `json:"group_score,omitempty"`
	ExperienceScore            float64            `json:"experience_score,omitempty"`
	EmptyOutputRate            float64            `json:"empty_output_rate,omitempty"`
	ExperienceIssueRate        float64            `json:"experience_issue_rate,omitempty"`
	ConfigErrorIsolated        bool               `json:"config_error_isolated,omitempty"`
	IsolationReason            string             `json:"isolation_reason,omitempty"`
	IsolationUntil             int64              `json:"isolation_until,omitempty"`
	AuthConfigErrorCount       int                `json:"auth_config_error_count,omitempty"`
	LastAuthConfigErrorAt      int64              `json:"last_auth_config_error_at,omitempty"`
	StickyMatched              bool               `json:"sticky_matched,omitempty"`
	Selected                   bool               `json:"selected,omitempty"`
	ScoreSampleSource          string             `json:"score_sample_source,omitempty"`
	MatchedRuntimeKey          RuntimeKey         `json:"matched_runtime_key,omitempty"`
}

type ScoreWeights struct {
	Success float64
	Speed   float64
	Load    float64
	Cost    float64
	Group   float64
}

type ScoreResult struct {
	Total            float64
	Breakdown        map[string]float64
	RoutingTotal     float64
	RoutingBreakdown map[string]float64
	Reason           string
}

type CacheAffinitySignal struct {
	Key                string
	KeyFingerprint     string
	Source             string
	TTLSeconds         int
	PreferredChannelID int
	PreferredGroup     string
}

type StickyRoute struct {
	ChannelID      int
	Group          string
	Source         string
	Key            string
	KeyFingerprint string
	CacheAware     bool
	KeepScoreRatio float64
}

type AttemptResult struct {
	Key                            RuntimeKey
	RequestID                      string
	UserID                         int
	TokenID                        int
	AttemptIndex                   int
	ChannelID                      int
	ChannelName                    string
	RequestedGroup                 string
	SelectedGroup                  string
	ModelName                      string
	EndpointType                   constant.EndpointType
	Success                        bool
	StatusCode                     int
	ErrorCode                      string
	ErrorType                      string
	ErrorMessage                   string
	ErrorCategory                  string
	ObservedAt                     time.Time
	Duration                       time.Duration
	TTFT                           time.Duration
	StreamInterrupted              bool
	ClientAborted                  bool
	WillRetry                      bool
	RetryAction                    string
	ConcurrencyLimited             bool
	BalanceInsufficient            bool
	EmptyOutput                    bool
	ExperienceIssue                string
	ActiveConcurrency              int
	ConfiguredConcurrencyLimit     int
	LearnedConcurrencyLimit        int
	LearnedConcurrencyLimitChanged bool
	UsedChannels                   []string
	IsHealthProbe                  bool
	ProbeReason                    string
}

func (r AttemptResult) RuntimeKey() RuntimeKey {
	key := r.Key
	if key.ChannelID == 0 {
		key.ChannelID = r.ChannelID
	}
	if key.RequestedModel == "" {
		key.RequestedModel = r.ModelName
	}
	if key.Group == "" {
		key.Group = r.SelectedGroup
	}
	if key.Group == "" {
		key.Group = r.RequestedGroup
	}
	if key.EndpointType == "" {
		key.EndpointType = r.EndpointType
	}
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	return key
}

type DispatchRecord struct {
	Request     DispatchRequest
	Policy      GroupSmartPolicy
	Plan        *DispatchPlan
	Actual      *model.Channel
	ActualGroup string
	Shadow      bool
	RecordedAt  time.Time
}
