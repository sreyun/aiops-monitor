#!/usr/bin/env bash
#
# secure-compose.sh — 下载正式编排并写入强随机密钥到 .env
# =========================================================================
# 适用环境：Linux / macOS（依赖 bash 3.2+、curl、awk、tr、head）
#
# 作用：
#   1. 自动检测网络，优先 GitHub 下载 docker-compose.yml，失败则切 Gitee
#   2. 同步下载 .env.example（若存在），生成/覆盖 .env
#   3. 写入随机 POSTGRES_PASSWORD（20 位）、AIOPS_SECRET_KEY（aiops- + 44 位）
#      与 AIOPS_INSTALL_TOKEN（32 位 hex，供 compose agent 自动加入主机列表）
#   4. 兼容旧版：若编排文件仍含明文密钥行，同步 patch（双保险）
#
# 用法：
#   bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh)
#   bash <(curl -fsSL https://gitee.com/bigdatasafe/aiops-monitor/raw/master/scripts/secure-compose.sh)
#   # 本地仓库内：
#   bash scripts/secure-compose.sh
#
# 环境变量：
#   COMPOSE_URL   编排文件地址（可选）
#   ENV_EXAMPLE_URL  .env.example 地址（可选）
#   OUT_FILE      编排输出（默认 docker-compose.yml）
#   ENV_FILE      密钥输出（默认 .env）
#   SKIP_DOWNLOAD=1  不下载，仅针对当前目录已有编排生成 .env

OUT_FILE="${OUT_FILE:-docker-compose.yml}"
ENV_FILE="${ENV_FILE:-.env}"
SKIP_DOWNLOAD="${SKIP_DOWNLOAD:-0}"

set -e

GITHUB_COMPOSE="https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml"
GITEE_COMPOSE="https://gitee.com/bigdatasafe/aiops-monitor/raw/master/docker-compose.yml"
GITHUB_ENV="https://raw.githubusercontent.com/sreyun/aiops-monitor/master/.env.example"
GITEE_ENV="https://gitee.com/bigdatasafe/aiops-monitor/raw/master/.env.example"

gen_pg_password() {
  LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c20
  printf '\n'
}

gen_secret_key() {
  printf 'aiops-'
  LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c44
  printf '\n'
}

gen_install_token() {
  LC_ALL=C tr -dc 'a-f0-9' < /dev/urandom | head -c32
  printf '\n'
}

download_with_fallback() {
  # $1=out $2=primary $3=fallback $4=label
  out="$1"
  primary="$2"
  fallback="$3"
  label="$4"
  if [ -n "${COMPOSE_URL:-}" ] && [ "$label" = "compose" ]; then
    echo "==> 使用指定编排: $COMPOSE_URL"
    curl -fsSL "$COMPOSE_URL" -o "$out"
    return
  fi
  if [ -n "${ENV_EXAMPLE_URL:-}" ] && [ "$label" = "env" ]; then
    echo "==> 使用指定 .env.example: $ENV_EXAMPLE_URL"
    curl -fsSL "$ENV_EXAMPLE_URL" -o "$out"
    return
  fi
  echo "==> 尝试从 GitHub 下载 $label…"
  if curl -fsSL --connect-timeout 3 --max-time 10 "$primary" -o "$out" 2>/dev/null; then
    echo "==> 已从 GitHub 下载 $label"
  else
    echo "==> GitHub 不可达，切换 Gitee 下载 $label"
    curl -fsSL "$fallback" -o "$out"
  fi
}

if [ "$SKIP_DOWNLOAD" != "1" ]; then
  download_with_fallback "$OUT_FILE" "$GITHUB_COMPOSE" "$GITEE_COMPOSE" "compose"
  # .env.example 下载失败不致命：后面会直接写最小 .env
  if ! download_with_fallback ".env.example.tmp" "$GITHUB_ENV" "$GITEE_ENV" "env" 2>/dev/null; then
    rm -f .env.example.tmp
  fi
else
  echo "==> SKIP_DOWNLOAD=1，跳过下载，使用本地 $OUT_FILE"
fi

SECRET_KEY=$(gen_secret_key)
PG_PASSWORD=$(gen_pg_password)
INSTALL_TOKEN=$(gen_install_token)

echo "==> 写入随机密钥到 $ENV_FILE"
if [ -f .env.example.tmp ]; then
  awk -v secret="$SECRET_KEY" -v pg="$PG_PASSWORD" -v tok="$INSTALL_TOKEN" '
    /^POSTGRES_PASSWORD=/ { print "POSTGRES_PASSWORD=" pg; next }
    /^AIOPS_SECRET_KEY=/ { print "AIOPS_SECRET_KEY=" secret; next }
    /^AIOPS_INSTALL_TOKEN=/ { print "AIOPS_INSTALL_TOKEN=" tok; next }
    { print }
  ' .env.example.tmp > "$ENV_FILE"
  # 若模板无 AIOPS_INSTALL_TOKEN 行，追加一行
  grep -q '^AIOPS_INSTALL_TOKEN=' "$ENV_FILE" || printf 'AIOPS_INSTALL_TOKEN=%s\n' "$INSTALL_TOKEN" >> "$ENV_FILE"
  rm -f .env.example.tmp
elif [ -f .env.example ]; then
  awk -v secret="$SECRET_KEY" -v pg="$PG_PASSWORD" -v tok="$INSTALL_TOKEN" '
    /^POSTGRES_PASSWORD=/ { print "POSTGRES_PASSWORD=" pg; next }
    /^AIOPS_SECRET_KEY=/ { print "AIOPS_SECRET_KEY=" secret; next }
    /^AIOPS_INSTALL_TOKEN=/ { print "AIOPS_INSTALL_TOKEN=" tok; next }
    { print }
  ' .env.example > "$ENV_FILE"
  grep -q '^AIOPS_INSTALL_TOKEN=' "$ENV_FILE" || printf 'AIOPS_INSTALL_TOKEN=%s\n' "$INSTALL_TOKEN" >> "$ENV_FILE"
else
  cat > "$ENV_FILE" <<EOF
POSTGRES_PASSWORD=$PG_PASSWORD
AIOPS_SECRET_KEY=$SECRET_KEY
AIOPS_INSTALL_TOKEN=$INSTALL_TOKEN
AIOPS_HTTP_PORT=8529
AIOPS_FORWARD_PORT_RANGE=10100-10300
TZ=Asia/Shanghai
AIOPS_VM_URL=http://victoriametrics:8428
EOF
fi

# 兼容：若编排里仍有明文密钥占位，一并替换（旧拷贝 / 无 .env 场景）
if [ -f "$OUT_FILE" ] && grep -q 'AIOPS_SECRET_KEY=' "$OUT_FILE" 2>/dev/null; then
  awk -v secret="$SECRET_KEY" -v pg="$PG_PASSWORD" '
    /AIOPS_SECRET_KEY=/ {
      # 仅替换「非 ${VAR}」形式的明文赋值行
      if ($0 ~ /\$\{AIOPS_SECRET_KEY/) { print; next }
      eq = index($0, "=")
      print substr($0, 1, eq) secret
      next
    }
    /POSTGRES_PASSWORD=/ {
      if ($0 ~ /\$\{POSTGRES_PASSWORD/) { print; next }
      eq = index($0, "=")
      print substr($0, 1, eq) pg
      next
    }
    /AIOPS_POSTGRES_DSN=/ {
      if ($0 ~ /\$\{POSTGRES_PASSWORD/) { print; next }
      eq = index($0, "=")
      print substr($0, 1, eq) "postgres://aiops:" pg "@postgres:5432/aiops?sslmode=disable"
      next
    }
    { print }
  ' "$OUT_FILE" > "$OUT_FILE.tmp" && mv "$OUT_FILE.tmp" "$OUT_FILE"
fi

echo ""
echo "✓ 完成！密钥已写入 ${ENV_FILE} (勿提交到 Git)"
echo "   PG 密码：20 位纯字母数字"
echo "   SECRET_KEY：aiops- + 44 位随机字母数字（共 50 位）"
echo "  下一步（正式环境）："
echo "    docker compose up -d"
echo "  开发环境："
echo "    docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build"
echo "  浏览器打开 http://localhost:8529  （默认 admin / admin，首次登录强制改密）"
