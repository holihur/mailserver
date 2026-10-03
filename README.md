# Mailserver — 轻量全栈邮件系统

前端 React + TailwindCSS + shadcn 风格 · 后端 Go + GORM · PostgreSQL

> 纯 Go 构建（无 CGO），前端已内嵌，后端为单二进制；数据库使用 PostgreSQL。
> **几乎所有配置都在管理后台 `/#/admin` 里改**，命令行只需引导项（数据库、JWT_SECRET）。

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
│   │   ├── handler/{auth,mail,dns,admin,admin_provider,admin_settings}.go
│   │   ├── provider/           # 阿里云 / Cloudflare DNS 下发
│   │   ├── certstore/          # TLS 证书热替换
│   │   ├── letsencrypt/        # ACME DNS-01 自动签发
│   │   ├── runtimecfg/ secret/ # 后台可改配置 / 凭证加密
│   │   ├── smtp/ pop3/ imap/ queue/ dkim/
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

## 快速启动

```bash
# 一键（推荐）：自动起 PostgreSQL + 后端 + DNS + 前端
docker compose up -d --build

# 本机开发：
#  - 需要 PostgreSQL（Docker 起一个即可）
cd backend
cp .env.example .env
#   DATABASE_URL=postgres://user:pass@localhost:5432/mailserver?sslmode=disable
go mod tidy          # 仅首次，需网络
go run .             # 或 go build -ldflags="-s -w" mailserver && ./mailserver

# 前端 dev
cd frontend
pnpm install
pnpm run dev
```

API 默认 `:8080`，SMTP 入站 `:2525`，前端 dev `:5173`。

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

## 管理后台（所有配置都在这里改）

访问 `/#/admin`（仅管理员可见；首个注册用户自动为管理员，或 `ADMIN_EMAILS` / `admin=true`）。

| 页面 | 能做什么 |
|------|----------|
| **概览** | 用户 / 域名 / 邮件 / 待发出 / 存储量 + 新手配置清单 |
| **邮件主机** | 邮件域名、服务器公网 IP、管理员邮箱；DKIM 一键生成/导入；外发中继（高级） |
| **SSL 证书** | 一键申请 **Let's Encrypt**（DNS-01，无需 80/443）或手动上传证书；到期自动续期 |
| **域名服务商** | 接入 **阿里云 DNS** / **Cloudflare**，选域名一键下发 `A/MX/SPF/DKIM/DMARC` |
| **用户账号** | 创建/禁用/删除邮箱账号、重置密码 |

说明：
- 大部分改动**保存后立即生效**（邮件域名、DKIM、证书、服务商均热生效），无需重启。
- 证书自动申请通过已接入的域名服务商写入 `_acme-challenge` TXT 完成验证，需要先在「域名服务商」接入一个账号。
- 服务商凭证使用 `JWT_SECRET` 派生的密钥 AES-GCM 加密存储；更换 `JWT_SECRET` 后需重新录入。

对应接口（均需管理员）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/PATCH | /api/admin/settings | 邮件域名 / 公网 IP / 管理员 / 中继 |
| GET/DELETE | /api/admin/tls | 证书状态 / 删除证书 |
| POST | /api/admin/tls/manual | 手动上传证书 `{cert,key}` |
| POST | /api/admin/tls/acme | 一键申请 `{domain,email,provider_id,auto_renew}` |
| POST | /api/admin/dkim/generate | 生成 DKIM 密钥 `{domain,selector}` |
| POST | /api/admin/dkim/upload | 导入 DKIM 私钥 |
| GET/POST | /api/admin/providers | 服务商列表 / 新增（新增时校验凭证） |
| DELETE | /api/admin/providers/:id | 删除 |
| POST | /api/admin/providers/:id/test | 测试连接 |
| GET | /api/admin/providers/:id/domains | 列出账号下域名 |
| POST | /api/admin/providers/:id/apply | 一键配齐解析 `{domain,ip,mail_host}` |
| GET/POST | /api/admin/users | 用户列表 / 创建 |
| PATCH/DELETE | /api/admin/users/:id | 改密 / 禁用 / 删除 |

## 一键安装（二进制）

无需编译、无需 Docker：自动下载预编译的单二进制（已内嵌网页），用 systemd 运行。
数据库使用 PostgreSQL；未提供 `--database-url` 时，脚本会尝试在本机自动安装并初始化 PostgreSQL。

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | sudo bash
```

指定域名 / 管理员 / 已有数据库：

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | sudo bash -s -- \
  --mail-host mail.example.com --admin admin@example.com \
  --database-url "postgres://user:pass@127.0.0.1:5432/mailserver?sslmode=disable"
```

常用可选参数（一般无需填写）：

| 选项 | 说明 |
|------|------|
| `--mail-host HOST` | 邮件域名，如 `mail.example.com` |
| `--admin EMAILS` | 管理员邮箱，逗号分隔 |
| `--database-url DSN` | 使用已有 PostgreSQL；不填则本机自动安装 |
| `--version TAG` | 指定版本（默认 latest） |
| `--no-dns` | 不安装内置 DNS |

安装后服务为 `mailserver.service`（配置在 `/etc/mailserver/mailserver.env`），Web 界面位于 `http://<服务器IP>/`。

## CI / 发布

- **CI**（`.github/workflows/ci.yml`）：push / PR 到 `main` 时校验 backend、dns（`go vet` + `build` + `test`）与 frontend（`pnpm install --frozen-lockfile` + `build`）。
- **Release**（`.github/workflows/release.yml`）：推送 `v*` 标签时自动：
  1. 用 GoReleaser 编译 Linux `amd64`/`arm64` 的单二进制（内嵌前端）与 DNS 二进制，附 `checksums.txt` 并创建 GitHub Release；
  2. 构建并推送多架构 Docker 镜像到 GHCR。

发布新版本：

```bash
git tag v0.2.1 && git push origin v0.2.1
```

本地构建（与 CI 一致，纯 Go 无需 CGO）：

```bash
cd frontend && pnpm install --frozen-lockfile && pnpm run build
rm -rf ../backend/web && mkdir -p ../backend/web && cp -r dist/. ../backend/web/
cd ../backend && CGO_ENABLED=0 go build -ldflags="-s -w" -o mailserver .
cd ../dns   && CGO_ENABLED=0 go build -ldflags="-s -w" -o nsd .
```
