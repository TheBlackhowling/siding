#!/usr/bin/env sh
# Cross-compile every GOOS/GOARCH into /src/dist (repo bind-mounted).
# Invoked from docker/build.ps1 -All and docker/build.sh all.
set -e
cd /src
go mod tidy
mkdir -p /src/dist /src/bin

build_one() {
  os="$1"
  arch="$2"
  ext=""
  if [ "$os" = windows ]; then
    ext=".exe"
  fi
  echo "Building ${os}/${arch} ..."
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags="-s -w" \
    -o "/src/dist/siding_${os}_${arch}${ext}" ./cmd/siding
}

build_one windows amd64
build_one windows arm64
build_one linux amd64
build_one linux arm64
build_one darwin amd64
build_one darwin arm64

if [ -f /src/dist/siding_windows_amd64.exe ]; then
  cp /src/dist/siding_windows_amd64.exe /src/bin/siding.exe || echo "note: could not refresh /src/bin/siding.exe (file locked on Windows host)"
fi
if [ -f /src/dist/siding_linux_amd64 ]; then
  cp /src/dist/siding_linux_amd64 /src/bin/siding
  chmod +x /src/bin/siding
fi

echo "Wrote dist/ and host bin/"
ls -1 /src/dist
