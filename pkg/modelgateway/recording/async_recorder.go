package recording

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability/userrequest"
)

const (
	defaultQueueSize             = 1024
	resultOverflowEnqueueTimeout = 100 * time.Millisecond
)

type AsyncExecutionRecorder struct {
	queue      chan event
	once       sync.Once
	downstream []core.ExecutionRecorder
}

type event struct {
	record *core.DispatchRecord
	result *core.AttemptResult
}

func NewAsyncExecutionRecorder(queueSize int) *AsyncExecutionRecorder {
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	r := &AsyncExecutionRecorder{
		queue: make(chan event, queueSize),
	}
	r.once.Do(func() {
		go r.run()
	})
	return r
}

func (r *AsyncExecutionRecorder) WithPostProcessors(recorders ...core.ExecutionRecorder) *AsyncExecutionRecorder {
	if r == nil {
		return nil
	}
	for _, recorder := range recorders {
		if recorder != nil {
			r.downstream = append(r.downstream, recorder)
		}
	}
	return r
}

func (r *AsyncExecutionRecorder) Record(ctx context.Context, record core.DispatchRecord) {
	if r == nil {
		return
	}
	r.offer(event{record: &record})
}

func (r *AsyncExecutionRecorder) Report(ctx context.Context, result core.AttemptResult) {
	if r == nil {
		return
	}
	result = normalizeAttemptResultLifecycle(result)
	userrequest.Finish(result, nil)
	r.offer(event{result: &result})
}

func (r *AsyncExecutionRecorder) offer(e event) {
	select {
	case r.queue <- e:
	default:
		if e.result != nil {
			r.offerResultOverflow(e)
			return
		}
		common.SysLog("modelgateway recorder queue full, dropping dispatch event")
	}
}

func (r *AsyncExecutionRecorder) run() {
	for e := range r.queue {
		r.process(e)
	}
}

func (r *AsyncExecutionRecorder) offerResultOverflow(e event) {
	timer := time.NewTimer(resultOverflowEnqueueTimeout)
	defer timer.Stop()
	select {
	case r.queue <- e:
	case <-timer.C:
		common.SysLog("modelgateway recorder queue full, processing result event out of band")
		go r.process(e)
	}
}

func (r *AsyncExecutionRecorder) process(e event) {
	if e.record != nil {
		userrequest.Start(*e.record)
		model.RecordModelExecution(modelExecutionRecordFromDispatch(*e.record))
		recordChannelAccountUsageDispatch(*e.record)
		r.forwardRecord(*e.record)
		return
	}
	if e.result != nil {
		summary := model.RecordModelGatewayUserRequestAttempt(modelGatewayUserRequestAttemptFromResult(*e.result))
		if summary != nil {
			userrequest.Finish(*e.result, summary)
		}
		model.RecordModelExecution(modelExecutionRecordFromAttempt(*e.result))
		recordChannelAccountUsageAttempt(*e.result)
		r.forwardResult(*e.result)
	}
}

func (r *AsyncExecutionRecorder) forwardRecord(record core.DispatchRecord) {
	if r == nil {
		return
	}
	for _, recorder := range r.downstream {
		recorder.Record(context.Background(), record)
	}
}

func (r *AsyncExecutionRecorder) forwardResult(result core.AttemptResult) {
	if r == nil {
		return
	}
	for _, recorder := range r.downstream {
		recorder.Report(context.Background(), result)
	}
}

func modelExecutionRecordFromDispatch(record core.DispatchRecord) *model.ModelExecutionRecord {
	createdAt := record.RecordedAt.Unix()
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	dbRecord := &model.ModelExecutionRecord{
		CreatedAt:       createdAt,
		RequestId:       record.Request.RequestID,
		UserId:          record.Request.UserID,
		TokenId:         record.Request.TokenID,
		RequestedGroup:  record.Request.RequestedGroup,
		RequestedModel:  record.Request.ModelName,
		EndpointType:    string(record.Request.EndpointType),
		PolicyMode:      record.Policy.Mode,
		AutoMode:        record.Policy.AutoMode,
		Strategy:        record.Policy.Strategy,
		Shadow:          record.Shadow,
		ActualGroup:     record.ActualGroup,
		CandidateGroups: marshalToString(record.Policy.CandidateGroups),
	}
	if record.Plan != nil {
		dbRecord.ChannelId = channelID(record.Plan.Channel)
		dbRecord.ChannelName = channelName(record.Plan.Channel)
		dbRecord.SelectedGroup = record.Plan.SelectedGroup
		dbRecord.ScoreTotal = record.Plan.ScoreTotal
		dbRecord.ScoreBreakdown = marshalToString(record.Plan.ScoreBreakdown)
		dbRecord.SelectedReason = record.Plan.SelectedReason
		dbRecord.SmartHandled = true
		dbRecord.FallbackUsed = record.Plan.FallbackUsed
		dbRecord.RequestMeta = marshalToString(dispatchRequestMetaFromPlan(record.Plan))
	}
	if record.Actual != nil {
		dbRecord.ActualChannelId = record.Actual.Id
		dbRecord.ActualChannelName = record.Actual.Name
	}
	return dbRecord
}

type dispatchRequestMeta struct {
	ProviderProfile           string                      `json:"provider_profile,omitempty"`
	ProxyMode                 string                      `json:"proxy_mode,omitempty"`
	QueueWaitMs               int                         `json:"queue_wait_ms,omitempty"`
	QueueEnabled              bool                        `json:"queue_enabled,omitempty"`
	QueueDepth                int                         `json:"queue_depth,omitempty"`
	QueueCapacity             int                         `json:"queue_capacity,omitempty"`
	QueuePriority             int                         `json:"queue_priority,omitempty"`
	BillingRatioMode          string                      `json:"billing_ratio_mode,omitempty"`
	Strategy                  string                      `json:"strategy,omitempty"`
	AutoMode                  string                      `json:"auto_mode,omitempty"`
	StickySource              string                      `json:"sticky_source,omitempty"`
	StickyKeyFP               string                      `json:"sticky_key_fp,omitempty"`
	StickyRetained            bool                        `json:"sticky_retained,omitempty"`
	StickyBreak               string                      `json:"sticky_break,omitempty"`
	StickyDecision            *core.StickyDecision        `json:"sticky_decision,omitempty"`
	CacheAffinity             bool                        `json:"cache_affinity,omitempty"`
	RequiresCodexImageTool    bool                        `json:"requires_codex_image_tool,omitempty"`
	RequiredTools             []string                    `json:"required_tools,omitempty"`
	CandidateFilterConditions []string                    `json:"candidate_filter_conditions,omitempty"`
	CandidateExplanations     []core.CandidateExplanation `json:"candidate_explanations,omitempty"`
	IsHealthProbe             bool                        `json:"is_health_probe,omitempty"`
	ProbeReason               string                      `json:"probe_reason,omitempty"`
	RetryRoutingIntent        *core.RetryRoutingIntent    `json:"retry_routing_intent,omitempty"`
	RetryIntentApplied        bool                        `json:"retry_intent_applied,omitempty"`
	RetryQueuePriorityBoost   bool                        `json:"retry_queue_priority_boost,omitempty"`
	CostGuardDecision         *core.CostGuardDecision     `json:"cost_guard_decision,omitempty"`
	ResourceProtectionEnabled bool                        `json:"resource_protection_enabled,omitempty"`
	ResourceProtectionPhase   string                      `json:"resource_protection_phase,omitempty"`
	ResourceProtectionReason  string                      `json:"resource_protection_reason,omitempty"`
	ResourceProtectionRole    string                      `json:"resource_protection_role,omitempty"`
	PrimaryChannelIDs         []int                       `json:"primary_channel_ids,omitempty"`
	FallbackChannelIDs        []int                       `json:"fallback_channel_ids,omitempty"`
	PrimaryWaitTimeoutMs      int                         `json:"primary_wait_timeout_ms,omitempty"`
	PrimaryQueueMaxDepth      int                         `json:"primary_queue_max_depth,omitempty"`
}

type attemptRequestMeta struct {
	ErrorMessage                   string             `json:"error_message,omitempty"`
	ErrorCategory                  string             `json:"error_category,omitempty"`
	WarningLevel                   string             `json:"warning_level,omitempty"`
	WarningFlags                   []string           `json:"warning_flags,omitempty"`
	WarningMessage                 string             `json:"warning_message,omitempty"`
	ChannelInducedClientAbort      bool               `json:"channel_induced_client_abort,omitempty"`
	RetryAction                    string             `json:"retry_action,omitempty"`
	RetryReason                    string             `json:"retry_reason,omitempty"`
	WillRetry                      bool               `json:"will_retry,omitempty"`
	ClientAborted                  bool               `json:"client_aborted,omitempty"`
	ConcurrencyLimited             bool               `json:"concurrency_limited,omitempty"`
	BalanceInsufficient            bool               `json:"balance_insufficient,omitempty"`
	EmptyOutput                    bool               `json:"empty_output,omitempty"`
	ExperienceIssue                string             `json:"experience_issue,omitempty"`
	UpstreamStatus                 int                `json:"upstream_status,omitempty"`
	DownstreamWriteStatus          string             `json:"downstream_write_status,omitempty"`
	KeepAliveCount                 int                `json:"keepalive_count,omitempty"`
	ClientReceivedStarted          bool               `json:"client_received_started,omitempty"`
	FinalClassification            string             `json:"final_classification,omitempty"`
	ActiveConcurrency              int                `json:"active_concurrency,omitempty"`
	ConfiguredConcurrencyLimit     int                `json:"configured_concurrency_limit,omitempty"`
	LearnedConcurrencyLimit        int                `json:"learned_concurrency_limit,omitempty"`
	LearnedConcurrencyLimitChanged bool               `json:"learned_concurrency_limit_changed,omitempty"`
	UsedChannels                   []string           `json:"used_channels,omitempty"`
	IsHealthProbe                  bool               `json:"is_health_probe,omitempty"`
	ProbeReason                    string             `json:"probe_reason,omitempty"`
	Timing                         *attemptTimingMeta `json:"timing,omitempty"`
}

type attemptTimingMeta struct {
	QueueWaitMs                  int64  `json:"queue_wait_ms,omitempty"`
	RelayToFirstByteMs           int64  `json:"relay_to_first_byte_ms,omitempty"`
	RelayTotalMs                 int64  `json:"relay_total_ms,omitempty"`
	UpstreamResponseHeaderMs     int64  `json:"upstream_response_header_ms,omitempty"`
	UpstreamFirstEventWaitMs     int64  `json:"upstream_first_event_wait_ms,omitempty"`
	PreFirstByteMs               int64  `json:"pre_first_byte_ms,omitempty"`
	PostFirstByteMs              int64  `json:"post_first_byte_ms,omitempty"`
	RequestBodyPrepareMs         int64  `json:"request_body_prepare_ms,omitempty"`
	RequestBodyBytes             int64  `json:"request_body_bytes,omitempty"`
	RequestBodyStorage           string `json:"request_body_storage,omitempty"`
	RequestBodySizeLikelyLatency bool   `json:"request_body_size_likely_latency,omitempty"`
}

func dispatchRequestMetaFromPlan(plan *core.DispatchPlan) dispatchRequestMeta {
	if plan == nil {
		return dispatchRequestMeta{}
	}
	return dispatchRequestMeta{
		ProviderProfile:           plan.ProviderProfile,
		ProxyMode:                 plan.ProxyMode,
		QueueWaitMs:               plan.QueueWaitMs,
		QueueEnabled:              plan.QueueEnabled,
		QueueDepth:                plan.QueueDepth,
		QueueCapacity:             plan.QueueCapacity,
		QueuePriority:             plan.QueuePriority,
		BillingRatioMode:          plan.BillingRatioMode,
		Strategy:                  plan.Strategy,
		AutoMode:                  plan.AutoMode,
		StickySource:              plan.StickySource,
		StickyKeyFP:               plan.StickyKeyFP,
		StickyRetained:            plan.StickyRetained,
		StickyBreak:               plan.StickyBreak,
		StickyDecision:            plan.StickyDecision,
		CacheAffinity:             plan.CacheAffinity,
		RequiresCodexImageTool:    false,
		RequiredTools:             dispatchRequiredToolsForRecord(plan.RequiredTools),
		CandidateFilterConditions: dispatchFilterConditionsForRecord(plan.CandidateFilterConditions),
		CandidateExplanations: append([]core.CandidateExplanation(nil),
			plan.Candidates...),
		IsHealthProbe:             plan.IsHealthProbe,
		ProbeReason:               plan.ProbeReason,
		RetryRoutingIntent:        plan.RetryRoutingIntent.Clone(),
		RetryIntentApplied:        plan.RetryIntentApplied,
		RetryQueuePriorityBoost:   plan.RetryQueuePriorityBoost,
		CostGuardDecision:         plan.CostGuardDecision,
		ResourceProtectionEnabled: plan.ResourceProtectionEnabled,
		ResourceProtectionPhase:   plan.ResourceProtectionPhase,
		ResourceProtectionReason:  plan.ResourceProtectionReason,
		ResourceProtectionRole:    plan.ResourceProtectionRole,
		PrimaryChannelIDs:         append([]int(nil), plan.PrimaryChannelIDs...),
		FallbackChannelIDs:        append([]int(nil), plan.FallbackChannelIDs...),
		PrimaryWaitTimeoutMs:      plan.PrimaryWaitTimeoutMs,
		PrimaryQueueMaxDepth:      plan.PrimaryQueueMaxDepth,
	}
}

func dispatchRequiredToolsForRecord(tools []string) []string {
	if len(tools) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == core.DispatchRequiredToolCodexImageGeneration {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

func dispatchFilterConditionsForRecord(conditions []string) []string {
	if len(conditions) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		if condition == core.DispatchFilterConditionCodexImageGenerationTool {
			continue
		}
		filtered = append(filtered, condition)
	}
	return filtered
}

func modelExecutionRecordFromAttempt(result core.AttemptResult) *model.ModelExecutionRecord {
	record := &model.ModelExecutionRecord{
		CreatedAt:         time.Now().Unix(),
		RequestId:         result.RequestID,
		UserId:            result.UserID,
		TokenId:           result.TokenID,
		AttemptIndex:      result.AttemptIndex,
		ChannelId:         result.ChannelID,
		ChannelName:       result.ChannelName,
		RequestedGroup:    result.RequestedGroup,
		SelectedGroup:     result.SelectedGroup,
		RequestedModel:    result.ModelName,
		EndpointType:      string(result.EndpointType),
		AutoMode:          result.AutoMode,
		Strategy:          result.Strategy,
		Success:           result.Success,
		StatusCode:        result.StatusCode,
		ErrorCode:         result.ErrorCode,
		ErrorType:         result.ErrorType,
		ErrorCategory:     result.ErrorCategory,
		DurationMs:        result.Duration.Milliseconds(),
		TTFTMs:            result.TTFT.Milliseconds(),
		StreamInterrupted: result.StreamInterrupted,
	}
	if result.Plan != nil {
		record.PolicyMode = result.Plan.PolicyMode
		record.AutoMode = result.Plan.AutoMode
		record.Strategy = result.Plan.Strategy
		record.SmartHandled = true
		record.FallbackUsed = result.Plan.FallbackUsed
		record.ScoreTotal = result.Plan.ScoreTotal
		record.ScoreBreakdown = marshalToString(result.Plan.ScoreBreakdown)
		record.SelectedReason = result.Plan.SelectedReason
		record.CandidateGroups = marshalToString([]string{result.Plan.SelectedGroup})
	}
	if meta := attemptRequestMetaFromResult(result); result.Plan != nil || !emptyAttemptRequestMeta(meta) {
		record.RequestMeta = marshalToString(requestMetaFromAttemptResult(result, meta))
	}
	return record
}

func requestMetaFromAttemptResult(result core.AttemptResult, attemptMeta attemptRequestMeta) map[string]any {
	out := make(map[string]any)
	if result.Plan != nil {
		dispatchBytes, err := common.Marshal(dispatchRequestMetaFromPlan(result.Plan))
		if err == nil {
			_ = common.Unmarshal(dispatchBytes, &out)
		}
	}
	attemptBytes, err := common.Marshal(attemptMeta)
	if err == nil {
		attemptMap := make(map[string]any)
		if err := common.Unmarshal(attemptBytes, &attemptMap); err == nil {
			for key, value := range attemptMap {
				out[key] = value
			}
		}
	}
	return out
}

func modelGatewayUserRequestAttemptFromResult(result core.AttemptResult) model.ModelGatewayUserRequestAttempt {
	createdAt := time.Now().Unix()
	if observedAt := result.ObservedAt.Unix(); observedAt > 0 {
		createdAt = observedAt
	}
	return model.ModelGatewayUserRequestAttempt{
		CreatedAt:         createdAt,
		RequestId:         result.RequestID,
		AttemptIndex:      result.AttemptIndex,
		RequestedGroup:    result.RequestedGroup,
		SelectedGroup:     result.SelectedGroup,
		ChannelID:         result.ChannelID,
		ChannelName:       result.ChannelName,
		RequestedModel:    result.ModelName,
		Success:           result.Success,
		StatusCode:        result.StatusCode,
		ErrorCode:         result.ErrorCode,
		ErrorType:         result.ErrorType,
		ErrorCategory:     result.ErrorCategory,
		DurationMs:        requestDuration(result).Milliseconds(),
		TTFTMs:            requestTTFT(result).Milliseconds(),
		StreamInterrupted: result.StreamInterrupted,
		WillRetry:         result.WillRetry,
		RetryAction:       result.RetryAction,
		ClientAborted:     result.ClientAborted,
		IsHealthProbe:     result.IsHealthProbe,
		ProbeReason:       result.ProbeReason,
		EmptyOutput:       result.EmptyOutput,
		ExperienceIssue:   result.ExperienceIssue,
	}
}

func normalizeAttemptResultLifecycle(result core.AttemptResult) core.AttemptResult {
	if !result.WillRetry {
		return result
	}
	if result.Success || result.ClientAborted {
		result.WillRetry = false
		return result
	}
	switch strings.ToLower(strings.TrimSpace(result.RetryAction)) {
	case "switch_channel", "retry", "resource_protection_fallback":
		return result
	default:
		result.WillRetry = false
		if strings.TrimSpace(result.RetryAction) == "" {
			result.RetryAction = "stop"
		}
		return result
	}
}

func recordChannelAccountUsageDispatch(record core.DispatchRecord) {
	if record.Plan == nil {
		return
	}
	if err := model.UpsertChannelAccountUsageDispatch(channelAccountUsageEventFromDispatch(record)); err != nil {
		common.SysLog("failed to upsert channel account usage dispatch: request_id=" + record.Request.RequestID + " error=" + err.Error())
	}
}

func recordChannelAccountUsageAttempt(result core.AttemptResult) {
	if err := model.UpsertChannelAccountUsageAttempt(channelAccountUsageEventFromAttempt(result)); err != nil {
		common.SysLog("failed to upsert channel account usage attempt: request_id=" + result.RequestID + " error=" + err.Error())
	}
}

func channelAccountUsageEventFromDispatch(record core.DispatchRecord) model.ChannelAccountUsageEvent {
	event := model.ChannelAccountUsageEvent{
		RequestId:       record.Request.RequestID,
		RequestedGroup:  record.Request.RequestedGroup,
		RequestedModel:  record.Request.ModelName,
		EndpointType:    string(record.Request.EndpointType),
		SelectedGroup:   record.ActualGroup,
		IsHealthProbe:   false,
		CredentialIndex: -1,
	}
	if record.RecordedAt.Unix() > 0 {
		event.CreatedAt = record.RecordedAt.Unix()
		event.UpdatedAt = event.CreatedAt
	}
	if record.Plan != nil {
		event.ChannelID = channelID(record.Plan.Channel)
		event.ChannelName = channelName(record.Plan.Channel)
		event.SelectedGroup = record.Plan.SelectedGroup
		event.IsHealthProbe = record.Plan.IsHealthProbe
		applyChannelAccountUsagePlan(&event, record.Plan)
	}
	if record.Actual != nil {
		event.ChannelID = record.Actual.Id
		event.ChannelName = record.Actual.Name
	}
	return event
}

func channelAccountUsageEventFromAttempt(result core.AttemptResult) model.ChannelAccountUsageEvent {
	observedAt := result.ObservedAt.Unix()
	if observedAt <= 0 {
		observedAt = time.Now().Unix()
	}
	event := model.ChannelAccountUsageEvent{
		RequestId:       result.RequestID,
		AttemptIndex:    result.AttemptIndex,
		ChannelID:       result.ChannelID,
		ChannelName:     result.ChannelName,
		CredentialIndex: -1,
		RequestedGroup:  result.RequestedGroup,
		SelectedGroup:   result.SelectedGroup,
		RequestedModel:  result.ModelName,
		EndpointType:    string(result.EndpointType),
		CompletedAt:     observedAt,
		Success:         result.Success && !result.StreamInterrupted && !result.ClientAborted,
		StatusCode:      result.StatusCode,
		ErrorCategory:   result.ErrorCategory,
		IsHealthProbe:   result.IsHealthProbe,
		DurationMs:      requestDuration(result).Milliseconds(),
		TTFTMs:          requestTTFT(result).Milliseconds(),
		CreatedAt:       observedAt,
		UpdatedAt:       observedAt,
	}
	if result.Plan != nil {
		applyChannelAccountUsagePlan(&event, result.Plan)
		if event.ChannelID <= 0 {
			event.ChannelID = channelID(result.Plan.Channel)
		}
		if event.ChannelName == "" {
			event.ChannelName = channelName(result.Plan.Channel)
		}
	}
	if event.ChannelID <= 0 {
		key := result.RuntimeKey()
		applyChannelAccountUsageRuntimeKey(&event, key)
	}
	return event
}

func applyChannelAccountUsagePlan(event *model.ChannelAccountUsageEvent, plan *core.DispatchPlan) {
	if event == nil || plan == nil {
		return
	}
	if plan.Channel != nil {
		event.ChannelID = plan.Channel.Id
		event.ChannelName = plan.Channel.Name
	}
	if plan.SelectedGroup != "" {
		event.SelectedGroup = plan.SelectedGroup
	}
	if plan.RuntimeKey.RequestedModel != "" {
		event.RequestedModel = plan.RuntimeKey.RequestedModel
	}
	if plan.RuntimeKey.Group != "" {
		event.SelectedGroup = plan.RuntimeKey.Group
	}
	if plan.RuntimeKey.EndpointType != "" {
		event.EndpointType = string(plan.RuntimeKey.EndpointType)
	}
	applyChannelAccountUsageRuntimeKey(event, plan.RuntimeKey)
	identity := plan.AccountIdentity
	if identity.AccountID != "" {
		event.AccountID = identity.AccountID
	}
	if identity.AccountIdentityKey != "" {
		event.AccountIdentityKey = identity.AccountIdentityKey
	}
	if identity.CredentialIndex >= 0 {
		event.CredentialIndex = identity.CredentialIndex
	}
	if identity.CredentialSubjectFingerprint != "" {
		event.CredentialSubjectFingerprint = identity.CredentialSubjectFingerprint
	}
	if identity.CredentialFingerprint != "" {
		event.CredentialFingerprint = identity.CredentialFingerprint
	}
	if identity.AccountType != "" {
		event.AccountType = identity.AccountType
	}
	if identity.Brand != "" {
		event.Brand = identity.Brand
	}
	if identity.Provider != "" {
		event.Provider = identity.Provider
	}
	if plan.CredentialRef.CredentialIndex >= 0 {
		event.CredentialIndex = plan.CredentialRef.CredentialIndex
	}
	if plan.CredentialRef.CredentialSubjectFingerprint != "" {
		event.CredentialSubjectFingerprint = plan.CredentialRef.CredentialSubjectFingerprint
	}
	if plan.CredentialRef.CredentialFingerprint != "" {
		event.CredentialFingerprint = plan.CredentialRef.CredentialFingerprint
	}
	if plan.ResourceRef.Brand != "" {
		event.Brand = plan.ResourceRef.Brand
	}
	if plan.ResourceRef.Provider != "" {
		event.Provider = plan.ResourceRef.Provider
	}
}

func applyChannelAccountUsageRuntimeKey(event *model.ChannelAccountUsageEvent, key core.RuntimeKey) {
	if event == nil {
		return
	}
	if key.ChannelID > 0 {
		event.ChannelID = key.ChannelID
	}
	if key.RequestedModel != "" {
		event.RequestedModel = key.RequestedModel
	}
	if key.Group != "" {
		event.SelectedGroup = key.Group
	}
	if key.EndpointType != "" {
		event.EndpointType = string(key.EndpointType)
	}
	if key.AccountID != "" {
		event.AccountID = key.AccountID
		if event.AccountIdentityKey == "" {
			event.AccountIdentityKey = key.AccountID
		}
	}
	if key.CredentialIndex >= 0 {
		event.CredentialIndex = key.CredentialIndex
	}
	if key.CredentialSubjectFP != "" {
		event.CredentialSubjectFingerprint = key.CredentialSubjectFP
	}
	if key.CredentialFP != "" {
		event.CredentialFingerprint = key.CredentialFP
	}
	if key.AccountType != "" {
		event.AccountType = key.AccountType
	}
	if key.Brand != "" {
		event.Brand = key.Brand
	}
	if key.Provider != "" {
		event.Provider = key.Provider
	}
}

func requestDuration(result core.AttemptResult) time.Duration {
	if result.RequestDuration > 0 {
		return result.RequestDuration
	}
	return result.Duration
}

func requestTTFT(result core.AttemptResult) time.Duration {
	if result.RequestTTFT > 0 {
		return result.RequestTTFT
	}
	return result.TTFT
}

func attemptRequestMetaFromResult(result core.AttemptResult) attemptRequestMeta {
	return attemptRequestMeta{
		ErrorMessage:                   result.ErrorMessage,
		ErrorCategory:                  result.ErrorCategory,
		WarningLevel:                   result.WarningLevel,
		WarningFlags:                   append([]string(nil), result.WarningFlags...),
		WarningMessage:                 result.WarningMessage,
		ChannelInducedClientAbort:      result.ChannelInducedClientAbort,
		RetryAction:                    result.RetryAction,
		RetryReason:                    result.RetryReason,
		WillRetry:                      result.WillRetry,
		ClientAborted:                  result.ClientAborted,
		ConcurrencyLimited:             result.ConcurrencyLimited,
		BalanceInsufficient:            result.BalanceInsufficient,
		EmptyOutput:                    result.EmptyOutput,
		ExperienceIssue:                result.ExperienceIssue,
		UpstreamStatus:                 result.UpstreamStatus,
		DownstreamWriteStatus:          result.DownstreamWriteStatus,
		KeepAliveCount:                 result.KeepAliveCount,
		ClientReceivedStarted:          result.ClientReceivedStarted,
		FinalClassification:            result.FinalClassification,
		ActiveConcurrency:              result.ActiveConcurrency,
		ConfiguredConcurrencyLimit:     result.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:        result.LearnedConcurrencyLimit,
		LearnedConcurrencyLimitChanged: result.LearnedConcurrencyLimitChanged,
		UsedChannels:                   append([]string(nil), result.UsedChannels...),
		IsHealthProbe:                  result.IsHealthProbe,
		ProbeReason:                    result.ProbeReason,
		Timing:                         attemptTimingMetaFromResult(result),
	}
}

func emptyAttemptRequestMeta(meta attemptRequestMeta) bool {
	return meta.ErrorMessage == "" &&
		meta.ErrorCategory == "" &&
		meta.WarningLevel == "" &&
		len(meta.WarningFlags) == 0 &&
		meta.WarningMessage == "" &&
		!meta.ChannelInducedClientAbort &&
		meta.RetryAction == "" &&
		meta.RetryReason == "" &&
		!meta.WillRetry &&
		!meta.ClientAborted &&
		!meta.ConcurrencyLimited &&
		!meta.BalanceInsufficient &&
		!meta.EmptyOutput &&
		meta.ExperienceIssue == "" &&
		meta.UpstreamStatus == 0 &&
		meta.DownstreamWriteStatus == "" &&
		meta.KeepAliveCount == 0 &&
		!meta.ClientReceivedStarted &&
		meta.FinalClassification == "" &&
		meta.ActiveConcurrency <= 0 &&
		meta.ConfiguredConcurrencyLimit <= 0 &&
		meta.LearnedConcurrencyLimit <= 0 &&
		!meta.LearnedConcurrencyLimitChanged &&
		len(meta.UsedChannels) == 0 &&
		!meta.IsHealthProbe &&
		meta.ProbeReason == "" &&
		meta.Timing == nil
}

func attemptTimingMetaFromResult(result core.AttemptResult) *attemptTimingMeta {
	timing := &attemptTimingMeta{
		QueueWaitMs:              positiveDurationMs(result.QueueWait),
		RelayToFirstByteMs:       positiveDurationMs(result.RelayToFirstByte),
		RelayTotalMs:             positiveDurationMs(result.RelayTotal),
		UpstreamResponseHeaderMs: positiveDurationMs(result.UpstreamResponseHeader),
		RequestBodyPrepareMs:     positiveDurationMs(result.RequestBodyPrepare),
		RequestBodyBytes:         result.RequestBodyBytes,
		RequestBodyStorage:       result.RequestBodyStorage,
	}
	if timing.RelayToFirstByteMs <= 0 {
		timing.RelayToFirstByteMs = subtractTimingSegment(positiveDurationMs(result.TTFT), timing.QueueWaitMs)
	}
	if timing.RelayTotalMs <= 0 {
		timing.RelayTotalMs = subtractTimingSegment(positiveDurationMs(result.Duration), timing.QueueWaitMs)
	}
	if timing.UpstreamResponseHeaderMs > 0 && timing.RelayToFirstByteMs > timing.UpstreamResponseHeaderMs {
		timing.UpstreamFirstEventWaitMs = timing.RelayToFirstByteMs - timing.UpstreamResponseHeaderMs
	}
	if timing.QueueWaitMs > 0 || timing.RelayToFirstByteMs > 0 {
		timing.PreFirstByteMs = timing.QueueWaitMs + timing.RelayToFirstByteMs
	}
	if timing.RelayTotalMs > 0 && timing.RelayToFirstByteMs > 0 && timing.RelayTotalMs > timing.RelayToFirstByteMs {
		timing.PostFirstByteMs = timing.RelayTotalMs - timing.RelayToFirstByteMs
	}
	if timing.RequestBodyBytes >= 1<<20 {
		timing.RequestBodySizeLikelyLatency = true
	}
	if timing.QueueWaitMs <= 0 &&
		timing.RelayToFirstByteMs <= 0 &&
		timing.RelayTotalMs <= 0 &&
		timing.UpstreamResponseHeaderMs <= 0 &&
		timing.UpstreamFirstEventWaitMs <= 0 &&
		timing.PreFirstByteMs <= 0 &&
		timing.PostFirstByteMs <= 0 &&
		timing.RequestBodyPrepareMs <= 0 &&
		timing.RequestBodyBytes <= 0 &&
		timing.RequestBodyStorage == "" {
		return nil
	}
	return timing
}

func subtractTimingSegment(totalMs, segmentMs int64) int64 {
	if totalMs <= 0 {
		return 0
	}
	if segmentMs <= 0 {
		return totalMs
	}
	if totalMs <= segmentMs {
		return 0
	}
	return totalMs - segmentMs
}

func positiveDurationMs(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return value.Milliseconds()
}

func marshalToString(v any) string {
	if v == nil {
		return ""
	}
	bytes, err := common.Marshal(v)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func channelID(channel *model.Channel) int {
	if channel == nil {
		return 0
	}
	return channel.Id
}

func channelName(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	return channel.Name
}
