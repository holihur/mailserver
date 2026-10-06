import { api } from '../api/client'
import { useI18n } from '../lib/i18n'
import { cn } from '../lib/utils'
import { recipientChip, attList, fmtWhen } from '../lib/mailFormat'
import { SendStatus } from './MailStatus'
import { Star, Paperclip } from 'lucide-react'

// 邮件列表单行（从 MailApp 拆出）。滑动操作 / 选中 / 星标。
export function MailListItem({ m, folder, sel, me, selectMode, checked, touch, open, toggleCheck, load }: any) {
  const { t } = useI18n()
  // 已发送/草稿：重要的是收件人；其他（收件箱/垃圾箱/自定义）重要的是发件人。
  const emphasisOnRecipient = folder === 'sent' || folder === 'draft'
  // 收件账户标识仅在“收到的邮件”上有意义。
  const showRecipientChip = !emphasisOnRecipient
  const strong = 'font-medium'
  const dim = 'text-muted-foreground'
  return (
    <div key={m.id} role="button" tabIndex={0} aria-current={sel?.id === m.id ? 'true' : undefined}
      onClick={() => { if (touch.current.moved) { touch.current.moved = false; return } selectMode ? toggleCheck(m.id) : open(m.id) }}
      onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectMode ? toggleCheck(m.id) : open(m.id) } }}
      onTouchStart={e => { touch.current = { x: e.touches[0].clientX, moved: false }; e.currentTarget.style.transition = 'none' }}
      onTouchMove={e => {
        const dx = e.touches[0].clientX - touch.current.x
        if (Math.abs(dx) > 12) touch.current.moved = true
        e.currentTarget.style.transform = `translateX(${Math.max(-80, Math.min(80, dx))}px)`
      }}
      onTouchEnd={async e => {
        e.currentTarget.style.transition = 'transform .15s'
        e.currentTarget.style.transform = ''
        if (!touch.current.moved) return
        const dx = e.changedTouches[0].clientX - touch.current.x
        if (dx < -60) { await api.trash(m.id); load() }
        else if (dx > 60) { await api.patch(m.id, { read: !m.read }); load() }
      }}
      className={cn('cv-auto relative w-full text-left px-3 pl-4 py-2.5 border-b border-border hover:bg-muted/60 transition-colors cursor-pointer',
        (selectMode ? checked.includes(m.id) : sel?.id === m.id) && 'bg-muted', !m.read && 'bg-primary/5')}>
      {!m.read && !selectMode && <span className="absolute left-0 top-0 bottom-0 w-[3px] bg-primary" aria-hidden="true" />}
      <div className="flex items-center gap-2">
        {selectMode && <input type="checkbox" readOnly checked={checked.includes(m.id)} className="pointer-events-none shrink-0" />}
        {!selectMode && (
          <span className={cn('grid place-items-center size-7 shrink-0 rounded-full text-[11px] font-semibold uppercase',
            m.read ? 'bg-muted text-muted-foreground' : 'bg-primary/15 text-primary')} aria-hidden="true">
            {((folder === 'sent' ? m.to : m.from) || '?').trim().slice(0, 1)}
          </span>
        )}
        <span className={cn('truncate flex-1 text-sm', !m.read ? 'font-semibold' : 'text-foreground')}>
          {m.subject || t('mail.noSubject')}
        </span>
        <SendStatus m={m} folder={folder} />
        {m.thread_count > 1 && <span className="shrink-0 rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{t('mail.threadN', { n: m.thread_count })}</span>}
        {attList(m.attachments).length > 0 && <Paperclip size={12} className="text-muted-foreground shrink-0" />}
        <span className="text-[10px] text-muted-foreground shrink-0">{fmtWhen(m.created_at)}</span>
        {!selectMode && (
          <button type="button" aria-label={m.starred ? t('mail.unstar') : t('mail.star')}
            className="-mr-1.5 rounded p-1.5 hover:bg-muted"
            onClick={async e => { e.stopPropagation(); await api.patch(m.id, { starred: !m.starred }); load() }}>
            <Star size={14} className={cn(m.starred ? 'fill-yellow-400 text-yellow-400' : 'text-muted-foreground')} />
          </button>
        )}
      </div>
      <div className="mt-0.5">
        <span className="text-xs text-muted-foreground line-clamp-1">{m.body?.slice(0, 80)}</span>
      </div>
      <div className="flex items-center justify-between gap-2 mt-0.5">
        <span className={cn('flex-1 min-w-0 truncate text-xs', !emphasisOnRecipient && !m.read ? strong : dim)} title={m.from}>
          {m.from}
        </span>
        <span className="flex items-center gap-1.5 min-w-0 max-w-[55%]" title={m.to}>
          {showRecipientChip && (() => {
            const rc = recipientChip(m.to, me?.email)
            return rc ? (
              <span className="shrink-0 max-w-[45%] truncate rounded px-1.5 py-0.5 text-[10px] font-medium text-white" style={{ background: rc.color }}>
                {rc.label}
              </span>
            ) : null
          })()}
          <span className={cn('truncate text-xs', emphasisOnRecipient && !m.read ? strong : dim)}>
            {m.to}
          </span>
        </span>
      </div>
    </div>
  )
}
