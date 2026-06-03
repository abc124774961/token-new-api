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
	"github.com/QuantumNous/new-api/token-account-automation/internal/gateway"
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

func TestDesktopTokenCanOnlyOperateDesktopSessionJobs(t *testing.T) {
	database, queueService, _, server := testHTTPAPIServer(t, config.Config{DesktopToken: "desktop-token"})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 93, 1)
	desktopJob, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorDesktopSession,
		TargetRef:    targetRef,
		Input:        map[string]any{"reason": "browser fallback"},
	})
	if err != nil {
		t.Fatalf("create desktop job: %v", err)
	}
	browserJob, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create browser job: %v", err)
	}

	rejectedBody, _ := jsonx.Marshal(map[string]any{
		"executor_type": model.ExecutorBrowserPlaywright,
		"worker_id":     "desktop-client",
	})
	rejectedReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/claim", bytes.NewReader(rejectedBody))
	rejectedReq.Header.Set("Authorization", "Bearer desktop-token")
	rejectedResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(rejectedResp, rejectedReq)
	if rejectedResp.Code != http.StatusForbidden {
		t.Fatalf("desktop token should not claim browser jobs: code=%d body=%s", rejectedResp.Code, rejectedResp.Body.String())
	}

	claimBody, _ := jsonx.Marshal(map[string]any{
		"executor_type": model.ExecutorDesktopSession,
		"worker_id":     "desktop-client",
	})
	claimReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/claim", bytes.NewReader(claimBody))
	claimReq.Header.Set("Authorization", "Bearer desktop-token")
	claimResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(claimResp, claimReq)
	if claimResp.Code != http.StatusOK {
		t.Fatalf("desktop claim failed: code=%d body=%s", claimResp.Code, claimResp.Body.String())
	}
	var claimPayload struct {
		Success bool `json:"success"`
		Data    struct {
			Job model.Job `json:"job"`
		} `json:"data"`
	}
	if err := jsonx.Decode(claimResp.Body, &claimPayload); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if !claimPayload.Success || claimPayload.Data.Job.JobID != desktopJob.JobID {
		t.Fatalf("unexpected claim payload: %+v", claimPayload)
	}

	stageBody, _ := jsonx.Marshal(map[string]any{
		"worker_id": "desktop-client",
		"stage":     "oauth_opened",
		"message":   "desktop browser opened",
		"data":      map[string]any{"proxy_id": "local-proxy-1"},
	})
	stageReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/"+desktopJob.JobID+"/stage", bytes.NewReader(stageBody))
	stageReq.Header.Set("Authorization", "Bearer desktop-token")
	stageResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(stageResp, stageReq)
	if stageResp.Code != http.StatusOK {
		t.Fatalf("desktop stage failed: code=%d body=%s", stageResp.Code, stageResp.Body.String())
	}

	forbiddenStageReq := httptest.NewRequest(http.MethodPost, "/internal/jobs/"+browserJob.JobID+"/stage", bytes.NewReader(stageBody))
	forbiddenStageReq.Header.Set("Authorization", "Bearer desktop-token")
	forbiddenStageResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(forbiddenStageResp, forbiddenStageReq)
	if forbiddenStageResp.Code != http.StatusForbidden {
		t.Fatalf("desktop token should not operate browser job: code=%d body=%s", forbiddenStageResp.Code, forbiddenStageResp.Body.String())
	}
}

func TestDesktopAccountsEndpointReturnsHydratedDesktopJobs(t *testing.T) {
	database, queueService, _, server := testHTTPAPIServer(t, config.Config{DesktopToken: "desktop-token"})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 94, 2)
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorDesktopSession,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorDesktopSession, WorkerID: "desktop-client"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := queueService.ReportStage(ctx, job.JobID, "desktop-client", "oauth_opened", "opened", map[string]any{"proxy_id": "local-proxy-2"}); err != nil {
		t.Fatalf("stage: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/desktop/accounts?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer desktop-token")
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
					JobID        string `json:"job_id"`
					ExecutorType string `json:"executor_type"`
				} `json:"job"`
				Locator struct {
					ChannelID       int `json:"channel_id"`
					CredentialIndex int `json:"credential_index"`
				} `json:"locator"`
				LatestEvent struct {
					EventType string `json:"event_type"`
					Stage     string `json:"stage"`
				} `json:"latest_event"`
				LatestEventData map[string]any `json:"latest_event_data"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Total != 1 || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	item := payload.Data.Items[0]
	if item.Job.JobID != job.JobID || item.Job.ExecutorType != model.ExecutorDesktopSession {
		t.Fatalf("unexpected job item: %+v", item.Job)
	}
	if item.Locator.ChannelID != 94 || item.Locator.CredentialIndex != 2 {
		t.Fatalf("unexpected locator: %+v", item.Locator)
	}
	if item.LatestEvent.EventType != "stage" || item.LatestEvent.Stage != "oauth_opened" || item.LatestEventData["proxy_id"] != "local-proxy-2" {
		t.Fatalf("unexpected latest event: event=%+v data=%+v", item.LatestEvent, item.LatestEventData)
	}
}

func TestDesktopRetryUpdatesProxyAndSessionOptions(t *testing.T) {
	_, queueService, _, server := testHTTPAPIServer(t, config.Config{DesktopToken: "desktop-token"})
	ctx := context.Background()
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorDesktopSession,
		MaxAttempts:  1,
		Input:        map[string]any{"reason": "auth invalid"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := queueService.Claim(ctx, queue.ClaimRequest{ExecutorType: model.ExecutorDesktopSession, WorkerID: "desktop-client"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := queueService.Fail(ctx, job.JobID, "desktop-client", "desktop_auth_failed", "timeout", 0); err != nil {
		t.Fatalf("fail: %v", err)
	}

	body, err := jsonx.Marshal(map[string]any{
		"preferred_proxy_id": "remote-7",
		"clear_session":      true,
		"reason":             "operator retry with proxy",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/desktop/jobs/"+job.JobID+"/retry", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer desktop-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}

	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusPending || detail.Job.AttemptCount != 0 || detail.Job.SanitizedError != "" {
		t.Fatalf("unexpected retried job: %+v", detail.Job)
	}
	var input map[string]any
	if err := jsonx.Unmarshal([]byte(detail.Job.InputJSON), &input); err != nil {
		t.Fatalf("decode input: %v", err)
	}
	if input["preferred_proxy_id"] != "remote-7" || input["clear_session"] != true || input["retry_reason"] != "operator retry with proxy" {
		t.Fatalf("retry input not patched: %+v", input)
	}
	foundInputUpdated := false
	foundRetried := false
	for _, event := range detail.Events {
		if event.EventType == "input_updated" {
			foundInputUpdated = true
		}
		if event.EventType == "retried" {
			foundRetried = true
		}
	}
	if !foundInputUpdated || !foundRetried {
		t.Fatalf("missing retry events: input_updated=%v retried=%v events=%+v", foundInputUpdated, foundRetried, detail.Events)
	}
}

func TestDesktopActionTemplatesExposeCurrentCapabilities(t *testing.T) {
	_, _, _, server := testHTTPAPIServer(t, config.Config{
		DesktopToken:         "desktop-token",
		GatewayCallbackURL:   "http://gateway.local",
		GatewayCallbackToken: "callback-token",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/desktop/action-templates", nil)
	req.Header.Set("Authorization", "Bearer desktop-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Total        int            `json:"total"`
			StatusCounts map[string]int `json:"status_counts"`
			Items        []struct {
				Key             string `json:"key"`
				Title           string `json:"title"`
				Category        string `json:"category"`
				TaskType        string `json:"task_type"`
				ExecutorType    string `json:"executor_type"`
				Status          string `json:"status"`
				Implemented     bool   `json:"implemented"`
				Enabled         bool   `json:"enabled"`
				RequiresProxy   bool   `json:"requires_proxy"`
				RequiresLocator bool   `json:"requires_locator"`
				Danger          bool   `json:"danger"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Total < 8 || len(payload.Data.Items) != payload.Data.Total {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	byKey := make(map[string]struct {
		Key             string `json:"key"`
		Title           string `json:"title"`
		Category        string `json:"category"`
		TaskType        string `json:"task_type"`
		ExecutorType    string `json:"executor_type"`
		Status          string `json:"status"`
		Implemented     bool   `json:"implemented"`
		Enabled         bool   `json:"enabled"`
		RequiresProxy   bool   `json:"requires_proxy"`
		RequiresLocator bool   `json:"requires_locator"`
		Danger          bool   `json:"danger"`
	})
	for _, item := range payload.Data.Items {
		byKey[item.Key] = item
	}
	browserLogin := byKey["auth_browser_login"]
	if !browserLogin.Implemented || !browserLogin.Enabled || !browserLogin.RequiresProxy || browserLogin.ExecutorType != model.ExecutorDesktopSession {
		t.Fatalf("unexpected browser login template: %+v", browserLogin)
	}
	tokenRefresh := byKey["auth_token_refresh"]
	if !tokenRefresh.Implemented || tokenRefresh.TaskType != model.TaskAuthTokenRefresh || tokenRefresh.ExecutorType != model.ExecutorInternalAPI {
		t.Fatalf("unexpected token refresh template: %+v", tokenRefresh)
	}
	archiveInvalid := byKey["archive_invalid_pool"]
	if !archiveInvalid.Enabled || !archiveInvalid.RequiresLocator || !archiveInvalid.Danger {
		t.Fatalf("unexpected invalid archive template: %+v", archiveInvalid)
	}
	accountProbe := byKey["account_probe"]
	if !accountProbe.Implemented || !accountProbe.Enabled || accountProbe.Status != "partial" || accountProbe.TaskType != model.TaskAccountProbe {
		t.Fatalf("unexpected account probe template: %+v", accountProbe)
	}
	profileVerify := byKey["profile_verify"]
	if !profileVerify.Implemented || !profileVerify.Enabled || profileVerify.Status != "partial" || profileVerify.TaskType != model.TaskAccountProfileVerify {
		t.Fatalf("unexpected profile verify template: %+v", profileVerify)
	}
	if payload.Data.StatusCounts["ready"] == 0 || payload.Data.StatusCounts["partial"] < 2 {
		t.Fatalf("missing status counts: %+v", payload.Data.StatusCounts)
	}
}

func TestDesktopAccountDiagnosticsActionsCreateChildJobs(t *testing.T) {
	var gotProfilePath string
	var gotProfileQuery string
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProfilePath = r.URL.Path
		gotProfileQuery = r.URL.RawQuery
		if r.Header.Get("Authorization") != "Bearer callback-token" {
			t.Fatalf("missing callback token: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"channel_id":96,"channel_name":"profile-channel","credential_index":5,"account":{"credential_index":5,"credential_short":"abc123","proxy":{"id":710,"name":"proxy-a"}},"snapshot_at":1893456000}}`))
	}))
	defer gatewayServer.Close()

	database, queueService, _, server := testHTTPAPIServer(t, config.Config{
		DesktopToken:         "desktop-token",
		GatewayCallbackURL:   gatewayServer.URL,
		GatewayCallbackToken: "callback-token",
	})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 96, 5)
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorDesktopSession,
		TargetRef:    targetRef,
		Priority:     7,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	body, err := jsonx.Marshal(map[string]any{
		"preferred_proxy_id": "proxy-main-1",
		"clear_session":      true,
		"reason":             "manual probe before restore",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/desktop/jobs/"+job.JobID+"/probe", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer desktop-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Action  string    `json:"action"`
			Created bool      `json:"created"`
			Job     model.Job `json:"job"`
		} `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || !payload.Data.Created || payload.Data.Action != "probe" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	child := payload.Data.Job
	if child.ParentJobID != job.JobID || child.TaskType != model.TaskAccountProbe || child.ExecutorType != model.ExecutorDesktopSession || child.Priority != 7 {
		t.Fatalf("unexpected child job: %+v", child)
	}
	var input map[string]any
	if err := jsonx.Unmarshal([]byte(child.InputJSON), &input); err != nil {
		t.Fatalf("decode child input: %v", err)
	}
	if input["source_job_id"] != job.JobID || input["target_ref"] != targetRef || input["preferred_proxy_id"] != "proxy-main-1" || input["clear_session"] != true {
		t.Fatalf("unexpected child input: %+v", input)
	}
	if int(input["channel_id"].(float64)) != 96 || int(input["credential_index"].(float64)) != 5 {
		t.Fatalf("missing locator in input: %+v", input)
	}
	if gotProfilePath != "/api/internal/token-account-automation/account-profile" || !strings.Contains(gotProfileQuery, "channel_id=96") || !strings.Contains(gotProfileQuery, "credential_index=5") {
		t.Fatalf("profile callback not called correctly: path=%s query=%s", gotProfilePath, gotProfileQuery)
	}
	if input["gateway_account_profile_status"] != "ok" {
		t.Fatalf("missing profile status: %+v", input)
	}
	profile, ok := input["gateway_account_profile"].(map[string]any)
	if !ok || profile["channel_name"] != "profile-channel" {
		t.Fatalf("missing gateway profile: %+v", input["gateway_account_profile"])
	}
}

func TestDesktopArchiveAccountUsesJobLocatorAndRecordsEvent(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody struct {
		Targets []struct {
			ChannelID       int `json:"channel_id"`
			CredentialIndex int `json:"credential_index"`
		} `json:"targets"`
		Reason      string `json:"reason"`
		Note        string `json:"note"`
		SourceJobID string `json:"source_job_id"`
	}
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := jsonx.Decode(r.Body, &gotBody); err != nil {
			t.Fatalf("decode gateway body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"operation":{"type":"pool","action":"archive_invalid","requested":1,"deleted":1}}}`))
	}))
	defer gatewayServer.Close()

	database, queueService, _, server := testHTTPAPIServer(t, config.Config{
		DesktopToken:         "desktop-token",
		GatewayCallbackURL:   gatewayServer.URL,
		GatewayCallbackToken: "callback-token",
	})
	ctx := context.Background()
	targetRef := createHTTPAPIChannelAccountTarget(t, database, 95, 4)
	job, _, err := queueService.CreateJob(ctx, queue.CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorDesktopSession,
		TargetRef:    targetRef,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	body, err := jsonx.Marshal(map[string]any{
		"reason": "operator confirmed invalid",
		"note":   "manual archive",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/desktop/jobs/"+job.JobID+"/archive-invalid", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer desktop-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	if gotPath != "/api/internal/token-account-automation/account-pools/invalid/archive" || gotAuth != "Bearer callback-token" {
		t.Fatalf("unexpected gateway request path=%s auth=%s", gotPath, gotAuth)
	}
	if len(gotBody.Targets) != 1 || gotBody.Targets[0].ChannelID != 95 || gotBody.Targets[0].CredentialIndex != 4 {
		t.Fatalf("unexpected gateway targets: %+v", gotBody)
	}
	if gotBody.Reason != "operator confirmed invalid" || gotBody.Note != "manual archive" || gotBody.SourceJobID != job.JobID {
		t.Fatalf("unexpected gateway body: %+v", gotBody)
	}
	detail, err := queueService.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Locator == nil || detail.Locator.ChannelID != 95 || detail.Locator.CredentialIndex != 4 {
		t.Fatalf("detail locator not hydrated: %+v", detail.Locator)
	}
	foundArchiveEvent := false
	for _, event := range detail.Events {
		if event.EventType == "account_pool_archived" && event.Stage == "invalid" {
			foundArchiveEvent = true
		}
	}
	if !foundArchiveEvent {
		t.Fatalf("missing archive event: %+v", detail.Events)
	}
}

func TestDesktopProxySyncEndpointReturnsGatewayProxies(t *testing.T) {
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/internal/token-account-automation/proxies" {
			t.Fatalf("unexpected gateway path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer callback-token" {
			t.Fatalf("missing callback token: %s", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("enabled_only") != "true" {
			t.Fatalf("unexpected enabled_only: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"id":7,"name":"proxy-a","protocol":"socks5","proxy_rules":"socks5://user:pass@127.0.0.1:1080","masked_address":"socks5://127.0.0.1:1080","enabled":true}]}`))
	}))
	defer gatewayServer.Close()

	_, _, _, server := testHTTPAPIServer(t, config.Config{
		DesktopToken:         "desktop-token",
		GatewayCallbackURL:   gatewayServer.URL,
		GatewayCallbackToken: "callback-token",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/desktop/proxies/sync", nil)
	req.Header.Set("Authorization", "Bearer desktop-token")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected response: code=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				ID         int    `json:"id"`
				Name       string `json:"name"`
				ProxyRules string `json:"proxy_rules"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := jsonx.Decode(resp.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Total != 1 || payload.Data.Items[0].ProxyRules != "socks5://user:pass@127.0.0.1:1080" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestDesktopInvalidPoolListAndReauthorizeUseGateway(t *testing.T) {
	var sawList bool
	var sawReauthorize bool
	gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer callback-token" {
			t.Fatalf("missing callback token: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/internal/token-account-automation/account-pools/invalid":
			sawList = true
			if r.URL.Query().Get("keyword") != "acct-a" {
				t.Fatalf("unexpected list query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"page":1,"page_size":20,"total":1,"items":[{"id":9,"pool":"invalid","channel_id":42,"credential_index":2,"account_id":"acct-a","credential_masked":"sk-...abcd"}]}}`))
		case "/api/internal/token-account-automation/account-pools/invalid/9/reauthorize":
			sawReauthorize = true
			var got gateway.InvalidAccountReauthorizeRequest
			if err := jsonx.Decode(r.Body, &got); err != nil {
				t.Fatalf("decode reauthorize body: %v", err)
			}
			if got.Reason != "desktop_pool_reauthorize" {
				t.Fatalf("unexpected reauthorize body: %+v", got)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"operation":{"type":"pool","action":"restore","requested":1},"automation":{"created":true}}}`))
		default:
			t.Fatalf("unexpected gateway path: %s", r.URL.Path)
		}
	}))
	defer gatewayServer.Close()

	_, _, _, server := testHTTPAPIServer(t, config.Config{
		DesktopToken:         "desktop-token",
		GatewayCallbackURL:   gatewayServer.URL,
		GatewayCallbackToken: "callback-token",
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/desktop/account-pools/invalid?keyword=acct-a", nil)
	listReq.Header.Set("Authorization", "Bearer desktop-token")
	listResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("unexpected list response: code=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Success bool `json:"success"`
		Data    struct {
			Total int `json:"total"`
			Items []struct {
				ID        int    `json:"id"`
				AccountID string `json:"account_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := jsonx.Decode(listResp.Body, &listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if !listPayload.Success || listPayload.Data.Total != 1 || listPayload.Data.Items[0].AccountID != "acct-a" {
		t.Fatalf("unexpected list payload: %+v", listPayload)
	}

	body, err := jsonx.Marshal(map[string]any{"reason": "desktop_pool_reauthorize"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	reauthReq := httptest.NewRequest(http.MethodPost, "/api/desktop/account-pools/invalid/9/reauthorize", bytes.NewReader(body))
	reauthReq.Header.Set("Authorization", "Bearer desktop-token")
	reauthResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(reauthResp, reauthReq)
	if reauthResp.Code != http.StatusOK {
		t.Fatalf("unexpected reauthorize response: code=%d body=%s", reauthResp.Code, reauthResp.Body.String())
	}
	if !sawList || !sawReauthorize {
		t.Fatalf("missing gateway calls: list=%v reauthorize=%v", sawList, sawReauthorize)
	}
}
