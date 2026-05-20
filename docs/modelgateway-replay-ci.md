# Model Gateway Replay CI

The model gateway replay runner is an offline golden replay check for smart dispatch records. It does not require SQL, Redis, upstream provider keys, or a running API server.

## Local Usage

Run the default golden directory and write the CI report:

```sh
scripts/modelgateway-replay.sh
```

Useful overrides:

```sh
MODEL_GATEWAY_REPLAY_GOLDEN=pkg/modelgateway/testdata/replay \
MODEL_GATEWAY_REPLAY_REPORT=tmp/modelgateway-replay-ci.json \
MODEL_GATEWAY_REPLAY_SCORE_TOLERANCE=0.01 \
MODEL_GATEWAY_REPLAY_BREAKDOWN_TOLERANCE=0.01 \
scripts/modelgateway-replay.sh
```

Direct CLI usage:

```sh
go run ./cmd/modelgateway-replay \
  -golden pkg/modelgateway/testdata/replay \
  -report tmp/modelgateway-replay-ci.json

go run ./cmd/modelgateway-replay \
  -golden 'pkg/modelgateway/testdata/replay/*.json' \
  -score-tolerance 0.01 \
  -breakdown-tolerance 0.01
```

Each `-golden` value may be a file, directory, or glob. Directories are scanned recursively for `.json` artifacts.

## CI Behavior

The workflow is `.github/workflows/modelgateway-replay.yml`.

It runs on `pull_request`, selected `push` branches, and manual `workflow_dispatch`. The workflow uses read-only permissions and executes code from the PR branch through `pull_request`, not `pull_request_target`.

The CI job runs:

```sh
go test ./cmd/modelgateway-replay ./pkg/modelgateway/testkit ./pkg/modelgateway/replay -run 'Test.*Replay|TestArtifact' -count=1
scripts/modelgateway-replay.sh
```

The JSON report is uploaded as `modelgateway-replay-ci` when available.

## Exit Codes

- `0`: no blocking regression. Score-only drift may still be reported as `status=drifted`.
- `1`: blocking regression or invalid golden artifact.

Blocking regressions include selected channel/group/handled mismatches and invalid replay artifacts. Score total or score breakdown changes are non-blocking drift so operators can review tuning changes without failing CI.

## Golden Update Flow

1. Export sanitized artifacts from the admin replay export UI or API.
2. Review the batch manifest and failed items before selecting samples.
3. Run the manual review checklist below for every artifact selected for golden storage.
4. Store reviewed artifacts under `pkg/modelgateway/testdata/replay`.
5. Run `scripts/modelgateway-replay.sh` locally and inspect the JSON report.
6. Commit the artifact together with the code or policy change that requires it.

For a batch export from the admin API:

```sh
curl -G 'http://localhost:3000/api/model_gateway/replay/export/batch' \
  --data-urlencode 'hours=24' \
  --data-urlencode 'limit=20' \
  --data-urlencode 'model=gpt-5.1-codex' \
  --data-urlencode 'group=auto' \
  --data-urlencode 'stable_ids=true' \
  --data-urlencode 'download=true' \
  -o tmp/modelgateway-replay-batch.json
```

The same export can be started from `/console/model-gateway` by using the batch replay action on the recent dispatch table. The UI reuses the current time window and model/group/request filters, supports request ID lists, previews the manifest, and downloads stable-ID JSON.

## Manual Review Checklist

Only artifacts that pass every item below may be copied into `pkg/modelgateway/testdata/replay`.

- Request and response bodies do not contain raw prompts, user content, files, image URLs, tool payloads, or provider response text.
- Headers do not contain `Authorization`, cookies, API keys, organization IDs, session IDs, trace IDs that identify a tenant, or full upstream URLs with credentials.
- User, token, channel, provider, and account identifiers are removed, hashed, or replaced with stable synthetic IDs.
- Channel names, provider error messages, and candidate explanations do not reveal secret keys, private base URLs, account emails, tenant names, or internal incident details.
- Quota, pre-consume, balance, and billing metadata are absent unless already normalized to non-identifying replay fields.
- `request_meta` only keeps scheduling signals needed for replay, such as candidate explanations, selected reason, score breakdown, queue/sticky/cache signals, and sanitized capability data.
- `manifest.failed_count` is reviewed. Missing or failed request IDs are either intentionally excluded or re-exported after fixing the source issue.
- Replay expectations match the intended regression boundary: selected channel/group/handled mismatches are blocking; score total and score breakdown drift may be accepted only when the policy change intentionally retunes scoring.

## Golden Storage Rules

- Store artifacts in `pkg/modelgateway/testdata/replay`.
- Use descriptive filenames when manually extracting from a batch, for example `codex_auto_queue_cooldown_replay.json`.
- Keep each golden artifact small enough to explain in review. Prefer several focused artifacts over one broad production dump.
- Do not store batch wrapper files directly unless the runner explicitly supports that shape; store individual `modelgateway_replay` artifacts.
- Keep artifacts deterministic by exporting with `stable_ids=true`.
- When a policy change intentionally changes selection, update the affected golden artifact and mention the expected behavior change in the commit or PR.

## Local Gate Before Commit

Run the replay test package and the batch runner:

```sh
go test ./cmd/modelgateway-replay ./pkg/modelgateway/testkit ./pkg/modelgateway/replay -run 'Test.*Replay|TestArtifact' -count=1
scripts/modelgateway-replay.sh
```

Review `tmp/modelgateway-replay-ci.json`:

- `status=passed`: ready to commit.
- `status=drifted` with `exit_code=0`: review non-blocking score drift and document why it is expected.
- `exit_code=1`: fix the regression or update the golden only if the selected route change is intentional.

For Redis-backed runtime sync smoke, use `scripts/modelgateway-runtime-smoke.sh --redis`; it is intentionally separate from replay CI because replay is offline and deterministic.
