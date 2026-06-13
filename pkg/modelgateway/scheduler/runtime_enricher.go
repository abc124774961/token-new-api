package scheduler

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
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
	FailureAvoidanceStatus(channelID int) *service.ChannelFailureAvoidanceStatus
	FirstBytePendingStatus(channelID int) *service.ChannelFirstBytePendingStatus
}

type RuntimeAccountStateProvider interface {
	FailureAvoidanceStatusForIdentity(identity service.ChannelRuntimeIdentity) *service.ChannelFailureAvoidanceStatus
	FirstBytePendingStatusForIdentity(identity service.ChannelRuntimeIdentity) *service.ChannelFirstBytePendingStatus
}

type RuntimeAccountConcurrencyProvider interface {
	ActiveConcurrencyForIdentity(identity service.ChannelRuntimeIdentity) int
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

func (p *ServiceRuntimeStateProvider) ActiveConcurrencyForIdentity(identity service.ChannelRuntimeIdentity) int {
	return service.GetChannelRuntimeEffectiveActiveConcurrency(identity)
}

func (p *ServiceRuntimeStateProvider) ConcurrencyCooldownActive(channelID int) bool {
	return service.GetChannelConcurrencyCooldownStatus(channelID) != nil
}

func (p *ServiceRuntimeStateProvider) FailureAvoidanceActive(channelID int) bool {
	return service.GetChannelFailureAvoidanceStatus(channelID) != nil
}

func (p *ServiceRuntimeStateProvider) FailureAvoidanceStatus(channelID int) *service.ChannelFailureAvoidanceStatus {
	return service.GetChannelFailureAvoidanceStatus(channelID)
}

func (p *ServiceRuntimeStateProvider) FailureAvoidanceStatusForIdentity(identity service.ChannelRuntimeIdentity) *service.ChannelFailureAvoidanceStatus {
	return service.GetChannelRuntimeFailureAvoidanceStatus(identity)
}

func (p *ServiceRuntimeStateProvider) FirstBytePendingStatus(channelID int) *service.ChannelFirstBytePendingStatus {
	return service.GetChannelFirstBytePendingStatus(channelID)
}

func (p *ServiceRuntimeStateProvider) FirstBytePendingStatusForIdentity(identity service.ChannelRuntimeIdentity) *service.ChannelFirstBytePendingStatus {
	return service.GetChannelRuntimeFirstBytePendingStatus(identity)
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
	identity := serviceRuntimeIdentityFromCandidate(candidate, snapshot)
	snapshot = e.applyConcurrency(snapshot, channelID, setting, policy, identity, accountMaxConcurrencyForCandidate(candidate))
	snapshot = e.applyFirstBytePending(snapshot, channelID, identity)
	snapshot.Cooldown = snapshot.Cooldown || e.stateProvider.ConcurrencyCooldownActive(channelID)
	snapshot = e.applyFailureAvoidance(snapshot, channelID, identity)
	snapshot = clearConfigErrorIsolationSnapshot(snapshot)
	snapshot = e.applyCostSnapshot(candidate, snapshot, policy)
	snapshot = e.applyCircuit(snapshot, policy)
	return snapshot
}

func (e *RuntimeSnapshotEnricher) applyFailureAvoidance(snapshot core.RuntimeSnapshot, channelID int, identity service.ChannelRuntimeIdentity) core.RuntimeSnapshot {
	status := e.stateProvider.FailureAvoidanceStatus(channelID)
	if accountProvider, ok := e.stateProvider.(RuntimeAccountStateProvider); ok && identity.HasAccountScope() {
		if accountStatus := accountProvider.FailureAvoidanceStatusForIdentity(identity); accountStatus != nil {
			status = accountStatus
		}
	}
	if status == nil {
		if identity.HasAccountScope() {
			return snapshot
		}
		snapshot.FailureAvoidance = snapshot.FailureAvoidance || e.stateProvider.FailureAvoidanceActive(channelID)
		return snapshot
	}
	snapshot.FailureAvoidance = true
	if service.IsProbeRecoveryReason(status.Reason) || status.ProbeRecoveryRequired {
		snapshot.ProbeRecoveryPending = true
		snapshot.ProbeTriggerReason = strings.TrimSpace(status.Reason)
		setting := scheduler_setting.GetSetting()
		required := setting.ProbeRecoverySuccessesRequired
		if service.IsTimeoutRecoveryReason(status.Reason) {
			required = setting.ChannelTimeoutRecoveryProbeSuccesses
			if required <= 0 {
				required = setting.ProbeRecoverySuccessesRequired
			}
		}
		if required <= 0 {
			required = 2
		}
		snapshot.ProbeRecoveryRequired = required
	}
	return snapshot
}

func clearConfigErrorIsolationSnapshot(snapshot core.RuntimeSnapshot) core.RuntimeSnapshot {
	snapshot.ConfigErrorIsolated = false
	snapshot.IsolationReason = ""
	snapshot.IsolationUntil = 0
	return snapshot
}

func (e *RuntimeSnapshotEnricher) applyFirstBytePending(snapshot core.RuntimeSnapshot, channelID int, identity service.ChannelRuntimeIdentity) core.RuntimeSnapshot {
	status := e.stateProvider.FirstBytePendingStatus(channelID)
	if accountProvider, ok := e.stateProvider.(RuntimeAccountStateProvider); ok && identity.HasAccountScope() {
		status = accountProvider.FirstBytePendingStatusForIdentity(identity)
	}
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

func (e *RuntimeSnapshotEnricher) applyConcurrency(snapshot core.RuntimeSnapshot, channelID int, setting dto.ChannelSettings, policy core.GroupSmartPolicy, identity service.ChannelRuntimeIdentity, accountMaxConcurrency int) core.RuntimeSnapshot {
	active := e.stateProvider.ActiveConcurrency(channelID)
	if accountMaxConcurrency > 0 && identity.HasAccountScope() {
		if provider, ok := e.stateProvider.(RuntimeAccountConcurrencyProvider); ok {
			active = provider.ActiveConcurrencyForIdentity(identity)
		}
		setting.MaxConcurrency = accountMaxConcurrency
		setting.MaxConcurrencyCeiling = accountMaxConcurrency
		snapshot.MaxConcurrency = accountMaxConcurrency
		snapshot.ConfiguredConcurrencyLimit = accountMaxConcurrency
		snapshot.LearnedConcurrencyLimit = accountMaxConcurrency
		snapshot.EffectiveConcurrencyLimit = accountMaxConcurrency
	}
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

func accountMaxConcurrencyForCandidate(candidate core.Candidate) int {
	if candidate.Channel == nil {
		return 0
	}
	index := candidate.CredentialRef.CredentialIndex
	if index == 0 && candidate.AccountIdentity.CredentialIndex != 0 {
		index = candidate.AccountIdentity.CredentialIndex
	}
	if index < 0 || !candidateHasCredentialScope(candidate) {
		return 0
	}
	return candidate.Channel.ChannelInfo.AccountMaxConcurrency(index)
}

func (e *RuntimeSnapshotEnricher) applyCircuit(snapshot core.RuntimeSnapshot, policy core.GroupSmartPolicy) core.RuntimeSnapshot {
	if e == nil || e.circuitBreaker == nil || !policy.CircuitBreakerEnabled {
		return snapshot
	}
	circuit := e.circuitBreaker.Snapshot(snapshot.Key)
	applyCircuitToRuntimeSnapshot(&snapshot, circuit)
	return snapshot
}

func applyCircuitToRuntimeSnapshot(snapshot *core.RuntimeSnapshot, circuit core.CircuitSnapshot) {
	if snapshot == nil {
		return
	}
	circuit = normalizeCircuitSnapshot(circuit)
	if circuit.State == "" {
		circuit.State = core.CircuitStateClosed
	}
	snapshot.CircuitState = circuit.State
	snapshot.CircuitOpen = circuit.State == core.CircuitStateOpen
	if circuit.State == core.CircuitStateHalfOpen {
		snapshot.CircuitOpen = circuit.HalfOpenProbeMax > 0 && circuit.HalfOpenProbeUsed >= circuit.HalfOpenProbeMax
	}
	if !circuit.OpenUntil.IsZero() {
		snapshot.CircuitOpenUntil = circuit.OpenUntil.Unix()
	} else {
		snapshot.CircuitOpenUntil = 0
	}
	snapshot.CircuitOpenReason = circuit.OpenReason
	snapshot.CircuitFailureCount = circuit.FailureCount
	snapshot.CircuitFailureRate = circuit.FailureRate
	snapshot.CircuitSampleCount = circuit.SampleCount
	snapshot.CircuitErrorCounts = copyCircuitErrorCounts(circuit.ErrorCounts)
	snapshot.CircuitHalfOpenProbeUsed = circuit.HalfOpenProbeUsed
	snapshot.CircuitHalfOpenProbeMax = circuit.HalfOpenProbeMax
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
	if ratio := candidateGroupRevenueRatio(candidate, policy); ratio > 0 {
		snapshot.RevenueRatio = ratio
	}
	if revenue := candidateRevenueRatio(candidate, policy); revenue > 0 {
		snapshot.RevenueRatio = revenue
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
		RequiresCodexImageTool: false,
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

func candidateGroupRevenueRatio(candidate core.Candidate, policy core.GroupSmartPolicy) float64 {
	group := strings.TrimSpace(candidate.Group)
	if group == "" {
		group = strings.TrimSpace(candidate.RuntimeKey.Group)
	}
	if group == "" || len(policy.GroupRevenueRatio) == 0 {
		return 0
	}
	return policy.GroupRevenueRatio[group]
}

func candidateRevenueRatio(candidate core.Candidate, policy core.GroupSmartPolicy) float64 {
	group := strings.TrimSpace(candidate.Group)
	if group == "" {
		group = strings.TrimSpace(candidate.RuntimeKey.Group)
	}
	if group == "" {
		return 0
	}
	modelName := strings.TrimSpace(candidate.RuntimeKey.RequestedModel)
	if modelName == "" {
		modelName = strings.TrimSpace(candidate.RuntimeKey.UpstreamModel)
	}
	if modelName == "" {
		modelName = strings.TrimSpace(candidate.UpstreamModel)
	}
	if modelName == "" || candidate.Channel == nil {
		return 0
	}
	groupRatio := candidateBillingGroupRatio(candidate, policy, group)
	groupRatio = dynamicCandidateGroupRatio(modelName, group, groupRatio, policy)
	return requestedModelRevenueRatio(modelName, groupRatio)
}

func candidateBillingGroupRatio(candidate core.Candidate, policy core.GroupSmartPolicy, group string) float64 {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0
	}
	if ratio, ok := ratio_setting.GetGroupGroupRatio(strings.TrimSpace(policy.UserGroup), group); ok && ratio > 0 {
		return ratio
	}
	if ratio := candidateGroupRevenueRatio(candidate, policy); ratio > 0 {
		return ratio
	}
	if ratio_setting.ContainsGroupRatio(group) {
		return ratio_setting.GetGroupRatio(group)
	}
	return 1
}

func dynamicCandidateGroupRatio(modelName string, group string, staticGroupRatio float64, policy core.GroupSmartPolicy) float64 {
	if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
		return staticGroupRatio
	}
	setting := scheduler_setting.GetSetting()
	snapshot := modelgatewaydynamicbilling.Apply(modelgatewaydynamicbilling.ApplyInput{
		RequestedModel:   modelName,
		Group:            group,
		StaticGroupRatio: staticGroupRatio,
		Mode:             scheduler_setting.BillingRatioModeDynamic,
		Setting:          setting,
		Provider:         modelgatewaydynamicbilling.DefaultRatioProvider(),
	})
	if snapshot.Applied && snapshot.DynamicRatio > 0 {
		return snapshot.DynamicRatio
	}
	return staticGroupRatio
}

func requestedModelRevenueRatio(modelName string, groupRatio float64) float64 {
	if groupRatio <= 0 {
		return 0
	}
	value, usePrice, ok := ratio_setting.GetModelRatioOrPrice(modelName)
	if !ok || value <= 0 {
		return 0
	}
	if usePrice {
		return value * groupRatio
	}
	return value * 2 * groupRatio
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
