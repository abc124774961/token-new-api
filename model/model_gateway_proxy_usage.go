package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModelGatewayProxyUsageStatusBound   = "bound"
	ModelGatewayProxyUsageStatusRequest = "request"
)

type ModelGatewayProxyUsage struct {
	ID                           int    `json:"id"`
	ProxyID                      int    `json:"proxy_id" gorm:"not null;index;uniqueIndex:uk_mgw_proxy_usage"`
	ChannelID                    int    `json:"channel_id" gorm:"index;uniqueIndex:uk_mgw_proxy_usage"`
	ResourceID                   string `json:"resource_id,omitempty" gorm:"size:128;index"`
	ResourceType                 string `json:"resource_type,omitempty" gorm:"size:32"`
	AccountID                    string `json:"account_id,omitempty" gorm:"size:160;index"`
	AccountType                  string `json:"account_type,omitempty" gorm:"size:32"`
	Brand                        string `json:"brand,omitempty" gorm:"size:64;index;uniqueIndex:uk_mgw_proxy_usage"`
	Provider                     string `json:"provider,omitempty" gorm:"size:64;index"`
	CredentialIndex              int    `json:"credential_index" gorm:"index"`
	CredentialSubjectFingerprint string `json:"credential_subject_fingerprint,omitempty" gorm:"size:128;index;uniqueIndex:uk_mgw_proxy_usage"`
	CredentialFingerprint        string `json:"credential_fingerprint,omitempty" gorm:"size:128;index;uniqueIndex:uk_mgw_proxy_usage"`
	LastStatus                   string `json:"last_status,omitempty" gorm:"size:32;index"`
	FirstUsedAt                  int64  `json:"first_used_at" gorm:"bigint;index"`
	LastUsedAt                   int64  `json:"last_used_at" gorm:"bigint;index"`
	UseCount                     int64  `json:"use_count" gorm:"bigint;default:0"`
	CreatedTime                  int64  `json:"created_time" gorm:"bigint;index"`
	UpdatedTime                  int64  `json:"updated_time" gorm:"bigint;index"`
}

func (u *ModelGatewayProxyUsage) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if u.FirstUsedAt == 0 {
		u.FirstUsedAt = now
	}
	if u.LastUsedAt == 0 {
		u.LastUsedAt = now
	}
	if u.CreatedTime == 0 {
		u.CreatedTime = now
	}
	u.UpdatedTime = now
	if u.UseCount <= 0 {
		u.UseCount = 1
	}
	return nil
}

func (u *ModelGatewayProxyUsage) BeforeUpdate(tx *gorm.DB) error {
	u.UpdatedTime = common.GetTimestamp()
	return nil
}

func RecordModelGatewayProxyUsage(usage ModelGatewayProxyUsage) error {
	if DB == nil || usage.ProxyID <= 0 {
		return nil
	}
	now := common.GetTimestamp()
	if usage.FirstUsedAt == 0 {
		usage.FirstUsedAt = now
	}
	usage.LastUsedAt = now
	usage.UpdatedTime = now
	if usage.LastStatus == "" {
		usage.LastStatus = ModelGatewayProxyUsageStatusRequest
	}
	err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "proxy_id"},
			{Name: "channel_id"},
			{Name: "brand"},
			{Name: "credential_subject_fingerprint"},
			{Name: "credential_fingerprint"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"resource_id":                    usage.ResourceID,
			"resource_type":                  usage.ResourceType,
			"account_id":                     usage.AccountID,
			"account_type":                   usage.AccountType,
			"provider":                       usage.Provider,
			"credential_index":               usage.CredentialIndex,
			"last_status":                    usage.LastStatus,
			"last_used_at":                   usage.LastUsedAt,
			"updated_time":                   usage.UpdatedTime,
			"use_count":                      gorm.Expr("use_count + ?", 1),
			"credential_subject_fingerprint": usage.CredentialSubjectFingerprint,
			"credential_fingerprint":         usage.CredentialFingerprint,
		}),
	}).Create(&usage).Error
	if err != nil {
		return err
	}
	return DB.Model(&ModelGatewayProxy{}).
		Where("id = ?", usage.ProxyID).
		Updates(map[string]interface{}{
			"last_used_at": usage.LastUsedAt,
			"use_count":    gorm.Expr("use_count + ?", 1),
			"updated_time": usage.UpdatedTime,
		}).Error
}

func ListModelGatewayProxyUsages(proxyIDs []int) ([]ModelGatewayProxyUsage, error) {
	usages := make([]ModelGatewayProxyUsage, 0)
	query := DB.Model(&ModelGatewayProxyUsage{})
	if len(proxyIDs) > 0 {
		query = query.Where("proxy_id IN ?", proxyIDs)
	}
	err := query.Order("last_used_at DESC").Find(&usages).Error
	return usages, err
}
