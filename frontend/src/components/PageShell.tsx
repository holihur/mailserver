import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { SettingsMenu } from './HeaderControls'
import { BRAND } from '../lib/brand'
import { Mail } from 'lucide-react'

// 次级页面统一外壳：顶部返回 + 标题（含图标）+ 语言/主题，内容居中定宽。
export default function PageShell({
  title,
  icon: Icon,
  children,
  maxWidth = 'max-w-3xl',
}: {
  title?: ReactNode
  icon?: any
  children?: ReactNode
  maxWidth?: string
}) {
  return (
    <div className="min-h-dvh bg-background flex flex-col">
      <header className="border-b border-border px-3 sm:px-4 h-14 flex items-center gap-2 sticky top-0 bg-background/90 backdrop-blur z-10">
        <Link to="/" className="flex items-center gap-2 font-semibold shrink-0 text-lg"><Mail size={20} />{BRAND}</Link>
        <span className="flex items-center gap-1.5 text-sm text-muted-foreground min-w-0">
          {Icon && <Icon size={15} className="shrink-0" />}
          <span className="truncate">{title}</span>
        </span>
        <div className="flex-1" />
        <SettingsMenu />
      </header>
      <div className={`${maxWidth} w-full mx-auto p-3 sm:p-4 space-y-4 flex-1`}>{children}</div>
    </div>
  )
}
