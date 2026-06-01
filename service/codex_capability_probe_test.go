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
