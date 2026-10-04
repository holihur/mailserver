import { useEffect, useState } from 'react'
import { toast, confirmAsync } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Label } from '../components/ui/controls'
import { RecipientInput } from '../components/RecipientInput'
import { SkeletonRows } from '../components/Skeleton'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, Pencil, Check, X } from 'lucide-react'

const EMPTY = { source: '', targets: '', keep: false, enabled: true }

export default function AdminAliases() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState<any>({ ...EMPTY })
  const [editId, setEditId] = useState<number | null>(null)
  const [sugg, setSugg] = useState<any[]>([])

  async function load() {
    try { setList(await api.adminAliases()) } finally { setLoading(false) }
  }
  useEffect(() => {
    load()
    Promise.all([api.contacts().catch(() => []), api.directory().catch(() => [])]).then(([cs, dir]) => {
      const map = new Map<string, any>()
      for (const d of dir) map.set(d.email.toLowerCase(), { email: d.email, name: d.name })
      for (const c of cs) map.set(c.email.toLowerCase(), { email: c.email, name: c.name, note: c.note })
      setSugg([...map.values()])
    })
  }, [])

  function reset() { setForm({ ...EMPTY }); setEditId(null) }

  async function save(e: any) {
    e.preventDefault()
    try {
      if (editId) await api.adminAliasPatch(editId, form)
      else await api.adminAliasCreate(form)
      reset()
      load()
    } catch (err: any) { toast(err.message) }
  }
  function startEdit(a: any) {
    setEditId(a.id)
    setForm({ source: a.source, targets: a.targets, keep: a.keep, enabled: a.enabled })
  }
  async function del(id: number) {
    if (!await confirmAsync(t('aliases.confirmDelete'))) return
    try { await api.adminAliasDelete(id); load() } catch (err: any) { toast(err.message) }
  }
  async function toggle(a: any) {
    try { await api.adminAliasPatch(a.id, { enabled: !a.enabled }); load() } catch (err: any) { toast(err.message) }
  }

  return (
    <AdminShell title={t('aliases.title')} desc={t('aliases.desc')}>
      <Card className="p-4">
        <b className="text-sm">{editId ? t('aliases.edit') : t('aliases.create')}</b>
        <form onSubmit={save} className="space-y-3 mt-3">
          <div>
            <Label className="text-xs text-muted-foreground">{t('aliases.source')}</Label>
            <Input className="mt-1 font-mono" value={form.source} onChange={e => setForm({ ...form, source: e.target.value })}
              placeholder="abc@example.com / @example.com" required />
            <p className="text-xs text-muted-foreground mt-1">{t('aliases.sourceHint')}</p>
          </div>
          <div>
            <Label className="text-xs text-muted-foreground">{t('aliases.targets')}</Label>
            <div className="mt-1">
              <RecipientInput placeholder="d@example.com, e@other.com" value={form.targets} onChange={v => setForm({ ...form, targets: v })} suggestions={sugg} />
            </div>
            <p className="text-xs text-muted-foreground mt-1">{t('aliases.targetsHint')}</p>
          </div>
          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.keep} onChange={e => setForm({ ...form, keep: e.target.checked })} />
              {t('aliases.keep')}
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
              {t('aliases.enabled')}
            </label>
            <div className="flex-1" />
            <Button size="sm" type="submit"><Check />{editId ? t('common.save') : t('aliases.create')}</Button>
            {editId && <Button size="sm" variant="outline" type="button" onClick={reset}><X />{t('common.cancel')}</Button>}
          </div>
        </form>
      </Card>

      <Card className="divide-y divide-border">
        {loading && <SkeletonRows rows={3} />}
        {!loading && list.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('aliases.empty')}</p>}
        {!loading && list.map(a => (
          <div key={a.id} className="p-3 flex items-start gap-3">
            <input type="checkbox" className="mt-1" checked={a.enabled} onChange={() => toggle(a)} aria-label={t('aliases.enabled')} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-mono font-medium">{a.source}</span>
                <span className="text-muted-foreground">→</span>
                <span className="text-sm break-all">{a.targets}</span>
                {a.keep && <Badge>{t('aliases.keepBadge')}</Badge>}
              </div>
            </div>
            <Button variant="ghost" size="icon" onClick={() => startEdit(a)} aria-label={t('common.edit')}><Pencil /></Button>
            <Button variant="ghost" size="icon" onClick={() => del(a.id)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </AdminShell>
  )
}
