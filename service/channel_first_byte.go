package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const channelFirstByteSlowThreshold = 8 * time.Second

var channelFirstByteWaiters sync.Map // channel_id -> *channelFirstByteWaitCounter

type channelFirstByteWaitCounter struct {
	mu      sync.Mutex
	waiters map[string]time.Time
}

type ChannelFirstByteWaitLease struct {
	ChannelID int
	key       string
	counter   *channelFirstByteWaitCounter
	once      sync.Once
}

type ChannelFirstBytePendingStatus struct {
	Pending       int   `json:"pending"`
	SlowPending   int   `json:"slow_pending"`
	OldestMs      int64 `json:"oldest_ms"`
	SlowestMs     int64 `json:"slowest_ms"`
	SlowThreshold int64 `json:"slow_threshold_ms"`
}

func getChannelFirstByteWaitCounter(channelID int) *channelFirstByteWaitCounter {
	actual, _ := channelFirstByteWaiters.LoadOrStore(channelID, &channelFirstByteWaitCounter{
		waiters: map[string]time.Time{},
	})
	return actual.(*channelFirstByteWaitCounter)
}

func BeginChannelFirstByteWait(ctx *gin.Context, channelID int, requestID string, attemptIndex int) *ChannelFirstByteWaitLease {
	if channelID <= 0 {
		return nil
	}
	counter := getChannelFirstByteWaitCounter(channelID)
	lease := &ChannelFirstByteWaitLease{
		ChannelID: channelID,
		key:       fmt.Sprintf("%s:%d:%d", requestID, attemptIndex, time.Now().UnixNano()),
		counter:   counter,
	}
	counter.mu.Lock()
	counter.waiters[lease.key] = time.Now()
	counter.mu.Unlock()
	if ctx != nil {
		common.SetContextKey(ctx, constant.ContextKeyChannelFirstByteWaitLease, lease)
	}
	return lease
}

func MarkChannelFirstByteObserved(ctx *gin.Context) {
	if ctx == nil {
		return
	}
	lease, ok := common.GetContextKeyType[*ChannelFirstByteWaitLease](ctx, constant.ContextKeyChannelFirstByteWaitLease)
	if !ok || lease == nil {
		return
	}
	lease.Release()
}

func (lease *ChannelFirstByteWaitLease) Release() {
	if lease == nil || lease.counter == nil || lease.key == "" {
		return
	}
	lease.once.Do(func() {
		lease.counter.mu.Lock()
		delete(lease.counter.waiters, lease.key)
		empty := len(lease.counter.waiters) == 0
		lease.counter.mu.Unlock()
		if empty {
			channelFirstByteWaiters.Delete(lease.ChannelID)
		}
	})
}

func GetChannelFirstBytePendingStatus(channelID int) *ChannelFirstBytePendingStatus {
	if channelID <= 0 {
		return nil
	}
	value, ok := channelFirstByteWaiters.Load(channelID)
	if !ok {
		return nil
	}
	counter, ok := value.(*channelFirstByteWaitCounter)
	if !ok {
		channelFirstByteWaiters.Delete(channelID)
		return nil
	}
	now := time.Now()
	status := &ChannelFirstBytePendingStatus{
		SlowThreshold: int64(channelFirstByteSlowThreshold / time.Millisecond),
	}
	counter.mu.Lock()
	for key, startedAt := range counter.waiters {
		if startedAt.IsZero() {
			delete(counter.waiters, key)
			continue
		}
		ageMs := now.Sub(startedAt).Milliseconds()
		if ageMs < 0 {
			ageMs = 0
		}
		status.Pending++
		if ageMs > status.OldestMs {
			status.OldestMs = ageMs
		}
		if ageMs >= status.SlowThreshold {
			status.SlowPending++
			if ageMs > status.SlowestMs {
				status.SlowestMs = ageMs
			}
		}
	}
	empty := len(counter.waiters) == 0
	counter.mu.Unlock()
	if empty || status.Pending == 0 {
		channelFirstByteWaiters.Delete(channelID)
		return nil
	}
	return status
}

func clearChannelFirstByteWaitersForTest() {
	channelFirstByteWaiters.Range(func(key, value any) bool {
		channelFirstByteWaiters.Delete(key)
		return true
	})
}
