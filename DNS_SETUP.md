# 自托管域名上线步骤（不用任何第三方 DNS 托管）

权威 DNS 已内置在**同一个 `mailserver` 二进制/进程**里（不再单独部署 `nsd`）：
backend 在管理后台增删解析记录 → 导出 `zones.json` → 内置 DNS 每 10s 热加载。
设置 `DNS_ADDR=off` 可关闭内置 DNS（只当普通邮件服务器用）。

## 0. 准备

- 一台有公网 IP 的云服务器 `S`（记下 IP，如 `1.2.3.4`），你的域名如 `example.com`
- 本机 53 端口未被占用；云安全组放行 `53/udp`、`53/tcp`（DNS），mail 另需 `25/587/993`

## 1. 启动

```bash
# 一键二进制（默认启用内置 DNS，监听 :53）
curl -fsSL https://raw.githubusercontent.com/holihur/mailserver/main/install.sh | sudo bash

# 或 Docker（内置 DNS 在容器内 5353，宿主机映射 53）
docker compose up -d --build
```

然后在管理后台 `/#/dns` 添加域名 `example.com`，IP 填 `1.2.3.4` → 自动生成 NS/A/MX/SPF/DMARC/DKIM。

> 端口：`HTTP_PORT`（Web，默认 80）、内置 DNS 默认 `:53`（Docker 内为 `:5353`）。

## 2. 注册商处设置（关键两步）

1. **Glue 记录**：`ns1.example.com → 1.2.3.4`
   - 阿里云/腾讯云/DNSPod/Namecheap/Cloudflare Registrar 都有 "自定义 DNS Host / Glue / Child Nameserver" 入口
2. **域名的 NS** 改为：`ns1.example.com`（只填这一个即可，以后可加 ns2）

## 3. 验证（本地教研室/家里都行）

```bash
dig @1.2.3.4 example.com NS      # 应返回 ns1.example.com.
dig @1.2.3.4 example.com MX      # 应返回 10 mail.example.com.
dig @1.2.3.4 mail.example.com A  # 应返回 1.2.3.4
# NS 生效后（最长 48h，一般 1h 内）：
dig example.com MX               # 不加 @，走公网递归，应同样返回
```

## 4. 邮件相关的收尾

| 项目 | 做法 |
|------|------|
| PTR 反向解析 | 云厂商控制台把 `1.2.3.4` 的 PTR 设为 `mail.example.com`（收件方反垃圾用） |
| DKIM | 管理后台「邮件主机」一键生成密钥，公钥自动进 `dkim._domainkey` |
| SPF/DMARC | 已自动生成，`dig TXT` 能查到即可 |
| SSL | 管理后台「SSL 证书」一键申请 Let's Encrypt（或手动上传） |
| 25 端口 | 很多云默认封 25，需工单解封；解封前只能收不能向外发，可先用中继（管理后台「发件中继」）过渡 |

## 5. 低内存说明

- 单个 `mailserver` 进程常驻约 20~35MB（含 API、SMTP/POP3/IMAP、权威 DNS、内嵌前端）
- 不做公网递归（收到非托管域直接 REFUSED），既省内存又防 DNS 放大攻击
- 记录全放 PostgreSQL，由 backend 统一导出 `zones.json`，内置 DNS 只读文件
