import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { toast } from '../lib/ui'
import {
  CheckCircle2, Circle, Loader2, ArrowRight, ArrowLeft, Globe, ShieldCheck,
  KeyRound, Cloud, Server, Send, PartyPopper, ExternalLink,
} from 'lucide-react'

const KEY = 'mailserver.onboarding.step'

function baseDomain(host: string) {
  const h = (host || '').trim().toLowerCase().replace(/\.$/, '')
  const p = h.split('.')
  return p.length > 2 ? p.slice(-2).join('.') : h
}

// 新手 3 步向导（#46）：域名/IP → DNS/证书/DKIM → 建账号 + 测试信。
export default function AdminWelcome() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [step, setStep] = useState(() => {
    const s = Number(localStorage.getItem(KEY) || 0)
    return s >= 1 && s <= 3 ? s : 1
  })
  const [settings, setSettings] = useState<any>(null)
  const [providers, setProviders] = useState<any[]>([])
  const [tls, setTls] = useState<any>(null)
  const [form, setForm] = useState({ mail_host: '', public_ip: '' })
  const [busy, setBusy] = useState('')
  const [checks, setChecks] = useState<any>(null)
  const [user, setUser] = useState({ email: '', name: '', password: '' })
  const [created, setCreated] = useState(false)
  const [testTo, setTestTo] = useState('')
  const [test, setTest] = useState<any>(null)
  const timer = useRef<any>(null)

  async function load() {
    try {
      const s = await api.settingsGet()
      setSettings(s)
      setForm(f => ({ mail_host: f.mail_host || s.mail_host || '', public_ip: f.public_ip || s.public_ip || '' }))
    } catch {}
    try { setProviders(await api.providers()) } catch {}
    try { setTls(await api.tlsGet()) } catch {}
  }
  useEffect(() => { load(); return () => timer.current && clearInterval(timer.current) }, [])
  useEffect(() => { localStorage.setItem(KEY, String(step)) }, [step])

  const domain = useMemo(() => baseDomain(form.mail_host || settings?.mail_host || ''), [form.mail_host, settings])
  const checkOk = !!checks && checks.mx_ok && checks.spf && checks.dkim && checks.dmarc

  async function saveStep1() {
    setBusy('save')
    try {
      const s = await api.settingsPatch({ mail_host: form.mail_host, public_ip: form.public_ip })
      setSettings(s); toast(t('onboarding.saved'))
      await check()
      setStep(2)
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  async function check() {
    const d = baseDomain(form.mail_host)
    if (!d) return
    setBusy('check')
    try { setChecks(await api.adminDomainCheck(d)) } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  async function genDKIM() {
    setBusy('dkim')
    try {
      await api.dkimGenerate({ domain, selector: settings?.dkim_selector || 'dkim' })
      toast(t('onboarding.saved')); await load()
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  async function createUser() {
    setBusy('user')
    try {
      await api.adminUserCreate(user)
      setCreated(true); toast(t('onboarding.saved'))
      setUser({ email: '', name: '', password: '' })
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  async function sendTest() {
    if (!testTo) return
    setBusy('test')
    try {
      const r: any = await api.send({ to: testTo, subject: 'Test mail / 测试邮件', body: '这是一封来自 Mailserver 的测试邮件，用于验证 SPF/DKIM/DMARC 与投递链路。', folder: 'sent' })
      setTest({ id: r.id, status: 'queued', to: testTo })
      poll(r.id)
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  function poll(id: number) {
    if (timer.current) clearInterval(timer.current)
    let n = 0
    timer.current = setInterval(async () => {
      n++
      try {
        const items: any[] = await api.outbox()
        const m = items.find(x => x.id === id)
        if (m) setTest({ id, to: m.to, status: m.status || (m.relayed ? 'sent' : 'queued'), err: m.relay_err })
        if ((m && (m.relayed || m.status === 'sent' || m.status === 'failed')) || n > 20) {
          clearInterval(timer.current); timer.current = null
        }
      } catch {}
    }, 3000)
  }
  function finish() {
    localStorage.removeItem(KEY)
    localStorage.setItem('mailserver.onboarded', '1')
    navigate('/admin')
  }

  const steps = [t('onboarding.step1'), t('onboarding.step2'), t('onboarding.step3')]

  return (
    <AdminShell title={t('onboarding.title')} desc={t('onboarding.desc')}>
      {/* 步骤条 */}
      <Card className="p-3">
        <div className="flex items-center gap-1">
          {steps.map((s, i) => {
            const n = i + 1
            const done = n < step
            const active = n === step
            return (
              <button key={n} onClick={() => n < step && setStep(n)} className="flex-1 min-w-0 flex items-center gap-1.5">
                <span className={`grid place-items-center size-6 shrink-0 rounded-full text-[11px] font-semibold ${done ? 'bg-success text-white' : active ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>
                  {done ? <CheckCircle2 size={14} /> : n}
                </span>
                <span className={`truncate text-xs ${active ? 'font-medium' : 'text-muted-foreground'}`}>{s}</span>
                {n < 3 && <span className="flex-1 h-px bg-border mx-0.5" />}
              </button>
            )
          })}
        </div>
        <div className="mt-2 h-1.5 rounded-full bg-muted overflow-hidden">
          <div className="h-full bg-primary transition-all" style={{ width: `${(step / 3) * 100}%` }} />
        </div>
      </Card>

      {step === 1 && (
        <Card className="p-4 space-y-3">
          <b className="text-sm flex items-center gap-1.5"><Globe size={15} />{t('onboarding.step1')}</b>
          <p className="text-xs text-muted-foreground">{t('onboarding.step1Hint')}</p>
          <label className="block text-xs space-y-1">
            <span className="text-muted-foreground">{t('settings.mailDomain')}</span>
            <Input placeholder="mail.example.com" value={form.mail_host} onChange={e => setForm({ ...form, mail_host: e.target.value })} />
          </label>
          <label className="block text-xs space-y-1">
            <span className="text-muted-foreground">{t('settings.publicIP')}</span>
            <Input placeholder="1.2.3.4" value={form.public_ip} onChange={e => setForm({ ...form, public_ip: e.target.value })} />
          </label>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={saveStep1} disabled={!!busy || !form.mail_host}>
              {busy === 'save' ? <Loader2 className="animate-spin" /> : <ArrowRight />}{t('onboarding.saveContinue')}
            </Button>
            <Button size="sm" variant="outline" onClick={check} disabled={!!busy || !form.mail_host}>
              {busy === 'check' ? <Loader2 className="animate-spin" /> : <ShieldCheck />}{t('onboarding.check')}
            </Button>
          </div>
          {checks && (
            <div className="text-sm space-y-1 pt-1">
              {[
                [checks.mx_ok, t('trust.mx')],
                [checks.spf, t('trust.spf')],
                [checks.dkim, t('trust.dkim')],
                [checks.dmarc, t('trust.dmarc')],
              ].map(([ok, label]: any, i) => (
                <div key={i} className="flex items-center gap-2">
                  {ok ? <CheckCircle2 size={15} className="text-success" /> : <Circle size={15} className="text-destructive" />}
                  <span className={ok ? 'text-muted-foreground' : ''}>{label}</span>
                </div>
              ))}
              <p className={`text-xs ${checkOk ? 'text-success' : 'text-warning'}`}>
                {checkOk ? t('onboarding.checkOk') : t('onboarding.checkBad', { n: [checks.mx_ok, checks.spf, checks.dkim, checks.dmarc].filter(x => !x).length })}
              </p>
            </div>
          )}
        </Card>
      )}

      {step === 2 && (
        <Card className="p-4 space-y-3">
          <b className="text-sm flex items-center gap-1.5"><Cloud size={15} />{t('onboarding.step2')}</b>
          <p className="text-xs text-muted-foreground">{t('onboarding.step2Hint')}</p>
          <Row ok={providers.length > 0} title={t('admin.providers')} desc={providers.length ? `${providers.length}` : t('onboarding.statusTodo')} icon={Cloud}>
            <Link to="/admin/providers"><Button size="sm" variant="outline">{t('onboarding.goProviders')}<ExternalLink size={12} /></Button></Link>
          </Row>
          <Row ok={!!tls?.cert?.exists} title={t('ssl.title')} desc={tls?.cert?.exists ? t('onboarding.statusDone') : t('onboarding.statusTodo')} icon={ShieldCheck}>
            <Link to="/admin/ssl"><Button size="sm" variant="outline">{t('onboarding.goSsl')}<ExternalLink size={12} /></Button></Link>
          </Row>
          <Row ok={!!settings?.dkim_ready} title={t('settings.dkim')} desc={settings?.dkim_ready ? t('onboarding.statusDone') : t('onboarding.statusWarn')} icon={KeyRound}>
            {!settings?.dkim_ready && (
              <Button size="sm" variant="outline" onClick={genDKIM} disabled={!!busy || !domain}>
                {busy === 'dkim' ? <Loader2 className="animate-spin" /> : <KeyRound />}{t('onboarding.dkimGenerate')}
              </Button>
            )}
          </Row>
          <div className="flex flex-wrap gap-2 pt-1">
            <Button size="sm" variant="outline" onClick={() => setStep(1)}><ArrowLeft />{t('onboarding.back')}</Button>
            <Button size="sm" variant="outline" onClick={load}>{t('common.refresh')}</Button>
            <Button size="sm" onClick={() => setStep(3)}>{t('onboarding.next')}<ArrowRight /></Button>
          </div>
        </Card>
      )}

      {step === 3 && (
        <div className="space-y-4">
          <Card className="p-4 space-y-3">
            <b className="text-sm flex items-center gap-1.5"><Server size={15} />{t('onboarding.createUser')}</b>
            <p className="text-xs text-muted-foreground">{t('onboarding.step3Hint')}</p>
            <div className="grid sm:grid-cols-3 gap-2">
              <Input placeholder={`you@${domain || 'example.com'}`} value={user.email} onChange={e => setUser({ ...user, email: e.target.value })} />
              <Input placeholder={t('users.name')} value={user.name} onChange={e => setUser({ ...user, name: e.target.value })} />
              <Input type="password" placeholder="••••••" value={user.password} onChange={e => setUser({ ...user, password: e.target.value })} />
            </div>
            <div className="flex items-center gap-2">
              <Button size="sm" onClick={createUser} disabled={!!busy || !user.email || user.password.length < 6}>
                {busy === 'user' ? <Loader2 className="animate-spin" /> : <Server />}{t('onboarding.createUser')}
              </Button>
              {created && <span className="text-xs text-success flex items-center gap-1"><CheckCircle2 size={13} />{t('onboarding.statusDone')}</span>}
            </div>
          </Card>

          <Card className="p-4 space-y-3">
            <b className="text-sm flex items-center gap-1.5"><Send size={15} />{t('onboarding.sendTest')}</b>
            <div className="flex flex-wrap items-center gap-2">
              <Input className="flex-1 min-w-[220px]" placeholder="you@gmail.com" value={testTo} onChange={e => setTestTo(e.target.value)} />
              <Button size="sm" onClick={sendTest} disabled={!!busy || !testTo || !testTo.includes('@')}>
                {busy === 'test' ? <Loader2 className="animate-spin" /> : <Send />}{t('onboarding.sendTest')}
              </Button>
            </div>
            {test && (
              <div className="text-sm rounded-md bg-muted/50 p-3 space-y-1">
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">{t('onboarding.testStatus')}:</span>
                  {test.status === 'sent' ? <Badge className="text-success">{t('onboarding.testOk')}</Badge>
                    : test.status === 'failed' ? <Badge className="text-destructive">{t('onboarding.testFail', { err: test.err || '-' })}</Badge>
                    : <Badge className="text-warning">{t('onboarding.testQueued')}</Badge>}
                </div>
                {test.status === 'failed' && (
                  <p className="text-xs text-muted-foreground">
                    {t('trust.desc')} <Link to="/admin/settings" className="underline">{t('settings.relay')}</Link>
                  </p>
                )}
              </div>
            )}
          </Card>

          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" variant="outline" onClick={() => setStep(2)}><ArrowLeft />{t('onboarding.back')}</Button>
            <Button size="sm" onClick={finish}><PartyPopper />{t('onboarding.done')}</Button>
            <Link to="/admin" className="text-xs text-muted-foreground underline flex items-center gap-1"><ArrowRight size={12} />{t('onboarding.later')}</Link>
          </div>
        </div>
      )}
    </AdminShell>
  )
}

function Row({ ok, title, desc, icon: Icon, children }: any) {
  return (
    <div className="flex items-center gap-3 rounded-md border border-border p-3">
      <div className={`grid place-items-center size-9 rounded-full ${ok ? 'bg-success/10 text-success' : 'bg-muted text-muted-foreground'}`}>
        {ok ? <CheckCircle2 size={18} /> : <Icon size={18} />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{title}</div>
        <div className="text-xs text-muted-foreground">{desc}</div>
      </div>
      {children}
    </div>
  )
}
