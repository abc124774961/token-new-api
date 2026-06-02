package scheduler

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

type StaticCandidatePoolBuilder struct {
	candidates []core.Candidate
}

func NewStaticCandidatePoolBuilder(candidates []core.Candidate) *StaticCandidatePoolBuilder {
	return &StaticCandidatePoolBuilder{candidates: append([]core.Candidate(nil), candidates...)}
}

func NewStaticCandidatePoolBuilderFromChannels(channels []*model.Channel, groups map[int]string) *StaticCandidatePoolBuilder {
	candidates := make([]core.Candidate, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		group := groups[channel.Id]
		if group == "" {
			group = channel.Group
		}
		candidates = append(candidates, core.Candidate{
			Channel: channel,
			Group:   group,
		})
	}
	return NewStaticCandidatePoolBuilder(candidates)
}

func (b *StaticCandidatePoolBuilder) Build(req *core.DispatchRequest, policy core.GroupSmartPolicy) []core.Candidate {
	if b == nil {
		return nil
	}
	allowedGroups := allowedGroupSet(policy)
	candidates := make([]core.Candidate, 0, len(b.candidates))
	for _, candidate := range b.candidates {
		if candidate.Channel == nil {
			continue
		}
		if len(allowedGroups) > 0 {
			if _, ok := allowedGroups[candidate.Group]; !ok {
				continue
			}
		}
		if candidate.RuntimeKey.ChannelID == 0 {
			candidate.RuntimeKey.ChannelID = candidate.Channel.Id
		}
		if candidate.RuntimeKey.Group == "" {
			candidate.RuntimeKey.Group = candidate.Group
		}
		if req != nil {
			if candidate.RuntimeKey.RequestedModel == "" {
				candidate.RuntimeKey.RequestedModel = req.ModelName
			}
			if candidate.RuntimeKey.EndpointType == "" {
				candidate.RuntimeKey.EndpointType = req.EndpointType
			}
			candidate.RequiresCodexImageTool = false
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func allowedGroupSet(policy core.GroupSmartPolicy) map[string]struct{} {
	if len(policy.CandidateGroups) == 0 {
		if policy.RequestedGroup == "" || policy.RequestedGroup == "auto" {
			return nil
		}
		return map[string]struct{}{policy.RequestedGroup: {}}
	}
	groups := make(map[string]struct{}, len(policy.CandidateGroups))
	for _, group := range policy.CandidateGroups {
		groups[group] = struct{}{}
	}
	return groups
}

var _ core.CandidatePoolBuilder = (*StaticCandidatePoolBuilder)(nil)
