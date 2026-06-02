package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestNotifyTokenAccountAutomationAuthInvalidPostsEvent(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotEvent TokenAccountAutomationAuthInvalidEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, common.DecodeJson(r.Body, &gotEvent))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	t.Setenv(tokenAccountAutomationURLEnv, server.URL)
	t.Setenv(tokenAccountAutomationAPITokenEnv, "api-token")
	t.Setenv(tokenAccountAutomationTimeoutSecondsEnv, "1")

	err := NotifyTokenAccountAutomationAuthInvalid(context.Background(), TokenAccountAutomationAuthInvalidEvent{
		ChannelID:       12,
		CredentialIndex: 3,
		Provider:        "codex_oauth",
		SubjectKey:      "account-key",
		Source:          "relay",
		Reason:          "auth_config_error",
	})

	require.NoError(t, err)
	require.Equal(t, "/api/events/account-auth-invalid", gotPath)
	require.Equal(t, "Bearer api-token", gotAuth)
	require.Equal(t, 12, gotEvent.ChannelID)
	require.Equal(t, 3, gotEvent.CredentialIndex)
	require.Equal(t, "codex_oauth", gotEvent.Provider)
	require.Equal(t, "account-key", gotEvent.SubjectKey)
}

func TestNotifyTokenAccountAutomationAuthInvalidNoopsWhenUnconfigured(t *testing.T) {
	t.Setenv(tokenAccountAutomationURLEnv, "")
	t.Setenv(tokenAccountAutomationAPITokenEnv, "")

	err := NotifyTokenAccountAutomationAuthInvalid(context.Background(), TokenAccountAutomationAuthInvalidEvent{
		ChannelID:       12,
		CredentialIndex: 3,
	})

	require.NoError(t, err)
	require.False(t, TokenAccountAutomationConfigured())
}
