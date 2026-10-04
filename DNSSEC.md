# DNSSEC 启用与 DS 设置

Sweetcorn 内置权威 DNS，可对托管域名做**在线 DNSSEC 签名**。DNSSEC 的信任链需要
**父域（注册商/DNS 服务商）设置 DS 记录**指向本域名的 DNSKEY —— 这一步只能在注册商处完成。

> 与普通解析记录不同，**DS 记录不在本系统内**，本系统只负责生成并展示它，你需要复制到注册商。

## 1. 服务端启用

在 `/etc/mailserver/mailserver.env`（或 compose 环境）设置：

```
DNSSEC_ENABLE=1
# 可选：否定应答用 NSEC3（默认 NSEC）
DNSSEC_NSEC3=0
```

重启服务。密钥会自动生成并存到 `DATA_DIR/dnssec/`（`<域名>.ksk.{key,private}`、`<域名>.zsk.*`）。
⚠️ 该目录需**持久化备份**，丢失后 DS 会失效。

## 2. 获取 DS 记录

启用后，**管理后台 →「域名 DNS」→ 选中域名 → DNSSEC 卡片**会显示：

```
<域名>   <keyTag> 13 2 <digest>
```

一行就是一个完整的 DS 记录（`KeyTag Algorithm DigestType Digest`）。点右侧按钮**复制**。

> 启动日志也会打印一条：`DNSSEC 已启用；请到注册商设置 DS: ...`。
> 也可在服务器执行：`dig @127.0.0.1 -p 5353 <域名> DNSKEY +short`（本机 DNS 端口以实际为准）。

DS 记录参数：

| 字段 | 值 | 说明 |
|------|----|------|
| Key Tag | 例 `12345` | 由 KSK 算出 |
| Algorithm | `13` | ECDSA P-256 / SHA-256 |
| Digest Type | `2` | SHA-256 |
| Digest | 长十六进制 | KSK 的公钥摘要 |

## 3. 在注册商 / DNS 服务商设置

进入域名的 **DNSSEC / DS 记录**设置页，新增一条 DS，把上面四个值填进去：

- **阿里云**：域名 → DNS 修改 → DNSSEC 设置 → 添加 DS（填 Key Tag / 算法 13 / 摘要类型 2 / 摘要）。
- **Cloudflare**：DNS → Settings → DNSSEC → Enable，然后把本系统的 DS 值填入。
- **GoDaddy / Namecheap / 万网等**：域名管理 → DNSSEC → 添加 DS 记录。
- **注册商不支持 DS**：可把域名 NS 托管到支持 DS 的服务商后再设置。

保存后，父域会发布 DS，信任链建立。

## 4. 验证

```bash
# 应返回 DNSKEY（带 RRSIG）
dig +dnssec <域名> DNSKEY

# 检查 DS 是否已由父域发布
dig +short DS <域名>

# 从根开始验证链
dig +trace <域名> DNSKEY
```

也可用在线工具：**Verisign DNSSEC Debugger**、**dnsviz.net**、**dnssec-analyzer.verisignlabs.com**，
或 `delv` / `drill -S`。

## 5. 维护与回滚

- **更换密钥**：删除 `DATA_DIR/dnssec/<域名>.*` 后重启会生成新密钥，此时**必须同步更新注册商的 DS**，否则解析会校验失败。
- **临时回滚**：从注册商删除 DS 记录即回到“未签名”，解析恢复；本系统可保持签名（无 DS 时不受影响）。
- **备份**：`DATA_DIR/dnssec/` 已包含在 `mailserver backup` 的 tar 包内（`DATA_DIR` 整目录打包）。
- 别名/子域若由本机权威 DNS 托管，同样各自有 DNSKEY/DS。
