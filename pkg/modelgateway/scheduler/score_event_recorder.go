package scheduler

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

type ScoreEventRecorder struct {
	queue chan model.ModelGatewayScoreEvent
}

func NewScoreEventRecorder(queueSize int) *ScoreEventRecorder {
	if queueSize <= 0 {
		queueSize = 1024
	}
	r := &ScoreEventRecorder{queue: make(chan model.ModelGatewayScoreEvent, queueSize)}
	go r.run()
	return r
}

func (r *ScoreEventRecorder) Record(ctx context.Context, record core.DispatchRecord) {}

func (r *ScoreEventRecorder) Report(ctx context.Context, result core.AttemptResult) {}

func (r *ScoreEventRecorder) ReportAdjustment(result core.AttemptResult, snapshot core.RuntimeSnapshot, decision core.ScoreSampleDecision, before core.ScoreResult, after core.ScoreResult) core.ScoreAdjustment {
	adjustment := scoreAdjustment(result, decision, before, after)
	if r == nil || !decision.ScoreSample || model.DB == nil {
		return adjustment
	}
	if adjustment.TraceID == "" {
		adjustment.TraceID = newScoreTraceID(result, result.ObservedAt)
	}
	event := scoreEventFromAdjustment(result, snapshot, adjustment)
	select {
	case r.queue <- event:
	default:
		common.SysLog("model gateway score event queue full, dropping event")
	}
	return adjustment
}

func (r *ScoreEventRecorder) run() {
	for event := range r.queue {
		if event.TraceID == "" || model.DB == nil {
			continue
		}
		if err := model.DB.Create(&event).Error; err != nil {
			common.SysLog(fmt.Sprintf("failed to write model gateway score event: trace_id=%s error=%v", event.TraceID, err))
		}
	}
}

func scoreAdjustment(result core.AttemptResult, decision core.ScoreSampleDecision, before core.ScoreResult, after core.ScoreResult) core.ScoreAdjustment {
	adjustment := core.ScoreAdjustment{
		TraceID:        newScoreTraceID(result, result.ObservedAt),
		BeforeTotal:    round4(before.Total),
		AfterTotal:     round4(after.Total),
		Delta:          round4(after.Total - before.Total),
		SampleDecision: decision,
		Items:          changedScoreItems(before.Items, after.Items),
	}
	return adjustment
}

func changedScoreItems(before []core.ScoreItem, after []core.ScoreItem) []core.ScoreAdjustmentItem {
	beforeByKey := make(map[string]core.ScoreItem, len(before))
	for _, item := range before {
		beforeByKey[item.Key] = item
	}
	out := make([]core.ScoreAdjustmentItem, 0)
	for _, afterItem := range after {
		beforeItem := beforeByKey[afterItem.Key]
		delta := round4(afterItem.Score - beforeItem.Score)
		weightedDelta := round4(afterItem.WeightedScore - beforeItem.WeightedScore)
		if math.Abs(delta) < 0.0001 && math.Abs(weightedDelta) < 0.0001 {
			continue
		}
		out = append(out, core.ScoreAdjustmentItem{
			Key:             afterItem.Key,
			Name:            afterItem.Name,
			BeforeScore:     beforeItem.Score,
			AfterScore:      afterItem.Score,
			Delta:           delta,
			Weight:          afterItem.Weight,
			WeightedDelta:   weightedDelta,
			BeforeRawValue:  beforeItem.RawValue,
			AfterRawValue:   afterItem.RawValue,
			BeforeRawNumber: beforeItem.RawNumber,
			AfterRawNumber:  afterItem.RawNumber,
			RawUnit:         afterItem.RawUnit,
			Reason:          afterItem.Reason,
		})
	}
	return out
}

func scoreEventFromAdjustment(result core.AttemptResult, snapshot core.RuntimeSnapshot, adjustment core.ScoreAdjustment) model.ModelGatewayScoreEvent {
	decisionJSON := marshalScoreEventJSON(adjustment.SampleDecision)
	itemsJSON := marshalScoreEventJSON(adjustment.Items)
	contextJSON := marshalScoreEventJSON(map[string]any{
		"runtime_key":   snapshot.Key,
		"score_version": 1,
	})
	createdAt := result.ObservedAt.Unix()
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	return model.ModelGatewayScoreEvent{
		TraceID:            adjustment.TraceID,
		RequestID:          strings.TrimSpace(result.RequestID),
		AttemptIndex:       result.AttemptIndex,
		ChannelID:          result.ChannelID,
		RequestedModel:     strings.TrimSpace(snapshot.Key.RequestedModel),
		UpstreamModel:      strings.TrimSpace(snapshot.Key.UpstreamModel),
		Group:              strings.TrimSpace(snapshot.Key.Group),
		EndpointType:       string(snapshot.Key.EndpointType),
		IsHealthProbe:      result.IsHealthProbe,
		Strategy:           strings.TrimSpace(result.Strategy),
		AutoMode:           strings.TrimSpace(result.AutoMode),
		BeforeTotal:        adjustment.BeforeTotal,
		AfterTotal:         adjustment.AfterTotal,
		Delta:              adjustment.Delta,
		SampleDecisionJSON: decisionJSON,
		ChangedItemsJSON:   itemsJSON,
		ContextJSON:        contextJSON,
		CreatedAt:          createdAt,
	}
}

func marshalScoreEventJSON(value any) string {
	data, err := common.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

var _ core.ExecutionRecorder = (*ScoreEventRecorder)(nil)
