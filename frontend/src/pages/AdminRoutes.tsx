import { useEffect, useState } from 'react'
import { toast, confirmDestructive } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Label, Select } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { EmptyState } from '../components/EmptyState'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, Pencil, Check, X, FlaskConical, Route } from 'lucide-react'

const EMPTY = {
  domain: '', action: 'relay', relay_host: '', relay_port: '587', relay_user: '',
  relay_pass: '', relay_from: '', insecure: false, priority: 0, enabled: true,
}

export default function AdminRoutes() {
  const { t } = useI18n()
  const [list, setList] = useState<any[]>([])
  const [form, setForm] = useState<any>({ ...EMPTY })
  const [editId, setEditId] = useState<number | null>(null)
  const [testDomain, setTestDomain] = useState('')
  const [testResult, setTestResult] = useState<any>(null)

  async function load() {
    try { setList(await api.adminRoutes()) } catch {}
  }
  useEffect(() => { load() }, [])

  function reset() { setForm({ ...EMPTY }); setEditId(null) }

  async function save(e: any) {
    e.preventDefault()
    try {
      if (editId) await api.adminRoutePatch(editId, form)
      else await api.adminRouteCreate(form)
      reset()
      load()
    } catch (err: any) { toast(err.message) }
  }
  function startEdit(r: any) {
    setEditId(r.id)
    setForm({
      domain: r.domain, action: r.action, relay_host: r.relay_host || '', relay_port: r.relay_port || '587',
      relay_user: r.relay_user || '', relay_pass: '', relay_from: r.relay_from || '',
      insecure: !!r.insecure, priority: r.priority || 0, enabled: r.enabled,
    })
  }
  async function del(id: number) {
    if (!await confirmDestructive(t('routes.confirmDelete'))) return
    try { await api.adminRouteDelete(id); load() } catch (err: any) { toast(err.message) }
  }
  async function toggle(r: any) {
    try {
      await api.adminRoutePatch(r.id, {
        domain: r.domain, action: r.action, relay_host: r.relay_host, relay_port: r.relay_port,
        relay_user: r.relay_user, relay_from: r.relay_from, insecure: r.insecure, priority: r.priority, enabled: !r.enabled,
      })
      load()
    } catch (err: any) { toast(err.message) }
  }
  async function runTest() {
    setTestResult(null)
    try { setTestResult(await api.adminRouteTest(testDomain)) } catch (err: any) { setTestResult({ error: err.message }) }
  }

  const actionLabel = (a: string) =>
    a === 'discard' ? t('routes.discard') : a === 'direct' ? t('routes.direct') : t('routes.relay')

  return (
    <AdminShell title={t('routes.title')} desc={t('routes.desc')}>
      <Card className="p-4">
        <b className="text-sm">{editId ? t('routes.edit') : t('routes.create')}</b>
        <form onSubmit={save} className="space-y-3 mt-3">
          <div className="grid sm:grid-cols-3 gap-3">
            <div>
              <Label className="text-xs text-muted-foreground">{t('routes.domain')}</Label>
              <Input className="mt-1 font-mono" value={form.domain} onChange={e => setForm({ ...form, domain: e.target.value })} placeholder="example.com / .example.com" required />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('routes.action')}</Label>
              <Select className="mt-1" value={form.action} onChange={e => setForm({ ...form, action: e.target.value })}>
                <option value="relay">{t('routes.relay')}</option>
                <option value="direct">{t('routes.direct')}</option>
                <option value="discard">{t('routes.discard')}</option>
              </Select>
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('routes.priority')}</Label>
              <Input type="number" className="mt-1" value={form.priority} onChange={e => setForm({ ...form, priority: +e.target.value })} />
            </div>
          </div>

          {form.action === 'relay' && (
            <div className="grid sm:grid-cols-3 gap-3">
              <div>
                <Label className="text-xs text-muted-foreground">{t('routes.host')}</Label>
                <Input className="mt-1" value={form.relay_host} onChange={e => setForm({ ...form, relay_host: e.target.value })} placeholder="smtp.example.com" />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">{t('routes.port')}</Label>
                <Input className="mt-1" value={form.relay_port} onChange={e => setForm({ ...form, relay_port: e.target.value })} placeholder="587" />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">{t('routes.user')}</Label>
                <Input className="mt-1" value={form.relay_user} onChange={e => setForm({ ...form, relay_user: e.target.value })} />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">{t('routes.pass')}</Label>
                <Input className="mt-1" type="password" value={form.relay_pass} onChange={e => setForm({ ...form, relay_pass: e.target.value })} placeholder={editId ? t('routes.passKeep') : ''} />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">{t('routes.from')}</Label>
                <Input className="mt-1" value={form.relay_from} onChange={e => setForm({ ...form, relay_from: e.target.value })} />
              </div>
              <label className="flex items-center gap-2 text-sm pt-6">
                <input type="checkbox" checked={form.insecure} onChange={e => setForm({ ...form, insecure: e.target.checked })} />
                {t('routes.insecure')}
              </label>
            </div>
          )}

          <div className="flex items-center gap-2">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
              {t('routes.enabled')}
            </label>
            <div className="flex-1" />
            <Button size="sm" type="submit"><Check />{editId ? t('common.save') : t('routes.create')}</Button>
            {editId && <Button size="sm" variant="outline" type="button" onClick={reset}><X />{t('common.cancel')}</Button>}
          </div>
        </form>

        <div className="mt-4 border-t border-border pt-3">
          <Label className="text-xs text-muted-foreground">{t('routes.test')}</Label>
          <div className="flex gap-2 mt-1">
            <Input value={testDomain} onChange={e => setTestDomain(e.target.value)} placeholder="user@example.com 的域名" />
            <Button size="sm" variant="outline" type="button" onClick={runTest}><FlaskConical />{t('routes.testBtn')}</Button>
          </div>
          {testResult && (
            <p className="text-xs mt-2 text-muted-foreground">
              {testResult.error ? testResult.error
                : testResult.matched
                  ? t('routes.testHit', { domain: testResult.route.domain, action: actionLabel(testResult.route.action) })
                  : t('routes.testMiss')}
            </p>
          )}
        </div>
      </Card>

      <Card className="divide-y divide-border">
        {list.length === 0 && <EmptyState icon={Route} title={t('routes.empty')} desc={t('routes.emptyDesc')} />}
        {list.map(r => (
          <div key={r.id} className="p-3 flex items-start gap-3">
            <input type="checkbox" className="mt-1" checked={r.enabled} onChange={() => toggle(r)} aria-label={t('routes.enabled')} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-mono font-medium">{r.domain}</span>
                <Badge>{actionLabel(r.action)}</Badge>
                {r.action === 'relay' && r.relay_host && <span className="text-xs text-muted-foreground">{r.relay_host}:{r.relay_port || '587'}</span>}
                {r.priority ? <Badge>{t('routes.priority')} {r.priority}</Badge> : null}
              </div>
            </div>
            <Button variant="ghost" size="icon" onClick={() => startEdit(r)} aria-label={t('common.edit')}><Pencil /></Button>
            <Button variant="ghost" size="icon" onClick={() => del(r.id)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </AdminShell>
  )
}
