# ModelGateway Typed Circuit Policy Ops Guide

## Purpose

This guide defines the recommended rollout and tuning policy for typed circuit breaking in the intelligent model gateway. It covers `rate_limit`, `server_error`, `stream_interrupted`, `upstream_error`, `auth`, and `quota`.

The goal is to protect Codex-style long running requests from unstable upstream routes without letting user-side quota, key, or account issues globally punish a healthy channel.

## Runtime Semantics

The circuit breaker works per runtime key:

```text
requested_model + upstream_model + channel_id + group + endpoint_type + capability_fingerprint
```

This means a bad model/channel combination can be isolated without taking down every model on the same upstream channel.

Default unified circuit behavior is still available:

```json
{
  "circuit_failure_threshold": 0.5,
  "circuit_min_samples": 10,
  "circuit_open_seconds": 30,
  "circuit_half_open_probe_count": 3,
  "circuit_error_policies": {}
}
```

When `circuit_error_policies` is empty, default counting keeps the previous behavior for these failures:

- `stream_interrupted`
- HTTP `429`
- HTTP `5xx`
- upstream errors with no HTTP status but with `error_code` or `error_type`

`auth` and `quota` are intentionally excluded from the default unified circuit path. They only participate when explicitly configured under `circuit_error_policies`.

## Error Types

| Error type | Typical source | Default behavior | Main risk |
| --- | --- | --- | --- |
| `rate_limit` | HTTP 429, provider throttle labels | Counted by default | Too aggressive policy may over-isolate a channel during short bursts. |
| `server_error` | HTTP 5xx | Counted by default | Too loose policy keeps sending traffic to an unhealthy provider. |
| `stream_interrupted` | SSE EOF, provider failed event after partial output | Counted by default | Very costly for Codex long tasks because partial output may already be sent. |
| `upstream_error` | No HTTP status but provider error label exists | Counted by default | Labels may be broad; use moderate thresholds. |
| `auth` | HTTP 401/403, invalid key, permission denied | Explicit only | User/key-specific failures can wrongly punish a shared channel. |
| `quota` | insufficient quota, balance not enough, pre-consume quota failure | Explicit only | User-side quota is not channel health; enable only for channel-owned upstream quota. |

## Recommended Profiles

### Conservative Initial Profile

Use this after at least one shadow observation window. It is suitable for small active rollout.

```json
{
  "circuit_error_policies": {
    "rate_limit": {
      "failure_threshold": 0.6,
      "min_samples": 5,
      "open_seconds": 20,
      "half_open_probe_count": 2
    },
    "server_error": {
      "failure_threshold": 0.5,
      "min_samples": 10,
      "open_seconds": 30,
      "half_open_probe_count": 3
    },
    "stream_interrupted": {
      "failure_threshold": 0.4,
      "min_samples": 5,
      "open_seconds": 60,
      "half_open_probe_count": 1
    },
    "upstream_error": {
      "failure_threshold": 0.6,
      "min_samples": 8,
      "open_seconds": 30,
      "half_open_probe_count": 2
    }
  }
}
```

Use cases:

- first active rollout for Codex traffic;
- mixed MiMo, DeepSeek V4 Pro, and OpenAI Codex compatible channels;
- limited real production sample size.

### Stream-Protective Profile

Use this for groups dominated by long Codex streams where interruption hurts more than slower failover.

```json
{
  "circuit_error_policies": {
    "rate_limit": {
      "failure_threshold": 0.5,
      "min_samples": 4,
      "open_seconds": 30,
      "half_open_probe_count": 2
    },
    "server_error": {
      "failure_threshold": 0.5,
      "min_samples": 8,
      "open_seconds": 45,
      "half_open_probe_count": 2
    },
    "stream_interrupted": {
      "failure_threshold": 0.35,
      "min_samples": 3,
      "open_seconds": 90,
      "half_open_probe_count": 1
    },
    "upstream_error": {
      "failure_threshold": 0.5,
      "min_samples": 5,
      "open_seconds": 45,
      "half_open_probe_count": 1
    }
  }
}
```

Use cases:

- Codex agent loops;
- long reasoning streams;
- tool-call heavy Responses-to-Chat bridge traffic;
- providers with a history of partial stream failure.

### Provider-Saturated Profile

Use this when rate limit is the dominant incident signal and alternative channels are available.

```json
{
  "circuit_error_policies": {
    "rate_limit": {
      "failure_threshold": 0.4,
      "min_samples": 3,
      "open_seconds": 45,
      "half_open_probe_count": 1
    },
    "server_error": {
      "failure_threshold": 0.5,
      "min_samples": 10,
      "open_seconds": 30,
      "half_open_probe_count": 3
    },
    "stream_interrupted": {
      "failure_threshold": 0.4,
      "min_samples": 5,
      "open_seconds": 60,
      "half_open_probe_count": 1
    },
    "upstream_error": {
      "failure_threshold": 0.6,
      "min_samples": 8,
      "open_seconds": 30,
      "half_open_probe_count": 2
    }
  }
}
```

Use cases:

- known provider burst limits;
- enough healthy backup channels;
- queue wait is already visible before failures.

## Auth And Quota Policy

Do not enable `auth` or `quota` circuit policies by default.

Only enable them when all conditions below are true:

- the failing key or quota belongs to the upstream channel itself, not an end user;
- one runtime key failure is expected to affect future requests on the same channel;
- replay and observability samples show repeated provider-side auth or provider-side quota errors;
- the channel has healthy alternatives in the same group or cross-group candidate pool.

Recommended emergency profile for provider-owned credentials:

```json
{
  "circuit_error_policies": {
    "auth": {
      "failure_threshold": 1.0,
      "min_samples": 2,
      "open_seconds": 300,
      "half_open_probe_count": 1
    },
    "quota": {
      "failure_threshold": 1.0,
      "min_samples": 2,
      "open_seconds": 300,
      "half_open_probe_count": 1
    }
  }
}
```

Treat this as an incident control, not as a normal steady-state policy. Remove it after the channel key, account, or upstream quota is repaired.

## Rollout Playbook

### 1. Shadow Observation

Start with intelligent scheduling in `shadow` for the target group:

```json
{
  "enabled": true,
  "group_policies": {
    "codex-pro": {
      "mode": "shadow",
      "strategy": "balanced",
      "auto_mode": "auto_sequential",
      "cross_group_fusion": false,
      "candidate_groups": [],
      "cache_affinity_enabled": true,
      "queue_enabled": false,
      "circuit_breaker_enabled": false
    }
  }
}
```

Observe at least one representative traffic window before active rollout:

- 6 hours for small traffic;
- 24 hours for normal production traffic;
- longer if the group has mostly scheduled or batch-like traffic.

### 2. Small Active Rollout

Turn on circuit breaking only for the target group:

```json
{
  "enabled": true,
  "rollout_percent": 5,
  "group_policies": {
    "codex-pro": {
      "mode": "active",
      "strategy": "balanced",
      "auto_mode": "auto_sequential",
      "cross_group_fusion": false,
      "candidate_groups": [],
      "cache_affinity_enabled": true,
      "queue_enabled": true,
      "circuit_breaker_enabled": true
    }
  }
}
```

Use the conservative profile first unless the group has a clear stream-interruption or rate-limit incident pattern.

### 3. Expand Rollout

Increase rollout only when these are stable:

- user-visible failure rate decreases or stays flat;
- P95 queue wait does not exceed the group SLO;
- `circuit_open_reasons` are dominated by expected provider-side reasons;
- sticky retention remains healthy unless a channel is genuinely degraded;
- no single fallback channel becomes saturated.

### 4. Rollback

Fast rollback options:

```json
{
  "group_policies": {
    "codex-pro": {
      "mode": "shadow",
      "circuit_breaker_enabled": false
    }
  }
}
```

or clear typed policies while keeping the old unified circuit behavior:

```json
{
  "circuit_error_policies": {}
}
```

For full isolation, set the group policy to `off`.

## Observability Checklist

Use the model gateway observability summary:

```bash
curl -G 'http://localhost:3000/api/model_gateway/observability/summary' \
  --data-urlencode 'hours=24' \
  --data-urlencode 'trend_bucket_seconds=3600' \
  --data-urlencode 'group=codex-pro'
```

Fields to watch:

- `summary.circuit_open_reasons`
- `summary.circuit_error_types`
- `summary.circuit_error_counts`
- `risk.top_circuit_open_reasons`
- `risk.top_circuit_error_types`
- `trends[].circuit_error_types`
- `runtime_status.items[].circuit_open_reason`
- `runtime_status.items[].circuit_error_counts`

Use trend export for offline review:

```bash
curl -G 'http://localhost:3000/api/model_gateway/observability/trends/export' \
  --data-urlencode 'hours=24' \
  --data-urlencode 'trend_bucket_seconds=3600' \
  --data-urlencode 'group=codex-pro' \
  --data-urlencode 'download=true' \
  -o tmp/modelgateway-circuit-trends.json
```

Interpretation rules:

- Historical trend buckets come from attempt records.
- Runtime status is a current snapshot.
- Do not treat `runtime_status.items[].circuit_error_counts` as historical bucket data.
- A high `stream_interrupted` count is more severe for Codex streams than the same count of short non-stream failures.
- A high `rate_limit` count should be compared with queue depth and active concurrency before shortening open windows.

## Offline Threshold Calibration

Use the calibration script when enough trend export data exists and the next step is turning the profile guidance above into group-specific starting thresholds.

The script reads one or more `modelgateway_trends_export` JSON files. It does not call the API, does not write settings, and does not need SQL, Redis, upstream credentials, or a running API service.

Export one file per target group:

```bash
mkdir -p tmp/modelgateway-calibration

curl -G 'http://localhost:3000/api/model_gateway/observability/trends/export' \
  --data-urlencode 'hours=24' \
  --data-urlencode 'trend_bucket_seconds=3600' \
  --data-urlencode 'group=codex-pro' \
  --data-urlencode 'download=true' \
  -o tmp/modelgateway-calibration/codex-pro-trends.json
```

Then run:

```bash
node scripts/modelgateway-circuit-calibrate.mjs \
  tmp/modelgateway-calibration/codex-pro-trends.json
```

For multiple groups:

```bash
node scripts/modelgateway-circuit-calibrate.mjs \
  tmp/modelgateway-calibration/*-trends.json \
  --format markdown \
  > tmp/modelgateway-calibration/recommendations.md
```

For automation:

```bash
node scripts/modelgateway-circuit-calibrate.mjs \
  tmp/modelgateway-calibration/*-trends.json \
  --format json \
  > tmp/modelgateway-calibration/recommendations.json
```

Useful knobs:

| Option | Default | Meaning |
| --- | ---: | --- |
| `--min-attempts` | `50` | Group attempts needed before normal confidence. |
| `--min-error-count` | `3` | Error events needed before recommending an override with normal confidence. |
| `--format` | `markdown` | Output `markdown` for review notes or `json` for automation. |

Calibration inputs:

- `trends[].attempts`
- `trends[].stream_interrupted`
- `trends[].circuit_error_types`
- `trends[].circuit_error_counts` as a compatibility alias
- `filters.group` to identify the group represented by an export

The script currently calibrates:

- `rate_limit`
- `server_error`
- `stream_interrupted`

It intentionally does not recommend `auth` or `quota` policy entries. Those remain explicit incident controls for provider-owned credential or upstream quota failures.

### Calibration Heuristic

For each group and error type the script computes:

- total attempts across buckets;
- total events;
- number of affected buckets;
- overall event rate;
- affected-bucket P50/P75/P90 event rate;
- maximum bucket event count.

It then starts from the conservative profile and adjusts only within bounded ranges:

| Error type | Threshold range | Open window range | Bias |
| --- | ---: | ---: | --- |
| `rate_limit` | `0.35`-`0.75` | `15`-`90s` | Treat single-bucket bursts as bursty; prefer more samples before long isolation. |
| `server_error` | `0.35`-`0.75` | `20`-`120s` | Increase open window when failures persist across several buckets. |
| `stream_interrupted` | `0.25`-`0.60` | `45`-`180s` | Prefer lower threshold and one half-open probe because partial Codex streams are expensive. |

Confidence levels:

- `low`: low attempts or too few error events. Keep the existing profile unless this is an active incident.
- `medium`: enough events but only one affected bucket. Treat as a burst and review manually.
- `normal`: enough attempts and repeated affected buckets. The recommendation is a reasonable starting point for a small active rollout.

### Applying Recommendations

The script prints a suggested `circuit_error_policies` snippet. Treat it as a review artifact, not an automatic setting patch.

Before applying it:

1. Confirm the export window matches the target rollout window.
2. Confirm the group has healthy alternatives.
3. Confirm the dominant error types are provider-side.
4. Compare the recommendation with the current profile in this guide.
5. Apply to one named group in `shadow` or a small `active` rollout.
6. Re-export trends after the observation window and rerun the script.

Do not copy one group's recommendation to another group without a separate export. Model mix, traffic volume, provider limits, queue depth, and Codex stream length all change the right threshold.

## Tuning Rules

Use these adjustments after observing at least one production window:

| Symptom | Suggested action |
| --- | --- |
| Repeated `stream_interrupted` while alternatives are healthy | Lower `stream_interrupted.failure_threshold` by 0.05 or increase `open_seconds` by 30 seconds. |
| Short rate-limit bursts recover quickly | Increase `rate_limit.min_samples` or lower `open_seconds`. |
| Rate-limit bursts spill into user failures | Lower `rate_limit.failure_threshold` or `min_samples`, and verify queue pressure. |
| Server errors persist across multiple buckets | Increase `server_error.open_seconds` to 45-60 seconds. |
| Circuit opens too often on low traffic | Increase `min_samples`; do not tune only by threshold. |
| Half-open immediately fails | Reduce `half_open_probe_count` to 1 or increase `open_seconds`. |
| Fallback channels become saturated | Do not make circuits more aggressive until queue/candidate capacity is increased. |
| Auth/quota appears in trends | Confirm whether it is provider-owned before enabling typed auth/quota policies. |

## Guardrails

- Keep `default_mode=off` and enable only named groups.
- Keep `circuit_error_policies.auth` and `circuit_error_policies.quota` disabled unless a provider-owned channel credential or account quota is confirmed broken.
- Do not copy thresholds from one group to all groups without checking traffic volume and model mix.
- Do not use runtime snapshot counts as long-term incident counts.
- Do not make Redis runtime sync or Pub/Sub a prerequisite for request-time circuit decisions.
- Pair every policy tightening with rollback steps and a target observation window.

## Suggested Validation

Configuration and runtime focused tests:

```bash
PATH=/opt/homebrew/bin:$PATH go test ./controller ./pkg/modelgateway/scheduler ./pkg/modelgateway/observability \
  -run 'TestModelGatewayConfig|TestCircuitBreaker|TestRuntimeStatus|TestGetModelGatewayObservabilitySummary|TestExportModelGatewayObservabilityTrends' \
  -count=1
```

Runtime smoke:

```bash
PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh
```

Replay gate when policy changes intentionally alter route selection:

```bash
scripts/modelgateway-replay.sh
```

Keep Redis smoke opt-in:

```bash
PATH=/opt/homebrew/bin:$PATH scripts/modelgateway-runtime-smoke.sh --redis
```

## Change Review Checklist

Before committing a policy change:

- The target group is explicit.
- The rollout mode is `shadow` or a small `active` rollout.
- `auth` and `quota` are absent unless there is a provider-owned credential/quota incident.
- The selected profile matches observed traffic, not guesswork.
- Observability fields show the expected dominant error type.
- Replay artifacts are updated only when route selection changes intentionally.
- Rollback is documented in the release note or operation ticket.
