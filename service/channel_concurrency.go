package service

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

var channelConcurrency sync.Map           // channel_id -> *channelConcurrencyCounter
var channelSelectionReservation sync.Map  // channel_id -> *channelConcurrencyCounter
var channelConcurrencyLearnLocks sync.Map // channel_id -> *sync.Mutex
var channelConcurrencyControl sync.Map    // channel_id -> *channelConcurrencyControlState

const (
	channelConcurrencyCeilingSettingKey = "max_concurrency_ceiling"
	channelConcurrencyRecoveryThreshold = 3
	channelConcurrencyMinLearnSample    = 3
	channelConcurrencyDefaultCooldown   = 45 * time.Second
	channelConcurrencyMaxCooldown       = 10 * time.Minute
)

type channelConcurrencyCounter struct {
	mu     sync.Mutex
	active int
}

type ChannelConcurrencyLease struct {
	ChannelID   int
	Limit       int
	activeAtHit int
	counter     *channelConcurrencyCounter
	released    bool
}

type channelConcurrencyControlState struct {
	mu            sync.Mutex
	cooldownUntil time.Time
	reason        string
	failureCount  int
	successStreak int
}

type ChannelConcurrencyControlStatus struct {
	Active        bool   `json:"active"`
	Reason        string `json:"reason,omitempty"`
	Until         int64  `json:"until,omitempty"`
	RemainingSec  int64  `json:"remaining_seconds,omitempty"`
	FailureCount  int    `json:"failure_count,omitempty"`
	SuccessStreak int    `json:"success_streak,omitempty"`
}

type ChannelConcurrencyLearnResult struct {
	LearnedLimit  int
	CeilingLimit  int
	PreviousLimit int
	Changed       bool
}

func getChannelConcurrencyCounter(channelID int) *channelConcurrencyCounter {
	actual, _ := channelConcurrency.LoadOrStore(channelID, &channelConcurrencyCounter{})
	return actual.(*channelConcurrencyCounter)
}

func getChannelSelectionReservationCounter(channelID int) *channelConcurrencyCounter {
	actual, _ := channelSelectionReservation.LoadOrStore(channelID, &channelConcurrencyCounter{})
	return actual.(*channelConcurrencyCounter)
}

func getChannelConcurrencyLearnLock(channelID int) *sync.Mutex {
	actual, _ := channelConcurrencyLearnLocks.LoadOrStore(channelID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func getChannelConcurrencyControlState(channelID int) *channelConcurrencyControlState {
	actual, _ := channelConcurrencyControl.LoadOrStore(channelID, &channelConcurrencyControlState{})
	return actual.(*channelConcurrencyControlState)
}

func GetChannelActiveConcurrency(channelID int) int {
	if channelID <= 0 {
		return 0
	}
	value, ok := channelConcurrency.Load(channelID)
	if !ok {
		return 0
	}
	counter := value.(*channelConcurrencyCounter)
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.active
}

func GetChannelSelectionReservations(channelID int) int {
	if channelID <= 0 {
		return 0
	}
	value, ok := channelSelectionReservation.Load(channelID)
	if !ok {
		return 0
	}
	counter := value.(*channelConcurrencyCounter)
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.active
}

func GetChannelEffectiveActiveConcurrency(channelID int) int {
	active := GetChannelActiveConcurrency(channelID)
	reserved := GetChannelSelectionReservations(channelID)
	return active + reserved
}

func IsChannelConcurrencyFull(channelID int, setting dto.ChannelSettings) bool {
	limit := setting.MaxConcurrency
	if channelID <= 0 || limit <= 0 {
		return false
	}
	return GetChannelEffectiveActiveConcurrency(channelID) >= limit
}

func ReserveChannelSelection(ctx *gin.Context, channelID int, setting dto.ChannelSettings) bool {
	return ReserveChannelSelectionWithLimit(ctx, channelID, setting, 0)
}

func ReserveChannelSelectionRouting(ctx *gin.Context, channelID int) bool {
	return reserveChannelSelection(ctx, channelID, 0)
}

func ReserveChannelSelectionWithLimit(ctx *gin.Context, channelID int, setting dto.ChannelSettings, reservationLimit int) bool {
	if ctx == nil || channelID <= 0 {
		return false
	}
	limit := setting.MaxConcurrency
	effectiveLimit := limit
	if reservationLimit > 0 && (effectiveLimit <= 0 || reservationLimit < effectiveLimit) {
		effectiveLimit = reservationLimit
	}
	return reserveChannelSelection(ctx, channelID, effectiveLimit)
}

func reserveChannelSelection(ctx *gin.Context, channelID int, effectiveLimit int) bool {
	if ctx == nil || channelID <= 0 {
		return false
	}
	activeCounter := getChannelConcurrencyCounter(channelID)
	reservationCounter := getChannelSelectionReservationCounter(channelID)
	activeCounter.mu.Lock()
	reservationCounter.mu.Lock()
	if effectiveLimit > 0 && activeCounter.active+reservationCounter.active >= effectiveLimit {
		reservationCounter.mu.Unlock()
		activeCounter.mu.Unlock()
		return false
	}
	reservationCounter.active++
	reservationCounter.mu.Unlock()
	activeCounter.mu.Unlock()
	reserved, _ := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionReserved)
	reserved = append(reserved, channelID)
	common.SetContextKey(ctx, constant.ContextKeyChannelSelectionReserved, reserved)
	return true
}

func MarkChannelSelectionSkipped(ctx *gin.Context, channelID int) {
	if ctx == nil || channelID <= 0 {
		return
	}
	channelIDs, _ := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionSkipped)
	for _, existing := range channelIDs {
		if existing == channelID {
			return
		}
	}
	channelIDs = append(channelIDs, channelID)
	common.SetContextKey(ctx, constant.ContextKeyChannelSelectionSkipped, channelIDs)
}

func MarkChannelRuntimeSelectionSkipped(ctx *gin.Context, identity ChannelRuntimeIdentity) {
	if ctx == nil {
		return
	}
	identity = identity.Normalize()
	if !identity.Valid() {
		return
	}
	if !identity.HasAccountScope() {
		MarkChannelSelectionSkipped(ctx, identity.ChannelID)
		return
	}
	identity = identity.AccountScope()
	identities, _ := common.GetContextKeyType[[]ChannelRuntimeIdentity](ctx, constant.ContextKeyChannelRuntimeSelectionSkipped)
	for _, existing := range identities {
		if runtimeSelectionSkipMatches(existing, identity) {
			return
		}
	}
	identities = append(identities, identity)
	common.SetContextKey(ctx, constant.ContextKeyChannelRuntimeSelectionSkipped, identities)
}

func IsChannelSelectionSkipped(ctx *gin.Context, channelID int) bool {
	if ctx == nil || channelID <= 0 {
		return false
	}
	channelIDs, ok := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionSkipped)
	if !ok {
		return false
	}
	for _, existing := range channelIDs {
		if existing == channelID {
			return true
		}
	}
	return false
}

func IsChannelRuntimeSelectionSkipped(ctx *gin.Context, identity ChannelRuntimeIdentity) bool {
	if ctx == nil {
		return false
	}
	identity = identity.Normalize()
	if !identity.Valid() {
		return false
	}
	if IsChannelSelectionSkipped(ctx, identity.ChannelID) {
		return true
	}
	if !identity.HasAccountScope() {
		return false
	}
	identities, ok := common.GetContextKeyType[[]ChannelRuntimeIdentity](ctx, constant.ContextKeyChannelRuntimeSelectionSkipped)
	if !ok {
		return false
	}
	for _, existing := range identities {
		if runtimeSelectionSkipMatches(existing, identity) {
			return true
		}
	}
	return false
}

func runtimeSelectionSkipMatches(existing ChannelRuntimeIdentity, identity ChannelRuntimeIdentity) bool {
	existing = existing.Normalize()
	identity = identity.Normalize()
	if existing.ChannelID <= 0 || identity.ChannelID <= 0 || existing.ChannelID != identity.ChannelID {
		return false
	}
	if !existing.HasAccountScope() || !identity.HasAccountScope() {
		return false
	}
	if existing.CredentialIndexSet && identity.CredentialIndexSet && existing.CredentialIndex == identity.CredentialIndex {
		return true
	}
	if existing.AccountID != "" && identity.AccountID != "" && existing.AccountID == identity.AccountID {
		return true
	}
	if existing.CredentialSubjectFP != "" && identity.CredentialSubjectFP != "" && existing.CredentialSubjectFP == identity.CredentialSubjectFP {
		return true
	}
	if existing.CredentialFP != "" && identity.CredentialFP != "" && existing.CredentialFP == identity.CredentialFP {
		return true
	}
	return existing.AccountScope() == identity.AccountScope()
}

func ReleaseChannelSelectionReservation(ctx *gin.Context, channelID int) {
	if ctx == nil || channelID <= 0 {
		return
	}
	counterValue, ok := channelSelectionReservation.Load(channelID)
	if ok {
		counter := counterValue.(*channelConcurrencyCounter)
		counter.mu.Lock()
		if counter.active > 0 {
			counter.active--
		}
		counter.mu.Unlock()
	}
	reserved, ok := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionReserved)
	if !ok || len(reserved) == 0 {
		return
	}
	for i, id := range reserved {
		if id == channelID {
			reserved = append(reserved[:i], reserved[i+1:]...)
			common.SetContextKey(ctx, constant.ContextKeyChannelSelectionReserved, reserved)
			return
		}
	}
}

func ReleaseChannelSelectionReservations(ctx *gin.Context) {
	if ctx == nil {
		return
	}
	reserved, ok := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionReserved)
	if !ok || len(reserved) == 0 {
		return
	}
	for len(reserved) > 0 {
		channelID := reserved[len(reserved)-1]
		ReleaseChannelSelectionReservation(ctx, channelID)
		reserved, _ = common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionReserved)
	}
}

func TryAcquireChannelConcurrency(channelID int, setting dto.ChannelSettings) (*ChannelConcurrencyLease, bool) {
	limit := setting.MaxConcurrency
	if channelID <= 0 {
		return &ChannelConcurrencyLease{ChannelID: channelID, Limit: limit}, true
	}
	counter := getChannelConcurrencyCounter(channelID)
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if limit > 0 && counter.active >= limit {
		return &ChannelConcurrencyLease{
			ChannelID:   channelID,
			Limit:       limit,
			activeAtHit: counter.active,
			counter:     counter,
		}, false
	}
	counter.active++
	return &ChannelConcurrencyLease{
		ChannelID:   channelID,
		Limit:       limit,
		activeAtHit: counter.active,
		counter:     counter,
	}, true
}

func TrackChannelConcurrency(channelID int, setting dto.ChannelSettings) *ChannelConcurrencyLease {
	limit := setting.MaxConcurrency
	if channelID <= 0 {
		return &ChannelConcurrencyLease{ChannelID: channelID, Limit: limit}
	}
	counter := getChannelConcurrencyCounter(channelID)
	counter.mu.Lock()
	counter.active++
	active := counter.active
	counter.mu.Unlock()
	return &ChannelConcurrencyLease{
		ChannelID:   channelID,
		Limit:       limit,
		activeAtHit: active,
		counter:     counter,
	}
}

func (lease *ChannelConcurrencyLease) Release() {
	if lease == nil || lease.released || lease.counter == nil {
		return
	}
	lease.counter.mu.Lock()
	if lease.counter.active > 0 {
		lease.counter.active--
	}
	lease.counter.mu.Unlock()
	lease.released = true
}

func (lease *ChannelConcurrencyLease) ActiveAtHit() int {
	if lease == nil {
		return 0
	}
	return lease.activeAtHit
}

func (lease *ChannelConcurrencyLease) CurrentActive() int {
	if lease == nil {
		return 0
	}
	if lease.counter == nil {
		return lease.activeAtHit
	}
	lease.counter.mu.Lock()
	defer lease.counter.mu.Unlock()
	return lease.counter.active
}

func IsUpstreamConcurrencyLimitError(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != http.StatusTooManyRequests {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "concurrency limit exceeded for user") ||
		strings.Contains(message, "too many pending requests")
}

func resolveChannelThrottleCooldown(err *types.NewAPIError) time.Duration {
	cooldown := channelConcurrencyDefaultCooldown
	if err == nil || len(err.Metadata) == 0 {
		return cooldown
	}
	var parsed map[string]any
	if common.Unmarshal(err.Metadata, &parsed) != nil {
		return cooldown
	}
	if sec, ok := parseMetadataInt64(parsed["retry_after_seconds"]); ok && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if retryAfter, ok := parsed["retry_after"].(string); ok {
		if sec, ok := parseRetryAfterHeader(retryAfter); ok && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	return cooldown
}

func parseRetryAfterHeader(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return seconds, true
	}
	if t, err := http.ParseTime(value); err == nil {
		seconds := int64(time.Until(t).Seconds())
		if seconds < 0 {
			return 0, true
		}
		return seconds, true
	}
	return 0, false
}

func parseMetadataInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
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

func GetChannelConcurrencyCooldownStatus(channelID int) *ChannelConcurrencyControlStatus {
	if channelID <= 0 {
		return nil
	}
	value, ok := channelConcurrencyControl.Load(channelID)
	if !ok {
		return nil
	}
	state, ok := value.(*channelConcurrencyControlState)
	if !ok {
		channelConcurrencyControl.Delete(channelID)
		return nil
	}
	now := time.Now()
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.cooldownUntil.After(now) {
		return nil
	}
	return &ChannelConcurrencyControlStatus{
		Active:        true,
		Reason:        state.reason,
		Until:         state.cooldownUntil.Unix(),
		RemainingSec:  int64(state.cooldownUntil.Sub(now).Seconds()),
		FailureCount:  state.failureCount,
		SuccessStreak: state.successStreak,
	}
}

func RecordChannelConcurrencyCooldown(channelID int, err *types.NewAPIError) {
	if channelID <= 0 || err == nil {
		return
	}
	if err.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit || IsUpstreamConcurrencyLimitError(err) {
		return
	}
	if err.StatusCode != http.StatusTooManyRequests && err.GetErrorCode() != types.ErrorCodeChannelConcurrencyLimit {
		return
	}
	if err.StatusCode == http.StatusTooManyRequests && !IsUpstreamConcurrencyLimitError(err) && err.GetErrorCode() != types.ErrorCodeChannelConcurrencyLimit {
		return
	}
	cooldown := resolveChannelThrottleCooldown(err)
	if cooldown <= 0 {
		cooldown = channelConcurrencyDefaultCooldown
	}
	state := getChannelConcurrencyControlState(channelID)
	now := time.Now()
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.cooldownUntil.After(now) {
		state.failureCount++
	} else {
		state.failureCount = 1
	}
	if state.failureCount > 1 {
		cooldown += time.Duration(state.failureCount-1) * (cooldown / 2)
	}
	if cooldown > channelConcurrencyMaxCooldown {
		cooldown = channelConcurrencyMaxCooldown
	}
	state.cooldownUntil = now.Add(cooldown)
	state.reason = err.Error()
	state.successStreak = 0
	common.SysLog(fmt.Sprintf("channel #%d concurrency cooldown for %s after %s", channelID, cooldown, err.Error()))
}

func RecordChannelConcurrencySuccess(channelID int) {
	if channelID <= 0 {
		return
	}
	value, ok := channelConcurrencyControl.Load(channelID)
	if !ok {
		return
	}
	state, ok := value.(*channelConcurrencyControlState)
	if !ok {
		channelConcurrencyControl.Delete(channelID)
		return
	}
	now := time.Now()
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.cooldownUntil.After(now) {
		return
	}
	state.successStreak++
	if state.successStreak < channelConcurrencyRecoveryThreshold {
		return
	}
	state.successStreak = 0

	channel, getErr := model.GetChannelById(channelID, true)
	if getErr != nil || channel == nil {
		logChannelConcurrencyError(nil, fmt.Sprintf("failed to recover channel concurrency limit: channel_id=%d, error=%v", channelID, getErr))
		return
	}
	setting := channel.GetSetting()
	currentLimit := setting.MaxConcurrency
	if currentLimit <= 0 {
		return
	}
	ceilingLimit := getChannelConcurrencyCeilingFromSetting(channel.Setting)
	if ceilingLimit <= 0 {
		ceilingLimit = currentLimit
	}
	if ceilingLimit <= currentLimit {
		return
	}
	nextLimit := currentLimit + 1
	if currentLimit < channelConcurrencyMinLearnSample {
		nextLimit = ceilingLimit
	}
	if nextLimit > ceilingLimit {
		nextLimit = ceilingLimit
	}
	if nextLimit <= currentLimit {
		return
	}
	settingStr, marshalErr := buildLearnedChannelSetting(channel, nextLimit, ceilingLimit)
	if marshalErr != nil {
		logChannelConcurrencyError(nil, fmt.Sprintf("failed to marshal recovered channel concurrency limit: channel_id=%d, error=%v", channelID, marshalErr))
		return
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("setting", settingStr).Error; err != nil {
		logChannelConcurrencyError(nil, fmt.Sprintf("failed to save recovered channel concurrency limit: channel_id=%d, error=%v", channelID, err))
		return
	}
	channel.Setting = &settingStr
	model.CacheUpdateChannel(channel)
	common.SysLog(fmt.Sprintf("recovered channel #%d max concurrency to %d (ceiling %d)", channelID, nextLimit, ceilingLimit))
}

func getChannelConcurrencyCeilingFromSetting(settingJSON *string) int {
	if settingJSON == nil || strings.TrimSpace(*settingJSON) == "" {
		return 0
	}
	settingMap := map[string]any{}
	if err := common.Unmarshal([]byte(*settingJSON), &settingMap); err != nil {
		return 0
	}
	ceiling, _ := parseSettingInt(settingMap[channelConcurrencyCeilingSettingKey])
	return ceiling
}

func parseSettingInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func LearnChannelConcurrencyLimit(ctx *gin.Context, channelID int, activeAtLimit int, err *types.NewAPIError) {
	_ = LearnChannelConcurrencyLimitWithResult(ctx, channelID, activeAtLimit, err)
}

func LearnChannelConcurrencyLimitWithResult(ctx *gin.Context, channelID int, activeAtLimit int, err *types.NewAPIError) ChannelConcurrencyLearnResult {
	if channelID <= 0 || activeAtLimit < channelConcurrencyMinLearnSample || !IsUpstreamConcurrencyLimitError(err) {
		return ChannelConcurrencyLearnResult{}
	}
	learnedLimit := activeAtLimit - 1
	if learnedLimit < 1 {
		learnedLimit = 1
	}
	learnLock := getChannelConcurrencyLearnLock(channelID)
	learnLock.Lock()
	defer learnLock.Unlock()

	channel, getErr := model.GetChannelById(channelID, true)
	if getErr != nil || channel == nil {
		logChannelConcurrencyError(ctx, fmt.Sprintf("failed to learn channel concurrency limit: channel_id=%d, error=%v", channelID, getErr))
		return ChannelConcurrencyLearnResult{}
	}
	setting := channel.GetSetting()
	currentLimit := setting.MaxConcurrency
	currentCeiling := getChannelConcurrencyCeilingFromSetting(channel.Setting)
	if currentCeiling <= 0 {
		currentCeiling = currentLimit
	}
	if currentCeiling <= 0 {
		currentCeiling = learnedLimit
	}
	if currentCeiling < currentLimit {
		currentCeiling = currentLimit
	}
	if currentCeiling < learnedLimit {
		currentCeiling = learnedLimit
	}
	if currentLimit > 0 && currentLimit <= learnedLimit {
		if getChannelConcurrencyCeilingFromSetting(channel.Setting) <= 0 && currentCeiling > 0 {
			settingStr, marshalErr := buildLearnedChannelSetting(channel, currentLimit, currentCeiling)
			if marshalErr != nil {
				logChannelConcurrencyError(ctx, fmt.Sprintf("failed to preserve channel concurrency ceiling: channel_id=%d, error=%v", channelID, marshalErr))
				return ChannelConcurrencyLearnResult{}
			}
			if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("setting", settingStr).Error; err != nil {
				logChannelConcurrencyError(ctx, fmt.Sprintf("failed to save channel concurrency ceiling: channel_id=%d, error=%v", channelID, err))
				return ChannelConcurrencyLearnResult{}
			}
			channel.Setting = &settingStr
			model.CacheUpdateChannel(channel)
		}
		return ChannelConcurrencyLearnResult{
			LearnedLimit:  learnedLimit,
			CeilingLimit:  currentCeiling,
			PreviousLimit: currentLimit,
			Changed:       false,
		}
	}
	settingStr, marshalErr := buildLearnedChannelSetting(channel, learnedLimit, currentCeiling)
	if marshalErr != nil {
		logChannelConcurrencyError(ctx, fmt.Sprintf("failed to marshal learned channel concurrency limit: channel_id=%d, error=%v", channelID, marshalErr))
		return ChannelConcurrencyLearnResult{}
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("setting", settingStr).Error; err != nil {
		logChannelConcurrencyError(ctx, fmt.Sprintf("failed to save learned channel concurrency limit: channel_id=%d, error=%v", channelID, err))
		return ChannelConcurrencyLearnResult{}
	}
	channel.Setting = &settingStr
	model.CacheUpdateChannel(channel)
	if currentLimit > 0 {
		logChannelConcurrencyInfo(ctx, fmt.Sprintf("learned channel #%d max concurrency: %d (was %d, ceiling %d)", channelID, learnedLimit, currentLimit, currentCeiling))
	} else {
		logChannelConcurrencyInfo(ctx, fmt.Sprintf("learned channel #%d max concurrency: %d (ceiling %d)", channelID, learnedLimit, currentCeiling))
	}
	return ChannelConcurrencyLearnResult{
		LearnedLimit:  learnedLimit,
		CeilingLimit:  currentCeiling,
		PreviousLimit: currentLimit,
		Changed:       true,
	}
}

func buildLearnedChannelSetting(channel *model.Channel, learnedLimit int, ceilingLimit int) (string, error) {
	settingMap := map[string]any{}
	if channel.Setting != nil && strings.TrimSpace(*channel.Setting) != "" {
		if err := common.Unmarshal([]byte(*channel.Setting), &settingMap); err != nil {
			return "", err
		}
	}
	settingMap["max_concurrency"] = learnedLimit
	if ceilingLimit > 0 {
		settingMap[channelConcurrencyCeilingSettingKey] = ceilingLimit
	}
	settingBytes, err := common.Marshal(settingMap)
	if err != nil {
		return "", err
	}
	return string(settingBytes), nil
}

func logChannelConcurrencyError(ctx *gin.Context, message string) {
	if ctx == nil {
		common.SysError(message)
		return
	}
	logger.LogError(ctx, message)
}

func logChannelConcurrencyInfo(ctx *gin.Context, message string) {
	if ctx == nil {
		common.SysLog(message)
		return
	}
	logger.LogInfo(ctx, message)
}

func ClearChannelConcurrencyForTest() {
	channelConcurrency.Range(func(key, value any) bool {
		channelConcurrency.Delete(key)
		return true
	})
	channelSelectionReservation.Range(func(key, value any) bool {
		channelSelectionReservation.Delete(key)
		return true
	})
	channelConcurrencyLearnLocks.Range(func(key, value any) bool {
		channelConcurrencyLearnLocks.Delete(key)
		return true
	})
	channelConcurrencyControl.Range(func(key, value any) bool {
		channelConcurrencyControl.Delete(key)
		return true
	})
	channelBalanceInsufficientRuntime.Range(func(key, value any) bool {
		channelBalanceInsufficientRuntime.Delete(key)
		return true
	})
	clearChannelFirstByteWaitersForTest()
}

func getChannelConcurrencyCooldownSet() map[int]struct{} {
	cooling := make(map[int]struct{})
	now := time.Now()
	channelConcurrencyControl.Range(func(key, value any) bool {
		channelID, ok := key.(int)
		if !ok {
			channelConcurrencyControl.Delete(key)
			return true
		}
		state, ok := value.(*channelConcurrencyControlState)
		if !ok {
			channelConcurrencyControl.Delete(key)
			return true
		}
		state.mu.Lock()
		active := state.cooldownUntil.After(now)
		state.mu.Unlock()
		if active {
			cooling[channelID] = struct{}{}
		}
		if !active {
			channelConcurrencyControl.Delete(channelID)
		}
		return true
	})
	return cooling
}
