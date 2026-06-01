package probe

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayprovider "github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
)

const (
	probeActivationWindow          = 30 * time.Minute
	probeDefaultGoodBaselineWindow = 24 * time.Hour
)

type ProbeSelector struct {
	store                core.RuntimeSnapshotStore
	breaker              core.CircuitBreaker
	costBaselineProvider core.CostBaselineProvider
	scoreWeights         core.ScoreWeights
	policyForGroup       func(group string) core.GroupSmartPolicy
	mu                   sync.Mutex
	lastProbe            map[core.RuntimeKey]time.Time
	lastOK               map[core.RuntimeKey]time.Time
	now                  func() time.Time
}

type probeRecentScopes struct {
	pairs                  map[string]struct{}
	groups                 map[string]struct{}
	latestSuccessByKey     map[string]int64
	latestSuccessByChannel map[int]int64
}

func (s *ProbeSelector) WithCostBaselineProvider(provider core.CostBaselineProvider) *ProbeSelector {
	if s == nil {
		return nil
	}
	s.costBaselineProvider = provider
	return s
}

func (s *ProbeSelector) WithScoreWeights(weights core.ScoreWeights) *ProbeSelector {
	if s == nil {
		return nil
	}
	s.scoreWeights = weights
	return s
}

func (s *ProbeSelector) WithPolicyForGroup(resolver func(group string) core.GroupSmartPolicy) *ProbeSelector {
	if s == nil {
		return nil
	}
	s.policyForGroup = resolver
	return s
}

type probeRecentRequestScopeRow struct {
	RequestedModel string
	RequestedGroup string
	SelectedGroup  string
	FinalChannelID int
	FinalSuccess   bool
	CompletedAt    int64
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
		if skipRecentRealRequestProbe(candidate, config, s.store, now) {
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
		reason, triggerItems := s.probeReasonForSnapshotLocked(snapshot, recent, now, config)
		if reason == "" {
			continue
		}
		candidate := probeCandidateFromSnapshot(channel, snapshot, reason)
		candidate.TriggerScoreItems = triggerItems
		candidate.PromptCategories = config.PromptCategories
		if !s.probeIntervalPassedLocked(candidate.Key, snapshot, now, config) {
			continue
		}
		candidates = append(candidates, candidate)
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

func (s *ProbeSelector) probeReasonForSnapshotLocked(snapshot core.RuntimeSnapshot, recent probeRecentScopes, now time.Time, config ProbeConfig) (string, []string) {
	key := normalizeProbeRuntimeKey(snapshot.Key)
	reason := ""
	if s.breaker != nil {
		circuit := s.breaker.Snapshot(key)
		if circuit.State == core.CircuitStateHalfOpen {
			if s.breaker.AllowProbe(key) {
				reason = reasonCircuitProbe
			} else {
				return "", nil
			}
		}
	}
	if reason == "" {
		if config.FailureAvoidancePriorityEnabled && snapshot.FailureAvoidance {
			if strings.TrimSpace(snapshot.ProbeTriggerReason) == reasonTimeoutRecovery {
				return reasonTimeoutRecovery, nil
			}
			return reasonFailureAvoidance, nil
		}
		if snapshot.Cooldown {
			return reasonCooldown, nil
		}
		var triggerItems []string
		reason, triggerItems = s.probeReason(snapshot, true, now, s.lastOKTimeLocked(key, snapshot), config)
		if reason == reasonLongNoSuccess && recent.RealSuccessWithin(key, now, config.LongNoSuccessThreshold) {
			return "", nil
		}
		return reason, triggerItems
	}
	return reason, nil
}

func (s *ProbeSelector) markProbeSelectionLocked(key core.RuntimeKey, reason string, config ProbeConfig) {
	if s == nil || s.store == nil {
		return
	}
	snapshot, ok := s.snapshotForKey(key)
	if !ok {
		return
	}
	snapshot = normalizeProbeSelectionSnapshot(snapshot)
	storedKey := normalizeProbeRuntimeKey(snapshot.Key)
	if storedKey.ChannelID <= 0 {
		storedKey = normalizeProbeRuntimeKey(key)
	}
	snapshot.Key = storedKey
	snapshot.ProbeTriggerReason = reason
	snapshot.ProbeRecoveryRequired = config.RecoverySuccessesRequired
	if reason == reasonTimeoutRecovery {
		snapshot.ProbeRecoveryRequired = config.TimeoutRecoverySuccessesRequired
	}
	snapshot.ProbeRecoveryPending = snapshot.FailureAvoidance || reason == reasonLowScore || reason == reasonFailureAvoidance || reason == reasonTimeoutRecovery || reason == reasonCircuitProbe
	snapshot.LastProbeAt = s.now().Unix()
	s.store.Put(snapshot)
	enrichedKey := normalizeProbeRuntimeKey(key)
	if enrichedKey != storedKey {
		enrichedSnapshot := snapshot
		enrichedSnapshot.Key = enrichedKey
		s.store.Put(enrichedSnapshot)
	}
}

func normalizeProbeSelectionSnapshot(snapshot core.RuntimeSnapshot) core.RuntimeSnapshot {
	snapshot.Key = normalizeProbeRuntimeKey(snapshot.Key)
	snapshot.MatchedRuntimeKey = normalizeProbeRuntimeKey(snapshot.MatchedRuntimeKey)
	return snapshot
}

func (s *ProbeSelector) probeIntervalPassedLocked(key core.RuntimeKey, snapshot core.RuntimeSnapshot, now time.Time, config ProbeConfig) bool {
	for _, probeKey := range probeIntervalKeys(key) {
		if last := s.lastProbe[probeKey]; !last.IsZero() && now.Sub(last) < config.MinChannelInterval {
			return false
		}
	}
	if stored, ok := s.snapshotForKey(key); ok && stored.LastProbeAt > 0 && now.Unix()-stored.LastProbeAt < int64(config.MinChannelInterval.Seconds()) {
		return false
	}
	if snapshot.LastProbeAt > 0 && now.Unix()-snapshot.LastProbeAt < int64(config.MinChannelInterval.Seconds()) {
		return false
	}
	return true
}

func probeIntervalKeys(key core.RuntimeKey) []core.RuntimeKey {
	key = normalizeProbeRuntimeKey(key)
	keys := []core.RuntimeKey{key}
	if key.CapabilityFingerprint != "" {
		legacyKey := key
		legacyKey.CapabilityFingerprint = ""
		keys = append(keys, legacyKey)
	}
	return keys
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

func (s *ProbeSelector) probeReason(snapshot core.RuntimeSnapshot, ok bool, now time.Time, lastOK time.Time, config ProbeConfig) (string, []string) {
	if !ok || snapshot.SampleCount <= 0 {
		return reasonNoSamples, nil
	}
	score := s.probeSnapshotScore(snapshot)
	if items := probeScoreNeedsRecovery(score, config.LowScoreThreshold, config.RecoverableScoreItems); len(items) > 0 {
		if probeGoodBaselineEligible(snapshot, now, config) {
			return reasonLowScore, items
		}
		return "", nil
	}
	if lastOK.IsZero() || now.Sub(lastOK) >= config.LongNoSuccessThreshold {
		if snapshot.SuccessRate < 0.99 {
			return reasonLongNoSuccess, nil
		}
	}
	return "", nil
}

func (s *ProbeSelector) probeSnapshotScore(snapshot core.RuntimeSnapshot) core.ScoreResult {
	key := normalizeProbeRuntimeKey(snapshot.Key)
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	snapshot.Key = key
	policy := probePolicyForGroup(key.Group, snapshot.GroupPriorityRatio, s.policyForGroup)
	return scheduler.NewCandidateScoringService().
		WithCostBaselineProvider(s.costBaselineProvider).
		EvaluatePreparedCandidate(core.Candidate{
			Group:         key.Group,
			UpstreamModel: key.UpstreamModel,
			RuntimeKey:    key,
		}, snapshot, policy, scheduler.ScoringContext{
			RequestedModel:  key.RequestedModel,
			EndpointType:    key.EndpointType,
			CandidateGroups: append([]string(nil), policy.CandidateGroups...),
			Strategy:        policy.Strategy,
			AutoMode:        policy.AutoMode,
			ScoreWeights:    s.scoreWeights,
		}, false).Score
}

func probeScoreNeedsRecovery(score core.ScoreResult, threshold float64, configuredItems []string) []string {
	if threshold <= 0 {
		return nil
	}
	recoverable := probeRecoverableScoreItemSet(configuredItems)
	matched := make([]string, 0)
	seen := map[string]struct{}{}
	checkItems := func(items []core.ScoreItem) {
		for _, item := range items {
			if item.MissingReason != "" || item.Weight <= 0 {
				continue
			}
			if _, ok := recoverable[item.Key]; !ok {
				continue
			}
			if item.Score < threshold {
				if _, exists := seen[item.Key]; exists {
					continue
				}
				seen[item.Key] = struct{}{}
				matched = append(matched, item.Key)
			}
		}
	}
	checkItems(score.Items)
	checkItems(score.RoutingItems)
	return matched
}

func probeRecoverableScoreItemSet(configuredItems []string) map[string]struct{} {
	items := NormalizeRecoverableScoreItems(configuredItems)
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

func probeGoodBaselineEligible(snapshot core.RuntimeSnapshot, now time.Time, config ProbeConfig) bool {
	if !config.GoodBaselineEnabled {
		return true
	}
	minSamples := config.GoodBaselineMinSamples
	if minSamples <= 0 {
		minSamples = config.MissingSampleThreshold
	}
	if minSamples <= 0 {
		minSamples = 3
	}
	if snapshot.SampleCount < minSamples {
		return false
	}
	if snapshot.SuccessRate > 0 && snapshot.SuccessRate < 0.5 {
		return false
	}
	if config.GoodBaselineWindow > 0 && snapshot.LastRealSuccessAt > 0 && !now.IsZero() {
		return now.Unix()-snapshot.LastRealSuccessAt <= int64(config.GoodBaselineWindow.Seconds())
	}
	return true
}

func skipRecentRealRequestProbe(candidate ProbeCandidate, config ProbeConfig, store core.RuntimeSnapshotStore, now time.Time) bool {
	config = normalizeProbeConfig(config)
	if !config.SkipRecentRealRequestEnabled {
		return false
	}
	switch strings.TrimSpace(candidate.Reason) {
	case reasonCircuitProbe, reasonTimeoutRecovery:
		return false
	}
	window := config.RecentRealRequestWindow
	if window <= 0 {
		window = probeActivationWindow
	}
	key := normalizeProbeRuntimeKey(candidate.Key)
	if key.ChannelID <= 0 {
		return false
	}
	cutoff := now.Add(-window).Unix()
	if store != nil {
		if snapshot, ok := store.Get(key); ok && snapshot.LastRealAttemptAt >= cutoff {
			return true
		}
		if key.CapabilityFingerprint != "" {
			legacyKey := key
			legacyKey.CapabilityFingerprint = ""
			if snapshot, ok := store.Get(legacyKey); ok && snapshot.LastRealAttemptAt >= cutoff {
				return true
			}
		}
	}
	return recentRealRequestExists(key, cutoff)
}

func recentRealRequestExists(key core.RuntimeKey, cutoff int64) bool {
	if model.DB == nil || cutoff <= 0 || key.ChannelID <= 0 {
		return false
	}
	query := model.DB.Model(&model.ModelGatewayUserRequestSummary{}).
		Where("completed_at >= ? AND is_health_probe = ? AND final_channel_id = ?", cutoff, false, key.ChannelID)
	if strings.TrimSpace(key.RequestedModel) != "" {
		query = query.Where("requested_model = ?", strings.TrimSpace(key.RequestedModel))
	}
	group := strings.TrimSpace(key.Group)
	if group != "" {
		query = query.Where("(selected_group = ? OR requested_group = ?)", group, group)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func probePolicyForGroup(group string, priorityRatio float64, resolver func(group string) core.GroupSmartPolicy) core.GroupSmartPolicy {
	group = strings.TrimSpace(group)
	var policy core.GroupSmartPolicy
	if resolver != nil {
		policy = resolver(group)
	} else {
		policy = probeDispatchPolicy(group)
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
	if group != "" && policy.GroupPriorityRatio[group] <= 0 {
		if priorityRatio <= 0 {
			priorityRatio = 1
		}
		policy.GroupPriorityRatio[group] = priorityRatio
	}
	return policy
}

func probeReasonPriority(reason string) int {
	switch strings.TrimSpace(reason) {
	case reasonCircuitProbe:
		return 1
	case reasonTimeoutRecovery:
		return 2
	case reasonFailureAvoidance:
		return 3
	case reasonCooldown:
		return 4
	case reasonLowScore:
		return 5
	case reasonLongNoSuccess:
		return 6
	case reasonNoSamples:
		return 7
	case reasonLowTraffic:
		return 8
	case reasonSampling:
		return 9
	default:
		return 99
	}
}

func probeCandidateFromSnapshot(channel *model.Channel, snapshot core.RuntimeSnapshot, reason string) ProbeCandidate {
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
	}
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
		pairs:                  map[string]struct{}{},
		groups:                 map[string]struct{}{},
		latestSuccessByKey:     map[string]int64{},
		latestSuccessByChannel: map[int]int64{},
	}
	if model.DB == nil {
		return scopes, nil
	}
	cutoff := now.Add(-probeActivationWindow).Unix()
	rows := make([]probeRecentRequestScopeRow, 0)
	err := model.DB.Model(&model.ModelGatewayUserRequestSummary{}).
		Select("requested_model, requested_group, selected_group, final_channel_id, final_success, completed_at").
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
			if row.FinalSuccess && row.CompletedAt > 0 {
				scopes.latestSuccessByKey[probeSuccessKey(modelName, group, row.FinalChannelID)] = maxInt64(
					scopes.latestSuccessByKey[probeSuccessKey(modelName, group, row.FinalChannelID)],
					row.CompletedAt,
				)
			}
		}
		if row.FinalSuccess && row.FinalChannelID > 0 && row.CompletedAt > 0 {
			scopes.latestSuccessByChannel[row.FinalChannelID] = maxInt64(
				scopes.latestSuccessByChannel[row.FinalChannelID],
				row.CompletedAt,
			)
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

func (s probeRecentScopes) RealSuccessWithin(key core.RuntimeKey, now time.Time, threshold time.Duration) bool {
	if threshold <= 0 {
		threshold = 30 * time.Minute
	}
	latest := int64(0)
	if key.ChannelID > 0 {
		latest = maxInt64(latest, s.latestSuccessByChannel[key.ChannelID])
	}
	if key.RequestedModel != "" && key.Group != "" && key.ChannelID > 0 {
		latest = maxInt64(latest, s.latestSuccessByKey[probeSuccessKey(key.RequestedModel, key.Group, key.ChannelID)])
	}
	if latest <= 0 {
		return false
	}
	return now.Unix()-latest < int64(threshold.Seconds())
}

func probeScopeKey(modelName string, group string) string {
	return strings.TrimSpace(modelName) + "\x00" + strings.TrimSpace(group)
}

func probeSuccessKey(modelName string, group string, channelID int) string {
	if channelID <= 0 {
		return ""
	}
	return probeScopeKey(modelName, group) + "\x00" + strconv.Itoa(channelID)
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
	if config.TimeoutRecoverySuccessesRequired <= 0 {
		config.TimeoutRecoverySuccessesRequired = config.RecoverySuccessesRequired
	}
	if config.HighScoreSamplingInterval <= 0 {
		config.HighScoreSamplingInterval = 6 * time.Hour
	}
	config.RecoverableScoreItems = NormalizeRecoverableScoreItems(config.RecoverableScoreItems)
	if config.RecentRealRequestWindow <= 0 {
		config.RecentRealRequestWindow = probeActivationWindow
	}
	if config.GoodBaselineMinSamples <= 0 {
		config.GoodBaselineMinSamples = config.MissingSampleThreshold
	}
	if config.GoodBaselineWindow <= 0 {
		config.GoodBaselineWindow = probeDefaultGoodBaselineWindow
	}
	config.PromptCategories = NormalizePromptCategories(config.PromptCategories)
	if !config.PromptLibraryEnabled {
		config.PromptCategories = []string{PromptCategoryShort}
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

func firstProbeString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	if channel != nil {
		key.EndpointType = probeEndpointType(channel, requestedModel, key.EndpointType)
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

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
