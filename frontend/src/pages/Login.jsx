import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { api } from '../api/client'
import { Button, Input, Card } from '../components/ui/controls'
import { Mail } from 'lucide-react'

export default function Login() {
  const nav = useNavigate()
  const [mode, setMode] = useState('login')
  const [form, setForm] = useState({ email: '', name: '', password: '' })
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)

  async function submit(e) {
    e.preventDefault()
    setErr(''); setLoading(true)
    try {
      const fn = mode === 'login' ? api.login : api.register
      const d = await fn({ email: form.email, name: form.name || form.email.split('@')[0], password: form.password })
      localStorage.setItem('token', d.token)
      nav('/')
    } catch (e2) { setErr(e2.message) }
    finally { setLoading(false) }
  }

  return (
    <div className="min-h-screen grid place-items-center bg-muted/40 p-4">
      <Card className="w-full max-w-sm p-6 space-y-4">
        <div className="flex items-center gap-2">
          <span className="grid place-items-center size-9 rounded-md bg-primary text-primary-foreground"><Mail size={18} /></span>
          <div><h1 className="font-semibold">Mailserver</h1><p className="text-xs text-muted-foreground">轻量 webmail</p></div>
        </div>
        <div className="flex gap-2 text-sm">
          <button onClick={() => setMode('login')} className={mode === 'login' ? 'font-semibold text-primary' : 'text-muted-foreground'}>登录</button>
          <span className="text-muted-foreground">/</span>
          <button onClick={() => setMode('register')} className={mode === 'register' ? 'font-semibold text-primary' : 'text-muted-foreground'}>注册</button>
        </div>
        <form onSubmit={submit} className="space-y-3">
          <Input placeholder="you@example.com" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} required />
          {mode === 'register' && <Input placeholder="昵称" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />}
          <Input type="password" placeholder="密码 ≥6位" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} required />
          {err && <p className="text-sm text-red-500">{err}</p>}
          <Button className="w-full" disabled={loading}>{loading ? '请稍候…' : mode === 'login' ? '登录' : '注册并登录'}</Button>
        </form>
        <p className="text-xs text-muted-foreground">默认后端 <code>/api</code>，Vite 已代理到 :8080。</p>
      </Card>
    </div>
  )
}
