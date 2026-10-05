import { useEffect, useRef, useState } from 'react'
import { toast, confirmDestructive } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Label } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { SkeletonList } from '../components/Skeleton'
import { EmptyState } from '../components/EmptyState'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, Pencil, Check, X, Search, UserRound, Upload } from 'lucide-react'

export default function Contacts() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [q, setQ] = useState('')
  const [form, setForm] = useState({ name: '', email: '', note: '' })
  const [editId, setEditId] = useState<number | null>(null)
  const [edit, setEdit] = useState({ name: '', email: '', note: '' })
  const fileRef = useRef<HTMLInputElement>(null)

  async function load() {
    try { setList(await api.contacts()) } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  async function add(e: any) {
    e.preventDefault()
    if (!form.email) return
    try {
      await api.contactCreate(form)
      setForm({ name: '', email: '', note: '' })
      load()
    } catch (err: any) { toast(err.message) }
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
    } catch (err: any) { toast(err.message) }
  }
  async function del(id: number) {
    if (!await confirmDestructive(t('contacts.confirmDelete'))) return
    try { await api.contactDelete(id); load() } catch (err: any) { toast(err.message) }
  }

  const filtered = list.filter(c => `${c.name} ${c.email} ${c.note}`.toLowerCase().includes(q.toLowerCase()))

  // 导入 CSV/vCard 联系人（#51）：自动识别 email/name/note 列，逐条创建。
  async function importCSV(file: File) {
    const rows = parseCSV(await file.text())
    if (!rows.length) { toast(t('contacts.importEmpty')); return }
    let emailIdx = 0, nameIdx = 1, noteIdx = 2, start = 0
    const header = rows[0].map(s => s.toLowerCase())
    const ei = header.findIndex(h => h.includes('email') || h.includes('mail') || h.includes('邮箱'))
    if (ei >= 0) {
      emailIdx = ei
      const ni = header.findIndex(h => h.includes('name') || h.includes('姓名') || h.includes('昵称'))
      const oi = header.findIndex(h => h.includes('note') || h.includes('备注'))
      nameIdx = ni; noteIdx = oi; start = 1
    }
    let n = 0, skip = 0
    for (let i = start; i < rows.length; i++) {
      const r = rows[i]
      const email = (r[emailIdx] || '').trim()
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) { skip++; continue }
      try {
        await api.contactCreate({ email, name: nameIdx >= 0 ? (r[nameIdx] || '').trim() : '', note: noteIdx >= 0 ? (r[noteIdx] || '').trim() : '' })
        n++
      } catch { skip++ }
    }
    toast(t('contacts.imported', { n, skip }), { type: 'success' })
    load()
  }

  return (
    <PageShell title={t('contacts.title')} icon={UserRound}>
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

        <div className="flex items-center gap-2">
          <div className="relative flex-1">
            <Search size={14} className="absolute left-2 top-2.5 text-muted-foreground" />
            <Input className="pl-7" placeholder={t('contacts.search')} value={q} onChange={e => setQ(e.target.value)} />
          </div>
          <Button variant="outline" size="sm" className="h-9 shrink-0" onClick={() => fileRef.current?.click()}><Upload />{t('contacts.import')}</Button>
          <input ref={fileRef} type="file" accept=".csv,text/csv" className="hidden"
            onChange={e => { const f = e.target.files?.[0]; if (f) importCSV(f); e.target.value = '' }} />
        </div>

        <Card className="divide-y divide-border">
          {loading && <SkeletonList rows={4} />}
          {!loading && filtered.length === 0 && <EmptyState icon={UserRound} title={t('contacts.empty')} desc={t('contacts.emptyDesc')} />}
          {!loading && filtered.map(c => (
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
    </PageShell>
  )
}

// 极简 CSV 解析：支持引号包裹与逗号/换行分隔。
function parseCSV(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cur = ''
  let inQ = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (inQ) {
      if (c === '"') {
        if (text[i + 1] === '"') { cur += '"'; i++ } else inQ = false
      } else cur += c
    } else if (c === '"') inQ = true
    else if (c === ',') { row.push(cur); cur = '' }
    else if (c === '\n') { row.push(cur); rows.push(row); row = []; cur = '' }
    else if (c !== '\r') cur += c
  }
  if (cur !== '' || row.length) { row.push(cur); rows.push(row) }
  return rows.filter(r => r.some(x => x.trim() !== ''))
}
