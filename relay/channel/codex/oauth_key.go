package codex

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type OAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	AccountID        string `json:"account_id,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	LastRefresh      string `json:"last_refresh,omitempty"`
	Email            string `json:"email,omitempty"`
	Type             string `json:"type,omitempty"`
	Expired          string `json:"expired,omitempty"`
}

func ParseOAuthKey(raw string) (*OAuthKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("codex channel: empty oauth key")
	}
	var payload map[string]interface{}
	if err := common.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, errors.New("codex channel: invalid oauth key json")
	}
	key := OAuthKey{
		IDToken:          stringValue(payload["id_token"]),
		AccessToken:      stringValue(payload["access_token"]),
		RefreshToken:     stringValue(payload["refresh_token"]),
		AccountID:        stringValue(payload["account_id"]),
		ChatGPTAccountID: stringValue(payload["chatgpt_account_id"]),
		LastRefresh:      stringValue(payload["last_refresh"]),
		Email:            stringValue(payload["email"]),
		Type:             stringValue(payload["type"]),
		Expired:          stringValue(payload["expired"]),
	}
	if key.AccountID == "" {
		key.AccountID = key.ChatGPTAccountID
	}
	if key.Expired == "" {
		key.Expired = stringValue(payload["expires_at"])
	}
	return &key, nil
}

func stringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}
