import { Fragment, useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Badge } from './ui/controls'
import { SettingsMenu } from './HeaderControls'
import { TopTabs } from './TopTabs'
import { useI18n } from '../lib/i18n'
import { LayoutDashboard, Server, ShieldCheck, Cloud, Users, Mail, Info, Filter, Route, AtSign, History, DatabaseBackup, Sparkles, Lock } from 'lucide-react'
import { cn } from '../lib/utils'
import { BRAND } from '../lib/brand'

export function useMe() {
  const [me, setMe] = useState<any>(null)
  useEffect(() => {
    api.me().then(setMe).catch(() => { location.href = '/login' })
  }, [])
  return me
}

const NAV = [
  { to: '/admin', labelKey: 'admin.dashboard', icon: LayoutDashboard, group: 'nav.groupOverview' },
  { to: '/admin/settings', labelKey: 'admin.host', icon: Server, group: 'nav.groupConfig' },
  { to: '/admin/ssl', labelKey: 'admin.ssl', icon: ShieldCheck, group: 'nav.groupConfig' },
  { to: '/admin/providers', labelKey: 'admin.providers', icon: Cloud, group: 'nav.groupConfig' },
  { to: '/admin/ai', labelKey: 'ai.nav', icon: Sparkles, group: 'nav.groupConfig' },
  { to: '/admin/users', labelKey: 'admin.users', icon: Users, group: 'nav.groupUsers' },
  { to: '/admin/rules', labelKey: 'rules.siteNav', icon: Filter, group: 'nav.groupUsers' },
  { to: '/admin/routes', labelKey: 'routes.nav', icon: Route, group: 'nav.groupUsers' },
  { to: '/admin/aliases', labelKey: 'aliases.nav', icon: AtSign, group: 'nav.groupUsers' },
  { to: '/admin/audit', labelKey: 'admin.audit', icon: History, group: 'nav.groupOps' },
  { to: '/admin/backup', labelKey: 'admin.backup', icon: DatabaseBackup, group: 'nav.groupOps' },
  { to: '/admin/about', labelKey: 'admin.about', icon: Info, group: 'nav.groupOps' },
]

export default function AdminShell({ title, desc, children }: { title?: any; desc?: any; children?: any }) {
  const { t } = useI18n()
  const me = useMe()
  const { pathname } = useLocation()
  const [ver, setVer] = useState<any>(null)
  useEffect(() => { api.version().then(setVer).catch(() => {}) }, [])

  return (
    <div className="min-h-dvh bg-background flex flex-col">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3 sticky top-0 bg-background/90 backdrop-blur z-10">
        <Link to="/" className="font-semibold flex items-center gap-2 text-lg"><Mail size={20} />{BRAND}</Link>
        <TopTabs className="hidden sm:flex ml-1" />
        <Badge className="hidden sm:inline-flex">{t('admin.title')}</Badge>
        {ver?.version && (
          <Badge className="hidden sm:inline-flex" title={`commit ${ver.commit || '-'} · ${ver.date || '-'}`}>
            v{String(ver.version).replace(/^v/, '')}
          </Badge>
        )}
        <div className="flex-1" />
        <SettingsMenu />
        <Link to="/"><Button variant="ghost" size="sm">{t('admin.backToMail')}</Button></Link>
      </header>

      {me && !me.admin ? (
        <div className="flex-1 grid place-items-center p-6">
          <div className="max-w-md w-full rounded-lg border border-border bg-card p-6 text-center space-y-3">
            <div className="mx-auto grid place-items-center size-12 rounded-full bg-muted"><Lock size={22} className="text-muted-foreground" /></div>
            <h2 className="font-semibold">{t('admin.trialTitle')}</h2>
            <p className="text-sm text-muted-foreground">{t('admin.trialHint')}</p>
            <Link to="/"><Button variant="outline" size="sm">{t('admin.backToMail')}</Button></Link>
          </div>
        </div>
      ) : (
        <div className="flex-1 max-w-5xl w-full mx-auto p-3 sm:p-4 md:grid md:grid-cols-[190px_1fr] md:gap-5">
          <aside className="mb-3 md:mb-0">
            <nav className="flex gap-1 overflow-x-auto pb-1 md:flex-col md:overflow-visible md:pb-0 md:sticky md:top-16">
              {NAV.map((n, i) => (
                <Fragment key={n.to}>
                  {n.group && (i === 0 || NAV[i - 1].group !== n.group) && (
                    <div className="hidden md:block px-3 pt-2 pb-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{t(n.group)}</div>
                  )}
                  <Link to={n.to} aria-current={pathname === n.to ? 'page' : undefined}
                    className={cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm whitespace-nowrap shrink-0',
                      pathname === n.to ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
                    <n.icon size={16} />{t(n.labelKey)}
                  </Link>
                </Fragment>
              ))}
            </nav>
          </aside>
          <main className="min-w-0 space-y-4">
            <div>
              <div className="text-xs text-muted-foreground flex items-center gap-1.5 mb-1">
                <span>{t('admin.breadcrumbHome')}</span><span className="opacity-50">/</span>
                <span className="text-foreground">{(() => { const n = NAV.find(x => pathname === x.to); return n ? t(n.labelKey) : title })()}</span>
              </div>
              <h1 className="text-xl font-semibold">{title}</h1>
              {desc && <p className="text-sm text-muted-foreground mt-0.5">{desc}</p>}
            </div>
            {children}
          </main>
        </div>
      )}
    </div>
  )
}
