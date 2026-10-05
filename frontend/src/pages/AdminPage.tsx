import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { Card, Button, Input } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { SkeletonCards } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { toast } from '../lib/ui'
import { Server, ShieldCheck, Cloud, Users, CheckCircle2, Circle, Globe, Loader2 } from 'lucide-react'

export default function AdminPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [ov, setOv] = useState(null)
  const [health, setHealth] = useState<any>(null)
  const [tls, setTls] = useState(null)
  const [settings, setSettings] = useState(null)
  const [providers, setProviders] = useState([])
  const [domains, setDomains] = useState<any[]>([])
  const [checkDomain, setCheckDomain] = useState('')
  const [checkResult, setCheckResult] = useState<any>(null)
  const [checking, setChecking] = useState(false)

  useEffect(() => {
    api.adminOverview().then(setOv).catch(() => {})
    api.tlsGet().then(setTls).catch(() => {})
    api.settingsGet().then(setSettings).catch(() => {})
    api.providers().then(setProviders).catch(() => {})
    api.dnsList().then((d: any) => { setDomains(d || []); if (d?.[0]?.name) setCheckDomain(d[0].name) }).catch(() => {})
    const loadHealth = () => api.adminHealth().then(setHealth).catch(() => {})
    loadHealth()
    const id = setInterval(loadHealth, 30000)
    return () => clearInterval(id)
  }, [])

  async function runCheck() {
    if (!checkDomain) return
    setChecking(true)
    try { setCheckResult(await api.adminDomainCheck(checkDomain)) } catch (e: any) { toast(e.message) } finally { setChecking(false) }
  }

  const steps = [
    { ok: !!settings?.mail_host, label: t('settings.mailDomain'), to: '/admin/settings' },
    { ok: providers.length > 0, label: t('providers.title'), to: '/admin/providers' },
    { ok: !!tls?.cert?.exists, label: t('ssl.title'), to: '/admin/ssl' },
    { ok: !!settings?.dkim_ready, label: t('settings.dkim'), to: '/admin/settings' },
    { ok: (ov?.users || 0) > 0, label: t('users.create'), to: '/admin/users' },
  ]
  const done = steps.filter(s => s.ok).length
  const next = steps.find(s => !s.ok)
  const checkRows: [boolean, string, string][] = checkResult ? [
    [checkResult.mx_ok, t('admin.mxOk', { host: checkResult.expected_mx || 'MX' }), t('admin.mxBad', { host: checkResult.expected_mx || 'MX' })],
    [checkResult.spf, t('admin.recOk', { name: 'SPF' }), t('admin.recBad', { name: 'SPF' })],
    [checkResult.dkim, t('admin.recOk', { name: 'DKIM' }), t('admin.recBad', { name: 'DKIM' })],
    [checkResult.dmarc, t('admin.recOk', { name: 'DMARC' }), t('admin.recBad', { name: 'DMARC' })],
  ] : []

  return (
    <AdminShell title={t('admin.dashboard')} desc={t('settings.desc')}>
      {!ov ? <SkeletonCards /> : (
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {[
          [t('admin.users2'), ov?.users ?? '—', Users],
          [t('admin.domains'), ov?.domains ?? '—', Globe],
          [t('admin.mails'), ov?.mails ?? '—', Server],
          [t('admin.pending'), ov?.pending ?? '—', Cloud],
        ].map(([k, v, Icon]) => (
          <Card key={k} className="p-3">
            <div className="flex items-center gap-1 text-xs text-muted-foreground"><Icon size={13} />{k}</div>
            <div className="text-2xl font-semibold mt-1">{v}</div>
          </Card>
        ))}
      </div>
      )}

      <Card className="p-4">
        <div className="flex items-center justify-between gap-2">
          <b className="text-sm">{t('admin.checklist')}</b>
          {next
            ? <Button size="sm" onClick={() => navigate(next.to)}>{t('admin.nextStep')}: {next.label}</Button>
            : <span className="text-xs text-success">{t('admin.allDone')}</span>}
        </div>
        <div className="flex items-center gap-2 mt-2 mb-3">
          <div className="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
            <div className="h-full bg-primary transition-all" style={{ width: `${Math.round((done / steps.length) * 100)}%` }} />
          </div>
          <span className="text-xs text-muted-foreground shrink-0">{t('admin.progress')} {done}/{steps.length}</span>
        </div>
        <div className="space-y-1">
          {steps.map((s, i) => (
            <Link key={i} to={s.to} className="flex items-center gap-2 rounded-md px-2 py-2 hover:bg-muted text-sm">
              {s.ok ? <CheckCircle2 size={16} className="text-success" /> : <Circle size={16} className="text-muted-foreground" />}
              <span className={s.ok ? 'text-muted-foreground line-through' : ''}>{s.label}</span>
            </Link>
          ))}
        </div>
      </Card>

      <Card className="p-4 space-y-3">
        <b className="text-sm flex items-center gap-1.5"><Globe size={15} />{t('admin.domainCheck')}</b>
        <p className="text-xs text-muted-foreground">{t('admin.domainCheckHint')}</p>
        <div className="flex items-center gap-2">
          {domains.length > 0 ? (
            <select value={checkDomain} onChange={e => setCheckDomain(e.target.value)}
              className="h-9 flex-1 rounded-md border border-border bg-background text-sm px-2">
              {domains.map((d: any) => <option key={d.id} value={d.name}>{d.name}</option>)}
            </select>
          ) : (
            <Input value={checkDomain} onChange={e => setCheckDomain(e.target.value)} placeholder="example.com" />
          )}
          <Button size="sm" className="h-9 shrink-0" disabled={checking || !checkDomain} onClick={runCheck}>
            {checking ? <Loader2 size={14} className="animate-spin" /> : null}{t('admin.check')}
          </Button>
        </div>
        {checkResult && (
          <div className="space-y-1 text-sm">
            {checkRows.map(([ok, good, bad], i) => (
              <div key={i} className="flex items-center gap-2">
                {ok ? <CheckCircle2 size={15} className="text-success shrink-0" /> : <Circle size={15} className="text-destructive shrink-0" />}
                <span className={ok ? 'text-muted-foreground' : ''}>{ok ? good : bad}</span>
              </div>
            ))}
            {checkResult.mx?.length > 0 && <p className="text-[11px] text-muted-foreground break-all">MX: {checkResult.mx.join(', ')}</p>}
          </div>
        )}
      </Card>

      <div className="grid sm:grid-cols-3 gap-3">
        <QuickCard to="/admin/settings" icon={Server} title={t('admin.host')} desc={t('admin.quickHost')} />
        <QuickCard to="/admin/ssl" icon={ShieldCheck} title={t('admin.ssl')} desc={t('admin.quickSsl')} />
        <QuickCard to="/admin/providers" icon={Cloud} title={t('admin.providers')} desc={t('admin.quickProviders')} />
      </div>
    </AdminShell>
  )
}

function QuickCard({ to, icon: Icon, title, desc }: any) {
  return (
    <Link to={to} className="block">
      <Card className="p-4 hover:bg-muted/50 transition-colors h-full">
        <Icon size={18} />
        <div className="font-medium mt-2">{title}</div>
        <div className="text-xs text-muted-foreground mt-0.5">{desc}</div>
      </Card>
    </Link>
  )
}
