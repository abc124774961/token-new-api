# ModelGateway Runtime Sync CI And Ops Guide

## Purpose

This guide documents the runtime sync test and operations boundary for the intelligent model gateway. It covers memory fallback smoke, Redis opt-in smoke, local dev Redis, Redis Pub/Sub notifications, subscriber companion lifecycle, and the troubleshooting path.

Runtime sync is intentionally not a hard dependency for the default test path. The request-time scheduler reads local runtime snapshots and does not synchronously wait on Redis or database reads.

## Runtime Sync Model

Runtime sync has three layers:

1. `HybridRuntimeSyncStore`
   - Stores runtime snapshot, circuit snapshot, and queue snapshot.
   - Uses Redis through `HybridCache` when Redis is available and enabled.
   - Falls back to process memory when Redis is unavailable or `runtime_sync_redis_enabled=false`.

2. `RuntimeSyncEventStore`
   - Optional wrapper enabled by `runtime_sync_event_push_enabled=true`.
   - Coalesces high-frequency snapshot updates by stable key.
   - Flushes to the underlying store in the background.
   - Keeps pending writes visible to reads before flush.
   - Falls back to direct store writes when its queue is full or after close.

3. Redis Pub/Sub notification layer
   - Optional notification path for flushed events.
   - It is not the source of truth.
   - The source of truth remains the shared snapshot store / `HybridCache`.
   - Lost, delayed, malformed, or duplicated Pub/Sub messages must not break scheduler or runtime status behavior.

## Configuration Boundary

Core scheduler settings:

| Setting | Default | Meaning |
| --- | --- | --- |
| `runtime_sync_enabled` | `true` | Enables cross-node runtime sync store wiring. If false, scheduler continues with local memory runtime state. |
| `runtime_sync_redis_enabled` | `true` | Allows runtime sync store to use Redis when global Redis is available. If false, forces memory fallback. |
| `runtime_sync_node_id` | empty | Node identity for queue snapshots. Empty falls back to hostname, then `local`. |
| `runtime_sync_ttl_seconds` | `90` | TTL for runtime snapshot, circuit snapshot, and queue snapshot. |
| `runtime_sync_queue_min_interval_ms` | `500` | Queue snapshot write throttle and event flush interval baseline. Values below 100 ms are normalized for event flush. |
| `runtime_sync_event_push_enabled` | `false` | Enables `RuntimeSyncEventStore` background coalescing and optional Pub/Sub publish. |
| `runtime_sync_event_subscribe_enabled` | `false` | Enables explicit observability subscriber companion if every Redis/runtime condition is satisfied. |

The subscriber companion starts only when all of these are true:

```text
runtime_sync_enabled
runtime_sync_redis_enabled
runtime_sync_event_subscribe_enabled
common.RedisEnabled
common.RDB != nil
```

If any condition becomes false after config save/rebuild, the old companion is closed and its Redis subscription loop exits. The companion is not lazily started by ordinary requests, scheduler `Select`, or runtime status queries.

## Local Dev Redis

The dev compose file includes Redis:

```bash
docker compose --env-file .env.dev -f docker-compose.dev.yml up -d redis
docker exec token-new-api-dev-redis redis-cli ping
```

Expected output:

```text
PONG
```

Local dev Redis facts:

- Compose service: `redis`
- Container: `token-new-api-dev-redis`
- Image: `redis:7-alpine`
- Container port: `6379`
- Host port: `6380` by default through `REDIS_PORT=6380`
- Backend connection string in compose: `REDIS_CONN_STRING=redis://redis:6379/0`
- Data volume: `redis-data`
- Redis command: `redis-server --appendonly yes`

The host port is intentionally `6380` to avoid colliding with a local Redis already using `6379`.

## Smoke Commands

Default memory fallback smoke:

```bash
PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh
```

This path does not require Redis. It covers:

- multi-node memory fallback runtime sync
- event store high-frequency coalescing
- runtime status multi-node queue snapshot filtering

Redis opt-in smoke:

```bash
docker compose --env-file .env.dev -f docker-compose.dev.yml up -d redis
docker exec token-new-api-dev-redis redis-cli ping
PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh --redis
```

The Redis smoke defaults to `localhost:6380`. Override it when needed:

```bash
MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR=127.0.0.1:6380 \
  PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh --redis
```

Redis-backed tests use an isolated Redis DB in the test code and clean it before/after the smoke. Keep this smoke opt-in so default CI remains independent from Redis availability.

## Suggested CI Split

Default CI gate:

```bash
PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh
```

Optional Redis CI gate:

```bash
docker compose --env-file .env.dev -f docker-compose.dev.yml up -d redis
docker exec token-new-api-dev-redis redis-cli ping
PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh --redis
```

Do not make the Redis gate block the default offline CI path unless the CI environment explicitly provisions Redis.

Focused Go tests:

```bash
PATH=/opt/homebrew/bin:$PATH go test ./pkg/modelgateway/scheduler ./pkg/modelgateway/observability ./pkg/modelgateway/integration \
  -run 'TestRuntimeSync|TestRuntimeEventSubscriber|TestDefaultRuntimeObservabilityDepsDoNotStartRuntimeEventSubscriber|TestExplicitRuntimeEventSubscriberLifecycle' \
  -count=1
```

Runtime status and config tests:

```bash
PATH=/opt/homebrew/bin:$PATH go test ./controller ./router \
  -run 'TestGetModelGatewayRuntimeStatus|TestModelGatewayConfig' \
  -count=1
```

## Operational Checklist

Before enabling runtime sync broadly:

- Keep group-level intelligent scheduling in `shadow` or small `active` rollout first.
- Set stable `runtime_sync_node_id` per node if hostname is not stable in the deployment platform.
- Keep `runtime_sync_ttl_seconds` greater than the expected status refresh interval and queue snapshot write interval.
- Start with `runtime_sync_event_push_enabled=false` unless high-frequency snapshot writes are already visible as pressure.
- Keep `runtime_sync_event_subscribe_enabled=false` unless an observability process needs active event statistics.
- Verify Redis health and latency before enabling Redis runtime sync across many nodes.

Safe fallback behavior:

- Redis unavailable: `HybridRuntimeSyncStore` continues through memory fallback.
- Runtime sync disabled: scheduler keeps using local snapshots and local circuit breaker.
- Event store queue full: writes fall back to direct store writes for new keys.
- Event store closed: writes fall back to direct store writes.
- Pub/Sub malformed message: subscriber reports error and continues.
- Pub/Sub duplicate or out-of-order event: subscriber filters by event watermark.
- Subscriber callback queue full: subscriber reports error and keeps the receive loop alive.

## Troubleshooting

Redis smoke cannot connect:

```bash
docker compose --env-file .env.dev -f docker-compose.dev.yml ps redis
docker exec token-new-api-dev-redis redis-cli ping
```

If Redis runs on another host or port, set `MODEL_GATEWAY_RUNTIME_SMOKE_REDIS_ADDR`.

Runtime status shows only one node:

- Confirm all nodes have `runtime_sync_enabled=true`.
- Confirm Redis runtime sync is enabled and Redis is reachable from every node.
- Confirm `runtime_sync_ttl_seconds` is not shorter than the status refresh interval.
- Confirm node IDs are unique.

Queue snapshot appears stale:

- Check `runtime_sync_queue_min_interval_ms`.
- Confirm local queue snapshot has data before cross-node aggregation.
- Confirm runtime status filters are not filtering out the node/channel/model/group.

Subscriber companion appears inactive:

- Confirm `runtime_sync_event_subscribe_enabled=true`.
- Confirm `runtime_sync_event_push_enabled=true` if expecting new Pub/Sub events from the default runtime.
- Confirm global Redis is enabled and `common.RDB` is initialized.
- Confirm config save/rebuild has run so lifecycle sync can start the companion.

## Maintenance Rules

- Keep memory fallback smoke as the default required gate.
- Keep Redis smoke opt-in and documented.
- Do not make Pub/Sub a consistency prerequisite.
- Do not start subscriber companion from request paths or runtime status lazy paths.
- When adding new runtime state kinds, update the event store, Pub/Sub event shape, smoke tests, and this guide together.
