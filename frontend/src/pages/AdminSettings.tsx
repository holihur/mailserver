import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { Save, Wand2, Upload, KeyRound, Server, Send, ShieldAlert, CheckCircle2, Loader2, UserPlus, PlugZap } from 'lucide-react'

export default function AdminSettings() {
  const { t } = useI18n()
  const [s, setS] = useState(null)
  const [relayPass, setRelayPass] = useState('')
  const [oidcSecret, setOidcSecret] = useState('')
  const [dkim, setDkim] = useState(null)
  const [busy, setBusy] = useState('')
  const [msg, setMsg] = useState('')
  const [relayMsg, setRelayMsg] = useState('')

  async function load() {
    try { setS(await api.settingsGet()) } catch (e) { setMsg(e.message) }
    try { setDkim(await api.dkimGet()) } catch {}
  }
  useEffect(() => { load() }, [])

  function field(k, v) { setS(prev => ({ ...prev, [k]: v })) }

  async function save() {
    setBusy('save'); setMsg('')
    try {
      const body: Record<string, any> = {
        mail_host: s.mail_host || '', public_ip: s.public_ip || '', admin_emails: s.admin_emails || '',
        relay_host: s.relay_host || '', relay_port: s.relay_port || '',
        relay_user: s.relay_user || '', relay_from: s.relay_from || '',
        registration_enabled: s.registration_enabled ? '1' : '0',
        relay_insecure: s.relay_insecure ? '1' : '0',
        direct_send: s.direct_send ? '1' : '0',
        oidc_enabled: s.oidc_enabled ? '1' : '0',
        oidc_issuer: s.oidc_issuer || '',
        oidc_client_id: s.oidc_client_id || '',
        oidc_auto_create: s.oidc_auto_create ? '1' : '0',
      }
      if (relayPass) body.relay_pass = relayPass
      if (oidcSecret) body.oidc_client_secret = oidcSecret
      const out = await api.settingsPatch(body)
      setS(out); setRelayPass(''); setOidcSecret('')
      setMsg(t('settings.saved'))
    } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  async function detectIP() {
    try {
      const r = await fetch('https://api.ipify.org')
      const ip = (await r.text()).trim()
      if (/^\d+\.\d+\.\d+\.\d+$/.test(ip)) field('public_ip', ip)
    } catch { setMsg('IP detection failed, please fill manually') }
  }

  async function testRelay() {
    setBusy('relay'); setRelayMsg('')
    try {
      const r: any = await api.relayTest()
      const steps = (r.steps || []).join(' → ')
      setRelayMsg((r.ok ? '✓ ' : '✗ ') + steps + (r.ok ? '' : ` (${r.error || ''})`) + (r.hint ? ` — ${r.hint}` : ''))
    } catch (e: any) { setRelayMsg(e.message) } finally { setBusy('') }
  }

  async function genDKIM() {
    setBusy('dkim'); setMsg('')
    try {
      const r = await api.dkimGenerate({ domain: s?.dkim_domain || '', selector: s?.dkim_selector || 'dkim' })
      setMsg(`${r.name}`)
      await load()
    } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  return (
    <AdminShell title={t('settings.title')} desc={t('settings.desc')}>
      {!s ? <p className="text-sm text-muted-foreground">{t('common.loading')}</p> : (
        <>
          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><Server size={16} /><b className="text-sm">{t('settings.mailDomain')}</b></div>
            <Field label={t('settings.mailDomain')} hint={t('settings.mailDomainHint')}>
              <Input placeholder="mail.example.com" value={s.mail_host || ''} onChange={e => field('mail_host', e.target.value)} />
            </Field>
            <Field label={t('settings.publicIP')} hint={t('settings.publicIPHint')}>
              <div className="flex gap-2">
                <Input placeholder="1.2.3.4" value={s.public_ip || ''} onChange={e => field('public_ip', e.target.value)} />
                <Button type="button" variant="outline" onClick={detectIP}>{t('settings.autoDetect')}</Button>
              </div>
            </Field>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><Send size={16} /><b className="text-sm">{t('settings.relay')}</b></div>
            <p className="text-xs text-muted-foreground">{t('settings.relayHint')}</p>
            <div className="grid sm:grid-cols-2 gap-3">
              <Field label={t('settings.relayHost')}><Input placeholder="smtp.example.com" value={s.relay_host || ''} onChange={e => field('relay_host', e.target.value)} /></Field>
              <Field label={t('settings.relayPort')}><Input placeholder="587" value={s.relay_port || ''} onChange={e => field('relay_port', e.target.value)} /></Field>
              <Field label={t('settings.relayUser')}><Input placeholder="user" value={s.relay_user || ''} onChange={e => field('relay_user', e.target.value)} /></Field>
              <Field label={t('settings.relayPass')}>
                <Input type="password" placeholder={s.relay_pass_set ? t('settings.relayPassKeep') : ''} value={relayPass} onChange={e => setRelayPass(e.target.value)} />
              </Field>
            </div>
            <Field label={t('settings.relayFrom')}><Input placeholder="noreply@example.com" value={s.relay_from || ''} onChange={e => field('relay_from', e.target.value)} /></Field>
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" variant="outline" type="button" onClick={testRelay} disabled={busy === 'relay'}>
                {busy === 'relay' ? <Loader2 className="animate-spin" /> : <PlugZap />}{t('settings.testRelay')}
              </Button>
              {relayMsg && <span className="text-xs text-muted-foreground break-all">{relayMsg}</span>}
            </div>
            <label className="flex items-start gap-2 text-xs text-muted-foreground">
              <input type="checkbox" className="mt-0.5" checked={!!s.relay_insecure}
                onChange={e => field('relay_insecure', e.target.checked ? '1' : '0')} />
              <span>{t('settings.relayInsecureHint')}</span>
            </label>
            <label className="flex items-start gap-2 text-xs text-muted-foreground">
              <input type="checkbox" className="mt-0.5" checked={!!s.direct_send}
                onChange={e => field('direct_send', e.target.checked ? '1' : '0')} />
              <span>{t('settings.directSendHint')}</span>
            </label>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><KeyRound size={16} /><b className="text-sm">{t('settings.oidc')}</b></div>
            <p className="text-xs text-muted-foreground">{t('settings.oidcHint')}</p>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={!!s.oidc_enabled} onChange={e => field('oidc_enabled', e.target.checked ? '1' : '0')} />
              {t('settings.oidcEnable')}
            </label>
            <Field label={t('settings.oidcIssuer')}><Input placeholder="https://accounts.google.com" value={s.oidc_issuer || ''} onChange={e => field('oidc_issuer', e.target.value)} /></Field>
            <div className="grid sm:grid-cols-2 gap-3">
              <Field label={t('settings.oidcClientId')}><Input value={s.oidc_client_id || ''} onChange={e => field('oidc_client_id', e.target.value)} /></Field>
              <Field label={t('settings.oidcClientSecret')}><Input type="password" placeholder={s.oidc_client_secret_set ? t('settings.relayPassKeep') : ''} value={oidcSecret} onChange={e => setOidcSecret(e.target.value)} /></Field>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={!!s.oidc_auto_create} onChange={e => field('oidc_auto_create', e.target.checked ? '1' : '0')} />
              {t('settings.oidcAutoCreate')}
            </label>
            <p className="text-xs text-muted-foreground break-all">{t('settings.oidcRedirect', { url: location.origin + '/api/oidc/callback' })}</p>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><KeyRound size={16} /><b className="text-sm">{t('settings.dkim')}</b>
              {s.dkim_ready ? <Badge className="text-success">{t('common.enabled')}</Badge> : <Badge className="text-warning">{t('common.disabled')}</Badge>}
            </div>
            <p className="text-xs text-muted-foreground">{t('settings.dkimHint')}</p>
            {s.dkim_ready && dkim?.ready && (
              <div className="text-xs bg-muted/50 rounded-md p-3 space-y-1">
                <div>selector: <code>{dkim.selector}</code> · domain: <code>{dkim.domain}</code></div>
                <div className="font-mono break-all">TXT: {dkim.name}.{dkim.domain} → "{dkim.txt}"</div>
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={genDKIM} disabled={busy === 'dkim'}>
                {busy === 'dkim' ? <Loader2 className="animate-spin" /> : <Wand2 />}{t('settings.dkimGenerate')}
              </Button>
              <UploadDKIM onDone={load} onMsg={setMsg} />
            </div>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><ShieldAlert size={16} /><b className="text-sm">{t('settings.admins')}</b></div>
            <Field label={t('settings.admins')} hint={t('settings.adminsHint')}>
              <Input placeholder="admin@example.com" value={s.admin_emails || ''} onChange={e => field('admin_emails', e.target.value)} />
            </Field>
          </Card>

          <Card className="p-4 space-y-3">
            <div className="flex items-center gap-2"><UserPlus size={16} /><b className="text-sm">{t('settings.registration')}</b></div>
            <label className="flex items-start gap-2 text-sm">
              <input type="checkbox" className="mt-0.5" checked={!!s.registration_enabled}
                onChange={e => field('registration_enabled', e.target.checked ? '1' : '0')} />
              <span className="text-muted-foreground">{t('settings.registrationHint')}</span>
            </label>
          </Card>

          {msg && <p className="text-sm flex items-center gap-1 text-muted-foreground"><CheckCircle2 size={14} />{msg}</p>}

          <div className="flex justify-end">
            <Button onClick={save} disabled={busy === 'save'}>
              {busy === 'save' ? <Loader2 className="animate-spin" /> : <Save />}{t('common.save')}
            </Button>
          </div>
        </>
      )}
    </AdminShell>
  )
}

function Field({ label, hint, children }: { label: any; hint?: any; children: any }) {
  return (
    <label className="block space-y-1">
      <span className="text-sm font-medium">{label}</span>
      {children}
      {hint && <span className="block text-xs text-muted-foreground">{hint}</span>}
    </label>
  )
}

function UploadDKIM({ onDone, onMsg }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ domain: '', selector: 'dkim', private_key: '' })
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    try { await api.dkimUpload(form); onMsg(t('settings.dkimImport')); setOpen(false); onDone() }
    catch (e) { onMsg(e.message) } finally { setBusy(false) }
  }
  if (!open) return <Button size="sm" variant="outline" onClick={() => setOpen(true)}><Upload />{t('settings.dkimImport')}</Button>
  return (
    <div className="w-full space-y-2 border border-border rounded-md p-3">
      <div className="grid sm:grid-cols-2 gap-2">
        <Input placeholder="example.com" value={form.domain} onChange={e => setForm({ ...form, domain: e.target.value })} />
        <Input placeholder="dkim" value={form.selector} onChange={e => setForm({ ...form, selector: e.target.value })} />
      </div>
      <textarea className="w-full h-28 rounded-md border border-border bg-background p-2 text-xs font-mono"
        placeholder="-----BEGIN RSA PRIVATE KEY-----" value={form.private_key} onChange={e => setForm({ ...form, private_key: e.target.value })} />
      <div className="flex gap-2">
        <Button size="sm" onClick={submit} disabled={busy}>{busy ? <Loader2 className="animate-spin" /> : <Save />}{t('common.save')}</Button>
        <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
      </div>
    </div>
  )
}
