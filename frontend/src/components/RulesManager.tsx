import { useCallback, useEffect, useState } from 'react'
import { toast, confirmDestructive } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Textarea, Card, Badge, Label, Select } from './ui/controls'
import { SkeletonRows } from './Skeleton'
import { useI18n } from '../lib/i18n'
import { Trash2, Pencil, Check, X, FlaskConical, RotateCcw } from 'lucide-react'

const DEFAULT_EXPR = 'subject.contains("促销") || from.endsWith("@spam.com")'
const EMPTY = { name: '', expression: DEFAULT_EXPR, action: 'trash', folder: 'trash', forward_to: '', priority: 0, enabled: true, shadow: false }
const RULE_EXAMPLES = [
  { labelKey: 'rules.exSpam', expr: 'subject.contains("促销") || subject.contains("优惠")' },
  { labelKey: 'rules.exFrom', expr: 'from.endsWith("@example.com")' },
  { labelKey: 'rules.exAttach', expr: 'attachments > 0' },
  { labelKey: 'rules.exLarge', expr: 'size > 5 * 1024 * 1024' },
]

// site=true 管理整站规则（/api/admin/rules），否则管理个人规则（/api/rules）。
export function RulesManager({ site = false }: { site?: boolean }) {
  const { t } = useI18n()
  const [rules, setRules] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState<any>({ ...EMPTY })
  const [editId, setEditId] = useState<number | null>(null)
  const [sample, setSample] = useState({ from: 'spam@example.com', subject: '你好', body: '' })
  const [testResult, setTestResult] = useState<any>(null)
  const [folders, setFolders] = useState<any[]>([])
  const [replayId, setReplayId] = useState<number | null>(null)
  const [replay, setReplay] = useState({ folder: 'inbox', limit: 100 })
  const [replayResult, setReplayResult] = useState<any>(null)
  useEffect(() => { api.folders().then(setFolders).catch(() => {}) }, [])

  const apiList = site ? api.adminRules : api.rules
  const apiCreate = site ? api.adminRuleCreate : api.ruleCreate
  const apiPatch = site ? api.adminRulePatch : api.rulePatch
  const apiDelete = site ? api.adminRuleDelete : api.ruleDelete
  const apiTest = site ? api.adminRuleTest : api.ruleTest
  const apiApply = site ? api.adminRuleApply : api.ruleApply

  const load = useCallback(async () => {
    try { setRules(await apiList()) } finally { setLoading(false) }
  }, [apiList])
  useEffect(() => { load() }, [load])

  function reset() { setForm({ ...EMPTY }); setEditId(null) }

  async function save(e: any) {
    e.preventDefault()
    if (!form.expression.trim()) { toast(t('rules.needExpr')); return }
    try {
      if (editId) await apiPatch(editId, form)
      else await apiCreate(form)
      reset()
      load()
    } catch (err: any) { toast(err.message) }
  }
  function startEdit(r: any) {
    setEditId(r.id)
    setForm({ name: r.name, expression: r.expression, action: r.action || 'trash', folder: r.folder || 'trash', forward_to: r.forward_to || '', priority: r.priority || 0, enabled: r.enabled, shadow: !!r.shadow })
  }
  async function del(id: number) {
    if (!await confirmDestructive(t('rules.confirmDelete'))) return
    try { await apiDelete(id); load() } catch (err: any) { toast(err.message) }
  }
  async function toggle(r: any) {
    try { await apiPatch(r.id, { enabled: !r.enabled }); load() } catch (err: any) { toast(err.message) }
  }
  async function runTest() {
    setTestResult(null)
    try {
      const r = await apiTest({ expression: form.expression, ...sample })
      setTestResult(r)
    } catch (err: any) { setTestResult({ error: err.message }) }
  }
  // 回放：把规则应用到已有邮件（dry=true 仅预览）
  async function doReplay(id: number, dry: boolean) {
    try {
      const res: any = await apiApply(id, { folder: replay.folder, limit: replay.limit, dry_run: dry })
      setReplayResult(res)
      if (!dry) { toast(t('rules.replayApplied', { n: res.applied }), { type: 'success' }); load() }
    } catch (err: any) { toast(err.message) }
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
            <div className="flex flex-wrap items-center gap-1 mt-2">
              <span className="text-[11px] text-muted-foreground">{t('rules.examples')}:</span>
              {RULE_EXAMPLES.map(ex => (
                <button key={ex.expr} type="button" onClick={() => setForm({ ...form, expression: ex.expr })}
                  className="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground hover:bg-muted">{t(ex.labelKey)}</button>
              ))}
            </div>
          </div>
          <div className="grid sm:grid-cols-[1fr_1fr_auto] gap-3 items-end">
            <div>
              <Label className="text-xs text-muted-foreground">{t('rules.action')}</Label>
              <Select className="mt-1" value={form.action} onChange={e => setForm({ ...form, action: e.target.value })}>
                <option value="trash">{t('rules.actionTrash')}</option>
                <option value="move">{t('rules.actionMove')}</option>
                <option value="forward">{t('rules.actionForward')}</option>
              </Select>
            </div>
            {form.action === 'move' && (
              <div>
                <Label className="text-xs text-muted-foreground">{t('rules.folder')}</Label>
                <Select className="mt-1" value={form.folder} onChange={e => setForm({ ...form, folder: e.target.value })}>
                  <option value="trash">{t('mail.trash')}</option>
                  <option value="inbox">{t('mail.inbox')}</option>
                  <option value="draft">{t('mail.draft')}</option>
                  {folders.map((f: any) => <option key={f.id} value={'c' + f.id}>{f.name}</option>)}
                </Select>
              </div>
            )}
            {form.action === 'forward' && (
              <div className="sm:col-span-2">
                <Label className="text-xs text-muted-foreground">{t('rules.forwardTo')}</Label>
                <Input className="mt-1 font-mono" value={form.forward_to} onChange={e => setForm({ ...form, forward_to: e.target.value })} placeholder="d@example.com, e@other.com" />
                <p className="text-xs text-muted-foreground mt-1">{t('rules.forwardHint')}</p>
              </div>
            )}
            <div className="flex flex-col gap-1 pb-2">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
                {t('rules.enabled')}
              </label>
              <label className="flex items-center gap-2 text-sm" title={t('rules.shadowHint')}>
                <input type="checkbox" checked={form.shadow} onChange={e => setForm({ ...form, shadow: e.target.checked })} />
                {t('rules.shadow')}
              </label>
            </div>
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
            <p className={`text-xs mt-2 ${testResult.error ? 'text-destructive' : testResult.matched ? 'text-success' : 'text-muted-foreground'}`}>
              {testResult.error ? testResult.error : testResult.matched ? t('rules.testHit') : t('rules.testMiss')}
            </p>
          )}
        </div>
      </Card>

      <Card className="divide-y divide-border">
        {loading && <SkeletonRows rows={3} />}
        {!loading && rules.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('rules.empty')}</p>}
        {!loading && rules.map(r => (
          <div key={r.id} className="p-3">
            <div className="flex items-start gap-3">
            <input type="checkbox" className="mt-1" checked={r.enabled} onChange={() => toggle(r)} aria-label={t('rules.enabled')} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium truncate">{r.name}</span>
                <Badge>{r.action === 'move' ? `${t('rules.actionMove')} → ${r.folder}` : r.action === 'forward' ? `${t('rules.actionForward')} → ${r.forward_to}` : t('rules.actionTrash')}</Badge>
                {r.priority ? <Badge>{t('rules.priority')} {r.priority}</Badge> : null}
                {r.shadow ? <Badge className="text-warning">{t('rules.shadowBadge')}</Badge> : null}
              </div>
              <code className="text-xs text-muted-foreground break-all">{r.expression}</code>
            </div>
            <Button variant="ghost" size="icon" onClick={() => { setReplayId(replayId === r.id ? null : r.id); setReplayResult(null) }} aria-label={t('rules.replay')}><RotateCcw /></Button>
            <Button variant="ghost" size="icon" onClick={() => startEdit(r)} aria-label={t('common.edit')}><Pencil /></Button>
            <Button variant="ghost" size="icon" onClick={() => del(r.id)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
          {replayId === r.id && (
            <div className="mt-3 rounded-md border border-border p-2 space-y-2 bg-muted/30">
              <div className="text-xs text-muted-foreground">{t('rules.replayTitle')}</div>
              <div className="flex flex-wrap items-center gap-2">
                <Select className="h-8 w-auto text-xs" value={replay.folder} onChange={e => setReplay({ ...replay, folder: e.target.value })}>
                  <option value="inbox">{t('mail.inbox')}</option>
                  <option value="sent">{t('mail.sent')}</option>
                  <option value="draft">{t('mail.draft')}</option>
                  <option value="trash">{t('mail.trash')}</option>
                  {folders.map((f: any) => <option key={f.id} value={'c' + f.id}>{f.name}</option>)}
                </Select>
                <Input type="number" className="h-8 w-24 text-xs" value={replay.limit} onChange={e => setReplay({ ...replay, limit: +e.target.value })} aria-label={t('rules.replayLimit')} />
                <Button size="sm" variant="outline" onClick={() => doReplay(r.id, true)}>{t('rules.replayPreview')}</Button>
                <Button size="sm" disabled={r.shadow} onClick={() => doReplay(r.id, false)}>{t('rules.replayApply')}</Button>
                <Button size="sm" variant="ghost" onClick={() => { setReplayId(null); setReplayResult(null) }} aria-label={t('common.cancel')}><X /></Button>
              </div>
              {replayResult && (
                <p className={`text-xs ${replayResult.applied ? 'text-success' : 'text-muted-foreground'}`}>
                  {replayResult.applied ? t('rules.replayApplied', { n: replayResult.applied }) : t('rules.replayMatched', { n: replayResult.matched })}
                  {r.shadow ? ' ' + t('rules.replayShadow') : ''}
                </p>
              )}
            </div>
          )}
        </div>
        ))}
      </Card>
    </>
  )
}
