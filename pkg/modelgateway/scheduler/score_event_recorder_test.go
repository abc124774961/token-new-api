package scheduler

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/stretchr/testify/require"
)

func TestScoreEventFromAdjustmentCarriesAccountScopeAndSwitchReason(t *testing.T) {
	key := core.RuntimeKey{
		RequestedModel:        "gpt-5.4",
		UpstreamModel:         "gpt-5.4",
		ChannelID:             77,
		ResourceID:            "platform:channel:77",
		ResourceType:          core.ResourceTypePlatformOwned,
		AccountID:             "openai:openai:acct-1",
		AccountType:           core.AccountTypeOAuthAccount,
		Brand:                 "openai",
		Provider:              "openai",
		CredentialIndex:       2,
		CredentialSubjectFP:   "subject-fp",
		CredentialFP:          "credential-fp",
		Group:                 "default",
		EndpointType:          constant.EndpointTypeOpenAI,
		CapabilityFingerprint: "openai_codex|native",
	}
	result := core.AttemptResult{
		RequestID:     "req-77",
		AttemptIndex:  1,
		ChannelID:     77,
		FailureScope:  core.FailureScopeAccount,
		SwitchReason:  core.RelayAttemptCancelReasonFirstByteTimeout,
		Strategy:      core.StrategyBalanced,
		AutoMode:      core.AutoModeFusion,
		ObservedAt:    time.Unix(1710000000, 0),
		IsHealthProbe: true,
	}
	snapshot := core.RuntimeSnapshot{Key: key, SampleCount: 3}
	adjustment := core.ScoreAdjustment{
		TraceID:     "trace-77",
		BeforeTotal: 0.92,
		AfterTotal:  0.81,
		Delta:       -0.11,
	}

	event := scoreEventFromAdjustment(result, snapshot, adjustment)

	require.Equal(t, "trace-77", event.TraceID)
	require.Equal(t, "req-77", event.RequestID)
	require.Equal(t, 1, event.AttemptIndex)
	require.Equal(t, 77, event.ChannelID)
	require.Equal(t, key.ResourceID, event.ResourceID)
	require.Equal(t, key.ResourceType, event.ResourceType)
	require.Equal(t, key.AccountID, event.AccountID)
	require.Equal(t, key.AccountType, event.AccountType)
	require.Equal(t, key.Brand, event.Brand)
	require.Equal(t, key.Provider, event.Provider)
	require.Equal(t, key.CredentialIndex, event.CredentialIndex)
	require.Equal(t, key.CredentialSubjectFP, event.CredentialSubjectFP)
	require.Equal(t, key.CredentialFP, event.CredentialFP)
	require.Equal(t, core.FailureScopeAccount, event.FailureScope)
	require.Equal(t, core.RelayAttemptCancelReasonFirstByteTimeout, event.SwitchReason)
	require.True(t, event.IsHealthProbe)
	require.Equal(t, int64(1710000000), event.CreatedAt)
	require.Contains(t, event.ContextJSON, `"runtime_key"`)
	require.Contains(t, event.ContextJSON, `"account_id":"openai:openai:acct-1"`)
}
