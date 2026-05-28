package realtime

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability/userrequest"
	bus "github.com/QuantumNous/new-api/pkg/realtime"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"golang.org/x/sync/singleflight"
)

const (
	TopicName             = "model_gateway.observability"
	defaultFlushInterval  = time.Second
	defaultRecentLimit    = 50
	defaultTopN           = 10
	defaultWindowHours    = 24
	defaultSubscriptionID = "model-gateway-observability"
)

type Topic struct {
	mu            sync.Mutex
	subscriptions map[string]*subscriptionState
	groups        map[string]map[string]*subscriptionState
	pending       map[string]bool
	cache         map[string]cacheEntry
	group         singleflight.Group
	flushInterval time.Duration
	eventCh       chan model.ModelExecutionRecord
	closeCh       chan struct{}
	closeOnce     sync.Once
}

type subscriptionState struct {
	id         string
	groupKey   string
	subscriber bus.Subscriber
	params     params
}

type params struct {
	Hours              int
	RecentLimit        int
	TopN               int
	TrendBucketSeconds int64
	ViewMode           string
	Model              string
	Group              string
	ChannelID          int
	RequestID          string
}

type cacheEntry struct {
	response  controller.ModelGatewayObservabilityResponse
	expiresAt time.Time
}

type Delta struct {
	RecentRecords      []controller.ModelGatewayObservabilityRecord `json:"recent_records,omitempty"`
	UserRequestsRecent []controller.ModelGatewayUserRequestRecord   `json:"user_requests_recent,omitempty"`
}

func NewTopic() *Topic {
	topic := &Topic{
		subscriptions: map[string]*subscriptionState{},
		groups:        map[string]map[string]*subscriptionState{},
		pending:       map[string]bool{},
		cache:         map[string]cacheEntry{},
		flushInterval: defaultFlushInterval,
		eventCh:       make(chan model.ModelExecutionRecord, 1024),
		closeCh:       make(chan struct{}),
	}
	go topic.run()
	return topic
}

func (t *Topic) Name() string {
	return TopicName
}

func (t *Topic) Subscribe(subscriber bus.Subscriber, subscription bus.Subscription) {
	if t == nil || subscriber == nil {
		return
	}
	params := parseParams(subscription.Params)
	id := strings.TrimSpace(subscription.ID)
	if id == "" {
		id = defaultSubscriptionID
	}
	state := &subscriptionState{
		id:         id,
		groupKey:   params.key(),
		subscriber: subscriber,
		params:     params,
	}
	t.mu.Lock()
	t.removeLocked(subscriber, id)
	t.subscriptions[subscriptionKey(subscriber, id)] = state
	if t.groups[state.groupKey] == nil {
		t.groups[state.groupKey] = map[string]*subscriptionState{}
	}
	t.groups[state.groupKey][subscriptionKey(subscriber, id)] = state
	t.mu.Unlock()

	go t.sendSnapshot(state)
}

func (t *Topic) Unsubscribe(subscriber bus.Subscriber, subscriptionID string) {
	if t == nil || subscriber == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.removeLocked(subscriber, strings.TrimSpace(subscriptionID))
}

func (t *Topic) Publish(record model.ModelExecutionRecord) {
	if t == nil {
		return
	}
	controller.InvalidateModelGatewayObservabilitySummaryCacheForRecord(record)
	select {
	case t.eventCh <- record:
	default:
		t.markAllPending()
	}
}

func (t *Topic) PublishUserRequest(event userrequest.Event) {
	if t == nil {
		return
	}
	record := userRequestRecordFromRealtimeRecord(event.Record)
	if event.Kind == userrequest.EventFinished {
		controller.InvalidateModelGatewayObservabilitySummaryCacheForUserRequest(record)
	}
	t.mu.Lock()
	deliveries := make(map[string][]*subscriptionState)
	for groupKey, groupSubscriptions := range t.groups {
		if len(groupSubscriptions) == 0 {
			continue
		}
		var sample *subscriptionState
		for _, subscription := range groupSubscriptions {
			sample = subscription
			break
		}
		if sample == nil || sample.params.ViewMode != "user_requests" || !sample.params.matchesUserRequest(record) {
			continue
		}
		if event.Kind == userrequest.EventFinished {
			t.pending[groupKey] = true
			delete(t.cache, groupKey)
		}
		for _, subscription := range groupSubscriptions {
			deliveries[groupKey] = append(deliveries[groupKey], subscription)
		}
	}
	t.mu.Unlock()
	if len(deliveries) == 0 {
		return
	}
	if event.Kind == userrequest.EventFinished {
		records := []controller.ModelGatewayUserRequestRecord{record}
		controller.AttachModelGatewayUserRequestDispatchRecords(records)
		record = records[0]
	}
	for _, subscriptions := range deliveries {
		for _, subscription := range subscriptions {
			subscription.subscriber.Send(bus.ServerMessage{
				Type:  bus.MessageTypeDelta,
				ID:    subscription.id,
				Topic: TopicName,
				Data:  Delta{UserRequestsRecent: []controller.ModelGatewayUserRequestRecord{record}},
			})
		}
	}
}

func (t *Topic) Close() {
	if t == nil {
		return
	}
	t.closeOnce.Do(func() {
		close(t.closeCh)
	})
}

func (t *Topic) run() {
	ticker := time.NewTicker(t.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case record := <-t.eventCh:
			t.handleRecord(record)
		case <-ticker.C:
			t.flushPending()
		case <-t.closeCh:
			return
		}
	}
}

func (t *Topic) handleRecord(record model.ModelExecutionRecord) {
	t.mu.Lock()
	deliveries := make(map[string][]*subscriptionState)
	for groupKey, groupSubscriptions := range t.groups {
		if len(groupSubscriptions) == 0 {
			continue
		}
		var sample *subscriptionState
		for _, subscription := range groupSubscriptions {
			sample = subscription
			break
		}
		if sample == nil || !sample.params.matches(record) {
			continue
		}
		t.pending[groupKey] = true
		delete(t.cache, groupKey)
		if sample.params.ViewMode == "user_requests" {
			continue
		}
		for _, subscription := range groupSubscriptions {
			deliveries[groupKey] = append(deliveries[groupKey], subscription)
		}
	}
	t.mu.Unlock()
	if len(deliveries) == 0 {
		return
	}
	recentRecord := controller.ModelGatewayObservabilityRecordFromModelRecord(record)
	for _, subscriptions := range deliveries {
		for _, subscription := range subscriptions {
			subscription.subscriber.Send(bus.ServerMessage{
				Type:  bus.MessageTypeDelta,
				ID:    subscription.id,
				Topic: TopicName,
				Data:  Delta{RecentRecords: []controller.ModelGatewayObservabilityRecord{recentRecord}},
			})
		}
	}
}

func (t *Topic) markAllPending() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for groupKey := range t.groups {
		t.pending[groupKey] = true
	}
}

func (t *Topic) flushPending() {
	t.mu.Lock()
	groupKeys := make([]string, 0, len(t.pending))
	for groupKey, pending := range t.pending {
		if pending {
			groupKeys = append(groupKeys, groupKey)
			t.pending[groupKey] = false
		}
	}
	t.mu.Unlock()
	for _, groupKey := range groupKeys {
		t.flushGroup(groupKey)
	}
}

func (t *Topic) flushGroup(groupKey string) {
	state := t.sampleState(groupKey)
	if state == nil {
		return
	}
	response, err := t.snapshot(state.params)
	if err != nil {
		t.broadcast(groupKey, bus.ServerMessage{Type: bus.MessageTypeError, Topic: TopicName, Message: err.Error()})
		return
	}
	t.broadcast(groupKey, bus.ServerMessage{Type: bus.MessageTypeSnapshot, Topic: TopicName, Data: response})
}

func (t *Topic) sendSnapshot(state *subscriptionState) {
	if state == nil {
		return
	}
	response, err := t.snapshot(state.params)
	if err != nil {
		state.subscriber.Send(bus.ServerMessage{Type: bus.MessageTypeError, ID: state.id, Topic: TopicName, Message: err.Error()})
		return
	}
	state.subscriber.Send(bus.ServerMessage{Type: bus.MessageTypeSnapshot, ID: state.id, Topic: TopicName, Data: response})
}

func (t *Topic) snapshot(params params) (controller.ModelGatewayObservabilityResponse, error) {
	groupKey := params.key()
	now := time.Now()
	t.mu.Lock()
	if entry, ok := t.cache[groupKey]; ok && now.Before(entry.expiresAt) {
		t.mu.Unlock()
		return mergeUserRequestPendingSnapshot(entry.response, params), nil
	}
	t.mu.Unlock()

	value, err, _ := t.group.Do(groupKey, func() (any, error) {
		response, err := controller.BuildModelGatewayObservabilitySummary(params.toControllerOptions())
		if err != nil {
			return controller.ModelGatewayObservabilityResponse{}, err
		}
		t.mu.Lock()
		t.cache[groupKey] = cacheEntry{response: response, expiresAt: time.Now().Add(t.flushInterval)}
		t.mu.Unlock()
		return response, nil
	})
	if err != nil {
		return controller.ModelGatewayObservabilityResponse{}, err
	}
	response, _ := value.(controller.ModelGatewayObservabilityResponse)
	return mergeUserRequestPendingSnapshot(response, params), nil
}

func (t *Topic) broadcast(groupKey string, message bus.ServerMessage) {
	t.mu.Lock()
	subscriptions := make([]*subscriptionState, 0, len(t.groups[groupKey]))
	for _, subscription := range t.groups[groupKey] {
		subscriptions = append(subscriptions, subscription)
	}
	t.mu.Unlock()
	for _, subscription := range subscriptions {
		message.ID = subscription.id
		subscription.subscriber.Send(message)
	}
}

func (t *Topic) sampleState(groupKey string) *subscriptionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, subscription := range t.groups[groupKey] {
		return subscription
	}
	return nil
}

func (t *Topic) removeLocked(subscriber bus.Subscriber, subscriptionID string) {
	key := subscriptionKey(subscriber, subscriptionID)
	state, ok := t.subscriptions[key]
	if !ok {
		return
	}
	delete(t.subscriptions, key)
	if groupSubscriptions := t.groups[state.groupKey]; groupSubscriptions != nil {
		delete(groupSubscriptions, key)
		if len(groupSubscriptions) == 0 {
			delete(t.groups, state.groupKey)
			delete(t.pending, state.groupKey)
		}
	}
}

func subscriptionKey(subscriber bus.Subscriber, subscriptionID string) string {
	value := reflect.ValueOf(subscriber)
	if value.IsValid() && value.Kind() == reflect.Pointer {
		return strconv.FormatUint(uint64(value.Pointer()), 10) + ":" + subscriptionID
	}
	return subscriptionID
}

func parseParams(raw map[string]any) params {
	result := params{
		Hours:       defaultWindowHours,
		RecentLimit: defaultRecentLimit,
		TopN:        defaultTopN,
	}
	if raw == nil {
		return result
	}
	result.Hours = intParam(raw, "hours", result.Hours)
	result.RecentLimit = intParam(raw, "recent_limit", result.RecentLimit)
	result.TopN = intParam(raw, "top_n", result.TopN)
	result.TrendBucketSeconds = int64Param(raw, "trend_bucket_seconds", 0)
	result.ViewMode = stringParam(raw, "view_mode")
	result.Model = stringParam(raw, "model")
	result.Group = stringParam(raw, "group")
	result.ChannelID = intParam(raw, "channel_id", 0)
	result.RequestID = stringParam(raw, "request_id")
	return result
}

func (p params) toControllerOptions() controller.ModelGatewayObservabilityOptions {
	return controller.ModelGatewayObservabilityOptions{
		Hours:              p.Hours,
		RecentLimit:        p.RecentLimit,
		TopN:               p.TopN,
		TrendBucketSeconds: p.TrendBucketSeconds,
		ViewMode:           p.ViewMode,
		Model:              p.Model,
		Group:              p.Group,
		ChannelID:          p.ChannelID,
		RequestID:          p.RequestID,
	}
}

func (p params) key() string {
	values := []string{
		"hours=" + strconv.Itoa(p.Hours),
		"recent_limit=" + strconv.Itoa(p.RecentLimit),
		"top_n=" + strconv.Itoa(p.TopN),
		"trend_bucket_seconds=" + strconv.FormatInt(p.TrendBucketSeconds, 10),
		"view_mode=" + p.ViewMode,
		"model=" + p.Model,
		"group=" + p.Group,
		"channel_id=" + strconv.Itoa(p.ChannelID),
		"request_id=" + p.RequestID,
	}
	sort.Strings(values)
	return strings.Join(values, "&")
}

func (p params) matches(record model.ModelExecutionRecord) bool {
	if p.Model != "" && p.Model != record.RequestedModel {
		return false
	}
	if p.Group != "" && p.Group != record.RequestedGroup && p.Group != record.SelectedGroup && p.Group != record.ActualGroup {
		return false
	}
	if p.ChannelID > 0 && p.ChannelID != record.ChannelId && p.ChannelID != record.ActualChannelId {
		return false
	}
	if p.RequestID != "" && p.RequestID != record.RequestId {
		return false
	}
	if p.Hours > 0 {
		cutoff := time.Now().Add(-time.Duration(p.Hours) * time.Hour).Unix()
		if record.CreatedAt < cutoff {
			return false
		}
	}
	return true
}

func (p params) matchesUserRequest(record controller.ModelGatewayUserRequestRecord) bool {
	if p.Model != "" && p.Model != record.RequestedModel {
		return false
	}
	if p.Group != "" && p.Group != record.RequestedGroup && p.Group != record.SelectedGroup && p.Group != record.ActualGroup {
		return false
	}
	if p.ChannelID > 0 && p.ChannelID != record.FinalChannelID {
		return false
	}
	if p.RequestID != "" && p.RequestID != record.RequestID {
		return false
	}
	if p.Hours > 0 {
		cutoff := time.Now().Add(-time.Duration(p.Hours) * time.Hour).Unix()
		if record.CreatedAt < cutoff {
			return false
		}
	}
	return true
}

func userRequestRecordFromRealtimeRecord(record userrequest.Record) controller.ModelGatewayUserRequestRecord {
	actualGroup := strings.TrimSpace(record.SelectedGroup)
	if actualGroup == "" {
		actualGroup = strings.TrimSpace(record.RequestedGroup)
	}
	actualGroupRatio := 0.0
	if record.CompletedAt > 0 && actualGroup != "" {
		actualGroupRatio = ratio_setting.GetGroupRatio(actualGroup)
	}
	return controller.ModelGatewayUserRequestRecord{
		ID:                        record.ID,
		CreatedAt:                 record.CreatedAt,
		CompletedAt:               record.CompletedAt,
		RequestID:                 record.RequestID,
		RequestedModel:            record.RequestedModel,
		RequestedGroup:            record.RequestedGroup,
		SelectedGroup:             record.SelectedGroup,
		ActualGroup:               actualGroup,
		ActualGroupRatio:          actualGroupRatio,
		FinalChannelID:            record.FinalChannelID,
		FinalChannelName:          record.FinalChannelName,
		Attempts:                  record.Attempts,
		FinalSuccess:              record.FinalSuccess,
		Recovered:                 record.Recovered,
		FinalStatusCode:           record.FinalStatusCode,
		FinalErrorCategory:        record.FinalErrorCategory,
		WarningLevel:              record.WarningLevel,
		WarningFlags:              append([]string(nil), record.WarningFlags...),
		WarningMessage:            record.WarningMessage,
		ChannelInducedClientAbort: record.ChannelInducedClientAbort,
		EmptyOutput:               record.EmptyOutput,
		ExperienceIssue:           record.ExperienceIssue,
		ClientAborted:             record.ClientAborted,
		IsHealthProbe:             record.IsHealthProbe,
		ProbeReason:               record.ProbeReason,
		DurationMs:                record.DurationMs,
		TTFTMs:                    record.TTFTMs,
		Status:                    record.Status,
	}
}

func mergeUserRequestPendingSnapshot(response controller.ModelGatewayObservabilityResponse, params params) controller.ModelGatewayObservabilityResponse {
	if params.ViewMode != "user_requests" {
		return response
	}
	response.UserRequests.RecentRequests = mergeUserRequestRealtimeRecords(
		response.UserRequests.RecentRequests,
		userrequest.Snapshot(params.RecentLimit, userrequest.Filters{
			Model:     params.Model,
			Group:     params.Group,
			ChannelID: params.ChannelID,
			RequestID: params.RequestID,
			Hours:     params.Hours,
		}),
		params.RecentLimit,
	)
	return response
}

func mergeUserRequestRealtimeRecords(completed []controller.ModelGatewayUserRequestRecord, pending []userrequest.Record, limit int) []controller.ModelGatewayUserRequestRecord {
	merged := make([]controller.ModelGatewayUserRequestRecord, 0, len(completed)+len(pending))
	seen := map[string]bool{}
	for _, item := range pending {
		record := userRequestRecordFromRealtimeRecord(item)
		if strings.TrimSpace(record.RequestID) == "" {
			continue
		}
		seen[record.RequestID] = true
		merged = append(merged, record)
	}
	for _, record := range completed {
		if strings.TrimSpace(record.RequestID) != "" && seen[record.RequestID] {
			continue
		}
		merged = append(merged, record)
	}
	sort.Slice(merged, func(i, j int) bool {
		left := userRequestSortTime(merged[i])
		right := userRequestSortTime(merged[j])
		if left != right {
			return left > right
		}
		return merged[i].RequestID > merged[j].RequestID
	})
	if limit > 0 && len(merged) > limit {
		return merged[:limit]
	}
	return merged
}

func userRequestSortTime(record controller.ModelGatewayUserRequestRecord) int64 {
	if record.CompletedAt > 0 {
		return record.CompletedAt
	}
	return record.CreatedAt
}

func stringParam(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(toString(value))
}

func intParam(raw map[string]any, key string, fallback int) int {
	value := strings.TrimSpace(toString(raw[key]))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func int64Param(raw map[string]any, key string, fallback int64) int64 {
	value := strings.TrimSpace(toString(raw[key]))
	if value == "" || strings.EqualFold(value, "auto") {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case float32:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
