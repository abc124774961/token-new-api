package scheduler

import (
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
)

func serviceRuntimeIdentityFromKey(key core.RuntimeKey) service.ChannelRuntimeIdentity {
	key = normalizeRuntimeKey(key)
	identity := service.ChannelRuntimeIdentity{
		ChannelID:           key.ChannelID,
		RequestedModel:      key.RequestedModel,
		SelectedGroup:       key.Group,
		EndpointType:        key.EndpointType,
		AccountID:           key.AccountID,
		CredentialIndex:     key.CredentialIndex,
		CredentialSubjectFP: key.CredentialSubjectFP,
		CredentialFP:        key.CredentialFP,
	}
	if key.AccountID != "" || key.CredentialSubjectFP != "" || key.CredentialFP != "" || key.ResourceID != "" || key.CredentialIndex > 0 {
		identity.CredentialIndexSet = true
	}
	return identity.Normalize()
}

func serviceRuntimeIdentityFromCandidate(candidate core.Candidate, snapshot core.RuntimeSnapshot) service.ChannelRuntimeIdentity {
	key := snapshot.Key
	if key.ChannelID <= 0 {
		key = candidate.RuntimeKey
	}
	if key.ChannelID <= 0 && candidate.Channel != nil {
		key.ChannelID = candidate.Channel.Id
	}
	if key.RequestedModel == "" {
		key.RequestedModel = candidate.RuntimeKey.RequestedModel
	}
	if key.UpstreamModel == "" {
		key.UpstreamModel = candidate.RuntimeKey.UpstreamModel
	}
	if key.Group == "" {
		key.Group = candidate.Group
	}
	if key.EndpointType == "" {
		key.EndpointType = candidate.RuntimeKey.EndpointType
	}
	if key.AccountID == "" {
		key.AccountID = candidate.RuntimeKey.AccountID
	}
	if key.AccountID == "" {
		key.AccountID = candidate.AccountIdentity.AccountID
	}
	if key.AccountID == "" {
		key.AccountID = candidate.CredentialRef.AccountID
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = candidate.RuntimeKey.CredentialSubjectFP
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = candidate.AccountIdentity.CredentialSubjectFingerprint
	}
	if key.CredentialSubjectFP == "" {
		key.CredentialSubjectFP = candidate.CredentialRef.CredentialSubjectFingerprint
	}
	if key.CredentialFP == "" {
		key.CredentialFP = candidate.RuntimeKey.CredentialFP
	}
	if key.CredentialFP == "" {
		key.CredentialFP = candidate.AccountIdentity.CredentialFingerprint
	}
	if key.CredentialFP == "" {
		key.CredentialFP = candidate.CredentialRef.CredentialFingerprint
	}
	if key.CredentialIndex == 0 && candidate.RuntimeKey.CredentialIndex != 0 {
		key.CredentialIndex = candidate.RuntimeKey.CredentialIndex
	}
	if key.CredentialIndex == 0 && candidate.AccountIdentity.CredentialIndex != 0 {
		key.CredentialIndex = candidate.AccountIdentity.CredentialIndex
	}
	if key.CredentialIndex == 0 && candidate.CredentialRef.CredentialIndex != 0 {
		key.CredentialIndex = candidate.CredentialRef.CredentialIndex
	}
	identity := serviceRuntimeIdentityFromKey(key)
	if !identity.CredentialIndexSet && candidateHasCredentialScope(candidate) {
		identity.CredentialIndexSet = true
		if candidate.AccountIdentity.CredentialIndex != 0 {
			identity.CredentialIndex = candidate.AccountIdentity.CredentialIndex
		} else if candidate.CredentialRef.CredentialIndex != 0 {
			identity.CredentialIndex = candidate.CredentialRef.CredentialIndex
		}
	}
	return identity.Normalize()
}

func candidateHasCredentialScope(candidate core.Candidate) bool {
	return candidate.AccountIdentity.AccountID != "" ||
		candidate.AccountIdentity.AccountIdentityKey != "" ||
		candidate.AccountIdentity.AccountUniqueKey != "" ||
		candidate.AccountIdentity.CredentialSubjectFingerprint != "" ||
		candidate.AccountIdentity.CredentialFingerprint != "" ||
		candidate.CredentialRef.ResourceID != "" ||
		candidate.CredentialRef.AccountID != "" ||
		candidate.CredentialRef.CredentialSubjectFingerprint != "" ||
		candidate.CredentialRef.CredentialFingerprint != "" ||
		candidate.CredentialRef.Resolver != ""
}
