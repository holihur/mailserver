import { useEffect, useRef, useState, type ComponentProps, type ReactNode } from 'react'
import { cn } from '../lib/utils'

// 轻量下拉菜单：点击触发器展开，点击外部 / Esc 关闭，选中项后自动关闭。
export function Dropdown({
  trigger,
  children,
  align = 'right',
  className,
}: {
  trigger: ReactNode
  children: ReactNode
  align?: 'left' | 'right'
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={ref} className="relative">
      <div onClick={() => setOpen(o => !o)}>{trigger}</div>
      {open && (
        <div
          role="menu"
          onClick={() => setOpen(false)}
          className={cn(
            'absolute z-50 mt-2 min-w-[13rem] rounded-md border border-border bg-card text-card-foreground shadow-lg p-1',
            align === 'right' ? 'right-0' : 'left-0',
            className,
          )}
        >
          {children}
        </div>
      )}
    </div>
  )
}

type ItemProps = ComponentProps<'button'> & { icon?: any }

export function DropdownItem({ className, icon: Icon, children, ...p }: ItemProps) {
  return (
    <button
      role="menuitem"
      className={cn('w-full flex items-center gap-2 rounded-sm px-2 py-2 text-sm text-left hover:bg-muted', className)}
      {...p}
    >
      {Icon && <Icon size={15} className="shrink-0 text-muted-foreground" />}
      <span className="truncate">{children}</span>
    </button>
  )
}

export function DropdownSeparator() {
  return <div className="my-1 h-px bg-border" />
}

export function DropdownLabel({ children }: { children: ReactNode }) {
  return <div className="px-2 py-1.5 text-xs text-muted-foreground truncate">{children}</div>
}
