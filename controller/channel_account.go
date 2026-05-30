package controller

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/gin-gonic/gin"
)

const channelAccountManualDisabledReason = "manual_disabled"
const channelAccountAllKeysDisabledReason = "All keys are disabled"
const channelAccountEmptyCodexReason = channelAccountAllKeysDisabledReason
const channelAccountImportMaxFileBytes int64 = 32 << 20
const channelAccountImportMaxZipEntryBytes int64 = 4 << 20
const channelAccountImportMaxZipEntries = 1000
const channelAccountDefaultPageSize = 20
const channelAccountMaxPageSize = 100

const (
	channelAccountViewManage = "manage"
	channelAccountViewStats  = "stats"
)

type ChannelAccountsResponse struct {
	ChannelID     int                          `json:"channel_id"`
	ChannelName   string                       `json:"channel_name,omitempty"`
	ResourceRef   modelgatewaycore.ResourceRef `json:"resource_ref"`
	View          string                       `json:"view,omitempty"`
	Page          int                          `json:"page,omitempty"`
	PageSize      int                          `json:"page_size,omitempty"`
	Total         int                          `json:"total"`
	FilteredTotal int                          `json:"filtered_total"`
	Enabled       int                          `json:"enabled"`
	Disabled      int                          `json:"disabled"`
	Items         []ChannelAccountItem         `json:"items"`
	Summary       ChannelAccountSummary        `json:"summary"`
	Operation     *ChannelAccountOperation     `json:"operation,omitempty"`
}

type ChannelAccountItem struct {
	ChannelID       int                                  `json:"channel_id"`
	CredentialIndex int                                  `json:"credential_index"`
	KeyEnabled      bool                                 `json:"key_enabled"`
	DisabledReason  string                               `json:"disabled_reason,omitempty"`
	ResourceRef     modelgatewaycore.ResourceRef         `json:"resource_ref"`
	AccountIdentity modelgatewaycore.AccountIdentity     `json:"account_identity"`
	CredentialRef   modelgatewaycore.CredentialRef       `json:"credential_ref"`
	Proxy           *ModelGatewayProxyResponse           `json:"proxy,omitempty"`
	Capabilities    *model.ChannelAccountCapability      `json:"capabilities,omitempty"`
	SubjectShort    string                               `json:"subject_short,omitempty"`
	CredentialShort string                               `json:"credential_short,omitempty"`
	Score           *ChannelAccountScoreSummary          `json:"score,omitempty"`
	RuntimeKeys     []ChannelAccountRuntimeScoreSnapshot `json:"runtime_keys,omitempty"`
	Stats           *ChannelAccountStats                 `json:"stats,omitempty"`
	Scheduling      *ChannelAccountSchedulingExplanation `json:"scheduling,omitempty"`
}

type ChannelAccountSummary struct {
	Scored              int64                             `json:"scored"`
	RealSampleCount30m  int64                             `json:"real_sample_count_30m"`
	Today               ChannelAccountUsageWindowResponse `json:"today"`
	Last5h              ChannelAccountUsageWindowResponse `json:"last_5h"`
	Last7d              ChannelAccountUsageWindowResponse `json:"last_7d"`
	HealthProbeExcluded bool                              `json:"health_probe_excluded"`
}

type ChannelAccountStats struct {
	Today              ChannelAccountUsageWindowResponse `json:"today"`
	Last5h             ChannelAccountUsageWindowResponse `json:"last_5h"`
	Last7d             ChannelAccountUsageWindowResponse `json:"last_7d"`
	LastActiveAt       int64                             `json:"last_active_at,omitempty"`
	MainErrorCategory  string                            `json:"main_error_category,omitempty"`
	ProbeRecoveryState *ChannelAccountProbeRecoveryState `json:"probe_recovery_state,omitempty"`
}

type ChannelAccountUsageWindowResponse struct {
	Requests          int64   `json:"requests"`
	SuccessRequests   int64   `json:"success_requests"`
	ErrorRequests     int64   `json:"error_requests"`
	TimeoutRequests   int64   `json:"timeout_requests"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	Quota             int64   `json:"quota"`
	UpstreamCostTotal float64 `json:"upstream_cost_total"`
	SuccessRate       float64 `json:"success_rate"`
	ErrorRate         float64 `json:"error_rate"`
	TimeoutRate       float64 `json:"timeout_rate"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	AvgTTFTMs         float64 `json:"avg_ttft_ms"`
	LastActiveAt      int64   `json:"last_active_at,omitempty"`
	TopErrorCategory  string  `json:"top_error_category,omitempty"`
	TopErrorCount     int64   `json:"top_error_count,omitempty"`
}

type ChannelAccountProbeRecoveryState struct {
	Pending      bool   `json:"pending,omitempty"`
	Reason       string `json:"reason,omitempty"`
	SuccessCount int    `json:"success_count,omitempty"`
	Required     int    `json:"required,omitempty"`
}

type ChannelAccountSchedulingExplanation struct {
	Schedulable              bool     `json:"schedulable"`
	PrimaryReason            string   `json:"primary_reason,omitempty"`
	BlockingReasons          []string `json:"blocking_reasons,omitempty"`
	WarningReasons           []string `json:"warning_reasons,omitempty"`
	Detail                   string   `json:"detail,omitempty"`
	RecoveryAt               int64    `json:"recovery_at,omitempty"`
	RecoverySource           string   `json:"recovery_source,omitempty"`
	ProbeRecoveryPending     bool     `json:"probe_recovery_pending,omitempty"`
	ProbeRecoverySuccesses   int      `json:"probe_recovery_successes,omitempty"`
	ProbeRecoveryRequired    int      `json:"probe_recovery_required,omitempty"`
	ActiveConcurrency        int      `json:"active_concurrency,omitempty"`
	EffectiveConcurrencyCap  int      `json:"effective_concurrency_limit,omitempty"`
	QueueDepth               int      `json:"queue_depth,omitempty"`
	QueueCapacity            int      `json:"queue_capacity,omitempty"`
	CapabilityClassification string   `json:"capability_classification,omitempty"`
}

type ChannelAccountRecentRequestsResponse struct {
	ChannelID        int                                `json:"channel_id"`
	CredentialIndex  int                                `json:"credential_index"`
	Items            []ChannelAccountRecentRequestItem  `json:"items"`
	RefreshResult    *ChannelAccountAttributionResponse `json:"refresh_result,omitempty"`
	AttributionSince int64                              `json:"attribution_since,omitempty"`
}

type ChannelAccountRecentRequestItem struct {
	RequestID                string  `json:"request_id"`
	AttemptIndex             int     `json:"attempt_index"`
	ChannelID                int     `json:"channel_id"`
	CredentialIndex          int     `json:"credential_index"`
	AccountDisplayIndex      int     `json:"account_display_index"`
	AccountIdentityKey       string  `json:"account_identity_key,omitempty"`
	RequestedModel           string  `json:"requested_model,omitempty"`
	RequestedGroup           string  `json:"requested_group,omitempty"`
	SelectedGroup            string  `json:"selected_group,omitempty"`
	EndpointType             string  `json:"endpoint_type,omitempty"`
	CompletedAt              int64   `json:"completed_at,omitempty"`
	Success                  bool    `json:"success"`
	StatusCode               int     `json:"status_code,omitempty"`
	ErrorCategory            string  `json:"error_category,omitempty"`
	IsHealthProbe            bool    `json:"is_health_probe"`
	DurationMs               int64   `json:"duration_ms,omitempty"`
	TTFTMs                   int64   `json:"ttft_ms,omitempty"`
	TotalTokens              int64   `json:"total_tokens,omitempty"`
	Quota                    int64   `json:"quota,omitempty"`
	UpstreamCostTotal        float64 `json:"upstream_cost_total,omitempty"`
	StatisticsRecorded       bool    `json:"statistics_recorded"`
	BillingRecorded          bool    `json:"billing_recorded"`
	CostRecorded             bool    `json:"cost_recorded"`
	AttributionComplete      bool    `json:"attribution_complete"`
	StatisticsStatus         string  `json:"statistics_status,omitempty"`
	StatisticsDiagnostic     string  `json:"statistics_diagnostic,omitempty"`
	UsageEstimated           bool    `json:"usage_estimated,omitempty"`
	ProviderSurface          string  `json:"provider_surface,omitempty"`
	CapabilityClassification string  `json:"capability_classification,omitempty"`
}

type ChannelAccountAttributionResponse struct {
	Since   int64 `json:"since"`
	Scanned int   `json:"scanned"`
	Updated int   `json:"updated"`
	Skipped int   `json:"skipped"`
}

type ChannelAccountRequestReconcileResponse struct {
	RequestID           string                                    `json:"request_id"`
	ChannelID           int                                       `json:"channel_id"`
	CredentialIndex     int                                       `json:"credential_index"`
	AccountDisplayIndex int                                       `json:"account_display_index"`
	Diagnoses           []ChannelAccountRequestReconcileDiagnosis `json:"diagnoses"`
	UsageEvent          *ChannelAccountRecentRequestItem          `json:"usage_event,omitempty"`
	UserRequest         *ChannelAccountUserRequestReconcile       `json:"user_request,omitempty"`
	ExecutionRecords    []ChannelAccountExecutionRecordReconcile  `json:"execution_records"`
	ScoreEvents         []ChannelAccountScoreEventReconcile       `json:"score_events"`
	CostSummary         *ChannelAccountRequestCostReconcile       `json:"cost_summary,omitempty"`
	Checks              []ChannelAccountRequestReconcileCheck     `json:"checks"`
}

type ChannelAccountUserRequestReconcile struct {
	CompletedAt        int64  `json:"completed_at,omitempty"`
	RequestedModel     string `json:"requested_model,omitempty"`
	RequestedGroup     string `json:"requested_group,omitempty"`
	SelectedGroup      string `json:"selected_group,omitempty"`
	FinalChannelID     int    `json:"final_channel_id,omitempty"`
	FinalChannelName   string `json:"final_channel_name,omitempty"`
	Attempts           int    `json:"attempts,omitempty"`
	LastAttemptIndex   int    `json:"last_attempt_index,omitempty"`
	FinalSuccess       bool   `json:"final_success"`
	Recovered          bool   `json:"recovered"`
	FinalStatusCode    int    `json:"final_status_code,omitempty"`
	FinalErrorCategory string `json:"final_error_category,omitempty"`
	IsHealthProbe      bool   `json:"is_health_probe"`
	ProbeReason        string `json:"probe_reason,omitempty"`
	DurationMs         int64  `json:"duration_ms,omitempty"`
	TTFTMs             int64  `json:"ttft_ms,omitempty"`
}

type ChannelAccountExecutionRecordReconcile struct {
	CreatedAt       int64   `json:"created_at,omitempty"`
	AttemptIndex    int     `json:"attempt_index"`
	ChannelID       int     `json:"channel_id,omitempty"`
	ChannelName     string  `json:"channel_name,omitempty"`
	ActualChannelID int     `json:"actual_channel_id,omitempty"`
	Success         bool    `json:"success"`
	StatusCode      int     `json:"status_code,omitempty"`
	ErrorCategory   string  `json:"error_category,omitempty"`
	DurationMs      int64   `json:"duration_ms,omitempty"`
	TTFTMs          int64   `json:"ttft_ms,omitempty"`
	SmartHandled    bool    `json:"smart_handled"`
	FallbackUsed    bool    `json:"fallback_used"`
	ScoreTotal      float64 `json:"score_total,omitempty"`
	SelectedReason  string  `json:"selected_reason,omitempty"`
}

type ChannelAccountScoreEventReconcile struct {
	CreatedAt           int64   `json:"created_at,omitempty"`
	AttemptIndex        int     `json:"attempt_index"`
	ChannelID           int     `json:"channel_id,omitempty"`
	CredentialIndex     int     `json:"credential_index"`
	AccountDisplayIndex int     `json:"account_display_index"`
	BeforeTotal         float64 `json:"before_total,omitempty"`
	AfterTotal          float64 `json:"after_total,omitempty"`
	Delta               float64 `json:"delta,omitempty"`
	FailureScope        string  `json:"failure_scope,omitempty"`
	SwitchReason        string  `json:"switch_reason,omitempty"`
	RequestedModel      string  `json:"requested_model,omitempty"`
	Group               string  `json:"group,omitempty"`
	IsHealthProbe       bool    `json:"is_health_probe"`
}

type ChannelAccountRequestCostReconcile struct {
	UpstreamModel     string  `json:"upstream_model,omitempty"`
	UpstreamCostTotal float64 `json:"upstream_cost_total,omitempty"`
	CostSource        string  `json:"cost_source,omitempty"`
	CostAccuracy      string  `json:"cost_accuracy,omitempty"`
	CalculatedAt      int64   `json:"calculated_at,omitempty"`
}

type ChannelAccountRequestReconcileCheck struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type ChannelAccountRequestReconcileDiagnosis struct {
	Key      string `json:"key"`
	Severity string `json:"severity"`
}

type channelAccountsQuery struct {
	View     string
	Page     int
	PageSize int
	Keyword  string
	Status   string
	Sort     string
	Order    string
}

type UpdateChannelAccountStatusRequest struct {
	Enabled *bool  `json:"enabled"`
	Action  string `json:"action,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type UpdateChannelAccountCredentialRequest struct {
	Credential     string `json:"credential"`
	CredentialType string `json:"credential_type,omitempty"`
}

type UpdateChannelAccountsStatusRequest struct {
	Enabled           *bool  `json:"enabled"`
	Action            string `json:"action,omitempty"`
	Reason            string `json:"reason,omitempty"`
	CredentialIndexes []int  `json:"credential_indexes"`
}

type ImportChannelAccountsRequest struct {
	Credentials    string   `json:"credentials"`
	CredentialList []string `json:"credential_list,omitempty"`
	OnlyNew        bool     `json:"only_new"`
}

type ChannelAccountImportParser struct {
	context     *gin.Context
	maxFileSize int64
	maxZipEntry int64
	maxZipFiles int
}

type DeleteChannelAccountsRequest struct {
	CredentialIndexes []int `json:"credential_indexes"`
}

type UpdateChannelAccountProxyRequest struct {
	ProxyID        *int `json:"proxy_id"`
	AllowReuseRisk bool `json:"allow_reuse_risk,omitempty"`
}

type UpdateChannelAccountsProxyRequest struct {
	CredentialIndexes []int `json:"credential_indexes"`
	ProxyID           *int  `json:"proxy_id"`
	AllowReuseRisk    bool  `json:"allow_reuse_risk,omitempty"`
}

type ChannelAccountOperation struct {
	Type             string `json:"type,omitempty"`
	Action           string `json:"action,omitempty"`
	Requested        int    `json:"requested,omitempty"`
	Affected         int    `json:"affected,omitempty"`
	Added            int    `json:"added,omitempty"`
	Deleted          int    `json:"deleted,omitempty"`
	Skipped          int    `json:"skipped,omitempty"`
	SkippedExisting  int    `json:"skipped_existing,omitempty"`
	SkippedDuplicate int    `json:"skipped_duplicate,omitempty"`
	TotalInput       int    `json:"total_input,omitempty"`
	ChannelRestored  bool   `json:"channel_restored,omitempty"`
	ChannelDisabled  bool   `json:"channel_disabled,omitempty"`
}

type ChannelAccountScoreSummary struct {
	RuntimeKey                modelgatewaycore.RuntimeKey `json:"runtime_key"`
	HealthStatus              string                      `json:"health_status,omitempty"`
	ScoreTotal                float64                     `json:"score_total,omitempty"`
	RoutingScoreTotal         float64                     `json:"routing_score_total,omitempty"`
	CostItemScore             float64                     `json:"cost_item_score,omitempty"`
	CostRatio                 float64                     `json:"cost_ratio,omitempty"`
	CostReferenceRatio        float64                     `json:"cost_reference_ratio,omitempty"`
	CostPricingMode           string                      `json:"cost_pricing_mode,omitempty"`
	SampleCount               int                         `json:"sample_count"`
	RealSampleCount30m        int                         `json:"real_sample_count_30m,omitempty"`
	SuccessRate               float64                     `json:"success_rate"`
	TTFTMs                    float64                     `json:"ttft_ms"`
	DurationMs                float64                     `json:"duration_ms"`
	TokensPerSecond           float64                     `json:"tokens_per_second,omitempty"`
	EmptyOutputRate           float64                     `json:"empty_output_rate,omitempty"`
	ExperienceIssueRate       float64                     `json:"experience_issue_rate,omitempty"`
	LastRealAttemptAt         int64                       `json:"last_real_attempt_at,omitempty"`
	LastRealSuccessAt         int64                       `json:"last_real_success_at,omitempty"`
	LastRealFailureAt         int64                       `json:"last_real_failure_at,omitempty"`
	LastProbeAt               int64                       `json:"last_probe_at,omitempty"`
	LastProbeSuccessAt        int64                       `json:"last_probe_success_at,omitempty"`
	ConfigErrorIsolated       bool                        `json:"config_error_isolated,omitempty"`
	IsolationReason           string                      `json:"isolation_reason,omitempty"`
	ProbeRecoveryPending      bool                        `json:"probe_recovery_pending,omitempty"`
	ProbeTriggerReason        string                      `json:"probe_trigger_reason,omitempty"`
	ProbeRecoverySuccessCount int                         `json:"probe_recovery_success_count,omitempty"`
	ProbeRecoveryRequired     int                         `json:"probe_recovery_required,omitempty"`
	ActiveConcurrency         int                         `json:"active_concurrency,omitempty"`
	EffectiveConcurrencyCap   int                         `json:"effective_concurrency_limit,omitempty"`
	QueueDepth                int                         `json:"queue_depth,omitempty"`
	QueueCapacity             int                         `json:"queue_capacity,omitempty"`
}

type ChannelAccountRuntimeScoreSnapshot struct {
	RuntimeKey          modelgatewaycore.RuntimeKey `json:"runtime_key"`
	HealthStatus        string                      `json:"health_status,omitempty"`
	ScoreTotal          float64                     `json:"score_total,omitempty"`
	RoutingScoreTotal   float64                     `json:"routing_score_total,omitempty"`
	SampleCount         int                         `json:"sample_count"`
	RealSampleCount30m  int                         `json:"real_sample_count_30m,omitempty"`
	SuccessRate         float64                     `json:"success_rate"`
	TTFTMs              float64                     `json:"ttft_ms"`
	LastRealSuccessAt   int64                       `json:"last_real_success_at,omitempty"`
	LastProbeSuccessAt  int64                       `json:"last_probe_success_at,omitempty"`
	ConfigErrorIsolated bool                        `json:"config_error_isolated,omitempty"`
}

func ListChannelAccounts(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}

	common.ApiSuccess(c, buildChannelAccountsResponse(channel, parseChannelAccountsQuery(c)))
}

func ListChannelAccountRecentRequests(c *gin.Context) {
	channelID, credentialIndex, ok := parseChannelAccountRequestTarget(c)
	if !ok {
		return
	}
	rows, err := model.QueryChannelAccountUsageRecentEvents(channelID, credentialIndex, 10)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ChannelAccountRecentRequestsResponse{
		ChannelID:       channelID,
		CredentialIndex: credentialIndex,
		Items:           buildChannelAccountRecentRequestItems(rows),
	})
}

func RefreshChannelAccountUsageAttribution(c *gin.Context) {
	channelID, credentialIndex, ok := parseChannelAccountRequestTarget(c)
	if !ok {
		return
	}
	since := common.GetTimestamp() - 6*60*60
	result, err := model.RefreshChannelAccountUsageAttribution(channelID, credentialIndex, since, 500)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows, err := model.QueryChannelAccountUsageRecentEvents(channelID, credentialIndex, 10)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ChannelAccountRecentRequestsResponse{
		ChannelID:       channelID,
		CredentialIndex: credentialIndex,
		Items:           buildChannelAccountRecentRequestItems(rows),
		RefreshResult: &ChannelAccountAttributionResponse{
			Since:   result.Since,
			Scanned: result.Scanned,
			Updated: result.Updated,
			Skipped: result.Skipped,
		},
		AttributionSince: result.Since,
	})
}

func GetChannelAccountRequestReconcile(c *gin.Context) {
	channelID, credentialIndex, ok := parseChannelAccountRequestTarget(c)
	if !ok {
		return
	}
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		common.ApiErrorMsg(c, "请求 ID 不能为空")
		return
	}
	response, err := buildChannelAccountRequestReconcileResponse(channelID, credentialIndex, requestID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func UpdateChannelAccountStatus(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	credentialIndex, ok := parseChannelAccountCredentialIndexParam(c)
	if !ok {
		return
	}

	var request UpdateChannelAccountStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	enabled, ok := request.TargetEnabled()
	if !ok {
		common.ApiErrorMsg(c, "账号状态参数无效")
		return
	}

	operation, err := updateChannelAccountStatus(channelID, credentialIndex, enabled, request.Reason)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func UpdateChannelAccountCredential(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	credentialIndex, ok := parseChannelAccountCredentialIndexParam(c)
	if !ok {
		return
	}

	var request UpdateChannelAccountCredentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}

	operation, err := updateChannelAccountCredential(channelID, credentialIndex, request.Credential, request.CredentialType)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func UpdateChannelAccountsStatus(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}

	var request UpdateChannelAccountsStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	enabled, ok := request.TargetEnabled()
	if !ok {
		common.ApiErrorMsg(c, "账号状态参数无效")
		return
	}

	operation, err := updateChannelAccountsStatus(channelID, request.CredentialIndexes, enabled, request.Reason)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func ImportChannelAccounts(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}

	request, err := NewChannelAccountImportParser(c).Parse()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	operation, err := importChannelAccounts(channelID, request.Credentials, request.CredentialList, request.OnlyNew)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func DeleteChannelAccounts(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}

	var request DeleteChannelAccountsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}

	operation, err := deleteChannelAccounts(channelID, request.CredentialIndexes)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func UpdateChannelAccountProxy(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	credentialIndex, ok := parseChannelAccountCredentialIndexParam(c)
	if !ok {
		return
	}
	var request UpdateChannelAccountProxyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	proxyID := 0
	if request.ProxyID != nil {
		proxyID = *request.ProxyID
	}
	operation, err := updateChannelAccountProxy(channelID, credentialIndex, proxyID, request.AllowReuseRisk)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func UpdateChannelAccountsProxy(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	var request UpdateChannelAccountsProxyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	proxyID := 0
	if request.ProxyID != nil {
		proxyID = *request.ProxyID
	}
	operation, err := updateChannelAccountsProxy(channelID, request.CredentialIndexes, proxyID, request.AllowReuseRisk)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	model.InitChannelCache()
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}
	common.ApiSuccess(c, buildChannelAccountsResponseWithOperation(channel, operation))
}

func (request UpdateChannelAccountStatusRequest) TargetEnabled() (bool, bool) {
	if request.Enabled != nil {
		return *request.Enabled, true
	}
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "enable", "enabled":
		return true, true
	case "disable", "disabled":
		return false, true
	default:
		return false, false
	}
}

func (request UpdateChannelAccountsStatusRequest) TargetEnabled() (bool, bool) {
	if request.Enabled != nil {
		return *request.Enabled, true
	}
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "enable", "enabled":
		return true, true
	case "disable", "disabled":
		return false, true
	default:
		return false, false
	}
}

func NewChannelAccountImportParser(c *gin.Context) *ChannelAccountImportParser {
	return &ChannelAccountImportParser{
		context:     c,
		maxFileSize: channelAccountImportMaxFileBytes,
		maxZipEntry: channelAccountImportMaxZipEntryBytes,
		maxZipFiles: channelAccountImportMaxZipEntries,
	}
}

func (parser *ChannelAccountImportParser) Parse() (ImportChannelAccountsRequest, error) {
	if parser == nil || parser.context == nil {
		return ImportChannelAccountsRequest{}, fmt.Errorf("导入请求无效")
	}
	if strings.HasPrefix(strings.ToLower(parser.context.GetHeader("Content-Type")), "multipart/") {
		return parser.parseMultipart()
	}

	var request ImportChannelAccountsRequest
	if err := parser.context.ShouldBindJSON(&request); err != nil {
		return request, err
	}
	return request, nil
}

func (parser *ChannelAccountImportParser) parseMultipart() (ImportChannelAccountsRequest, error) {
	form, err := parser.context.MultipartForm()
	if err != nil {
		return ImportChannelAccountsRequest{}, err
	}
	request := ImportChannelAccountsRequest{
		Credentials:    strings.Join(form.Value["credentials"], "\n"),
		CredentialList: append([]string{}, form.Value["credential_list"]...),
	}
	if values := form.Value["only_new"]; len(values) > 0 {
		request.OnlyNew = channelAccountImportBool(values[len(values)-1])
	}

	for _, files := range form.File {
		for _, fileHeader := range files {
			credentials, err := parser.credentialsFromUploadedFile(fileHeader)
			if err != nil {
				return request, err
			}
			request.CredentialList = append(request.CredentialList, credentials...)
		}
	}
	return request, nil
}

func (parser *ChannelAccountImportParser) credentialsFromUploadedFile(fileHeader *multipart.FileHeader) ([]string, error) {
	if fileHeader == nil {
		return nil, nil
	}
	if fileHeader.Size > parser.maxFileSize {
		return nil, fmt.Errorf("导入文件过大，最大支持 %d MB", parser.maxFileSize>>20)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := readChannelAccountImportBytes(file, parser.maxFileSize)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fileHeader.Filename, err)
	}
	name := strings.TrimSpace(fileHeader.Filename)
	if channelAccountImportLooksLikeZip(name, data) {
		return parser.credentialsFromZip(data, name)
	}
	return parser.credentialsFromPayload(data, name, false)
}

func (parser *ChannelAccountImportParser) credentialsFromZip(data []byte, sourceName string) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%s 不是有效的 ZIP 压缩包", sourceName)
	}
	isXAutoPackage := parser.zipLooksLikeXAutoPackage(reader)
	credentials := make([]string, 0)
	visited := 0
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		visited++
		if visited > parser.maxZipFiles {
			return nil, fmt.Errorf("压缩包文件数量过多，最多支持 %d 个文件", parser.maxZipFiles)
		}
		if !parser.shouldParseZipEntry(file.Name, isXAutoPackage) {
			continue
		}
		entryBytes, err := parser.readZipEntry(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, err)
		}
		entryCredentials, err := parser.credentialsFromPayload(entryBytes, file.Name, isXAutoPackage)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Name, err)
		}
		credentials = append(credentials, entryCredentials...)
	}
	if len(credentials) == 0 {
		if isXAutoPackage {
			return nil, fmt.Errorf("未在 xauto 导出包中找到可导入的账号凭证")
		}
		return nil, fmt.Errorf("压缩包中未找到可导入的账号凭证")
	}
	return credentials, nil
}

func (parser *ChannelAccountImportParser) zipLooksLikeXAutoPackage(reader *zip.Reader) bool {
	if reader == nil {
		return false
	}
	for _, file := range reader.File {
		if strings.EqualFold(path.Base(file.Name), "manifest.json") {
			data, err := parser.readZipEntry(file)
			if err != nil {
				return false
			}
			var payload map[string]interface{}
			if err := common.Unmarshal(data, &payload); err != nil {
				return false
			}
			packageType, _ := payload["type"].(string)
			return strings.EqualFold(strings.TrimSpace(packageType), "newapi-channel-files")
		}
	}
	return false
}

func (parser *ChannelAccountImportParser) shouldParseZipEntry(name string, isXAutoPackage bool) bool {
	baseName := strings.ToLower(path.Base(name))
	if baseName == "" || strings.HasPrefix(baseName, ".") || baseName == "manifest.json" {
		return false
	}
	if isXAutoPackage {
		return strings.HasSuffix(baseName, ".json")
	}
	return strings.HasSuffix(baseName, ".json") ||
		strings.HasSuffix(baseName, ".txt") ||
		strings.HasSuffix(baseName, ".ndjson")
}

func (parser *ChannelAccountImportParser) readZipEntry(file *zip.File) ([]byte, error) {
	if file == nil {
		return nil, fmt.Errorf("压缩包条目无效")
	}
	if int64(file.UncompressedSize64) > parser.maxZipEntry {
		return nil, fmt.Errorf("压缩包内文件过大，最大支持 %d MB", parser.maxZipEntry>>20)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readChannelAccountImportBytes(reader, parser.maxZipEntry)
}

func (parser *ChannelAccountImportParser) credentialsFromPayload(data []byte, sourceName string, xautoPackage bool) ([]string, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		credentials, ok, err := parser.credentialsFromJSONPayload([]byte(trimmed), xautoPackage)
		if err != nil {
			return nil, err
		}
		if ok {
			return credentials, nil
		}
		if xautoPackage || strings.HasSuffix(strings.ToLower(sourceName), ".json") {
			return nil, nil
		}
	}
	if xautoPackage {
		return nil, nil
	}
	return strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n"), nil
}

func (parser *ChannelAccountImportParser) credentialsFromJSONPayload(data []byte, xautoPackage bool) ([]string, bool, error) {
	var payload interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, false, err
	}
	credentials, ok, err := parser.credentialsFromJSONValue(payload, xautoPackage)
	return credentials, ok, err
}

func (parser *ChannelAccountImportParser) credentialsFromJSONValue(value interface{}, xautoPackage bool) ([]string, bool, error) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, false, nil
		}
		return []string{typed}, true, nil
	case []interface{}:
		credentials := make([]string, 0, len(typed))
		for _, item := range typed {
			itemCredentials, ok, err := parser.credentialsFromJSONValue(item, xautoPackage)
			if err != nil {
				return nil, false, err
			}
			if ok {
				credentials = append(credentials, itemCredentials...)
			}
		}
		return credentials, len(credentials) > 0, nil
	case map[string]interface{}:
		return parser.credentialsFromJSONObject(typed, xautoPackage)
	default:
		return nil, false, nil
	}
}

func (parser *ChannelAccountImportParser) credentialsFromJSONObject(payload map[string]interface{}, xautoPackage bool) ([]string, bool, error) {
	if key := channelAccountImportStringAtPath(payload, "channel", "key"); key != "" {
		return []string{key}, true, nil
	}
	if credentialItems, ok, err := parser.credentialsFromJSONField(payload["credential_list"], xautoPackage); err != nil || ok {
		return credentialItems, ok, err
	}
	if credentialItems, ok, err := parser.credentialsFromJSONField(payload["accounts"], xautoPackage); err != nil || ok {
		return credentialItems, ok, err
	}
	if credentialItems, ok, err := parser.credentialsFromJSONField(payload["credentials"], xautoPackage); err != nil || ok {
		return credentialItems, ok, err
	}
	if credential := channelAccountImportString(payload["credential"]); credential != "" {
		return []string{credential}, true, nil
	}
	if credential := channelAccountImportString(payload["key"]); credential != "" && channelAccountImportLooksLikeCredentialPayload(payload) {
		return []string{credential}, true, nil
	}
	if xautoPackage || channelAccountImportLooksLikeManifest(payload) {
		return nil, false, nil
	}
	compacted, err := common.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return []string{string(compacted)}, true, nil
}

func (parser *ChannelAccountImportParser) credentialsFromJSONField(value interface{}, xautoPackage bool) ([]string, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	if text := channelAccountImportString(value); text != "" {
		if parsed, ok := parseJSONCredentialInput(text); ok {
			return parsed, true, nil
		}
		return []string{text}, true, nil
	}
	return parser.credentialsFromJSONValue(value, xautoPackage)
}

func readChannelAccountImportBytes(reader io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("导入文件过大，最大支持 %d MB", maxBytes>>20)
	}
	return data, nil
}

func channelAccountImportBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func channelAccountImportLooksLikeZip(name string, data []byte) bool {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".zip") {
		return true
	}
	return len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4})
}

func channelAccountImportString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func channelAccountImportStringAtPath(payload map[string]interface{}, pathParts ...string) string {
	var current interface{} = payload
	for _, part := range pathParts {
		object, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = object[part]
	}
	return channelAccountImportString(current)
}

func channelAccountImportLooksLikeManifest(payload map[string]interface{}) bool {
	packageType := channelAccountImportString(payload["type"])
	return strings.EqualFold(packageType, "newapi-channel-files")
}

func channelAccountImportLooksLikeCredentialPayload(payload map[string]interface{}) bool {
	for _, key := range []string{"access_token", "refresh_token", "account_id", "api_key", "private_key", "client_email"} {
		if channelAccountImportString(payload[key]) != "" {
			return true
		}
	}
	return false
}

func parseChannelAccountCredentialIndexParam(c *gin.Context) (int, bool) {
	credentialIndex, err := strconv.Atoi(c.Param("credential_index"))
	if err != nil || credentialIndex < 0 {
		common.ApiError(c, fmt.Errorf("账号索引无效"))
		return 0, false
	}
	return credentialIndex, true
}

func parseChannelAccountRequestTarget(c *gin.Context) (int, int, bool) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return 0, 0, false
	}
	credentialIndex, ok := parseChannelAccountCredentialIndexParam(c)
	if !ok {
		return 0, 0, false
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil || channel == nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return 0, 0, false
	}
	if credentialIndex >= len(channel.GetKeys()) {
		common.ApiErrorMsg(c, "账号索引超出范围")
		return 0, 0, false
	}
	return channelID, credentialIndex, true
}

func buildChannelAccountsResponse(channel *model.Channel, query ...channelAccountsQuery) ChannelAccountsResponse {
	params := normalizeChannelAccountsQuery(channelAccountsQuery{})
	if len(query) > 0 {
		params = normalizeChannelAccountsQuery(query[0])
	}
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(channel)
	response := ChannelAccountsResponse{
		ChannelID:     channel.Id,
		ChannelName:   channel.Name,
		ResourceRef:   modelgatewayaccount.ResourceRefForChannel(channel),
		View:          params.View,
		Page:          params.Page,
		PageSize:      params.PageSize,
		Total:         len(accounts),
		FilteredTotal: len(accounts),
		Items:         make([]ChannelAccountItem, 0, params.PageSize),
		Summary:       ChannelAccountSummary{HealthProbeExcluded: true},
	}
	for _, account := range accounts {
		if account.KeyEnabled {
			response.Enabled++
		} else {
			response.Disabled++
		}
	}

	statsByAccount, summary := channelAccountStatsForAccounts(channel.Id, accounts)
	response.Summary = summary

	filtered := filterChannelAccounts(accounts, params)
	sortChannelAccounts(filtered, params, statsByAccount)
	response.FilteredTotal = len(filtered)
	pageAccounts := paginateChannelAccounts(filtered, params)
	runtimeItems := runtimeItemsForChannelAccounts(channel.Id, pageAccounts, len(accounts) == 1)

	proxiesByID := channelAccountProxiesByID(channel, pageAccounts)
	proxyUsagesByID := channelAccountProxyUsagesByID(proxiesByID)
	for _, account := range pageAccounts {
		item := buildChannelAccountItem(account, runtimeItems, len(accounts) == 1)
		item.Capabilities = keyStatusCapabilities(channel, account.CredentialIndex)
		if item.Capabilities != nil {
			item.Capabilities.CapabilityClassification = item.Capabilities.EffectiveClassification()
		}
		if stats, ok := statsByAccount[channelAccountStatsKey(account)]; ok {
			item.Stats = stats
		}
		if account.ProxyRef.ProxyID > 0 {
			if proxyConfig, ok := proxiesByID[account.ProxyRef.ProxyID]; ok {
				proxyResponse := buildModelGatewayProxyResponse(proxyConfig, proxyUsagesByID[account.ProxyRef.ProxyID])
				item.Proxy = &proxyResponse
			} else {
				item.Proxy = &ModelGatewayProxyResponse{
					ID:             account.ProxyRef.ProxyID,
					Name:           fmt.Sprintf("Proxy #%d", account.ProxyRef.ProxyID),
					Enabled:        false,
					PasswordMasked: true,
				}
			}
		}
		item.Scheduling = buildChannelAccountSchedulingExplanation(item)
		response.Summary.Scored += boolToInt64(item.Score != nil)
		if item.Score != nil {
			response.Summary.RealSampleCount30m += int64(item.Score.RealSampleCount30m)
		}
		response.Items = append(response.Items, item)
	}
	return response
}

func channelAccountProxyUsagesByID(proxiesByID map[int]model.ModelGatewayProxy) map[int][]model.ModelGatewayProxyUsage {
	result := make(map[int][]model.ModelGatewayProxyUsage)
	if len(proxiesByID) == 0 {
		return result
	}
	proxyIDs := make([]int, 0, len(proxiesByID))
	for proxyID := range proxiesByID {
		if proxyID > 0 {
			proxyIDs = append(proxyIDs, proxyID)
		}
	}
	usages, err := model.ListModelGatewayProxyUsages(proxyIDs)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load account proxy usages: proxy_ids=%v, error=%v", proxyIDs, err))
		return result
	}
	for _, usage := range usages {
		result[usage.ProxyID] = append(result[usage.ProxyID], usage)
	}
	return result
}

func channelAccountProxiesByID(channel *model.Channel, accounts []modelgatewayaccount.ChannelAccount) map[int]model.ModelGatewayProxy {
	if channel == nil || channel.ChannelInfo.MultiKeyProxyIDs == nil {
		return nil
	}
	proxySet := make(map[int]struct{})
	for _, account := range accounts {
		if account.ProxyRef.ProxyID > 0 {
			proxySet[account.ProxyRef.ProxyID] = struct{}{}
		}
	}
	proxyIDs := make([]int, 0, len(proxySet))
	for proxyID := range proxySet {
		if proxyID > 0 {
			proxyIDs = append(proxyIDs, proxyID)
		}
	}
	proxiesByID, err := model.GetModelGatewayProxiesByIDs(proxyIDs)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load account proxies: channel_id=%d, error=%v", channel.Id, err))
		return nil
	}
	return proxiesByID
}

func buildChannelAccountsResponseWithOperation(channel *model.Channel, operation *ChannelAccountOperation) ChannelAccountsResponse {
	response := buildChannelAccountsResponse(channel)
	response.Operation = operation
	return response
}

func parseChannelAccountsQuery(c *gin.Context) channelAccountsQuery {
	if c == nil {
		return normalizeChannelAccountsQuery(channelAccountsQuery{})
	}
	return normalizeChannelAccountsQuery(channelAccountsQuery{
		View:     c.DefaultQuery("view", channelAccountViewManage),
		Page:     parsePositiveQueryInt(c, "page", 1),
		PageSize: parsePositiveQueryInt(c, "page_size", channelAccountDefaultPageSize),
		Keyword:  c.Query("keyword"),
		Status:   c.DefaultQuery("status", "all"),
		Sort:     c.Query("sort"),
		Order:    c.Query("order"),
	})
}

func parsePositiveQueryInt(c *gin.Context, key string, fallback int) int {
	if c == nil {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func normalizeChannelAccountsQuery(query channelAccountsQuery) channelAccountsQuery {
	query.View = strings.ToLower(strings.TrimSpace(query.View))
	if query.View != channelAccountViewStats {
		query.View = channelAccountViewManage
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = channelAccountDefaultPageSize
	}
	if query.PageSize > channelAccountMaxPageSize {
		query.PageSize = channelAccountMaxPageSize
	}
	query.Keyword = strings.ToLower(strings.TrimSpace(query.Keyword))
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status != "enabled" && query.Status != "disabled" {
		query.Status = "all"
	}
	query.Sort = strings.ToLower(strings.TrimSpace(query.Sort))
	query.Order = strings.ToLower(strings.TrimSpace(query.Order))
	if query.Order == "" {
		query.Order = "asc"
		if query.View == channelAccountViewStats && query.Sort == "" {
			query.Sort = "last_active_at"
			query.Order = "desc"
		}
	} else if query.Order != "asc" {
		query.Order = "desc"
	}
	return query
}

func filterChannelAccounts(accounts []modelgatewayaccount.ChannelAccount, query channelAccountsQuery) []modelgatewayaccount.ChannelAccount {
	filtered := make([]modelgatewayaccount.ChannelAccount, 0, len(accounts))
	for _, account := range accounts {
		if query.Status == "enabled" && !account.KeyEnabled {
			continue
		}
		if query.Status == "disabled" && account.KeyEnabled {
			continue
		}
		if query.Keyword != "" && !strings.Contains(channelAccountSearchText(account), query.Keyword) {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered
}

func channelAccountSearchText(account modelgatewayaccount.ChannelAccount) string {
	identity := account.AccountIdentity
	resource := account.ResourceRef
	proxy := account.ProxyRef
	parts := []string{
		identity.DisplayName,
		identity.AccountID,
		identity.AccountIdentityKey,
		identity.AccountType,
		identity.Brand,
		identity.Provider,
		resource.Brand,
		resource.Provider,
		account.DisabledReason,
		modelgatewayaccount.ShortFingerprint(identity.CredentialSubjectFingerprint),
		modelgatewayaccount.ShortFingerprint(identity.CredentialFingerprint),
		identity.CredentialSubjectFingerprint,
		identity.CredentialFingerprint,
		proxy.Name,
		proxy.MaskedAddress,
		strconv.Itoa(account.CredentialIndex + 1),
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func sortChannelAccounts(accounts []modelgatewayaccount.ChannelAccount, query channelAccountsQuery, statsByAccount map[string]*ChannelAccountStats) {
	sort.SliceStable(accounts, func(i, j int) bool {
		left := accounts[i]
		right := accounts[j]
		less := false
		switch query.Sort {
		case "status":
			less = channelAccountEnabledFirstLess(left, right)
		case "today_requests":
			less = channelAccountStatRequests(statsByAccount, left, "today") < channelAccountStatRequests(statsByAccount, right, "today")
		case "last_5h_requests":
			less = channelAccountStatRequests(statsByAccount, left, "last_5h") < channelAccountStatRequests(statsByAccount, right, "last_5h")
		case "last_7d_requests":
			less = channelAccountStatRequests(statsByAccount, left, "last_7d") < channelAccountStatRequests(statsByAccount, right, "last_7d")
		case "last_active_at":
			less = channelAccountStatLastActive(statsByAccount, left) < channelAccountStatLastActive(statsByAccount, right)
		case "success_rate":
			less = channelAccountStatSuccessRate(statsByAccount, left) < channelAccountStatSuccessRate(statsByAccount, right)
		case "score":
			less = left.CredentialIndex < right.CredentialIndex
		case "":
			less = channelAccountEnabledFirstLess(left, right)
		case "credential_index":
			fallthrough
		default:
			less = left.CredentialIndex < right.CredentialIndex
		}
		if query.Order == "asc" {
			return less
		}
		return !less && !channelAccountSortEqual(left, right, query, statsByAccount)
	})
}

func channelAccountEnabledFirstLess(left, right modelgatewayaccount.ChannelAccount) bool {
	if left.KeyEnabled != right.KeyEnabled {
		return left.KeyEnabled
	}
	return left.CredentialIndex < right.CredentialIndex
}

func channelAccountSortEqual(left, right modelgatewayaccount.ChannelAccount, query channelAccountsQuery, statsByAccount map[string]*ChannelAccountStats) bool {
	switch query.Sort {
	case "status", "":
		return left.KeyEnabled == right.KeyEnabled && left.CredentialIndex == right.CredentialIndex
	case "today_requests":
		return channelAccountStatRequests(statsByAccount, left, "today") == channelAccountStatRequests(statsByAccount, right, "today")
	case "last_5h_requests":
		return channelAccountStatRequests(statsByAccount, left, "last_5h") == channelAccountStatRequests(statsByAccount, right, "last_5h")
	case "last_7d_requests":
		return channelAccountStatRequests(statsByAccount, left, "last_7d") == channelAccountStatRequests(statsByAccount, right, "last_7d")
	case "last_active_at":
		return channelAccountStatLastActive(statsByAccount, left) == channelAccountStatLastActive(statsByAccount, right)
	case "success_rate":
		return channelAccountStatSuccessRate(statsByAccount, left) == channelAccountStatSuccessRate(statsByAccount, right)
	case "score":
		return left.CredentialIndex == right.CredentialIndex
	default:
		return left.CredentialIndex == right.CredentialIndex
	}
}

func paginateChannelAccounts(accounts []modelgatewayaccount.ChannelAccount, query channelAccountsQuery) []modelgatewayaccount.ChannelAccount {
	start := (query.Page - 1) * query.PageSize
	if start >= len(accounts) {
		return nil
	}
	end := start + query.PageSize
	if end > len(accounts) {
		end = len(accounts)
	}
	return accounts[start:end]
}

func runtimeItemsForChannelAccounts(channelID int, accounts []modelgatewayaccount.ChannelAccount, allowChannelFallback bool) []modelgatewayobservability.RuntimeStatusItem {
	runtimeQuery := modelgatewayobservability.RuntimeStatusQuery{
		ChannelID: channelID,
		Limit:     modelGatewayRuntimeStatusMaxLimit,
	}
	if len(accounts) > 0 && !allowChannelFallback {
		runtimeQuery.StrictAccountScope = true
		runtimeQuery.AccountIDs = make([]string, 0, len(accounts))
		runtimeQuery.CredentialIndexes = make([]int, 0, len(accounts))
		runtimeQuery.CredentialSubjectFPs = make([]string, 0, len(accounts))
		runtimeQuery.CredentialFPs = make([]string, 0, len(accounts))
		for _, account := range accounts {
			runtimeQuery.AccountIDs = append(runtimeQuery.AccountIDs, account.AccountIdentity.AccountID)
			runtimeQuery.CredentialIndexes = append(runtimeQuery.CredentialIndexes, account.CredentialIndex)
			runtimeQuery.CredentialSubjectFPs = append(runtimeQuery.CredentialSubjectFPs, account.AccountIdentity.CredentialSubjectFingerprint)
			runtimeQuery.CredentialFPs = append(runtimeQuery.CredentialFPs, account.AccountIdentity.CredentialFingerprint)
		}
	}
	return defaultModelGatewayRuntimeStatusService().Build(runtimeQuery).Items
}

func channelAccountStatsForAccounts(channelID int, accounts []modelgatewayaccount.ChannelAccount) (map[string]*ChannelAccountStats, ChannelAccountSummary) {
	result := make(map[string]*ChannelAccountStats, len(accounts))
	summary := ChannelAccountSummary{HealthProbeExcluded: true}
	if channelID <= 0 || len(accounts) == 0 {
		return result, summary
	}
	now := time.Now()
	windows := []model.ChannelAccountUsageWindowSpec{
		{Name: "today", Since: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()},
		{Name: "last_5h", Since: now.Add(-5 * time.Hour).Unix()},
		{Name: "last_7d", Since: now.Add(-7 * 24 * time.Hour).Unix()},
	}
	aggregates, err := model.QueryChannelAccountUsageWindowAggregates(channelID, windows, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load channel account usage stats: channel_id=%d error=%v", channelID, err))
		return result, summary
	}
	accountKeys := make(map[string]string, len(accounts)*2)
	for _, account := range accounts {
		result[channelAccountStatsKey(account)] = &ChannelAccountStats{}
		accountKeys[model.ChannelAccountUsageAggregateKey(account.AccountIdentity.AccountIdentityKey, account.CredentialIndex)] = channelAccountStatsKey(account)
		accountKeys[model.ChannelAccountUsageAggregateKey(account.AccountIdentity.AccountID, account.CredentialIndex)] = channelAccountStatsKey(account)
		accountKeys[model.ChannelAccountUsageAggregateKey("", account.CredentialIndex)] = channelAccountStatsKey(account)
	}
	for _, aggregate := range aggregates {
		targetKey, ok := accountKeys[model.ChannelAccountUsageAggregateKey(aggregate.AccountIdentityKey, aggregate.CredentialIndex)]
		if !ok {
			targetKey, ok = accountKeys[model.ChannelAccountUsageAggregateKey("", aggregate.CredentialIndex)]
		}
		if !ok {
			continue
		}
		stats := result[targetKey]
		if stats == nil {
			stats = &ChannelAccountStats{}
			result[targetKey] = stats
		}
		window := channelAccountUsageWindowFromAggregate(aggregate)
		switch aggregate.Window {
		case "today":
			stats.Today = window
			summary.Today = addChannelAccountUsageWindow(summary.Today, window)
		case "last_5h":
			stats.Last5h = window
			summary.Last5h = addChannelAccountUsageWindow(summary.Last5h, window)
		case "last_7d":
			stats.Last7d = window
			summary.Last7d = addChannelAccountUsageWindow(summary.Last7d, window)
		}
		if window.LastActiveAt > stats.LastActiveAt {
			stats.LastActiveAt = window.LastActiveAt
		}
		if stats.MainErrorCategory == "" && window.TopErrorCategory != "" {
			stats.MainErrorCategory = window.TopErrorCategory
		}
	}
	summary.Today = finalizeChannelAccountUsageWindow(summary.Today)
	summary.Last5h = finalizeChannelAccountUsageWindow(summary.Last5h)
	summary.Last7d = finalizeChannelAccountUsageWindow(summary.Last7d)
	return result, summary
}

func channelAccountStatsKey(account modelgatewayaccount.ChannelAccount) string {
	return model.ChannelAccountUsageAggregateKey(account.AccountIdentity.AccountIdentityKey, account.CredentialIndex)
}

func buildChannelAccountRequestReconcileResponse(channelID int, credentialIndex int, requestID string) (ChannelAccountRequestReconcileResponse, error) {
	response := ChannelAccountRequestReconcileResponse{
		RequestID:           strings.TrimSpace(requestID),
		ChannelID:           channelID,
		CredentialIndex:     credentialIndex,
		AccountDisplayIndex: channelAccountDisplayIndex(credentialIndex),
		Diagnoses:           make([]ChannelAccountRequestReconcileDiagnosis, 0),
		ExecutionRecords:    make([]ChannelAccountExecutionRecordReconcile, 0),
		ScoreEvents:         make([]ChannelAccountScoreEventReconcile, 0),
		Checks:              make([]ChannelAccountRequestReconcileCheck, 0, 6),
	}

	usageEvent, err := model.GetChannelAccountUsageEventByRequestId(requestID)
	if err != nil {
		return response, err
	}
	if usageEvent != nil {
		items := buildChannelAccountRecentRequestItems([]model.ChannelAccountUsageEvent{*usageEvent})
		if len(items) > 0 {
			response.UsageEvent = &items[0]
		}
	}

	userRequest, err := model.GetModelGatewayUserRequestSummaryByRequestId(requestID)
	if err != nil {
		return response, err
	}
	if userRequest != nil {
		response.UserRequest = channelAccountUserRequestReconcile(*userRequest)
	}

	executionRecords, err := model.QueryModelExecutionRecordsByRequestId(requestID, 20)
	if err != nil {
		return response, err
	}
	response.ExecutionRecords = buildChannelAccountExecutionRecordReconcile(executionRecords)

	scoreEvents, err := model.QueryModelGatewayScoreEventsByRequestId(requestID, 20)
	if err != nil {
		return response, err
	}
	response.ScoreEvents = buildChannelAccountScoreEventReconcile(scoreEvents)

	costSummary, err := model.GetModelGatewayRequestCostSummaryByRequestId(requestID)
	if err != nil {
		return response, err
	}
	if costSummary != nil {
		response.CostSummary = channelAccountRequestCostReconcile(*costSummary)
	}
	response.Checks = buildChannelAccountRequestReconcileChecks(channelID, credentialIndex, usageEvent, userRequest, executionRecords, scoreEvents, costSummary)
	response.Diagnoses = buildChannelAccountRequestReconcileDiagnoses(channelID, credentialIndex, usageEvent, userRequest, executionRecords, scoreEvents, costSummary)
	return response, nil
}

func channelAccountUserRequestReconcile(row model.ModelGatewayUserRequestSummary) *ChannelAccountUserRequestReconcile {
	return &ChannelAccountUserRequestReconcile{
		CompletedAt:        row.CompletedAt,
		RequestedModel:     row.RequestedModel,
		RequestedGroup:     row.RequestedGroup,
		SelectedGroup:      row.SelectedGroup,
		FinalChannelID:     row.FinalChannelID,
		FinalChannelName:   row.FinalChannelName,
		Attempts:           row.Attempts,
		LastAttemptIndex:   row.LastAttemptIndex,
		FinalSuccess:       row.FinalSuccess,
		Recovered:          row.Recovered,
		FinalStatusCode:    row.FinalStatusCode,
		FinalErrorCategory: row.FinalErrorCategory,
		IsHealthProbe:      row.IsHealthProbe,
		ProbeReason:        row.ProbeReason,
		DurationMs:         row.DurationMs,
		TTFTMs:             row.TTFTMs,
	}
}

func buildChannelAccountExecutionRecordReconcile(rows []model.ModelExecutionRecord) []ChannelAccountExecutionRecordReconcile {
	items := make([]ChannelAccountExecutionRecordReconcile, 0, len(rows))
	for _, row := range rows {
		items = append(items, ChannelAccountExecutionRecordReconcile{
			CreatedAt:       row.CreatedAt,
			AttemptIndex:    row.AttemptIndex,
			ChannelID:       row.ChannelId,
			ChannelName:     row.ChannelName,
			ActualChannelID: row.ActualChannelId,
			Success:         row.Success,
			StatusCode:      row.StatusCode,
			ErrorCategory:   row.ErrorCategory,
			DurationMs:      row.DurationMs,
			TTFTMs:          row.TTFTMs,
			SmartHandled:    row.SmartHandled,
			FallbackUsed:    row.FallbackUsed,
			ScoreTotal:      row.ScoreTotal,
			SelectedReason:  row.SelectedReason,
		})
	}
	return items
}

func buildChannelAccountScoreEventReconcile(rows []model.ModelGatewayScoreEvent) []ChannelAccountScoreEventReconcile {
	items := make([]ChannelAccountScoreEventReconcile, 0, len(rows))
	for _, row := range rows {
		items = append(items, ChannelAccountScoreEventReconcile{
			CreatedAt:           row.CreatedAt,
			AttemptIndex:        row.AttemptIndex,
			ChannelID:           row.ChannelID,
			CredentialIndex:     row.CredentialIndex,
			AccountDisplayIndex: channelAccountDisplayIndex(row.CredentialIndex),
			BeforeTotal:         row.BeforeTotal,
			AfterTotal:          row.AfterTotal,
			Delta:               row.Delta,
			FailureScope:        row.FailureScope,
			SwitchReason:        row.SwitchReason,
			RequestedModel:      row.RequestedModel,
			Group:               row.Group,
			IsHealthProbe:       row.IsHealthProbe,
		})
	}
	return items
}

func channelAccountRequestCostReconcile(row model.ModelGatewayRequestCostSummary) *ChannelAccountRequestCostReconcile {
	return &ChannelAccountRequestCostReconcile{
		UpstreamModel:     row.UpstreamModel,
		UpstreamCostTotal: row.UpstreamCostTotal,
		CostSource:        row.CostSource,
		CostAccuracy:      row.CostAccuracy,
		CalculatedAt:      row.CalculatedAt,
	}
}

func buildChannelAccountRequestReconcileChecks(channelID int, credentialIndex int, usageEvent *model.ChannelAccountUsageEvent, userRequest *model.ModelGatewayUserRequestSummary, executionRecords []model.ModelExecutionRecord, scoreEvents []model.ModelGatewayScoreEvent, costSummary *model.ModelGatewayRequestCostSummary) []ChannelAccountRequestReconcileCheck {
	checks := make([]ChannelAccountRequestReconcileCheck, 0, 6)
	if usageEvent == nil {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "usage_event", Status: "missing", Detail: "usage_event_missing"})
	} else {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "usage_event", Status: "ok", Detail: "usage_event_found"})
		if usageEvent.ChannelID == channelID && usageEvent.CredentialIndex == credentialIndex {
			checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "account_match", Status: "ok", Detail: "account_match"})
		} else {
			checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "account_match", Status: "warning", Detail: "account_mismatch"})
		}
		if status, detail := channelAccountUsageStatisticsDiagnostic(*usageEvent, channelAccountUsageStatisticsRecorded(*usageEvent), usageEvent.TotalTokens > 0 || usageEvent.Quota != 0, usageEvent.CostCalculatedAt > 0 || usageEvent.UpstreamCostTotal != 0, usageEvent.CredentialIndex >= 0 && (strings.TrimSpace(usageEvent.AccountIdentityKey) != "" || strings.TrimSpace(usageEvent.CredentialFingerprint) != "")); status == "complete" {
			checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "statistics", Status: "ok", Detail: detail})
		} else {
			checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "statistics", Status: status, Detail: detail})
		}
	}
	if userRequest == nil {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "user_request", Status: "missing", Detail: "user_request_missing"})
	} else {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "user_request", Status: "ok", Detail: "user_request_found"})
	}
	if len(executionRecords) == 0 && len(scoreEvents) == 0 {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "samples", Status: "missing", Detail: "attempt_samples_missing"})
	} else {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "samples", Status: "ok", Detail: "attempt_samples_found"})
	}
	if costSummary == nil {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "cost", Status: "pending", Detail: "cost_summary_missing"})
	} else {
		checks = append(checks, ChannelAccountRequestReconcileCheck{Key: "cost", Status: "ok", Detail: "cost_summary_found"})
	}
	return checks
}

func buildChannelAccountRequestReconcileDiagnoses(channelID int, credentialIndex int, usageEvent *model.ChannelAccountUsageEvent, userRequest *model.ModelGatewayUserRequestSummary, executionRecords []model.ModelExecutionRecord, scoreEvents []model.ModelGatewayScoreEvent, costSummary *model.ModelGatewayRequestCostSummary) []ChannelAccountRequestReconcileDiagnosis {
	diagnoses := make([]ChannelAccountRequestReconcileDiagnosis, 0, 4)
	add := func(key string, severity string) {
		diagnoses = append(diagnoses, ChannelAccountRequestReconcileDiagnosis{Key: key, Severity: severity})
	}

	if usageEvent == nil {
		if len(executionRecords) > 0 || len(scoreEvents) > 0 || userRequest != nil {
			add("usage_event_missing_but_samples_exist", "error")
		} else {
			add("request_trace_missing", "error")
		}
		return diagnoses
	}

	if usageEvent.ChannelID != channelID || usageEvent.CredentialIndex != credentialIndex {
		add("account_mismatch", "warning")
	}
	if usageEvent.IsHealthProbe {
		add("health_probe_excluded", "info")
	}

	billingRecorded := usageEvent.TotalTokens > 0 || usageEvent.Quota != 0
	costRecorded := usageEvent.CostCalculatedAt > 0 || usageEvent.UpstreamCostTotal != 0 || costSummary != nil
	attributionComplete := usageEvent.CredentialIndex >= 0 && (strings.TrimSpace(usageEvent.AccountIdentityKey) != "" || strings.TrimSpace(usageEvent.CredentialFingerprint) != "")
	statisticsRecorded := channelAccountUsageStatisticsRecorded(*usageEvent)
	status, _ := channelAccountUsageStatisticsDiagnostic(*usageEvent, statisticsRecorded, billingRecorded, costRecorded, attributionComplete)
	switch status {
	case "attribution_missing":
		add("account_attribution_missing", "error")
	case "dispatch_only":
		add("dispatch_only", "warning")
	case "billing_pending":
		add("billing_pending", "warning")
	case "cost_pending":
		add("cost_pending", "warning")
	}

	if userRequest == nil {
		add("user_request_summary_missing", "warning")
	} else if !userRequest.FinalSuccess && !userRequest.IsHealthProbe {
		add("request_failed", "warning")
	}
	if len(executionRecords) == 0 && len(scoreEvents) == 0 {
		add("attempt_samples_missing", "warning")
	}
	if costSummary == nil && billingRecorded {
		add("cost_summary_pending", "warning")
	}
	if len(diagnoses) == 0 {
		add("trace_complete", "ok")
	}
	return diagnoses
}

func buildChannelAccountRecentRequestItems(rows []model.ChannelAccountUsageEvent) []ChannelAccountRecentRequestItem {
	items := make([]ChannelAccountRecentRequestItem, 0, len(rows))
	for _, row := range rows {
		completedAt := row.CompletedAt
		if completedAt <= 0 {
			if row.UpdatedAt > 0 {
				completedAt = row.UpdatedAt
			} else {
				completedAt = row.CreatedAt
			}
		}
		statisticsRecorded := channelAccountUsageStatisticsRecorded(row)
		billingRecorded := row.TotalTokens > 0 || row.Quota != 0
		costRecorded := row.CostCalculatedAt > 0 || row.UpstreamCostTotal != 0
		attributionComplete := row.CredentialIndex >= 0 && (strings.TrimSpace(row.AccountIdentityKey) != "" || strings.TrimSpace(row.CredentialFingerprint) != "")
		statisticsStatus, statisticsDiagnostic := channelAccountUsageStatisticsDiagnostic(row, statisticsRecorded, billingRecorded, costRecorded, attributionComplete)
		items = append(items, ChannelAccountRecentRequestItem{
			RequestID:                row.RequestId,
			AttemptIndex:             row.AttemptIndex,
			ChannelID:                row.ChannelID,
			CredentialIndex:          row.CredentialIndex,
			AccountDisplayIndex:      channelAccountDisplayIndex(row.CredentialIndex),
			AccountIdentityKey:       row.AccountIdentityKey,
			RequestedModel:           row.RequestedModel,
			RequestedGroup:           row.RequestedGroup,
			SelectedGroup:            row.SelectedGroup,
			EndpointType:             row.EndpointType,
			CompletedAt:              completedAt,
			Success:                  row.Success,
			StatusCode:               row.StatusCode,
			ErrorCategory:            row.ErrorCategory,
			IsHealthProbe:            row.IsHealthProbe,
			DurationMs:               row.DurationMs,
			TTFTMs:                   row.TTFTMs,
			TotalTokens:              row.TotalTokens,
			Quota:                    row.Quota,
			UpstreamCostTotal:        row.UpstreamCostTotal,
			StatisticsRecorded:       statisticsRecorded,
			BillingRecorded:          billingRecorded,
			CostRecorded:             costRecorded,
			AttributionComplete:      attributionComplete,
			StatisticsStatus:         statisticsStatus,
			StatisticsDiagnostic:     statisticsDiagnostic,
			UsageEstimated:           row.UsageEstimated,
			ProviderSurface:          row.ProviderSurface,
			CapabilityClassification: row.CapabilityClassification,
		})
	}
	return items
}

func channelAccountDisplayIndex(credentialIndex int) int {
	if credentialIndex < 0 {
		return 0
	}
	return credentialIndex + 1
}

func channelAccountUsageStatisticsRecorded(row model.ChannelAccountUsageEvent) bool {
	return row.CompletedAt > 0 ||
		row.StatusCode > 0 ||
		row.DurationMs > 0 ||
		row.TTFTMs > 0 ||
		row.TotalTokens > 0 ||
		row.Quota != 0 ||
		strings.TrimSpace(row.ErrorCategory) != ""
}

func channelAccountUsageStatisticsDiagnostic(row model.ChannelAccountUsageEvent, statisticsRecorded bool, billingRecorded bool, costRecorded bool, attributionComplete bool) (string, string) {
	if row.IsHealthProbe {
		return "health_probe", "health_probe_excluded"
	}
	if !attributionComplete {
		return "attribution_missing", "missing_account_attribution"
	}
	if !statisticsRecorded {
		return "dispatch_only", "dispatch_record_only"
	}
	if !billingRecorded && row.Success {
		return "billing_pending", "waiting_for_billing"
	}
	if !costRecorded && billingRecorded {
		return "cost_pending", "waiting_for_cost"
	}
	return "complete", "statistics_complete"
}

func channelAccountUsageWindowFromAggregate(aggregate model.ChannelAccountUsageWindowAggregate) ChannelAccountUsageWindowResponse {
	return finalizeChannelAccountUsageWindow(ChannelAccountUsageWindowResponse{
		Requests:          aggregate.Requests,
		SuccessRequests:   aggregate.SuccessRequests,
		ErrorRequests:     aggregate.ErrorRequests,
		TimeoutRequests:   aggregate.TimeoutRequests,
		PromptTokens:      aggregate.PromptTokens,
		CompletionTokens:  aggregate.CompletionTokens,
		TotalTokens:       aggregate.TotalTokens,
		Quota:             aggregate.Quota,
		UpstreamCostTotal: aggregate.UpstreamCostTotal,
		AvgDurationMs:     aggregate.AvgDurationMs,
		AvgTTFTMs:         aggregate.AvgTTFTMs,
		LastActiveAt:      aggregate.LastActiveAt,
		TopErrorCategory:  aggregate.TopErrorCategory,
		TopErrorCount:     aggregate.TopErrorCount,
	})
}

func addChannelAccountUsageWindow(left ChannelAccountUsageWindowResponse, right ChannelAccountUsageWindowResponse) ChannelAccountUsageWindowResponse {
	totalRequests := left.Requests + right.Requests
	leftAvgDurationWeighted := left.AvgDurationMs * float64(left.Requests)
	rightAvgDurationWeighted := right.AvgDurationMs * float64(right.Requests)
	leftAvgTTFTWeighted := left.AvgTTFTMs * float64(left.Requests)
	rightAvgTTFTWeighted := right.AvgTTFTMs * float64(right.Requests)
	out := ChannelAccountUsageWindowResponse{
		Requests:          totalRequests,
		SuccessRequests:   left.SuccessRequests + right.SuccessRequests,
		ErrorRequests:     left.ErrorRequests + right.ErrorRequests,
		TimeoutRequests:   left.TimeoutRequests + right.TimeoutRequests,
		PromptTokens:      left.PromptTokens + right.PromptTokens,
		CompletionTokens:  left.CompletionTokens + right.CompletionTokens,
		TotalTokens:       left.TotalTokens + right.TotalTokens,
		Quota:             left.Quota + right.Quota,
		UpstreamCostTotal: left.UpstreamCostTotal + right.UpstreamCostTotal,
		TopErrorCategory:  left.TopErrorCategory,
		TopErrorCount:     left.TopErrorCount,
	}
	if right.TopErrorCount > out.TopErrorCount {
		out.TopErrorCategory = right.TopErrorCategory
		out.TopErrorCount = right.TopErrorCount
	}
	if right.LastActiveAt > left.LastActiveAt {
		out.LastActiveAt = right.LastActiveAt
	} else {
		out.LastActiveAt = left.LastActiveAt
	}
	if totalRequests > 0 {
		out.AvgDurationMs = (leftAvgDurationWeighted + rightAvgDurationWeighted) / float64(totalRequests)
		out.AvgTTFTMs = (leftAvgTTFTWeighted + rightAvgTTFTWeighted) / float64(totalRequests)
	}
	return out
}

func finalizeChannelAccountUsageWindow(window ChannelAccountUsageWindowResponse) ChannelAccountUsageWindowResponse {
	if window.Requests > 0 {
		window.SuccessRate = float64(window.SuccessRequests) / float64(window.Requests)
		window.ErrorRate = float64(window.ErrorRequests) / float64(window.Requests)
		window.TimeoutRate = float64(window.TimeoutRequests) / float64(window.Requests)
	}
	window.UpstreamCostTotal = roundChannelAccountFloat(window.UpstreamCostTotal)
	window.AvgDurationMs = roundChannelAccountFloat(window.AvgDurationMs)
	window.AvgTTFTMs = roundChannelAccountFloat(window.AvgTTFTMs)
	window.SuccessRate = roundChannelAccountFloat(window.SuccessRate)
	window.ErrorRate = roundChannelAccountFloat(window.ErrorRate)
	window.TimeoutRate = roundChannelAccountFloat(window.TimeoutRate)
	return window
}

func channelAccountStatRequests(statsByAccount map[string]*ChannelAccountStats, account modelgatewayaccount.ChannelAccount, window string) int64 {
	stats := statsByAccount[channelAccountStatsKey(account)]
	if stats == nil {
		return 0
	}
	switch window {
	case "today":
		return stats.Today.Requests
	case "last_5h":
		return stats.Last5h.Requests
	case "last_7d":
		return stats.Last7d.Requests
	default:
		return 0
	}
}

func channelAccountStatSuccessRate(statsByAccount map[string]*ChannelAccountStats, account modelgatewayaccount.ChannelAccount) float64 {
	stats := statsByAccount[channelAccountStatsKey(account)]
	if stats == nil {
		return 0
	}
	return stats.Today.SuccessRate
}

func channelAccountStatLastActive(statsByAccount map[string]*ChannelAccountStats, account modelgatewayaccount.ChannelAccount) int64 {
	stats := statsByAccount[channelAccountStatsKey(account)]
	if stats == nil {
		return 0
	}
	return stats.LastActiveAt
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func roundChannelAccountFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*10000) / 10000
}

func updateChannelAccountStatus(channelID int, credentialIndex int, enabled bool, reason string) (*ChannelAccountOperation, error) {
	return updateChannelAccountsStatus(channelID, []int{credentialIndex}, enabled, reason)
}

func updateChannelAccountCredential(channelID int, credentialIndex int, credential string, credentialType string) (*ChannelAccountOperation, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("渠道不存在")
	}
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, fmt.Errorf("渠道不存在")
	}
	normalizedCredential, accountType, err := normalizeChannelAccountEditableCredentialForChannel(channel, credential, credentialType)
	if err != nil {
		return nil, err
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("渠道没有可管理的账号")
	}
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return nil, fmt.Errorf("账号索引超出范围")
	}
	for index, key := range keys {
		if index == credentialIndex {
			continue
		}
		if strings.TrimSpace(key) == normalizedCredential {
			return nil, fmt.Errorf("账号凭证已存在")
		}
	}

	keys[credentialIndex] = normalizedCredential
	channel.Key = strings.Join(keys, "\n")
	channel.ChannelInfo.IsMultiKey = len(keys) > 1
	channel.ChannelInfo.MultiKeySize = len(keys)
	if accountType != "" {
		if channel.ChannelInfo.MultiKeyAccountTypes == nil {
			channel.ChannelInfo.MultiKeyAccountTypes = make(map[int]string)
		}
		channel.ChannelInfo.MultiKeyAccountTypes[credentialIndex] = accountType
	} else if channel.ChannelInfo.MultiKeyAccountTypes != nil {
		delete(channel.ChannelInfo.MultiKeyAccountTypes, credentialIndex)
		if len(channel.ChannelInfo.MultiKeyAccountTypes) == 0 {
			channel.ChannelInfo.MultiKeyAccountTypes = nil
		}
	}
	cleanupChannelAccountStatusMaps(channel, len(keys))
	if err := saveChannelAccountsAfterDelete(channel); err != nil {
		return nil, err
	}
	return &ChannelAccountOperation{
		Type:      "credential",
		Action:    "update",
		Requested: 1,
		Affected:  1,
	}, nil
}

func updateChannelAccountsStatus(channelID int, credentialIndexes []int, enabled bool, reason string) (*ChannelAccountOperation, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("渠道不存在")
	}
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, fmt.Errorf("渠道不存在")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("渠道没有可管理的账号")
	}
	indexes, err := normalizeChannelAccountIndexes(credentialIndexes, len(keys))
	if err != nil {
		return nil, err
	}
	operation := &ChannelAccountOperation{
		Type:      "status",
		Action:    channelAccountStatusAction(enabled),
		Requested: len(credentialIndexes),
		Affected:  len(indexes),
	}
	beforeStatus := channel.Status
	beforeAllKeysDisabled := beforeStatus == common.ChannelStatusAutoDisabled && channelAccountStatusReasonIsAllKeysDisabled(channel)
	if channel.ChannelInfo.IsMultiKey {
		err := updateMultiKeyChannelAccountsStatusLocked(channel, indexes, enabled, reason, len(keys))
		operation.ChannelRestored = beforeAllKeysDisabled && channel.Status == common.ChannelStatusEnabled
		operation.ChannelDisabled = beforeStatus == common.ChannelStatusEnabled && channel.Status == common.ChannelStatusAutoDisabled
		return operation, err
	}
	if len(indexes) != 1 || indexes[0] != 0 {
		return nil, fmt.Errorf("该渠道不是多账号模式")
	}
	err = updateSingleKeyChannelAccountStatus(channel, enabled, reason)
	operation.ChannelRestored = beforeStatus != common.ChannelStatusEnabled && channel.Status == common.ChannelStatusEnabled
	operation.ChannelDisabled = beforeStatus == common.ChannelStatusEnabled && channel.Status != common.ChannelStatusEnabled
	return operation, err
}

func normalizeChannelAccountIndexes(credentialIndexes []int, keyCount int) ([]int, error) {
	if len(credentialIndexes) == 0 {
		return nil, fmt.Errorf("请先选择账号")
	}
	indexSet := make(map[int]struct{}, len(credentialIndexes))
	for _, credentialIndex := range credentialIndexes {
		if credentialIndex < 0 || credentialIndex >= keyCount {
			return nil, fmt.Errorf("账号索引超出范围")
		}
		indexSet[credentialIndex] = struct{}{}
	}
	indexes := make([]int, 0, len(indexSet))
	for index := range indexSet {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes, nil
}

func updateChannelAccountProxy(channelID int, credentialIndex int, proxyID int, allowReuseRisk bool) (*ChannelAccountOperation, error) {
	return updateChannelAccountsProxy(channelID, []int{credentialIndex}, proxyID, allowReuseRisk)
}

func updateChannelAccountsProxy(channelID int, credentialIndexes []int, proxyID int, allowReuseRisk bool) (*ChannelAccountOperation, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("渠道不存在")
	}
	if proxyID > 0 {
		if _, err := getModelGatewayProxyOrNil(proxyID); err != nil {
			return nil, err
		}
	}
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, fmt.Errorf("渠道不存在")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("渠道没有可管理的账号")
	}
	indexes, err := normalizeChannelAccountIndexes(credentialIndexes, len(keys))
	if err != nil {
		return nil, err
	}
	if proxyID > 0 {
		accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(channel)
		targetAccounts := make([]modelgatewayaccount.ChannelAccount, 0, len(indexes))
		for _, credentialIndex := range indexes {
			account, ok := channelAccountByCredentialIndex(accounts, credentialIndex)
			if !ok {
				return nil, fmt.Errorf("账号不存在")
			}
			targetAccounts = append(targetAccounts, account)
		}
		if err := enforceChannelAccountsProxyReusePolicy(proxyID, targetAccounts, allowReuseRisk); err != nil {
			return nil, err
		}
	}
	cleanupChannelAccountStatusMaps(channel, len(keys))
	if proxyID > 0 {
		if channel.ChannelInfo.MultiKeyProxyIDs == nil {
			channel.ChannelInfo.MultiKeyProxyIDs = make(map[int]int)
		}
		for _, credentialIndex := range indexes {
			channel.ChannelInfo.MultiKeyProxyIDs[credentialIndex] = proxyID
		}
	} else if channel.ChannelInfo.MultiKeyProxyIDs != nil {
		for _, credentialIndex := range indexes {
			delete(channel.ChannelInfo.MultiKeyProxyIDs, credentialIndex)
		}
		if len(channel.ChannelInfo.MultiKeyProxyIDs) == 0 {
			channel.ChannelInfo.MultiKeyProxyIDs = nil
		}
	}
	if err := channel.SaveWithoutKey(); err != nil {
		return nil, err
	}
	if proxyID > 0 {
		for _, credentialIndex := range indexes {
			recordChannelAccountProxyBinding(channel, credentialIndex, proxyID)
		}
	}
	return &ChannelAccountOperation{
		Type:      "proxy",
		Action:    channelAccountProxyAction(proxyID),
		Requested: len(indexes),
		Affected:  len(indexes),
	}, nil
}

func channelAccountByCredentialIndex(accounts []modelgatewayaccount.ChannelAccount, credentialIndex int) (modelgatewayaccount.ChannelAccount, bool) {
	for _, account := range accounts {
		if account.CredentialIndex == credentialIndex {
			return account, true
		}
	}
	return modelgatewayaccount.ChannelAccount{}, false
}

func enforceChannelAccountProxyReusePolicy(proxyID int, account modelgatewayaccount.ChannelAccount, allowReuseRisk bool) error {
	return enforceChannelAccountsProxyReusePolicy(proxyID, []modelgatewayaccount.ChannelAccount{account}, allowReuseRisk)
}

func enforceChannelAccountsProxyReusePolicy(proxyID int, accounts []modelgatewayaccount.ChannelAccount, allowReuseRisk bool) error {
	policy := scheduler_setting.GetSetting().ProxySameBrandReusePolicy
	if policy == "" {
		policy = scheduler_setting.ProxyReusePolicyWarn
	}
	if policy == scheduler_setting.ProxyReusePolicyWarn {
		return nil
	}
	risk, err := detectChannelAccountsProxyReuseRisk(proxyID, accounts)
	if err != nil {
		return err
	}
	if risk == nil {
		return nil
	}
	switch policy {
	case scheduler_setting.ProxyReusePolicyConfirm:
		if allowReuseRisk {
			return nil
		}
		return fmt.Errorf("该代理已被同品牌其他账号使用，请确认后继续绑定")
	case scheduler_setting.ProxyReusePolicyBlock:
		return fmt.Errorf("该代理已被同品牌其他账号使用，请选择其它代理")
	default:
		return fmt.Errorf("invalid proxy_same_brand_reuse_policy")
	}
}

func detectChannelAccountProxyReuseRisk(proxyID int, account modelgatewayaccount.ChannelAccount) (*ModelGatewayProxyReuseRisk, error) {
	return detectChannelAccountsProxyReuseRisk(proxyID, []modelgatewayaccount.ChannelAccount{account})
}

func detectChannelAccountsProxyReuseRisk(proxyID int, accounts []modelgatewayaccount.ChannelAccount) (*ModelGatewayProxyReuseRisk, error) {
	if proxyID <= 0 || len(accounts) == 0 {
		return nil, nil
	}
	targetUsages := make([]model.ModelGatewayProxyUsage, 0, len(accounts))
	for _, account := range accounts {
		accountUsage, ok := channelAccountProxyRiskUsage(proxyID, account)
		if !ok {
			continue
		}
		targetUsages = append(targetUsages, accountUsage)
	}
	if len(targetUsages) == 0 {
		return nil, nil
	}
	usages, err := model.ListModelGatewayProxyUsages([]int{proxyID})
	if err != nil {
		return nil, err
	}
	relevant := make([]model.ModelGatewayProxyUsage, 0, len(usages)+len(targetUsages))
	for _, usage := range usages {
		for _, target := range targetUsages {
			if sameProxyRiskBrand(usage, target) && !sameProxyRiskCredentialScope(usage, target) {
				relevant = append(relevant, usage)
				break
			}
		}
	}
	seenTargets := make(map[string]struct{}, len(targetUsages))
	for _, target := range targetUsages {
		scopeKey := proxyRiskCredentialScopeKey(target)
		if scopeKey != "" {
			if _, ok := seenTargets[scopeKey]; ok {
				continue
			}
			seenTargets[scopeKey] = struct{}{}
		}
		relevant = append(relevant, target)
	}
	risks := buildModelGatewayProxyReuseRisks(relevant)
	if len(risks) == 0 {
		return nil, nil
	}
	return &risks[0], nil
}

func channelAccountProxyRiskUsage(proxyID int, account modelgatewayaccount.ChannelAccount) (model.ModelGatewayProxyUsage, bool) {
	brand := strings.ToLower(strings.TrimSpace(account.AccountIdentity.Brand))
	if brand == "" {
		brand = strings.ToLower(strings.TrimSpace(account.AccountIdentity.Provider))
	}
	if proxyID <= 0 || brand == "" {
		return model.ModelGatewayProxyUsage{}, false
	}
	return model.ModelGatewayProxyUsage{
		ProxyID:                      proxyID,
		ChannelID:                    account.ChannelID,
		ResourceID:                   account.ResourceRef.ResourceID,
		ResourceType:                 account.ResourceRef.ResourceType,
		AccountID:                    account.AccountIdentity.AccountID,
		AccountType:                  account.AccountIdentity.AccountType,
		Brand:                        account.AccountIdentity.Brand,
		Provider:                     account.AccountIdentity.Provider,
		CredentialIndex:              account.CredentialIndex,
		CredentialSubjectFingerprint: account.AccountIdentity.CredentialSubjectFingerprint,
		CredentialFingerprint:        account.AccountIdentity.CredentialFingerprint,
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
	}, true
}

func sameProxyRiskBrand(left model.ModelGatewayProxyUsage, right model.ModelGatewayProxyUsage) bool {
	return proxyRiskBrandKey(left) != "" && proxyRiskBrandKey(left) == proxyRiskBrandKey(right)
}

func proxyRiskBrandKey(usage model.ModelGatewayProxyUsage) string {
	brand := strings.ToLower(strings.TrimSpace(usage.Brand))
	if brand == "" {
		brand = strings.ToLower(strings.TrimSpace(usage.Provider))
	}
	return brand
}

func sameProxyRiskCredentialScope(left model.ModelGatewayProxyUsage, right model.ModelGatewayProxyUsage) bool {
	leftSubject := strings.TrimSpace(left.CredentialSubjectFingerprint)
	rightSubject := strings.TrimSpace(right.CredentialSubjectFingerprint)
	if leftSubject != "" && rightSubject != "" {
		return leftSubject == rightSubject
	}
	leftCredential := strings.TrimSpace(left.CredentialFingerprint)
	rightCredential := strings.TrimSpace(right.CredentialFingerprint)
	if leftCredential != "" && rightCredential != "" {
		return leftCredential == rightCredential
	}
	return false
}

func proxyRiskCredentialScopeKey(usage model.ModelGatewayProxyUsage) string {
	brand := proxyRiskBrandKey(usage)
	if brand == "" {
		return ""
	}
	if subject := strings.TrimSpace(usage.CredentialSubjectFingerprint); subject != "" {
		return brand + "|subject|" + subject
	}
	if credential := strings.TrimSpace(usage.CredentialFingerprint); credential != "" {
		return brand + "|credential|" + credential
	}
	return ""
}

func recordChannelAccountProxyBinding(channel *model.Channel, credentialIndex int, proxyID int) {
	if channel == nil || proxyID <= 0 {
		return
	}
	accounts := modelgatewayaccount.NewRegistry().AccountsForChannel(channel)
	for _, account := range accounts {
		if account.CredentialIndex != credentialIndex {
			continue
		}
		_ = model.RecordModelGatewayProxyUsage(model.ModelGatewayProxyUsage{
			ProxyID:                      proxyID,
			ChannelID:                    channel.Id,
			ResourceID:                   account.ResourceRef.ResourceID,
			ResourceType:                 account.ResourceRef.ResourceType,
			AccountID:                    account.AccountIdentity.AccountID,
			AccountType:                  account.AccountIdentity.AccountType,
			Brand:                        account.AccountIdentity.Brand,
			Provider:                     account.AccountIdentity.Provider,
			CredentialIndex:              account.CredentialIndex,
			CredentialSubjectFingerprint: account.AccountIdentity.CredentialSubjectFingerprint,
			CredentialFingerprint:        account.AccountIdentity.CredentialFingerprint,
			LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
		})
		return
	}
}

func channelAccountProxyAction(proxyID int) string {
	if proxyID > 0 {
		return "bind"
	}
	return "clear"
}

func normalizeChannelAccountEditableCredential(credential string, credentialType string) (string, string, error) {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return "", "", fmt.Errorf("请填写账号凭证")
	}
	accountType := strings.ToLower(strings.TrimSpace(credentialType))
	switch accountType {
	case "", "auto":
		if compacted, ok := compactJSONCredential(credential); ok {
			return compacted, "", nil
		}
		return credential, "", nil
	case modelgatewaycore.AccountTypeAPIKey:
		if strings.HasPrefix(credential, "{") || strings.HasPrefix(credential, "[") {
			return "", "", fmt.Errorf("API Key 类型不支持 JSON 凭证")
		}
		return credential, modelgatewaycore.AccountTypeAPIKey, nil
	case modelgatewaycore.AccountTypeJSONAuth,
		modelgatewaycore.AccountTypeOAuthAccount,
		modelgatewaycore.AccountTypeTokenKey,
		modelgatewaycore.AccountTypeSessionCookie,
		modelgatewaycore.AccountTypeComposite:
		compacted, ok := compactJSONCredential(credential)
		if !ok {
			return "", "", fmt.Errorf("该凭证类型需要填写 JSON 对象")
		}
		return compacted, accountType, nil
	default:
		return "", "", fmt.Errorf("账号凭证类型无效")
	}
}

func normalizeChannelAccountEditableCredentialForChannel(channel *model.Channel, credential string, credentialType string) (string, string, error) {
	normalizedCredential, accountType, err := normalizeChannelAccountEditableCredential(credential, credentialType)
	if err != nil {
		return "", "", err
	}
	if err := validateChannelAccountCredentialsForChannel(channel, []string{normalizedCredential}); err != nil {
		return "", "", err
	}
	return normalizedCredential, accountType, nil
}

func validateChannelAccountCredentialsForChannel(channel *model.Channel, credentials []string) error {
	if channel == nil || channel.Type != constant.ChannelTypeCodex {
		return nil
	}
	for _, credential := range credentials {
		if err := validateCodexChannelCredential(credential); err != nil {
			return err
		}
	}
	return nil
}

type normalizedChannelAccountCredentials struct {
	Keys              []string
	InputCount        int
	DuplicateInInput  int
	SkippedBlankInput int
}

func importChannelAccounts(channelID int, credentials string, credentialList []string, onlyNew bool) (*ChannelAccountOperation, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("渠道不存在")
	}
	normalizedCredentials := normalizeChannelAccountCredentialLines(credentials, credentialList)
	if len(normalizedCredentials.Keys) == 0 {
		return nil, fmt.Errorf("请先输入账号凭证")
	}

	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, fmt.Errorf("渠道不存在")
	}
	if err := validateChannelAccountCredentialsForChannel(channel, normalizedCredentials.Keys); err != nil {
		return nil, err
	}
	existingKeys := channel.GetKeys()
	existingSet := make(map[string]struct{}, len(existingKeys))
	for _, key := range existingKeys {
		normalized := strings.TrimSpace(key)
		if normalized != "" {
			existingSet[normalized] = struct{}{}
		}
	}
	nextKeys := make([]string, 0, len(existingKeys)+len(normalizedCredentials.Keys))
	nextKeys = append(nextKeys, existingKeys...)
	added := 0
	skippedExisting := 0
	for _, key := range normalizedCredentials.Keys {
		if _, exists := existingSet[key]; exists {
			if onlyNew {
				skippedExisting++
				continue
			}
			return nil, fmt.Errorf("账号凭证已存在")
		}
		existingSet[key] = struct{}{}
		nextKeys = append(nextKeys, key)
		added++
	}
	if added == 0 {
		return nil, fmt.Errorf("没有可导入的新账号")
	}

	wasAutoDisabledByAllKeys := channelAccountShouldRestoreAfterImport(channel)
	channel.Key = strings.Join(nextKeys, "\n")
	channel.ChannelInfo.IsMultiKey = len(nextKeys) > 1
	channel.ChannelInfo.MultiKeySize = len(nextKeys)
	if channel.ChannelInfo.IsMultiKey && channel.ChannelInfo.MultiKeyMode == "" {
		channel.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
	}
	cleanupChannelAccountStatusMaps(channel, len(nextKeys))
	if wasAutoDisabledByAllKeys {
		channel.Status = common.ChannelStatusEnabled
		clearChannelAccountStatusReason(channel)
	}
	if err := channel.Update(); err != nil {
		return nil, err
	}
	if err := model.UpdateAbilityStatus(channel.Id, channel.Status == common.ChannelStatusEnabled); err != nil {
		return nil, err
	}
	skippedDuplicate := normalizedCredentials.DuplicateInInput
	return &ChannelAccountOperation{
		Type:             "import",
		TotalInput:       normalizedCredentials.InputCount,
		Requested:        normalizedCredentials.InputCount,
		Affected:         added,
		Added:            added,
		Skipped:          skippedExisting + skippedDuplicate,
		SkippedExisting:  skippedExisting,
		SkippedDuplicate: skippedDuplicate,
		ChannelRestored:  wasAutoDisabledByAllKeys && channel.Status == common.ChannelStatusEnabled,
	}, nil
}

func normalizeChannelAccountCredentialLines(credentials string, credentialList []string) normalizedChannelAccountCredentials {
	lines := make([]string, 0)
	for _, value := range credentialList {
		lines = append(lines, value)
	}
	trimmedCredentials := strings.TrimSpace(credentials)
	if jsonCredentials, ok := parseJSONCredentialInput(trimmedCredentials); ok {
		lines = append(lines, jsonCredentials...)
	} else {
		lines = append(lines, strings.Split(strings.ReplaceAll(credentials, "\r\n", "\n"), "\n")...)
	}

	result := normalizedChannelAccountCredentials{
		Keys: make([]string, 0, len(lines)),
	}
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key == "" {
			result.SkippedBlankInput++
			continue
		}
		if compacted, ok := compactJSONCredential(key); ok {
			key = compacted
		}
		result.InputCount++
		if _, ok := seen[key]; ok {
			result.DuplicateInInput++
			continue
		}
		seen[key] = struct{}{}
		result.Keys = append(result.Keys, key)
	}
	return result
}

func parseJSONCredentialInput(value string) ([]string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if strings.HasPrefix(value, "{") {
		var payload map[string]interface{}
		if err := common.Unmarshal([]byte(value), &payload); err != nil {
			return nil, false
		}
		if parsed, ok := parseJSONCredentialContainer(payload); ok {
			return parsed, true
		}
		compacted, err := common.Marshal(payload)
		if err != nil {
			return nil, false
		}
		return []string{string(compacted)}, true
	}
	if strings.HasPrefix(value, "[") {
		var rawItems []interface{}
		if err := common.Unmarshal([]byte(value), &rawItems); err != nil {
			return nil, false
		}
		items := make([]string, 0, len(rawItems))
		for _, rawItem := range rawItems {
			switch item := rawItem.(type) {
			case string:
				item = strings.TrimSpace(item)
				if compacted, ok := compactJSONCredential(item); ok {
					item = compacted
				}
				items = append(items, item)
			default:
				itemBytes, err := common.Marshal(item)
				if err != nil {
					return nil, false
				}
				items = append(items, string(itemBytes))
			}
		}
		return items, true
	}
	return nil, false
}

func parseJSONCredentialContainer(payload map[string]interface{}) ([]string, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	parser := &ChannelAccountImportParser{}
	for _, key := range []string{"credential_list", "accounts", "credentials"} {
		credentials, ok, err := parser.credentialsFromJSONField(payload[key], false)
		if err != nil {
			return nil, false
		}
		if ok {
			return credentials, true
		}
	}
	return nil, false
}

func compactJSONCredential(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "{") {
		return "", false
	}
	var payload map[string]interface{}
	if err := common.Unmarshal([]byte(value), &payload); err != nil {
		return "", false
	}
	compacted, err := common.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(compacted), true
}

func deleteChannelAccounts(channelID int, credentialIndexes []int) (*ChannelAccountOperation, error) {
	if channelID <= 0 {
		return nil, fmt.Errorf("渠道不存在")
	}
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, fmt.Errorf("渠道不存在")
	}
	keys := channel.GetKeys()
	indexes, err := normalizeChannelAccountIndexes(credentialIndexes, len(keys))
	if err != nil {
		return nil, err
	}

	deleteSet := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		deleteSet[index] = struct{}{}
	}
	beforeStatus := channel.Status
	beforeAllKeysDisabled := beforeStatus == common.ChannelStatusAutoDisabled && channelAccountStatusReasonIsAllKeysDisabled(channel)
	remainingKeys := make([]string, 0, len(keys)-len(indexes))
	newStatusList := make(map[int]int)
	newDisabledTime := make(map[int]int64)
	newDisabledReason := make(map[int]string)
	newProxyIDs := make(map[int]int)
	newAccountTypes := make(map[int]string)
	newCapabilities := make(map[int]model.ChannelAccountCapability)
	newIndex := 0
	for oldIndex, key := range keys {
		if _, shouldDelete := deleteSet[oldIndex]; shouldDelete {
			continue
		}
		remainingKeys = append(remainingKeys, key)
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			if status, exists := channel.ChannelInfo.MultiKeyStatusList[oldIndex]; exists && status != common.ChannelStatusEnabled {
				newStatusList[newIndex] = status
			}
		}
		if channel.ChannelInfo.MultiKeyDisabledTime != nil {
			if disabledTime, exists := channel.ChannelInfo.MultiKeyDisabledTime[oldIndex]; exists {
				newDisabledTime[newIndex] = disabledTime
			}
		}
		if channel.ChannelInfo.MultiKeyDisabledReason != nil {
			if disabledReason, exists := channel.ChannelInfo.MultiKeyDisabledReason[oldIndex]; exists {
				newDisabledReason[newIndex] = disabledReason
			}
		}
		if channel.ChannelInfo.MultiKeyProxyIDs != nil {
			if proxyID, exists := channel.ChannelInfo.MultiKeyProxyIDs[oldIndex]; exists && proxyID > 0 {
				newProxyIDs[newIndex] = proxyID
			}
		}
		if channel.ChannelInfo.MultiKeyAccountTypes != nil {
			if accountType, exists := channel.ChannelInfo.MultiKeyAccountTypes[oldIndex]; exists && isKnownChannelAccountType(accountType) {
				newAccountTypes[newIndex] = strings.ToLower(strings.TrimSpace(accountType))
			}
		}
		if channel.ChannelInfo.MultiKeyCapabilities != nil {
			if capability, exists := channel.ChannelInfo.MultiKeyCapabilities[oldIndex]; exists {
				newCapabilities[newIndex] = capability
			}
		}
		newIndex++
	}

	if err := deleteChannelAccountBoundProxyUsages(channel.Id, indexes); err != nil {
		return nil, err
	}

	channel.Key = strings.Join(remainingKeys, "\n")
	channel.ChannelInfo.IsMultiKey = len(remainingKeys) > 1
	channel.ChannelInfo.MultiKeySize = len(remainingKeys)
	channel.ChannelInfo.MultiKeyStatusList = newStatusList
	channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
	channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason
	channel.ChannelInfo.MultiKeyProxyIDs = newProxyIDs
	channel.ChannelInfo.MultiKeyAccountTypes = newAccountTypes
	channel.ChannelInfo.MultiKeyCapabilities = newCapabilities
	if !channel.ChannelInfo.IsMultiKey {
		channel.ChannelInfo.MultiKeyStatusList = nil
		channel.ChannelInfo.MultiKeyDisabledTime = nil
		channel.ChannelInfo.MultiKeyDisabledReason = nil
	}
	if len(channel.ChannelInfo.MultiKeyProxyIDs) == 0 {
		channel.ChannelInfo.MultiKeyProxyIDs = nil
	}
	if len(channel.ChannelInfo.MultiKeyAccountTypes) == 0 {
		channel.ChannelInfo.MultiKeyAccountTypes = nil
	}
	if len(channel.ChannelInfo.MultiKeyCapabilities) == 0 {
		channel.ChannelInfo.MultiKeyCapabilities = nil
	}
	if channel.ChannelInfo.MultiKeyPollingIndex >= len(remainingKeys) {
		channel.ChannelInfo.MultiKeyPollingIndex = 0
	}
	if len(remainingKeys) == 0 {
		channel.ChannelInfo.MultiKeyPollingIndex = 0
	}

	enabledCount := channelAccountEnabledCount(channel, len(remainingKeys))
	if enabledCount == 0 {
		channel.Status = common.ChannelStatusAutoDisabled
		setChannelAccountStatusReason(channel, channelAccountAllKeysDisabledReason)
	} else if channel.Status == common.ChannelStatusAutoDisabled && channelAccountStatusReasonIsAllKeysDisabled(channel) {
		channel.Status = common.ChannelStatusEnabled
		clearChannelAccountStatusReason(channel)
	}

	if err := saveChannelAccountsAfterDelete(channel); err != nil {
		return nil, err
	}
	if err := model.UpdateAbilityStatus(channel.Id, channel.Status == common.ChannelStatusEnabled); err != nil {
		return nil, err
	}
	return &ChannelAccountOperation{
		Type:            "delete",
		Requested:       len(credentialIndexes),
		Affected:        len(indexes),
		Deleted:         len(indexes),
		ChannelRestored: beforeAllKeysDisabled && channel.Status == common.ChannelStatusEnabled,
		ChannelDisabled: beforeStatus == common.ChannelStatusEnabled && channel.Status == common.ChannelStatusAutoDisabled,
	}, nil
}

func saveChannelAccountsAfterDelete(channel *model.Channel) error {
	if channel == nil {
		return fmt.Errorf("渠道不存在")
	}
	if strings.TrimSpace(channel.Key) != "" {
		return channel.Update()
	}
	return model.DB.Model(&model.Channel{}).
		Where("id = ?", channel.Id).
		Select("key", "status", "channel_info", "other_info").
		Updates(channel).Error
}

func deleteChannelAccountBoundProxyUsages(channelID int, credentialIndexes []int) error {
	if len(credentialIndexes) == 0 {
		return nil
	}
	return model.DB.
		Where("channel_id = ? AND credential_index IN ? AND last_status = ?", channelID, credentialIndexes, model.ModelGatewayProxyUsageStatusBound).
		Delete(&model.ModelGatewayProxyUsage{}).Error
}

func channelAccountStatusAction(enabled bool) string {
	if enabled {
		return "enable"
	}
	return "disable"
}

func updateSingleKeyChannelAccountStatus(channel *model.Channel, enabled bool, reason string) error {
	beforeStatus := channel.Status
	if enabled {
		channel.Status = common.ChannelStatusEnabled
		clearChannelAccountStatusReason(channel)
	} else {
		channel.Status = common.ChannelStatusManuallyDisabled
		setChannelAccountStatusReason(channel, normalizeChannelAccountDisabledReason(reason))
	}
	if err := channel.SaveWithoutKey(); err != nil {
		return err
	}
	if beforeStatus != channel.Status {
		if err := model.UpdateAbilityStatus(channel.Id, channel.Status == common.ChannelStatusEnabled); err != nil {
			return err
		}
	}
	return nil
}

func updateMultiKeyChannelAccountStatus(channel *model.Channel, credentialIndex int, enabled bool, reason string, keyCount int) error {
	return updateMultiKeyChannelAccountsStatusLocked(channel, []int{credentialIndex}, enabled, reason, keyCount)
}

func updateMultiKeyChannelAccountsStatusLocked(channel *model.Channel, credentialIndexes []int, enabled bool, reason string, keyCount int) error {
	beforeStatus := channel.Status
	channel.ChannelInfo.MultiKeySize = keyCount
	cleanupChannelAccountStatusMaps(channel, keyCount)
	if !enabled {
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
	}
	now := common.GetTimestamp()
	disabledReason := normalizeChannelAccountDisabledReason(reason)
	for _, credentialIndex := range credentialIndexes {
		if enabled {
			delete(channel.ChannelInfo.MultiKeyStatusList, credentialIndex)
			delete(channel.ChannelInfo.MultiKeyDisabledReason, credentialIndex)
			delete(channel.ChannelInfo.MultiKeyDisabledTime, credentialIndex)
			continue
		}
		channel.ChannelInfo.MultiKeyStatusList[credentialIndex] = common.ChannelStatusManuallyDisabled
		channel.ChannelInfo.MultiKeyDisabledReason[credentialIndex] = disabledReason
		channel.ChannelInfo.MultiKeyDisabledTime[credentialIndex] = now
	}

	enabledCount := channelAccountEnabledCount(channel, keyCount)
	if enabledCount == 0 {
		if channel.Status == common.ChannelStatusEnabled ||
			(channel.Status == common.ChannelStatusAutoDisabled && channelAccountStatusReasonIsAllKeysDisabled(channel)) {
			channel.Status = common.ChannelStatusAutoDisabled
			setChannelAccountStatusReason(channel, channelAccountAllKeysDisabledReason)
		}
	} else if channel.Status == common.ChannelStatusAutoDisabled && channelAccountStatusReasonIsAllKeysDisabled(channel) {
		channel.Status = common.ChannelStatusEnabled
		clearChannelAccountStatusReason(channel)
	}

	if err := channel.SaveWithoutKey(); err != nil {
		return err
	}
	if beforeStatus != channel.Status {
		if err := model.UpdateAbilityStatus(channel.Id, channel.Status == common.ChannelStatusEnabled); err != nil {
			return err
		}
	}
	return nil
}

func normalizeChannelAccountDisabledReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return channelAccountManualDisabledReason
	}
	return reason
}

func cleanupChannelAccountStatusMaps(channel *model.Channel, keyCount int) {
	if channel.ChannelInfo.MultiKeyStatusList != nil {
		for index, status := range channel.ChannelInfo.MultiKeyStatusList {
			if index < 0 || index >= keyCount || status == common.ChannelStatusEnabled {
				delete(channel.ChannelInfo.MultiKeyStatusList, index)
			}
		}
	}
	if channel.ChannelInfo.MultiKeyDisabledReason != nil {
		for index := range channel.ChannelInfo.MultiKeyDisabledReason {
			if index < 0 || index >= keyCount {
				delete(channel.ChannelInfo.MultiKeyDisabledReason, index)
			}
		}
	}
	if channel.ChannelInfo.MultiKeyDisabledTime != nil {
		for index := range channel.ChannelInfo.MultiKeyDisabledTime {
			if index < 0 || index >= keyCount {
				delete(channel.ChannelInfo.MultiKeyDisabledTime, index)
			}
		}
	}
	if channel.ChannelInfo.MultiKeyProxyIDs != nil {
		for index, proxyID := range channel.ChannelInfo.MultiKeyProxyIDs {
			if index < 0 || index >= keyCount || proxyID <= 0 {
				delete(channel.ChannelInfo.MultiKeyProxyIDs, index)
			}
		}
		if len(channel.ChannelInfo.MultiKeyProxyIDs) == 0 {
			channel.ChannelInfo.MultiKeyProxyIDs = nil
		}
	}
	if channel.ChannelInfo.MultiKeyAccountTypes != nil {
		for index, accountType := range channel.ChannelInfo.MultiKeyAccountTypes {
			if index < 0 || index >= keyCount || !isKnownChannelAccountType(accountType) {
				delete(channel.ChannelInfo.MultiKeyAccountTypes, index)
			}
		}
		if len(channel.ChannelInfo.MultiKeyAccountTypes) == 0 {
			channel.ChannelInfo.MultiKeyAccountTypes = nil
		}
	}
	if channel.ChannelInfo.MultiKeyCapabilities != nil {
		for index := range channel.ChannelInfo.MultiKeyCapabilities {
			if index < 0 || index >= keyCount {
				delete(channel.ChannelInfo.MultiKeyCapabilities, index)
			}
		}
		if len(channel.ChannelInfo.MultiKeyCapabilities) == 0 {
			channel.ChannelInfo.MultiKeyCapabilities = nil
		}
	}
}

func isKnownChannelAccountType(accountType string) bool {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case modelgatewaycore.AccountTypeAPIKey,
		modelgatewaycore.AccountTypeJSONAuth,
		modelgatewaycore.AccountTypeOAuthAccount,
		modelgatewaycore.AccountTypeTokenKey,
		modelgatewaycore.AccountTypeSessionCookie,
		modelgatewaycore.AccountTypeComposite:
		return true
	default:
		return false
	}
}

func channelAccountEnabledCount(channel *model.Channel, keyCount int) int {
	if keyCount <= 0 {
		return 0
	}
	count := 0
	for index := 0; index < keyCount; index++ {
		status := common.ChannelStatusEnabled
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			if value, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok {
				status = value
			}
		}
		if status == common.ChannelStatusEnabled {
			count++
		}
	}
	return count
}

func channelAccountShouldRestoreAfterImport(channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusAutoDisabled {
		return false
	}
	return channelAccountStatusReasonIsAllKeysDisabled(channel)
}

func setChannelAccountStatusReason(channel *model.Channel, reason string) {
	info := channel.GetOtherInfo()
	info["status_reason"] = reason
	info["status_time"] = common.GetTimestamp()
	channel.SetOtherInfo(info)
}

func clearChannelAccountStatusReason(channel *model.Channel) {
	info := channel.GetOtherInfo()
	delete(info, "status_reason")
	delete(info, "status_time")
	delete(info, "pause_type")
	channel.SetOtherInfo(info)
}

func channelAccountStatusReasonIsAllKeysDisabled(channel *model.Channel) bool {
	info := channel.GetOtherInfo()
	reason, _ := info["status_reason"].(string)
	return strings.TrimSpace(reason) == "" || strings.TrimSpace(reason) == channelAccountAllKeysDisabledReason
}

func buildChannelAccountItem(account modelgatewayaccount.ChannelAccount, runtimeItems []modelgatewayobservability.RuntimeStatusItem, allowChannelFallback bool) ChannelAccountItem {
	matches := make([]modelgatewayobservability.RuntimeStatusItem, 0)
	for _, item := range runtimeItems {
		if channelAccountRuntimeItemMatches(account, item, allowChannelFallback) {
			matches = append(matches, item)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return channelAccountRuntimeItemBetter(matches[i], matches[j])
	})

	item := ChannelAccountItem{
		ChannelID:       account.ChannelID,
		CredentialIndex: account.CredentialIndex,
		KeyEnabled:      account.KeyEnabled,
		DisabledReason:  account.DisabledReason,
		ResourceRef:     account.ResourceRef,
		AccountIdentity: account.AccountIdentity,
		CredentialRef:   account.CredentialRef,
		SubjectShort:    modelgatewayaccount.ShortFingerprint(account.AccountIdentity.CredentialSubjectFingerprint),
		CredentialShort: modelgatewayaccount.ShortFingerprint(account.AccountIdentity.CredentialFingerprint),
		RuntimeKeys:     make([]ChannelAccountRuntimeScoreSnapshot, 0, min(len(matches), 5)),
	}
	for _, match := range matches {
		if item.Score == nil {
			summary := channelAccountScoreSummaryFromRuntimeItem(match)
			item.Score = &summary
		}
		if len(item.RuntimeKeys) >= 5 {
			continue
		}
		item.RuntimeKeys = append(item.RuntimeKeys, channelAccountRuntimeSnapshotFromItem(match))
	}
	if len(item.RuntimeKeys) == 0 {
		item.RuntimeKeys = nil
	}
	if item.Score != nil && item.Score.ProbeRecoveryPending {
		item.Stats = ensureChannelAccountStats(item.Stats)
		item.Stats.ProbeRecoveryState = &ChannelAccountProbeRecoveryState{
			Pending:      true,
			Reason:       item.Score.ProbeTriggerReason,
			SuccessCount: item.Score.ProbeRecoverySuccessCount,
			Required:     item.Score.ProbeRecoveryRequired,
		}
	}
	return item
}

func ensureChannelAccountStats(stats *ChannelAccountStats) *ChannelAccountStats {
	if stats != nil {
		return stats
	}
	return &ChannelAccountStats{}
}

func buildChannelAccountSchedulingExplanation(item ChannelAccountItem) *ChannelAccountSchedulingExplanation {
	explanation := &ChannelAccountSchedulingExplanation{
		Schedulable: true,
	}
	blocking := make([]string, 0, 4)
	warnings := make([]string, 0, 4)

	if !item.KeyEnabled {
		blocking = appendChannelAccountSchedulingReason(blocking, "account_disabled")
		if item.DisabledReason != "" {
			explanation.Detail = item.DisabledReason
		}
	}

	if capability := item.Capabilities; capability != nil {
		explanation.CapabilityClassification = capability.EffectiveClassification()
		if capability.UsageLimitActiveAt(common.GetTimestamp()) {
			blocking = appendChannelAccountSchedulingReason(blocking, channelcapability.ClassificationAccountUsageLimited)
			explanation.RecoveryAt = capability.UsageLimitExpiresAt
			explanation.RecoverySource = capability.UsageLimitResetSource
			if capability.UsageLimitMessage != "" {
				explanation.Detail = capability.UsageLimitMessage
			}
		}
		if strings.TrimSpace(capability.ProxyLastError) != "" {
			blocking = appendChannelAccountSchedulingReason(blocking, channelcapability.ClassificationProxyError)
			if explanation.Detail == "" {
				explanation.Detail = capability.ProxyLastError
			}
		}
		if capability.CodexBackendResponsesStreamWrite != nil && !capability.HasCodexBackendResponsesStreamAllowed() {
			blocking = appendChannelAccountSchedulingReason(blocking, "codex_stream_unavailable")
		}
		if capability.CodexBackendCompactWrite != nil && !capability.HasCodexBackendCompactAllowed() {
			warnings = appendChannelAccountSchedulingReason(warnings, "codex_compact_unavailable")
		}
		if capability.CapabilityClassification == channelcapability.ClassificationAuthError {
			blocking = appendChannelAccountSchedulingReason(blocking, channelcapability.ClassificationAuthError)
		}
	}

	if score := item.Score; score != nil {
		explanation.ActiveConcurrency = score.ActiveConcurrency
		explanation.EffectiveConcurrencyCap = score.EffectiveConcurrencyCap
		explanation.QueueDepth = score.QueueDepth
		explanation.QueueCapacity = score.QueueCapacity
		if score.ConfigErrorIsolated {
			blocking = appendChannelAccountSchedulingReason(blocking, "config_error_isolated")
			if explanation.Detail == "" {
				explanation.Detail = score.IsolationReason
			}
		}
		if score.ProbeRecoveryPending {
			blocking = appendChannelAccountSchedulingReason(blocking, "probe_recovery_pending")
			explanation.ProbeRecoveryPending = true
			explanation.ProbeRecoverySuccesses = score.ProbeRecoverySuccessCount
			explanation.ProbeRecoveryRequired = score.ProbeRecoveryRequired
			if explanation.Detail == "" {
				explanation.Detail = score.ProbeTriggerReason
			}
		}
		if score.EffectiveConcurrencyCap > 0 && score.ActiveConcurrency >= score.EffectiveConcurrencyCap {
			blocking = appendChannelAccountSchedulingReason(blocking, "concurrency_full")
		}
		if score.QueueCapacity > 0 && score.QueueDepth >= score.QueueCapacity {
			blocking = appendChannelAccountSchedulingReason(blocking, "queue_full")
		}
		if score.SampleCount == 0 {
			warnings = appendChannelAccountSchedulingReason(warnings, "no_score_sample")
		}
	} else {
		warnings = appendChannelAccountSchedulingReason(warnings, "no_runtime_snapshot")
	}

	if item.Proxy != nil && !item.Proxy.Enabled {
		blocking = appendChannelAccountSchedulingReason(blocking, "proxy_unavailable")
		if explanation.Detail == "" {
			explanation.Detail = item.Proxy.Name
		}
	}

	explanation.BlockingReasons = blocking
	explanation.WarningReasons = warnings
	if len(blocking) > 0 {
		explanation.Schedulable = false
		explanation.PrimaryReason = blocking[0]
	} else if len(warnings) > 0 {
		explanation.PrimaryReason = warnings[0]
	} else {
		explanation.PrimaryReason = "schedulable"
	}
	return explanation
}

func appendChannelAccountSchedulingReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func channelAccountRuntimeItemMatches(account modelgatewayaccount.ChannelAccount, item modelgatewayobservability.RuntimeStatusItem, allowChannelFallback bool) bool {
	if account.ChannelID <= 0 || item.ChannelID != account.ChannelID {
		return false
	}
	if strings.TrimSpace(item.AccountID) == "" &&
		strings.TrimSpace(item.CredentialSubjectFP) == "" &&
		strings.TrimSpace(item.CredentialFP) == "" {
		return allowChannelFallback
	}
	if account.AccountIdentity.AccountID != "" && item.AccountID != "" && item.AccountID == account.AccountIdentity.AccountID {
		return true
	}
	if account.AccountIdentity.CredentialSubjectFingerprint != "" &&
		item.CredentialSubjectFP != "" &&
		item.CredentialSubjectFP == account.AccountIdentity.CredentialSubjectFingerprint {
		return true
	}
	if account.AccountIdentity.CredentialFingerprint != "" &&
		item.CredentialFP != "" &&
		item.CredentialFP == account.AccountIdentity.CredentialFingerprint {
		return true
	}
	return false
}

func channelAccountRuntimeItemBetter(left, right modelgatewayobservability.RuntimeStatusItem) bool {
	leftTime := channelAccountRuntimeItemLatestTime(left)
	rightTime := channelAccountRuntimeItemLatestTime(right)
	if left.SampleCount != right.SampleCount {
		return left.SampleCount > right.SampleCount
	}
	if left.RealSampleCount30m != right.RealSampleCount30m {
		return left.RealSampleCount30m > right.RealSampleCount30m
	}
	if left.ScoreTotal != right.ScoreTotal {
		return left.ScoreTotal > right.ScoreTotal
	}
	if leftTime != rightTime {
		return leftTime > rightTime
	}
	if left.RequestedModel != right.RequestedModel {
		return left.RequestedModel < right.RequestedModel
	}
	return left.Group < right.Group
}

func channelAccountRuntimeItemLatestTime(item modelgatewayobservability.RuntimeStatusItem) int64 {
	return maxInt64(
		maxInt64(item.LastRealAttemptAt, item.LastRealSuccessAt),
		maxInt64(item.LastRealFailureAt, maxInt64(item.LastProbeAt, item.LastProbeSuccessAt)),
	)
}

func channelAccountScoreSummaryFromRuntimeItem(item modelgatewayobservability.RuntimeStatusItem) ChannelAccountScoreSummary {
	return ChannelAccountScoreSummary{
		RuntimeKey:                channelAccountRuntimeKeyFromItem(item),
		HealthStatus:              item.HealthStatus,
		ScoreTotal:                item.ScoreTotal,
		RoutingScoreTotal:         item.RoutingScoreTotal,
		CostItemScore:             item.ScoreBreakdown["cost"],
		CostRatio:                 item.CostRatio,
		CostReferenceRatio:        item.CostReferenceRatio,
		CostPricingMode:           item.CostPricingMode,
		SampleCount:               item.SampleCount,
		RealSampleCount30m:        item.RealSampleCount30m,
		SuccessRate:               item.SuccessRate,
		TTFTMs:                    item.TTFTMs,
		DurationMs:                item.DurationMs,
		TokensPerSecond:           item.TokensPerSecond,
		EmptyOutputRate:           item.EmptyOutputRate,
		ExperienceIssueRate:       item.ExperienceIssueRate,
		LastRealAttemptAt:         item.LastRealAttemptAt,
		LastRealSuccessAt:         item.LastRealSuccessAt,
		LastRealFailureAt:         item.LastRealFailureAt,
		LastProbeAt:               item.LastProbeAt,
		LastProbeSuccessAt:        item.LastProbeSuccessAt,
		ConfigErrorIsolated:       item.ConfigErrorIsolated,
		IsolationReason:           item.IsolationReason,
		ProbeRecoveryPending:      item.ProbeRecoveryPending,
		ProbeTriggerReason:        item.ProbeTriggerReason,
		ProbeRecoverySuccessCount: item.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:     item.ProbeRecoveryRequired,
		ActiveConcurrency:         item.ActiveConcurrency,
		EffectiveConcurrencyCap:   item.EffectiveConcurrencyLimit,
		QueueDepth:                item.QueueDepth,
		QueueCapacity:             item.QueueCapacity,
	}
}

func channelAccountRuntimeSnapshotFromItem(item modelgatewayobservability.RuntimeStatusItem) ChannelAccountRuntimeScoreSnapshot {
	return ChannelAccountRuntimeScoreSnapshot{
		RuntimeKey:          channelAccountRuntimeKeyFromItem(item),
		HealthStatus:        item.HealthStatus,
		ScoreTotal:          item.ScoreTotal,
		RoutingScoreTotal:   item.RoutingScoreTotal,
		SampleCount:         item.SampleCount,
		RealSampleCount30m:  item.RealSampleCount30m,
		SuccessRate:         item.SuccessRate,
		TTFTMs:              item.TTFTMs,
		LastRealSuccessAt:   item.LastRealSuccessAt,
		LastProbeSuccessAt:  item.LastProbeSuccessAt,
		ConfigErrorIsolated: item.ConfigErrorIsolated,
	}
}

func channelAccountRuntimeKeyFromItem(item modelgatewayobservability.RuntimeStatusItem) modelgatewaycore.RuntimeKey {
	return modelgatewaycore.RuntimeKey{
		RequestedModel:        item.RequestedModel,
		UpstreamModel:         item.UpstreamModel,
		ChannelID:             item.ChannelID,
		ResourceID:            item.ResourceID,
		ResourceType:          item.ResourceType,
		AccountID:             item.AccountID,
		AccountType:           item.AccountType,
		Brand:                 item.Brand,
		Provider:              item.Provider,
		CredentialIndex:       item.CredentialIndex,
		CredentialSubjectFP:   item.CredentialSubjectFP,
		CredentialFP:          item.CredentialFP,
		Group:                 item.Group,
		EndpointType:          constant.EndpointType(item.EndpointType),
		CapabilityFingerprint: item.CapabilityFingerprint,
	}
}
