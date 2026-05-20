package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	modelgatewayscheduler "github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	modelGatewayObservabilityDefaultHours       = 24
	modelGatewayObservabilityMaxHours           = 24 * 30
	modelGatewayObservabilityDefaultRecentLimit = 50
	modelGatewayObservabilityMaxRecentLimit     = 200
	modelGatewayObservabilityDefaultTopN        = 10
	modelGatewayObservabilityMaxTopN            = 50
	modelGatewayObservabilityDefaultScanLimit   = 5000
	modelGatewayObservabilityMaxScanLimit       = 20000
	modelGatewayObservabilityMinTrendBucket     = 60
	modelGatewayObservabilityMaxTrendBucket     = 24 * 3600
	modelGatewayObservabilityMaxTrendBuckets    = 360
	modelGatewayObservabilityTrendTopN          = 5
	modelGatewayObservabilityRiskTimelineLimit  = 100
	modelGatewayTrendExportPreviewLimit         = 20
	modelGatewayRuntimeStatusDefaultLimit       = 200
	modelGatewayRuntimeStatusMaxLimit           = 1000
	modelGatewayObservabilitySummaryFreshTTL    = 3 * time.Second
	modelGatewayObservabilitySummaryStaleTTL    = 30 * time.Second
	modelGatewayObservabilitySummaryMaxCache    = 128
)

type ModelGatewayObservabilityResponse struct {
	Summary           ModelGatewayObservabilitySummary                `json:"summary"`
	Trends            []ModelGatewayObservabilityTrendPoint           `json:"trends"`
	Risk              ModelGatewayRiskSnapshot                        `json:"risk"`
	RiskTimeline      []ModelGatewayRiskEvent                         `json:"risk_timeline"`
	RiskEvents        []ModelGatewayRiskEvent                         `json:"risk_events"`
	ByModel           []ModelGatewayObservabilityAggregate            `json:"by_model"`
	ByGroup           []ModelGatewayObservabilityAggregate            `json:"by_group"`
	ByChannel         []ModelGatewayObservabilityAggregate            `json:"by_channel"`
	ByProviderProfile []ModelGatewayObservabilityAggregate            `json:"by_provider_profile"`
	ByProxyMode       []ModelGatewayObservabilityAggregate            `json:"by_proxy_mode"`
	RecentRecords     []ModelGatewayObservabilityRecord               `json:"recent_records"`
	ScoreBreakdown    ModelGatewayObservabilityScoreBreakdown         `json:"score_breakdown"`
	RuntimeStatus     modelgatewayobservability.RuntimeStatusResponse `json:"runtime_status"`
}

type ModelGatewayStickyStoreResponse struct {
	Items []ModelGatewayStickyStoreItem `json:"items"`
	Total int                           `json:"total"`
}

type ModelGatewayStickyStoreItem struct {
	KeyID          string `json:"key_id"`
	ChannelID      int    `json:"channel_id"`
	Group          string `json:"group,omitempty"`
	KeyFingerprint string `json:"key_fingerprint,omitempty"`
	ExpiresAt      int64  `json:"expires_at,omitempty"`
	TTLSeconds     int64  `json:"ttl_seconds,omitempty"`
}

type ModelGatewayStickyClearResponse struct {
	KeyID     string `json:"key_id,omitempty"`
	Group     string `json:"group,omitempty"`
	ChannelID int    `json:"channel_id,omitempty"`
	Cleared   bool   `json:"cleared"`
	Deleted   int    `json:"deleted"`
}

type ModelGatewayTrendExport struct {
	Kind      string                                `json:"kind"`
	Filters   ModelGatewayTrendExportFilters        `json:"filters"`
	Summary   ModelGatewayObservabilitySummary      `json:"summary"`
	Trends    []ModelGatewayObservabilityTrendPoint `json:"trends"`
	Preview   ModelGatewayTrendExportPreview        `json:"preview"`
	Generated int64                                 `json:"generated"`
}

type ModelGatewayTrendExportFilters struct {
	Hours              int    `json:"hours"`
	TrendBucketSeconds int64  `json:"trend_bucket_seconds"`
	Model              string `json:"model,omitempty"`
	Group              string `json:"group,omitempty"`
	ChannelID          int    `json:"channel_id,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
}

type ModelGatewayTrendExportPreview struct {
	BucketCount            int                            `json:"bucket_count"`
	NonEmptyBuckets        int                            `json:"non_empty_buckets"`
	RiskEvents             int64                          `json:"risk_events"`
	RiskEventCount         int64                          `json:"risk_event_count"`
	RiskStatusChanges      int64                          `json:"risk_status_changes"`
	CurrentRiskRuntimeKeys int                            `json:"current_risk_runtime_keys"`
	Risk                   ModelGatewayRiskSnapshot       `json:"risk"`
	RiskTimeline           []ModelGatewayRiskEvent        `json:"risk_timeline,omitempty"`
	TopRiskStatuses        []ModelGatewayRiskStatusCount  `json:"top_risk_statuses,omitempty"`
	TopRejectReasons       []ModelGatewayTrendReasonCount `json:"top_reject_reasons,omitempty"`
	TopCircuitOpenReasons  []ModelGatewayTrendReasonCount `json:"top_circuit_open_reasons,omitempty"`
	TopCircuitErrorTypes   []ModelGatewayTrendReasonCount `json:"top_circuit_error_types,omitempty"`
}

type ModelGatewayObservabilitySummary struct {
	WindowHours             int                            `json:"window_hours"`
	TrendBucketSeconds      int64                          `json:"trend_bucket_seconds"`
	StartTime               int64                          `json:"start_time"`
	EndTime                 int64                          `json:"end_time"`
	TotalRecords            int64                          `json:"total_records"`
	ScannedRecords          int                            `json:"scanned_records"`
	Truncated               bool                           `json:"truncated"`
	Dispatches              int64                          `json:"dispatches"`
	Attempts                int64                          `json:"attempts"`
	Successes               int64                          `json:"successes"`
	Failures                int64                          `json:"failures"`
	StreamInterrupted       int64                          `json:"stream_interrupted"`
	FallbackUsed            int64                          `json:"fallback_used"`
	SuccessRate             float64                        `json:"success_rate"`
	AvgDurationMs           int64                          `json:"avg_duration_ms"`
	AvgTTFTMs               int64                          `json:"avg_ttft_ms"`
	AvgScoreTotal           float64                        `json:"avg_score_total"`
	QueueEnabledDispatches  int64                          `json:"queue_enabled_dispatches"`
	QueuedDispatches        int64                          `json:"queued_dispatches"`
	AvgQueueWaitMs          int64                          `json:"avg_queue_wait_ms"`
	StickyRoutes            int64                          `json:"sticky_routes"`
	StickyRetained          int64                          `json:"sticky_retained"`
	StickyBroken            int64                          `json:"sticky_broken"`
	CacheAffinityRoutes     int64                          `json:"cache_affinity_routes"`
	ScoreBreakdownSamples   int64                          `json:"score_breakdown_samples"`
	ScoreBreakdownParseErrs int64                          `json:"score_breakdown_parse_errors"`
	RiskEvents              int64                          `json:"risk_events"`
	RiskStatusChanges       int64                          `json:"risk_status_changes"`
	CurrentRiskRuntimeKeys  int                            `json:"current_risk_runtime_keys"`
	CircuitOpenReasons      []ModelGatewayTrendReasonCount `json:"circuit_open_reasons,omitempty"`
	CircuitErrorCounts      []ModelGatewayTrendReasonCount `json:"circuit_error_counts,omitempty"`
	CircuitErrorTypes       []ModelGatewayTrendReasonCount `json:"circuit_error_types,omitempty"`
}

type ModelGatewayObservabilityTrendPoint struct {
	BucketStart            int64                                `json:"bucket_start"`
	BucketEnd              int64                                `json:"bucket_end"`
	Records                int64                                `json:"records"`
	Dispatches             int64                                `json:"dispatches"`
	Attempts               int64                                `json:"attempts"`
	Successes              int64                                `json:"successes"`
	Failures               int64                                `json:"failures"`
	StreamInterrupted      int64                                `json:"stream_interrupted"`
	FallbackUsed           int64                                `json:"fallback_used"`
	SuccessRate            float64                              `json:"success_rate"`
	AvgDurationMs          int64                                `json:"avg_duration_ms"`
	AvgTTFTMs              int64                                `json:"avg_ttft_ms"`
	QueueEnabledDispatches int64                                `json:"queue_enabled_dispatches"`
	QueuedDispatches       int64                                `json:"queued_dispatches"`
	AvgQueueWaitMs         int64                                `json:"avg_queue_wait_ms"`
	QueueWaitP50Ms         int64                                `json:"queue_wait_p50_ms"`
	QueueWaitP90Ms         int64                                `json:"queue_wait_p90_ms"`
	QueueWaitP95Ms         int64                                `json:"queue_wait_p95_ms"`
	StickyRoutes           int64                                `json:"sticky_routes"`
	StickyRetained         int64                                `json:"sticky_retained"`
	StickyBroken           int64                                `json:"sticky_broken"`
	CacheAffinityRoutes    int64                                `json:"cache_affinity_routes"`
	ByProviderProfile      []ModelGatewayObservabilityAggregate `json:"by_provider_profile,omitempty"`
	ByProxyMode            []ModelGatewayObservabilityAggregate `json:"by_proxy_mode,omitempty"`
	RejectReasons          []ModelGatewayTrendReasonCount       `json:"reject_reasons,omitempty"`
	CircuitOpenReasons     []ModelGatewayTrendReasonCount       `json:"circuit_open_reasons,omitempty"`
	CircuitErrorCounts     []ModelGatewayTrendReasonCount       `json:"circuit_error_counts,omitempty"`
	CircuitErrorTypes      []ModelGatewayTrendReasonCount       `json:"circuit_error_types,omitempty"`
	Risk                   *ModelGatewayRiskSnapshot            `json:"risk,omitempty"`
	RiskEvents             []ModelGatewayRiskEvent              `json:"risk_events,omitempty"`
}

type ModelGatewayTrendReasonCount struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type ModelGatewayRiskStatusCount struct {
	Status   string `json:"status"`
	Severity string `json:"severity,omitempty"`
	Count    int64  `json:"count"`
}

type ModelGatewayRiskEvent struct {
	Timestamp       int64  `json:"timestamp,omitempty"`
	BucketStart     int64  `json:"bucket_start,omitempty"`
	BucketEnd       int64  `json:"bucket_end,omitempty"`
	Source          string `json:"source"`
	EventType       string `json:"event_type"`
	Severity        string `json:"severity"`
	Status          string `json:"status"`
	PreviousStatus  string `json:"previous_status,omitempty"`
	CurrentStatus   string `json:"current_status,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Count           int64  `json:"count"`
	RequestedModel  string `json:"requested_model,omitempty"`
	UpstreamModel   string `json:"upstream_model,omitempty"`
	Group           string `json:"group,omitempty"`
	ChannelID       int    `json:"channel_id,omitempty"`
	EndpointType    string `json:"endpoint_type,omitempty"`
	ProviderProfile string `json:"provider_profile,omitempty"`
	ProxyMode       string `json:"proxy_mode,omitempty"`
}

type ModelGatewayRiskSnapshot struct {
	EventCount             int64                          `json:"event_count"`
	RiskEventCount         int64                          `json:"risk_event_count"`
	StatusChanges          int64                          `json:"status_changes"`
	RiskStatusChanges      int64                          `json:"risk_status_changes"`
	CurrentRuntimeKeys     int                            `json:"current_runtime_keys"`
	CurrentRiskRuntimeKeys int                            `json:"current_risk_runtime_keys"`
	Events                 []ModelGatewayRiskEvent        `json:"events"`
	RiskEvents             []ModelGatewayRiskEvent        `json:"risk_events"`
	Timeline               []ModelGatewayRiskEvent        `json:"timeline"`
	RiskTimeline           []ModelGatewayRiskEvent        `json:"risk_timeline"`
	TopStatuses            []ModelGatewayRiskStatusCount  `json:"top_statuses,omitempty"`
	TopRiskStatuses        []ModelGatewayRiskStatusCount  `json:"top_risk_statuses,omitempty"`
	TopRejectReasons       []ModelGatewayTrendReasonCount `json:"top_reject_reasons,omitempty"`
	TopCircuitOpenReasons  []ModelGatewayTrendReasonCount `json:"top_circuit_open_reasons,omitempty"`
	TopCircuitErrorTypes   []ModelGatewayTrendReasonCount `json:"top_circuit_error_types,omitempty"`
}

type ModelGatewayObservabilityAggregate struct {
	Key                     string             `json:"key"`
	Name                    string             `json:"name,omitempty"`
	ChannelID               int                `json:"channel_id,omitempty"`
	Records                 int64              `json:"records"`
	Dispatches              int64              `json:"dispatches"`
	Attempts                int64              `json:"attempts"`
	Successes               int64              `json:"successes"`
	Failures                int64              `json:"failures"`
	StreamInterrupted       int64              `json:"stream_interrupted"`
	FallbackUsed            int64              `json:"fallback_used"`
	SuccessRate             float64            `json:"success_rate"`
	AvgDurationMs           int64              `json:"avg_duration_ms"`
	AvgTTFTMs               int64              `json:"avg_ttft_ms"`
	AvgScoreTotal           float64            `json:"avg_score_total"`
	QueueEnabledDispatches  int64              `json:"queue_enabled_dispatches"`
	QueuedDispatches        int64              `json:"queued_dispatches"`
	AvgQueueWaitMs          int64              `json:"avg_queue_wait_ms"`
	StickyRoutes            int64              `json:"sticky_routes"`
	StickyRetained          int64              `json:"sticky_retained"`
	StickyBroken            int64              `json:"sticky_broken"`
	CacheAffinityRoutes     int64              `json:"cache_affinity_routes"`
	ScoreBreakdown          map[string]float64 `json:"score_breakdown,omitempty"`
	ScoreBreakdownSamples   int64              `json:"score_breakdown_samples"`
	ScoreBreakdownParseErrs int64              `json:"score_breakdown_parse_errors"`
	LastRecordAt            int64              `json:"last_record_at"`
}

type ModelGatewayObservabilityScoreBreakdown struct {
	Samples     int64              `json:"samples"`
	Average     map[string]float64 `json:"average"`
	ParseErrors int64              `json:"parse_errors"`
}

type ModelGatewayObservabilityRecord struct {
	ID                    int                                `json:"id"`
	Kind                  string                             `json:"kind"`
	CreatedAt             int64                              `json:"created_at"`
	RequestID             string                             `json:"request_id"`
	AttemptIndex          int                                `json:"attempt_index"`
	RequestedGroup        string                             `json:"requested_group"`
	SelectedGroup         string                             `json:"selected_group"`
	ActualGroup           string                             `json:"actual_group,omitempty"`
	RequestedModel        string                             `json:"requested_model"`
	ChannelID             int                                `json:"channel_id"`
	ChannelName           string                             `json:"channel_name,omitempty"`
	ActualChannelID       int                                `json:"actual_channel_id,omitempty"`
	ActualChannelName     string                             `json:"actual_channel_name,omitempty"`
	EndpointType          string                             `json:"endpoint_type,omitempty"`
	PolicyMode            string                             `json:"policy_mode,omitempty"`
	AutoMode              string                             `json:"auto_mode,omitempty"`
	Strategy              string                             `json:"strategy,omitempty"`
	Shadow                bool                               `json:"shadow"`
	SmartHandled          bool                               `json:"smart_handled"`
	FallbackUsed          bool                               `json:"fallback_used"`
	Success               bool                               `json:"success"`
	StatusCode            int                                `json:"status_code,omitempty"`
	ErrorCode             string                             `json:"error_code,omitempty"`
	ErrorType             string                             `json:"error_type,omitempty"`
	DurationMs            int64                              `json:"duration_ms,omitempty"`
	TTFTMs                int64                              `json:"ttft_ms,omitempty"`
	StreamInterrupted     bool                               `json:"stream_interrupted"`
	ScoreTotal            float64                            `json:"score_total,omitempty"`
	ScoreBreakdown        map[string]float64                 `json:"score_breakdown,omitempty"`
	CandidateGroups       []string                           `json:"candidate_groups,omitempty"`
	CandidateExplanations []ModelGatewayCandidateExplanation `json:"candidate_explanations,omitempty"`
	SelectedReason        string                             `json:"selected_reason,omitempty"`
	QueueEnabled          bool                               `json:"queue_enabled,omitempty"`
	QueueWaitMs           int64                              `json:"queue_wait_ms,omitempty"`
	QueueDepth            int                                `json:"queue_depth,omitempty"`
	QueueCapacity         int                                `json:"queue_capacity,omitempty"`
	StickySource          string                             `json:"sticky_source,omitempty"`
	StickyRetained        bool                               `json:"sticky_retained,omitempty"`
	StickyBreak           string                             `json:"sticky_break,omitempty"`
	CacheAffinity         bool                               `json:"cache_affinity,omitempty"`
	RequestMeta           map[string]any                     `json:"request_meta,omitempty"`
	ScoreBreakdownError   bool                               `json:"score_breakdown_error,omitempty"`
	CandidateGroupsError  bool                               `json:"candidate_groups_error,omitempty"`
	RequestMetaError      bool                               `json:"request_meta_error,omitempty"`
}

type ModelGatewayCandidateExplanation struct {
	ChannelID       int                    `json:"channel_id"`
	ChannelName     string                 `json:"channel_name,omitempty"`
	Group           string                 `json:"group,omitempty"`
	UpstreamModel   string                 `json:"upstream_model,omitempty"`
	ProviderProfile string                 `json:"provider_profile,omitempty"`
	ProxyMode       string                 `json:"proxy_mode,omitempty"`
	RuntimeKey      ModelGatewayRuntimeKey `json:"runtime_key"`
	Available       bool                   `json:"available"`
	RejectReason    string                 `json:"reject_reason,omitempty"`
	ScoreTotal      float64                `json:"score_total,omitempty"`
	ScoreBreakdown  map[string]float64     `json:"score_breakdown,omitempty"`
	StickyMatched   bool                   `json:"sticky_matched,omitempty"`
	Selected        bool                   `json:"selected,omitempty"`
}

type ModelGatewayRuntimeKey struct {
	RequestedModel        string `json:"requested_model,omitempty"`
	UpstreamModel         string `json:"upstream_model,omitempty"`
	ChannelID             int    `json:"channel_id,omitempty"`
	Group                 string `json:"group,omitempty"`
	EndpointType          string `json:"endpoint_type,omitempty"`
	CapabilityFingerprint string `json:"capability_fingerprint,omitempty"`
}

type ModelGatewayObservabilityOptions struct {
	Hours              int
	RecentLimit        int
	TopN               int
	ScanLimit          int
	TrendBucketSeconds int64
	Model              string
	Group              string
	ChannelID          int
	RequestID          string
	IncludeTotal       bool
}

type modelGatewayObservabilityAccumulator struct {
	ModelGatewayObservabilityAggregate
	durationSum       int64
	durationSamples   int64
	ttftSum           int64
	ttftSamples       int64
	scoreTotalSum     float64
	scoreTotalSamples int64
	queueWaitSum      int64
	queueWaitSamples  int64
	queueWaitValues   []int64
	scoreSums         map[string]float64
}

type modelGatewayObservabilityTrendAccumulator struct {
	*modelGatewayObservabilityAccumulator
	ProviderProfileAccumulators map[string]*modelGatewayObservabilityAccumulator
	ProxyModeAccumulators       map[string]*modelGatewayObservabilityAccumulator
	RejectReasons               map[string]int64
	CircuitOpenReasons          map[string]int64
	CircuitErrorTypes           map[string]int64
}

type modelGatewayObservabilitySummaryCacheEntry struct {
	options    ModelGatewayObservabilityOptions
	response   ModelGatewayObservabilityResponse
	createdAt  time.Time
	expiresAt  time.Time
	staleUntil time.Time
}

type modelGatewayObservabilitySummaryCacheStore struct {
	mu      sync.Mutex
	group   singleflight.Group
	entries map[string]modelGatewayObservabilitySummaryCacheEntry
}

var modelGatewayObservabilitySummaryCache = &modelGatewayObservabilitySummaryCacheStore{
	entries: make(map[string]modelGatewayObservabilitySummaryCacheEntry),
}

func resetModelGatewayObservabilitySummaryCache() {
	modelGatewayObservabilitySummaryCache = &modelGatewayObservabilitySummaryCacheStore{
		entries: make(map[string]modelGatewayObservabilitySummaryCacheEntry),
	}
}

func GetModelGatewayObservabilitySummary(c *gin.Context) {
	options, err := parseModelGatewayObservabilityOptions(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	response, err := BuildModelGatewayObservabilitySummary(options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func ExportModelGatewayObservabilityTrends(c *gin.Context) {
	options, err := parseModelGatewayObservabilityOptions(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	options.IncludeTotal = true
	response, err := BuildModelGatewayObservabilitySummary(options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	export := ModelGatewayTrendExport{
		Kind: "modelgateway_observability_trends",
		Filters: ModelGatewayTrendExportFilters{
			Hours:              response.Summary.WindowHours,
			TrendBucketSeconds: response.Summary.TrendBucketSeconds,
			Model:              options.Model,
			Group:              options.Group,
			ChannelID:          options.ChannelID,
			RequestID:          options.RequestID,
		},
		Summary:   response.Summary,
		Trends:    response.Trends,
		Preview:   buildModelGatewayTrendExportPreview(response),
		Generated: common.GetTimestamp(),
	}

	if c.Query("download") == "true" {
		data, err := common.Marshal(export)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filename := fmt.Sprintf("modelgateway-trends-%dh-%ds.json", export.Filters.Hours, export.Filters.TrendBucketSeconds)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/json; charset=utf-8", append(data, '\n'))
		return
	}

	common.ApiSuccess(c, export)
}

func GetModelGatewayRuntimeStatus(c *gin.Context) {
	query, err := parseModelGatewayRuntimeStatusQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, defaultModelGatewayRuntimeStatusService().Build(query))
}

func GetModelGatewayStickyStore(c *gin.Context) {
	limit := normalizeModelGatewayObservabilityInt(c.Query("limit"), 100, 1, 500)
	items := modelGatewayStickyStoreItems(limit)
	common.ApiSuccess(c, ModelGatewayStickyStoreResponse{
		Items: items,
		Total: len(items),
	})
}

func ClearModelGatewayStickyStoreEntry(c *gin.Context) {
	keyID := strings.TrimSpace(c.Param("key_id"))
	if keyID == "" {
		common.ApiErrorMsg(c, "missing sticky key id")
		return
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	deleted := 0
	if runtimeDeps != nil && runtimeDeps.StickyRouter != nil && runtimeDeps.StickyRouter.ClearStickyEntryByID(keyID) {
		deleted = 1
	}
	common.ApiSuccess(c, ModelGatewayStickyClearResponse{
		KeyID:   keyID,
		Cleared: deleted > 0,
		Deleted: deleted,
	})
}

func ClearModelGatewayStickyStore(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			common.ApiErrorMsg(c, "invalid channel_id")
			return
		}
		channelID = parsed
	}
	if group == "" && channelID <= 0 {
		common.ApiErrorMsg(c, "missing sticky clear filter")
		return
	}
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	deleted := 0
	if runtimeDeps != nil && runtimeDeps.StickyRouter != nil {
		deleted = runtimeDeps.StickyRouter.ClearStickyEntries(modelgatewayscheduler.StickyClearFilter{
			Group:     group,
			ChannelID: channelID,
		})
	}
	common.ApiSuccess(c, ModelGatewayStickyClearResponse{
		Group:     group,
		ChannelID: channelID,
		Cleared:   deleted > 0,
		Deleted:   deleted,
	})
}

func parseModelGatewayObservabilityOptions(c *gin.Context) (ModelGatewayObservabilityOptions, error) {
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return ModelGatewayObservabilityOptions{}, errors.New("invalid channel_id")
		}
		channelID = parsed
	}
	trendBucketSeconds, err := parseModelGatewayTrendBucketSeconds(c.Query("trend_bucket_seconds"))
	if err != nil {
		return ModelGatewayObservabilityOptions{}, err
	}

	return ModelGatewayObservabilityOptions{
		Hours:              normalizeModelGatewayObservabilityInt(c.Query("hours"), modelGatewayObservabilityDefaultHours, 1, modelGatewayObservabilityMaxHours),
		RecentLimit:        normalizeModelGatewayObservabilityInt(c.Query("recent_limit"), modelGatewayObservabilityDefaultRecentLimit, 0, modelGatewayObservabilityMaxRecentLimit),
		TopN:               normalizeModelGatewayObservabilityInt(c.Query("top_n"), modelGatewayObservabilityDefaultTopN, 1, modelGatewayObservabilityMaxTopN),
		ScanLimit:          normalizeModelGatewayObservabilityInt(c.Query("scan_limit"), modelGatewayObservabilityDefaultScanLimit, 1, modelGatewayObservabilityMaxScanLimit),
		TrendBucketSeconds: trendBucketSeconds,
		Model:              strings.TrimSpace(c.Query("model")),
		Group:              strings.TrimSpace(c.Query("group")),
		ChannelID:          channelID,
		RequestID:          strings.TrimSpace(c.Query("request_id")),
		IncludeTotal:       parseModelGatewayObservabilityBool(c.Query("include_total")),
	}, nil
}

func parseModelGatewayRuntimeStatusQuery(c *gin.Context) (modelgatewayobservability.RuntimeStatusQuery, error) {
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return modelgatewayobservability.RuntimeStatusQuery{}, errors.New("invalid channel_id")
		}
		channelID = parsed
	}
	return modelgatewayobservability.RuntimeStatusQuery{
		Model:     strings.TrimSpace(c.Query("model")),
		Group:     strings.TrimSpace(c.Query("group")),
		ChannelID: channelID,
		Limit:     normalizeModelGatewayObservabilityInt(c.Query("limit"), modelGatewayRuntimeStatusDefaultLimit, 1, modelGatewayRuntimeStatusMaxLimit),
	}, nil
}

func modelGatewayStickyStoreItems(limit int) []ModelGatewayStickyStoreItem {
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	if runtimeDeps == nil || runtimeDeps.StickyRouter == nil {
		return []ModelGatewayStickyStoreItem{}
	}
	entries := runtimeDeps.StickyRouter.StickyEntries()
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	items := make([]ModelGatewayStickyStoreItem, 0, len(entries))
	now := time.Now()
	for _, entry := range entries {
		items = append(items, modelGatewayStickyStoreItem(entry, now))
	}
	return items
}

func modelGatewayStickyStoreItem(entry modelgatewayscheduler.StickyStoreEntry, now time.Time) ModelGatewayStickyStoreItem {
	item := ModelGatewayStickyStoreItem{
		KeyID:          entry.KeyID,
		ChannelID:      entry.ChannelID,
		Group:          entry.Group,
		KeyFingerprint: entry.KeyFingerprint,
	}
	if !entry.ExpiresAt.IsZero() {
		item.ExpiresAt = entry.ExpiresAt.Unix()
		ttl := int64(time.Until(entry.ExpiresAt).Seconds())
		if !now.IsZero() {
			ttl = int64(entry.ExpiresAt.Sub(now).Seconds())
		}
		if ttl < 0 {
			ttl = 0
		}
		item.TTLSeconds = ttl
	}
	return item
}

func normalizeModelGatewayObservabilityInt(raw string, fallback int, minValue int, maxValue int) int {
	parsed := fallback
	if strings.TrimSpace(raw) != "" {
		if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			parsed = value
		}
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func parseModelGatewayTrendBucketSeconds(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "auto") {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid trend_bucket_seconds")
	}
	return parsed, nil
}

func parseModelGatewayObservabilityBool(raw string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && parsed
}

func normalizeModelGatewayObservabilityOptions(options ModelGatewayObservabilityOptions) ModelGatewayObservabilityOptions {
	options.Hours = clampModelGatewayObservabilityValue(options.Hours, 1, modelGatewayObservabilityMaxHours)
	options.RecentLimit = clampModelGatewayObservabilityValue(options.RecentLimit, 0, modelGatewayObservabilityMaxRecentLimit)
	options.TopN = clampModelGatewayObservabilityValue(options.TopN, 1, modelGatewayObservabilityMaxTopN)
	options.ScanLimit = normalizeModelGatewayObservabilityScanLimit(options.ScanLimit)
	options.TrendBucketSeconds = modelGatewayObservabilityTrendBucketSeconds(options.Hours, options.TrendBucketSeconds)
	options.Model = strings.TrimSpace(options.Model)
	options.Group = strings.TrimSpace(options.Group)
	options.RequestID = strings.TrimSpace(options.RequestID)
	if options.ChannelID < 0 {
		options.ChannelID = 0
	}
	return options
}

func modelGatewayObservabilitySummaryCacheKey(options ModelGatewayObservabilityOptions) string {
	options = normalizeModelGatewayObservabilityOptions(options)
	values := url.Values{}
	values.Set("hours", strconv.Itoa(options.Hours))
	values.Set("recent_limit", strconv.Itoa(options.RecentLimit))
	values.Set("top_n", strconv.Itoa(options.TopN))
	values.Set("scan_limit", strconv.Itoa(options.ScanLimit))
	values.Set("trend_bucket_seconds", strconv.FormatInt(options.TrendBucketSeconds, 10))
	values.Set("model", options.Model)
	values.Set("group", options.Group)
	values.Set("channel_id", strconv.Itoa(options.ChannelID))
	values.Set("request_id", options.RequestID)
	values.Set("include_total", strconv.FormatBool(options.IncludeTotal))
	return values.Encode()
}

func refreshModelGatewayObservabilitySummaryCache(key string, options ModelGatewayObservabilityOptions) {
	_, _, _ = modelGatewayObservabilitySummaryCache.group.Do(key, func() (any, error) {
		response, err := buildModelGatewayObservabilitySummaryUncached(options)
		if err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
		modelGatewayObservabilitySummaryCache.store(key, options, response, time.Now())
		return response, nil
	})
}

func (store *modelGatewayObservabilitySummaryCacheStore) lookupFresh(key string, now time.Time) (ModelGatewayObservabilityResponse, bool) {
	if store == nil {
		return ModelGatewayObservabilityResponse{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[key]
	if !ok || now.After(entry.expiresAt) {
		return ModelGatewayObservabilityResponse{}, false
	}
	return entry.response, true
}

func (store *modelGatewayObservabilitySummaryCacheStore) lookupStale(key string, now time.Time) (ModelGatewayObservabilityResponse, bool) {
	if store == nil {
		return ModelGatewayObservabilityResponse{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[key]
	if !ok || now.After(entry.staleUntil) {
		return ModelGatewayObservabilityResponse{}, false
	}
	return entry.response, true
}

func (store *modelGatewayObservabilitySummaryCacheStore) store(key string, options ModelGatewayObservabilityOptions, response ModelGatewayObservabilityResponse, now time.Time) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.entries == nil {
		store.entries = make(map[string]modelGatewayObservabilitySummaryCacheEntry)
	}
	store.entries[key] = modelGatewayObservabilitySummaryCacheEntry{
		options:    options,
		response:   response,
		createdAt:  now,
		expiresAt:  now.Add(modelGatewayObservabilitySummaryFreshTTL),
		staleUntil: now.Add(modelGatewayObservabilitySummaryStaleTTL),
	}
	store.pruneLocked(now)
}

func (store *modelGatewayObservabilitySummaryCacheStore) pruneLocked(now time.Time) {
	if len(store.entries) <= modelGatewayObservabilitySummaryMaxCache {
		return
	}
	for key, entry := range store.entries {
		if now.After(entry.staleUntil) {
			delete(store.entries, key)
		}
	}
	for len(store.entries) > modelGatewayObservabilitySummaryMaxCache {
		var oldestKey string
		var oldestTime time.Time
		for key, entry := range store.entries {
			if oldestKey == "" || entry.createdAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.createdAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(store.entries, oldestKey)
	}
}

func InvalidateModelGatewayObservabilitySummaryCacheForRecord(record model.ModelExecutionRecord) {
	if modelGatewayObservabilitySummaryCache == nil {
		return
	}
	modelGatewayObservabilitySummaryCache.invalidateForRecord(record)
}

func (store *modelGatewayObservabilitySummaryCacheStore) invalidateForRecord(record model.ModelExecutionRecord) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for key, entry := range store.entries {
		if entry.options.matchesRecord(record) {
			delete(store.entries, key)
		}
	}
}

func (options ModelGatewayObservabilityOptions) matchesRecord(record model.ModelExecutionRecord) bool {
	options = normalizeModelGatewayObservabilityOptions(options)
	if options.Model != "" && options.Model != record.RequestedModel {
		return false
	}
	if options.Group != "" && options.Group != record.RequestedGroup && options.Group != record.SelectedGroup && options.Group != record.ActualGroup {
		return false
	}
	if options.ChannelID > 0 && options.ChannelID != record.ChannelId && options.ChannelID != record.ActualChannelId {
		return false
	}
	if options.RequestID != "" && options.RequestID != record.RequestId {
		return false
	}
	if options.Hours > 0 {
		cutoff := time.Now().Add(-time.Duration(options.Hours) * time.Hour).Unix()
		return record.CreatedAt >= cutoff
	}
	return true
}

func BuildModelGatewayObservabilitySummary(options ModelGatewayObservabilityOptions) (ModelGatewayObservabilityResponse, error) {
	if model.DB == nil {
		return ModelGatewayObservabilityResponse{}, errors.New("database is not initialized")
	}
	options = normalizeModelGatewayObservabilityOptions(options)
	key := modelGatewayObservabilitySummaryCacheKey(options)
	now := time.Now()
	if response, ok := modelGatewayObservabilitySummaryCache.lookupFresh(key, now); ok {
		return response, nil
	}
	if response, ok := modelGatewayObservabilitySummaryCache.lookupStale(key, now); ok {
		go refreshModelGatewayObservabilitySummaryCache(key, options)
		return response, nil
	}

	value, err, _ := modelGatewayObservabilitySummaryCache.group.Do(key, func() (any, error) {
		response, err := buildModelGatewayObservabilitySummaryUncached(options)
		if err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
		modelGatewayObservabilitySummaryCache.store(key, options, response, time.Now())
		return response, nil
	})
	if err != nil {
		if response, ok := modelGatewayObservabilitySummaryCache.lookupStale(key, time.Now()); ok {
			return response, nil
		}
		return ModelGatewayObservabilityResponse{}, err
	}
	response, _ := value.(ModelGatewayObservabilityResponse)
	return response, nil
}

func buildModelGatewayObservabilitySummaryUncached(options ModelGatewayObservabilityOptions) (ModelGatewayObservabilityResponse, error) {
	options.Hours = clampModelGatewayObservabilityValue(options.Hours, 1, modelGatewayObservabilityMaxHours)
	options.RecentLimit = clampModelGatewayObservabilityValue(options.RecentLimit, 0, modelGatewayObservabilityMaxRecentLimit)
	options.TopN = clampModelGatewayObservabilityValue(options.TopN, 1, modelGatewayObservabilityMaxTopN)
	options.ScanLimit = normalizeModelGatewayObservabilityScanLimit(options.ScanLimit)
	options.TrendBucketSeconds = modelGatewayObservabilityTrendBucketSeconds(options.Hours, options.TrendBucketSeconds)

	endTime := common.GetTimestamp()
	startTime := endTime - int64(options.Hours*3600)
	base := model.DB.Model(&model.ModelExecutionRecord{}).Where("created_at >= ?", startTime)
	base = applyModelGatewayObservabilityFilters(base, options)

	records := make([]model.ModelExecutionRecord, 0)
	queryLimit := options.ScanLimit
	var totalRecords int64
	if options.IncludeTotal {
		if err := base.Count(&totalRecords).Error; err != nil {
			return ModelGatewayObservabilityResponse{}, err
		}
	} else {
		queryLimit = options.ScanLimit + 1
	}
	if err := base.Order("created_at desc, id desc").Limit(queryLimit).Find(&records).Error; err != nil {
		return ModelGatewayObservabilityResponse{}, err
	}
	if !options.IncludeTotal {
		totalRecords = int64(len(records))
		if len(records) > options.ScanLimit {
			records = records[:options.ScanLimit]
		}
	}

	response := buildModelGatewayObservabilityFromRecords(records, totalRecords, startTime, endTime, options)
	response.RuntimeStatus = defaultModelGatewayRuntimeStatusService().Build(modelgatewayobservability.RuntimeStatusQuery{
		Model:     options.Model,
		Group:     options.Group,
		ChannelID: options.ChannelID,
		Limit:     modelGatewayRuntimeStatusDefaultLimit,
	})
	applyModelGatewayRuntimeRiskEvents(&response)
	return response, nil
}

func defaultModelGatewayRuntimeStatusService() *modelgatewayobservability.RuntimeStatusService {
	runtimeDeps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	deps := modelgatewayobservability.RuntimeStatusDeps{
		QueueSnapshot: func() map[int]int {
			return currentRelayQueueManager().Snapshot()
		},
		QueueDetailSnapshot: func() modelgatewaycore.RuntimeQueueSnapshot {
			local := currentRelayQueueManager().DetailedSnapshot()
			if runtimeDeps == nil || runtimeDeps.QueueSnapshotSyncer == nil {
				return local
			}
			return runtimeDeps.QueueSnapshotSyncer.SyncAndAggregate(local)
		},
	}
	if runtimeDeps != nil {
		deps.SnapshotStore = runtimeDeps.SnapshotStore
		deps.Circuit = runtimeDeps.CircuitBreaker
	}
	return modelgatewayobservability.NewRuntimeStatusService(deps)
}

func applyModelGatewayObservabilityFilters(tx *gorm.DB, options ModelGatewayObservabilityOptions) *gorm.DB {
	if options.Model != "" {
		tx = tx.Where("requested_model = ?", options.Model)
	}
	if options.Group != "" {
		tx = tx.Where("(requested_group = ? OR selected_group = ? OR actual_group = ?)", options.Group, options.Group, options.Group)
	}
	if options.ChannelID > 0 {
		tx = tx.Where("(channel_id = ? OR actual_channel_id = ?)", options.ChannelID, options.ChannelID)
	}
	if options.RequestID != "" {
		tx = tx.Where("request_id = ?", options.RequestID)
	}
	return tx
}

func buildModelGatewayObservabilityFromRecords(records []model.ModelExecutionRecord, totalRecords int64, startTime int64, endTime int64, options ModelGatewayObservabilityOptions) ModelGatewayObservabilityResponse {
	response := ModelGatewayObservabilityResponse{
		Summary: ModelGatewayObservabilitySummary{
			WindowHours:        options.Hours,
			TrendBucketSeconds: options.TrendBucketSeconds,
			StartTime:          startTime,
			EndTime:            endTime,
			TotalRecords:       totalRecords,
			ScannedRecords:     len(records),
			Truncated:          totalRecords > int64(len(records)),
		},
		ByModel:           make([]ModelGatewayObservabilityAggregate, 0),
		ByGroup:           make([]ModelGatewayObservabilityAggregate, 0),
		ByChannel:         make([]ModelGatewayObservabilityAggregate, 0),
		ByProviderProfile: make([]ModelGatewayObservabilityAggregate, 0),
		ByProxyMode:       make([]ModelGatewayObservabilityAggregate, 0),
		Trends:            make([]ModelGatewayObservabilityTrendPoint, 0),
		RiskTimeline:      make([]ModelGatewayRiskEvent, 0),
		RecentRecords:     make([]ModelGatewayObservabilityRecord, 0, minModelGatewayObservabilityInt(options.RecentLimit, len(records))),
		ScoreBreakdown: ModelGatewayObservabilityScoreBreakdown{
			Average: make(map[string]float64),
		},
	}

	modelAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	groupAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	channelAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	providerProfileAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	proxyModeAccumulators := make(map[string]*modelGatewayObservabilityAccumulator)
	totalAccumulator := newModelGatewayObservabilityAccumulator("all", "", 0)
	trendBucketSeconds := options.TrendBucketSeconds
	trendAccumulators := make(map[int64]*modelGatewayObservabilityTrendAccumulator)
	dispatchMetaByRequestID := buildModelGatewayDispatchMetaIndex(records)

	for idx, record := range records {
		scoreBreakdown, scoreErr := parseModelGatewayScoreBreakdown(record.ScoreBreakdown)
		candidateGroups, candidateErr := parseModelGatewayCandidateGroups(record.CandidateGroups)
		requestMeta, requestMetaErr := parseModelGatewayRequestMeta(record.RequestMeta)
		applyModelGatewayObservabilityRecord(totalAccumulator, record, scoreBreakdown, requestMeta, scoreErr != nil)
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(modelAccumulators, modelGatewayObservabilityModelKey(record), "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(groupAccumulators, modelGatewayObservabilityGroupKey(record), "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)

		channelID, channelName := modelGatewayObservabilityChannel(record)
		channelKey := strconv.Itoa(channelID)
		if channelID == 0 {
			channelKey = "unknown"
		}
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(channelAccumulators, channelKey, channelName, channelID), record, scoreBreakdown, requestMeta, scoreErr != nil)

		profileProxyMeta := modelGatewayProfileProxyMetaForRecord(record, requestMeta, dispatchMetaByRequestID)
		providerProfileKey := profileProxyMeta.ProviderProfile
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(providerProfileAccumulators, providerProfileKey, "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)
		proxyModeKey := profileProxyMeta.ProxyMode
		applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(proxyModeAccumulators, proxyModeKey, "", 0), record, scoreBreakdown, requestMeta, scoreErr != nil)
		if bucketStart, ok := modelGatewayObservabilityTrendBucketStart(record.CreatedAt, startTime, endTime, trendBucketSeconds); ok {
			applyModelGatewayObservabilityTrendRecord(modelGatewayObservabilityTrendAccumulatorFor(trendAccumulators, bucketStart), record, scoreBreakdown, requestMeta, scoreErr != nil, profileProxyMeta)
		}

		if idx < options.RecentLimit {
			recent := modelGatewayObservabilityRecentRecord(record, scoreBreakdown, candidateGroups, requestMeta)
			recent.ScoreBreakdownError = scoreErr != nil
			recent.CandidateGroupsError = candidateErr != nil
			recent.RequestMetaError = requestMetaErr != nil
			response.RecentRecords = append(response.RecentRecords, recent)
		}
	}

	response.Summary = modelGatewayObservabilitySummaryFromAccumulator(response.Summary, totalAccumulator)
	scoreBreakdownAverage := averageModelGatewayObservabilityScoreBreakdown(totalAccumulator)
	if scoreBreakdownAverage == nil {
		scoreBreakdownAverage = map[string]float64{}
	}
	response.ScoreBreakdown = ModelGatewayObservabilityScoreBreakdown{
		Samples:     totalAccumulator.ScoreBreakdownSamples,
		Average:     scoreBreakdownAverage,
		ParseErrors: totalAccumulator.ScoreBreakdownParseErrs,
	}
	response.ByModel = finalizeModelGatewayObservabilityAggregates(modelAccumulators, options.TopN)
	response.ByGroup = finalizeModelGatewayObservabilityAggregates(groupAccumulators, options.TopN)
	response.ByChannel = finalizeModelGatewayObservabilityAggregates(channelAccumulators, options.TopN)
	response.ByProviderProfile = finalizeModelGatewayObservabilityAggregates(providerProfileAccumulators, options.TopN)
	response.ByProxyMode = finalizeModelGatewayObservabilityAggregates(proxyModeAccumulators, options.TopN)
	response.Trends = finalizeModelGatewayObservabilityTrends(trendAccumulators, startTime, endTime, trendBucketSeconds)
	response.Summary.CircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(modelGatewayCircuitOpenReasonCountsFromTrends(response.Trends), modelGatewayObservabilityTrendTopN)
	response.Summary.CircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(modelGatewayCircuitErrorTypeCountsFromTrends(response.Trends), modelGatewayObservabilityTrendTopN)
	response.Summary.CircuitErrorCounts = response.Summary.CircuitErrorTypes
	applyModelGatewayTrendRiskTimeline(&response)
	return response
}

func modelGatewayObservabilitySummaryFromAccumulator(summary ModelGatewayObservabilitySummary, accumulator *modelGatewayObservabilityAccumulator) ModelGatewayObservabilitySummary {
	summary.Dispatches = accumulator.Dispatches
	summary.Attempts = accumulator.Attempts
	summary.Successes = accumulator.Successes
	summary.Failures = accumulator.Failures
	summary.StreamInterrupted = accumulator.StreamInterrupted
	summary.FallbackUsed = accumulator.FallbackUsed
	summary.SuccessRate = successRateModelGatewayObservability(accumulator.Successes, accumulator.Attempts)
	summary.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
	summary.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
	summary.AvgScoreTotal = averageFloat64(accumulator.scoreTotalSum, accumulator.scoreTotalSamples)
	summary.QueueEnabledDispatches = accumulator.QueueEnabledDispatches
	summary.QueuedDispatches = accumulator.QueuedDispatches
	summary.AvgQueueWaitMs = averageInt64(accumulator.queueWaitSum, accumulator.queueWaitSamples)
	summary.StickyRoutes = accumulator.StickyRoutes
	summary.StickyRetained = accumulator.StickyRetained
	summary.StickyBroken = accumulator.StickyBroken
	summary.CacheAffinityRoutes = accumulator.CacheAffinityRoutes
	summary.ScoreBreakdownSamples = accumulator.ScoreBreakdownSamples
	summary.ScoreBreakdownParseErrs = accumulator.ScoreBreakdownParseErrs
	return summary
}

func modelGatewayObservabilityRecentRecord(record model.ModelExecutionRecord, scoreBreakdown map[string]float64, candidateGroups []string, requestMeta map[string]any) ModelGatewayObservabilityRecord {
	meta := modelGatewayObservabilityMetaFromRequestMeta(requestMeta)
	candidateExplanations := modelGatewayCandidateExplanationsFromRequestMeta(requestMeta)
	return ModelGatewayObservabilityRecord{
		ID:                    record.Id,
		Kind:                  modelGatewayObservabilityRecordKind(record),
		CreatedAt:             record.CreatedAt,
		RequestID:             record.RequestId,
		AttemptIndex:          record.AttemptIndex,
		RequestedGroup:        record.RequestedGroup,
		SelectedGroup:         record.SelectedGroup,
		ActualGroup:           record.ActualGroup,
		RequestedModel:        record.RequestedModel,
		ChannelID:             record.ChannelId,
		ChannelName:           record.ChannelName,
		ActualChannelID:       record.ActualChannelId,
		ActualChannelName:     record.ActualChannelName,
		EndpointType:          record.EndpointType,
		PolicyMode:            record.PolicyMode,
		AutoMode:              record.AutoMode,
		Strategy:              record.Strategy,
		Shadow:                record.Shadow,
		SmartHandled:          record.SmartHandled,
		FallbackUsed:          record.FallbackUsed,
		Success:               record.Success,
		StatusCode:            record.StatusCode,
		ErrorCode:             record.ErrorCode,
		ErrorType:             record.ErrorType,
		DurationMs:            record.DurationMs,
		TTFTMs:                record.TTFTMs,
		StreamInterrupted:     record.StreamInterrupted,
		ScoreTotal:            roundModelGatewayObservabilityFloat(record.ScoreTotal),
		ScoreBreakdown:        scoreBreakdown,
		CandidateGroups:       candidateGroups,
		CandidateExplanations: candidateExplanations,
		SelectedReason:        record.SelectedReason,
		QueueEnabled:          meta.QueueEnabled,
		QueueWaitMs:           meta.QueueWaitMs,
		QueueDepth:            meta.QueueDepth,
		QueueCapacity:         meta.QueueCapacity,
		StickySource:          meta.StickySource,
		StickyRetained:        meta.StickyRetained,
		StickyBreak:           meta.StickyBreak,
		CacheAffinity:         meta.CacheAffinity,
		RequestMeta:           requestMeta,
	}
}

func ModelGatewayObservabilityRecordFromModelRecord(record model.ModelExecutionRecord) ModelGatewayObservabilityRecord {
	scoreBreakdown, scoreErr := parseModelGatewayScoreBreakdown(record.ScoreBreakdown)
	candidateGroups, candidateErr := parseModelGatewayCandidateGroups(record.CandidateGroups)
	requestMeta, requestMetaErr := parseModelGatewayRequestMeta(record.RequestMeta)
	recent := modelGatewayObservabilityRecentRecord(record, scoreBreakdown, candidateGroups, requestMeta)
	recent.ScoreBreakdownError = scoreErr != nil
	recent.CandidateGroupsError = candidateErr != nil
	recent.RequestMetaError = requestMetaErr != nil
	return recent
}

func applyModelGatewayObservabilityRecord(accumulator *modelGatewayObservabilityAccumulator, record model.ModelExecutionRecord, scoreBreakdown map[string]float64, requestMeta map[string]any, scoreErr bool) {
	accumulator.Records++
	if record.CreatedAt > accumulator.LastRecordAt {
		accumulator.LastRecordAt = record.CreatedAt
	}
	if isModelGatewayDispatchRecord(record) {
		accumulator.Dispatches++
		applyModelGatewayObservabilityDispatchMeta(accumulator, requestMeta)
	}
	if record.FallbackUsed {
		accumulator.FallbackUsed++
	}
	if isModelGatewayAttemptRecord(record) {
		accumulator.Attempts++
		if record.Success {
			accumulator.Successes++
		} else {
			accumulator.Failures++
		}
		if record.StreamInterrupted {
			accumulator.StreamInterrupted++
		}
		if record.DurationMs > 0 {
			accumulator.durationSum += record.DurationMs
			accumulator.durationSamples++
		}
		if record.TTFTMs > 0 {
			accumulator.ttftSum += record.TTFTMs
			accumulator.ttftSamples++
		}
	}
	if hasModelGatewayScoreTotalSample(record) {
		accumulator.scoreTotalSum += record.ScoreTotal
		accumulator.scoreTotalSamples++
	}
	if scoreErr {
		accumulator.ScoreBreakdownParseErrs++
		return
	}
	if len(scoreBreakdown) == 0 {
		return
	}
	if accumulator.scoreSums == nil {
		accumulator.scoreSums = make(map[string]float64, len(scoreBreakdown))
	}
	for key, value := range scoreBreakdown {
		accumulator.scoreSums[key] += value
	}
	accumulator.ScoreBreakdownSamples++
}

type modelGatewayObservabilityDispatchMeta struct {
	QueueEnabled   bool
	QueueWaitMs    int64
	QueueWaitSet   bool
	QueueDepth     int
	QueueCapacity  int
	StickySource   string
	StickyRetained bool
	StickyBreak    string
	CacheAffinity  bool
}

type modelGatewayProfileProxyMeta struct {
	ProviderProfile string
	ProxyMode       string
}

func buildModelGatewayDispatchMetaIndex(records []model.ModelExecutionRecord) map[string]modelGatewayProfileProxyMeta {
	index := make(map[string]modelGatewayProfileProxyMeta)
	for _, record := range records {
		if strings.TrimSpace(record.RequestId) == "" || !isModelGatewayDispatchRecord(record) {
			continue
		}
		requestMeta, err := parseModelGatewayRequestMeta(record.RequestMeta)
		if err != nil {
			continue
		}
		meta := modelGatewayProfileProxyMeta{
			ProviderProfile: modelGatewayObservabilityProviderProfileKey(requestMeta),
			ProxyMode:       modelGatewayObservabilityProxyModeKey(requestMeta),
		}
		if meta.ProviderProfile == "unknown" && meta.ProxyMode == "unknown" {
			continue
		}
		index[record.RequestId] = meta
	}
	return index
}

func applyModelGatewayObservabilityDispatchMeta(accumulator *modelGatewayObservabilityAccumulator, requestMeta map[string]any) {
	if accumulator == nil {
		return
	}
	meta := modelGatewayObservabilityMetaFromRequestMeta(requestMeta)
	if meta.QueueEnabled {
		accumulator.QueueEnabledDispatches++
	}
	if meta.QueueWaitMs > 0 {
		accumulator.QueuedDispatches++
	}
	if meta.QueueWaitSet {
		accumulator.queueWaitSum += meta.QueueWaitMs
		accumulator.queueWaitSamples++
		accumulator.queueWaitValues = append(accumulator.queueWaitValues, meta.QueueWaitMs)
	}
	if meta.StickySource != "" {
		accumulator.StickyRoutes++
	}
	if meta.StickyRetained {
		accumulator.StickyRetained++
	}
	if meta.StickyBreak != "" {
		accumulator.StickyBroken++
	}
	if meta.CacheAffinity {
		accumulator.CacheAffinityRoutes++
	}
}

func modelGatewayObservabilityMetaFromRequestMeta(requestMeta map[string]any) modelGatewayObservabilityDispatchMeta {
	if len(requestMeta) == 0 {
		return modelGatewayObservabilityDispatchMeta{}
	}
	queueWaitMs, queueWaitSet := modelGatewayObservabilityMetaNonNegativeInt64(requestMeta["queue_wait_ms"])
	return modelGatewayObservabilityDispatchMeta{
		QueueEnabled:   modelGatewayObservabilityMetaBool(requestMeta["queue_enabled"]),
		QueueWaitMs:    queueWaitMs,
		QueueWaitSet:   queueWaitSet,
		QueueDepth:     int(modelGatewayObservabilityMetaInt64(requestMeta["queue_depth"])),
		QueueCapacity:  int(modelGatewayObservabilityMetaInt64(requestMeta["queue_capacity"])),
		StickySource:   strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["sticky_source"])),
		StickyRetained: modelGatewayObservabilityMetaBool(requestMeta["sticky_retained"]),
		StickyBreak:    strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["sticky_break"])),
		CacheAffinity:  modelGatewayObservabilityMetaBool(requestMeta["cache_affinity"]),
	}
}

func modelGatewayCandidateExplanationsFromRequestMeta(requestMeta map[string]any) []ModelGatewayCandidateExplanation {
	if len(requestMeta) == 0 {
		return nil
	}
	raw, ok := requestMeta["candidate_explanations"]
	if !ok || raw == nil {
		return nil
	}
	bytes, err := common.Marshal(raw)
	if err != nil {
		return nil
	}
	candidates := make([]modelgatewaycore.CandidateExplanation, 0)
	if err := common.Unmarshal(bytes, &candidates); err != nil {
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	out := make([]ModelGatewayCandidateExplanation, 0, len(candidates))
	for _, candidate := range candidates {
		item := ModelGatewayCandidateExplanation{
			ChannelID:       candidate.ChannelID,
			ChannelName:     candidate.ChannelName,
			Group:           candidate.Group,
			UpstreamModel:   candidate.UpstreamModel,
			ProviderProfile: candidate.ProviderProfile,
			ProxyMode:       candidate.ProxyMode,
			RuntimeKey:      modelGatewayRuntimeKeyFromCore(candidate.RuntimeKey),
			Available:       candidate.Available,
			RejectReason:    candidate.RejectReason,
			ScoreTotal:      roundModelGatewayObservabilityFloat(candidate.ScoreTotal),
			ScoreBreakdown:  roundModelGatewayScoreMap(candidate.ScoreBreakdown),
			StickyMatched:   candidate.StickyMatched,
			Selected:        candidate.Selected,
		}
		out = append(out, item)
	}
	return out
}

func modelGatewayRejectReasonsFromRequestMeta(requestMeta map[string]any) []string {
	if len(requestMeta) == 0 {
		return nil
	}
	raw, ok := requestMeta["candidate_explanations"]
	if !ok || raw == nil {
		return nil
	}
	bytes, err := common.Marshal(raw)
	if err != nil {
		return nil
	}
	candidates := make([]modelgatewaycore.CandidateExplanation, 0)
	if err := common.Unmarshal(bytes, &candidates); err != nil {
		return nil
	}
	reasons := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		reason := strings.TrimSpace(candidate.RejectReason)
		if candidate.Available || reason == "" {
			continue
		}
		reasons = append(reasons, reason)
	}
	return reasons
}

func modelGatewayRuntimeKeyFromCore(key modelgatewaycore.RuntimeKey) ModelGatewayRuntimeKey {
	return ModelGatewayRuntimeKey{
		RequestedModel:        key.RequestedModel,
		UpstreamModel:         key.UpstreamModel,
		ChannelID:             key.ChannelID,
		Group:                 key.Group,
		EndpointType:          string(key.EndpointType),
		CapabilityFingerprint: key.CapabilityFingerprint,
	}
}

func roundModelGatewayScoreMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	rounded := make(map[string]float64, len(values))
	for key, value := range values {
		rounded[key] = roundModelGatewayObservabilityFloat(value)
	}
	return rounded
}

func modelGatewayObservabilityMetaBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func modelGatewayObservabilityMetaInt64(value any) int64 {
	parsed, ok := modelGatewayObservabilityMetaInt64Value(value)
	if !ok {
		return 0
	}
	return parsed
}

func modelGatewayObservabilityMetaNonNegativeInt64(value any) (int64, bool) {
	parsed, ok := modelGatewayObservabilityMetaInt64Value(value)
	if !ok || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func modelGatewayObservabilityMetaInt64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return math.MaxInt64, true
		}
		return int64(typed), true
	case float32:
		return int64(math.Round(float64(typed))), true
	case float64:
		return int64(math.Round(typed)), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed, true
		}
		parsedFloat, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return int64(math.Round(parsedFloat)), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func modelGatewayObservabilityMetaString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func finalizeModelGatewayObservabilityAggregates(accumulators map[string]*modelGatewayObservabilityAccumulator, topN int) []ModelGatewayObservabilityAggregate {
	items := make([]ModelGatewayObservabilityAggregate, 0, len(accumulators))
	for _, accumulator := range accumulators {
		item := accumulator.ModelGatewayObservabilityAggregate
		item.SuccessRate = successRateModelGatewayObservability(item.Successes, item.Attempts)
		item.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
		item.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
		item.AvgScoreTotal = averageFloat64(accumulator.scoreTotalSum, accumulator.scoreTotalSamples)
		item.AvgQueueWaitMs = averageInt64(accumulator.queueWaitSum, accumulator.queueWaitSamples)
		item.ScoreBreakdown = averageModelGatewayObservabilityScoreBreakdown(accumulator)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Attempts != items[j].Attempts {
			return items[i].Attempts > items[j].Attempts
		}
		if items[i].Failures != items[j].Failures {
			return items[i].Failures > items[j].Failures
		}
		if items[i].Records != items[j].Records {
			return items[i].Records > items[j].Records
		}
		return items[i].Key < items[j].Key
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func finalizeModelGatewayObservabilityTrends(accumulators map[int64]*modelGatewayObservabilityTrendAccumulator, startTime int64, endTime int64, bucketSeconds int64) []ModelGatewayObservabilityTrendPoint {
	if startTime <= 0 || endTime <= startTime || bucketSeconds <= 0 {
		return []ModelGatewayObservabilityTrendPoint{}
	}
	bucketCount := int((endTime - startTime + bucketSeconds - 1) / bucketSeconds)
	if bucketCount <= 0 {
		return []ModelGatewayObservabilityTrendPoint{}
	}
	items := make([]ModelGatewayObservabilityTrendPoint, 0, bucketCount)
	for idx := 0; idx < bucketCount; idx++ {
		bucketStart := startTime + int64(idx)*bucketSeconds
		bucketEnd := bucketStart + bucketSeconds
		if bucketEnd > endTime {
			bucketEnd = endTime
		}
		items = append(items, modelGatewayObservabilityTrendPointFromAccumulator(bucketStart, bucketEnd, accumulators[bucketStart]))
	}
	return items
}

func modelGatewayObservabilityTrendPointFromAccumulator(bucketStart int64, bucketEnd int64, accumulator *modelGatewayObservabilityTrendAccumulator) ModelGatewayObservabilityTrendPoint {
	point := ModelGatewayObservabilityTrendPoint{
		BucketStart: bucketStart,
		BucketEnd:   bucketEnd,
	}
	if accumulator == nil {
		return point
	}
	point.Records = accumulator.Records
	point.Dispatches = accumulator.Dispatches
	point.Attempts = accumulator.Attempts
	point.Successes = accumulator.Successes
	point.Failures = accumulator.Failures
	point.StreamInterrupted = accumulator.StreamInterrupted
	point.FallbackUsed = accumulator.FallbackUsed
	point.SuccessRate = successRateModelGatewayObservability(accumulator.Successes, accumulator.Attempts)
	point.AvgDurationMs = averageInt64(accumulator.durationSum, accumulator.durationSamples)
	point.AvgTTFTMs = averageInt64(accumulator.ttftSum, accumulator.ttftSamples)
	point.QueueEnabledDispatches = accumulator.QueueEnabledDispatches
	point.QueuedDispatches = accumulator.QueuedDispatches
	point.AvgQueueWaitMs = averageInt64(accumulator.queueWaitSum, accumulator.queueWaitSamples)
	point.QueueWaitP50Ms = percentileModelGatewayObservabilityInt64(accumulator.queueWaitValues, 0.50)
	point.QueueWaitP90Ms = percentileModelGatewayObservabilityInt64(accumulator.queueWaitValues, 0.90)
	point.QueueWaitP95Ms = percentileModelGatewayObservabilityInt64(accumulator.queueWaitValues, 0.95)
	point.StickyRoutes = accumulator.StickyRoutes
	point.StickyRetained = accumulator.StickyRetained
	point.StickyBroken = accumulator.StickyBroken
	point.CacheAffinityRoutes = accumulator.CacheAffinityRoutes
	point.ByProviderProfile = finalizeModelGatewayObservabilityAggregates(accumulator.ProviderProfileAccumulators, modelGatewayObservabilityTrendTopN)
	point.ByProxyMode = finalizeModelGatewayObservabilityAggregates(accumulator.ProxyModeAccumulators, modelGatewayObservabilityTrendTopN)
	point.RejectReasons = finalizeModelGatewayTrendReasonCounts(accumulator.RejectReasons, modelGatewayObservabilityTrendTopN)
	point.CircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(accumulator.CircuitOpenReasons, modelGatewayObservabilityTrendTopN)
	point.CircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(accumulator.CircuitErrorTypes, modelGatewayObservabilityTrendTopN)
	point.CircuitErrorCounts = point.CircuitErrorTypes
	return point
}

func applyModelGatewayTrendRiskTimeline(response *ModelGatewayObservabilityResponse) {
	if response == nil {
		return
	}
	timeline := make([]ModelGatewayRiskEvent, 0)
	previousStatus := "healthy"
	for idx := range response.Trends {
		point := &response.Trends[idx]
		if point.Records <= 0 {
			continue
		}
		currentStatus, currentSeverity := modelGatewayTrendRiskStatus(*point)
		if currentStatus != previousStatus {
			event := ModelGatewayRiskEvent{
				Timestamp:      point.BucketStart,
				BucketStart:    point.BucketStart,
				BucketEnd:      point.BucketEnd,
				Source:         "trend_bucket",
				EventType:      "risk_status_change",
				Severity:       currentSeverity,
				Status:         currentStatus,
				PreviousStatus: previousStatus,
				CurrentStatus:  currentStatus,
				Count:          1,
			}
			point.RiskEvents = append(point.RiskEvents, event)
			timeline = append(timeline, event)
			response.Summary.RiskStatusChanges++
		}
		previousStatus = currentStatus
		events := modelGatewayTrendDetailRiskEvents(*point)
		point.RiskEvents = append(point.RiskEvents, events...)
		timeline = append(timeline, events...)
		if len(point.RiskEvents) > 0 {
			point.RiskEvents = finalizeModelGatewayRiskTimeline(point.RiskEvents, 0)
			risk := buildModelGatewayRiskSnapshot(
				point.RiskEvents,
				int64(len(point.RiskEvents)),
				modelGatewayRiskStatusChangesFromEvents(point.RiskEvents),
				0,
				modelGatewayTrendRejectReasonCounts(point.RejectReasons),
				modelGatewayTrendReasonCountsToMap(point.CircuitOpenReasons),
				modelGatewayTrendReasonCountsToMap(point.CircuitErrorTypes),
			)
			point.Risk = &risk
		}
	}
	response.Summary.RiskEvents += int64(len(timeline))
	response.RiskTimeline = finalizeModelGatewayRiskTimeline(append(response.RiskTimeline, timeline...), modelGatewayObservabilityRiskTimelineLimit)
	syncModelGatewayRiskCompatibility(response)
}

func applyModelGatewayRuntimeRiskEvents(response *ModelGatewayObservabilityResponse) {
	if response == nil || len(response.RuntimeStatus.Items) == 0 {
		return
	}
	timestamp := response.RuntimeStatus.Summary.UpdatedAt
	if timestamp <= 0 {
		timestamp = common.GetTimestamp()
	}
	circuitOpenReasons := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitOpenReasons)
	circuitErrorTypes := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitErrorTypes)
	circuitSummaryChanged := false
	events := make([]ModelGatewayRiskEvent, 0)
	for _, item := range response.RuntimeStatus.Items {
		if reason := strings.TrimSpace(item.CircuitOpenReason); reason != "" {
			circuitOpenReasons[reason]++
			circuitSummaryChanged = true
		}
		for kind, count := range item.CircuitErrorCounts {
			kind = strings.TrimSpace(kind)
			if kind == "" || count <= 0 {
				continue
			}
			circuitErrorTypes[kind] += int64(count)
			circuitSummaryChanged = true
		}
		status := strings.TrimSpace(item.HealthStatus)
		if status == "" || status == "healthy" {
			continue
		}
		response.Summary.CurrentRiskRuntimeKeys++
		events = append(events, ModelGatewayRiskEvent{
			Timestamp:      timestamp,
			Source:         "runtime_status",
			EventType:      "current_runtime_status",
			Severity:       modelGatewayRiskSeverityForStatus(status),
			Status:         status,
			CurrentStatus:  status,
			Reason:         modelGatewayRuntimeRiskReason(item),
			Count:          1,
			RequestedModel: item.RequestedModel,
			UpstreamModel:  item.UpstreamModel,
			Group:          item.Group,
			ChannelID:      item.ChannelID,
			EndpointType:   item.EndpointType,
		})
	}
	if circuitSummaryChanged {
		response.Summary.CircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(circuitOpenReasons, modelGatewayObservabilityTrendTopN)
		response.Summary.CircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(circuitErrorTypes, modelGatewayObservabilityTrendTopN)
		response.Summary.CircuitErrorCounts = response.Summary.CircuitErrorTypes
	}
	if len(events) == 0 {
		if circuitSummaryChanged {
			syncModelGatewayRiskCompatibility(response)
		}
		return
	}
	response.Summary.RiskEvents += int64(len(events))
	response.RiskTimeline = finalizeModelGatewayRiskTimeline(append(response.RiskTimeline, events...), modelGatewayObservabilityRiskTimelineLimit)
	syncModelGatewayRiskCompatibility(response)
}

func modelGatewayTrendRiskStatus(point ModelGatewayObservabilityTrendPoint) (string, string) {
	switch {
	case point.StreamInterrupted > 0:
		return "stream_interrupted", "critical"
	case point.Attempts > 0 && point.Failures > 0 && point.SuccessRate < 0.5:
		return "failure_spike", "critical"
	case point.FallbackUsed > 0:
		return "fallback", "warning"
	case len(point.RejectReasons) > 0:
		return "candidate_rejected", "warning"
	case point.QueueWaitP95Ms > 0 || point.QueuedDispatches > 0:
		return "queued", "warning"
	case point.StickyBroken > 0:
		return "sticky_broken", "info"
	case point.Attempts > 0 && point.SuccessRate > 0 && point.SuccessRate < 0.8:
		return "degraded", "warning"
	default:
		return "healthy", "info"
	}
}

func modelGatewayTrendDetailRiskEvents(point ModelGatewayObservabilityTrendPoint) []ModelGatewayRiskEvent {
	events := make([]ModelGatewayRiskEvent, 0)
	base := ModelGatewayRiskEvent{
		Timestamp:   point.BucketStart,
		BucketStart: point.BucketStart,
		BucketEnd:   point.BucketEnd,
		Source:      "trend_bucket",
	}
	if point.StreamInterrupted > 0 {
		event := base
		event.EventType = "stream_interrupted"
		event.Severity = "critical"
		event.Status = "stream_interrupted"
		event.Count = point.StreamInterrupted
		events = append(events, event)
	}
	if point.FallbackUsed > 0 {
		event := base
		event.EventType = "fallback_used"
		event.Severity = "warning"
		event.Status = "fallback"
		event.Count = point.FallbackUsed
		events = append(events, event)
	}
	if point.Failures > 0 {
		event := base
		event.EventType = "attempt_failure"
		event.Severity = modelGatewayRiskSeverityForStatus("degraded")
		event.Status = "degraded"
		event.Count = point.Failures
		events = append(events, event)
	}
	if point.QueueWaitP95Ms > 0 || point.QueuedDispatches > 0 {
		event := base
		event.EventType = "queue_wait"
		event.Severity = "warning"
		event.Status = "queued"
		event.Count = point.QueuedDispatches
		if event.Count <= 0 {
			event.Count = 1
		}
		events = append(events, event)
	}
	if point.StickyBroken > 0 {
		event := base
		event.EventType = "sticky_break"
		event.Severity = "info"
		event.Status = "sticky_broken"
		event.Count = point.StickyBroken
		events = append(events, event)
	}
	for _, reason := range point.RejectReasons {
		if reason.Count <= 0 || strings.TrimSpace(reason.Reason) == "" {
			continue
		}
		event := base
		event.EventType = "reject_reason"
		event.Status = modelGatewayRiskStatusForRejectReason(reason.Reason)
		event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		event.Reason = reason.Reason
		event.Count = reason.Count
		events = append(events, event)
	}
	for _, reason := range point.CircuitOpenReasons {
		if reason.Count <= 0 || strings.TrimSpace(reason.Reason) == "" {
			continue
		}
		event := base
		event.EventType = "circuit_open_reason"
		event.Status = "circuit_open"
		event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		event.Reason = reason.Reason
		event.Count = reason.Count
		events = append(events, event)
	}
	for _, kind := range point.CircuitErrorTypes {
		if kind.Count <= 0 || strings.TrimSpace(kind.Reason) == "" {
			continue
		}
		event := base
		event.EventType = "circuit_error_type"
		event.Status = modelGatewayRiskStatusForCircuitErrorType(kind.Reason)
		event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		event.Reason = kind.Reason
		event.Count = kind.Count
		events = append(events, event)
	}
	sortModelGatewayRiskEvents(events)
	return events
}

func buildModelGatewayTrendExportPreview(response ModelGatewayObservabilityResponse) ModelGatewayTrendExportPreview {
	preview := ModelGatewayTrendExportPreview{
		BucketCount:            len(response.Trends),
		RiskEvents:             response.Summary.RiskEvents,
		RiskEventCount:         response.Summary.RiskEvents,
		RiskStatusChanges:      response.Summary.RiskStatusChanges,
		CurrentRiskRuntimeKeys: response.Summary.CurrentRiskRuntimeKeys,
	}
	rejectReasons := make(map[string]int64)
	circuitOpenReasons := make(map[string]int64)
	circuitErrorTypes := make(map[string]int64)
	for _, trend := range response.Trends {
		if trend.Records > 0 {
			preview.NonEmptyBuckets++
		}
		for _, reason := range trend.RejectReasons {
			rejectReasons[reason.Reason] += reason.Count
		}
		for _, reason := range trend.CircuitOpenReasons {
			circuitOpenReasons[reason.Reason] += reason.Count
		}
		for _, kind := range trend.CircuitErrorTypes {
			circuitErrorTypes[kind.Reason] += kind.Count
		}
	}
	for _, reason := range response.Summary.CircuitOpenReasons {
		if _, ok := circuitOpenReasons[reason.Reason]; !ok {
			circuitOpenReasons[reason.Reason] = reason.Count
		}
	}
	for _, kind := range response.Summary.CircuitErrorTypes {
		if _, ok := circuitErrorTypes[kind.Reason]; !ok {
			circuitErrorTypes[kind.Reason] = kind.Count
		}
	}
	preview.RiskTimeline = limitModelGatewayRiskEvents(response.RiskTimeline, modelGatewayTrendExportPreviewLimit)
	preview.TopRiskStatuses = finalizeModelGatewayRiskStatusCounts(response.RiskTimeline, modelGatewayObservabilityTrendTopN)
	preview.TopRejectReasons = finalizeModelGatewayTrendReasonCounts(rejectReasons, modelGatewayObservabilityTrendTopN)
	preview.TopCircuitOpenReasons = finalizeModelGatewayTrendReasonCounts(circuitOpenReasons, modelGatewayObservabilityTrendTopN)
	preview.TopCircuitErrorTypes = finalizeModelGatewayTrendReasonCounts(circuitErrorTypes, modelGatewayObservabilityTrendTopN)
	preview.Risk = buildModelGatewayRiskSnapshot(
		preview.RiskTimeline,
		preview.RiskEvents,
		preview.RiskStatusChanges,
		preview.CurrentRiskRuntimeKeys,
		rejectReasons,
		circuitOpenReasons,
		circuitErrorTypes,
	)
	return preview
}

func syncModelGatewayRiskCompatibility(response *ModelGatewayObservabilityResponse) {
	if response == nil {
		return
	}
	response.RiskTimeline = finalizeModelGatewayRiskTimeline(response.RiskTimeline, modelGatewayObservabilityRiskTimelineLimit)
	response.RiskEvents = limitModelGatewayRiskEvents(response.RiskTimeline, 0)
	if response.RiskEvents == nil {
		response.RiskEvents = []ModelGatewayRiskEvent{}
	}
	response.Risk = buildModelGatewayRiskSnapshot(
		response.RiskTimeline,
		response.Summary.RiskEvents,
		response.Summary.RiskStatusChanges,
		response.Summary.CurrentRiskRuntimeKeys,
		modelGatewayRejectReasonCountsFromTrends(response.Trends),
		modelGatewayCircuitOpenReasonCountsFromSummaryOrTrends(*response),
		modelGatewayCircuitErrorTypeCountsFromSummaryOrTrends(*response),
	)
}

func buildModelGatewayRiskSnapshot(events []ModelGatewayRiskEvent, eventCount int64, statusChanges int64, currentRuntimeKeys int, rejectReasons map[string]int64, circuitOpenReasons map[string]int64, circuitErrorTypes map[string]int64) ModelGatewayRiskSnapshot {
	timeline := limitModelGatewayRiskEvents(events, 0)
	if timeline == nil {
		timeline = []ModelGatewayRiskEvent{}
	}
	if eventCount <= 0 && len(timeline) > 0 {
		eventCount = int64(len(timeline))
	}
	topStatuses := finalizeModelGatewayRiskStatusCounts(timeline, modelGatewayObservabilityTrendTopN)
	topRejectReasons := finalizeModelGatewayTrendReasonCounts(rejectReasons, modelGatewayObservabilityTrendTopN)
	topCircuitOpenReasons := finalizeModelGatewayTrendReasonCounts(circuitOpenReasons, modelGatewayObservabilityTrendTopN)
	topCircuitErrorTypes := finalizeModelGatewayTrendReasonCounts(circuitErrorTypes, modelGatewayObservabilityTrendTopN)
	return ModelGatewayRiskSnapshot{
		EventCount:             eventCount,
		RiskEventCount:         eventCount,
		StatusChanges:          statusChanges,
		RiskStatusChanges:      statusChanges,
		CurrentRuntimeKeys:     currentRuntimeKeys,
		CurrentRiskRuntimeKeys: currentRuntimeKeys,
		Events:                 timeline,
		RiskEvents:             timeline,
		Timeline:               timeline,
		RiskTimeline:           timeline,
		TopStatuses:            topStatuses,
		TopRiskStatuses:        topStatuses,
		TopRejectReasons:       topRejectReasons,
		TopCircuitOpenReasons:  topCircuitOpenReasons,
		TopCircuitErrorTypes:   topCircuitErrorTypes,
	}
}

func modelGatewayRiskStatusChangesFromEvents(events []ModelGatewayRiskEvent) int64 {
	var count int64
	for _, event := range events {
		if event.EventType == "risk_status_change" {
			count++
		}
	}
	return count
}

func modelGatewayRejectReasonCountsFromTrends(trends []ModelGatewayObservabilityTrendPoint) map[string]int64 {
	counts := make(map[string]int64)
	for _, trend := range trends {
		for _, reason := range trend.RejectReasons {
			if strings.TrimSpace(reason.Reason) == "" || reason.Count <= 0 {
				continue
			}
			counts[reason.Reason] += reason.Count
		}
	}
	return counts
}

func modelGatewayCircuitOpenReasonCountsFromTrends(trends []ModelGatewayObservabilityTrendPoint) map[string]int64 {
	counts := make(map[string]int64)
	for _, trend := range trends {
		for _, reason := range trend.CircuitOpenReasons {
			if strings.TrimSpace(reason.Reason) == "" || reason.Count <= 0 {
				continue
			}
			counts[reason.Reason] += reason.Count
		}
	}
	return counts
}

func modelGatewayCircuitErrorTypeCountsFromTrends(trends []ModelGatewayObservabilityTrendPoint) map[string]int64 {
	counts := make(map[string]int64)
	for _, trend := range trends {
		for _, kind := range trend.CircuitErrorTypes {
			if strings.TrimSpace(kind.Reason) == "" || kind.Count <= 0 {
				continue
			}
			counts[kind.Reason] += kind.Count
		}
	}
	return counts
}

func modelGatewayCircuitOpenReasonCountsFromSummaryOrTrends(response ModelGatewayObservabilityResponse) map[string]int64 {
	counts := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitOpenReasons)
	if len(counts) == 0 {
		return modelGatewayCircuitOpenReasonCountsFromTrends(response.Trends)
	}
	return counts
}

func modelGatewayCircuitErrorTypeCountsFromSummaryOrTrends(response ModelGatewayObservabilityResponse) map[string]int64 {
	counts := modelGatewayTrendReasonCountsToMap(response.Summary.CircuitErrorTypes)
	if len(counts) == 0 {
		return modelGatewayCircuitErrorTypeCountsFromTrends(response.Trends)
	}
	return counts
}

func modelGatewayTrendRejectReasonCounts(reasons []ModelGatewayTrendReasonCount) map[string]int64 {
	return modelGatewayTrendReasonCountsToMap(reasons)
}

func modelGatewayTrendReasonCountsToMap(reasons []ModelGatewayTrendReasonCount) map[string]int64 {
	counts := make(map[string]int64)
	for _, reason := range reasons {
		if strings.TrimSpace(reason.Reason) == "" || reason.Count <= 0 {
			continue
		}
		counts[reason.Reason] += reason.Count
	}
	return counts
}

func modelGatewayRuntimeRiskReason(item modelgatewayobservability.RuntimeStatusItem) string {
	switch item.HealthStatus {
	case "circuit_open":
		if reason := strings.TrimSpace(item.CircuitOpenReason); reason != "" {
			return reason
		}
		return "circuit_open"
	case "cooldown":
		return strings.TrimSpace(item.CooldownReason)
	case "failure_avoidance":
		return strings.TrimSpace(item.FailureAvoidanceReason)
	case "queued":
		return "queue_depth"
	case "saturated":
		return "concurrency_saturated"
	case "degraded":
		return "success_rate_below_threshold"
	default:
		return ""
	}
}

func modelGatewayRiskStatusForRejectReason(reason string) string {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case strings.Contains(normalized, "circuit"):
		return "circuit_open"
	case strings.Contains(normalized, "cooldown"):
		return "cooldown"
	case strings.Contains(normalized, "avoid"):
		return "failure_avoidance"
	case strings.Contains(normalized, "concurrency") || strings.Contains(normalized, "queue"):
		return "saturated"
	case strings.Contains(normalized, "fallback"):
		return "fallback"
	default:
		return "candidate_rejected"
	}
}

func modelGatewayRiskStatusForCircuitErrorType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case modelgatewayscheduler.CircuitErrorStreamInterrupted:
		return "stream_interrupted"
	case modelgatewayscheduler.CircuitErrorRateLimit, modelgatewayscheduler.CircuitErrorServer, modelgatewayscheduler.CircuitErrorUpstream:
		return "circuit_error"
	default:
		return "candidate_rejected"
	}
}

func modelGatewayRiskSeverityForStatus(status string) string {
	switch status {
	case "stream_interrupted", "failure_spike", "circuit_open":
		return "critical"
	case "fallback", "candidate_rejected", "queued", "saturated", "cooldown", "failure_avoidance", "degraded", "circuit_error":
		return "warning"
	default:
		return "info"
	}
}

func finalizeModelGatewayRiskTimeline(events []ModelGatewayRiskEvent, limit int) []ModelGatewayRiskEvent {
	if len(events) == 0 {
		return []ModelGatewayRiskEvent{}
	}
	filtered := make([]ModelGatewayRiskEvent, 0, len(events))
	for _, event := range events {
		if event.Count <= 0 {
			event.Count = 1
		}
		if strings.TrimSpace(event.Status) == "" || strings.TrimSpace(event.EventType) == "" {
			continue
		}
		if strings.TrimSpace(event.Severity) == "" {
			event.Severity = modelGatewayRiskSeverityForStatus(event.Status)
		}
		filtered = append(filtered, event)
	}
	sortModelGatewayRiskEvents(filtered)
	return limitModelGatewayRiskEvents(filtered, limit)
}

func limitModelGatewayRiskEvents(events []ModelGatewayRiskEvent, limit int) []ModelGatewayRiskEvent {
	if len(events) == 0 {
		return nil
	}
	if limit > 0 && len(events) > limit {
		return append([]ModelGatewayRiskEvent(nil), events[:limit]...)
	}
	return append([]ModelGatewayRiskEvent(nil), events...)
}

func sortModelGatewayRiskEvents(events []ModelGatewayRiskEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp != events[j].Timestamp {
			return events[i].Timestamp > events[j].Timestamp
		}
		leftSeverity := modelGatewayRiskSeverityRank(events[i].Severity)
		rightSeverity := modelGatewayRiskSeverityRank(events[j].Severity)
		if leftSeverity != rightSeverity {
			return leftSeverity > rightSeverity
		}
		if events[i].Count != events[j].Count {
			return events[i].Count > events[j].Count
		}
		if events[i].Status != events[j].Status {
			return events[i].Status < events[j].Status
		}
		return events[i].EventType < events[j].EventType
	})
}

func finalizeModelGatewayRiskStatusCounts(events []ModelGatewayRiskEvent, topN int) []ModelGatewayRiskStatusCount {
	if len(events) == 0 {
		return nil
	}
	counts := make(map[string]int64)
	severities := make(map[string]string)
	for _, event := range events {
		status := strings.TrimSpace(event.Status)
		if status == "" {
			continue
		}
		count := event.Count
		if count <= 0 {
			count = 1
		}
		counts[status] += count
		severities[status] = modelGatewayRiskSeverityForStatus(status)
		if event.Severity != "" && modelGatewayRiskSeverityRank(event.Severity) > modelGatewayRiskSeverityRank(severities[status]) {
			severities[status] = event.Severity
		}
	}
	items := make([]ModelGatewayRiskStatusCount, 0, len(counts))
	for status, count := range counts {
		if count <= 0 {
			continue
		}
		items = append(items, ModelGatewayRiskStatusCount{
			Status:   status,
			Severity: severities[status],
			Count:    count,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		leftSeverity := modelGatewayRiskSeverityRank(items[i].Severity)
		rightSeverity := modelGatewayRiskSeverityRank(items[j].Severity)
		if leftSeverity != rightSeverity {
			return leftSeverity > rightSeverity
		}
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Status < items[j].Status
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func modelGatewayRiskSeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func modelGatewayObservabilityAccumulatorFor(accumulators map[string]*modelGatewayObservabilityAccumulator, key string, name string, channelID int) *modelGatewayObservabilityAccumulator {
	if key == "" {
		key = "unknown"
	}
	if existing, ok := accumulators[key]; ok {
		if existing.Name == "" && name != "" {
			existing.Name = name
		}
		if existing.ChannelID == 0 && channelID > 0 {
			existing.ChannelID = channelID
		}
		return existing
	}
	accumulator := newModelGatewayObservabilityAccumulator(key, name, channelID)
	accumulators[key] = accumulator
	return accumulator
}

func modelGatewayObservabilityTrendAccumulatorFor(accumulators map[int64]*modelGatewayObservabilityTrendAccumulator, bucketStart int64) *modelGatewayObservabilityTrendAccumulator {
	if existing, ok := accumulators[bucketStart]; ok {
		return existing
	}
	accumulator := &modelGatewayObservabilityTrendAccumulator{
		modelGatewayObservabilityAccumulator: newModelGatewayObservabilityAccumulator(strconv.FormatInt(bucketStart, 10), "", 0),
		ProviderProfileAccumulators:          make(map[string]*modelGatewayObservabilityAccumulator),
		ProxyModeAccumulators:                make(map[string]*modelGatewayObservabilityAccumulator),
		RejectReasons:                        make(map[string]int64),
		CircuitOpenReasons:                   make(map[string]int64),
		CircuitErrorTypes:                    make(map[string]int64),
	}
	accumulators[bucketStart] = accumulator
	return accumulator
}

func applyModelGatewayObservabilityTrendRecord(accumulator *modelGatewayObservabilityTrendAccumulator, record model.ModelExecutionRecord, scoreBreakdown map[string]float64, requestMeta map[string]any, scoreErr bool, profileProxyMeta modelGatewayProfileProxyMeta) {
	if accumulator == nil {
		return
	}
	applyModelGatewayObservabilityRecord(accumulator.modelGatewayObservabilityAccumulator, record, scoreBreakdown, requestMeta, scoreErr)
	applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(accumulator.ProviderProfileAccumulators, profileProxyMeta.ProviderProfile, "", 0), record, scoreBreakdown, requestMeta, scoreErr)
	applyModelGatewayObservabilityRecord(modelGatewayObservabilityAccumulatorFor(accumulator.ProxyModeAccumulators, profileProxyMeta.ProxyMode, "", 0), record, scoreBreakdown, requestMeta, scoreErr)
	for _, reason := range modelGatewayRejectReasonsFromRequestMeta(requestMeta) {
		accumulator.RejectReasons[reason]++
		if modelGatewayRiskStatusForRejectReason(reason) == "circuit_open" {
			accumulator.CircuitOpenReasons[reason]++
		}
	}
	if kind := modelGatewayCircuitErrorTypeFromRecord(record); kind != "" {
		accumulator.CircuitErrorTypes[kind]++
	}
}

func modelGatewayCircuitErrorTypeFromRecord(record model.ModelExecutionRecord) string {
	if !isModelGatewayAttemptRecord(record) || record.Success {
		return ""
	}
	result := modelgatewaycore.AttemptResult{
		ChannelID:         record.ChannelId,
		RequestedGroup:    record.RequestedGroup,
		SelectedGroup:     record.SelectedGroup,
		ModelName:         record.RequestedModel,
		EndpointType:      constantEndpointTypeFromString(record.EndpointType),
		Success:           record.Success,
		StatusCode:        record.StatusCode,
		ErrorCode:         record.ErrorCode,
		ErrorType:         record.ErrorType,
		StreamInterrupted: record.StreamInterrupted,
	}
	return modelgatewayscheduler.ClassifyCircuitError(result)
}

func constantEndpointTypeFromString(value string) constant.EndpointType {
	return constant.EndpointType(strings.TrimSpace(value))
}

func finalizeModelGatewayTrendReasonCounts(counts map[string]int64, topN int) []ModelGatewayTrendReasonCount {
	if len(counts) == 0 {
		return nil
	}
	items := make([]ModelGatewayTrendReasonCount, 0, len(counts))
	for reason, count := range counts {
		if strings.TrimSpace(reason) == "" || count <= 0 {
			continue
		}
		items = append(items, ModelGatewayTrendReasonCount{
			Reason: reason,
			Count:  count,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Reason < items[j].Reason
	})
	if topN > 0 && len(items) > topN {
		return items[:topN]
	}
	return items
}

func newModelGatewayObservabilityAccumulator(key string, name string, channelID int) *modelGatewayObservabilityAccumulator {
	return &modelGatewayObservabilityAccumulator{
		ModelGatewayObservabilityAggregate: ModelGatewayObservabilityAggregate{
			Key:       key,
			Name:      name,
			ChannelID: channelID,
		},
	}
}

func parseModelGatewayScoreBreakdown(raw string) (map[string]float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	values := make(map[string]float64)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func parseModelGatewayCandidateGroups(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	values := make([]string, 0)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func parseModelGatewayRequestMeta(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	values := make(map[string]any)
	if err := common.UnmarshalJsonStr(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func modelGatewayObservabilityTrendBucketSeconds(hours int, overrideSeconds int64) int64 {
	if overrideSeconds > 0 {
		return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, overrideSeconds)
	}
	if hours <= 6 {
		return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, 3600)
	}
	if hours <= 72 {
		return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, 6*3600)
	}
	return modelGatewayObservabilityNormalizeTrendBucketSeconds(hours, 24*3600)
}

func modelGatewayObservabilityNormalizeTrendBucketSeconds(hours int, bucketSeconds int64) int64 {
	if bucketSeconds < modelGatewayObservabilityMinTrendBucket {
		bucketSeconds = modelGatewayObservabilityMinTrendBucket
	}
	if bucketSeconds > modelGatewayObservabilityMaxTrendBucket {
		bucketSeconds = modelGatewayObservabilityMaxTrendBucket
	}
	windowSeconds := int64(hours) * 3600
	if windowSeconds <= 0 {
		return bucketSeconds
	}
	minBucketForWindow := (windowSeconds + modelGatewayObservabilityMaxTrendBuckets - 1) / modelGatewayObservabilityMaxTrendBuckets
	if bucketSeconds < minBucketForWindow {
		bucketSeconds = minBucketForWindow
	}
	return bucketSeconds
}

func modelGatewayObservabilityTrendBucketStart(createdAt int64, startTime int64, endTime int64, bucketSeconds int64) (int64, bool) {
	if createdAt < startTime || endTime <= startTime || bucketSeconds <= 0 {
		return 0, false
	}
	if createdAt >= endTime {
		createdAt = endTime - 1
	}
	if createdAt < startTime {
		return 0, false
	}
	return startTime + ((createdAt-startTime)/bucketSeconds)*bucketSeconds, true
}

func averageModelGatewayObservabilityScoreBreakdown(accumulator *modelGatewayObservabilityAccumulator) map[string]float64 {
	if accumulator == nil || accumulator.ScoreBreakdownSamples == 0 || len(accumulator.scoreSums) == 0 {
		return nil
	}
	average := make(map[string]float64, len(accumulator.scoreSums))
	for key, value := range accumulator.scoreSums {
		average[key] = roundModelGatewayObservabilityFloat(value / float64(accumulator.ScoreBreakdownSamples))
	}
	return average
}

func modelGatewayObservabilityModelKey(record model.ModelExecutionRecord) string {
	if record.RequestedModel != "" {
		return record.RequestedModel
	}
	return "unknown"
}

func modelGatewayObservabilityGroupKey(record model.ModelExecutionRecord) string {
	if record.ActualGroup != "" {
		return record.ActualGroup
	}
	if record.SelectedGroup != "" {
		return record.SelectedGroup
	}
	if record.RequestedGroup != "" {
		return record.RequestedGroup
	}
	return "unknown"
}

func modelGatewayObservabilityChannel(record model.ModelExecutionRecord) (int, string) {
	if record.ActualChannelId > 0 {
		return record.ActualChannelId, record.ActualChannelName
	}
	if record.ChannelId > 0 {
		return record.ChannelId, record.ChannelName
	}
	return 0, ""
}

func modelGatewayObservabilityProviderProfileKey(requestMeta map[string]any) string {
	key := strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["provider_profile"]))
	if key == "" {
		return "unknown"
	}
	return key
}

func modelGatewayObservabilityProxyModeKey(requestMeta map[string]any) string {
	key := strings.TrimSpace(modelGatewayObservabilityMetaString(requestMeta["proxy_mode"]))
	if key == "" {
		return "unknown"
	}
	return key
}

func modelGatewayProfileProxyMetaForRecord(record model.ModelExecutionRecord, requestMeta map[string]any, dispatchMetaByRequestID map[string]modelGatewayProfileProxyMeta) modelGatewayProfileProxyMeta {
	meta := modelGatewayProfileProxyMeta{
		ProviderProfile: modelGatewayObservabilityProviderProfileKey(requestMeta),
		ProxyMode:       modelGatewayObservabilityProxyModeKey(requestMeta),
	}
	if meta.ProviderProfile != "unknown" || meta.ProxyMode != "unknown" {
		return meta
	}
	if strings.TrimSpace(record.RequestId) == "" {
		return meta
	}
	if dispatchMeta, ok := dispatchMetaByRequestID[record.RequestId]; ok {
		if meta.ProviderProfile == "unknown" {
			meta.ProviderProfile = dispatchMeta.ProviderProfile
		}
		if meta.ProxyMode == "unknown" {
			meta.ProxyMode = dispatchMeta.ProxyMode
		}
	}
	return meta
}

func modelGatewayObservabilityRecordKind(record model.ModelExecutionRecord) string {
	if isModelGatewayAttemptRecord(record) {
		return "attempt"
	}
	if isModelGatewayDispatchRecord(record) {
		return "dispatch"
	}
	return "record"
}

func isModelGatewayDispatchRecord(record model.ModelExecutionRecord) bool {
	return record.SmartHandled ||
		record.PolicyMode != "" ||
		record.AutoMode != "" ||
		record.Strategy != "" ||
		record.ScoreTotal > 0 ||
		strings.TrimSpace(record.ScoreBreakdown) != "" ||
		strings.TrimSpace(record.CandidateGroups) != ""
}

func isModelGatewayAttemptRecord(record model.ModelExecutionRecord) bool {
	return record.Success ||
		record.StreamInterrupted ||
		record.StatusCode != 0 ||
		record.ErrorCode != "" ||
		record.ErrorType != "" ||
		record.DurationMs != 0 ||
		record.TTFTMs != 0
}

func hasModelGatewayScoreTotalSample(record model.ModelExecutionRecord) bool {
	return record.ScoreTotal != 0 ||
		strings.TrimSpace(record.ScoreBreakdown) != "" ||
		record.SelectedReason != ""
}

func successRateModelGatewayObservability(successes int64, attempts int64) float64 {
	if attempts <= 0 {
		return 0
	}
	return roundModelGatewayObservabilityFloat(float64(successes) / float64(attempts))
}

func averageInt64(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return int64(math.Round(float64(sum) / float64(count)))
}

func percentileModelGatewayObservabilityInt64(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sortedValues := append([]int64(nil), values...)
	sort.Slice(sortedValues, func(i, j int) bool {
		return sortedValues[i] < sortedValues[j]
	})
	if percentile <= 0 {
		return sortedValues[0]
	}
	if percentile >= 1 {
		return sortedValues[len(sortedValues)-1]
	}
	index := int(math.Ceil(percentile*float64(len(sortedValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

func averageFloat64(sum float64, count int64) float64 {
	if count <= 0 {
		return 0
	}
	return roundModelGatewayObservabilityFloat(sum / float64(count))
}

func roundModelGatewayObservabilityFloat(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func clampModelGatewayObservabilityValue(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeModelGatewayObservabilityScanLimit(value int) int {
	if value <= 0 {
		return modelGatewayObservabilityDefaultScanLimit
	}
	return clampModelGatewayObservabilityValue(value, 1, modelGatewayObservabilityMaxScanLimit)
}

func minModelGatewayObservabilityInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
