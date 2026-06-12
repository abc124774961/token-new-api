package core

import (
	"context"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type SmartDispatchFacadeInterface interface {
	Select(c *gin.Context, param *service.RetryParam) (*DispatchPlan, bool, *types.NewAPIError)
	Shadow(c *gin.Context, param *service.RetryParam, actual *model.Channel, actualGroup string)
	Report(c *gin.Context, result *AttemptResult)
}

type GroupPolicyResolver interface {
	Resolve(c *gin.Context, req *DispatchRequest) GroupSmartPolicy
}

type AutoGroupResolver interface {
	Resolve(c *gin.Context, req *DispatchRequest, policy GroupSmartPolicy) AutoGroupPlan
}

type SmartChannelSelector interface {
	Select(c *gin.Context, param *service.RetryParam, policy GroupSmartPolicy) (*DispatchPlan, bool, *types.NewAPIError)
}

type LegacyChannelSelector interface {
	Select(param *service.RetryParam) (*model.Channel, string, error)
}

type ExecutionRecorder interface {
	Record(ctx context.Context, record DispatchRecord)
	Report(ctx context.Context, result AttemptResult)
}

type CircuitBreaker interface {
	Snapshot(key RuntimeKey) CircuitSnapshot
	AllowProbe(key RuntimeKey) bool
	Report(result AttemptResult)
	Reset(key RuntimeKey) bool
	ResetChannel(channelID int) int
}

type GroupPermissionService interface {
	GetUserUsableGroups(userGroup string) map[string]string
	GroupInUserUsableGroups(userGroup, groupName string) bool
	GetUserAutoGroup(userGroup string) []string
}

type SchedulerSettingsProvider interface {
	Get() SchedulerSettings
}

type SchedulerSettings struct {
	Enabled               bool
	DefaultMode           string
	RolloutPercent        int
	DefaultStrategy       string
	CacheAffinityEnabled  bool
	QueueEnabled          bool
	QueueFairness         QueueFairnessPolicySetting
	CircuitBreakerEnabled bool
	GroupPriorityRatio    map[string]float64
	GroupRevenueRatio     map[string]float64
	GroupPolicies         map[string]GroupPolicySetting
}

type QueueFairnessPolicySetting struct {
	HighPriorityGroups        []string
	HighPriorityThreshold     int
	HighPriorityExtraDepth    int
	HighPriorityReservedDepth int
	AbsoluteMaxDepth          int
}

type GroupPolicySetting struct {
	Mode                      string
	Strategy                  string
	AutoMode                  string
	CrossGroupFusion          bool
	CandidateGroups           []string
	BillingRatioMode          string
	CacheAffinityEnabled      bool
	QueueEnabled              bool
	QueueHighPriority         bool
	CircuitBreakerEnabled     bool
	ResourceProtectionEnabled bool
	PrimaryChannelIDs         []int
	PrimaryWaitTimeoutMs      int
	PrimaryQueueMaxDepth      int
	FallbackChannelIDs        []int
}

type RuntimeSnapshotStore interface {
	Get(key RuntimeKey) (RuntimeSnapshot, bool)
	Put(snapshot RuntimeSnapshot)
	ListCandidates(req *DispatchRequest) []RuntimeSnapshot
}

type CostBaselineScope struct {
	RequestedModel         string
	Group                  string
	EndpointType           string
	RequiresCodexImageTool bool
}

type CostBaselineProvider interface {
	Baseline(scope CostBaselineScope) (float64, bool)
}

type RuntimeSnapshotEnricher interface {
	Enrich(candidate Candidate, snapshot RuntimeSnapshot, policy GroupSmartPolicy) RuntimeSnapshot
	ReserveCircuitProbe(key RuntimeKey) bool
}

type CandidatePoolBuilder interface {
	Build(req *DispatchRequest, policy GroupSmartPolicy) []Candidate
}

type CacheAffinitySignalAdapter interface {
	Extract(c *gin.Context, req *DispatchRequest, policy GroupSmartPolicy) (CacheAffinitySignal, bool)
}

type StickyRouter interface {
	Route(c *gin.Context, req *DispatchRequest, policy GroupSmartPolicy) (StickyRoute, bool)
	Save(c *gin.Context, req *DispatchRequest, plan *DispatchPlan)
	Clear(c *gin.Context, req *DispatchRequest, policy GroupSmartPolicy)
	Report(c *gin.Context, req *DispatchRequest, plan *DispatchPlan, result AttemptResult)
}
