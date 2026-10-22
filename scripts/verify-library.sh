#!/usr/bin/env bash
# Verify the public siding library builds the way an external Go module would.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "go test ./siding/..."
go test ./siding/...

echo "go build -C testdata/externalconsumer ."
go build -C testdata/externalconsumer -o /dev/null .

echo "Library release checks passed."
