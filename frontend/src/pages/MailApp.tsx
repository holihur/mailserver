import { Fragment, useEffect, useRef, useState } from 'react'
import { toast, confirmDestructive, promptAsync } from '../lib/ui'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Card, Select } from '../components/ui/controls'
import { Dropdown, DropdownItem, DropdownSeparator, DropdownLabel } from '../components/Dropdown'
import { SkeletonList } from '../components/Skeleton'
import { EmptyState } from '../components/EmptyState'
import { SettingsMenu } from '../components/HeaderControls'
import MailReader from '../components/MailReader'
import { MailListItem } from '../components/MailListItem'
import Compose from '../components/Compose'
import { fmtUnread, sumUnread } from '../lib/mailFormat'
import { useI18n } from '../lib/i18n'
import {
  Inbox, Send, FileEdit, Trash2, Trash, Star, Search, PenLine, LogOut,
  RefreshCw, Globe, Settings, ShieldCheck, Loader2, X,
  ChevronDown, MoreVertical, MailOpen, RotateCcw,
  Contact, Filter, KeyRound, AtSign, Download, Folder, Plus, FileCode, Clock, Mail, Upload, Sparkles,
} from 'lucide-react'
import { cn, setUnreadBadge, quoteMail } from '../lib/utils'
import { BRAND } from '../lib/brand'

const FOLDERS = [
  { k: 'inbox', labelKey: 'mail.inbox', icon: Inbox },
  { k: 'sent', labelKey: 'mail.sent', icon: Send },
  { k: 'draft', labelKey: 'mail.draft', icon: FileEdit },
  { k: 'trash', labelKey: 'mail.trash', icon: Trash2 },
  { k: 'deleted', labelKey: 'mail.deleted', icon: Trash },
]

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
  const [recipient, setRecipient] = useState('')
  const [recipients, setRecipients] = useState<string[]>([])
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
  const firstRecipient = useRef(true)
  const lastFocusRefresh = useRef(0)
  const notifiedFailed = useRef<Set<number>>(new Set())
  const failedSeeded = useRef(false)
  const [preview, setPreview] = useState<any>(null)
  const [showHelp, setShowHelp] = useState(false)
  const [showImages, setShowImages] = useState(false)
  const bodyRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  // 供 effect 读取最新的函数/状态，避免把它们放进依赖导致重复触发。
  const latest = useRef<any>({})
  const importRef = useRef<HTMLInputElement>(null)
  const pageSize = 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  async function load(p = page, s = sort, silent = false, g = group) {
    if (!silent) setLoading(true)
    try {
      const effQ = recipient ? (q.trim() ? `to:${recipient} ${q.trim()}` : `to:${recipient}`) : q
      const d = await api.list(folder, effQ, p, s, g ? 'thread' : '')
      setItems(d.items); setTotal(d.total); setPage(d.page || p)
    } catch {} finally { if (!silent) setLoading(false) }
    api.unread().then((u: any) => {
      setUnread(u)
      prevUnread.current = sumUnread(u)
    }).catch(() => {})
    api.folders().then(setFolders).catch(() => {})
  }
  useEffect(() => { api.me().then((m: any) => { setMe(m); if (m?.must_change_password) navigate('/security') }).catch(() => { location.href = '/login' }) }, [navigate])
  useEffect(() => { setSel(null); setView('list'); setPage(1); latest.current.load(1, latest.current.sort) }, [folder])
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
        if (folder === 'inbox' && page === 1 && !q) latest.current.load(1, latest.current.sort, true)
      }
      prevUnread.current = tot
      setUnread(u)
      // 已发送：轮询刷新投递状态
      if (folder === 'sent' && page === 1 && !q) latest.current.load(1, latest.current.sort, true)
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
      latest.current.load(page, latest.current.sort, true)
    }
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('focus', onVisible)
    return () => {
      clearInterval(id)
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('focus', onVisible)
    }
  }, [t, folder, q, page])
  // PWA 安装
  useEffect(() => {
    const h = (e: any) => { e.preventDefault(); setInstallEvt(e) }
    window.addEventListener('beforeinstallprompt', h)
    return () => window.removeEventListener('beforeinstallprompt', h)
  }, [])
  // 搜索防抖（300ms）
  useEffect(() => {
    if (firstQ.current) { firstQ.current = false; return }
    const id = setTimeout(() => { setPage(1); latest.current.load(1, latest.current.sort) }, 300)
    return () => clearTimeout(id)
  }, [q])

  // 收件账户筛选：由 effect 驱动重载，避免在 onChange 里 setState 后同步调用 load
  // 读到尚未更新的旧值（React 状态更新不会在同一事件内立即生效）。
  useEffect(() => {
    if (firstRecipient.current) { firstRecipient.current = false; return }
    setPage(1)
    latest.current.load(1, latest.current.sort)
  }, [recipient])

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
      const focus = (i: number) => { if (i >= 0 && i < items.length) latest.current.open(items[i].id) }
      switch (e.key) {
        case 'j': case 'ArrowDown': e.preventDefault(); focus(idx < 0 ? 0 : idx + 1); break
        case 'k': case 'ArrowUp': e.preventDefault(); focus(idx < 0 ? items.length - 1 : idx - 1); break
        case 'Enter': case 'o': if (idx >= 0) { e.preventDefault(); latest.current.open(items[idx].id) } break
        case 'u': case 'Escape': if (sel) { e.preventDefault(); navigate('/') } break
        case 's': if (sel) { e.preventDefault(); api.patch(sel.id, { starred: !sel.starred }).then(() => { setSel({ ...sel, starred: !sel.starred }); latest.current.load() }) } break
        case 'e': if (sel) { e.preventDefault(); setItems(items.map((i: any) => i.id === sel.id ? { ...i, read: true } : i)); api.patch(sel.id, { read: true }) } break
        case '#': if (sel) { e.preventDefault(); api.trash(sel.id).then(() => { setSel(null); navigate('/'); latest.current.load() }) } break
        case 'r': if (sel) { e.preventDefault(); latest.current.reply(sel, false) } break
        case 'c': e.preventDefault(); setShowCompose(true); break
        case '/': e.preventDefault(); searchRef.current?.focus(); break
        case '?': e.preventDefault(); setShowHelp(true); break
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [items, sel, showCompose, preview, showHelp, navigate])

  // 路由化阅读（#43）：/m/:id 可直接打开、刷新与分享。
  useEffect(() => {
    if (routeId) {
      if (String(latest.current.sel?.id) !== routeId) latest.current.open(routeId)
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

  useEffect(() => { api.recipients().then((rs: any) => setRecipients(Array.isArray(rs) ? rs : [])).catch(() => {}) }, [])

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

  // 每次渲染刷新，供上面的 effect 在回调时读取最新引用。
  latest.current = { load, open, reply, sort, sel }

  return (
    <div className="h-dvh bg-background flex flex-col overflow-hidden">
      <header className="border-b border-border px-3 sm:px-4 h-14 flex items-center gap-2 sticky top-0 bg-background/90 backdrop-blur z-10">
        <b className="shrink-0 truncate max-w-[45vw] flex items-center gap-2 text-lg"><Mail size={20} />{BRAND}</b>
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
          <DropdownItem icon={Sparkles} onClick={() => navigate('/ai')}>{t('nav.ai')}</DropdownItem>
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
              {recipients.length > 0 && (
                <Select value={recipient} className="h-9 w-auto text-xs shrink-0" aria-label={t('mail.recipientFilter')}
                  onChange={e => setRecipient(e.target.value)}>
                  <option value="">{t('mail.allRecipients')}</option>
                  {recipients.map(r => <option key={r} value={r}>{r}</option>)}
                </Select>
              )}
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
                <MailListItem key={m.id} m={m} folder={folder} sel={sel} me={me}
                  selectMode={selectMode} checked={checked.includes(m.id)} touch={touch}
                  open={open} toggleCheck={toggleCheck} load={load} />
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
              <MailReader
                sel={sel} setSel={setSel} items={items} setItems={setItems} load={load}
                folder={folder} thread={thread} me={me} open={open} reply={reply}
                forwardMail={forwardMail} showImages={showImages} revealImages={revealImages}
                bodyRef={bodyRef} onBodyClick={onBodyClick} setPreview={setPreview} />
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
