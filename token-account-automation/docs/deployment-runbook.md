# Token-Account-Automation Deployment Runbook

This service is deployed independently from the main gateway. The gateway only emits account auth-invalid events and accepts authenticated credential writeback callbacks.

## 1. Prepare Secrets

Use different tokens for each boundary:

- `AUTOMATION_API_TOKEN`: main gateway -> automation service event ingestion.
- `AUTOMATION_WORKER_TOKEN`: browser/internal workers -> automation service worker API.
- `AUTOMATION_GATEWAY_CALLBACK_TOKEN`: automation service -> main gateway credential writeback.
- `AUTOMATION_SECRET_KEY`: encrypts `automation_secrets`; keep it stable across restarts.

Do not reuse provider credentials as service tokens.

## 2. Start Automation Service

```bash
cd token-account-automation
cp .env.example .env
docker compose --env-file .env -f docker-compose.example.yml up -d --build automation
```

Check health:

```bash
curl http://127.0.0.1:8091/health
```

Check queue stats:

```bash
curl http://127.0.0.1:8091/api/stats/jobs \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN"
```

Operator console:

```text
http://127.0.0.1:8091/operator
```

Use it to inspect `WAITING_HUMAN` jobs, review attempts/events history, and resume or cancel jobs.

## 3. Start Browser Worker

```bash
docker compose --env-file .env -f docker-compose.example.yml up -d --build browser-worker
```

For first deployment, prefer `BROWSER_HEADLESS=false` in a controlled environment to validate the OAuth path. Switch to headless only after the callback and login behavior are verified.

Optional persistent browser sessions:

```bash
BROWSER_PROFILE_DIR=/worker/profiles
```

This reuses browser state per target account. Protect `browser-profiles/` as secret material because it can contain cookies and browser storage.

## 4. Configure Main Gateway

Set these env vars on the main gateway:

```bash
TOKEN_ACCOUNT_AUTOMATION_URL=http://automation-host:8091
TOKEN_ACCOUNT_AUTOMATION_API_TOKEN=replace-with-main-gateway-event-token
TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN=replace-with-gateway-callback-token
TOKEN_ACCOUNT_AUTOMATION_TIMEOUT_SECONDS=3
```

Restart the gateway after updating env.

## 5. Smoke Test

Emit a test auth-invalid event:

```bash
curl -X POST http://automation-host:8091/api/events/account-auth-invalid \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "channel_id": 12,
    "credential_index": 0,
    "provider": "codex_oauth",
    "subject_key": "codex:account_id:acct_123",
    "display_name": "codex account 0",
    "source": "smoke",
    "reason": "token_invalidated"
  }'
```

Verify:

- `GET /api/stats/jobs` shows an `auth_recover` job.
- The internal executor creates `auth_token_refresh` when a refresh token secret exists.
- The browser worker claims `auth_browser_login` when refresh is impossible.
- Main gateway logs contain `token account automation credential writeback success`.
- Only the target `channel_id + credential_index` is updated.

## 6. Operational Checks

Watch these queues first:

- `WAITING_HUMAN`: browser login needs operator action or risk-control handling.
- `FAILED`: worker or gateway callback failure.
- `PENDING` with no claims: missing worker, wrong token, or executor type mismatch.

List jobs waiting for operator action:

```bash
curl http://automation-host:8091/api/operator/waiting-human \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN"
```

Resume a waiting-human job after the operator completes the required action:

```bash
curl -X POST http://automation-host:8091/api/jobs/{job_id}/resume \
  -H "Authorization: Bearer $AUTOMATION_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason":"operator completed browser authorization"}'
```

The resumed job is claimable again by the matching worker type. If the previous attempt already reached `max_attempts`, the service grants one additional attempt.

The automation service never returns plaintext secrets through job results. Use `credential_secret_ref` and `fingerprint` for audit correlation.
