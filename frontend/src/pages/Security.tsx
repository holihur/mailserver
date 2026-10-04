import { useEffect, useState } from 'react'
import { toast, confirmAsync, promptAsync } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Label } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { useI18n } from '../lib/i18n'
import { ShieldCheck, Copy, KeyRound, Check, Download, Trash2 } from 'lucide-react'

export default function Security() {
  const { t } = useI18n()
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [setup, setSetup] = useState<any>(null) // {secret, url}
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)

  async function load() {
    try { const s = await api.totp(); setEnabled(!!s.enabled) } catch { setEnabled(false) }
  }
  useEffect(() => { load() }, [])

  async function beginSetup() {
    setBusy(true)
    try { setSetup(await api.totpSetup()) } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  async function enable() {
    setBusy(true)
    try {
      await api.totpEnable(code)
      setSetup(null); setCode(''); await load()
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  async function disable() {
    setBusy(true)
    try {
      await api.totpDisable(code)
      setCode(''); await load()
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  function copy(s: string) { navigator.clipboard?.writeText(s) }

  async function exportData() {
    try {
      const data = await api.gdprExport()
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `sweetcorn-export-${Date.now()}.json`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e: any) { toast(e.message) }
  }
  async function deleteAccount() {
    if (!await confirmAsync(t('security.deleteConfirm'))) return
    const pw = await promptAsync(t('security.passwordPrompt'), { password: true })
    if (!pw) return
    try {
      await api.gdprDelete(pw)
      localStorage.removeItem('token')
      location.assign('/login')
    } catch (e: any) { toast(e.message) }
  }

  return (
    <PageShell title={t('security.title')} icon={ShieldCheck} maxWidth="max-w-2xl">
        <Card className="p-4 space-y-3">
          <div className="flex items-center gap-2">
            <KeyRound size={16} />
            <b className="text-sm">{t('security.totp')}</b>
            {enabled === null ? null : enabled
              ? <Badge className="text-green-600">{t('common.enabled')}</Badge>
              : <Badge className="text-yellow-600">{t('common.disabled')}</Badge>}
          </div>
          <p className="text-sm text-muted-foreground">{t('security.totpHint')}</p>

          {!enabled && !setup && (
            <Button size="sm" disabled={busy} onClick={beginSetup}><ShieldCheck />{t('security.enable')}</Button>
          )}

          {!enabled && setup && (
            <div className="space-y-3">
              <p className="text-sm">{t('security.scanHint')}</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 break-all text-xs font-mono bg-muted/60 rounded px-2 py-1">{setup.secret}</code>
                <Button size="sm" variant="outline" type="button" onClick={() => copy(setup.secret)}><Copy />{t('common.copy')}</Button>
              </div>
              <p className="text-xs text-muted-foreground break-all">{setup.url}</p>
              <div className="flex items-end gap-2">
                <div>
                  <Label className="text-xs text-muted-foreground">{t('security.code')}</Label>
                  <Input className="mt-1 w-32 tracking-widest" inputMode="numeric" maxLength={6} value={code} onChange={e => setCode(e.target.value)} placeholder="000000" />
                </div>
                <Button size="sm" disabled={busy || code.length !== 6} onClick={enable}><Check />{t('security.confirmEnable')}</Button>
              </div>
            </div>
          )}

          {enabled && (
            <div className="flex items-end gap-2">
              <div>
                <Label className="text-xs text-muted-foreground">{t('security.code')}</Label>
                <Input className="mt-1 w-32 tracking-widest" inputMode="numeric" maxLength={6} value={code} onChange={e => setCode(e.target.value)} placeholder="000000" />
              </div>
              <Button size="sm" variant="destructive" disabled={busy || code.length !== 6} onClick={disable}>{t('security.disable')}</Button>
            </div>
          )}
        </Card>

        <Card className="p-4 space-y-3">
          <b className="text-sm">{t('security.dataTitle')}</b>
          <p className="text-sm text-muted-foreground">{t('security.exportHint')}</p>
          <Button size="sm" variant="outline" onClick={exportData}><Download />{t('security.export')}</Button>
          <div className="border-t border-border pt-3 space-y-2">
            <p className="text-sm text-muted-foreground">{t('security.deleteHint')}</p>
            <Button size="sm" variant="destructive" onClick={deleteAccount}><Trash2 />{t('security.deleteAccount')}</Button>
          </div>
        </Card>
    </PageShell>
  )
}
