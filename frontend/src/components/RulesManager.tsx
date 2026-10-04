import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Textarea, Card, Badge, Label, Select } from './ui/controls'
import { SkeletonRows } from './Skeleton'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, Pencil, Check, X, FlaskConical } from 'lucide-react'

const EMPTY = { name: '', expression: '', action: 'trash', folder: 'trash', priority: 0, enabled: true }

// site=true 管理整站规则（/api/admin/rules），否则管理个人规则（/api/rules）。
export function RulesManager({ site = false }: { site?: boolean }) {
  const { t } = useI18n()
  const [rules, setRules] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState<any>({ ...EMPTY })
  const [editId, setEditId] = useState<number | null>(null)
  const [sample, setSample] = useState({ from: 'spam@example.com', subject: '你好', body: '' })
  const [testResult, setTestResult] = useState<any>(null)

  const apiList = site ? api.adminRules : api.rules
  const apiCreate = site ? api.adminRuleCreate : api.ruleCreate
  const apiPatch = site ? api.adminRulePatch : api.rulePatch
  const apiDelete = site ? api.adminRuleDelete : api.ruleDelete
  const apiTest = site ? api.adminRuleTest : api.ruleTest

  async function load() {
    try { setRules(await apiList()) } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [site])

  function reset() { setForm({ ...EMPTY }); setEditId(null) }

  async function save(e: any) {
    e.preventDefault()
    if (!form.expression.trim()) { alert(t('rules.needExpr')); return }
    try {
      if (editId) await apiPatch(editId, form)
      else await apiCreate(form)
      reset()
      load()
    } catch (err: any) { alert(err.message) }
  }
  function startEdit(r: any) {
    setEditId(r.id)
    setForm({ name: r.name, expression: r.expression, action: r.action || 'trash', folder: r.folder || 'trash', priority: r.priority || 0, enabled: r.enabled })
  }
  async function del(id: number) {
    if (!confirm(t('rules.confirmDelete'))) return
    try { await apiDelete(id); load() } catch (err: any) { alert(err.message) }
  }
  async function toggle(r: any) {
    try { await apiPatch(r.id, { enabled: !r.enabled }); load() } catch (err: any) { alert(err.message) }
  }
  async function runTest() {
    setTestResult(null)
    try {
      const r = await apiTest({ expression: form.expression, ...sample })
      setTestResult(r)
    } catch (err: any) { setTestResult({ error: err.message }) }
  }

  return (
    <>
      <Card className="p-4">
        <b className="text-sm">{editId ? t('rules.edit') : t('rules.create')}</b>
        <form onSubmit={save} className="space-y-3 mt-3">
          <div className="grid sm:grid-cols-2 gap-3">
            <div>
              <Label className="text-xs text-muted-foreground">{t('rules.name')}</Label>
              <Input className="mt-1" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} placeholder={t('rules.namePlaceholder')} />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('rules.priority')}</Label>
              <Input type="number" className="mt-1" value={form.priority} onChange={e => setForm({ ...form, priority: +e.target.value })} />
            </div>
          </div>
          <div>
            <Label className="text-xs text-muted-foreground">{t('rules.expr')}</Label>
            <Textarea rows={3} className="mt-1 font-mono text-xs" value={form.expression}
              onChange={e => setForm({ ...form, expression: e.target.value })}
              placeholder='subject.contains("促销") || from.endsWith("@spam.com")' />
            <p className="text-xs text-muted-foreground mt-1">{t('rules.exprHint')}</p>
          </div>
          <div className="grid sm:grid-cols-[1fr_1fr_auto] gap-3 items-end">
            <div>
              <Label className="text-xs text-muted-foreground">{t('rules.action')}</Label>
              <Select className="mt-1" value={form.action} onChange={e => setForm({ ...form, action: e.target.value })}>
                <option value="trash">{t('rules.actionTrash')}</option>
                <option value="move">{t('rules.actionMove')}</option>
              </Select>
            </div>
            {form.action === 'move' && (
              <div>
                <Label className="text-xs text-muted-foreground">{t('rules.folder')}</Label>
                <Select className="mt-1" value={form.folder} onChange={e => setForm({ ...form, folder: e.target.value })}>
                  <option value="trash">{t('mail.trash')}</option>
                  <option value="inbox">{t('mail.inbox')}</option>
                  <option value="draft">{t('mail.draft')}</option>
                </Select>
              </div>
            )}
            <label className="flex items-center gap-2 text-sm pb-2">
              <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
              {t('rules.enabled')}
            </label>
          </div>
          <div className="flex items-center gap-2">
            <Button size="sm" type="submit"><Check />{editId ? t('common.save') : t('rules.create')}</Button>
            {editId && <Button size="sm" variant="outline" type="button" onClick={reset}><X />{t('common.cancel')}</Button>}
            <div className="flex-1" />
            <Button size="sm" variant="outline" type="button" onClick={runTest}><FlaskConical />{t('rules.test')}</Button>
          </div>
        </form>

        <div className="mt-4 border-t border-border pt-3">
          <Label className="text-xs text-muted-foreground">{t('rules.testSample')}</Label>
          <div className="grid sm:grid-cols-2 gap-2 mt-1">
            <Input value={sample.from} onChange={e => setSample({ ...sample, from: e.target.value })} placeholder="from" />
            <Input value={sample.subject} onChange={e => setSample({ ...sample, subject: e.target.value })} placeholder="subject" />
          </div>
          {testResult && (
            <p className={`text-xs mt-2 ${testResult.error ? 'text-red-500' : testResult.matched ? 'text-green-600' : 'text-muted-foreground'}`}>
              {testResult.error ? testResult.error : testResult.matched ? t('rules.testHit') : t('rules.testMiss')}
            </p>
          )}
        </div>
      </Card>

      <Card className="divide-y divide-border">
        {loading && <SkeletonRows rows={3} />}
        {!loading && rules.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('rules.empty')}</p>}
        {!loading && rules.map(r => (
          <div key={r.id} className="p-3 flex items-start gap-3">
            <input type="checkbox" className="mt-1" checked={r.enabled} onChange={() => toggle(r)} aria-label={t('rules.enabled')} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium truncate">{r.name}</span>
                <Badge>{r.action === 'move' ? `${t('rules.actionMove')} → ${r.folder}` : t('rules.actionTrash')}</Badge>
                {r.priority ? <Badge>{t('rules.priority')} {r.priority}</Badge> : null}
              </div>
              <code className="text-xs text-muted-foreground break-all">{r.expression}</code>
            </div>
            <Button variant="ghost" size="icon" onClick={() => startEdit(r)} aria-label={t('common.edit')}><Pencil /></Button>
            <Button variant="ghost" size="icon" onClick={() => del(r.id)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </>
  )
}
