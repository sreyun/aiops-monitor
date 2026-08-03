#!/usr/bin/env bash
# Start the Docker stack (server bound to localhost:18529) and a host-side
# reverse proxy on :8529 that injects the real visitor IP via X-Real-IP.
# Required on Docker Desktop (macOS/Windows): published ports rewrite the
# source address to the bridge gateway (e.g. 192.168.97.1).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export AIOPS_DOCKER_HTTP_PORT="${AIOPS_DOCKER_HTTP_PORT:-18529}"
LISTEN="${AIOPS_HTTP_LISTEN:-:8529}"
TARGET="${AIOPS_PROXY_TARGET:-http://127.0.0.1:${AIOPS_DOCKER_HTTP_PORT}}"

docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build "$@"

echo "Waiting for upstream ${TARGET} ..."
for i in $(seq 1 60); do
  if curl -fsS -o /dev/null "${TARGET}/healthz" 2>/dev/null; then
    break
  fi
  sleep 1
  if [[ "$i" -eq 60 ]]; then
    echo "upstream not healthy: ${TARGET}/healthz" >&2
    exit 1
  fi
done

echo "Starting hostproxy ${LISTEN} → ${TARGET}"
exec go run ./cmd/hostproxy -listen "${LISTEN}" -target "${TARGET}"
