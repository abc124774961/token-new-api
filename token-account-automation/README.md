# Token-Account-Automation

Independent automation service for account authorization recovery, browser simulation tasks, and future computer-operation workflows.

## Scope

This service is deployed separately from the main API gateway. The gateway should not own automation tables, routes, browser dependencies, or worker state. It only emits events to this service.

Phase 1 focuses on Codex/OpenAI OAuth account authorization recovery:

1. Main gateway detects account-level authorization invalidation.
2. Main gateway calls `POST /api/events/account-auth-invalid`.
3. This service idempotently creates an `auth_recover` job.
4. Internal/API workers can later create `auth_token_refresh` and `auth_browser_login` child jobs.
5. Refresh or browser workers store the new credential as an encrypted secret.
6. This service calls the main gateway callback to write the credential back to the exact channel account.
7. Browser workers claim `browser_playwright` jobs through the internal worker API.

## Deployment

Required tokens:

- `AUTOMATION_API_TOKEN`: used by the main gateway or admin tools.
- `AUTOMATION_WORKER_TOKEN`: used by workers.
- `AUTOMATION_SECRET_KEY`: encrypts credentials in `automation_secrets`.

Deployment runbook: [docs/deployment-runbook.md](docs/deployment-runbook.md).

Copy the environment template before a standalone deployment:

```bash
cp .env.example .env
```

Gateway callback:

- `AUTOMATION_GATEWAY_CALLBACK_URL`: main gateway base URL, for example `https://gateway.example.com`.
- `AUTOMATION_GATEWAY_CALLBACK_TOKEN`: token accepted by the main gateway callback.
- `AUTOMATION_GATEWAY_CALLBACK_TIMEOUT_SECONDS=5`

Main gateway env:

- `TOKEN_ACCOUNT_AUTOMATION_URL`: automation service base URL.
- `TOKEN_ACCOUNT_AUTOMATION_API_TOKEN`: token used when the gateway emits auth-invalid events.
- `TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN`: token accepted by the gateway writeback callback. If omitted, the gateway falls back to `TOKEN_ACCOUNT_AUTOMATION_API_TOKEN`.

Internal executor:

- `AUTOMATION_INTERNAL_EXECUTOR=true`: runs the built-in `internal_api` worker loop.
- `AUTOMATION_INTERNAL_WORKER_ID=internal-api-1`
- `AUTOMATION_INTERNAL_POLL_INTERVAL=2`
- `AUTOMATION_INTERNAL_LEASE_SECONDS=60`

Database:

- `AUTOMATION_DB_DRIVER=sqlite|mysql|postgres`
- `AUTOMATION_DB_DSN=...`

Default local SQLite DSN:

```bash
AUTOMATION_DB_DRIVER=sqlite
AUTOMATION_DB_DSN=./data/token-account-automation.db
```

Run locally:

```bash
go test ./...
go run ./cmd/server
```

Health check:

```bash
curl http://127.0.0.1:8091/health
```

Emit auth-invalid event:

```bash
curl -X POST http://127.0.0.1:8091/api/events/account-auth-invalid \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": 12,
    "credential_index": 3,
    "provider": "codex_oauth",
    "subject_key": "codex:account_id:acct_123",
    "display_name": "codex #4",
    "source": "relay",
    "reason": "token_invalidated"
  }'
```

Worker claim:

```bash
curl -X POST http://127.0.0.1:8091/internal/jobs/claim \
  -H "Authorization: Bearer $AUTOMATION_WORKER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"executor_type":"browser_playwright","worker_id":"worker-1"}'
```

Queue stats:

```bash
curl http://127.0.0.1:8091/api/stats/jobs \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN"
```

Operator console:

```text
http://127.0.0.1:8091/operator
```

The console lists waiting-human jobs, shows attempts/events history for the selected job, and can resume or cancel jobs with the API token.

Waiting-human operator queue:

```bash
curl http://127.0.0.1:8091/api/operator/waiting-human \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN"
```

The response includes the job, latest waiting reason, event data, channel account locator, and action paths for resume/cancel.

Browser worker:

```bash
cd workers/browser
npm ci
AUTOMATION_BASE_URL=http://127.0.0.1:8091 \
AUTOMATION_WORKER_TOKEN=$AUTOMATION_WORKER_TOKEN \
BROWSER_WORKER_ID=browser-worker-1 \
BROWSER_HEADLESS=false \
npm run start
```

Optional persistent browser profiles:

- Set `BROWSER_PROFILE_DIR=/worker/profiles` in container deployments to reuse browser session state by target account.
- Treat the profile directory as sensitive secret material because cookies and browser storage may be stored there.
- Leave it empty for fully ephemeral browser sessions.

Worker-only credential completion endpoint:

```bash
POST /internal/jobs/{job_id}/succeed-credential
```

The worker sends the fresh credential in `value`; the service encrypts it, links it to the job, writes it back to the main gateway when callback config is present, and returns only `credential_secret_ref`. If gateway writeback fails, the job is not marked successful.

Human handoff endpoint:

```bash
POST /internal/jobs/{job_id}/waiting-human
```

Use this when captcha, risk control, SSO, or any platform protection requires human action. Do not bypass those controls.

After the operator completes the required action, resume the job:

```bash
curl -X POST http://127.0.0.1:8091/api/jobs/{job_id}/resume \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason":"operator completed browser authorization"}'
```

The service moves the job from `WAITING_HUMAN` back to `PENDING` and grants one additional claim attempt when needed.

Create an encrypted refresh token secret:

```bash
curl -X POST http://127.0.0.1:8091/api/secrets \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "secret_type": "oauth_refresh_token",
    "scope_ref": "auto_target_xxx",
    "value": "refresh-token-value"
  }'
```

The API response returns `secret_ref` and `fingerprint`, never plaintext.

## Database Design

The queue layer is generic and does not contain channel/account-specific columns:

- `automation_jobs`
- `automation_attempts`
- `automation_job_events`

Business objects are resolved through target bindings:

- `automation_targets`
- `automation_target_bindings`

Secrets are separated from job input/results:

- `automation_secrets`
- `automation_job_secret_refs`

`locator_json` is supplementary only. Business lookup should use `binding_type + external_ref_hash`, not JSON queries.

## Authorization Recovery Workflow

`account-auth-invalid` creates one active `auth_recover` job per target. The internal executor consumes `auth_recover`, creates an `auth_token_refresh` child job, then consumes that child job.

For `auth_token_refresh`, the executor:

1. Loads a refresh token from `refresh_token_secret_ref` in job input, or from the latest secret scoped to `target_ref`.
2. Calls Codex/OpenAI OAuth refresh using `grant_type=refresh_token`.
3. Stores the refreshed credential as a new encrypted `codex_oauth` secret.
4. Calls the main gateway writeback callback for the target channel account when configured.
5. Marks the job successful with only `credential_secret_ref`, `fingerprint`, `expires_at`, and `target_ref`.
6. Creates an `auth_browser_login` child job when no refresh token exists or OAuth reports a revoked/invalid token.

Job input, result, events, and attempts must not contain plaintext tokens.

## Task Types

- `auth_recover`: parent recovery task.
- `auth_token_refresh`: API-based OAuth refresh.
- `auth_browser_login`: browser-simulated OAuth login.

Future task types can reuse the same queue:

- account balance sync
- provider console sync
- account health probe
- report export
- file upload/download
- desktop/human handoff

## Implementation Backlog

1. Add production browser profiles/storage policy if persistent sessions are required.
2. Add more workflow types: balance sync, provider console sync, health probe, report export.
3. Add authentication integration for the operator console beyond API-token entry.
