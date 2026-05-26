package probe

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayprovider "github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/service"
)

const probeActivationWindow = 30 * time.Minute

type ProbeSelector struct {
	store     core.RuntimeSnapshotStore
	breaker   core.CircuitBreaker
	mu        sync.Mutex
	lastProbe map[core.RuntimeKey]time.Time
	lastOK    map[core.RuntimeKey]time.Time
	now       func() time.Time
}

type probeRecentScopes struct {
	pairs  map[string]struct{}
	groups map[string]struct{}
}

type probeRecentRequestScopeRow struct {
	RequestedModel string
	RequestedGroup string
	SelectedGroup  string
}

func NewProbeSelector(store core.RuntimeSnapshotStore, breaker core.CircuitBreaker) *ProbeSelector {
	return &ProbeSelector{
		store:     store,
		breaker:   breaker,
		lastProbe: map[core.RuntimeKey]time.Time{},
		lastOK:    map[core.RuntimeKey]time.Time{},
		now:       time.Now,
	}
}

func (s *ProbeSelector) Select(config ProbeConfig) ([]ProbeCandidate, error) {
	if s == nil {
		return nil, nil
	}
	now := s.now()
	config = normalizeProbeConfig(config)
	recent, err := recentProbeScopes(now)
	if err != nil {
		return nil, err
	}
	if recent.Empty() {
		return nil, nil
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	channelByID := eligibleProbeChannels(channels)
	candidateByKey := map[core.RuntimeKey]ProbeCandidate{}
	for _, candidate := range s.lowHealthCandidatesLocked(channelByID, recent, now, config) {
		candidateByKey[candidate.Key] = candidate
	}
	for _, candidate := range s.lowTrafficCandidatesLocked(channelByID, recent, now, config) {
		if _, exists := candidateByKey[candidate.Key]; exists {
			continue
		}
		candidateByKey[candidate.Key] = candidate
	}
	candidates := make([]ProbeCandidate, 0, len(candidateByKey))
	for _, candidate := range candidateByKey {
		candidates = append(candidates, candidate)
	}
	candidates = collapseProbeCandidatesByChannel(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		return compareProbeCandidates(candidates[i], candidates[j]) < 0
	})
	if config.MaxPerTick > 0 && len(candidates) > config.MaxPerTick {
		candidates = candidates[:config.MaxPerTick]
	}
	for _, candidate := range candidates {
		s.lastProbe[candidate.Key] = now
		s.markProbeSelectionLocked(candidate.Key, candidate.Reason, config)
	}
	return candidates, nil
}

func collapseProbeCandidatesByChannel(candidates []ProbeCandidate) []ProbeCandidate {
	if len(candidates) <= 1 {
		return candidates
	}
	bestByChannel := make(map[int]ProbeCandidate, len(candidates))
	for _, candidate := range candidates {
		channelID := 0
		if candidate.Channel != nil {
			channelID = candidate.Channel.Id
		}
		if channelID <= 0 {
			continue
		}
		current, exists := bestByChannel[channelID]
		if !exists || compareProbeCandidates(candidate, current) < 0 {
			bestByChannel[channelID] = candidate
		}
	}
	collapsed := make([]ProbeCandidate, 0, len(bestByChannel))
	for _, candidate := range bestByChannel {
		collapsed = append(collapsed, candidate)
	}
	return collapsed
}

func compareProbeCandidates(left ProbeCandidate, right ProbeCandidate) int {
	leftPriority := probeReasonPriority(left.Reason)
	rightPriority := probeReasonPriority(right.Reason)
	if leftPriority != rightPriority {
		if leftPriority < rightPriority {
			return -1
		}
		return 1
	}
	if left.Score != right.Score {
		if left.Score > right.Score {
			return -1
		}
		return 1
	}
	leftPreferred := probeCandidateUsesPreferredModel(left)
	rightPreferred := probeCandidateUsesPreferredModel(right)
	if leftPreferred != rightPreferred {
		if leftPreferred {
			return -1
		}
		return 1
	}
	leftChannelID := 0
	if left.Channel != nil {
		leftChannelID = left.Channel.Id
	}
	rightChannelID := 0
	if right.Channel != nil {
		rightChannelID = right.Channel.Id
	}
	if leftChannelID != rightChannelID {
		if leftChannelID < rightChannelID {
			return -1
		}
		return 1
	}
	if left.Model != right.Model {
		if left.Model < right.Model {
			return -1
		}
		return 1
	}
	if left.Group != right.Group {
		if left.Group < right.Group {
			return -1
		}
		return 1
	}
	return 0
}

func probeCandidateUsesPreferredModel(candidate ProbeCandidate) bool {
	if candidate.Channel == nil {
		return false
	}
	return strings.TrimSpace(candidate.Model) == selectProbeModel(candidate.Channel)
}

func (s *ProbeSelector) MarkResult(result ProbeRunResult) {
	if s == nil || !result.Success {
		return
	}
	key := normalizeProbeRuntimeKey(result.AttemptRuntimeKey())
	if key.ChannelID <= 0 {
		return
	}
	s.mu.Lock()
	s.lastOK[key] = s.now()
	s.mu.Unlock()
}

func (s *ProbeSelector) lowHealthCandidatesLocked(channelByID map[int]*model.Channel, recent probeRecentScopes, now time.Time, config ProbeConfig) []ProbeCandidate {
	if s.store == nil {
		return nil
	}
	snapshots := s.store.ListCandidates(nil)
	candidates := make([]ProbeCandidate, 0, len(snapshots))
	for _, snapshot := range snapshots {
		key := normalizeProbeRuntimeKey(snapshot.Key)
		if key.ChannelID <= 0 || !recent.Contains(key.RequestedModel, key.Group) {
			continue
		}
		snapshot.Key = key
		if configErrorIsolatedSnapshot(snapshot) || configErrorIsolatedRuntimeKey(key) {
			continue
		}
		channel := channelByID[key.ChannelID]
		if channel == nil || !probeRuntimeKeyModelSupported(key) || !probeChannelSupportsKey(channel, key) {
			continue
		}
		reason, score := s.probeReasonForSnapshotLocked(snapshot, now, config)
		if reason == "" {
			continue
		}
		if !s.probeIntervalPassedLocked(key, snapshot, now, config) {
			continue
		}
		candidates = append(candidates, probeCandidateFromSnapshot(channel, snapshot, reason, score))
	}
	return candidates
}

func (s *ProbeSelector) lowTrafficCandidatesLocked(channelByID map[int]*model.Channel, recent probeRecentScopes, now time.Time, config ProbeConfig) []ProbeCandidate {
	candidates := make([]ProbeCandidate, 0)
	for _, channel := range channelByID {
		for _, group := range channel.GetGroups() {
			group = strings.TrimSpace(group)
			if group == "" || !recent.HasGroup(group) {
				continue
			}
			for _, modelName := range channel.GetModels() {
				modelName = strings.TrimSpace(modelName)
				if !recent.Contains(modelName, group) || !probeModelSupported(modelName) {
					continue
				}
				key := probeRuntimeKeyForChannel(channel, modelName, group, endpointTypeForProbe(channel, modelName), core.RuntimeKey{})
				if !probeChannelSupportsKey(channel, key) {
					continue
				}
				snapshot, ok := s.snapshotForKey(key)
				if configErrorIsolatedRuntimeKey(key) || (ok && configErrorIsolatedSnapshot(snapshot)) {
					continue
				}
				if !lowTrafficProbeNeeded(snapshot, ok, now, config) {
					continue
				}
				if !s.probeIntervalPassedLocked(key, snapshot, now, config) {
					continue
				}
				score := 55.0
				if !ok || snapshot.SampleCount <= 0 {
					score = 58
				}
				candidates = append(candidates, ProbeCandidate{
					Channel: channel,
					Model:   modelName,
					Group:   group,
					Key:     key,
					Reason:  reasonLowTraffic,
					Score:   score,
				})
			}
		}
	}
	return candidates
}

func (s *ProbeSelector) snapshotForKey(key core.RuntimeKey) (core.RuntimeSnapshot, bool) {
	if s.store == nil {
		return core.RuntimeSnapshot{}, false
	}
	key = normalizeProbeRuntimeKey(key)
	if snapshot, ok := s.store.Get(key); ok {
		return snapshot, true
	}
	if key.CapabilityFingerprint == "" {
		return core.RuntimeSnapshot{}, false
	}
	legacyKey := key
	legacyKey.CapabilityFingerprint = ""
	return s.store.Get(legacyKey)
}

func (s *ProbeSelector) probeReasonForSnapshotLocked(snapshot core.RuntimeSnapshot, now time.Time, config ProbeConfig) (string, float64) {
	key := normalizeProbeRuntimeKey(snapshot.Key)
	reason := ""
	score := 0.0
	if s.breaker != nil {
		circuit := s.breaker.Snapshot(key)
		if circuit.State == core.CircuitStateHalfOpen {
			if s.breaker.AllowProbe(key) {
				reason = reasonCircuitProbe
				score = 100
			} else {
				return "", 0
			}
		}
	}
	if reason == "" {
		if config.FailureAvoidancePriorityEnabled && snapshot.FailureAvoidance {
			return reasonFailureAvoidance, 96
		}
		if snapshot.Cooldown {
			return reasonCooldown, 92
		}
		reason, score = probeReason(snapshot, true, now, s.lastOKTimeLocked(key, snapshot), config)
	}
	return reason, score
}

func (s *ProbeSelector) markProbeSelectionLocked(key core.RuntimeKey, reason string, config ProbeConfig) {
	if s == nil || s.store == nil {
		return
	}
	snapshot, ok := s.store.Get(key)
	if !ok {
		return
	}
	snapshot = normalizeProbeSelectionSnapshot(snapshot)
	snapshot.HealthScoreAverage = effectiveProbeHealthScore(snapshot)
	snapshot.ProbeTriggerReason = reason
	snapshot.ProbeRecoveryRequired = config.RecoverySuccessesRequired
	snapshot.ProbeRecoveryPending = snapshot.FailureAvoidance || reason == reasonLowScore || reason == reasonFailureAvoidance
	s.store.Put(snapshot)
}

func normalizeProbeSelectionSnapshot(snapshot core.RuntimeSnapshot) core.RuntimeSnapshot {
	snapshot.Key = normalizeProbeRuntimeKey(snapshot.Key)
	snapshot.MatchedRuntimeKey = normalizeProbeRuntimeKey(snapshot.MatchedRuntimeKey)
	return snapshot
}

func (s *ProbeSelector) probeIntervalPassedLocked(key core.RuntimeKey, snapshot core.RuntimeSnapshot, now time.Time, config ProbeConfig) bool {
	if last := s.lastProbe[key]; !last.IsZero() && now.Sub(last) < config.MinChannelInterval {
		return false
	}
	if snapshot.LastProbeAt > 0 && now.Unix()-snapshot.LastProbeAt < int64(config.MinChannelInterval.Seconds()) {
		return false
	}
	return true
}

func (s *ProbeSelector) lastOKTimeLocked(key core.RuntimeKey, snapshot core.RuntimeSnapshot) time.Time {
	if last := s.lastOK[key]; !last.IsZero() {
		return last
	}
	if snapshot.LastProbeSuccessAt > 0 {
		return time.Unix(snapshot.LastProbeSuccessAt, 0)
	}
	if snapshot.LastRealSuccessAt > 0 {
		return time.Unix(snapshot.LastRealSuccessAt, 0)
	}
	return time.Time{}
}

func probeReason(snapshot core.RuntimeSnapshot, ok bool, now time.Time, lastOK time.Time, config ProbeConfig) (string, float64) {
	if !ok || snapshot.SampleCount <= 0 {
		return reasonNoSamples, 58
	}
	healthScore := effectiveProbeHealthScore(snapshot)
	if healthScore > 0 && healthScore < config.LowScoreThreshold {
		return reasonLowScore, 70 + (config.LowScoreThreshold-healthScore)*20
	}
	if lastOK.IsZero() || now.Sub(lastOK) >= config.LongNoSuccessThreshold {
		if snapshot.SuccessRate < 0.99 || snapshot.SuccessScore < 0.99 {
			return reasonLongNoSuccess, 60
		}
	}
	return "", 0
}

func probeReasonPriority(reason string) int {
	switch strings.TrimSpace(reason) {
	case reasonCircuitProbe:
		return 1
	case reasonFailureAvoidance:
		return 2
	case reasonCooldown:
		return 3
	case reasonLowScore:
		return 4
	case reasonLongNoSuccess:
		return 5
	case reasonNoSamples:
		return 6
	case reasonLowTraffic:
		return 7
	case reasonSampling:
		return 8
	default:
		return 99
	}
}

func probeCandidateFromSnapshot(channel *model.Channel, snapshot core.RuntimeSnapshot, reason string, score float64) ProbeCandidate {
	key := normalizeProbeRuntimeKey(snapshot.Key)
	modelName := strings.TrimSpace(key.RequestedModel)
	if modelName == "" {
		modelName = strings.TrimSpace(key.UpstreamModel)
	}
	key = probeRuntimeKeyForChannel(channel, modelName, key.Group, key.EndpointType, key)
	return ProbeCandidate{
		Channel: channel,
		Model:   modelName,
		Group:   key.Group,
		Key:     key,
		Reason:  reason,
		Score:   score,
	}
}

func lowTrafficProbeNeeded(snapshot core.RuntimeSnapshot, ok bool, now time.Time, config ProbeConfig) bool {
	if !ok || snapshot.SampleCount <= 0 {
		return true
	}
	if snapshot.LastRealAttemptAt <= 0 || snapshot.LastRealAttemptAt < now.Add(-probeActivationWindow).Unix() {
		return true
	}
	if snapshot.RealSampleCount30m <= 0 {
		return true
	}
	return snapshot.SampleCount < config.MissingSampleThreshold
}

func eligibleProbeChannels(channels []*model.Channel) map[int]*model.Channel {
	result := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		if !probeChannelEligible(channel) {
			continue
		}
		result[channel.Id] = channel
	}
	return result
}

func recentProbeScopes(now time.Time) (probeRecentScopes, error) {
	scopes := probeRecentScopes{
		pairs:  map[string]struct{}{},
		groups: map[string]struct{}{},
	}
	if model.DB == nil {
		return scopes, nil
	}
	cutoff := now.Add(-probeActivationWindow).Unix()
	rows := make([]probeRecentRequestScopeRow, 0)
	err := model.DB.Model(&model.ModelGatewayUserRequestSummary{}).
		Select("requested_model, requested_group, selected_group").
		Where("completed_at >= ? AND is_health_probe = ?", cutoff, false).
		Find(&rows).Error
	if err != nil {
		return scopes, err
	}
	for _, row := range rows {
		modelName := strings.TrimSpace(row.RequestedModel)
		if !probeModelSupported(modelName) {
			continue
		}
		for _, group := range []string{row.SelectedGroup, row.RequestedGroup} {
			group = strings.TrimSpace(group)
			if group == "" || group == "auto" {
				continue
			}
			scopes.groups[group] = struct{}{}
			scopes.pairs[probeScopeKey(modelName, group)] = struct{}{}
		}
	}
	return scopes, nil
}

func (s probeRecentScopes) Empty() bool {
	return len(s.pairs) == 0
}

func (s probeRecentScopes) Contains(modelName string, group string) bool {
	_, ok := s.pairs[probeScopeKey(modelName, group)]
	return ok
}

func (s probeRecentScopes) HasGroup(group string) bool {
	_, ok := s.groups[strings.TrimSpace(group)]
	return ok
}

func probeScopeKey(modelName string, group string) string {
	return strings.TrimSpace(modelName) + "\x00" + strings.TrimSpace(group)
}

func probeChannelEligible(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	switch channel.Status {
	case common.ChannelStatusEnabled:
	case common.ChannelStatusAutoDisabled:
		if configErrorIsolatedChannel(channel) {
			return false
		}
		if !service.IsErrorPausedChannel(channel) || !service.ShouldResumeErrorPausedChannel(channel, nil) {
			return false
		}
	default:
		return false
	}
	if configErrorIsolatedChannel(channel) {
		return false
	}
	if service.IsConfirmedBalanceInsufficientChannel(channel) || service.IsRuntimeBalanceInsufficientChannel(channel) {
		return false
	}
	return true
}

func configErrorIsolatedChannel(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return configErrorIsolatedInfo(channel.GetOtherInfo())
}

func configErrorIsolatedSnapshot(snapshot core.RuntimeSnapshot) bool {
	return snapshot.ConfigErrorIsolated
}

func configErrorIsolatedRuntimeKey(key core.RuntimeKey) bool {
	key = normalizeProbeRuntimeKey(key)
	if key.ChannelID <= 0 || key.RequestedModel == "" || key.Group == "" {
		return false
	}
	return service.IsChannelConfigIsolated(service.NewChannelConfigIsolationKey(
		key.ChannelID,
		key.RequestedModel,
		key.Group,
		key.EndpointType,
	))
}

func configErrorIsolatedInfo(info map[string]any) bool {
	if value, ok := info["config_error_isolated"].(bool); ok && value {
		return true
	}
	if value, ok := info["config_error_isolated"].(string); ok && strings.EqualFold(strings.TrimSpace(value), "true") {
		return true
	}
	reason, _ := info["isolation_reason"].(string)
	return strings.TrimSpace(reason) == core.ErrorCategoryAuthConfigError
}

func selectProbeModel(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	if channel.TestModel != nil {
		if modelName := strings.TrimSpace(*channel.TestModel); probeModelSupported(modelName) {
			return modelName
		}
	}
	for _, modelName := range channel.GetModels() {
		modelName = strings.TrimSpace(modelName)
		if probeModelSupported(modelName) {
			return modelName
		}
	}
	return ""
}

func probeModelSupported(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if modelName == "" {
		return false
	}
	for _, keyword := range []string{
		"embedding", "embed", "rerank", "whisper", "tts", "image", "video", "suno", "midjourney", "mj-",
	} {
		if strings.Contains(modelName, keyword) {
			return false
		}
	}
	return true
}

func probeRuntimeKeyModelSupported(key core.RuntimeKey) bool {
	modelName := strings.TrimSpace(key.RequestedModel)
	if modelName == "" {
		modelName = strings.TrimSpace(key.UpstreamModel)
	}
	return probeModelSupported(modelName)
}

func probeChannelSupportsKey(channel *model.Channel, key core.RuntimeKey) bool {
	if channel == nil || key.ChannelID <= 0 || channel.Id != key.ChannelID {
		return false
	}
	modelName := strings.TrimSpace(key.RequestedModel)
	if modelName == "" {
		modelName = strings.TrimSpace(key.UpstreamModel)
	}
	group := strings.TrimSpace(key.Group)
	if modelName == "" || group == "" {
		return false
	}
	return model.IsChannelEnabledForGroupModel(group, modelName, channel.Id)
}

func effectiveProbeHealthScore(snapshot core.RuntimeSnapshot) float64 {
	scores := []float64{}
	if snapshot.SuccessScore > 0 {
		scores = append(scores, snapshot.SuccessScore)
	}
	if snapshot.SpeedScore > 0 {
		scores = append(scores, snapshot.SpeedScore)
	}
	if snapshot.ExperienceScore > 0 {
		scores = append(scores, snapshot.ExperienceScore)
	}
	if len(scores) == 0 {
		return 0
	}
	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
}

func normalizeProbeConfig(config ProbeConfig) ProbeConfig {
	if config.Interval <= 0 {
		config.Interval = time.Minute
	}
	if config.WorkerCount <= 0 {
		config.WorkerCount = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 8 * time.Second
	}
	if config.MaxPerTick <= 0 {
		config.MaxPerTick = 5
	}
	if config.MinChannelInterval <= 0 {
		config.MinChannelInterval = 5 * time.Minute
	}
	if config.LowScoreThreshold <= 0 {
		config.LowScoreThreshold = 0.62
	}
	if config.MissingSampleThreshold <= 0 {
		config.MissingSampleThreshold = 3
	}
	if config.LongNoSuccessThreshold <= 0 {
		config.LongNoSuccessThreshold = 30 * time.Minute
	}
	if config.RecoverySuccessesRequired <= 0 {
		config.RecoverySuccessesRequired = 2
	}
	if config.HighScoreSamplingInterval <= 0 {
		config.HighScoreSamplingInterval = 6 * time.Hour
	}
	return config
}

func runtimeKeyEndpointType(key core.RuntimeKey) constant.EndpointType {
	if key.EndpointType == "" {
		return constant.EndpointTypeOpenAI
	}
	return key.EndpointType
}

func normalizeProbeRuntimeKey(key core.RuntimeKey) core.RuntimeKey {
	key.RequestedModel = strings.TrimSpace(key.RequestedModel)
	key.UpstreamModel = strings.TrimSpace(key.UpstreamModel)
	key.Group = strings.TrimSpace(key.Group)
	key.CapabilityFingerprint = strings.TrimSpace(key.CapabilityFingerprint)
	key.EndpointType = runtimeKeyEndpointType(key)
	return key
}

func probeRuntimeKeyForChannel(channel *model.Channel, requestedModel string, group string, endpointType constant.EndpointType, seed core.RuntimeKey) core.RuntimeKey {
	key := seed
	requestedModel = firstProbeString(key.RequestedModel, requestedModel, key.UpstreamModel)
	if channel != nil && key.ChannelID <= 0 {
		key.ChannelID = channel.Id
	}
	if strings.TrimSpace(key.RequestedModel) == "" {
		key.RequestedModel = requestedModel
	}
	if strings.TrimSpace(key.UpstreamModel) == "" {
		if channel != nil && requestedModel != "" {
			key.UpstreamModel = channel.ResolveMappedModelName(requestedModel)
		} else {
			key.UpstreamModel = strings.TrimSpace(seed.UpstreamModel)
		}
	}
	if strings.TrimSpace(key.Group) == "" {
		key.Group = strings.TrimSpace(group)
	}
	if key.EndpointType == "" {
		key.EndpointType = endpointType
	}
	if key.EndpointType == "" && channel != nil {
		key.EndpointType = endpointTypeForProbe(channel, requestedModel)
	}
	if channel != nil {
		profile := probeProviderProfile(channel, requestedModel)
		capability := profile.Capabilities(channel, requestedModel)
		if strings.TrimSpace(key.CapabilityFingerprint) == "" {
			key.CapabilityFingerprint = capability.CapabilityFingerprint
		}
		key.CapabilityFingerprint = appendProbeCapabilityPart(key.CapabilityFingerprint, profile.Name())
		key.CapabilityFingerprint = appendProbeCapabilityPart(key.CapabilityFingerprint, profile.ProxyMode(channel, requestedModel))
	}
	return normalizeProbeRuntimeKey(key)
}

func probeProviderProfile(channel *model.Channel, modelName string) modelgatewayprovider.ProviderProfile {
	registry := modelgatewayprovider.NewStandardProviderRegistry()
	if profile := registry.Best(channel, modelName); profile != nil {
		return profile
	}
	return modelgatewayprovider.NewStandardOpenAICompatibleProfile()
}

func appendProbeCapabilityPart(fingerprint string, part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return strings.TrimSpace(fingerprint)
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return part
	}
	parts := strings.Split(fingerprint, "|")
	for _, existing := range parts {
		if strings.TrimSpace(existing) == part {
			return fingerprint
		}
	}
	parts = append(parts, part)
	return strings.Join(parts, "|")
}
