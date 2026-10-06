import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import { toast, confirmAsync } from '../lib/ui'
import { Button, Input, Textarea, Card } from './ui/controls'
import { RecipientInput } from './RecipientInput'
import { useI18n } from '../lib/i18n'
import { fmtSize } from '../lib/mailFormat'
import { Paperclip, X, Clock, Loader2 } from 'lucide-react'

// 写信弹窗：草稿自动保存、附件、抄送、定时/周期发送。从 MailApp 拆出。
export default function Compose({ me, init, onClose }: any) {
  const { t } = useI18n()
  const [f, setF] = useState({ from: init.from || '', to: init.to || '', cc: init.cc || '', bcc: init.bcc || '', subject: init.subject || '', body: init.body || (me?.signature ? '\n\n-- \n' + me.signature : '') })
  const [atts, setAtts] = useState<any[]>([])
  const [showCC, setShowCC] = useState(!!(init.cc || init.bcc))
  const [saving, setSaving] = useState(false)
  const [draftId, setDraftId] = useState<number | null>(null)
  const [sugg, setSugg] = useState<any[]>([])
  const [ids, setIds] = useState<any[]>([])
  const [schedOpen, setSchedOpen] = useState(false)
  const [sendAt, setSendAt] = useState('')
  const [repeat, setRepeat] = useState('')
  const [savedAt, setSavedAt] = useState<Date | null>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  const dirtyRef = useRef(false)
  useEffect(() => { api.external().then((xs: any[]) => setIds(xs.filter(x => x.enabled))).catch(() => {}) }, [])
  // 草稿自动保存（25s 一次，有内容才存）
  useEffect(() => {
    const id = setInterval(async () => {
      if (saving) return
      if (!f.to && !f.subject && !f.body.trim()) return
      try {
        if (draftId) await api.patch(draftId, { to: f.to, cc: f.cc, bcc: f.bcc, subject: f.subject, body: f.body })
        else { const r = await api.send({ ...f, folder: 'draft' }); setDraftId(r.id) }
        setSavedAt(new Date())
      } catch {}
    }, 25000)
    return () => clearInterval(id)
  }, [f, draftId, saving])
  useEffect(() => {
    Promise.all([api.contacts().catch(() => []), api.directory().catch(() => [])]).then(([cs, dir]) => {
      const map = new Map<string, any>()
      for (const d of dir) map.set(d.email.toLowerCase(), { email: d.email, name: d.name })
      for (const c of cs) map.set(c.email.toLowerCase(), { email: c.email, name: c.name, note: c.note })
      setSugg([...map.values()])
    })
  }, [])
  useEffect(() => {
    const onKey = async (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (dirtyRef.current && !(await confirmAsync(t('mail.discardDraft')))) return
      onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose, t])

  const total = atts.reduce((s, a) => s + (a.size || 0), 0)
  dirtyRef.current = !!(f.to || f.subject || f.body.trim() || atts.length)

  // 关闭前确认：有未保存内容时二次确认（#39）。
  async function requestClose() {
    if (dirtyRef.current && !(await confirmAsync(t('mail.discardDraft')))) return
    onClose()
  }

  function onFiles(e: any) {
    const files: File[] = Array.from(e.target.files || [])
    let cur = total
    for (const file of files) {
      if (cur + file.size > 8 * 1024 * 1024) { toast(t('mail.tooLarge')); break }
      cur += file.size
      const fr = new FileReader()
      fr.onload = () => {
        const data = String(fr.result || '').split(',')[1] || ''
        setAtts(a => [...a, { name: file.name, type: file.type || 'application/octet-stream', data, size: file.size }])
      }
      fr.readAsDataURL(file)
    }
    e.target.value = ''
  }

  async function submit(folder: string) {
    setSaving(true)
    try {
      const body: any = { ...f, attachments: atts, folder }
      const scheduling = folder === 'sent' && schedOpen && !!sendAt
      if (scheduling) {
        const d = new Date(sendAt)
        if (isNaN(d.getTime())) { toast(t('mail.badSchedule'), { type: 'error' }); setSaving(false); return }
        body.send_at = d.toISOString()
        body.repeat = repeat
      }
      const r = await api.send(body)
      if (draftId) api.batch([draftId], 'purge').catch(() => {})
      if (scheduling) {
        toast(t('mail.scheduledToast', { when: new Date(sendAt).toLocaleString() }), { type: 'success' })
      } else if (folder === 'sent') {
        toast(t('mail.sentToast'), {
          type: 'success',
          action: { label: t('mail.undo'), onClick: () => { api.undoSend(r.id).then(() => toast(t('mail.undoOk'))).catch(() => toast(t('mail.undoFail'), { type: 'error' })) } },
        })
      }
      onClose()
    } catch (e: any) { toast(e.message, { type: 'error' }) }
    finally { setSaving(false) }
  }

  return (
    <div className="fixed inset-0 bg-black/40 z-50 flex items-end justify-center sm:items-center sm:p-4" onClick={requestClose} role="dialog" aria-modal="true" aria-label={t('mail.compose')}>
      <Card ref={cardRef} className="w-full max-h-[92dvh] sm:max-h-[88vh] sm:max-w-lg p-4 rounded-t-2xl sm:rounded-lg flex flex-col"
        onClick={(e: any) => e.stopPropagation()}
        onKeyDown={(e: any) => {
          if (e.key !== 'Tab') return
          const nodes = cardRef.current?.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])')
          const list = nodes ? Array.from(nodes).filter(n => n.offsetParent !== null) : []
          if (!list.length) return
          const first = list[0], last = list[list.length - 1]
          if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
          else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
        }}>
        <div className="flex items-center gap-2">
          <b>{t('mail.compose')}</b>
          <span className="text-[11px] text-muted-foreground" aria-live="polite">
            {saving ? t('mail.draftSaving') : savedAt ? t('mail.draftSaved', { time: savedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) }) : ''}
          </span>
          <div className="flex-1" />
          <Button variant="ghost" size="sm" onClick={() => setShowCC(v => !v)}>{t('mail.ccBcc')}</Button>
        </div>
        <div className="flex-1 min-h-0 overflow-auto space-y-3 pt-3">
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground shrink-0">{t('mail.from')}</span>
          <select className="h-9 flex-1 rounded-md border border-border bg-background text-sm px-2" value={f.from} onChange={e => setF({ ...f, from: e.target.value })}>
            <option value="">{me?.email || ''}</option>
            {ids.map((a: any) => <option key={a.id} value={a.email}>{a.name ? `${a.name} <${a.email}>` : a.email}</option>)}
          </select>
        </div>
        <RecipientInput autoFocus placeholder={t('mail.to')} value={f.to} onChange={v => setF({ ...f, to: v })} suggestions={sugg} />
        {showCC && <>
          <RecipientInput placeholder={t('mail.cc')} value={f.cc} onChange={v => setF({ ...f, cc: v })} suggestions={sugg} />
          <RecipientInput placeholder={t('mail.bcc')} value={f.bcc} onChange={v => setF({ ...f, bcc: v })} suggestions={sugg} />
        </>}
        <Input placeholder={t('mail.subject')} value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} />
        <Textarea rows={8} placeholder={t('mail.body')} value={f.body} onChange={e => setF({ ...f, body: e.target.value })}
          onKeyDown={(e: any) => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); if (!saving) submit('sent') } }} />
        {atts.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {atts.map((a, i) => (
              <span key={i} className="inline-flex items-center gap-1 text-xs rounded-md border border-border px-2 py-1">
                📎 {a.name} ({fmtSize(a.size)})
                <button onClick={() => setAtts(atts.filter((_, j) => j !== i))} aria-label={t('common.delete')}><X size={12} /></button>
              </span>
            ))}
          </div>
        )}
        {schedOpen && (
          <div className="rounded-md border border-border p-2 space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground shrink-0">{t('mail.sendAt')}</span>
              <Input type="datetime-local" className="h-8 text-xs" value={sendAt} onChange={e => setSendAt(e.target.value)} />
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground shrink-0">{t('mail.repeat')}</span>
              <select className="h-8 flex-1 rounded-md border border-border bg-background text-xs px-2" value={repeat} onChange={e => setRepeat(e.target.value)}>
                <option value="">{t('mail.repeatNone')}</option>
                <option value="daily">{t('mail.repeatDaily')}</option>
                <option value="weekly">{t('mail.repeatWeekly')}</option>
                <option value="monthly">{t('mail.repeatMonthly')}</option>
              </select>
            </div>
          </div>
        )}
        </div>
        <div className="mt-auto flex flex-wrap items-center gap-2 pt-3 border-t border-border safe-bottom">
          <label className="inline-flex items-center gap-1 text-sm cursor-pointer text-muted-foreground hover:text-foreground">
            <Paperclip size={16} />{t('mail.attach')}
            <input type="file" multiple className="hidden" onChange={onFiles} />
          </label>
          <Button variant={schedOpen ? 'default' : 'outline'} size="sm" type="button" onClick={() => setSchedOpen(v => !v)}><Clock />{t('mail.schedule')}</Button>
          <div className="flex-1" />
          <Button variant="outline" onClick={requestClose}>{t('common.cancel')}</Button>
          <Button variant="outline" disabled={saving} onClick={() => submit('draft')}>{t('mail.saveDraft')}</Button>
          <Button disabled={saving} onClick={() => submit('sent')}>
            {saving ? <Loader2 className="animate-spin" /> : null}{schedOpen && sendAt ? t('mail.scheduleSend') : t('mail.send')}
          </Button>
        </div>
      </Card>
    </div>
  )
}
