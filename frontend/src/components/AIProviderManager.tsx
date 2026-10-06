import { useCallback, useEffect, useState } from 'react'
import { toast, confirmDestructive } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Select, Card, Badge, Label } from './ui/controls'
import { SkeletonRows } from './Skeleton'
import { useI18n } from '../lib/i18n'
import { Trash2, Pencil, Check, X, Plug, KeyRound, Sparkles } from 'lucide-react'

const EMPTY = { name: '', provider: 'openai', base_url: '', model: '', api_key: '', enabled: true }

// 用户级 / 系统级 BYOK 配置管理共用组件。scope='system' 时走管理员接口。
export default function AIProviderManager({ scope = 'user' }: { scope?: 'user' | 'system' }) {
  const { t } = useI18n()
  const admin = scope === 'system'
  const [catalog, setCatalog] = useState<any[]>([])
  const [status, setStatus] = useState<any>(null)
  const [list, setList] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState<any>({ ...EMPTY })
  const [editId, setEditId] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const [cat, items] = await Promise.all([
        api.aiCatalog(),
        admin ? api.adminAIProviders() : api.aiProviders(),
      ])
      setCatalog(cat?.providers || [])
      setStatus(cat?.status || null)
      setList(items || [])
    } catch (e: any) {
      toast(e.message)
    } finally {
      setLoading(false)
    }
  }, [admin])
  useEffect(() => { load() }, [load])

  function reset() { setForm({ ...EMPTY }); setEditId(null) }

  function changeProvider(id: string) {
    const cat = catalog.find(c => c.id === id)
    setForm((f: any) => ({
      ...f,
      provider: id,
      base_url: cat?.base_url || '',
      model: cat?.models?.[0] || '',
    }))
  }

  async function save(e: any) {
    e.preventDefault()
    const cat = catalog.find(c => c.id === form.provider)
    if (!admin && !form.api_key && !editId && cat && !cat.local) { toast(t('ai.needKey')); return }
    setBusy(true)
    try {
      if (editId) await (admin ? api.adminAIProviderPatch(editId, form) : api.aiProviderPatch(editId, form))
      else await (admin ? api.adminAIProviderCreate(form) : api.aiProviderCreate(form))
      reset()
      await load()
    } catch (err: any) { toast(err.message) } finally { setBusy(false) }
  }

  function startEdit(p: any) {
    setEditId(p.id)
    setForm({
      name: p.name || '', provider: p.provider, base_url: p.base_url || '',
      model: p.model || '', api_key: '', enabled: !!p.enabled,
    })
  }

  async function del(id: number) {
    if (!await confirmDestructive(t('ai.confirmDelete'))) return
    try {
      await (admin ? api.adminAIProviderDelete(id) : api.aiProviderDelete(id))
      if (editId === id) reset()
      await load()
    } catch (err: any) { toast(err.message) }
  }

  async function test(id: number) {
    setBusy(true)
    try {
      await (admin ? api.adminAIProviderTest(id) : api.aiProviderTest(id))
      toast(t('ai.testOk'), { type: 'success' })
    } catch (err: any) { toast(err.message) } finally { setBusy(false) }
  }

  const models: string[] = catalog.find(c => c.id === form.provider)?.models || []

  return (
    <>
      {!admin && status && (
        <Card className="p-3 flex items-start gap-2 text-sm">
          <Sparkles size={16} className={status.enabled ? 'text-success mt-0.5 shrink-0' : 'text-muted-foreground mt-0.5 shrink-0'} />
          <div>
            <b>{t('ai.statusTitle')}：</b>
            {status.has_user_key ? t('ai.statusOwn')
              : status.has_system_key ? t('ai.statusSystem')
                : <span className="text-warning">{t('ai.statusNone')}</span>}
          </div>
        </Card>
      )}
      {admin && <p className="text-xs text-muted-foreground">{t('ai.adminHint')}</p>}

      <Card className="p-4">
        <b className="text-sm flex items-center gap-1.5"><KeyRound size={15} />{editId ? t('ai.edit') : t('ai.add')}</b>
        <form onSubmit={save} className="space-y-3 mt-3">
          <div className="grid sm:grid-cols-2 gap-3">
            <div>
              <Label className="text-xs text-muted-foreground">{t('ai.provider')}</Label>
              <Select className="mt-1" value={form.provider} onChange={e => changeProvider(e.target.value)}>
                {catalog.map(c => <option key={c.id} value={c.id}>{c.label}</option>)}
              </Select>
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('ai.name')}</Label>
              <Input className="mt-1" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} placeholder={t('ai.namePlaceholder')} />
            </div>
          </div>

          <div>
            <Label className="text-xs text-muted-foreground">{t('ai.baseUrl')}</Label>
            <Input className="mt-1" value={form.base_url} onChange={e => setForm({ ...form, base_url: e.target.value })} placeholder="https://api.openai.com/v1" />
            <p className="text-[11px] text-muted-foreground mt-1">{t('ai.baseUrlHint')}</p>
          </div>

          <div className="grid sm:grid-cols-2 gap-3">
            <div>
              <Label className="text-xs text-muted-foreground">{t('ai.model')}</Label>
              <Input className="mt-1" list="ai-models" value={form.model} onChange={e => setForm({ ...form, model: e.target.value })} placeholder={models[0] || ''} />
              <datalist id="ai-models">{models.map(m => <option key={m} value={m} />)}</datalist>
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">{t('ai.apiKey')}</Label>
              <Input className="mt-1" type="password" autoComplete="off" value={form.api_key}
                onChange={e => setForm({ ...form, api_key: e.target.value })}
                placeholder={editId ? t('ai.apiKeyKeep') : 'sk-...'} />
              <p className="text-[11px] text-muted-foreground mt-1">{t('ai.keyHint')}</p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />
              {t('ai.enabled')}
            </label>
            <div className="flex-1" />
            <Button size="sm" type="submit" disabled={busy}><Check />{editId ? t('common.save') : t('ai.add')}</Button>
            {editId && <Button size="sm" variant="outline" type="button" onClick={reset}><X />{t('common.cancel')}</Button>}
          </div>
        </form>
      </Card>

      <Card className="divide-y divide-border">
        {loading && <SkeletonRows rows={3} />}
        {!loading && list.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('ai.empty')}</p>}
        {!loading && list.map(p => (
          <div key={p.id} className="p-3 flex items-start gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium truncate">{p.name}</span>
                <Badge>{p.provider_label || p.provider}</Badge>
                {p.api_key_set ? <Badge className="text-success">{t('ai.keySet')}</Badge> : <Badge className="text-warning">{t('ai.keyMissing')}</Badge>}
                {p.enabled ? <Badge className="text-success">{t('common.enabled')}</Badge> : <Badge className="text-warning">{t('common.disabled')}</Badge>}
              </div>
              <div className="text-xs text-muted-foreground truncate">
                {p.base_url || '—'}{p.model ? ` · ${p.model}` : ''}
              </div>
            </div>
            <Button variant="ghost" size="icon" disabled={busy} onClick={() => test(p.id)} aria-label={t('ai.test')}><Plug /></Button>
            <Button variant="ghost" size="icon" onClick={() => startEdit(p)} aria-label={t('common.edit')}><Pencil /></Button>
            <Button variant="ghost" size="icon" onClick={() => del(p.id)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </>
  )
}
