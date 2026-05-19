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
	CacheAffinityEnabled  bool
	QueueEnabled          bool
	CircuitBreakerEnabled bool
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
	Channel         *model.Channel
	SelectedGroup   string
	RequestedGroup  string
	RuntimeKey      RuntimeKey
	ProviderProfile string
	ProxyMode       string
	ScoreTotal      float64
	ScoreBreakdown  map[string]float64
	QueueWaitMs     int
	QueueEnabled    bool
	QueueDepth      int
	QueueCapacity   int
	SelectedReason  string
	StickySource    string
	StickyKeyFP     string
	StickyRetained  bool
	StickyBreak     string
	CacheAffinity   bool
	FallbackUsed    bool
	PolicyMode      string
	AutoMode        string
	Candidates      []CandidateExplanation
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
	Key                  RuntimeKey
	SuccessRate          float64
	TTFTMs               float64
	DurationMs           float64
	TokensPerSecond      float64
	ActiveConcurrency    int
	MaxConcurrency       int
	QueueDepth           int
	QueueCapacity        int
	QueueTimeoutMs       int
	EstimatedQueueWaitMs float64
	CostRatio            float64
	GroupPriorityRatio   float64
	CircuitState         CircuitState
	CircuitOpen          bool
	Cooldown             bool
	FailureAvoidance     bool
	SampleCount          int
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
	OpenUntil         time.Time
	HalfOpenProbeUsed int
	HalfOpenProbeMax  int
}

type Candidate struct {
	Channel         *model.Channel
	Group           string
	UpstreamModel   string
	ProviderProfile string
	ProxyMode       string
	RuntimeKey      RuntimeKey
}

type CandidateExplanation struct {
	ChannelID       int                `json:"channel_id"`
	ChannelName     string             `json:"channel_name,omitempty"`
	Group           string             `json:"group,omitempty"`
	UpstreamModel   string             `json:"upstream_model,omitempty"`
	ProviderProfile string             `json:"provider_profile,omitempty"`
	ProxyMode       string             `json:"proxy_mode,omitempty"`
	RuntimeKey      RuntimeKey         `json:"runtime_key"`
	Available       bool               `json:"available"`
	RejectReason    string             `json:"reject_reason,omitempty"`
	ScoreTotal      float64            `json:"score_total,omitempty"`
	ScoreBreakdown  map[string]float64 `json:"score_breakdown,omitempty"`
	StickyMatched   bool               `json:"sticky_matched,omitempty"`
	Selected        bool               `json:"selected,omitempty"`
}

type ScoreWeights struct {
	Success float64
	Speed   float64
	Load    float64
	Cost    float64
	Group   float64
}

type ScoreResult struct {
	Total     float64
	Breakdown map[string]float64
	Reason    string
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
	Key               RuntimeKey
	RequestID         string
	AttemptIndex      int
	ChannelID         int
	RequestedGroup    string
	SelectedGroup     string
	ModelName         string
	EndpointType      constant.EndpointType
	Success           bool
	StatusCode        int
	ErrorCode         string
	ErrorType         string
	Duration          time.Duration
	TTFT              time.Duration
	StreamInterrupted bool
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
