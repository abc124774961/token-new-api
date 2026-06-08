#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TARGETS="${TARGETS:-web gateway}"
IMAGE_NAME="${IMAGE_NAME:-}"
IMAGE_NAMES="${IMAGE_NAMES:-}"
PLATFORM="${PLATFORM:-linux/amd64}"
DOCKERFILE="${DOCKERFILE:-Dockerfile.pro.cn}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/output/deploy}"
STAMP="${STAMP:-$(date +%Y%m%d%H%M%S)}"
ARCHIVE_NAME="${ARCHIVE_NAME:-token-new-api-pro-${TARGETS// /-}-${PLATFORM//\//-}-$STAMP.tar.gz}"
ARCHIVE="${ARCHIVE:-$OUT_DIR/$ARCHIVE_NAME}"

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

read_words() {
  local input="$1"
  # shellcheck disable=SC2206
  local words=($input)
  printf '%s\n' "${words[@]}"
}

default_image_for_target() {
  case "$1" in
    web)
      printf '%s\n' "token-new-api-web-pro:latest"
      ;;
    gateway)
      printf '%s\n' "token-new-api-gateway-pro:latest"
      ;;
    full)
      printf '%s\n' "token-new-api-pro:latest"
      ;;
    *)
      printf '%s\n' "token-new-api-$1-pro:latest"
      ;;
  esac
}

mapfile -t target_list < <(read_words "$TARGETS")
if [ "${#target_list[@]}" -eq 0 ]; then
  echo "TARGETS must not be empty." >&2
  exit 1
fi

image_list=()
if [ -n "$IMAGE_NAMES" ]; then
  mapfile -t image_list < <(read_words "$IMAGE_NAMES")
  if [ "${#image_list[@]}" -ne "${#target_list[@]}" ]; then
    echo "IMAGE_NAMES count must match TARGETS count." >&2
    exit 1
  fi
elif [ -n "$IMAGE_NAME" ]; then
  if [ "${#target_list[@]}" -ne 1 ]; then
    echo "IMAGE_NAME can only be used with a single TARGET. Use IMAGE_NAMES for multiple targets." >&2
    exit 1
  fi
  image_list=("$IMAGE_NAME")
else
  for target in "${target_list[@]}"; do
    image_list+=("$(default_image_for_target "$target")")
  done
fi

echo "Building image:"
echo "  root:      $ROOT_DIR"
echo "  dockerfile:$DOCKERFILE"
echo "  targets:   ${target_list[*]}"
echo "  images:    ${image_list[*]}"
echo "  platform:  $PLATFORM"
echo "  archive:   $ARCHIVE"

docker buildx inspect >/dev/null 2>&1 || docker buildx create --use >/dev/null
docker buildx inspect --bootstrap >/dev/null

for idx in "${!target_list[@]}"; do
  target="${target_list[$idx]}"
  image="${image_list[$idx]}"
  echo "Building target '$target' as '$image'..."
  docker buildx build \
    --platform "$PLATFORM" \
    --target "$target" \
    -f "$ROOT_DIR/$DOCKERFILE" \
    -t "$image" \
    --load \
    "$ROOT_DIR"
done

docker save "${image_list[@]}" | gzip -c > "$ARCHIVE"

echo "Image archive written:"
echo "$ARCHIVE"
