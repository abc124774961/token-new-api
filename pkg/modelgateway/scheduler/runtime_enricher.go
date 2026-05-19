package scheduler

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
)

const (
	defaultQueueTimeoutMs       = 2000
	defaultQueueDepthMultiplier = 2
	defaultQueueMaxDepth        = 64
)

type RuntimeStateProvider interface {
	ActiveConcurrency(channelID int) int
	ConcurrencyCooldownActive(channelID int) bool
	FailureAvoidanceActive(channelID int) bool
}

type ServiceRuntimeStateProvider struct{}

func NewServiceRuntimeStateProvider() *ServiceRuntimeStateProvider {
	return &ServiceRuntimeStateProvider{}
}

func (p *ServiceRuntimeStateProvider) ActiveConcurrency(channelID int) int {
	return service.GetChannelActiveConcurrency(channelID)
}

func (p *ServiceRuntimeStateProvider) ConcurrencyCooldownActive(channelID int) bool {
	return service.GetChannelConcurrencyCooldownStatus(channelID) != nil
}

func (p *ServiceRuntimeStateProvider) FailureAvoidanceActive(channelID int) bool {
	return service.GetChannelFailureAvoidanceStatus(channelID) != nil
}

type RuntimeSnapshotEnricher struct {
	stateProvider        RuntimeStateProvider
	circuitBreaker       core.CircuitBreaker
	queueTimeoutMs       int
	queueMaxDepth        int
	queueDepthMultiplier int
}

func NewRuntimeSnapshotEnricher(stateProvider RuntimeStateProvider, queueTimeoutMs int, queueMaxDepth int, queueDepthMultiplier int) *RuntimeSnapshotEnricher {
	if stateProvider == nil {
		stateProvider = NewServiceRuntimeStateProvider()
	}
	if queueTimeoutMs <= 0 {
		queueTimeoutMs = defaultQueueTimeoutMs
	}
	if queueMaxDepth <= 0 {
		queueMaxDepth = defaultQueueMaxDepth
	}
	if queueDepthMultiplier <= 0 {
		queueDepthMultiplier = defaultQueueDepthMultiplier
	}
	return &RuntimeSnapshotEnricher{
		stateProvider:        stateProvider,
		queueTimeoutMs:       queueTimeoutMs,
		queueMaxDepth:        queueMaxDepth,
		queueDepthMultiplier: queueDepthMultiplier,
	}
}

func (e *RuntimeSnapshotEnricher) WithCircuitBreaker(breaker core.CircuitBreaker) *RuntimeSnapshotEnricher {
	if e == nil {
		return nil
	}
	e.circuitBreaker = breaker
	return e
}

func (e *RuntimeSnapshotEnricher) ReserveCircuitProbe(key core.RuntimeKey) bool {
	if e == nil || e.circuitBreaker == nil {
		return true
	}
	return e.circuitBreaker.AllowProbe(key)
}

func (e *RuntimeSnapshotEnricher) Enrich(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	if candidate.Channel == nil {
		return snapshot
	}
	if snapshot.Key.ChannelID == 0 {
		snapshot.Key.ChannelID = candidate.Channel.Id
	}
	if snapshot.Key.Group == "" {
		snapshot.Key.Group = candidate.Group
	}
	if snapshot.Key.RequestedModel == "" {
		snapshot.Key.RequestedModel = candidate.RuntimeKey.RequestedModel
	}
	if snapshot.Key.UpstreamModel == "" {
		snapshot.Key.UpstreamModel = candidate.RuntimeKey.UpstreamModel
	}
	if snapshot.Key.EndpointType == "" {
		snapshot.Key.EndpointType = candidate.RuntimeKey.EndpointType
	}
	snapshot.Key.CapabilityFingerprint = appendCapabilityPart(snapshot.Key.CapabilityFingerprint, candidate.ProviderProfile)
	snapshot.Key.CapabilityFingerprint = appendCapabilityPart(snapshot.Key.CapabilityFingerprint, candidate.ProxyMode)
	if e == nil || e.stateProvider == nil {
		return snapshot
	}
	channelID := candidate.Channel.Id
	setting := candidate.Channel.GetSetting()
	snapshot = e.applyConcurrency(snapshot, channelID, setting, policy)
	snapshot.Cooldown = snapshot.Cooldown || e.stateProvider.ConcurrencyCooldownActive(channelID)
	snapshot.FailureAvoidance = snapshot.FailureAvoidance || e.stateProvider.FailureAvoidanceActive(channelID)
	snapshot = e.applyCircuit(snapshot, policy)
	return snapshot
}

func (e *RuntimeSnapshotEnricher) applyConcurrency(snapshot core.RuntimeSnapshot, channelID int, setting dto.ChannelSettings, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	active := e.stateProvider.ActiveConcurrency(channelID)
	if active > snapshot.ActiveConcurrency {
		snapshot.ActiveConcurrency = active
	}
	if snapshot.MaxConcurrency <= 0 && setting.MaxConcurrency > 0 {
		snapshot.MaxConcurrency = setting.MaxConcurrency
	}
	if snapshot.MaxConcurrency <= 0 {
		return snapshot
	}
	if policy.QueueEnabled {
		capacity := e.queueCapacity(snapshot.MaxConcurrency)
		if snapshot.QueueCapacity <= 0 {
			snapshot.QueueCapacity = capacity
		}
		if snapshot.QueueTimeoutMs <= 0 {
			snapshot.QueueTimeoutMs = e.queueTimeoutMs
		}
		if snapshot.ActiveConcurrency >= snapshot.MaxConcurrency {
			overflow := snapshot.ActiveConcurrency - snapshot.MaxConcurrency + 1
			if overflow > snapshot.QueueDepth {
				snapshot.QueueDepth = overflow
			}
			if snapshot.EstimatedQueueWaitMs <= 0 {
				snapshot.EstimatedQueueWaitMs = estimateQueueWaitMs(snapshot)
			}
		}
	}
	return snapshot
}

func (e *RuntimeSnapshotEnricher) queueCapacity(maxConcurrency int) int {
	capacity := maxConcurrency * e.queueDepthMultiplier
	if capacity <= 0 {
		capacity = 1
	}
	return int(math.Min(float64(capacity), float64(e.queueMaxDepth)))
}

func (e *RuntimeSnapshotEnricher) applyCircuit(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	if e == nil || e.circuitBreaker == nil || !policy.CircuitBreakerEnabled {
		return snapshot
	}
	circuit := e.circuitBreaker.Snapshot(snapshot.Key)
	snapshot.CircuitState = circuit.State
	switch circuit.State {
	case core.CircuitStateOpen:
		snapshot.CircuitOpen = true
	case core.CircuitStateHalfOpen:
		snapshot.CircuitOpen = circuit.HalfOpenProbeMax > 0 && circuit.HalfOpenProbeUsed >= circuit.HalfOpenProbeMax
	}
	return snapshot
}

func estimateQueueWaitMs(snapshot core.RuntimeSnapshot) float64 {
	if snapshot.TokensPerSecond > 0 && snapshot.DurationMs > 0 {
		return math.Max(snapshot.DurationMs/2, 1)
	}
	if snapshot.DurationMs > 0 {
		return math.Max(snapshot.DurationMs/2, 1)
	}
	if snapshot.TTFTMs > 0 {
		return math.Max(snapshot.TTFTMs*2, 1)
	}
	return 1000
}

func appendCapabilityPart(fingerprint string, part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return fingerprint
	}
	if fingerprint == "" {
		return part
	}
	parts := strings.Split(fingerprint, "|")
	for _, existing := range parts {
		if existing == part {
			return fingerprint
		}
	}
	parts = append(parts, part)
	return strings.Join(parts, "|")
}

var _ core.RuntimeSnapshotEnricher = (*RuntimeSnapshotEnricher)(nil)
