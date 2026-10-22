#!/usr/bin/env bash
# Run siding dev commands inside Docker (no local Go required).
#
#   ./docker/dev.sh test
#   ./docker/dev.sh run plan --path examples/hello --json
#   ./docker/dev.sh doctor --path examples/hello
#   ./docker/dev.sh shell

set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p .docker/siding-data

cmd="${1:-test}"
shift || true

run_dev() {
  docker compose -f docker-compose.dev.yml run --rm "$@"
}

case "$cmd" in
  test)
    run_dev dev sh -c "go mod tidy && go test ./..."
    ;;
  tidy)
    run_dev dev sh -c "go mod tidy"
    ;;
  build)
    exec "$ROOT/docker/build.sh" "${1:-linux}" "${2:-amd64}"
    ;;
  run)
    run_dev dev sh -c "go mod tidy && go run ./cmd/siding $*"
    ;;
  doctor)
    run_dev doctor sh -c "go mod tidy && go run ./cmd/siding doctor $*"
    ;;
  shell)
    run_dev dev sh
    ;;
  *)
    echo "Usage: $0 {test|tidy|build|run|doctor|shell} [args...]" >&2
    exit 1
    ;;
esac
