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

func TestParseOAuthJSONCredentialAcceptsTeamAccountJWTClaims(t *testing.T) {
	accessToken := testJWT(t, map[string]interface{}{
		"aud":       []interface{}{"https://api.openai.com/v1"},
		"client_id": "app_fake_client",
		"exp":       float64(1781221788),
		"iat":       float64(1780357787),
		"iss":       "https://auth.openai.com",
		"sub":       "auth0|fake-user",
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id":        "acct-team-123",
			"chatgpt_account_user_id":   "user-fake__acct-team-123",
			"chatgpt_compute_residency": "no_constraint",
			"chatgpt_plan_type":         "team",
			"chatgpt_user_id":           "user-fake",
			"localhost":                 true,
			"user_id":                   "user-fake",
		},
		"https://api.openai.com/profile": map[string]interface{}{
			"email":          "TEAM-USER@example.com",
			"email_verified": true,
		},
	})
	payload := map[string]interface{}{
		"type":          "codex",
		"access_token":  accessToken,
		"refresh_token": "rt_fake_refresh",
	}
	raw, err := common.Marshal(payload)
	require.NoError(t, err)

	credential, ok := ParseOAuthJSONCredential(string(raw))
	require.True(t, ok)
	require.Equal(t, "acct-team-123", credential.AccountID)
	require.Equal(t, "acct-team-123", credential.ChatGPTAccountID)
	require.Equal(t, "team-user@example.com", credential.Email)
	require.Equal(t, "user-fake", credential.ChatGPTUserID)
	require.Equal(t, "team", credential.ChatGPTPlanType)
	require.Equal(t, "2026-06-11T23:49:48Z", credential.ExpiresAt)
}

func TestNormalizeOAuthJSONCredentialMapFillsTeamPlanTypeFromJWTClaims(t *testing.T) {
	accessToken := testJWT(t, map[string]interface{}{
		"iss": "https://auth.openai.com",
		"exp": float64(1781221788),
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-team-123",
			"chatgpt_plan_type":  "team",
			"chatgpt_user_id":    "user-fake",
		},
		"https://api.openai.com/profile": map[string]interface{}{
			"email": "TEAM-USER@example.com",
		},
	})
	payload := map[string]interface{}{
		"type":          "codex",
		"access_token":  accessToken,
		"refresh_token": "rt_fake_refresh",
	}

	require.True(t, NormalizeOAuthJSONCredentialMap(payload))
	require.Equal(t, "acct-team-123", payload["account_id"])
	require.Equal(t, "acct-team-123", payload["chatgpt_account_id"])
	require.Equal(t, "team-user@example.com", payload["email"])
	require.Equal(t, "user-fake", payload["chatgpt_user_id"])
	require.Equal(t, "team", payload["chatgpt_plan_type"])
	require.Equal(t, "2026-06-11T23:49:48Z", payload["expires_at"])
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
