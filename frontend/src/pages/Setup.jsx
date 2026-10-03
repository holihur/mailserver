import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Card, Badge } from '../components/ui/controls'
import { RefreshCw, CheckCircle2, Clock, XCircle } from 'lucide-react'

// 客户端（Foxmail/Outlook/Thunderbird/手机邮箱）配置 + 发件状态
export default function Setup() {
  const [outbox, setOutbox] = useState([])
  const [dkim, setDkim] = useState(null)
  const host = location.hostname

  async function load() {
    try { setOutbox(await api.outbox()) } catch {}
    try { setDkim(await api.dkimGet()) } catch {}
  }
  useEffect(() => { load() }, [])

  const rows = [
    ['发件 SMTP', `${host}:587`, 'STARTTLS（有证书时强制）/ 明文仅内网', 'AUTH 登录：完整邮箱 + 注册密码'],
    ['取信 POP3', `${host}:110`, '明文 + 可选 STLS；995 需先配证书', 'USER/PASS：完整邮箱 + 注册密码'],
    ['同步 IMAP', `${host}:143`, '已读/星标/删除双向同步；993 需先配证书', 'LOGIN：完整邮箱 + 注册密码'],
    ['Webmail', `${host}/#/`, '—', 'JWT 登录'],
  ]

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3">
        <a href="#/" className="font-semibold">← Mailserver</a>
        <Badge>客户端配置</Badge>
      </header>
      <div className="max-w-3xl mx-auto p-4 space-y-4">
        <Card className="p-4">
          <b className="text-sm">邮箱客户端参数（通用：Foxmail / Outlook / Thunderbird / iOS / 安卓）</b>
          <table className="w-full text-sm mt-2">
            <thead><tr className="text-left text-muted-foreground text-xs"><th className="py-1">用途</th><th>服务器</th><th>加密</th><th>账号</th></tr></thead>
            <tbody>
              {rows.map(r => (
                <tr key={r[0]} className="border-t border-border">
                  <td className="py-2 pr-2 font-medium">{r[0]}</td>
                  <td className="pr-2 font-mono text-xs">{r[1]}</td>
                  <td className="pr-2 text-xs text-muted-foreground">{r[2]}</td>
                  <td className="text-xs text-muted-foreground">{r[3]}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="text-xs text-muted-foreground mt-2">注意：POP3 取走后默认保留在服务器（本实现按“收取即保留”，客户端删才会进垃圾箱）；发件人必须与登录邮箱一致，否则 553 拒绝。</p>
        </Card>

        <Card className="p-4">
          <div className="flex items-center gap-2 mb-2">
            <b className="text-sm">发件状态（25 被封排障看这里）</b>
            <div className="flex-1" />
            <Button variant="outline" size="sm" onClick={load}><RefreshCw />刷新</Button>
          </div>
          {outbox.length === 0 && <p className="text-sm text-muted-foreground">还没有发过信。</p>}
          {outbox.map(m => (
            <div key={m.id} className="flex items-center gap-2 text-sm border-t border-border py-2">
              {m.relayed
                ? <span className="text-green-600 flex items-center gap-1 text-xs"><CheckCircle2 size={14} />已发出/已投递</span>
                : m.attempts >= 8
                  ? <span className="text-red-500 flex items-center gap-1 text-xs"><XCircle size={14} />失败（8次）</span>
                  : <span className="text-yellow-600 flex items-center gap-1 text-xs"><Clock size={14} />队列中({m.attempts})</span>}
              <span className="truncate flex-1">→ {m.to}《{m.subject || '(无主题)'}》</span>
            </div>
          ))}
          {outbox.some(m => !m.relayed && m.relay_err) && (
            <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 mt-2 whitespace-pre-wrap">{outbox.find(m => !m.relayed && m.relay_err)?.relay_err}</pre>
          )}
        </Card>

        <Card className="p-4">
          <b className="text-sm">DKIM 自签名 {dkim?.ready ? <Badge>已启用</Badge> : <Badge>未启用</Badge>}</b>
          {!dkim?.ready ? (
            <div className="text-sm text-muted-foreground mt-1 space-y-1">
              <p>{dkim?.hint || '后端未配置 DKIM_KEY，发出的信无 DKIM 签名（经中继发出时由中继签）。'}</p>
              <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto">openssl genrsa -out dkim.pem 2048 && chmod 600 dkim.pem
              {'# compose: DKIM_KEY=/data/dkim.pem，DKIM_DOMAIN=你的域名，重启 api'}             </pre>
            </div>
          ) : (
            <div className="text-sm mt-1 space-y-2">
              <p className="text-muted-foreground">选择器 <code>{dkim.selector}</code> · 域名 <code>{dkim.domain}</code> · 发件域一致时自动签名</p>
              <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto break-all whitespace-pre-wrap">{dkim.name}.{dkim.domain} TXT "{dkim.txt}"</pre>
              <Button size="sm" variant="outline" onClick={async () => { const r = await api.dkimPublish({ domain: dkim.domain }); alert('已写入 DNS：' + r.name); }}>一键发布公钥到自家 DNS</Button>
            </div>
          )}
        </Card>

        <Card className="p-4 text-sm text-muted-foreground space-y-1">
          <b className="text-foreground text-sm">为什么不用 25？</b>
          <p>云厂商普遍封 25 出站（防垃圾），且家宽/轻量云也常被收件方直接拒收。本系统：客户端→:587 提交→队列→中继 :587 发出，全程不碰 25。收信靠 MX 指到你服务器（需 25 入站）或中继转发到 :2525，详见 MAIL_CLIENTS.md。</p>
        </Card>
      </div>
    </div>
  )
}
