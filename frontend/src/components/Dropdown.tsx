import { useEffect, useLayoutEffect, useRef, useState, type ComponentProps, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '../lib/utils'

// 轻量下拉菜单：点击触发器展开，点击外部 / Esc 关闭，选中项后自动关闭。
// 菜单通过 portal 渲染到 body 并使用 fixed 定位，避免被 overflow 容器裁剪或被层叠上下文遮挡。
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
  const menuRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; left?: number; right?: number } | null>(null)

  useLayoutEffect(() => {
    if (!open) return
    const compute = () => {
      const r = ref.current?.getBoundingClientRect()
      if (!r) return
      setPos(
        align === 'right'
          ? { top: r.bottom + 8, right: Math.max(8, window.innerWidth - r.right) }
          : { top: r.bottom + 8, left: Math.max(8, r.left) },
      )
    }
    compute()
    window.addEventListener('resize', compute)
    window.addEventListener('scroll', compute, true)
    return () => {
      window.removeEventListener('resize', compute)
      window.removeEventListener('scroll', compute, true)
    }
  }, [open, align])

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      const target = e.target as Node
      if (ref.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
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
      {open && pos && createPortal(
        <div
          ref={menuRef}
          role="menu"
          style={{ position: 'fixed', top: pos.top, left: pos.left, right: pos.right }}
          onClick={() => setOpen(false)}
          className={cn(
            'z-[100] min-w-[13rem] rounded-md border border-border bg-card text-card-foreground shadow-lg p-1',
            className,
          )}
        >
          {children}
        </div>,
        document.body,
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
