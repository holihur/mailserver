import { useEffect, useState } from 'react'
import { Button } from './ui/controls'
import { useI18n } from '../lib/i18n'
import { subscribeUI, getToasts, getConfirms, dismissToast, resolveConfirm } from '../lib/ui'
import { CheckCircle2, AlertCircle, Info, X } from 'lucide-react'

// 全局挂载：右下角 Toast 通知 + 居中确认框。
export function UIHost() {
  const { t } = useI18n()
  const [, force] = useState(0)
  useEffect(() => subscribeUI(() => force(n => n + 1)), [])
  const toasts = getToasts()
  const confirms = getConfirms()

  return (
    <>
      <div className="fixed bottom-4 right-4 z-[100] space-y-2 w-[min(92vw,22rem)] pointer-events-none">
        {toasts.map(x => {
          const Icon = x.type === 'success' ? CheckCircle2 : x.type === 'error' ? AlertCircle : Info
          const color = x.type === 'success' ? 'text-green-600' : x.type === 'error' ? 'text-red-500' : 'text-primary'
          return (
            <div key={x.id} className="pointer-events-auto flex items-start gap-2 rounded-md border border-border bg-card shadow-lg p-3 text-sm">
              <Icon size={16} className={`shrink-0 mt-0.5 ${color}`} />
              <span className="flex-1 break-words">{x.msg}</span>
              {x.action && (
                <button className="text-primary font-medium shrink-0 hover:underline"
                  onClick={() => { x.action!.onClick(); dismissToast(x.id) }}>
                  {x.action.label}
                </button>
              )}
              <button className="text-muted-foreground shrink-0" onClick={() => dismissToast(x.id)} aria-label="close"><X size={14} /></button>
            </div>
          )
        })}
      </div>

      {confirms.map(c => (
        <div key={c.id} className="fixed inset-0 z-[100] grid place-items-center bg-black/40 p-4"
          onClick={() => resolveConfirm(c.id, false)}>
          <div className="w-full max-w-sm rounded-lg border border-border bg-card p-4 space-y-4 shadow-xl"
            onClick={e => e.stopPropagation()}>
            <p className="text-sm whitespace-pre-line break-words">{c.msg}</p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={() => resolveConfirm(c.id, false)}>{t('common.cancel')}</Button>
              <Button variant="destructive" size="sm" onClick={() => resolveConfirm(c.id, true)}>{t('common.confirm')}</Button>
            </div>
          </div>
        </div>
      ))}
    </>
  )
}
