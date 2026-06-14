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

var channelFirstByteWaiters sync.Map        // channel_id -> *channelFirstByteWaitCounter
var channelRuntimeFirstByteWaiters sync.Map // ChannelRuntimeIdentity -> *channelFirstByteWaitCounter

type channelFirstByteWaitCounter struct {
	mu      sync.Mutex
	waiters map[string]time.Time
}

type ChannelFirstByteWaitLease struct {
	ChannelID       int
	RuntimeIdentity ChannelRuntimeIdentity
	key             string
	counter         *channelFirstByteWaitCounter
	once            sync.Once
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

func getChannelRuntimeFirstByteWaitCounter(identity ChannelRuntimeIdentity) *channelFirstByteWaitCounter {
	actual, _ := channelRuntimeFirstByteWaiters.LoadOrStore(identity.Normalize(), &channelFirstByteWaitCounter{
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

func BeginChannelRuntimeFirstByteWait(ctx *gin.Context, identity ChannelRuntimeIdentity, requestID string, attemptIndex int) *ChannelFirstByteWaitLease {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return BeginChannelFirstByteWait(ctx, identity.ChannelID, requestID, attemptIndex)
	}
	counter := getChannelRuntimeFirstByteWaitCounter(identity)
	lease := &ChannelFirstByteWaitLease{
		ChannelID:       identity.ChannelID,
		RuntimeIdentity: identity,
		key:             fmt.Sprintf("%s:%d:%d", requestID, attemptIndex, time.Now().UnixNano()),
		counter:         counter,
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
			if lease.RuntimeIdentity.HasAccountScope() {
				channelRuntimeFirstByteWaiters.Delete(lease.RuntimeIdentity.Normalize())
			} else {
				channelFirstByteWaiters.Delete(lease.ChannelID)
			}
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

func GetChannelRuntimeFirstBytePendingStatus(identity ChannelRuntimeIdentity) *ChannelFirstBytePendingStatus {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return GetChannelFirstBytePendingStatus(identity.ChannelID)
	}
	return channelFirstBytePendingStatusFromMap(&channelRuntimeFirstByteWaiters, identity)
}

func ClearChannelFirstBytePendingForChannel(channelID int) int {
	if channelID <= 0 {
		return 0
	}
	cleared := 0
	if _, ok := channelFirstByteWaiters.Load(channelID); ok {
		channelFirstByteWaiters.Delete(channelID)
		cleared++
	}
	channelRuntimeFirstByteWaiters.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if !ok {
			channelRuntimeFirstByteWaiters.Delete(key)
			return true
		}
		if identity.Normalize().ChannelID == channelID {
			channelRuntimeFirstByteWaiters.Delete(key)
			cleared++
		}
		return true
	})
	return cleared
}

func channelFirstBytePendingStatusFromMap(waiters *sync.Map, identity ChannelRuntimeIdentity) *ChannelFirstBytePendingStatus {
	if waiters == nil {
		return nil
	}
	value, ok := waiters.Load(identity.Normalize())
	if !ok {
		return nil
	}
	counter, ok := value.(*channelFirstByteWaitCounter)
	if !ok {
		waiters.Delete(identity.Normalize())
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
	if empty {
		waiters.Delete(identity.Normalize())
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
