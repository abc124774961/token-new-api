package userrequest

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	StatusProcessing  = "processing"
	StatusSuccess     = "success"
	StatusProbe       = "health_probe"
	StatusFailed      = "failed"
	StatusProbeFailed = "health_probe_failed"

	defaultMaxPending = 200
	defaultTTL        = 10 * time.Minute
)

type EventKind string

const (
	EventStarted  EventKind = "started"
	EventFinished EventKind = "finished"
)

type Event struct {
	Kind   EventKind
	Record Record
}

type Record struct {
	ID                 int    `json:"id"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at"`
	RequestID          string `json:"request_id"`
	RequestedModel     string `json:"requested_model"`
	RequestedGroup     string `json:"requested_group"`
	SelectedGroup      string `json:"selected_group,omitempty"`
	FinalChannelID     int    `json:"final_channel_id,omitempty"`
	FinalChannelName   string `json:"final_channel_name,omitempty"`
	Attempts           int    `json:"attempts"`
	FinalSuccess       bool   `json:"final_success"`
	Recovered          bool   `json:"recovered"`
	FinalStatusCode    int    `json:"final_status_code,omitempty"`
	FinalErrorCategory string `json:"final_error_category,omitempty"`
	EmptyOutput        bool   `json:"empty_output,omitempty"`
	ExperienceIssue    string `json:"experience_issue,omitempty"`
	ClientAborted      bool   `json:"client_aborted,omitempty"`
	IsHealthProbe      bool   `json:"is_health_probe,omitempty"`
	ProbeReason        string `json:"probe_reason,omitempty"`
	DurationMs         int64  `json:"duration_ms,omitempty"`
	TTFTMs             int64  `json:"ttft_ms,omitempty"`
	Status             string `json:"status,omitempty"`
}

type Observer func(Event)

type Tracker struct {
	mu         sync.RWMutex
	pending    map[string]Record
	finished   map[string]int64
	maxPending int
	ttl        time.Duration
	observerMu sync.RWMutex
	observers  []Observer
}

var DefaultTracker = NewTracker(defaultMaxPending, defaultTTL)

func NewTracker(maxPending int, ttl time.Duration) *Tracker {
	if maxPending <= 0 {
		maxPending = defaultMaxPending
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Tracker{
		pending:    map[string]Record{},
		finished:   map[string]int64{},
		maxPending: maxPending,
		ttl:        ttl,
	}
}

func AddObserver(observer Observer) {
	DefaultTracker.AddObserver(observer)
}

func Start(record core.DispatchRecord) {
	DefaultTracker.Start(record)
}

func Finish(result core.AttemptResult, summary *model.ModelGatewayUserRequestSummary) {
	DefaultTracker.Finish(result, summary)
}

func Snapshot(limit int, filters Filters) []Record {
	return DefaultTracker.Snapshot(limit, filters)
}

func (t *Tracker) AddObserver(observer Observer) {
	if t == nil || observer == nil {
		return
	}
	t.observerMu.Lock()
	defer t.observerMu.Unlock()
	t.observers = append(t.observers, observer)
}

func (t *Tracker) Start(record core.DispatchRecord) {
	if t == nil || strings.TrimSpace(record.Request.RequestID) == "" || record.Shadow {
		return
	}
	now := record.RecordedAt.Unix()
	if now == 0 {
		now = time.Now().Unix()
	}
	selectedGroup := ""
	finalChannelID := 0
	finalChannelName := ""
	if record.Plan != nil {
		selectedGroup = strings.TrimSpace(record.Plan.SelectedGroup)
		if record.Plan.Channel != nil {
			finalChannelID = record.Plan.Channel.Id
			finalChannelName = strings.TrimSpace(record.Plan.Channel.Name)
		}
	}
	item := Record{
		CreatedAt:        now,
		CompletedAt:      0,
		RequestID:        strings.TrimSpace(record.Request.RequestID),
		RequestedModel:   strings.TrimSpace(record.Request.ModelName),
		RequestedGroup:   strings.TrimSpace(record.Request.RequestedGroup),
		SelectedGroup:    selectedGroup,
		FinalChannelID:   finalChannelID,
		FinalChannelName: finalChannelName,
		Attempts:         0,
		IsHealthProbe:    record.Plan != nil && record.Plan.IsHealthProbe,
		Status:           StatusProcessing,
	}
	if record.Plan != nil {
		item.ProbeReason = strings.TrimSpace(record.Plan.ProbeReason)
	}

	t.mu.Lock()
	t.pruneLocked(time.Now())
	if _, done := t.finished[item.RequestID]; done {
		t.mu.Unlock()
		return
	}
	t.pending[item.RequestID] = item
	t.pruneOverflowLocked()
	t.mu.Unlock()

	t.notify(Event{Kind: EventStarted, Record: item})
}

func (t *Tracker) Finish(result core.AttemptResult, summary *model.ModelGatewayUserRequestSummary) {
	if t == nil || strings.TrimSpace(result.RequestID) == "" {
		return
	}
	requestID := strings.TrimSpace(result.RequestID)
	t.mu.Lock()
	pending, hadPending := t.pending[requestID]
	if modelGatewayAttemptFinalized(result) {
		delete(t.pending, requestID)
		t.finished[requestID] = time.Now().Unix()
	} else {
		if hadPending {
			pending.Attempts = maxInt(pending.Attempts, result.AttemptIndex+1)
			if pending.RequestedModel == "" {
				pending.RequestedModel = strings.TrimSpace(result.ModelName)
			}
			if pending.RequestedGroup == "" {
				pending.RequestedGroup = strings.TrimSpace(result.RequestedGroup)
			}
			if strings.TrimSpace(result.SelectedGroup) != "" {
				pending.SelectedGroup = strings.TrimSpace(result.SelectedGroup)
			}
			if result.ChannelID > 0 {
				pending.FinalChannelID = result.ChannelID
			}
			if strings.TrimSpace(result.ChannelName) != "" {
				pending.FinalChannelName = strings.TrimSpace(result.ChannelName)
			}
			t.pending[requestID] = pending
		}
		t.mu.Unlock()
		if hadPending {
			t.notify(Event{Kind: EventStarted, Record: pending})
		}
		return
	}
	t.mu.Unlock()

	record := userRequestRecordFromResult(result, summary, pending)
	t.notify(Event{Kind: EventFinished, Record: record})
}

func (t *Tracker) Snapshot(limit int, filters Filters) []Record {
	if t == nil {
		return []Record{}
	}
	now := time.Now()
	t.mu.Lock()
	t.pruneLocked(now)
	items := make([]Record, 0, len(t.pending))
	for _, item := range t.pending {
		if filters.Match(item) {
			items = append(items, item)
		}
	}
	t.mu.Unlock()
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt > items[j].CreatedAt
		}
		return items[i].RequestID > items[j].RequestID
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (t *Tracker) notify(event Event) {
	t.observerMu.RLock()
	observers := append([]Observer(nil), t.observers...)
	t.observerMu.RUnlock()
	for _, observer := range observers {
		observer(event)
	}
}

func (t *Tracker) pruneLocked(now time.Time) {
	if t == nil || t.ttl <= 0 {
		return
	}
	cutoff := now.Add(-t.ttl).Unix()
	for requestID, item := range t.pending {
		if item.CreatedAt > 0 && item.CreatedAt < cutoff {
			delete(t.pending, requestID)
		}
	}
	for requestID, finishedAt := range t.finished {
		if finishedAt > 0 && finishedAt < cutoff {
			delete(t.finished, requestID)
		}
	}
}

func (t *Tracker) pruneOverflowLocked() {
	if t == nil || t.maxPending <= 0 || len(t.pending) <= t.maxPending {
		return
	}
	items := make([]Record, 0, len(t.pending))
	for _, item := range t.pending {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt > items[j].CreatedAt
		}
		return items[i].RequestID > items[j].RequestID
	})
	for idx := t.maxPending; idx < len(items); idx++ {
		delete(t.pending, items[idx].RequestID)
	}
}

type Filters struct {
	Model     string
	Group     string
	ChannelID int
	RequestID string
	Hours     int
}

func (f Filters) Match(record Record) bool {
	if f.Model != "" && f.Model != record.RequestedModel {
		return false
	}
	if f.Group != "" && f.Group != record.RequestedGroup && f.Group != record.SelectedGroup {
		return false
	}
	if f.ChannelID > 0 && f.ChannelID != record.FinalChannelID {
		return false
	}
	if f.RequestID != "" && f.RequestID != record.RequestID {
		return false
	}
	if f.Hours > 0 {
		cutoff := time.Now().Add(-time.Duration(f.Hours) * time.Hour).Unix()
		if record.CreatedAt < cutoff {
			return false
		}
	}
	return true
}

func userRequestRecordFromResult(result core.AttemptResult, summary *model.ModelGatewayUserRequestSummary, pending Record) Record {
	if summary != nil {
		clientAborted := userRequestSummaryClientAborted(summary)
		return Record{
			ID:                 summary.Id,
			CreatedAt:          summary.CreatedAt,
			CompletedAt:        summary.CompletedAt,
			RequestID:          summary.RequestId,
			RequestedModel:     summary.RequestedModel,
			RequestedGroup:     summary.RequestedGroup,
			SelectedGroup:      summary.SelectedGroup,
			FinalChannelID:     summary.FinalChannelID,
			FinalChannelName:   summary.FinalChannelName,
			Attempts:           summary.Attempts,
			FinalSuccess:       summary.FinalSuccess,
			Recovered:          summary.Recovered,
			FinalStatusCode:    summary.FinalStatusCode,
			FinalErrorCategory: summary.FinalErrorCategory,
			EmptyOutput:        summary.EmptyOutput,
			ExperienceIssue:    summary.ExperienceIssue,
			ClientAborted:      clientAborted,
			IsHealthProbe:      result.IsHealthProbe || summary.IsHealthProbe,
			ProbeReason:        firstNonEmpty(result.ProbeReason, summary.ProbeReason),
			DurationMs:         summary.DurationMs,
			TTFTMs:             summary.TTFTMs,
			Status:             userRequestStatus(summary.FinalSuccess, clientAborted, result.IsHealthProbe || summary.IsHealthProbe),
		}
	}
	clientAborted := userRequestResultClientAborted(result)
	success := result.Success && !result.StreamInterrupted && !clientAborted
	completedAt := time.Now().Unix()
	record := Record{
		CreatedAt:          pending.CreatedAt,
		CompletedAt:        completedAt,
		RequestID:          strings.TrimSpace(result.RequestID),
		RequestedModel:     strings.TrimSpace(result.ModelName),
		RequestedGroup:     strings.TrimSpace(result.RequestedGroup),
		SelectedGroup:      strings.TrimSpace(result.SelectedGroup),
		FinalChannelID:     result.ChannelID,
		FinalChannelName:   strings.TrimSpace(result.ChannelName),
		Attempts:           maxInt(pending.Attempts, result.AttemptIndex+1),
		FinalSuccess:       success,
		Recovered:          false,
		FinalStatusCode:    0,
		FinalErrorCategory: "",
		EmptyOutput:        result.EmptyOutput,
		ExperienceIssue:    strings.TrimSpace(result.ExperienceIssue),
		ClientAborted:      clientAborted,
		IsHealthProbe:      result.IsHealthProbe,
		ProbeReason:        strings.TrimSpace(result.ProbeReason),
		DurationMs:         userRequestResultDuration(result).Milliseconds(),
		TTFTMs:             userRequestResultTTFT(result).Milliseconds(),
		Status:             userRequestStatus(success, clientAborted, result.IsHealthProbe),
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = completedAt
	}
	if !success {
		record.FinalStatusCode = result.StatusCode
		if clientAborted {
			record.FinalErrorCategory = model.ModelGatewayUserRequestErrorClientAborted
		} else {
			record.FinalErrorCategory = model.NormalizeModelGatewayUserRequestErrorCategory(
				result.ErrorCategory,
				result.ErrorCode,
				result.ErrorType,
				result.StatusCode,
				result.StreamInterrupted,
				success,
			)
		}
	}
	if record.RequestedModel == "" {
		record.RequestedModel = pending.RequestedModel
	}
	if record.RequestedGroup == "" {
		record.RequestedGroup = pending.RequestedGroup
	}
	if record.SelectedGroup == "" {
		record.SelectedGroup = pending.SelectedGroup
	}
	if record.FinalChannelID == 0 {
		record.FinalChannelID = pending.FinalChannelID
	}
	if record.FinalChannelName == "" {
		record.FinalChannelName = pending.FinalChannelName
	}
	return record
}

func userRequestResultDuration(result core.AttemptResult) time.Duration {
	if result.RequestDuration > 0 {
		return result.RequestDuration
	}
	return result.Duration
}

func userRequestResultTTFT(result core.AttemptResult) time.Duration {
	if result.RequestTTFT > 0 {
		return result.RequestTTFT
	}
	return result.TTFT
}

func modelGatewayAttemptFinalized(result core.AttemptResult) bool {
	return !result.WillRetry || result.Success || result.StreamInterrupted
}

func userRequestStatus(success bool, clientAborted bool, healthProbe bool) string {
	if clientAborted {
		return "client_aborted"
	}
	if healthProbe {
		if success {
			return StatusProbe
		}
		return StatusProbeFailed
	}
	if success {
		return StatusSuccess
	}
	return StatusFailed
}

func userRequestSummaryClientAborted(summary *model.ModelGatewayUserRequestSummary) bool {
	if summary == nil {
		return false
	}
	category := strings.ToLower(strings.TrimSpace(summary.FinalErrorCategory))
	return summary.ClientAborted ||
		summary.FinalStatusCode == 499 ||
		category == model.ModelGatewayUserRequestErrorClientAborted ||
		strings.Contains(category, "client_abort") ||
		strings.Contains(category, "client_gone")
}

func userRequestResultClientAborted(result core.AttemptResult) bool {
	category := strings.ToLower(strings.TrimSpace(result.ErrorCategory))
	return result.ClientAborted ||
		result.StatusCode == 499 ||
		strings.Contains(category, "client_abort") ||
		strings.Contains(category, "client_gone")
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
