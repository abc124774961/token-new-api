package observability

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
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
	SnapshotStore        core.RuntimeSnapshotStore
	Circuit              CircuitSnapshotProvider
	QueueSnapshot        func() map[int]int
	QueueDetailSnapshot  func() core.RuntimeQueueSnapshot
	StateProvider        RuntimeStateProvider
	ScoreWeights         core.ScoreWeights
	CostBaselineProvider core.CostBaselineProvider
	PolicyForGroup       func(group string) core.GroupSmartPolicy
	ChannelProvider      func(channelID int) (*model.Channel, bool)
	Now                  func() time.Time
}

type RuntimeStatusService struct {
	deps RuntimeStatusDeps
}

type RuntimeStatusQuery struct {
	Model                string
	Group                string
	ChannelID            int
	Limit                int
	AccountIDs           []string
	CredentialIndexes    []int
	CredentialSubjectFPs []string
	CredentialFPs        []string
	StrictAccountScope   bool
}

type RuntimeStatusResponse struct {
	Summary       RuntimeStatusSummary       `json:"summary"`
	QueueSnapshot *core.RuntimeQueueSnapshot `json:"queue_snapshot,omitempty"`
	Items         []RuntimeStatusItem        `json:"items"`
}

type RuntimeStatusSummary struct {
	UpdatedAt                    int64 `json:"updated_at"`
	RuntimeKeys                  int   `json:"runtime_keys"`
	Channels                     int   `json:"channels"`
	ActiveConcurrency            int   `json:"active_concurrency"`
	QueuedRequests               int   `json:"queued_requests"`
	QueueChannels                int   `json:"queue_channels"`
	MaxQueueDepth                int   `json:"max_queue_depth"`
	CircuitOpen                  int   `json:"circuit_open"`
	CircuitHalfOpen              int   `json:"circuit_half_open"`
	CooldownChannels             int   `json:"cooldown_channels"`
	FailureAvoidanceChannels     int   `json:"failure_avoidance_channels"`
	LowScoreRecoveryChannels     int   `json:"low_score_recovery_channels"`
	ProbeRecoveryPendingChannels int   `json:"probe_recovery_pending_channels"`
	RecentlyRecoveredChannels    int   `json:"recently_recovered_channels"`
	HighPressureChannels         int   `json:"high_pressure_channels"`
	SaturatedChannels            int   `json:"saturated_channels"`
}

type RuntimeStatusItem struct {
	RequestedModel                   string                      `json:"requested_model,omitempty"`
	UpstreamModel                    string                      `json:"upstream_model,omitempty"`
	ChannelID                        int                         `json:"channel_id,omitempty"`
	ChannelName                      string                      `json:"channel_name,omitempty"`
	ResourceID                       string                      `json:"resource_id,omitempty"`
	ResourceType                     string                      `json:"resource_type,omitempty"`
	AccountID                        string                      `json:"account_id,omitempty"`
	AccountType                      string                      `json:"account_type,omitempty"`
	Brand                            string                      `json:"brand,omitempty"`
	Provider                         string                      `json:"provider,omitempty"`
	CredentialIndex                  int                         `json:"credential_index,omitempty"`
	CredentialSubjectFP              string                      `json:"credential_subject_fingerprint,omitempty"`
	CredentialFP                     string                      `json:"credential_fingerprint,omitempty"`
	Group                            string                      `json:"group,omitempty"`
	EndpointType                     string                      `json:"endpoint_type,omitempty"`
	CapabilityFingerprint            string                      `json:"capability_fingerprint,omitempty"`
	SuccessRate                      float64                     `json:"success_rate"`
	TTFTMs                           float64                     `json:"ttft_ms"`
	DurationMs                       float64                     `json:"duration_ms"`
	TokensPerSecond                  float64                     `json:"tokens_per_second"`
	ActiveConcurrency                int                         `json:"active_concurrency"`
	MaxConcurrency                   int                         `json:"max_concurrency"`
	ConfiguredConcurrencyLimit       int                         `json:"configured_concurrency_limit,omitempty"`
	LearnedConcurrencyLimit          int                         `json:"learned_concurrency_limit,omitempty"`
	EffectiveConcurrencyLimit        int                         `json:"effective_concurrency_limit,omitempty"`
	QueueDepth                       int                         `json:"queue_depth"`
	QueueCapacity                    int                         `json:"queue_capacity"`
	QueueTimeoutMs                   int                         `json:"queue_timeout_ms"`
	EstimatedQueueWaitMs             float64                     `json:"estimated_queue_wait_ms"`
	FirstBytePending                 int                         `json:"first_byte_pending,omitempty"`
	SlowFirstBytePending             int                         `json:"slow_first_byte_pending,omitempty"`
	OldestFirstByteWaitMs            float64                     `json:"oldest_first_byte_wait_ms,omitempty"`
	CostRatio                        float64                     `json:"cost_ratio"`
	CostReferenceRatio               float64                     `json:"cost_reference_ratio,omitempty"`
	CostPricingMode                  string                      `json:"cost_pricing_mode,omitempty"`
	GroupPriorityRatio               float64                     `json:"group_priority_ratio"`
	CircuitState                     string                      `json:"circuit_state,omitempty"`
	CircuitOpen                      bool                        `json:"circuit_open"`
	CircuitOpenUntil                 int64                       `json:"circuit_open_until,omitempty"`
	CircuitOpenReason                string                      `json:"circuit_open_reason,omitempty"`
	CircuitFailureCount              int                         `json:"circuit_failure_count,omitempty"`
	CircuitFailureRate               float64                     `json:"circuit_failure_rate,omitempty"`
	CircuitSampleCount               int                         `json:"circuit_sample_count,omitempty"`
	CircuitErrorCounts               map[string]int              `json:"circuit_error_counts,omitempty"`
	CircuitHalfOpenProbeUsed         int                         `json:"circuit_half_open_probe_used,omitempty"`
	CircuitHalfOpenProbeMax          int                         `json:"circuit_half_open_probe_max,omitempty"`
	Cooldown                         bool                        `json:"cooldown"`
	CooldownRemainingSeconds         int64                       `json:"cooldown_remaining_seconds,omitempty"`
	CooldownReason                   string                      `json:"cooldown_reason,omitempty"`
	CooldownFailureCount             int                         `json:"cooldown_failure_count,omitempty"`
	FailureAvoidance                 bool                        `json:"failure_avoidance"`
	FailureAvoidanceRemainingSeconds int64                       `json:"failure_avoidance_remaining_seconds,omitempty"`
	FailureAvoidanceReason           string                      `json:"failure_avoidance_reason,omitempty"`
	FailureAvoidanceCount            int                         `json:"failure_avoidance_count,omitempty"`
	ScoreTotal                       float64                     `json:"score_total,omitempty"`
	ScoreBreakdown                   map[string]float64          `json:"score_breakdown,omitempty"`
	ScoreItems                       []core.ScoreItem            `json:"score_items,omitempty"`
	RoutingScoreTotal                float64                     `json:"routing_score_total,omitempty"`
	RoutingScoreBreakdown            map[string]float64          `json:"routing_score_breakdown,omitempty"`
	RoutingScoreItems                []core.ScoreItem            `json:"routing_score_items,omitempty"`
	StateTags                        []string                    `json:"state_tags,omitempty"`
	CostReferenceMissing             bool                        `json:"cost_reference_missing,omitempty"`
	EmptyOutputRate                  float64                     `json:"empty_output_rate,omitempty"`
	ExperienceIssueRate              float64                     `json:"experience_issue_rate,omitempty"`
	ProbeRecoveryPending             bool                        `json:"probe_recovery_pending,omitempty"`
	ProbeRecoverySuccessCount        int                         `json:"probe_recovery_success_count,omitempty"`
	ProbeRecoveryRequired            int                         `json:"probe_recovery_required,omitempty"`
	ProbeTriggerReason               string                      `json:"probe_trigger_reason,omitempty"`
	ConfigErrorIsolated              bool                        `json:"config_error_isolated,omitempty"`
	IsolationReason                  string                      `json:"isolation_reason,omitempty"`
	IsolationUntil                   int64                       `json:"isolation_until,omitempty"`
	AuthConfigErrorCount             int                         `json:"auth_config_error_count,omitempty"`
	LastAuthConfigErrorAt            int64                       `json:"last_auth_config_error_at,omitempty"`
	SampleCount                      int                         `json:"sample_count"`
	LastRealAttemptAt                int64                       `json:"last_real_attempt_at,omitempty"`
	LastRealSuccessAt                int64                       `json:"last_real_success_at,omitempty"`
	LastRealFailureAt                int64                       `json:"last_real_failure_at,omitempty"`
	RealSampleCount30m               int                         `json:"real_sample_count_30m,omitempty"`
	LastProbeAt                      int64                       `json:"last_probe_at,omitempty"`
	LastProbeSuccessAt               int64                       `json:"last_probe_success_at,omitempty"`
	RecentLatencySamples             []core.RuntimeLatencySample `json:"-"`
	HealthStatus                     string                      `json:"health_status"`
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
	accountScoped := runtimeStatusStrictAccountScope(query)
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
	if accountScoped {
		for _, item := range items {
			clearAccountScopedChannelDynamicFields(item)
		}
	}
	if !accountScoped {
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
	}
	detailedQueue := s.queueDetailSnapshot()
	for _, queueKey := range detailedQueue.RuntimeKeys {
		if queueKey.QueueDepth <= 0 {
			continue
		}
		key := runtimeQueueSnapshotKey(queueKey)
		if !runtimeStatusMatchesQuery(key, query) {
			continue
		}
		matched := false
		for _, item := range items {
			if !runtimeStatusItemMatchesQueueKey(*item, key) {
				continue
			}
			applyRuntimeQueueKeySnapshot(item, queueKey)
			matched = true
		}
		if !matched && (query.Model == "" || key.RequestedModel == query.Model) && (query.Group == "" || key.Group == query.Group) {
			item := itemForRuntimeKey(items, key)
			applyRuntimeQueueKeySnapshot(item, queueKey)
		}
	}
	if !accountScoped {
		for _, channel := range detailedQueue.Channels {
			if channel.ChannelID <= 0 || channel.QueueDepth <= 0 {
				continue
			}
			if query.ChannelID > 0 && query.ChannelID != channel.ChannelID {
				continue
			}
			matched := false
			for _, item := range items {
				if item.ChannelID != channel.ChannelID {
					continue
				}
				applyRuntimeQueueChannelSnapshot(item, channel)
				matched = true
			}
			if !matched && query.Model == "" && query.Group == "" {
				item := itemForRuntimeKey(items, core.RuntimeKey{ChannelID: channel.ChannelID})
				applyRuntimeQueueChannelSnapshot(item, channel)
			}
		}
	}

	result := make([]RuntimeStatusItem, 0, len(items))
	for _, item := range items {
		s.applyLiveState(item, query)
		if !runtimeStatusItemMatchesQuery(*item, query) {
			continue
		}
		result = append(result, *item)
	}
	for i := range result {
		s.applyScore(&result[i])
		result[i].HealthStatus = runtimeHealthStatus(result[i])
	}
	sortRuntimeStatusItems(result)
	if len(result) > query.Limit {
		result = result[:query.Limit]
	}
	filteredQueue := filteredRuntimeQueueSnapshot(detailedQueue, result, query)
	s.applyQueueCooldownHints(filteredQueue, result)
	return RuntimeStatusResponse{
		Summary:       summarizeRuntimeStatus(result, s.deps.Now()),
		QueueSnapshot: filteredQueue,
		Items:         result,
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

func (s *RuntimeStatusService) queueDetailSnapshot() core.RuntimeQueueSnapshot {
	if s == nil || s.deps.QueueDetailSnapshot == nil {
		return core.RuntimeQueueSnapshot{}
	}
	return s.deps.QueueDetailSnapshot()
}

func (s *RuntimeStatusService) applyLiveState(item *RuntimeStatusItem, query RuntimeStatusQuery) {
	if s == nil || s.deps.StateProvider == nil || item == nil || item.ChannelID <= 0 {
		return
	}
	if runtimeStatusStrictAccountScope(query) {
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

func (s *RuntimeStatusService) applyScore(item *RuntimeStatusItem) {
	if item == nil || item.ChannelID <= 0 {
		return
	}
	snapshot := core.RuntimeSnapshot{
		Key: core.RuntimeKey{
			RequestedModel:        item.RequestedModel,
			UpstreamModel:         item.UpstreamModel,
			ChannelID:             item.ChannelID,
			Group:                 item.Group,
			EndpointType:          constant.EndpointType(item.EndpointType),
			CapabilityFingerprint: item.CapabilityFingerprint,
		},
		SuccessRate:                item.SuccessRate,
		TTFTMs:                     item.TTFTMs,
		DurationMs:                 item.DurationMs,
		TokensPerSecond:            item.TokensPerSecond,
		ActiveConcurrency:          item.ActiveConcurrency,
		MaxConcurrency:             item.MaxConcurrency,
		ConfiguredConcurrencyLimit: item.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:    item.LearnedConcurrencyLimit,
		EffectiveConcurrencyLimit:  item.EffectiveConcurrencyLimit,
		QueueDepth:                 item.QueueDepth,
		QueueCapacity:              item.QueueCapacity,
		QueueTimeoutMs:             item.QueueTimeoutMs,
		EstimatedQueueWaitMs:       item.EstimatedQueueWaitMs,
		FirstBytePending:           item.FirstBytePending,
		SlowFirstBytePending:       item.SlowFirstBytePending,
		OldestFirstByteWaitMs:      item.OldestFirstByteWaitMs,
		CostRatio:                  item.CostRatio,
		CostReferenceRatio:         item.CostReferenceRatio,
		CostPricingMode:            item.CostPricingMode,
		GroupPriorityRatio:         item.GroupPriorityRatio,
		EmptyOutputRate:            item.EmptyOutputRate,
		ExperienceIssueRate:        item.ExperienceIssueRate,
		CircuitState:               core.CircuitState(item.CircuitState),
		CircuitOpen:                item.CircuitOpen,
		CircuitOpenUntil:           item.CircuitOpenUntil,
		CircuitOpenReason:          item.CircuitOpenReason,
		CircuitFailureCount:        item.CircuitFailureCount,
		CircuitFailureRate:         item.CircuitFailureRate,
		CircuitSampleCount:         item.CircuitSampleCount,
		CircuitErrorCounts:         copyRuntimeStatusCircuitErrorCounts(item.CircuitErrorCounts),
		CircuitHalfOpenProbeUsed:   item.CircuitHalfOpenProbeUsed,
		CircuitHalfOpenProbeMax:    item.CircuitHalfOpenProbeMax,
		Cooldown:                   item.Cooldown,
		FailureAvoidance:           item.FailureAvoidance,
		ProbeRecoveryPending:       item.ProbeRecoveryPending,
		ProbeRecoverySuccessCount:  item.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:      item.ProbeRecoveryRequired,
		ProbeTriggerReason:         item.ProbeTriggerReason,
		ConfigErrorIsolated:        item.ConfigErrorIsolated,
		IsolationReason:            item.IsolationReason,
		IsolationUntil:             item.IsolationUntil,
		AuthConfigErrorCount:       item.AuthConfigErrorCount,
		LastAuthConfigErrorAt:      item.LastAuthConfigErrorAt,
		SampleCount:                item.SampleCount,
		LastRealAttemptAt:          item.LastRealAttemptAt,
		LastRealSuccessAt:          item.LastRealSuccessAt,
		LastRealFailureAt:          item.LastRealFailureAt,
		RealSampleCount30m:         item.RealSampleCount30m,
		LastProbeAt:                item.LastProbeAt,
		LastProbeSuccessAt:         item.LastProbeSuccessAt,
		RecentLatencySamples:       append([]core.RuntimeLatencySample(nil), item.RecentLatencySamples...),
	}
	if item.CircuitState != "" {
		snapshot.CircuitState = core.CircuitState(item.CircuitState)
	}
	channel := s.channelForRuntimeStatusItem(item)
	if channel != nil && strings.TrimSpace(item.ChannelName) == "" {
		item.ChannelName = channel.Name
	}
	candidate := core.Candidate{
		Channel:       channel,
		Group:         item.Group,
		UpstreamModel: item.UpstreamModel,
		RuntimeKey:    snapshot.Key,
	}
	policy := s.policyForRuntimeStatusItem(item)
	service := scheduler.NewCandidateScoringService().
		WithCostBaselineProvider(s.deps.CostBaselineProvider)
	score := service.EvaluateCandidate(candidate, snapshot, policy, scheduler.ScoringContext{
		RequestedModel:  item.RequestedModel,
		EndpointType:    constant.EndpointType(item.EndpointType),
		CandidateGroups: append([]string(nil), policy.CandidateGroups...),
		Strategy:        policy.Strategy,
		AutoMode:        policy.AutoMode,
		ScoreWeights:    s.deps.ScoreWeights,
		ExplainEnabled:  true,
	}).Score
	item.ScoreTotal = roundRuntimeStatusFloat(score.Total)
	item.ScoreBreakdown = roundRuntimeStatusScoreMap(score.Breakdown)
	item.ScoreItems = roundRuntimeStatusScoreItems(score.Items)
	item.RoutingScoreTotal = roundRuntimeStatusFloat(score.RoutingTotal)
	item.RoutingScoreBreakdown = roundRuntimeStatusScoreMap(score.RoutingBreakdown)
	item.RoutingScoreItems = roundRuntimeStatusScoreItems(score.RoutingItems)
	item.StateTags = append([]string(nil), score.StateTags...)
	item.CostReferenceMissing = score.CostReferenceMissing
}

func (s *RuntimeStatusService) channelForRuntimeStatusItem(item *RuntimeStatusItem) *model.Channel {
	if s == nil || item == nil || item.ChannelID <= 0 {
		return nil
	}
	if s.deps.ChannelProvider != nil {
		channel, ok := s.deps.ChannelProvider(item.ChannelID)
		if ok {
			return channel
		}
		return nil
	}
	if model.DB == nil {
		return nil
	}
	channel, err := model.CacheGetChannel(item.ChannelID)
	if err != nil {
		return nil
	}
	return channel
}

func (s *RuntimeStatusService) policyForRuntimeStatusItem(item *RuntimeStatusItem) core.GroupSmartPolicy {
	group := strings.TrimSpace(item.Group)
	if s != nil && s.deps.PolicyForGroup != nil {
		policy := s.deps.PolicyForGroup(group)
		policy = completeRuntimeStatusPolicy(policy, item)
		return policy
	}
	return completeRuntimeStatusPolicy(core.GroupSmartPolicy{}, item)
}

func completeRuntimeStatusPolicy(policy core.GroupSmartPolicy, item *RuntimeStatusItem) core.GroupSmartPolicy {
	group := ""
	if item != nil {
		group = strings.TrimSpace(item.Group)
	}
	if strings.TrimSpace(policy.RequestedGroup) == "" {
		policy.RequestedGroup = group
	}
	if strings.TrimSpace(policy.UserGroup) == "" {
		policy.UserGroup = group
	}
	if strings.TrimSpace(policy.Strategy) == "" {
		policy.Strategy = core.StrategyBalanced
	}
	if strings.TrimSpace(policy.AutoMode) == "" {
		policy.AutoMode = core.AutoModeSequential
	}
	if len(policy.CandidateGroups) == 0 && group != "" {
		policy.CandidateGroups = []string{group}
	}
	if policy.GroupPriorityRatio == nil {
		policy.GroupPriorityRatio = map[string]float64{}
	}
	if group != "" {
		ratio := 0.0
		if item != nil {
			ratio = item.GroupPriorityRatio
		}
		if ratio <= 0 {
			ratio = 1
		}
		if policy.GroupPriorityRatio[group] <= 0 {
			policy.GroupPriorityRatio[group] = ratio
		}
	}
	return policy
}

func roundRuntimeStatusScoreMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = roundRuntimeStatusFloat(value)
	}
	return out
}

func roundRuntimeStatusScoreItems(values []core.ScoreItem) []core.ScoreItem {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.ScoreItem, 0, len(values))
	for _, item := range values {
		item.Score = roundRuntimeStatusFloat(item.Score)
		item.Weight = roundRuntimeStatusFloat(item.Weight)
		item.WeightedScore = roundRuntimeStatusFloat(item.WeightedScore)
		item.PreviousScore = roundRuntimeStatusFloat(item.PreviousScore)
		item.Delta = roundRuntimeStatusFloat(item.Delta)
		if item.RawNumber != nil {
			value := roundRuntimeStatusFloat(*item.RawNumber)
			item.RawNumber = &value
		}
		if item.ReferenceNumber != nil {
			value := roundRuntimeStatusFloat(*item.ReferenceNumber)
			item.ReferenceNumber = &value
		}
		for key, value := range item.FormulaParameters {
			item.FormulaParameters[key] = roundRuntimeStatusFloat(value)
		}
		out = append(out, item)
	}
	return out
}

func roundRuntimeStatusFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*10000) / 10000
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
		ResourceID:            key.ResourceID,
		ResourceType:          key.ResourceType,
		AccountID:             key.AccountID,
		AccountType:           key.AccountType,
		Brand:                 key.Brand,
		Provider:              key.Provider,
		CredentialIndex:       key.CredentialIndex,
		CredentialSubjectFP:   key.CredentialSubjectFP,
		CredentialFP:          key.CredentialFP,
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
	item.ConfiguredConcurrencyLimit = snapshot.ConfiguredConcurrencyLimit
	item.LearnedConcurrencyLimit = snapshot.LearnedConcurrencyLimit
	item.EffectiveConcurrencyLimit = snapshot.EffectiveConcurrencyLimit
	item.QueueDepth = snapshot.QueueDepth
	item.QueueCapacity = snapshot.QueueCapacity
	item.QueueTimeoutMs = snapshot.QueueTimeoutMs
	item.EstimatedQueueWaitMs = snapshot.EstimatedQueueWaitMs
	item.FirstBytePending = snapshot.FirstBytePending
	item.SlowFirstBytePending = snapshot.SlowFirstBytePending
	item.OldestFirstByteWaitMs = snapshot.OldestFirstByteWaitMs
	item.CostRatio = snapshot.CostRatio
	item.CostReferenceRatio = snapshot.CostReferenceRatio
	item.CostPricingMode = snapshot.CostPricingMode
	item.GroupPriorityRatio = snapshot.GroupPriorityRatio
	if snapshot.CircuitState != "" {
		item.CircuitState = string(snapshot.CircuitState)
	}
	item.CircuitOpen = snapshot.CircuitOpen || snapshot.CircuitState == core.CircuitStateOpen
	item.CircuitOpenUntil = snapshot.CircuitOpenUntil
	item.CircuitOpenReason = snapshot.CircuitOpenReason
	item.CircuitFailureCount = snapshot.CircuitFailureCount
	item.CircuitFailureRate = snapshot.CircuitFailureRate
	item.CircuitSampleCount = snapshot.CircuitSampleCount
	item.CircuitErrorCounts = copyRuntimeStatusCircuitErrorCounts(snapshot.CircuitErrorCounts)
	item.CircuitHalfOpenProbeUsed = snapshot.CircuitHalfOpenProbeUsed
	item.CircuitHalfOpenProbeMax = snapshot.CircuitHalfOpenProbeMax
	item.Cooldown = snapshot.Cooldown
	item.FailureAvoidance = snapshot.FailureAvoidance
	item.EmptyOutputRate = snapshot.EmptyOutputRate
	item.ExperienceIssueRate = snapshot.ExperienceIssueRate
	item.ProbeRecoveryPending = snapshot.ProbeRecoveryPending
	item.ProbeRecoverySuccessCount = snapshot.ProbeRecoverySuccessCount
	item.ProbeRecoveryRequired = snapshot.ProbeRecoveryRequired
	item.ProbeTriggerReason = snapshot.ProbeTriggerReason
	item.ConfigErrorIsolated = snapshot.ConfigErrorIsolated
	item.IsolationReason = snapshot.IsolationReason
	item.IsolationUntil = snapshot.IsolationUntil
	item.AuthConfigErrorCount = snapshot.AuthConfigErrorCount
	item.LastAuthConfigErrorAt = snapshot.LastAuthConfigErrorAt
	item.SampleCount = snapshot.SampleCount
	item.LastRealAttemptAt = snapshot.LastRealAttemptAt
	item.LastRealSuccessAt = snapshot.LastRealSuccessAt
	item.LastRealFailureAt = snapshot.LastRealFailureAt
	item.RealSampleCount30m = snapshot.RealSampleCount30m
	item.LastProbeAt = snapshot.LastProbeAt
	item.LastProbeSuccessAt = snapshot.LastProbeSuccessAt
	item.RecentLatencySamples = append([]core.RuntimeLatencySample(nil), snapshot.RecentLatencySamples...)
}

func applyRuntimeQueueChannelSnapshot(item *RuntimeStatusItem, snapshot core.RuntimeQueueChannelSnapshot) {
	if item == nil {
		return
	}
	if snapshot.QueueDepth > item.QueueDepth {
		item.QueueDepth = snapshot.QueueDepth
	}
	if snapshot.QueueCapacity > 0 && item.QueueCapacity <= 0 {
		item.QueueCapacity = snapshot.QueueCapacity
	}
}

func applyRuntimeQueueKeySnapshot(item *RuntimeStatusItem, snapshot core.RuntimeQueueKeySnapshot) {
	if item == nil {
		return
	}
	if snapshot.QueueDepth > item.QueueDepth {
		item.QueueDepth = snapshot.QueueDepth
	}
}

func clearAccountScopedChannelDynamicFields(item *RuntimeStatusItem) {
	if item == nil {
		return
	}
	item.ActiveConcurrency = 0
	item.MaxConcurrency = 0
	item.ConfiguredConcurrencyLimit = 0
	item.LearnedConcurrencyLimit = 0
	item.EffectiveConcurrencyLimit = 0
	item.QueueDepth = 0
	item.QueueCapacity = 0
	item.QueueTimeoutMs = 0
	item.EstimatedQueueWaitMs = 0
	item.FirstBytePending = 0
	item.SlowFirstBytePending = 0
	item.OldestFirstByteWaitMs = 0
}

func (s *RuntimeStatusService) applyQueueCooldownHints(snapshot *core.RuntimeQueueSnapshot, items []RuntimeStatusItem) {
	if s == nil || snapshot == nil || s.deps.StateProvider == nil {
		return
	}
	seen := map[int]struct{}{}
	for _, item := range items {
		if item.ChannelID <= 0 {
			continue
		}
		if _, ok := seen[item.ChannelID]; ok {
			continue
		}
		seen[item.ChannelID] = struct{}{}
		if item.Cooldown {
			snapshot.Cooldowns = append(snapshot.Cooldowns, core.RuntimeQueueCooldownHint{
				ChannelID:                item.ChannelID,
				Group:                    item.Group,
				Reason:                   item.CooldownReason,
				CooldownRemainingSeconds: item.CooldownRemainingSeconds,
			})
		}
		if item.FailureAvoidance {
			snapshot.Cooldowns = append(snapshot.Cooldowns, core.RuntimeQueueCooldownHint{
				ChannelID:                        item.ChannelID,
				Group:                            item.Group,
				Reason:                           item.FailureAvoidanceReason,
				FailureAvoidanceRemainingSeconds: item.FailureAvoidanceRemainingSeconds,
			})
		}
	}
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
	item.CircuitOpenReason = snapshot.OpenReason
	item.CircuitFailureCount = snapshot.FailureCount
	item.CircuitFailureRate = snapshot.FailureRate
	item.CircuitSampleCount = snapshot.SampleCount
	item.CircuitErrorCounts = snapshot.ErrorCounts
	item.CircuitHalfOpenProbeUsed = snapshot.HalfOpenProbeUsed
	item.CircuitHalfOpenProbeMax = snapshot.HalfOpenProbeMax
}

func copyRuntimeStatusCircuitErrorCounts(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
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
	if !runtimeStatusMatchesAccountFilters(key.AccountID, key.CredentialIndex, key.CredentialSubjectFP, key.CredentialFP, query) {
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
	if !runtimeStatusMatchesAccountFilters(item.AccountID, item.CredentialIndex, item.CredentialSubjectFP, item.CredentialFP, query) {
		return false
	}
	return true
}

func runtimeStatusMatchesAccountFilters(accountID string, credentialIndex int, subjectFP string, credentialFP string, query RuntimeStatusQuery) bool {
	if !runtimeStatusHasAccountFilters(query) {
		return true
	}
	if query.StrictAccountScope && !runtimeStatusHasAccountIdentity(accountID, subjectFP, credentialFP) {
		return false
	}
	if accountID != "" && stringSliceContains(query.AccountIDs, accountID) {
		return true
	}
	if intSliceContains(query.CredentialIndexes, credentialIndex) {
		return true
	}
	if subjectFP != "" && stringSliceContains(query.CredentialSubjectFPs, subjectFP) {
		return true
	}
	if credentialFP != "" && stringSliceContains(query.CredentialFPs, credentialFP) {
		return true
	}
	return false
}

func runtimeStatusStrictAccountScope(query RuntimeStatusQuery) bool {
	return query.StrictAccountScope && runtimeStatusHasAccountFilters(query)
}

func runtimeStatusHasAccountFilters(query RuntimeStatusQuery) bool {
	return len(query.AccountIDs) > 0 ||
		len(query.CredentialIndexes) > 0 ||
		len(query.CredentialSubjectFPs) > 0 ||
		len(query.CredentialFPs) > 0
}

func runtimeStatusHasAccountIdentity(accountID string, subjectFP string, credentialFP string) bool {
	return strings.TrimSpace(accountID) != "" ||
		strings.TrimSpace(subjectFP) != "" ||
		strings.TrimSpace(credentialFP) != ""
}

func runtimeQueueSnapshotKey(snapshot core.RuntimeQueueKeySnapshot) core.RuntimeKey {
	key := snapshot.RuntimeKey
	if key.RequestedModel == "" {
		key.RequestedModel = snapshot.RequestedModel
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = snapshot.UpstreamModel
	}
	if key.ChannelID <= 0 {
		key.ChannelID = snapshot.ChannelID
	}
	if key.Group == "" {
		key.Group = snapshot.Group
	}
	if key.EndpointType == "" && snapshot.EndpointType != "" {
		key.EndpointType = constant.EndpointType(snapshot.EndpointType)
	}
	if key.CapabilityFingerprint == "" {
		key.CapabilityFingerprint = snapshot.CapabilityFingerprint
	}
	return key
}

func runtimeStatusItemMatchesQueueKey(item RuntimeStatusItem, key core.RuntimeKey) bool {
	if key.ChannelID > 0 && item.ChannelID != key.ChannelID {
		return false
	}
	if key.RequestedModel != "" && item.RequestedModel != "" && item.RequestedModel != key.RequestedModel {
		return false
	}
	if key.UpstreamModel != "" && item.UpstreamModel != "" && item.UpstreamModel != key.UpstreamModel {
		return false
	}
	if key.Group != "" && item.Group != "" && item.Group != key.Group {
		return false
	}
	if key.EndpointType != "" && item.EndpointType != "" && item.EndpointType != string(key.EndpointType) {
		return false
	}
	if key.CapabilityFingerprint != "" && item.CapabilityFingerprint != "" && item.CapabilityFingerprint != key.CapabilityFingerprint {
		return false
	}
	if !runtimeStatusQueueAccountIdentityMatches(item, key) {
		return false
	}
	return true
}

func runtimeStatusQueueAccountIdentityMatches(item RuntimeStatusItem, key core.RuntimeKey) bool {
	hasItemAccount := runtimeStatusHasAccountIdentity(item.AccountID, item.CredentialSubjectFP, item.CredentialFP)
	hasKeyAccount := runtimeStatusHasAccountIdentity(key.AccountID, key.CredentialSubjectFP, key.CredentialFP)
	if !hasItemAccount && !hasKeyAccount {
		return true
	}
	if item.AccountID != "" && key.AccountID != "" && item.AccountID == key.AccountID {
		return true
	}
	if item.CredentialSubjectFP != "" && key.CredentialSubjectFP != "" && item.CredentialSubjectFP == key.CredentialSubjectFP {
		return true
	}
	if item.CredentialFP != "" && key.CredentialFP != "" && item.CredentialFP == key.CredentialFP {
		return true
	}
	return false
}

func stringSliceContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func intSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func runtimeHealthStatus(item RuntimeStatusItem) string {
	switch {
	case item.CircuitOpen:
		return "circuit_open"
	case item.Cooldown:
		return "cooldown"
	case item.FailureAvoidance:
		return "failure_avoidance"
	case item.QueueDepth > 0 && runtimeStatusQueueOnly(item):
		return "queued"
	case runtimeConcurrencyPressureRatio(item) >= 0.90:
		return "high_pressure"
	case item.SuccessRate > 0 && item.SuccessRate < 0.80:
		return "degraded"
	default:
		return "healthy"
	}
}

func runtimeStatusQueueOnly(item RuntimeStatusItem) bool {
	return item.SampleCount <= 0 &&
		strings.TrimSpace(item.RequestedModel) == "" &&
		strings.TrimSpace(item.UpstreamModel) == "" &&
		strings.TrimSpace(item.Group) == "" &&
		item.ActiveConcurrency <= 0 &&
		item.SuccessRate <= 0 &&
		item.TTFTMs <= 0 &&
		item.DurationMs <= 0
}

func summarizeRuntimeStatus(items []RuntimeStatusItem, now time.Time) RuntimeStatusSummary {
	summary := RuntimeStatusSummary{UpdatedAt: now.Unix(), RuntimeKeys: len(items)}
	channelIDs := map[int]struct{}{}
	queueChannels := map[int]struct{}{}
	cooldownChannels := map[int]struct{}{}
	avoidanceChannels := map[int]struct{}{}
	lowScoreRecoveryChannels := map[int]struct{}{}
	pendingRecoveryChannels := map[int]struct{}{}
	recentlyRecoveredChannels := map[int]struct{}{}
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
		if item.ProbeTriggerReason == "low_score" {
			lowScoreRecoveryChannels[item.ChannelID] = struct{}{}
		}
		if item.ProbeRecoveryPending {
			pendingRecoveryChannels[item.ChannelID] = struct{}{}
		}
		if !item.FailureAvoidance && item.ProbeRecoverySuccessCount > 0 {
			recentlyRecoveredChannels[item.ChannelID] = struct{}{}
		}
		if runtimeConcurrencyPressureRatio(item) >= 0.90 {
			summary.HighPressureChannels++
		}
	}
	summary.Channels = len(channelIDs)
	summary.QueueChannels = len(queueChannels)
	summary.CooldownChannels = len(cooldownChannels)
	summary.FailureAvoidanceChannels = len(avoidanceChannels)
	summary.LowScoreRecoveryChannels = len(lowScoreRecoveryChannels)
	summary.ProbeRecoveryPendingChannels = len(pendingRecoveryChannels)
	summary.RecentlyRecoveredChannels = len(recentlyRecoveredChannels)
	return summary
}

func runtimeConcurrencyPressureRatio(item RuntimeStatusItem) float64 {
	limit := item.MaxConcurrency
	if limit <= 0 {
		return 0
	}
	return float64(item.ActiveConcurrency) / float64(limit)
}

func filteredRuntimeQueueSnapshot(snapshot core.RuntimeQueueSnapshot, items []RuntimeStatusItem, query RuntimeStatusQuery) *core.RuntimeQueueSnapshot {
	if len(snapshot.Channels) == 0 && len(snapshot.RuntimeKeys) == 0 && len(snapshot.Groups) == 0 && len(snapshot.RejectReasons) == 0 && snapshot.Summary.TotalQueued == 0 {
		return nil
	}
	accountScoped := runtimeStatusStrictAccountScope(query)
	allowedChannels := map[int]struct{}{}
	for _, item := range items {
		if item.ChannelID > 0 {
			allowedChannels[item.ChannelID] = struct{}{}
		}
	}
	if query.ChannelID > 0 {
		allowedChannels[query.ChannelID] = struct{}{}
	}
	out := core.RuntimeQueueSnapshot{
		UpdatedAt:     snapshot.UpdatedAt,
		Summary:       core.RuntimeQueueSummary{UpdatedAt: snapshot.Summary.UpdatedAt},
		RejectReasons: append([]core.RuntimeQueueReasonCount(nil), snapshot.RejectReasons...),
		Nodes:         filterRuntimeQueueNodes(snapshot.Nodes, allowedChannels, query),
	}
	if out.UpdatedAt == 0 {
		out.UpdatedAt = snapshot.Summary.UpdatedAt
	}
	if out.Summary.UpdatedAt == 0 {
		out.Summary.UpdatedAt = out.UpdatedAt
	}
	if !accountScoped {
		for _, channel := range snapshot.Channels {
			if len(allowedChannels) > 0 {
				if _, ok := allowedChannels[channel.ChannelID]; !ok {
					continue
				}
			}
			if query.Group != "" && !runtimeQueueChannelHasGroup(channel, query.Group) {
				continue
			}
			filteredChannel := channel
			filteredChannel.Groups = filterRuntimeQueueGroups(channel.Groups, query.Group)
			out.Channels = append(out.Channels, filteredChannel)
			addRuntimeQueueChannelToSummary(&out.Summary, filteredChannel)
		}
	}
	queueKeyChannels := map[int]struct{}{}
	for _, item := range snapshot.RuntimeKeys {
		key := runtimeQueueSnapshotKey(item)
		if len(allowedChannels) > 0 {
			if _, ok := allowedChannels[key.ChannelID]; !ok {
				continue
			}
		}
		if !runtimeStatusMatchesQuery(key, query) {
			continue
		}
		out.RuntimeKeys = append(out.RuntimeKeys, item)
		if accountScoped {
			addRuntimeQueueKeyToSummary(&out.Summary, item)
			if key.ChannelID > 0 && item.QueueDepth > 0 {
				queueKeyChannels[key.ChannelID] = struct{}{}
			}
		}
	}
	if accountScoped {
		out.Summary.QueueChannels = len(queueKeyChannels)
		out.Groups = aggregateRuntimeQueueSnapshotGroupsFromKeys(out.RuntimeKeys)
	} else {
		out.Groups = aggregateRuntimeQueueSnapshotGroups(out.Channels)
	}
	out.Summary.QueueGroups = len(out.Groups)
	out.Summary.QueueNodes = len(out.Nodes)
	if len(out.Channels) == 0 && len(out.RuntimeKeys) == 0 && len(out.RejectReasons) == 0 {
		return nil
	}
	return &out
}

func filterRuntimeQueueNodes(nodes []core.RuntimeQueueNodeSnapshot, allowedChannels map[int]struct{}, query RuntimeStatusQuery) []core.RuntimeQueueNodeSnapshot {
	if len(nodes) == 0 {
		return nil
	}
	accountScoped := runtimeStatusStrictAccountScope(query)
	out := make([]core.RuntimeQueueNodeSnapshot, 0, len(nodes))
	for _, node := range nodes {
		filtered := core.RuntimeQueueNodeSnapshot{
			NodeID:        node.NodeID,
			UpdatedAt:     node.UpdatedAt,
			Summary:       core.RuntimeQueueSummary{UpdatedAt: node.Summary.UpdatedAt},
			RejectReasons: append([]core.RuntimeQueueReasonCount(nil), node.RejectReasons...),
			Cooldowns:     append([]core.RuntimeQueueCooldownHint(nil), node.Cooldowns...),
		}
		if filtered.UpdatedAt == 0 {
			filtered.UpdatedAt = filtered.Summary.UpdatedAt
		}
		if filtered.Summary.UpdatedAt == 0 {
			filtered.Summary.UpdatedAt = filtered.UpdatedAt
		}
		if !accountScoped {
			for _, channel := range node.Channels {
				if len(allowedChannels) > 0 {
					if _, ok := allowedChannels[channel.ChannelID]; !ok {
						continue
					}
				}
				if query.Group != "" && !runtimeQueueChannelHasGroup(channel, query.Group) {
					continue
				}
				filteredChannel := channel
				filteredChannel.Groups = filterRuntimeQueueGroups(channel.Groups, query.Group)
				filtered.Channels = append(filtered.Channels, filteredChannel)
				addRuntimeQueueChannelToSummary(&filtered.Summary, filteredChannel)
			}
		}
		queueKeyChannels := map[int]struct{}{}
		for _, item := range node.RuntimeKeys {
			key := runtimeQueueSnapshotKey(item)
			if len(allowedChannels) > 0 {
				if _, ok := allowedChannels[key.ChannelID]; !ok {
					continue
				}
			}
			if !runtimeStatusMatchesQuery(key, query) {
				continue
			}
			filtered.RuntimeKeys = append(filtered.RuntimeKeys, item)
			if accountScoped {
				addRuntimeQueueKeyToSummary(&filtered.Summary, item)
				if key.ChannelID > 0 && item.QueueDepth > 0 {
					queueKeyChannels[key.ChannelID] = struct{}{}
				}
			}
		}
		if accountScoped {
			filtered.Summary.QueueChannels = len(queueKeyChannels)
			filtered.Groups = aggregateRuntimeQueueSnapshotGroupsFromKeys(filtered.RuntimeKeys)
		} else {
			filtered.Groups = aggregateRuntimeQueueSnapshotGroups(filtered.Channels)
		}
		filtered.Summary.QueueGroups = len(filtered.Groups)
		if len(filtered.Channels) == 0 && len(filtered.RuntimeKeys) == 0 && len(filtered.RejectReasons) == 0 && filtered.Summary.TotalQueued == 0 {
			continue
		}
		out = append(out, filtered)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Summary.TotalQueued != out[j].Summary.TotalQueued {
			return out[i].Summary.TotalQueued > out[j].Summary.TotalQueued
		}
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func runtimeQueueChannelHasGroup(channel core.RuntimeQueueChannelSnapshot, group string) bool {
	if group == "" {
		return true
	}
	for _, item := range channel.Groups {
		if item.Group == group {
			return true
		}
	}
	return false
}

func filterRuntimeQueueGroups(groups []core.RuntimeQueueGroupSnapshot, group string) []core.RuntimeQueueGroupSnapshot {
	if len(groups) == 0 {
		return nil
	}
	if group == "" {
		return append([]core.RuntimeQueueGroupSnapshot(nil), groups...)
	}
	filtered := make([]core.RuntimeQueueGroupSnapshot, 0, len(groups))
	for _, item := range groups {
		if item.Group == group {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func addRuntimeQueueChannelToSummary(summary *core.RuntimeQueueSummary, channel core.RuntimeQueueChannelSnapshot) {
	if summary == nil {
		return
	}
	summary.TotalQueued += channel.QueueDepth
	summary.TotalDepth += channel.QueueDepth
	summary.Waiting += channel.WaitingRequests
	summary.QueuedRequests += channel.QueuedRequests
	summary.WaitingRequests += channel.WaitingRequests
	summary.HighPriorityDepth += channel.HighPriorityDepth
	summary.NormalDepth += channel.NormalDepth
	summary.QueueCapacity += channel.QueueCapacity
	summary.TotalCapacity += channel.QueueCapacity
	summary.HighPriorityCapacity += channel.HighPriorityCapacity
	summary.NormalCapacity += channel.NormalCapacity
	if channel.QueueDepth > summary.MaxQueueDepth {
		summary.MaxQueueDepth = channel.QueueDepth
	}
	if channel.QueueDepth > 0 {
		summary.QueueChannels++
	}
}

func addRuntimeQueueKeyToSummary(summary *core.RuntimeQueueSummary, item core.RuntimeQueueKeySnapshot) {
	if summary == nil {
		return
	}
	summary.TotalQueued += item.QueueDepth
	summary.TotalDepth += item.QueueDepth
	summary.Waiting += item.WaitingRequests
	summary.QueuedRequests += item.QueuedRequests
	summary.WaitingRequests += item.WaitingRequests
	summary.HighPriorityDepth += item.HighPriorityDepth
	summary.NormalDepth += item.NormalDepth
	if item.QueueDepth > summary.MaxQueueDepth {
		summary.MaxQueueDepth = item.QueueDepth
	}
}

func aggregateRuntimeQueueSnapshotGroups(channels []core.RuntimeQueueChannelSnapshot) []core.RuntimeQueueGroupSnapshot {
	groupMap := map[string]*core.RuntimeQueueGroupSnapshot{}
	for _, channel := range channels {
		for _, group := range channel.Groups {
			key := group.Group
			if key == "" {
				key = "_default"
			}
			target := groupMap[key]
			if target == nil {
				target = &core.RuntimeQueueGroupSnapshot{Group: group.Group}
				groupMap[key] = target
			}
			target.QueueDepth += group.QueueDepth
			target.QueuedRequests += group.QueuedRequests
			target.WaitingRequests += group.WaitingRequests
			target.HighPriorityDepth += group.HighPriorityDepth
			target.NormalDepth += group.NormalDepth
		}
	}
	groups := make([]core.RuntimeQueueGroupSnapshot, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].QueueDepth != groups[j].QueueDepth {
			return groups[i].QueueDepth > groups[j].QueueDepth
		}
		return groups[i].Group < groups[j].Group
	})
	return groups
}

func aggregateRuntimeQueueSnapshotGroupsFromKeys(keys []core.RuntimeQueueKeySnapshot) []core.RuntimeQueueGroupSnapshot {
	groupMap := map[string]*core.RuntimeQueueGroupSnapshot{}
	for _, item := range keys {
		if item.QueueDepth <= 0 {
			continue
		}
		runtimeKey := runtimeQueueSnapshotKey(item)
		groupKey := runtimeKey.Group
		if groupKey == "" {
			groupKey = "_default"
		}
		target := groupMap[groupKey]
		if target == nil {
			target = &core.RuntimeQueueGroupSnapshot{
				ChannelID: runtimeKey.ChannelID,
				Group:     runtimeKey.Group,
			}
			groupMap[groupKey] = target
		}
		target.QueueDepth += item.QueueDepth
		target.QueuedRequests += item.QueuedRequests
		target.WaitingRequests += item.WaitingRequests
		target.HighPriorityDepth += item.HighPriorityDepth
		target.NormalDepth += item.NormalDepth
	}
	groups := make([]core.RuntimeQueueGroupSnapshot, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].QueueDepth != groups[j].QueueDepth {
			return groups[i].QueueDepth > groups[j].QueueDepth
		}
		return groups[i].Group < groups[j].Group
	})
	return groups
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
	case item.QueueDepth > 0:
		return 35
	case runtimeConcurrencyPressureRatio(item) >= 0.90:
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
