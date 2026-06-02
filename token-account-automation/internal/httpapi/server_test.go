package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	"github.com/QuantumNous/new-api/token-account-automation/internal/db"
	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/QuantumNous/new-api/token-account-automation/internal/queue"
	"github.com/QuantumNous/new-api/token-account-automation/internal/secret"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testHTTPAPIServer(t *testing.T, cfg config.Config) (*gorm.DB, *queue.Service, *secret.Service, *Server) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	queueService := queue.New(database)
	secretService := secret.New(database, "httpapi-test-secret-key")
	if cfg.WorkerToken == "" {
		cfg.WorkerToken = "worker-token"
	}
	if cfg.APIToken == "" {
		cfg.APIToken = "api-token"
	}
	return database, queueService, secretService, New(cfg, queueService, secretService)
}

func createHTTPAPIChannelAccountTarget(t *testing.T, database *gorm.DB, channelID int, credentialIndex int) string {
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

func TestSucceedCredentialWritesBackToGatewayBeforeSuccess(t *testing.T) {
	var gotBody map[string]any
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := jsonx.Decode(r.Body, &gotBody); err != nil {
			t.Fatalf("decode gateway body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"channel_id":88,"credential_index":3,"account_type":"oauth_account","account_enabled":true}}`))
	}))
	defer gatewayServer.Close()

	database, queueService, secretService, server := testHTTPAPIServer(t, config.Config{
		GatewayCallbackURL:   gatewayServer.URL,
		GatewayCallbackToken: "callback-token",
	})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 88, 3)
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	claim, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker"})
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	body, err := jsonx.Marshal(map[string]any{
		"worker_id":  "browser-worker",
		"expires_at": int64(1893456000),
		"value": map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"provider":      "codex",
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/"+job.JobID+"/succeed-credential", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer worker-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	if gotBody["channel_id"].(float64) != 88 || gotBody["credential_index"].(float64) != 3 {
		t.Fatalf("unexpected gateway body: %+v", gotBody)
	}
	credential, ok := gotBody["credential"].(map[string]any)
	if !ok || credential["refresh_token"] != "new-refresh-token" {
		t.Fatalf("unexpected gateway credential: %+v", gotBody)
	}
	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusSuccess {
		t.Fatalf("job not success: %+v", detail.Job)
	}
	resultSecret, err := secretService.GetJobLinkedSecret(ctx, job.JobID, "credential")
	if err != nil || resultSecret == nil {
		t.Fatalf("missing linked secret: %v", err)
	}
}

func TestOperatorPageServed(t *testing.T) {
	_, _, _, server := testHTTPAPIServer(t, config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/operator", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(resp.Body.String(), "Token Account Automation") ||
		!strings.Contains(resp.Body.String(), "/api/operator/waiting-human") ||
		!strings.Contains(resp.Body.String(), "/api/jobs/") ||
		!strings.Contains(resp.Body.String(), "History") {
		t.Fatalf("operator page not served as expected")
	}
}

func TestSucceedCredentialDoesNotSucceedWhenGatewayRejects(t *testing.T) {
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"message":"writeback rejected"}`))
	}))
	defer gatewayServer.Close()

	database, queueService, _, server := testHTTPAPIServer(t, config.Config{
		GatewayCallbackURL:   gatewayServer.URL,
		GatewayCallbackToken: "callback-token",
	})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 89, 0)
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	body, err := jsonx.Marshal(map[string]any{
		"worker_id": "browser-worker",
		"value":     map[string]any{"access_token": "new-access-token", "refresh_token": "new-refresh-token"},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/"+job.JobID+"/succeed-credential", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer worker-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Job.Status == model.JobStatusSuccess {
		t.Fatalf("job should not succeed after gateway rejection: %+v", detail.Job)
	}
}

func TestJobStatsEndpointReturnsGroupedCounts(t *testing.T) {
	database, queueService, _, server := testHTTPAPIServer(t, config.Config{})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 91, 0)
	if _, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthRecover,
		ExecutorType: model.ExecutorInternalAPI,
		TargetRef:    targetRef,
	}); err != nil {
		t.Fatalf("create recover job: %v", err)
	}
	refreshJob, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthTokenRefresh,
		ExecutorType: model.ExecutorInternalAPI,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create refresh job: %v", err)
	}
	browserJob, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create browser job: %v", err)
	}
	if err := database.Model(&model.Job{}).Where("job_id = ?", refreshJob.JobID).Update("status", model.JobStatusSuccess).Error; err != nil {
		t.Fatalf("mark refresh success: %v", err)
	}
	if err := database.Model(&model.Job{}).Where("job_id = ?", browserJob.JobID).Update("status", model.JobStatusWaitingHuman).Error; err != nil {
		t.Fatalf("mark browser waiting human: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/stats/jobs?target_ref="+targetRef, nil)
	req.Header.Set("Authorization", "Bearer api-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Total              int64            `json:"total"`
			ActiveTotal        int64            `json:"active_total"`
			TerminalTotal      int64            `json:"terminal_total"`
			StatusCounts       map[string]int64 `json:"status_counts"`
			TaskTypeCounts     map[string]int64 `json:"task_type_counts"`
			ExecutorTypeCounts map[string]int64 `json:"executor_type_counts"`
		} `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Total != 3 || payload.Data.ActiveTotal != 2 || payload.Data.TerminalTotal != 1 {
		t.Fatalf("unexpected totals: %+v", payload.Data)
	}
	if payload.Data.StatusCounts[model.JobStatusPending] != 1 ||
		payload.Data.StatusCounts[model.JobStatusWaitingHuman] != 1 ||
		payload.Data.StatusCounts[model.JobStatusSuccess] != 1 {
		t.Fatalf("unexpected status counts: %+v", payload.Data.StatusCounts)
	}
	if payload.Data.TaskTypeCounts[model.TaskAuthBrowserLogin] != 1 ||
		payload.Data.ExecutorTypeCounts[model.ExecutorInternalAPI] != 2 {
		t.Fatalf("unexpected grouped counts: task=%+v executor=%+v", payload.Data.TaskTypeCounts, payload.Data.ExecutorTypeCounts)
	}
}

func TestResumeWaitingHumanEndpoint(t *testing.T) {
	_, queueService, _, server := testHTTPAPIServer(t, config.Config{})
	ctx := context.Background()
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		MaxAttempts:  1,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := queueService.WaitingHuman(ctx, job.JobID, "browser-worker", "operator action required", nil); err != nil {
		t.Fatalf("waiting human: %v", err)
	}

	body, err := jsonx.Marshal(map[string]any{"reason": "operator completed login"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/"+job.JobID+"/resume", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer api-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Success bool      `json:"success"`
		Data    model.Job `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Status != model.JobStatusPending || payload.Data.MaxAttempts != 2 {
		t.Fatalf("unexpected resumed payload: %+v", payload)
	}
	if _, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker-2"}); err != nil {
		t.Fatalf("claim after resume: %v", err)
	}
}

func TestOperatorWaitingHumanEndpointReturnsActionableItems(t *testing.T) {
	database, queueService, _, server := testHTTPAPIServer(t, config.Config{})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 92, 4)
	waitingJob, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create waiting job: %v", err)
	}
	nonWaitingJob, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthTokenRefresh,
		ExecutorType: model.ExecutorInternalAPI,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create non-waiting job: %v", err)
	}
	if _, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker"}); err != nil {
		t.Fatalf("claim waiting job: %v", err)
	}
	if err := queueService.WaitingHuman(ctx, waitingJob.JobID, "browser-worker", "captcha required", map[string]any{
		"callback_port": 1455,
		"headless":      false,
	}); err != nil {
		t.Fatalf("waiting human: %v", err)
	}
	if err := database.Model(&model.Job{}).Where("job_id = ?", nonWaitingJob.JobID).Update("status", model.JobStatusFailed).Error; err != nil {
		t.Fatalf("mark non-waiting failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/operator/waiting-human?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Job struct {
					JobID     string `json:"job_id"`
					Status    string `json:"status"`
					TargetRef string `json:"target_ref"`
				} `json:"job"`
				Locator struct {
					ChannelID       int `json:"channel_id"`
					CredentialIndex int `json:"credential_index"`
				} `json:"locator"`
				Reason         string         `json:"reason"`
				EventData      map[string]any `json:"event_data"`
				EventCreatedAt int64          `json:"event_created_at"`
				ResumePath     string         `json:"resume_path"`
				CancelPath     string         `json:"cancel_path"`
			} `json:"items"`
			Total    int64 `json:"total"`
			Page     int   `json:"page"`
			PageSize int   `json:"page_size"`
		} `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Total != 1 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected payload totals: %+v", payload.Data)
	}
	item := payload.Data.Items[0]
	if item.Job.JobID != waitingJob.JobID || item.Job.Status != model.JobStatusWaitingHuman || item.Job.TargetRef != targetRef {
		t.Fatalf("unexpected item job: %+v", item.Job)
	}
	if item.Locator.ChannelID != 92 || item.Locator.CredentialIndex != 4 {
		t.Fatalf("unexpected locator: %+v", item.Locator)
	}
	if item.Reason != "captcha required" || item.EventCreatedAt <= 0 || item.EventData["callback_port"].(float64) != 1455 {
		t.Fatalf("unexpected event info: reason=%q created=%d data=%+v", item.Reason, item.EventCreatedAt, item.EventData)
	}
	if item.ResumePath != "/api/jobs/"+waitingJob.JobID+"/resume" || item.CancelPath != "/api/jobs/"+waitingJob.JobID+"/cancel" {
		t.Fatalf("unexpected action paths: resume=%s cancel=%s", item.ResumePath, item.CancelPath)
	}
}
