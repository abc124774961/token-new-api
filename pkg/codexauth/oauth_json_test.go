package codexauth

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestParseOAuthJSONCredentialLooseAcceptsSub2APIWithoutAccountID(t *testing.T) {
	accessToken := testJWT(t, map[string]interface{}{
		"iss": "https://auth.openai.com",
		"exp": float64(1780000000),
		"https://api.openai.com/profile": map[string]interface{}{
			"email": "user@example.com",
		},
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_user_id": "user-123",
		},
	})
	credential, ok := ParseOAuthJSONCredentialLoose(`{"access_token":"` + accessToken + `","email":"USER@example.com"}`)

	require.True(t, ok)
	require.Equal(t, "user@example.com", credential.Email)
	require.Equal(t, "user-123", credential.ChatGPTUserID)
	require.Empty(t, credential.AccountID)
	require.True(t, LooksLikeOAuthJSONCredential(`{"access_token":"`+accessToken+`","email":"USER@example.com"}`))
	_, complete := ParseOAuthJSONCredential(`{"access_token":"` + accessToken + `","email":"USER@example.com"}`)
	require.False(t, complete)
}

func TestNormalizeOAuthJSONCredentialMapFillsChatGPTAccountID(t *testing.T) {
	payload := map[string]interface{}{
		"access_token":       "access",
		"chatgpt_account_id": "acct-123",
		"email":              "USER@example.com",
		"expires_at":         float64(1781145008),
	}

	require.True(t, NormalizeOAuthJSONCredentialMap(payload))
	require.Equal(t, "acct-123", payload["account_id"])
	require.Equal(t, "user@example.com", payload["email"])
	require.Equal(t, "2026-06-11T02:30:08Z", payload["expires_at"])
}

func testJWT(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	header, err := common.Marshal(map[string]interface{}{"alg": "none"})
	require.NoError(t, err)
	payload, err := common.Marshal(claims)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
