#!/usr/bin/env bash
# Cross-compile siding via Docker — no local Go required.
#
#   ./docker/build.sh                 # linux/amd64 → bin/siding
#   ./docker/build.sh all             # every OS/arch → dist/ + linux host bin/siding
#   ./docker/build.sh darwin arm64    # one target → bin/siding
#   ./docker/build.sh windows amd64   # → bin/siding.exe

set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ "${1:-}" == "all" ]]; then
  mkdir -p bin dist .docker/siding-data
  echo "Building all platforms -> dist/ ..."
  docker compose -f docker-compose.dev.yml run --rm dev sh /src/docker/build-all.sh
  echo
  echo "Launch zips: tag vX.Y.Z and push — see docs/RELEASE.md"
  exit 0
fi

platform="${1:-linux}"
arch="${2:-amd64}"

case "$platform" in
  windows|linux|darwin) ;;
  *)
    echo "Usage: $0 [all|windows|linux|darwin] [amd64|arm64]" >&2
    exit 1
    ;;
esac

out_name="siding"
if [[ "$platform" == "windows" ]]; then
  out_name="siding.exe"
fi

mkdir -p bin .docker/siding-data

echo "Building ${platform}/${arch} -> bin/${out_name} ..."
docker compose -f docker-compose.dev.yml run --rm dev sh -c \
  "go mod tidy && CGO_ENABLED=0 GOOS=${platform} GOARCH=${arch} go build -ldflags='-s -w' -o /src/bin/${out_name} ./cmd/siding"

echo ""
echo "Built: ${ROOT}/bin/${out_name}"
echo ""
echo "Try:"
echo "  ./bin/${out_name} version"
echo "  ./bin/${out_name} plan --path examples/hello --json"
