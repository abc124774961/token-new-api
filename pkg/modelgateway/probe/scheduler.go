package probe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/recording"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
)

type ProbeScheduler struct {
	config   ProbeConfig
	selector *ProbeSelector
	executor *ProbeExecutor
	recorder *recording.AsyncExecutionRecorder
	stop     chan struct{}
	once     sync.Once
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
		config:   config,
		selector: selector,
		executor: executor,
		recorder: recorder,
		stop:     make(chan struct{}),
	}
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
				result := s.executor.Execute(ctx, candidate)
				s.selector.MarkResult(result)
				if s.recorder != nil {
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
		Enabled:            setting.ProbeEnabled,
		Interval:           time.Duration(setting.ProbeIntervalSeconds) * time.Second,
		WorkerCount:        setting.ProbeWorkerCount,
		Timeout:            time.Duration(setting.ProbeTimeoutSeconds) * time.Second,
		MaxPerTick:         setting.ProbeMaxPerTick,
		MinChannelInterval: time.Duration(setting.ProbeMinChannelIntervalSeconds) * time.Second,
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
	s := NewProbeScheduler(config, selector, executor, recorder)
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
