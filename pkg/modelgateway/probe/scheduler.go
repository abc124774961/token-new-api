package probe

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/recording"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type ProbeScheduler struct {
	config        ProbeConfig
	selector      *ProbeSelector
	executor      *ProbeExecutor
	recorder      *recording.AsyncExecutionRecorder
	snapshotStore core.RuntimeSnapshotStore
	scorerFactory core.ScoreCalculatorFactory
	stop          chan struct{}
	once          sync.Once
}

var (
	defaultProbeSchedulerMu sync.Mutex
	defaultProbeScheduler   *ProbeScheduler
)

func NewProbeScheduler(config ProbeConfig, selector *ProbeSelector, executor *ProbeExecutor, recorder *recording.AsyncExecutionRecorder) *ProbeScheduler {
	config = normalizeProbeConfig(config)
	if executor == nil {
		executor = NewProbeExecutor(config.Timeout, NewProbeBillingRecorder())
	}
	return &ProbeScheduler{
		config:        config,
		selector:      selector,
		executor:      executor,
		recorder:      recorder,
		scorerFactory: scheduler.NewScoreCalculatorFactory(modelgatewayintegration.RuntimePolicySetting().ScoreWeights),
		stop:          make(chan struct{}),
	}
}

func (s *ProbeScheduler) WithSnapshotStore(store core.RuntimeSnapshotStore) *ProbeScheduler {
	if s == nil {
		return nil
	}
	s.snapshotStore = store
	return s
}

func (s *ProbeScheduler) WithScoreCalculatorFactory(factory core.ScoreCalculatorFactory) *ProbeScheduler {
	if s == nil {
		return nil
	}
	s.scorerFactory = factory
	return s
}

func (s *ProbeScheduler) Start(ctx context.Context) {
	if s == nil || !s.config.Enabled {
		return
	}
	if !common.IsMasterNode {
		common.SysLog("model gateway probe scheduler skipped on non-master node")
		return
	}
	s.once.Do(func() {
		go s.run(ctx)
	})
}

func (s *ProbeScheduler) Stop() {
	if s == nil || s.stop == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

func (s *ProbeScheduler) run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()
	common.SysLog(fmt.Sprintf("model gateway probe scheduler started: interval=%s workers=%d max_per_tick=%d timeout=%s",
		s.config.Interval, s.config.WorkerCount, s.config.MaxPerTick, s.config.Timeout))
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *ProbeScheduler) tick(ctx context.Context) {
	if s == nil || s.selector == nil || s.executor == nil {
		return
	}
	active, err := recentRealUserTrafficActive()
	if err != nil {
		common.SysLog(fmt.Sprintf("model gateway probe traffic gate failed: %v", err))
		return
	}
	if !active {
		return
	}
	candidates, err := s.selector.Select(s.config)
	if err != nil {
		common.SysLog(fmt.Sprintf("model gateway probe select failed: %v", err))
		return
	}
	if len(candidates) == 0 {
		return
	}
	jobs := make(chan ProbeCandidate)
	var wg sync.WaitGroup
	workers := s.config.WorkerCount
	if workers <= 0 {
		workers = 1
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				candidate.Plan = s.buildDispatchPlan(candidate)
				result := s.executor.Execute(ctx, candidate)
				s.selector.MarkResult(result)
				if s.recorder != nil {
					s.recorder.Record(context.Background(), result.DispatchRecord())
					s.recorder.Report(context.Background(), result.AttemptResult())
				}
				logProbeResult(result)
			}
		}()
	}
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- candidate:
		}
	}
	close(jobs)
	wg.Wait()
}

func SyncDefaultProbeSchedulerLifecycle() *ProbeScheduler {
	defaultProbeSchedulerMu.Lock()
	defer defaultProbeSchedulerMu.Unlock()
	stopDefaultProbeSchedulerLocked()
	setting := scheduler_setting.GetSetting()
	config := ProbeConfig{
		Enabled:                         setting.ProbeEnabled,
		Interval:                        time.Duration(setting.ProbeIntervalSeconds) * time.Second,
		WorkerCount:                     setting.ProbeWorkerCount,
		Timeout:                         time.Duration(setting.ProbeTimeoutSeconds) * time.Second,
		MaxPerTick:                      setting.ProbeMaxPerTick,
		MinChannelInterval:              time.Duration(setting.ProbeMinChannelIntervalSeconds) * time.Second,
		LowScoreThreshold:               setting.ProbeLowScoreThreshold,
		MissingSampleThreshold:          setting.ProbeMissingSampleThreshold,
		LongNoSuccessThreshold:          time.Duration(setting.ProbeLongNoSuccessSeconds) * time.Second,
		RecoverySuccessesRequired:       setting.ProbeRecoverySuccessesRequired,
		FailureAvoidancePriorityEnabled: setting.ProbeFailureAvoidancePriorityEnabled,
	}
	if !config.Enabled {
		return nil
	}
	if relayInvoker == nil {
		common.SysLog("model gateway probe scheduler skipped: relay invoker is not registered")
		return nil
	}
	deps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	if deps == nil {
		return nil
	}
	healthMonitor := scheduler.NewRuntimeHealthMonitor(deps.SnapshotStore, deps.CircuitBreaker)
	recorder := recording.NewAsyncExecutionRecorder(256).WithPostProcessors(healthMonitor)
	selector := NewProbeSelector(deps.SnapshotStore, deps.CircuitBreaker)
	executor := NewProbeExecutor(config.Timeout, NewProbeBillingRecorder())
	s := NewProbeScheduler(config, selector, executor, recorder).
		WithSnapshotStore(deps.SnapshotStore)
	s.Start(context.Background())
	defaultProbeScheduler = s
	return s
}

func StartDefaultProbeScheduler() *ProbeScheduler {
	return SyncDefaultProbeSchedulerLifecycle()
}

func StopDefaultProbeScheduler() {
	defaultProbeSchedulerMu.Lock()
	defer defaultProbeSchedulerMu.Unlock()
	stopDefaultProbeSchedulerLocked()
}

func stopDefaultProbeSchedulerLocked() {
	if defaultProbeScheduler == nil {
		return
	}
	defaultProbeScheduler.Stop()
	defaultProbeScheduler = nil
}

func logProbeResult(result ProbeRunResult) {
	channelID := 0
	if result.Channel != nil {
		channelID = result.Channel.Id
	}
	if result.Success {
		common.SysLog(fmt.Sprintf("model gateway probe success: probe_id=%s channel_id=%d model=%s reason=%s quota=%d latency_ms=%d",
			result.ProbeID, channelID, result.Model, result.Reason, result.Quota, result.Duration.Milliseconds()))
		return
	}
	errText := ""
	if result.NewAPIError != nil {
		errText = result.NewAPIError.ErrorWithStatusCode()
	} else if result.Err != nil {
		errText = result.Err.Error()
	}
	common.SysLog(fmt.Sprintf("model gateway probe failed: probe_id=%s channel_id=%d model=%s reason=%s error=%s",
		result.ProbeID, channelID, result.Model, result.Reason, common.MaskSensitiveInfo(errText)))
}

func recentRealUserTrafficActive() (bool, error) {
	if model.DB == nil {
		return false, nil
	}
	cutoff := time.Now().Add(-probeActivationWindow).Unix()
	var count int64
	err := model.DB.Model(&model.ModelGatewayUserRequestSummary{}).
		Where("completed_at >= ? AND is_health_probe = ?", cutoff, false).
		Count(&count).Error
	return count > 0, err
}

func (s *ProbeScheduler) buildDispatchPlan(candidate ProbeCandidate) *core.DispatchPlan {
	endpointType := probeEndpointType(candidate.Channel, candidate.Model, candidate.Key.EndpointType)
	result := ProbeRunResult{
		Reason:     candidate.Reason,
		Channel:    candidate.Channel,
		Model:      candidate.Model,
		Group:      candidate.Group,
		RuntimeKey: candidate.Key,
		TargetKey:  candidate.Key,
	}
	plan := buildProbeDispatchPlan(result, endpointType)
	if plan == nil {
		return nil
	}
	policy := probeDispatchPolicy(candidate.Group)
	plan.PolicyMode = policy.Mode
	plan.AutoMode = policy.AutoMode
	plan.SelectedReason = "health_probe_" + strings.TrimSpace(candidate.Reason)
	plan.IsHealthProbe = true
	plan.ProbeReason = strings.TrimSpace(candidate.Reason)
	s.applyProbeScore(plan, candidate, policy)
	return plan
}

func (s *ProbeScheduler) applyProbeScore(plan *core.DispatchPlan, candidate ProbeCandidate, policy core.GroupSmartPolicy) {
	if plan == nil || candidate.Channel == nil {
		return
	}
	snapshot := core.RuntimeSnapshot{
		Key:                plan.RuntimeKey,
		CostRatio:          0,
		GroupPriorityRatio: 1,
		SampleSource:       "none",
	}
	if s != nil && s.snapshotStore != nil {
		if stored, ok := s.snapshotStore.Get(plan.RuntimeKey); ok {
			snapshot = stored
			snapshot.SampleSource = "exact"
			snapshot.MatchedRuntimeKey = stored.Key
		} else if candidate.Key.ChannelID > 0 {
			if stored, ok := s.snapshotStore.Get(candidate.Key); ok {
				snapshot = stored
				snapshot.SampleSource = "legacy_exact"
				snapshot.MatchedRuntimeKey = stored.Key
			}
		}
	}
	snapshot.Key = plan.RuntimeKey
	if snapshot.GroupPriorityRatio <= 0 {
		snapshot.GroupPriorityRatio = 1
	}
	if snapshot.HealthScoreAverage <= 0 {
		snapshot.HealthScoreAverage = scheduler.HealthScoreAverage(snapshot)
	}
	snapshot.ProbeTriggerReason = strings.TrimSpace(candidate.Reason)
	if snapshot.ProbeRecoveryRequired <= 0 {
		snapshot.ProbeRecoveryRequired = normalizeProbeConfig(s.config).RecoverySuccessesRequired
	}
	if candidate.Reason == reasonLowScore || candidate.Reason == reasonFailureAvoidance || snapshot.FailureAvoidance {
		snapshot.ProbeRecoveryPending = true
	}

	probeCandidate := core.Candidate{
		Channel:         candidate.Channel,
		Group:           plan.SelectedGroup,
		UpstreamModel:   plan.RuntimeKey.UpstreamModel,
		ProviderProfile: plan.ProviderProfile,
		ProxyMode:       plan.ProxyMode,
		RuntimeKey:      plan.RuntimeKey,
	}
	scorerFactory := s.scorerFactory
	if scorerFactory == nil {
		scorerFactory = scheduler.NewScoreCalculatorFactory(modelgatewayintegration.RuntimePolicySetting().ScoreWeights)
	}
	score := scorerFactory.ForStrategy(policy.Strategy).Score(probeCandidate, snapshot, policy)
	plan.ScoreTotal = score.Total
	plan.ScoreBreakdown = score.Breakdown
	plan.RoutingScoreTotal = score.RoutingTotal
	plan.RoutingScoreBreakdown = score.RoutingBreakdown
	plan.Candidates = []core.CandidateExplanation{
		probeCandidateExplanation(probeCandidate, snapshot, score, candidate.Reason),
	}
}

func probeDispatchPolicy(group string) core.GroupSmartPolicy {
	setting := scheduler_setting.GetSetting()
	strategy := strings.TrimSpace(setting.DefaultStrategy)
	if strategy == "" {
		strategy = core.StrategyBalanced
	}
	policy := core.GroupSmartPolicy{
		RequestedGroup:        strings.TrimSpace(group),
		UserGroup:             strings.TrimSpace(group),
		Mode:                  core.ModeActive,
		Strategy:              strategy,
		AutoMode:              core.AutoModeSequential,
		CandidateGroups:       []string{strings.TrimSpace(group)},
		QueueEnabled:          setting.QueueEnabled,
		QueuePriority:         0,
		CircuitBreakerEnabled: setting.CircuitBreakerEnabled,
		GroupPriorityRatio:    copyProbeGroupPriorityRatio(setting.GroupPriorityRatio),
	}
	if groupPolicy, ok := setting.GroupPolicies[strings.TrimSpace(group)]; ok {
		if strings.TrimSpace(groupPolicy.Strategy) != "" {
			policy.Strategy = strings.TrimSpace(groupPolicy.Strategy)
		}
		if strings.TrimSpace(groupPolicy.AutoMode) != "" {
			policy.AutoMode = strings.TrimSpace(groupPolicy.AutoMode)
		}
		if len(groupPolicy.CandidateGroups) > 0 {
			policy.CandidateGroups = append([]string(nil), groupPolicy.CandidateGroups...)
		}
		policy.QueueEnabled = groupPolicy.QueueEnabled
		policy.QueueHighPriority = groupPolicy.QueueHighPriority
		policy.CircuitBreakerEnabled = groupPolicy.CircuitBreakerEnabled
	}
	if strings.TrimSpace(policy.Strategy) == "" {
		policy.Strategy = core.StrategyBalanced
	}
	return policy
}

func copyProbeGroupPriorityRatio(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for group, ratio := range values {
		group = strings.TrimSpace(group)
		if group != "" && ratio > 0 {
			out[group] = ratio
		}
	}
	return out
}

func probeCandidateExplanation(candidate core.Candidate, snapshot core.RuntimeSnapshot, score core.ScoreResult, reason string) core.CandidateExplanation {
	explanation := core.CandidateExplanation{
		ChannelID:                  snapshot.Key.ChannelID,
		Group:                      firstProbeString(candidate.Group, snapshot.Key.Group),
		UpstreamModel:              firstProbeString(candidate.UpstreamModel, snapshot.Key.UpstreamModel),
		ProviderProfile:            candidate.ProviderProfile,
		ProxyMode:                  candidate.ProxyMode,
		RuntimeKey:                 snapshot.Key,
		Available:                  true,
		Selected:                   true,
		ScoreTotal:                 score.Total,
		ScoreBreakdown:             copyProbeScoreMap(score.Breakdown),
		RoutingScoreTotal:          score.RoutingTotal,
		RoutingScoreBreakdown:      copyProbeScoreMap(score.RoutingBreakdown),
		SuccessRate:                snapshot.SuccessRate,
		TTFTMs:                     snapshot.TTFTMs,
		DurationMs:                 snapshot.DurationMs,
		TokensPerSecond:            snapshot.TokensPerSecond,
		SampleCount:                snapshot.SampleCount,
		ActiveConcurrency:          snapshot.ActiveConcurrency,
		MaxConcurrency:             snapshot.MaxConcurrency,
		ConfiguredConcurrencyLimit: snapshot.ConfiguredConcurrencyLimit,
		LearnedConcurrencyLimit:    snapshot.LearnedConcurrencyLimit,
		EffectiveConcurrencyLimit:  snapshot.EffectiveConcurrencyLimit,
		QueueDepth:                 snapshot.QueueDepth,
		QueueCapacity:              snapshot.QueueCapacity,
		EstimatedQueueWaitMs:       snapshot.EstimatedQueueWaitMs,
		FirstBytePending:           snapshot.FirstBytePending,
		SlowFirstBytePending:       snapshot.SlowFirstBytePending,
		OldestFirstByteWaitMs:      snapshot.OldestFirstByteWaitMs,
		CostRatio:                  snapshot.CostRatio,
		CostReferenceRatio:         snapshot.CostReferenceRatio,
		CostPricingMode:            snapshot.CostPricingMode,
		GroupPriorityRatio:         snapshot.GroupPriorityRatio,
		HealthScoreAverage:         snapshot.HealthScoreAverage,
		ProbeRecoveryPending:       snapshot.ProbeRecoveryPending,
		ProbeRecoverySuccessCount:  snapshot.ProbeRecoverySuccessCount,
		ProbeRecoveryRequired:      snapshot.ProbeRecoveryRequired,
		ProbeTriggerReason:         firstProbeString(reason, snapshot.ProbeTriggerReason),
		ScoreSampleSource:          snapshot.SampleSource,
		MatchedRuntimeKey:          snapshot.MatchedRuntimeKey,
	}
	if candidate.Channel != nil {
		explanation.ChannelID = candidate.Channel.Id
		explanation.ChannelName = candidate.Channel.Name
		explanation.ChannelStatus = candidate.Channel.Status
		explanation.StatusReason = service.ChannelStatusReason(candidate.Channel)
		explanation.BalanceInsufficient = service.IsKnownBalanceInsufficientChannel(candidate.Channel)
	}
	if explanation.ChannelID == 0 {
		explanation.ChannelID = candidate.RuntimeKey.ChannelID
	}
	explanation.SuccessScore = probeScoreValue(score.Breakdown, "success", snapshot.SuccessScore)
	explanation.ScoreSpeedFactor = probeScoreValue(score.Breakdown, "speed", snapshot.SpeedScore)
	explanation.SpeedScore = firstPositiveFloat(snapshot.SpeedScore, explanation.ScoreSpeedFactor)
	explanation.LoadScore = probeScoreValue(score.Breakdown, "load", 0)
	explanation.CostScore = probeScoreValue(score.Breakdown, "cost", 0)
	explanation.GroupScore = probeScoreValue(score.Breakdown, "group", 0)
	explanation.ExperienceScore = probeScoreValue(score.Breakdown, "experience", snapshot.ExperienceScore)
	return explanation
}

func copyProbeScoreMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func probeScoreValue(values map[string]float64, key string, fallback float64) float64 {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

func firstProbeString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (r ProbeRunResult) DispatchRecord() core.DispatchRecord {
	plan := r.Plan
	if plan == nil {
		plan = buildProbeDispatchPlan(r, r.AttemptRuntimeKey().EndpointType)
		if plan != nil {
			plan.IsHealthProbe = true
			plan.ProbeReason = r.Reason
		}
	}
	endpointType := r.AttemptRuntimeKey().EndpointType
	if endpointType == "" {
		endpointType = constant.EndpointTypeOpenAI
	}
	requestedGroup := strings.TrimSpace(r.Group)
	if requestedGroup == "" && plan != nil {
		requestedGroup = strings.TrimSpace(plan.RequestedGroup)
	}
	policy := probeDispatchPolicy(requestedGroup)
	return core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestID:      r.ProbeID,
			RequestedGroup: requestedGroup,
			UserGroup:      requestedGroup,
			ModelName:      r.Model,
			EndpointType:   endpointType,
		},
		Policy:      policy,
		Plan:        plan,
		Actual:      r.Channel,
		ActualGroup: requestedGroup,
		RecordedAt:  r.StartedAt,
	}
}
