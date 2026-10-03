import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import { Plus, Trash2, Globe, Copy, CheckCircle2 } from 'lucide-react'

const TYPES = ['A', 'AAAA', 'MX', 'TXT', 'CNAME', 'NS', 'SRV', 'CAA']

export default function DnsPage() {
  const [domains, setDomains] = useState([])
  const [sel, setSel] = useState(null) // {domain, records}
  const [newDomain, setNewDomain] = useState({ name: '', ip: '' })
  const [newRec, setNewRec] = useState({ name: '', type: 'A', value: '', ttl: 600, prio: 10 })
  const [zone, setZone] = useState('')

  async function loadDomains() {
    try { setDomains(await api.dnsList()) } catch {}
  }
  useEffect(() => { loadDomains() }, [])

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

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border px-4 h-14 flex items-center gap-3">
        <a href="#/" className="font-semibold">← Mailserver</a>
        <Badge><Globe size={12} /> 自托管 DNS</Badge>
        <div className="flex-1" />
        <span className="text-xs text-muted-foreground hidden sm:block">权威应答本机 :53，非托管域拒绝递归</span>
      </header>

      <div className="max-w-6xl mx-auto p-4 grid md:grid-cols-[260px_1fr] gap-4">
        <Card className="p-4 space-y-3 h-fit">
          <b className="text-sm">我的域名</b>
          <form onSubmit={createDomain} className="space-y-2">
            <Input placeholder="example.com" value={newDomain.name} onChange={e => setNewDomain({ ...newDomain, name: e.target.value })} required />
            <Input placeholder="服务器公网 IP" value={newDomain.ip} onChange={e => setNewDomain({ ...newDomain, ip: e.target.value })} required />
            <Button className="w-full" size="sm"><Plus />添加并自动配 mail 记录</Button>
          </form>
          <div className="space-y-1">
            {domains.map(d => (
              <div key={d.id} className="flex items-center gap-1">
                <button onClick={() => open(d.id)} className={'flex-1 text-left text-sm rounded-md px-3 py-2 hover:bg-muted ' + (sel?.domain.id === d.id ? 'bg-muted font-semibold' : '')}>{d.name}</button>
                <Button variant="ghost" size="icon" onClick={async () => { if (confirm('删除 ' + d.name + '？')) { await api.dnsDelete(d.id); setSel(null); loadDomains() } }}><Trash2 /></Button>
              </div>
            ))}
            {domains.length === 0 && <p className="text-xs text-muted-foreground">还没有域名，先添加一个。</p>}
          </div>
        </Card>

        <div className="space-y-4 min-w-0">
          {!sel ? (
            <Card className="p-6 text-sm text-muted-foreground space-y-2">
              <p className="font-semibold text-foreground">把域名指向你自己的 DNS（不用托管商）：</p>
              <ol className="list-decimal ml-5 space-y-1">
                <li>云服务器放行 <code>53/udp + 53/tcp</code>（以及 mail 的 25/587/993）。</li>
                <li>注册商处把域名的 NS 改为 <code>ns1.你的域名</code>，并加 Glue 记录指向你的公网 IP。</li>
                <li>本页添加域名 → 自动生成 NS / A / MX / SPF / DMARC / DKIM 占位。</li>
                <li><code>dig @你的IP example.com MX</code> 验证，再等 DNS 生效。</li>
              </ol>
              <p>详细步骤见 <code>mailserver/DNS_SETUP.md</code>。</p>
            </Card>
          ) : (
            <>
              <Card className="p-4">
                <div className="flex items-center gap-2 mb-3">
                  <b>{sel.domain.name}</b>
                  <Badge>{sel.records.length} 条记录</Badge>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead><tr className="text-left text-muted-foreground text-xs"><th className="py-1">主机</th><th>类型</th><th>值</th><th>TTL</th><th></th></tr></thead>
                    <tbody>
                      {sel.records.map(r => (
                        <tr key={r.id} className="border-t border-border">
                          <td className="py-1.5 pr-2 font-mono">{r.name}</td>
                          <td className="pr-2"><Badge>{r.type}</Badge></td>
                          <td className="pr-2 font-mono text-xs break-all max-w-[280px]">{r.prio && r.type === 'MX' ? r.prio + ' ' : ''}{r.value}</td>
                          <td className="pr-2 text-muted-foreground">{r.ttl}</td>
                          <td><Button variant="ghost" size="icon" onClick={async () => { await api.dnsRecDelete(sel.domain.id, r.id); open(sel.domain.id) }}><Trash2 /></Button></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <form onSubmit={createRec} className="grid grid-cols-2 sm:grid-cols-6 gap-2 mt-3">
                  <Input placeholder="@/mail/…" value={newRec.name} onChange={e => setNewRec({ ...newRec, name: e.target.value })} required />
                  <select className="h-9 rounded-md border border-border bg-background text-sm" value={newRec.type} onChange={e => setNewRec({ ...newRec, type: e.target.value })}>
                    {TYPES.map(t => <option key={t}>{t}</option>)}
                  </select>
                  <Input className="col-span-2" placeholder="值" value={newRec.value} onChange={e => setNewRec({ ...newRec, value: e.target.value })} required />
                  <Input type="number" placeholder="TTL" value={newRec.ttl} onChange={e => setNewRec({ ...newRec, ttl: +e.target.value })} />
                  <Button size="sm"><Plus />添加</Button>
                </form>
                {newRec.type === 'MX' && <p className="text-xs text-muted-foreground mt-1">MX 优先级填 prio（默认 10），目标写 <code>mail.你的域名.</code>（末尾加点）。</p>}
              </Card>

              <Card className="p-4">
                <div className="flex items-center gap-2 mb-2">
                  <b className="text-sm">Zone 预览（BIND 风格）</b>
                  <Button variant="outline" size="sm" onClick={() => { navigator.clipboard.writeText(zone); }}><Copy />复制</Button>
                  <span className="text-xs text-green-600 flex items-center gap-1"><CheckCircle2 size={12} />已实时同步到自研 DNS</span>
                </div>
                <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto whitespace-pre">{zone}</pre>
              </Card>

              <Card className="p-4 text-sm space-y-1">
                <b className="text-sm">上线检查（把 example.com 换成你的）</b>
                <pre className="text-xs font-mono bg-muted/60 rounded-md p-3 overflow-x-auto">{`dig @你的服务器IP ${sel.domain.name} NS\n dig @你的服务器IP ${sel.domain.name} MX\n dig @你的服务器IP mail.${sel.domain.name} A\n # 注册商 NS 生效后（去掉 @）：\n dig ${sel.domain.name} MX`}</pre>
              </Card>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
