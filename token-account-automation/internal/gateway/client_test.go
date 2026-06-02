package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
)

func TestWriteCredentialPostsGatewayCallback(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotRequest CredentialWritebackRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := jsonx.Decode(r.Body, &gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"channel_id":42,"credential_index":2,"account_type":"oauth_account","account_enabled":true}}`))
	}))
	defer server.Close()

	client := New(server.URL, "callback-token", 1)
	result, err := client.WriteCredential(context.Background(), CredentialWritebackRequest{
		ChannelID:       42,
		CredentialIndex: 2,
		CredentialType:  "oauth_account",
		Credential: map[string]any{
			"access_token": "new-access",
		},
		SourceJobID: "job-1",
	})
	if err != nil {
		t.Fatalf("write credential: %v", err)
	}
	if gotPath != credentialWritebackPath || gotAuth != "Bearer callback-token" {
		t.Fatalf("unexpected callback request path=%s auth=%s", gotPath, gotAuth)
	}
	if gotRequest.ChannelID != 42 || gotRequest.CredentialIndex != 2 || gotRequest.SourceJobID != "job-1" {
		t.Fatalf("unexpected request: %+v", gotRequest)
	}
	if result == nil || !result.AccountEnabled || result.ChannelID != 42 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestWriteCredentialReturnsSanitizedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"message":"credential already exists"}`))
	}))
	defer server.Close()

	client := New(server.URL, "callback-token", 1)
	_, err := client.WriteCredential(context.Background(), CredentialWritebackRequest{
		ChannelID:       42,
		CredentialIndex: 2,
		Credential:      map[string]any{"access_token": "secret-token"},
	})
	if err == nil {
		t.Fatalf("expected writeback error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked credential: %v", err)
	}
}
