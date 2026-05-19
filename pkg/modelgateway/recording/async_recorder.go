package recording

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const defaultQueueSize = 1024

type AsyncExecutionRecorder struct {
	queue chan event
	once  sync.Once
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
			model.RecordModelExecution(modelExecutionRecordFromDispatch(*e.record))
			continue
		}
		if e.result != nil {
			model.RecordModelExecution(modelExecutionRecordFromAttempt(*e.result))
		}
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
	return &model.ModelExecutionRecord{
		CreatedAt:         time.Now().Unix(),
		RequestId:         result.RequestID,
		AttemptIndex:      result.AttemptIndex,
		ChannelId:         result.ChannelID,
		RequestedGroup:    result.RequestedGroup,
		SelectedGroup:     result.SelectedGroup,
		RequestedModel:    result.ModelName,
		Success:           result.Success,
		StatusCode:        result.StatusCode,
		ErrorCode:         result.ErrorCode,
		ErrorType:         result.ErrorType,
		DurationMs:        result.Duration.Milliseconds(),
		TTFTMs:            result.TTFT.Milliseconds(),
		StreamInterrupted: result.StreamInterrupted,
	}
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
