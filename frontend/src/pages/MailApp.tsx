import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Textarea, Card, Badge } from '../components/ui/controls'
import { ThemeToggle, LangToggle } from '../components/HeaderControls'
import { useI18n } from '../lib/i18n'
import {
  Inbox, Send, FileEdit, Trash2, Star, Search, PenLine, LogOut,
  RefreshCw, Globe, Settings, ShieldCheck, ArrowLeft, Loader2,
} from 'lucide-react'
import { cn } from '../lib/utils'

const FOLDERS = [
  { k: 'inbox', labelKey: 'mail.inbox', icon: Inbox },
  { k: 'sent', labelKey: 'mail.sent', icon: Send },
  { k: 'draft', labelKey: 'mail.draft', icon: FileEdit },
  { k: 'trash', labelKey: 'mail.trash', icon: Trash2 },
]

export default function MailApp() {
  const { t } = useI18n()
  const [folder, setFolder] = useState('inbox')
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(null)
  const [showCompose, setShowCompose] = useState<any>(false)
  const [me, setMe] = useState(null)
  const [view, setView] = useState('list') // 移动端：list | read

  async function load() {
    try {
      const d = await api.list(folder, q)
      setItems(d.items); setTotal(d.total)
    } catch {}
  }
  useEffect(() => { api.me().then(setMe).catch(() => location.hash = '#/login'); }, [])
  useEffect(() => { setSel(null); setView('list'); load() }, [folder])

  async function open(id) {
    const d = await api.get(id)
    setSel(d)
    setView('read')
    setItems(items.map(i => i.id === id ? { ...i, read: true } : i))
  }

  function logout() { localStorage.removeItem('token'); location.hash = '#/login' }

  return (
    <div className="min-h-screen bg-background flex flex-col">
      <header className="border-b border-border px-3 sm:px-4 h-14 flex items-center gap-2 sticky top-0 bg-background/90 backdrop-blur z-10">
        <b className="hidden sm:inline">📮 Mailserver</b>
        <b className="sm:hidden">📮</b>
        {me && <Badge className="max-w-[38vw] truncate">{me.email}</Badge>}
        <div className="flex-1" />
        <a href="#/dns" title={t('nav.dns')}><Button variant="ghost" size="icon" aria-label={t('nav.dns')}><Globe /></Button></a>
        <a href="#/setup" title={t('nav.setup')} className="hidden sm:block"><Button variant="ghost" size="icon" aria-label={t('nav.setup')}><Settings /></Button></a>
        {me?.admin && <a href="#/admin" title={t('nav.admin')}><Button variant="ghost" size="icon" aria-label={t('nav.admin')}><ShieldCheck /></Button></a>}
        <div className="hidden sm:flex items-center">
          <LangToggle />
          <ThemeToggle />
        </div>
        <Button variant="ghost" size="icon" onClick={load} aria-label={t('common.refresh')}><RefreshCw /></Button>
        <Button variant="ghost" size="icon" onClick={logout} aria-label={t('nav.logout')}><LogOut /></Button>
        <Button size="sm" onClick={() => setShowCompose(true)}><PenLine /><span className="hidden sm:inline">{t('mail.compose')}</span></Button>
      </header>

      {/* 移动端：文件夹横向标签 */}
      <div className="md:hidden flex gap-1 overflow-x-auto border-b border-border px-2 py-2">
        {FOLDERS.map(f => (
          <button key={f.k} onClick={() => setFolder(f.k)}
            className={cn('flex items-center gap-1 rounded-md px-3 py-1.5 text-sm whitespace-nowrap',
              folder === f.k ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
            <f.icon size={15} />{t(f.labelKey)}
          </button>
        ))}
      </div>

      <div className="flex-1 flex max-w-6xl w-full mx-auto min-h-0">
        <aside className="hidden md:block w-44 shrink-0 p-3 space-y-1 border-r border-border">
          {FOLDERS.map(f => (
            <button key={f.k} onClick={() => setFolder(f.k)}
              className={cn('w-full flex items-center gap-2 rounded-md px-3 py-2 text-sm', folder === f.k ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
              <f.icon size={16} />{t(f.labelKey)}
            </button>
          ))}
          <p className="text-xs text-muted-foreground px-3 pt-4">{t('mail.total', { n: total })}</p>
        </aside>

        <section className="flex-1 min-w-0 flex">
          {/* 列表 */}
          <div className={cn('w-full md:w-80 md:shrink-0 border-r border-border flex-col',
            view === 'read' ? 'hidden md:flex' : 'flex')}>
            <div className="p-3 border-b border-border flex gap-2">
              <div className="relative flex-1">
                <Search size={14} className="absolute left-2 top-2.5 text-muted-foreground" />
                <Input className="pl-7" placeholder={t('mail.search')} value={q}
                  onChange={e => setQ(e.target.value)} onKeyDown={e => e.key === 'Enter' && load()} />
              </div>
            </div>
            <div className="flex-1 overflow-auto">
              {items.map(m => (
                <button key={m.id} onClick={() => open(m.id)}
                  className={cn('w-full text-left px-3 py-2.5 border-b border-border hover:bg-muted/60',
                    sel?.id === m.id && 'bg-muted', !m.read && 'font-semibold')}>
                  <div className="flex items-center gap-2 text-sm">
                    <span className="truncate flex-1">{folder === 'sent' ? m.to : m.from}</span>
                    <Star size={14} className={m.starred ? 'fill-yellow-400 text-yellow-400' : 'text-muted-foreground'}
                      onClick={async e => { e.stopPropagation(); await api.patch(m.id, { starred: !m.starred }); load() }} />
                  </div>
                  <div className="text-sm truncate">{m.subject || t('mail.noSubject')}</div>
                  <div className="text-xs text-muted-foreground truncate">{m.body?.slice(0, 60)}</div>
                </button>
              ))}
              {items.length === 0 && <p className="p-6 text-sm text-muted-foreground text-center">{t('mail.empty')}</p>}
            </div>
          </div>

          {/* 详情 */}
          <div className={cn('flex-1 min-w-0 p-4', view === 'list' && 'hidden md:block')}>
            {!sel ? (
              <p className="text-muted-foreground text-sm mt-10 text-center">{t('mail.selectHint')}</p>
            ) : (
              <>
                <button onClick={() => setView('list')} className="md:hidden mb-3 flex items-center gap-1 text-sm text-muted-foreground">
                  <ArrowLeft size={16} />{t('common.back')}
                </button>
                <Card className="p-4 sm:p-5">
                  <h2 className="text-lg font-semibold break-words">{sel.subject || t('mail.noSubject')}</h2>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t('mail.fromTo', { from: sel.from, to: sel.to })} · {new Date(sel.created_at).toLocaleString()}
                  </p>
                  <div className="flex flex-wrap gap-2 mt-3">
                    <Button variant="outline" size="sm" onClick={async () => { await api.patch(sel.id, { starred: !sel.starred }); setSel({ ...sel, starred: !sel.starred }) }}>
                      {sel.starred ? t('mail.unstar') : t('mail.star')}
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => { setShowCompose({ to: sel.from, subject: 'Re: ' + sel.subject, body: '\n\n---\n' + sel.body }) }}>{t('mail.reply')}</Button>
                    <Button variant="outline" size="sm" onClick={async () => { await api.trash(sel.id); setSel(null); setView('list'); load() }}>{t('mail.delete')}</Button>
                  </div>
                  <pre className="whitespace-pre-wrap text-sm mt-4 font-sans break-words">{sel.body}</pre>
                </Card>
              </>
            )}
          </div>
        </section>
      </div>

      {showCompose && <Compose init={typeof showCompose === 'object' ? showCompose : {}} onClose={() => { setShowCompose(false); load() }} />}
    </div>
  )
}

function Compose({ init, onClose }) {
  const { t } = useI18n()
  const [f, setF] = useState({ to: init.to || '', subject: init.subject || '', body: init.body || '' })
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    const onKey = e => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])
  async function submit(folder) {
    setSaving(true)
    try { await api.send({ ...f, folder }); onClose() } catch (e) { alert(e.message) }
    finally { setSaving(false) }
  }
  return (
    <div className="fixed inset-0 bg-black/40 grid place-items-center sm:p-4 z-50" onClick={onClose} role="dialog" aria-modal="true" aria-label={t('mail.compose')}>
      <Card className="w-full h-full sm:h-auto sm:max-w-lg p-4 space-y-3 rounded-none sm:rounded-lg" onClick={e => e.stopPropagation()}>
        <b>{t('mail.compose')}</b>
        <Input autoFocus placeholder={t('mail.to')} value={f.to} onChange={e => setF({ ...f, to: e.target.value })} />
        <Input placeholder={t('mail.subject')} value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} />
        <Textarea rows={10} placeholder={t('mail.body')} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} />
        <div className="flex gap-2 justify-end">
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button variant="outline" disabled={saving} onClick={() => submit('draft')}>{t('mail.saveDraft')}</Button>
          <Button disabled={saving} onClick={() => submit('sent')}>
            {saving ? <Loader2 className="animate-spin" /> : null}{t('mail.send')}
          </Button>
        </div>
      </Card>
    </div>
  )
}
