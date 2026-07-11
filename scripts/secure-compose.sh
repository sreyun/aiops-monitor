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
#   4. 两个密码均满足：长度 ≥ 24，且同时包含「大写 / 小写 / 数字 / 特殊字符」
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
#   PW_LEN       密码长度（默认 24）

set -eu

# ---------- 可配置项 ----------
# 编排文件地址：默认自动检测（GitHub 可达 → GitHub，否则 → Gitee 镜像）
# 也可通过环境变量 COMPOSE_URL 强制指定
GITHUB_COMPOSE="https://raw.githubusercontent.com/sreyun/aiops-monitor/master/docker-compose.yml"
GITEE_COMPOSE="https://gitee.com/bigdatasafe/aiops-monitor/raw/master/docker-compose.yml"
OUT_FILE="${OUT_FILE:-docker-compose.yml}"
PW_LEN="${PW_LEN:-24}"

# 安全字符集：排除在「YAML 单引号」与「Postgres URI 密码」中会引发歧义的字符。
# 排除：单引号 '  与号 &  反斜杠 \  @  :  /  ?  #  %  +  空格
# 排除：$ 符号（Docker Compose 会将其后跟的 {VAR} 当作变量插值，即使 YAML 单引号也无法阻止）
# 注意：连字符 - 必须放在集合末尾，避免被 tr 当作范围运算符。
SPECIAL_CHARS='!~()*.=^-'
FULL_CHARS="A-Za-z0-9${SPECIAL_CHARS}"

# ---------- 生成密码（保证四类中每类至少一个，并洗牌） ----------
gen_password() {
  local n="$1"
  local u l d s r
  u=$(LC_ALL=C tr -dc 'A-Z'            < <(head -c 128 /dev/urandom) | head -c1)
  l=$(LC_ALL=C tr -dc 'a-z'            < <(head -c 128 /dev/urandom) | head -c1)
  d=$(LC_ALL=C tr -dc '0-9'            < <(head -c 128 /dev/urandom) | head -c1)
  s=$(LC_ALL=C tr -dc "$SPECIAL_CHARS" < <(head -c 256 /dev/urandom) | head -c1)
  r=$(LC_ALL=C tr -dc "$FULL_CHARS"    < <(head -c 512 /dev/urandom) | head -c$((n - 4)))
  # Fisher–Yates 洗牌（POSIX awk，无需 shuf；macOS 默认无 shuf）
  printf '%s%s%s%s%s' "$u" "$l" "$d" "$s" "$r" | awk '{
    n = length($0); s = $0
    for (i = n; i > 1; i--) {
      j = int(rand() * i) + 1
      c = substr(s, i, 1); t = substr(s, j, 1)
      s = substr(s, 1, i - 1) t substr(s, i + 1)
      s = substr(s, 1, j - 1) c substr(s, j + 1)
    }
    printf "%s", s
  }'
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
SECRET_KEY=$(gen_password "$PW_LEN")
PG_PASSWORD=$(gen_password "$PW_LEN")

echo "==> 正在将随机密钥写入 $OUT_FILE（无需手动修改）"
awk -v q="'" -v secret="$SECRET_KEY" -v pg="$PG_PASSWORD" '
  /AIOPS_SECRET_KEY=/ {
    eq = index($0, "=")
    print substr($0, 1, eq) q secret q
    next
  }
  /POSTGRES_PASSWORD=/ {
    eq = index($0, "=")
    print substr($0, 1, eq) q pg q
    next
  }
  /AIOPS_POSTGRES_DSN=/ {
    eq = index($0, "=")
    print substr($0, 1, eq) q "postgres://aiops:" pg "@postgres:5432/aiops?sslmode=disable" q
    next
  }
  { print }
' "$OUT_FILE" > "$OUT_FILE.tmp" && mv "$OUT_FILE.tmp" "$OUT_FILE"

echo ""
echo "✓ 完成！docker-compose.yml 已写入随机密钥（长度 $PW_LEN，含大小写 / 数字 / 特殊字符）"
echo "  下一步："
echo "    docker compose up -d"
echo "  浏览器打开 http://localhost:8529  （默认账号 admin / admin，首次登录强制修改密码）"
