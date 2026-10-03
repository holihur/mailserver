import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { ShieldCheck, ShieldAlert, Loader2, Trash2, Wand2, Upload, CheckCircle2, AlertCircle, Clock } from 'lucide-react'

export default function AdminSSL() {
  const [tls, setTls] = useState(null)
  const [settings, setSettings] = useState(null)
  const [providers, setProviders] = useState([])
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')

  const [acme, setAcme] = useState({ domain: '', email: '', provider_id: 0, auto_renew: true, staging: false })
  const [manual, setManual] = useState({ cert: '', key: '' })
  const certFile = useRef(), keyFile = useRef()

  async function load() {
    try {
      const t = await api.tlsGet()
      setTls(t)
      setProviders(t.providers || [])
      if (t.acme) setAcme(a => ({ ...a, domain: t.acme.domain || a.domain, email: t.acme.email || a.email, provider_id: t.acme.provider_id || a.provider_id, auto_renew: t.acme.auto_renew, staging: false }))
      else if (t.providers?.length === 1) setAcme(a => ({ ...a, provider_id: t.providers[0].id }))
    } catch (e) { setMsg(e.message) }
    try {
      const s = await api.settingsGet()
      setSettings(s)
      setAcme(a => ({ ...a, domain: a.domain || s.mail_host || '' }))
    } catch {}
  }
  useEffect(() => { load() }, [])

  async function run(kind, fn) {
    setBusy(kind); setMsg('')
    try { await fn() } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  const issueAcme = () => run('acme', async () => {
    if (!acme.domain) { setMsg('请填写要签发证书的域名'); return }
    if (!acme.provider_id) { setMsg('请先在「域名服务商」接入一个服务商，再回来申请'); return }
    const r = await api.tlsAcme(acme)
    setTls(t => ({ ...t, cert: r.cert, acme: r.acme }))
    setMsg('证书已签发并生效')
  })

  const saveManual = () => run('manual', async () => {
    const r = await api.tlsManual(manual)
    setTls(t => ({ ...t, cert: r.cert }))
    setManual({ cert: '', key: '' })
    setMsg('证书已保存并生效')
  })

  const remove = () => run('del', async () => {
    await api.tlsDelete()
    setTls(t => ({ ...t, cert: {} }))
    setMsg('证书已删除')
  })

  async function readFile(e, field) {
    const f = e.target.files?.[0]
    if (!f) return
    const text = await f.text()
    setManual(m => ({ ...m, [field]: text }))
  }

  const cert = tls?.cert || {}
  const baseDomain = (settings?.mail_host || '').replace(/^[^.]+\./, '')

  return (
    <AdminShell title="SSL 证书" desc="证书用于加密邮件收发（STARTTLS / 465 / 993 / 995）。可以一键申请免费证书，也可以手动上传。">
      <Card className="p-4">
        <div className="flex items-center gap-2">
          {cert.exists ? <ShieldCheck className="text-green-600" size={18} /> : <ShieldAlert className="text-yellow-600" size={18} />}
          <b className="text-sm">{cert.exists ? '证书已配置' : '尚未配置证书（当前为明文连接）'}</b>
          <div className="flex-1" />
          {cert.exists && <Button variant="ghost" size="sm" onClick={remove} disabled={!!busy}><Trash2 />删除</Button>}
        </div>
        {cert.exists && (
          <div className="text-xs text-muted-foreground mt-2 space-y-0.5">
            <p>域名：{(cert.domains || []).join(', ')}</p>
            <p>颁发者：{cert.issuer} · 来源：{sourceLabel(cert.source)}</p>
            <p className="flex items-center gap-1"><Clock size={12} />有效期至 {fmt(cert.not_after)}</p>
          </div>
        )}
      </Card>

      <Card className="p-4 space-y-3">
        <div className="flex items-center gap-2"><Wand2 size={16} /><b className="text-sm">一键申请（Let's Encrypt，免费）</b></div>
        <p className="text-xs text-muted-foreground">
          通过 DNS 自动验证域名所有权，无需开放 80/443。需要先接入服务商（阿里云 / Cloudflare）。
        </p>
        <div className="grid sm:grid-cols-2 gap-3">
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">证书域名</span>
            <Input placeholder="mail.example.com" value={acme.domain} onChange={e => setAcme({ ...acme, domain: e.target.value })} />
          </label>
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">DNS 服务商（用于 DNS 验证）</span>
            <select className="h-9 w-full rounded-md border border-border bg-background text-sm px-2"
              value={acme.provider_id} onChange={e => setAcme({ ...acme, provider_id: +e.target.value })}>
              <option value={0}>请选择…</option>
              {providers.map(p => <option key={p.id} value={p.id}>{p.name}（{p.type}）</option>)}
            </select>
          </label>
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">通知邮箱（证书过期提醒）</span>
            <Input placeholder={`admin@${baseDomain || 'example.com'}`} value={acme.email} onChange={e => setAcme({ ...acme, email: e.target.value })} />
          </label>
          <label className="text-xs flex items-center gap-2 mt-5">
            <input type="checkbox" checked={acme.auto_renew} onChange={e => setAcme({ ...acme, auto_renew: e.target.checked })} />
            自动续期（到期前自动重新申请）
          </label>
        </div>
        <div className="flex items-center gap-3">
          <Button size="sm" onClick={issueAcme} disabled={!!busy}>
            {busy === 'acme' ? <Loader2 className="animate-spin" /> : <Wand2 />}{busy === 'acme' ? '申请中（约 1 分钟）…' : '申请证书'}
          </Button>
          <label className="text-xs text-muted-foreground flex items-center gap-1">
            <input type="checkbox" checked={acme.staging} onChange={e => setAcme({ ...acme, staging: e.target.checked })} />
            测试模式（staging，验证流程用）
          </label>
        </div>
      </Card>

      <Card className="p-4 space-y-3">
        <div className="flex items-center gap-2"><Upload size={16} /><b className="text-sm">手动上传证书</b></div>
        <p className="text-xs text-muted-foreground">适用于已有证书（如从其他服务商购买或自签）。</p>
        <div className="flex gap-2 text-xs">
          <Button variant="outline" size="sm" onClick={() => certFile.current?.click()}>选择证书文件</Button>
          <Button variant="outline" size="sm" onClick={() => keyFile.current?.click()}>选择私钥文件</Button>
          <input ref={certFile} type="file" hidden onChange={e => readFile(e, 'cert')} />
          <input ref={keyFile} type="file" hidden onChange={e => readFile(e, 'key')} />
        </div>
        <textarea className="w-full h-24 rounded-md border border-border bg-background p-2 text-xs font-mono"
          placeholder="-----BEGIN CERTIFICATE-----（证书链 fullchain.pem）" value={manual.cert} onChange={e => setManual({ ...manual, cert: e.target.value })} />
        <textarea className="w-full h-24 rounded-md border border-border bg-background p-2 text-xs font-mono"
          placeholder="-----BEGIN PRIVATE KEY-----（privkey.pem）" value={manual.key} onChange={e => setManual({ ...manual, key: e.target.value })} />
        <Button size="sm" onClick={saveManual} disabled={!!busy || !manual.cert || !manual.key}>
          {busy === 'manual' ? <Loader2 className="animate-spin" /> : <Upload />}保存证书
        </Button>
      </Card>

      {msg && <p className="text-sm flex items-center gap-1">
        {msg.includes('已') && !msg.includes('失败') && !msg.includes('不存在') ? <CheckCircle2 size={14} className="text-green-600" /> : <AlertCircle size={14} className="text-yellow-600" />}{msg}
      </p>}
    </AdminShell>
  )
}

function sourceLabel(s) {
  return { manual: '手动上传', acme: 'Let\'s Encrypt', env: '环境变量', file: '本地文件' }[s] || s || '—'
}
function fmt(t) {
  if (!t) return '—'
  const d = new Date(t)
  const days = Math.ceil((d - Date.now()) / 86400000)
  return `${d.toLocaleDateString()}（${days > 0 ? days + ' 天后到期' : '已过期'}）`
}
