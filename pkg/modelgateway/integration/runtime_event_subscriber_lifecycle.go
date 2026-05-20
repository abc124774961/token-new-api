package integration

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
)

var (
	runtimeEventSubscriberMu      sync.Mutex
	defaultRuntimeEventSubscriber *observability.RuntimeEventSubscriber
)

func SyncRuntimeEventSubscriberLifecycle() {
	runtimePolicy := RuntimePolicySetting()
	runtimeEventSubscriberMu.Lock()
	defer runtimeEventSubscriberMu.Unlock()
	if !runtimeEventSubscriberEnabled(runtimePolicy) {
		_ = closeRuntimeEventSubscriberLocked()
		return
	}
	_ = closeRuntimeEventSubscriberLocked()
	startRuntimeEventSubscriberLocked()
}

func StartRuntimeEventSubscriber() {
	runtimeEventSubscriberMu.Lock()
	defer runtimeEventSubscriberMu.Unlock()
	if !runtimeEventSubscriberEnabled(RuntimePolicySetting()) {
		_ = closeRuntimeEventSubscriberLocked()
		return
	}
	if defaultRuntimeEventSubscriber != nil {
		return
	}
	startRuntimeEventSubscriberLocked()
}

func startRuntimeEventSubscriberLocked() {
	defaultRuntimeEventSubscriber = observability.NewRedisRuntimeEventSubscriber(common.RDB, observability.RuntimeEventSubscriberOptions{
		RedisOptions: scheduler.RedisRuntimeSyncEventSubscriberOptions{
			Enabled: func() bool {
				runtimePolicy := RuntimePolicySetting()
				return runtimePolicy.RuntimeSyncEnabled &&
					runtimePolicy.RuntimeSyncRedisEnabled &&
					runtimePolicy.RuntimeSyncEventSubscribeEnabled &&
					common.RedisEnabled &&
					common.RDB != nil
			},
		},
	})
}

func CloseRuntimeEventSubscriber() error {
	runtimeEventSubscriberMu.Lock()
	defer runtimeEventSubscriberMu.Unlock()
	return closeRuntimeEventSubscriberLocked()
}

func RuntimeEventSubscriberSnapshot() observability.RuntimeEventSubscriberSnapshot {
	runtimeEventSubscriberMu.Lock()
	defer runtimeEventSubscriberMu.Unlock()
	if defaultRuntimeEventSubscriber == nil {
		return observability.RuntimeEventSubscriberSnapshot{}
	}
	return defaultRuntimeEventSubscriber.Snapshot()
}

func closeRuntimeEventSubscriberLocked() error {
	if defaultRuntimeEventSubscriber == nil {
		return nil
	}
	err := defaultRuntimeEventSubscriber.Close()
	defaultRuntimeEventSubscriber = nil
	return err
}

func runtimeEventSubscriberEnabled(runtimePolicy RuntimePolicySettings) bool {
	return runtimePolicy.RuntimeSyncEnabled &&
		runtimePolicy.RuntimeSyncRedisEnabled &&
		runtimePolicy.RuntimeSyncEventSubscribeEnabled &&
		common.RedisEnabled &&
		common.RDB != nil
}
