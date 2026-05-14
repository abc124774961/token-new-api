package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	Retry        *int
	ExtraRetries *int
	resetNextTry bool
}

type channelAvoidanceEntry struct {
	until        time.Time
	reason       string
	failureCount int
}

var channelFailureAvoidance sync.Map

type ChannelFailureAvoidanceStatus struct {
	Active       bool   `json:"active"`
	Reason       string `json:"reason,omitempty"`
	Until        int64  `json:"until,omitempty"`
	RemainingSec int64  `json:"remaining_seconds,omitempty"`
	FailureCount int    `json:"failure_count,omitempty"`
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) GetExtraRetries() int {
	if p.ExtraRetries == nil {
		return 0
	}
	return *p.ExtraRetries
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) SetExtraRetries(retry int) {
	p.ExtraRetries = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.ExtraRetries != nil && *p.ExtraRetries > 0 {
		*p.ExtraRetries--
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func (p *RetryParam) AllowExtraRetry(count int) {
	if count <= 0 {
		return
	}
	current := p.GetExtraRetries()
	p.SetExtraRetries(current + count)
}

func (p *RetryParam) HasBudget(maxRetry int) bool {
	return p.GetRetry() <= maxRetry || p.GetExtraRetries() > 0
}

func getUsedChannelSet(ctx *gin.Context) map[int]struct{} {
	usedChannelSet := make(map[int]struct{})
	if ctx == nil {
		return usedChannelSet
	}
	for _, channelIDStr := range ctx.GetStringSlice("use_channel") {
		channelID, err := strconv.Atoi(channelIDStr)
		if err != nil {
			continue
		}
		usedChannelSet[channelID] = struct{}{}
	}
	return usedChannelSet
}

func getAvoidedChannelSet() map[int]struct{} {
	avoided := make(map[int]struct{})
	if !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return avoided
	}
	now := time.Now()
	channelFailureAvoidance.Range(func(key, value any) bool {
		channelID, ok := key.(int)
		if !ok {
			channelFailureAvoidance.Delete(key)
			return true
		}
		entry, ok := value.(channelAvoidanceEntry)
		if !ok || !entry.until.After(now) {
			channelFailureAvoidance.Delete(key)
			return true
		}
		avoided[channelID] = struct{}{}
		return true
	})
	return avoided
}

func GetChannelFailureAvoidanceStatus(channelID int) *ChannelFailureAvoidanceStatus {
	if channelID <= 0 || !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return nil
	}
	value, ok := channelFailureAvoidance.Load(channelID)
	if !ok {
		return nil
	}
	entry, ok := value.(channelAvoidanceEntry)
	if !ok {
		channelFailureAvoidance.Delete(channelID)
		return nil
	}
	now := time.Now()
	if !entry.until.After(now) {
		channelFailureAvoidance.Delete(channelID)
		return nil
	}
	return &ChannelFailureAvoidanceStatus{
		Active:       true,
		Reason:       entry.reason,
		Until:        entry.until.Unix(),
		RemainingSec: int64(entry.until.Sub(now).Seconds()),
		FailureCount: entry.failureCount,
	}
}

func mergeChannelSets(sets ...map[int]struct{}) map[int]struct{} {
	merged := make(map[int]struct{})
	for _, set := range sets {
		for channelID := range set {
			merged[channelID] = struct{}{}
		}
	}
	return merged
}

func RecordChannelFailureAvoidance(channelID int, reason string) {
	if channelID <= 0 || !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return
	}
	now := time.Now()
	ttl := time.Duration(common.ChannelFailureAvoidanceTTLSeconds) * time.Second
	failureCount := 1
	if value, ok := channelFailureAvoidance.Load(channelID); ok {
		if entry, ok := value.(channelAvoidanceEntry); ok && entry.until.After(now) {
			failureCount = entry.failureCount + 1
			ttl *= time.Duration(failureCount)
			if maxTTL := 5 * time.Duration(common.ChannelFailureAvoidanceTTLSeconds) * time.Second; ttl > maxTTL {
				ttl = maxTTL
			}
		}
	}
	until := now.Add(ttl)
	channelFailureAvoidance.Store(channelID, channelAvoidanceEntry{
		until:        until,
		reason:       reason,
		failureCount: failureCount,
	})
	common.SysLog(fmt.Sprintf("channel #%d temporarily circuit-broken for %s after %s", channelID, ttl, reason))
}

func ClearChannelFailureAvoidance(channelID int) {
	if channelID <= 0 {
		return
	}
	channelFailureAvoidance.Delete(channelID)
}

func getChannelFailureAvoidanceForTest(channelID int) (channelAvoidanceEntry, bool) {
	value, ok := channelFailureAvoidance.Load(channelID)
	if !ok {
		return channelAvoidanceEntry{}, false
	}
	entry, ok := value.(channelAvoidanceEntry)
	return entry, ok
}

func clearAllChannelFailureAvoidanceForTest() {
	channelFailureAvoidance.Range(func(key, value any) bool {
		channelFailureAvoidance.Delete(key)
		return true
	})
}

func selectChannelForGroup(ctx *gin.Context, group string, modelName string, retry int, allowUsedChannelFallback bool) (*model.Channel, error) {
	excludedChannelIDs := getUsedChannelSet(ctx)
	avoidedChannelIDs := getAvoidedChannelSet()
	excludedWithAvoided := mergeChannelSets(excludedChannelIDs, avoidedChannelIDs)
	channel, err := model.GetRandomSatisfiedChannel(group, modelName, retry, excludedWithAvoided)
	if err != nil {
		return nil, err
	}
	if channel != nil {
		return channel, nil
	}
	if len(avoidedChannelIDs) > 0 && allowUsedChannelFallback {
		// Prefer a temporarily avoided channel over failing the request when no healthy peer exists.
		channel, err = model.GetRandomSatisfiedChannel(group, modelName, retry, excludedChannelIDs)
		if err != nil {
			return nil, err
		}
		if channel != nil {
			logger.LogWarn(ctx, "All available channels are temporarily avoided; falling back to an avoided channel")
			return channel, nil
		}
	}
	if !allowUsedChannelFallback || len(excludedChannelIDs) == 0 {
		return channel, nil
	}
	// All peer channels in the current priority/group have been tried. Allow reusing an
	// already-used channel so multi-key channels can continue rotating to another key.
	if len(avoidedChannelIDs) > 0 {
		channel, err = model.GetRandomSatisfiedChannel(group, modelName, retry, avoidedChannelIDs)
		if err != nil {
			return nil, err
		}
		if channel != nil {
			return channel, nil
		}
	}
	return model.GetRandomSatisfiedChannel(group, modelName, retry, nil)
}

func hasAlternativeChannelInGroup(ctx *gin.Context, group string, modelName string, retry int) bool {
	channel, err := model.GetRandomSatisfiedChannel(group, modelName, retry, mergeChannelSets(getUsedChannelSet(ctx), getAvoidedChannelSet()))
	return err == nil && channel != nil
}

func resolveAutoGroupCursor(ctx *gin.Context, autoGroups []string) (string, int) {
	currentGroup := common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup)
	if currentGroup != "" {
		for i, group := range autoGroups {
			if group == currentGroup {
				return currentGroup, i
			}
		}
	}
	if index, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := index.(int); ok && idx >= 0 && idx < len(autoGroups) {
			return autoGroups[idx], idx
		}
	}
	if len(autoGroups) == 0 {
		return "", 0
	}
	return autoGroups[0], 0
}

func GetChannelFailoverPlan(param *RetryParam) (bool, bool) {
	if param == nil || param.Ctx == nil {
		return false, false
	}
	if param.TokenGroup != "auto" {
		return hasAlternativeChannelInGroup(param.Ctx, param.TokenGroup, param.ModelName, param.GetRetry()), false
	}
	autoGroups := GetUserAutoGroup(common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup))
	if len(autoGroups) == 0 {
		return false, false
	}
	currentGroup, currentIndex := resolveAutoGroupCursor(param.Ctx, autoGroups)
	if currentGroup != "" && hasAlternativeChannelInGroup(param.Ctx, currentGroup, param.ModelName, param.GetRetry()) {
		return true, false
	}
	for i := currentIndex + 1; i < len(autoGroups); i++ {
		if hasAlternativeChannelInGroup(param.Ctx, autoGroups[i], param.ModelName, 0) {
			return true, true
		}
	}
	return false, false
}

func GetConcurrencyLimitFailoverPlan(param *RetryParam) (bool, bool) {
	return GetChannelFailoverPlan(param)
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// It prefers untried channels within the current group first, and only moves to lower priorities
// or the next auto-group when the current candidate set is exhausted.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			priorityRetry := param.GetRetry()
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)
			forceNextAutoGroup := common.GetContextKeyBool(param.Ctx, constant.ContextKeyForceNextAutoGroup)
			if forceNextAutoGroup {
				if channel, selectErr := selectChannelForGroup(param.Ctx, autoGroup, param.ModelName, priorityRetry, false); selectErr != nil {
					return nil, autoGroup, selectErr
				} else if channel != nil {
					common.SetContextKey(param.Ctx, constant.ContextKeyForceNextAutoGroup, false)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
					selectGroup = autoGroup
					logger.LogDebug(param.Ctx, "Using remaining peer channel in auto group %s before crossing groups", autoGroup)
					return channel, selectGroup, nil
				}
				common.SetContextKey(param.Ctx, constant.ContextKeyForceNextAutoGroup, false)
				logger.LogDebug(param.Ctx, "Force skipping auto group %s due to upstream concurrency limit failover", autoGroup)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				continue
			}

			channel, err = selectChannelForGroup(param.Ctx, autoGroup, param.ModelName, priorityRetry, false)
			if err != nil {
				return nil, autoGroup, err
			}
			if channel == nil {
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			if crossGroupRetry {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
		if channel == nil && len(getAvoidedChannelSet()) > 0 {
			usedChannelIDs := getUsedChannelSet(param.Ctx)
			for i := startGroupIndex; i < len(autoGroups); i++ {
				autoGroup := autoGroups[i]
				channel, err = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, param.GetRetry(), usedChannelIDs)
				if err != nil {
					return nil, autoGroup, err
				}
				if channel != nil {
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
					selectGroup = autoGroup
					logger.LogWarn(param.Ctx, "All auto-group candidates are temporarily avoided; falling back to an avoided channel")
					break
				}
			}
		}
	} else {
		channel, err = selectChannelForGroup(param.Ctx, param.TokenGroup, param.ModelName, param.GetRetry(), true)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}
