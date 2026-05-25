package scheduler

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
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
	FirstBytePendingStatus(channelID int) *service.ChannelFirstBytePendingStatus
	ConfigErrorIsolationStatus(key core.RuntimeKey) *service.ChannelConfigIsolationStatus
}

type CostProfileProvider interface {
	CostRatio(channelID int, upstreamModel string) (float64, bool)
}

type CostReferenceProvider interface {
	CostReferenceRatio(upstreamModel string, pricingMode string) (float64, bool)
}

type costPricingModeProvider interface {
	CostPricingMode(channelID int, upstreamModel string) string
}

type ServiceRuntimeStateProvider struct{}

func NewServiceRuntimeStateProvider() *ServiceRuntimeStateProvider {
	return &ServiceRuntimeStateProvider{}
}

func (p *ServiceRuntimeStateProvider) ActiveConcurrency(channelID int) int {
	return service.GetChannelEffectiveActiveConcurrency(channelID)
}

func (p *ServiceRuntimeStateProvider) ConcurrencyCooldownActive(channelID int) bool {
	return service.GetChannelConcurrencyCooldownStatus(channelID) != nil
}

func (p *ServiceRuntimeStateProvider) FailureAvoidanceActive(channelID int) bool {
	return service.GetChannelFailureAvoidanceStatus(channelID) != nil
}

func (p *ServiceRuntimeStateProvider) FirstBytePendingStatus(channelID int) *service.ChannelFirstBytePendingStatus {
	return service.GetChannelFirstBytePendingStatus(channelID)
}

func (p *ServiceRuntimeStateProvider) ConfigErrorIsolationStatus(key core.RuntimeKey) *service.ChannelConfigIsolationStatus {
	return service.GetChannelConfigIsolationStatus(service.NewChannelConfigIsolationKey(
		key.ChannelID,
		key.RequestedModel,
		key.Group,
		key.EndpointType,
	))
}

type RuntimeSnapshotEnricher struct {
	stateProvider        RuntimeStateProvider
	costProvider         CostProfileProvider
	costBaselineProvider core.CostBaselineProvider
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

func (e *RuntimeSnapshotEnricher) WithCostProfileProvider(provider CostProfileProvider) *RuntimeSnapshotEnricher {
	if e == nil {
		return nil
	}
	e.costProvider = provider
	return e
}

func (e *RuntimeSnapshotEnricher) WithCostBaselineProvider(provider core.CostBaselineProvider) *RuntimeSnapshotEnricher {
	if e == nil {
		return nil
	}
	e.costBaselineProvider = provider
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
	snapshot = e.applyFirstBytePending(snapshot, channelID)
	snapshot.Cooldown = snapshot.Cooldown || e.stateProvider.ConcurrencyCooldownActive(channelID)
	snapshot.FailureAvoidance = snapshot.FailureAvoidance || e.stateProvider.FailureAvoidanceActive(channelID)
	snapshot = e.applyConfigErrorIsolation(snapshot)
	snapshot = e.applyCostSnapshot(candidate, snapshot, policy)
	snapshot = e.applyCircuit(snapshot, policy)
	return snapshot
}

func (e *RuntimeSnapshotEnricher) applyConfigErrorIsolation(snapshot core.RuntimeSnapshot) core.RuntimeSnapshot {
	status := e.stateProvider.ConfigErrorIsolationStatus(snapshot.Key)
	if status == nil {
		return snapshot
	}
	snapshot.ConfigErrorIsolated = status.Active
	snapshot.IsolationReason = status.Reason
	snapshot.IsolationUntil = status.Until
	snapshot.AuthConfigErrorCount = status.FailureCount
	snapshot.LastAuthConfigErrorAt = status.LastErrorAt
	return snapshot
}

func (e *RuntimeSnapshotEnricher) applyFirstBytePending(snapshot core.RuntimeSnapshot, channelID int) core.RuntimeSnapshot {
	status := e.stateProvider.FirstBytePendingStatus(channelID)
	if status == nil {
		return snapshot
	}
	if status.Pending > snapshot.FirstBytePending {
		snapshot.FirstBytePending = status.Pending
	}
	if status.SlowPending > snapshot.SlowFirstBytePending {
		snapshot.SlowFirstBytePending = status.SlowPending
	}
	oldestMs := float64(status.OldestMs)
	if oldestMs > snapshot.OldestFirstByteWaitMs {
		snapshot.OldestFirstByteWaitMs = oldestMs
	}
	return snapshot
}

func (e *RuntimeSnapshotEnricher) applyConcurrency(snapshot core.RuntimeSnapshot, channelID int, setting dto.ChannelSettings, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	active := e.stateProvider.ActiveConcurrency(channelID)
	if active > snapshot.ActiveConcurrency {
		snapshot.ActiveConcurrency = active
	}
	configuredLimit := setting.MaxConcurrencyCeiling
	if configuredLimit <= 0 {
		configuredLimit = setting.MaxConcurrency
	}
	if snapshot.ConfiguredConcurrencyLimit <= 0 {
		snapshot.ConfiguredConcurrencyLimit = configuredLimit
	}
	if snapshot.LearnedConcurrencyLimit <= 0 && setting.MaxConcurrency > 0 {
		snapshot.LearnedConcurrencyLimit = setting.MaxConcurrency
	}
	effectiveLimit := setting.MaxConcurrency
	if effectiveLimit <= 0 && snapshot.LearnedConcurrencyLimit > 0 {
		effectiveLimit = snapshot.LearnedConcurrencyLimit
	}
	if effectiveLimit <= 0 && snapshot.EffectiveConcurrencyLimit > 0 {
		effectiveLimit = snapshot.EffectiveConcurrencyLimit
	}
	if effectiveLimit <= 0 && snapshot.MaxConcurrency > 0 {
		effectiveLimit = snapshot.MaxConcurrency
	}
	if effectiveLimit <= 0 && configuredLimit > 0 {
		effectiveLimit = configuredLimit
	}
	if effectiveLimit > 0 {
		snapshot.EffectiveConcurrencyLimit = effectiveLimit
	}
	if snapshot.MaxConcurrency <= 0 && effectiveLimit > 0 {
		snapshot.MaxConcurrency = effectiveLimit
	}
	if snapshot.ConfiguredConcurrencyLimit <= 0 && snapshot.MaxConcurrency > 0 {
		snapshot.ConfiguredConcurrencyLimit = snapshot.MaxConcurrency
	}
	if snapshot.LearnedConcurrencyLimit <= 0 && snapshot.MaxConcurrency > 0 {
		snapshot.LearnedConcurrencyLimit = snapshot.MaxConcurrency
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

func (e *RuntimeSnapshotEnricher) applyCostSnapshot(candidate core.Candidate, snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	if candidate.Channel == nil {
		return snapshot
	}
	if ratio, ok := e.lookupProfileCostRatio(candidate); ok {
		snapshot.CostRatio = ratio
		snapshot.CostPricingMode = e.lookupProfileCostPricingMode(candidate)
	} else {
		snapshot.CostRatio = 0
		snapshot.CostPricingMode = ""
	}
	if snapshot.CostRatio > 0 {
		if reference, ok := e.lookupCandidateSetCostBaseline(candidate); ok && reference > 0 {
			snapshot.CostReferenceRatio = reference
		} else if reference, ok := e.lookupReferenceCostRatio(candidate, snapshot.CostPricingMode); ok && reference > 0 {
			snapshot.CostReferenceRatio = reference
		}
	}
	if ratio := candidateGroupPriorityRatio(candidate, policy); ratio > 0 {
		snapshot.GroupPriorityRatio = ratio
	} else if snapshot.GroupPriorityRatio <= 0 {
		snapshot.GroupPriorityRatio = 1
	}
	return snapshot
}

func (e *RuntimeSnapshotEnricher) lookupProfileCostRatio(candidate core.Candidate) (float64, bool) {
	if candidate.Channel == nil {
		return 0, false
	}
	modelName := candidateCostModelName(candidate)
	if e != nil && e.costProvider != nil {
		if ratio, ok := e.costProvider.CostRatio(candidate.Channel.Id, modelName); ok && ratio > 0 {
			return ratio, true
		}
	}
	return modelgatewaycost.CostRatioFromProfileForModel(modelgatewaycost.LookupCachedDefaultProfile(candidate.Channel.Id, modelName), modelName)
}

func (e *RuntimeSnapshotEnricher) lookupProfileCostPricingMode(candidate core.Candidate) string {
	if candidate.Channel == nil {
		return ""
	}
	modelName := candidateCostModelName(candidate)
	if e != nil && e.costProvider != nil {
		if provider, ok := e.costProvider.(costPricingModeProvider); ok {
			if mode := strings.TrimSpace(provider.CostPricingMode(candidate.Channel.Id, modelName)); mode != "" {
				return mode
			}
		}
	}
	return modelgatewaycost.CostPricingModeFromProfileForModel(modelgatewaycost.LookupCachedDefaultProfile(candidate.Channel.Id, modelName), modelName)
}

func (e *RuntimeSnapshotEnricher) lookupReferenceCostRatio(candidate core.Candidate, pricingMode string) (float64, bool) {
	modelName := candidateCostModelName(candidate)
	if modelName == "" {
		return 0, false
	}
	if e != nil && e.costProvider != nil {
		if provider, ok := e.costProvider.(CostReferenceProvider); ok {
			if ratio, found := provider.CostReferenceRatio(modelName, pricingMode); found && ratio > 0 {
				return ratio, true
			}
		}
	}
	return modelgatewaycost.LookupCachedReferenceCostRatio(modelName, pricingMode)
}

func (e *RuntimeSnapshotEnricher) lookupCandidateSetCostBaseline(candidate core.Candidate) (float64, bool) {
	if e == nil || e.costBaselineProvider == nil {
		return 0, false
	}
	scope := core.CostBaselineScope{
		RequestedModel:         strings.TrimSpace(candidate.RuntimeKey.RequestedModel),
		Group:                  strings.TrimSpace(candidate.Group),
		EndpointType:           strings.TrimSpace(string(candidate.RuntimeKey.EndpointType)),
		RequiresCodexImageTool: candidate.RequiresCodexImageTool,
	}
	return e.costBaselineProvider.Baseline(scope)
}

func candidateGroupPriorityRatio(candidate core.Candidate, policy core.GroupSmartPolicy) float64 {
	group := strings.TrimSpace(candidate.Group)
	if group == "" {
		group = strings.TrimSpace(candidate.RuntimeKey.Group)
	}
	if group == "" || len(policy.GroupPriorityRatio) == 0 {
		return 0
	}
	return policy.GroupPriorityRatio[group]
}

func candidateCostModelName(candidate core.Candidate) string {
	modelName := strings.TrimSpace(candidate.RuntimeKey.UpstreamModel)
	if modelName == "" {
		modelName = strings.TrimSpace(candidate.UpstreamModel)
	}
	if modelName == "" && candidate.Channel != nil {
		modelName = candidate.Channel.ResolveMappedModelName(candidate.RuntimeKey.RequestedModel)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(candidate.RuntimeKey.RequestedModel)
	}
	return modelName
}

var _ core.RuntimeSnapshotEnricher = (*RuntimeSnapshotEnricher)(nil)
