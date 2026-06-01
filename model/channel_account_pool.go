package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	ChannelAccountPoolInvalid   = "invalid"
	ChannelAccountPoolDiscarded = "discarded"
)

type ChannelAccountArchiveFields struct {
	ID                           int    `json:"id" gorm:"primaryKey"`
	ChannelID                    int    `json:"channel_id" gorm:"index"`
	ChannelName                  string `json:"channel_name" gorm:"size:255"`
	CredentialIndex              int    `json:"credential_index" gorm:"index"`
	Credential                   string `json:"-" gorm:"type:text"`
	AccountID                    string `json:"account_id" gorm:"size:255;index"`
	AccountIdentityKey           string `json:"account_identity_key" gorm:"size:255;index"`
	CredentialSubjectFingerprint string `json:"credential_subject_fingerprint" gorm:"size:128;index"`
	CredentialFingerprint        string `json:"credential_fingerprint" gorm:"size:128;index"`
	AccountType                  string `json:"account_type" gorm:"size:64;index"`
	Brand                        string `json:"brand" gorm:"size:64;index"`
	Provider                     string `json:"provider" gorm:"size:64;index"`
	ResourceID                   string `json:"resource_id" gorm:"size:255"`
	ResourceType                 string `json:"resource_type" gorm:"size:64"`
	ProxyID                      int    `json:"proxy_id" gorm:"index"`
	CodexEnvironmentID           int    `json:"codex_environment_id" gorm:"index"`
	CapabilitySnapshot           string `json:"capability_snapshot,omitempty" gorm:"type:text"`
	Reason                       string `json:"reason,omitempty" gorm:"type:varchar(255)"`
	Note                         string `json:"note,omitempty" gorm:"type:text"`
	ArchivedAt                   int64  `json:"archived_at" gorm:"bigint;index"`
	UpdatedAt                    int64  `json:"updated_at" gorm:"bigint;index"`
}

type ChannelInvalidAccount struct {
	ChannelAccountArchiveFields
}

func (ChannelInvalidAccount) TableName() string {
	return "channel_invalid_accounts"
}

func (record *ChannelInvalidAccount) BeforeCreate(tx *gorm.DB) error {
	record.ChannelAccountArchiveFields.beforeCreate()
	return nil
}

func (record *ChannelInvalidAccount) BeforeUpdate(tx *gorm.DB) error {
	record.ChannelAccountArchiveFields.beforeUpdate()
	return nil
}

type ChannelDiscardedAccount struct {
	ChannelAccountArchiveFields
}

func (ChannelDiscardedAccount) TableName() string {
	return "channel_discarded_accounts"
}

func (record *ChannelDiscardedAccount) BeforeCreate(tx *gorm.DB) error {
	record.ChannelAccountArchiveFields.beforeCreate()
	return nil
}

func (record *ChannelDiscardedAccount) BeforeUpdate(tx *gorm.DB) error {
	record.ChannelAccountArchiveFields.beforeUpdate()
	return nil
}

type ChannelAccountPoolQuery struct {
	ChannelID   int
	Keyword     string
	AccountType string
	Brand       string
	Provider    string
	Page        int
	PageSize    int
}

func (fields *ChannelAccountArchiveFields) beforeCreate() {
	now := common.GetTimestamp()
	if fields.ArchivedAt == 0 {
		fields.ArchivedAt = now
	}
	fields.UpdatedAt = now
	fields.AccountType = strings.ToLower(strings.TrimSpace(fields.AccountType))
	fields.Brand = strings.ToLower(strings.TrimSpace(fields.Brand))
	fields.Provider = strings.ToLower(strings.TrimSpace(fields.Provider))
}

func (fields *ChannelAccountArchiveFields) beforeUpdate() {
	fields.UpdatedAt = common.GetTimestamp()
	fields.AccountType = strings.ToLower(strings.TrimSpace(fields.AccountType))
	fields.Brand = strings.ToLower(strings.TrimSpace(fields.Brand))
	fields.Provider = strings.ToLower(strings.TrimSpace(fields.Provider))
}

func ListChannelInvalidAccounts(query ChannelAccountPoolQuery) ([]ChannelInvalidAccount, int64, error) {
	records := make([]ChannelInvalidAccount, 0)
	var total int64
	err := applyChannelAccountPoolQuery(DB.Model(&ChannelInvalidAccount{}), query).
		Count(&total).Error
	if err != nil {
		return records, 0, err
	}
	err = applyChannelAccountPoolPagination(applyChannelAccountPoolQuery(DB.Model(&ChannelInvalidAccount{}), query), query).
		Order("archived_at DESC").
		Find(&records).Error
	return records, total, err
}

func ListChannelDiscardedAccounts(query ChannelAccountPoolQuery) ([]ChannelDiscardedAccount, int64, error) {
	records := make([]ChannelDiscardedAccount, 0)
	var total int64
	err := applyChannelAccountPoolQuery(DB.Model(&ChannelDiscardedAccount{}), query).
		Count(&total).Error
	if err != nil {
		return records, 0, err
	}
	err = applyChannelAccountPoolPagination(applyChannelAccountPoolQuery(DB.Model(&ChannelDiscardedAccount{}), query), query).
		Order("archived_at DESC").
		Find(&records).Error
	return records, total, err
}

func applyChannelAccountPoolQuery(tx *gorm.DB, query ChannelAccountPoolQuery) *gorm.DB {
	if query.ChannelID > 0 {
		tx = tx.Where("channel_id = ?", query.ChannelID)
	}
	if query.AccountType != "" {
		tx = tx.Where("account_type = ?", strings.ToLower(strings.TrimSpace(query.AccountType)))
	}
	if query.Brand != "" {
		tx = tx.Where("brand = ?", strings.ToLower(strings.TrimSpace(query.Brand)))
	}
	if query.Provider != "" {
		tx = tx.Where("provider = ?", strings.ToLower(strings.TrimSpace(query.Provider)))
	}
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where(
			"LOWER(channel_name) LIKE ? OR LOWER(account_id) LIKE ? OR LOWER(account_identity_key) LIKE ? OR LOWER(credential_subject_fingerprint) LIKE ? OR LOWER(credential_fingerprint) LIKE ? OR LOWER(reason) LIKE ? OR LOWER(note) LIKE ?",
			like, like, like, like, like, like, like,
		)
	}
	return tx
}

func applyChannelAccountPoolPagination(tx *gorm.DB, query ChannelAccountPoolQuery) *gorm.DB {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return tx.Offset((page - 1) * pageSize).Limit(pageSize)
}
