import { useEffect, useState } from 'react'
import { toast, confirmAsync, confirmDestructive, promptAsync } from '../lib/ui'
import { api } from '../api/client'
import { Button, Input, Card, Badge, Label } from '../components/ui/controls'
import PageShell from '../components/PageShell'
import { useI18n } from '../lib/i18n'
import { ShieldCheck, Copy, KeyRound, Check, Download, Trash2, History, LogOut } from 'lucide-react'
import { cn } from '../lib/utils'
import { QRCodeSVG } from 'qrcode.react'

export default function Security() {
  const { t } = useI18n()
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [setup, setSetup] = useState<any>(null) // {secret, url}
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [pwOld, setPwOld] = useState('')
  const [pwNew, setPwNew] = useState('')
  const [pwBusy, setPwBusy] = useState(false)
  const [logins, setLogins] = useState<any[]>([])
  const [sessions, setSessions] = useState<any[]>([])
  const [mustChange, setMustChange] = useState(false)

  async function load() {
    try { const s = await api.totp(); setEnabled(!!s.enabled) } catch { setEnabled(false) }
    api.me().then((m: any) => setMustChange(!!m.must_change_password)).catch(() => {})
    api.logins().then(setLogins).catch(() => {})
    api.sessions().then(setSessions).catch(() => {})
  }
  useEffect(() => { load() }, [])

  async function logoutAll() {
    if (!await confirmAsync(t('security.confirmLogoutAll'))) return
    try {
      const r: any = await api.logoutAll()
      if (r?.token) localStorage.setItem('token', r.token)
      toast(t('security.logoutAllOk'), { type: 'success' })
      load()
    } catch (e: any) { toast(e.message) }
  }
  async function revoke(jti: string) {
    try { await api.revokeSession(jti); load() } catch (e: any) { toast(e.message) }
  }

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
      const r: any = await api.totpDisable(code)
      if (r?.token) localStorage.setItem('token', r.token)
      setCode(''); await load()
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  async function changePw(e: any) {
    e.preventDefault()
    setPwBusy(true)
    try {
      const r: any = await api.changePassword(pwOld, pwNew)
      if (r?.token) localStorage.setItem('token', r.token)
      setPwOld(''); setPwNew('')
      toast(t('security.passwordChanged'), { type: 'success' })
    } catch (e: any) { toast(e.message, { type: 'error' }) } finally { setPwBusy(false) }
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
    if (!await confirmDestructive(t('security.deleteConfirm'))) return
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
        {mustChange && (
          <p className="text-sm rounded-md border border-warning/40 bg-warning/10 p-3">{t('security.mustChange')}</p>
        )}
        <Card className="p-4 space-y-3">
          <div className="flex items-center gap-2">
            <KeyRound size={16} />
            <b className="text-sm">{t('security.totp')}</b>
            {enabled === null ? null : enabled
              ? <Badge className="text-success">{t('common.enabled')}</Badge>
              : <Badge className="text-warning">{t('common.disabled')}</Badge>}
          </div>
          <p className="text-sm text-muted-foreground">{t('security.totpHint')}</p>

          {!enabled && !setup && (
            <Button size="sm" disabled={busy} onClick={beginSetup}><ShieldCheck />{t('security.enable')}</Button>
          )}

          {!enabled && setup && (
            <div className="space-y-3">
              <p className="text-sm">{t('security.scanHint')}</p>
              <div className="flex justify-center">
                <div className="rounded-lg bg-white p-3 shadow-xs">
                  <QRCodeSVG value={setup.url} size={168} level="M" marginSize={0} />
                </div>
              </div>
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
          <div className="flex items-center gap-2">
            <KeyRound size={16} />
            <b className="text-sm">{t('security.passwordTitle')}</b>
          </div>
          <form onSubmit={changePw} className="space-y-2">
            <Input type="password" autoComplete="current-password" placeholder={t('security.oldPassword')} value={pwOld} onChange={e => setPwOld(e.target.value)} required />
            <Input type="password" autoComplete="new-password" placeholder={t('security.newPassword')} value={pwNew} onChange={e => setPwNew(e.target.value)} required />
            <p className="text-xs text-muted-foreground">{t('security.passwordHint')}</p>
            <Button size="sm" disabled={pwBusy || pwNew.length < 8}>{t('security.changePassword')}</Button>
          </form>
        </Card>

        <Card className="p-4 space-y-3">
          <div className="flex items-center gap-2">
            <History size={16} />
            <b className="text-sm">{t('security.activeSessions')}</b>
          </div>
          {sessions.length === 0
            ? <p className="text-sm text-muted-foreground">{t('security.noLogins')}</p>
            : sessions.map((s: any) => (
              <div key={s.jti} className="flex items-center gap-2 text-xs">
                <span className="text-muted-foreground whitespace-nowrap">{new Date(s.created_at).toLocaleString()}</span>
                <span className="font-mono">{s.ip}</span>
                <span className="text-muted-foreground truncate flex-1">{s.user_agent}</span>
                <Button variant="ghost" size="sm" onClick={() => revoke(s.jti)}>{t('security.revoke')}</Button>
              </div>
            ))}
        </Card>

        <Card className="p-4 space-y-3">
          <div className="flex items-center gap-2">
            <History size={16} />
            <b className="text-sm">{t('security.sessions')}</b>
            <div className="flex-1" />
            <Button size="sm" variant="outline" onClick={logoutAll}><LogOut />{t('security.logoutAll')}</Button>
          </div>
          {logins.length === 0
            ? <p className="text-sm text-muted-foreground">{t('security.noLogins')}</p>
            : (
              <div className="text-sm divide-y divide-border">
                {logins.map((l: any) => (
                  <div key={l.id} className="flex items-center gap-2 py-1.5">
                    <span className={cn('size-2 rounded-full shrink-0', l.success ? 'bg-success' : 'bg-destructive')} />
                    <span className="text-xs text-muted-foreground whitespace-nowrap">{new Date(l.created_at).toLocaleString()}</span>
                    <span className="text-xs font-mono">{l.ip}</span>
                    <span className="text-xs text-muted-foreground truncate ml-auto max-w-[40%]">{l.user_agent}</span>
                  </div>
                ))}
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
