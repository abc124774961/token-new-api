#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

ENV_FILE="${ENV_FILE:-.env.pro}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.pro.yml}"
SERVICES="${SERVICES:-all}"
GIT_PULL="${GIT_PULL:-true}"
BUILD="${BUILD:-true}"
HEALTH_CHECK="${HEALTH_CHECK:-true}"
WEB_SERVICE="${WEB_SERVICE:-new-api-web}"
GATEWAY_SERVICE="${GATEWAY_SERVICE:-new-api-gateway}"
WEB_HEALTH_PATH="${WEB_HEALTH_PATH:-/-/healthz}"
WEB_API_STATUS_PATH="${WEB_API_STATUS_PATH:-/api/status}"
GATEWAY_HEALTH_PATH="${GATEWAY_HEALTH_PATH:-/-/healthz}"
HEALTH_RETRIES="${HEALTH_RETRIES:-60}"
HEALTH_INTERVAL_SECONDS="${HEALTH_INTERVAL_SECONDS:-2}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/deploy-pro-split.sh [options]

Options:
  --all             Deploy web first, then gateway (default).
  --web            Deploy only new-api-web.
  --gateway        Deploy only new-api-gateway.
  --no-pull        Skip git fetch/pull.
  --no-build       Skip docker compose build.
  --skip-health    Skip HTTP health checks.
  -h, --help       Show this help.

Environment:
  ENV_FILE=.env.pro
  COMPOSE_FILE=docker-compose.pro.yml
  GIT_PULL=true
  BUILD=true
  HEALTH_CHECK=true
  WEB_APP_PORT=3000
  GATEWAY_APP_PORT=3001

Typical server deploy:
  cd /www/wwwroot/token-new-api
  scripts/deploy-pro-split.sh
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --all)
      SERVICES="all"
      ;;
    --web|--web-only)
      SERVICES="$WEB_SERVICE"
      ;;
    --gateway|--gateway-only)
      SERVICES="$GATEWAY_SERVICE"
      ;;
    --no-pull)
      GIT_PULL="false"
      ;;
    --no-build)
      BUILD="false"
      ;;
    --skip-health)
      HEALTH_CHECK="false"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

env_value() {
  local key="$1"
  local fallback="${2:-}"
  local value="${!key:-}"
  if [ -z "$value" ] && [ -f "$ROOT_DIR/$ENV_FILE" ]; then
    value="$(awk -F= -v key="$key" '
      $0 !~ /^[[:space:]]*#/ && $1 == key {
        value = substr($0, index($0, "=") + 1)
      }
      END { print value }
    ' "$ROOT_DIR/$ENV_FILE")"
    value="${value%$'\r'}"
    value="${value%\"}"
    value="${value#\"}"
    value="${value%\'}"
    value="${value#\'}"
  fi
  if [ -z "$value" ]; then
    value="$fallback"
  fi
  printf '%s' "$value"
}

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

wait_for_success_json() {
  local name="$1"
  local url="$2"
  local body=""

  if [ "$HEALTH_CHECK" != "true" ]; then
    return 0
  fi

  echo "Waiting for $name health: $url"
  for _ in $(seq 1 "$HEALTH_RETRIES"); do
    if command -v curl >/dev/null 2>&1; then
      body="$(curl -fsS "$url" || true)"
    elif command -v wget >/dev/null 2>&1; then
      body="$(wget -q -O - "$url" || true)"
    else
      echo "Neither curl nor wget is available for health checks." >&2
      exit 1
    fi

    if printf '%s' "$body" | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
      echo "$name health check passed."
      return 0
    fi
    sleep "$HEALTH_INTERVAL_SECONDS"
  done

  echo "$name health check failed: $url" >&2
  echo "Recent $name logs:" >&2
  compose logs --tail=160 "$name" >&2 || true
  exit 1
}

wait_for_http_ok() {
  local name="$1"
  local url="$2"
  local status=""

  if [ "$HEALTH_CHECK" != "true" ]; then
    return 0
  fi

  echo "Checking $name HTTP endpoint: $url"
  for _ in $(seq 1 "$HEALTH_RETRIES"); do
    if command -v curl >/dev/null 2>&1; then
      status="$(curl -fsS -o /dev/null -w '%{http_code}' "$url" || true)"
    elif command -v wget >/dev/null 2>&1; then
      if wget -q -O /dev/null "$url"; then
        status="200"
      else
        status=""
      fi
    else
      echo "Neither curl nor wget is available for health checks." >&2
      exit 1
    fi

    case "$status" in
      2*|3*)
        echo "$name HTTP check passed."
        return 0
        ;;
    esac
    sleep "$HEALTH_INTERVAL_SECONDS"
  done

  echo "$name HTTP check failed: $url" >&2
  compose logs --tail=160 "$name" >&2 || true
  exit 1
}

deploy_service() {
  local service="$1"
  if [ "$BUILD" = "true" ]; then
    compose build "$service"
  fi
  compose up -d --no-deps --force-recreate "$service"
}

normalize_services() {
  case "$SERVICES" in
    all|"")
      printf '%s\n%s\n' "$WEB_SERVICE" "$GATEWAY_SERVICE"
      ;;
    web)
      printf '%s\n' "$WEB_SERVICE"
      ;;
    gateway)
      printf '%s\n' "$GATEWAY_SERVICE"
      ;;
    *)
      printf '%s\n' $SERVICES
      ;;
  esac
}

require_command docker

cd "$ROOT_DIR"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing env file: $ROOT_DIR/$ENV_FILE" >&2
  echo "Create it from .env.pro.example and fill SQL_DSN, SESSION_SECRET, CRYPTO_SECRET first." >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "Missing compose file: $ROOT_DIR/$COMPOSE_FILE" >&2
  exit 1
fi

mkdir -p data logs-web logs-gateway

echo "Deploy context:"
echo "  root:      $ROOT_DIR"
echo "  env:       $ENV_FILE"
echo "  compose:   $COMPOSE_FILE"
echo "  services:  $SERVICES"
echo "  git pull:  $GIT_PULL"
echo "  build:     $BUILD"

if [ "$GIT_PULL" = "true" ]; then
  require_command git
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    current_branch="$(git rev-parse --abbrev-ref HEAD)"
    echo "Updating git branch: $current_branch"
    git fetch --all --prune
    git pull --ff-only
  fi
fi

compose config --services >/dev/null

web_port="$(env_value WEB_APP_PORT "$(env_value APP_PORT 3000)")"
gateway_port="$(env_value GATEWAY_APP_PORT 3001)"

while IFS= read -r service; do
  [ -n "$service" ] || continue
  case "$service" in
    "$WEB_SERVICE")
      echo "Deploying web service..."
      deploy_service "$WEB_SERVICE"
      wait_for_success_json "$WEB_SERVICE" "http://127.0.0.1:${web_port}${WEB_HEALTH_PATH}"
      wait_for_http_ok "$WEB_SERVICE api/status" "http://127.0.0.1:${web_port}${WEB_API_STATUS_PATH}"
      ;;
    "$GATEWAY_SERVICE")
      echo "Deploying gateway service..."
      deploy_service "$GATEWAY_SERVICE"
      wait_for_success_json "$GATEWAY_SERVICE" "http://127.0.0.1:${gateway_port}${GATEWAY_HEALTH_PATH}"
      ;;
    *)
      echo "Deploying custom service: $service"
      deploy_service "$service"
      ;;
  esac
done < <(normalize_services)

echo "Current compose status:"
compose ps

cat <<EOF

Deploy finished.
Nginx split routing example:
  $ROOT_DIR/docs/split-services-nginx.example.conf

Expected ports:
  web:     127.0.0.1:$web_port
  gateway: 127.0.0.1:$gateway_port
EOF
