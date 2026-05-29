package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/codexauth"
)

const codexAccountUsageLimitWriteThrottleSec = 60

func MarkCodexAccountUsageLimited(channelID int, credentialIndex int, message string) (bool, error) {
	return MarkCodexAccountUsageLimitedWithCooldown(channelID, credentialIndex, message, 0, "")
}

func MarkCodexAccountUsageLimitedWithCooldown(channelID int, credentialIndex int, message string, cooldownSec int64, resetSource string) (bool, error) {
	if channelID <= 0 || credentialIndex < 0 || !IsCodexAccountUsageLimitMessage(message) {
		return false, nil
	}
	return updateCodexAccountUsageLimit(channelID, credentialIndex, func(capability model.ChannelAccountCapability, now int64) (model.ChannelAccountCapability, bool) {
		if capability.UsageLimitActiveAt(now) && now-capability.UsageLimitDetectedTime < codexAccountUsageLimitWriteThrottleSec {
			return capability, false
		}
		return applyCodexAccountUsageLimitWithCooldown(capability, message, now, cooldownSec, resetSource), true
	})
}

func CodexAccountUsageLimitCooldownFromMetadata(metadata json.RawMessage, now int64) (int64, string) {
	if len(metadata) == 0 {
		return 0, ""
	}
	values := map[string]any{}
	if err := common.Unmarshal(metadata, &values); err != nil {
		return 0, ""
	}
	for _, key := range []string{"retry_after_seconds", "retry_after"} {
		if seconds, ok := parseCodexUsageLimitResetSeconds(values[key], now); ok {
			return seconds, key
		}
	}
	bestSeconds := int64(0)
	bestSource := ""
	for _, key := range []string{"rate_limit_reset", "rate_limit_reset_requests", "rate_limit_reset_tokens"} {
		seconds, ok := parseCodexUsageLimitResetSeconds(values[key], now)
		if !ok {
			continue
		}
		if bestSeconds <= 0 || seconds < bestSeconds {
			bestSeconds = seconds
			bestSource = key
		}
	}
	return bestSeconds, bestSource
}

func ClearCodexAccountUsageLimit(channelID int, credentialIndex int) (bool, error) {
	if channelID <= 0 || credentialIndex < 0 {
		return false, nil
	}
	return updateCodexAccountUsageLimit(channelID, credentialIndex, func(capability model.ChannelAccountCapability, now int64) (model.ChannelAccountCapability, bool) {
		if !capability.UsageLimitActiveAt(now) && strings.TrimSpace(capability.UsageLimitStatus) == "" {
			return capability, false
		}
		capability = capability.ClearUsageLimit()
		capability.CapabilityClassification = capability.EffectiveClassification()
		return capability, true
	})
}

func updateCodexAccountUsageLimit(channelID int, credentialIndex int, mutate func(model.ChannelAccountCapability, int64) (model.ChannelAccountCapability, bool)) (bool, error) {
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return false, err
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return false, errors.New("账号索引超出范围")
	}
	if !codexAccountUsageLimitApplies(channel, strings.TrimSpace(keys[credentialIndex])) {
		return false, nil
	}
	if channel.ChannelInfo.MultiKeyCapabilities == nil {
		channel.ChannelInfo.MultiKeyCapabilities = make(map[int]model.ChannelAccountCapability)
	}
	capability := channel.ChannelInfo.MultiKeyCapabilities[credentialIndex]
	next, changed := mutate(capability, common.GetTimestamp())
	if !changed {
		return false, nil
	}
	if next.CheckedTime <= 0 {
		next.CheckedTime = common.GetTimestamp()
	}
	channel.ChannelInfo.MultiKeyCapabilities[credentialIndex] = next
	if err := channel.SaveChannelInfo(); err != nil {
		return false, err
	}
	model.InitChannelCache()
	return true, nil
}

func codexAccountUsageLimitApplies(channel *model.Channel, rawKey string) bool {
	if channel == nil {
		return false
	}
	if channel.Type == constant.ChannelTypeCodex {
		return true
	}
	return channel.Type == constant.ChannelTypeOpenAI && codexauth.IsOAuthJSONCredential(rawKey)
}

func ChannelAccountCapabilityUsageLimited(capability model.ChannelAccountCapability) bool {
	return capability.UsageLimitActiveAt(common.GetTimestamp())
}

func ChannelAccountCapabilityAllowsScheduling(capability model.ChannelAccountCapability) bool {
	return !ChannelAccountCapabilityUsageLimited(capability)
}

func AccountUsageLimitedRejectReason(capability model.ChannelAccountCapability) string {
	if !ChannelAccountCapabilityUsageLimited(capability) {
		return ""
	}
	if reason := strings.TrimSpace(capability.UsageLimitReason); reason != "" {
		return reason
	}
	return channelcapability.ClassificationAccountUsageLimited
}

func parseCodexUsageLimitResetSeconds(value any, now int64) (int64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		return normalizeCodexUsageLimitNumericReset(typed, now)
	case float32:
		return normalizeCodexUsageLimitNumericReset(float64(typed), now)
	case int:
		return normalizeCodexUsageLimitNumericReset(float64(typed), now)
	case int64:
		return normalizeCodexUsageLimitNumericReset(float64(typed), now)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return normalizeCodexUsageLimitNumericReset(parsed, now)
	case string:
		return parseCodexUsageLimitResetString(typed, now)
	default:
		return parseCodexUsageLimitResetString(fmt.Sprint(typed), now)
	}
}

func parseCodexUsageLimitResetString(value string, now int64) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return normalizeCodexUsageLimitNumericReset(parsed, now)
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return normalizeCodexUsageLimitDurationSeconds(duration)
	}
	if resetAt, err := http.ParseTime(value); err == nil {
		return normalizeCodexUsageLimitDurationSeconds(time.Until(resetAt))
	}
	return 0, false
}

func normalizeCodexUsageLimitNumericReset(value float64, now int64) (int64, bool) {
	if !isFinitePositive(value) {
		return 0, false
	}
	if value > 1_000_000_000_000 {
		return normalizeCodexUsageLimitTimestampSeconds(int64(value/1000), now)
	}
	if value > float64(now+60) {
		return normalizeCodexUsageLimitTimestampSeconds(int64(value), now)
	}
	return clampCodexUsageLimitCooldownSeconds(int64(math.Ceil(value))), true
}

func normalizeCodexUsageLimitTimestampSeconds(timestamp int64, now int64) (int64, bool) {
	if timestamp <= now {
		return 0, false
	}
	return clampCodexUsageLimitCooldownSeconds(timestamp - now), true
}

func normalizeCodexUsageLimitDurationSeconds(duration time.Duration) (int64, bool) {
	if duration <= 0 {
		return 0, false
	}
	return clampCodexUsageLimitCooldownSeconds(int64(math.Ceil(duration.Seconds()))), true
}

func clampCodexUsageLimitCooldownSeconds(seconds int64) int64 {
	if seconds < 1 {
		return 1
	}
	maxSeconds := int64((24 * time.Hour) / time.Second)
	if seconds > maxSeconds {
		return maxSeconds
	}
	return seconds
}

func isFinitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
