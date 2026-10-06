import { useI18n } from '../lib/i18n'
import { cn } from '../lib/utils'
import { XCircle, Loader2, Clock, ShieldCheck } from 'lucide-react'

// 发件队列状态徽标（已发送文件夹）。
export function SendStatus({ m, folder }: any) {
  const { t } = useI18n()
  if (folder !== 'sent' || !m.status || m.status === 'sent') return null
  if (m.status === 'failed') return <span title={m.relay_err || ''} className="flex items-center gap-0.5 text-[10px] text-destructive shrink-0"><XCircle size={11} />{t('mail.stFailed')}</span>
  if (m.status === 'sending') return <span className="flex items-center gap-0.5 text-[10px] text-primary shrink-0"><Loader2 size={11} className="animate-spin" />{t('mail.stSending')}</span>
  return <span className="flex items-center gap-0.5 text-[10px] text-warning shrink-0"><Clock size={11} />{t('mail.stQueued')}</span>
}

// 发件人认证徽章：把 SPF/DKIM/DMARC 结果翻译成通过/未通过（#49）。
export function AuthBadges({ raw, t }: { raw?: string; t: (k: string) => string }) {
  if (!raw) return null
  const parts: { k: string; v: string }[] = []
  for (const seg of raw.split(';')) {
    const [k, v] = seg.trim().split('=').map(s => (s || '').trim().toLowerCase())
    if (k === 'spf' || k === 'dkim' || k === 'dmarc') parts.push({ k, v })
  }
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
