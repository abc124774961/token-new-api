package scheduler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const AccountRateLimitRejectReason = "account_rate_limited"

const accountRateLimitRedisScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local max_requests = tonumber(ARGV[3])
local reserve = tonumber(ARGV[4])
redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)
local count = redis.call('ZCARD', key)
if count >= max_requests then
  return {0, count}
end
if reserve == 1 then
  local seq_key = key .. ':seq'
  local seq = redis.call('INCR', seq_key)
  redis.call('ZADD', key, now, tostring(now) .. '-' .. tostring(seq))
  redis.call('PEXPIRE', key, window + 5000)
  redis.call('PEXPIRE', seq_key, window + 5000)
  count = count + 1
end
return {1, count}
`

var defaultAccountRateLimiter = newAccountRateLimiter()

type accountRateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]int64
}

func newAccountRateLimiter() *accountRateLimiter {
	return &accountRateLimiter{buckets: make(map[string][]int64)}
}

func ResetAccountRateLimiterForTest() {
	defaultAccountRateLimiter.reset()
}

func candidateAccountRateLimitRejected(candidate core.Candidate) bool {
	return !defaultAccountRateLimiter.allow(candidate, false)
}

func reserveCandidateAccountRateLimit(candidate core.Candidate) bool {
	return defaultAccountRateLimiter.allow(candidate, true)
}

func (l *accountRateLimiter) reset() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string][]int64)
}

func (l *accountRateLimiter) allow(candidate core.Candidate, reserve bool) bool {
	limit, ok := candidateAccountRateLimit(candidate)
	if !ok {
		return true
	}
	key := candidateAccountRateLimitKey(candidate)
	if key == "" {
		return true
	}
	now := time.Now()
	if common.RedisEnabled && common.RDB != nil {
		if allowed, err := redisAccountRateLimitAllow(key, limit, now, reserve); err == nil {
			return allowed
		}
	}
	return l.memoryAllow(key, limit, now, reserve)
}

func candidateAccountRateLimit(candidate core.Candidate) (model.ChannelAccountRateLimit, bool) {
	if candidate.Channel == nil {
		return model.ChannelAccountRateLimit{}, false
	}
	index := candidate.CredentialRef.CredentialIndex
	if index < 0 {
		index = candidate.RuntimeKey.CredentialIndex
	}
	if index < 0 {
		return model.ChannelAccountRateLimit{}, false
	}
	return candidate.Channel.ChannelInfo.AccountRateLimit(index)
}

func candidateAccountRateLimitKey(candidate core.Candidate) string {
	channelID := candidate.RuntimeKey.ChannelID
	if channelID <= 0 && candidate.Channel != nil {
		channelID = candidate.Channel.Id
	}
	if channelID <= 0 {
		return ""
	}
	index := candidate.CredentialRef.CredentialIndex
	if index < 0 {
		index = candidate.RuntimeKey.CredentialIndex
	}
	if index < 0 {
		return ""
	}
	parts := []string{
		strconv.Itoa(channelID),
		strconv.Itoa(index),
		strings.TrimSpace(candidate.AccountIdentity.AccountID),
		strings.TrimSpace(candidate.AccountIdentity.CredentialSubjectFingerprint),
		strings.TrimSpace(candidate.AccountIdentity.CredentialFingerprint),
		strings.TrimSpace(candidate.CredentialRef.CredentialFingerprint),
	}
	return strings.Join(parts, "|")
}

func redisAccountRateLimitAllow(key string, limit model.ChannelAccountRateLimit, now time.Time, reserve bool) (bool, error) {
	redisKey := accountRateLimitRedisKey(key)
	reserveFlag := 0
	if reserve {
		reserveFlag = 1
	}
	result, err := common.RDB.Eval(
		context.Background(),
		accountRateLimitRedisScript,
		[]string{redisKey},
		now.UnixMilli(),
		int64(limit.WindowSeconds)*int64(time.Second/time.Millisecond),
		limit.MaxRequests,
		reserveFlag,
	).Result()
	if err != nil {
		return false, err
	}
	values, ok := result.([]interface{})
	if !ok || len(values) == 0 {
		return false, nil
	}
	allowed, ok := redisScriptInt(values[0])
	if !ok {
		return false, nil
	}
	return allowed == 1, nil
}

func accountRateLimitRedisKey(key string) string {
	sum := sha1.Sum([]byte(key))
	return "modelgateway:account_rate_limit:" + hex.EncodeToString(sum[:])
}

func redisScriptInt(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (l *accountRateLimiter) memoryAllow(key string, limit model.ChannelAccountRateLimit, now time.Time, reserve bool) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-time.Duration(limit.WindowSeconds) * time.Second).UnixNano()
	events := l.buckets[key]
	kept := events[:0]
	for _, eventAt := range events {
		if eventAt > cutoff {
			kept = append(kept, eventAt)
		}
	}
	if len(kept) >= limit.MaxRequests {
		l.buckets[key] = kept
		return false
	}
	if reserve {
		kept = append(kept, now.UnixNano())
	}
	if len(kept) == 0 {
		delete(l.buckets, key)
	} else {
		l.buckets[key] = kept
	}
	return true
}
