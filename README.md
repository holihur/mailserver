# Sweetcorn — 轻量全栈邮件系统

自托管邮件系统：单个 Go 二进制（API + SMTP/POP3/IMAP + 权威 DNS + 内嵌网页），PostgreSQL + Redis。

## 特性

- **网页邮箱**：收件箱 / 已发送 / 草稿 / 垃圾箱 / 已删除 + 自定义文件夹；搜索、排序、分页、批量操作、星标、已读未读、移动端滑动。
- **收发信**：入站 SMTP；发件经中继或直连；asynq(Redis) 异步发信队列（重试 + 发信状态）；支持附件、Cc/Bcc 多收件人。
- **安全**：登录支持 TOTP 两步验证；IMAP/POP3/SMTP 与 MCP 使用「应用专用密码」(PAT)，可限制来源 CIDR；密码 bcrypt、凭证 AES-GCM 加密、登录限流。
- **收信规则（CEL）**：用户级 / 整站规则，命中后进垃圾箱、移动到文件夹或转发到指定邮箱。
- **别名 / 转发**：按地址或整域 catch-all 投递到多个目标（本地收件箱或外部转发）。
- **邮件路由**：按收件人域名指定中继 / 直连 / 丢弃。
- **第三方账号**：绑定其他邮箱（IMAP 收信 + SMTP 发信），可直接以其身份发信。
- **联系人**：通讯录（含备注），写信时下拉选择。
- **MCP（HTTP）**：通过 PAT 授权，供 AI 客户端操作邮箱。
- **自托管 DNS + TLS**：内置权威 DNS；一键申请 Let's Encrypt（DNS-01）或手动上传证书，热生效；DKIM 签名。
- **域名服务商**：阿里云 DNS / Cloudflare 一键下发解析。
- **GDPR**：一键导出个人数据、注销并删除全部数据；隐私政策页。
- **体验**：多语言、明暗主题、响应式、PWA、骨架屏、Toast 通知、撤销发送、草稿自动保存、后台自动更新。

## 与竞品对比

> 大致对比，具体以各项目官方文档为准。

| 项目 | 技术栈 | 单二进制 | 内置网页邮箱 | 内置权威 DNS | 收信规则 | MCP | GDPR 工具 | 自动更新 |
|------|--------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **Sweetcorn** | Go | ✅ | ✅ | ✅ | ✅ CEL | ✅ | ✅ | ✅ |
| mailcow | Docker 套件（Postfix/Dovecot/SOGo） | ❌ | ✅ | ❌ | ✅ Sieve | ❌ | 部分 | ⚠️ 脚本 |
| Mailu | Docker 套件（Postfix/Dovecot） | ❌ | ✅ | ❌ | ✅ Sieve | ❌ | 部分 | ⚠️ 脚本 |
| Mail-in-a-Box | Ubuntu 脚本 | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ⚠️ 脚本 |
| iRedMail | 脚本（Postfix/Dovecot） | ❌ | ✅ | ❌ | ✅ Sieve | ❌ | ❌ | ❌ |
| Stalwart | Rust | ✅ | ✅ | ❌ | ✅ Sieve | ❌ | 部分 | ❌ |
| docker-mailserver | Docker（Postfix/Dovecot） | ❌ | ❌ | ❌ | ✅ Sieve | ❌ | ❌ | ⚠️ 脚本 |

Sweetcorn 的取舍：**不搭 Postfix/Dovecot 套件**，用单进程 + 单二进制换取部署简单、资源占用低，并内置网页邮箱、权威 DNS 与一键自动更新，覆盖自托管个人 / 小团队邮箱的核心需求。

我们领先的地方：单二进制 / 内置权威 DNS / **CEL 收信规则** / **MCP（AI 客户端）** / **GDPR 数据导出与删除** / 后台自动更新。

### 我们暂缺（后期补齐）

以下为竞品已有、Sweetcorn 尚未实现或仍在完善的能力，按优先级逐步补齐：

| 能力 | 说明 | 竞品参考 | 状态 |
|------|------|----------|------|
| Sieve / ManageSieve | 服务端过滤脚本（当前为常用子集） | mailcow / Mailu / iRedMail | ✅ 已支持子集，持续完善 |
| JMAP | 现代 JSON 邮件协议 | Stalwart | 后期 |
| CalDAV / CardDAV | 日历与联系人同步 | SOGo(mailcow)、Radicale(iRedMail) | 后期 |
| ActiveSync (EAS) | 移动端原生邮件 / 日历 / 联系人 | Z-Push、SOGo | 后期 |
| 反垃圾 / 反病毒 | Rspamd / SpamAssassin + ClamAV | mailcow / Mailu / iRedMail | 后期 |
| 全文检索 | 索引式搜索（Solr / Xapian） | mailcow(Solr)、Mailu(Xapian) | 后期 |
| 每用户配额 | 存储配额与告警 | mailcow / Mailu | ✅ 已支持（入站拒绝 + 用量展示） |
| LDAP / OIDC SSO | 目录服务与单点登录 | Mailu / iRedMail / Stalwart | ✅ OIDC 已支持，LDAP 后期 |
| IMAP ACL / 共享邮箱 | 委派与共享 | Dovecot 系 | 后期 |
| 会话聚合 / HTML 渲染 | 邮件串、HTML 正文与远程图片拦截 | 主流 Webmail | ✅ HTML 渲染已支持（清洗 + 远程图片代理），会话聚合后期 |
| 监控 / 指标 | Prometheus / Grafana、审计日志 | mailcow | 后期 |
| 备份 / 恢复 | 一键备份与恢复 | mailcow | 后期 |

## 快速开始

```bash
# Docker（推荐）：起 PostgreSQL + Redis + 后端 + 内置 DNS
docker compose up -d --build

# 本机开发（需 PostgreSQL + Redis）
cd backend && cp .env.example .env && go run .
cd frontend && pnpm install && pnpm run dev
```

API `:8080`，SMTP 入站 `:2525`，前端 dev `:5173`。

## 管理后台

访问 `/admin`（首个注册用户自动为管理员，或 `ADMIN_EMAILS` / `admin=true`）。
邮件域名、DKIM、证书、域名服务商、用户、收信规则、邮件路由、别名等都在这里配置，保存后热生效。

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

## 邮件客户端

IMAP / POP3 / SMTP 使用「应用专用密码」(PAT)，**不能使用网页登录密码**；在 Web 邮箱「客户端配置」页生成。
详见 [`MAIL_CLIENTS.md`](./MAIL_CLIENTS.md)。

## 发布

推送 `v*` 标签会触发 CI 发布：GoReleaser 编译 Linux amd64/arm64 单二进制（内嵌前端 + DNS），并推送多架构 Docker 镜像到 GHCR。

```bash
git tag v0.9.1 && git push origin v0.9.1
```
