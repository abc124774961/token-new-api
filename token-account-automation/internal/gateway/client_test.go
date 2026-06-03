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

func TestArchiveAccountPostsGatewayCallback(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotRequest AccountPoolArchiveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := jsonx.Decode(r.Body, &gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"operation":{"type":"pool","action":"archive_invalid","requested":1,"deleted":1}}}`))
	}))
	defer server.Close()

	client := New(server.URL, "callback-token", 1)
	result, err := client.ArchiveAccountToInvalidPool(context.Background(), AccountPoolArchiveRequest{
		Targets: []ChannelAccountArchiveTarget{{
			ChannelID:       42,
			CredentialIndex: 2,
		}},
		Reason:      "desktop_operator_invalid",
		Note:        "desktop job job-1",
		SourceJobID: "job-1",
	})
	if err != nil {
		t.Fatalf("archive account: %v", err)
	}
	if gotPath != invalidAccountArchivePath || gotAuth != "Bearer callback-token" {
		t.Fatalf("unexpected callback request path=%s auth=%s", gotPath, gotAuth)
	}
	if len(gotRequest.Targets) != 1 || gotRequest.Targets[0].ChannelID != 42 || gotRequest.Targets[0].CredentialIndex != 2 || gotRequest.SourceJobID != "job-1" {
		t.Fatalf("unexpected request: %+v", gotRequest)
	}
	if result == nil || result.Operation == nil || result.Operation.Deleted != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestListInvalidAccountPoolPostsGatewayCallback(t *testing.T) {
	var gotPath string
	var gotQuery string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"page":1,"page_size":20,"total":1,"items":[{"id":9,"pool":"invalid","channel_id":42,"credential_index":2,"account_id":"acct-a","credential_masked":"sk-...abcd"}]}}`))
	}))
	defer server.Close()

	client := New(server.URL, "callback-token", 1)
	result, err := client.ListInvalidAccountPool(context.Background(), AccountPoolListParams{
		Page:     1,
		PageSize: 20,
		Keyword:  "acct-a",
	})
	if err != nil {
		t.Fatalf("list invalid pool: %v", err)
	}
	if gotPath != invalidAccountPoolListPath || gotAuth != "Bearer callback-token" || !strings.Contains(gotQuery, "keyword=acct-a") {
		t.Fatalf("unexpected gateway request path=%s query=%s auth=%s", gotPath, gotQuery, gotAuth)
	}
	if result == nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != 9 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestReauthorizeInvalidAccountPostsGatewayCallback(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotRequest InvalidAccountReauthorizeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := jsonx.Decode(r.Body, &gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"operation":{"type":"pool","action":"restore","requested":1},"automation":{"created":true}}}`))
	}))
	defer server.Close()

	client := New(server.URL, "callback-token", 1)
	result, err := client.ReauthorizeInvalidAccount(context.Background(), 9, InvalidAccountReauthorizeRequest{
		Reason: "desktop_pool_reauthorize",
	})
	if err != nil {
		t.Fatalf("reauthorize invalid account: %v", err)
	}
	if gotPath != invalidAccountReauthorizePrefix+"9/reauthorize" || gotAuth != "Bearer callback-token" || gotRequest.Reason != "desktop_pool_reauthorize" {
		t.Fatalf("unexpected gateway request path=%s auth=%s body=%+v", gotPath, gotAuth, gotRequest)
	}
	if result == nil || result.Operation == nil || result.Operation.Type != "pool" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
