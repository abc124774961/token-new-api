package probe

import (
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSource          = "model_gateway_probe"
	TokenName              = "系统健康探活"
	ConsumeLogContent      = "模型网关健康探活"
	probeIDPrefix          = "mg_probe_"
	reasonNoSamples        = "missing_samples"
	reasonLowScore         = "low_score"
	reasonLongNoSuccess    = "long_no_success"
	reasonCircuitProbe     = "circuit_half_open"
	reasonFailureAvoidance = "failure_avoidance"
	reasonCooldown         = "cooldown"
	reasonSampling         = "sampling"
	reasonLowTraffic       = "low_traffic"
)

type ProbeConfig struct {
	Enabled                         bool
	Interval                        time.Duration
	WorkerCount                     int
	Timeout                         time.Duration
	MaxPerTick                      int
	MinChannelInterval              time.Duration
	LowScoreThreshold               float64
	MissingSampleThreshold          int
	LongNoSuccessThreshold          time.Duration
	RecoverySuccessesRequired       int
	FailureAvoidancePriorityEnabled bool
	HighScoreSamplingInterval       time.Duration
}

type ProbeCandidate struct {
	Channel *model.Channel
	Model   string
	Group   string
	Key     core.RuntimeKey
	Reason  string
	Score   float64
	Plan    *core.DispatchPlan
}

type ProbeRunResult struct {
	ProbeID     string
	Reason      string
	Channel     *model.Channel
	Model       string
	Group       string
	RuntimeKey  core.RuntimeKey
	TargetKey   core.RuntimeKey
	Context     *gin.Context
	RelayInfo   *relaycommon.RelayInfo
	Usage       *dto.Usage
	PriceData   types.PriceData
	Quota       int
	Success     bool
	StatusCode  int
	NewAPIError *types.NewAPIError
	Err         error
	StartedAt   time.Time
	Duration    time.Duration
	TTFT        time.Duration
	Plan        *core.DispatchPlan
}
