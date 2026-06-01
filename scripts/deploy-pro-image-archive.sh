#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SERVER_HOST="${SERVER_HOST:-35.224.150.95}"
SERVER_USER="${SERVER_USER:-root}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/gcp-abc124774961}"
SERVER_APP_DIR="${SERVER_APP_DIR:-/www/wwwroot/token-new-api}"
IMAGE_NAME="${IMAGE_NAME:-token-new-api-pro:latest}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.pro.yml}"
ENV_FILE="${ENV_FILE:-.env.pro}"
SERVICE="${SERVICE:-new-api}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:3000/api/status}"
REMOTE_RELEASE_DIR="${REMOTE_RELEASE_DIR:-/root/new-api-image-releases}"
ARCHIVE="${ARCHIVE:-}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

shell_quote() {
  printf "%q" "$1"
}

require_command scp
require_command ssh

if [ -z "$ARCHIVE" ]; then
  echo "ARCHIVE is required. Build one first, for example:" >&2
  echo "  scripts/build-pro-image-archive.sh" >&2
  exit 1
fi

if [ ! -f "$ARCHIVE" ]; then
  echo "Archive not found: $ARCHIVE" >&2
  exit 1
fi

if [ ! -r "$SSH_KEY" ]; then
  echo "SSH key is not readable: $SSH_KEY" >&2
  exit 1
fi

REMOTE_ARCHIVE="$REMOTE_RELEASE_DIR/$(basename "$ARCHIVE")"
SSH_TARGET="$SERVER_USER@$SERVER_HOST"
SSH_OPTS=(
  -i "$SSH_KEY"
  -o StrictHostKeyChecking=accept-new
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=4
)

echo "Preflight on $SSH_TARGET..."
ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
  "set -e; test -d $(shell_quote "$SERVER_APP_DIR"); test -f $(shell_quote "$SERVER_APP_DIR/$ENV_FILE"); test -f $(shell_quote "$SERVER_APP_DIR/$COMPOSE_FILE"); mkdir -p $(shell_quote "$REMOTE_RELEASE_DIR")"

echo "Uploading archive:"
echo "  local:  $ARCHIVE"
echo "  remote: $SSH_TARGET:$REMOTE_ARCHIVE"
scp "${SSH_OPTS[@]}" "$ARCHIVE" "$SSH_TARGET:$REMOTE_ARCHIVE"

echo "Loading image and recreating service..."
ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
  "SERVER_APP_DIR=$(shell_quote "$SERVER_APP_DIR") IMAGE_NAME=$(shell_quote "$IMAGE_NAME") COMPOSE_FILE=$(shell_quote "$COMPOSE_FILE") ENV_FILE=$(shell_quote "$ENV_FILE") SERVICE=$(shell_quote "$SERVICE") HEALTH_URL=$(shell_quote "$HEALTH_URL") REMOTE_ARCHIVE=$(shell_quote "$REMOTE_ARCHIVE") bash -s" <<'REMOTE_SCRIPT'
set -euo pipefail

cd "$SERVER_APP_DIR"

echo "Remote deploy context:"
echo "  app dir:   $SERVER_APP_DIR"
echo "  env file:  $ENV_FILE"
echo "  compose:   $COMPOSE_FILE"
echo "  service:   $SERVICE"
echo "  image:     $IMAGE_NAME"
echo "  archive:   $REMOTE_ARCHIVE"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing server env file: $SERVER_APP_DIR/$ENV_FILE" >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "Missing compose file: $SERVER_APP_DIR/$COMPOSE_FILE" >&2
  exit 1
fi

before_image_id="$(docker image inspect "$IMAGE_NAME" --format '{{.Id}}' 2>/dev/null || true)"
if [ -n "$before_image_id" ]; then
  echo "Previous image id: $before_image_id"
fi

case "$REMOTE_ARCHIVE" in
  *.gz|*.tgz)
    gzip -dc "$REMOTE_ARCHIVE" | docker load
    ;;
  *)
    docker load -i "$REMOTE_ARCHIVE"
    ;;
esac

after_image_id="$(docker image inspect "$IMAGE_NAME" --format '{{.Id}}')"
echo "Loaded image id: $after_image_id"

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --no-build --force-recreate "$SERVICE"

echo "Waiting for health check: $HEALTH_URL"
for i in $(seq 1 45); do
  if command -v curl >/dev/null 2>&1; then
    health_body="$(curl -fsS "$HEALTH_URL" || true)"
  elif command -v wget >/dev/null 2>&1; then
    health_body="$(wget -q -O - "$HEALTH_URL" || true)"
  else
    echo "Neither curl nor wget is available on the server." >&2
    exit 1
  fi

  if printf '%s' "$health_body" | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
    echo "Health check passed."
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps "$SERVICE"
    exit 0
  fi
  sleep 2
done

echo "Health check failed. Recent service logs:" >&2
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" logs --tail=160 "$SERVICE" >&2
exit 1
REMOTE_SCRIPT

echo "Deploy finished."
