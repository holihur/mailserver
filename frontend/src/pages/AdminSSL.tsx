import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { ShieldCheck, Wand2, Upload, Trash2, Loader2, AlertCircle, CheckCircle2, Clock } from 'lucide-react'

export default function AdminSSL() {
  const { t } = useI18n()
  const [tls, setTls] = useState(null)
  const [mailHost, setMailHost] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')

  async function load() {
    try { setTls(await api.tlsGet()) } catch (e) { setMsg(e.message) }
    try { const s = await api.settingsGet(); setMailHost(s.mail_host || '') } catch {}
  }
  useEffect(() => { load() }, [])

  const cert = tls?.cert

  return (
    <AdminShell title={t('ssl.title')} desc={t('ssl.desc')}>
      <Card className="p-4">
        <div className="flex items-center gap-2">
          <ShieldCheck size={16} />
          <b className="text-sm">{t('ssl.current')}</b>
          {cert?.exists ? <Badge className="text-green-600">{t('ssl.configured')}</Badge> : <Badge className="text-yellow-600">{t('ssl.notConfigured')}</Badge>}
        </div>
        {cert?.exists ? (
          <div className="mt-3 text-sm space-y-1">
            <div>{t('ssl.domains')}: <code>{cert.domains?.join(', ')}</code></div>
            <div>{t('ssl.source')}: {sourceLabel(cert.source, t)} · {t('ssl.issuer')}: {cert.issuer || '—'}</div>
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Clock size={12} />{t('ssl.validUntil', { date: fmt(cert.not_after), days: daysLeft(cert.not_after) })}
            </div>
            <Button variant="outline" size="sm" className="mt-1" onClick={async () => {
              if (confirm(t('ssl.deleteCert'))) { await api.tlsDelete(); load() }
            }}><Trash2 />{t('ssl.deleteCert')}</Button>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground mt-2">{t('ssl.notConfigured')}</p>
        )}
      </Card>

      <Card className="p-4 space-y-3">
        <div className="flex items-center gap-2"><Wand2 size={16} /><b className="text-sm">{t('ssl.auto')}</b></div>
        <p className="text-xs text-muted-foreground">{t('ssl.autoHint')} <Link to="/admin/providers" className="underline">{t('providers.title')}</Link></p>
        <AcmeForm providers={tls?.providers || []} defaultDomain={tls?.acme?.domain || mailHost} acme={tls?.acme}
          onMsg={setMsg} onDone={load} busy={busy} setBusy={setBusy} />
      </Card>

      <Card className="p-4 space-y-3">
        <div className="flex items-center gap-2"><Upload size={16} /><b className="text-sm">{t('ssl.manual')}</b></div>
        <ManualForm onMsg={setMsg} onDone={load} />
      </Card>

      {msg && <p className="text-sm flex items-center gap-1 text-muted-foreground"><AlertCircle size={14} />{msg}</p>}
    </AdminShell>
  )
}

function AcmeForm({ providers, defaultDomain, acme, onMsg, onDone, busy, setBusy }) {
  const { t } = useI18n()
  const [f, setF] = useState({ domain: '', email: '', provider_id: '', auto_renew: true, staging: false })
  useEffect(() => {
    setF(v => ({
      ...v,
      domain: v.domain || defaultDomain || '',
      email: v.email || acme?.email || '',
      provider_id: v.provider_id || (providers[0]?.id ?? ''),
      auto_renew: acme ? acme.auto_renew : true,
    }))
  }, [defaultDomain, providers, acme])

  async function submit() {
    setBusy('acme'); onMsg('')
    try {
      await api.tlsAcme({ ...f, provider_id: Number(f.provider_id) })
      onMsg(t('ssl.issue'))
      onDone()
    } catch (e) { onMsg(e.message) } finally { setBusy('') }
  }

  if (providers.length === 0) {
    return <p className="text-sm text-yellow-600">{t('providers.none')}</p>
  }

  return (
    <div className="space-y-3">
      <div className="grid sm:grid-cols-2 gap-3">
        <label className="text-sm space-y-1">
          <span className="font-medium">{t('ssl.issueDomain')}</span>
          <Input placeholder="mail.example.com" value={f.domain} onChange={e => setF({ ...f, domain: e.target.value })} />
        </label>
        <label className="text-sm space-y-1">
          <span className="font-medium">{t('ssl.email')}</span>
          <Input placeholder="you@example.com" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} />
        </label>
        <label className="text-sm space-y-1">
          <span className="font-medium">{t('ssl.provider')}</span>
          <select className="h-9 w-full rounded-md border border-border bg-background text-sm px-2" value={f.provider_id} onChange={e => setF({ ...f, provider_id: e.target.value })}>
            {providers.map(p => <option key={p.id} value={p.id}>{p.type} · {p.name}</option>)}
          </select>
        </label>
        <label className="flex items-center gap-2 text-sm pt-6">
          <input type="checkbox" checked={f.auto_renew} onChange={e => setF({ ...f, auto_renew: e.target.checked })} />
          {t('ssl.autoRenew')}
        </label>
      </div>
      <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer">{t('ssl.advanced')}</summary>
        <label className="flex items-center gap-2 mt-2">
          <input type="checkbox" checked={f.staging} onChange={e => setF({ ...f, staging: e.target.checked })} />
          {t('ssl.staging')}
        </label>
      </details>
      <Button onClick={submit} disabled={busy === 'acme'}>
        {busy === 'acme' ? <Loader2 className="animate-spin" /> : <Wand2 />}
        {busy === 'acme' ? t('ssl.issuing') : t('ssl.issue')}
      </Button>
    </div>
  )
}

function ManualForm({ onMsg, onDone }) {
  const { t } = useI18n()
  const [cert, setCert] = useState('')
  const [key, setKey] = useState('')
  const [busy, setBusy] = useState(false)

  function readFile(setter) {
    return e => {
      const file = e.target.files?.[0]
      if (!file) return
      const fr = new FileReader()
      fr.onload = () => setter(String(fr.result || ''))
      fr.readAsText(file)
    }
  }

  async function submit() {
    setBusy(true); onMsg('')
    try { await api.tlsManual({ cert, key }); onMsg(t('ssl.saveEnable')); setCert(''); setKey(''); onDone() }
    catch (e) { onMsg(e.message) } finally { setBusy(false) }
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t('ssl.manualHint')}</p>
      <div className="grid sm:grid-cols-2 gap-3">
        <div className="space-y-1">
          <span className="text-sm font-medium">{t('ssl.certPem')}</span>
          <input type="file" accept=".pem,.crt,.cer" onChange={readFile(setCert)} className="text-xs" />
          <textarea className="w-full h-28 rounded-md border border-border bg-background p-2 text-xs font-mono"
            placeholder="-----BEGIN CERTIFICATE-----" value={cert} onChange={e => setCert(e.target.value)} />
        </div>
        <div className="space-y-1">
          <span className="text-sm font-medium">{t('ssl.keyPem')}</span>
          <input type="file" accept=".pem,.key" onChange={readFile(setKey)} className="text-xs" />
          <textarea className="w-full h-28 rounded-md border border-border bg-background p-2 text-xs font-mono"
            placeholder="-----BEGIN PRIVATE KEY-----" value={key} onChange={e => setKey(e.target.value)} />
        </div>
      </div>
      <Button onClick={submit} disabled={busy || !cert || !key}>
        {busy ? <Loader2 className="animate-spin" /> : <CheckCircle2 />}{t('ssl.saveEnable')}
      </Button>
    </div>
  )
}

function sourceLabel(s, t) {
  return { manual: t('ssl.manual'), acme: "Let's Encrypt", env: 'env', file: 'file' }[s] || s || '—'
}
function fmt(x) { try { return new Date(x).toLocaleString() } catch { return '—' } }
function daysLeft(x) {
  const d = Math.ceil((new Date(x).getTime() - Date.now()) / 86400000)
  return isNaN(d) ? '—' : d
}
