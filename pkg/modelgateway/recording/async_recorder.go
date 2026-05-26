package recording

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability/userrequest"
)

const defaultQueueSize = 1024

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
	userrequest.Finish(result, nil)
	r.offer(event{result: &result})
}

func (r *AsyncExecutionRecorder) offer(e event) {
	select {
	case r.queue <- e:
	default:
		common.SysLog("modelgateway recorder queue full, dropping event")
	}
}

func (r *AsyncExecutionRecorder) run() {
	for e := range r.queue {
		if e.record != nil {
			userrequest.Start(*e.record)
			model.RecordModelExecution(modelExecutionRecordFromDispatch(*e.record))
			r.forwardRecord(*e.record)
			continue
		}
		if e.result != nil {
			summary := model.RecordModelGatewayUserRequestAttempt(modelGatewayUserRequestAttemptFromResult(*e.result))
			if summary != nil {
				userrequest.Finish(*e.result, summary)
			}
			model.RecordModelExecution(modelExecutionRecordFromAttempt(*e.result))
			r.forwardResult(*e.result)
		}
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
	BillingRatioMode          string                      `json:"billing_ratio_mode,omitempty"`
	StickySource              string                      `json:"sticky_source,omitempty"`
	StickyKeyFP               string                      `json:"sticky_key_fp,omitempty"`
	StickyRetained            bool                        `json:"sticky_retained,omitempty"`
	StickyBreak               string                      `json:"sticky_break,omitempty"`
	CacheAffinity             bool                        `json:"cache_affinity,omitempty"`
	RequiresCodexImageTool    bool                        `json:"requires_codex_image_tool,omitempty"`
	RequiredTools             []string                    `json:"required_tools,omitempty"`
	CandidateFilterConditions []string                    `json:"candidate_filter_conditions,omitempty"`
	CandidateExplanations     []core.CandidateExplanation `json:"candidate_explanations,omitempty"`
	IsHealthProbe             bool                        `json:"is_health_probe,omitempty"`
	ProbeReason               string                      `json:"probe_reason,omitempty"`
}

type attemptRequestMeta struct {
	ErrorMessage                   string             `json:"error_message,omitempty"`
	ErrorCategory                  string             `json:"error_category,omitempty"`
	RetryAction                    string             `json:"retry_action,omitempty"`
	WillRetry                      bool               `json:"will_retry,omitempty"`
	ClientAborted                  bool               `json:"client_aborted,omitempty"`
	ConcurrencyLimited             bool               `json:"concurrency_limited,omitempty"`
	BalanceInsufficient            bool               `json:"balance_insufficient,omitempty"`
	EmptyOutput                    bool               `json:"empty_output,omitempty"`
	ExperienceIssue                string             `json:"experience_issue,omitempty"`
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
	QueueWaitMs        int64 `json:"queue_wait_ms,omitempty"`
	RelayToFirstByteMs int64 `json:"relay_to_first_byte_ms,omitempty"`
	RelayTotalMs       int64 `json:"relay_total_ms,omitempty"`
	PreFirstByteMs     int64 `json:"pre_first_byte_ms,omitempty"`
	PostFirstByteMs    int64 `json:"post_first_byte_ms,omitempty"`
}

func dispatchRequestMetaFromPlan(plan *core.DispatchPlan) dispatchRequestMeta {
	if plan == nil {
		return dispatchRequestMeta{}
	}
	return dispatchRequestMeta{
		ProviderProfile:        plan.ProviderProfile,
		ProxyMode:              plan.ProxyMode,
		QueueWaitMs:            plan.QueueWaitMs,
		QueueEnabled:           plan.QueueEnabled,
		QueueDepth:             plan.QueueDepth,
		QueueCapacity:          plan.QueueCapacity,
		BillingRatioMode:       plan.BillingRatioMode,
		StickySource:           plan.StickySource,
		StickyKeyFP:            plan.StickyKeyFP,
		StickyRetained:         plan.StickyRetained,
		StickyBreak:            plan.StickyBreak,
		CacheAffinity:          plan.CacheAffinity,
		RequiresCodexImageTool: plan.RequiresCodexImageTool,
		RequiredTools:          append([]string(nil), plan.RequiredTools...),
		CandidateFilterConditions: append([]string(nil),
			plan.CandidateFilterConditions...),
		CandidateExplanations: append([]core.CandidateExplanation(nil),
			plan.Candidates...),
		IsHealthProbe: plan.IsHealthProbe,
		ProbeReason:   plan.ProbeReason,
	}
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
		Success:           result.Success,
		StatusCode:        result.StatusCode,
		ErrorCode:         result.ErrorCode,
		ErrorType:         result.ErrorType,
		ErrorCategory:     result.ErrorCategory,
		DurationMs:        result.Duration.Milliseconds(),
		TTFTMs:            result.TTFT.Milliseconds(),
		StreamInterrupted: result.StreamInterrupted,
	}
	if meta := attemptRequestMetaFromResult(result); !emptyAttemptRequestMeta(meta) {
		record.RequestMeta = marshalToString(meta)
	}
	return record
}

func modelGatewayUserRequestAttemptFromResult(result core.AttemptResult) model.ModelGatewayUserRequestAttempt {
	createdAt := time.Now().Unix()
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
		DurationMs:        result.Duration.Milliseconds(),
		TTFTMs:            result.TTFT.Milliseconds(),
		StreamInterrupted: result.StreamInterrupted,
		WillRetry:         result.WillRetry,
		ClientAborted:     result.ClientAborted,
		IsHealthProbe:     result.IsHealthProbe,
		ProbeReason:       result.ProbeReason,
		EmptyOutput:       result.EmptyOutput,
		ExperienceIssue:   result.ExperienceIssue,
	}
}

func attemptRequestMetaFromResult(result core.AttemptResult) attemptRequestMeta {
	return attemptRequestMeta{
		ErrorMessage:                   result.ErrorMessage,
		ErrorCategory:                  result.ErrorCategory,
		RetryAction:                    result.RetryAction,
		WillRetry:                      result.WillRetry,
		ClientAborted:                  result.ClientAborted,
		ConcurrencyLimited:             result.ConcurrencyLimited,
		BalanceInsufficient:            result.BalanceInsufficient,
		EmptyOutput:                    result.EmptyOutput,
		ExperienceIssue:                result.ExperienceIssue,
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
		meta.RetryAction == "" &&
		!meta.WillRetry &&
		!meta.ClientAborted &&
		!meta.ConcurrencyLimited &&
		!meta.BalanceInsufficient &&
		!meta.EmptyOutput &&
		meta.ExperienceIssue == "" &&
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
		QueueWaitMs:        positiveDurationMs(result.QueueWait),
		RelayToFirstByteMs: positiveDurationMs(result.RelayToFirstByte),
		RelayTotalMs:       positiveDurationMs(result.RelayTotal),
	}
	if timing.RelayToFirstByteMs <= 0 {
		timing.RelayToFirstByteMs = subtractTimingSegment(positiveDurationMs(result.TTFT), timing.QueueWaitMs)
	}
	if timing.RelayTotalMs <= 0 {
		timing.RelayTotalMs = subtractTimingSegment(positiveDurationMs(result.Duration), timing.QueueWaitMs)
	}
	if timing.QueueWaitMs > 0 || timing.RelayToFirstByteMs > 0 {
		timing.PreFirstByteMs = timing.QueueWaitMs + timing.RelayToFirstByteMs
	}
	if timing.RelayTotalMs > 0 && timing.RelayToFirstByteMs > 0 && timing.RelayTotalMs > timing.RelayToFirstByteMs {
		timing.PostFirstByteMs = timing.RelayTotalMs - timing.RelayToFirstByteMs
	}
	if timing.QueueWaitMs <= 0 &&
		timing.RelayToFirstByteMs <= 0 &&
		timing.RelayTotalMs <= 0 &&
		timing.PreFirstByteMs <= 0 &&
		timing.PostFirstByteMs <= 0 {
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
