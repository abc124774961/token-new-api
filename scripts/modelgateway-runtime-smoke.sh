#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
REDIS_RUN=0

if [ "${1:-}" = "--redis" ]; then
  REDIS_RUN=1
  shift
fi

cd "$ROOT_DIR"

go test ./pkg/modelgateway/scheduler ./pkg/modelgateway/observability \
  -run 'TestRuntimeSyncMultiNodeSmokeWithEventStoreAndMemoryFallback|TestRuntimeSyncEventStoreCoalescesHighFrequencyUpdates|TestRuntimeStatusServiceFiltersMultiNodeQueueSnapshotByChannel' \
  -count=1

if [ "$REDIS_RUN" -eq 1 ]; then
  MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR=${MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR:-localhost:6380} \
    go test ./pkg/modelgateway/scheduler ./pkg/modelgateway/observability \
      -run 'TestRuntimeSyncMultiNodeSmokeWithRedisOptIn|TestRedisRuntimeSyncEventPubSubOptIn|TestRuntimeEventSubscriberRedisOptInSmoke' \
      -count=1
fi
