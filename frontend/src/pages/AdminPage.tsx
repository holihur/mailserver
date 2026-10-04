import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { Card } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { SkeletonCards } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { Server, ShieldCheck, Cloud, Users, CheckCircle2, Circle, Globe, Activity } from 'lucide-react'
import { cn } from '../lib/utils'

export default function AdminPage() {
  const { t } = useI18n()
  const [ov, setOv] = useState(null)
  const [health, setHealth] = useState<any>(null)
  const [tls, setTls] = useState(null)
  const [settings, setSettings] = useState(null)
  const [providers, setProviders] = useState([])

  useEffect(() => {
    api.adminOverview().then(setOv).catch(() => {})
    api.tlsGet().then(setTls).catch(() => {})
    api.settingsGet().then(setSettings).catch(() => {})
    api.providers().then(setProviders).catch(() => {})
    const loadHealth = () => api.adminHealth().then(setHealth).catch(() => {})
    loadHealth()
    const id = setInterval(loadHealth, 30000)
    return () => clearInterval(id)
  }, [])

  const steps = [
    { ok: !!settings?.mail_host, label: t('settings.mailDomain'), to: '/admin/settings' },
    { ok: providers.length > 0, label: t('providers.title'), to: '/admin/providers' },
    { ok: !!tls?.cert?.exists, label: t('ssl.title'), to: '/admin/ssl' },
    { ok: !!settings?.dkim_ready, label: t('settings.dkim'), to: '/admin/settings' },
    { ok: (ov?.users || 0) > 0, label: t('users.create'), to: '/admin/users' },
  ]

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
        <b className="text-sm">{t('admin.checklist')}</b>
        <div className="mt-3 space-y-1">
          {steps.map((s, i) => (
            <Link key={i} to={s.to} className="flex items-center gap-2 rounded-md px-2 py-2 hover:bg-muted text-sm">
              {s.ok ? <CheckCircle2 size={16} className="text-green-600" /> : <Circle size={16} className="text-muted-foreground" />}
              <span className={s.ok ? 'text-muted-foreground line-through' : ''}>{s.label}</span>
            </Link>
          ))}
        </div>
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
