import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Badge } from './ui/controls'
import { FooterControls } from './HeaderControls'
import { useI18n } from '../lib/i18n'
import { LayoutDashboard, Server, ShieldCheck, Cloud, Users, Mail, Info, Filter, Route, AtSign, History, DatabaseBackup } from 'lucide-react'
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
  { to: '/admin', labelKey: 'admin.dashboard', icon: LayoutDashboard },
  { to: '/admin/settings', labelKey: 'admin.host', icon: Server },
  { to: '/admin/ssl', labelKey: 'admin.ssl', icon: ShieldCheck },
  { to: '/admin/providers', labelKey: 'admin.providers', icon: Cloud },
  { to: '/admin/users', labelKey: 'admin.users', icon: Users },
  { to: '/admin/rules', labelKey: 'rules.siteNav', icon: Filter },
  { to: '/admin/routes', labelKey: 'routes.nav', icon: Route },
  { to: '/admin/aliases', labelKey: 'aliases.nav', icon: AtSign },
  { to: '/admin/audit', labelKey: 'admin.audit', icon: History },
  { to: '/admin/backup', labelKey: 'admin.backup', icon: DatabaseBackup },
  { to: '/admin/about', labelKey: 'admin.about', icon: Info },
]

export default function AdminShell({ title, desc, children }: { title?: any; desc?: any; children?: any }) {
  const { t } = useI18n()
  const me = useMe()
  const { pathname } = useLocation()
  const [ver, setVer] = useState<any>(null)
  useEffect(() => { api.version().then(setVer).catch(() => {}) }, [])

  return (
    <div className="min-h-screen bg-background flex flex-col">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3 sticky top-0 bg-background/90 backdrop-blur z-10">
        <Link to="/" className="font-semibold flex items-center gap-2"><Mail size={16} />{BRAND}</Link>
        <Badge className="hidden sm:inline-flex">{t('admin.title')}</Badge>
        {ver?.version && (
          <Badge className="hidden sm:inline-flex" title={`commit ${ver.commit || '-'} · ${ver.date || '-'}`}>
            v{String(ver.version).replace(/^v/, '')}
          </Badge>
        )}
        <div className="flex-1" />
        <Link to="/"><Button variant="ghost" size="sm">{t('admin.backToMail')}</Button></Link>
      </header>

      {me && !me.admin ? (
        <div className="flex-1 max-w-xl mx-auto p-8 text-sm text-muted-foreground space-y-2">
          <p className="font-semibold text-foreground">{t('admin.needAdmin')}</p>
          <p>{t('admin.needAdminHint')}</p>
        </div>
      ) : (
        <div className="flex-1 max-w-5xl w-full mx-auto p-3 sm:p-4 md:grid md:grid-cols-[190px_1fr] md:gap-5">
          <aside className="mb-3 md:mb-0">
            <nav className="flex gap-1 overflow-x-auto pb-1 md:flex-col md:overflow-visible md:pb-0 md:sticky md:top-16">
              {NAV.map(n => (
                <Link key={n.to} to={n.to} aria-current={pathname === n.to ? 'page' : undefined}
                  className={cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm whitespace-nowrap shrink-0',
                    pathname === n.to ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
                  <n.icon size={16} />{t(n.labelKey)}
                </Link>
              ))}
            </nav>
          </aside>
          <main className="min-w-0 space-y-4">
            <div>
              <h1 className="text-xl font-semibold">{title}</h1>
              {desc && <p className="text-sm text-muted-foreground mt-0.5">{desc}</p>}
            </div>
            {children}
          </main>
        </div>
      )}
      <FooterControls />
    </div>
  )
}
