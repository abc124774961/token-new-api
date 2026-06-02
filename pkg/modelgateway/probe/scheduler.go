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
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type ProbeScheduler struct {
	config        ProbeConfig
	selector      *ProbeSelector
	executor      *ProbeExecutor
	recorder      *recording.AsyncExecutionRecorder
	snapshotStore core.RuntimeSnapshotStore
	enricher      core.RuntimeSnapshotEnricher
	costBaseline  core.CostBaselineProvider
	scoreWeights  core.ScoreWeights
	stop          chan struct{}
	cancel        context.CancelFunc
	once          sync.Once
	stateMu       sync.RWMutex
	startedAt     int64
	lastTickAt    int64
	nextTickAt    int64
}

var (
	defaultProbeSchedulerMu sync.Mutex
	defaultProbeScheduler   *ProbeScheduler
	immediateProbeRecorder  = recording.NewAsyncExecutionRecorder(64)
)

type SchedulerStatus struct {
	Enabled                bool  `json:"enabled"`
	Running                bool  `json:"running"`
	StartedAt              int64 `json:"started_at,omitempty"`
	LastTickAt             int64 `json:"last_tick_at,omitempty"`
	NextTickAt             int64 `json:"next_tick_at,omitempty"`
	RemainingSeconds       int64 `json:"remaining_seconds,omitempty"`
	ProbeIntervalSeconds   int64 `json:"probe_interval_seconds,omitempty"`
	ProbeWorkerCount       int   `json:"probe_worker_count,omitempty"`
	ProbeMaxPerTick        int   `json:"probe_max_per_tick,omitempty"`
	MasterNode             bool  `json:"master_node"`
	RelayInvokerRegistered bool  `json:"relay_invoker_registered"`
}

func NewProbeScheduler(config ProbeConfig, selector *ProbeSelector, executor *ProbeExecutor, recorder *recording.AsyncExecutionRecorder) *ProbeScheduler {
	config = normalizeProbeConfig(config)
	if executor == nil {
		executor = NewProbeExecutor(config.Timeout, NewProbeBillingRecorder())
	}
	return &ProbeScheduler{
		config:       config,
		selector:     selector,
		executor:     executor,
		recorder:     recorder,
		scoreWeights: modelgatewayintegration.RuntimePolicySetting().ScoreWeights,
		stop:         make(chan struct{}),
	}
}

func (s *ProbeScheduler) WithSnapshotStore(store core.RuntimeSnapshotStore) *ProbeScheduler {
	if s == nil {
		return nil
	}
	s.snapshotStore = store
	return s
}

func (s *ProbeScheduler) WithRuntimeSnapshotEnricher(enricher core.RuntimeSnapshotEnricher) *ProbeScheduler {
	if s == nil {
		return nil
	}
	s.enricher = enricher
	return s
}

func (s *ProbeScheduler) WithCostBaselineProvider(provider core.CostBaselineProvider) *ProbeScheduler {
	if s == nil {
		return nil
	}
	s.costBaseline = provider
	return s
}

func (s *ProbeScheduler) WithScoreWeights(weights core.ScoreWeights) *ProbeScheduler {
	if s == nil {
		return nil
	}
	s.scoreWeights = weights
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
		runCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		go s.run(runCtx)
	})
}

func (s *ProbeScheduler) Stop() {
	if s == nil || s.stop == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
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
	now := time.Now()
	s.markTickSchedule(now, now.Add(s.config.Interval))
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case tickAt := <-ticker.C:
			nextTickAt := tickAt.Add(s.config.Interval)
			for !nextTickAt.After(time.Now()) {
				nextTickAt = nextTickAt.Add(s.config.Interval)
			}
			s.markTickSchedule(time.Now(), nextTickAt)
			s.tick(ctx)
		}
	}
}

func (s *ProbeScheduler) markTickSchedule(tickAt time.Time, nextTickAt time.Time) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.startedAt <= 0 {
		s.startedAt = tickAt.Unix()
	}
	s.lastTickAt = tickAt.Unix()
	s.nextTickAt = nextTickAt.Unix()
}

func (s *ProbeScheduler) Status() SchedulerStatus {
	if s == nil {
		return SchedulerStatus{}
	}
	config := normalizeProbeConfig(s.config)
	status := SchedulerStatus{
		Enabled:              config.Enabled,
		Running:              true,
		ProbeIntervalSeconds: int64(config.Interval.Seconds()),
		ProbeWorkerCount:     config.WorkerCount,
		ProbeMaxPerTick:      config.MaxPerTick,
		MasterNode:           common.IsMasterNode,
	}
	s.stateMu.RLock()
	status.StartedAt = s.startedAt
	status.LastTickAt = s.lastTickAt
	status.NextTickAt = s.nextTickAt
	s.stateMu.RUnlock()
	if status.NextTickAt > 0 {
		status.RemainingSeconds = status.NextTickAt - common.GetTimestamp()
		if status.RemainingSeconds < 0 {
			status.RemainingSeconds = 0
		}
	}
	return status
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
				if skipRecentRealRequestProbe(candidate, s.config, s.snapshotStore, time.Now()) {
					logProbeSkipped(candidate, reasonRecentRealRequest)
					continue
				}
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
		Enabled:                          setting.ProbeEnabled,
		Interval:                         time.Duration(setting.ProbeIntervalSeconds) * time.Second,
		WorkerCount:                      setting.ProbeWorkerCount,
		Timeout:                          time.Duration(setting.ProbeTimeoutSeconds) * time.Second,
		MaxPerTick:                       setting.ProbeMaxPerTick,
		MinChannelInterval:               time.Duration(setting.ProbeMinChannelIntervalSeconds) * time.Second,
		LowScoreThreshold:                setting.ProbeLowScoreThreshold,
		MissingSampleThreshold:           setting.ProbeMissingSampleThreshold,
		LongNoSuccessThreshold:           time.Duration(setting.ProbeLongNoSuccessSeconds) * time.Second,
		RecoverySuccessesRequired:        setting.ProbeRecoverySuccessesRequired,
		TimeoutRecoverySuccessesRequired: setting.ChannelTimeoutRecoveryProbeSuccesses,
		FirstByteTimeout:                 20 * time.Second,
		TotalTimeout:                     time.Duration(setting.RelayTotalTimeoutSeconds) * time.Second,
		FailureAvoidancePriorityEnabled:  setting.ProbeFailureAvoidancePriorityEnabled,
		RecoverableScoreItems:            setting.ProbeRecoverableScoreItems,
		SkipRecentRealRequestEnabled:     setting.ProbeSkipRecentRealRequestEnabled,
		RecentRealRequestWindow:          time.Duration(setting.ProbeRecentRealRequestWindowSeconds) * time.Second,
		GoodBaselineEnabled:              setting.ProbeGoodBaselineEnabled,
		GoodBaselineMinSamples:           setting.ProbeGoodBaselineMinSamples,
		GoodBaselineWindow:               time.Duration(setting.ProbeGoodBaselineWindowSeconds) * time.Second,
		PromptLibraryEnabled:             setting.ProbePromptLibraryEnabled,
		PromptCategories:                 setting.ProbePromptCategories,
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
	healthMonitor := scheduler.NewRuntimeHealthMonitor(deps.SnapshotStore, deps.CircuitBreaker).
		WithScoringService(scheduler.NewCandidateScoringService().WithCostBaselineProvider(deps.CostBaselineCache)).
		WithScoreWeights(modelgatewayintegration.RuntimePolicySetting().ScoreWeights)
	recorder := recording.NewAsyncExecutionRecorder(256).WithPostProcessors(healthMonitor)
	runtimePolicy := modelgatewayintegration.RuntimePolicySetting()
	selector := NewProbeSelector(deps.SnapshotStore, deps.CircuitBreaker).
		WithCostBaselineProvider(deps.CostBaselineCache).
		WithScoreWeights(runtimePolicy.ScoreWeights).
		WithPolicyForGroup(probeDispatchPolicy)
	executor := NewProbeExecutor(config.Timeout, NewProbeBillingRecorder()).
		WithTimeoutRecoveryThresholds(config.FirstByteTimeout, config.TotalTimeout)
	s := NewProbeScheduler(config, selector, executor, recorder).
		WithSnapshotStore(deps.SnapshotStore).
		WithRuntimeSnapshotEnricher(deps.RuntimeEnricher).
		WithCostBaselineProvider(deps.CostBaselineCache).
		WithScoreWeights(runtimePolicy.ScoreWeights)
	s.Start(context.Background())
	defaultProbeScheduler = s
	return s
}

func StartDefaultProbeScheduler() *ProbeScheduler {
	return SyncDefaultProbeSchedulerLifecycle()
}

type ImmediateProbeOptions struct {
	Channel           *model.Channel
	RuntimeKey        core.RuntimeKey
	Model             string
	Group             string
	Reason            string
	TriggerScoreItems []string
}

func RunImmediateProbe(ctx context.Context, options ImmediateProbeOptions) (ProbeRunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	channel := options.Channel
	if channel == nil && options.RuntimeKey.ChannelID > 0 {
		loaded, err := model.CacheGetChannel(options.RuntimeKey.ChannelID)
		if err != nil {
			return ProbeRunResult{}, err
		}
		channel = loaded
	}
	if channel == nil || channel.Id <= 0 {
		return ProbeRunResult{}, fmt.Errorf("probe channel is nil")
	}
	if !probeChannelEligible(channel) {
		return ProbeRunResult{}, fmt.Errorf("channel %d is not eligible for health probe", channel.Id)
	}
	setting := scheduler_setting.GetSetting()
	config := ProbeConfig{
		Enabled:                          true,
		Interval:                         time.Duration(setting.ProbeIntervalSeconds) * time.Second,
		WorkerCount:                      setting.ProbeWorkerCount,
		Timeout:                          time.Duration(setting.ProbeTimeoutSeconds) * time.Second,
		MaxPerTick:                       setting.ProbeMaxPerTick,
		MinChannelInterval:               time.Duration(setting.ProbeMinChannelIntervalSeconds) * time.Second,
		LowScoreThreshold:                setting.ProbeLowScoreThreshold,
		MissingSampleThreshold:           setting.ProbeMissingSampleThreshold,
		LongNoSuccessThreshold:           time.Duration(setting.ProbeLongNoSuccessSeconds) * time.Second,
		RecoverySuccessesRequired:        setting.ProbeRecoverySuccessesRequired,
		TimeoutRecoverySuccessesRequired: setting.ChannelTimeoutRecoveryProbeSuccesses,
		FirstByteTimeout:                 20 * time.Second,
		TotalTimeout:                     time.Duration(setting.RelayTotalTimeoutSeconds) * time.Second,
		FailureAvoidancePriorityEnabled:  setting.ProbeFailureAvoidancePriorityEnabled,
		RecoverableScoreItems:            setting.ProbeRecoverableScoreItems,
		SkipRecentRealRequestEnabled:     setting.ProbeSkipRecentRealRequestEnabled,
		RecentRealRequestWindow:          time.Duration(setting.ProbeRecentRealRequestWindowSeconds) * time.Second,
		GoodBaselineEnabled:              setting.ProbeGoodBaselineEnabled,
		GoodBaselineMinSamples:           setting.ProbeGoodBaselineMinSamples,
		GoodBaselineWindow:               time.Duration(setting.ProbeGoodBaselineWindowSeconds) * time.Second,
		PromptLibraryEnabled:             setting.ProbePromptLibraryEnabled,
		PromptCategories:                 setting.ProbePromptCategories,
	}
	config = normalizeProbeConfig(config)
	key := normalizeProbeRuntimeKey(options.RuntimeKey)
	key.ChannelID = channel.Id
	modelName := strings.TrimSpace(options.Model)
	if modelName == "" {
		modelName = strings.TrimSpace(key.RequestedModel)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(key.UpstreamModel)
	}
	if modelName == "" {
		modelName = selectProbeModel(channel)
	}
	if modelName == "" {
		return ProbeRunResult{}, fmt.Errorf("probe model is empty")
	}
	group := strings.TrimSpace(options.Group)
	if group == "" {
		group = strings.TrimSpace(key.Group)
	}
	if key.RequestedModel == "" {
		key.RequestedModel = modelName
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = channel.ResolveMappedModelName(modelName)
	}
	if key.Group == "" {
		key.Group = group
	}
	if key.EndpointType == "" {
		key.EndpointType = probeEndpointType(channel, modelName, "")
	}
	key = probeRuntimeKeyForChannel(channel, modelName, key.Group, key.EndpointType, key)
	if !ProbeRuntimeKeyEligible(channel, key) {
		return ProbeRunResult{}, fmt.Errorf("channel %d runtime key is not eligible for health probe", channel.Id)
	}
	reason := NormalizeProbeReason(options.Reason)
	if reason == "" {
		reason = reasonLowScore
	}
	deps := modelgatewayintegration.DefaultRuntimeObservabilityDeps()
	var snapshotStore core.RuntimeSnapshotStore
	var enricher core.RuntimeSnapshotEnricher
	var costBaseline core.CostBaselineProvider
	var breaker core.CircuitBreaker
	if deps != nil {
		snapshotStore = deps.SnapshotStore
		enricher = deps.RuntimeEnricher
		costBaseline = deps.CostBaselineCache
		breaker = deps.CircuitBreaker
	}
	healthMonitor := scheduler.NewRuntimeHealthMonitor(snapshotStore, breaker).
		WithScoringService(scheduler.NewCandidateScoringService().WithCostBaselineProvider(costBaseline)).
		WithScoreWeights(modelgatewayintegration.RuntimePolicySetting().ScoreWeights)
	executor := NewProbeExecutor(config.Timeout, NewProbeBillingRecorder()).
		WithTimeoutRecoveryThresholds(config.FirstByteTimeout, config.TotalTimeout)
	schedulerRunner := NewProbeScheduler(config, nil, executor, immediateProbeRecorder).
		WithSnapshotStore(snapshotStore).
		WithRuntimeSnapshotEnricher(enricher).
		WithCostBaselineProvider(costBaseline).
		WithScoreWeights(modelgatewayintegration.RuntimePolicySetting().ScoreWeights)
	candidate := ProbeCandidate{
		Channel:           channel,
		Model:             modelName,
		Group:             key.Group,
		Key:               key,
		Reason:            reason,
		TriggerScoreItems: append([]string(nil), options.TriggerScoreItems...),
		PromptCategories:  config.PromptCategories,
	}
	candidate.Plan = schedulerRunner.buildDispatchPlan(candidate)
	result := executor.Execute(ctx, candidate)
	healthMonitor.Record(context.Background(), result.DispatchRecord())
	healthMonitor.Report(context.Background(), result.AttemptResult())
	immediateProbeRecorder.Record(context.Background(), result.DispatchRecord())
	immediateProbeRecorder.Report(context.Background(), result.AttemptResult())
	logProbeResult(result)
	return result, nil
}

func StopDefaultProbeScheduler() {
	defaultProbeSchedulerMu.Lock()
	defer defaultProbeSchedulerMu.Unlock()
	stopDefaultProbeSchedulerLocked()
}

func DefaultProbeSchedulerRunning() bool {
	defaultProbeSchedulerMu.Lock()
	defer defaultProbeSchedulerMu.Unlock()
	return defaultProbeScheduler != nil
}

func DefaultProbeSchedulerStatus() SchedulerStatus {
	setting := scheduler_setting.GetSetting()
	config := normalizeProbeConfig(ProbeConfig{
		Enabled:     setting.ProbeEnabled,
		Interval:    time.Duration(setting.ProbeIntervalSeconds) * time.Second,
		WorkerCount: setting.ProbeWorkerCount,
		MaxPerTick:  setting.ProbeMaxPerTick,
	})
	status := SchedulerStatus{
		Enabled:                config.Enabled,
		Running:                false,
		ProbeIntervalSeconds:   int64(config.Interval.Seconds()),
		ProbeWorkerCount:       config.WorkerCount,
		ProbeMaxPerTick:        config.MaxPerTick,
		MasterNode:             common.IsMasterNode,
		RelayInvokerRegistered: relayInvoker != nil,
	}
	defaultProbeSchedulerMu.Lock()
	scheduler := defaultProbeScheduler
	defaultProbeSchedulerMu.Unlock()
	if scheduler == nil {
		return status
	}
	status = scheduler.Status()
	status.RelayInvokerRegistered = relayInvoker != nil
	return status
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

func logProbeSkipped(candidate ProbeCandidate, reason string) {
	channelID := 0
	if candidate.Channel != nil {
		channelID = candidate.Channel.Id
	}
	common.SysLog(fmt.Sprintf("model gateway probe skipped: channel_id=%d model=%s reason=%s skip_reason=%s",
		channelID, candidate.Model, candidate.Reason, reason))
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
	plan.Strategy = policy.Strategy
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
	probeCandidate := core.Candidate{
		Channel:                candidate.Channel,
		Group:                  plan.SelectedGroup,
		UpstreamModel:          plan.RuntimeKey.UpstreamModel,
		ProviderProfile:        plan.ProviderProfile,
		ProxyMode:              plan.ProxyMode,
		RequiresCodexImageTool: false,
		RuntimeKey:             plan.RuntimeKey,
	}
	weights := s.scoreWeights
	if weights.Success == 0 && weights.Speed == 0 && weights.Load == 0 && weights.Cost == 0 && weights.Group == 0 {
		weights = modelgatewayintegration.RuntimePolicySetting().ScoreWeights
	}
	scoreSelector := scheduler.NewDefaultSmartChannelSelector(nil, s.snapshotStore, weights).
		WithRuntimeSnapshotEnricher(s.enricher).
		WithCostBaselineProvider(s.costBaseline)
	scored := scoreSelector.ScoreCandidate(probeCandidate, policy)
	snapshot := scored.Snapshot
	snapshot.Key = plan.RuntimeKey
	snapshot.ProbeTriggerReason = strings.TrimSpace(candidate.Reason)
	if snapshot.ProbeRecoveryRequired <= 0 {
		snapshot.ProbeRecoveryRequired = normalizeProbeConfig(s.config).RecoverySuccessesRequired
	}
	if candidate.Reason == reasonTimeoutRecovery {
		snapshot.ProbeRecoveryRequired = normalizeProbeConfig(s.config).TimeoutRecoverySuccessesRequired
	}
	if candidate.Reason == reasonScoreAnomaly {
		snapshot.ProbeRecoveryPhase = core.ProbeRecoveryPhaseFastProbe
	}
	if candidate.Reason == reasonLowScore || candidate.Reason == reasonFailureAvoidance || candidate.Reason == reasonTimeoutRecovery || candidate.Reason == reasonCircuitProbe || candidate.Reason == reasonScoreAnomaly || snapshot.FailureAvoidance {
		snapshot.ProbeRecoveryPending = true
	}
	score := scored.Score
	explanation := scored.Explanation
	explanation.Available = true
	explanation.Selected = true
	explanation.ProbeRecoveryPending = snapshot.ProbeRecoveryPending
	explanation.ProbeRecoveryRequired = snapshot.ProbeRecoveryRequired
	explanation.ProbeRecoverySuccessCount = snapshot.ProbeRecoverySuccessCount
	explanation.ProbeTriggerReason = snapshot.ProbeTriggerReason
	explanation.ProbeRecoveryPhase = snapshot.ProbeRecoveryPhase
	explanation.ProbeFastRecoveryAttempts = snapshot.ProbeFastRecoveryAttempts
	explanation.ProbeAnomalyTriggerItems = append([]string(nil), snapshot.ProbeAnomalyTriggerItems...)
	plan.ScoreTotal = score.Total
	plan.ScoreBreakdown = score.Breakdown
	plan.RoutingScoreTotal = score.RoutingTotal
	plan.RoutingScoreBreakdown = score.RoutingBreakdown
	plan.Candidates = []core.CandidateExplanation{explanation}
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
