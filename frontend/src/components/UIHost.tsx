import { useEffect, useState } from 'react'
import { Button, Input } from './ui/controls'
import { useI18n } from '../lib/i18n'
import { subscribeUI, getToasts, getConfirms, getPrompts, dismissToast, resolveConfirm, resolvePrompt, type PromptItem } from '../lib/ui'
import { CheckCircle2, AlertCircle, Info, X } from 'lucide-react'

// 全局挂载：右下角 Toast 通知 + 居中确认框。
export function UIHost() {
  const { t } = useI18n()
  const [, force] = useState(0)
  useEffect(() => subscribeUI(() => force(n => n + 1)), [])
  const toasts = getToasts()
  const confirms = getConfirms()
  const prompts = getPrompts()

  return (
    <>
      <div className="fixed bottom-4 right-4 z-[100] space-y-2 w-[min(92vw,22rem)] pointer-events-none">
        {toasts.map(x => {
          const Icon = x.type === 'success' ? CheckCircle2 : x.type === 'error' ? AlertCircle : Info
          const color = x.type === 'success' ? 'text-success' : x.type === 'error' ? 'text-destructive' : 'text-primary'
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
          role={c.destructive ? 'alertdialog' : 'dialog'} aria-modal="true"
          onClick={() => resolveConfirm(c.id, false)}>
          <div className="w-full max-w-sm rounded-lg border border-border bg-card p-4 space-y-4 shadow-xl"
            onClick={e => e.stopPropagation()}>
            <div className="flex items-start gap-2">
              {c.destructive && <AlertCircle size={16} className="text-destructive shrink-0 mt-0.5" />}
              <p className="text-sm whitespace-pre-line break-words">{c.msg}</p>
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={() => resolveConfirm(c.id, false)}>{t('common.cancel')}</Button>
              <Button variant={c.destructive ? 'destructive' : 'default'} size="sm" onClick={() => resolveConfirm(c.id, true)}>{t('common.confirm')}</Button>
            </div>
          </div>
        </div>
      ))}

      {prompts.map(p => <PromptDialog key={p.id} item={p} />)}
    </>
  )
}

function PromptDialog({ item }: { item: PromptItem }) {
  const { t } = useI18n()
  const [v, setV] = useState(item.defaultValue || '')
  const submit = (e: any) => { e.preventDefault(); resolvePrompt(item.id, v) }
  return (
    <div className="fixed inset-0 z-[100] grid place-items-center bg-black/40 p-4"
      onClick={() => resolvePrompt(item.id, null)}>
      <form className="w-full max-w-sm rounded-lg border border-border bg-card p-4 space-y-4 shadow-xl"
        onClick={e => e.stopPropagation()} onSubmit={submit}>
        <p className="text-sm whitespace-pre-line break-words">{item.msg}</p>
        <Input autoFocus
          type={item.password ? 'password' : item.inputType === 'number' ? 'number' : 'text'}
          value={v} placeholder={item.placeholder}
          onChange={e => setV(e.target.value)}
          onKeyDown={e => { if (e.key === 'Escape') resolvePrompt(item.id, null) }} />
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" type="button" onClick={() => resolvePrompt(item.id, null)}>{t('common.cancel')}</Button>
          <Button size="sm" type="submit">{t('common.confirm')}</Button>
        </div>
      </form>
    </div>
  )
}
