# Implementation Plan

## Phase 1A: Independent Service Foundation

- Standalone Go module and Docker image.
- SQL-backed generic job queue.
- Decoupled target and binding model.
- Admin/API token and worker token authentication.
- Admin APIs for job create/list/detail/retry/cancel.
- Worker APIs for claim/heartbeat/stage/succeed/fail.

## Phase 1B: Authorization Recovery Workflow

- Done: `account_auth_invalid` event ingestion.
- Done: idempotent `auth_recover:{target_ref}` parent job.
- Done: encrypted secret storage API.
- Done: internal executor for `auth_recover`.
- Done: `auth_token_refresh` child job creation.
- Done: refresh-token secret lookup and Codex/OpenAI token refresh.
- Done: browser fallback child job creation when refresh is impossible.

Current boundary:

- The automation service stores refreshed credentials as encrypted `codex_oauth` secrets.
- Job results expose only secret references and fingerprints.
- Browser login jobs are executed by a separate Playwright worker process.
- Gateway credential writeback is outside this service's queue core and uses a minimal authenticated callback contract.

## Phase 1C: Browser Worker

- Done: separate Playwright worker process.
- Done: claim `browser_playwright` jobs from this service.
- Done: run `auth_browser_login` workflow in isolated browser contexts.
- Done: capture OAuth callback code.
- Done: exchange callback code for tokens.
- Done: report stage, success, waiting-human, and sanitized failures.
- Done: optional persistent browser profile directory per target account.

Remaining:

- Validate the login path in target deployment environments.
- Add operator UI/session handoff for accounts that require manual action.
- Define production retention/rotation policy for persistent browser profiles if enabled.

## Phase 1D: Gateway Integration

- Done: main gateway emits account-scoped auth invalid events to this service.
- Done: main gateway does not own automation tables, browser workers, or queue state.
- Done: credential writeback uses `POST /api/internal/token-account-automation/credential`.
- Done: writeback updates only the target channel account credential.
- Done: successful recovery clears only the current account's auth isolation and auth-error capability state.
- Done: refresh/browser jobs are not marked successful when gateway writeback fails.
- Done: successful gateway credential writeback records a sanitized audit log.

## Phase 1E: Deployment and Operations

- Done: standalone Dockerfile for the automation service.
- Done: separate Dockerfile for the Playwright browser worker.
- Done: compose example for automation service + browser worker.
- Done: `.env.example` and deployment runbook.
- Done: queue stats API at `GET /api/stats/jobs`.

## Phase 2

- Done: admin API can resume `WAITING_HUMAN` jobs back to `PENDING` and grant one additional claim attempt when needed.
- Done: operator API lists `WAITING_HUMAN` jobs with reason, event data, target, locator, and action paths.
- Done: built-in operator console for listing, inspecting attempts/events history, resuming, and canceling `WAITING_HUMAN` jobs.
- Desktop session executor.
- Artifact storage with redaction.
- Additional workflow types: balance sync, provider console sync, report export, file upload/download.
