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
	if !result.IsHealthProbe {
		userrequest.Finish(result, nil)
	}
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
			if !e.result.IsHealthProbe {
				summary := model.RecordModelGatewayUserRequestAttempt(modelGatewayUserRequestAttemptFromResult(*e.result))
				if summary != nil {
					userrequest.Finish(*e.result, summary)
				}
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
	ProviderProfile       string                      `json:"provider_profile,omitempty"`
	ProxyMode             string                      `json:"proxy_mode,omitempty"`
	QueueWaitMs           int                         `json:"queue_wait_ms,omitempty"`
	QueueEnabled          bool                        `json:"queue_enabled,omitempty"`
	QueueDepth            int                         `json:"queue_depth,omitempty"`
	QueueCapacity         int                         `json:"queue_capacity,omitempty"`
	StickySource          string                      `json:"sticky_source,omitempty"`
	StickyKeyFP           string                      `json:"sticky_key_fp,omitempty"`
	StickyRetained        bool                        `json:"sticky_retained,omitempty"`
	StickyBreak           string                      `json:"sticky_break,omitempty"`
	CacheAffinity         bool                        `json:"cache_affinity,omitempty"`
	CandidateExplanations []core.CandidateExplanation `json:"candidate_explanations,omitempty"`
}

type attemptRequestMeta struct {
	ErrorMessage                   string   `json:"error_message,omitempty"`
	ErrorCategory                  string   `json:"error_category,omitempty"`
	RetryAction                    string   `json:"retry_action,omitempty"`
	WillRetry                      bool     `json:"will_retry,omitempty"`
	ClientAborted                  bool     `json:"client_aborted,omitempty"`
	ConcurrencyLimited             bool     `json:"concurrency_limited,omitempty"`
	EmptyOutput                    bool     `json:"empty_output,omitempty"`
	ExperienceIssue                string   `json:"experience_issue,omitempty"`
	ActiveConcurrency              int      `json:"active_concurrency,omitempty"`
	ConfiguredConcurrencyLimit     int      `json:"configured_concurrency_limit,omitempty"`
	LearnedConcurrencyLimit        int      `json:"learned_concurrency_limit,omitempty"`
	LearnedConcurrencyLimitChanged bool     `json:"learned_concurrency_limit_changed,omitempty"`
	UsedChannels                   []string `json:"used_channels,omitempty"`
	IsHealthProbe                  bool     `json:"is_health_probe,omitempty"`
	ProbeReason                    string   `json:"probe_reason,omitempty"`
}

func dispatchRequestMetaFromPlan(plan *core.DispatchPlan) dispatchRequestMeta {
	if plan == nil {
		return dispatchRequestMeta{}
	}
	return dispatchRequestMeta{
		ProviderProfile: plan.ProviderProfile,
		ProxyMode:       plan.ProxyMode,
		QueueWaitMs:     plan.QueueWaitMs,
		QueueEnabled:    plan.QueueEnabled,
		QueueDepth:      plan.QueueDepth,
		QueueCapacity:   plan.QueueCapacity,
		StickySource:    plan.StickySource,
		StickyKeyFP:     plan.StickyKeyFP,
		StickyRetained:  plan.StickyRetained,
		StickyBreak:     plan.StickyBreak,
		CacheAffinity:   plan.CacheAffinity,
		CandidateExplanations: append([]core.CandidateExplanation(nil),
			plan.Candidates...),
	}
}

func modelExecutionRecordFromAttempt(result core.AttemptResult) *model.ModelExecutionRecord {
	record := &model.ModelExecutionRecord{
		CreatedAt:         time.Now().Unix(),
		RequestId:         result.RequestID,
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
		EmptyOutput:                    result.EmptyOutput,
		ExperienceIssue:                result.ExperienceIssue,
		ActiveConcurrency:              result.ActiveConcurrency,
		ConfiguredConcurrencyLimit:     result.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:        result.LearnedConcurrencyLimit,
		LearnedConcurrencyLimitChanged: result.LearnedConcurrencyLimitChanged,
		UsedChannels:                   append([]string(nil), result.UsedChannels...),
		IsHealthProbe:                  result.IsHealthProbe,
		ProbeReason:                    result.ProbeReason,
	}
}

func emptyAttemptRequestMeta(meta attemptRequestMeta) bool {
	return meta.ErrorMessage == "" &&
		meta.ErrorCategory == "" &&
		meta.RetryAction == "" &&
		!meta.WillRetry &&
		!meta.ClientAborted &&
		!meta.ConcurrencyLimited &&
		!meta.EmptyOutput &&
		meta.ExperienceIssue == "" &&
		meta.ActiveConcurrency <= 0 &&
		meta.ConfiguredConcurrencyLimit <= 0 &&
		meta.LearnedConcurrencyLimit <= 0 &&
		!meta.LearnedConcurrencyLimitChanged &&
		len(meta.UsedChannels) == 0 &&
		!meta.IsHealthProbe &&
		meta.ProbeReason == ""
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
