import { Fragment, useEffect, useRef, useState } from 'react'
import { toast, confirmAsync, confirmDestructive, promptAsync } from '../lib/ui'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Textarea, Card } from '../components/ui/controls'
import { Dropdown, DropdownItem, DropdownSeparator, DropdownLabel } from '../components/Dropdown'
import { RecipientInput } from '../components/RecipientInput'
import { SkeletonList } from '../components/Skeleton'
import { EmptyState } from '../components/EmptyState'
import { SettingsMenu } from '../components/HeaderControls'
import { useI18n } from '../lib/i18n'
import {
  Inbox, Send, FileEdit, Trash2, Trash, Star, Search, PenLine, LogOut,
  RefreshCw, Globe, Settings, ShieldCheck, ArrowLeft, Loader2, Paperclip, X,
  ChevronDown, MoreVertical, Reply, ReplyAll, Forward, MailOpen, RotateCcw,
  Contact, Filter, KeyRound, AtSign, Download, Folder, Plus, FileCode, Clock, XCircle, Mail, Upload,
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

// 未读数超过 99 显示 99+（#38）。
const fmtUnread = (n: number) => (n > 99 ? '99+' : String(n))

// 写信浮动按钮：可拖动并记忆位置（#50）。
const FAB_KEY = 'pref.fab'
const FAB_SIZE = 56
function loadFab(): { x: number; y: number } | null {
  try {
    const v = JSON.parse(localStorage.getItem(FAB_KEY) || 'null')
    return v && typeof v.x === 'number' && typeof v.y === 'number' ? v : null
  } catch { return null }
}
function clampFab(x: number, y: number, w = FAB_SIZE, h = FAB_SIZE) {
  const pad = 8
  return {
    x: Math.min(Math.max(pad, x), Math.max(pad, window.innerWidth - w - pad)),
    y: Math.min(Math.max(pad, y), Math.max(pad, window.innerHeight - h - pad)),
  }
}

export default function MailApp() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { id: routeId } = useParams()
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
  const [installDismissed, setInstallDismissed] = useState(false)
  const [offline, setOffline] = useState(typeof navigator !== 'undefined' && !navigator.onLine)
  const [fab, setFab] = useState(() => loadFab() || clampFab(window.innerWidth - FAB_SIZE - 16, window.innerHeight - FAB_SIZE - 88))
  const fabDrag = useRef<{ x0: number; y0: number; dx: number; dy: number; moved: boolean; w: number; h: number } | null>(null)
  const prevUnread = useRef(0)
  const touch = useRef<{ x: number; moved: boolean }>({ x: 0, moved: false })
  const firstQ = useRef(true)
  const lastFocusRefresh = useRef(0)
  const notifiedFailed = useRef<Set<number>>(new Set())
  const failedSeeded = useRef(false)
  const [preview, setPreview] = useState<any>(null)
  const [showHelp, setShowHelp] = useState(false)
  const [showImages, setShowImages] = useState(false)
  const bodyRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const importRef = useRef<HTMLInputElement>(null)
  const pageSize = 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  async function load(p = page, s = sort, silent = false, g = group) {
    if (!silent) setLoading(true)
    try {
      const d = await api.list(folder, q, p, s, g ? 'thread' : '')
      setItems(d.items); setTotal(d.total); setPage(d.page || p)
    } catch {} finally { if (!silent) setLoading(false) }
    api.unread().then((u: any) => {
      setUnread(u)
      prevUnread.current = sumUnread(u)
    }).catch(() => {})
    api.folders().then(setFolders).catch(() => {})
  }
  useEffect(() => { api.me().then((m: any) => { setMe(m); if (m?.must_change_password) navigate('/security') }).catch(() => { location.href = '/login' }) }, [])
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

  // 快捷键：Ctrl/Cmd+K 聚焦搜索（#45）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  // 离线状态提示（#50）
  useEffect(() => {
    const on = () => setOffline(false)
    const off = () => setOffline(true)
    window.addEventListener('online', on)
    window.addEventListener('offline', off)
    return () => { window.removeEventListener('online', on); window.removeEventListener('offline', off) }
  }, [])

  // 视口变化时把浮动按钮拉回可见范围（#50）
  useEffect(() => {
    const onResize = () => setFab(f => clampFab(f.x, f.y))
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  // Gmail 风格键盘流（#48）：列表输入框内不拦截；写信/预览打开时不生效。
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      const el = e.target as HTMLElement | null
      const tag = el?.tagName?.toLowerCase()
      if (tag === 'input' || tag === 'textarea' || tag === 'select' || el?.isContentEditable) return
      if (showHelp) { if (e.key === 'Escape' || e.key === '?') setShowHelp(false); return }
      if (showCompose || preview) return
      const idx = sel ? items.findIndex((m: any) => m.id === sel.id) : -1
      const focus = (i: number) => { if (i >= 0 && i < items.length) open(items[i].id) }
      switch (e.key) {
        case 'j': case 'ArrowDown': e.preventDefault(); focus(idx < 0 ? 0 : idx + 1); break
        case 'k': case 'ArrowUp': e.preventDefault(); focus(idx < 0 ? items.length - 1 : idx - 1); break
        case 'Enter': case 'o': if (idx >= 0) { e.preventDefault(); open(items[idx].id) } break
        case 'u': case 'Escape': if (sel) { e.preventDefault(); navigate('/') } break
        case 's': if (sel) { e.preventDefault(); api.patch(sel.id, { starred: !sel.starred }).then(() => { setSel({ ...sel, starred: !sel.starred }); load() }) } break
        case 'e': if (sel) { e.preventDefault(); setItems(items.map((i: any) => i.id === sel.id ? { ...i, read: true } : i)); api.patch(sel.id, { read: true }) } break
        case '#': if (sel) { e.preventDefault(); api.trash(sel.id).then(() => { setSel(null); navigate('/'); load() }) } break
        case 'r': if (sel) { e.preventDefault(); reply(sel, false) } break
        case 'c': e.preventDefault(); setShowCompose(true); break
        case '/': e.preventDefault(); searchRef.current?.focus(); break
        case '?': e.preventDefault(); setShowHelp(true); break
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [items, sel, showCompose, preview, showHelp])

  // 路由化阅读（#43）：/m/:id 可直接打开、刷新与分享。
  useEffect(() => {
    if (routeId) {
      if (String(sel?.id) !== routeId) open(routeId)
    } else {
      setView('list')
    }
  }, [routeId])

  // 附件预览弹窗 Esc 关闭（#44）
  useEffect(() => {
    if (!preview) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setPreview(null) }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [preview])

  const [selectMode, setSelectMode] = useState(false)
  const [group, setGroup] = useState(false)
  const [thread, setThread] = useState<any[]>([])
  const [checked, setChecked] = useState<number[]>([])
  const [allSelected, setAllSelected] = useState(false)
  const hasSel = allSelected || checked.length > 0
  function toggleCheck(id: number) { setChecked(c => c.includes(id) ? c.filter(x => x !== id) : [...c, id]) }
  function toggleAll() { setAllSelected(false); setChecked(c => c.length === items.length ? [] : items.map((m: any) => m.id)) }
  const allStarred = checked.length > 0 && checked.every(id => items.find((m: any) => m.id === id)?.starred)
  async function batch(action: string, folderArg = '') {
    if (!hasSel) return
    try {
      if (allSelected) await api.batch([], action, folderArg, { all: true, q, src_folder: folder })
      else await api.batch(checked, action, folderArg)
    } catch (e: any) { toast(e.message) }
    setChecked([]); setAllSelected(false); setSelectMode(false); load(page, sort)
  }

  async function emptyTrash() {
    if (!await confirmDestructive(t('mail.confirmPurge'))) return
    try { await api.emptyTrash() } catch (e: any) { toast(e.message) }
    setChecked([]); setSelectMode(false); setSel(null); load(1, sort)
  }

  async function open(id) {
    const d = await api.get(id)
    setSel(d)
    setShowImages(false)
    setView('read')
    if (String(id) !== routeId) navigate('/m/' + id)
    setItems(items.map(i => i.id === id ? { ...i, read: true } : i))
    api.thread(id).then((ms: any) => setThread(ms || [])).catch(() => setThread([]))
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
    if (!await confirmDestructive(t('folders.confirmDelete', { name: f.label }))) return
    try { await api.folderDelete(f.id); if (folder === f.k) setFolder('inbox'); await loadFolders() } catch (e: any) { toast(e.message) }
  }

  // 导入 mbox / EML（#51）
  async function onImport(e: any) {
    const input = e.target as HTMLInputElement
    const f = input.files?.[0]
    if (!f) return
    toast(t('mail.importing'))
    try {
      const r: any = await api.importMails(f, folder)
      toast(t('mail.imported', { n: r.imported }), { type: 'success' })
      load()
    } catch (err: any) { toast(err.message, { type: 'error' }) } finally { input.value = '' }
  }

  // 拖动浮动写信按钮：未移动则视为点击（打开写信），移动则吸附到最近一侧。
  function fabDown(e: any) {
    const el = e.currentTarget as HTMLElement
    try { el.setPointerCapture(e.pointerId) } catch {}
    fabDrag.current = { x0: e.clientX, y0: e.clientY, dx: e.clientX - fab.x, dy: e.clientY - fab.y, moved: false, w: el.offsetWidth, h: el.offsetHeight }
  }
  function fabMove(e: any) {
    const d = fabDrag.current
    if (!d) return
    if (!d.moved && (Math.abs(e.clientX - d.x0) > 6 || Math.abs(e.clientY - d.y0) > 6)) d.moved = true
    if (d.moved) setFab(clampFab(e.clientX - d.dx, e.clientY - d.dy, d.w, d.h))
  }
  function fabUp(e: any) {
    const d = fabDrag.current
    fabDrag.current = null
    if (!d) return
    if (!d.moved) { setShowCompose(true); return }
    const x = e.clientX - d.dx
    const y = e.clientY - d.dy
    const snapped = clampFab(x + d.w / 2 < window.innerWidth / 2 ? 12 : window.innerWidth - d.w - 12, y, d.w, d.h)
    setFab(snapped)
    try { localStorage.setItem(FAB_KEY, JSON.stringify(snapped)) } catch {}
  }

  return (
    <div className="h-dvh bg-background flex flex-col overflow-hidden">
      <header className="border-b border-border px-3 sm:px-4 h-14 flex items-center gap-2 sticky top-0 bg-background/90 backdrop-blur z-10">
        <b className="shrink-0 truncate max-w-[45vw] flex items-center gap-1.5"><Mail size={16} />{BRAND}</b>
        <div className="flex-1" />
        <Button size="sm" className="hidden sm:inline-flex" onClick={() => setShowCompose(true)}><PenLine /><span className="hidden sm:inline">{t('mail.compose')}</span></Button>
        <SettingsMenu />
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
          <DropdownLabel>{t('nav.groupMail')}</DropdownLabel>
          <DropdownItem icon={Settings} onClick={() => navigate('/setup')}>{t('nav.setup')}</DropdownItem>
          <DropdownItem icon={Contact} onClick={() => navigate('/contacts')}>{t('nav.contacts')}</DropdownItem>
          <DropdownItem icon={AtSign} onClick={() => navigate('/accounts')}>{t('nav.accounts')}</DropdownItem>
          <DropdownItem icon={Filter} onClick={() => navigate('/rules')}>{t('nav.rules')}</DropdownItem>
          <DropdownItem icon={FileCode} onClick={() => navigate('/sieve')}>{t('nav.sieve')}</DropdownItem>
          <DropdownItem icon={Clock} onClick={() => navigate('/scheduled')}>{t('nav.scheduled')}</DropdownItem>
          <DropdownItem icon={Upload} onClick={() => importRef.current?.click()}>{t('mail.importMails')}</DropdownItem>
          <DropdownSeparator />
          <DropdownLabel>{t('nav.groupAccount')}</DropdownLabel>
          <DropdownItem icon={KeyRound} onClick={() => navigate('/security')}>{t('nav.security')}</DropdownItem>
          <DropdownItem icon={ShieldCheck} onClick={() => navigate('/privacy')}>{t('nav.privacy')}</DropdownItem>
          {me?.admin && <>
            <DropdownSeparator />
            <DropdownLabel>{t('nav.groupAdmin')}</DropdownLabel>
            <DropdownItem icon={Globe} onClick={() => navigate('/dns')}>{t('nav.dns')}</DropdownItem>
            <DropdownItem icon={ShieldCheck} onClick={() => navigate('/admin')}>{t('nav.admin')}</DropdownItem>
          </>}
          <DropdownSeparator />
          {installEvt && <DropdownItem icon={Download} onClick={() => { installEvt.prompt(); setInstallEvt(null) }}>{t('nav.install')}</DropdownItem>}
          <DropdownItem icon={RefreshCw} onClick={() => load()}>{t('common.refresh')}</DropdownItem>
          <DropdownItem icon={LogOut} onClick={logout} className="text-destructive hover:bg-destructive/10">{t('nav.logout')}</DropdownItem>
        </Dropdown>
      </header>

      {offline && (
        <div className="flex items-center justify-center gap-1.5 border-b border-warning/40 bg-warning/10 px-3 py-1 text-xs text-warning">
          <span className="size-2 rounded-full bg-warning" />{t('mail.offline')}
        </div>
      )}

      {/* PWA 安装引导（#50） */}
      {installEvt && !installDismissed && (
        <div className="sm:hidden flex items-center gap-2 border-b border-border bg-primary/5 px-3 py-2 text-xs">
          <Download size={14} className="text-primary shrink-0" />
          <span className="flex-1">{t('mail.installHint')}</span>
          <Button size="sm" onClick={() => { installEvt.prompt(); setInstallEvt(null) }}>{t('nav.install')}</Button>
          <button onClick={() => setInstallDismissed(true)} aria-label={t('common.close')} className="text-muted-foreground"><X size={14} /></button>
        </div>
      )}

      {/* 移动端：底部悬浮写信按钮（拇指区，适配安全区） */}
      <input ref={importRef} type="file" accept=".mbox,.eml,message/rfc822" className="hidden" onChange={onImport} />
      <button onPointerDown={fabDown} onPointerMove={fabMove} onPointerUp={fabUp} onPointerCancel={() => { fabDrag.current = null }}
        style={{ left: fab.x, top: fab.y }} aria-label={t('mail.compose')}
        className="sm:hidden fixed z-30 grid place-items-center size-14 rounded-full bg-primary text-primary-foreground shadow-lg active:scale-95 transition-transform touch-none select-none cursor-grab">
        <PenLine size={22} />
      </button>

      <div className="flex-1 flex max-w-6xl w-full mx-auto min-h-0">
        <aside className="hidden md:block w-44 shrink-0 p-3 space-y-1 border-r border-border">
          {allFolders.map(f => (
            <div key={f.k} className="group flex items-center gap-0.5">
              <button onClick={() => setFolder(f.k)} aria-current={folder === f.k ? 'true' : undefined}
                className={cn('flex-1 flex items-center gap-2 rounded-md px-3 py-2 text-sm min-w-0', folder === f.k ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
                <f.icon size={16} className="shrink-0" />
                <span className="truncate">{f.labelKey ? t(f.labelKey) : f.label}</span>
                {unread[f.k] > 0 && <span className={cn('ml-auto rounded-full px-1.5 text-[10px] font-semibold shrink-0', folder === f.k ? 'bg-primary-foreground/20' : 'bg-primary/15 text-primary')}>{fmtUnread(unread[f.k])}</span>}
              </button>
              {f.custom && (
                <Dropdown align="right" trigger={
                  <button className="px-1 py-2 text-muted-foreground" aria-label={t('common.edit')}><MoreVertical size={14} /></button>
                }>
                  <DropdownItem icon={PenLine} onClick={() => renameFolder(f)}>{t('folders.rename')}</DropdownItem>
                  <DropdownItem icon={Trash2} className="text-destructive hover:bg-destructive/10" onClick={() => deleteFolder(f)}>{t('folders.delete')}</DropdownItem>
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
          <div className={cn('w-full md:w-80 md:shrink-0 border-r border-border flex-col min-h-0',
            view === 'read' ? 'hidden md:flex' : 'flex')}>
            <div className="p-3 border-b border-border flex flex-wrap gap-2">
              <div className="relative flex-1">
                <Search size={14} className="absolute left-2 top-2.5 text-muted-foreground" />
                <Input ref={searchRef} className="pl-7 pr-7" placeholder={t('mail.searchPlaceholder')} title={t('mail.searchHint')} value={q}
                  onChange={e => { setQ(e.target.value); setShowHistory(true) }}
                  onFocus={() => setShowHistory(true)}
                  onBlur={() => { setShowHistory(false); rememberSearch(q) }}
                  onKeyDown={e => {
                    if (e.key === 'Enter') { setPage(1); load(1, sort); rememberSearch(q); setShowHistory(false) }
                    else if (e.key === 'Escape') setShowHistory(false)
                  }} />
                {loading && q && (
                  <Loader2 size={14} className="absolute right-7 top-2.5 animate-spin text-muted-foreground" />
                )}
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
              <Button variant={group ? 'default' : 'outline'} size="default" className="h-9 shrink-0"
                onClick={() => { const g = !group; setGroup(g); setPage(1); load(1, sort, false, g) }}>
                {t('mail.threads')}
              </Button>
              <Button variant={selectMode ? 'default' : 'outline'} size="default" className="h-9 shrink-0"
                onClick={() => { setSelectMode(v => !v); setChecked([]); setAllSelected(false) }}>
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
                <label className="flex items-center gap-1"><input type="checkbox" checked={!allSelected && checked.length > 0 && checked.length === items.length} onChange={toggleAll} />{t('mail.selectAll')}</label>
                {total > items.length && (
                  <button type="button" className="text-primary hover:underline"
                    onClick={() => { setAllSelected(v => !v); setChecked([]) }}>
                    {allSelected ? t('mail.clearSelection') : t('mail.selectAllResults', { n: total })}
                  </button>
                )}
                <span className="text-muted-foreground">{allSelected ? t('mail.selectedAll', { n: total }) : t('mail.selected', { n: checked.length })}</span>
                <div className="flex-1" />
                <Dropdown align="right" trigger={<Button variant="ghost" size="sm" disabled={!hasSel}><Folder />{t('mail.moveTo')}</Button>}>
                  <DropdownLabel>{t('mail.moveTo')}</DropdownLabel>
                  {allFolders.map(f => (
                    <DropdownItem key={f.k} icon={f.icon} onClick={() => batch('move', f.k)}>{f.labelKey ? t(f.labelKey) : f.label}</DropdownItem>
                  ))}
                </Dropdown>
                <Button variant="ghost" size="sm" disabled={!hasSel} onClick={() => batch('read')}><MailOpen />{t('mail.markRead')}</Button>
                <Button variant="ghost" size="sm" disabled={!hasSel} onClick={() => batch('unread')}><MailOpen />{t('mail.markUnread')}</Button>
                <Button variant="ghost" size="sm" disabled={!hasSel || allSelected} onClick={() => batch(allStarred ? 'unstar' : 'star')}><Star />{allStarred ? t('mail.unstar') : t('mail.batchStar')}</Button>
                {folder === 'deleted' ? (
                  <>
                    <Button variant="ghost" size="sm" disabled={!hasSel} onClick={() => batch('move', 'inbox')}><RotateCcw />{t('mail.restore')}</Button>
                    <Button variant="ghost" size="sm" disabled={!hasSel} onClick={async () => { if (!await confirmDestructive(t('mail.confirmPurge'))) return; batch('purge') }}><Trash2 />{t('mail.purge')}</Button>
                  </>
                ) : (
                  <Button variant="ghost" size="sm" disabled={!hasSel} onClick={async () => {
                    if (folder === 'trash' && !await confirmDestructive(t('mail.confirmPurge'))) return
                    batch(folder === 'trash' ? 'delete' : 'trash')
                  }}><Trash2 />{t('mail.batchDelete')}</Button>
                )}
              </div>
            )}
            <div className="flex-1 min-h-0 overflow-auto overscroll-contain">
              {loading && <SkeletonList rows={6} />}
              {!loading && items.map(m => (
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
                  <div className="flex items-center justify-end gap-2 mt-0.5">
                    <span className={cn('text-xs truncate max-w-[70%]', m.read ? 'text-muted-foreground' : 'font-medium')}>
                      {folder === 'sent' ? m.to : m.from}
                    </span>
                    <span className="text-[10px] text-muted-foreground shrink-0">{fmtWhen(m.created_at)}</span>
                  </div>
                </div>
              ))}
              {!loading && items.length === 0 && (
                q ? (
                  <EmptyState icon={Search} title={t('mail.noResults')} action={
                    <Button size="sm" variant="outline" onClick={() => { setQ(''); load(1, sort) }}>{t('mail.clearSearch')}</Button>
                  } />
                ) : (
                  <EmptyState icon={folder === 'inbox' ? Inbox : Folder}
                    title={folder === 'inbox' ? t('mail.emptyInbox') : t('mail.emptyFolder')}
                    desc={folder === 'inbox' ? t('mail.emptyInboxDesc') : undefined}
                    action={folder === 'inbox' ? (
                      <Button size="sm" variant="outline" onClick={() => setShowCompose(true)}><PenLine />{t('mail.compose')}</Button>
                    ) : undefined} />
                )
              )}
            </div>
            <div className="border-t border-border p-2 flex items-center justify-between text-xs safe-bottom">
              <Button variant="ghost" size="sm" disabled={page <= 1} onClick={() => load(page - 1, sort)}>‹ {t('mail.prev')}</Button>
              <span className="text-muted-foreground">{page} / {totalPages}</span>
              <Button variant="ghost" size="sm" disabled={page >= totalPages} onClick={() => load(page + 1, sort)}>{t('mail.next')} ›</Button>
            </div>
          </div>

          {/* 详情 */}
          <div className={cn('flex-1 min-w-0 p-4 overflow-auto overscroll-contain', view === 'list' && 'hidden md:block')}>
            {!sel ? (
              <p className="text-muted-foreground text-sm mt-10 text-center">{t('mail.selectHint')}</p>
            ) : (
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
            )}
          </div>
        </section>
      </div>

      {/* 移动端：底部 Tab（拇指优先，#50） */}
      <nav role="navigation" aria-label={t('mail.folders')}
        className="md:hidden border-t border-border bg-background/95 backdrop-blur safe-bottom flex">
        {FOLDERS.map(f => (
          <button key={f.k} onClick={() => { setFolder(f.k); setView('list') }} aria-current={folder === f.k ? 'true' : undefined}
            className={cn('flex-1 flex flex-col items-center gap-0.5 py-2 text-[10px] min-h-[52px]', folder === f.k ? 'text-primary' : 'text-muted-foreground')}>
            <span className="relative">
              <f.icon size={19} />
              {unread[f.k] > 0 && <span className="absolute -top-1 -right-2 rounded-full bg-primary text-primary-foreground px-1 text-[9px] font-semibold">{fmtUnread(unread[f.k])}</span>}
            </span>
            {t(f.labelKey)}
          </button>
        ))}
        <Dropdown align="right" trigger={
          <button className="flex-1 flex flex-col items-center gap-0.5 py-2 text-[10px] min-h-[52px] text-muted-foreground" aria-label={t('mail.folders')}>
            <MoreVertical size={19} />{t('common.more')}
          </button>
        }>
          <DropdownLabel>{t('mail.folders')}</DropdownLabel>
          {allFolders.filter(f => !FOLDERS.some(x => x.k === f.k)).map(f => (
            <div key={f.k} className="flex items-center">
              <DropdownItem icon={f.icon} onClick={() => { setFolder(f.k); setView('list') }}>{f.labelKey ? t(f.labelKey) : f.label}</DropdownItem>
              {f.custom && <>
                <button className="px-1.5 text-muted-foreground" aria-label={t('common.edit')} onClick={() => renameFolder(f)}><PenLine size={13} /></button>
                <button className="px-1.5 text-destructive" aria-label={t('common.delete')} onClick={() => deleteFolder(f)}><Trash2 size={13} /></button>
              </>}
            </div>
          ))}
          <DropdownSeparator />
          <DropdownItem icon={Plus} onClick={createFolder}>{t('folders.new')}</DropdownItem>
        </Dropdown>
      </nav>

      {preview && (
        <div className="fixed inset-0 z-[90] grid place-items-center bg-black/70 p-4" onClick={() => setPreview(null)} role="dialog" aria-modal="true" aria-label={preview.name}>
          <div className="w-full max-w-4xl max-h-[92vh] flex flex-col" onClick={e => e.stopPropagation()}>
            <div className="flex items-center gap-2 text-white mb-2">
              <span className="truncate text-sm flex-1">{preview.name}</span>
              <a href={preview.dataUrl} download={preview.name} className="text-white/80 hover:text-white" aria-label={t('mail.download')}><Download size={16} /></a>
              <button onClick={() => setPreview(null)} autoFocus aria-label={t('common.close')} className="text-white/80 hover:text-white"><X size={18} /></button>
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

      {showHelp && (
        <div className="fixed inset-0 z-[95] grid place-items-center bg-black/50 p-4" onClick={() => setShowHelp(false)} role="dialog" aria-modal="true" aria-label={t('mail.shortcuts')}>
          <Card className="w-full max-w-md p-4 space-y-3" onClick={(e: any) => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <b className="text-sm">{t('mail.shortcuts')}</b>
              <button onClick={() => setShowHelp(false)} aria-label={t('common.close')}><X size={16} /></button>
            </div>
            <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
              {([['j / k', 'mail.scNext'], ['o / Enter', 'mail.scOpen'], ['u / Esc', 'mail.scBack'], ['s', 'mail.scStar'], ['e', 'mail.scRead'], ['#', 'mail.scDelete'], ['r', 'mail.scReply'], ['c', 'mail.scCompose'], ['/', 'mail.scSearch'], ['?', 'mail.scHelp']] as const).map(([k, label]) => (
                <Fragment key={k}>
                  <kbd className="justify-self-start rounded border border-border bg-muted px-1.5 py-0.5 text-xs font-mono">{k}</kbd>
                  <span className="text-muted-foreground">{t(label)}</span>
                </Fragment>
              ))}
            </div>
          </Card>
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
  }, [onClose])

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

function SendStatus({ m, folder }: any) {
  const { t } = useI18n()
  if (folder !== 'sent' || !m.status || m.status === 'sent') return null
  if (m.status === 'failed') return <span title={m.relay_err || ''} className="flex items-center gap-0.5 text-[10px] text-destructive shrink-0"><XCircle size={11} />{t('mail.stFailed')}</span>
  if (m.status === 'sending') return <span className="flex items-center gap-0.5 text-[10px] text-primary shrink-0"><Loader2 size={11} className="animate-spin" />{t('mail.stSending')}</span>
  return <span className="flex items-center gap-0.5 text-[10px] text-warning shrink-0"><Clock size={11} />{t('mail.stQueued')}</span>
}

function attList(s: any): any[] {
  try { const a = JSON.parse(s || '[]'); return Array.isArray(a) ? a : [] } catch { return [] }
}

// 发件人认证徽章：把 SPF/DKIM/DMARC 结果翻译成通过/未通过（#49）。
function AuthBadges({ raw, t }: { raw?: string; t: (k: string) => string }) {
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
