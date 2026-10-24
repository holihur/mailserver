import { useState } from 'react'
import { useI18n } from '../lib/i18n'
import { cn } from '../lib/utils'
import { XCircle, Loader2, Clock, ShieldCheck, ShieldAlert, ShieldQuestion } from 'lucide-react'

// 发件队列状态徽标（已发送文件夹）。
export function SendStatus({ m, folder }: any) {
  const { t } = useI18n()
  if (folder !== 'sent' || !m.status || m.status === 'sent') return null
  if (m.status === 'failed') return <span title={m.relay_err || ''} className="flex items-center gap-0.5 text-[10px] text-destructive shrink-0"><XCircle size={11} />{t('mail.stFailed')}</span>
  if (m.status === 'sending') return <span className="flex items-center gap-0.5 text-[10px] text-primary shrink-0"><Loader2 size={11} className="animate-spin" />{t('mail.stSending')}</span>
  return <span className="flex items-center gap-0.5 text-[10px] text-warning shrink-0"><Clock size={11} />{t('mail.stQueued')}</span>
}

function parseAuth(raw?: string) {
  const parts: { k: string; v: string }[] = []
  if (!raw) return parts
  for (const seg of raw.split(';')) {
    const [k, v] = seg.trim().split('=').map(s => (s || '').trim().toLowerCase())
    if (k === 'spf' || k === 'dkim' || k === 'dmarc') parts.push({ k, v })
  }
  return parts
}

// 发件人认证徽章：把 SPF/DKIM/DMARC 结果翻译成通过/未通过（#49）。
export function AuthBadges({ raw, t }: { raw?: string; t: (k: string) => string }) {
  if (!raw) return null
  const parts = parseAuth(raw)
  if (!parts.length) return null
  const cls = (v: string) => v === 'pass' ? 'bg-success/10 text-success dark:text-success'
    : v === 'fail' ? 'bg-destructive/10 text-destructive dark:text-destructive'
    : 'bg-muted text-muted-foreground'
  const label = (v: string) => v === 'pass' ? t('mail.authPass') : v === 'fail' ? t('mail.authFail') : t('mail.authUnknown')
  return (
    <div className="flex flex-wrap items-center gap-1 mt-1" title={t('mail.trustTitle')}>
      <ShieldCheck size={12} className="text-muted-foreground" />
      {parts.map(p => (
        <span key={p.k} className={cn('rounded px-1.5 py-0.5 text-[10px] font-medium uppercase', cls(p.v))}>
          {p.k} {label(p.v)}
        </span>
      ))}
    </div>
  )
}

// 信任条（#49 让可信可见）：三态 + 人话解释，DMARC 失败时提示疑似仿冒。
export function TrustBar({ raw, from, t }: { raw?: string; from?: string; t: (k: string, vars?: any) => string }) {
  const [open, setOpen] = useState(false)
  const parts = parseAuth(raw)
  if (!parts.length) return null
  const dmarc = parts.find(p => p.k === 'dmarc')?.v
  const allPass = parts.every(p => p.v === 'pass')
  const state = dmarc === 'fail' ? 'spoof' : (allPass || dmarc === 'pass') ? 'verified' : 'unknown'
  const cfg = {
    verified: { cls: 'border-success/30 bg-success/10 text-success', Icon: ShieldCheck, label: t('trust.verified') },
    spoof: { cls: 'border-destructive/40 bg-destructive/10 text-destructive', Icon: ShieldAlert, label: t('trust.spoof') },
    unknown: { cls: 'border-warning/30 bg-warning/10 text-warning', Icon: ShieldQuestion, label: t('trust.unverified') },
  }[state]
  return (
    <div className={cn('mt-2 rounded-md border px-2.5 py-1.5 text-xs', cfg.cls)} role={state === 'spoof' ? 'alert' : undefined}>
      <div className="flex flex-wrap items-center gap-1.5">
        <cfg.Icon size={14} className="shrink-0" />
        <span className="font-medium">{cfg.label}</span>
        <span className="ml-auto flex items-center gap-1">
          {parts.map(p => (
            <span key={p.k} className="rounded bg-background/60 px-1 text-[10px] uppercase" title={p.v}>{p.k}</span>
          ))}
        </span>
        <button type="button" onClick={() => setOpen(!open)} className="underline shrink-0">{t('trust.why')}</button>
      </div>
      {open && (
        <p className="mt-1 leading-relaxed opacity-90">
          {t('trust.explain', { from: from || '-', auth: raw })}
        </p>
      )}
    </div>
  )
}
