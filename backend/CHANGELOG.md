# 更新日志 / Changelog

本文件面向使用者，记录 **Sweetcorn** 的重要变更，按版本倒序排列。
格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循语义化版本。

## [未发布] / Unreleased

### 新增
- **历史邮件导入**：头像菜单「导入邮件」支持 **mbox / EML**（上限 100MB），解析后写入当前文件夹。
- **规则影子模式**：CEL 规则可设「只匹配不执行」，安全灰度验证后再启用。
- **离线壳**：Service Worker 缓存应用壳与静态资源（导航 network-first、静态 stale-while-revalidate，API 不缓存），顶部显示离线提示。

### 变更
- 用户列表 / DNS 记录表在小屏（<sm）改为**卡片列表**，避免横向滚动。

## [v0.17.0] - 2026-10-05

### 新增
- **TOTP 二维码**：启用两步验证时展示 `otpauth://` 二维码（白底，深色模式也可扫）。
- **联系人导入**：支持导入 CSV（自动识别 email/name/note 列，跳过非法/重复）。
- **键盘快捷键**：列表 `j/k` 上下、`o/Enter` 打开、`u/Esc` 返回、`s` 星标、`e` 已读、`#` 删除、`r` 回复、`c` 写信、`/` 搜索、`?` 帮助浮层；写信 `Cmd/Ctrl+Enter` 发送。
- **导航与引导**：头像下拉按「邮箱/账户与安全/管理」分组，后台侧边栏分「概览/接入与证书/用户与邮件/运维」；后台配置清单增加进度条与「下一步」。
- **移动端**：底部悬浮写信按钮（拇指区 + 安全区）；列表新增发件人首字母头像、未读左侧主色条。
- **反馈与信任**：`confirmAsync` 支持 destructive 变体；投递失败在阅读区显示原因横幅、列表悬停提示；健康状态语义色。
- **批量增强**：批量栏新增「移动到…」文件夹，支持「全选搜索结果」（按当前文件夹 + 搜索条件作用于全部邮件，不止当前页）。
- **阅读路由化**：`/m/:id` 可直接打开、刷新与分享；移动端文件夹改为**底部 Tab**（拇指优先 + 安全区）、**PWA 安装引导**、列表滑动跟手反馈。
- **域名解析预检**：后台新增「域名解析检查」，实时校验托管域名的 MX / SPF / DKIM / DMARC。

### 修复
- **入站 DKIM 验签（Gmail 等）**：relaxed body 规范化漏了「行内连续空白压成单个空格」（RFC 6376 §3.4.4），导致含连续空格的 Gmail 邮件 body 哈希不匹配、`dkim=fail`；同时修正 simple 头部按原文（原大小写/折行）参与签名。新增 RFC 6376 示例与独立 relaxed/relaxed 端到端单测。

## [v0.16.0] - 2026-10-05

### 安全
- **自助注册限制邮箱后缀**：只允许**已托管域名**（`domains` 表）注册，与管理员建号规则一致；首个引导管理员不受限，同时校验邮箱格式（必须含 `@`）。OIDC 自动建号同样受此限制（失败跳转 `oidc_error=domain`）。
- **自助改密 + JWT 撤销**（#9）：新增 `POST /api/me/password`（验旧密码；新密码 ≥8 位 + 弱密码拦截）；`users.token_version` 写入 JWT，鉴权时比对，**改密 / 管理员重置密码 / 关闭 TOTP** 时自增，旧 access token 立即失效；注册与管理员建号同步改为 ≥8 位。「账户安全」页新增改密入口。
- **明文端口认证保护**（#3）：IMAP/POP3/ManageSieve 在**配了证书但未加密**时拒绝明文认证（IMAP 通告 `LOGINDISABLED`，POP3 要求先 `STLS`，ManageSieve 不通告 SASL）；STARTTLS 后重置会话状态。
- **协议层认证失败限流**（#4）：新增进程内 `authguard`（默认 10 分钟 30 次/IP），IMAP/POP3/SMTP 提交/ManageSieve 认证失败计数 + 0.5s 延迟 + 日志（便于 fail2ban），成功即清零，不依赖 Redis。
- **供应链加固**（#5）：`install.sh` 下载后校验 `checksums.txt` 的 sha256，失败即中止；GoReleaser 增加 cosign keyless 签名 + SBOM，Docker 构建开启 provenance/SBOM 证明；`selfupdate` 限制更新源为 `holihur/*`（`MAILSERVER_REPO_ALLOW_ANY=1` 可放开）。
- **入站发件人认证**（#7）：新增 `emailauth`（SPF / DKIM 验签 / DMARC 对齐），入站邮件计算并将结果写入 `mails.auth_results`，前端详情展示；`DMARC_ENFORCE=none|quarantine|reject` 可配置隔离/拒收（默认仅标记，避免误杀）。
- **管理员操作审计**（#10）：新增 `audit_logs` 表，`/api/admin/*` 写操作自动记录操作人/动作/路径/来源 IP，敏感字段脱敏；后台「审计日志」页可查（保留 180 天）。
- **图片代理 SSRF 加固**（#11）：连接时二次校验 IP（防 DNS rebinding）、禁止重定向、只允许公网可路由地址（含拒绝 CGNAT/保留段/IPv4-mapped）。
- **发信节流**（#12）：每用户每日/每分钟发信配额（`SEND_DAILY_LIMIT=500`、`SEND_PER_MINUTE=20`），超限拒绝入队并标记失败，覆盖网页/JMAP/MCP/SMTP 提交。
- **PAT 最小权限**（#18）：新建应用专用密码默认只勾 `imap,smtp`，显式全选才存全量（不再用空串代表全选，防止未来 scope 静默扩权）；旧空-scope token 标「全权限（旧）」。
- **/metrics 鉴权**（#19）：配 `METRICS_TOKEN` 则要求 Bearer，否则仅允许本机访问，不再公网裸奔。
- **进阶加固**：#4 补 **JMAP/MCP 的 HTTP 层 IP 限流**（Redis，120/分）；#5 自更新增加 **cosign 运行时验签**（`SELFUPDATE_REQUIRE_SIGNATURE=1` 强制）；#7 入站 SMTP 增加 **STARTTLS**；#9 增加**登录历史 / 新 IP 邮件提醒 / 管理员重置后强制改密 / 逐会话 jti 踢出 / 一键退出所有设备**；#12 **新账号 24h 降限额**、`[ALERT]` 日志、后台「今日已发」。

### 新增
- **附件 blob 落盘**（#6）：新增内容寻址 `internal/blob`（sha256 去重），入站附件改为落盘（DB 仅存元数据 + blob id），读写路径自动兼容旧 base64；提供 `mailserver migrate-blobs`（存量迁移）与每日 blob GC。**配额改为按附件解码后真实字节统计**（正文 + HTML + 附件 `size` 求和，blob 化不再少计），并新增 `mailserver fix-attachment-sizes` 回填存量（`migrate-blobs` 会一并执行）。
- **一键 / 定时备份恢复**（#13）：`mailserver backup [文件]` / `restore <文件>` 导出 DB 全表 JSON + `DATA_DIR`（含证书/blob）为 tar.gz；`BACKUP_DIR` 启用定时备份 + 保留份数 + `BACKUP_HOOK` 异地（如 rclone）。后台新增「备份」页：可调目录/间隔/保留份数、一键立即备份、列表（大小/时间/SHA256/加密状态）、下载/删除、二次确认恢复；备份整包用站内主密钥 **AES-256-GCM 流式加密**；`restore` 前**自动另存当前状态**（防恢复包损坏），并拒绝备份包内的路径穿越。
- **正文全文检索**（#14）：搜索覆盖 **正文**；启用 `pg_trgm` + GIN 索引加速 `ILIKE` 子串搜索（扩展不可用时自动退化）。新增统一检索包 `internal/mailsearch`：网页 `q=` 支持 **`from:` / `to:` / `subject:` / `body:` / `after:` / `before:` / `has:attachment`**（含引号值）；IMAP `SEARCH TEXT/BODY/...` 改为**数据库粗筛 + 精确匹配**并还原邮箱序号（不再受 500 封快照与内存全扫限制）；JMAP `Email/query` 的 `text/subject/from/to` 共用同一实现（大小写不敏感）。
- **投递可达性**（#17）：MTA-STS（`MTA_STS_MODE` 提供 `/.well-known/mta-sts.txt`）；后台可查 MTA-STS/TLS-RPT/DANE(TLSA) 建议记录；`DNSSEC_ENABLE=1` 启用**内置 DNS 完整签名**：KSK/ZSK + 正向应答 RRSIG + DNSKEY（KSK 自签）+ **否定应答（NXDOMAIN/NODATA，支持 NSEC，`DNSSEC_NSEC3=1` 切换为 NSEC3）**，启动日志给出 DS；DS/TLSA 等建议记录支持**后台一键写入 DNS**。新增**出站 DANE 验证**（`DANE_ENABLE=1`，`DANE_RESOLVER` 可指定会置 AD 的递归解析器）：直连对方 MX 时若其 `_25._tcp` TLSA 经 DNSSEC 验证通过，则**强制 STARTTLS 并校验证书（RFC 6698 / RFC 7672）**，不匹配或对端不支持 STARTTLS 则退回队列重试，**绝不降级明文**。
- **工程护栏**（#20）：Dependabot、golangci-lint（CI，仅新版问题）、GitHub **CodeQL**、前端 **vitest** 单测（CI 运行）。补齐：CI 加 **`govulncheck`**（依赖 CVE 门禁）、**`docker build`**（不推送，仅验证 Dockerfile + 内嵌前端）与 **`shellcheck install.sh`**；发布前 **Trivy 镜像扫描**（HIGH/CRITICAL 阻断）；前端新增 **ESLint**（`pnpm lint`）与核心文件（`api/client`、`lib/utils`）**覆盖率门**；新增 **`.github/ISSUE_TEMPLATE/`**（bug / 功能 / 安全三模板）、**`SECURITY.md`**（私密报告渠道 + 响应 SLA）与 **`CONTRIBUTING.md`**。
- **UX 速赢**（#38/#42/#45/#49/#51）：阅读区新增**发件人认证徽章**（SPF/DKIM/DMARC → 通过/未通过/未知，取代难懂的 `auth_results` 原文）；搜索框提示可用语法、支持 **Ctrl/⌘+K** 聚焦、无结果提供「清除搜索」；未读徽标超过 99 显示 **99+**；新增统一 **`EmptyState`** 组件并用于邮箱空态/无结果、联系人、Sieve、定时发送。
- **UX 第二批**（#38/#41/#42/#44）：新增 **success/warning/destructive 语义色 token**（浅/深色）并替换全站硬编码状态色；无障碍基础（Dropdown `aria-haspopup/expanded` + 键盘展开、导航 `aria-current`、Login `aria-label`、附件预览 `role=dialog`+Esc）；邮箱列表未读改**左侧主色条**、去 button 嵌套改 `role=button`+键盘可达、星标独立按钮加大点击区；`EmptyState` 推广到别名/路由/服务商/审计页。
- **写信与移动端**（#39/#43）：写信弹窗移动端改 **bottom-sheet**（顶部圆角、`max-h-92dvh`），底部操作栏固定 + `safe-bottom`，字段区独立滚动；草稿自动保存显示**「保存中…/已保存 HH:MM」**，关闭（遮罩/Esc/取消）时若有未保存内容**二次确认**；收件人即时**邮箱格式校验**（非法 chip 标红 + `aria-invalid` + 行内提示）；写信弹窗 Tab **焦点陷阱**；分页栏 `safe-bottom`。
- **CI 修复与工具链升级**：后端测试加 Redis 服务，且 `mailqueue` 配额测试在 Redis 不可达时正确 **skip**（此前 LLM 环境下因 `NewClient` 不探活而误判失败）；Go 升级到 **1.27**，依赖全面升级到最新稳定版（`x/net`、`x/crypto`、`golang-jwt/jwt/v5`、`go-redis/v9`、`pgx`、`asynq`、`gorm` 等），**`govulncheck` 0 漏洞**；golangci-lint 迁移到 **v2**（action v8 + v2 配置）；修复 Go 1.27 `go vet` 新报的非常量格式串问题。

### 修复
- **入站 SMTP 多收件人静默丢信**（#1）：`RCPT TO` 改为逐个收集并逐个投递，`MAIL FROM`/`RSET` 清空收件人，另补 DATA 段 `.` 透明传输；补单测。
- **邮件大小上限写死 1MB**（#2）：新增可配置 `MAX_MESSAGE_MB`（默认 25），入站 / 提交统一；超限仍回 `552 5.3.4` 但**保持协议同步**（丢弃剩余数据直到结束符），不再协议错位；补单测。
- **IMAP 假 IDLE + 缺扩展**（#8）：IDLE 改为阻塞等 DONE 的同时每 3s 推送 `EXISTS`（不再死等）；新增 `MOVE`/`UIDPLUS`（COPYUID/APPENDUID）/`NAMESPACE`/`ID`/`QUOTA`（GETQUOTA/GETQUOTAROOT）；连接加 30 分钟读超时；补单测。
- **零环路保护**（#15）：入站按 `Received` 跳数 >50 拒收；别名改为递归展开（visited + 深度上限），互指/自指不再滚雪球；补单测。
- **Sieve `reject` 空操作**（#16）：实现 `reject`/`ereject`（拒收不投递）；未知动作改为**编译期报错**（`CHECKSCRIPT`/`PUTSCRIPT` 返回 NO），不再静默吞掉；新增 `vacation` 自动回复（`VacationHook` + Redis 7 天去重，**批量/自动邮件不回复**）；ManageSieve 能力改为从引擎支持集生成；Sieve 页新增「外出自动回复」入口；补单测。
- **环路保护补全**（#15）：渲染/外发邮件带 `Delivered-To` 与 `Received` 头；转发时用 **SRS0** 重写信封发件人（`Mail.EnvelopeFrom`），并保留 `X-Original-From`。

## [v0.15.0] - 2026-10-04

### 新增
- **文件日志系统**：标准库 `log` + 成熟库 `lumberjack`，日志**同时写入文件与 stdout**；默认 `DATA_DIR/logs/mailserver.log`，按大小 / 份数 / 天数滚动并 gzip 压缩。可用 `LOG_FILE`（`off` 关闭）、`LOG_MAX_MB`（默认 50）、`LOG_MAX_BACKUPS`（5）、`LOG_MAX_AGE_DAYS`（30）、`LOG_COMPRESS` 调节。
- **定时 / 周期性发送**：写信时可设置发送时间与重复周期（不重复 / 每天 / 每周 / 每月），由发信队列到点自动发出；新增「定时发送」管理页，可暂停 / 恢复 / 取消。
- **PAT 授权范围**：应用专用密码可按协议限定权限（IMAP / POP3 / SMTP / JMAP / ManageSieve / MCP），创建时勾选，空=不限（兼容旧令牌）；访问未授权协议会被拒绝。

### 改进
- 网页邮箱：新邮件到达时**自动刷新当前列表**（收件箱第一页）并更新未读角标；**切回标签页 / 窗口重新聚焦时自动刷新**，不再需要手动点「刷新」。
- 邮件搜索支持**历史记录**：本地保存最近关键词，聚焦搜索框即展示，可点击复用或一键清除。
- 发信时（**点发送即**）把收件人（**To / Cc / Bcc**）**自动收录到通讯录**，投递时再兜底一次（网页 / JMAP / MCP / SMTP 提交均覆盖）；已存在的邮箱不覆盖，保留用户填写的姓名/备注。
- 「已发送」列表与详情**展示投递状态**（排队中 / 发送中 / 发送失败 + 失败原因），投递失败会主动 Toast 提醒。
- 写信时**自动聚焦收件人**输入框；批量操作支持**取消星标**。
- 列表空状态按上下文区分（收件箱 / 其他文件夹 / 无搜索结果），仅在空收件箱展示写信入口。
- 回复/转发引文与 Sieve 示例脚本改为**跟随界面语言**（原为固定中文）。
- 用**应用内输入弹窗**替换浏览器原生 `prompt`（新建/重命名文件夹、重置密码、设置配额、注销账号），移动端与无头环境更可靠、风格统一。
- 文档与网页「客户端配置」页**明确标注不支持日历**（无 CalDAV / 不解析会议邀请，通讯录仅站内管理）。
- **日志滚动**：Docker compose 内置 `json-file` 驱动（`max-size=10m` / `max-file=3`）自动轮转 stdout，systemd 显式输出到 journald（自带轮转）——与上面的文件日志互补，避免磁盘被日志撑满。

### 修复
- **自定义文件夹操作菜单（重命名/删除）不再被裁剪或遮挡**：菜单改用 portal 渲染到 `body` 并用 `fixed` 定位，修复移动端横向标签栏 `overflow` 裁剪及层叠上下文导致的显示问题。
- **「显示远程图片」无效**：CSP 的 `img-src` 增加 `blob:`，修复图片经 `fetch → blob:` 代理加载时被 CSP 拦截的问题。
- **邮件 HTML 渲染超出容器**：正文容器限宽并允许横向滚动，图片 / 表格 / 代码块限制到容器宽度，避免撑破布局。

## [v0.14.0] - 2026-10-04

### 新增
- **JMAP**（RFC 8620 / 8621 标准方法集）：Session、Mailbox/get|query|changes|set、Email/get|query|changes|set|copy|import、Thread、Identity、EmailSubmission、SearchSnippet，以及 Blob 上传/下载；PAT 鉴权。**单元测试覆盖率 100%**。
- **Prometheus 指标**：`GET /metrics`（CPU/内存/磁盘、用户/邮件/待发/未读、Goroutine/堆、构建信息）。
- **系统健康告警**：内存 / 磁盘 / CPU 按 **80% / 90%** 分级预警，管理后台概览页实时展示（20s 采样，30s 刷新）。

### 改进
- 自定义文件夹的「⋯」菜单改为**常显**，并支持**手机端**重命名 / 删除。
- 邮件列表批量按钮与搜索/排序**高度对齐**。
- README **重新整理**（特性分组、协议端口表、竞品对比、暂缺清单）。

## [v0.13.1] - 2026-10-04

### 改进
- 用户可在「客户端配置」页查看自己的存储用量与配额（未设配额显示「不限」）。
- 管理员在「用户账号」页可查看并**一键设置配额**（用户列表新增配额列 + 「设置配额」按钮）。

## [v0.13.0] - 2026-10-04

### 安全（严格策略）
- **OIDC 严格校验**：`id_token` 走 **JWKS 验签**（RS256/ES256 等）+ 校验 `iss` / `aud` / `exp` / `nonce`（授权请求带 nonce），不再仅依赖 userinfo。
- **HTML 严格白名单清洗**：只保留安全标签/属性，未知标签展开为文字，整体丢弃 script/style/iframe/form/svg 等；去除 on* 事件、style、危险 URL；注释节点移除。

### 性能
- **路由级代码分割**（React.lazy + Suspense）：首屏只加载 React/Router + 登录页，其余页面按需加载。
- **React 运行时独立 chunk**，利于长期缓存；`MailApp` 等页面各自成块。
- **服务端 gzip + 缓存头**：静态资源 gzip 压缩；`/assets/*` 使用 `immutable` 长缓存，HTML 不缓存。
- 搜索**防抖**（300ms）；列表项 `content-visibility` 跳过离屏渲染。

### 界面
- 邮件列表：发件人/时间移至**底部右侧**，主题与摘要更突出。

## [v0.12.0] - 2026-10-04

### 新增
- **OIDC 单点登录**：登录页「使用 SSO 登录」，支持发现/授权码/换 token/userinfo；后台可配 issuer、client_id、client_secret、首次登录自动建号。
- **HTML 邮件渲染**：解析并保存 HTML 正文，服务端安全清洗（移除脚本/事件/危险链接），远程图片默认拦截，点「显示远程图片」时经带 SSRF 防护的图片代理按需加载。
- **每用户配额**：后台可为用户设置配额（MB）；超限时入站 SMTP 在 RCPT 阶段返回 452 拒绝，投递时再兜底；客户端配置页展示用量进度条。

### 文档
- README 竞品对比：OIDC、HTML 渲染、每用户配额 标记为已支持。

## [v0.11.0] - 2026-10-04

### 新增
- **Sieve 支持**：内置 Sieve（RFC 5228 子集）解释器，收信时先执行用户的启用脚本（fileinto / redirect / discard / keep / addflag），支持 header / address / size / exists / allof / anyof / not 测试；未知 fileinto 目标自动创建文件夹。
- **ManageSieve（4190）**：Thunderbird 等客户端可管理脚本（LISTSCRIPTS / GETSCRIPT / PUTSCRIPT / SETACTIVE / DELETESCRIPT / CHECKSCRIPT / RENAME），用「应用专用密码」(PAT) 授权。
- **Sieve 管理页**：网页内编辑 / 语法检查 / 启用 / 删除脚本；接口 `/api/sieve`。
- 解释器 **100% 行覆盖率** + **Fuzz 测试**（`FuzzSieve`）。

### 文档
- README 竞品对比新增「我们暂缺（后期补齐）」清单（JMAP / CalDAV / ActiveSync / 反垃圾 / 全文检索 / 配额 / LDAP / 共享邮箱 / 监控 / 备份等）。

## [v0.10.0] - 2026-10-04

### 新增
- **MCP（HTTP）**：新增 `/mcp`（Streamable HTTP / JSON-RPC 2.0），用「应用专用密码」(PAT) 授权，提供 `list_mails` / `search_mails` / `get_mail` / `send_mail` / `mark_read` / `move_mail` / `delete_mail` / `list_folders` / `list_contacts` 等工具，供 AI 客户端操作邮箱。
- **PAT 来源 CIDR 限制**：生成/编辑应用专用密码时可限定允许的 IP 网段，IMAP/POP3/SMTP/MCP 通用。

### 文档
- README 精简：新增「特性」与「与竞品对比」列表，移除目录树与接口列表。

## [v0.9.0] - 2026-10-04

### 新增
- **撤销发送**：发送后 8 秒内可一键撤销（基于 asynq 延迟任务）。
- **自定义文件夹**：可新建/重命名/删除，收信规则可自动归并到指定文件夹。
- **已删除列表**：垃圾箱删除 → 「已删除」（可恢复或彻底删除）。
- **附件预览**：图片 / PDF / 视频 / 音频 直接在弹窗内预览，其余可下载。
- **邮件详情操作移至右上角**，并新增回复全部、转发。
- **回复增强**：引用原文、回复全部、邮件签名（可在个人资料设置）。
- **草稿自动保存**、**新邮件自动刷新与桌面通知**。
- **移动端滑动操作**：左滑删除、右滑已读。
- **正文链接化 + 验证码一键复制**。
- **未读角标**写入标题与 favicon；记住上次文件夹/排序；PWA 安装入口；空状态引导。

### 变更
- 全局用 **Toast + 确认弹窗** 替换原生 `alert/confirm`（31+15 处）。

### 后端
- 新增 `MailFolder`、`/api/folders`；`PATCH /api/mails/:id` 支持修改草稿字段；`POST /api/mails/:id/undo`；`PATCH /api/me` 修改昵称/签名。

## [v0.8.0] - 2026-10-04

### 新增
- **第三方邮箱账号**：用户可绑定其他邮箱（IMAP 收信 + SMTP 发信），在网页里直接收取，并可选择以其为发件身份发信。
- **asynq 异步发信**：发信改为 Redis(asynq) 队列异步处理，带重试与定时兑底；新增 queued/sending/sent/failed 状态，在「客户端配置」页实时展示。

### 变更
- 发件队列由定时轮询改为 asynq worker + 定时 sweep；以第三方账号身份发信时自动改用其 SMTP 配置。

## [v0.7.0] - 2026-10-04

### 新增
- **收件人别名 / 转发（管理员）**：把发给某地址（或整域 catch-all）的邮件投递到一个或多个目标：本地用户进收件箱，外部地址自动转发。
- **收信规则新增「转发」动作**：命中后转发到指定邮箱（可多个），并在收件箱保留副本。
- **已读 / 未读**：邮件详情可标记已读/未读；文件夹显示未读计数徽标。
- 邮件列表优化：未读高亮、未读圆点、发件人/主题层级、附件与时间显示。
- 登录后左上角显示品牌名称。

### 变更
- 语言 / 主题切换移至页面底部。

## [v0.6.0] - 2026-10-04

### 新增
- **GDPR 支持**：账户安全页可一键导出全部个人数据（JSON），以及注销账户并删除全部数据（被遗忘权）；新增「隐私政策 / GDPR」页面。
- **骨架屏**：邮件列表、联系人、规则、域名、管理概览等加载时显示骨架占位。

### 变更
- **统一展示风格**：次级页面（客户端配置、域名 DNS、联系人、收信规则、账户安全、隐私政策）共用统一页壳（返回 + 标题 + 语言/主题）。

## [v0.5.0] - 2026-10-04

### 新增
- **CEL 收信规则引擎**：基于 CEL 表达式匹配来信，命中后直接进垃圾箱或移动到指定文件夹；分「用户级规则」与「整站规则」（管理员），整站优先。
- **联系人（通讯录）**：新增 / 编辑（含备注）/ 删除 / 搜索；写信时收件人下拉可选择联系人与站内用户。
- **两步验证（TOTP）**：账户安全页可启用 / 关闭，登录需输入验证器动态码；管理员可重置。
- **邮件路由（管理员）**：按收件人域名指定外发方式（中继 / 直连 / 丢弃），优先于全局中继。
- 邮箱顶部账户下拉菜单；邮件详情「标星 / 回复 / 删除」改为下拉菜单。
- 收件人 / 抄送 / 密送支持多个，写信时支持下拉选择。

### 变更
- 登录支持两步验证挑战令牌（与访问令牌分离，防越权）。

## [v0.4.1] - 2026-10-04

### 修复
- 手机端也显示「客户端配置」入口，便于管理应用专用密码（PAT）。

## [v0.4.0] - 2026-10-04

### 安全
- **自托管 DNS 仅管理员可管理**：`/api/domains`、`/api/domains/{id}`、`/api/dkim` 现在要求管理员权限，修复了普通用户可越权查看/修改/删除解析记录的问题。
- **邮件客户端改用「应用专用密码」（PAT）**：IMAP / POP3 / SMTP 不再接受网页登录密码，降低登录密码泄露带来的收发信风险。

### 新增
- **应用专用密码（PAT）**：在「客户端配置」页可生成、查看与吊销，专供手机 / Outlook / Foxmail / Thunderbird 等客户端使用；明文仅显示一次，服务端只保存摘要。
- **管理后台「关于」页**：展示版本号、提交与构建时间，支持「检查更新 / 一键更新」，并内嵌本更新日志。
- **自动更新**：后台可开启自动安装新版本，检查间隔可调（默认 10 分钟）；关闭时仅检查并记录日志。
- 垃圾箱支持**彻底删除**与**一键清空**。
- 启用品牌名称 **Sweetcorn**。

### 修复
- 修复垃圾箱内点「删除」无效的问题（此前只是把邮件再次标记为垃圾箱，现在会真正删除）。
- 修复「客户端配置」页 IMAP 一行「加密」列误显示 DKIM 文案的问题，并补充 465 / 995 / 993 端口说明。

## [v0.3.14]

### 安全
- 密码使用 bcrypt 哈希存储；登录令牌（JWT）签名校验；登录/注册接口加入 Redis 限流。
- 入站邮件解析加固，补充安全响应头（CSP / X-Frame-Options 等）。

## [v0.3.13]
### 新增
- 邮件列表支持分页、搜索与排序，以及批量删除 / 星标。

## [v0.3.12]
### 修复
- 登录 / 登出改为整页跳转，避免前后端版本不一致或缓存导致登录后不跳转。

## [v0.3.11]
### 新增
- 新增「无中继时直连对方 MX:25」投递选项。

## [v0.3.10]
### 修复
- 二进制安装默认监听 25 入站（此前误用 2525，导致外部收信连不上）。

## [v0.3.9]
### 新增
- 入站邮件解析：解码 RFC2047 主题与 MIME 正文 / 附件。

## [v0.3.8]
### 新增
- 邮件支持抄送（Cc）、密送（Bcc）与附件。

## [v0.3.7]
### 修复
- 入站未知收件人改为在 RCPT 阶段拒绝；中继支持跳过 TLS 校验。

## [v0.3.6]
### 新增
- 管理后台顶部显示当前程序版本（含 commit / date 悬浮提示）。

## [v0.3.5]
### 修复
- 修复 `mailserver update` 校验失败的问题。

## [v0.3.4]
### 修复
- 修复服务商域名列表为空；中继支持 465 隐式 TLS，并加入一键测试。

## [v0.3.3]
### 变更
- 前端改用 History 路由；注册默认关闭，可在管理后台开启。

## [v0.3.2]
### 变更
- 前端迁移到 TypeScript；新增 `mailserver update` 命令。

## [v0.3.1]
### 新增
- 合并为单个二进制：API + SMTP/POP3/IMAP + 权威 DNS + 内嵌前端。

## [v0.3.0]
### 新增
- 前端多语言、明暗主题与响应式布局，全站覆盖与易用性优化。

## [v0.2.0]
### 变更
- 移除 CGO，去掉 SQLite，改为纯 Go + PostgreSQL。

## 更早
- 管理后台接入阿里云 DNS / Cloudflare，一键下发邮件解析。
- 新增 GitHub CI / Release 与一键安装脚本。
- 首个版本：轻量全栈邮件系统（Go 后端 + 权威 DNS + React 前端）。
