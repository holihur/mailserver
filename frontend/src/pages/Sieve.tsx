import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Textarea, Card, Badge, Label } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { EmptyState } from '../components/EmptyState'
import { toast, confirmDestructive } from '../lib/ui'
import { useI18n } from '../lib/i18n'
import { Filter, Plus, Trash2, Check, Save, Power } from 'lucide-react'

export default function Sieve() {
  const { t } = useI18n()
  const template = t('sieve.template')
  const examples = [
    { labelKey: 'sieve.exWork', script: 'require ["fileinto"];\n\nif address :is "From" "boss@example.com" {\n  fileinto "Work";\n}\n' },
    { labelKey: 'sieve.exUrgent', script: 'require ["redirect"];\n\nif header :contains "Subject" "紧急" {\n  redirect "oncall@example.com";\n}\n' },
    { labelKey: 'sieve.exBig', script: 'require [];\n\nif size :over 5242880 {\n  discard;\n}\n' },
  ]
  const [list, setList] = useState<any[]>([])
  const [name, setName] = useState('main')
  const [script, setScript] = useState(template)
  const [vac, setVac] = useState('')

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
    if (!await confirmDestructive(t('sieve.confirmDelete', { name: sc.name }))) return
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
        <div className="flex flex-wrap items-center gap-1">
          <span className="text-[11px] text-muted-foreground">{t('sieve.examples')}:</span>
          {examples.map(ex => (
            <button key={ex.labelKey} type="button" onClick={() => setScript(ex.script)}
              className="rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground hover:bg-muted">{t(ex.labelKey)}</button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <Input className="h-8 text-xs" placeholder={t('sieve.vacationPlaceholder')} value={vac} onChange={e => setVac(e.target.value)} />
          <Button size="sm" variant="outline" type="button" disabled={!vac.trim()}
            onClick={() => setScript(`require ["vacation"];\n\nvacation :days 7 "${vac.replace(/"/g, '')}";\n`)}>
            {t('sieve.vacation')}
          </Button>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" onClick={save}><Save />{t('common.save')}</Button>
          <Button size="sm" variant="outline" onClick={check}><Check />{t('sieve.check')}</Button>
          <span className="text-xs text-muted-foreground">{t('sieve.hint')}</span>
        </div>
      </Card>

      <Card className="divide-y divide-border">
        {list.length === 0 && <EmptyState icon={Filter} title={t('sieve.empty')} desc={t('sieve.emptyDesc')} />}
        {list.map(sc => (
          <div key={sc.id} className="p-3 flex items-center gap-2">
            <button className="text-sm font-medium hover:underline truncate" onClick={() => edit(sc)}>{sc.name}</button>
            {sc.active && <Badge className="text-success">{t('sieve.active')}</Badge>}
            <div className="flex-1" />
            {!sc.active && <Button variant="ghost" size="sm" onClick={() => activate(sc)}><Power />{t('sieve.activate')}</Button>}
            <Button variant="ghost" size="icon" onClick={() => del(sc)} aria-label={t('common.delete')}><Trash2 /></Button>
          </div>
        ))}
      </Card>
    </PageShell>
  )
}
