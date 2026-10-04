import { useEffect, useState } from 'react'
import { toast, confirmAsync } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Label } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { SkeletonRows } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, Pencil, Check, X, Plug, AtSign } from 'lucide-react'

const EMPTY = {
  email: '', name: '',
  imap_host: '', imap_port: '993', imap_ssl: true, imap_user: '', imap_pass: '',
  smtp_host: '', smtp_port: '465', smtp_ssl: true, smtp_user: '', smtp_pass: '',
  enabled: true,
}

export default function Accounts() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState<any>({ ...EMPTY })
  const [editId, setEditId] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  async function load() {
    try { setList(await api.external()) } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  function reset() { setForm({ ...EMPTY }); setEditId(null) }

  async function save(e: any) {
    e.preventDefault()
    try {
      if (editId) await api.externalPatch(editId, form)
      else await api.externalCreate(form)
      reset()
      load()
    } catch (err: any) { toast(err.message) }
  }
  function startEdit(a: any) {
    setEditId(a.id)
    setForm({
      email: a.email, name: a.name || '',
      imap_host: a.imap_host, imap_port: a.imap_port || '993', imap_ssl: !!a.imap_ssl, imap_user: a.imap_user || '', imap_pass: '',
      smtp_host: a.smtp_host || '', smtp_port: a.smtp_port || '465', smtp_ssl: !!a.smtp_ssl, smtp_user: a.smtp_user || '', smtp_pass: '',
      enabled: a.enabled,
    })
  }
  async function del(id: number) {
    if (!await confirmAsync(t('accounts.confirmDelete'))) return
    try { await api.externalDelete(id); load() } catch (err: any) { toast(err.message) }
  }
  async function test(id: number) {
    setBusy(true)
    try {
      const r = await api.externalTest(id)
      toast(r.ok ? t('accounts.testOk') : r.error)
      load()
    } catch (err: any) { toast(err.message) } finally { setBusy(false) }
  }

  return (
    <PageShell title={t('accounts.title')} icon={AtSign}>
      <p className="text-sm text-muted-foreground">{t('accounts.desc')}</p>

      <Card className="p-4">
        <b className="text-sm">{editId ? t('accounts.edit') : t('accounts.create')}</b>
        <form onSubmit={save} className="space-y-3 mt-3">
          <div className="grid sm:grid-cols-2 gap-3">
            <div>
              <Label className="text-xs text-muted-foreground">{t('accounts.email')}</Label>
              <Input className="mt-1" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} placeholder="me@gmail.com" required />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('accounts.name')}</Label>
              <Input className="mt-1" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
          </div>

          <div className="rounded-md border border-border p-3 space-y-2">
            <b className="text-xs text-muted-foreground">{t('accounts.imap')}</b>
            <div className="grid sm:grid-cols-[2fr_1fr_auto] gap-2 items-end">
              <Input value={form.imap_host} onChange={e => setForm({ ...form, imap_host: e.target.value })} placeholder="imap.example.com" />
              <Input value={form.imap_port} onChange={e => setForm({ ...form, imap_port: e.target.value })} placeholder="993" />
              <label className="flex items-center gap-2 text-sm pb-2"><input type="checkbox" checked={form.imap_ssl} onChange={e => setForm({ ...form, imap_ssl: e.target.checked })} />SSL</label>
            </div>
            <div className="grid sm:grid-cols-2 gap-2">
              <Input value={form.imap_user} onChange={e => setForm({ ...form, imap_user: e.target.value })} placeholder={t('accounts.user')} />
              <Input type="password" value={form.imap_pass} onChange={e => setForm({ ...form, imap_pass: e.target.value })} placeholder={editId ? t('accounts.passKeep') : t('accounts.pass')} />
            </div>
          </div>

          <div className="rounded-md border border-border p-3 space-y-2">
            <b className="text-xs text-muted-foreground">{t('accounts.smtp')}</b>
            <div className="grid sm:grid-cols-[2fr_1fr_auto] gap-2 items-end">
              <Input value={form.smtp_host} onChange={e => setForm({ ...form, smtp_host: e.target.value })} placeholder="smtp.example.com" />
              <Input value={form.smtp_port} onChange={e => setForm({ ...form, smtp_port: e.target.value })} placeholder="465" />
              <label className="flex items-center gap-2 text-sm pb-2"><input type="checkbox" checked={form.smtp_ssl} onChange={e => setForm({ ...form, smtp_ssl: e.target.checked })} />SSL</label>
            </div>
            <div className="grid sm:grid-cols-2 gap-2">
              <Input value={form.smtp_user} onChange={e => setForm({ ...form, smtp_user: e.target.value })} placeholder={t('accounts.user')} />
              <Input type="password" value={form.smtp_pass} onChange={e => setForm({ ...form, smtp_pass: e.target.value })} placeholder={editId ? t('accounts.passKeep') : t('accounts.pass')} />
            </div>
          </div>

          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
              {t('accounts.enabled')}
            </label>
            <div className="flex-1" />
            <Button size="sm" type="submit"><Check />{editId ? t('common.save') : t('accounts.create')}</Button>
            {editId && <Button size="sm" variant="outline" type="button" onClick={reset}><X />{t('common.cancel')}</Button>}
          </div>
        </form>
      </Card>

      <Card className="divide-y divide-border">
        {loading && <SkeletonRows rows={3} />}
        {!loading && list.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('accounts.empty')}</p>}
        {!loading && list.map(a => (
          <div key={a.id} className="p-3 flex items-start gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium truncate">{a.name || a.email}</span>
                {a.name && <span className="text-xs text-muted-foreground truncate">{a.email}</span>}
                {a.enabled ? <Badge className="text-green-600">{t('common.enabled')}</Badge> : <Badge className="text-yellow-600">{t('common.disabled')}</Badge>}
              </div>
              <div className="text-xs text-muted-foreground truncate">
                IMAP {a.imap_host}:{a.imap_port} · SMTP {a.smtp_host || '—'}
                {a.last_error && <span className="text-red-500"> · {a.last_error}</span>}
              </div>
            </div>
            <Button variant="ghost" size="icon" disabled={busy} onClick={() => test(a.id)} aria-label={t('accounts.test')}><Plug /></Button>
            <Button variant="ghost" size="icon" onClick={() => startEdit(a)} aria-label={t('common.edit')}><Pencil /></Button>
            <Button variant="ghost" size="icon" onClick={() => del(a.id)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </PageShell>
  )
}
