package integration

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/service"
)

const defaultMaxCandidatesPerGroup = 128

type ModelCandidatePoolBuilder struct {
	MaxCandidatesPerGroup int
	ProviderRegistry      provider.ProviderRegistry
}

func NewModelCandidatePoolBuilder() *ModelCandidatePoolBuilder {
	return &ModelCandidatePoolBuilder{
		MaxCandidatesPerGroup: defaultMaxCandidatesPerGroup,
		ProviderRegistry:      provider.NewStandardProviderRegistry(),
	}
}

func (b *ModelCandidatePoolBuilder) Build(req *core.DispatchRequest, policy core.GroupSmartPolicy) []core.Candidate {
	if req == nil {
		return nil
	}
	groups := policy.CandidateGroups
	if len(groups) == 0 && req.RequestedGroup != "" && req.RequestedGroup != "auto" {
		groups = []string{req.RequestedGroup}
	}
	if len(groups) == 0 {
		return nil
	}
	maxPerGroup := b.MaxCandidatesPerGroup
	if maxPerGroup <= 0 {
		maxPerGroup = defaultMaxCandidatesPerGroup
	}
	candidates := make([]core.Candidate, 0)
	for _, group := range groups {
		groupCandidates := b.buildGroupCandidates(req, group, maxPerGroup)
		candidates = append(candidates, groupCandidates...)
	}
	return candidates
}

func (b *ModelCandidatePoolBuilder) buildGroupCandidates(req *core.DispatchRequest, group string, maxCandidates int) []core.Candidate {
	excluded := map[int]struct{}{}
	candidates := make([]core.Candidate, 0)
	for len(excluded) < maxCandidates {
		channel, err := model.GetRandomSatisfiedChannel(group, req.ModelName, req.Retry, excluded)
		if err != nil || channel == nil {
			break
		}
		excluded[channel.Id] = struct{}{}
		if !service.ChannelSupportsRequiredCapabilities(channel, req.ModelName, req.EndpointType, req.RequiresCodexImageTool) {
			continue
		}
		upstreamModel := channel.ResolveMappedModelName(req.ModelName)
		profile := b.providerProfile(channel, req.ModelName)
		capability := profile.Capabilities(channel, req.ModelName)
		candidates = append(candidates, core.Candidate{
			Channel:         channel,
			Group:           group,
			UpstreamModel:   upstreamModel,
			ProviderProfile: profile.Name(),
			ProxyMode:       profile.ProxyMode(channel, req.ModelName),
			RuntimeKey: core.RuntimeKey{
				RequestedModel:        req.ModelName,
				UpstreamModel:         upstreamModel,
				ChannelID:             channel.Id,
				Group:                 group,
				EndpointType:          req.EndpointType,
				CapabilityFingerprint: capability.CapabilityFingerprint,
			},
		})
	}
	return candidates
}

func (b *ModelCandidatePoolBuilder) providerProfile(channel *model.Channel, modelName string) provider.ProviderProfile {
	registry := b.ProviderRegistry
	if registry == nil {
		registry = provider.NewStandardProviderRegistry()
	}
	if profile := registry.Best(channel, modelName); profile != nil {
		return profile
	}
	return provider.NewStandardOpenAICompatibleProfile()
}

var _ core.CandidatePoolBuilder = (*ModelCandidatePoolBuilder)(nil)
