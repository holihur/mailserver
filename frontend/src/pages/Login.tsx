import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Card } from '../components/ui/controls'
import { LangToggle, ThemeToggle } from '../components/HeaderControls'
import { useI18n } from '../lib/i18n'
import { Mail, Loader2 } from 'lucide-react'
import { BRAND } from '../lib/brand'

export default function Login() {
  const { t } = useI18n()
  const [mode, setMode] = useState('login')
  const [form, setForm] = useState({ email: '', name: '', password: '' })
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [regOpen, setRegOpen] = useState(false)
  const [challenge, setChallenge] = useState<string | null>(null)
  const [code, setCode] = useState('')

  useEffect(() => { api.site().then((s: any) => setRegOpen(!!s.registration)).catch(() => {}) }, [])
  useEffect(() => { if (!regOpen && mode === 'register') setMode('login') }, [regOpen, mode])

  async function submit(e) {
    e.preventDefault()
    setErr(''); setLoading(true)
    try {
      if (mode === 'register') {
        const d = await api.register({ email: form.email, name: form.name || form.email.split('@')[0], password: form.password })
        localStorage.setItem('token', d.token)
        window.location.assign('/')
        return
      }
      const d = await api.login({ email: form.email, password: form.password })
      // 启用了两步验证：先拿挑战令牌，再输入动态码
      if (d.totp_required) { setChallenge(d.challenge); return }
      localStorage.setItem('token', d.token)
      // 整页跳转最稳：避免 SPA 路由/缓存导致登录后不跳转
      window.location.assign('/')
    } catch (e2) { setErr(e2.message) }
    finally { setLoading(false) }
  }

  async function submitCode(e) {
    e.preventDefault()
    setErr(''); setLoading(true)
    try {
      const d = await api.loginTOTP({ challenge, code })
      localStorage.setItem('token', d.token)
      window.location.assign('/')
    } catch (e2) { setErr(e2.message) }
    finally { setLoading(false) }
  }

  return (
    <div className="relative min-h-screen grid place-items-center bg-muted/40 p-4">
      <div className="absolute top-3 right-3 flex items-center gap-1">
        <LangToggle />
        <ThemeToggle />
      </div>
      <Card className="w-full max-w-sm p-6 space-y-4">
        <div className="flex items-center gap-2">
          <span className="grid place-items-center size-9 rounded-md bg-primary text-primary-foreground"><Mail size={18} /></span>
          <div><h1 className="font-semibold">{BRAND}</h1><p className="text-xs text-muted-foreground">{t('login.title')}</p></div>
        </div>
        {challenge ? (
          <form onSubmit={submitCode} className="space-y-3">
            <p className="text-sm text-muted-foreground">{t('login.totpHint')}</p>
            <Input autoFocus inputMode="numeric" maxLength={6} className="tracking-widest text-center"
              placeholder="000000" value={code} onChange={e => setCode(e.target.value)} required />
            {err && <p className="text-sm text-red-500" role="alert">{err}</p>}
            <Button className="w-full" disabled={loading}>
              {loading ? <><Loader2 className="animate-spin" />{t('login.pleaseWait')}</> : t('login.verify')}
            </Button>
            <button type="button" className="text-xs text-muted-foreground underline"
              onClick={() => { setChallenge(null); setCode(''); setErr('') }}>{t('login.back')}</button>
          </form>
        ) : (
          <>
            <div className="flex gap-2 text-sm">
              <button onClick={() => setMode('login')} className={mode === 'login' ? 'font-semibold text-primary' : 'text-muted-foreground'}>{t('login.login')}</button>
              {regOpen && <>
                <span className="text-muted-foreground">/</span>
                <button onClick={() => setMode('register')} className={mode === 'register' ? 'font-semibold text-primary' : 'text-muted-foreground'}>{t('login.register')}</button>
              </>}
            </div>
            <form onSubmit={submit} className="space-y-3">
              <Input type="text" inputMode="email" autoCapitalize="none" autoCorrect="off" autoComplete="username"
                placeholder="you@example.com · admin" value={form.email}
                onChange={e => setForm({ ...form, email: e.target.value })} required />
              {mode === 'register' && <Input autoComplete="nickname" placeholder={t('login.nickname')} value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />}
              <Input type="password" autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                placeholder={t('login.password')} value={form.password}
                onChange={e => setForm({ ...form, password: e.target.value })} required />
              {err && <p className="text-sm text-red-500" role="alert">{err}</p>}
              <Button className="w-full" disabled={loading}>
                {loading ? <><Loader2 className="animate-spin" />{t('login.pleaseWait')}</> : mode === 'login' ? t('login.loginBtn') : t('login.registerBtn')}
              </Button>
            </form>
          </>
        )}
        <p className="text-xs text-muted-foreground">{regOpen ? t('login.hint') : t('login.registerClosed')}</p>
        <p className="text-xs text-center"><Link to="/privacy" className="text-muted-foreground underline">{t('nav.privacy')}</Link></p>
      </Card>
    </div>
  )
}
