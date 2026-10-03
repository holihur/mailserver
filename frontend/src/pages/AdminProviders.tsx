import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Select } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import {
  Plus, Trash2, RefreshCw, Cloud, KeyRound,
  CheckCircle2, XCircle, AlertCircle, Loader2,
} from 'lucide-react'

const PROVIDERS = [
  { v: 'aliyun', label: 'Aliyun DNS', hintKey: 'providers.aliyunHint' },
  { v: 'cloudflare', label: 'Cloudflare', hintKey: 'providers.cfHint' },
]

const emptyForm = { name: '', type: 'aliyun', access_key_id: '', access_key_secret: '', api_token: '', email: '', api_key: '' }

export default function AdminProviders() {
  const { t } = useI18n()
  const [providers, setProviders] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  async function load() {
    try { setProviders(await api.providers()) } catch (e) { setErr(e.message) }
  }
  useEffect(() => { load() }, [])

  async function addProvider(e) {
    e.preventDefault()
    setErr(''); setBusy(true)
    try {
      await api.providerCreate(form)
      setForm(emptyForm)
      await load()
    } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }

  const p = PROVIDERS.find(x => x.v === form.type)

  return (
    <AdminShell title={t('providers.title')} desc={t('providers.desc')}>
      <Card className="p-4 space-y-3">
        <b className="text-sm">{t('providers.connected')}</b>
        {providers.map(pr => <ProviderCard key={pr.id} provider={pr} onChanged={load} />)}
        {providers.length === 0 && <p className="text-sm text-muted-foreground">{t('providers.none')}</p>}
      </Card>

      <Card className="p-4">
        <b className="text-sm">{t('providers.addNew')}</b>
        <form onSubmit={addProvider} className="mt-3 space-y-3">
          <div className="grid sm:grid-cols-2 gap-3">
            <label className="text-xs space-y-1">
              <span className="text-muted-foreground">{t('providers.kind')}</span>
              <Select value={form.type} onChange={e => setForm({ ...form, type: e.target.value })}>
                {PROVIDERS.map(x => <option key={x.v} value={x.v}>{x.label}</option>)}
              </Select>
            </label>
            <label className="text-xs space-y-1">
              <span className="text-muted-foreground">{t('providers.name')}</span>
              <Input placeholder={p?.label} value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </label>
          </div>

          <p className="text-xs text-muted-foreground flex items-start gap-1"><KeyRound size={13} className="mt-0.5 shrink-0" />{t(p?.hintKey)}</p>

          {form.type === 'aliyun' ? (
            <div className="grid sm:grid-cols-2 gap-3">
              <Input placeholder="AccessKey ID" value={form.access_key_id} onChange={e => setForm({ ...form, access_key_id: e.target.value })} required />
              <Input type="password" placeholder="AccessKey Secret" value={form.access_key_secret} onChange={e => setForm({ ...form, access_key_secret: e.target.value })} required />
            </div>
          ) : (
            <div className="space-y-3">
              <Input type="password" placeholder="API Token" value={form.api_token} onChange={e => setForm({ ...form, api_token: e.target.value })} />
              <div className="grid sm:grid-cols-2 gap-3">
                <Input placeholder="Email (Global API Key)" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} />
                <Input type="password" placeholder="Global API Key" value={form.api_key} onChange={e => setForm({ ...form, api_key: e.target.value })} />
              </div>
            </div>
          )}

          {err && <p className="text-xs text-red-500 flex items-center gap-1"><AlertCircle size={12} />{err}</p>}

          <Button size="sm" disabled={busy}>
            {busy ? <Loader2 className="animate-spin" /> : <Plus />}
            {busy ? t('providers.validating') : t('providers.validateSave')}
          </Button>
        </form>
      </Card>
    </AdminShell>
  )
}

function ProviderCard({ provider, onChanged }) {
  const { t } = useI18n()
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
    if (ds.length === 1) setSel(ds[0].name)
    if (ds.length === 0) setMsg(t('providers.noDomains'))
  })

  const test = () => run('test', async () => { await api.providerTest(provider.id); setMsg('OK ✓') })

  const apply = () => run('apply', async () => {
    if (!sel) return
    const r = await api.providerApply(provider.id, { domain: sel, ip, mail_host: mailHost })
    setResults(r.results)
    setMsg(r.warning || `${r.results.length}`)
  })

  return (
    <div className="border border-border rounded-lg p-3 space-y-3">
      <div className="flex items-center gap-2 flex-wrap">
        <Badge className="uppercase">{provider.type}</Badge>
        <b className="text-sm">{provider.name}</b>
        <div className="flex-1" />
        <Button variant="outline" size="sm" onClick={test} disabled={!!busy}>
          {busy === 'test' ? <Loader2 className="animate-spin" /> : <CheckCircle2 />}{t('common.test')}
        </Button>
        <Button variant="outline" size="sm" onClick={fetchDomains} disabled={!!busy}>
          {busy === 'domains' ? <Loader2 className="animate-spin" /> : <RefreshCw />}{t('providers.getDomains')}
        </Button>
        <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={async () => {
          if (confirm(`${t('common.delete')} ${provider.name}?`)) { await api.providerDelete(provider.id); onChanged() }
        }}><Trash2 /></Button>
      </div>

      {msg && <p className="text-xs text-muted-foreground flex items-center gap-1">
        {msg.endsWith('✓') ? <CheckCircle2 size={12} className="text-green-600" /> : <AlertCircle size={12} />}{msg}
      </p>}

      {domains && (
        <div className="grid sm:grid-cols-[1fr_1fr_1fr_auto] gap-2 items-end bg-muted/40 rounded-md p-3">
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">{t('providers.domain')}</span>
            <Select value={sel} onChange={e => setSel(e.target.value)}>
              <option value="">{t('providers.choose')}</option>
              {domains.map((d: any) => <option key={d.id || d.name} value={d.name}>{d.name}</option>)}
            </Select>
          </label>
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">{t('providers.publicIP')}</span>
            <Input placeholder="1.2.3.4" value={ip} onChange={e => setIp(e.target.value)} />
          </label>
          <label className="text-xs space-y-1">
            <span className="text-muted-foreground">{t('providers.mxHost')}</span>
            <Input placeholder="mail.<domain>" value={mailHost} onChange={e => setMailHost(e.target.value)} />
          </label>
          <Button size="sm" onClick={apply} disabled={!!busy || !sel}>
            {busy === 'apply' ? <Loader2 className="animate-spin" /> : <Cloud />}{t('providers.apply')}
          </Button>
        </div>
      )}

      {results && (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead><tr className="text-left text-muted-foreground"><th className="py-1">{t('dns.host')}</th><th>{t('dns.type')}</th><th>{t('dns.value')}</th><th>{t('users.status')}</th></tr></thead>
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
    created: ['+', 'text-green-600'], updated: ['↻', 'text-blue-600'],
    unchanged: ['=', 'text-muted-foreground'], failed: ['✗', 'text-red-500'],
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
