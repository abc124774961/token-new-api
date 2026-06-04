package userrequest

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

func init() {
	service.RegisterRelayUpstreamCompletedObserver(func(requestID string, observedAt time.Time, duration time.Duration) {
		ObserveUpstreamCompleted(UpstreamCompletedObservation{
			RequestID:  requestID,
			ObservedAt: observedAt,
			Duration:   duration,
		})
	})
}

const (
	StatusProcessing  = "processing"
	StatusSettling    = "settling"
	StatusSuccess     = "success"
	StatusProbe       = "health_probe"
	StatusFailed      = "failed"
	StatusProbeFailed = "health_probe_failed"

	defaultMaxPending = 200
	defaultTTL        = 10 * time.Minute

	defaultStaleProcessingGrace = 30 * time.Second
	defaultFirstByteTimeout     = 20 * time.Second
	defaultStreamingIdleTimeout = 300 * time.Second
	minStaleProcessingTimeout   = 30 * time.Second
)

type EventKind string

const (
	EventStarted  EventKind = "started"
	EventUpdated  EventKind = "updated"
	EventFinished EventKind = "finished"
)

type Event struct {
	Kind   EventKind
	Record Record
}

type Record struct {
	ID                        int      `json:"id"`
	CreatedAt                 int64    `json:"created_at"`
	UpdatedAt                 int64    `json:"updated_at"`
	CompletedAt               int64    `json:"completed_at"`
	RequestID                 string   `json:"request_id"`
	UserID                    int      `json:"user_id,omitempty"`
	Username                  string   `json:"username,omitempty"`
	RequestedModel            string   `json:"requested_model"`
	RequestedGroup            string   `json:"requested_group"`
	SelectedGroup             string   `json:"selected_group,omitempty"`
	FinalChannelID            int      `json:"final_channel_id,omitempty"`
	FinalChannelName          string   `json:"final_channel_name,omitempty"`
	Attempts                  int      `json:"attempts"`
	FinalSuccess              bool     `json:"final_success"`
	Recovered                 bool     `json:"recovered"`
	FinalStatusCode           int      `json:"final_status_code,omitempty"`
	FinalErrorCategory        string   `json:"final_error_category,omitempty"`
	WarningLevel              string   `json:"warning_level,omitempty"`
	WarningFlags              []string `json:"warning_flags,omitempty"`
	WarningMessage            string   `json:"warning_message,omitempty"`
	ChannelInducedClientAbort bool     `json:"channel_induced_client_abort,omitempty"`
	EmptyOutput               bool     `json:"empty_output,omitempty"`
	ExperienceIssue           string   `json:"experience_issue,omitempty"`
	ClientAborted             bool     `json:"client_aborted,omitempty"`
	IsHealthProbe             bool     `json:"is_health_probe,omitempty"`
	ProbeReason               string   `json:"probe_reason,omitempty"`
	DurationMs                int64    `json:"duration_ms,omitempty"`
	TTFTMs                    int64    `json:"ttft_ms,omitempty"`
	Status                    string   `json:"status,omitempty"`
}

type FirstByteObservation struct {
	RequestID  string
	ObservedAt time.Time
	TTFT       time.Duration
}

type ActivityObservation struct {
	RequestID  string
	ObservedAt time.Time
}

type UpstreamCompletedObservation struct {
	RequestID  string
	ObservedAt time.Time
	Duration   time.Duration
}

type Observer func(Event)

type Tracker struct {
	mu          sync.RWMutex
	pending     map[string]Record
	finished    map[string]int64
	staleFinals map[string]Record
	firstBytes  map[string]FirstByteObservation
	completed   map[string]UpstreamCompletedObservation
	maxPending  int
	ttl         time.Duration
	observerMu  sync.RWMutex
	observers   []Observer
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
		pending:     map[string]Record{},
		finished:    map[string]int64{},
		staleFinals: map[string]Record{},
		firstBytes:  map[string]FirstByteObservation{},
		completed:   map[string]UpstreamCompletedObservation{},
		maxPending:  maxPending,
		ttl:         ttl,
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

func ObserveFirstByte(observation FirstByteObservation) {
	DefaultTracker.ObserveFirstByte(observation)
}

func ObserveActivity(observation ActivityObservation) {
	DefaultTracker.ObserveActivity(observation)
}

func ObserveUpstreamCompleted(observation UpstreamCompletedObservation) {
	DefaultTracker.ObserveUpstreamCompleted(observation)
}

func Snapshot(limit int, filters Filters) []Record {
	return DefaultTracker.Snapshot(limit, filters)
}

func SweepStale() []Record {
	return DefaultTracker.SweepStale()
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
		UpdatedAt:        now,
		CompletedAt:      0,
		RequestID:        strings.TrimSpace(record.Request.RequestID),
		UserID:           record.Request.UserID,
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
	if observation, ok := t.firstBytes[item.RequestID]; ok {
		applyFirstByteObservation(&item, observation)
		delete(t.firstBytes, item.RequestID)
	}
	if observation, ok := t.completed[item.RequestID]; ok {
		applyUpstreamCompletedObservation(&item, observation)
		delete(t.completed, item.RequestID)
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
		delete(t.firstBytes, requestID)
		delete(t.completed, requestID)
		delete(t.staleFinals, requestID)
		t.finished[requestID] = time.Now().Unix()
	} else {
		if hadPending {
			now := time.Now().Unix()
			pending.UpdatedAt = now
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

func (t *Tracker) ObserveFirstByte(observation FirstByteObservation) {
	if t == nil || strings.TrimSpace(observation.RequestID) == "" {
		return
	}
	requestID := strings.TrimSpace(observation.RequestID)
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	ttftMs := observation.TTFT.Milliseconds()
	if ttftMs <= 0 {
		return
	}

	t.mu.Lock()
	pending, hadPending := t.pending[requestID]
	if !hadPending {
		if _, done := t.finished[requestID]; !done {
			t.firstBytes[requestID] = observation
		}
		t.mu.Unlock()
		return
	}
	applyFirstByteObservation(&pending, observation)
	t.pending[requestID] = pending
	t.mu.Unlock()

	t.notify(Event{Kind: EventStarted, Record: pending})
}

func (t *Tracker) ObserveActivity(observation ActivityObservation) {
	if t == nil || strings.TrimSpace(observation.RequestID) == "" {
		return
	}
	requestID := strings.TrimSpace(observation.RequestID)
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	updatedAt := observedAt.Unix()
	if updatedAt <= 0 {
		return
	}

	t.mu.Lock()
	pending, hadPending := t.pending[requestID]
	if !hadPending {
		t.mu.Unlock()
		return
	}
	if pending.UpdatedAt < updatedAt {
		pending.UpdatedAt = updatedAt
		t.pending[requestID] = pending
	}
	t.mu.Unlock()
}

func (t *Tracker) ObserveUpstreamCompleted(observation UpstreamCompletedObservation) {
	if t == nil || strings.TrimSpace(observation.RequestID) == "" {
		return
	}
	requestID := strings.TrimSpace(observation.RequestID)
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	t.mu.Lock()
	pending, hadPending := t.pending[requestID]
	if !hadPending {
		if _, done := t.finished[requestID]; !done {
			t.completed[requestID] = observation
		}
		t.mu.Unlock()
		return
	}
	applyUpstreamCompletedObservation(&pending, observation)
	t.pending[requestID] = pending
	t.mu.Unlock()

	t.notify(Event{Kind: EventUpdated, Record: pending})
}

func (t *Tracker) Snapshot(limit int, filters Filters) []Record {
	if t == nil {
		return []Record{}
	}
	now := time.Now()
	t.mu.Lock()
	t.pruneLocked(now)
	staleRecords := t.collectStaleFinalsLocked(now)
	items := make([]Record, 0, len(t.pending)+len(t.staleFinals))
	for _, item := range t.pending {
		if processingRecordStale(item, now) {
			continue
		}
		if filters.Match(item) {
			items = append(items, item)
		}
	}
	for _, item := range t.staleFinals {
		if filters.Match(item) {
			items = append(items, item)
		}
	}
	t.mu.Unlock()
	for idx := range staleRecords {
		staleRecords[idx] = persistStaleProcessingFinal(staleRecords[idx])
		t.notify(Event{Kind: EventFinished, Record: staleRecords[idx]})
	}
	sort.Slice(items, func(i, j int) bool {
		return recordSortLess(items[i], items[j])
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (t *Tracker) SweepStale() []Record {
	if t == nil {
		return []Record{}
	}
	now := time.Now()
	t.mu.Lock()
	t.pruneLocked(now)
	records := t.collectStaleFinalsLocked(now)
	t.mu.Unlock()
	for idx := range records {
		records[idx] = persistStaleProcessingFinal(records[idx])
		t.notify(Event{Kind: EventFinished, Record: records[idx]})
	}
	return records
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
			delete(t.staleFinals, requestID)
		}
	}
	for requestID, item := range t.staleFinals {
		if item.CompletedAt > 0 && item.CompletedAt < cutoff {
			delete(t.staleFinals, requestID)
		}
	}
	for requestID, observation := range t.firstBytes {
		observedAt := observation.ObservedAt
		if observedAt.IsZero() {
			delete(t.firstBytes, requestID)
			continue
		}
		if observedAt.Unix() < cutoff {
			delete(t.firstBytes, requestID)
		}
	}
	for requestID, observation := range t.completed {
		observedAt := observation.ObservedAt
		if observedAt.IsZero() {
			delete(t.completed, requestID)
			continue
		}
		if observedAt.Unix() < cutoff {
			delete(t.completed, requestID)
		}
	}
}

func (t *Tracker) collectStaleFinalsLocked(now time.Time) []Record {
	if t == nil {
		return nil
	}
	records := make([]Record, 0)
	for requestID, item := range t.pending {
		if !processingRecordStale(item, now) {
			continue
		}
		delete(t.pending, requestID)
		delete(t.firstBytes, requestID)
		record := staleProcessingFinalRecord(item, now)
		t.finished[requestID] = record.CompletedAt
		t.staleFinals[requestID] = record
		records = append(records, record)
	}
	return records
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
		return recordSortLess(items[i], items[j])
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
		updatedAt := summary.UpdatedAt
		if updatedAt <= 0 {
			updatedAt = summary.CompletedAt
		}
		if updatedAt <= 0 {
			updatedAt = summary.CreatedAt
		}
		completedAt := summary.CompletedAt
		durationMs := summary.DurationMs
		if pending.Status == StatusSettling {
			if pending.CompletedAt > 0 && (completedAt <= 0 || pending.CompletedAt < completedAt) {
				completedAt = pending.CompletedAt
			}
			if pending.DurationMs > 0 && (durationMs <= 0 || pending.CompletedAt == completedAt) {
				durationMs = pending.DurationMs
			}
		}
		return Record{
			ID:                        summary.Id,
			CreatedAt:                 summary.CreatedAt,
			UpdatedAt:                 updatedAt,
			CompletedAt:               completedAt,
			RequestID:                 summary.RequestId,
			UserID:                    firstPositiveInt(pending.UserID, result.UserID),
			Username:                  strings.TrimSpace(pending.Username),
			RequestedModel:            summary.RequestedModel,
			RequestedGroup:            summary.RequestedGroup,
			SelectedGroup:             summary.SelectedGroup,
			FinalChannelID:            summary.FinalChannelID,
			FinalChannelName:          summary.FinalChannelName,
			Attempts:                  summary.Attempts,
			FinalSuccess:              summary.FinalSuccess,
			Recovered:                 summary.Recovered,
			FinalStatusCode:           summary.FinalStatusCode,
			FinalErrorCategory:        summary.FinalErrorCategory,
			WarningLevel:              strings.TrimSpace(result.WarningLevel),
			WarningFlags:              append([]string(nil), result.WarningFlags...),
			WarningMessage:            strings.TrimSpace(result.WarningMessage),
			ChannelInducedClientAbort: result.ChannelInducedClientAbort,
			EmptyOutput:               summary.EmptyOutput,
			ExperienceIssue:           summary.ExperienceIssue,
			ClientAborted:             clientAborted,
			IsHealthProbe:             result.IsHealthProbe || summary.IsHealthProbe,
			ProbeReason:               firstNonEmpty(result.ProbeReason, summary.ProbeReason),
			DurationMs:                durationMs,
			TTFTMs:                    summary.TTFTMs,
			Status:                    userRequestStatus(summary.FinalSuccess, clientAborted, result.IsHealthProbe || summary.IsHealthProbe),
		}
	}
	clientAborted := userRequestResultClientAborted(result)
	success := result.Success && !result.StreamInterrupted && !clientAborted
	completedAt := time.Now().Unix()
	record := Record{
		CreatedAt:                 pending.CreatedAt,
		UpdatedAt:                 completedAt,
		CompletedAt:               completedAt,
		RequestID:                 strings.TrimSpace(result.RequestID),
		UserID:                    firstPositiveInt(pending.UserID, result.UserID),
		Username:                  strings.TrimSpace(pending.Username),
		RequestedModel:            strings.TrimSpace(result.ModelName),
		RequestedGroup:            strings.TrimSpace(result.RequestedGroup),
		SelectedGroup:             strings.TrimSpace(result.SelectedGroup),
		FinalChannelID:            result.ChannelID,
		FinalChannelName:          strings.TrimSpace(result.ChannelName),
		Attempts:                  maxInt(pending.Attempts, result.AttemptIndex+1),
		FinalSuccess:              success,
		Recovered:                 false,
		FinalStatusCode:           0,
		FinalErrorCategory:        "",
		WarningLevel:              strings.TrimSpace(result.WarningLevel),
		WarningFlags:              append([]string(nil), result.WarningFlags...),
		WarningMessage:            strings.TrimSpace(result.WarningMessage),
		ChannelInducedClientAbort: result.ChannelInducedClientAbort,
		EmptyOutput:               result.EmptyOutput,
		ExperienceIssue:           strings.TrimSpace(result.ExperienceIssue),
		ClientAborted:             clientAborted,
		IsHealthProbe:             result.IsHealthProbe,
		ProbeReason:               strings.TrimSpace(result.ProbeReason),
		DurationMs:                userRequestResultDuration(result).Milliseconds(),
		TTFTMs:                    userRequestResultTTFT(result).Milliseconds(),
		Status:                    userRequestStatus(success, clientAborted, result.IsHealthProbe),
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

func recordSortLess(left, right Record) bool {
	leftProcessing := recordProcessing(left)
	rightProcessing := recordProcessing(right)
	if leftProcessing != rightProcessing {
		return leftProcessing
	}
	if left.CreatedAt != right.CreatedAt {
		return left.CreatedAt > right.CreatedAt
	}
	if left.CompletedAt != right.CompletedAt {
		return left.CompletedAt > right.CompletedAt
	}
	if left.ID != right.ID {
		return left.ID > right.ID
	}
	return left.RequestID > right.RequestID
}

func recordProcessing(record Record) bool {
	return strings.TrimSpace(record.Status) == StatusProcessing && record.CompletedAt <= 0
}

func processingRecordStale(record Record, now time.Time) bool {
	timeout := staleProcessingTimeoutForRecord(record)
	if timeout <= 0 || !recordProcessing(record) {
		return false
	}
	anchor := record.UpdatedAt
	if anchor <= 0 {
		anchor = record.CreatedAt
	}
	if anchor <= 0 {
		return false
	}
	return !time.Unix(anchor, 0).Add(timeout).After(now)
}

func staleProcessingTimeoutForRecord(record Record) time.Duration {
	if record.TTFTMs <= 0 {
		return firstByteProcessingTimeout()
	}
	return staleProcessingTimeout()
}

func staleProcessingFinalRecord(record Record, now time.Time) Record {
	completedAt := now.Unix()
	if record.CreatedAt <= 0 {
		record.CreatedAt = completedAt
	}
	record.UpdatedAt = completedAt
	record.CompletedAt = completedAt
	record.FinalSuccess = false
	record.FinalStatusCode = http.StatusGatewayTimeout
	record.FinalErrorCategory = model.ModelGatewayUserRequestErrorTimeout
	record.Status = StatusFailed
	if record.IsHealthProbe {
		record.Status = StatusProbeFailed
	}
	if record.Attempts <= 0 {
		record.Attempts = 1
	}
	if record.DurationMs <= 0 && completedAt >= record.CreatedAt {
		record.DurationMs = (completedAt - record.CreatedAt) * int64(time.Second/time.Millisecond)
	}
	return record
}

func persistStaleProcessingFinal(record Record) Record {
	requestID := strings.TrimSpace(record.RequestID)
	if requestID == "" {
		return record
	}
	completedAt := record.CompletedAt
	if completedAt <= 0 {
		completedAt = time.Now().Unix()
		record.CompletedAt = completedAt
	}
	attemptIndex := record.Attempts - 1
	if attemptIndex < 0 {
		attemptIndex = 0
	}
	duration := time.Duration(record.DurationMs) * time.Millisecond
	summary := model.RecordModelGatewayUserRequestAttempt(model.ModelGatewayUserRequestAttempt{
		CreatedAt:      completedAt,
		RequestId:      requestID,
		AttemptIndex:   attemptIndex,
		RequestedGroup: record.RequestedGroup,
		SelectedGroup:  record.SelectedGroup,
		ChannelID:      record.FinalChannelID,
		ChannelName:    record.FinalChannelName,
		RequestedModel: record.RequestedModel,
		Success:        false,
		StatusCode:     http.StatusGatewayTimeout,
		ErrorCategory:  model.ModelGatewayUserRequestErrorTimeout,
		DurationMs:     duration.Milliseconds(),
		TTFTMs:         record.TTFTMs,
		RetryAction:    "stale_timeout",
		IsHealthProbe:  record.IsHealthProbe,
		ProbeReason:    record.ProbeReason,
	})
	if summary == nil {
		return record
	}
	record.ID = summary.Id
	record.FinalStatusCode = summary.FinalStatusCode
	record.FinalErrorCategory = summary.FinalErrorCategory
	if summary.DurationMs > 0 {
		record.DurationMs = summary.DurationMs
	}
	if summary.TTFTMs > 0 {
		record.TTFTMs = summary.TTFTMs
	}
	return record
}

func staleProcessingTimeout() time.Duration {
	timeout := currentStreamingIdleTimeout()
	if totalTimeout := currentRelayTotalTimeout(); totalTimeout > timeout {
		timeout = totalTimeout
	}
	if timeout <= 0 {
		timeout = defaultStreamingIdleTimeout
	}
	timeout += defaultStaleProcessingGrace
	if timeout < minStaleProcessingTimeout {
		return minStaleProcessingTimeout
	}
	return timeout
}

func firstByteProcessingTimeout() time.Duration {
	timeout := defaultFirstByteTimeout
	if totalTimeout := currentRelayTotalTimeout(); totalTimeout > 0 && totalTimeout < timeout {
		timeout = totalTimeout
	}
	if streamingTimeout := currentStreamingIdleTimeout(); streamingTimeout > 0 && streamingTimeout < timeout {
		timeout = streamingTimeout
	}
	timeout += defaultStaleProcessingGrace
	if timeout < minStaleProcessingTimeout {
		return minStaleProcessingTimeout
	}
	return timeout
}

func currentStreamingIdleTimeout() time.Duration {
	if constant.StreamingTimeout > 0 {
		return time.Duration(constant.StreamingTimeout) * time.Second
	}
	if common.RelayTimeout > 0 {
		return time.Duration(common.RelayTimeout) * time.Second
	}
	return defaultStreamingIdleTimeout
}

func currentRelayTotalTimeout() time.Duration {
	setting := scheduler_setting.GetSetting()
	if !setting.RelayTotalTimeoutEnabled {
		return 0
	}
	seconds := setting.RelayTotalTimeoutSeconds
	if seconds <= 0 {
		seconds = scheduler_setting.DefaultSetting().RelayTotalTimeoutSeconds
	}
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func applyFirstByteObservation(record *Record, observation FirstByteObservation) {
	if record == nil {
		return
	}
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	ttftMs := observation.TTFT.Milliseconds()
	if ttftMs <= 0 {
		return
	}
	record.UpdatedAt = observedAt.Unix()
	record.TTFTMs = ttftMs
}

func applyUpstreamCompletedObservation(record *Record, observation UpstreamCompletedObservation) {
	if record == nil {
		return
	}
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	completedAt := observedAt.Unix()
	if completedAt <= 0 {
		return
	}
	record.UpdatedAt = completedAt
	record.CompletedAt = completedAt
	if record.CreatedAt <= 0 {
		record.CreatedAt = completedAt
	}
	durationMs := observation.Duration.Milliseconds()
	if durationMs <= 0 && record.CreatedAt > 0 && completedAt >= record.CreatedAt {
		durationMs = (completedAt - record.CreatedAt) * int64(time.Second/time.Millisecond)
	}
	if durationMs > 0 {
		record.DurationMs = durationMs
	}
	record.FinalSuccess = true
	if record.Attempts <= 0 {
		record.Attempts = 1
	}
	if record.IsHealthProbe {
		record.Status = StatusProbe
		return
	}
	record.Status = StatusSettling
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
	if result.Success || userRequestResultClientAborted(result) {
		return true
	}
	return !modelGatewayAttemptWillRetry(result)
}

func modelGatewayAttemptWillRetry(result core.AttemptResult) bool {
	if !result.WillRetry {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(result.RetryAction)) {
	case "switch_channel", "retry", "resource_protection_fallback":
		return true
	default:
		return false
	}
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

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
