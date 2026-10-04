import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Textarea, Card, Badge, Label } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { toast, confirmAsync } from '../lib/ui'
import { useI18n } from '../lib/i18n'
import { Filter, Plus, Trash2, Check, Save, Power } from 'lucide-react'

export default function Sieve() {
  const { t } = useI18n()
  const template = t('sieve.template')
  const [list, setList] = useState<any[]>([])
  const [name, setName] = useState('main')
  const [script, setScript] = useState(template)

  async function load() {
    try { setList(await api.sieve()) } catch {}
  }
  useEffect(() => { load() }, [])

  function edit(sc: any) { setName(sc.name); setScript(sc.script) }
  function reset() { setName('main'); setScript(template) }

  async function save() {
    try {
      await api.sieveSave({ name, script })
      toast(t('sieve.saved'), { type: 'success' })
      load()
    } catch (e: any) { toast(e.message, { type: 'error' }) }
  }
  async function check() {
    try {
      const r = await api.sieveCheck(script)
      if (r.ok) toast(t('sieve.checkOk'), { type: 'success' })
      else toast(r.error, { type: 'error' })
    } catch (e: any) { toast(e.message, { type: 'error' }) }
  }
  async function activate(sc: any) {
    try { await api.sieveActivate(sc.id); load() } catch (e: any) { toast(e.message, { type: 'error' }) }
  }
  async function del(sc: any) {
    if (!await confirmAsync(t('sieve.confirmDelete', { name: sc.name }))) return
    try { await api.sieveDelete(sc.id); load() } catch (e: any) { toast(e.message, { type: 'error' }) }
  }

  return (
    <PageShell title={t('sieve.title')} icon={Filter}>
      <p className="text-sm text-muted-foreground">{t('sieve.desc')}</p>

      <Card className="p-4 space-y-3">
        <div className="flex items-end gap-2">
          <div className="flex-1">
            <Label className="text-xs text-muted-foreground">{t('sieve.name')}</Label>
            <Input className="mt-1" value={name} onChange={e => setName(e.target.value)} />
          </div>
          <Button size="sm" variant="outline" onClick={reset}><Plus />{t('sieve.new')}</Button>
        </div>
        <Textarea rows={14} className="font-mono text-xs" value={script} onChange={e => setScript(e.target.value)} />
        <div className="flex items-center gap-2">
          <Button size="sm" onClick={save}><Save />{t('common.save')}</Button>
          <Button size="sm" variant="outline" onClick={check}><Check />{t('sieve.check')}</Button>
          <span className="text-xs text-muted-foreground">{t('sieve.hint')}</span>
        </div>
      </Card>

      <Card className="divide-y divide-border">
        {list.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('sieve.empty')}</p>}
        {list.map(sc => (
          <div key={sc.id} className="p-3 flex items-center gap-2">
            <button className="text-sm font-medium hover:underline truncate" onClick={() => edit(sc)}>{sc.name}</button>
            {sc.active && <Badge className="text-green-600">{t('sieve.active')}</Badge>}
            <div className="flex-1" />
            {!sc.active && <Button variant="ghost" size="sm" onClick={() => activate(sc)}><Power />{t('sieve.activate')}</Button>}
            <Button variant="ghost" size="icon" onClick={() => del(sc)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </PageShell>
  )
}
