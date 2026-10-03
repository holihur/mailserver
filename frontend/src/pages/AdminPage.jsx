import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import {
  ShieldCheck, Plus, Trash2, RefreshCw, Cloud, KeyRound,
  CheckCircle2, XCircle, AlertCircle, Loader2, Globe,
} from 'lucide-react'

const PROVIDERS = [
  { v: 'aliyun', label: '阿里云 DNS', hint: 'RAM 用户 AccessKey，需 AliyunDNSFullAccess 权限' },
  { v: 'cloudflare', label: 'Cloudflare', hint: '推荐 API Token（Zone.DNS 编辑权限）；也可用 Global API Key + 邮箱' },
]

const emptyForm = { name: '', type: 'aliyun', access_key_id: '', access_key_secret: '', api_token: '', email: '', api_key: '' }

export default function AdminPage() {
  const [me, setMe] = useState(null)
  const [overview, setOverview] = useState(null)
  const [providers, setProviders] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  async function load() {
    try { setProviders(await api.providers()) } catch (e) { setErr(e.message) }
    try { setOverview(await api.adminOverview()) } catch {}
  }
  useEffect(() => {
    api.me().then(m => { setMe(m); if (m.admin) load() }).catch(() => { location.hash = '#/login' })
  }, [])

  async function addProvider(e) {
    e.preventDefault()
    setErr(''); setBusy(true)
    try {
      await api.providerCreate(form)
      setForm(emptyForm)
      await load()
    } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }

  if (me && !me.admin) {
    return (
      <div className="min-h-screen bg-background">
        <header className="border-b border-border px-4 h-14 flex items-center gap-3">
          <a href="#/" className="font-semibold">← Mailserver</a>
          <Badge><ShieldCheck size={12} /> 管理后台</Badge>
        </header>
        <div className="max-w-xl mx-auto p-8 text-sm text-muted-foreground">
          需要管理员权限。可在 <code>.env</code> 的 <code>ADMIN_EMAILS</code> 中加入当前邮箱后重登。
        </div>
      </div>
    )
  }

  const p = PROVIDERS.find(x => x.v === form.type)

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3">
        <a href="#/" className="font-semibold">← Mailserver</a>
        <Badge><ShieldCheck size={12} /> 管理后台</Badge>
        <div className="flex-1" />
        <a href="#/dns"><Button variant="ghost" size="sm"><Globe />域名 DNS</Button></a>
        <Button variant="ghost" size="icon" onClick={load}><RefreshCw /></Button>
      </header>

      <div className="max-w-4xl mx-auto p-4 space-y-4">
        {overview && (
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
            {[
              ['用户', overview.users], ['域名', overview.domains], ['邮件', overview.mails],
              ['待发出', overview.pending], ['存储', fmtBytes(overview.storage_bytes)],
            ].map(([k, v]) => (
              <Card key={k} className="p-3">
                <div className="text-xs text-muted-foreground">{k}</div>
                <div className="text-xl font-semibold">{v}</div>
              </Card>
            ))}
          </div>
        )}

        <Card className="p-4">
          <div className="flex items-center gap-2 mb-3">
            <Cloud size={16} />
            <b className="text-sm">域名服务商（一键下发邮件解析）</b>
          </div>
          <p className="text-xs text-muted-foreground mb-3">
            录入凭证后，可列出账号下的域名，选择域名即自动配齐 <code>A / MX / SPF / DKIM / DMARC</code>，无需手动逐条添加。
            凭证使用 <code>JWT_SECRET</code> 派生的密钥加密存储。
          </p>

          <div className="space-y-3">
            {providers.map(pr => <ProviderCard key={pr.id} provider={pr} onChanged={load} />)}
            {providers.length === 0 && <p className="text-sm text-muted-foreground">还没有接入服务商。</p>}
          </div>
        </Card>

        <Card className="p-4">
          <b className="text-sm">接入新的服务商</b>
          <form onSubmit={addProvider} className="mt-3 space-y-3">
            <div className="grid sm:grid-cols-2 gap-3">
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground">类型</span>
                <select className="h-9 w-full rounded-md border border-border bg-background text-sm px-2"
                  value={form.type} onChange={e => setForm({ ...form, type: e.target.value })}>
                  {PROVIDERS.map(x => <option key={x.v} value={x.v}>{x.label}</option>)}
                </select>
              </label>
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground">备注名（可选）</span>
                <Input placeholder={p?.label} value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
              </label>
            </div>

            <p className="text-xs text-muted-foreground flex items-center gap-1"><KeyRound size={12} />{p?.hint}</p>

            {form.type === 'aliyun' ? (
              <div className="grid sm:grid-cols-2 gap-3">
                <Input placeholder="AccessKey ID" value={form.access_key_id} onChange={e => setForm({ ...form, access_key_id: e.target.value })} required />
                <Input type="password" placeholder="AccessKey Secret" value={form.access_key_secret} onChange={e => setForm({ ...form, access_key_secret: e.target.value })} required />
              </div>
            ) : (
              <div className="space-y-3">
                <Input type="password" placeholder="API Token（推荐）" value={form.api_token} onChange={e => setForm({ ...form, api_token: e.target.value })} />
                <div className="grid sm:grid-cols-2 gap-3">
                  <Input placeholder="或 Global API Key 的邮箱" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} />
                  <Input type="password" placeholder="Global API Key" value={form.api_key} onChange={e => setForm({ ...form, api_key: e.target.value })} />
                </div>
              </div>
            )}

            {err && <p className="text-xs text-red-500 flex items-center gap-1"><AlertCircle size={12} />{err}</p>}

            <Button size="sm" disabled={busy}>
              {busy ? <Loader2 className="animate-spin" /> : <Plus />}
              {busy ? '校验并保存中…' : '校验并保存'}
            </Button>
          </form>
        </Card>
      </div>
    </div>
  )
}

function ProviderCard({ provider, onChanged }) {
  const [domains, setDomains] = useState(null)
  const [sel, setSel] = useState('')
  const [ip, setIp] = useState(looksLikeIP(location.hostname) ? location.hostname : '')
  const [mailHost, setMailHost] = useState('')
  const [results, setResults] = useState(null)
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')

  async function run(kind, fn) {
    setBusy(kind); setMsg(''); setResults(null)
    try { await fn() } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  const fetchDomains = () => run('domains', async () => {
    const ds = await api.providerDomains(provider.id)
    setDomains(ds)
    if (ds.length === 1) setSel(ds[0].Name)
    if (ds.length === 0) setMsg('账号下没有可管理的域名，请先在服务商处添加域名。')
  })

  const test = () => run('test', async () => { await api.providerTest(provider.id); setMsg('连接正常 ✓') })

  const apply = () => run('apply', async () => {
    if (!sel) { setMsg('请先选择域名'); return }
    const r = await api.providerApply(provider.id, { domain: sel, ip, mail_host: mailHost })
    setResults(r.results)
    setMsg(r.warning || `已处理 ${r.results.length} 条记录（域名 ${r.domain}）`)
  })

  return (
    <div className="border border-border rounded-lg p-3 space-y-3">
      <div className="flex items-center gap-2 flex-wrap">
        <Badge className="uppercase">{provider.type}</Badge>
        <b className="text-sm">{provider.name}</b>
        <div className="flex-1" />
        <Button variant="outline" size="sm" onClick={test} disabled={!!busy}>
          {busy === 'test' ? <Loader2 className="animate-spin" /> : <CheckCircle2 />}测试
        </Button>
        <Button variant="outline" size="sm" onClick={fetchDomains} disabled={!!busy}>
          {busy === 'domains' ? <Loader2 className="animate-spin" /> : <RefreshCw />}获取域名
        </Button>
        <Button variant="ghost" size="icon" onClick={async () => {
          if (confirm(`删除服务商「${provider.name}」？`)) { await api.providerDelete(provider.id); onChanged() }
        }}><Trash2 /></Button>
      </div>

      {msg && <p className="text-xs text-muted-foreground flex items-center gap-1">
        {msg.startsWith('连接正常') ? <CheckCircle2 size={12} className="text-green-600" /> : <AlertCircle size={12} />}{msg}
      </p>}

      {domains && (
        <div className="grid sm:grid-cols-[1fr_1fr_1fr_auto] gap-2 items-end bg-muted/40 rounded-md p-3">
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">域名</span>
            <select className="h-9 w-full rounded-md border border-border bg-background text-sm px-2"
              value={sel} onChange={e => setSel(e.target.value)}>
              <option value="">请选择…</option>
              {domains.map(d => <option key={d.ID || d.Name} value={d.Name}>{d.Name}</option>)}
            </select>
          </label>
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">服务器公网 IP</span>
            <Input placeholder="自动探测" value={ip} onChange={e => setIp(e.target.value)} />
          </label>
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">MX 主机（可选）</span>
            <Input placeholder="mail.<域名>" value={mailHost} onChange={e => setMailHost(e.target.value)} />
          </label>
          <Button size="sm" onClick={apply} disabled={!!busy || !sel}>
            {busy === 'apply' ? <Loader2 className="animate-spin" /> : <Cloud />}一键配置
          </Button>
        </div>
      )}

      {results && (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead><tr className="text-left text-muted-foreground"><th className="py-1">主机</th><th>类型</th><th>值</th><th>结果</th></tr></thead>
            <tbody>
              {results.map((r, i) => (
                <tr key={i} className="border-t border-border">
                  <td className="py-1 pr-2 font-mono">{r.name}</td>
                  <td className="pr-2"><Badge>{r.type}</Badge></td>
                  <td className="pr-2 font-mono break-all max-w-[280px]">{r.value}</td>
                  <td><ActionBadge action={r.action} error={r.error} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function ActionBadge({ action, error }) {
  const map = {
    created: ['新增', 'text-green-600'],
    updated: ['更新', 'text-blue-600'],
    unchanged: ['无变化', 'text-muted-foreground'],
    failed: ['失败', 'text-red-500'],
  }
  const [label, cls] = map[action] || [action, '']
  return (
    <span className={'inline-flex items-center gap-1 ' + cls} title={error || ''}>
      {action === 'failed' ? <XCircle size={12} /> : <CheckCircle2 size={12} />}{label}
      {error ? <span className="text-muted-foreground truncate max-w-[160px]">({error})</span> : null}
    </span>
  )
}

function looksLikeIP(s) {
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(s || '')
}

function fmtBytes(n) {
  if (!n) return '0'
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return n.toFixed(i ? 1 : 0) + ' ' + u[i]
}
