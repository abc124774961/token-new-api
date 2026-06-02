package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/token-account-automation/internal/db"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testService(t *testing.T) *Service {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return New(database)
}

func TestCreateClaimStageAndSucceed(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	job, created, err := svc.CreateJob(ctx, CreateJobRequest{
		TaskType:       model.TaskAuthTokenRefresh,
		ExecutorType:   model.ExecutorInternalAPI,
		IdempotencyKey: "auth_recover:auto_target_1",
		TargetRef:      "auto_target_1",
		Input:          map[string]any{"reason": "token_invalidated"},
	})
	if err != nil || !created {
		t.Fatalf("create job: created=%v err=%v", created, err)
	}
	duplicate, created, err := svc.CreateJob(ctx, CreateJobRequest{
		TaskType:       model.TaskAuthTokenRefresh,
		ExecutorType:   model.ExecutorInternalAPI,
		IdempotencyKey: "auth_recover:auto_target_1",
		TargetRef:      "auto_target_1",
	})
	if err != nil || created || duplicate.JobID != job.JobID {
		t.Fatalf("duplicate mismatch: created=%v duplicate=%v job=%v err=%v", created, duplicate.JobID, job.JobID, err)
	}

	claim, err := svc.Claim(ctx, ClaimRequest{ExecutorType: model.ExecutorInternalAPI, WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claim.Job.JobID != job.JobID || claim.Job.Status != model.JobStatusLeased {
		t.Fatalf("unexpected claim: %+v", claim.Job)
	}
	if err := svc.ReportStage(ctx, job.JobID, "worker-a", "refresh_started", "starting", nil); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := svc.Succeed(ctx, job.JobID, "worker-a", map[string]any{"credential_written": true}); err != nil {
		t.Fatalf("succeed: %v", err)
	}
	detail, err := svc.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusSuccess || len(detail.Attempts) != 1 || len(detail.Events) == 0 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestAuthInvalidCreatesOneActiveRecoveryJob(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	event := AccountAuthInvalidEvent{
		ChannelID:       42,
		CredentialIndex: 2,
		Provider:        "codex_oauth",
		SubjectKey:      "acct-42",
		DisplayName:     "codex #3",
		Source:          "relay",
		Reason:          "token_invalidated",
	}
	job, created, err := svc.EnqueueAuthInvalid(ctx, event)
	if err != nil || !created {
		t.Fatalf("enqueue auth invalid: created=%v err=%v", created, err)
	}
	duplicate, created, err := svc.EnqueueAuthInvalid(ctx, event)
	if err != nil || created || duplicate.JobID != job.JobID {
		t.Fatalf("duplicate recovery mismatch: created=%v duplicate=%v job=%v err=%v", created, duplicate.JobID, job.JobID, err)
	}
	if job.TaskType != model.TaskAuthRecover || job.ExecutorType != model.ExecutorInternalAPI {
		t.Fatalf("unexpected recovery job: %+v", job)
	}
	locator, err := svc.ChannelAccountLocatorForTarget(ctx, job.TargetRef)
	if err != nil {
		t.Fatalf("locator: %v", err)
	}
	if locator.ChannelID != 42 || locator.CredentialIndex != 2 {
		t.Fatalf("unexpected locator: %+v", locator)
	}
}

func TestFailWithRemainingAttemptsReturnsPending(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	job, _, err := svc.CreateJob(ctx, CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		MaxAttempts:  2,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Claim(ctx, ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.Fail(ctx, job.JobID, "browser-worker", "network_error", "temporary network error", 30); err != nil {
		t.Fatalf("fail: %v", err)
	}
	detail, err := svc.GetDetail(ctx, job.JobID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Job.Status != model.JobStatusPending || detail.Job.AttemptCount != 1 || detail.Job.RunAfter <= model.Now() {
		t.Fatalf("unexpected retry state: %+v", detail.Job)
	}
}

func TestResumeWaitingHumanAllowsAnotherClaim(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	job, _, err := svc.CreateJob(ctx, CreateJobRequest{
		TaskType:     model.TaskAuthBrowserLogin,
		ExecutorType: model.ExecutorBrowserPlaywright,
		MaxAttempts:  1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Claim(ctx, ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker-a"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.WaitingHuman(ctx, job.JobID, "browser-worker-a", "captcha required", map[string]any{"stage": "login"}); err != nil {
		t.Fatalf("waiting human: %v", err)
	}
	resumed, err := svc.ResumeWaitingHuman(ctx, job.JobID, 0, "operator completed captcha")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Status != model.JobStatusPending || resumed.MaxAttempts != 2 || resumed.AttemptCount != 1 {
		t.Fatalf("unexpected resumed job: %+v", resumed)
	}
	claim, err := svc.Claim(ctx, ClaimRequest{ExecutorType: model.ExecutorBrowserPlaywright, WorkerID: "browser-worker-b"})
	if err != nil {
		t.Fatalf("claim after resume: %v", err)
	}
	if claim.Job.JobID != job.JobID || claim.Job.AttemptCount != 2 {
		t.Fatalf("unexpected resumed claim: %+v", claim.Job)
	}
}
