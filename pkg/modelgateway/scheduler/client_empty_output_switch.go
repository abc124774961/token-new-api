package scheduler

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	ClientEmptyOutputSwitchReason = "client_empty_output_switch"

	defaultClientEmptyOutputSwitchMinSamples      = 3
	defaultClientEmptyOutputSwitchDetectionWindow = 30 * time.Second
	defaultClientEmptyOutputSwitchAvoidanceTTL    = 10 * time.Minute
	defaultClientEmptyOutputSwitchMaxEntries      = 100000
)

type ClientEmptyOutputSwitchConfig struct {
	DetectionWindow time.Duration
	AvoidanceTTL    time.Duration
	MinSamples      int
	MaxEntries      int
}

type ClientEmptyOutputSwitchTracker struct {
	mu              sync.Mutex
	detectionWindow time.Duration
	avoidanceTTL    time.Duration
	minSamples      int
	maxEntries      int
	now             func() time.Time
	entries         map[clientEmptyOutputSwitchKey]clientEmptyOutputSwitchEntry
}

type clientEmptyOutputSwitchKey struct {
	SessionKey     string
	ChannelID      int
	RequestedModel string
	Group          string
	EndpointType   constant.EndpointType
}

type clientEmptyOutputSwitchEntry struct {
	Events     []time.Time
	AvoidUntil time.Time
}

type ClientEmptyOutputSwitchScope struct {
	SessionKey     string                `json:"session_key"`
	ChannelID      int                   `json:"channel_id"`
	RequestedModel string                `json:"requested_model,omitempty"`
	Group          string                `json:"group,omitempty"`
	EndpointType   constant.EndpointType `json:"endpoint_type,omitempty"`
}

type ClientEmptyOutputSwitchAvoidance struct {
	Scope            ClientEmptyOutputSwitchScope `json:"scope"`
	Until            int64                        `json:"until,omitempty"`
	RemainingSeconds int64                        `json:"remaining_seconds,omitempty"`
	EmptyOutputCount int                          `json:"empty_output_count,omitempty"`
}

func NewClientEmptyOutputSwitchTracker(config ClientEmptyOutputSwitchConfig) *ClientEmptyOutputSwitchTracker {
	if config.DetectionWindow <= 0 {
		config.DetectionWindow = defaultClientEmptyOutputSwitchDetectionWindow
	}
	if config.AvoidanceTTL <= 0 {
		config.AvoidanceTTL = defaultClientEmptyOutputSwitchAvoidanceTTL
	}
	if config.MinSamples <= 0 {
		config.MinSamples = defaultClientEmptyOutputSwitchMinSamples
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultClientEmptyOutputSwitchMaxEntries
	}
	return &ClientEmptyOutputSwitchTracker{
		detectionWindow: config.DetectionWindow,
		avoidanceTTL:    config.AvoidanceTTL,
		minSamples:      config.MinSamples,
		maxEntries:      config.MaxEntries,
		now:             time.Now,
		entries:         map[clientEmptyOutputSwitchKey]clientEmptyOutputSwitchEntry{},
	}
}

func (t *ClientEmptyOutputSwitchTracker) Record(ctx context.Context, record core.DispatchRecord) {}

func (t *ClientEmptyOutputSwitchTracker) Report(ctx context.Context, result core.AttemptResult) {
	if t == nil || result.IsHealthProbe || result.ClientAborted {
		return
	}
	key, ok := clientEmptyOutputSwitchKeyFromResult(result)
	if !ok {
		return
	}
	if result.EmptyOutput || strings.TrimSpace(result.ExperienceIssue) == "empty_output" {
		t.recordEmptyOutput(key, result.ObservedAt)
		return
	}
	if result.Success && strings.TrimSpace(result.ExperienceIssue) == "" {
		t.clear(key)
	}
}

func (t *ClientEmptyOutputSwitchTracker) AvoidanceReason(req core.DispatchRequest, candidate core.Candidate, snapshot core.RuntimeSnapshot) string {
	if t == nil {
		return ""
	}
	key, ok := clientEmptyOutputSwitchKeyFromCandidate(req, candidate, snapshot)
	if !ok {
		return ""
	}
	if t.shouldAvoid(key) {
		return ClientEmptyOutputSwitchReason
	}
	return ""
}

func (t *ClientEmptyOutputSwitchTracker) AvoidanceInfo(req core.DispatchRequest, candidate core.Candidate, snapshot core.RuntimeSnapshot) (ClientEmptyOutputSwitchAvoidance, bool) {
	if t == nil {
		return ClientEmptyOutputSwitchAvoidance{}, false
	}
	key, ok := clientEmptyOutputSwitchKeyFromCandidate(req, candidate, snapshot)
	if !ok {
		return ClientEmptyOutputSwitchAvoidance{}, false
	}
	return t.avoidanceInfo(key)
}

func (t *ClientEmptyOutputSwitchTracker) Clear(scope ClientEmptyOutputSwitchScope) bool {
	if t == nil {
		return false
	}
	key, ok := clientEmptyOutputSwitchKeyFromScope(scope)
	if !ok {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.entries[key]; !exists {
		return false
	}
	delete(t.entries, key)
	return true
}

func (t *ClientEmptyOutputSwitchTracker) CountForTest(req core.DispatchRequest, candidate core.Candidate, snapshot core.RuntimeSnapshot) int {
	if t == nil {
		return 0
	}
	key, ok := clientEmptyOutputSwitchKeyFromCandidate(req, candidate, snapshot)
	if !ok {
		return 0
	}
	now := t.currentTime()
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[key]
	entry.Events = pruneClientEmptyOutputEvents(entry.Events, now, t.detectionWindow)
	if len(entry.Events) == 0 && !entry.AvoidUntil.After(now) {
		delete(t.entries, key)
		return 0
	}
	t.entries[key] = entry
	return len(entry.Events)
}

func (t *ClientEmptyOutputSwitchTracker) WithNowForTest(now func() time.Time) *ClientEmptyOutputSwitchTracker {
	if t == nil || now == nil {
		return t
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = now
	return t
}

func (t *ClientEmptyOutputSwitchTracker) recordEmptyOutput(key clientEmptyOutputSwitchKey, observedAt time.Time) {
	now := t.currentTime()
	if observedAt.IsZero() {
		observedAt = now
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[key]
	entry.Events = pruneClientEmptyOutputEvents(entry.Events, now, t.detectionWindow)
	entry.Events = append(entry.Events, observedAt)
	entry.Events = pruneClientEmptyOutputEvents(entry.Events, now, t.detectionWindow)
	if len(entry.Events) >= t.minSamples {
		entry.AvoidUntil = now.Add(t.avoidanceTTL)
	}
	t.entries[key] = entry
	t.pruneOverflowLocked(now)
}

func (t *ClientEmptyOutputSwitchTracker) clear(key clientEmptyOutputSwitchKey) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

func (t *ClientEmptyOutputSwitchTracker) shouldAvoid(key clientEmptyOutputSwitchKey) bool {
	_, ok := t.avoidanceInfo(key)
	return ok
}

func (t *ClientEmptyOutputSwitchTracker) avoidanceInfo(key clientEmptyOutputSwitchKey) (ClientEmptyOutputSwitchAvoidance, bool) {
	now := t.currentTime()
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[key]
	entry.Events = pruneClientEmptyOutputEvents(entry.Events, now, t.detectionWindow)
	if !entry.AvoidUntil.After(now) {
		if len(entry.Events) == 0 {
			delete(t.entries, key)
		} else {
			entry.AvoidUntil = time.Time{}
			t.entries[key] = entry
		}
		return ClientEmptyOutputSwitchAvoidance{}, false
	}
	t.entries[key] = entry
	remaining := int64(entry.AvoidUntil.Sub(now).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	return ClientEmptyOutputSwitchAvoidance{
		Scope:            clientEmptyOutputSwitchScopeFromKey(key),
		Until:            entry.AvoidUntil.Unix(),
		RemainingSeconds: remaining,
		EmptyOutputCount: len(entry.Events),
	}, true
}

func (t *ClientEmptyOutputSwitchTracker) currentTime() time.Time {
	t.mu.Lock()
	now := t.now
	t.mu.Unlock()
	if now == nil {
		return time.Now()
	}
	return now()
}

func (t *ClientEmptyOutputSwitchTracker) pruneOverflowLocked(now time.Time) {
	if t == nil || t.maxEntries <= 0 || len(t.entries) <= t.maxEntries {
		return
	}
	for key, events := range t.entries {
		entry := events
		entry.Events = pruneClientEmptyOutputEvents(entry.Events, now, t.detectionWindow)
		if len(entry.Events) == 0 && !entry.AvoidUntil.After(now) {
			delete(t.entries, key)
			continue
		}
		t.entries[key] = entry
		if len(t.entries) <= t.maxEntries {
			return
		}
	}
	for key := range t.entries {
		delete(t.entries, key)
		if len(t.entries) <= t.maxEntries {
			return
		}
	}
}

func pruneClientEmptyOutputEvents(events []time.Time, now time.Time, window time.Duration) []time.Time {
	if len(events) == 0 {
		return nil
	}
	cutoff := now.Add(-window)
	pruned := make([]time.Time, 0, len(events))
	for _, event := range events {
		if event.Before(cutoff) {
			continue
		}
		pruned = append(pruned, event)
	}
	if len(pruned) == 0 {
		return nil
	}
	return pruned
}

func clientEmptyOutputSwitchKeyFromResult(result core.AttemptResult) (clientEmptyOutputSwitchKey, bool) {
	runtimeKey := result.RuntimeKey()
	modelName := firstNonEmptyString(runtimeKey.RequestedModel, result.ModelName, runtimeKey.UpstreamModel)
	group := firstNonEmptyString(runtimeKey.Group, result.SelectedGroup, result.RequestedGroup)
	return newClientEmptyOutputSwitchKey(result.ClientSessionKey, runtimeKey.ChannelID, modelName, group, runtimeKey.EndpointType)
}

func clientEmptyOutputSwitchKeyFromCandidate(req core.DispatchRequest, candidate core.Candidate, snapshot core.RuntimeSnapshot) (clientEmptyOutputSwitchKey, bool) {
	channelID := 0
	if candidate.Channel != nil {
		channelID = candidate.Channel.Id
	}
	if channelID <= 0 {
		channelID = firstPositiveInt(snapshot.Key.ChannelID, candidate.RuntimeKey.ChannelID)
	}
	modelName := firstNonEmptyString(req.ModelName, snapshot.Key.RequestedModel, candidate.RuntimeKey.RequestedModel, candidate.UpstreamModel, snapshot.Key.UpstreamModel, candidate.RuntimeKey.UpstreamModel)
	group := firstNonEmptyString(candidate.RuntimeKey.Group, snapshot.Key.Group, candidate.Group, req.RequestedGroup, req.UserGroup)
	endpointType := req.EndpointType
	if endpointType == "" {
		endpointType = firstNonEmptyEndpoint(snapshot.Key.EndpointType, candidate.RuntimeKey.EndpointType)
	}
	return newClientEmptyOutputSwitchKey(req.ClientSessionKey, channelID, modelName, group, endpointType)
}

func clientEmptyOutputSwitchKeyFromScope(scope ClientEmptyOutputSwitchScope) (clientEmptyOutputSwitchKey, bool) {
	return newClientEmptyOutputSwitchKey(scope.SessionKey, scope.ChannelID, scope.RequestedModel, scope.Group, scope.EndpointType)
}

func newClientEmptyOutputSwitchKey(sessionKey string, channelID int, modelName string, group string, endpointType constant.EndpointType) (clientEmptyOutputSwitchKey, bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	modelName = strings.TrimSpace(modelName)
	group = strings.TrimSpace(group)
	if endpointType == "" {
		endpointType = constant.EndpointTypeOpenAI
	}
	if sessionKey == "" || channelID <= 0 || modelName == "" || group == "" {
		return clientEmptyOutputSwitchKey{}, false
	}
	return clientEmptyOutputSwitchKey{
		SessionKey:     sessionKey,
		ChannelID:      channelID,
		RequestedModel: modelName,
		Group:          group,
		EndpointType:   endpointType,
	}, true
}

func clientEmptyOutputSwitchScopeFromKey(key clientEmptyOutputSwitchKey) ClientEmptyOutputSwitchScope {
	return ClientEmptyOutputSwitchScope{
		SessionKey:     key.SessionKey,
		ChannelID:      key.ChannelID,
		RequestedModel: key.RequestedModel,
		Group:          key.Group,
		EndpointType:   key.EndpointType,
	}
}

func firstNonEmptyEndpoint(values ...constant.EndpointType) constant.EndpointType {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

var _ core.ExecutionRecorder = (*ClientEmptyOutputSwitchTracker)(nil)
