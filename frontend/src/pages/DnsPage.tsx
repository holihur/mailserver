import { useEffect, useState } from 'react'
import { toast, confirmAsync } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Select } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { SkeletonList } from '../components/Skeleton'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, Globe, Copy, CheckCircle2 } from 'lucide-react'

const TYPES = ['A', 'AAAA', 'MX', 'TXT', 'CNAME', 'NS', 'SRV', 'CAA', 'TLSA']

export default function DnsPage() {
  const { t } = useI18n()
  const [allowed, setAllowed] = useState<boolean | null>(null)
  const [domains, setDomains] = useState([])
  const [loading, setLoading] = useState(true)
  const [sel, setSel] = useState(null)
  const [newDomain, setNewDomain] = useState({ name: '', ip: '' })
  const [newRec, setNewRec] = useState({ name: '', type: 'A', value: '', ttl: 600, prio: 10 })
  const [zone, setZone] = useState('')
  const [hints, setHints] = useState<any[]>([])
  const [dnssec, setDnssec] = useState<any>(null)
  useEffect(() => {
    api.deliverability().then(setHints).catch(() => {})
    api.dnssec().then(setDnssec).catch(() => {})
  }, [])
  async function publishHints() {
    if (!sel) return
    try {
      const r: any = await api.publishDeliverability(sel.domain.id)
      toast(t('dns.publishHintsOk', { n: r.created }))
      await open(sel.domain.id)
    } catch (e: any) { toast(e.message) }
  }

  async function loadDomains() {
    try { setDomains(await api.dnsList()) } catch {} finally { setLoading(false) }
  }
  // 自托管 DNS 仅管理员可用，非管理员直接回到邮箱
  useEffect(() => {
    api.me()
      .then(u => { if (u.admin) setAllowed(true); else location.replace('/') })
      .catch(() => { location.href = '/login' })
  }, [])
  useEffect(() => { if (allowed) loadDomains() }, [allowed])

  async function open(id) {
    const d = await api.dnsGet(id)
    setSel(d)
    const z = await api.dnsZone(id)
    setZone(z)
  }

  async function createDomain(e) {
    e.preventDefault()
    const d = await api.dnsCreate(newDomain)
    setNewDomain({ name: '', ip: '' })
    await loadDomains()
    open(d.id)
  }

  async function createRec(e) {
    e.preventDefault()
    await api.dnsRecCreate(sel.domain.id, newRec)
    setNewRec({ name: '', type: 'A', value: '', ttl: 600, prio: 10 })
    open(sel.domain.id)
  }

  if (!allowed) return null

  return (
    <PageShell title={t('dns.title')} icon={Globe} maxWidth="max-w-6xl">
      <div className="grid md:grid-cols-[260px_1fr] gap-4">
        <Card className="p-4 space-y-3 h-fit">
          <b className="text-sm">{t('dns.myDomains')}</b>
          <form onSubmit={createDomain} className="space-y-2">
            <Input placeholder="example.com" value={newDomain.name} onChange={e => setNewDomain({ ...newDomain, name: e.target.value })} required />
            <Input placeholder={t('dns.ipPlaceholder')} value={newDomain.ip} onChange={e => setNewDomain({ ...newDomain, ip: e.target.value })} required />
            <Button className="w-full" size="sm"><Plus />{t('dns.addRecords')}</Button>
          </form>
          <div className="space-y-1">
            {loading && <SkeletonList rows={3} />}
            {!loading && domains.map(d => (
              <div key={d.id} className="flex items-center gap-1">
                <button onClick={() => open(d.id)} className={'flex-1 text-left text-sm rounded-md px-3 py-2 hover:bg-muted ' + (sel?.domain.id === d.id ? 'bg-muted font-semibold' : '')}>{d.name}</button>
                <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={async () => { if (await confirmAsync(t('dns.deleteConfirm', { name: d.name }))) { await api.dnsDelete(d.id); setSel(null); loadDomains() } }}><Trash2 /></Button>
              </div>
            ))}
            {!loading && domains.length === 0 && <p className="text-xs text-muted-foreground">{t('dns.noDomains')}</p>}
          </div>
        </Card>

        <div className="space-y-4 min-w-0">
          {!sel ? (
            <Card className="p-6 text-sm text-muted-foreground space-y-2">
              <p className="font-semibold text-foreground">{t('dns.explain')}</p>
              <ol className="list-decimal ml-5 space-y-1">
                <li>{t('dns.step1')}</li>
                <li>{t('dns.step2')}</li>
                <li>{t('dns.step3')}</li>
                <li>{t('dns.step4')}</li>
              </ol>
              <p>{t('dns.docs')}</p>
            </Card>
          ) : (
            <>
              <Card className="p-4">
                <div className="flex items-center gap-2 mb-3">
                  <b>{sel.domain.name}</b>
                  <Badge>{t('dns.records', { n: sel.records.length })}</Badge>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm min-w-[480px]">
                    <thead><tr className="text-left text-muted-foreground text-xs">
                      <th className="py-1">{t('dns.host')}</th><th>{t('dns.type')}</th><th>{t('dns.value')}</th><th>{t('dns.ttl')}</th><th></th>
                    </tr></thead>
                    <tbody>
                      {sel.records.map(r => (
                        <tr key={r.id} className="border-t border-border">
                          <td className="py-1.5 pr-2 font-mono">{r.name}</td>
                          <td className="pr-2"><Badge>{r.type}</Badge></td>
                          <td className="pr-2 font-mono text-xs break-all max-w-[280px]">{r.prio && r.type === 'MX' ? r.prio + ' ' : ''}{r.value}</td>
                          <td className="pr-2 text-muted-foreground">{r.ttl}</td>
                          <td><Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={async () => { await api.dnsRecDelete(sel.domain.id, r.id); open(sel.domain.id) }}><Trash2 /></Button></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <form onSubmit={createRec} className="grid grid-cols-2 sm:grid-cols-6 gap-2 mt-3">
                  <Input placeholder={t('dns.hostPlaceholder')} value={newRec.name} onChange={e => setNewRec({ ...newRec, name: e.target.value })} required />
                  <Select value={newRec.type} onChange={e => setNewRec({ ...newRec, type: e.target.value })}>
                    {TYPES.map(x => <option key={x}>{x}</option>)}
                  </Select>
                  <Input className="col-span-2" placeholder={t('dns.valuePlaceholder')} value={newRec.value} onChange={e => setNewRec({ ...newRec, value: e.target.value })} required />
                  <Input type="number" placeholder={t('dns.ttl')} value={newRec.ttl} onChange={e => setNewRec({ ...newRec, ttl: +e.target.value })} />
                  <Button size="sm"><Plus />{t('dns.add')}</Button>
                </form>
                {newRec.type === 'MX' && <p className="text-xs text-muted-foreground mt-1">{t('dns.mxHint')}</p>}
              </Card>

              <Card className="p-4">
                <div className="flex items-center gap-2 mb-2">
                  <b className="text-sm">{t('dns.zonePreview')}</b>
                  <Button variant="outline" size="sm" onClick={() => { navigator.clipboard.writeText(zone) }}><Copy />{t('common.copy')}</Button>
                  <span className="text-xs text-success flex items-center gap-1"><CheckCircle2 size={12} />{t('dns.synced')}</span>
                </div>
                <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto whitespace-pre">{zone}</pre>
              </Card>

              <Card className="p-4 text-sm space-y-1">
                <b className="text-sm">{t('dns.checklist')}</b>
                <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto">{`dig @你的服务器IP ${sel.domain.name} NS\n dig @你的服务器IP ${sel.domain.name} MX\n dig @你的服务器IP mail.${sel.domain.name} A\n dig ${sel.domain.name} MX`}</pre>
              </Card>

              <Card className="p-4 text-sm space-y-2">
                <div className="flex items-center gap-2">
                  <b className="text-sm">{t('dns.deliverability')}</b>
                  <div className="flex-1" />
                  <Button size="sm" variant="outline" onClick={publishHints}><Plus />{t('dns.publishHints')}</Button>
                </div>
                <p className="text-xs text-muted-foreground">{t('dns.deliverabilityHint')}</p>
                {hints.length === 0 && <p className="text-xs text-muted-foreground">—</p>}
                {hints.map((h, i) => (
                  <div key={i} className="flex items-center gap-2 text-xs">
                    <Badge>{h.type}</Badge>
                    <span className="font-mono truncate">{h.name}</span>
                    <span className="text-muted-foreground truncate flex-1">{h.value}</span>
                    <Button variant="ghost" size="icon" onClick={() => navigator.clipboard.writeText(h.value)} aria-label={t('common.copy')}><Copy size={14} /></Button>
                  </div>
                ))}
              </Card>

              <Card className="p-4 text-sm space-y-2">
                <b className="text-sm">{t('dns.dnssec')}</b>
                {!dnssec?.enabled
                  ? <p className="text-xs text-muted-foreground">{t('dns.dnssecDisabled')}</p>
                  : (
                    <>
                      <p className="text-xs text-muted-foreground">{t('dns.dnssecSteps')}</p>
                      {(dnssec.domains || []).length === 0 && <p className="text-xs text-muted-foreground">—</p>}
                      {(dnssec.domains || []).map((x: any) => (
                        <div key={x.domain} className="flex items-center gap-2 text-xs">
                          <span className="font-mono">{x.domain}</span>
                          <span className="font-mono truncate flex-1">{x.ds || '—'}</span>
                          {x.ds && <Button variant="ghost" size="icon" onClick={() => navigator.clipboard.writeText(x.ds)} aria-label={t('dns.copyDs')}><Copy size={14} /></Button>}
                        </div>
                      ))}
                    </>
                  )}
              </Card>
            </>
          )}
        </div>
      </div>
    </PageShell>
  )
}
