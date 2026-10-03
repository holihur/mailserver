# 自托管域名上线步骤（不用任何第三方 DNS 托管）

你的 DNS 就是 `mailserver/dns/` 这个自研权威服务器，`backend` 是控制面（增删改解析记录 → 导出 `zones.json` → dns 10s 内热加载）。

## 0. 准备

- 一台有公网 IP 的云服务器 `S`（记下 IP，如 `1.2.3.4`），你的域名如 `example.com`
- 本机 53 端口未被占用；云安全组放行 `53/udp`、`53/tcp`（DNS），mail 另需 `25/587/993`

## 1. 启动

```bash
cd mailserver
JWT_SECRET=$(openssl rand -hex 32) docker compose up -d --build
# 本页添加域名 example.com，IP 填 1.2.3.4 → 自动生成 NS/A/MX/SPF/DMARC/DKIM
```

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
| DKIM | 服务器上 `openssl genrsa 2048` 生成 key，公钥替换 `dkim._domainkey` 那条 TXT 的 `PASTE_PUBLIC_KEY_HERE`，私钥给 SMTP 发件签名用 |
| SPF/DMARC | 已自动生成，`dig TXT` 能查到即可 |
| 25 端口 | 很多云默认封 25，需工单解封；解封前只能收不能向外发，可先用中继（`.env` 的 `SMTP_RELAY_*`）过渡 |

## 5. 低内存说明

- `dns` 服务常驻约 8~15MB（单进程、无递归、无缓存堆积），`api` 约 20~30MB
- 不做公网递归（收到非托管域直接 REFUSED），既省内存又防 DNS 放大攻击连带
- 记录全放 SQLite，由 backend 统一导出文件，dns 只读文件，两个进程不争库
