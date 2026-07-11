#!/usr/bin/env bash
#
# secure-compose.sh — 下载 docker-compose.yml 并自动注入强随机密钥
# =========================================================================
# 适用环境：Linux / macOS（依赖 bash 3.2+、curl、awk、tr、head，均为系统自带）
#
# 作用：
#   1. 自动检测网络环境，优先从 GitHub 下载 docker-compose.yml，不可达时降级到 Gitee 镜像
#   2. 生成 AIOPS_SECRET_KEY（配置落库主密钥，用于 AES-256-GCM 静态加密）
#   3. 生成 PostgreSQL 密码，并同步写入 AIOPS_POSTGRES_DSN
#   4. 两个密钥均满足：PG 密码 20 位纯字母数字，SECRET_KEY 为 aiops- + 44 位随机字母数字（共 50 位）
#
# 执行后 docker-compose.yml 可直接 `docker compose up -d`，无需任何手动修改。
#
# 用法：
#   # GitHub（海外/网络畅通）
#   bash <(curl -fsSL https://raw.githubusercontent.com/sreyun/aiops-monitor/master/scripts/secure-compose.sh)
#   # Gitee 镜像（GitHub 访问受限时推荐）
#   bash <(curl -fsSL https://gitee.com/bigdatasafe/aiops-monitor/raw/master/scripts/secure-compose.sh)
#   # 本地下载后执行：
#   curl -fsSL <URL> -o secure-compose.sh && bash secure-compose.sh
#
# 可用环境变量覆盖：
#   COMPOSE_URL  编排文件地址（默认自动检测 GitHub/Gitee）
#   OUT_FILE     输出文件名（默认 docker-compose.yml）

# 在 set -e 之前设置默认值
OUT_FILE="${OUT_FILE:-docker-compose.yml}"

# set -e：命令失败时退出；不使用 -u（macOS bash 3.2 在 process substitution
# 模式下会对整个脚本做静态解析，即使变量在 set -u 前已赋值，后续引用仍被拦截）
set -e

# ---------- 可配置项 ----------
# 编排文件地址：默认自动检测（GitHub 可达 → GitHub，否则 → Gitee 镜像）
# 也可通过环境变量 COMPOSE_URL 强制指定
GITHUB_COMPOSE="https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml"
GITEE_COMPOSE="https://gitee.com/bigdatasafe/aiops-monitor/raw/master/docker-compose.yml"

# ---------- 生成 PostgreSQL 密码（20 位，仅 A-Za-z0-9） ----------
gen_pg_password() {
  LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c20
  printf '\n'
}

# ---------- 生成 AIOPS_SECRET_KEY（aiops- + 44 位 A-Za-z0-9，共 50 位） ----------
gen_secret_key() {
  printf 'aiops-'
  LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c44
  printf '\n'
}

# ---------- 1. 下载编排文件（自动检测网络环境） ----------
if [ -n "${COMPOSE_URL:-}" ]; then
  echo "==> 使用指定编排文件: $COMPOSE_URL"
  curl -fsSL "$COMPOSE_URL" -o "$OUT_FILE"
else
  echo "==> 尝试从 GitHub 下载编排文件…"
  if curl -fsSL --connect-timeout 3 --max-time 10 "$GITHUB_COMPOSE" -o "$OUT_FILE" 2>/dev/null; then
    echo "==> 已从 GitHub 下载"
  else
    echo "==> GitHub 不可达，自动切换为 Gitee 镜像下载"
    curl -fsSL "$GITEE_COMPOSE" -o "$OUT_FILE"
  fi
fi

# ---------- 2. 生成并注入密钥 ----------
SECRET_KEY=$(gen_secret_key)
PG_PASSWORD=$(gen_pg_password)

echo "==> 正在将随机密钥写入 $OUT_FILE（无需手动修改）"
echo "    PG 密码：20 位纯字母数字"
echo "    SECRET_KEY：aiops- + 44 位随机字母数字（共 50 位）"
awk -v secret="$SECRET_KEY" -v pg="$PG_PASSWORD" '
  /AIOPS_SECRET_KEY=/ {
    eq = index($0, "=")
    print substr($0, 1, eq) secret
    next
  }
  /POSTGRES_PASSWORD=/ {
    eq = index($0, "=")
    print substr($0, 1, eq) pg
    next
  }
  /AIOPS_POSTGRES_DSN=/ {
    eq = index($0, "=")
    print substr($0, 1, eq) "postgres://aiops:" pg "@postgres:5432/aiops?sslmode=disable"
    next
  }
  { print }
' "$OUT_FILE" > "$OUT_FILE.tmp" && mv "$OUT_FILE.tmp" "$OUT_FILE"

echo ""
echo "✓ 完成！docker-compose.yml 已写入随机密钥"
echo "   PG 密码：20 位纯字母数字（A-Za-z0-9）"
echo "   SECRET_KEY：aiops- + 44 位随机字母数字（共 50 位）"
echo "  下一步："
echo "    docker compose up -d"
echo "  浏览器打开 http://localhost:8529  （默认账号 admin / admin，首次登录强制修改密码）"
