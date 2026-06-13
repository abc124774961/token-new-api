#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SERVER_HOST="${SERVER_HOST:-153.75.90.233}"
SERVER_USER="${SERVER_USER:-root}"
SSH_KEY="${SSH_KEY:-}"
SERVER_APP_DIR="${SERVER_APP_DIR:-/www/wwwroot/token-new-api}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.pro.yml}"
ENV_FILE="${ENV_FILE:-.env.pro}"
SERVICES="${SERVICES:-${SERVICE:-new-api-web new-api-gateway}}"
WEB_SERVICE="${WEB_SERVICE:-new-api-web}"
GATEWAY_SERVICE="${GATEWAY_SERVICE:-new-api-gateway}"
WEB_HEALTH_URL="${WEB_HEALTH_URL:-http://127.0.0.1:3000/-/healthz}"
WEB_API_STATUS_URL="${WEB_API_STATUS_URL:-http://127.0.0.1:3000/api/status}"
GATEWAY_HEALTH_URL="${GATEWAY_HEALTH_URL:-http://127.0.0.1:3001/-/healthz}"
HEALTH_URL="${HEALTH_URL:-}"
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

REMOTE_ARCHIVE="$REMOTE_RELEASE_DIR/$(basename "$ARCHIVE")"
SSH_TARGET="$SERVER_USER@$SERVER_HOST"
SSH_OPTS=(
  -o StrictHostKeyChecking=accept-new
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=4
)
if [ -n "$SSH_KEY" ]; then
  if [ ! -r "$SSH_KEY" ]; then
    echo "SSH key is not readable: $SSH_KEY" >&2
    exit 1
  fi
  SSH_OPTS=(-i "$SSH_KEY" "${SSH_OPTS[@]}")
fi

echo "Preflight on $SSH_TARGET..."
ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
  "set -e; test -d $(shell_quote "$SERVER_APP_DIR"); test -f $(shell_quote "$SERVER_APP_DIR/$ENV_FILE"); test -f $(shell_quote "$SERVER_APP_DIR/$COMPOSE_FILE"); mkdir -p $(shell_quote "$REMOTE_RELEASE_DIR")"

echo "Uploading archive:"
echo "  local:  $ARCHIVE"
echo "  remote: $SSH_TARGET:$REMOTE_ARCHIVE"
scp "${SSH_OPTS[@]}" "$ARCHIVE" "$SSH_TARGET:$REMOTE_ARCHIVE"

echo "Loading image and recreating service..."
ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
  "SERVER_APP_DIR=$(shell_quote "$SERVER_APP_DIR") COMPOSE_FILE=$(shell_quote "$COMPOSE_FILE") ENV_FILE=$(shell_quote "$ENV_FILE") SERVICES=$(shell_quote "$SERVICES") WEB_SERVICE=$(shell_quote "$WEB_SERVICE") GATEWAY_SERVICE=$(shell_quote "$GATEWAY_SERVICE") WEB_HEALTH_URL=$(shell_quote "$WEB_HEALTH_URL") WEB_API_STATUS_URL=$(shell_quote "$WEB_API_STATUS_URL") GATEWAY_HEALTH_URL=$(shell_quote "$GATEWAY_HEALTH_URL") HEALTH_URL=$(shell_quote "$HEALTH_URL") REMOTE_ARCHIVE=$(shell_quote "$REMOTE_ARCHIVE") bash -s" <<'REMOTE_SCRIPT'
set -euo pipefail

cd "$SERVER_APP_DIR"

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

health_url_for_service() {
  local service="$1"
  if [ -n "$HEALTH_URL" ] && [ "$SERVICES" = "$service" ]; then
    printf '%s\n' "$HEALTH_URL"
    return 0
  fi
  case "$service" in
    "$WEB_SERVICE")
      printf '%s\n' "$WEB_HEALTH_URL"
      ;;
    "$GATEWAY_SERVICE")
      printf '%s\n' "$GATEWAY_HEALTH_URL"
      ;;
    *)
      printf '%s\n' ""
      ;;
  esac
}

wait_for_success_json() {
  local service="$1"
  local url="$2"
  local body=""
  [ -n "$url" ] || return 0

  echo "Waiting for health check: $service $url"
  for i in $(seq 1 60); do
    if command -v curl >/dev/null 2>&1; then
      body="$(curl -fsS "$url" || true)"
    elif command -v wget >/dev/null 2>&1; then
      body="$(wget -q -O - "$url" || true)"
    else
      echo "Neither curl nor wget is available on the server." >&2
      exit 1
    fi

    if printf '%s' "$body" | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
      echo "$service health check passed."
      return 0
    fi
    sleep 2
  done

  echo "$service health check failed. Recent logs:" >&2
  compose logs --tail=160 "$service" >&2
  exit 1
}

wait_for_http_ok() {
  local service="$1"
  local url="$2"
  local status=""
  [ -n "$url" ] || return 0

  echo "Checking HTTP endpoint: $service $url"
  for i in $(seq 1 60); do
    if command -v curl >/dev/null 2>&1; then
      status="$(curl -fsS -o /dev/null -w '%{http_code}' "$url" || true)"
    elif command -v wget >/dev/null 2>&1; then
      if wget -q -O /dev/null "$url"; then
        status="200"
      else
        status=""
      fi
    fi
    case "$status" in
      2*|3*)
        echo "$service HTTP check passed."
        return 0
        ;;
    esac
    sleep 2
  done

  echo "$service HTTP check failed. Recent logs:" >&2
  compose logs --tail=160 "$service" >&2
  exit 1
}

echo "Remote deploy context:"
echo "  app dir:   $SERVER_APP_DIR"
echo "  env file:  $ENV_FILE"
echo "  compose:   $COMPOSE_FILE"
echo "  services:  $SERVICES"
echo "  archive:   $REMOTE_ARCHIVE"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing server env file: $SERVER_APP_DIR/$ENV_FILE" >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "Missing compose file: $SERVER_APP_DIR/$COMPOSE_FILE" >&2
  exit 1
fi

case "$REMOTE_ARCHIVE" in
  *.gz|*.tgz)
    gzip -dc "$REMOTE_ARCHIVE" | docker load
    ;;
  *)
    docker load -i "$REMOTE_ARCHIVE"
    ;;
esac

compose config --services >/dev/null

for service in $SERVICES; do
  echo "Recreating service: $service"
  compose up -d --no-build --force-recreate "$service"
  wait_for_success_json "$service" "$(health_url_for_service "$service")"
  if [ "$service" = "$WEB_SERVICE" ]; then
    wait_for_http_ok "$service api/status" "$WEB_API_STATUS_URL"
  fi
done

compose ps $SERVICES
REMOTE_SCRIPT

echo "Deploy finished."
