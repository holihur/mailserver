import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Badge } from './ui/controls'
import { ThemeToggle, LangToggle } from './HeaderControls'
import { useI18n } from '../lib/i18n'
import { LayoutDashboard, Server, ShieldCheck, Cloud, Users, Mail } from 'lucide-react'
import { cn } from '../lib/utils'

export function useMe() {
  const [me, setMe] = useState(null)
  useEffect(() => {
    api.me().then(setMe).catch(() => { location.hash = '#/login' })
  }, [])
  return me
}

const NAV = [
  { to: '#/admin', labelKey: 'admin.dashboard', icon: LayoutDashboard },
  { to: '#/admin/settings', labelKey: 'admin.host', icon: Server },
  { to: '#/admin/ssl', labelKey: 'admin.ssl', icon: ShieldCheck },
  { to: '#/admin/providers', labelKey: 'admin.providers', icon: Cloud },
  { to: '#/admin/users', labelKey: 'admin.users', icon: Users },
]

export default function AdminShell({ title, desc, children }) {
  const { t } = useI18n()
  const me = useMe()
  const [hash, setHash] = useState(location.hash || '#/admin')
  useEffect(() => {
    const on = () => setHash(location.hash || '#/admin')
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3 sticky top-0 bg-background/90 backdrop-blur z-10">
        <a href="#/" className="font-semibold flex items-center gap-2"><Mail size={16} />Mailserver</a>
        <Badge className="hidden sm:inline-flex">{t('admin.title')}</Badge>
        <div className="flex-1" />
        <LangToggle />
        <ThemeToggle />
        <a href="#/"><Button variant="ghost" size="sm">{t('admin.backToMail')}</Button></a>
      </header>

      {me && !me.admin ? (
        <div className="max-w-xl mx-auto p-8 text-sm text-muted-foreground space-y-2">
          <p className="font-semibold text-foreground">{t('admin.needAdmin')}</p>
          <p>{t('admin.needAdminHint')}</p>
        </div>
      ) : (
        <div className="max-w-5xl mx-auto p-3 sm:p-4 md:grid md:grid-cols-[190px_1fr] md:gap-5">
          <aside className="mb-3 md:mb-0">
            <nav className="flex gap-1 overflow-x-auto pb-1 md:flex-col md:overflow-visible md:pb-0 md:sticky md:top-16">
              {NAV.map(n => (
                <a key={n.to} href={n.to}
                  className={cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm whitespace-nowrap shrink-0',
                    hash === n.to ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
                  <n.icon size={16} />{t(n.labelKey)}
                </a>
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
    </div>
  )
}
