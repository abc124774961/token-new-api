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

const (
	DispatchRequiredToolCodexImageGeneration        = "image_generation"
	DispatchFilterConditionCodexImageGenerationTool = "codex_image_generation_tool"
)

const (
	RelayAttemptCancelReasonFirstByteTimeout          = "first_byte_timeout"
	RelayAttemptCancelReasonTotalDurationTimeout      = "total_duration_timeout"
	RelayAttemptCancelReasonTotalDurationAfterOutput  = "total_duration_timeout_after_output"
	RelayAttemptCancelReasonChannelInducedClientAbort = "channel_induced_client_abort"
)

const (
	RetryRoutingStrategyFirstByteRecovery = "first_byte_recovery"
	RetryRoutingQueuePriority             = 1000
)

const (
	ResourceTypePlatformOwned = "platform_owned"
	ResourceTypeSupplierOwned = "supplier_owned"
	ResourceTypePartnerOwned  = "partner_owned"
)

const (
	AccountTypeAPIKey        = "api_key"
	AccountTypeOAuthAccount  = "oauth_account"
	AccountTypeJSONAuth      = "json_auth"
	AccountTypeTokenKey      = "token_key"
	AccountTypeSessionCookie = "session_cookie"
	AccountTypeComposite     = "composite"
)

const (
	FailureScopeAccount  = "account"
	FailureScopeResource = "resource"
	FailureScopeProvider = "provider"
	FailureScopeSystem   = "system"
	FailureScopeClient   = "client"
)

const (
	CandidatePoolPro = "pro"
)

type ResourceRef struct {
	ResourceID         string `json:"resource_id,omitempty"`
	ResourceType       string `json:"resource_type,omitempty"`
	ExecutionBindingID int    `json:"execution_binding_id,omitempty"`
	Provider           string `json:"provider,omitempty"`
	Brand              string `json:"brand,omitempty"`
}

type AccountIdentity struct {
	AccountID                    string `json:"account_id,omitempty"`
	AccountType                  string `json:"account_type,omitempty"`
	Brand                        string `json:"brand,omitempty"`
	Provider                     string `json:"provider,omitempty"`
	CredentialIndex              int    `json:"credential_index,omitempty"`
	CredentialSubjectFingerprint string `json:"credential_subject_fingerprint,omitempty"`
	CredentialFingerprint        string `json:"credential_fingerprint,omitempty"`
	AccountIdentityKey           string `json:"account_identity_key,omitempty"`
	AccountUniqueKey             string `json:"account_unique_key,omitempty"`
	DisplayName                  string `json:"display_name,omitempty"`
	Status                       string `json:"status,omitempty"`
}

type CredentialRef struct {
	ResourceID                   string `json:"resource_id,omitempty"`
	AccountID                    string `json:"account_id,omitempty"`
	CredentialIndex              int    `json:"credential_index,omitempty"`
	CredentialSubjectFingerprint string `json:"credential_subject_fingerprint,omitempty"`
	CredentialFingerprint        string `json:"credential_fingerprint,omitempty"`
	Resolver                     string `json:"resolver,omitempty"`
}

type ProxyRef struct {
	ProxyID       int    `json:"proxy_id,omitempty"`
	Name          string `json:"name,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	MaskedAddress string `json:"masked_address,omitempty"`
}

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
	RetryRoutingIntent       *RetryRoutingIntent
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
	Channel                   *model.Channel
	ResourceRef               ResourceRef
	AccountIdentity           AccountIdentity
	CredentialRef             CredentialRef
	ProxyRef                  ProxyRef
	SelectedGroup             string
	RequestedGroup            string
	BillingRatioMode          string
	RuntimeKey                RuntimeKey
	ProviderProfile           string
	ProxyMode                 string
	ScoreTotal                float64
	ScoreBreakdown            map[string]float64
	RoutingScoreTotal         float64
	RoutingScoreBreakdown     map[string]float64
	QueueWaitMs               int
	QueueEnabled              bool
	QueueDepth                int
	QueueCapacity             int
	QueuePriority             int
	SelectedReason            string
	StickySource              string
	StickyKeyFP               string
	StickyRetained            bool
	StickyBreak               string
	StickyDecision            *StickyDecision
	CacheAffinity             bool
	FallbackUsed              bool
	PolicyMode                string
	AutoMode                  string
	Strategy                  string
	RequiresCodexImageTool    bool
	RequiredTools             []string
	CandidateFilterConditions []string
	Candidates                []CandidateExplanation
	IsHealthProbe             bool
	ProbeReason               string
	PoolLevel                 string
	SwitchReason              string
	RetryRoutingIntent        *RetryRoutingIntent
	RetryIntentApplied        bool
	RetryQueuePriorityBoost   bool
	CostGuardDecision         *CostGuardDecision
}

type RetryRoutingIntent struct {
	Reason             string `json:"reason,omitempty"`
	Strategy           string `json:"strategy,omitempty"`
	PreferLowTTFT      bool   `json:"prefer_low_ttft,omitempty"`
	PreferHighSuccess  bool   `json:"prefer_high_success,omitempty"`
	QueuePriorityBoost bool   `json:"queue_priority_boost,omitempty"`
	QueuePriority      int    `json:"queue_priority,omitempty"`
	FailedChannelID    int    `json:"failed_channel_id,omitempty"`
	FailedChannelName  string `json:"failed_channel_name,omitempty"`
	AttemptIndex       int    `json:"attempt_index,omitempty"`
}

type CostGuardDecision struct {
	Reason               string  `json:"reason,omitempty"`
	Decision             string  `json:"decision,omitempty"`
	BaselineChannelID    int     `json:"baseline_channel_id,omitempty"`
	BaselineChannelName  string  `json:"baseline_channel_name,omitempty"`
	CandidateChannelID   int     `json:"candidate_channel_id,omitempty"`
	CandidateChannelName string  `json:"candidate_channel_name,omitempty"`
	BaselineCost         float64 `json:"baseline_cost,omitempty"`
	CandidateCost        float64 `json:"candidate_cost,omitempty"`
	CostMultiple         float64 `json:"cost_multiple,omitempty"`
	CostGuardMultiple    float64 `json:"cost_guard_multiple,omitempty"`
	SuccessDelta         float64 `json:"success_delta,omitempty"`
	SuccessAdvantage     float64 `json:"success_advantage,omitempty"`
	SpeedScoreDelta      float64 `json:"speed_score_delta,omitempty"`
	SpeedAdvantage       float64 `json:"speed_advantage,omitempty"`
	BaselineRisk         bool    `json:"baseline_risk,omitempty"`
	RetryIntent          bool    `json:"retry_intent,omitempty"`
	BaselineSampleCount  int     `json:"baseline_sample_count,omitempty"`
	CandidateSampleCount int     `json:"candidate_sample_count,omitempty"`
}

func NewFirstByteTimeoutRetryRoutingIntent(channelID int, channelName string, attemptIndex int) *RetryRoutingIntent {
	return &RetryRoutingIntent{
		Reason:             RelayAttemptCancelReasonFirstByteTimeout,
		Strategy:           RetryRoutingStrategyFirstByteRecovery,
		PreferLowTTFT:      true,
		PreferHighSuccess:  true,
		QueuePriorityBoost: true,
		QueuePriority:      RetryRoutingQueuePriority,
		FailedChannelID:    channelID,
		FailedChannelName:  channelName,
		AttemptIndex:       attemptIndex,
	}
}

func NewTotalDurationTimeoutRetryRoutingIntent(channelID int, channelName string, attemptIndex int) *RetryRoutingIntent {
	return &RetryRoutingIntent{
		Reason:             RelayAttemptCancelReasonTotalDurationTimeout,
		Strategy:           RetryRoutingStrategyFirstByteRecovery,
		PreferLowTTFT:      true,
		PreferHighSuccess:  true,
		QueuePriorityBoost: true,
		QueuePriority:      RetryRoutingQueuePriority,
		FailedChannelID:    channelID,
		FailedChannelName:  channelName,
		AttemptIndex:       attemptIndex,
	}
}

func NewChannelInducedClientAbortRetryRoutingIntent(channelID int, channelName string, attemptIndex int) *RetryRoutingIntent {
	return &RetryRoutingIntent{
		Reason:             RelayAttemptCancelReasonChannelInducedClientAbort,
		Strategy:           RetryRoutingStrategyFirstByteRecovery,
		PreferLowTTFT:      true,
		PreferHighSuccess:  true,
		QueuePriorityBoost: true,
		QueuePriority:      RetryRoutingQueuePriority,
		FailedChannelID:    channelID,
		FailedChannelName:  channelName,
		AttemptIndex:       attemptIndex,
	}
}

func (i *RetryRoutingIntent) Clone() *RetryRoutingIntent {
	if i == nil {
		return nil
	}
	clone := *i
	return &clone
}

func (i *RetryRoutingIntent) Active() bool {
	return i != nil && (i.Reason != "" || i.Strategy != "")
}

func (i *RetryRoutingIntent) FirstByteRecovery() bool {
	if i == nil {
		return false
	}
	return i.Reason == RelayAttemptCancelReasonFirstByteTimeout || i.Strategy == RetryRoutingStrategyFirstByteRecovery
}

type RuntimeKey struct {
	RequestedModel        string                `json:"requested_model,omitempty"`
	UpstreamModel         string                `json:"upstream_model,omitempty"`
	ChannelID             int                   `json:"channel_id,omitempty"`
	ResourceID            string                `json:"resource_id,omitempty"`
	ResourceType          string                `json:"resource_type,omitempty"`
	AccountID             string                `json:"account_id,omitempty"`
	AccountType           string                `json:"account_type,omitempty"`
	Brand                 string                `json:"brand,omitempty"`
	Provider              string                `json:"provider,omitempty"`
	CredentialIndex       int                   `json:"credential_index,omitempty"`
	CredentialSubjectFP   string                `json:"credential_subject_fingerprint,omitempty"`
	CredentialFP          string                `json:"credential_fingerprint,omitempty"`
	Group                 string                `json:"group,omitempty"`
	EndpointType          constant.EndpointType `json:"endpoint_type,omitempty"`
	CapabilityFingerprint string                `json:"capability_fingerprint,omitempty"`
}

type RuntimeSnapshot struct {
	Key                        RuntimeKey
	MatchedRuntimeKey          RuntimeKey
	SampleSource               string
	ScoreStatsJSON             string
	RecentLatencySamples       []RuntimeLatencySample
	SuccessRate                float64
	TTFTMs                     float64
	DurationMs                 float64
	TokensPerSecond            float64
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
	ProbeRecoveryPending       bool
	ProbeRecoverySuccessCount  int
	ProbeRecoveryRequired      int
	ProbeTriggerReason         string
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
	ResourceRef            ResourceRef
	AccountIdentity        AccountIdentity
	CredentialRef          CredentialRef
	ProxyRef               ProxyRef
	Group                  string
	UpstreamModel          string
	ProviderProfile        string
	ProxyMode              string
	RequiresCodexImageTool bool
	RuntimeKey             RuntimeKey
	PoolLevel              string
}

type CandidateExplanation struct {
	ChannelID                  int                `json:"channel_id"`
	ChannelName                string             `json:"channel_name,omitempty"`
	ResourceID                 string             `json:"resource_id,omitempty"`
	ResourceType               string             `json:"resource_type,omitempty"`
	AccountID                  string             `json:"account_id,omitempty"`
	AccountType                string             `json:"account_type,omitempty"`
	Brand                      string             `json:"brand,omitempty"`
	Provider                   string             `json:"provider,omitempty"`
	CredentialIndex            int                `json:"credential_index,omitempty"`
	CredentialSubjectFP        string             `json:"credential_subject_fingerprint,omitempty"`
	CredentialFP               string             `json:"credential_fingerprint,omitempty"`
	ProxyID                    int                `json:"proxy_id,omitempty"`
	PoolLevel                  string             `json:"pool_level,omitempty"`
	SwitchReason               string             `json:"switch_reason,omitempty"`
	FailureScope               string             `json:"failure_scope,omitempty"`
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
	ScoreItems                 []ScoreItem        `json:"score_items,omitempty"`
	RoutingScoreTotal          float64            `json:"routing_score_total,omitempty"`
	RoutingScoreBreakdown      map[string]float64 `json:"routing_score_breakdown,omitempty"`
	RoutingScoreItems          []ScoreItem        `json:"routing_score_items,omitempty"`
	StateTags                  []string           `json:"state_tags,omitempty"`
	CostReferenceMissing       bool               `json:"cost_reference_missing,omitempty"`
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
	EmptyOutputRate            float64            `json:"empty_output_rate,omitempty"`
	ExperienceIssueRate        float64            `json:"experience_issue_rate,omitempty"`
	ProbeRecoveryPending       bool               `json:"probe_recovery_pending,omitempty"`
	ProbeRecoverySuccessCount  int                `json:"probe_recovery_success_count,omitempty"`
	ProbeRecoveryRequired      int                `json:"probe_recovery_required,omitempty"`
	ProbeTriggerReason         string             `json:"probe_trigger_reason,omitempty"`
	ConfigErrorIsolated        bool               `json:"config_error_isolated,omitempty"`
	IsolationReason            string             `json:"isolation_reason,omitempty"`
	IsolationUntil             int64              `json:"isolation_until,omitempty"`
	AuthConfigErrorCount       int                `json:"auth_config_error_count,omitempty"`
	LastAuthConfigErrorAt      int64              `json:"last_auth_config_error_at,omitempty"`
	StickyMatched              bool               `json:"sticky_matched,omitempty"`
	Selected                   bool               `json:"selected,omitempty"`
	CostGuardReason            string             `json:"cost_guard_reason,omitempty"`
	CostGuardDecision          string             `json:"cost_guard_decision,omitempty"`
	ScoreSampleSource          string             `json:"score_sample_source,omitempty"`
	MatchedRuntimeKey          RuntimeKey         `json:"matched_runtime_key,omitempty"`
	RetryIntentApplied         bool               `json:"retry_intent_applied,omitempty"`
	RetryIntentReason          string             `json:"retry_intent_reason,omitempty"`
}

type ScoreWeights struct {
	Success float64
	Speed   float64
	Load    float64
	Cost    float64
	Group   float64
}

type ScoreItem struct {
	Key               string             `json:"key"`
	Name              string             `json:"name,omitempty"`
	Category          string             `json:"category,omitempty"`
	RawValue          string             `json:"raw_value,omitempty"`
	RawNumber         *float64           `json:"raw_number,omitempty"`
	RawUnit           string             `json:"raw_unit,omitempty"`
	Source            string             `json:"source,omitempty"`
	ReferenceNumber   *float64           `json:"reference_number,omitempty"`
	ReferenceUnit     string             `json:"reference_unit,omitempty"`
	FormulaParameters map[string]float64 `json:"formula_parameters,omitempty"`
	Window            string             `json:"window,omitempty"`
	BaseScore         *float64           `json:"base_score,omitempty"`
	ScoreBoost        float64            `json:"score_boost,omitempty"`
	ScoreAdjusted     bool               `json:"score_adjusted,omitempty"`
	Score             float64            `json:"score"`
	Weight            float64            `json:"weight"`
	WeightedScore     float64            `json:"weighted_score"`
	PreviousScore     float64            `json:"previous_score,omitempty"`
	Delta             float64            `json:"delta,omitempty"`
	SampleCount       int                `json:"sample_count,omitempty"`
	MissingReason     string             `json:"missing_reason,omitempty"`
	Formula           string             `json:"formula,omitempty"`
	Reason            string             `json:"reason,omitempty"`
}

type ScoreResult struct {
	Total                float64
	Breakdown            map[string]float64
	Items                []ScoreItem
	RoutingTotal         float64
	RoutingBreakdown     map[string]float64
	RoutingItems         []ScoreItem
	StateTags            []string
	CostReferenceMissing bool
	Reason               string
}

type ScoreSampleDecision struct {
	ScoreSample         bool   `json:"score_sample"`
	RealUserMetric      bool   `json:"real_user_metric"`
	DynamicPriceSample  bool   `json:"dynamic_price_sample"`
	CircuitSample       bool   `json:"circuit_sample"`
	ProbeRecoverySample bool   `json:"probe_recovery_sample"`
	Reason              string `json:"reason,omitempty"`
	SkipReason          string `json:"skip_reason,omitempty"`
}

type ScoreAdjustmentItem struct {
	Key             string   `json:"key"`
	Name            string   `json:"name,omitempty"`
	BeforeScore     float64  `json:"before_score"`
	AfterScore      float64  `json:"after_score"`
	Delta           float64  `json:"delta"`
	Weight          float64  `json:"weight"`
	WeightedDelta   float64  `json:"weighted_delta"`
	BeforeRawValue  string   `json:"before_raw_value,omitempty"`
	AfterRawValue   string   `json:"after_raw_value,omitempty"`
	BeforeRawNumber *float64 `json:"before_raw_number,omitempty"`
	AfterRawNumber  *float64 `json:"after_raw_number,omitempty"`
	RawUnit         string   `json:"raw_unit,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type ScoreAdjustment struct {
	TraceID        string                `json:"trace_id,omitempty"`
	BeforeTotal    float64               `json:"before_total"`
	AfterTotal     float64               `json:"after_total"`
	Delta          float64               `json:"delta"`
	SampleDecision ScoreSampleDecision   `json:"sample_decision"`
	Items          []ScoreAdjustmentItem `json:"items,omitempty"`
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

type StickyDecision struct {
	Reason               string  `json:"reason,omitempty"`
	CandidateChannelID   int     `json:"candidate_channel_id,omitempty"`
	CandidateChannelName string  `json:"candidate_channel_name,omitempty"`
	StickyCost           float64 `json:"sticky_cost,omitempty"`
	CandidateCost        float64 `json:"candidate_cost,omitempty"`
	CostRatio            float64 `json:"cost_ratio,omitempty"`
	CostThreshold        float64 `json:"cost_threshold,omitempty"`
	StickySpeedScore     float64 `json:"sticky_speed_score,omitempty"`
	CandidateSpeedScore  float64 `json:"candidate_speed_score,omitempty"`
	SpeedScoreDelta      float64 `json:"speed_score_delta,omitempty"`
	MaxSpeedScoreDrop    float64 `json:"max_speed_score_drop,omitempty"`
	StickySuccessRate    float64 `json:"sticky_success_rate,omitempty"`
	CandidateSuccessRate float64 `json:"candidate_success_rate,omitempty"`
	SuccessSlack         float64 `json:"success_slack,omitempty"`
	StickySampleCount    int     `json:"sticky_sample_count,omitempty"`
	CandidateSampleCount int     `json:"candidate_sample_count,omitempty"`
	MinSamples           int     `json:"min_samples,omitempty"`
	CacheAware           bool    `json:"cache_aware,omitempty"`
	Decision             string  `json:"decision,omitempty"`
}

type AttemptResult struct {
	Key                            RuntimeKey
	Plan                           *DispatchPlan
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
	Strategy                       string
	AutoMode                       string
	Success                        bool
	StatusCode                     int
	ErrorCode                      string
	ErrorType                      string
	ErrorMessage                   string
	ErrorCategory                  string
	WarningLevel                   string
	WarningFlags                   []string
	WarningMessage                 string
	ChannelInducedClientAbort      bool
	ObservedAt                     time.Time
	Duration                       time.Duration
	TTFT                           time.Duration
	RequestDuration                time.Duration
	RequestTTFT                    time.Duration
	QueueWait                      time.Duration
	RelayToFirstByte               time.Duration
	RelayTotal                     time.Duration
	UpstreamResponseHeader         time.Duration
	RequestBodyPrepare             time.Duration
	RequestBodyBytes               int64
	RequestBodyStorage             string
	StreamInterrupted              bool
	ClientAborted                  bool
	WillRetry                      bool
	RetryAction                    string
	RetryReason                    string
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
	FailureScope                   string
	SwitchReason                   string
}

func (r AttemptResult) RuntimeKey() RuntimeKey {
	key := r.Key
	if r.Plan != nil {
		key = mergeRuntimeKeyFallback(key, r.Plan.RuntimeKey)
	}
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

func mergeRuntimeKeyFallback(key RuntimeKey, fallback RuntimeKey) RuntimeKey {
	if key.RequestedModel == "" {
		key.RequestedModel = fallback.RequestedModel
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = fallback.UpstreamModel
	}
	if key.ChannelID == 0 {
		key.ChannelID = fallback.ChannelID
	}
	if key.ResourceID == "" {
		key.ResourceID = fallback.ResourceID
	}
	if key.ResourceType == "" {
		key.ResourceType = fallback.ResourceType
	}
	if key.AccountID == "" {
		key.AccountID = fallback.AccountID
	}
	if key.AccountType == "" {
		key.AccountType = fallback.AccountType
	}
	if key.Brand == "" {
		key.Brand = fallback.Brand
	}
	if key.Provider == "" {
		key.Provider = fallback.Provider
	}
	if key.CredentialIndex == 0 {
		key.CredentialIndex = fallback.CredentialIndex
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = fallback.CredentialSubjectFP
	}
	if key.CredentialFP == "" {
		key.CredentialFP = fallback.CredentialFP
	}
	if key.Group == "" {
		key.Group = fallback.Group
	}
	if key.EndpointType == "" {
		key.EndpointType = fallback.EndpointType
	}
	if key.CapabilityFingerprint == "" {
		key.CapabilityFingerprint = fallback.CapabilityFingerprint
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
