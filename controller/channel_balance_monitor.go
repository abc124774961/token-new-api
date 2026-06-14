package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	channelBalanceMonitorScopeAccount = "account"
	channelBalanceMonitorScopeChannel = "channel"
	channelBalanceMonitorScopeRatio   = "ratio"

	channelBalanceMonitorEventBalanceOK        = "balance_ok"
	channelBalanceMonitorEventBalanceLow       = "balance_low"
	channelBalanceMonitorEventBalanceEmpty     = "balance_empty"
	channelBalanceMonitorEventBalanceRecovered = "balance_recovered"
	channelBalanceMonitorEventUnsupported      = "unsupported"
	channelBalanceMonitorEventRefreshFailed    = "refresh_failed"
	channelBalanceMonitorEventRatioApplied     = "ratio_applied"
	channelBalanceMonitorEventRatioConflict    = "ratio_conflict"
	channelBalanceMonitorEventRatioFailed      = "ratio_failed"
	channelBalanceMonitorEventCostApplied      = "cost_applied"
	channelBalanceMonitorEventCostConflict     = "cost_conflict"
	channelBalanceMonitorEventCostFailed       = "cost_failed"

	channelBalanceMonitorStatusOK          = "ok"
	channelBalanceMonitorStatusLow         = "low"
	channelBalanceMonitorStatusEmpty       = "empty"
	channelBalanceMonitorStatusUnsupported = "unsupported"
	channelBalanceMonitorStatusError       = "error"

	channelBalanceInsufficientAccountReason = "balance_insufficient"
	channelBalanceAllAccountsDisabledReason = "All keys are disabled"
)

type ChannelBalanceMonitorSummary struct {
	AccountTotal        int   `json:"account_total"`
	LowBalanceAccounts  int   `json:"low_balance_accounts"`
	EmptyAccounts       int   `json:"empty_accounts"`
	UnsupportedAccounts int   `json:"unsupported_accounts"`
	ErrorAccounts       int   `json:"error_accounts"`
	AffectedChannels    int   `json:"affected_channels"`
	RatioAutoApplied    int64 `json:"ratio_auto_applied"`
	RatioConflicts      int64 `json:"ratio_conflicts"`
	LastSyncTime        int64 `json:"last_sync_time"`
}

type ChannelBalanceMonitorAccountItem struct {
	ChannelID          int                               `json:"channel_id"`
	ChannelName        string                            `json:"channel_name"`
	ChannelType        int                               `json:"channel_type"`
	ChannelTypeName    string                            `json:"channel_type_name"`
	Group              string                            `json:"group"`
	ChannelStatus      int                               `json:"channel_status"`
	IsMultiKey         bool                              `json:"is_multi_key"`
	CredentialIndex    int                               `json:"credential_index"`
	AccountID          string                            `json:"account_id"`
	AccountIdentityKey string                            `json:"account_identity_key"`
	AccountDisplayName string                            `json:"account_display_name"`
	AccountType        string                            `json:"account_type"`
	KeyEnabled         bool                              `json:"key_enabled"`
	DisabledReason     string                            `json:"disabled_reason,omitempty"`
	Balance            float64                           `json:"balance"`
	Threshold          float64                           `json:"threshold"`
	Status             string                            `json:"status"`
	Supported          bool                              `json:"supported"`
	LastUpdatedTime    int64                             `json:"last_updated_time"`
	LastEventTime      int64                             `json:"last_event_time"`
	LastError          string                            `json:"last_error,omitempty"`
	Models             string                            `json:"models,omitempty"`
	AffectedChannels   []string                          `json:"affected_channels,omitempty"`
	RatioSummary       ChannelBalanceMonitorRatioSummary `json:"ratio_summary"`
}

type ChannelBalanceMonitorChannelItem struct {
	ChannelID           int                               `json:"channel_id"`
	ChannelName         string                            `json:"channel_name"`
	ChannelType         int                               `json:"channel_type"`
	ChannelTypeName     string                            `json:"channel_type_name"`
	Group               string                            `json:"group"`
	Status              int                               `json:"status"`
	IsMultiKey          bool                              `json:"is_multi_key"`
	AccountTotal        int                               `json:"account_total"`
	EnabledAccounts     int                               `json:"enabled_accounts"`
	LowBalanceAccounts  int                               `json:"low_balance_accounts"`
	EmptyAccounts       int                               `json:"empty_accounts"`
	UnsupportedAccounts int                               `json:"unsupported_accounts"`
	ErrorAccounts       int                               `json:"error_accounts"`
	AggregateStatus     string                            `json:"aggregate_status"`
	RatioSummary        ChannelBalanceMonitorRatioSummary `json:"ratio_summary"`
}

type ChannelBalanceMonitorRatioSummary struct {
	Configured            bool    `json:"configured"`
	PriceConfigured       bool    `json:"price_configured"`
	PricingMode           string  `json:"pricing_mode,omitempty"`
	CostMultiplier        float64 `json:"cost_multiplier"`
	ActualTokenMultiplier float64 `json:"actual_token_multiplier"`
	CostCoefficient       float64 `json:"cost_coefficient"`
	TokenMultiplier       float64 `json:"token_multiplier"`
	RechargeMultiplier    float64 `json:"recharge_multiplier"`
	RequestPrice          float64 `json:"request_price,omitempty"`
	ActualRequestPrice    float64 `json:"actual_request_price,omitempty"`
	Source                string  `json:"source,omitempty"`
	Accuracy              string  `json:"accuracy,omitempty"`
	UpdatedAt             int64   `json:"updated_at,omitempty"`
	SyncedAt              int64   `json:"synced_at,omitempty"`
}

type ChannelBalanceMonitorSettings struct {
	Enabled                  bool    `json:"enabled"`
	BalanceIntervalMinutes   float64 `json:"balance_interval_minutes"`
	WarningThreshold         float64 `json:"warning_threshold"`
	RatioSyncEnabled         bool    `json:"ratio_sync_enabled"`
	RatioSyncIntervalMinutes float64 `json:"ratio_sync_interval_minutes"`
	RatioTrustedAutoApply    bool    `json:"ratio_trusted_auto_apply"`
	RetentionDays            int     `json:"retention_days"`
}

type ChannelBalanceMonitorResponse struct {
	Summary  ChannelBalanceMonitorSummary       `json:"summary"`
	Accounts []ChannelBalanceMonitorAccountItem `json:"accounts"`
	Channels []ChannelBalanceMonitorChannelItem `json:"channels"`
	Events   []model.ChannelBalanceMonitorEvent `json:"events"`
	Settings ChannelBalanceMonitorSettings      `json:"settings"`
}

type ChannelBalanceMonitorRefreshRequest struct {
	Mode            string `json:"mode"`
	ChannelID       int    `json:"channel_id,omitempty"`
	CredentialIndex *int   `json:"credential_index,omitempty"`
}

type channelBalanceMonitorAccountRef struct {
	channel *model.Channel
	account modelgatewayaccount.ChannelAccount
}

type channelBalanceMonitorRunResult struct {
	BalanceChecked int `json:"balance_checked"`
	BalanceErrors  int `json:"balance_errors"`
	RatioApplied   int `json:"ratio_applied"`
	RatioConflicts int `json:"ratio_conflicts"`
}

var channelBalanceMonitorRunMu sync.Mutex
var channelBalanceMonitorOnce sync.Once

func GetChannelBalanceMonitor(c *gin.Context) {
	response, err := buildChannelBalanceMonitorResponse(20)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func GetChannelBalanceMonitorEvents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var total int64
	query := model.DB.Model(&model.ChannelBalanceMonitorEvent{})
	if scope := strings.TrimSpace(c.Query("scope")); scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if eventType := strings.TrimSpace(c.Query("event_type")); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if channelID, err := strconv.Atoi(c.Query("channel_id")); err == nil && channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	events := make([]model.ChannelBalanceMonitorEvent, 0)
	if err := query.
		Order("created_time DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&events).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     events,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func RefreshChannelBalanceMonitor(c *gin.Context) {
	var request ChannelBalanceMonitorRefreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(request.Mode) == "" {
		request.Mode = "all"
	}
	result, err := runChannelBalanceMonitorOnce(c.Request.Context(), request, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response, err := buildChannelBalanceMonitorResponse(20)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"operation": result,
		"monitor":   response,
	})
}

func StartChannelBalanceMonitorTask() {
	if !common.IsMasterNode {
		return
	}
	channelBalanceMonitorOnce.Do(func() {
		go func() {
			for {
				setting := operation_setting.GetMonitorSetting()
				if !setting.ChannelBalanceMonitorEnabled {
					time.Sleep(time.Minute)
					continue
				}
				time.Sleep(channelBalanceMonitorIntervalDuration(setting.ChannelBalanceMonitorIntervalMinutes, 10))
				_, err := runChannelBalanceMonitorOnce(context.Background(), ChannelBalanceMonitorRefreshRequest{Mode: "balance"}, false)
				if err != nil {
					common.SysLog(fmt.Sprintf("channel balance monitor task failed: %s", err.Error()))
				}
			}
		}()
		go func() {
			for {
				setting := operation_setting.GetMonitorSetting()
				if !setting.ChannelRatioSyncEnabled {
					time.Sleep(time.Minute)
					continue
				}
				time.Sleep(channelBalanceMonitorIntervalDuration(setting.ChannelRatioSyncIntervalMinutes, 60))
				_, err := runChannelBalanceMonitorOnce(context.Background(), ChannelBalanceMonitorRefreshRequest{Mode: "ratio"}, false)
				if err != nil {
					common.SysLog(fmt.Sprintf("channel ratio sync monitor task failed: %s", err.Error()))
				}
			}
		}()
	})
}

func channelBalanceMonitorIntervalDuration(minutes float64, fallbackMinutes int) time.Duration {
	if minutes <= 0 {
		minutes = float64(fallbackMinutes)
	}
	rounded := int(math.Round(minutes))
	if rounded <= 0 {
		rounded = fallbackMinutes
	}
	return time.Duration(rounded) * time.Minute
}

func buildChannelBalanceMonitorResponse(eventLimit int) (*ChannelBalanceMonitorResponse, error) {
	accounts, channels, summary, err := buildChannelBalanceMonitorItems()
	if err != nil {
		return nil, err
	}
	events := make([]model.ChannelBalanceMonitorEvent, 0)
	if eventLimit > 0 {
		_ = model.DB.Order("created_time DESC").Limit(eventLimit).Find(&events).Error
	}
	summary.RatioAutoApplied, summary.RatioConflicts = countRecentRatioMonitorEvents()
	summary.LastSyncTime = latestMonitorEventTime()
	return &ChannelBalanceMonitorResponse{
		Summary:  summary,
		Accounts: accounts,
		Channels: channels,
		Events:   events,
		Settings: channelBalanceMonitorSettings(),
	}, nil
}

func channelBalanceMonitorSettings() ChannelBalanceMonitorSettings {
	setting := operation_setting.GetMonitorSetting()
	retentionDays := setting.ChannelBalanceMonitorRetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}
	threshold := setting.ChannelBalanceWarningThreshold
	if threshold <= 0 {
		threshold = 10
	}
	return ChannelBalanceMonitorSettings{
		Enabled:                  setting.ChannelBalanceMonitorEnabled,
		BalanceIntervalMinutes:   setting.ChannelBalanceMonitorIntervalMinutes,
		WarningThreshold:         threshold,
		RatioSyncEnabled:         setting.ChannelRatioSyncEnabled,
		RatioSyncIntervalMinutes: setting.ChannelRatioSyncIntervalMinutes,
		RatioTrustedAutoApply:    setting.ChannelRatioSyncTrustedAutoApply,
		RetentionDays:            retentionDays,
	}
}

func buildChannelBalanceMonitorItems() ([]ChannelBalanceMonitorAccountItem, []ChannelBalanceMonitorChannelItem, ChannelBalanceMonitorSummary, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, nil, ChannelBalanceMonitorSummary{}, err
	}
	refs := buildChannelBalanceMonitorAccountRefs(channels)
	latestEvents := latestAccountMonitorEvents()
	threshold := channelBalanceMonitorSettings().WarningThreshold
	affectedChannels := buildAffectedChannelLabelsByAccount(refs)
	costProfiles := loadChannelBalanceMonitorCostProfiles(channels)
	accounts := make([]ChannelBalanceMonitorAccountItem, 0, len(refs))
	channelMap := make(map[int]*ChannelBalanceMonitorChannelItem)
	for _, ref := range refs {
		item := buildChannelBalanceMonitorAccountItem(ref, latestEvents[accountMonitorKey(ref.channel.Id, ref.account.CredentialIndex, ref.account.AccountIdentity.AccountID)], threshold, costProfiles[ref.channel.Id])
		item.AffectedChannels = affectedChannels[strings.TrimSpace(ref.account.AccountIdentity.AccountIdentityKey)]
		accounts = append(accounts, item)
		channelItem := channelMap[item.ChannelID]
		if channelItem == nil {
			channelItem = &ChannelBalanceMonitorChannelItem{
				ChannelID:       item.ChannelID,
				ChannelName:     item.ChannelName,
				ChannelType:     item.ChannelType,
				ChannelTypeName: item.ChannelTypeName,
				Group:           item.Group,
				Status:          item.ChannelStatus,
				IsMultiKey:      item.IsMultiKey,
				AggregateStatus: channelBalanceMonitorStatusOK,
				RatioSummary:    item.RatioSummary,
			}
			channelMap[item.ChannelID] = channelItem
		}
		channelItem.AccountTotal++
		if item.KeyEnabled {
			channelItem.EnabledAccounts++
		}
		switch item.Status {
		case channelBalanceMonitorStatusLow:
			channelItem.LowBalanceAccounts++
		case channelBalanceMonitorStatusEmpty:
			channelItem.EmptyAccounts++
		case channelBalanceMonitorStatusUnsupported:
			channelItem.UnsupportedAccounts++
		case channelBalanceMonitorStatusError:
			channelItem.ErrorAccounts++
		}
		channelItem.AggregateStatus = aggregateChannelBalanceMonitorStatus(*channelItem)
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Status != accounts[j].Status {
			return monitorStatusRank(accounts[i].Status) > monitorStatusRank(accounts[j].Status)
		}
		if accounts[i].ChannelID != accounts[j].ChannelID {
			return accounts[i].ChannelID < accounts[j].ChannelID
		}
		return accounts[i].CredentialIndex < accounts[j].CredentialIndex
	})
	channelItems := make([]ChannelBalanceMonitorChannelItem, 0, len(channelMap))
	for _, item := range channelMap {
		channelItems = append(channelItems, *item)
	}
	sort.Slice(channelItems, func(i, j int) bool {
		if channelItems[i].AggregateStatus != channelItems[j].AggregateStatus {
			return monitorStatusRank(channelItems[i].AggregateStatus) > monitorStatusRank(channelItems[j].AggregateStatus)
		}
		return channelItems[i].ChannelID < channelItems[j].ChannelID
	})
	summary := ChannelBalanceMonitorSummary{AccountTotal: len(accounts)}
	affected := make(map[int]struct{})
	for _, account := range accounts {
		switch account.Status {
		case channelBalanceMonitorStatusLow:
			summary.LowBalanceAccounts++
			affected[account.ChannelID] = struct{}{}
		case channelBalanceMonitorStatusEmpty:
			summary.EmptyAccounts++
			affected[account.ChannelID] = struct{}{}
		case channelBalanceMonitorStatusUnsupported:
			summary.UnsupportedAccounts++
		case channelBalanceMonitorStatusError:
			summary.ErrorAccounts++
			affected[account.ChannelID] = struct{}{}
		}
	}
	summary.AffectedChannels = len(affected)
	return accounts, channelItems, summary, nil
}

func buildAffectedChannelLabelsByAccount(refs []channelBalanceMonitorAccountRef) map[string][]string {
	labels := make(map[string][]string)
	seen := make(map[string]map[int]struct{})
	for _, ref := range refs {
		if ref.channel == nil {
			continue
		}
		identityKey := strings.TrimSpace(ref.account.AccountIdentity.AccountIdentityKey)
		if identityKey == "" {
			continue
		}
		if seen[identityKey] == nil {
			seen[identityKey] = make(map[int]struct{})
		}
		if _, exists := seen[identityKey][ref.channel.Id]; exists {
			continue
		}
		seen[identityKey][ref.channel.Id] = struct{}{}
		labels[identityKey] = append(labels[identityKey], fmt.Sprintf("%s(#%d)", ref.channel.Name, ref.channel.Id))
	}
	for identityKey := range labels {
		sort.Strings(labels[identityKey])
	}
	return labels
}

func loadChannelBalanceMonitorCostProfiles(channels []*model.Channel) map[int]model.ModelGatewayChannelCostProfile {
	result := make(map[int]model.ModelGatewayChannelCostProfile)
	if len(channels) == 0 || model.DB == nil {
		return result
	}
	ids := make([]int, 0, len(channels))
	seen := make(map[int]struct{}, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 {
			continue
		}
		if _, exists := seen[channel.Id]; exists {
			continue
		}
		seen[channel.Id] = struct{}{}
		ids = append(ids, channel.Id)
	}
	if len(ids) == 0 {
		return result
	}

	profiles := make([]model.ModelGatewayChannelCostProfile, 0, len(ids))
	if err := model.DB.
		Where("channel_id IN ? AND upstream_model = ?", ids, defaultChannelCostModel).
		Find(&profiles).Error; err != nil {
		common.SysError("failed to load channel balance monitor cost profiles: " + err.Error())
		return result
	}
	now := common.GetTimestamp()
	for _, profile := range profiles {
		if profile.ChannelID <= 0 || profile.EffectiveTime > now {
			continue
		}
		current, exists := result[profile.ChannelID]
		if !exists || betterChannelCostDisplayProfile(profile, current) {
			result[profile.ChannelID] = profile
		}
	}
	return result
}

func buildChannelBalanceMonitorRatioSummary(profile model.ModelGatewayChannelCostProfile, configured bool) ChannelBalanceMonitorRatioSummary {
	if !configured || profile.Id <= 0 {
		profile = defaultChannelCostProfile(profile.ChannelID)
		configured = false
	}
	costCoefficient := positiveOrDefault(profile.CostCoefficient, 1)
	tokenMultiplier := positiveOrDefault(profile.TokenMultiplier, profile.InputCostMultiplier)
	tokenMultiplier = positiveOrDefault(tokenMultiplier, 1)
	rechargeMultiplier := positiveOrDefault(profile.RechargeMultiplier, 1)
	actualTokenMultiplier := roundRatioValue(costCoefficient * tokenMultiplier / rechargeMultiplier)
	requestPrice := nonNegativeFloat(profile.RequestPrice)
	actualRequestPrice := 0.0
	if requestPrice > 0 {
		actualRequestPrice = roundRatioValue(requestPrice / rechargeMultiplier)
	}
	pricingMode := strings.TrimSpace(profile.PricingMode)
	if pricingMode == "" {
		pricingMode = defaultChannelCostPricingMode
	}
	return ChannelBalanceMonitorRatioSummary{
		Configured:            configured,
		PriceConfigured:       channelBalanceMonitorCostProfileHasPrice(profile),
		PricingMode:           pricingMode,
		CostMultiplier:        actualTokenMultiplier,
		ActualTokenMultiplier: actualTokenMultiplier,
		CostCoefficient:       costCoefficient,
		TokenMultiplier:       tokenMultiplier,
		RechargeMultiplier:    rechargeMultiplier,
		RequestPrice:          requestPrice,
		ActualRequestPrice:    actualRequestPrice,
		Source:                strings.TrimSpace(profile.Source),
		Accuracy:              strings.TrimSpace(profile.Accuracy),
		UpdatedAt:             profile.UpdatedAt,
		SyncedAt:              profile.SyncedAt,
	}
}

func positiveOrDefault(value float64, fallback float64) float64 {
	if value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return value
	}
	if fallback > 0 && !math.IsNaN(fallback) && !math.IsInf(fallback, 0) {
		return fallback
	}
	return 1
}

func nonNegativeFloat(value float64) float64 {
	if value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return value
	}
	return 0
}

func channelBalanceMonitorCostProfileHasPrice(profile model.ModelGatewayChannelCostProfile) bool {
	return profile.InputPerMillion > 0 ||
		profile.OutputPerMillion > 0 ||
		profile.CacheReadPerMillion > 0 ||
		profile.CacheWritePerMillion > 0 ||
		profile.CacheWrite5mPerMillion > 0 ||
		profile.CacheWrite1hPerMillion > 0 ||
		profile.ImageInputPerMillion > 0 ||
		profile.ImageOutputPerMillion > 0 ||
		profile.AudioInputPerMillion > 0 ||
		profile.AudioOutputPerMillion > 0 ||
		profile.RequestPrice > 0
}

func buildChannelBalanceMonitorAccountRefs(channels []*model.Channel) []channelBalanceMonitorAccountRef {
	registry := modelgatewayaccount.NewRegistry()
	refs := make([]channelBalanceMonitorAccountRef, 0, len(channels))
	for _, channel := range channels {
		accounts := registry.AccountsForChannel(channel)
		for _, account := range accounts {
			refs = append(refs, channelBalanceMonitorAccountRef{
				channel: channel,
				account: account,
			})
		}
	}
	return refs
}

func buildChannelBalanceMonitorAccountItem(ref channelBalanceMonitorAccountRef, latest *model.ChannelBalanceMonitorEvent, threshold float64, costProfile model.ModelGatewayChannelCostProfile) ChannelBalanceMonitorAccountItem {
	channel := ref.channel
	account := ref.account
	item := ChannelBalanceMonitorAccountItem{
		ChannelID:          channel.Id,
		ChannelName:        channel.Name,
		ChannelType:        channel.Type,
		ChannelTypeName:    constant.GetChannelTypeName(channel.Type),
		Group:              channel.Group,
		ChannelStatus:      channel.Status,
		IsMultiKey:         channel.ChannelInfo.IsMultiKey,
		CredentialIndex:    account.CredentialIndex,
		AccountID:          account.AccountIdentity.AccountID,
		AccountIdentityKey: account.AccountIdentity.AccountIdentityKey,
		AccountDisplayName: account.AccountIdentity.DisplayName,
		AccountType:        account.AccountIdentity.AccountType,
		KeyEnabled:         account.KeyEnabled,
		DisabledReason:     account.DisabledReason,
		Threshold:          threshold,
		Supported:          supportsChannelBalanceMonitor(channel),
		Models:             channel.Models,
		RatioSummary:       buildChannelBalanceMonitorRatioSummary(costProfile, costProfile.Id > 0),
		Status:             channelBalanceMonitorStatusUnsupported,
	}
	if latest != nil {
		item.Balance = latest.Balance
		item.LastEventTime = latest.CreatedTime
		item.LastUpdatedTime = latest.CreatedTime
		item.LastError = latest.Error
		item.Status = statusFromBalanceMonitorEvent(*latest, threshold)
	} else if !channel.ChannelInfo.IsMultiKey && channel.BalanceUpdatedTime > 0 {
		item.Balance = channel.Balance
		item.LastUpdatedTime = channel.BalanceUpdatedTime
		item.Status = statusFromBalance(channel.Balance, threshold)
	} else if item.Supported {
		item.Status = channelBalanceMonitorStatusOK
	}
	if !item.Supported && latest != nil && latest.EventType == channelBalanceMonitorEventRefreshFailed {
		item.Status = channelBalanceMonitorStatusError
	}
	if service.IsBalanceInsufficientPausedChannel(channel) && !channel.ChannelInfo.IsMultiKey {
		item.Status = channelBalanceMonitorStatusEmpty
	}
	if channel.ChannelInfo.IsMultiKey && service.IsBalanceInsufficientStatusReason(account.DisabledReason) {
		item.Status = channelBalanceMonitorStatusEmpty
	}
	return item
}

func supportsChannelBalanceMonitor(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	switch channel.Type {
	case constant.ChannelTypeAIProxy,
		constant.ChannelTypeAPI2GPT,
		constant.ChannelTypeAIGC2D,
		constant.ChannelTypeSiliconFlow,
		constant.ChannelTypeOpenRouter,
		constant.ChannelTypeMoonshot:
		return true
	default:
		return isOpenAICompatibleBalanceChannel(channel.Type)
	}
}

func statusFromBalance(balance float64, threshold float64) string {
	if balance <= 0 {
		return channelBalanceMonitorStatusEmpty
	}
	if balance < threshold {
		return channelBalanceMonitorStatusLow
	}
	return channelBalanceMonitorStatusOK
}

func statusFromBalanceMonitorEvent(event model.ChannelBalanceMonitorEvent, threshold float64) string {
	switch event.EventType {
	case channelBalanceMonitorEventBalanceLow:
		return channelBalanceMonitorStatusLow
	case channelBalanceMonitorEventBalanceEmpty:
		return channelBalanceMonitorStatusEmpty
	case channelBalanceMonitorEventUnsupported:
		return channelBalanceMonitorStatusUnsupported
	case channelBalanceMonitorEventRefreshFailed:
		return channelBalanceMonitorStatusError
	default:
		if event.Balance != 0 || event.EventType == channelBalanceMonitorEventBalanceOK || event.EventType == channelBalanceMonitorEventBalanceRecovered {
			return statusFromBalance(event.Balance, threshold)
		}
		return channelBalanceMonitorStatusOK
	}
}

func monitorStatusRank(status string) int {
	switch status {
	case channelBalanceMonitorStatusError:
		return 5
	case channelBalanceMonitorStatusEmpty:
		return 4
	case channelBalanceMonitorStatusLow:
		return 3
	case channelBalanceMonitorStatusUnsupported:
		return 2
	default:
		return 1
	}
}

func aggregateChannelBalanceMonitorStatus(item ChannelBalanceMonitorChannelItem) string {
	if item.AccountTotal > 0 && item.EmptyAccounts >= item.AccountTotal {
		return channelBalanceMonitorStatusEmpty
	}
	if item.ErrorAccounts > 0 {
		return channelBalanceMonitorStatusError
	}
	if item.EmptyAccounts > 0 || item.LowBalanceAccounts > 0 {
		return channelBalanceMonitorStatusLow
	}
	if item.UnsupportedAccounts >= item.AccountTotal && item.AccountTotal > 0 {
		return channelBalanceMonitorStatusUnsupported
	}
	return channelBalanceMonitorStatusOK
}

func latestAccountMonitorEvents() map[string]*model.ChannelBalanceMonitorEvent {
	events := make([]model.ChannelBalanceMonitorEvent, 0)
	_ = model.DB.
		Where("scope = ?", channelBalanceMonitorScopeAccount).
		Order("created_time DESC").
		Limit(5000).
		Find(&events).Error
	result := make(map[string]*model.ChannelBalanceMonitorEvent)
	for i := range events {
		event := events[i]
		key := accountMonitorKey(event.ChannelID, event.CredentialIndex, event.AccountID)
		if _, exists := result[key]; exists {
			continue
		}
		result[key] = &event
	}
	return result
}

func accountMonitorKey(channelID int, credentialIndex int, accountID string) string {
	return fmt.Sprintf("%d:%d:%s", channelID, credentialIndex, strings.TrimSpace(accountID))
}

func countRecentRatioMonitorEvents() (int64, int64) {
	retentionDays := channelBalanceMonitorSettings().RetentionDays
	since := common.GetTimestamp() - int64(retentionDays*86400)
	var applied int64
	var conflicts int64
	_ = model.DB.Model(&model.ChannelBalanceMonitorEvent{}).
		Where("scope = ? AND event_type IN ? AND created_time >= ?", channelBalanceMonitorScopeRatio, []string{channelBalanceMonitorEventRatioApplied, channelBalanceMonitorEventCostApplied}, since).
		Count(&applied).Error
	_ = model.DB.Model(&model.ChannelBalanceMonitorEvent{}).
		Where("scope = ? AND event_type IN ? AND created_time >= ?", channelBalanceMonitorScopeRatio, []string{channelBalanceMonitorEventRatioConflict, channelBalanceMonitorEventCostConflict}, since).
		Count(&conflicts).Error
	return applied, conflicts
}

func latestMonitorEventTime() int64 {
	var event model.ChannelBalanceMonitorEvent
	if err := model.DB.Order("created_time DESC").First(&event).Error; err != nil {
		return 0
	}
	return event.CreatedTime
}

func runChannelBalanceMonitorOnce(ctx context.Context, request ChannelBalanceMonitorRefreshRequest, force bool) (*channelBalanceMonitorRunResult, error) {
	if !channelBalanceMonitorRunMu.TryLock() {
		return nil, fmt.Errorf("渠道余额监控正在运行")
	}
	defer channelBalanceMonitorRunMu.Unlock()

	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = "all"
	}
	result := &channelBalanceMonitorRunResult{}
	if mode == "all" || mode == "balance" {
		checked, errorsCount, err := runChannelBalanceRefresh(ctx, request)
		result.BalanceChecked = checked
		result.BalanceErrors = errorsCount
		if err != nil && force {
			return result, err
		}
	}
	if mode == "all" || mode == "ratio" {
		applied, conflicts, err := runChannelRatioMonitorSync(ctx, force)
		result.RatioApplied = applied
		result.RatioConflicts = conflicts
		if err != nil && force {
			return result, err
		}
	}
	cleanupChannelBalanceMonitorEvents()
	return result, nil
}

func runChannelBalanceRefresh(ctx context.Context, request ChannelBalanceMonitorRefreshRequest) (int, int, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return 0, 0, err
	}
	refs := buildChannelBalanceMonitorAccountRefs(channels)
	checked := 0
	errorsCount := 0
	for _, ref := range refs {
		if ctx.Err() != nil {
			return checked, errorsCount, ctx.Err()
		}
		if request.ChannelID > 0 && ref.channel.Id != request.ChannelID {
			continue
		}
		if request.CredentialIndex != nil && ref.account.CredentialIndex != *request.CredentialIndex {
			continue
		}
		checked++
		if err := refreshChannelBalanceMonitorAccount(ref); err != nil {
			errorsCount++
		}
		time.Sleep(common.RequestInterval)
	}
	return checked, errorsCount, nil
}

func refreshChannelBalanceMonitorAccount(ref channelBalanceMonitorAccountRef) error {
	channel := ref.channel
	account := ref.account
	threshold := channelBalanceMonitorSettings().WarningThreshold
	if !supportsChannelBalanceMonitor(channel) {
		recordChannelBalanceMonitorEvent(channel, account, channelBalanceMonitorEventUnsupported, 0, threshold, "暂不支持账号余额查询", nil)
		return fmt.Errorf("unsupported channel type")
	}
	accountChannel := cloneChannelForBalanceAccount(channel, account.CredentialIndex)
	result, err := fetchChannelBalanceResult(accountChannel)
	if err != nil {
		recordChannelBalanceMonitorEvent(channel, account, channelBalanceMonitorEventRefreshFailed, 0, threshold, err.Error(), nil)
		return err
	}
	balance := result.Balance
	if !channel.ChannelInfo.IsMultiKey {
		channel.UpdateBalance(balance)
	}
	eventType := channelBalanceMonitorEventBalanceOK
	if balance <= 0 {
		eventType = channelBalanceMonitorEventBalanceEmpty
	} else if balance < threshold {
		eventType = channelBalanceMonitorEventBalanceLow
	} else if wasAccountBalanceInsufficient(channel, account) {
		eventType = channelBalanceMonitorEventBalanceRecovered
	}
	recordChannelBalanceMonitorEvent(channel, account, eventType, balance, threshold, "", map[string]any{
		"key_enabled":      account.KeyEnabled,
		"is_multi_key":     channel.ChannelInfo.IsMultiKey,
		"balance_source":   result.Source,
		"balance_endpoint": result.Endpoint,
		"balance_currency": result.Currency,
		"balance_raw_unit": result.RawUnit,
	})
	reconcileChannelAccountBalanceStatus(channel, account, balance, threshold)
	return nil
}

func cloneChannelForBalanceAccount(channel *model.Channel, credentialIndex int) *model.Channel {
	cloned := *channel
	keys := channel.GetKeys()
	if credentialIndex >= 0 && credentialIndex < len(keys) {
		cloned.Key = strings.TrimSpace(keys[credentialIndex])
	}
	cloned.ChannelInfo.IsMultiKey = false
	cloned.ChannelInfo.MultiKeyStatusList = nil
	cloned.ChannelInfo.MultiKeyDisabledReason = nil
	cloned.ChannelInfo.MultiKeyDisabledTime = nil
	return &cloned
}

func reconcileChannelAccountBalanceStatus(channel *model.Channel, account modelgatewayaccount.ChannelAccount, balance float64, threshold float64) {
	if channel == nil {
		return
	}
	if !channel.ChannelInfo.IsMultiKey {
		reconcileChannelBalanceStatus(channel, balance)
		if balance > 0 && balance < threshold {
			notifyLowBalanceAccount(channel, account, balance, threshold, false)
		}
		if balance > threshold && service.IsBalanceInsufficientPausedChannel(channel) {
			service.EnableChannel(channel.Id, "", channel.Name)
		}
		return
	}
	if balance <= 0 {
		pauseMultiKeyChannelAccountForBalance(channel.Id, account.CredentialIndex)
		notifyLowBalanceAccount(channel, account, balance, threshold, true)
		return
	}
	if balance < threshold {
		notifyLowBalanceAccount(channel, account, balance, threshold, false)
		return
	}
	if wasAccountBalanceInsufficient(channel, account) {
		enableMultiKeyChannelAccountForBalance(channel.Id, account.CredentialIndex)
	}
}

func wasAccountBalanceInsufficient(channel *model.Channel, account modelgatewayaccount.ChannelAccount) bool {
	if channel == nil {
		return false
	}
	if !channel.ChannelInfo.IsMultiKey {
		return service.IsKnownBalanceInsufficientChannel(channel)
	}
	return service.IsBalanceInsufficientStatusReason(account.DisabledReason)
}

func pauseMultiKeyChannelAccountForBalance(channelID int, credentialIndex int) {
	common.SysLog(fmt.Sprintf("channel balance monitor detected empty balance but auto-disable is disabled: channel_id=%d credential_index=%d", channelID, credentialIndex))
}

func enableMultiKeyChannelAccountForBalance(channelID int, credentialIndex int) {
	updateMultiKeyBalanceStatus(channelID, credentialIndex, true)
}

func updateMultiKeyBalanceStatus(channelID int, credentialIndex int, enabled bool) {
	channel, err := model.GetChannelById(channelID, true)
	if err != nil || channel == nil || !channel.ChannelInfo.IsMultiKey {
		return
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return
	}
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()
	if channel.ChannelInfo.MultiKeyStatusList == nil {
		channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
	}
	if channel.ChannelInfo.MultiKeyDisabledReason == nil {
		channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
	}
	if channel.ChannelInfo.MultiKeyDisabledTime == nil {
		channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
	}
	if enabled {
		if !service.IsBalanceInsufficientStatusReason(channel.ChannelInfo.MultiKeyDisabledReason[credentialIndex]) {
			return
		}
		delete(channel.ChannelInfo.MultiKeyStatusList, credentialIndex)
		delete(channel.ChannelInfo.MultiKeyDisabledReason, credentialIndex)
		delete(channel.ChannelInfo.MultiKeyDisabledTime, credentialIndex)
	} else {
		channel.ChannelInfo.MultiKeyStatusList[credentialIndex] = common.ChannelStatusAutoDisabled
		channel.ChannelInfo.MultiKeyDisabledReason[credentialIndex] = channelBalanceInsufficientAccountReason
		channel.ChannelInfo.MultiKeyDisabledTime[credentialIndex] = common.GetTimestamp()
	}
	enabledCount := 0
	for i := range keys {
		status := common.ChannelStatusEnabled
		if value, ok := channel.ChannelInfo.MultiKeyStatusList[i]; ok {
			status = value
		}
		if status == common.ChannelStatusEnabled {
			enabledCount++
		}
	}
	info := channel.GetOtherInfo()
	if enabledCount == 0 {
		channel.Status = common.ChannelStatusAutoDisabled
		info["status_reason"] = channelBalanceAllAccountsDisabledReason
		info["status_time"] = common.GetTimestamp()
		channel.SetOtherInfo(info)
	} else if channel.Status == common.ChannelStatusAutoDisabled {
		reason, _ := info["status_reason"].(string)
		if strings.TrimSpace(reason) == "" || strings.TrimSpace(reason) == channelBalanceAllAccountsDisabledReason {
			channel.Status = common.ChannelStatusEnabled
			delete(info, "status_reason")
			delete(info, "status_time")
			channel.SetOtherInfo(info)
		}
	}
	if err := channel.SaveWithoutKey(); err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel account balance status: channel_id=%d, credential_index=%d, error=%s", channelID, credentialIndex, err.Error()))
		return
	}
	if enabled {
		clearChannelAccountRuntimeBlocks(channelID, credentialIndex, false)
	}
	if common.MemoryCacheEnabled {
		model.CacheUpdateChannel(channel)
	}
	if enabledCount == 0 {
		_ = model.UpdateAbilityStatus(channelID, false)
	} else {
		_ = model.UpdateAbilityStatus(channelID, true)
	}
	modelgatewayintegration.RefreshDefaultRoutingCaches(modelgatewayintegration.RoutingCacheRefreshOptions{
		Reason: "channel_account_balance_status",
	})
}

func recordChannelBalanceMonitorEvent(channel *model.Channel, account modelgatewayaccount.ChannelAccount, eventType string, balance float64, threshold float64, errMessage string, details map[string]any) {
	if channel == nil {
		return
	}
	detailsText := marshalMonitorDetails(details)
	event := model.ChannelBalanceMonitorEvent{
		CreatedTime:        common.GetTimestamp(),
		Scope:              channelBalanceMonitorScopeAccount,
		ChannelID:          channel.Id,
		ChannelName:        channel.Name,
		ChannelType:        channel.Type,
		Group:              channel.Group,
		CredentialIndex:    account.CredentialIndex,
		AccountID:          account.AccountIdentity.AccountID,
		AccountIdentityKey: account.AccountIdentity.AccountIdentityKey,
		Balance:            balance,
		Threshold:          threshold,
		EventType:          eventType,
		Error:              errMessage,
		Details:            detailsText,
	}
	if err := model.DB.Create(&event).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to create channel balance monitor event: %s", err.Error()))
	}
}

func recordRatioMonitorEvent(eventType string, modelName string, field string, autoApplied bool, errMessage string, details map[string]any) {
	event := model.ChannelBalanceMonitorEvent{
		CreatedTime: common.GetTimestamp(),
		Scope:       channelBalanceMonitorScopeRatio,
		EventType:   eventType,
		Field:       field,
		ModelName:   modelName,
		AutoApplied: autoApplied,
		Error:       errMessage,
		Details:     marshalMonitorDetails(details),
	}
	if err := model.DB.Create(&event).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to create ratio monitor event: %s", err.Error()))
	}
}

func marshalMonitorDetails(details map[string]any) string {
	if len(details) == 0 {
		return ""
	}
	bytes, err := common.Marshal(details)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func notifyLowBalanceAccount(channel *model.Channel, account modelgatewayaccount.ChannelAccount, balance float64, threshold float64, empty bool) {
	if channel == nil {
		return
	}
	now := common.GetTimestamp()
	var count int64
	eventType := channelBalanceMonitorEventBalanceLow
	if empty {
		eventType = channelBalanceMonitorEventBalanceEmpty
	}
	dedupWhere, dedupArgs := accountMonitorDedupWhere(channel, account, eventType, now-86400)
	_ = model.DB.Model(&model.ChannelBalanceMonitorEvent{}).
		Where(dedupWhere, dedupArgs...).
		Count(&count).Error
	if count > 1 {
		return
	}
	statusText := "低余额"
	if empty {
		statusText = "余额耗尽"
	}
	subject := fmt.Sprintf("渠道账号%s：%s #%d", statusText, channel.Name, account.CredentialIndex+1)
	content := fmt.Sprintf(
		"渠道「%s」（#%d）的账号 %s（凭证 #%d）%s，当前余额 %.4f USD，告警阈值 %.4f USD。受影响渠道：%s。",
		channel.Name,
		channel.Id,
		account.AccountIdentity.AccountID,
		account.CredentialIndex+1,
		statusText,
		balance,
		threshold,
		strings.Join(affectedChannelLabelsForMonitorAccount(account.AccountIdentity.AccountIdentityKey, channel), "、"),
	)
	service.NotifyRootUser(fmt.Sprintf("%s_%d_%d_%s", dto.NotifyTypeChannelUpdate, channel.Id, account.CredentialIndex, eventType), subject, content)
}

func accountMonitorDedupWhere(channel *model.Channel, account modelgatewayaccount.ChannelAccount, eventType string, since int64) (string, []any) {
	identityKey := strings.TrimSpace(account.AccountIdentity.AccountIdentityKey)
	if identityKey != "" {
		return "scope = ? AND account_identity_key = ? AND event_type = ? AND created_time >= ?",
			[]any{channelBalanceMonitorScopeAccount, identityKey, eventType, since}
	}
	accountID := strings.TrimSpace(account.AccountIdentity.AccountID)
	if accountID != "" {
		return "scope = ? AND account_id = ? AND event_type = ? AND created_time >= ?",
			[]any{channelBalanceMonitorScopeAccount, accountID, eventType, since}
	}
	return "scope = ? AND channel_id = ? AND credential_index = ? AND event_type = ? AND created_time >= ?",
		[]any{channelBalanceMonitorScopeAccount, channel.Id, account.CredentialIndex, eventType, since}
}

func affectedChannelLabelsForMonitorAccount(accountIdentityKey string, current *model.Channel) []string {
	currentLabel := ""
	if current != nil {
		currentLabel = fmt.Sprintf("%s(#%d)", current.Name, current.Id)
	}
	accountIdentityKey = strings.TrimSpace(accountIdentityKey)
	if accountIdentityKey == "" {
		if currentLabel == "" {
			return []string{"--"}
		}
		return []string{currentLabel}
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		if currentLabel == "" {
			return []string{"--"}
		}
		return []string{currentLabel}
	}
	registry := modelgatewayaccount.NewRegistry()
	seen := make(map[int]struct{})
	labels := make([]string, 0, 4)
	for _, channel := range channels {
		for _, account := range registry.AccountsForChannel(channel) {
			if strings.TrimSpace(account.AccountIdentity.AccountIdentityKey) != accountIdentityKey {
				continue
			}
			if _, exists := seen[channel.Id]; exists {
				continue
			}
			seen[channel.Id] = struct{}{}
			labels = append(labels, fmt.Sprintf("%s(#%d)", channel.Name, channel.Id))
			break
		}
	}
	if len(labels) == 0 && currentLabel != "" {
		labels = append(labels, currentLabel)
	}
	if len(labels) > 8 {
		return append(labels[:8], fmt.Sprintf("等 %d 个", len(labels)))
	}
	if len(labels) == 0 {
		return []string{"--"}
	}
	return labels
}

func runChannelRatioMonitorSync(ctx context.Context, force bool) (int, int, error) {
	setting := operation_setting.GetMonitorSetting()
	if !force && !setting.ChannelRatioSyncEnabled {
		return 0, 0, nil
	}
	req := dto.UpstreamRequest{
		Upstreams: []dto.UpstreamDTO{
			{
				ID:       officialRatioPresetID,
				Name:     officialRatioPresetName,
				BaseURL:  officialRatioPresetBaseURL,
				Endpoint: "/llm-metadata/api/newapi/ratio_config-v1-base.json",
			},
			{
				ID:       modelsDevPresetID,
				Name:     modelsDevPresetName,
				BaseURL:  modelsDevPresetBaseURL,
				Endpoint: "https://models.dev/api.json",
			},
		},
		Timeout: defaultTimeoutSeconds,
	}
	result, err := FetchUpstreamRatioDifferences(ctx, req)
	if err != nil {
		recordRatioMonitorEvent(channelBalanceMonitorEventRatioFailed, "", "", false, err.Error(), nil)
		return 0, 0, err
	}
	for _, testResult := range result.TestResults {
		if testResult.Status == "error" {
			recordRatioMonitorEvent(channelBalanceMonitorEventRatioFailed, "", "", false, testResult.Error, map[string]any{"source": testResult.Name})
		}
	}
	applied, conflicts := applyUpstreamChannelCostSync(result.Differences, setting.ChannelRatioSyncTrustedAutoApply)
	return applied, conflicts, nil
}

func applyUpstreamChannelCostSync(differences map[string]map[string]dto.DifferenceItem, autoApply bool) (int, int) {
	fields, ok := differences[upstreamChannelCostField]
	if !ok || len(fields) == 0 {
		return 0, 0
	}
	applied := 0
	conflicts := 0
	for channelIDText, item := range fields {
		channelID, err := strconv.Atoi(strings.TrimSpace(channelIDText))
		if err != nil || channelID <= 0 {
			conflicts++
			recordRatioMonitorEvent(channelBalanceMonitorEventCostConflict, channelIDText, upstreamChannelCostField, false, "渠道 ID 无效", map[string]any{"channel_id": channelIDText})
			continue
		}
		value, source, ok, reason := chooseTrustedRatioValue(item)
		if !ok {
			conflicts++
			recordRatioMonitorEvent(channelBalanceMonitorEventCostConflict, channelIDText, upstreamChannelCostField, false, reason, map[string]any{
				"current":   item.Current,
				"upstreams": item.Upstreams,
			})
			continue
		}
		syncItem, ok := normalizeUpstreamChannelCostSyncValue(value)
		if !ok || syncItem.CostMultiplier <= 0 {
			conflicts++
			recordRatioMonitorEvent(channelBalanceMonitorEventCostConflict, channelIDText, upstreamChannelCostField, false, "上游通道倍率无法解析", map[string]any{"value": value})
			continue
		}
		syncItem.ChannelID = channelID
		if !autoApply {
			conflicts++
			recordRatioMonitorEvent(channelBalanceMonitorEventCostConflict, channelIDText, upstreamChannelCostField, false, "可信自动应用未开启", map[string]any{
				"value":  syncItem,
				"source": source,
			})
			continue
		}
		changed, err := applyChannelCostMultiplierSync(syncItem)
		if err != nil {
			conflicts++
			recordRatioMonitorEvent(channelBalanceMonitorEventCostFailed, channelIDText, upstreamChannelCostField, false, err.Error(), map[string]any{
				"value":  syncItem,
				"source": source,
			})
			continue
		}
		if !changed {
			recordRatioMonitorEvent(channelBalanceMonitorEventCostApplied, channelIDText, upstreamChannelCostField, true, "", map[string]any{
				"value":   syncItem.CostMultiplier,
				"source":  source,
				"changed": false,
			})
			continue
		}
		applied++
		recordRatioMonitorEvent(channelBalanceMonitorEventCostApplied, channelIDText, upstreamChannelCostField, true, "", map[string]any{
			"value":   syncItem.CostMultiplier,
			"source":  source,
			"changed": true,
		})
	}
	return applied, conflicts
}

func normalizeUpstreamChannelCostSyncValue(value any) (UpstreamChannelCostSyncItem, bool) {
	switch typed := value.(type) {
	case UpstreamChannelCostSyncItem:
		return normalizeUpstreamChannelCostSyncItem(typed)
	case map[string]any:
		return normalizeUpstreamChannelCostSyncMap(typed)
	default:
		if numeric, ok := asFloat64(value); ok {
			return normalizeUpstreamChannelCostSyncItem(UpstreamChannelCostSyncItem{CostMultiplier: numeric})
		}
		return UpstreamChannelCostSyncItem{}, false
	}
}

func normalizeUpstreamChannelCostSyncMap(value map[string]any) (UpstreamChannelCostSyncItem, bool) {
	item := UpstreamChannelCostSyncItem{}
	item.ChannelID = intFromAny(value["channel_id"])
	item.ChannelName = stringFromAny(value["channel_name"])
	item.CostMultiplier = floatFromAny(value["cost_multiplier"])
	item.CostCoefficient = floatFromAny(value["cost_coefficient"])
	item.TokenMultiplier = floatFromAny(value["token_multiplier"])
	item.RechargeMultiplier = floatFromAny(value["recharge_multiplier"])
	item.RequestPrice = floatFromAny(value["request_price"])
	item.Source = stringFromAny(value["source"])
	item.Endpoint = stringFromAny(value["endpoint"])
	item.UpstreamCostSource = stringFromAny(value["upstream_cost_source"])
	item.UpstreamCostChannel = stringFromAny(value["upstream_cost_channel"])
	item.UpstreamCostFieldPath = stringFromAny(value["upstream_cost_field_path"])
	return normalizeUpstreamChannelCostSyncItem(item)
}

func normalizeUpstreamChannelCostSyncItem(item UpstreamChannelCostSyncItem) (UpstreamChannelCostSyncItem, bool) {
	if item.CostMultiplier <= 0 {
		item.CostMultiplier = item.CostCoefficient
	}
	if item.CostMultiplier <= 0 {
		return UpstreamChannelCostSyncItem{}, false
	}
	item.CostMultiplier = roundRatioValue(item.CostMultiplier)
	if item.CostCoefficient <= 0 {
		item.CostCoefficient = item.CostMultiplier
	}
	if item.TokenMultiplier <= 0 {
		item.TokenMultiplier = 1
	}
	if item.RechargeMultiplier <= 0 {
		item.RechargeMultiplier = 1
	}
	if strings.TrimSpace(item.Source) == "" {
		item.Source = modelgatewaycost.SourceAutoSynced
	}
	return item, true
}

func applyChannelCostMultiplierSync(item UpstreamChannelCostSyncItem) (bool, error) {
	channel, err := model.GetChannelById(item.ChannelID, true)
	if err != nil {
		return false, fmt.Errorf("渠道不存在: %w", err)
	}
	existing, err := findChannelDefaultCostProfile(item.ChannelID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	profile := existing
	if profile.Id == 0 {
		profile = defaultChannelCostProfile(item.ChannelID)
	}
	oldMultiplier := buildChannelBalanceMonitorRatioSummary(profile, profile.Id > 0).CostMultiplier
	newMultiplier := roundRatioValue(item.CostMultiplier)
	if nearlyEqual(oldMultiplier, newMultiplier) && profile.Id > 0 {
		return false, nil
	}
	now := common.GetTimestamp()
	profile.ChannelID = item.ChannelID
	profile.UpstreamModel = defaultChannelCostModel
	profile.PricingMode = defaultChannelCostPricingMode
	profile.Currency = defaultChannelCostCurrency
	profile.Accuracy = defaultChannelCostAccuracy
	profile.Source = item.Source
	profile.CostCoefficient = newMultiplier
	profile.TokenMultiplier = 1
	profile.InputCostMultiplier = 1
	profile.OutputCostMultiplier = 1
	profile.CacheReadMultiplier = 1
	profile.CacheWriteMultiplier = 1
	profile.RequestCostMultiplier = 1
	profile.RechargeMultiplier = 1
	profile.RequestPrice = nonNegativeFloat(item.RequestPrice)
	profile.EffectiveTime = 0
	profile.Version = 1
	profile.SyncedAt = now
	profile.UpdatedAt = now
	profile.MetadataJSON = channelCostSyncMetadataJSON(item, channel, oldMultiplier, now)
	if profile.Id == 0 {
		profile.CreatedAt = now
		if err := model.DB.Create(&profile).Error; err != nil {
			return false, err
		}
	} else if err := model.DB.Save(&profile).Error; err != nil {
		return false, err
	}
	modelgatewaycost.StoreCachedDefaultProfile(profile)
	return true, nil
}

func channelCostSyncMetadataJSON(item UpstreamChannelCostSyncItem, channel *model.Channel, oldMultiplier float64, syncedAt int64) string {
	payload := map[string]any{
		"old_cost_multiplier": oldMultiplier,
		"new_cost_multiplier": item.CostMultiplier,
		"synced_at":           syncedAt,
	}
	if channel != nil {
		payload["channel_name"] = channel.Name
	}
	if item.UpstreamCostSource != "" {
		payload["upstream_source"] = item.UpstreamCostSource
	}
	if item.UpstreamCostChannel != "" {
		payload["upstream_channel"] = item.UpstreamCostChannel
	}
	if item.UpstreamCostFieldPath != "" {
		payload["upstream_field_path"] = item.UpstreamCostFieldPath
	}
	if item.Endpoint != "" {
		payload["endpoint"] = item.Endpoint
	}
	bytes, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func intFromAny(value any) int {
	if numeric, ok := asFloat64(value); ok {
		return int(numeric)
	}
	return 0
}

func floatFromAny(value any) float64 {
	if numeric, ok := asFloat64(value); ok {
		return numeric
	}
	if text, ok := value.(string); ok {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "x")), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func applyTrustedRatioDifferences(differences map[string]map[string]dto.DifferenceItem, autoApply bool) (int, int) {
	if len(differences) == 0 {
		return 0, 0
	}
	current := currentRatioOptionMaps()
	pending := cloneRatioOptionMaps(current)
	applied := 0
	conflicts := 0
	for modelName, fields := range differences {
		if modelName == upstreamChannelCostField {
			continue
		}
		for field, item := range fields {
			value, source, ok, reason := chooseTrustedRatioValue(item)
			if !ok {
				conflicts++
				recordRatioMonitorEvent(channelBalanceMonitorEventRatioConflict, modelName, field, false, reason, map[string]any{
					"current":   item.Current,
					"upstreams": item.Upstreams,
				})
				continue
			}
			if !autoApply {
				recordRatioMonitorEvent(channelBalanceMonitorEventRatioConflict, modelName, field, false, "可信自动应用未开启", map[string]any{
					"value":  value,
					"source": source,
				})
				continue
			}
			optionKey := optionKeyByPricingSyncField(field)
			if optionKey == "" {
				continue
			}
			if numericPricingSyncFields[field] {
				parsed, ok := asFloat64(value)
				if !ok {
					conflicts++
					recordRatioMonitorEvent(channelBalanceMonitorEventRatioConflict, modelName, field, false, "数值无法解析", map[string]any{"value": value})
					continue
				}
				pending[optionKey][modelName] = parsed
			} else {
				pending[optionKey][modelName] = value
			}
			applied++
			recordRatioMonitorEvent(channelBalanceMonitorEventRatioApplied, modelName, field, true, "", map[string]any{
				"value":  value,
				"source": source,
			})
		}
	}
	for key, values := range pending {
		if !ratioOptionMapChanged(current[key], values) {
			continue
		}
		bytes, err := common.Marshal(values)
		if err != nil {
			recordRatioMonitorEvent(channelBalanceMonitorEventRatioFailed, "", key, false, err.Error(), nil)
			continue
		}
		if err := model.UpdateOption(key, string(bytes)); err != nil {
			recordRatioMonitorEvent(channelBalanceMonitorEventRatioFailed, "", key, false, err.Error(), nil)
		}
	}
	return applied, conflicts
}

func chooseTrustedRatioValue(item dto.DifferenceItem) (any, string, bool, string) {
	var selected any
	sources := make([]string, 0)
	sameSources := make([]string, 0)
	hasSelected := false
	upstreams := make([]string, 0, len(item.Upstreams))
	for upstream := range item.Upstreams {
		upstreams = append(upstreams, upstream)
	}
	sort.Strings(upstreams)
	for _, upstream := range upstreams {
		value := item.Upstreams[upstream]
		if value == nil {
			continue
		}
		if item.Confidence != nil && !item.Confidence[upstream] {
			return nil, "", false, "存在不可信上游值"
		}
		if value == "same" {
			sameSources = append(sameSources, upstream)
			continue
		}
		if !hasSelected {
			selected = value
			sources = append(sources, upstream)
			hasSelected = true
			continue
		}
		if !valuesEqual(normalizeComparableValue(selected), normalizeComparableValue(value)) {
			return nil, "", false, "多个可信上游值不一致"
		}
		sources = append(sources, upstream)
	}
	if hasSelected && len(sameSources) > 0 {
		return nil, "", false, "可信上游值与当前配置不一致"
	}
	if !hasSelected {
		return nil, "", false, "无可应用上游值"
	}
	return selected, strings.Join(sources, ","), true, ""
}

func normalizeComparableValue(value any) any {
	if parsed, ok := asFloat64(value); ok {
		return parsed
	}
	if item, ok := normalizeUpstreamChannelCostSyncValue(value); ok && item.CostMultiplier > 0 {
		return item.CostMultiplier
	}
	return value
}

func optionKeyByPricingSyncField(field string) string {
	switch field {
	case "model_ratio":
		return "ModelRatio"
	case "completion_ratio":
		return "CompletionRatio"
	case "cache_ratio":
		return "CacheRatio"
	case "create_cache_ratio":
		return "CreateCacheRatio"
	case "image_ratio":
		return "ImageRatio"
	case "audio_ratio":
		return "AudioRatio"
	case "audio_completion_ratio":
		return "AudioCompletionRatio"
	case "model_price":
		return "ModelPrice"
	case billing_setting.BillingModeField:
		return "billing_setting.billing_mode"
	case billing_setting.BillingExprField:
		return "billing_setting.billing_expr"
	default:
		return ""
	}
}

func currentRatioOptionMaps() map[string]map[string]any {
	return map[string]map[string]any{
		"ModelRatio":                   floatMapToAny(ratio_setting.GetModelRatioCopy()),
		"CompletionRatio":              floatMapToAny(ratio_setting.GetCompletionRatioCopy()),
		"CacheRatio":                   floatMapToAny(ratio_setting.GetCacheRatioCopy()),
		"CreateCacheRatio":             floatMapToAny(ratio_setting.GetCreateCacheRatioCopy()),
		"ImageRatio":                   floatMapToAny(ratio_setting.GetImageRatioCopy()),
		"AudioRatio":                   floatMapToAny(ratio_setting.GetAudioRatioCopy()),
		"AudioCompletionRatio":         floatMapToAny(ratio_setting.GetAudioCompletionRatioCopy()),
		"ModelPrice":                   floatMapToAny(ratio_setting.GetModelPriceCopy()),
		"billing_setting.billing_mode": stringMapToAny(billing_setting.GetBillingModeCopy()),
		"billing_setting.billing_expr": stringMapToAny(billing_setting.GetBillingExprCopy()),
	}
}

func floatMapToAny(src map[string]float64) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func stringMapToAny(src map[string]string) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneRatioOptionMaps(src map[string]map[string]any) map[string]map[string]any {
	dst := make(map[string]map[string]any, len(src))
	for key, values := range src {
		dst[key] = make(map[string]any, len(values))
		for modelName, value := range values {
			dst[key][modelName] = value
		}
	}
	return dst
}

func ratioOptionMapChanged(a map[string]any, b map[string]any) bool {
	if len(a) != len(b) {
		return true
	}
	for key, valueA := range a {
		valueB, ok := b[key]
		if !ok || !valuesEqual(normalizeComparableValue(valueA), normalizeComparableValue(valueB)) {
			return true
		}
	}
	return false
}

func cleanupChannelBalanceMonitorEvents() {
	retentionDays := channelBalanceMonitorSettings().RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}
	before := common.GetTimestamp() - int64(retentionDays*86400)
	if err := model.DB.Where("created_time < ?", before).Delete(&model.ChannelBalanceMonitorEvent{}).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to cleanup channel balance monitor events: %s", err.Error()))
	}
}
