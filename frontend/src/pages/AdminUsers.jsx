import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Button, Input, Card, Badge } from '../components/ui/controls'
import AdminShell from '../components/AdminShell'
import { useI18n } from '../lib/i18n'
import { Plus, Trash2, KeyRound, Loader2, AlertCircle, CheckCircle2, UserCog } from 'lucide-react'

export default function AdminUsers() {
  const { t } = useI18n()
  const [users, setUsers] = useState([])
  const [form, setForm] = useState({ email: '', name: '', password: '' })
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')

  async function load() {
    try { setUsers(await api.adminUsers()) } catch (e) { setMsg(e.message) }
  }
  useEffect(() => { load() }, [])

  async function create(e) {
    e.preventDefault()
    setBusy('create'); setMsg('')
    try {
      await api.adminUserCreate(form)
      setForm({ email: '', name: '', password: '' })
      setMsg(t('users.create'))
      load()
    } catch (e) { setMsg(e.message) } finally { setBusy('') }
  }

  async function resetPass(u) {
    const pw = prompt(`${t('users.changePass')}: ${u.email} (≥6)`)
    if (!pw) return
    try { await api.adminUserPatch(u.id, { password: pw }); setMsg(t('users.changePass')) } catch (e) { setMsg(e.message) }
  }

  async function toggle(u, key) {
    try { await api.adminUserPatch(u.id, { [key]: !u[key] }); load() } catch (e) { setMsg(e.message) }
  }

  async function del(u) {
    if (!confirm(`${t('common.delete')} ${u.email}?`)) return
    try { await api.adminUserDelete(u.id); setMsg(t('common.delete')); load() } catch (e) { setMsg(e.message) }
  }

  return (
    <AdminShell title={t('users.title')} desc={t('users.desc')}>
      <Card className="p-4">
        <div className="flex items-center gap-2 mb-3"><UserCog size={16} /><b className="text-sm">{t('users.create')}</b></div>
        <form onSubmit={create} className="grid sm:grid-cols-4 gap-3 items-end">
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.email')}</span>
            <Input type="email" placeholder="user@example.com" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} required />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.name')}</span>
            <Input placeholder="Zhang San" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
          </label>
          <label className="text-sm space-y-1">
            <span className="font-medium">{t('users.password')}</span>
            <Input type="text" placeholder="≥6" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} required />
          </label>
          <Button disabled={busy === 'create'}>
            {busy === 'create' ? <Loader2 className="animate-spin" /> : <Plus />}{t('common.create')}
          </Button>
        </form>
        <p className="text-xs text-muted-foreground mt-2">{t('users.hostHint')}</p>
      </Card>

      {msg && <p className="text-sm flex items-center gap-1 text-muted-foreground"><AlertCircle size={14} />{msg}</p>}

      <Card className="p-4 overflow-x-auto">
        <b className="text-sm">{t('users.all', { n: users.length })}</b>
        <table className="w-full text-sm mt-3 min-w-[560px]">
          <thead><tr className="text-left text-xs text-muted-foreground">
            <th className="py-1">{t('users.email')}</th><th>{t('users.name')}</th><th>{t('users.mails')}</th><th>{t('users.status')}</th><th className="text-right">{t('users.actions')}</th>
          </tr></thead>
          <tbody>
            {users.map(u => (
              <tr key={u.id} className="border-t border-border">
                <td className="py-2 pr-2">{u.email} {u.admin && <Badge>{t('users.adminBadge')}</Badge>}</td>
                <td className="pr-2">{u.name}</td>
                <td className="pr-2">{u.mail_count}</td>
                <td className="pr-2">{u.disabled ? <span className="text-red-500 text-xs">{t('users.disabled')}</span> : <span className="text-green-600 text-xs">{t('users.normal')}</span>}</td>
                <td className="text-right whitespace-nowrap">
                  <Button variant="ghost" size="sm" onClick={() => resetPass(u)}><KeyRound />{t('users.changePass')}</Button>
                  <Button variant="ghost" size="sm" onClick={() => toggle(u, 'admin')}>{u.admin ? t('users.unsetAdmin') : t('users.setAdmin')}</Button>
                  <Button variant="ghost" size="sm" onClick={() => toggle(u, 'disabled')}>{u.disabled ? t('users.enable') : t('users.disable')}</Button>
                  <Button variant="ghost" size="icon" onClick={() => del(u)} aria-label={t('common.delete')}><Trash2 /></Button>
                </td>
              </tr>
            ))}
            {users.length === 0 && <tr><td colSpan={5} className="py-4 text-center text-muted-foreground">{t('users.none')}</td></tr>}
          </tbody>
        </table>
      </Card>
    </AdminShell>
  )
}
