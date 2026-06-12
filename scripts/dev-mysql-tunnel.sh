#!/bin/sh
set -eu

LOCAL_HOST=${DEV_MYSQL_TUNNEL_LOCAL_HOST:-127.0.0.1}
LOCAL_PORT=${DEV_MYSQL_TUNNEL_LOCAL_PORT:-13306}
REMOTE_HOST=${DEV_MYSQL_TUNNEL_REMOTE_HOST:-127.0.0.1}
REMOTE_PORT=${DEV_MYSQL_TUNNEL_REMOTE_PORT:-3306}
SSH_HOST=${DEV_MYSQL_TUNNEL_SSH_HOST:-144.172.102.41}
SSH_USER=${DEV_MYSQL_TUNNEL_SSH_USER:-root}
SSH_KEY=${DEV_MYSQL_TUNNEL_SSH_KEY:-"$HOME/.ssh/cloudzy_ed25519"}

if nc -z "$LOCAL_HOST" "$LOCAL_PORT" 2>/dev/null; then
  echo "MySQL tunnel is already reachable on ${LOCAL_HOST}:${LOCAL_PORT}"
  exit 0
fi

if lsof -nP -iTCP:"$LOCAL_PORT" -sTCP:LISTEN 2>/dev/null | grep -q "ssh"; then
  lsof -nP -tiTCP:"$LOCAL_PORT" -sTCP:LISTEN 2>/dev/null | xargs kill 2>/dev/null || true
  sleep 1
fi

ssh -i "$SSH_KEY" \
  -f -N \
  -L "${LOCAL_HOST}:${LOCAL_PORT}:${REMOTE_HOST}:${REMOTE_PORT}" \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  "${SSH_USER}@${SSH_HOST}"

echo "MySQL tunnel listening on ${LOCAL_HOST}:${LOCAL_PORT} -> ${SSH_HOST}:${REMOTE_PORT}"
