#!/usr/bin/env bash
# build-agent-win2012.sh — build a Windows amd64 Agent that still runs on
# Windows Server 2012 / 2012 R2 (and Win8 / 8.1).
#
# Why a separate binary?
#   Go ≥1.21 refuses to start on kernels older than Windows 10 / Server 2016.
#   The main module targets modern Go and uses log/slog (Go 1.21+). This script
#   builds a Go 1.20 binary in a temp module, rewriting slog imports to
#   golang.org/x/exp/slog (pinned under scripts/win2012/).
#
# Usage:
#   ./scripts/build-agent-win2012.sh
#   VERSION=v0.19.26 ./scripts/build-agent-win2012.sh
#   OUT=dist/aiops-agent-windows-amd64-win2012.exe ./scripts/build-agent-win2012.sh
#   WIN2012_NATIVE=1 ./scripts/build-agent-win2012.sh   # use host/container go (no nested docker)
#
# Requires: Docker (golang:1.20 image) OR a local go1.20.x toolchain.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags 2>/dev/null || echo dev)}"
OUT="${OUT:-$ROOT/dist/aiops-agent-windows-amd64-win2012.exe}"
LDFLAGS="-s -w -X main.appVersion=${VERSION}"
PIN_MOD="$ROOT/scripts/win2012/go.mod"
PIN_SUM="$ROOT/scripts/win2012/go.sum"
TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

if [[ ! -f "$PIN_MOD" || ! -f "$PIN_SUM" ]]; then
  echo "[win2012] FATAL: missing pinned module files under scripts/win2012/" >&2
  exit 1
fi

echo "[win2012] version=$VERSION"
echo "[win2012] staging sources in $TMP"

mkdir -p "$TMP/src/cmd" "$TMP/src/shared" "$(dirname "$OUT")"
cp -a "$ROOT/cmd/agent/." "$TMP/src/cmd/agent/"
cp -a "$ROOT/shared/." "$TMP/src/shared/"
cp "$PIN_MOD" "$TMP/src/go.mod"
cp "$PIN_SUM" "$TMP/src/go.sum"
cp "$ROOT/scripts/win2012/rewrite_slog.sh" "$TMP/src/rewrite_slog.sh"
# embed companion (Dockerfile copies this too)
if [[ -f "$ROOT/config.example.yaml" ]]; then
  cp "$ROOT/config.example.yaml" "$TMP/src/cmd/agent/config_example.yaml"
fi

# Prefer a working module proxy; CI/docker.io may hit flaky endpoints.
export GOPROXY="${GOPROXY:-https://proxy.golang.org,https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"

rewrite_sources() {
  # Adapt log/slog → x/exp/slog import + NewTextHandler call shape for Go 1.20.
  bash "$TMP/src/rewrite_slog.sh" "$TMP/src/cmd/agent"
}

build_go() {
  # Use pinned go.sum — do not tidy (avoids accidental upgrades that drop slog).
  go mod download
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -trimpath -ldflags="$LDFLAGS" -o "$1" ./cmd/agent
}

build_with_docker() {
  echo "[win2012] building via docker golang:1.20 ..."
  # Rewrite inside the Linux container (host sed/PowerShell can corrupt sources).
  docker run --rm \
    -e GOPROXY -e GOSUMDB \
    -v "$TMP/src:/src" \
    -v "$(cd "$(dirname "$OUT")" && pwd):/out" \
    -w /src \
    golang:1.20-bullseye \
    bash -c "set -euo pipefail
      bash /src/rewrite_slog.sh /src/cmd/agent
      go mod download
      CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
        go build -trimpath -ldflags='$LDFLAGS' \
        -o /out/$(basename "$OUT") ./cmd/agent
    "
}

build_with_local() {
  echo "[win2012] building via local Go ($(go env GOVERSION 2>/dev/null || go version)) ..."
  (
    cd "$TMP/src"
    export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.20.14}"
    export GOTOOLCHAIN_VERSION_CHECK=0
    build_go "$OUT"
  )
}

if [[ "${WIN2012_NATIVE:-}" == "1" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "[win2012] FATAL: WIN2012_NATIVE=1 requires a local Go toolchain" >&2
    exit 1
  fi
  rewrite_sources
  build_with_local
elif command -v docker >/dev/null 2>&1; then
  build_with_docker
elif command -v go >/dev/null 2>&1; then
  rewrite_sources
  build_with_local
else
  echo "[win2012] FATAL: need docker or a local Go toolchain" >&2
  exit 1
fi

if [[ ! -f "$OUT" ]]; then
  echo "[win2012] FATAL: output missing: $OUT" >&2
  exit 1
fi

# Companion checksum (same format as release CI / install.ps1)
SUM="$(sha256sum "$OUT" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$OUT" | awk '{print $1}')"
echo "$SUM  $(basename "$OUT")" >"${OUT}.sha256"
echo "[win2012] ok: $OUT"
echo "[win2012] sha256: $SUM"
ls -lh "$OUT" "${OUT}.sha256"
