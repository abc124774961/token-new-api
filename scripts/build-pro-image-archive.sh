#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

IMAGE_NAME="${IMAGE_NAME:-token-new-api-pro:latest}"
PLATFORM="${PLATFORM:-linux/amd64}"
DOCKERFILE="${DOCKERFILE:-Dockerfile.pro.cn}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/output/deploy}"
STAMP="${STAMP:-$(date +%Y%m%d%H%M%S)}"
SAFE_IMAGE_NAME="$(printf '%s' "$IMAGE_NAME" | tr '/:' '--')"
ARCHIVE="${ARCHIVE:-$OUT_DIR/${SAFE_IMAGE_NAME}-${PLATFORM//\//-}-$STAMP.tar.gz}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

require_command docker
require_command gzip

if [ ! -f "$ROOT_DIR/$DOCKERFILE" ]; then
  echo "Dockerfile not found: $ROOT_DIR/$DOCKERFILE" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "Building image:"
echo "  root:      $ROOT_DIR"
echo "  dockerfile:$DOCKERFILE"
echo "  image:     $IMAGE_NAME"
echo "  platform:  $PLATFORM"
echo "  archive:   $ARCHIVE"

docker buildx inspect >/dev/null 2>&1 || docker buildx create --use >/dev/null
docker buildx inspect --bootstrap >/dev/null

docker buildx build \
  --platform "$PLATFORM" \
  -f "$ROOT_DIR/$DOCKERFILE" \
  -t "$IMAGE_NAME" \
  --load \
  "$ROOT_DIR"

docker save "$IMAGE_NAME" | gzip -c > "$ARCHIVE"

echo "Image archive written:"
echo "$ARCHIVE"
