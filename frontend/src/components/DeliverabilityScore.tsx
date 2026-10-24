import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { Card, Button } from '../components/ui/controls'
import { useI18n } from '../lib/i18n'
import { toast } from '../lib/ui'
import { CheckCircle2, Circle, ShieldCheck, Loader2, ExternalLink, Gauge } from 'lucide-react'

// 发信健康分（#49）：把 SPF/DKIM/DMARC/MX/TLS/外发 6 项汇总成一眼可懂的分数。
export function DeliverabilityScore({ domain }: { domain?: string }) {
  const { t } = useI18n()
  const [s, setS] = useState<any>(null)
  const [tls, setTls] = useState<any>(null)
  const [check, setCheck] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [publishing, setPublishing] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try { setS(await api.settingsGet()) } catch {}
    try { setTls(await api.tlsGet()) } catch {}
    if (domain) { try { setCheck(await api.adminDomainCheck(domain)) } catch {} }
    setLoading(false)
  }, [domain])
  useEffect(() => { load() }, [load])

  const outbound = !!s?.relay_host || !!s?.direct_send
  const items = [
    { ok: !!check?.mx_ok, label: t('trust.mx'), to: '/admin/providers' },
    { ok: !!check?.spf, label: t('trust.spf'), to: '/admin/providers' },
    { ok: !!check?.dkim, label: t('trust.dkim'), to: '/admin/settings' },
    { ok: !!check?.dmarc, label: t('trust.dmarc'), to: '/admin/providers' },
    { ok: !!tls?.cert?.exists, label: t('trust.tls'), to: '/admin/ssl' },
    { ok: outbound, label: t('trust.relay'), to: '/admin/settings' },
  ]
  const passed = items.filter(i => i.ok).length
  const score = Math.round((passed / items.length) * 100)
  const tone = score >= 80 ? 'text-success' : score >= 50 ? 'text-warning' : 'text-destructive'
  const ring = score >= 80 ? 'stroke-success' : score >= 50 ? 'stroke-warning' : 'stroke-destructive'

  async function publishMta() {
    setPublishing(true)
    try {
      const ds: any[] = await api.dnsList()
      const dm = ds.find(d => d.name === domain)
      if (!dm) throw new Error('domain not hosted locally')
      const r: any = await api.publishDeliverability(dm.id)
      toast(t('trust.published', { n: r.created ?? 0 }))
    } catch (e: any) {
      toast(e.message)
    } finally { setPublishing(false) }
  }

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-2">
        <b className="text-sm flex items-center gap-1.5"><Gauge size={15} />{t('trust.title')}</b>
        {loading && <Loader2 size={14} className="animate-spin text-muted-foreground" />}
      </div>
      <div className="flex items-center gap-4 mt-3">
        <div className="relative shrink-0 size-20">
          <svg viewBox="0 0 36 36" className="size-20 -rotate-90">
            <circle cx="18" cy="18" r="15.9" fill="none" className="stroke-muted" strokeWidth="3" />
            <circle cx="18" cy="18" r="15.9" fill="none" className={ring} strokeWidth="3" strokeLinecap="round"
              strokeDasharray={`${score} ${100 - score}`} pathLength={100} />
          </svg>
          <div className="absolute inset-0 grid place-items-center">
            <span className={`text-xl font-bold ${tone}`}>{score}</span>
          </div>
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-xs text-muted-foreground">{t('trust.desc')}</p>
          <p className="text-xs mt-1">{t('trust.score')}: <span className={`font-semibold ${tone}`}>{score}/100</span> · {passed}/{items.length}</p>
          <div className="mt-2 flex flex-wrap gap-1">
            {items.map((it, i) => (
              <span key={i} className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] ${it.ok ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive'}`}>
                {it.ok ? <CheckCircle2 size={11} /> : <Circle size={11} />}{it.label}
              </span>
            ))}
          </div>
        </div>
      </div>
      {items.some(i => !i.ok) && (
        <div className="mt-3 space-y-1">
          {items.filter(i => !i.ok).map((it, i) => (
            <div key={i} className="flex items-center gap-2 text-xs">
              <Circle size={12} className="text-destructive shrink-0" />
              <span className="flex-1">{it.label}</span>
              <Link to={it.to} className="text-primary underline flex items-center gap-0.5">{t('trust.fix')}<ExternalLink size={10} /></Link>
            </div>
          ))}
        </div>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        <Button size="sm" variant="outline" onClick={load} disabled={loading}>{t('common.refresh')}</Button>
        {domain && (
          <Button size="sm" variant="outline" onClick={publishMta} disabled={publishing}>
            {publishing ? <Loader2 className="animate-spin" /> : <ShieldCheck />}{t('trust.publish')}
          </Button>
        )}
      </div>
    </Card>
  )
}
