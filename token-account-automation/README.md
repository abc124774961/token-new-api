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
- `AUTOMATION_DESKTOP_TOKEN`: used by the Electron desktop `desktop_session` executor.
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
- `AUTOMATION_BROWSER_LOGIN_EXECUTOR=browser_playwright|desktop_session`: controls where browser-login fallback jobs are routed.

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

Run with the project root dev/pro env files:

```bash
# Dev: listens on :18091, matches TOKEN_ACCOUNT_AUTOMATION_URL in ../.env.dev.
AUTOMATION_ENV_FILE=../.env.dev go run ./cmd/server

# Pro: listens on :18091, replace pro tokens in ../.env.pro before use.
AUTOMATION_ENV_FILE=../.env.pro go run ./cmd/server
```

For a local long-running desktop test, building a binary first avoids tying the service to `go run` wrapper processes:

```bash
go build -o /tmp/token-account-automation-server ./cmd/server
AUTOMATION_ENV_FILE=/Users/frode.luo/project/token-new-api/.env.dev /tmp/token-account-automation-server
```

Health check:

```bash
curl http://127.0.0.1:18091/health
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

Electron desktop executor:

```bash
cd desktop
bun install
bun run start
```

`bun run start` is the development launcher. It picks an available renderer port automatically from `5174` upward and passes it to Electron, so it will not fail just because `5173` is already occupied. To open the built local client without a Vite dev server, run `bun run build && bun run open`.

Configure the desktop app with the automation service URL and `AUTOMATION_DESKTOP_TOKEN`. For local dev with the root `.env.dev`, use:

```text
Automation URL: http://127.0.0.1:18091
Desktop Token: dev-desktop-client-token
```

To route browser-login fallback jobs to Electron instead of the Playwright worker, set:

```bash
AUTOMATION_BROWSER_LOGIN_EXECUTOR=desktop_session
AUTOMATION_DESKTOP_TOKEN=replace-with-desktop-client-token
```

Desktop tokens can only claim and operate `desktop_session` jobs. The general worker token remains required for `browser_playwright` and `internal_api` jobs.

Desktop diagnostics:

- The local environment panel checks the automation health endpoint, desktop API, callback port, env file, gateway callback URL, callback token presence, gateway proxy endpoint, and gateway invalid-pool endpoint.
- A 404 on the gateway invalid-pool endpoint usually means the main gateway process is still running an older build and needs to be published or restarted.
- The callback port shows whether it is available or currently owned by the desktop callback server.

Desktop account pool actions:

- The task detail panel can move the channel account bound to a `desktop_session` job into the main gateway invalid pool or discarded pool.
- Bulk task selection supports the same invalid/discarded pool actions for jobs with a channel account locator.
- The desktop home view can list the main gateway invalid account pool with masked credentials and submit a reauthorization request for a pool item.
- The desktop client calls the automation service only; the automation service resolves the job locator and forwards the archive request to the main gateway through the callback-token internal endpoints.
- Invalid-pool reauthorization is forwarded to the main gateway, which restores the account into the source channel and enqueues a new authorization recovery job.
- Archive actions append an `account_pool_archived` event to the job timeline for operations audit.

Desktop proxy health:

- Remote proxies synced from the main gateway are scored locally with source, geo status, recent success, recent failure, failure count, use count, and cooldown state.
- Authorization jobs automatically pick the highest-scored available proxy unless the operator sets `preferred_proxy_id` during retry.
- Successful authorization records local proxy success; failed authorization records local proxy failure and applies incremental cooldown.
- The desktop proxy panel and retry dropdown display score, status, and the short recommendation reason.

Desktop automation capability map:

- `GET /api/desktop/action-templates` returns the current desktop-visible automation capability list.
- The response separates ready, partial, and planned capabilities across authorization, operations, account pools, diagnostics, and security handoff.
- The desktop sidebar uses this list to show product value, technical entry point, executor/task type, proxy or locator requirements, and whether the action is currently enabled by gateway callback configuration.
- Ready capabilities include auth recovery orchestration, refresh-token recovery, desktop browser login, retry with proxy, local session cleanup, gateway proxy sync, invalid/discarded archive, and invalid-pool reauthorization.
- Partial capabilities include desktop account availability probe and account profile verification jobs. They currently collect local target, browser partition, proxy selection, desktop runtime, and operator context; real upstream account permission/profile callbacks can be attached later without redesigning the client navigation.

Desktop account diagnostics:

- Task detail can enqueue `account_probe` through `POST /api/desktop/jobs/{job_id}/probe`.
- Task detail can enqueue `account_profile_verify` through `POST /api/desktop/jobs/{job_id}/profile-verify`.
- These child jobs reuse the source task target ref, account locator, selected proxy, and optional local-session cleanup flag.
- The desktop executor handles these task types separately from `auth_browser_login`, so diagnostics cannot accidentally open the OAuth login flow.

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
