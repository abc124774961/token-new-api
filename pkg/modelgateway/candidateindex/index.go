package candidateindex

import (
	"sort"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	"github.com/QuantumNous/new-api/pkg/modelgateway/account"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const defaultMaxQueryCandidates = 256

type Registry interface {
	AccountsForChannel(channel *model.Channel) []account.ChannelAccount
}

type Query struct {
	Groups                 []string
	ModelName              string
	EndpointType           constant.EndpointType
	RequiresCodexImageTool bool
	MaxCandidates          int
	ExcludedChannelIDs     map[int]struct{}
	ExcludedAccountIDs     map[string]struct{}
	ExcludedRuntimeKeyIDs  map[string]struct{}
	Provider               string
	Brand                  string
	CapabilityFingerprint  string
}

type IndexStats struct {
	Version       int64 `json:"version"`
	Channels      int   `json:"channels"`
	Accounts      int   `json:"accounts"`
	Candidates    int   `json:"candidates"`
	Groups        int   `json:"groups"`
	GroupModels   int   `json:"group_models"`
	SkippedKeys   int   `json:"skipped_keys"`
	DisabledKeys  int   `json:"disabled_keys"`
	BuildSequence int64 `json:"build_sequence"`
}

type CandidateIndex struct {
	registry         Registry
	providerRegistry provider.ProviderRegistry
	snapshot         atomic.Pointer[snapshot]
	version          atomic.Int64
}

func New(registry Registry, providerRegistry provider.ProviderRegistry) *CandidateIndex {
	if registry == nil {
		registry = account.NewRegistry()
	}
	if providerRegistry == nil {
		providerRegistry = provider.NewStandardProviderRegistry()
	}
	index := &CandidateIndex{
		registry:         registry,
		providerRegistry: providerRegistry,
	}
	index.snapshot.Store(&snapshot{
		byGroupModel: make(map[string][]core.Candidate),
		stats:        IndexStats{},
	})
	return index
}

func (i *CandidateIndex) Rebuild(channels []*model.Channel) IndexStats {
	if i == nil {
		return IndexStats{}
	}
	builder := newSnapshotBuilder(i.registry, i.providerRegistry)
	next := builder.Build(channels)
	next.stats.BuildSequence = i.version.Add(1)
	next.stats.Version = next.stats.BuildSequence
	i.snapshot.Store(next)
	return next.stats
}

func (i *CandidateIndex) Query(query Query) []core.Candidate {
	if i == nil {
		return nil
	}
	current := i.snapshot.Load()
	if current == nil {
		return nil
	}
	maxCandidates := query.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = defaultMaxQueryCandidates
	}
	groups := normalizedGroups(query.Groups)
	if len(groups) == 0 {
		return nil
	}
	modelNames := candidateModelNames(query.ModelName)
	results := make([]core.Candidate, 0, maxCandidates)
	needsDedupe := len(groups)*len(modelNames) > 1
	var seenRuntimeKeys map[string]struct{}
	if needsDedupe {
		seenRuntimeKeys = make(map[string]struct{}, maxCandidates)
	}
	for _, group := range groups {
		for _, modelName := range modelNames {
			items := current.byGroupModel[groupModelKey(group, modelName)]
			for _, candidate := range items {
				if len(results) >= maxCandidates {
					return results
				}
				if !candidateMatchesQuery(candidate, query) {
					continue
				}
				if len(query.ExcludedRuntimeKeyIDs) > 0 {
					if _, excluded := query.ExcludedRuntimeKeyIDs[runtimeKeyID(candidate.RuntimeKey)]; excluded {
						continue
					}
				}
				if needsDedupe {
					keyID := runtimeKeyID(candidate.RuntimeKey)
					if _, seen := seenRuntimeKeys[keyID]; seen {
						continue
					}
					seenRuntimeKeys[keyID] = struct{}{}
				}
				results = append(results, prepareCandidateForQuery(candidate, query))
			}
			if len(results) >= maxCandidates {
				return results
			}
		}
	}
	return results
}

func (i *CandidateIndex) Stats() IndexStats {
	if i == nil {
		return IndexStats{}
	}
	current := i.snapshot.Load()
	if current == nil {
		return IndexStats{}
	}
	return current.stats
}

type snapshot struct {
	byGroupModel map[string][]core.Candidate
	stats        IndexStats
}

type snapshotBuilder struct {
	registry         Registry
	providerRegistry provider.ProviderRegistry
	next             *snapshot
}

func newSnapshotBuilder(registry Registry, providerRegistry provider.ProviderRegistry) *snapshotBuilder {
	return &snapshotBuilder{
		registry:         registry,
		providerRegistry: providerRegistry,
		next: &snapshot{
			byGroupModel: make(map[string][]core.Candidate),
			stats:        IndexStats{},
		},
	}
}

func (b *snapshotBuilder) Build(channels []*model.Channel) *snapshot {
	if b == nil || b.next == nil {
		return &snapshot{byGroupModel: make(map[string][]core.Candidate)}
	}
	for _, channel := range channels {
		b.addChannel(channel)
	}
	b.sort()
	b.next.stats.GroupModels = len(b.next.byGroupModel)
	groupSet := make(map[string]struct{})
	for key := range b.next.byGroupModel {
		if group, _, ok := strings.Cut(key, "\x00"); ok {
			groupSet[group] = struct{}{}
		}
	}
	b.next.stats.Groups = len(groupSet)
	return b.next
}

func (b *snapshotBuilder) addChannel(channel *model.Channel) {
	if channel == nil || channel.Id <= 0 {
		return
	}
	b.next.stats.Channels++
	accounts := b.registry.AccountsForChannel(channel)
	if channel.Status != common.ChannelStatusEnabled {
		if len(accounts) == 0 {
			b.next.stats.SkippedKeys++
			return
		}
		b.next.stats.Accounts += len(accounts)
		b.next.stats.DisabledKeys += len(accounts)
		return
	}
	if len(accounts) == 0 {
		b.next.stats.SkippedKeys++
		return
	}
	groups := channel.GetGroups()
	models := channel.GetModels()
	if len(groups) == 0 || len(models) == 0 {
		b.next.stats.SkippedKeys += len(accounts)
		return
	}
	enabledAccounts := make([]account.ChannelAccount, 0, len(accounts))
	for _, accountRef := range accounts {
		b.next.stats.Accounts++
		if !accountRef.KeyEnabled {
			b.next.stats.DisabledKeys++
			continue
		}
		enabledAccounts = append(enabledAccounts, accountRef)
	}
	baseCandidates := b.buildChannelCandidates(channel, groups, models)
	if len(baseCandidates) == 0 {
		b.next.stats.SkippedKeys += len(accounts)
		return
	}
	if len(enabledAccounts) == 0 {
		return
	}
	for _, accountRef := range enabledAccounts {
		for _, baseCandidate := range baseCandidates {
			candidate := baseCandidate
			account.CandidateAccountFields(&candidate, accountRef)
			b.next.byGroupModel[groupModelKey(candidate.RuntimeKey.Group, candidate.RuntimeKey.RequestedModel)] = append(b.next.byGroupModel[groupModelKey(candidate.RuntimeKey.Group, candidate.RuntimeKey.RequestedModel)], candidate)
			b.next.stats.Candidates++
		}
	}
}

func (b *snapshotBuilder) buildChannelCandidates(channel *model.Channel, groups []string, models []string) []core.Candidate {
	candidates := make([]core.Candidate, 0, len(groups)*len(models))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		for _, modelName := range models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			if !service.ChannelSupportsRequiredCapabilities(channel, modelName, "", false) {
				continue
			}
			candidates = append(candidates, b.buildChannelCandidate(channel, group, modelName))
		}
	}
	return candidates
}

func (b *snapshotBuilder) buildChannelCandidate(channel *model.Channel, group string, modelName string) core.Candidate {
	profile := b.providerProfile(channel, modelName)
	capability := profile.Capabilities(channel, modelName)
	upstreamModel := channel.ResolveMappedModelName(modelName)
	return core.Candidate{
		Channel:         channel,
		Group:           group,
		UpstreamModel:   upstreamModel,
		ProviderProfile: profile.Name(),
		ProxyMode:       profile.ProxyMode(channel, modelName),
		PoolLevel:       core.CandidatePoolPro,
		RuntimeKey: core.RuntimeKey{
			RequestedModel:        modelName,
			UpstreamModel:         upstreamModel,
			ChannelID:             channel.Id,
			Group:                 group,
			EndpointType:          "",
			CapabilityFingerprint: capability.CapabilityFingerprint,
		},
	}
}

func (b *snapshotBuilder) providerProfile(channel *model.Channel, modelName string) provider.ProviderProfile {
	registry := b.providerRegistry
	if registry == nil {
		registry = provider.NewStandardProviderRegistry()
	}
	if profile := registry.Best(channel, modelName); profile != nil {
		return profile
	}
	return provider.NewStandardOpenAICompatibleProfile()
}

func (b *snapshotBuilder) sort() {
	for key, candidates := range b.next.byGroupModel {
		sort.SliceStable(candidates, func(left, right int) bool {
			leftCandidate := candidates[left]
			rightCandidate := candidates[right]
			leftPriority := int64(0)
			rightPriority := int64(0)
			if leftCandidate.Channel != nil {
				leftPriority = leftCandidate.Channel.GetPriority()
			}
			if rightCandidate.Channel != nil {
				rightPriority = rightCandidate.Channel.GetPriority()
			}
			if leftPriority != rightPriority {
				return leftPriority > rightPriority
			}
			if leftCandidate.RuntimeKey.ChannelID != rightCandidate.RuntimeKey.ChannelID {
				return leftCandidate.RuntimeKey.ChannelID < rightCandidate.RuntimeKey.ChannelID
			}
			return leftCandidate.RuntimeKey.CredentialIndex < rightCandidate.RuntimeKey.CredentialIndex
		})
		b.next.byGroupModel[key] = candidates
	}
}

func candidateMatchesQuery(candidate core.Candidate, query Query) bool {
	if query.EndpointType != "" && !service.ChannelSupportsRequiredCapabilities(candidate.Channel, candidate.RuntimeKey.RequestedModel, query.EndpointType, query.RequiresCodexImageTool) {
		return false
	}
	if !candidateProxyAvailable(candidate) {
		return false
	}
	if !candidateAccountSupportsRequiredCapabilities(candidate, query) {
		return false
	}
	if len(query.ExcludedChannelIDs) > 0 {
		if _, excluded := query.ExcludedChannelIDs[candidate.RuntimeKey.ChannelID]; excluded {
			return false
		}
	}
	if len(query.ExcludedAccountIDs) > 0 {
		if _, excluded := query.ExcludedAccountIDs[candidate.RuntimeKey.AccountID]; excluded {
			return false
		}
	}
	if query.Provider != "" && !strings.EqualFold(query.Provider, candidate.AccountIdentity.Provider) {
		return false
	}
	if query.Brand != "" && !strings.EqualFold(query.Brand, candidate.AccountIdentity.Brand) {
		return false
	}
	if query.CapabilityFingerprint != "" && query.CapabilityFingerprint != candidate.RuntimeKey.CapabilityFingerprint {
		return false
	}
	return true
}

func candidateProxyAvailable(candidate core.Candidate) bool {
	proxyID := candidate.ProxyRef.ProxyID
	if proxyID <= 0 {
		return true
	}
	proxy, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil || proxy == nil || !proxy.Enabled {
		return false
	}
	proxyURL, err := proxy.ProxyURL()
	return err == nil && strings.TrimSpace(proxyURL) != ""
}

func candidateAccountSupportsRequiredCapabilities(candidate core.Candidate, query Query) bool {
	channel := candidate.Channel
	usesCodexBackend, applies := candidateUsesCodexBackendForEndpoint(candidate, query.EndpointType)
	if !applies {
		return true
	}
	if !usesCodexBackend {
		return service.ChannelSupportsRequiredEndpoint(channel, candidate.RuntimeKey.RequestedModel, query.EndpointType)
	}
	if channel == nil || len(channel.ChannelInfo.MultiKeyCapabilities) == 0 {
		return true
	}
	capability, ok := channel.ChannelInfo.MultiKeyCapabilities[candidate.CredentialRef.CredentialIndex]
	if !ok {
		return true
	}
	if !service.ChannelAccountCapabilityAllowsScheduling(capability) {
		return false
	}
	if query.EndpointType == constant.EndpointTypeOpenAIResponseCompact {
		return capability.CodexBackendCompactWrite == nil || capability.HasCodexBackendCompactAllowed()
	}
	if capability.CodexBackendResponsesStreamWrite == nil {
		return true
	}
	return capability.HasCodexBackendResponsesStreamAllowed()
}

func candidateUsesCodexBackendForEndpoint(candidate core.Candidate, endpointType constant.EndpointType) (bool, bool) {
	channel := candidate.Channel
	if channel == nil {
		return false, false
	}
	switch endpointType {
	case constant.EndpointTypeOpenAIResponse, constant.EndpointTypeOpenAIResponseCompact:
	default:
		return false, false
	}
	if channel.Type == constant.ChannelTypeCodex {
		return true, true
	}
	if channel.Type != constant.ChannelTypeOpenAI {
		return false, false
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return false, false
	}
	hasCodexOAuthCredential := false
	for _, key := range keys {
		if codexauth.IsOAuthJSONCredential(key) {
			hasCodexOAuthCredential = true
			break
		}
	}
	if !hasCodexOAuthCredential {
		return false, false
	}
	index := candidate.CredentialRef.CredentialIndex
	if !channel.ChannelInfo.IsMultiKey {
		index = 0
	}
	if index < 0 || index >= len(keys) {
		return false, true
	}
	return codexauth.IsOAuthJSONCredential(keys[index]), true
}

func prepareCandidateForQuery(candidate core.Candidate, query Query) core.Candidate {
	candidate.RequiresCodexImageTool = query.RequiresCodexImageTool
	candidate.RuntimeKey.EndpointType = query.EndpointType
	requestedModel := strings.TrimSpace(query.ModelName)
	if requestedModel != "" {
		candidate.RuntimeKey.RequestedModel = requestedModel
		if candidate.Channel != nil {
			candidate.UpstreamModel = candidate.Channel.ResolveMappedModelName(requestedModel)
			candidate.RuntimeKey.UpstreamModel = candidate.UpstreamModel
		}
	}
	return candidate
}

func normalizedGroups(groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(groups))
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return result
}

func candidateModelNames(modelName string) []string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	names := []string{modelName}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		names = append(names, normalized)
	}
	return names
}

func groupModelKey(group string, modelName string) string {
	return strings.TrimSpace(group) + "\x00" + strings.TrimSpace(modelName)
}

func runtimeKeyID(key core.RuntimeKey) string {
	parts := []string{
		key.RequestedModel,
		key.UpstreamModel,
		key.Group,
		string(key.EndpointType),
		key.CapabilityFingerprint,
		key.ResourceID,
		key.AccountID,
		key.CredentialSubjectFP,
		key.CredentialFP,
	}
	return strings.Join(parts, "\x00")
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
