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
	"github.com/QuantumNous/new-api/service"
)

type ProbeSelector struct {
	store     core.RuntimeSnapshotStore
	breaker   core.CircuitBreaker
	mu        sync.Mutex
	lastProbe map[int]time.Time
	lastOK    map[int]time.Time
	now       func() time.Time
}

func NewProbeSelector(store core.RuntimeSnapshotStore, breaker core.CircuitBreaker) *ProbeSelector {
	return &ProbeSelector{
		store:     store,
		breaker:   breaker,
		lastProbe: map[int]time.Time{},
		lastOK:    map[int]time.Time{},
		now:       time.Now,
	}
}

func (s *ProbeSelector) Select(config ProbeConfig) ([]ProbeCandidate, error) {
	if s == nil {
		return nil, nil
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, err
	}
	now := s.now()
	config = normalizeProbeConfig(config)
	s.mu.Lock()
	defer s.mu.Unlock()

	candidates := make([]ProbeCandidate, 0, len(channels))
	for _, channel := range channels {
		candidate, ok := s.candidateForChannelLocked(channel, now, config)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Channel.Id < candidates[j].Channel.Id
	})
	if config.MaxPerTick > 0 && len(candidates) > config.MaxPerTick {
		candidates = candidates[:config.MaxPerTick]
	}
	for _, candidate := range candidates {
		s.lastProbe[candidate.Channel.Id] = now
	}
	return candidates, nil
}

func (s *ProbeSelector) MarkResult(result ProbeRunResult) {
	if s == nil || result.Channel == nil || !result.Success {
		return
	}
	s.mu.Lock()
	s.lastOK[result.Channel.Id] = s.now()
	s.mu.Unlock()
}

func (s *ProbeSelector) candidateForChannelLocked(channel *model.Channel, now time.Time, config ProbeConfig) (ProbeCandidate, bool) {
	if !probeChannelEligible(channel) {
		return ProbeCandidate{}, false
	}
	if last := s.lastProbe[channel.Id]; !last.IsZero() && now.Sub(last) < config.MinChannelInterval {
		return ProbeCandidate{}, false
	}
	modelName := selectProbeModel(channel)
	if modelName == "" {
		return ProbeCandidate{}, false
	}
	group := selectProbeGroup(channel)
	key := core.RuntimeKey{
		RequestedModel: modelName,
		UpstreamModel:  modelName,
		ChannelID:      channel.Id,
		Group:          group,
		EndpointType:   endpointTypeForProbe(channel, modelName),
	}
	snapshot, ok := s.bestSnapshot(key, modelName, channel.Id, group)
	reason := ""
	score := 0.0
	if s.breaker != nil {
		circuit := s.breaker.Snapshot(key)
		if circuit.State == core.CircuitStateHalfOpen {
			if s.breaker.AllowProbe(key) {
				reason = reasonCircuitProbe
				score = 100
			} else {
				return ProbeCandidate{}, false
			}
		}
	}
	if reason == "" {
		reason, score = probeReason(snapshot, ok, now, s.lastOK[channel.Id], config)
	}
	if reason == "" {
		return ProbeCandidate{}, false
	}
	if ok {
		key = snapshot.Key
		if key.EndpointType == "" {
			key.EndpointType = endpointTypeForProbe(channel, modelName)
		}
	}
	return ProbeCandidate{
		Channel: channel,
		Model:   modelName,
		Group:   group,
		Key:     key,
		Reason:  reason,
		Score:   score,
	}, true
}

func (s *ProbeSelector) bestSnapshot(key core.RuntimeKey, modelName string, channelID int, group string) (core.RuntimeSnapshot, bool) {
	if s.store == nil {
		return core.RuntimeSnapshot{}, false
	}
	if snapshot, ok := s.store.Get(key); ok {
		return snapshot, true
	}
	snapshots := s.store.ListCandidates(&core.DispatchRequest{ModelName: modelName})
	var best core.RuntimeSnapshot
	found := false
	for _, snapshot := range snapshots {
		if snapshot.Key.ChannelID != channelID {
			continue
		}
		if group != "" && snapshot.Key.Group != "" && snapshot.Key.Group != group {
			continue
		}
		if !found || snapshot.SampleCount > best.SampleCount {
			best = snapshot
			found = true
		}
	}
	return best, found
}

func probeReason(snapshot core.RuntimeSnapshot, ok bool, now time.Time, lastOK time.Time, config ProbeConfig) (string, float64) {
	if !ok || snapshot.SampleCount <= 0 {
		return reasonNoSamples, 90
	}
	if snapshot.SampleCount < config.MissingSampleThreshold {
		return reasonNoSamples, 80 - float64(snapshot.SampleCount)
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
	if config.HighScoreSamplingInterval > 0 {
		if lastOK.IsZero() || now.Sub(lastOK) >= config.HighScoreSamplingInterval {
			return reasonSampling, 10
		}
	}
	return "", 0
}

func probeChannelEligible(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	switch channel.Status {
	case common.ChannelStatusEnabled:
	case common.ChannelStatusAutoDisabled:
		if !service.IsErrorPausedChannel(channel) || !service.ShouldResumeErrorPausedChannel(channel, nil) {
			return false
		}
	default:
		return false
	}
	if service.IsConfirmedBalanceInsufficientChannel(channel) || service.IsRuntimeBalanceInsufficientChannel(channel) {
		return false
	}
	return true
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

func selectProbeGroup(channel *model.Channel) string {
	groups := channel.GetGroups()
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			return group
		}
	}
	return "default"
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
