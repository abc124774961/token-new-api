package service

import (
	"math"
	"sync"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

var relayUpstreamCompletedObserver struct {
	mu sync.RWMutex
	fn func(requestID string, observedAt time.Time, duration time.Duration)
}

func RegisterRelayUpstreamCompletedObserver(observer func(requestID string, observedAt time.Time, duration time.Duration)) {
	relayUpstreamCompletedObserver.mu.Lock()
	defer relayUpstreamCompletedObserver.mu.Unlock()
	relayUpstreamCompletedObserver.fn = observer
}

func MarkRelayUpstreamCompleted(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) (time.Time, bool) {
	completedAt := time.Now()
	first := false
	if relayInfo != nil {
		first = relayInfo.SetUpstreamCompletedTime(completedAt)
	}
	ReleaseChannelConcurrencyLease(ctx)
	if first && relayInfo != nil {
		notifyRelayUpstreamCompleted(relayInfo.RequestId, completedAt, relayInfo.UpstreamCompletedDuration())
	}
	return completedAt, first
}

func notifyRelayUpstreamCompleted(requestID string, observedAt time.Time, duration time.Duration) {
	relayUpstreamCompletedObserver.mu.RLock()
	observer := relayUpstreamCompletedObserver.fn
	relayUpstreamCompletedObserver.mu.RUnlock()
	if observer == nil {
		return
	}
	observer(requestID, observedAt, duration)
}

func relayUseTimeSeconds(relayInfo *relaycommon.RelayInfo) int64 {
	if relayInfo == nil {
		return 0
	}
	duration := relayInfo.CurrentRequestDuration()
	if duration <= 0 && !relayInfo.StartTime.IsZero() {
		duration = time.Since(relayInfo.StartTime)
	}
	if duration <= 0 {
		return 0
	}
	return int64(math.Ceil(duration.Seconds()))
}
