package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
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
var accountConcurrency sync.Map           // account concurrency scope -> *channelConcurrencyCounter
var channelSelectionReservation sync.Map  // channel_id -> *channelConcurrencyCounter
var channelConcurrencyLearnLocks sync.Map // channel_id -> *sync.Mutex
var channelConcurrencyControl sync.Map    // channel_id -> *channelConcurrencyControlState

const (
	channelConcurrencyCeilingSettingKey = "max_concurrency_ceiling"
	channelConcurrencyRecoveryThreshold = 3
	channelConcurrencyMinLearnSample    = 3
	channelConcurrencyDefaultCooldown   = 45 * time.Second
	channelConcurrencyMaxCooldown       = 10 * time.Minute
	accountConcurrencyRedisTTL          = 6 * time.Hour
)

const accountConcurrencyRedisAcquireScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local current = tonumber(redis.call('GET', key) or '0')
if current >= limit then
  return {0, current}
end
current = redis.call('INCR', key)
redis.call('PEXPIRE', key, ttl)
if current > limit then
  current = redis.call('DECR', key)
  if current <= 0 then
    redis.call('DEL', key)
    current = 0
  end
  return {0, current}
end
return {1, current}
`

const accountConcurrencyRedisReleaseScript = `
local key = KEYS[1]
local current = tonumber(redis.call('GET', key) or '0')
if current <= 1 then
  redis.call('DEL', key)
  return 0
end
current = redis.call('DECR', key)
return current
`

type channelConcurrencyCounter struct {
	mu     sync.Mutex
	active int
}

type ChannelConcurrencyLease struct {
	ChannelID   int
	Limit       int
	ScopeKey    string
	activeAtHit int
	counter     *channelConcurrencyCounter
	accountHeld *channelConcurrencyCounter
	channelHeld *channelConcurrencyCounter
	redisKey    string
	redisHeld   bool
	releaseOnce sync.Once
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

func getAccountConcurrencyCounter(scopeKey string) *channelConcurrencyCounter {
	actual, _ := accountConcurrency.LoadOrStore(scopeKey, &channelConcurrencyCounter{})
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

func ChannelRuntimeConcurrencyScopeKey(identity ChannelRuntimeIdentity) string {
	identity = identity.AccountScope().Normalize()
	if identity.ChannelID <= 0 || !identity.HasAccountScope() {
		return ""
	}
	parts := []string{fmt.Sprintf("channel:%d", identity.ChannelID)}
	if identity.AccountID != "" {
		parts = append(parts, "account:"+identity.AccountID)
	} else if identity.CredentialSubjectFP != "" {
		parts = append(parts, "subject:"+identity.CredentialSubjectFP)
	} else if identity.CredentialFP != "" {
		parts = append(parts, "credential:"+identity.CredentialFP)
	} else if identity.CredentialIndexSet {
		parts = append(parts, fmt.Sprintf("index:%d", identity.CredentialIndex))
	}
	return strings.Join(parts, "|")
}

func accountConcurrencyRedisKey(scopeKey string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(scopeKey)))
	return "channel_concurrency:account:" + hex.EncodeToString(sum[:])
}

func tryAcquireRedisAccountConcurrency(scopeKey string, limit int) (bool, int, string, error) {
	if !common.RedisEnabled || common.RDB == nil || strings.TrimSpace(scopeKey) == "" || limit <= 0 {
		return false, 0, "", fmt.Errorf("redis account concurrency disabled")
	}
	redisKey := accountConcurrencyRedisKey(scopeKey)
	result, err := common.RDB.Eval(
		context.Background(),
		accountConcurrencyRedisAcquireScript,
		[]string{redisKey},
		limit,
		int64(accountConcurrencyRedisTTL/time.Millisecond),
	).Result()
	if err != nil {
		return false, 0, redisKey, err
	}
	values, ok := result.([]interface{})
	if !ok || len(values) < 2 {
		return false, 0, redisKey, fmt.Errorf("redis account concurrency result invalid")
	}
	allowed, ok := redisScriptInt(values[0])
	if !ok {
		return false, 0, redisKey, fmt.Errorf("redis account concurrency status invalid")
	}
	active, ok := redisScriptInt(values[1])
	if !ok {
		return false, 0, redisKey, fmt.Errorf("redis account concurrency active invalid")
	}
	if active < 0 {
		active = 0
	}
	return allowed == 1, int(active), redisKey, nil
}

func releaseRedisAccountConcurrency(redisKey string) {
	if !common.RedisEnabled || common.RDB == nil || strings.TrimSpace(redisKey) == "" {
		return
	}
	_, _ = common.RDB.Eval(
		context.Background(),
		accountConcurrencyRedisReleaseScript,
		[]string{strings.TrimSpace(redisKey)},
	).Result()
}

func getRedisAccountConcurrency(scopeKey string) (int, bool) {
	if !common.RedisEnabled || common.RDB == nil || strings.TrimSpace(scopeKey) == "" {
		return 0, false
	}
	active, err := common.RDB.Get(context.Background(), accountConcurrencyRedisKey(scopeKey)).Int()
	if err != nil {
		return 0, false
	}
	if active < 0 {
		active = 0
	}
	return active, true
}

func redisScriptInt(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func GetChannelRuntimeActiveConcurrency(identity ChannelRuntimeIdentity) int {
	scopeKey := ChannelRuntimeConcurrencyScopeKey(identity)
	if scopeKey == "" {
		return GetChannelActiveConcurrency(identity.ChannelID)
	}
	if active, ok := getRedisAccountConcurrency(scopeKey); ok {
		return active
	}
	value, ok := accountConcurrency.Load(scopeKey)
	if !ok {
		return 0
	}
	counter := value.(*channelConcurrencyCounter)
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.active
}

func GetChannelRuntimeEffectiveActiveConcurrency(identity ChannelRuntimeIdentity) int {
	scopeKey := ChannelRuntimeConcurrencyScopeKey(identity)
	if scopeKey == "" {
		return GetChannelEffectiveActiveConcurrency(identity.ChannelID)
	}
	return GetChannelRuntimeActiveConcurrency(identity)
}

func IsChannelConcurrencyFull(channelID int, setting dto.ChannelSettings) bool {
	if setting.AccountMaxConcurrency > 0 && strings.TrimSpace(setting.AccountConcurrencyKey) != "" {
		accountScopeKey := strings.TrimSpace(setting.AccountConcurrencyKey)
		if active, ok := getRedisAccountConcurrency(accountScopeKey); ok && active >= setting.AccountMaxConcurrency {
			return true
		}
		value, ok := accountConcurrency.Load(accountScopeKey)
		if ok {
			counter := value.(*channelConcurrencyCounter)
			counter.mu.Lock()
			full := counter.active >= setting.AccountMaxConcurrency
			counter.mu.Unlock()
			if full {
				return true
			}
		}
	}
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

func MarkChannelRuntimeAttempted(ctx *gin.Context, identity ChannelRuntimeIdentity) {
	if ctx == nil {
		return
	}
	identity = identity.Normalize()
	if !identity.Valid() {
		return
	}
	if identity.HasAccountScope() {
		identity = identity.AccountScope()
	} else {
		identity = identity.ChannelScope()
	}
	identities, _ := common.GetContextKeyType[[]ChannelRuntimeIdentity](ctx, constant.ContextKeyChannelRuntimeAttempted)
	for _, existing := range identities {
		if runtimeAttemptMatches(existing, identity) {
			return
		}
	}
	identities = append(identities, identity)
	common.SetContextKey(ctx, constant.ContextKeyChannelRuntimeAttempted, identities)
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

func IsChannelRuntimeAttempted(ctx *gin.Context, identity ChannelRuntimeIdentity) bool {
	if ctx == nil {
		return false
	}
	identity = identity.Normalize()
	if !identity.Valid() {
		return false
	}
	identities, ok := common.GetContextKeyType[[]ChannelRuntimeIdentity](ctx, constant.ContextKeyChannelRuntimeAttempted)
	if !ok {
		return false
	}
	for _, existing := range identities {
		if runtimeAttemptMatches(existing, identity) {
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

func runtimeAttemptMatches(existing ChannelRuntimeIdentity, identity ChannelRuntimeIdentity) bool {
	existing = existing.Normalize()
	identity = identity.Normalize()
	if existing.ChannelID <= 0 || identity.ChannelID <= 0 || existing.ChannelID != identity.ChannelID {
		return false
	}
	if !existing.HasAccountScope() || !identity.HasAccountScope() {
		return true
	}
	return runtimeSelectionSkipMatches(existing, identity)
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
	channelLimit := setting.MaxConcurrency
	accountLimit := setting.AccountMaxConcurrency
	accountScopeKey := strings.TrimSpace(setting.AccountConcurrencyKey)
	if channelID <= 0 {
		return &ChannelConcurrencyLease{ChannelID: channelID, Limit: channelLimit}, true
	}
	channelCounter := getChannelConcurrencyCounter(channelID)
	channelCounter.mu.Lock()
	if channelLimit > 0 && channelCounter.active >= channelLimit {
		active := channelCounter.active
		channelCounter.mu.Unlock()
		return &ChannelConcurrencyLease{
			ChannelID:   channelID,
			Limit:       channelLimit,
			activeAtHit: active,
			counter:     channelCounter,
		}, false
	}

	var accountCounter *channelConcurrencyCounter
	if accountLimit > 0 && accountScopeKey != "" {
		if acquired, active, redisKey, err := tryAcquireRedisAccountConcurrency(accountScopeKey, accountLimit); err == nil {
			if !acquired {
				channelCounter.mu.Unlock()
				return &ChannelConcurrencyLease{
					ChannelID:   channelID,
					Limit:       accountLimit,
					ScopeKey:    accountScopeKey,
					activeAtHit: active,
					redisKey:    redisKey,
				}, false
			}
			channelCounter.active++
			channelCounter.mu.Unlock()
			return &ChannelConcurrencyLease{
				ChannelID:   channelID,
				Limit:       accountLimit,
				ScopeKey:    accountScopeKey,
				activeAtHit: active,
				channelHeld: channelCounter,
				redisKey:    redisKey,
				redisHeld:   true,
			}, true
		}
		accountCounter = getAccountConcurrencyCounter(accountScopeKey)
		accountCounter.mu.Lock()
		if accountCounter.active >= accountLimit {
			active := accountCounter.active
			accountCounter.mu.Unlock()
			channelCounter.mu.Unlock()
			return &ChannelConcurrencyLease{
				ChannelID:   channelID,
				Limit:       accountLimit,
				ScopeKey:    accountScopeKey,
				activeAtHit: active,
				counter:     accountCounter,
			}, false
		}
		accountCounter.active++
		accountActive := accountCounter.active
		channelCounter.active++
		accountCounter.mu.Unlock()
		channelCounter.mu.Unlock()
		return &ChannelConcurrencyLease{
			ChannelID:   channelID,
			Limit:       accountLimit,
			ScopeKey:    accountScopeKey,
			activeAtHit: accountActive,
			counter:     accountCounter,
			accountHeld: accountCounter,
			channelHeld: channelCounter,
		}, true
	}

	channelCounter.active++
	active := channelCounter.active
	channelCounter.mu.Unlock()
	return &ChannelConcurrencyLease{
		ChannelID:   channelID,
		Limit:       channelLimit,
		activeAtHit: active,
		counter:     channelCounter,
		channelHeld: channelCounter,
	}, true
}

func TrackChannelConcurrency(channelID int, setting dto.ChannelSettings) *ChannelConcurrencyLease {
	limit := setting.MaxConcurrency
	if channelID <= 0 {
		return &ChannelConcurrencyLease{ChannelID: channelID, Limit: limit}
	}
	accountLimit := setting.AccountMaxConcurrency
	accountScopeKey := strings.TrimSpace(setting.AccountConcurrencyKey)
	counter := getChannelConcurrencyCounter(channelID)
	counter.mu.Lock()
	if accountLimit > 0 && accountScopeKey != "" {
		accountCounter := getAccountConcurrencyCounter(accountScopeKey)
		accountCounter.mu.Lock()
		accountCounter.active++
		active := accountCounter.active
		counter.active++
		accountCounter.mu.Unlock()
		counter.mu.Unlock()
		return &ChannelConcurrencyLease{
			ChannelID:   channelID,
			Limit:       accountLimit,
			ScopeKey:    accountScopeKey,
			activeAtHit: active,
			counter:     accountCounter,
			accountHeld: accountCounter,
			channelHeld: counter,
		}
	}
	counter.active++
	active := counter.active
	counter.mu.Unlock()
	return &ChannelConcurrencyLease{
		ChannelID:   channelID,
		Limit:       limit,
		activeAtHit: active,
		counter:     counter,
		channelHeld: counter,
	}
}

func (lease *ChannelConcurrencyLease) Release() {
	if lease == nil {
		return
	}
	lease.releaseOnce.Do(func() {
		for _, counter := range []*channelConcurrencyCounter{lease.channelHeld, lease.accountHeld} {
			if counter == nil {
				continue
			}
			counter.mu.Lock()
			if counter.active > 0 {
				counter.active--
			}
			counter.mu.Unlock()
		}
		if lease.accountHeld == nil && lease.channelHeld == nil && lease.counter != nil {
			lease.counter.mu.Lock()
			if lease.counter.active > 0 {
				lease.counter.active--
			}
			lease.counter.mu.Unlock()
		}
		if lease.redisHeld && lease.redisKey != "" {
			releaseRedisAccountConcurrency(lease.redisKey)
		}
	})
}

func BindChannelConcurrencyLease(ctx *gin.Context, lease *ChannelConcurrencyLease) {
	if ctx == nil || lease == nil {
		return
	}
	common.SetContextKey(ctx, constant.ContextKeyChannelConcurrencyLease, lease)
}

func ReleaseChannelConcurrencyLease(ctx *gin.Context) {
	if ctx == nil {
		return
	}
	lease, ok := common.GetContextKeyType[*ChannelConcurrencyLease](ctx, constant.ContextKeyChannelConcurrencyLease)
	if !ok || lease == nil {
		return
	}
	lease.Release()
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
	if lease.redisHeld && lease.ScopeKey != "" {
		if active, ok := getRedisAccountConcurrency(lease.ScopeKey); ok {
			return active
		}
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

func ClearChannelConcurrencyCooldown(channelID int) bool {
	if channelID <= 0 {
		return false
	}
	if _, ok := channelConcurrencyControl.Load(channelID); !ok {
		return false
	}
	channelConcurrencyControl.Delete(channelID)
	return true
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
	accountConcurrency.Range(func(key, value any) bool {
		accountConcurrency.Delete(key)
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
