package codexauth

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type OAuthJSONCredential struct {
	AccessToken      string `json:"access_token,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	Email            string `json:"email,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	IDToken          string `json:"id_token,omitempty"`
	ChatGPTUserID    string `json:"chatgpt_user_id,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
}

func ParseOAuthJSONCredential(raw string) (OAuthJSONCredential, bool) {
	credential, ok := ParseOAuthJSONCredentialLoose(raw)
	if !ok {
		return OAuthJSONCredential{}, false
	}
	if credential.AccessToken == "" || credential.AccountID == "" {
		return OAuthJSONCredential{}, false
	}
	return credential, true
}

func ParseOAuthJSONCredentialLoose(raw string) (OAuthJSONCredential, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return OAuthJSONCredential{}, false
	}
	var payload map[string]interface{}
	if err := common.Unmarshal([]byte(raw), &payload); err != nil {
		return OAuthJSONCredential{}, false
	}
	credential := OAuthJSONCredential{
		AccessToken:      stringClaim(payload, "access_token"),
		AccountID:        stringClaim(payload, "account_id"),
		ChatGPTAccountID: stringClaim(payload, "chatgpt_account_id"),
		Email:            stringClaim(payload, "email"),
		RefreshToken:     stringClaim(payload, "refresh_token"),
		IDToken:          stringClaim(payload, "id_token"),
		ChatGPTUserID:    stringClaim(payload, "chatgpt_user_id"),
		ExpiresAt:        expiresAtClaim(payload, "expires_at"),
	}
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	credential.AccountID = strings.TrimSpace(credential.AccountID)
	if credential.AccountID == "" {
		credential.AccountID = strings.TrimSpace(credential.ChatGPTAccountID)
	}
	credential.ChatGPTAccountID = strings.TrimSpace(credential.ChatGPTAccountID)
	credential.Email = strings.TrimSpace(strings.ToLower(credential.Email))
	credential.RefreshToken = strings.TrimSpace(credential.RefreshToken)
	credential.IDToken = strings.TrimSpace(credential.IDToken)
	credential.ChatGPTUserID = strings.TrimSpace(credential.ChatGPTUserID)
	credential.ExpiresAt = strings.TrimSpace(credential.ExpiresAt)
	enrichOAuthJSONCredential(&credential)
	if credential.AccessToken == "" && credential.RefreshToken == "" && credential.IDToken == "" {
		return OAuthJSONCredential{}, false
	}
	if !looksLikeOpenAIOAuthCredential(credential) {
		return OAuthJSONCredential{}, false
	}
	return credential, true
}

func IsOAuthJSONCredential(raw string) bool {
	_, ok := ParseOAuthJSONCredential(raw)
	return ok
}

func LooksLikeOAuthJSONCredential(raw string) bool {
	_, ok := ParseOAuthJSONCredentialLoose(raw)
	return ok
}

func NormalizeOAuthJSONCredentialMap(payload map[string]interface{}) bool {
	if payload == nil {
		return false
	}
	raw, err := common.Marshal(payload)
	if err != nil {
		return false
	}
	credential, ok := ParseOAuthJSONCredentialLoose(string(raw))
	if !ok {
		return false
	}
	if credential.AccountID != "" {
		if _, ok := payload["account_id"]; !ok || strings.TrimSpace(anyToString(payload["account_id"])) == "" {
			payload["account_id"] = credential.AccountID
		}
	}
	if credential.Email != "" {
		payload["email"] = credential.Email
	}
	if credential.ChatGPTUserID != "" {
		payload["chatgpt_user_id"] = credential.ChatGPTUserID
	}
	if credential.ExpiresAt != "" {
		payload["expires_at"] = credential.ExpiresAt
	}
	return true
}

func enrichOAuthJSONCredential(credential *OAuthJSONCredential) {
	if credential == nil {
		return
	}
	for _, token := range []string{credential.AccessToken, credential.IDToken} {
		claims, ok := decodeJWTClaims(token)
		if !ok {
			continue
		}
		auth, _ := claims["https://api.openai.com/auth"].(map[string]interface{})
		profile, _ := claims["https://api.openai.com/profile"].(map[string]interface{})
		if credential.AccountID == "" {
			if accountID := stringClaim(auth, "chatgpt_account_id"); accountID != "" {
				credential.AccountID = accountID
			}
		}
		if credential.ChatGPTAccountID == "" && credential.AccountID != "" {
			credential.ChatGPTAccountID = credential.AccountID
		}
		if credential.ChatGPTUserID == "" {
			if userID := stringClaim(auth, "chatgpt_user_id"); userID != "" {
				credential.ChatGPTUserID = userID
			} else if userID := stringClaim(auth, "user_id"); userID != "" {
				credential.ChatGPTUserID = userID
			}
		}
		if credential.Email == "" {
			if email := stringClaim(profile, "email"); email != "" {
				credential.Email = strings.ToLower(email)
			} else if email := stringClaim(claims, "email"); email != "" {
				credential.Email = strings.ToLower(email)
			}
		}
		if credential.ExpiresAt == "" {
			if exp, ok := numericClaim(claims, "exp"); ok && exp > 0 {
				credential.ExpiresAt = time.Unix(exp, 0).UTC().Format(time.RFC3339)
			}
		}
	}
}

func looksLikeOpenAIOAuthCredential(credential OAuthJSONCredential) bool {
	if credential.AccountID != "" || credential.ChatGPTAccountID != "" || credential.ChatGPTUserID != "" {
		return true
	}
	if credential.Email != "" && (credential.AccessToken != "" || credential.RefreshToken != "" || credential.IDToken != "") {
		return true
	}
	for _, token := range []string{credential.AccessToken, credential.IDToken} {
		claims, ok := decodeJWTClaims(token)
		if !ok {
			continue
		}
		if iss := stringClaim(claims, "iss"); strings.Contains(iss, "auth.openai.com") {
			return true
		}
		if _, ok := claims["https://api.openai.com/auth"]; ok {
			return true
		}
		if _, ok := claims["https://api.openai.com/profile"]; ok {
			return true
		}
	}
	return false
}

func decodeJWTClaims(token string) (map[string]interface{}, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims map[string]interface{}
	if err := common.Unmarshal(payloadRaw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}

func stringClaim(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func anyToString(value interface{}) string {
	if value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func expiresAtClaim(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	if text := anyToString(value); strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	var seconds int64
	switch typed := value.(type) {
	case float64:
		seconds = int64(typed)
	case int64:
		seconds = typed
	case int:
		seconds = int64(typed)
	default:
		return ""
	}
	if seconds <= 0 {
		return ""
	}
	if seconds > 1000000000000 {
		seconds /= 1000
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}

func numericClaim(payload map[string]interface{}, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	switch value := payload[key].(type) {
	case float64:
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	default:
		return 0, false
	}
}
