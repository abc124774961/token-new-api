package codex

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOAuthKeyAcceptsChatGPTAccountIDFallback(t *testing.T) {
	key, err := ParseOAuthKey(`{
		"access_token": "access-token",
		"refresh_token": "refresh-token",
		"chatgpt_account_id": "acct-from-chatgpt",
		"expires_at": "2026-06-11T12:00:00Z"
	}`)

	require.NoError(t, err)
	require.Equal(t, "access-token", key.AccessToken)
	require.Equal(t, "refresh-token", key.RefreshToken)
	require.Equal(t, "acct-from-chatgpt", key.AccountID)
	require.Equal(t, "2026-06-11T12:00:00Z", key.Expired)
}

func TestParseOAuthKeyRejectsInvalidJSON(t *testing.T) {
	_, err := ParseOAuthKey(`{client_header:Authorization}`)

	require.EqualError(t, err, "codex channel: invalid oauth key json")
}
