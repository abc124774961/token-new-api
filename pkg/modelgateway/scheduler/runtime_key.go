package scheduler

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

func normalizeRuntimeKey(key core.RuntimeKey) core.RuntimeKey {
	key.RequestedModel = strings.TrimSpace(key.RequestedModel)
	key.UpstreamModel = strings.TrimSpace(key.UpstreamModel)
	key.ResourceID = strings.TrimSpace(key.ResourceID)
	key.ResourceType = strings.TrimSpace(key.ResourceType)
	key.AccountID = strings.TrimSpace(key.AccountID)
	key.AccountType = strings.TrimSpace(key.AccountType)
	key.Brand = strings.TrimSpace(key.Brand)
	key.Provider = strings.TrimSpace(key.Provider)
	key.CredentialSubjectFP = strings.TrimSpace(key.CredentialSubjectFP)
	key.CredentialFP = strings.TrimSpace(key.CredentialFP)
	key.Group = strings.TrimSpace(key.Group)
	key.EndpointType = constant.EndpointType(strings.TrimSpace(string(key.EndpointType)))
	key.CapabilityFingerprint = strings.TrimSpace(key.CapabilityFingerprint)
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	return key
}

func normalizeRuntimeSnapshot(snapshot core.RuntimeSnapshot) core.RuntimeSnapshot {
	snapshot.Key = normalizeRuntimeKey(snapshot.Key)
	if snapshot.MatchedRuntimeKey.ChannelID > 0 {
		snapshot.MatchedRuntimeKey = normalizeRuntimeKey(snapshot.MatchedRuntimeKey)
	}
	snapshot.ProbeTriggerReason = strings.TrimSpace(snapshot.ProbeTriggerReason)
	return snapshot
}

func normalizeCircuitSnapshot(snapshot core.CircuitSnapshot) core.CircuitSnapshot {
	snapshot.Key = normalizeRuntimeKey(snapshot.Key)
	return snapshot
}
