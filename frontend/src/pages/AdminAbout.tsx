import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import { Button, Card, Badge, Input, Label } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { renderMarkdown } from '../lib/markdown'
import { BRAND } from '../lib/brand'
import { RefreshCw, DownloadCloud, Loader2, CheckCircle2, AlertCircle, ScrollText } from 'lucide-react'

export default function AdminAbout() {
  const { t } = useI18n()
  const [about, setAbout] = useState<any>(null)
  const [settings, setSettings] = useState<any>(null)
  const [changelog, setChangelog] = useState('')
  const [check, setCheck] = useState<any>(null)
  const [checking, setChecking] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [msg, setMsg] = useState('')
  const [intervalMin, setIntervalMin] = useState(10)
  const pollRef = useRef<any>(null)

  async function load() {
    try { setAbout(await api.adminAbout()) } catch {}
    try {
      const s = await api.settingsGet()
      setSettings(s)
      setIntervalMin(s.update_interval || 10)
    } catch {}
    try { const c = await api.changelog(); setChangelog(c.markdown || '') } catch {}
  }
  useEffect(() => { load(); return () => clearInterval(pollRef.current) }, [])

  async function doCheck() {
    setChecking(true); setMsg('')
    try { setCheck(await api.adminUpdateCheck()) } catch (e: any) { setMsg(e.message) }
    finally { setChecking(false) }
  }

  async function doUpdate() {
    if (!confirm(t('about.confirmUpdate'))) return
    setUpdating(true); setMsg('')
    try {
      const r = await api.adminUpdate()
      setMsg(r.message || t('about.updating'))
      pollVersion()
    } catch (e: any) { setUpdating(false); setMsg(e.message) }
  }

  // 更新后服务会重启：轮询 /api/version，恢复且版本变化即提示完成
  function pollVersion() {
    const before = about?.version
    let tries = 0
    pollRef.current = setInterval(async () => {
      tries++
      try {
        const v = await api.version()
        if (v?.version && v.version !== before) {
          clearInterval(pollRef.current)
          setUpdating(false)
          setCheck(null)
          setMsg(t('about.updated', { v: v.version }))
          setAbout((a: any) => ({ ...a, ...v }))
        }
      } catch { /* 重启中，继续等待 */ }
      if (tries > 40) { clearInterval(pollRef.current); setUpdating(false) }
    }, 3000)
  }

  async function toggleAuto(v: boolean) {
    try {
      setSettings(await api.settingsPatch({ auto_update: v ? '1' : '0' }))
    } catch (e: any) { alert(e.message) }
  }
  async function saveInterval() {
    try {
      const s = await api.settingsPatch({ update_interval: String(intervalMin) })
      setSettings(s); setIntervalMin(s.update_interval || 10)
    } catch (e: any) { alert(e.message) }
  }

  const available = !!check?.update_available
  const autoOn = !!settings?.auto_update

  return (
    <AdminShell title={t('about.title')} desc={t('about.desc')}>
      <Card className="p-4 space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-2xl">🌽</span>
          <b className="text-lg">{BRAND}</b>
          <Badge className="font-mono">v{String(about?.version || '').replace(/^v/, '') || '—'}</Badge>
          {available && <Badge className="text-green-600">{t('about.available', { v: check.latest })}</Badge>}
        </div>
        <div className="text-xs text-muted-foreground font-mono space-y-0.5">
          <div>commit: {about?.commit || '—'}</div>
          <div>build: {about?.date || '—'}</div>
          <div>repo: {about?.repo || '—'}</div>
        </div>

        <div className="flex flex-wrap items-center gap-2 pt-1">
          <Button variant="outline" size="sm" disabled={checking || updating} onClick={doCheck}>
            {checking ? <Loader2 className="animate-spin" /> : <RefreshCw />}{t('about.check')}
          </Button>
          <Button size="sm" disabled={updating} onClick={doUpdate}>
            {updating ? <Loader2 className="animate-spin" /> : <DownloadCloud />}{t('about.update')}
          </Button>
          {check && !check.error && !available && (
            <span className="text-xs text-green-600 flex items-center gap-1"><CheckCircle2 size={14} />{t('about.latest')}</span>
          )}
        </div>
        {(msg || check?.error) && (
          <p className={`text-xs flex items-start gap-1 ${check?.error ? 'text-red-500' : 'text-muted-foreground'}`}>
            {check?.error ? <AlertCircle size={14} className="mt-0.5 shrink-0" /> : null}
            <span className="break-all">{check?.error || msg}</span>
          </p>
        )}
      </Card>

      <Card className="p-4 space-y-3">
        <b className="text-sm">{t('about.autoTitle')}</b>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={autoOn} onChange={e => toggleAuto(e.target.checked)} />
          <span>{t('about.autoEnable')}</span>
        </label>
        <p className="text-xs text-muted-foreground">{t('about.autoHint')}</p>
        <div className="flex items-end gap-2">
          <div>
            <Label className="text-xs text-muted-foreground">{t('about.interval')}</Label>
            <Input type="number" min={1} max={1440} className="w-28 mt-1" value={intervalMin}
              onChange={e => setIntervalMin(+e.target.value)} />
          </div>
          <Button variant="outline" size="sm" onClick={saveInterval}>{t('common.save')}</Button>
        </div>
      </Card>

      <Card className="p-4">
        <div className="flex items-center gap-2 mb-2">
          <ScrollText size={16} />
          <b className="text-sm">{t('about.changelog')}</b>
        </div>
        {changelog
          ? <div className="text-sm max-h-[60vh] overflow-auto pr-1" dangerouslySetInnerHTML={{ __html: renderMarkdown(changelog) }} />
          : <p className="text-sm text-muted-foreground">{t('about.noChangelog')}</p>}
      </Card>
    </AdminShell>
  )
}
