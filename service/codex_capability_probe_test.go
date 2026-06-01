package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProbeCodexOAuthAccountCapabilitiesRequiresAccountIDForBackend(t *testing.T) {
	channel := &model.Channel{
		Id:   7,
		Type: constant.ChannelTypeOpenAI,
		Key:  `{"access_token":"access","email":"user@example.com","chatgpt_user_id":"user-123"}`,
	}

	_, err := ProbeCodexOAuthAccountCapabilities(context.Background(), channel, 0, CodexCapabilityProbeOptions{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id/chatgpt_account_id")
}

func TestParseCodexProbeCredentialKeepsRefreshTokenOnlyCardPayload(t *testing.T) {
	credential, err := parseCodexProbeCredential(`{"account_id":"acct-card","chatgpt_account_id":"acct-card","email":"user@example.com","refresh_token":"refresh-token","type":"codex"}`)

	require.NoError(t, err)
	require.Equal(t, "acct-card", credential.AccountID)
	require.Empty(t, credential.AccessToken)
	require.Equal(t, "refresh-token", credential.RefreshToken)
}
