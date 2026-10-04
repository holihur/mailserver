# 邮件客户端接入与 SMTP / POP3 / IMAP 方案

## 用户接入（客户端怎么填）

> 安全要求：**邮件客户端不能使用网页登录密码**。IMAP / POP3 / SMTP 一律使用「应用专用密码」（PAT，形如 `mst_xxxxxxxx…`）。网页登录密码仅用于登录 Web 邮箱，泄露风险隔离。

### 四步接入

1. **登录 Web 邮箱** → 右上角齿轮「客户端配置」进入 `/setup`。
2. **生成应用专用密码**：在「应用专用密码（PAT）」卡片里填个备注（如 `iPhone 邮件`），点「生成」。
   - 生成后**只显示一次**，请立刻复制保存；关闭后无法再次查看。
   - 每个设备建议单独生成一个，便于单独吊销。
3. **在客户端填入以下参数**（账号统一填**完整邮箱**，密码填**刚生成的 PAT**）：

| 用途 | 服务器 | 端口 | 加密 | 账号 / 密码 |
|------|--------|------|------|-------------|
| 发件 SMTP | `mail.你的域` | 587 | STARTTLS | 完整邮箱 / **PAT** |
| 发件 SMTP（隐式 TLS） | `mail.你的域` | 465 | SSL/TLS | 完整邮箱 / **PAT** |
| 收信 POP3 | `mail.你的域` | 110 | 明文或 STLS | 完整邮箱 / **PAT** |
| 收信 POP3S | `mail.你的域` | 995 | SSL/TLS | 完整邮箱 / **PAT** |
| 同步 IMAP | `mail.你的域` | 143 | 明文或 STARTTLS | 完整邮箱 / **PAT** |
| 同步 IMAPS | `mail.你的域` | 993 | SSL/TLS | 完整邮箱 / **PAT** |
| 网页邮箱 | `你的域` | 443/80 | HTTPS | 网页登录密码 |

4. **发一封测试信**，在 `/setup` 页「发件状态」确认已发出。

### 常见客户端要点

- **Foxmail**：手动设置 → 收信选 IMAP，服务器 `mail.你的域:993`，SSL；发信 `mail.你的域:465`，SSL；密码填 PAT。
- **Outlook**：账户类型选 IMAP；「密码」填 PAT（不要填登录密码）。
- **Thunderbird**：IMAP `993` SSL、SMTP `465` SSL；认证方式「普通密码」，密码填 PAT。
- **iPhone / iPad**：设置 → 邮件 → 账户 → 其他 → 添加邮件账户；收发服务器同表，密码填 PAT。
- **Android（自带/Gmail App）**：选「其他/IMAP」，服务器同上，密码填 PAT。

### 注意事项

- 忘记/换设备：在 `/setup` 页吊销旧 PAT（点垃圾桶），再生成新的。吊销后该客户端会立刻掉线。
- 「最近使用」时间可用来判断某个 PAT 是否还在用。
- 若客户端报认证失败，先确认：账号是**完整邮箱**、密码是 **PAT**（以 `mst_` 开头），而不是网页登录密码。
- **不含日历**：本服务不提供日历（无 CalDAV / 不解析 iCalendar 邀请），客户端的日历功能不可用；通讯录也不与客户端同步（CardDAV 未提供）。请在手机 / 客户端里只添加「邮件」账户，不要添加 CalDAV 账户。

## 现状

云厂商（阿里/腾讯/华为/AWS）默认封 **25 出站**，部分连 25 入站也封。后果：

- **发不出**：直连对方 MX:25 必超时 → 本系统改走 **587 中继**发出，全程不碰 25
- **收不到直投**：公网投给你域名的信只能走你 MX:25 → 若入站也被封，必须用**转发**（见下）

## 架构

```
Foxmail/手机 --587 AUTH--> submit(存 sent) --20s 队列--> 本站:inbox直投 / 站外:中继:587发出
Foxmail/手机 --110/995------> pop3(读 inbox，DELE=进trash)
Foxmail/手机 --143/993------> imap(INBOX/Sent/Drafts/Trash 同步已读星标删除)
公网 --25--> MX(mail.你的域) --25入站若通--> :2525 入站SMTP(存 inbox)
公网 --25--> 中继商 --转发到 你IP:2525--> 入站SMTP（25入站被封时的方案）
```

## 端口表

| 端口 | 服务 | 客户端怎么填 |
|------|------|--------------|
| 587 | SMTP 提交 | 服务器 `mail.你的域`，STARTTLS，账号=完整邮箱，密码=**应用专用密码(PAT)** |
| 110 | POP3 | 同上，USER/PASS，密码=**PAT** |
| 143 | IMAP | 服务器 `mail.你的域`，LOGIN；993 需证书。INBOX/Sent/Drafts/Trash 双向同步，删信进 Trash | 同上，USER/PASS，密码=**PAT** |
| 465/995 | 隐式 TLS | 配好 `TLS_CERT/KEY` 后启用（compose 里还是注释，按需开） |
| 2525 | 入站 SMTP | 只给中继转发用，不给客户端 |
| 25 | 不用 | 被封，不监听也无妨 |

## 外发中继配置（必做，否则站外信发不出去）

`.env` / compose 环境：

```
SMTP_RELAY_HOST=smtpdm.aliyun.com   # 示例：阿里云邮件推送
SMTP_RELAY_PORT=587
SMTP_RELAY_USER=你的推送账号
SMTP_RELAY_PASS=密码
SMTP_RELAY_FROM=和账号一致的地址     # 部分中继强制 envelope-from=账号
```

队列每 20s 投递，最多试 8 次；状态在前端 `#/setup` 页可见（队列中/已发出/失败+原因）。

## 收信三选一

1. **25 入站通**（自有 IDC/解封）：MX 指 `mail.你的域`，防火墙把 25 转到容器 2525
2. **25 入站被封**：用中继商的“邮件转发/收信路由”转到 `你IP:2525`（ recomended，Cloudflare Email Routing 也行但会经 CF）
3. **纯内网/测试**：`swaks --to 本地用户 --server 你IP:2525` 直发，或 webmail 对发（本站互投秒到）

## 证书（465/995 + 587 STARTTLS）

```bash
# 有公网 80 且域名已解析到本机：用 caddy/acme 申请后把 pem 指给 TLS_CERT/KEY
# 或自签（客户端需手动信任）：
openssl req -x509 -newkey rsa:2048 -days 365 -nodes \
  -keyout privkey.pem -out fullchain.pem -subj "/CN=mail.你的域"
```

无证书时 587/110 明文可用（仅可信网络），有证书后明文 AUTH 会被要求先 STARTTLS。

## DKIM 自签名（三步）

```bash
openssl genrsa -out dkim.pem 2048 && chmod 600 dkim.pem
# 把 dkim.pem 放到数据卷（如 /data/dkim.pem），compose 加 DKIM_KEY=/data/dkim.pem，重启 api
```

1. 前端 `#/setup` 页 DKIM 卡会显示 `dkim._domainkey.你的域 TXT "v=DKIM1..."`，点**一键发布**写入自家 DNS（长 TXT 自动多段，无需手动）
2. 验证：`dig dkim._domainkey.你的域 TXT` 能查到；发一封到 `check-auth@verifier.port25.com` 或 Gmail 看 `Authentication-Results: dkim=pass`
3. 只有**发件域 == DKIM_DOMAIN** 才签名；经中继发出时若中继也签，会形成双签名（正常，收件方任一 pass 即可）

## 排障

```bash
telnet 你IP 587            # 应见 220
openssl s_client -starttls smtp -connect 你IP:587   # 证书链
telnet 你IP 110            # 应见 +OK
# 发信卡住先看 #/setup 页 relay_err，再看 api 日志 queue: mail N fail…
```

IMAP 已支持（SELECT/FETCH/STORE/SEARCH/COPY/APPEND/IDLE），UID 取 Mail.ID。UIDVALIDITY 固定为 1：正常使用无影响，若手动动过数据库导致 ID 复用，客户端重建账户即可。

## 运维 / 安全说明

- 邮件协议（IMAP/POP3/SMTP）只接受 PAT，**不接受网页登录密码**：即使登录密码泄露，也无法直接用于收发信。
- PAT 明文只在创建时返回一次，数据库仅保存 `sha256` 摘要；忘记只能吊销重建。
- PAT 与账号绑定，客户端填写的用户名必须与令牌归属一致。
- 相关接口：`GET/POST /api/tokens`（列出/生成）、`DELETE /api/tokens/{id}`（吊销），均只作用于当前登录用户。
