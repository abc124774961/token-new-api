package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

const ChannelStatusReasonBalanceInsufficient = "balance_insufficient"
const ChannelStatusReasonErrorPaused = "error_paused"

const channelBalanceInsufficientRuntimeTTL = 10 * time.Minute

var channelBalanceInsufficientRuntime sync.Map // channel_id -> time.Time

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func DisableChannelForBalance(channelError types.ChannelError) {
	MarkChannelBalanceInsufficient(channelError.ChannelId)
	DisableChannel(channelError, ChannelStatusReasonBalanceInsufficient)
}

func PauseChannelForError(channelError types.ChannelError, until time.Time, reason string) bool {
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过错误暂停操作", channelError.ChannelName, channelError.ChannelId))
		return false
	}
	if reason == "" {
		reason = ChannelStatusReasonErrorPaused
	}
	return model.UpdateChannelStatusWholeChannelWithInfo(
		channelError.ChannelId,
		common.ChannelStatusAutoDisabled,
		ChannelStatusReasonErrorPaused,
		map[string]interface{}{
			"pause_type":   ChannelStatusReasonErrorPaused,
			"pause_reason": reason,
			"pause_until":  until.Unix(),
		},
	)
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		ClearChannelBalanceInsufficient(channelId)
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ClearChannelBalanceInsufficientAfterSuccess(channelID int, usingKey string, channelName string) (int, bool) {
	if channelID <= 0 {
		return 0, false
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		ClearChannelBalanceInsufficient(channelID)
		return 0, false
	}

	wasBalancePaused := IsBalanceInsufficientPausedChannel(channel)
	wasConfirmedBalanceInsufficient := IsConfirmedBalanceInsufficientChannel(channel)
	rawReason := rawChannelStatusReason(channel)
	hasBalanceReason := IsBalanceInsufficientStatusReason(rawReason)
	wasRuntimeBalanceInsufficient := IsRuntimeBalanceInsufficientChannel(channel)
	if !wasBalancePaused && !wasConfirmedBalanceInsufficient && !hasBalanceReason && !wasRuntimeBalanceInsufficient {
		return channel.Status, false
	}

	ClearChannelBalanceInsufficient(channelID)
	cleared := wasRuntimeBalanceInsufficient
	if wasConfirmedBalanceInsufficient && model.ClearChannelBalanceInsufficientMarker(channelID) {
		channel.BalanceUpdatedTime = 0
		cleared = true
	}

	nextStatus := channel.Status
	if wasBalancePaused {
		EnableChannel(channelID, usingKey, channelName)
		nextStatus = common.ChannelStatusEnabled
		cleared = true
	} else if hasBalanceReason {
		if model.UpdateChannelStatusWholeChannelWithInfo(channelID, channel.Status, "", nil) {
			cleared = true
		}
	}
	return nextStatus, cleared
}

func MarkChannelBalanceInsufficient(channelID int) {
	if channelID <= 0 {
		return
	}
	channelBalanceInsufficientRuntime.Store(channelID, time.Now().Add(channelBalanceInsufficientRuntimeTTL))
}

func ClearChannelBalanceInsufficient(channelID int) {
	if channelID <= 0 {
		return
	}
	channelBalanceInsufficientRuntime.Delete(channelID)
}

func IsRuntimeBalanceInsufficientChannelID(channelID int) bool {
	if channelID <= 0 {
		return false
	}
	value, ok := channelBalanceInsufficientRuntime.Load(channelID)
	if !ok {
		return false
	}
	until, ok := value.(time.Time)
	if !ok || !until.After(time.Now()) {
		channelBalanceInsufficientRuntime.Delete(channelID)
		return false
	}
	return true
}

func IsRuntimeBalanceInsufficientChannel(channel *model.Channel) bool {
	return channel != nil && IsRuntimeBalanceInsufficientChannelID(channel.Id)
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if err.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit {
		return false
	}
	if IsUpstreamConcurrencyLimitError(err) {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if IsBalanceInsufficientError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldDisableChannelForBalance(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil || types.IsSkipRetryError(err) {
		return false
	}
	if types.IsChannelError(err) {
		return false
	}
	return IsBalanceInsufficientError(err)
}

func IsBalanceInsufficientError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
		return true
	}
	return IsBalanceInsufficientMessage(err.Error())
}

func IsBalanceInsufficientMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	for _, keyword := range []string{
		"balance_insufficient",
		"insufficient_user_quota",
		"insufficient account balance",
		"insufficient balance",
		"insufficient credit",
		"insufficient credits",
		"credit balance is too low",
		"balance is too low",
		"not enough balance",
		"quota_not_enough",
		"quota not enough",
		"余额不足",
		"余额不够",
		"用户额度不足",
		"账户余额",
	} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}

func IsBalanceInsufficientStatusReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	return normalized == ChannelStatusReasonBalanceInsufficient || strings.Contains(normalized, "余额不足")
}

func ChannelStatusReason(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	reason := rawChannelStatusReason(channel)
	if reason == "" && IsConfirmedBalanceInsufficientChannel(channel) {
		return ChannelStatusReasonBalanceInsufficient
	}
	if reason == "" && IsRuntimeBalanceInsufficientChannel(channel) {
		return ChannelStatusReasonBalanceInsufficient
	}
	return reason
}

func rawChannelStatusReason(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	info := channel.GetOtherInfo()
	reason, _ := info["status_reason"].(string)
	return strings.TrimSpace(reason)
}

func IsErrorPausedStatusReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	return normalized == ChannelStatusReasonErrorPaused || strings.Contains(normalized, "错误暂停") || strings.Contains(normalized, "故障暂停")
}

func IsPauseStatusReason(reason string) bool {
	return IsBalanceInsufficientStatusReason(reason) || IsErrorPausedStatusReason(reason)
}

func IsBalanceInsufficientPausedChannel(channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusAutoDisabled {
		return false
	}
	return IsBalanceInsufficientStatusReason(ChannelStatusReason(channel))
}

func IsConfirmedBalanceInsufficientChannel(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return channel.BalanceUpdatedTime > 0 && channel.Balance <= 0
}

func IsKnownBalanceInsufficientChannel(channel *model.Channel) bool {
	return IsBalanceInsufficientPausedChannel(channel) || IsConfirmedBalanceInsufficientChannel(channel) || IsRuntimeBalanceInsufficientChannel(channel)
}

func IsErrorPausedChannel(channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusAutoDisabled {
		return false
	}
	return IsErrorPausedStatusReason(ChannelStatusReason(channel))
}

func IsManagedPausedChannel(channel *model.Channel) bool {
	return IsBalanceInsufficientPausedChannel(channel) || IsErrorPausedChannel(channel)
}

func ShouldResumeErrorPausedChannel(channel *model.Channel, err *types.NewAPIError) bool {
	if channel == nil || !IsErrorPausedChannel(channel) || err != nil {
		return false
	}
	info := channel.GetOtherInfo()
	if until, ok := parsePauseUntil(info["pause_until"]); ok && time.Now().Unix() < until {
		return false
	}
	return true
}

func ShouldResumeBalancePausedChannel(balance float64) bool {
	if !common.ChannelBalanceAutoResumeEnabled {
		return false
	}
	return balance > common.ChannelBalanceRecoveryThreshold
}

func parsePauseUntil(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
