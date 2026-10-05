import type { ComponentType, ReactNode } from 'react'

// 统一空状态：图标 + 标题 + 说明 + 主/次操作。
// 收敛各页面「暂无…」的样式差异（见 #42 / #51）。
export function EmptyState({ icon: Icon, title, desc, action }: {
  icon?: ComponentType<{ size?: number; className?: string; strokeWidth?: number }>
  title: ReactNode
  desc?: ReactNode
  action?: ReactNode
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-10 px-4 text-center">
      {Icon && <Icon size={40} className="text-muted-foreground/60" strokeWidth={1.25} />}
      <p className="text-sm font-medium">{title}</p>
      {desc && <p className="text-xs text-muted-foreground max-w-xs">{desc}</p>}
      {action && <div className="mt-2 flex flex-wrap items-center justify-center gap-2">{action}</div>}
    </div>
  )
}
