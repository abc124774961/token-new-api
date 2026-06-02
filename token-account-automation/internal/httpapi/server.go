package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	"github.com/QuantumNous/new-api/token-account-automation/internal/gateway"
	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
	"github.com/QuantumNous/new-api/token-account-automation/internal/queue"
	"github.com/QuantumNous/new-api/token-account-automation/internal/secret"
	"gorm.io/gorm"
)

type Server struct {
	cfg     config.Config
	queue   *queue.Service
	secrets *secret.Service
	gateway *gateway.Client
	mux     *http.ServeMux
}

type createJobRequest struct {
	ParentJobID    string         `json:"parent_job_id,omitempty"`
	TaskType       string         `json:"task_type"`
	ExecutorType   string         `json:"executor_type"`
	Priority       int            `json:"priority,omitempty"`
	RunAfter       int64          `json:"run_after,omitempty"`
	MaxAttempts    int            `json:"max_attempts,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	TargetRef      string         `json:"target_ref,omitempty"`
	Input          map[string]any `json:"input,omitempty"`
}

type claimRequest struct {
	ExecutorType string `json:"executor_type"`
	WorkerID     string `json:"worker_id"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type heartbeatRequest struct {
	WorkerID     string `json:"worker_id"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

type stageRequest struct {
	WorkerID string         `json:"worker_id"`
	Stage    string         `json:"stage"`
	Message  string         `json:"message,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type succeedRequest struct {
	WorkerID string         `json:"worker_id"`
	Result   map[string]any `json:"result,omitempty"`
}

type credentialSucceedRequest struct {
	WorkerID  string         `json:"worker_id"`
	Value     any            `json:"value"`
	ExpiresAt int64          `json:"expires_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type failRequest struct {
	WorkerID          string `json:"worker_id"`
	ErrorCode         string `json:"error_code,omitempty"`
	Error             string `json:"error,omitempty"`
	RetryAfterSeconds int64  `json:"retry_after_seconds,omitempty"`
}

type waitingHumanRequest struct {
	WorkerID string         `json:"worker_id"`
	Reason   string         `json:"reason,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type retryRequest struct {
	RunAfter int64 `json:"run_after,omitempty"`
}

type resumeRequest struct {
	RunAfter int64  `json:"run_after,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type cancelRequest struct {
	Reason string `json:"reason,omitempty"`
}

type createSecretRequest struct {
	SecretType string `json:"secret_type"`
	ScopeRef   string `json:"scope_ref,omitempty"`
	Value      any    `json:"value"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
}

func New(cfg config.Config, queueService *queue.Service, secretService *secret.Service) *Server {
	s := &Server{cfg: cfg, queue: queueService, secrets: secretService, gateway: gateway.New(cfg.GatewayCallbackURL, cfg.GatewayCallbackToken, cfg.GatewayTimeoutSecs), mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/operator", s.operatorPage)
	s.mux.HandleFunc("/operator/", s.operatorPage)
	s.mux.HandleFunc("/api/jobs", s.apiJobs)
	s.mux.HandleFunc("/api/jobs/", s.apiJobByID)
	s.mux.HandleFunc("/api/operator/waiting-human", s.apiOperatorWaitingHuman)
	s.mux.HandleFunc("/api/stats/jobs", s.apiJobStats)
	s.mux.HandleFunc("/api/secrets", s.apiSecrets)
	s.mux.HandleFunc("/api/events/account-auth-invalid", s.apiAccountAuthInvalid)
	s.mux.HandleFunc("/internal/jobs/claim", s.internalClaim)
	s.mux.HandleFunc("/internal/jobs/", s.internalJobByID)
}

func (s *Server) apiSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req createSecretRequest
	if err := jsonx.Decode(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	value, err := secretValueToString(req.Value)
	if err != nil {
		writeError(w, err)
		return
	}
	record, err := s.secrets.Create(r.Context(), secret.CreateRequest{
		SecretType: req.SecretType,
		ScopeRef:   req.ScopeRef,
		Value:      value,
		ExpiresAt:  req.ExpiresAt,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"secret": record}})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "service": "token-account-automation"})
}

func (s *Server) apiJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		jobs, total, err := s.queue.ListJobs(r.Context(), queue.JobFilter{
			TaskType:     r.URL.Query().Get("task_type"),
			ExecutorType: r.URL.Query().Get("executor_type"),
			Status:       r.URL.Query().Get("status"),
			TargetRef:    r.URL.Query().Get("target_ref"),
			Page:         page,
			PageSize:     pageSize,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"items": jobs, "total": total}})
	case http.MethodPost:
		var req createJobRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		job, created, err := s.queue.CreateJob(r.Context(), queue.CreateJobRequest{
			ParentJobID:    req.ParentJobID,
			TaskType:       req.TaskType,
			ExecutorType:   req.ExecutorType,
			Priority:       req.Priority,
			RunAfter:       req.RunAfter,
			MaxAttempts:    req.MaxAttempts,
			IdempotencyKey: req.IdempotencyKey,
			TargetRef:      req.TargetRef,
			Input:          req.Input,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job": job, "created": created}})
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) apiJobByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	jobID, action, ok := splitIDAction(strings.TrimPrefix(r.URL.Path, "/api/jobs/"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if action == "" && r.Method == http.MethodGet {
		detail, err := s.queue.GetDetail(r.Context(), jobID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": detail})
		return
	}
	if action == "retry" && r.Method == http.MethodPost {
		var req retryRequest
		_ = jsonx.Decode(r.Body, &req)
		job, err := s.queue.Retry(r.Context(), jobID, req.RunAfter)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": job})
		return
	}
	if action == "resume" && r.Method == http.MethodPost {
		var req resumeRequest
		_ = jsonx.Decode(r.Body, &req)
		job, err := s.queue.ResumeWaitingHuman(r.Context(), jobID, req.RunAfter, req.Reason)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": job})
		return
	}
	if action == "cancel" && r.Method == http.MethodPost {
		var req cancelRequest
		_ = jsonx.Decode(r.Body, &req)
		if err := s.queue.Cancel(r.Context(), jobID, req.Reason); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job_id": jobID, "status": "CANCELED"}})
		return
	}
	http.NotFound(w, r)
}

func (s *Server) apiOperatorWaitingHuman(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	result, err := s.queue.ListWaitingHuman(r.Context(), queue.JobFilter{
		TaskType:     r.URL.Query().Get("task_type"),
		ExecutorType: r.URL.Query().Get("executor_type"),
		TargetRef:    r.URL.Query().Get("target_ref"),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) apiJobStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	stats, err := s.queue.JobStats(r.Context(), queue.JobFilter{
		TaskType:     r.URL.Query().Get("task_type"),
		ExecutorType: r.URL.Query().Get("executor_type"),
		Status:       r.URL.Query().Get("status"),
		TargetRef:    r.URL.Query().Get("target_ref"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": stats})
}

func (s *Server) apiAccountAuthInvalid(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var event queue.AccountAuthInvalidEvent
	if err := jsonx.Decode(r.Body, &event); err != nil {
		writeError(w, err)
		return
	}
	job, created, err := s.queue.EnqueueAuthInvalid(r.Context(), event)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job": job, "created": created}})
}

func (s *Server) internalClaim(w http.ResponseWriter, r *http.Request) {
	if !s.requireWorkerToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req claimRequest
	if err := jsonx.Decode(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.queue.Claim(r.Context(), queue.ClaimRequest{
		ExecutorType: req.ExecutorType,
		WorkerID:     req.WorkerID,
		LeaseSeconds: req.LeaseSeconds,
		Limit:        req.Limit,
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job": nil}})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) internalJobByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireWorkerToken(w, r) {
		return
	}
	jobID, action, ok := splitIDAction(strings.TrimPrefix(r.URL.Path, "/internal/jobs/"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	switch action {
	case "heartbeat":
		var req heartbeatRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.Heartbeat(r.Context(), jobID, req.WorkerID, req.LeaseSeconds))
	case "stage":
		var req stageRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.ReportStage(r.Context(), jobID, req.WorkerID, req.Stage, req.Message, req.Data))
	case "succeed":
		var req succeedRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.Succeed(r.Context(), jobID, req.WorkerID, req.Result))
	case "succeed-credential":
		var req credentialSucceedRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		s.succeedCredential(w, r, jobID, req)
	case "fail":
		var req failRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.Fail(r.Context(), jobID, req.WorkerID, req.ErrorCode, req.Error, req.RetryAfterSeconds))
	case "waiting-human":
		var req waitingHumanRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.WaitingHuman(r.Context(), jobID, req.WorkerID, req.Reason, req.Data))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) succeedCredential(w http.ResponseWriter, r *http.Request, jobID string, req credentialSucceedRequest) {
	if err := s.queue.ReportStage(r.Context(), jobID, req.WorkerID, "credential_received", "credential received", nil); err != nil {
		writeError(w, err)
		return
	}
	detail, err := s.queue.GetDetail(r.Context(), jobID)
	if err != nil {
		writeError(w, err)
		return
	}
	value, err := secretValueToString(req.Value)
	if err != nil {
		writeError(w, err)
		return
	}
	record, err := s.secrets.Create(r.Context(), secret.CreateRequest{
		SecretType: secret.SecretTypeCodexOAuth,
		ScopeRef:   detail.Job.TargetRef,
		Value:      value,
		ExpiresAt:  req.ExpiresAt,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.secrets.LinkJobSecret(r.Context(), jobID, record.SecretRef, "credential")
	if err := s.writeCredentialToGateway(r, detail.Job.TargetRef, jobID, req.Value, record.SecretRef, record.Fingerprint, req.ExpiresAt, req.Metadata); err != nil {
		writeError(w, err)
		return
	}
	result := map[string]any{
		"credential_secret_ref": record.SecretRef,
		"fingerprint":           record.Fingerprint,
		"expires_at":            record.ExpiresAt,
		"target_ref":            detail.Job.TargetRef,
	}
	if req.Metadata != nil {
		result["metadata"] = req.Metadata
	}
	if err := s.queue.Succeed(r.Context(), jobID, req.WorkerID, result); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) writeCredentialToGateway(r *http.Request, targetRef string, jobID string, value any, secretRef string, fingerprint string, expiresAt int64, metadata map[string]any) error {
	if s == nil || s.gateway == nil || !s.gateway.Enabled() {
		return nil
	}
	channelID, credentialIndex, err := s.channelAccountLocator(r, targetRef)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if expiresAt > 0 {
		metadata["expires_at"] = expiresAt
	}
	_, err = s.gateway.WriteCredential(r.Context(), gateway.CredentialWritebackRequest{
		ChannelID:       channelID,
		CredentialIndex: credentialIndex,
		CredentialType:  "oauth_account",
		Credential:      value,
		SourceJobID:     jobID,
		SecretRef:       secretRef,
		Fingerprint:     fingerprint,
		Metadata:        metadata,
	})
	return err
}

func (s *Server) channelAccountLocator(r *http.Request, targetRef string) (int, int, error) {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return 0, -1, errors.New("target_ref is required")
	}
	if channelID, credentialIndex, ok := queue.ParseChannelAccountExternalRef(targetRef); ok {
		return channelID, credentialIndex, nil
	}
	locator, err := s.queue.ChannelAccountLocatorForTarget(r.Context(), targetRef)
	if err == nil && locator != nil {
		return locator.ChannelID, locator.CredentialIndex, nil
	}
	return 0, -1, errors.New("channel account binding not found")
}

func (s *Server) requireAPIToken(w http.ResponseWriter, r *http.Request) bool {
	return requireToken(w, r, s.cfg.APIToken, "api token is not configured")
}

func (s *Server) requireWorkerToken(w http.ResponseWriter, r *http.Request) bool {
	return requireToken(w, r, s.cfg.WorkerToken, "worker token is not configured")
}

func requireToken(w http.ResponseWriter, r *http.Request, expected string, missingMessage string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": missingMessage})
		return false
	}
	actual := strings.TrimSpace(r.Header.Get("Authorization"))
	actual = strings.TrimPrefix(actual, "Bearer ")
	if actual == "" {
		actual = strings.TrimSpace(r.Header.Get("X-Automation-Token"))
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "invalid token"})
		return false
	}
	return true
}

func splitIDAction(value string) (string, string, bool) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func writeResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"ok": true}})
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]any{"success": false, "message": err.Error()})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := jsonx.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"success":false,"message":"failed to encode response"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func secretValueToString(value any) (string, error) {
	if value == nil {
		return "", errors.New("value is required")
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", errors.New("value is required")
		}
		return text, nil
	}
	data, err := jsonx.Marshal(value)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return "", errors.New("value is required")
	}
	return text, nil
}
