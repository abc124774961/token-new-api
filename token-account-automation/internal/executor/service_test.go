package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	"github.com/QuantumNous/new-api/token-account-automation/internal/db"
	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/QuantumNous/new-api/token-account-automation/internal/oauth"
	"github.com/QuantumNous/new-api/token-account-automation/internal/queue"
	"github.com/QuantumNous/new-api/token-account-automation/internal/secret"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakeRefreshClient struct {
	token  string
	result *oauth.CodexTokenResult
	err    error
}

func createChannelAccountTarget(t *testing.T, database *gorm.DB, channelID int, credentialIndex int) string {
	t.Helper()
	target := model.Target{
		TargetRef:  model.NewID("auto_target"),
		TargetType: model.TargetTypeAccount,
		Provider:   "codex_oauth",
		Status:     "active",
	}
	if err := database.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	externalRef := queue.ChannelAccountExternalRef(channelID, credentialIndex)
	if err := database.Create(&model.TargetBinding{
		TargetRef:       target.TargetRef,
		BindingType:     model.BindingTypeChannelAccount,
		ExternalRef:     externalRef,
		ExternalRefHash: model.ExternalRefHash(model.BindingTypeChannelAccount, externalRef),
	}).Error; err != nil {
		t.Fatalf("create target binding: %v", err)
	}
	return target.TargetRef
}

func (f *fakeRefreshClient) RefreshCodexOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*oauth.CodexTokenResult, error) {
	f.token = refreshToken
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func testExecutor(t *testing.T, refreshClient *fakeRefreshClient) (*gorm.DB, *queue.Service, *secret.Service, *Service) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	queueService := queue.New(database)
	secretService := secret.New(database, "executor-test-secret-key")
	executorService := New(config.Config{
		InternalWorkerID:     "executor-test",
		InternalLeaseSeconds: 60,
	}, queueService, secretService, refreshClient, nil)
	return database, queueService, secretService, executorService
}

func TestAuthRecoverDispatchesTokenRefresh(t *testing.T) {
	_, queueService, _, executorService := testExecutor(t, &fakeRefreshClient{})
	ctx := context.Background()
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthRecover,
		ExecutorType: model.ExecutorInternalAPI,
		TargetRef:    "auto_target_1",
	})
	if err != nil {
		t.Fatalf("create recover job: %v", err)
	}
	claim, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorInternalAPI, WorkerID: "executor-test"})
	if err != nil {
		t.Fatalf("claim recover job: %v", err)
	}
	if err := executorService.handleClaim(ctx, claim); err != nil {
		t.Fatalf("handle recover job: %v", err)
	}

	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("recover detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusSuccess {
		t.Fatalf("recover did not succeed: %+v", detail.Job)
	}
	children, total, err := queueService.ListJobs(ctx, queue.JobFilter{
		TaskType:  model.TaskAuthTokenRefresh,
		TargetRef: "auto_target_1",
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("list children: %v", err)
	}
	if total != 1 || len(children) != 1 || children[0].ParentJobID != job.JobID {
		t.Fatalf("unexpected token refresh child: total=%d children=%+v", total, children)
	}
}

func TestAuthTokenRefreshStoresCredentialSecret(t *testing.T) {
	refreshClient := &fakeRefreshClient{
		result: &oauth.CodexTokenResult{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresAt:    1893456000,
		},
	}
	_, queueService, secretService, executorService := testExecutor(t, refreshClient)
	ctx := context.Background()
	initial, err := secretService.Create(ctx, secret.CreateRequest{
		SecretType: secret.SecretTypeOAuthRefreshToken,
		ScopeRef:   "auto_target_1",
		Value:      "old-refresh-token",
	})
	if err != nil {
		t.Fatalf("create initial secret: %v", err)
	}
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthTokenRefresh,
		ExecutorType: model.ExecutorInternalAPI,
		TargetRef:    "auto_target_1",
		Input: map[string]any{
			"refresh_token_secret_ref": initial.SecretRef,
		},
	})
	if err != nil {
		t.Fatalf("create refresh job: %v", err)
	}
	claim, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorInternalAPI, WorkerID: "executor-test"})
	if err != nil {
		t.Fatalf("claim refresh job: %v", err)
	}
	if err := executorService.handleClaim(ctx, claim); err != nil {
		t.Fatalf("handle refresh job: %v", err)
	}
	if refreshClient.token != "old-refresh-token" {
		t.Fatalf("unexpected refresh token used: %q", refreshClient.token)
	}

	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("refresh detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusSuccess {
		t.Fatalf("refresh did not succeed: %+v", detail.Job)
	}
	if strings.Contains(detail.Job.ResultJSON, "new-access-token") || strings.Contains(detail.Job.ResultJSON, "new-refresh-token") {
		t.Fatalf("job result leaked token: %s", detail.Job.ResultJSON)
	}
	var result map[string]any
	if err := jsonx.Unmarshal([]byte(detail.Job.ResultJSON), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	credentialRef, ok := result["credential_secret_ref"].(string)
	if !ok || credentialRef == "" {
		t.Fatalf("missing credential secret ref: %+v", result)
	}
	plain, err := secretService.GetPlaintext(ctx, credentialRef)
	if err != nil {
		t.Fatalf("load output credential secret: %v", err)
	}
	if !strings.Contains(plain.Value, "new-refresh-token") || !strings.Contains(plain.Value, "new-access-token") {
		t.Fatalf("unexpected credential secret payload: %s", plain.Value)
	}
}

func TestAuthTokenRefreshWritesCredentialToGateway(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := jsonx.Decode(r.Body, &gotBody); err != nil {
			t.Fatalf("decode gateway body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"channel_id":77,"credential_index":1,"account_type":"oauth_account","account_enabled":true}}`))
	}))
	defer server.Close()
	refreshClient := &fakeRefreshClient{
		result: &oauth.CodexTokenResult{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresAt:    1893456000,
		},
	}
	database, queueService, secretService, executorService := testExecutor(t, refreshClient)
	executorService.cfg.GatewayCallbackURL = server.URL
	executorService.cfg.GatewayCallbackToken = "callback-token"
	executorService.gateway = nil
	executorService = New(executorService.cfg, queueService, secretService, refreshClient, nil)
	ctx := context.Background()
	targetRef := createChannelAccountTarget(t, database, 77, 1)
	initial, err := secretService.Create(ctx, secret.CreateRequest{
		SecretType: secret.SecretTypeOAuthRefreshToken,
		ScopeRef:   targetRef,
		Value:      "old-refresh-token",
	})
	if err != nil {
		t.Fatalf("create initial secret: %v", err)
	}
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthTokenRefresh,
		ExecutorType: model.ExecutorInternalAPI,
		MaxAttempts:  2,
		TargetRef:    targetRef,
		Input: map[string]any{
			"refresh_token_secret_ref": initial.SecretRef,
		},
	})
	if err != nil {
		t.Fatalf("create refresh job: %v", err)
	}
	claim, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorInternalAPI, WorkerID: "executor-test"})
	if err != nil {
		t.Fatalf("claim refresh job: %v", err)
	}
	if err := executorService.handleClaim(ctx, claim); err != nil {
		t.Fatalf("handle refresh job: %v", err)
	}

	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("refresh detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusSuccess {
		t.Fatalf("refresh did not succeed: %+v", detail.Job)
	}
	if gotPath != "/api/internal/token-account-automation/credential" || gotAuth != "Bearer callback-token" {
		t.Fatalf("unexpected gateway request path=%s auth=%s", gotPath, gotAuth)
	}
	if gotBody["channel_id"].(float64) != 77 || gotBody["credential_index"].(float64) != 1 {
		t.Fatalf("unexpected gateway body: %+v", gotBody)
	}
	credential, ok := gotBody["credential"].(map[string]any)
	if !ok || credential["refresh_token"] != "new-refresh-token" {
		t.Fatalf("unexpected gateway credential payload: %+v", gotBody)
	}
}

func TestAuthTokenRefreshFailsWhenGatewayWritebackFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"message":"writeback rejected"}`))
	}))
	defer server.Close()
	refreshClient := &fakeRefreshClient{
		result: &oauth.CodexTokenResult{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresAt:    1893456000,
		},
	}
	database, queueService, secretService, executorService := testExecutor(t, refreshClient)
	executorService.cfg.GatewayCallbackURL = server.URL
	executorService.cfg.GatewayCallbackToken = "callback-token"
	executorService = New(executorService.cfg, queueService, secretService, refreshClient, nil)
	ctx := context.Background()
	targetRef := createChannelAccountTarget(t, database, 78, 0)
	initial, err := secretService.Create(ctx, secret.CreateRequest{
		SecretType: secret.SecretTypeOAuthRefreshToken,
		ScopeRef:   targetRef,
		Value:      "old-refresh-token",
	})
	if err != nil {
		t.Fatalf("create initial secret: %v", err)
	}
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthTokenRefresh,
		ExecutorType: model.ExecutorInternalAPI,
		MaxAttempts:  2,
		TargetRef:    targetRef,
		Input: map[string]any{
			"refresh_token_secret_ref": initial.SecretRef,
		},
	})
	if err != nil {
		t.Fatalf("create refresh job: %v", err)
	}
	claim, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorInternalAPI, WorkerID: "executor-test"})
	if err != nil {
		t.Fatalf("claim refresh job: %v", err)
	}
	if err := executorService.handleClaim(ctx, claim); err != nil {
		t.Fatalf("handle refresh job: %v", err)
	}

	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("refresh detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusPending || detail.Job.ErrorCode != "gateway_writeback_failed" {
		t.Fatalf("expected retry after gateway failure: %+v", detail.Job)
	}
	if strings.Contains(detail.Job.SanitizedError, "new-refresh-token") || strings.Contains(detail.Job.SanitizedError, "new-access-token") {
		t.Fatalf("gateway error leaked token: %s", detail.Job.SanitizedError)
	}
}

func TestAuthTokenRefreshFallsBackToBrowserWhenRefreshTokenMissing(t *testing.T) {
	refreshClient := &fakeRefreshClient{err: errors.New("should not be called")}
	_, queueService, _, executorService := testExecutor(t, refreshClient)
	ctx := context.Background()
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthTokenRefresh,
		ExecutorType: model.ExecutorInternalAPI,
		TargetRef:    "auto_target_1",
	})
	if err != nil {
		t.Fatalf("create refresh job: %v", err)
	}
	claim, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorInternalAPI, WorkerID: "executor-test"})
	if err != nil {
		t.Fatalf("claim refresh job: %v", err)
	}
	if err := executorService.handleClaim(ctx, claim); err != nil {
		t.Fatalf("handle refresh job: %v", err)
	}
	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("refresh detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusSuccess {
		t.Fatalf("refresh should succeed with browser fallback: %+v", detail.Job)
	}
	children, total, err := queueService.ListJobs(ctx, queue.JobFilter{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		TargetRef:    "auto_target_1",
		PageSize:     10,
	})
	if err != nil {
		t.Fatalf("list browser children: %v", err)
	}
	if total != 1 || len(children) != 1 || children[0].ParentJobID != job.JobID {
		t.Fatalf("unexpected browser fallback child: total=%d children=%+v", total, children)
	}
	if refreshClient.token != "" {
		t.Fatalf("refresh client should not be called, got token=%q", refreshClient.token)
	}
}
