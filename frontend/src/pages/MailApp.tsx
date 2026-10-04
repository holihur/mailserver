import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Textarea, Card } from '../components/ui/controls'
import { Dropdown, DropdownItem, DropdownSeparator, DropdownLabel } from '../components/Dropdown'
import { RecipientInput } from '../components/RecipientInput'
import { SkeletonList } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/theme'
import {
  Inbox, Send, FileEdit, Trash2, Star, Search, PenLine, LogOut,
  RefreshCw, Globe, Settings, ShieldCheck, ArrowLeft, Loader2, Paperclip, X,
  ChevronDown, Languages, Sun, Moon, Monitor, MoreVertical, Reply,
  Contact, Filter, KeyRound,
} from 'lucide-react'
import { cn } from '../lib/utils'
import { BRAND } from '../lib/brand'

const FOLDERS = [
  { k: 'inbox', labelKey: 'mail.inbox', icon: Inbox },
  { k: 'sent', labelKey: 'mail.sent', icon: Send },
  { k: 'draft', labelKey: 'mail.draft', icon: FileEdit },
  { k: 'trash', labelKey: 'mail.trash', icon: Trash2 },
]

export default function MailApp() {
  const { t, lang, setLang } = useI18n()
  const navigate = useNavigate()
  const { theme, cycle: cycleTheme } = useTheme()
  const ThemeIcon = theme === 'light' ? Sun : theme === 'dark' ? Moon : Monitor
  const [folder, setFolder] = useState('inbox')
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [total, setTotal] = useState(0)
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(null)
  const [showCompose, setShowCompose] = useState<any>(false)
  const [me, setMe] = useState(null)
  const [view, setView] = useState('list') // 移动端：list | read
  const [page, setPage] = useState(1)
  const [sort, setSort] = useState('newest')
  const pageSize = 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  async function load(p = page, s = sort) {
    setLoading(true)
    try {
      const d = await api.list(folder, q, p, s)
      setItems(d.items); setTotal(d.total); setPage(d.page || p)
    } catch {} finally { setLoading(false) }
  }
  useEffect(() => { api.me().then(setMe).catch(() => { location.href = '/login' }) }, [])
  useEffect(() => { setSel(null); setView('list'); setPage(1); load(1, sort) }, [folder])

  const [selectMode, setSelectMode] = useState(false)
  const [checked, setChecked] = useState<number[]>([])
  function toggleCheck(id: number) { setChecked(c => c.includes(id) ? c.filter(x => x !== id) : [...c, id]) }
  function toggleAll() { setChecked(c => c.length === items.length ? [] : items.map((m: any) => m.id)) }
  async function batch(action: string) {
    if (!checked.length) return
    try { await api.batch(checked, action) } catch (e: any) { alert(e.message) }
    setChecked([]); setSelectMode(false); load(page, sort)
  }

  async function emptyTrash() {
    if (!confirm(t('mail.confirmPurge'))) return
    try { await api.emptyTrash() } catch (e: any) { alert(e.message) }
    setChecked([]); setSelectMode(false); setSel(null); load(1, sort)
  }

  async function open(id) {
    const d = await api.get(id)
    setSel(d)
    setView('read')
    setItems(items.map(i => i.id === id ? { ...i, read: true } : i))
  }

  function logout() { localStorage.removeItem('token'); location.assign('/login') }

  return (
    <div className="min-h-screen bg-background flex flex-col">
      <header className="border-b border-border px-3 sm:px-4 h-14 flex items-center gap-2 sticky top-0 bg-background/90 backdrop-blur z-10">
        <b className="hidden sm:inline">📮 {BRAND}</b>
        <b className="sm:hidden">📮</b>
        <div className="flex-1" />
        <Button size="sm" onClick={() => setShowCompose(true)}><PenLine /><span className="hidden sm:inline">{t('mail.compose')}</span></Button>
        <Dropdown align="right" trigger={
          <Button variant="ghost" size="sm" className="gap-1.5 px-1.5" aria-label={t('nav.account')}>
            <span className="grid place-items-center size-7 rounded-full bg-primary text-primary-foreground text-xs font-semibold">
              {((me?.email || '?')[0] || '?').toUpperCase()}
            </span>
            <ChevronDown size={15} className="text-muted-foreground" />
          </Button>
        }>
          {me?.email && <DropdownLabel>{me.email}</DropdownLabel>}
          <DropdownSeparator />
          <DropdownItem icon={Settings} onClick={() => navigate('/setup')}>{t('nav.setup')}</DropdownItem>
          <DropdownItem icon={Contact} onClick={() => navigate('/contacts')}>{t('nav.contacts')}</DropdownItem>
          <DropdownItem icon={Filter} onClick={() => navigate('/rules')}>{t('nav.rules')}</DropdownItem>
          <DropdownItem icon={KeyRound} onClick={() => navigate('/security')}>{t('nav.security')}</DropdownItem>
          <DropdownItem icon={ShieldCheck} onClick={() => navigate('/privacy')}>{t('nav.privacy')}</DropdownItem>
          {me?.admin && <DropdownItem icon={Globe} onClick={() => navigate('/dns')}>{t('nav.dns')}</DropdownItem>}
          {me?.admin && <DropdownItem icon={ShieldCheck} onClick={() => navigate('/admin')}>{t('nav.admin')}</DropdownItem>}
          <DropdownSeparator />
          <DropdownItem icon={Languages} onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')}>
            {lang === 'zh' ? 'English' : '中文'}
          </DropdownItem>
          <DropdownItem icon={ThemeIcon} onClick={cycleTheme}>{t('theme.' + theme)}</DropdownItem>
          <DropdownSeparator />
          <DropdownItem icon={RefreshCw} onClick={() => load()}>{t('common.refresh')}</DropdownItem>
          <DropdownItem icon={LogOut} onClick={logout} className="text-red-500 hover:bg-red-500/10">{t('nav.logout')}</DropdownItem>
        </Dropdown>
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
                  onChange={e => setQ(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') { setPage(1); load(1, sort) } }} />
              </div>
              <select value={sort} onChange={e => { setSort(e.target.value); setPage(1); load(1, e.target.value) }}
                className="h-9 rounded-md border border-border bg-background text-xs px-2" aria-label={t('mail.sort')}>
                <option value="newest">{t('mail.sortNewest')}</option>
                <option value="oldest">{t('mail.sortOldest')}</option>
                <option value="subject">{t('mail.sortSubject')}</option>
                <option value="sender">{t('mail.sortSender')}</option>
              </select>
              <Button variant={selectMode ? 'default' : 'outline'} size="sm" className="shrink-0"
                onClick={() => { setSelectMode(v => !v); setChecked([]) }}>
                {selectMode ? t('mail.done') : t('mail.batch')}
              </Button>
              {folder === 'trash' && total > 0 && (
                <Button variant="outline" size="sm" className="shrink-0" onClick={emptyTrash}>
                  <Trash2 />{t('mail.emptyTrash')}
                </Button>
              )}
            </div>
            {selectMode && (
              <div className="p-2 border-b border-border flex items-center gap-2 text-xs">
                <label className="flex items-center gap-1"><input type="checkbox" checked={checked.length > 0 && checked.length === items.length} onChange={toggleAll} />{t('mail.selectAll')}</label>
                <span className="text-muted-foreground">{t('mail.selected', { n: checked.length })}</span>
                <div className="flex-1" />
                <Button variant="ghost" size="sm" disabled={!checked.length} onClick={() => batch('star')}><Star />{t('mail.batchStar')}</Button>
                <Button variant="ghost" size="sm" disabled={!checked.length} onClick={() => {
                  if (folder === 'trash' && !confirm(t('mail.confirmPurge'))) return
                  batch(folder === 'trash' ? 'delete' : 'trash')
                }}><Trash2 />{folder === 'trash' ? t('mail.purge') : t('mail.batchDelete')}</Button>
              </div>
            )}
            <div className="flex-1 overflow-auto">
              {loading && <SkeletonList rows={6} />}
              {!loading && items.map(m => (
                <button key={m.id} onClick={() => selectMode ? toggleCheck(m.id) : open(m.id)}
                  className={cn('w-full text-left px-3 py-2.5 border-b border-border hover:bg-muted/60',
                    (selectMode ? checked.includes(m.id) : sel?.id === m.id) && 'bg-muted', !m.read && 'font-semibold')}>
                  <div className="flex items-center gap-2 text-sm">
                    {selectMode && <input type="checkbox" readOnly checked={checked.includes(m.id)} className="pointer-events-none" />}
                    <span className="truncate flex-1">{folder === 'sent' ? m.to : m.from}</span>
                    {!selectMode && <Star size={14} className={m.starred ? 'fill-yellow-400 text-yellow-400' : 'text-muted-foreground'}
                      onClick={async e => { e.stopPropagation(); await api.patch(m.id, { starred: !m.starred }); load() }} />}
                  </div>
                  <div className="text-sm truncate">{m.subject || t('mail.noSubject')}</div>
                  <div className="text-xs text-muted-foreground truncate">{m.body?.slice(0, 60)}</div>
                </button>
              ))}
              {!loading && items.length === 0 && <p className="p-6 text-sm text-muted-foreground text-center">{t('mail.empty')}</p>}
            </div>
            <div className="border-t border-border p-2 flex items-center justify-between text-xs">
              <Button variant="ghost" size="sm" disabled={page <= 1} onClick={() => load(page - 1, sort)}>‹ {t('mail.prev')}</Button>
              <span className="text-muted-foreground">{page} / {totalPages}</span>
              <Button variant="ghost" size="sm" disabled={page >= totalPages} onClick={() => load(page + 1, sort)}>{t('mail.next')} ›</Button>
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
                  {sel.cc && <p className="text-xs text-muted-foreground mt-0.5">Cc: {sel.cc}</p>}
                  <div className="mt-3">
                    <Dropdown align="left" trigger={
                      <Button variant="outline" size="sm"><MoreVertical />{t('mail.actions')}</Button>
                    }>
                      <DropdownItem icon={Star} onClick={async () => { await api.patch(sel.id, { starred: !sel.starred }); setSel({ ...sel, starred: !sel.starred }) }}>
                        {sel.starred ? t('mail.unstar') : t('mail.star')}
                      </DropdownItem>
                      <DropdownItem icon={Reply} onClick={() => { setShowCompose({ to: sel.from, subject: 'Re: ' + sel.subject, body: '\n\n---\n' + sel.body }) }}>{t('mail.reply')}</DropdownItem>
                      <DropdownSeparator />
                      <DropdownItem icon={Trash2} className="text-red-500 hover:bg-red-500/10" onClick={async () => {
                        if (folder === 'trash' && !confirm(t('mail.confirmPurge'))) return
                        await api.trash(sel.id); setSel(null); setView('list'); load()
                      }}>{folder === 'trash' ? t('mail.purge') : t('mail.delete')}</DropdownItem>
                    </Dropdown>
                  </div>
                  <pre className="whitespace-pre-wrap text-sm mt-4 font-sans break-words">{sel.body}</pre>
                  {attList(sel.attachments).length > 0 && (
                    <div className="mt-4 border-t border-border pt-3">
                      <b className="text-sm">{t('mail.attachments')}</b>
                      <div className="flex flex-wrap gap-2 mt-2">
                        {attList(sel.attachments).map((a: any, i: number) => (
                          <a key={i} href={`data:${a.type || 'application/octet-stream'};base64,${a.data}`} download={a.name}
                            className="text-xs rounded-md border border-border px-2 py-1 hover:bg-muted">
                            📎 {a.name} ({fmtSize(a.size)})
                          </a>
                        ))}
                      </div>
                    </div>
                  )}
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

function Compose({ init, onClose }: any) {
  const { t } = useI18n()
  const [f, setF] = useState({ to: init.to || '', cc: init.cc || '', bcc: init.bcc || '', subject: init.subject || '', body: init.body || '' })
  const [atts, setAtts] = useState<any[]>([])
  const [showCC, setShowCC] = useState(!!(init.cc || init.bcc))
  const [saving, setSaving] = useState(false)
  const [sugg, setSugg] = useState<any[]>([])
  useEffect(() => {
    Promise.all([api.contacts().catch(() => []), api.directory().catch(() => [])]).then(([cs, dir]) => {
      const map = new Map<string, any>()
      for (const d of dir) map.set(d.email.toLowerCase(), { email: d.email, name: d.name })
      for (const c of cs) map.set(c.email.toLowerCase(), { email: c.email, name: c.name, note: c.note })
      setSugg([...map.values()])
    })
  }, [])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const total = atts.reduce((s, a) => s + (a.size || 0), 0)

  function onFiles(e: any) {
    const files: File[] = Array.from(e.target.files || [])
    let cur = total
    for (const file of files) {
      if (cur + file.size > 8 * 1024 * 1024) { alert(t('mail.tooLarge')); break }
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
    try { await api.send({ ...f, attachments: atts, folder }); onClose() } catch (e: any) { alert(e.message) }
    finally { setSaving(false) }
  }

  return (
    <div className="fixed inset-0 bg-black/40 grid place-items-center sm:p-4 z-50" onClick={onClose} role="dialog" aria-modal="true" aria-label={t('mail.compose')}>
      <Card className="w-full h-full sm:h-auto sm:max-w-lg p-4 space-y-3 rounded-none sm:rounded-lg flex flex-col" onClick={(e: any) => e.stopPropagation()}>
        <div className="flex items-center gap-2">
          <b>{t('mail.compose')}</b>
          <div className="flex-1" />
          <Button variant="ghost" size="sm" onClick={() => setShowCC(v => !v)}>{t('mail.ccBcc')}</Button>
        </div>
        <RecipientInput placeholder={t('mail.to')} value={f.to} onChange={v => setF({ ...f, to: v })} suggestions={sugg} />
        {showCC && <>
          <RecipientInput placeholder={t('mail.cc')} value={f.cc} onChange={v => setF({ ...f, cc: v })} suggestions={sugg} />
          <RecipientInput placeholder={t('mail.bcc')} value={f.bcc} onChange={v => setF({ ...f, bcc: v })} suggestions={sugg} />
        </>}
        <Input placeholder={t('mail.subject')} value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} />
        <Textarea rows={8} placeholder={t('mail.body')} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} />
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
        <div className="flex items-center gap-2">
          <label className="inline-flex items-center gap-1 text-sm cursor-pointer text-muted-foreground hover:text-foreground">
            <Paperclip size={16} />{t('mail.attach')}
            <input type="file" multiple className="hidden" onChange={onFiles} />
          </label>
          <div className="flex-1" />
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

function attList(s: any): any[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a : [] } catch { return [] }
}
function fmtSize(n: number) {
  if (!n) return '0B'
  if (n < 1024) return n + 'B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + 'KB'
  return (n / 1024 / 1024).toFixed(1) + 'MB'
}
