# Mailserver — 轻量全栈邮件系统

前端 React + TailwindCSS + shadcn 风格 · 后端 Go + GORM · 单文件 SQLite

> 低内存设计：后端单二进制 + SQLite（无 Postgres/Redis），前端 Vite 构建后仅静态文件。
> 内存占用：后端常驻约 15~30MB。

## 目录

```
mailserver/
├── backend/          # Go (stdlib net/http + GORM, 无 Gin)
│   ├── main.go
│   ├── internal/
│   │   ├── config/config.go
│   │   ├── model/models.go
│   │   ├── db/db.go
│   │   ├── auth/jwt.go
│   │   ├── handler/{auth,mail,dns,admin,admin_provider}.go
│   │   ├── provider/           # 阿里云 / Cloudflare DNS 下发
│   │   ├── smtp/ pop3/ imap/ queue/ dkim/ secret/
│   ├── go.mod
│   └── .env.example
├── dns/              # 自研权威 DNS (miekg/dns)
├── frontend/         # React + Vite + Tailwind + shadcn 风格 ui/ (pnpm)
│   ├── package.json
│   ├── pnpm-lock.yaml
│   ├── vite.config.js
│   ├── tailwind.config.js
│   ├── index.html
│   └── src/
├── .github/workflows/  # CI + Release
├── install.sh          # 一键安装脚本
├── docker-compose.yml         # 本地源码构建
└── docker-compose.release.yml # GHCR 预构建镜像
```

## 快速启动（低内存）

```bash
# 后端 (~20MB)
cd backend
cp .env.example .env
go mod tidy          # 仅首次，需网络
go run .             # 或 go build -ldflags="-s -w" mailserver && ./mailserver

# 前端 dev
cd frontend
pnpm install        # 内存小可加 --config.side-effects-cache=false
pnpm run dev

# 或 docker（仅 2 容器，总 <150MB）
docker compose up -d --build
```

API 默认 `:8080`，SMTP 入站 `:2525`，前端 `:5173`。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/register | 注册 |
| POST | /api/login | 登录 -> JWT |
| GET | /api/me | 当前用户 |
| GET | /api/mails?folder=inbox\|sent&page=1 | 邮件列表 |
| GET | /api/mails/:id | 详情（自动标已读） |
| POST | /api/mails | 发件/存草稿 `{to,subject,body,folder}` |
| PATCH | /api/mails/:id | 星标/已读/移动文件夹 |
| DELETE | /api/mails/:id | 移入 trash |

## 管理后台 & 域名服务商一键配置

访问 `/#/admin`（仅管理员可见，`ADMIN_EMAILS` 或 `admin=true` 用户）。

- **概览**：用户 / 域名 / 邮件 / 待发出 / 存储量。
- **域名服务商**：录入凭证后可一键下发邮件解析（开箱即用）。
  - 已支持 **阿里云 DNS**（AccessKey ID + Secret）与 **Cloudflare**（API Token 或 Email + Global API Key）。
  - 流程：录入凭证 → 自动校验 → 列出账号下域名 → 选择域名（可手填公网 IP，留空则自动探测）→ 自动创建/更新 `A(mail)`、`MX`、`SPF`、`DMARC`、`DKIM`。
  - 凭证使用 `JWT_SECRET` 派生的密钥 AES-GCM 加密存储；更换 `JWT_SECRET` 后需重新录入。

对应接口（均需管理员）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | /api/admin/providers | 服务商列表 / 新增（新增时校验凭证） |
| DELETE | /api/admin/providers/:id | 删除 |
| POST | /api/admin/providers/:id/test | 测试连接 |
| GET | /api/admin/providers/:id/domains | 列出账号下域名 |
| POST | /api/admin/providers/:id/apply | 一键配齐解析 `{domain,ip,mail_host}` |

## 一键安装

`install.sh` 会自动选择 Docker（已安装 Docker 时）或二进制 + systemd 方式部署。

```bash
# 默认：自动选择（推荐 Docker）
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | bash

# 显式指定方式 / 版本 / 域名
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh \
  | bash -s -- --docker --version v1.0.0 --mail-host mail.example.com

# 二进制 + systemd（需要 root）
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh \
  | sudo bash -s -- --binary --mail-host mail.example.com
```

| 选项 | 说明 |
|------|------|
| `--docker` / `--binary` | 安装方式（默认已有 Docker 则用 docker） |
| `--dir DIR` | 安装目录（默认 `/opt/mailserver`） |
| `--version TAG` | 指定版本，如 `v1.0.0`（默认 latest） |
| `--mail-host HOST` | 邮件域名，如 `mail.example.com` |
| `--admin EMAILS` | 管理员邮箱，逗号分隔 |
| `--no-dns` | 不安装内置权威 DNS |

Docker 方式使用 GHCR 预构建镜像（`ghcr.io/holihur/mailserver-{api,dns,web}`），自动生成随机 `JWT_SECRET`；二进制方式生成 systemd 服务与 `/etc/mailserver/mailserver.env`。

## CI / 发布

- **CI**（`.github/workflows/ci.yml`）：push / PR 到 `main` 时校验 backend、dns（`go vet` + `build` + `test`）与 frontend（`pnpm install --frozen-lockfile` + `build`）。
- **Release**（`.github/workflows/release.yml`）：推送 `v*` 标签时自动：
  1. 编译 Linux `amd64`/`arm64` 的 backend、DNS 二进制与前端静态资源，打包 `tar.gz` + `checksums.txt` 并创建 GitHub Release；
  2. 构建并推送多架构 Docker 镜像到 GHCR。

发布新版本：

```bash
git tag v1.0.0 && git push origin v1.0.0
```

本地构建（与 CI 一致）：

```bash
cd backend && CGO_ENABLED=1 go build -ldflags="-s -w" -o mailserver .
cd dns     && CGO_ENABLED=0 go build -ldflags="-s -w" -o nsd .
cd frontend && pnpm install --frozen-lockfile && pnpm run build
```
