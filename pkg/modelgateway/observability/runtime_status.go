package observability

import (
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
)

const (
	defaultRuntimeStatusLimit = 200
	maxRuntimeStatusLimit     = 1000
)

type CircuitSnapshotProvider interface {
	Snapshot(key core.RuntimeKey) core.CircuitSnapshot
	ListSnapshots() []core.CircuitSnapshot
}

type RuntimeStateProvider interface {
	ActiveConcurrency(channelID int) int
	ConcurrencyCooldownStatus(channelID int) *service.ChannelConcurrencyControlStatus
	FailureAvoidanceStatus(channelID int) *service.ChannelFailureAvoidanceStatus
}

type RuntimeStatusDeps struct {
	SnapshotStore core.RuntimeSnapshotStore
	Circuit       CircuitSnapshotProvider
	QueueSnapshot func() map[int]int
	StateProvider RuntimeStateProvider
	Now           func() time.Time
}

type RuntimeStatusService struct {
	deps RuntimeStatusDeps
}

type RuntimeStatusQuery struct {
	Model     string
	Group     string
	ChannelID int
	Limit     int
}

type RuntimeStatusResponse struct {
	Summary RuntimeStatusSummary `json:"summary"`
	Items   []RuntimeStatusItem  `json:"items"`
}

type RuntimeStatusSummary struct {
	UpdatedAt                int64 `json:"updated_at"`
	RuntimeKeys              int   `json:"runtime_keys"`
	Channels                 int   `json:"channels"`
	ActiveConcurrency        int   `json:"active_concurrency"`
	QueuedRequests           int   `json:"queued_requests"`
	QueueChannels            int   `json:"queue_channels"`
	MaxQueueDepth            int   `json:"max_queue_depth"`
	CircuitOpen              int   `json:"circuit_open"`
	CircuitHalfOpen          int   `json:"circuit_half_open"`
	CooldownChannels         int   `json:"cooldown_channels"`
	FailureAvoidanceChannels int   `json:"failure_avoidance_channels"`
	SaturatedChannels        int   `json:"saturated_channels"`
}

type RuntimeStatusItem struct {
	RequestedModel                   string  `json:"requested_model,omitempty"`
	UpstreamModel                    string  `json:"upstream_model,omitempty"`
	ChannelID                        int     `json:"channel_id,omitempty"`
	Group                            string  `json:"group,omitempty"`
	EndpointType                     string  `json:"endpoint_type,omitempty"`
	CapabilityFingerprint            string  `json:"capability_fingerprint,omitempty"`
	SuccessRate                      float64 `json:"success_rate"`
	TTFTMs                           float64 `json:"ttft_ms"`
	DurationMs                       float64 `json:"duration_ms"`
	TokensPerSecond                  float64 `json:"tokens_per_second"`
	ActiveConcurrency                int     `json:"active_concurrency"`
	MaxConcurrency                   int     `json:"max_concurrency"`
	QueueDepth                       int     `json:"queue_depth"`
	QueueCapacity                    int     `json:"queue_capacity"`
	QueueTimeoutMs                   int     `json:"queue_timeout_ms"`
	EstimatedQueueWaitMs             float64 `json:"estimated_queue_wait_ms"`
	CostRatio                        float64 `json:"cost_ratio"`
	GroupPriorityRatio               float64 `json:"group_priority_ratio"`
	CircuitState                     string  `json:"circuit_state,omitempty"`
	CircuitOpen                      bool    `json:"circuit_open"`
	CircuitOpenUntil                 int64   `json:"circuit_open_until,omitempty"`
	CircuitFailureCount              int     `json:"circuit_failure_count,omitempty"`
	CircuitFailureRate               float64 `json:"circuit_failure_rate,omitempty"`
	CircuitSampleCount               int     `json:"circuit_sample_count,omitempty"`
	CircuitHalfOpenProbeUsed         int     `json:"circuit_half_open_probe_used,omitempty"`
	CircuitHalfOpenProbeMax          int     `json:"circuit_half_open_probe_max,omitempty"`
	Cooldown                         bool    `json:"cooldown"`
	CooldownRemainingSeconds         int64   `json:"cooldown_remaining_seconds,omitempty"`
	CooldownReason                   string  `json:"cooldown_reason,omitempty"`
	CooldownFailureCount             int     `json:"cooldown_failure_count,omitempty"`
	FailureAvoidance                 bool    `json:"failure_avoidance"`
	FailureAvoidanceRemainingSeconds int64   `json:"failure_avoidance_remaining_seconds,omitempty"`
	FailureAvoidanceReason           string  `json:"failure_avoidance_reason,omitempty"`
	FailureAvoidanceCount            int     `json:"failure_avoidance_count,omitempty"`
	SampleCount                      int     `json:"sample_count"`
	HealthStatus                     string  `json:"health_status"`
}

type ServiceRuntimeStateProvider struct{}

func NewRuntimeStatusService(deps RuntimeStatusDeps) *RuntimeStatusService {
	if deps.StateProvider == nil {
		deps.StateProvider = ServiceRuntimeStateProvider{}
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &RuntimeStatusService{deps: deps}
}

func (p ServiceRuntimeStateProvider) ActiveConcurrency(channelID int) int {
	return service.GetChannelActiveConcurrency(channelID)
}

func (p ServiceRuntimeStateProvider) ConcurrencyCooldownStatus(channelID int) *service.ChannelConcurrencyControlStatus {
	return service.GetChannelConcurrencyCooldownStatus(channelID)
}

func (p ServiceRuntimeStateProvider) FailureAvoidanceStatus(channelID int) *service.ChannelFailureAvoidanceStatus {
	return service.GetChannelFailureAvoidanceStatus(channelID)
}

func (s *RuntimeStatusService) Build(query RuntimeStatusQuery) RuntimeStatusResponse {
	if s == nil {
		return RuntimeStatusResponse{}
	}
	query.Limit = normalizeRuntimeStatusLimit(query.Limit)
	items := map[core.RuntimeKey]*RuntimeStatusItem{}

	for _, snapshot := range s.snapshots(query.Model) {
		if !runtimeStatusMatchesQuery(snapshot.Key, query) {
			continue
		}
		item := itemForRuntimeKey(items, snapshot.Key)
		applyRuntimeSnapshot(item, snapshot)
	}
	for _, circuit := range s.circuitSnapshots() {
		if !runtimeStatusMatchesQuery(circuit.Key, query) {
			continue
		}
		item := itemForRuntimeKey(items, circuit.Key)
		applyCircuitSnapshot(item, circuit)
	}
	for channelID, depth := range s.queueSnapshot() {
		if channelID <= 0 || depth <= 0 {
			continue
		}
		if query.ChannelID > 0 && query.ChannelID != channelID {
			continue
		}
		matched := false
		for _, item := range items {
			if item.ChannelID != channelID {
				continue
			}
			if depth > item.QueueDepth {
				item.QueueDepth = depth
			}
			matched = true
		}
		if !matched && query.Model == "" && query.Group == "" {
			key := core.RuntimeKey{ChannelID: channelID}
			item := itemForRuntimeKey(items, key)
			item.QueueDepth = depth
		}
	}

	result := make([]RuntimeStatusItem, 0, len(items))
	for _, item := range items {
		s.applyLiveState(item)
		if !runtimeStatusItemMatchesQuery(*item, query) {
			continue
		}
		item.HealthStatus = runtimeHealthStatus(*item)
		result = append(result, *item)
	}
	sortRuntimeStatusItems(result)
	if len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return RuntimeStatusResponse{
		Summary: summarizeRuntimeStatus(result, s.deps.Now()),
		Items:   result,
	}
}

func (s *RuntimeStatusService) snapshots(modelName string) []core.RuntimeSnapshot {
	if s == nil || s.deps.SnapshotStore == nil {
		return nil
	}
	var req *core.DispatchRequest
	if strings.TrimSpace(modelName) != "" {
		req = &core.DispatchRequest{ModelName: strings.TrimSpace(modelName)}
	}
	return s.deps.SnapshotStore.ListCandidates(req)
}

func (s *RuntimeStatusService) circuitSnapshots() []core.CircuitSnapshot {
	if s == nil || s.deps.Circuit == nil {
		return nil
	}
	return s.deps.Circuit.ListSnapshots()
}

func (s *RuntimeStatusService) queueSnapshot() map[int]int {
	if s == nil || s.deps.QueueSnapshot == nil {
		return nil
	}
	return s.deps.QueueSnapshot()
}

func (s *RuntimeStatusService) applyLiveState(item *RuntimeStatusItem) {
	if s == nil || s.deps.StateProvider == nil || item == nil || item.ChannelID <= 0 {
		return
	}
	if active := s.deps.StateProvider.ActiveConcurrency(item.ChannelID); active > item.ActiveConcurrency {
		item.ActiveConcurrency = active
	}
	if cooldown := s.deps.StateProvider.ConcurrencyCooldownStatus(item.ChannelID); cooldown != nil && cooldown.Active {
		item.Cooldown = true
		item.CooldownRemainingSeconds = cooldown.RemainingSec
		item.CooldownReason = cooldown.Reason
		item.CooldownFailureCount = cooldown.FailureCount
	}
	if avoidance := s.deps.StateProvider.FailureAvoidanceStatus(item.ChannelID); avoidance != nil && avoidance.Active {
		item.FailureAvoidance = true
		item.FailureAvoidanceRemainingSeconds = avoidance.RemainingSec
		item.FailureAvoidanceReason = avoidance.Reason
		item.FailureAvoidanceCount = avoidance.FailureCount
	}
}

func itemForRuntimeKey(items map[core.RuntimeKey]*RuntimeStatusItem, key core.RuntimeKey) *RuntimeStatusItem {
	item := items[key]
	if item != nil {
		return item
	}
	item = &RuntimeStatusItem{
		RequestedModel:        key.RequestedModel,
		UpstreamModel:         key.UpstreamModel,
		ChannelID:             key.ChannelID,
		Group:                 key.Group,
		EndpointType:          string(key.EndpointType),
		CapabilityFingerprint: key.CapabilityFingerprint,
		CircuitState:          string(core.CircuitStateClosed),
	}
	items[key] = item
	return item
}

func applyRuntimeSnapshot(item *RuntimeStatusItem, snapshot core.RuntimeSnapshot) {
	if item == nil {
		return
	}
	item.SuccessRate = snapshot.SuccessRate
	item.TTFTMs = snapshot.TTFTMs
	item.DurationMs = snapshot.DurationMs
	item.TokensPerSecond = snapshot.TokensPerSecond
	item.ActiveConcurrency = snapshot.ActiveConcurrency
	item.MaxConcurrency = snapshot.MaxConcurrency
	item.QueueDepth = snapshot.QueueDepth
	item.QueueCapacity = snapshot.QueueCapacity
	item.QueueTimeoutMs = snapshot.QueueTimeoutMs
	item.EstimatedQueueWaitMs = snapshot.EstimatedQueueWaitMs
	item.CostRatio = snapshot.CostRatio
	item.GroupPriorityRatio = snapshot.GroupPriorityRatio
	if snapshot.CircuitState != "" {
		item.CircuitState = string(snapshot.CircuitState)
	}
	item.CircuitOpen = snapshot.CircuitOpen || snapshot.CircuitState == core.CircuitStateOpen
	item.Cooldown = snapshot.Cooldown
	item.FailureAvoidance = snapshot.FailureAvoidance
	item.SampleCount = snapshot.SampleCount
}

func applyCircuitSnapshot(item *RuntimeStatusItem, snapshot core.CircuitSnapshot) {
	if item == nil {
		return
	}
	if snapshot.State == "" {
		snapshot.State = core.CircuitStateClosed
	}
	item.CircuitState = string(snapshot.State)
	item.CircuitOpen = snapshot.State == core.CircuitStateOpen
	if !snapshot.OpenUntil.IsZero() {
		item.CircuitOpenUntil = snapshot.OpenUntil.Unix()
	}
	item.CircuitFailureCount = snapshot.FailureCount
	item.CircuitFailureRate = snapshot.FailureRate
	item.CircuitSampleCount = snapshot.SampleCount
	item.CircuitHalfOpenProbeUsed = snapshot.HalfOpenProbeUsed
	item.CircuitHalfOpenProbeMax = snapshot.HalfOpenProbeMax
}

func runtimeStatusMatchesQuery(key core.RuntimeKey, query RuntimeStatusQuery) bool {
	if query.Model != "" && key.RequestedModel != "" && key.RequestedModel != query.Model {
		return false
	}
	if query.Group != "" && key.Group != "" && key.Group != query.Group {
		return false
	}
	if query.ChannelID > 0 && key.ChannelID > 0 && key.ChannelID != query.ChannelID {
		return false
	}
	return true
}

func runtimeStatusItemMatchesQuery(item RuntimeStatusItem, query RuntimeStatusQuery) bool {
	if query.Model != "" && item.RequestedModel != query.Model {
		return false
	}
	if query.Group != "" && item.Group != query.Group {
		return false
	}
	if query.ChannelID > 0 && item.ChannelID != query.ChannelID {
		return false
	}
	return true
}

func runtimeHealthStatus(item RuntimeStatusItem) string {
	switch {
	case item.CircuitOpen:
		return "circuit_open"
	case item.Cooldown:
		return "cooldown"
	case item.FailureAvoidance:
		return "failure_avoidance"
	case item.MaxConcurrency > 0 && item.ActiveConcurrency >= item.MaxConcurrency && item.QueueDepth > 0:
		return "queued"
	case item.MaxConcurrency > 0 && item.ActiveConcurrency >= item.MaxConcurrency:
		return "saturated"
	case item.SuccessRate > 0 && item.SuccessRate < 0.80:
		return "degraded"
	default:
		return "healthy"
	}
}

func summarizeRuntimeStatus(items []RuntimeStatusItem, now time.Time) RuntimeStatusSummary {
	summary := RuntimeStatusSummary{UpdatedAt: now.Unix(), RuntimeKeys: len(items)}
	channelIDs := map[int]struct{}{}
	queueChannels := map[int]struct{}{}
	cooldownChannels := map[int]struct{}{}
	avoidanceChannels := map[int]struct{}{}
	for _, item := range items {
		if item.ChannelID > 0 {
			channelIDs[item.ChannelID] = struct{}{}
		}
		summary.ActiveConcurrency += item.ActiveConcurrency
		summary.QueuedRequests += item.QueueDepth
		if item.QueueDepth > 0 {
			queueChannels[item.ChannelID] = struct{}{}
			if item.QueueDepth > summary.MaxQueueDepth {
				summary.MaxQueueDepth = item.QueueDepth
			}
		}
		switch core.CircuitState(item.CircuitState) {
		case core.CircuitStateOpen:
			summary.CircuitOpen++
		case core.CircuitStateHalfOpen:
			summary.CircuitHalfOpen++
		}
		if item.Cooldown {
			cooldownChannels[item.ChannelID] = struct{}{}
		}
		if item.FailureAvoidance {
			avoidanceChannels[item.ChannelID] = struct{}{}
		}
		if item.MaxConcurrency > 0 && item.ActiveConcurrency >= item.MaxConcurrency {
			summary.SaturatedChannels++
		}
	}
	summary.Channels = len(channelIDs)
	summary.QueueChannels = len(queueChannels)
	summary.CooldownChannels = len(cooldownChannels)
	summary.FailureAvoidanceChannels = len(avoidanceChannels)
	return summary
}

func sortRuntimeStatusItems(items []RuntimeStatusItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left := runtimeStatusSeverity(items[i])
		right := runtimeStatusSeverity(items[j])
		if left != right {
			return left > right
		}
		if items[i].QueueDepth != items[j].QueueDepth {
			return items[i].QueueDepth > items[j].QueueDepth
		}
		if items[i].ActiveConcurrency != items[j].ActiveConcurrency {
			return items[i].ActiveConcurrency > items[j].ActiveConcurrency
		}
		if items[i].ChannelID != items[j].ChannelID {
			return items[i].ChannelID < items[j].ChannelID
		}
		if items[i].RequestedModel != items[j].RequestedModel {
			return items[i].RequestedModel < items[j].RequestedModel
		}
		if items[i].Group != items[j].Group {
			return items[i].Group < items[j].Group
		}
		return items[i].EndpointType < items[j].EndpointType
	})
}

func runtimeStatusSeverity(item RuntimeStatusItem) int {
	switch {
	case item.CircuitOpen:
		return 60
	case item.Cooldown:
		return 50
	case item.FailureAvoidance:
		return 45
	case item.MaxConcurrency > 0 && item.ActiveConcurrency >= item.MaxConcurrency && item.QueueDepth > 0:
		return 40
	case item.MaxConcurrency > 0 && item.ActiveConcurrency >= item.MaxConcurrency:
		return 35
	case item.QueueDepth > 0:
		return 30
	case item.SuccessRate > 0 && item.SuccessRate < 0.80:
		return 20
	default:
		return 10
	}
}

func normalizeRuntimeStatusLimit(limit int) int {
	if limit <= 0 {
		return defaultRuntimeStatusLimit
	}
	if limit > maxRuntimeStatusLimit {
		return maxRuntimeStatusLimit
	}
	return limit
}
