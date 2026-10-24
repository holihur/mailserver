import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api } from '../api/client'
import { useI18n } from '../lib/i18n'
import { cn } from '../lib/utils'
import { Mail, ShieldCheck } from 'lucide-react'

// 顶级双 Tab（#47）：把「管自己（邮箱）」与「管全站（管理）」分开。
// 管理 Tab 仅管理员可见；普通用户只会看到「邮箱」。
export function TopTabs({ className }: { className?: string }) {
  const { t } = useI18n()
  const { pathname } = useLocation()
  const [admin, setAdmin] = useState<boolean | null>(null)

  useEffect(() => {
    api.me().then((m: any) => setAdmin(!!m?.admin)).catch(() => {})
  }, [])

  const inAdmin = pathname.startsWith('/admin')
  const tab = 'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors'
  const active = 'bg-background shadow-sm font-medium text-foreground'
  const idle = 'text-muted-foreground hover:text-foreground'

  return (
    <nav className={cn('flex items-center gap-1 rounded-lg bg-muted/60 p-0.5', className)} aria-label="primary">
      <Link to="/" className={cn(tab, !inAdmin ? active : idle)}>
        <Mail size={14} />{t('nav.mailTab')}
      </Link>
      {admin && (
        <Link to="/admin" className={cn(tab, inAdmin ? active : idle)}>
          <ShieldCheck size={14} />{t('nav.adminTab')}
        </Link>
      )}
    </nav>
  )
}
