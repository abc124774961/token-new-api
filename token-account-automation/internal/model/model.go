package model

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	JobStatusPending      = "PENDING"
	JobStatusLeased       = "LEASED"
	JobStatusRunning      = "RUNNING"
	JobStatusWaitingHuman = "WAITING_HUMAN"
	JobStatusSuccess      = "SUCCESS"
	JobStatusFailed       = "FAILED"
	JobStatusCanceled     = "CANCELED"
	JobStatusExpired      = "EXPIRED"
)

const (
	ExecutorInternalAPI       = "internal_api"
	ExecutorBrowserPlaywright = "browser_playwright"
	ExecutorDesktopSession    = "desktop_session"
)

const (
	TaskAuthRecover      = "auth_recover"
	TaskAuthTokenRefresh = "auth_token_refresh"
	TaskAuthBrowserLogin = "auth_browser_login"
)

const (
	TargetTypeAccount         = "account"
	BindingTypeChannelAccount = "channel_account"
)

type Job struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	JobID          string `json:"job_id" gorm:"type:varchar(64);uniqueIndex"`
	ParentJobID    string `json:"parent_job_id,omitempty" gorm:"type:varchar(64);index;default:''"`
	TaskType       string `json:"task_type" gorm:"type:varchar(64);index;not null"`
	ExecutorType   string `json:"executor_type" gorm:"type:varchar(64);index;not null"`
	Status         string `json:"status" gorm:"type:varchar(32);index;not null;default:'PENDING'"`
	Priority       int    `json:"priority" gorm:"index;default:0"`
	RunAfter       int64  `json:"run_after" gorm:"bigint;index;default:0"`
	LeaseOwner     string `json:"lease_owner,omitempty" gorm:"type:varchar(191);index;default:''"`
	LeaseUntil     int64  `json:"lease_until,omitempty" gorm:"bigint;index;default:0"`
	HeartbeatAt    int64  `json:"heartbeat_at,omitempty" gorm:"bigint;index;default:0"`
	AttemptCount   int    `json:"attempt_count" gorm:"default:0"`
	MaxAttempts    int    `json:"max_attempts" gorm:"default:1"`
	IdempotencyKey string `json:"idempotency_key,omitempty" gorm:"type:varchar(191);index;default:''"`
	TargetRef      string `json:"target_ref,omitempty" gorm:"type:varchar(191);index;default:''"`
	InputJSON      string `json:"input_json,omitempty" gorm:"type:text"`
	ResultJSON     string `json:"result_json,omitempty" gorm:"type:text"`
	ErrorCode      string `json:"error_code,omitempty" gorm:"type:varchar(64);index;default:''"`
	SanitizedError string `json:"sanitized_error,omitempty" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;index"`
	StartedAt      int64  `json:"started_at,omitempty" gorm:"bigint;index;default:0"`
	FinishedAt     int64  `json:"finished_at,omitempty" gorm:"bigint;index;default:0"`
}

type Attempt struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	JobID          string `json:"job_id" gorm:"type:varchar(64);index;not null"`
	AttemptNo      int    `json:"attempt_no" gorm:"index;default:1"`
	WorkerID       string `json:"worker_id,omitempty" gorm:"type:varchar(191);index;default:''"`
	ExecutorType   string `json:"executor_type,omitempty" gorm:"type:varchar(64);index;default:''"`
	Stage          string `json:"stage,omitempty" gorm:"type:varchar(64);index;default:''"`
	Status         string `json:"status" gorm:"type:varchar(32);index;not null;default:'RUNNING'"`
	ErrorCode      string `json:"error_code,omitempty" gorm:"type:varchar(64);index;default:''"`
	SanitizedError string `json:"sanitized_error,omitempty" gorm:"type:text"`
	StartedAt      int64  `json:"started_at" gorm:"bigint;index"`
	FinishedAt     int64  `json:"finished_at,omitempty" gorm:"bigint;index;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;index"`
}

type JobEvent struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	JobID     string `json:"job_id" gorm:"type:varchar(64);index;not null"`
	EventType string `json:"event_type" gorm:"type:varchar(64);index;not null"`
	Stage     string `json:"stage,omitempty" gorm:"type:varchar(64);index;default:''"`
	Status    string `json:"status,omitempty" gorm:"type:varchar(32);index;default:''"`
	Message   string `json:"message,omitempty" gorm:"type:text"`
	DataJSON  string `json:"data_json,omitempty" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
}

type Target struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	TargetRef    string `json:"target_ref" gorm:"type:varchar(191);uniqueIndex"`
	TargetType   string `json:"target_type" gorm:"type:varchar(64);index;not null"`
	Provider     string `json:"provider,omitempty" gorm:"type:varchar(64);index;default:''"`
	SubjectKey   string `json:"subject_key,omitempty" gorm:"type:varchar(191);index;default:''"`
	DisplayName  string `json:"display_name,omitempty" gorm:"type:varchar(191);default:''"`
	Status       string `json:"status,omitempty" gorm:"type:varchar(32);index;default:'active'"`
	MetadataJSON string `json:"metadata_json,omitempty" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;index"`
}

type TargetBinding struct {
	ID              int64  `json:"id" gorm:"primaryKey"`
	TargetRef       string `json:"target_ref" gorm:"type:varchar(191);index;not null"`
	BindingType     string `json:"binding_type" gorm:"type:varchar(64);uniqueIndex:idx_target_binding_ref;index;not null"`
	ExternalRef     string `json:"external_ref" gorm:"type:varchar(191);index;not null"`
	ExternalRefHash string `json:"external_ref_hash" gorm:"type:varchar(64);uniqueIndex:idx_target_binding_ref;index;not null"`
	LocatorJSON     string `json:"locator_json,omitempty" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint;index"`
}

type Secret struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	SecretRef   string `json:"secret_ref" gorm:"type:varchar(191);uniqueIndex"`
	SecretType  string `json:"secret_type" gorm:"type:varchar(64);index;not null"`
	ScopeRef    string `json:"scope_ref,omitempty" gorm:"type:varchar(191);index;default:''"`
	Ciphertext  string `json:"-" gorm:"type:text;not null"`
	Fingerprint string `json:"fingerprint,omitempty" gorm:"type:varchar(64);index;default:''"`
	ExpiresAt   int64  `json:"expires_at,omitempty" gorm:"bigint;index;default:0"`
	RotatedAt   int64  `json:"rotated_at,omitempty" gorm:"bigint;index;default:0"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;index"`
}

type JobSecretRef struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	JobID     string `json:"job_id" gorm:"type:varchar(64);index;not null"`
	SecretRef string `json:"secret_ref" gorm:"type:varchar(191);index;not null"`
	Alias     string `json:"alias,omitempty" gorm:"type:varchar(64);index;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
}

func (Job) TableName() string           { return "automation_jobs" }
func (Attempt) TableName() string       { return "automation_attempts" }
func (JobEvent) TableName() string      { return "automation_job_events" }
func (Target) TableName() string        { return "automation_targets" }
func (TargetBinding) TableName() string { return "automation_target_bindings" }
func (Secret) TableName() string        { return "automation_secrets" }
func (JobSecretRef) TableName() string  { return "automation_job_secret_refs" }

func (j *Job) BeforeCreate(tx *gorm.DB) error {
	now := Now()
	if j.JobID == "" {
		j.JobID = NewID("auto_job")
	}
	if j.CreatedAt <= 0 {
		j.CreatedAt = now
	}
	j.UpdatedAt = now
	j.Normalize()
	return nil
}

func (j *Job) BeforeUpdate(tx *gorm.DB) error {
	j.UpdatedAt = Now()
	j.Normalize()
	return nil
}

func (j *Job) Normalize() {
	j.JobID = strings.TrimSpace(j.JobID)
	j.ParentJobID = strings.TrimSpace(j.ParentJobID)
	j.TaskType = strings.ToLower(strings.TrimSpace(j.TaskType))
	j.ExecutorType = strings.ToLower(strings.TrimSpace(j.ExecutorType))
	j.Status = NormalizeStatus(j.Status)
	j.LeaseOwner = strings.TrimSpace(j.LeaseOwner)
	j.IdempotencyKey = strings.TrimSpace(j.IdempotencyKey)
	j.TargetRef = strings.TrimSpace(j.TargetRef)
	j.ErrorCode = strings.TrimSpace(j.ErrorCode)
	if j.MaxAttempts <= 0 {
		j.MaxAttempts = 1
	}
}

func (a *Attempt) BeforeCreate(tx *gorm.DB) error {
	now := Now()
	if a.CreatedAt <= 0 {
		a.CreatedAt = now
	}
	if a.StartedAt <= 0 {
		a.StartedAt = now
	}
	a.UpdatedAt = now
	a.Normalize()
	return nil
}

func (a *Attempt) BeforeUpdate(tx *gorm.DB) error {
	a.UpdatedAt = Now()
	a.Normalize()
	return nil
}

func (a *Attempt) Normalize() {
	a.JobID = strings.TrimSpace(a.JobID)
	a.WorkerID = strings.TrimSpace(a.WorkerID)
	a.ExecutorType = strings.ToLower(strings.TrimSpace(a.ExecutorType))
	a.Stage = strings.TrimSpace(a.Stage)
	a.Status = NormalizeStatus(a.Status)
	a.ErrorCode = strings.TrimSpace(a.ErrorCode)
	if a.AttemptNo <= 0 {
		a.AttemptNo = 1
	}
}

func (e *JobEvent) BeforeCreate(tx *gorm.DB) error {
	if e.CreatedAt <= 0 {
		e.CreatedAt = Now()
	}
	e.JobID = strings.TrimSpace(e.JobID)
	e.EventType = strings.TrimSpace(e.EventType)
	e.Stage = strings.TrimSpace(e.Stage)
	e.Status = NormalizeStatus(e.Status)
	return nil
}

func (t *Target) BeforeCreate(tx *gorm.DB) error {
	now := Now()
	if t.TargetRef == "" {
		t.TargetRef = NewID("auto_target")
	}
	if t.CreatedAt <= 0 {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	t.Normalize()
	return nil
}

func (t *Target) BeforeUpdate(tx *gorm.DB) error {
	t.UpdatedAt = Now()
	t.Normalize()
	return nil
}

func (t *Target) Normalize() {
	t.TargetRef = strings.TrimSpace(t.TargetRef)
	t.TargetType = strings.ToLower(strings.TrimSpace(t.TargetType))
	t.Provider = strings.ToLower(strings.TrimSpace(t.Provider))
	t.SubjectKey = strings.TrimSpace(t.SubjectKey)
	t.DisplayName = strings.TrimSpace(t.DisplayName)
	t.Status = strings.ToLower(strings.TrimSpace(t.Status))
	if t.Status == "" {
		t.Status = "active"
	}
}

func (b *TargetBinding) BeforeCreate(tx *gorm.DB) error {
	now := Now()
	if b.CreatedAt <= 0 {
		b.CreatedAt = now
	}
	b.UpdatedAt = now
	b.Normalize()
	return nil
}

func (b *TargetBinding) BeforeUpdate(tx *gorm.DB) error {
	b.UpdatedAt = Now()
	b.Normalize()
	return nil
}

func (b *TargetBinding) Normalize() {
	b.TargetRef = strings.TrimSpace(b.TargetRef)
	b.BindingType = strings.ToLower(strings.TrimSpace(b.BindingType))
	b.ExternalRef = strings.TrimSpace(b.ExternalRef)
	if b.ExternalRefHash == "" && b.ExternalRef != "" {
		b.ExternalRefHash = ExternalRefHash(b.BindingType, b.ExternalRef)
	}
	b.ExternalRefHash = strings.TrimSpace(b.ExternalRefHash)
}

func (s *Secret) BeforeCreate(tx *gorm.DB) error {
	now := Now()
	if s.SecretRef == "" {
		s.SecretRef = NewID("auto_secret")
	}
	if s.CreatedAt <= 0 {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	s.Normalize()
	return nil
}

func (s *Secret) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = Now()
	s.Normalize()
	return nil
}

func (s *Secret) Normalize() {
	s.SecretRef = strings.TrimSpace(s.SecretRef)
	s.SecretType = strings.ToLower(strings.TrimSpace(s.SecretType))
	s.ScopeRef = strings.TrimSpace(s.ScopeRef)
	s.Fingerprint = strings.TrimSpace(s.Fingerprint)
}

func (r *JobSecretRef) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedAt <= 0 {
		r.CreatedAt = Now()
	}
	r.JobID = strings.TrimSpace(r.JobID)
	r.SecretRef = strings.TrimSpace(r.SecretRef)
	r.Alias = strings.ToLower(strings.TrimSpace(r.Alias))
	return nil
}

func NormalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case JobStatusLeased:
		return JobStatusLeased
	case JobStatusRunning:
		return JobStatusRunning
	case JobStatusWaitingHuman:
		return JobStatusWaitingHuman
	case JobStatusSuccess:
		return JobStatusSuccess
	case JobStatusFailed:
		return JobStatusFailed
	case JobStatusCanceled:
		return JobStatusCanceled
	case JobStatusExpired:
		return JobStatusExpired
	default:
		return JobStatusPending
	}
}

func TerminalStatus(status string) bool {
	switch NormalizeStatus(status) {
	case JobStatusSuccess, JobStatusFailed, JobStatusCanceled, JobStatusExpired:
		return true
	default:
		return false
	}
}

func ActiveStatuses() []string {
	return []string{JobStatusPending, JobStatusLeased, JobStatusRunning, JobStatusWaitingHuman}
}

func Now() int64 {
	return time.Now().Unix()
}

func NewID(prefix string) string {
	var data [18]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}

func ExternalRefHash(bindingType string, externalRef string) string {
	mac := hmac.New(sha256.New, []byte("token-account-automation-binding"))
	mac.Write([]byte(strings.ToLower(strings.TrimSpace(bindingType))))
	mac.Write([]byte(":"))
	mac.Write([]byte(strings.TrimSpace(externalRef)))
	return hex.EncodeToString(mac.Sum(nil))
}
