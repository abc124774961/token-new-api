package executor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	"github.com/QuantumNous/new-api/token-account-automation/internal/gateway"
	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/QuantumNous/new-api/token-account-automation/internal/oauth"
	"github.com/QuantumNous/new-api/token-account-automation/internal/queue"
	"github.com/QuantumNous/new-api/token-account-automation/internal/secret"
	"gorm.io/gorm"
)

type RefreshClient interface {
	RefreshCodexOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*oauth.CodexTokenResult, error)
}

type Service struct {
	cfg     config.Config
	queue   *queue.Service
	secrets *secret.Service
	refresh RefreshClient
	gateway *gateway.Client
	logger  *log.Logger
}

type claimJob struct {
	model.Job
	Attempt model.Attempt
}

func New(cfg config.Config, queueService *queue.Service, secretService *secret.Service, refreshClient RefreshClient, logger *log.Logger) *Service {
	if refreshClient == nil {
		refreshClient = oauth.NewCodexHTTPClient()
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Service{cfg: cfg, queue: queueService, secrets: secretService, refresh: refreshClient, gateway: gateway.New(cfg.GatewayCallbackURL, cfg.GatewayCallbackToken, cfg.GatewayTimeoutSecs), logger: logger}
}

func (s *Service) Run(ctx context.Context) error {
	pollInterval := time.Duration(s.cfg.InternalPollInterval) * time.Second
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	for {
		if err := s.drain(ctx); err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Printf("automation executor drain error: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (s *Service) drain(ctx context.Context) error {
	for {
		claim, err := s.queue.Claim(ctx, queue.ClaimRequest{
			ExecutorType: model.ExecutorInternalAPI,
			WorkerID:     s.workerID(),
			LeaseSeconds: s.leaseSeconds(),
			Limit:        5,
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if claim == nil {
			return nil
		}
		if err := s.handleClaim(ctx, claim); err != nil {
			s.logger.Printf("automation executor job=%s handle error: %v", claim.Job.JobID, err)
		}
	}
}

func (s *Service) handleClaim(ctx context.Context, claim *queue.ClaimResult) error {
	switch claim.Job.TaskType {
	case model.TaskAuthRecover:
		return s.handleAuthRecover(ctx, claim)
	case model.TaskAuthTokenRefresh:
		return s.handleAuthTokenRefresh(ctx, claim)
	default:
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "unsupported_task", fmt.Sprintf("unsupported task_type=%s", claim.Job.TaskType), 0)
	}
}

func (s *Service) handleAuthRecover(ctx context.Context, claim *queue.ClaimResult) error {
	if err := s.queue.ReportStage(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "dispatch_refresh", "dispatching token refresh", map[string]any{
		"target_ref": claim.Job.TargetRef,
	}); err != nil {
		return err
	}
	child, created, err := s.queue.CreateJob(ctx, queue.CreateJobRequest{
		ParentJobID:    claim.Job.JobID,
		TaskType:       model.TaskAuthTokenRefresh,
		ExecutorType:   model.ExecutorInternalAPI,
		Priority:       claim.Job.Priority,
		MaxAttempts:    3,
		IdempotencyKey: authTokenRefreshKey(claim.Job.TargetRef),
		TargetRef:      claim.Job.TargetRef,
		Input: map[string]any{
			"parent_job_id": claim.Job.JobID,
			"target_ref":    claim.Job.TargetRef,
		},
	})
	if err != nil {
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "child_job_create_failed", err.Error(), 30)
	}
	result := map[string]any{
		"next_task_type": model.TaskAuthTokenRefresh,
		"next_job_id":    child.JobID,
		"created":        created,
	}
	if err := s.queue.Succeed(ctx, claim.Job.JobID, claim.Attempt.WorkerID, result); err != nil {
		return err
	}
	return nil
}

func (s *Service) handleAuthTokenRefresh(ctx context.Context, claim *queue.ClaimResult) error {
	input := decodeJobInput(claim.Job.InputJSON)
	targetRef := strings.TrimSpace(claim.Job.TargetRef)
	if targetRef == "" {
		targetRef = stringValue(input, "target_ref")
	}
	if targetRef == "" {
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "missing_target_ref", "target_ref is required", 0)
	}
	if existingCredential, err := s.secrets.GetJobLinkedSecret(ctx, claim.Job.JobID, "credential"); err == nil && existingCredential != nil {
		return s.queue.Succeed(ctx, claim.Job.JobID, claim.Attempt.WorkerID, map[string]any{
			"credential_secret_ref": existingCredential.SecretRef,
			"fingerprint":           existingCredential.Fingerprint,
			"expires_at":            existingCredential.ExpiresAt,
			"target_ref":            targetRef,
			"reused":                true,
		})
	}
	refreshTokenRef := stringValue(input, "refresh_token_secret_ref")
	proxyURL := stringValue(input, "proxy_url")

	refreshToken, secretRecord, err := s.lookupRefreshToken(ctx, targetRef, refreshTokenRef)
	if err != nil {
		return s.dispatchBrowserFallback(ctx, claim, targetRef, err)
	}
	if secretRecord != nil {
		_ = s.secrets.LinkJobSecret(ctx, claim.Job.JobID, secretRecord.Secret.SecretRef, "refresh_token")
	}

	result, refreshErr := s.refresh.RefreshCodexOAuthToken(ctx, refreshToken, proxyURL)
	if refreshErr != nil {
		if oauth.ShouldFallbackToBrowser(refreshErr) {
			return s.dispatchBrowserFallback(ctx, claim, targetRef, refreshErr)
		}
		retryAfter := int64(60)
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "refresh_failed", sanitizeText(refreshErr.Error()), retryAfter)
	}

	payload := map[string]any{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
		"expires_at":    result.ExpiresAt,
		"provider":      "codex",
		"target_ref":    targetRef,
	}
	if !s.secrets.Enabled() {
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "secret_vault_unavailable", "AUTOMATION_SECRET_KEY is not configured", 0)
	}
	secretRecordOut, err := s.secrets.Create(ctx, secret.CreateRequest{
		SecretType: secret.SecretTypeCodexOAuth,
		ScopeRef:   targetRef,
		Value:      mustJSON(payload),
		ExpiresAt:  result.ExpiresAt,
	})
	if err != nil {
		return err
	}
	_ = s.secrets.LinkJobSecret(ctx, claim.Job.JobID, secretRecordOut.SecretRef, "credential")
	if err := s.writeCredentialToGateway(ctx, claim.Job.JobID, targetRef, payload, secretRecordOut.SecretRef, secretRecordOut.Fingerprint, result.ExpiresAt, nil); err != nil {
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "gateway_writeback_failed", sanitizeText(err.Error()), 60)
	}
	if err := s.queue.Succeed(ctx, claim.Job.JobID, claim.Attempt.WorkerID, map[string]any{
		"credential_secret_ref": secretRecordOut.SecretRef,
		"fingerprint":           secretRecordOut.Fingerprint,
		"expires_at":            result.ExpiresAt,
		"target_ref":            targetRef,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) writeCredentialToGateway(ctx context.Context, jobID string, targetRef string, credential any, secretRef string, fingerprint string, expiresAt int64, metadata map[string]any) error {
	if s == nil || s.gateway == nil || !s.gateway.Enabled() {
		return nil
	}
	channelID, credentialIndex, err := s.channelAccountLocator(ctx, targetRef)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if expiresAt > 0 {
		metadata["expires_at"] = expiresAt
	}
	_, err = s.gateway.WriteCredential(ctx, gateway.CredentialWritebackRequest{
		ChannelID:       channelID,
		CredentialIndex: credentialIndex,
		CredentialType:  "oauth_account",
		Credential:      credential,
		SourceJobID:     jobID,
		SecretRef:       secretRef,
		Fingerprint:     fingerprint,
		Metadata:        metadata,
	})
	return err
}

func (s *Service) channelAccountLocator(ctx context.Context, targetRef string) (int, int, error) {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return 0, -1, errors.New("target_ref is required")
	}
	if channelID, credentialIndex, ok := queue.ParseChannelAccountExternalRef(targetRef); ok {
		return channelID, credentialIndex, nil
	}
	locator, err := s.queue.ChannelAccountLocatorForTarget(ctx, targetRef)
	if err == nil && locator != nil {
		return locator.ChannelID, locator.CredentialIndex, nil
	}
	return 0, -1, fmt.Errorf("channel account binding not found for target_ref=%s", targetRef)
}

func (s *Service) dispatchBrowserFallback(ctx context.Context, claim *queue.ClaimResult, targetRef string, fallbackErr error) error {
	executorType := s.browserLoginExecutor()
	child, created, err := s.queue.CreateJob(ctx, queue.CreateJobRequest{
		ParentJobID:    claim.Job.JobID,
		TaskType:       model.TaskAuthBrowserLogin,
		ExecutorType:   executorType,
		Priority:       claim.Job.Priority,
		MaxAttempts:    1,
		IdempotencyKey: authBrowserLoginKey(targetRef),
		TargetRef:      targetRef,
		Input: map[string]any{
			"parent_job_id": claim.Job.JobID,
			"target_ref":    targetRef,
			"reason":        sanitizeText(fallbackErr.Error()),
		},
	})
	if err != nil {
		return s.queue.Fail(ctx, claim.Job.JobID, claim.Attempt.WorkerID, "browser_job_create_failed", err.Error(), 30)
	}
	return s.queue.Succeed(ctx, claim.Job.JobID, claim.Attempt.WorkerID, map[string]any{
		"next_task_type":     model.TaskAuthBrowserLogin,
		"next_executor_type": executorType,
		"next_job_id":        child.JobID,
		"created":            created,
		"fallback":           true,
		"reason":             sanitizeText(fallbackErr.Error()),
	})
}

func (s *Service) browserLoginExecutor() string {
	executorType := strings.ToLower(strings.TrimSpace(s.cfg.BrowserLoginExecutor))
	switch executorType {
	case model.ExecutorDesktopSession, model.ExecutorBrowserPlaywright:
		return executorType
	default:
		return model.ExecutorBrowserPlaywright
	}
}

func (s *Service) lookupRefreshToken(ctx context.Context, targetRef string, refreshTokenRef string) (string, *secret.Plaintext, error) {
	if s.secrets == nil {
		return "", nil, errors.New("secret service is not configured")
	}
	if strings.TrimSpace(refreshTokenRef) != "" {
		record, err := s.secrets.GetPlaintext(ctx, refreshTokenRef)
		if err != nil {
			return "", nil, err
		}
		refreshToken, err := extractRefreshToken(record.Value)
		if err != nil {
			return "", nil, err
		}
		return refreshToken, record, nil
	}
	record, err := s.secrets.FindLatestPlaintext(ctx, targetRef,
		secret.SecretTypeCodexOAuth,
		secret.SecretTypeOAuthRefreshToken,
		secret.SecretTypeRefreshToken,
	)
	if err != nil {
		return "", nil, err
	}
	refreshToken, err := extractRefreshToken(record.Value)
	if err != nil {
		return "", nil, err
	}
	return refreshToken, record, nil
}

func (s *Service) workerID() string {
	workerID := strings.TrimSpace(s.cfg.InternalWorkerID)
	if workerID == "" {
		return "internal-api-1"
	}
	return workerID
}

func (s *Service) leaseSeconds() int {
	if s.cfg.InternalLeaseSeconds <= 0 {
		return 60
	}
	return s.cfg.InternalLeaseSeconds
}

func authTokenRefreshKey(targetRef string) string {
	return "auth_token_refresh:" + strings.TrimSpace(targetRef)
}

func authBrowserLoginKey(targetRef string) string {
	return "auth_browser_login:" + strings.TrimSpace(targetRef)
}

func decodeJobInput(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var payload map[string]any
	if err := jsonx.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func extractRefreshToken(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("refresh token secret is empty")
	}
	if strings.HasPrefix(value, "{") {
		var payload map[string]any
		if err := jsonx.Unmarshal([]byte(value), &payload); err != nil {
			return "", errors.New("refresh token secret json is invalid")
		}
		refreshToken := stringValue(payload, "refresh_token")
		if refreshToken == "" {
			refreshToken = stringValue(payload, "RefreshToken")
		}
		if refreshToken == "" {
			return "", errors.New("refresh_token missing in secret")
		}
		return refreshToken, nil
	}
	return value, nil
}

func mustJSON(v any) string {
	data, err := jsonx.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func sanitizeText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		value = value[:1000]
	}
	return value
}
