#!/usr/bin/env bash
# Year-1 AI 闭环可演示脚本：dry-run → propose → approve → verify → promote
# 用法：
#   BASE=https://aiops.example:8529 COOKIE='session=...' INCIDENT_ID=42 ./scripts/demo-year1-loop.sh
# 或：
#   BASE=... USER=admin PASS='...' INCIDENT_ID=42 ./scripts/demo-year1-loop.sh
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8529}"
INCIDENT_ID="${INCIDENT_ID:?set INCIDENT_ID}"
API="$BASE/api/v1"

cookie_jar="$(mktemp)"
cleanup() { rm -f "$cookie_jar"; }
trap cleanup EXIT

if [[ -z "${COOKIE:-}" ]]; then
  USER="${USER:-admin}"
  PASS="${PASS:?set PASS or COOKIE}"
  curl -fsS -c "$cookie_jar" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" \
    "$API/login" >/dev/null
  AUTH=(-b "$cookie_jar")
else
  AUTH=(-H "Cookie: $COOKIE")
fi

step() {
  local action="$1"
  echo "==> loop/$action"
  curl -fsS "${AUTH[@]}" -X POST "$API/incidents/${INCIDENT_ID}/loop/${action}" \
    -H 'Content-Type: application/json' \
    ${2:+-d "$2"} | tee "/tmp/aiops-loop-${action}.json"
  echo
}

step dry-run
step propose
# 审批可能需要管理员；失败时请在控制台手动批准后继续
step approve '{"note":"demo approve"}' || true
step verify
step promote '{"note":"demo promote to skill"}' || true

echo "Done. Check GET $API/sre/effect and /tmp/aiops-loop-*.json"
echo "Docs: docs/engineering/year1-acceptance.md"
