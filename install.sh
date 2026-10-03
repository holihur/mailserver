#!/usr/bin/env bash
#
# Mailserver 一键安装脚本（二进制，默认）
#
#   一键安装（自动下载预编译单二进制并安装为 systemd 服务，数据库用 PostgreSQL）：
#     curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | sudo bash
#
#   Docker 方式（可选）：
#     curl -fsSL .../install.sh | bash -s -- --docker
#
#   指定版本 / 域名 / 已有数据库：
#     curl -fsSL .../install.sh | sudo bash -s -- --version v0.2.0 --mail-host mail.example.com \
#       --database-url "postgres://user:pass@127.0.0.1:5432/mailserver?sslmode=disable"
#
# 可用环境变量：MAILSERVER_VERSION MAILSERVER_DIR MAILSERVER_MODE MAIL_HOST ADMIN_EMAILS
#
set -euo pipefail

REPO="${MAILSERVER_REPO:-holihur/mailserver}"
GH="https://github.com/${REPO}"
RAW="https://raw.githubusercontent.com/${REPO}"
API="https://api.github.com/repos/${REPO}"

VERSION="${MAILSERVER_VERSION:-latest}"
MODE="${MAILSERVER_MODE:-}"          # docker | binary
DIR="${MAILSERVER_DIR:-/opt/mailserver}"
MAIL_HOST="${MAIL_HOST:-}"
ADMIN_EMAILS="${ADMIN_EMAILS:-}"
WITH_DNS="${WITH_DNS:-1}"           # 1 启用内置权威 DNS（DNS_ADDR=:53）
WEB_PORT="${WEB_PORT:-}"            # Web/API 端口：二进制默认 80，Docker 默认宿主机 80
DATABASE_URL_IN="${DATABASE_URL:-}" # 二进制方式：使用已有 PostgreSQL；为空则本机自动安装
TAG=""                              # 解析后的版本号，如 v1.0.0

# ---------------------------------------------------------------------------
# 基础工具
# ---------------------------------------------------------------------------
if [ -t 1 ]; then
  C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YLW=$'\033[33m'; C_CYN=$'\033[36m'; C_RST=$'\033[0m'
else
  C_RED=""; C_GRN=""; C_YLW=""; C_CYN=""; C_RST=""
fi
log()  { printf '%s[mailserver]%s %s\n' "$C_CYN" "$C_RST" "$*"; }
ok()   { printf '%s[  ok  ]%s %s\n' "$C_GRN" "$C_RST" "$*"; }
warn() { printf '%s[ warn ]%s %s\n' "$C_YLW" "$C_RST" "$*" >&2; }
die()  { printf '%s[ fail ]%s %s\n' "$C_RED" "$C_RST" "$*" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

# 全局清理临时目录
CLEANUP_DIRS=()
cleanup() {
  local d
  for d in ${CLEANUP_DIRS[@]+"${CLEANUP_DIRS[@]}"}; do
    [ -n "$d" ] && rm -rf "$d"
  done
}
trap cleanup EXIT

usage() {
  cat <<'EOF'
用法: install.sh [选项]

选项:
  --docker            使用 Docker + GHCR 预构建镜像安装（默认，若已装 Docker）
  --binary            使用二进制 + systemd 安装（需要 root）
  --dir DIR           安装目录（默认 /opt/mailserver）
  --version TAG       指定版本，如 v1.0.0（默认 latest）
  --mail-host HOST    邮件域名，如 mail.example.com
  --admin EMAILS      管理员邮箱，逗号分隔
  --database-url DSN  使用已有 PostgreSQL（如 postgres://user:pass@host:5432/db?sslmode=disable）；不填则本机自动安装
  --port PORT         Web/API 端口（二进制默认 80，Docker 默认宿主机 80）
  --no-dns            不启用内置权威 DNS
  -h, --help          显示帮助
EOF
}

# 从 /dev/tty 读取，兼容 `curl | bash`
ask() { # <变量名> <提示> <默认值>
  local __var="$1" __prompt="$2" __def="${3:-}" __val=""
  if [ -r /dev/tty ]; then
    printf '%s' "$__prompt" >/dev/tty
    IFS= read -r __val < /dev/tty || __val=""
  fi
  [ -n "$__val" ] || __val="$__def"
  printf -v "$__var" '%s' "$__val"
}

download() { # <url> <输出路径>
  local url="$1" out="$2"
  if have curl; then
    curl -fsSL --retry 3 --connect-timeout 15 -o "$out" "$url"
  elif have wget; then
    wget -q -O "$out" "$url"
  else
    die "需要 curl 或 wget 才能下载文件"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) die "不支持的架构: $(uname -m)（仅支持 amd64 / arm64）" ;;
  esac
}

rand_secret() {
  if have openssl; then openssl rand -hex 32
  else head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
  fi
}

resolve_version() {
  [ "$VERSION" = "latest" ] || { TAG="$VERSION"; return; }
  log "查询最新版本…"
  local tmp; tmp="$(mktemp)"
  download "${API}/releases/latest" "$tmp" || die "无法获取最新版本，请用 --version 指定"
  TAG="$(grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' "$tmp" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
  rm -f "$tmp"
  [ -n "$TAG" ] || die "无法解析最新版本号，请用 --version 指定"
  ok "最新版本: $TAG"
}

# ---------------------------------------------------------------------------
# 参数解析
# ---------------------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --docker) MODE=docker ;;
    --binary) MODE=binary ;;
    --dir) DIR="${2:?--dir 需要参数}"; shift ;;
    --version) VERSION="${2:?--version 需要参数}"; shift ;;
    --mail-host) MAIL_HOST="${2:?--mail-host 需要参数}"; shift ;;
    --admin) ADMIN_EMAILS="${2:?--admin 需要参数}"; shift ;;
    --database-url) DATABASE_URL_IN="${2:?--database-url 需要参数}"; shift ;;
    --no-dns) WITH_DNS=0 ;;
    --port) WEB_PORT="${2:?--port 需要参数}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（--help 查看用法）" ;;
  esac
  shift
done

# 默认二进制安装（无需编译）；如要 Docker 请显式 --docker
if [ -z "$MODE" ]; then
  MODE=binary
fi

# 收集域名
if [ -z "$MAIL_HOST" ]; then
  ask MAIL_HOST "请输入邮件域名 (如 mail.example.com) [mail.example.com]: " "mail.example.com"
fi
[ -n "$ADMIN_EMAILS" ] || ask ADMIN_EMAILS "管理员邮箱 (可留空，逗号分隔): " ""

# 指定了版本则解析为镜像/下载用的 TAG
if [ "$VERSION" != "latest" ]; then TAG="$VERSION"; fi

log "安装方式: ${MODE} | 安装目录: ${DIR} | 域名: ${MAIL_HOST}"

# ---------------------------------------------------------------------------
# PostgreSQL 准备（二进制方式）：未提供 DSN 时尝试本机安装并初始化
# ---------------------------------------------------------------------------
setup_postgres() {
  [ -n "$DATABASE_URL_IN" ] && return 0
  log "未提供数据库，尝试在本机安装并初始化 PostgreSQL…"
  if ! have psql; then
    if have apt-get; then
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq && apt-get install -y -qq postgresql >/dev/null
    elif have dnf; then
      dnf install -y -q postgresql-server postgresql >/dev/null && (postgresql-setup --initdb >/dev/null 2>&1 || true)
    elif have yum; then
      yum install -y -q postgresql-server postgresql >/dev/null && (postgresql-setup --initdb >/dev/null 2>&1 || true)
    else
      die "未找到包管理器，请用 --database-url 指定已有 PostgreSQL"
    fi
  fi
  systemctl enable --now postgresql >/dev/null 2>&1 || service postgresql start >/dev/null 2>&1 || true
  local i=0
  until sudo -u postgres psql -tAc "SELECT 1" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -ge 15 ] && die "PostgreSQL 启动失败，请用 --database-url 指定已有实例"
    sleep 1
  done
  local pw; pw="$(rand_secret | cut -c1-24)"
  sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='mailserver'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE ROLE mailserver LOGIN PASSWORD '$pw'" >/dev/null
  sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='mailserver'" | grep -q 1 || \
    sudo -u postgres createdb -O mailserver mailserver
  DATABASE_URL_IN="postgres://mailserver:${pw}@127.0.0.1:5432/mailserver?sslmode=disable"
  ok "PostgreSQL 已就绪"
}

# ---------------------------------------------------------------------------
# Docker 安装
# ---------------------------------------------------------------------------
install_docker() {
  have docker || die "未安装 Docker，请先安装：https://docs.docker.com/engine/install/"
  docker compose version >/dev/null 2>&1 || die "缺少 docker compose 插件"

  mkdir -p "$DIR"
  log "下载 compose 文件…"
  download "${RAW}/main/docker-compose.release.yml" "${DIR}/docker-compose.yml"

  if [ ! -f "${DIR}/.env" ]; then
    log "生成 ${DIR}/.env …"
    cat > "${DIR}/.env" <<EOF
# 由 install.sh 生成于 $(date -u +%Y-%m-%dT%H:%M:%SZ)
JWT_SECRET=$(rand_secret)
POSTGRES_PASSWORD=$(rand_secret)
MAIL_HOST=${MAIL_HOST}
ADMIN_EMAILS=${ADMIN_EMAILS}
MAILSERVER_TAG=${TAG:-latest}
# Web 端口：宿主机 HTTP_PORT，容器内 PORT
HTTP_PORT=${WEB_PORT:-80}
PORT=8080
EOF
    chmod 600 "${DIR}/.env"
  else
    warn "${DIR}/.env 已存在，保留原配置"
  fi

  log "拉取镜像并启动…"
  ( cd "$DIR" && docker compose pull && docker compose up -d )

  ok "Docker 部署完成"
  ( cd "$DIR" && docker compose ps ) || true
  echo
  log "Web 界面/API:  http://<服务器IP>/"
  log "SMTP 入站:  25 / 2525   客户端提交: 587   取信: 110/143"
  if [ "$WITH_DNS" = "1" ]; then log "权威 DNS:   53/udp+tcp"; fi
  log "查看日志:   cd ${DIR} && docker compose logs -f"
}

# ---------------------------------------------------------------------------
# 二进制 + systemd 安装
# ---------------------------------------------------------------------------
install_binary() {
  [ "$(id -u)" -eq 0 ] || die "二进制安装需要 root：请用 sudo bash -s -- --binary"
  [ "$(uname -s)" = "Linux" ] || die "二进制安装仅支持 Linux"

  local arch; arch="$(detect_arch)"
  resolve_version

  local tmp; tmp="$(mktemp -d)"
  CLEANUP_DIRS+=("$tmp")

  log "下载二进制 (${TAG} / linux-${arch})…"
  download "${GH}/releases/download/${TAG}/mailserver_${TAG}_linux_${arch}.tar.gz" "${tmp}/api.tar.gz"

  mkdir -p "${DIR}/bin" "${DIR}/data" /etc/mailserver

  # 单二进制：已内嵌前端与权威 DNS
  mkdir -p "${tmp}/x"
  tar -xzf "${tmp}/api.tar.gz" -C "${tmp}/x"
  install -m 0755 "${tmp}/x/mailserver" "${DIR}/bin/mailserver"
  if [ -f "${tmp}/x/.env.example" ]; then
    cp "${tmp}/x/.env.example" /etc/mailserver/mailserver.env.example
  fi

  if [ ! -f /etc/mailserver/mailserver.env ]; then
    log "生成 /etc/mailserver/mailserver.env …"
    setup_postgres
    cat > /etc/mailserver/mailserver.env <<EOF
PORT=${WEB_PORT:-80}
DATA_DIR=${DIR}/data
DATABASE_URL=${DATABASE_URL_IN}
CERT_DIR=${DIR}/data/certs
DNS_ADDR=$([ "$WITH_DNS" = "1" ] && echo ":53" || echo "off")
JWT_SECRET=$(rand_secret)
MAIL_HOST=${MAIL_HOST}
ADMIN_EMAILS=${ADMIN_EMAILS}
SMTP_PORT=25
SUBMIT_PORT=587
SUBMIT_TLS_PORT=465
POP3_PORT=110
POP3_TLS_PORT=995
IMAP_PORT=143
IMAP_TLS_PORT=993
EOF
    chmod 600 /etc/mailserver/mailserver.env
  else
    warn "/etc/mailserver/mailserver.env 已存在，保留原配置"
  fi

  log "写入 systemd 服务…"
  cat > /etc/systemd/system/mailserver.service <<EOF
[Unit]
Description=Mailserver (HTTP + SMTP + POP3 + IMAP + DNS)
After=network.target

[Service]
Type=simple
WorkingDirectory=${DIR}
EnvironmentFile=/etc/mailserver/mailserver.env
ExecStart=${DIR}/bin/mailserver
Restart=always
RestartSec=3
# 内置 DNS 需要绑定 :53
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
# 低内存环境限制（可按需调整）
MemoryMax=320M

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable --now mailserver.service

  sleep 1
  ok "二进制部署完成（版本 ${TAG}）"
  systemctl --no-pager --lines=0 status mailserver.service || true
  echo
  log "Web 界面:  http://<服务器IP>:${WEB_PORT:-80}/（前端已内嵌）"
  [ "$WITH_DNS" = "1" ] && log "权威 DNS:  53/udp+tcp（内置在同一进程）"
  log "配置文件:  /etc/mailserver/mailserver.env"
  log "数据目录:  ${DIR}/data"
  log "查看日志:  journalctl -u mailserver -f"
}

case "$MODE" in
  docker) install_docker ;;
  binary) install_binary ;;
  *) die "未知安装方式: $MODE" ;;
esac
