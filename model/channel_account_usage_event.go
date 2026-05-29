package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

const unknownChannelAccountCredentialIndex = -1

type ChannelAccountUsageEvent struct {
	Id                           int     `json:"id" gorm:"primaryKey"`
	RequestId                    string  `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	CreatedAt                    int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt                    int64   `json:"updated_at" gorm:"bigint;index"`
	ChannelID                    int     `json:"channel_id" gorm:"index:idx_caue_channel_completed,priority:1;index:idx_caue_identity_completed,priority:1;index:idx_caue_credential_completed,priority:1;index:idx_caue_probe_completed,priority:1;default:0"`
	ChannelName                  string  `json:"channel_name" gorm:"type:varchar(191);default:''"`
	CredentialIndex              int     `json:"credential_index" gorm:"index:idx_caue_credential_completed,priority:2;default:-1"`
	AccountID                    string  `json:"account_id" gorm:"type:varchar(191);index;default:''"`
	AccountIdentityKey           string  `json:"account_identity_key" gorm:"type:varchar(191);index:idx_caue_identity_completed,priority:2;default:''"`
	CredentialSubjectFingerprint string  `json:"credential_subject_fingerprint" gorm:"type:varchar(64);index;default:''"`
	CredentialFingerprint        string  `json:"credential_fingerprint" gorm:"type:varchar(64);index;default:''"`
	AccountType                  string  `json:"account_type" gorm:"type:varchar(64);index;default:''"`
	Brand                        string  `json:"brand" gorm:"type:varchar(64);index;default:''"`
	Provider                     string  `json:"provider" gorm:"type:varchar(64);index;default:''"`
	RequestedModel               string  `json:"requested_model" gorm:"type:varchar(191);index;default:''"`
	RequestedGroup               string  `json:"requested_group" gorm:"type:varchar(64);index;default:''"`
	SelectedGroup                string  `json:"selected_group" gorm:"type:varchar(64);index;default:''"`
	EndpointType                 string  `json:"endpoint_type" gorm:"type:varchar(64);index;default:''"`
	CompletedAt                  int64   `json:"completed_at" gorm:"bigint;index:idx_caue_channel_completed,priority:2;index:idx_caue_identity_completed,priority:3;index:idx_caue_credential_completed,priority:3;index:idx_caue_probe_completed,priority:3"`
	Success                      bool    `json:"success" gorm:"default:false;index"`
	StatusCode                   int     `json:"status_code" gorm:"default:0;index"`
	ErrorCategory                string  `json:"error_category" gorm:"type:varchar(64);index;default:''"`
	IsHealthProbe                bool    `json:"is_health_probe" gorm:"index:idx_caue_probe_completed,priority:2;default:false"`
	DurationMs                   int64   `json:"duration_ms" gorm:"default:0"`
	TTFTMs                       int64   `json:"ttft_ms" gorm:"default:0"`
	PromptTokens                 int64   `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens             int64   `json:"completion_tokens" gorm:"default:0"`
	TotalTokens                  int64   `json:"total_tokens" gorm:"default:0"`
	Quota                        int64   `json:"quota" gorm:"default:0"`
	UpstreamCostTotal            float64 `json:"upstream_cost_total" gorm:"type:decimal(18,12);default:0"`
	CostSource                   string  `json:"cost_source" gorm:"type:varchar(32);index;default:'pending'"`
	CostAccuracy                 string  `json:"cost_accuracy" gorm:"type:varchar(32);index;default:'pending'"`
	CostCalculatedAt             int64   `json:"cost_calculated_at" gorm:"bigint;index"`
	UsageEstimated               bool    `json:"usage_estimated" gorm:"default:false;index"`
	ProviderSurface              string  `json:"provider_surface" gorm:"type:varchar(64);index;default:''"`
	CapabilityClassification     string  `json:"capability_classification" gorm:"type:varchar(64);index;default:''"`
	ProxyID                      int     `json:"proxy_id" gorm:"default:0;index"`
}

func (ChannelAccountUsageEvent) TableName() string {
	return "channel_account_usage_events"
}

type ChannelAccountUsageWindowSpec struct {
	Name  string
	Since int64
}

type ChannelAccountUsageWindowAggregate struct {
	Window             string `gorm:"column:window_name"`
	AccountIdentityKey string
	CredentialIndex    int
	Requests           int64
	SuccessRequests    int64
	ErrorRequests      int64
	TimeoutRequests    int64
	PromptTokens       int64
	CompletionTokens   int64
	TotalTokens        int64
	Quota              int64
	UpstreamCostTotal  float64
	AvgDurationMs      float64
	AvgTTFTMs          float64
	LastActiveAt       int64
	TopErrorCategory   string
	TopErrorCount      int64
}

func UpsertChannelAccountUsageDispatch(event ChannelAccountUsageEvent) error {
	event = normalizeChannelAccountUsageEvent(event)
	updates := baseChannelAccountUsageAssignments(event)
	addChannelAccountUsageIdentityAssignments(updates, event)
	addChannelAccountUsageRequestAssignments(updates, event)
	return upsertChannelAccountUsageEvent(event, updates)
}

func UpsertChannelAccountUsageAttempt(event ChannelAccountUsageEvent) error {
	event = normalizeChannelAccountUsageEvent(event)
	updates := baseChannelAccountUsageAssignments(event)
	addChannelAccountUsageIdentityAssignments(updates, event)
	addChannelAccountUsageRequestAssignments(updates, event)
	updates["completed_at"] = event.CompletedAt
	updates["success"] = event.Success
	updates["status_code"] = event.StatusCode
	updates["error_category"] = event.ErrorCategory
	updates["is_health_probe"] = event.IsHealthProbe
	updates["duration_ms"] = event.DurationMs
	updates["ttft_ms"] = event.TTFTMs
	return upsertChannelAccountUsageEvent(event, updates)
}

func UpsertChannelAccountUsageBilling(event ChannelAccountUsageEvent) error {
	event = normalizeChannelAccountUsageEvent(event)
	updates := baseChannelAccountUsageAssignments(event)
	addChannelAccountUsageRequestAssignments(updates, event)
	if event.ChannelID > 0 {
		updates["channel_id"] = event.ChannelID
	}
	if event.CompletedAt > 0 {
		updates["completed_at"] = event.CompletedAt
	}
	updates["prompt_tokens"] = event.PromptTokens
	updates["completion_tokens"] = event.CompletionTokens
	updates["total_tokens"] = event.TotalTokens
	updates["quota"] = event.Quota
	updates["usage_estimated"] = event.UsageEstimated
	if event.ProviderSurface != "" {
		updates["provider_surface"] = event.ProviderSurface
	}
	if event.CapabilityClassification != "" {
		updates["capability_classification"] = event.CapabilityClassification
	}
	if event.ProxyID > 0 {
		updates["proxy_id"] = event.ProxyID
	}
	return upsertChannelAccountUsageEvent(event, updates)
}

func UpsertChannelAccountUsageCost(summary ModelGatewayRequestCostSummary) error {
	event := normalizeChannelAccountUsageEvent(ChannelAccountUsageEvent{
		RequestId:         summary.RequestId,
		ChannelID:         summary.ChannelID,
		RequestedModel:    summary.UpstreamModel,
		UpstreamCostTotal: summary.UpstreamCostTotal,
		CostSource:        summary.CostSource,
		CostAccuracy:      summary.CostAccuracy,
		CostCalculatedAt:  summary.CalculatedAt,
	})
	updates := baseChannelAccountUsageAssignments(event)
	if event.ChannelID > 0 {
		updates["channel_id"] = event.ChannelID
	}
	if event.RequestedModel != "" {
		updates["requested_model"] = event.RequestedModel
	}
	updates["upstream_cost_total"] = event.UpstreamCostTotal
	updates["cost_source"] = event.CostSource
	updates["cost_accuracy"] = event.CostAccuracy
	updates["cost_calculated_at"] = event.CostCalculatedAt
	return upsertChannelAccountUsageEvent(event, updates)
}

func QueryChannelAccountUsageWindowAggregates(channelID int, windows []ChannelAccountUsageWindowSpec, includeHealthProbes bool) ([]ChannelAccountUsageWindowAggregate, error) {
	if DB == nil || channelID <= 0 || len(windows) == 0 {
		return nil, nil
	}
	aggregates := make([]ChannelAccountUsageWindowAggregate, 0)
	for _, window := range windows {
		name := strings.TrimSpace(window.Name)
		if name == "" || window.Since <= 0 {
			continue
		}
		rows, err := queryChannelAccountUsageWindowAggregate(channelID, name, window.Since, includeHealthProbes)
		if err != nil {
			return nil, err
		}
		topErrors, err := queryChannelAccountUsageTopErrors(channelID, name, window.Since, includeHealthProbes)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			if top, ok := topErrors[channelAccountUsageAggregateKey(rows[i].AccountIdentityKey, rows[i].CredentialIndex)]; ok {
				rows[i].TopErrorCategory = top.TopErrorCategory
				rows[i].TopErrorCount = top.TopErrorCount
			}
		}
		aggregates = append(aggregates, rows...)
	}
	return aggregates, nil
}

func ChannelAccountUsageAggregateKey(accountIdentityKey string, credentialIndex int) string {
	return channelAccountUsageAggregateKey(accountIdentityKey, credentialIndex)
}

func queryChannelAccountUsageWindowAggregate(channelID int, window string, since int64, includeHealthProbes bool) ([]ChannelAccountUsageWindowAggregate, error) {
	rows := make([]ChannelAccountUsageWindowAggregate, 0)
	query := DB.Model(&ChannelAccountUsageEvent{}).
		Select(
			"? AS window_name, account_identity_key, credential_index, COUNT(*) AS requests, "+
				"COALESCE(SUM(CASE WHEN success = ? THEN 1 ELSE 0 END), 0) AS success_requests, "+
				"COALESCE(SUM(CASE WHEN success = ? THEN 0 ELSE 1 END), 0) AS error_requests, "+
				"COALESCE(SUM(CASE WHEN error_category = ? OR status_code = ? OR status_code = ? OR status_code = ? THEN 1 ELSE 0 END), 0) AS timeout_requests, "+
				"COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens, "+
				"COALESCE(SUM(total_tokens), 0) AS total_tokens, COALESCE(SUM(quota), 0) AS quota, "+
				"COALESCE(SUM(upstream_cost_total), 0) AS upstream_cost_total, "+
				"COALESCE(AVG(CASE WHEN duration_ms > 0 THEN duration_ms ELSE NULL END), 0) AS avg_duration_ms, "+
				"COALESCE(AVG(CASE WHEN ttft_ms > 0 THEN ttft_ms ELSE NULL END), 0) AS avg_ttft_ms, "+
				"COALESCE(MAX(completed_at), 0) AS last_active_at",
			window, true, true, ModelGatewayUserRequestErrorTimeout, 408, 504, 524,
		).
		Where("channel_id = ? AND completed_at >= ?", channelID, since).
		Group("account_identity_key, credential_index")
	if !includeHealthProbes {
		query = query.Where("is_health_probe = ?", false)
	}
	return rows, query.Scan(&rows).Error
}

func queryChannelAccountUsageTopErrors(channelID int, window string, since int64, includeHealthProbes bool) (map[string]ChannelAccountUsageWindowAggregate, error) {
	type row struct {
		AccountIdentityKey string
		CredentialIndex    int
		ErrorCategory      string
		ErrorCount         int64
	}
	rows := make([]row, 0)
	query := DB.Model(&ChannelAccountUsageEvent{}).
		Select("account_identity_key, credential_index, error_category, COUNT(*) AS error_count").
		Where("channel_id = ? AND completed_at >= ? AND success = ? AND error_category <> ?", channelID, since, false, "").
		Group("account_identity_key, credential_index, error_category").
		Order("error_count DESC")
	if !includeHealthProbes {
		query = query.Where("is_health_probe = ?", false)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]ChannelAccountUsageWindowAggregate)
	for _, item := range rows {
		key := channelAccountUsageAggregateKey(item.AccountIdentityKey, item.CredentialIndex)
		if _, exists := result[key]; exists {
			continue
		}
		result[key] = ChannelAccountUsageWindowAggregate{
			Window:             window,
			AccountIdentityKey: strings.TrimSpace(item.AccountIdentityKey),
			CredentialIndex:    item.CredentialIndex,
			TopErrorCategory:   strings.TrimSpace(item.ErrorCategory),
			TopErrorCount:      item.ErrorCount,
		}
	}
	return result, nil
}

func upsertChannelAccountUsageEvent(event ChannelAccountUsageEvent, updates map[string]any) error {
	if DB == nil {
		return nil
	}
	event = normalizeChannelAccountUsageEvent(event)
	if event.RequestId == "" {
		return nil
	}
	if len(updates) == 0 {
		updates = baseChannelAccountUsageAssignments(event)
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "request_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&event).Error
}

func normalizeChannelAccountUsageEvent(event ChannelAccountUsageEvent) ChannelAccountUsageEvent {
	now := common.GetTimestamp()
	event.RequestId = strings.TrimSpace(event.RequestId)
	event.ChannelName = strings.TrimSpace(event.ChannelName)
	event.AccountID = strings.TrimSpace(event.AccountID)
	event.AccountIdentityKey = strings.TrimSpace(event.AccountIdentityKey)
	if event.AccountIdentityKey == "" {
		event.AccountIdentityKey = event.AccountID
	}
	event.CredentialSubjectFingerprint = strings.TrimSpace(event.CredentialSubjectFingerprint)
	event.CredentialFingerprint = strings.TrimSpace(event.CredentialFingerprint)
	event.AccountType = strings.TrimSpace(event.AccountType)
	event.Brand = strings.TrimSpace(event.Brand)
	event.Provider = strings.TrimSpace(event.Provider)
	event.RequestedModel = strings.TrimSpace(event.RequestedModel)
	event.RequestedGroup = strings.TrimSpace(event.RequestedGroup)
	event.SelectedGroup = strings.TrimSpace(event.SelectedGroup)
	event.EndpointType = strings.TrimSpace(event.EndpointType)
	event.ErrorCategory = strings.TrimSpace(event.ErrorCategory)
	event.CostSource = strings.TrimSpace(event.CostSource)
	event.CostAccuracy = strings.TrimSpace(event.CostAccuracy)
	if event.CostSource == "" {
		event.CostSource = "pending"
	}
	if event.CostAccuracy == "" {
		event.CostAccuracy = "pending"
	}
	if event.CredentialIndex < 0 {
		event.CredentialIndex = unknownChannelAccountCredentialIndex
	}
	if event.CreatedAt <= 0 {
		event.CreatedAt = now
	}
	if event.UpdatedAt <= 0 {
		event.UpdatedAt = now
	}
	return event
}

func baseChannelAccountUsageAssignments(event ChannelAccountUsageEvent) map[string]any {
	return map[string]any{
		"updated_at": event.UpdatedAt,
	}
}

func addChannelAccountUsageIdentityAssignments(updates map[string]any, event ChannelAccountUsageEvent) {
	if event.ChannelID > 0 {
		updates["channel_id"] = event.ChannelID
	}
	if event.ChannelName != "" {
		updates["channel_name"] = event.ChannelName
	}
	if event.CredentialIndex >= 0 {
		updates["credential_index"] = event.CredentialIndex
	}
	if event.AccountID != "" {
		updates["account_id"] = event.AccountID
	}
	if event.AccountIdentityKey != "" {
		updates["account_identity_key"] = event.AccountIdentityKey
	}
	if event.CredentialSubjectFingerprint != "" {
		updates["credential_subject_fingerprint"] = event.CredentialSubjectFingerprint
	}
	if event.CredentialFingerprint != "" {
		updates["credential_fingerprint"] = event.CredentialFingerprint
	}
	if event.AccountType != "" {
		updates["account_type"] = event.AccountType
	}
	if event.Brand != "" {
		updates["brand"] = event.Brand
	}
	if event.Provider != "" {
		updates["provider"] = event.Provider
	}
	if event.ProviderSurface != "" {
		updates["provider_surface"] = event.ProviderSurface
	}
	if event.CapabilityClassification != "" {
		updates["capability_classification"] = event.CapabilityClassification
	}
	if event.ProxyID > 0 {
		updates["proxy_id"] = event.ProxyID
	}
}

func addChannelAccountUsageRequestAssignments(updates map[string]any, event ChannelAccountUsageEvent) {
	if event.RequestedModel != "" {
		updates["requested_model"] = event.RequestedModel
	}
	if event.RequestedGroup != "" {
		updates["requested_group"] = event.RequestedGroup
	}
	if event.SelectedGroup != "" {
		updates["selected_group"] = event.SelectedGroup
	}
	if event.EndpointType != "" {
		updates["endpoint_type"] = event.EndpointType
	}
}

func channelAccountUsageAggregateKey(accountIdentityKey string, credentialIndex int) string {
	accountIdentityKey = strings.TrimSpace(accountIdentityKey)
	if accountIdentityKey != "" {
		return "identity:" + accountIdentityKey
	}
	return fmt.Sprintf("credential:%d", credentialIndex)
}
