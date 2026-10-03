import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Badge } from './ui/controls'
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
  { to: '#/admin', label: '概览', icon: LayoutDashboard },
  { to: '#/admin/settings', label: '邮件主机', icon: Server },
  { to: '#/admin/ssl', label: 'SSL 证书', icon: ShieldCheck },
  { to: '#/admin/providers', label: '域名服务商', icon: Cloud },
  { to: '#/admin/users', label: '用户账号', icon: Users },
]

export default function AdminShell({ title, desc, children }) {
  const me = useMe()
  const [hash, setHash] = useState(location.hash || '#/admin')
  useEffect(() => {
    const on = () => setHash(location.hash || '#/admin')
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3">
        <a href="#/" className="font-semibold flex items-center gap-2"><Mail size={16} />Mailserver</a>
        <Badge>管理后台</Badge>
        <div className="flex-1" />
        <a href="#/"><Button variant="ghost" size="sm">返回邮箱</Button></a>
      </header>

      {me && !me.admin ? (
        <div className="max-w-xl mx-auto p-8 text-sm text-muted-foreground space-y-2">
          <p className="font-semibold text-foreground">需要管理员权限</p>
          <p>请让管理员把你的邮箱加入管理员名单，或用管理员账号登录。</p>
        </div>
      ) : (
        <div className="max-w-5xl mx-auto p-4 grid md:grid-cols-[190px_1fr] gap-5">
          <aside className="space-y-1 h-fit md:sticky md:top-4">
            {NAV.map(n => (
              <a key={n.to} href={n.to}
                className={cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm',
                  hash === n.to ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
                <n.icon size={16} />{n.label}
              </a>
            ))}
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
