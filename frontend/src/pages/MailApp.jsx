import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Textarea, Card, Badge } from '../components/ui/controls'
import { Inbox, Send, FileEdit, Trash2, Star, Search, PenLine, LogOut, Moon, Sun, RefreshCw, Globe, Settings } from 'lucide-react'
import { cn } from '../lib/utils'

const FOLDERS = [
  { k: 'inbox', label: '收件箱', icon: Inbox },
  { k: 'sent', label: '已发送', icon: Send },
  { k: 'draft', label: '草稿', icon: FileEdit },
  { k: 'trash', label: '垃圾箱', icon: Trash2 },
]

export default function MailApp() {
  const [folder, setFolder] = useState('inbox')
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [q, setQ] = useState('')
  const [sel, setSel] = useState(null)
  const [showCompose, setShowCompose] = useState(false)
  const [dark, setDark] = useState(false)
  const [me, setMe] = useState(null)

  async function load() {
    try {
      const d = await api.list(folder, q)
      setItems(d.items); setTotal(d.total)
    } catch {}
  }
  useEffect(() => { api.me().then(setMe).catch(() => location.hash = '#/login'); }, [])
  useEffect(() => { setSel(null); load() }, [folder])
  useEffect(() => { document.documentElement.classList.toggle('dark', dark) }, [dark])

  async function open(id) {
    const d = await api.get(id)
    setSel(d)
    setItems(items.map(i => i.id === id ? { ...i, read: true } : i))
  }

  function logout() { localStorage.removeItem('token'); location.hash = '#/login' }

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3">
        <b>📮 Mailserver</b>
        {me && <Badge>{me.email}</Badge>}
        <div className="flex-1" />
        <a href="#/dns" title="域名 DNS"><Button variant="ghost" size="icon"><Globe /></Button></a>
        <a href="#/setup" title="客户端配置"><Button variant="ghost" size="icon"><Settings /></Button></a>
        <Button variant="ghost" size="icon" onClick={() => setDark(!dark)}>{dark ? <Sun /> : <Moon />}</Button>
        <Button variant="ghost" size="icon" onClick={load}><RefreshCw /></Button>
        <Button variant="ghost" size="icon" onClick={logout}><LogOut /></Button>
        <Button size="sm" onClick={() => setShowCompose(true)}><PenLine />写信</Button>
      </header>

      <div className="flex max-w-6xl mx-auto">
        <aside className="w-44 shrink-0 p-3 space-y-1 border-r border-border min-h-[calc(100vh-3.5rem)]">
          {FOLDERS.map(f => (
            <button key={f.k} onClick={() => setFolder(f.k)}
              className={cn('w-full flex items-center gap-2 rounded-md px-3 py-2 text-sm', folder === f.k ? 'bg-primary text-primary-foreground' : 'hover:bg-muted')}>
              <f.icon size={16} />{f.label}
            </button>
          ))}
          <p className="text-xs text-muted-foreground px-3 pt-4">共 {total} 封</p>
        </aside>

        <section className="flex-1 min-w-0 flex">
          <div className="w-80 shrink-0 border-r border-border flex flex-col">
            <div className="p-3 border-b border-border flex gap-2">
              <div className="relative flex-1">
                <Search size={14} className="absolute left-2 top-2.5 text-muted-foreground" />
                <Input className="pl-7" placeholder="搜索…" value={q}
                  onChange={e => setQ(e.target.value)} onKeyDown={e => e.key === 'Enter' && load()} />
              </div>
            </div>
            <div className="flex-1 overflow-auto">
              {items.map(m => (
                <button key={m.id} onClick={() => open(m.id)}
                  className={cn('w-full text-left px-3 py-2.5 border-b border-border hover:bg-muted/60', sel?.id === m.id && 'bg-muted', !m.read && 'font-semibold')}>
                  <div className="flex items-center gap-2 text-sm">
                    <span className="truncate flex-1">{folder === 'sent' ? m.to : m.from}</span>
                    <Star size={14} className={m.starred ? 'fill-yellow-400 text-yellow-400' : 'text-muted-foreground'}
                      onClick={async e => { e.stopPropagation(); await api.patch(m.id, { starred: !m.starred }); load() }} />
                  </div>
                  <div className="text-sm truncate">{m.subject || '(无主题)'}</div>
                  <div className="text-xs text-muted-foreground truncate">{m.body?.slice(0, 60)}</div>
                </button>
              ))}
              {items.length === 0 && <p className="p-6 text-sm text-muted-foreground text-center">空</p>}
            </div>
          </div>

          <div className="flex-1 min-w-0 p-4">
            {!sel ? <p className="text-muted-foreground text-sm mt-10 text-center">← 选择一封邮件查看</p> : (
              <Card className="p-5">
                <h2 className="text-lg font-semibold">{sel.subject}</h2>
                <p className="text-xs text-muted-foreground mt-1">从 {sel.from} → {sel.to} · {new Date(sel.created_at).toLocaleString()}</p>
                <div className="flex gap-2 mt-3">
                  <Button variant="outline" size="sm" onClick={async () => { await api.patch(sel.id, { starred: !sel.starred }); setSel({ ...sel, starred: !sel.starred }) }}>星标</Button>
                  <Button variant="outline" size="sm" onClick={async () => { await api.trash(sel.id); setSel(null); load() }}>删除</Button>
                  <Button variant="outline" size="sm" onClick={() => { setShowCompose({ to: sel.from, subject: 'Re: ' + sel.subject, body: '\n\n---\n' + sel.body }) }}>回复</Button>
                </div>
                <pre className="whitespace-pre-wrap text-sm mt-4 font-sans">{sel.body}</pre>
              </Card>
            )}
          </div>
        </section>
      </div>

      {showCompose && <Compose init={typeof showCompose === 'object' ? showCompose : {}} onClose={() => { setShowCompose(false); load() }} />}
    </div>
  )
}

function Compose({ init, onClose }) {
  const [f, setF] = useState({ to: init.to || '', subject: init.subject || '', body: init.body || '' })
  const [saving, setSaving] = useState(false)
  async function submit(folder) {
    setSaving(true)
    try { await api.send({ ...f, folder }); onClose() } catch (e) { alert(e.message) }
    finally { setSaving(false) }
  }
  return (
    <div className="fixed inset-0 bg-black/40 grid place-items-center p-4" onClick={onClose}>
      <Card className="w-full max-w-lg p-4 space-y-3" onClick={e => e.stopPropagation()}>
        <b>写邮件</b>
        <Input placeholder="收件人" value={f.to} onChange={e => setF({ ...f, to: e.target.value })} />
        <Input placeholder="主题" value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} />
        <Textarea rows={8} placeholder="正文…" value={f.body} onChange={e => setF({ ...f, body: e.target.value })} />
        <div className="flex gap-2 justify-end">
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button variant="outline" disabled={saving} onClick={() => submit('draft')}>存草稿</Button>
          <Button disabled={saving} onClick={() => submit('sent')}>发送</Button>
        </div>
      </Card>
    </div>
  )
}
