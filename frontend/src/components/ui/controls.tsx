import type { ComponentProps } from 'react'
import { cn } from '../../lib/utils'

type ButtonProps = ComponentProps<'button'> & {
  variant?: 'default' | 'outline' | 'ghost' | 'destructive'
  size?: 'default' | 'sm' | 'icon'
}

export function Button({ className, variant = 'default', size = 'default', ...p }: ButtonProps) {
  const base = 'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus:outline-hidden disabled:opacity-50 [&_svg]:size-4'
  const variants = {
    default: 'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90',
    outline: 'border border-border bg-background hover:bg-muted',
    ghost: 'hover:bg-muted',
    destructive: 'bg-red-500 text-white hover:bg-red-600',
  }
  const sizes = { default: 'h-9 px-4 py-2', sm: 'h-8 px-3 text-xs', icon: 'h-9 w-9' }
  return <button className={cn(base, variants[variant], sizes[size], className)} {...p} />
}

export function Input({ className, ...p }: ComponentProps<'input'>) {
  return <input className={cn('flex h-9 w-full rounded-md border border-border bg-background px-3 py-1 text-sm outline-hidden focus:ring-2 focus:ring-primary/30', className)} {...p} />
}

export function Select({ className, ...p }: ComponentProps<'select'>) {
  return <select className={cn('flex h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-hidden focus:ring-2 focus:ring-primary/30', className)} {...p} />
}

export function Label({ className, ...p }: ComponentProps<'label'>) {
  return <label className={cn('text-sm font-medium', className)} {...p} />
}

export function Textarea({ className, ...p }: ComponentProps<'textarea'>) {
  return <textarea className={cn('flex min-h-[120px] w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-hidden focus:ring-2 focus:ring-primary/30', className)} {...p} />
}

export function Card({ className, ...p }: ComponentProps<'div'>) {
  return <div className={cn('rounded-lg border border-border bg-card text-card-foreground shadow-xs', className)} {...p} />
}

export function Badge({ className, ...p }: ComponentProps<'span'>) {
  return <span className={cn('inline-flex items-center rounded-full bg-muted px-2.5 py-0.5 text-xs text-muted-foreground', className)} {...p} />
}
