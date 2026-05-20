#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
GOLDEN_PATH=${MODEL_GATEWAY_REPLAY_GOLDEN:-"$ROOT_DIR/pkg/modelgateway/testdata/replay"}
REPORT_PATH=${MODEL_GATEWAY_REPLAY_REPORT:-"$ROOT_DIR/tmp/modelgateway-replay-ci.json"}
SCORE_TOLERANCE=${MODEL_GATEWAY_REPLAY_SCORE_TOLERANCE:-}
BREAKDOWN_TOLERANCE=${MODEL_GATEWAY_REPLAY_BREAKDOWN_TOLERANCE:-}

cd "$ROOT_DIR"

if [ "$#" -eq 0 ]; then
  set -- -golden "$GOLDEN_PATH" -report "$REPORT_PATH"
  if [ -n "$SCORE_TOLERANCE" ]; then
    set -- "$@" -score-tolerance "$SCORE_TOLERANCE"
  fi
  if [ -n "$BREAKDOWN_TOLERANCE" ]; then
    set -- "$@" -breakdown-tolerance "$BREAKDOWN_TOLERANCE"
  fi
fi

exec go run ./cmd/modelgateway-replay "$@"
