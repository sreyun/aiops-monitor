#!/bin/sh
# Drop to the unprivileged aiops user before running the agent.
# When started as root (image default for volume ownership fix), chown the
# writable data dir then exec as uid 10001 — the long-running process never
# stays root.
set -e
if [ "$(id -u)" = "0" ]; then
  mkdir -p /app/data
  chown -R aiops:aiops /app/data 2>/dev/null || true
  exec su-exec aiops /app/aiops-agent "$@"
fi
exec /app/aiops-agent "$@"
