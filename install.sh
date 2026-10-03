#!/usr/bin/env bash
#
# Mailserver 一键安装脚本
#
#   Docker 方式（推荐，自动拉取 GHCR 镜像）：
#     curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | bash
#
#   二进制方式（systemd，需 root）：
#     curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | sudo bash -s -- --binary
#
#   指定版本 / 目录 / 域名：
#     curl -fsSL .../install.sh | bash -s -- --version v1.0.0 --mail-host mail.example.com
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
WITH_DNS="${WITH_DNS:-1}"           # 1 安装内置 DNS
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
  --no-dns            不安装内置权威 DNS
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
    --no-dns) WITH_DNS=0 ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数: $1（--help 查看用法）" ;;
  esac
  shift
done

# 选择安装方式
if [ -z "$MODE" ]; then
  if have docker && docker compose version >/dev/null 2>&1; then
    MODE=docker
  else
    MODE=binary
  fi
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
MAIL_HOST=${MAIL_HOST}
ADMIN_EMAILS=${ADMIN_EMAILS}
MAILSERVER_TAG=${TAG:-latest}
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
  log "Web 界面:   http://<服务器IP>/"
  log "API:        http://<服务器IP>:8080/"
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
  download "${GH}/releases/download/${TAG}/mailserver-web_${TAG}.tar.gz"       "${tmp}/web.tar.gz"
  if [ "$WITH_DNS" = "1" ]; then
    download "${GH}/releases/download/${TAG}/nsd_${TAG}_linux_${arch}.tar.gz"  "${tmp}/dns.tar.gz"
  fi

  mkdir -p "${DIR}/bin" "${DIR}/web" "${DIR}/data" /etc/mailserver

  tar -xzf "${tmp}/api.tar.gz" -C "${DIR}/bin"
  tar -xzf "${tmp}/web.tar.gz" -C "${DIR}/web"
  if [ -f "${DIR}/bin/.env.example" ]; then
    mv "${DIR}/bin/.env.example" /etc/mailserver/mailserver.env.example
  fi
  if [ "$WITH_DNS" = "1" ]; then
    tar -xzf "${tmp}/dns.tar.gz" -C "${DIR}/bin"
  fi
  chmod +x "${DIR}/bin/mailserver" "${DIR}/bin/nsd" 2>/dev/null || true

  if [ ! -f /etc/mailserver/mailserver.env ]; then
    log "生成 /etc/mailserver/mailserver.env …"
    cat > /etc/mailserver/mailserver.env <<EOF
PORT=8080
DB_PATH=${DIR}/data/mail.db
JWT_SECRET=$(rand_secret)
MAIL_HOST=${MAIL_HOST}
ADMIN_EMAILS=${ADMIN_EMAILS}
SMTP_PORT=2525
SUBMIT_PORT=587
POP3_PORT=110
IMAP_PORT=143
EOF
    chmod 600 /etc/mailserver/mailserver.env
  else
    warn "/etc/mailserver/mailserver.env 已存在，保留原配置"
  fi

  log "写入 systemd 服务…"
  cat > /etc/systemd/system/mailserver.service <<EOF
[Unit]
Description=Mailserver API (HTTP + SMTP + POP3 + IMAP)
After=network.target

[Service]
Type=simple
WorkingDirectory=${DIR}
EnvironmentFile=/etc/mailserver/mailserver.env
ExecStart=${DIR}/bin/mailserver
Restart=always
RestartSec=3
# 低内存环境限制（可按需调整）
MemoryMax=256M

[Install]
WantedBy=multi-user.target
EOF

  if [ "$WITH_DNS" = "1" ]; then
    cat > /etc/systemd/system/mailserver-dns.service <<EOF
[Unit]
Description=Mailserver authoritative DNS
After=network.target

[Service]
Type=simple
WorkingDirectory=${DIR}
Environment=ZONES_PATH=${DIR}/data/zones.json
Environment=DNS_ADDR=:53
ExecStart=${DIR}/bin/nsd
Restart=always
RestartSec=3
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
MemoryMax=128M

[Install]
WantedBy=multi-user.target
EOF
  fi

  # 前端静态资源：交给 nginx（如已安装）
  if have nginx; then
    log "配置 nginx…"
    cat > /etc/nginx/conf.d/mailserver.conf <<EOF
server {
    listen 80;
    server_name ${MAIL_HOST};
    root ${DIR}/web;
    index index.html;
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }
    location / { try_files \$uri /index.html; }
}
EOF
    nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || warn "nginx 配置已写入但重载失败，请检查"
  else
    warn "未检测到 nginx，前端静态文件在 ${DIR}/web，请自行用 nginx 等托管（/api 反向代理到 127.0.0.1:8080）"
  fi

  systemctl daemon-reload
  systemctl enable --now mailserver.service
  if [ "$WITH_DNS" = "1" ]; then systemctl enable --now mailserver-dns.service; fi

  sleep 1
  ok "二进制部署完成（版本 ${TAG}）"
  systemctl --no-pager --lines=0 status mailserver.service || true
  echo
  log "Web 界面:  http://<服务器IP>/（需 nginx）"
  log "配置文件:  /etc/mailserver/mailserver.env"
  log "数据目录:  ${DIR}/data"
  log "查看日志:  journalctl -u mailserver -f"
}

case "$MODE" in
  docker) install_docker ;;
  binary) install_binary ;;
  *) die "未知安装方式: $MODE" ;;
esac
