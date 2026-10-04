# Sweetcorn — 轻量全栈邮件系统

自托管邮件系统：**单个 Go 二进制**（API + SMTP/POP3/IMAP + JMAP + 权威 DNS + 内嵌网页），PostgreSQL + Redis。

## 特性

**邮件**
- 网页邮箱：收件箱 / 已发送 / 草稿 / 垃圾箱 / 已删除 + **自定义文件夹**；搜索（防抖）、排序、分页、批量操作、星标、已读未读、移动端滑动。
- **HTML 正文渲染**（服务端白名单清洗，远程图片默认拦截、经代理按需加载）、附件预览（图片 / PDF / 视频 / 音频）、**撤销发送**、草稿自动保存、回复/回复全部/转发、邮件签名。
- 多收件人（To/Cc/Bcc）+ 下拉选择联系人；通讯录（含备注）。

**协议**
- SMTP 收发、POP3 / IMAP 同步、**JMAP**（RFC 8620/8621 标准方法集）、**ManageSieve**（RFC 5804）、**MCP**（HTTP，AI 客户端）。
- 发信经中继 / 直连；**asynq(Redis) 异步发信队列**（重试 + 发信状态）。

**安全**
- 网页登录 **TOTP 两步验证**、**OIDC 单点登录**（严格校验 id_token：JWKS 验签 + iss/aud/exp/nonce）。
- **应用专用密码（PAT）** 用于 IMAP/POP3/SMTP/JMAP/ManageSieve/MCP，可限制**来源 CIDR**；密码 bcrypt、凭证 AES-GCM 加密、登录限流。

**过滤与路由**
- **CEL 收信规则**（用户级 / 整站）：进垃圾箱、移动文件夹、转发。
- **Sieve 脚本**（RFC 5228 子集）：fileinto / redirect / discard / keep。
- **别名 / 转发**（按地址或整域 catch-all）、**邮件路由**（按域名中继/直连/丢弃）。

**运维**
- 自托管权威 DNS；一键 Let's Encrypt（DNS-01）或手动证书，热生效；DKIM 签名；阿里云 / Cloudflare 一键下发解析。
- **系统健康告警**（内存 / 磁盘 / CPU 80% / 90% 分级预警）、**Prometheus `/metrics`**、**后台自动更新**（默认 10 分钟检查）。
- **每用户存储配额**、**GDPR**（数据导出 + 注销删除）、隐私政策页。
- 多语言、明暗主题、响应式、PWA、骨架屏、Toast 通知。

## 协议与端口

| 协议 | 端口 | 说明 |
|------|------|------|
| SMTP 入站 | `25 → 2525` | 收信 |
| SMTP 提交 | `587`(STARTTLS) / `465`(TLS) | 客户端发信 |
| POP3 | `110` / `995` | 取信 |
| IMAP | `143` / `993` | 同步 |
| ManageSieve | `4190` | Sieve 脚本管理 |
| JMAP / MCP | `HTTP /jmap` · `/mcp` | JSON 邮件协议 / AI 客户端 |
| Web + REST API | `8080` | 网页邮箱与接口 |
| 权威 DNS | `5353`（容器内） | 自托管解析 |
| Prometheus | `HTTP /metrics` | 指标采集 |

邮件客户端（IMAP/POP3/SMTP）使用「应用专用密码」(PAT)，**不能用网页登录密码**；详见 [`MAIL_CLIENTS.md`](./MAIL_CLIENTS.md)。

## 与竞品对比

> 大致对比，具体以各项目官方文档为准。

| 项目 | 技术栈 | 单二进制 | 网页邮箱 | 权威 DNS | 过滤 | JMAP | MCP | 监控 | 自动更新 |
|------|--------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **Sweetcorn** | Go | ✅ | ✅ | ✅ | ✅ CEL+Sieve | ✅ | ✅ | ✅ | ✅ |
| mailcow | Docker 套件 | ❌ | ✅ | ❌ | ✅ Sieve | ❌ | ❌ | ✅ | ⚠️ 脚本 |
| Mailu | Docker 套件 | ❌ | ✅ | ❌ | ✅ Sieve | ❌ | ❌ | 部分 | ⚠️ 脚本 |
| Mail-in-a-Box | Ubuntu 脚本 | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | 部分 | ⚠️ 脚本 |
| iRedMail | 脚本 | ❌ | ✅ | ❌ | ✅ Sieve | ❌ | ❌ | 部分 | ❌ |
| Stalwart | Rust | ✅ | ✅ | ❌ | ✅ Sieve | ✅ | ❌ | ✅ | ❌ |
| docker-mailserver | Docker | ❌ | ❌ | ❌ | ✅ Sieve | ❌ | ❌ | 部分 | ⚠️ 脚本 |

Sweetcorn 的取舍：**不搭 Postfix/Dovecot 套件**，用单进程 + 单二进制换取部署简单、资源占用低。

> **明确不支持日历**：本系统是纯邮件系统，不提供 CalDAV / iCalendar 同步，也不解析会议邀请；通讯录（CardDAV）同样仅站内管理。请搭配独立日历 / 联系人服务使用。

**领先点**：单二进制 / 内置权威 DNS / CEL 收信规则 / MCP / JMAP / GDPR / 后台自动更新。

### 明确不支持 / 后期规划

**明确不支持：**

| 能力 | 说明 |
|------|------|
| 日历（CalDAV / iCalendar） | 不含日历，不解析会议邀请；不提供 CalDAV 同步 |
| CardDAV 联系人同步 | 通讯录仅站内管理，暂不与客户端双向同步 |

**后期规划：**

| 能力 | 说明 | 状态 |
|------|------|------|
| ActiveSync (EAS) | 移动端原生同步（仅邮件，不含日历/联系人） | 后期 |
| 反垃圾 / 反病毒 | Rspamd / SpamAssassin + ClamAV | 后期 |
| 全文检索 | 索引式搜索（Solr / Xapian） | 后期 |
| LDAP SSO | 目录服务（OIDC 已支持） | 后期 |
| IMAP ACL / 共享邮箱 | 委派与共享 | 后期 |
| 会话聚合 | 邮件串视图 | 后期 |
| 备份 / 恢复 | 一键备份与恢复 | 后期 |
| JMAP Push | EventSource 推送（当前为保活/轮询） | 后期 |

## 快速开始

```bash
# Docker（推荐）：起 PostgreSQL + Redis + 后端 + 内置 DNS
docker compose up -d --build

# 本机开发（需 PostgreSQL + Redis）
cd backend && cp .env.example .env && go run .
cd frontend && pnpm install && pnpm run dev
```

## 管理后台

访问 `/admin`（首个注册用户自动为管理员，或 `ADMIN_EMAILS` / `admin=true`）。
邮件域名、DKIM、证书、域名服务商、用户与配额、收信规则、邮件路由、别名、OIDC、健康告警等都在这里配置，保存后热生效。

## 一键安装（二进制）

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | sudo bash
```

可选参数：`--mail-host` `--admin` `--database-url` `--version` `--no-dns`。
安装后服务为 `mailserver.service`，Web 界面在 `http://<服务器IP>/`。

## 更新

```bash
sudo /opt/mailserver/bin/mailserver update    # 拉取最新 Release 并重启
sudo /opt/mailserver/bin/mailserver version   # 查看当前版本
```

## 日志

应用日志**同时写入文件与 stdout**：文件用成熟库 `lumberjack` 按大小滚动，stdout 交给运行时收集。

- **文件**：默认 `DATA_DIR/logs/mailserver.log`；`LOG_MAX_MB=50`（单文件上限）、`LOG_MAX_BACKUPS=5`（保留份数）、`LOG_MAX_AGE_DAYS=30`（保留天数）、默认 gzip 压缩。`LOG_FILE=off` 可关闭文件输出，`LOG_FILE=/path` 可自定义路径。
- **Docker**：stdout 另由 compose 内置的 `json-file` 驱动轮转（`10m×3`）；文件写在 `maildata` 卷内持久化。查看 `docker compose logs -f api`。
- **systemd / 二进制**：stdout 由 journald 自带轮转，`journalctl -u mailserver -f`；如需限制总量，在 `/etc/systemd/journald.conf` 设 `SystemMaxUse=500M` 后 `systemctl restart systemd-journald`。

## 发布

推送 `v*` 标签触发 CI：GoReleaser 编译 Linux amd64/arm64 单二进制（内嵌前端 + DNS），并推送多架构 Docker 镜像到 GHCR。

```bash
git tag v0.14.0 && git push origin v0.14.0
```
