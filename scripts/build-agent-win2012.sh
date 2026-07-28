#!/usr/bin/env bash
# build-agent-win2012.sh — build a Windows amd64 Agent that still runs on
# Windows Server 2012 / 2012 R2 (and Win8 / 8.1).
#
# Why a separate binary?
#   Go ≥1.21 refuses to start on kernels older than Windows 10 / Server 2016.
#   The main module targets Go 1.26 and uses log/slog (Go 1.21+). This script
#   builds a Go 1.20 binary in a temp module, rewriting slog imports to
#   golang.org/x/exp/slog.
#
# Usage:
#   ./scripts/build-agent-win2012.sh
#   VERSION=v0.19.21 ./scripts/build-agent-win2012.sh
#   OUT=dist/aiops-agent-windows-amd64-win2012.exe ./scripts/build-agent-win2012.sh
#
# Requires: Docker (uses golang:1.20 image) OR a local go1.20.x toolchain.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags 2>/dev/null || echo dev)}"
OUT="${OUT:-$ROOT/dist/aiops-agent-windows-amd64-win2012.exe}"
LDFLAGS="-s -w -X main.appVersion=${VERSION}"
TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

echo "[win2012] version=$VERSION"
echo "[win2012] staging sources in $TMP"

mkdir -p "$TMP/src/cmd" "$TMP/src/shared" "$(dirname "$OUT")"
cp -a "$ROOT/cmd/agent/." "$TMP/src/cmd/agent/"
cp -a "$ROOT/shared/." "$TMP/src/shared/"
# embed companion (Dockerfile copies this too)
if [[ -f "$ROOT/config.example.yaml" ]]; then
  cp "$ROOT/config.example.yaml" "$TMP/src/cmd/agent/config_example.yaml"
fi

# Rewrite stdlib slog → x/exp/slog for Go 1.20.
find "$TMP/src/cmd/agent" -type f -name '*.go' -print0 \
  | xargs -0 sed -i.bak 's|"log/slog"|"golang.org/x/exp/slog"|g' 2>/dev/null \
  || find "$TMP/src/cmd/agent" -type f -name '*.go' -print0 \
       | xargs -0 sed -i '' 's|"log/slog"|"golang.org/x/exp/slog"|g'
find "$TMP/src/cmd/agent" -type f -name '*.bak' -delete 2>/dev/null || true

cat >"$TMP/src/go.mod" <<'EOF'
module aiops-monitor

go 1.20

require (
	golang.org/x/exp v0.0.0-20231108232855-2478ac86d95e
	gopkg.in/yaml.v3 v3.0.1
)
EOF

build_with_docker() {
  echo "[win2012] building via docker golang:1.20 ..."
  docker run --rm \
    -v "$TMP/src:/src" \
    -v "$(cd "$(dirname "$OUT")" && pwd):/out" \
    -w /src \
    golang:1.20-bullseye \
    bash -c "set -euo pipefail
      go mod tidy
      CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
        go build -trimpath -ldflags='$LDFLAGS' \
        -o /out/$(basename "$OUT") ./cmd/agent
    "
}

build_with_local() {
  echo "[win2012] building via local GOTOOLCHAIN=go1.20.14 ..."
  (
    cd "$TMP/src"
    export GOTOOLCHAIN=go1.20.14
    export GOTOOLCHAIN_VERSION_CHECK=0
    go mod tidy
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
      go build -trimpath -ldflags="$LDFLAGS" -o "$OUT" ./cmd/agent
  )
}

if command -v docker >/dev/null 2>&1; then
  build_with_docker
elif command -v go >/dev/null 2>&1; then
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
