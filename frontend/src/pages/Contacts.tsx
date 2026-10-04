import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Card, Label } from '../components/ui/controls'
import { ThemeToggle, LangToggle } from '../components/HeaderControls'
import { useI18n } from '../lib/i18n'
import { BRAND } from '../lib/brand'
import { Plus, Trash2, Pencil, Check, X, Search, UserRound } from 'lucide-react'

export default function Contacts() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [q, setQ] = useState('')
  const [form, setForm] = useState({ name: '', email: '', note: '' })
  const [editId, setEditId] = useState<number | null>(null)
  const [edit, setEdit] = useState({ name: '', email: '', note: '' })

  async function load() {
    try { setList(await api.contacts()) } catch {}
  }
  useEffect(() => { load() }, [])

  async function add(e: any) {
    e.preventDefault()
    if (!form.email) return
    try {
      await api.contactCreate(form)
      setForm({ name: '', email: '', note: '' })
      load()
    } catch (err: any) { alert(err.message) }
  }
  function startEdit(c: any) {
    setEditId(c.id)
    setEdit({ name: c.name, email: c.email, note: c.note })
  }
  async function saveEdit() {
    try {
      await api.contactPatch(editId, edit)
      setEditId(null)
      load()
    } catch (err: any) { alert(err.message) }
  }
  async function del(id: number) {
    if (!confirm(t('contacts.confirmDelete'))) return
    try { await api.contactDelete(id); load() } catch (err: any) { alert(err.message) }
  }

  const filtered = list.filter(c => `${c.name} ${c.email} ${c.note}`.toLowerCase().includes(q.toLowerCase()))

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3 sticky top-0 bg-background/90 backdrop-blur z-10">
        <Link to="/" className="font-semibold">← {BRAND}</Link>
        <span className="flex items-center gap-1 text-sm text-muted-foreground"><UserRound size={15} />{t('contacts.title')}</span>
        <div className="flex-1" />
        <LangToggle />
        <ThemeToggle />
      </header>

      <div className="max-w-3xl mx-auto p-4 space-y-4">
        <Card className="p-4">
          <b className="text-sm">{t('contacts.add')}</b>
          <form onSubmit={add} className="grid sm:grid-cols-[1fr_1fr_1.2fr_auto] gap-2 mt-2 items-end">
            <div>
              <Label className="text-xs text-muted-foreground">{t('contacts.name')}</Label>
              <Input className="mt-1" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('contacts.email')}</Label>
              <Input className="mt-1" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} placeholder="name@example.com" required />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('contacts.note')}</Label>
              <Input className="mt-1" value={form.note} onChange={e => setForm({ ...form, note: e.target.value })} />
            </div>
            <Button type="submit" size="sm" className="h-9"><Plus />{t('contacts.addBtn')}</Button>
          </form>
        </Card>

        <div className="relative">
          <Search size={14} className="absolute left-2 top-2.5 text-muted-foreground" />
          <Input className="pl-7" placeholder={t('contacts.search')} value={q} onChange={e => setQ(e.target.value)} />
        </div>

        <Card className="divide-y divide-border">
          {filtered.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('contacts.empty')}</p>}
          {filtered.map(c => (
            <div key={c.id} className="p-3">
              {editId === c.id ? (
                <div className="grid sm:grid-cols-[1fr_1fr_1.2fr_auto] gap-2 items-center">
                  <Input value={edit.name} onChange={e => setEdit({ ...edit, name: e.target.value })} placeholder={t('contacts.name')} />
                  <Input value={edit.email} onChange={e => setEdit({ ...edit, email: e.target.value })} />
                  <Input value={edit.note} onChange={e => setEdit({ ...edit, note: e.target.value })} placeholder={t('contacts.note')} />
                  <div className="flex gap-1">
                    <Button variant="ghost" size="icon" onClick={saveEdit} aria-label={t('common.save')}><Check /></Button>
                    <Button variant="ghost" size="icon" onClick={() => setEditId(null)} aria-label={t('common.cancel')}><X /></Button>
                  </div>
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium truncate">{c.name || c.email}</div>
                    {c.name && <div className="text-xs text-muted-foreground truncate">{c.email}</div>}
                    {c.note && <div className="text-xs text-muted-foreground truncate">📝 {c.note}</div>}
                  </div>
                  <Button variant="ghost" size="icon" onClick={() => startEdit(c)} aria-label={t('common.edit')}><Pencil /></Button>
                  <Button variant="ghost" size="icon" onClick={() => del(c.id)} aria-label={t('common.delete')}><Trash2 /></Button>
                </div>
              )}
            </div>
          ))}
        </Card>
      </div>
    </div>
  )
}
