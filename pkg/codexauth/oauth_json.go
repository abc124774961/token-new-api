package codexauth

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type OAuthJSONCredential struct {
	AccessToken      string `json:"access_token,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	Email            string `json:"email,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
}

func ParseOAuthJSONCredential(raw string) (OAuthJSONCredential, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return OAuthJSONCredential{}, false
	}
	var credential OAuthJSONCredential
	if err := common.Unmarshal([]byte(raw), &credential); err != nil {
		return OAuthJSONCredential{}, false
	}
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	credential.AccountID = strings.TrimSpace(credential.AccountID)
	if credential.AccountID == "" {
		credential.AccountID = strings.TrimSpace(credential.ChatGPTAccountID)
	}
	credential.Email = strings.TrimSpace(strings.ToLower(credential.Email))
	credential.RefreshToken = strings.TrimSpace(credential.RefreshToken)
	if credential.AccessToken == "" || credential.AccountID == "" {
		return OAuthJSONCredential{}, false
	}
	return credential, true
}

func IsOAuthJSONCredential(raw string) bool {
	_, ok := ParseOAuthJSONCredential(raw)
	return ok
}
