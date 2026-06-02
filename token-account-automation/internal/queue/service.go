package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"gorm.io/gorm"
)

const (
	DefaultPageSize     = 20
	MaxPageSize         = 100
	DefaultLeaseSeconds = 60
	MaxErrorTextLength  = 1000
)

type Service struct {
	db *gorm.DB
}

type CreateJobRequest struct {
	ParentJobID    string
	TaskType       string
	ExecutorType   string
	Priority       int
	RunAfter       int64
	MaxAttempts    int
	IdempotencyKey string
	TargetRef      string
	Input          any
}

type JobFilter struct {
	TaskType     string
	ExecutorType string
	Status       string
	TargetRef    string
	Page         int
	PageSize     int
}

type ClaimRequest struct {
	ExecutorType string
	WorkerID     string
	LeaseSeconds int
	Limit        int
}

type ClaimResult struct {
	Job     model.Job     `json:"job"`
	Attempt model.Attempt `json:"attempt"`
}

type JobDetail struct {
	Job      model.Job        `json:"job"`
	Attempts []model.Attempt  `json:"attempts"`
	Events   []model.JobEvent `json:"events"`
}

type JobStats struct {
	Total              int64            `json:"total"`
	ActiveTotal        int64            `json:"active_total"`
	TerminalTotal      int64            `json:"terminal_total"`
	StatusCounts       map[string]int64 `json:"status_counts"`
	TaskTypeCounts     map[string]int64 `json:"task_type_counts"`
	ExecutorTypeCounts map[string]int64 `json:"executor_type_counts"`
}

type WaitingHumanList struct {
	Items []WaitingHumanItem `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"page_size"`
}

type WaitingHumanItem struct {
	Job            model.Job              `json:"job"`
	Target         *model.Target          `json:"target,omitempty"`
	Locator        *ChannelAccountLocator `json:"locator,omitempty"`
	Reason         string                 `json:"reason,omitempty"`
	EventData      map[string]any         `json:"event_data,omitempty"`
	EventCreatedAt int64                  `json:"event_created_at,omitempty"`
	ResumePath     string                 `json:"resume_path"`
	CancelPath     string                 `json:"cancel_path"`
}

type AccountAuthInvalidEvent struct {
	TargetRef       string         `json:"target_ref,omitempty"`
	ChannelID       int            `json:"channel_id,omitempty"`
	CredentialIndex int            `json:"credential_index,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	SubjectKey      string         `json:"subject_key,omitempty"`
	DisplayName     string         `json:"display_name,omitempty"`
	Source          string         `json:"source,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Context         map[string]any `json:"context,omitempty"`
}

type ChannelAccountLocator struct {
	TargetRef       string `json:"target_ref,omitempty"`
	ExternalRef     string `json:"external_ref,omitempty"`
	ChannelID       int    `json:"channel_id"`
	CredentialIndex int    `json:"credential_index"`
}

func New(database *gorm.DB) *Service {
	return &Service{db: database}
}

func (s *Service) CreateJob(ctx context.Context, req CreateJobRequest) (*model.Job, bool, error) {
	req.TaskType = strings.ToLower(strings.TrimSpace(req.TaskType))
	req.ExecutorType = strings.ToLower(strings.TrimSpace(req.ExecutorType))
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.TargetRef = strings.TrimSpace(req.TargetRef)
	req.ParentJobID = strings.TrimSpace(req.ParentJobID)
	if req.TaskType == "" {
		return nil, false, errors.New("task_type is required")
	}
	if req.ExecutorType == "" {
		return nil, false, errors.New("executor_type is required")
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 1
	}
	inputJSON, err := encodeJSON(req.Input)
	if err != nil {
		return nil, false, err
	}

	var job model.Job
	created := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IdempotencyKey != "" {
			existing, err := findActiveJobTx(tx, req.TaskType, req.IdempotencyKey, req.TargetRef)
			if err != nil {
				return err
			}
			if existing != nil {
				job = *existing
				return nil
			}
		}
		job = model.Job{
			ParentJobID:    req.ParentJobID,
			TaskType:       req.TaskType,
			ExecutorType:   req.ExecutorType,
			Status:         model.JobStatusPending,
			Priority:       req.Priority,
			RunAfter:       req.RunAfter,
			MaxAttempts:    req.MaxAttempts,
			IdempotencyKey: req.IdempotencyKey,
			TargetRef:      req.TargetRef,
			InputJSON:      inputJSON,
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		created = true
		return createEventTx(tx, job.JobID, "created", "", job.Status, "job created", "")
	})
	if err != nil {
		return nil, false, err
	}
	return &job, created, nil
}

func (s *Service) ListJobs(ctx context.Context, filter JobFilter) ([]model.Job, int64, error) {
	filter = normalizeFilter(filter)
	var jobs []model.Job
	var total int64
	query := applyFilter(s.db.WithContext(ctx).Model(&model.Job{}), filter)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := applyFilter(s.db.WithContext(ctx).Model(&model.Job{}), filter).
		Order("id DESC").
		Limit(filter.PageSize).
		Offset((filter.Page - 1) * filter.PageSize).
		Find(&jobs).Error
	return jobs, total, err
}

func (s *Service) GetDetail(ctx context.Context, jobID string) (*JobDetail, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("job_id is required")
	}
	var job model.Job
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error; err != nil {
		return nil, err
	}
	var attempts []model.Attempt
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).Order("id DESC").Limit(100).Find(&attempts).Error; err != nil {
		return nil, err
	}
	var events []model.JobEvent
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).Order("id DESC").Limit(200).Find(&events).Error; err != nil {
		return nil, err
	}
	return &JobDetail{Job: job, Attempts: attempts, Events: events}, nil
}

func (s *Service) JobStats(ctx context.Context, filter JobFilter) (*JobStats, error) {
	filter = normalizeStatsFilter(filter)
	stats := &JobStats{
		StatusCounts:       initStatusCounts(),
		TaskTypeCounts:     make(map[string]int64),
		ExecutorTypeCounts: make(map[string]int64),
	}
	query := applyFilter(s.db.WithContext(ctx).Model(&model.Job{}), filter)
	if err := query.Count(&stats.Total).Error; err != nil {
		return nil, err
	}
	if err := groupedCounts(applyFilter(s.db.WithContext(ctx).Model(&model.Job{}), filter), "status", stats.StatusCounts); err != nil {
		return nil, err
	}
	if err := groupedCounts(applyFilter(s.db.WithContext(ctx).Model(&model.Job{}), filter), "task_type", stats.TaskTypeCounts); err != nil {
		return nil, err
	}
	if err := groupedCounts(applyFilter(s.db.WithContext(ctx).Model(&model.Job{}), filter), "executor_type", stats.ExecutorTypeCounts); err != nil {
		return nil, err
	}
	for status, count := range stats.StatusCounts {
		if model.TerminalStatus(status) {
			stats.TerminalTotal += count
		} else {
			stats.ActiveTotal += count
		}
	}
	return stats, nil
}

func (s *Service) ListWaitingHuman(ctx context.Context, filter JobFilter) (*WaitingHumanList, error) {
	filter.Status = model.JobStatusWaitingHuman
	jobs, total, err := s.ListJobs(ctx, filter)
	if err != nil {
		return nil, err
	}
	filter = normalizeFilter(filter)
	items := make([]WaitingHumanItem, 0, len(jobs))
	if len(jobs) == 0 {
		return &WaitingHumanList{Items: items, Total: total, Page: filter.Page, Size: filter.PageSize}, nil
	}
	jobIDs := make([]string, 0, len(jobs))
	targetRefs := make([]string, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.JobID)
		if strings.TrimSpace(job.TargetRef) != "" {
			targetRefs = append(targetRefs, job.TargetRef)
		}
	}
	events, err := s.latestWaitingHumanEvents(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	targets, err := s.targetsByRef(ctx, targetRefs)
	if err != nil {
		return nil, err
	}
	locators, err := s.channelAccountLocatorsByTargetRef(ctx, targetRefs)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		item := WaitingHumanItem{
			Job:        job,
			ResumePath: "/api/jobs/" + job.JobID + "/resume",
			CancelPath: "/api/jobs/" + job.JobID + "/cancel",
		}
		if target, ok := targets[job.TargetRef]; ok {
			targetCopy := target
			item.Target = &targetCopy
		}
		if locator, ok := locators[job.TargetRef]; ok {
			locatorCopy := locator
			item.Locator = &locatorCopy
		}
		if event, ok := events[job.JobID]; ok {
			item.Reason = event.Message
			item.EventData = decodeMap(event.DataJSON)
			item.EventCreatedAt = event.CreatedAt
		}
		items = append(items, item)
	}
	return &WaitingHumanList{Items: items, Total: total, Page: filter.Page, Size: filter.PageSize}, nil
}

func (s *Service) Claim(ctx context.Context, req ClaimRequest) (*ClaimResult, error) {
	req.ExecutorType = strings.ToLower(strings.TrimSpace(req.ExecutorType))
	req.WorkerID = strings.TrimSpace(req.WorkerID)
	if req.ExecutorType == "" {
		return nil, errors.New("executor_type is required")
	}
	if req.WorkerID == "" {
		return nil, errors.New("worker_id is required")
	}
	if req.LeaseSeconds <= 0 {
		req.LeaseSeconds = DefaultLeaseSeconds
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 10
	}
	now := model.Now()
	var candidates []model.Job
	if err := s.db.WithContext(ctx).
		Where("executor_type = ?", req.ExecutorType).
		Where("attempt_count < max_attempts").
		Where("(status = ? AND run_after <= ?) OR (status IN ? AND lease_until > 0 AND lease_until <= ?)",
			model.JobStatusPending,
			now,
			[]string{model.JobStatusLeased, model.JobStatusRunning},
			now,
		).
		Order("priority DESC, run_after ASC, id ASC").
		Limit(req.Limit).
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		claimed, err := s.claimCandidate(ctx, req, candidate, now, now+int64(req.LeaseSeconds))
		if err != nil {
			return nil, err
		}
		if claimed != nil {
			return claimed, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *Service) Heartbeat(ctx context.Context, jobID string, workerID string, leaseSeconds int) error {
	jobID = strings.TrimSpace(jobID)
	workerID = strings.TrimSpace(workerID)
	if jobID == "" || workerID == "" {
		return errors.New("job_id and worker_id are required")
	}
	if leaseSeconds <= 0 {
		leaseSeconds = DefaultLeaseSeconds
	}
	now := model.Now()
	result := s.db.WithContext(ctx).Model(&model.Job{}).
		Where("job_id = ? AND lease_owner = ? AND status IN ?", jobID, workerID, []string{model.JobStatusLeased, model.JobStatusRunning}).
		Updates(map[string]any{
			"lease_until":  now + int64(leaseSeconds),
			"heartbeat_at": now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) ReportStage(ctx context.Context, jobID string, workerID string, stage string, message string, data any) error {
	jobID = strings.TrimSpace(jobID)
	workerID = strings.TrimSpace(workerID)
	stage = strings.TrimSpace(stage)
	if jobID == "" || workerID == "" || stage == "" {
		return errors.New("job_id, worker_id, and stage are required")
	}
	dataJSON, err := encodeJSON(data)
	if err != nil {
		return err
	}
	now := model.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Job{}).
			Where("job_id = ? AND lease_owner = ? AND status IN ?", jobID, workerID, []string{model.JobStatusLeased, model.JobStatusRunning}).
			Updates(map[string]any{"status": model.JobStatusRunning, "heartbeat_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&model.Attempt{}).
			Where("job_id = ? AND worker_id = ? AND finished_at = 0", jobID, workerID).
			Updates(map[string]any{"stage": stage, "status": model.JobStatusRunning, "updated_at": now}).Error; err != nil {
			return err
		}
		return createEventTx(tx, jobID, "stage", stage, model.JobStatusRunning, Sanitize(message), dataJSON)
	})
}

func (s *Service) WaitingHuman(ctx context.Context, jobID string, workerID string, reason string, data any) error {
	jobID = strings.TrimSpace(jobID)
	workerID = strings.TrimSpace(workerID)
	reason = Sanitize(reason)
	if jobID == "" || workerID == "" {
		return errors.New("job_id and worker_id are required")
	}
	dataJSON, err := encodeJSON(data)
	if err != nil {
		return err
	}
	now := model.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Job{}).
			Where("job_id = ? AND lease_owner = ? AND status IN ?", jobID, workerID, []string{model.JobStatusLeased, model.JobStatusRunning}).
			Updates(map[string]any{
				"status":       model.JobStatusWaitingHuman,
				"lease_owner":  "",
				"lease_until":  int64(0),
				"heartbeat_at": int64(0),
				"updated_at":   now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&model.Attempt{}).
			Where("job_id = ? AND worker_id = ? AND finished_at = 0", jobID, workerID).
			Updates(map[string]any{
				"stage":       "waiting_human",
				"status":      model.JobStatusWaitingHuman,
				"finished_at": now,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		return createEventTx(tx, jobID, "waiting_human", "waiting_human", model.JobStatusWaitingHuman, reason, dataJSON)
	})
}

func (s *Service) Succeed(ctx context.Context, jobID string, workerID string, result any) error {
	resultJSON, err := encodeJSON(result)
	if err != nil {
		return err
	}
	return s.finish(ctx, jobID, workerID, model.JobStatusSuccess, "", "", resultJSON)
}

func (s *Service) Fail(ctx context.Context, jobID string, workerID string, errorCode string, errText string, retryAfterSeconds int64) error {
	jobID = strings.TrimSpace(jobID)
	workerID = strings.TrimSpace(workerID)
	errorCode = strings.TrimSpace(errorCode)
	errText = Sanitize(errText)
	if jobID == "" || workerID == "" {
		return errors.New("job_id and worker_id are required")
	}
	now := model.Now()
	var job model.Job
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ? AND lease_owner = ?", jobID, workerID).First(&job).Error; err != nil {
			return err
		}
		nextStatus := model.JobStatusFailed
		runAfter := int64(0)
		finishedAt := now
		if job.AttemptCount < job.MaxAttempts {
			nextStatus = model.JobStatusPending
			finishedAt = 0
			if retryAfterSeconds > 0 {
				runAfter = now + retryAfterSeconds
			}
		}
		if err := tx.Model(&model.Job{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":          nextStatus,
			"run_after":       runAfter,
			"lease_owner":     "",
			"lease_until":     int64(0),
			"heartbeat_at":    int64(0),
			"error_code":      errorCode,
			"sanitized_error": errText,
			"finished_at":     finishedAt,
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Attempt{}).
			Where("job_id = ? AND worker_id = ? AND finished_at = 0", jobID, workerID).
			Updates(map[string]any{
				"status":          model.JobStatusFailed,
				"error_code":      errorCode,
				"sanitized_error": errText,
				"finished_at":     now,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}
		return createEventTx(tx, jobID, "failed", "", nextStatus, errText, "")
	})
}

func (s *Service) Cancel(ctx context.Context, jobID string, reason string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return errors.New("job_id is required")
	}
	now := model.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Job{}).
			Where("job_id = ? AND status IN ?", jobID, model.ActiveStatuses()).
			Updates(map[string]any{
				"status":          model.JobStatusCanceled,
				"lease_owner":     "",
				"lease_until":     int64(0),
				"heartbeat_at":    int64(0),
				"sanitized_error": Sanitize(reason),
				"finished_at":     now,
				"updated_at":      now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return createEventTx(tx, jobID, "canceled", "", model.JobStatusCanceled, Sanitize(reason), "")
	})
}

func (s *Service) Retry(ctx context.Context, jobID string, runAfter int64) (*model.Job, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("job_id is required")
	}
	now := model.Now()
	var job model.Job
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", jobID).First(&job).Error; err != nil {
			return err
		}
		if !model.TerminalStatus(job.Status) {
			return errors.New("job is already active")
		}
		if err := tx.Model(&model.Job{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":          model.JobStatusPending,
			"run_after":       runAfter,
			"lease_owner":     "",
			"lease_until":     int64(0),
			"heartbeat_at":    int64(0),
			"attempt_count":   0,
			"error_code":      "",
			"sanitized_error": "",
			"finished_at":     int64(0),
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		if err := createEventTx(tx, job.JobID, "retried", "", model.JobStatusPending, "job retried", ""); err != nil {
			return err
		}
		return tx.Where("id = ?", job.ID).First(&job).Error
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Service) ResumeWaitingHuman(ctx context.Context, jobID string, runAfter int64, reason string) (*model.Job, error) {
	jobID = strings.TrimSpace(jobID)
	reason = Sanitize(reason)
	if jobID == "" {
		return nil, errors.New("job_id is required")
	}
	now := model.Now()
	var job model.Job
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", jobID).First(&job).Error; err != nil {
			return err
		}
		if job.Status != model.JobStatusWaitingHuman {
			return errors.New("job is not waiting for human")
		}
		maxAttempts := job.MaxAttempts
		if maxAttempts <= job.AttemptCount {
			maxAttempts = job.AttemptCount + 1
		}
		if err := tx.Model(&model.Job{}).Where("id = ?", job.ID).Updates(map[string]any{
			"status":          model.JobStatusPending,
			"run_after":       runAfter,
			"lease_owner":     "",
			"lease_until":     int64(0),
			"heartbeat_at":    int64(0),
			"max_attempts":    maxAttempts,
			"error_code":      "",
			"sanitized_error": "",
			"finished_at":     int64(0),
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		message := reason
		if message == "" {
			message = "waiting human job resumed"
		}
		if err := createEventTx(tx, job.JobID, "resumed", "", model.JobStatusPending, message, ""); err != nil {
			return err
		}
		return tx.Where("id = ?", job.ID).First(&job).Error
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Service) EnsureChannelAccountTarget(ctx context.Context, event AccountAuthInvalidEvent) (*model.Target, error) {
	if strings.TrimSpace(event.TargetRef) != "" {
		var target model.Target
		if err := s.db.WithContext(ctx).Where("target_ref = ?", strings.TrimSpace(event.TargetRef)).First(&target).Error; err == nil {
			return &target, nil
		}
	}
	if event.ChannelID <= 0 || event.CredentialIndex < 0 {
		return nil, errors.New("target_ref or channel_id + credential_index is required")
	}
	externalRef := ChannelAccountExternalRef(event.ChannelID, event.CredentialIndex)
	if target, _, err := s.FindTargetByBinding(ctx, model.BindingTypeChannelAccount, externalRef); err == nil {
		return target, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	metadata, _ := encodeJSON(map[string]any{"channel_id": event.ChannelID, "credential_index": event.CredentialIndex})
	target := model.Target{
		TargetType:   model.TargetTypeAccount,
		Provider:     strings.ToLower(strings.TrimSpace(event.Provider)),
		SubjectKey:   strings.TrimSpace(event.SubjectKey),
		DisplayName:  strings.TrimSpace(event.DisplayName),
		Status:       "active",
		MetadataJSON: metadata,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&target).Error; err != nil {
			return err
		}
		locator, _ := encodeJSON(map[string]any{"channel_id": event.ChannelID, "credential_index": event.CredentialIndex})
		return tx.Create(&model.TargetBinding{
			TargetRef:       target.TargetRef,
			BindingType:     model.BindingTypeChannelAccount,
			ExternalRef:     externalRef,
			ExternalRefHash: model.ExternalRefHash(model.BindingTypeChannelAccount, externalRef),
			LocatorJSON:     locator,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (s *Service) FindTargetByBinding(ctx context.Context, bindingType string, externalRef string) (*model.Target, *model.TargetBinding, error) {
	bindingType = strings.ToLower(strings.TrimSpace(bindingType))
	externalRef = strings.TrimSpace(externalRef)
	var binding model.TargetBinding
	if err := s.db.WithContext(ctx).
		Where("binding_type = ? AND external_ref_hash = ?", bindingType, model.ExternalRefHash(bindingType, externalRef)).
		First(&binding).Error; err != nil {
		return nil, nil, err
	}
	var target model.Target
	if err := s.db.WithContext(ctx).Where("target_ref = ?", binding.TargetRef).First(&target).Error; err != nil {
		return nil, nil, err
	}
	return &target, &binding, nil
}

func (s *Service) ChannelAccountLocatorForTarget(ctx context.Context, targetRef string) (*ChannelAccountLocator, error) {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return nil, errors.New("target_ref is required")
	}
	var binding model.TargetBinding
	if err := s.db.WithContext(ctx).
		Where("target_ref = ? AND binding_type = ?", targetRef, model.BindingTypeChannelAccount).
		Order("id DESC").
		First(&binding).Error; err != nil {
		return nil, err
	}
	channelID, credentialIndex, ok := ParseChannelAccountExternalRef(binding.ExternalRef)
	if !ok {
		return nil, fmt.Errorf("invalid channel account external_ref=%s", binding.ExternalRef)
	}
	return &ChannelAccountLocator{
		TargetRef:       targetRef,
		ExternalRef:     binding.ExternalRef,
		ChannelID:       channelID,
		CredentialIndex: credentialIndex,
	}, nil
}

func (s *Service) latestWaitingHumanEvents(ctx context.Context, jobIDs []string) (map[string]model.JobEvent, error) {
	result := make(map[string]model.JobEvent)
	jobIDs = compactStrings(jobIDs)
	if len(jobIDs) == 0 {
		return result, nil
	}
	var events []model.JobEvent
	if err := s.db.WithContext(ctx).
		Where("job_id IN ? AND event_type = ?", jobIDs, "waiting_human").
		Order("id DESC").
		Find(&events).Error; err != nil {
		return nil, err
	}
	for _, event := range events {
		if _, exists := result[event.JobID]; exists {
			continue
		}
		result[event.JobID] = event
	}
	return result, nil
}

func (s *Service) targetsByRef(ctx context.Context, targetRefs []string) (map[string]model.Target, error) {
	result := make(map[string]model.Target)
	targetRefs = compactStrings(targetRefs)
	if len(targetRefs) == 0 {
		return result, nil
	}
	var targets []model.Target
	if err := s.db.WithContext(ctx).Where("target_ref IN ?", targetRefs).Find(&targets).Error; err != nil {
		return nil, err
	}
	for _, target := range targets {
		result[target.TargetRef] = target
	}
	return result, nil
}

func (s *Service) channelAccountLocatorsByTargetRef(ctx context.Context, targetRefs []string) (map[string]ChannelAccountLocator, error) {
	result := make(map[string]ChannelAccountLocator)
	targetRefs = compactStrings(targetRefs)
	if len(targetRefs) == 0 {
		return result, nil
	}
	var bindings []model.TargetBinding
	if err := s.db.WithContext(ctx).
		Where("target_ref IN ? AND binding_type = ?", targetRefs, model.BindingTypeChannelAccount).
		Order("id DESC").
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if _, exists := result[binding.TargetRef]; exists {
			continue
		}
		channelID, credentialIndex, ok := ParseChannelAccountExternalRef(binding.ExternalRef)
		if !ok {
			continue
		}
		result[binding.TargetRef] = ChannelAccountLocator{
			TargetRef:       binding.TargetRef,
			ExternalRef:     binding.ExternalRef,
			ChannelID:       channelID,
			CredentialIndex: credentialIndex,
		}
	}
	return result, nil
}

func (s *Service) EnqueueAuthInvalid(ctx context.Context, event AccountAuthInvalidEvent) (*model.Job, bool, error) {
	target, err := s.EnsureChannelAccountTarget(ctx, event)
	if err != nil {
		return nil, false, err
	}
	input := map[string]any{
		"source": Sanitize(event.Source),
		"reason": Sanitize(event.Reason),
	}
	if event.Context != nil {
		input["context"] = event.Context
	}
	return s.CreateJob(ctx, CreateJobRequest{
		TaskType:       model.TaskAuthRecover,
		ExecutorType:   model.ExecutorInternalAPI,
		IdempotencyKey: AuthRecoverIdempotencyKey(target.TargetRef),
		TargetRef:      target.TargetRef,
		MaxAttempts:    1,
		Input:          input,
	})
}

func (s *Service) claimCandidate(ctx context.Context, req ClaimRequest, candidate model.Job, now int64, leaseUntil int64) (*ClaimResult, error) {
	var claimed model.Job
	var attempt model.Attempt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Job{}).
			Where("id = ?", candidate.ID).
			Where("attempt_count < max_attempts").
			Where("(status = ? AND run_after <= ?) OR (status IN ? AND lease_until > 0 AND lease_until <= ?)",
				model.JobStatusPending,
				now,
				[]string{model.JobStatusLeased, model.JobStatusRunning},
				now,
			).
			Updates(map[string]any{
				"status":        model.JobStatusLeased,
				"lease_owner":   req.WorkerID,
				"lease_until":   leaseUntil,
				"heartbeat_at":  now,
				"started_at":    now,
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"updated_at":    now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Where("id = ?", candidate.ID).First(&claimed).Error; err != nil {
			return err
		}
		attempt = model.Attempt{
			JobID:        claimed.JobID,
			AttemptNo:    claimed.AttemptCount,
			WorkerID:     req.WorkerID,
			ExecutorType: req.ExecutorType,
			Status:       model.JobStatusRunning,
			StartedAt:    now,
		}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		return createEventTx(tx, claimed.JobID, "claimed", "", model.JobStatusLeased, "job claimed", "")
	})
	if err != nil {
		return nil, err
	}
	if claimed.ID == 0 {
		return nil, nil
	}
	return &ClaimResult{Job: claimed, Attempt: attempt}, nil
}

func (s *Service) finish(ctx context.Context, jobID string, workerID string, status string, errorCode string, errText string, resultJSON string) error {
	jobID = strings.TrimSpace(jobID)
	workerID = strings.TrimSpace(workerID)
	if jobID == "" || workerID == "" {
		return errors.New("job_id and worker_id are required")
	}
	status = model.NormalizeStatus(status)
	if !model.TerminalStatus(status) {
		return errors.New("finish status must be terminal")
	}
	now := model.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Job{}).
			Where("job_id = ? AND lease_owner = ? AND status IN ?", jobID, workerID, []string{model.JobStatusLeased, model.JobStatusRunning}).
			Updates(map[string]any{
				"status":          status,
				"lease_owner":     "",
				"lease_until":     int64(0),
				"heartbeat_at":    int64(0),
				"result_json":     resultJSON,
				"error_code":      errorCode,
				"sanitized_error": Sanitize(errText),
				"finished_at":     now,
				"updated_at":      now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&model.Attempt{}).
			Where("job_id = ? AND worker_id = ? AND finished_at = 0", jobID, workerID).
			Updates(map[string]any{"status": status, "finished_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return createEventTx(tx, jobID, strings.ToLower(status), "", status, Sanitize(errText), resultJSON)
	})
}

func findActiveJobTx(tx *gorm.DB, taskType string, idempotencyKey string, targetRef string) (*model.Job, error) {
	var job model.Job
	query := tx.Where("task_type = ? AND idempotency_key = ? AND status IN ?", taskType, idempotencyKey, model.ActiveStatuses())
	if strings.TrimSpace(targetRef) != "" {
		query = query.Where("target_ref = ?", targetRef)
	}
	err := query.Order("id DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func createEventTx(tx *gorm.DB, jobID string, eventType string, stage string, status string, message string, dataJSON string) error {
	return tx.Create(&model.JobEvent{
		JobID:     strings.TrimSpace(jobID),
		EventType: strings.TrimSpace(eventType),
		Stage:     strings.TrimSpace(stage),
		Status:    model.NormalizeStatus(status),
		Message:   Sanitize(message),
		DataJSON:  strings.TrimSpace(dataJSON),
	}).Error
}

func normalizeFilter(filter JobFilter) JobFilter {
	filter.TaskType = strings.ToLower(strings.TrimSpace(filter.TaskType))
	filter.ExecutorType = strings.ToLower(strings.TrimSpace(filter.ExecutorType))
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	filter.TargetRef = strings.TrimSpace(filter.TargetRef)
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = DefaultPageSize
	}
	if filter.PageSize > MaxPageSize {
		filter.PageSize = MaxPageSize
	}
	return filter
}

func applyFilter(tx *gorm.DB, filter JobFilter) *gorm.DB {
	if filter.TaskType != "" {
		tx = tx.Where("task_type = ?", filter.TaskType)
	}
	if filter.ExecutorType != "" {
		tx = tx.Where("executor_type = ?", filter.ExecutorType)
	}
	if filter.Status != "" {
		tx = tx.Where("status = ?", model.NormalizeStatus(filter.Status))
	}
	if filter.TargetRef != "" {
		tx = tx.Where("target_ref = ?", filter.TargetRef)
	}
	return tx
}

func normalizeStatsFilter(filter JobFilter) JobFilter {
	filter = normalizeFilter(filter)
	filter.Page = 1
	filter.PageSize = MaxPageSize
	return filter
}

func initStatusCounts() map[string]int64 {
	return map[string]int64{
		model.JobStatusPending:      0,
		model.JobStatusLeased:       0,
		model.JobStatusRunning:      0,
		model.JobStatusWaitingHuman: 0,
		model.JobStatusSuccess:      0,
		model.JobStatusFailed:       0,
		model.JobStatusCanceled:     0,
		model.JobStatusExpired:      0,
	}
}

func groupedCounts(tx *gorm.DB, column string, dest map[string]int64) error {
	type row struct {
		GroupKey string `gorm:"column:group_key"`
		Total    int64  `gorm:"column:total"`
	}
	var rows []row
	if err := tx.Select(column + " AS group_key, COUNT(*) AS total").Group(column).Find(&rows).Error; err != nil {
		return err
	}
	for _, item := range rows {
		key := strings.TrimSpace(item.GroupKey)
		if key == "" {
			continue
		}
		if column == "status" {
			key = model.NormalizeStatus(key)
		}
		dest[key] = item.Total
	}
	return nil
}

func compactStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func decodeMap(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var result map[string]any
	if err := jsonx.Unmarshal([]byte(value), &result); err != nil {
		return nil
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func encodeJSON(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	data, err := jsonx.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ChannelAccountExternalRef(channelID int, credentialIndex int) string {
	return fmt.Sprintf("channel_account:%d:%d", channelID, credentialIndex)
}

func ParseChannelAccountExternalRef(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 || parts[0] != "channel_account" {
		return 0, -1, false
	}
	channelID, err := strconv.Atoi(parts[1])
	if err != nil || channelID <= 0 {
		return 0, -1, false
	}
	credentialIndex, err := strconv.Atoi(parts[2])
	if err != nil || credentialIndex < 0 {
		return 0, -1, false
	}
	return channelID, credentialIndex, true
}

func AuthRecoverIdempotencyKey(targetRef string) string {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return ""
	}
	return "auth_recover:" + targetRef
}

func BrowserLoginCooldownUntil(now time.Time, cooldown time.Duration) int64 {
	if cooldown <= 0 {
		cooldown = 6 * time.Hour
	}
	return now.Add(cooldown).Unix()
}

func Sanitize(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > MaxErrorTextLength {
		value = value[:MaxErrorTextLength]
	}
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "passwd", "access_token", "refresh_token", "id_token", "authorization", "cookie", "totp", "secret"} {
		if strings.Contains(lower, marker) {
			return "[redacted sensitive automation text]"
		}
	}
	return value
}
