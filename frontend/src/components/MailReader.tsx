import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { confirmDestructive } from '../lib/ui'
import { Button, Card } from './ui/controls'
import { Dropdown, DropdownItem, DropdownSeparator } from './Dropdown'
import { useI18n } from '../lib/i18n'
import { cn, linkify } from '../lib/utils'
import { recipientChip, attList, attKind, fmtSize, fmtWhen } from '../lib/mailFormat'
import { AuthBadges } from './MailStatus'
import { ArrowLeft, MoreVertical, Star, MailOpen, Reply, ReplyAll, Forward, Trash2, XCircle, Clock, Download } from 'lucide-react'

// 邮件阅读区（详情面板）。从 MailApp 拆出，降低单文件复杂度。
export default function MailReader({
  sel, setSel, items, setItems, load, folder, thread, me,
  open, reply, forwardMail, showImages, revealImages, bodyRef, onBodyClick, setPreview,
}: any) {
  const { t } = useI18n()
  const navigate = useNavigate()
  return (
      <>
        <button onClick={() => navigate('/')} className="md:hidden mb-3 flex items-center gap-1 text-sm text-muted-foreground">
          <ArrowLeft size={16} />{t('common.back')}
        </button>
        {thread.length > 1 && (
          <div className="mb-3 rounded-md border border-border divide-y divide-border text-sm overflow-hidden">
            {thread.map((tm: any) => (
              <button key={tm.id} onClick={() => open(tm.id)}
                className={cn('w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-muted', tm.id === sel.id && 'bg-muted')}>
                <span className="truncate flex-1">{tm.subject || t('mail.noSubject')}</span>
                <span className="text-xs text-muted-foreground shrink-0 max-w-[40%] truncate">{tm.from}</span>
                <span className="text-[10px] text-muted-foreground shrink-0">{fmtWhen(tm.created_at)}</span>
              </button>
            ))}
          </div>
        )}
        <Card className="p-4 sm:p-5">
          <div className="flex items-start gap-3">
            <div className="min-w-0 flex-1">
              <h2 className="text-lg font-semibold break-words">{sel.subject || t('mail.noSubject')}</h2>
              <p className="text-xs text-muted-foreground mt-1">
                {t('mail.fromTo', { from: sel.from, to: sel.to })} · {new Date(sel.created_at).toLocaleString()}
              </p>
              {(() => {
                const rc = recipientChip(sel.to, me?.email)
                return rc ? <span className="inline-block mt-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-white" style={{ background: rc.color }} title={sel.to}>{rc.label}</span> : null
              })()}
              {sel.cc && <p className="text-xs text-muted-foreground mt-0.5">Cc: {sel.cc}</p>}
              <AuthBadges raw={sel.auth_results} t={t} />
              {sel.status === 'failed' && sel.relay_err && (
                <div className="mt-2 rounded-md border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive" role="alert">
                  <b>{t('mail.stFailed')}:</b> {sel.relay_err}
                </div>
              )}
            </div>
            <Dropdown align="right" trigger={
              <Button variant="ghost" size="icon" aria-label={t('mail.actions')}><MoreVertical /></Button>
            }>
              <DropdownItem icon={Star} onClick={async () => { await api.patch(sel.id, { starred: !sel.starred }); setSel({ ...sel, starred: !sel.starred }) }}>
                {sel.starred ? t('mail.unstar') : t('mail.star')}
              </DropdownItem>
              <DropdownItem icon={MailOpen} onClick={async () => {
                const next = !sel.read
                await api.patch(sel.id, { read: next })
                setSel({ ...sel, read: next })
                setItems(items.map((i: any) => i.id === sel.id ? { ...i, read: next } : i))
              }}>{sel.read ? t('mail.markUnread') : t('mail.markRead')}</DropdownItem>
              <DropdownSeparator />
              <DropdownItem icon={Reply} onClick={() => reply(sel, false)}>{t('mail.reply')}</DropdownItem>
              <DropdownItem icon={ReplyAll} onClick={() => reply(sel, true)}>{t('mail.replyAll')}</DropdownItem>
              <DropdownItem icon={Forward} onClick={() => forwardMail(sel)}>{t('mail.forward')}</DropdownItem>
              <DropdownSeparator />
              <DropdownItem icon={Trash2} className="text-destructive hover:bg-destructive/10" onClick={async () => {
                if ((folder === 'trash' || folder === 'deleted') && !await confirmDestructive(t('mail.confirmPurge'))) return
                await api.trash(sel.id); setSel(null); navigate('/'); load()
              }}>{folder === 'deleted' ? t('mail.purge') : t('mail.delete')}</DropdownItem>
            </Dropdown>
          </div>
          {folder === 'sent' && sel.status && sel.status !== 'sent' && (
            <div className={cn('mt-4 rounded-md border p-3 text-sm', sel.status === 'failed' ? 'border-destructive/40 bg-destructive/10 text-destructive' : 'border-border bg-muted/50 text-muted-foreground')}>
              <div className="flex items-center gap-1.5 font-medium">
                {sel.status === 'failed' ? <XCircle size={14} /> : <Clock size={14} />}
                {sel.status === 'failed' ? t('mail.stFailed') : sel.status === 'sending' ? t('mail.stSending') : t('mail.stQueued')}
              </div>
              {sel.relay_err && <p className="mt-1 text-xs break-words">{sel.relay_err}</p>}
              {sel.status === 'failed' && <p className="mt-1 text-xs">{t('mail.failedHint')}</p>}
            </div>
          )}
          {sel.body_html ? (
            <div className="mt-4 text-sm break-words">
              {!showImages && String(sel.body_html).includes('data-blocked-src') && (
                <button className="text-xs text-primary underline mb-2" onClick={revealImages}>{t('mail.showImages')}</button>
              )}
              <div ref={bodyRef} onClick={onBodyClick}
                className={cn(
                  'max-w-full overflow-x-auto',
                  '[&_img]:max-w-full [&_img]:h-auto [&_a]:text-primary [&_a]:underline',
                  '[&_table]:max-w-full [&_table]:w-auto [&_td]:w-auto [&_th]:w-auto',
                  '[&_pre]:max-w-full [&_pre]:overflow-x-auto',
                  '[&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground',
                  '[&_*]:break-words',
                )}
                dangerouslySetInnerHTML={{ __html: sel.body_html }} />
            </div>
          ) : (
            <pre onClick={onBodyClick} className="whitespace-pre-wrap text-sm mt-4 font-sans break-words max-w-full overflow-x-auto" dangerouslySetInnerHTML={{ __html: linkify(sel.body) }} />
          )}
          {attList(sel.attachments).length > 0 && (
            <div className="mt-4 border-t border-border pt-3">
              <b className="text-sm">{t('mail.attachments')}</b>
              <div className="flex flex-wrap gap-2 mt-2">
                {attList(sel.attachments).map((a: any, i: number) => {
                  const dataUrl = `data:${a.type || 'application/octet-stream'};base64,${a.data}`
                  const k = attKind(a.type)
                  return (
                    <div key={i} className="text-xs rounded-md border border-border px-2 py-1 hover:bg-muted flex items-center gap-1.5">
                      {k
                        ? <button className="hover:underline" onClick={() => setPreview({ ...a, dataUrl, kind: k })}>👁 {a.name}</button>
                        : <span>📎 {a.name}</span>}
                      <span className="text-muted-foreground">({fmtSize(a.size)})</span>
                      <a href={dataUrl} download={a.name} aria-label={t('mail.download')} className="text-muted-foreground hover:text-foreground"><Download size={12} /></a>
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </Card>
      </>
  )
}
