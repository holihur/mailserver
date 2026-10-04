import { useEffect, useRef, useState } from 'react'
import { toast, confirmAsync, promptAsync } from '../lib/ui'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Textarea, Card } from '../components/ui/controls'
import { Dropdown, DropdownItem, DropdownSeparator, DropdownLabel } from '../components/Dropdown'
import { RecipientInput } from '../components/RecipientInput'
import { SkeletonList } from '../components/Skeleton'
import { FooterControls } from '../components/HeaderControls'
import { useI18n } from '../lib/i18n'
import {
  Inbox, Send, FileEdit, Trash2, Trash, Star, Search, PenLine, LogOut,
  RefreshCw, Globe, Settings, ShieldCheck, ArrowLeft, Loader2, Paperclip, X,
  ChevronDown, MoreVertical, Reply, ReplyAll, Forward, MailOpen, RotateCcw,
  Contact, Filter, KeyRound, AtSign, Download, Folder, Plus, FileCode, Clock, XCircle,
} from 'lucide-react'
import { cn, linkify, setUnreadBadge, quoteMail } from '../lib/utils'
import { BRAND } from '../lib/brand'

const FOLDERS = [
  { k: 'inbox', labelKey: 'mail.inbox', icon: Inbox },
  { k: 'sent', labelKey: 'mail.sent', icon: Send },
  { k: 'draft', labelKey: 'mail.draft', icon: FileEdit },
  { k: 'trash', labelKey: 'mail.trash', icon: Trash2 },
  { k: 'deleted', labelKey: 'mail.deleted', icon: Trash },
]

export default function MailApp() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [folder, setFolder] = useState(() => localStorage.getItem('pref.folder') || 'inbox')
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [unread, setUnread] = useState<any>({})
  const [folders, setFolders] = useState<any[]>([])
  const [total, setTotal] = useState(0)
  const [q, setQ] = useState('')
  const [recent, setRecent] = useState<string[]>(() => {
    try { const v = JSON.parse(localStorage.getItem('pref.searches') || '[]'); return Array.isArray(v) ? v : [] } catch { return [] }
  })
  const [showHistory, setShowHistory] = useState(false)
  const [sel, setSel] = useState(null)
  const [showCompose, setShowCompose] = useState<any>(false)
  const [me, setMe] = useState(null)
  const [view, setView] = useState('list') // 移动端：list | read
  const [page, setPage] = useState(1)
  const [sort, setSort] = useState(() => localStorage.getItem('pref.sort') || 'newest')
  const [installEvt, setInstallEvt] = useState<any>(null)
  const prevUnread = useRef(0)
  const touch = useRef<{ x: number; moved: boolean }>({ x: 0, moved: false })
  const firstQ = useRef(true)
  const lastFocusRefresh = useRef(0)
  const notifiedFailed = useRef<Set<number>>(new Set())
  const failedSeeded = useRef(false)
  const [preview, setPreview] = useState<any>(null)
  const [showImages, setShowImages] = useState(false)
  const bodyRef = useRef<HTMLDivElement>(null)
  const pageSize = 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  async function load(p = page, s = sort, silent = false) {
    if (!silent) setLoading(true)
    try {
      const d = await api.list(folder, q, p, s)
      setItems(d.items); setTotal(d.total); setPage(d.page || p)
    } catch {} finally { if (!silent) setLoading(false) }
    api.unread().then((u: any) => {
      setUnread(u)
      prevUnread.current = sumUnread(u)
    }).catch(() => {})
    api.folders().then(setFolders).catch(() => {})
  }
  useEffect(() => { api.me().then(setMe).catch(() => { location.href = '/login' }) }, [])
  useEffect(() => { setSel(null); setView('list'); setPage(1); load(1, sort) }, [folder])
  useEffect(() => { localStorage.setItem('pref.folder', folder) }, [folder])
  useEffect(() => { localStorage.setItem('pref.sort', sort) }, [sort])
  useEffect(() => {
    setUnreadBadge(sumUnread(unread))
  }, [unread])
  // 自动刷新 + 新邮件提示；回到前台/窗口重新聚焦时也刷新一次
  useEffect(() => {
    async function poll() {
      const u: any = await api.unread().catch(() => null)
      if (!u) return
      const tot = sumUnread(u)
      if (tot > prevUnread.current) {
        toast(t('mail.newMail'), { type: 'success' })
        if ('Notification' in window && Notification.permission === 'granted') {
          new Notification(BRAND, { body: t('mail.newMail') })
        }
        // 正在收件箱第一页且未搜索时，静默刷新列表让新邮件直接出现
        if (folder === 'inbox' && page === 1 && !q) load(1, sort, true)
      }
      prevUnread.current = tot
      setUnread(u)
      // 已发送：轮询刷新投递状态
      if (folder === 'sent' && page === 1 && !q) load(1, sort, true)
      // 投递失败提醒（首次轮询只登记，不弹历史失败）
      const box: any = await api.outbox().catch(() => null)
      if (Array.isArray(box)) {
        for (const m of box as any[]) {
          if (m.status !== 'failed' || notifiedFailed.current.has(m.id)) continue
          notifiedFailed.current.add(m.id)
          if (failedSeeded.current) toast(`${t('mail.stFailed')}: ${m.subject || t('mail.noSubject')}`, { type: 'error' })
        }
        failedSeeded.current = true
      }
    }
    const id = setInterval(poll, 45000)
    const onVisible = () => {
      if (document.visibilityState !== 'visible') return
      const now = Date.now()
      if (now - lastFocusRefresh.current < 1000) return // 去重：visibilitychange 与 focus 可能同时触发
      lastFocusRefresh.current = now
      load(page, sort, true)
    }
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('focus', onVisible)
    return () => {
      clearInterval(id)
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('focus', onVisible)
    }
  }, [t, folder, q, page, sort])
  // PWA 安装
  useEffect(() => {
    const h = (e: any) => { e.preventDefault(); setInstallEvt(e) }
    window.addEventListener('beforeinstallprompt', h)
    return () => window.removeEventListener('beforeinstallprompt', h)
  }, [])
  // 搜索防抖（300ms）
  useEffect(() => {
    if (firstQ.current) { firstQ.current = false; return }
    const id = setTimeout(() => { setPage(1); load(1, sort) }, 300)
    return () => clearTimeout(id)
  }, [q])

  const [selectMode, setSelectMode] = useState(false)
  const [checked, setChecked] = useState<number[]>([])
  function toggleCheck(id: number) { setChecked(c => c.includes(id) ? c.filter(x => x !== id) : [...c, id]) }
  function toggleAll() { setChecked(c => c.length === items.length ? [] : items.map((m: any) => m.id)) }
  const allStarred = checked.length > 0 && checked.every(id => items.find((m: any) => m.id === id)?.starred)
  async function batch(action: string, folderArg = '') {
    if (!checked.length) return
    try { await api.batch(checked, action, folderArg) } catch (e: any) { toast(e.message) }
    setChecked([]); setSelectMode(false); load(page, sort)
  }

  async function emptyTrash() {
    if (!await confirmAsync(t('mail.confirmPurge'))) return
    try { await api.emptyTrash() } catch (e: any) { toast(e.message) }
    setChecked([]); setSelectMode(false); setSel(null); load(1, sort)
  }

  async function open(id) {
    const d = await api.get(id)
    setSel(d)
    setShowImages(false)
    setView('read')
    setItems(items.map(i => i.id === id ? { ...i, read: true } : i))
    // 已读后立即刷新未读角标与标签页标题，无需等待下一轮轮询
    api.unread().then((u: any) => { setUnread(u); prevUnread.current = sumUnread(u) }).catch(() => {})
  }

  async function revealImages() {
    setShowImages(true)
    const el = bodyRef.current
    if (!el) return
    const imgs = Array.from(el.querySelectorAll('img[data-blocked-src]')) as HTMLImageElement[]
    for (const img of imgs) {
      const src = img.getAttribute('data-blocked-src') || ''
      try {
        img.src = await api.proxyImage(src)
        img.removeAttribute('data-blocked-src')
      } catch {}
    }
  }

  function reply(m: any, all: boolean) {
    const sig = me?.signature ? `\n\n-- \n${me.signature}` : ''
    let cc = ''
    if (all) {
      const mine = (me?.email || '').toLowerCase()
      const others = [m.to, m.cc].filter(Boolean).join(',').split(',').map((s: string) => s.trim())
        .filter((a: string) => a && a.toLowerCase() !== mine)
      cc = [...new Set(others)].join(', ')
    }
    setShowCompose({ to: m.from, cc, subject: 'Re: ' + (m.subject || ''), body: quoteMail(m, t) + sig })
  }
  function forwardMail(m: any) {
    const sig = me?.signature ? `\n\n-- \n${me.signature}` : ''
    const head = `\n\n${t('mail.forwardHeader')}\n${t('mail.fwdFrom')}: ${m.from}\n${t('mail.fwdTo')}: ${m.to}\n${t('mail.fwdSubject')}: ${m.subject || ''}\n\n`
    setShowCompose({ subject: 'Fwd: ' + (m.subject || ''), body: head + (m.body || '') + sig })
  }
  function onBodyClick(e: any) {
    const code = e.target?.dataset?.code
    if (code) {
      navigator.clipboard?.writeText(code)
      toast(t('mail.codeCopied', { code }), { type: 'success' })
    }
  }

  function logout() { localStorage.removeItem('token'); location.assign('/login') }

  function rememberSearch(term: string) {
    const s = term.trim()
    if (s.length < 2) return
    setRecent(prev => {
      const next = [s, ...prev.filter(x => x !== s)].slice(0, 10)
      localStorage.setItem('pref.searches', JSON.stringify(next))
      return next
    })
  }
  function clearHistory() { setRecent([]); localStorage.removeItem('pref.searches') }

  const allFolders: any[] = [
    ...FOLDERS,
    ...folders.map((f: any) => ({ k: 'c' + f.id, label: f.name, icon: Folder, custom: true, id: f.id })),
  ]
  async function loadFolders() { try { setFolders(await api.folders()) } catch {} }
  async function createFolder() {
    const name = (await promptAsync(t('folders.newPrompt')))?.trim()
    if (!name) return
    try { await api.folderCreate(name); await loadFolders() } catch (e: any) { toast(e.message) }
  }
  async function renameFolder(f: any) {
    const name = (await promptAsync(t('folders.renamePrompt'), { defaultValue: f.label }))?.trim()
    if (!name) return
    try { await api.folderPatch(f.id, name); await loadFolders() } catch (e: any) { toast(e.message) }
  }
  async function deleteFolder(f: any) {
    if (!await confirmAsync(t('folders.confirmDelete', { name: f.label }))) return
    try { await api.folderDelete(f.id); if (folder === f.k) setFolder('inbox'); await loadFolders() } catch (e: any) { toast(e.message) }
  }

  return (
    <div className="min-h-screen bg-background flex flex-col">
      <header className="border-b border-border px-3 sm:px-4 h-14 flex items-center gap-2 sticky top-0 bg-background/90 backdrop-blur z-10">
        <b className="shrink-0 truncate max-w-[45vw]">📮 {BRAND}</b>
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
          <DropdownItem icon={AtSign} onClick={() => navigate('/accounts')}>{t('nav.accounts')}</DropdownItem>
          <DropdownItem icon={Filter} onClick={() => navigate('/rules')}>{t('nav.rules')}</DropdownItem>
          <DropdownItem icon={FileCode} onClick={() => navigate('/sieve')}>{t('nav.sieve')}</DropdownItem>
          <DropdownItem icon={Clock} onClick={() => navigate('/scheduled')}>{t('nav.scheduled')}</DropdownItem>
          <DropdownItem icon={KeyRound} onClick={() => navigate('/security')}>{t('nav.security')}</DropdownItem>
          <DropdownItem icon={ShieldCheck} onClick={() => navigate('/privacy')}>{t('nav.privacy')}</DropdownItem>
          {me?.admin && <DropdownItem icon={Globe} onClick={() => navigate('/dns')}>{t('nav.dns')}</DropdownItem>}
          {me?.admin && <DropdownItem icon={ShieldCheck} onClick={() => navigate('/admin')}>{t('nav.admin')}</DropdownItem>}
          {installEvt && <DropdownItem icon={Download} onClick={() => { installEvt.prompt(); setInstallEvt(null) }}>{t('nav.install')}</DropdownItem>}
          <DropdownSeparator />
          <DropdownItem icon={RefreshCw} onClick={() => load()}>{t('common.refresh')}</DropdownItem>
          <DropdownItem icon={LogOut} onClick={logout} className="text-red-500 hover:bg-red-500/10">{t('nav.logout')}</DropdownItem>
        </Dropdown>
      </header>

      {/* 移动端：文件夹横向标签 */}
      <div className="md:hidden flex gap-1 overflow-x-auto border-b border-border px-2 py-2">
        {allFolders.map(f => (
          <div key={f.k} className="flex items-center gap-0.5 shrink-0">
            <button onClick={() => setFolder(f.k)}
              className={cn('flex items-center gap-1 rounded-md px-3 py-1.5 text-sm whitespace-nowrap',
                folder === f.k ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
              <f.icon size={15} />{f.labelKey ? t(f.labelKey) : f.label}
              {unread[f.k] > 0 && <span className={cn('ml-0.5 rounded-full px-1.5 text-[10px] font-semibold', folder === f.k ? 'bg-primary-foreground/20' : 'bg-primary/15 text-primary')}>{unread[f.k]}</span>}
            </button>
            {f.custom && (
              <Dropdown align="right" trigger={
                <button className="px-1 text-muted-foreground" aria-label={t('common.edit')}><MoreVertical size={13} /></button>
              }>
                <DropdownItem icon={PenLine} onClick={() => renameFolder(f)}>{t('folders.rename')}</DropdownItem>
                <DropdownItem icon={Trash2} className="text-red-500 hover:bg-red-500/10" onClick={() => deleteFolder(f)}>{t('folders.delete')}</DropdownItem>
              </Dropdown>
            )}
          </div>
        ))}
        <button onClick={createFolder} className="flex items-center rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted" aria-label={t('folders.new')}><Plus size={15} /></button>
      </div>

      <div className="flex-1 flex max-w-6xl w-full mx-auto min-h-0">
        <aside className="hidden md:block w-44 shrink-0 p-3 space-y-1 border-r border-border">
          {allFolders.map(f => (
            <div key={f.k} className="group flex items-center gap-0.5">
              <button onClick={() => setFolder(f.k)}
                className={cn('flex-1 flex items-center gap-2 rounded-md px-3 py-2 text-sm min-w-0', folder === f.k ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
                <f.icon size={16} className="shrink-0" />
                <span className="truncate">{f.labelKey ? t(f.labelKey) : f.label}</span>
                {unread[f.k] > 0 && <span className={cn('ml-auto rounded-full px-1.5 text-[10px] font-semibold shrink-0', folder === f.k ? 'bg-primary-foreground/20' : 'bg-primary/15 text-primary')}>{unread[f.k]}</span>}
              </button>
              {f.custom && (
                <Dropdown align="right" trigger={
                  <button className="px-1 py-2 text-muted-foreground" aria-label={t('common.edit')}><MoreVertical size={14} /></button>
                }>
                  <DropdownItem icon={PenLine} onClick={() => renameFolder(f)}>{t('folders.rename')}</DropdownItem>
                  <DropdownItem icon={Trash2} className="text-red-500 hover:bg-red-500/10" onClick={() => deleteFolder(f)}>{t('folders.delete')}</DropdownItem>
                </Dropdown>
              )}
            </div>
          ))}
          <button onClick={createFolder} className="w-full flex items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-muted">
            <Plus size={16} />{t('folders.new')}
          </button>
          <p className="text-xs text-muted-foreground px-3 pt-4">{t('mail.total', { n: total })}</p>
        </aside>

        <section className="flex-1 min-w-0 flex">
          {/* 列表 */}
          <div className={cn('w-full md:w-80 md:shrink-0 border-r border-border flex-col',
            view === 'read' ? 'hidden md:flex' : 'flex')}>
            <div className="p-3 border-b border-border flex gap-2">
              <div className="relative flex-1">
                <Search size={14} className="absolute left-2 top-2.5 text-muted-foreground" />
                <Input className="pl-7 pr-7" placeholder={t('mail.search')} value={q}
                  onChange={e => { setQ(e.target.value); setShowHistory(true) }}
                  onFocus={() => setShowHistory(true)}
                  onBlur={() => { setShowHistory(false); rememberSearch(q) }}
                  onKeyDown={e => {
                    if (e.key === 'Enter') { setPage(1); load(1, sort); rememberSearch(q); setShowHistory(false) }
                    else if (e.key === 'Escape') setShowHistory(false)
                  }} />
                {q && (
                  <button type="button" aria-label={t('common.clear')}
                    className="absolute right-2 top-2.5 text-muted-foreground hover:text-foreground"
                    onMouseDown={e => e.preventDefault()}
                    onClick={() => { setQ(''); setShowHistory(false) }}>
                    <X size={14} />
                  </button>
                )}
                {showHistory && (q ? recent.filter(s => s.toLowerCase().includes(q.toLowerCase())) : recent).length > 0 && (
                  <div className="absolute z-50 mt-1 w-full rounded-md border border-border bg-card shadow-lg p-1"
                    onMouseDown={e => e.preventDefault()}>
                    <div className="flex items-center justify-between px-2 py-1">
                      <span className="text-xs text-muted-foreground">{t('mail.recentSearches')}</span>
                      <button type="button" className="text-xs text-muted-foreground hover:text-foreground"
                        onClick={clearHistory}>{t('mail.clearHistory')}</button>
                    </div>
                    {(q ? recent.filter(s => s.toLowerCase().includes(q.toLowerCase())) : recent).slice(0, 8).map(s => (
                      <button key={s} type="button"
                        className="w-full flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-left hover:bg-muted"
                        onClick={() => { setQ(s); setShowHistory(false); rememberSearch(s) }}>
                        <Clock size={13} className="text-muted-foreground shrink-0" />
                        <span className="truncate">{s}</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <select value={sort} onChange={e => { setSort(e.target.value); setPage(1); load(1, e.target.value) }}
                className="h-9 rounded-md border border-border bg-background text-xs px-2" aria-label={t('mail.sort')}>
                <option value="newest">{t('mail.sortNewest')}</option>
                <option value="oldest">{t('mail.sortOldest')}</option>
                <option value="subject">{t('mail.sortSubject')}</option>
                <option value="sender">{t('mail.sortSender')}</option>
              </select>
              <Button variant={selectMode ? 'default' : 'outline'} size="default" className="h-9 shrink-0"
                onClick={() => { setSelectMode(v => !v); setChecked([]) }}>
                {selectMode ? t('mail.done') : t('mail.batch')}
              </Button>
              {folder === 'trash' && total > 0 && (
                <Button variant="outline" size="default" className="h-9 shrink-0" onClick={emptyTrash}>
                  <Trash2 />{t('mail.emptyTrash')}
                </Button>
              )}
            </div>
            {selectMode && (
              <div className="p-2 border-b border-border flex items-center gap-1 flex-wrap text-xs">
                <label className="flex items-center gap-1"><input type="checkbox" checked={checked.length > 0 && checked.length === items.length} onChange={toggleAll} />{t('mail.selectAll')}</label>
                <span className="text-muted-foreground">{t('mail.selected', { n: checked.length })}</span>
                <div className="flex-1" />
                <Button variant="ghost" size="sm" disabled={!checked.length} onClick={() => batch('read')}><MailOpen />{t('mail.markRead')}</Button>
                <Button variant="ghost" size="sm" disabled={!checked.length} onClick={() => batch('unread')}><MailOpen />{t('mail.markUnread')}</Button>
                <Button variant="ghost" size="sm" disabled={!checked.length} onClick={() => batch(allStarred ? 'unstar' : 'star')}><Star />{allStarred ? t('mail.unstar') : t('mail.batchStar')}</Button>
                {folder === 'deleted' ? (
                  <>
                    <Button variant="ghost" size="sm" disabled={!checked.length} onClick={() => batch('move', 'inbox')}><RotateCcw />{t('mail.restore')}</Button>
                    <Button variant="ghost" size="sm" disabled={!checked.length} onClick={async () => { if (!await confirmAsync(t('mail.confirmPurge'))) return; batch('purge') }}><Trash2 />{t('mail.purge')}</Button>
                  </>
                ) : (
                  <Button variant="ghost" size="sm" disabled={!checked.length} onClick={async () => {
                    if (folder === 'trash' && !await confirmAsync(t('mail.confirmPurge'))) return
                    batch(folder === 'trash' ? 'delete' : 'trash')
                  }}><Trash2 />{t('mail.batchDelete')}</Button>
                )}
              </div>
            )}
            <div className="flex-1 overflow-auto">
              {loading && <SkeletonList rows={6} />}
              {!loading && items.map(m => (
                <button key={m.id} onClick={() => { if (touch.current.moved) { touch.current.moved = false; return } selectMode ? toggleCheck(m.id) : open(m.id) }}
                  onTouchStart={e => { touch.current = { x: e.touches[0].clientX, moved: false } }}
                  onTouchMove={e => { if (Math.abs(e.touches[0].clientX - touch.current.x) > 12) touch.current.moved = true }}
                  onTouchEnd={async e => {
                    if (!touch.current.moved) return
                    const dx = e.changedTouches[0].clientX - touch.current.x
                    if (dx < -60) { await api.trash(m.id); load() }
                    else if (dx > 60) { await api.patch(m.id, { read: !m.read }); load() }
                  }}
                  className={cn('cv-auto w-full text-left px-3 py-2.5 border-b border-border hover:bg-muted/60 transition-colors',
                    (selectMode ? checked.includes(m.id) : sel?.id === m.id) && 'bg-muted', !m.read && 'bg-primary/5')}>
                  <div className="flex items-center gap-2">
                    {selectMode && <input type="checkbox" readOnly checked={checked.includes(m.id)} className="pointer-events-none shrink-0" />}
                    {!m.read && !selectMode && <span className="size-2 rounded-full bg-primary shrink-0" />}
                    <span className={cn('truncate flex-1 text-sm', !m.read ? 'font-semibold' : 'text-foreground')}>
                      {m.subject || t('mail.noSubject')}
                    </span>
                    <SendStatus m={m} folder={folder} />
                    {attList(m.attachments).length > 0 && <Paperclip size={12} className="text-muted-foreground shrink-0" />}
                    {!selectMode && <Star size={14} className={cn('shrink-0', m.starred ? 'fill-yellow-400 text-yellow-400' : 'text-muted-foreground')}
                      onClick={async e => { e.stopPropagation(); await api.patch(m.id, { starred: !m.starred }); load() }} />}
                  </div>
                  <div className="mt-0.5">
                    <span className="text-xs text-muted-foreground line-clamp-1">{m.body?.slice(0, 80)}</span>
                  </div>
                  <div className="flex items-center justify-end gap-2 mt-0.5">
                    <span className={cn('text-xs truncate max-w-[70%]', m.read ? 'text-muted-foreground' : 'font-medium')}>
                      {folder === 'sent' ? m.to : m.from}
                    </span>
                    <span className="text-[10px] text-muted-foreground shrink-0">{fmtWhen(m.created_at)}</span>
                  </div>
                </button>
              ))}
              {!loading && items.length === 0 && (
                <div className="p-8 text-center space-y-3">
                  <p className="text-sm text-muted-foreground">
                    {q ? t('mail.noResults') : folder === 'inbox' ? t('mail.emptyInbox') : t('mail.emptyFolder')}
                  </p>
                  {folder === 'inbox' && !q && (
                    <Button size="sm" variant="outline" onClick={() => setShowCompose(true)}><PenLine />{t('mail.compose')}</Button>
                  )}
                </div>
              )}
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
                  <div className="flex items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <h2 className="text-lg font-semibold break-words">{sel.subject || t('mail.noSubject')}</h2>
                      <p className="text-xs text-muted-foreground mt-1">
                        {t('mail.fromTo', { from: sel.from, to: sel.to })} · {new Date(sel.created_at).toLocaleString()}
                      </p>
                      {sel.cc && <p className="text-xs text-muted-foreground mt-0.5">Cc: {sel.cc}</p>}
                      {sel.auth_results && (
                        <p className={cn('text-[11px] mt-0.5', sel.auth_results.includes('dmarc=fail') ? 'text-red-500' : sel.auth_results.includes('dmarc=pass') ? 'text-green-600' : 'text-muted-foreground')}>
                          {sel.auth_results}
                        </p>
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
                      <DropdownItem icon={Trash2} className="text-red-500 hover:bg-red-500/10" onClick={async () => {
                        if ((folder === 'trash' || folder === 'deleted') && !await confirmAsync(t('mail.confirmPurge'))) return
                        await api.trash(sel.id); setSel(null); setView('list'); load()
                      }}>{folder === 'deleted' ? t('mail.purge') : t('mail.delete')}</DropdownItem>
                    </Dropdown>
                  </div>
                  {folder === 'sent' && sel.status && sel.status !== 'sent' && (
                    <div className={cn('mt-4 rounded-md border p-3 text-sm', sel.status === 'failed' ? 'border-red-500/40 bg-red-500/10 text-red-600' : 'border-border bg-muted/50 text-muted-foreground')}>
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
            )}
          </div>
        </section>
      </div>

      <FooterControls />

      {preview && (
        <div className="fixed inset-0 z-[90] grid place-items-center bg-black/70 p-4" onClick={() => setPreview(null)}>
          <div className="w-full max-w-4xl max-h-[92vh] flex flex-col" onClick={e => e.stopPropagation()}>
            <div className="flex items-center gap-2 text-white mb-2">
              <span className="truncate text-sm flex-1">{preview.name}</span>
              <a href={preview.dataUrl} download={preview.name} className="text-white/80 hover:text-white" aria-label={t('mail.download')}><Download size={16} /></a>
              <button onClick={() => setPreview(null)} aria-label="close" className="text-white/80 hover:text-white"><X size={18} /></button>
            </div>
            <div className="overflow-auto grid place-items-center">
              {preview.kind === 'image' && <img src={preview.dataUrl} alt={preview.name} className="max-h-[84vh] rounded" />}
              {preview.kind === 'video' && <video src={preview.dataUrl} controls className="max-h-[84vh] w-full rounded bg-black" />}
              {preview.kind === 'audio' && <audio src={preview.dataUrl} controls className="w-full" />}
              {preview.kind === 'pdf' && <iframe src={preview.dataUrl} title={preview.name} className="w-full h-[84vh] rounded bg-white" />}
            </div>
          </div>
        </div>
      )}

      {showCompose && <Compose me={me} init={typeof showCompose === 'object' ? showCompose : {}} onClose={() => { setShowCompose(false); load() }} />}
    </div>
  )
}

function Compose({ me, init, onClose }: any) {
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
  useEffect(() => { api.external().then((xs: any[]) => setIds(xs.filter(x => x.enabled))).catch(() => {}) }, [])
  // 草稿自动保存（25s 一次，有内容才存）
  useEffect(() => {
    const id = setInterval(async () => {
      if (saving) return
      if (!f.to && !f.subject && !f.body.trim()) return
      try {
        if (draftId) await api.patch(draftId, { to: f.to, cc: f.cc, bcc: f.bcc, subject: f.subject, body: f.body })
        else { const r = await api.send({ ...f, folder: 'draft' }); setDraftId(r.id) }
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
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const total = atts.reduce((s, a) => s + (a.size || 0), 0)

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
    <div className="fixed inset-0 bg-black/40 grid place-items-center sm:p-4 z-50" onClick={onClose} role="dialog" aria-modal="true" aria-label={t('mail.compose')}>
      <Card className="w-full h-full sm:h-auto sm:max-w-lg p-4 space-y-3 rounded-none sm:rounded-lg flex flex-col" onClick={(e: any) => e.stopPropagation()}>
        <div className="flex items-center gap-2">
          <b>{t('mail.compose')}</b>
          <div className="flex-1" />
          <Button variant="ghost" size="sm" onClick={() => setShowCC(v => !v)}>{t('mail.ccBcc')}</Button>
        </div>
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
        <div className="flex items-center gap-2">
          <label className="inline-flex items-center gap-1 text-sm cursor-pointer text-muted-foreground hover:text-foreground">
            <Paperclip size={16} />{t('mail.attach')}
            <input type="file" multiple className="hidden" onChange={onFiles} />
          </label>
          <Button variant={schedOpen ? 'default' : 'outline'} size="sm" type="button" onClick={() => setSchedOpen(v => !v)}><Clock />{t('mail.schedule')}</Button>
          <div className="flex-1" />
          <Button variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button variant="outline" disabled={saving} onClick={() => submit('draft')}>{t('mail.saveDraft')}</Button>
          <Button disabled={saving} onClick={() => submit('sent')}>
            {saving ? <Loader2 className="animate-spin" /> : null}{schedOpen && sendAt ? t('mail.scheduleSend') : t('mail.send')}
          </Button>
        </div>
      </Card>
    </div>
  )
}

function SendStatus({ m, folder }: any) {
  const { t } = useI18n()
  if (folder !== 'sent' || !m.status || m.status === 'sent') return null
  if (m.status === 'failed') return <span className="flex items-center gap-0.5 text-[10px] text-red-500 shrink-0"><XCircle size={11} />{t('mail.stFailed')}</span>
  if (m.status === 'sending') return <span className="flex items-center gap-0.5 text-[10px] text-blue-500 shrink-0"><Loader2 size={11} className="animate-spin" />{t('mail.stSending')}</span>
  return <span className="flex items-center gap-0.5 text-[10px] text-yellow-600 shrink-0"><Clock size={11} />{t('mail.stQueued')}</span>
}

function attList(s: any): any[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a : [] } catch { return [] }
}

function sumUnread(u: any): number {
  const vals = Object.values(u || {}) as number[]
  return vals.reduce((a, b) => a + (Number(b) || 0), 0)
}

function attKind(type: string): string {
  if (!type) return ''
  if (type.startsWith('image/')) return 'image'
  if (type.startsWith('video/')) return 'video'
  if (type.startsWith('audio/')) return 'audio'
  if (type === 'application/pdf') return 'pdf'
  return ''
}
function fmtSize(n: number) {
  if (!n) return '0B'
  if (n < 1024) return n + 'B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + 'KB'
  return (n / 1024 / 1024).toFixed(1) + 'MB'
}

function fmtWhen(s: string) {
  if (!s) return ''
  const d = new Date(s)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return d.toLocaleDateString()
}
