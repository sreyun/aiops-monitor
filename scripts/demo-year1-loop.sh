#!/usr/bin/env bash
# Year-1 AI 闭环一键 Demo
# 用法：
#   BASE=https://aiops.example:8529 AIOPS_USER=admin PASS='...' INCIDENT_ID=42 ./scripts/demo-year1-loop.sh
#   BASE=... COOKIE='aiops_session=...' INCIDENT_ID=42 ./scripts/demo-year1-loop.sh
#   ONE_CLICK=1  → 调用 POST .../loop/demo（管理员，自动补诊断证据并跑完整闭环）
#   FORCE=1      → 分步执行时带 force=true（跳过闸门）
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8529}"
INCIDENT_ID="${INCIDENT_ID:?set INCIDENT_ID}"
API="$BASE/api/v1"
ONE_CLICK="${ONE_CLICK:-1}"
FORCE="${FORCE:-1}"

cookie_jar="$(mktemp)"
cleanup() { rm -f "$cookie_jar"; }
trap cleanup EXIT

if [[ -z "${COOKIE:-}" ]]; then
  AIOPS_USER="${AIOPS_USER:-${LOGIN_USER:-admin}}"
  PASS="${PASS:?set PASS or COOKIE}"
  code="$(curl -sS -o /tmp/aiops-login.json -w '%{http_code}' -c "$cookie_jar" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$AIOPS_USER\",\"password\":\"$PASS\"}" \
    "$API/login" || true)"
  if [[ "$code" != "200" ]]; then
    echo "login failed HTTP $code: $(cat /tmp/aiops-login.json 2>/dev/null || true)" >&2
    exit 1
  fi
  AUTH=(-b "$cookie_jar")
else
  # Cookie name is aiops_session (not "session")
  AUTH=(-H "Cookie: $COOKIE")
fi

step() {
  local action="$1"
  local body="${2:-{}}"
  echo "==> loop/$action"
  local out="/tmp/aiops-loop-${action}.json"
  local http
  http="$(curl -sS -o "$out" -w '%{http_code}' "${AUTH[@]}" -X POST \
    "$API/incidents/${INCIDENT_ID}/loop/${action}" \
    -H 'Content-Type: application/json' \
    -d "$body" || true)"
  echo "HTTP $http"
  cat "$out"; echo
  if [[ "$http" != "200" ]]; then
    echo "step $action failed" >&2
    return 1
  fi
}

if [[ "$ONE_CLICK" == "1" ]]; then
  step demo '{}'
else
  force_body='{"force":true}'
  if [[ "$FORCE" != "1" ]]; then
    force_body='{}'
  fi
  step dry-run "$force_body"
  step propose "$force_body" || {
    echo "propose blocked — set FORCE=1 or seed AI diagnosis with citations" >&2
    exit 1
  }
  step approve '{}' || echo "(approve skipped/failed — approve in UI if needed)"
  step verify '{}'
  step promote "$force_body" || echo "(promote skipped/failed)"
fi

echo "Done. Check GET $API/sre/effect and /tmp/aiops-loop-*.json"
echo "Docs: docs/engineering/year1-acceptance.md"
